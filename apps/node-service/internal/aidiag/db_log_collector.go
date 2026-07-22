package aidiag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// DBLogCollector - 真实日志/指标采集器
//
// 数据源（均由 node-agent 心跳上报沉淀到 DB）：
//   1. channel_health_current      - 当前通道状态（rtt/fail_count/online_users/failover）
//   2. channel_health_snapshots     - 时间窗口内的通道健康快照序列
//   3. channel_failover_events      - 通道降级事件（含 reason/detail）
//   4. node_doctor_reports          - 节点体检报告（含每项 check 结果）
//
// 不依赖 node-agent 实时 gRPC 流，因为：
//   - DB 中已有 agent 上报沉淀的历史数据
//   - 即便 agent 离线，仍可用历史数据做诊断
//   - 避免 LLM 调用阻塞在 gRPC 流上
// ============================================================================

type DBLogCollector struct {
	pool *pgxpool.Pool
}

func NewDBLogCollector(pool *pgxpool.Pool) *DBLogCollector {
	return &DBLogCollector{pool: pool}
}

// CollectLogs 聚合诊断时间窗口内的所有可观测数据为结构化文本，供 LLM 分析
func (c *DBLogCollector) CollectLogs(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("=== 诊断时间窗口: %s ~ %s ===\n", start.Format(time.RFC3339), end.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("服务器ID: %s\n", serverID))
	if nodeID != nil {
		b.WriteString(fmt.Sprintf("节点ID: %s\n", *nodeID))
	}
	b.WriteString("\n")

	if err := c.appendChannelCurrent(ctx, &b, serverID); err != nil {
		b.WriteString(fmt.Sprintf("[警告] 获取 channel_health_current 失败: %v\n\n", err))
	}

	if err := c.appendSnapshots(ctx, &b, serverID, start, end); err != nil {
		b.WriteString(fmt.Sprintf("[警告] 获取 channel_health_snapshots 失败: %v\n\n", err))
	}

	if err := c.appendFailoverEvents(ctx, &b, serverID, start, end); err != nil {
		b.WriteString(fmt.Sprintf("[警告] 获取 channel_failover_events 失败: %v\n\n", err))
	}

	if nodeID != nil {
		if err := c.appendDoctorReports(ctx, &b, *nodeID, start, end); err != nil {
			b.WriteString(fmt.Sprintf("[警告] 获取 node_doctor_reports 失败: %v\n\n", err))
		}
	}

	return b.String(), nil
}

// CollectMetrics 返回结构化指标 map（供 LLM 做数值分析）
func (c *DBLogCollector) CollectMetrics(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (map[string]interface{}, error) {
	metrics := map[string]interface{}{
		"server_id":   serverID.String(),
		"time_window": fmt.Sprintf("%s ~ %s", start.Format(time.RFC3339), end.Format(time.RFC3339)),
	}
	if nodeID != nil {
		metrics["node_id"] = nodeID.String()
	}

	// 1. 当前通道状态
	metrics["channel_current"] = c.fetchChannelCurrentMetrics(ctx, serverID)

	// 2. 时间窗口统计
	var snapCount, failoverCount int
	_ = c.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM channel_health_snapshots
		WHERE server_id = $1 AND reported_at BETWEEN $2 AND $3
	`, serverID, start, end).Scan(&snapCount)
	_ = c.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM channel_failover_events
		WHERE server_id = $1 AND occurred_at BETWEEN $2 AND $3
	`, serverID, start, end).Scan(&failoverCount)
	metrics["snapshots_in_window"] = snapCount
	metrics["failover_events_in_window"] = failoverCount

	// 3. 节点级 doctor 报告（如果指定了 nodeID）
	if nodeID != nil {
		if dr := c.fetchLatestDoctorMetrics(ctx, *nodeID, start, end); dr != nil {
			metrics["latest_doctor_report"] = dr
		}
	}

	return metrics, nil
}

// ============================================================================
// 内部辅助函数 - CollectLogs 的各段
// ============================================================================

func (c *DBLogCollector) appendChannelCurrent(ctx context.Context, b *strings.Builder, serverID uuid.UUID) error {
	var (
		activeChannel, channelState         string
		rttMs, failCount1h, onlineUsers     int
		failover1h, failover24h             int
		lastError                           *string
		lastFailoverAt                      *time.Time
		lastFailoverFrom, lastFailoverTo     *string
		lastFailoverReason                  *string
		updatedAt                           time.Time
	)
	err := c.pool.QueryRow(ctx, `
		SELECT active_channel, channel_state, COALESCE(rtt_ms, 0), fail_count_1h, online_users,
		       failover_count_1h, failover_count_24h, last_error, last_failover_at,
		       last_failover_from, last_failover_to, last_failover_reason, updated_at
		FROM channel_health_current WHERE server_id = $1
	`, serverID).Scan(
		&activeChannel, &channelState, &rttMs, &failCount1h, &onlineUsers,
		&failover1h, &failover24h, &lastError, &lastFailoverAt,
		&lastFailoverFrom, &lastFailoverTo, &lastFailoverReason, &updatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			b.WriteString("--- 当前通道状态 ---\n(无数据：agent 未连接或未上报心跳)\n\n")
			return nil
		}
		return err
	}
	b.WriteString("--- 当前通道状态 ---\n")
	b.WriteString(fmt.Sprintf("  active_channel:     %s\n", activeChannel))
	b.WriteString(fmt.Sprintf("  channel_state:      %s\n", channelState))
	b.WriteString(fmt.Sprintf("  rtt_ms:             %d\n", rttMs))
	b.WriteString(fmt.Sprintf("  fail_count_1h:      %d\n", failCount1h))
	b.WriteString(fmt.Sprintf("  online_users:       %d\n", onlineUsers))
	b.WriteString(fmt.Sprintf("  failover_count_1h:  %d\n", failover1h))
	b.WriteString(fmt.Sprintf("  failover_count_24h: %d\n", failover24h))
	if lastError != nil && *lastError != "" {
		b.WriteString(fmt.Sprintf("  last_error:         %s\n", *lastError))
	}
	if lastFailoverAt != nil {
		b.WriteString(fmt.Sprintf("  last_failover_at:   %s\n", lastFailoverAt.Format(time.RFC3339)))
	}
	if lastFailoverFrom != nil && lastFailoverTo != nil {
		b.WriteString(fmt.Sprintf("  last_failover:      %s -> %s\n", *lastFailoverFrom, *lastFailoverTo))
	}
	if lastFailoverReason != nil && *lastFailoverReason != "" {
		b.WriteString(fmt.Sprintf("  failover_reason:    %s\n", *lastFailoverReason))
	}
	b.WriteString(fmt.Sprintf("  updated_at:         %s\n\n", updatedAt.Format(time.RFC3339)))
	return nil
}

func (c *DBLogCollector) appendSnapshots(ctx context.Context, b *strings.Builder, serverID uuid.UUID, start, end time.Time) error {
	rows, err := c.pool.Query(ctx, `
		SELECT active_channel, channel_state, COALESCE(rtt_ms, 0), fail_count_1h, online_users, last_error, reported_at
		FROM channel_health_snapshots
		WHERE server_id = $1 AND reported_at BETWEEN $2 AND $3
		ORDER BY reported_at DESC LIMIT 50
	`, serverID, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.WriteString("--- 通道健康快照（时间窗口内，最新 50 条）---\n")
	count := 0
	for rows.Next() {
		var (
			activeChannel, channelState string
			rttMs, failCount1h, onlineUsers int
			lastError *string
			reportedAt time.Time
		)
		if err := rows.Scan(&activeChannel, &channelState, &rttMs, &failCount1h, &onlineUsers, &lastError, &reportedAt); err != nil {
			return err
		}
		count++
		b.WriteString(fmt.Sprintf("  [%s] ch=%s state=%s rtt=%dms fails=%d users=%d",
			reportedAt.Format(time.RFC3339), activeChannel, channelState, rttMs, failCount1h, onlineUsers))
		if lastError != nil && *lastError != "" {
			b.WriteString(fmt.Sprintf(" err=%q", *lastError))
		}
		b.WriteString("\n")
	}
	if count == 0 {
		b.WriteString("  (无快照)\n")
	}
	b.WriteString("\n")
	return rows.Err()
}

func (c *DBLogCollector) appendFailoverEvents(ctx context.Context, b *strings.Builder, serverID uuid.UUID, start, end time.Time) error {
	rows, err := c.pool.Query(ctx, `
		SELECT from_channel, to_channel, reason, COALESCE(detail, ''), occurred_at
		FROM channel_failover_events
		WHERE server_id = $1 AND occurred_at BETWEEN $2 AND $3
		ORDER BY occurred_at DESC LIMIT 20
	`, serverID, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.WriteString("--- 通道降级事件（时间窗口内，最新 20 条）---\n")
	count := 0
	for rows.Next() {
		var from, to, reason, detail string
		var occurredAt time.Time
		if err := rows.Scan(&from, &to, &reason, &detail, &occurredAt); err != nil {
			return err
		}
		count++
		b.WriteString(fmt.Sprintf("  [%s] %s -> %s reason=%s", occurredAt.Format(time.RFC3339), from, to, reason))
		if detail != "" {
			b.WriteString(fmt.Sprintf(" detail=%q", detail))
		}
		b.WriteString("\n")
	}
	if count == 0 {
		b.WriteString("  (无降级事件)\n")
	}
	b.WriteString("\n")
	return rows.Err()
}

func (c *DBLogCollector) appendDoctorReports(ctx context.Context, b *strings.Builder, nodeID uuid.UUID, start, end time.Time) error {
	rows, err := c.pool.Query(ctx, `
		SELECT report_type, overall_status, checks, summary_ok, summary_warn, summary_fail, duration_ms, created_at
		FROM node_doctor_reports
		WHERE node_id = $1 AND created_at BETWEEN $2 AND $3
		ORDER BY created_at DESC LIMIT 5
	`, nodeID, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.WriteString("--- 节点体检报告（时间窗口内，最新 5 条）---\n")
	count := 0
	for rows.Next() {
		var reportType, overallStatus string
		var checksJSON []byte
		var ok, warn, fail int
		var durationMs *int
		var createdAt time.Time
		if err := rows.Scan(&reportType, &overallStatus, &checksJSON, &ok, &warn, &fail, &durationMs, &createdAt); err != nil {
			return err
		}
		count++
		b.WriteString(fmt.Sprintf("  [%s] type=%s overall=%s ok=%d warn=%d fail=%d",
			createdAt.Format(time.RFC3339), reportType, overallStatus, ok, warn, fail))
		if durationMs != nil {
			b.WriteString(fmt.Sprintf(" duration=%dms", *durationMs))
		}
		b.WriteString("\n")

		// 解析 checks JSONB（每项有 check_code/check_name/category/severity/status/message/fix_status/details）
		var checks []map[string]interface{}
		if err := json.Unmarshal(checksJSON, &checks); err == nil {
			for _, ck := range checks {
				checkCode, _ := ck["check_code"].(string)
				checkName, _ := ck["check_name"].(string)
				status, _ := ck["status"].(string)
				sev, _ := ck["severity"].(string)
				msg, _ := ck["message"].(string)
				b.WriteString(fmt.Sprintf("    - %s [%s] (%s/%s): %s\n", checkCode, status, checkName, sev, msg))
			}
		}
	}
	if count == 0 {
		b.WriteString("  (无体检报告)\n")
	}
	b.WriteString("\n")
	return rows.Err()
}

// ============================================================================
// 内部辅助函数 - CollectMetrics 的各段
// ============================================================================

func (c *DBLogCollector) fetchChannelCurrentMetrics(ctx context.Context, serverID uuid.UUID) map[string]interface{} {
	var (
		activeChannel, channelState     string
		rttMs, failCount1h, onlineUsers int
		failover1h, failover24h         int
		lastError                       *string
		lastFailoverAt                  *time.Time
		lastFailoverReason              *string
		updatedAt                       time.Time
	)
	err := c.pool.QueryRow(ctx, `
		SELECT active_channel, channel_state, COALESCE(rtt_ms, 0), fail_count_1h, online_users,
		       failover_count_1h, failover_count_24h, last_error, last_failover_at,
		       last_failover_reason, updated_at
		FROM channel_health_current WHERE server_id = $1
	`, serverID).Scan(
		&activeChannel, &channelState, &rttMs, &failCount1h, &onlineUsers,
		&failover1h, &failover24h, &lastError, &lastFailoverAt,
		&lastFailoverReason, &updatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return map[string]interface{}{
				"available": false,
				"note":      "服务器无通道健康数据（agent 未连接或未上报）",
			}
		}
		return map[string]interface{}{
			"available": false,
			"error":    err.Error(),
		}
	}
	m := map[string]interface{}{
		"available":          true,
		"active_channel":     activeChannel,
		"channel_state":      channelState,
		"rtt_ms":             rttMs,
		"fail_count_1h":      failCount1h,
		"online_users":       onlineUsers,
		"failover_count_1h":  failover1h,
		"failover_count_24h": failover24h,
		"updated_at":         updatedAt.Format(time.RFC3339),
	}
	if lastError != nil && *lastError != "" {
		m["last_error"] = *lastError
	}
	if lastFailoverAt != nil {
		m["last_failover_at"] = lastFailoverAt.Format(time.RFC3339)
	}
	if lastFailoverReason != nil && *lastFailoverReason != "" {
		m["last_failover_reason"] = *lastFailoverReason
	}
	return m
}

func (c *DBLogCollector) fetchLatestDoctorMetrics(ctx context.Context, nodeID uuid.UUID, start, end time.Time) map[string]interface{} {
	var (
		overallStatus string
		ok, warn, fail int
		checksJSON    []byte
		createdAt     time.Time
	)
	err := c.pool.QueryRow(ctx, `
		SELECT overall_status, summary_ok, summary_warn, summary_fail, checks, created_at
		FROM node_doctor_reports
		WHERE node_id = $1 AND created_at BETWEEN $2 AND $3
		ORDER BY created_at DESC LIMIT 1
	`, nodeID, start, end).Scan(&overallStatus, &ok, &warn, &fail, &checksJSON, &createdAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return map[string]interface{}{"error": err.Error()}
	}
	m := map[string]interface{}{
		"overall_status": overallStatus,
		"summary_ok":     ok,
		"summary_warn":   warn,
		"summary_fail":   fail,
		"created_at":     createdAt.Format(time.RFC3339),
	}
	// 解析 checks 提取每项 check 的状态
	var checks []map[string]interface{}
	if err := json.Unmarshal(checksJSON, &checks); err == nil && len(checks) > 0 {
		checksSummary := make([]map[string]string, 0, len(checks))
		for _, ck := range checks {
			entry := map[string]string{}
			if v, ok := ck["check_code"].(string); ok {
				entry["check_code"] = v
			}
			if v, ok := ck["status"].(string); ok {
				entry["status"] = v
			}
			if v, ok := ck["severity"].(string); ok {
				entry["severity"] = v
			}
			if v, ok := ck["message"].(string); ok {
				entry["message"] = v
			}
			checksSummary = append(checksSummary, entry)
		}
		m["checks"] = checksSummary
	}
	return m
}
