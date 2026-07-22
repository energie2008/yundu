package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeTrafficQuotaService P3-N: 节点级流量限额检查定时任务。
//
// 对应需求：当节点累计流量（上行+下行）超过 transfer_enable_bytes 时，
// 将节点标记为"流量超限"——通过设置 is_enabled = false 禁用节点，
// 使面板不再将该节点纳入下发配置，从而停止服务。
//
// 检查周期：每 5 分钟执行一次（与 traffic-service 的分钟级定时任务对齐）。
type NodeTrafficQuotaService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewNodeTrafficQuotaService 创建节点流量限额检查服务。
func NewNodeTrafficQuotaService(pool *pgxpool.Pool, logger *slog.Logger) *NodeTrafficQuotaService {
	if logger == nil {
		logger = slog.Default()
	}
	return &NodeTrafficQuotaService{
		pool:   pool,
		logger: logger.With("component", "node-traffic-quota"),
	}
}

// nodeQuotaRow 是查询节点限额与累计流量的中间结构。
type nodeQuotaRow struct {
	NodeID              uuid.UUID
	NodeCode            string
	NodeName            string
	TransferEnableBytes int64
	UsedBytes           int64
}

// CheckQuotas 检查所有设置了 transfer_enable_bytes > 0 的节点，
// 若累计流量超过限额则禁用节点（is_enabled = false）。
//
// 流量计算来源：traffic_usage_daily 表中该节点的所有上行+下行流量总和。
// 禁用操作：UPDATE nodes SET is_enabled = false WHERE id = $1。
//
// 返回被禁用的节点数量。单个节点处理失败仅记录日志，不影响其他节点。
func (s *NodeTrafficQuotaService) CheckQuotas(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.id,
		       n.code,
		       n.name,
		       COALESCE(n.transfer_enable_bytes, 0) AS transfer_enable_bytes,
		       COALESCE(t.used, 0) AS used_bytes
		FROM nodes n
		LEFT JOIN (
		    SELECT node_id, COALESCE(SUM(upload_bytes + download_bytes), 0) AS used
		    FROM traffic_usage_daily
		    WHERE node_id IS NOT NULL
		    GROUP BY node_id
		) t ON t.node_id = n.id
		WHERE n.deleted_at IS NULL
		  AND n.is_enabled = true
		  AND COALESCE(n.transfer_enable_bytes, 0) > 0`)
	if err != nil {
		s.logger.Error("node quota: query nodes failed", "error", err)
		return 0, fmt.Errorf("query nodes with quota: %w", err)
	}
	defer rows.Close()

	var quotaRows []nodeQuotaRow
	for rows.Next() {
		var r nodeQuotaRow
		if err := rows.Scan(&r.NodeID, &r.NodeCode, &r.NodeName, &r.TransferEnableBytes, &r.UsedBytes); err != nil {
			s.logger.Error("node quota: scan row failed", "error", err)
			continue
		}
		quotaRows = append(quotaRows, r)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate quota rows: %w", err)
	}

	if len(quotaRows) == 0 {
		return 0, nil
	}

	s.logger.Info("node quota: checking nodes", "count", len(quotaRows))

	disabled := 0
	for _, r := range quotaRows {
		if r.UsedBytes >= r.TransferEnableBytes {
			s.logger.Warn("node quota: traffic exceeded, disabling node",
				"node_id", r.NodeID,
				"node_code", r.NodeCode,
				"used_bytes", r.UsedBytes,
				"quota_bytes", r.TransferEnableBytes,
			)
			if err := s.disableNode(ctx, r.NodeID); err != nil {
				s.logger.Error("node quota: disable node failed",
					"node_id", r.NodeID, "node_code", r.NodeCode, "error", err)
				continue
			}
			disabled++
		} else {
			s.logger.Debug("node quota: node within limit",
				"node_code", r.NodeCode,
				"used_bytes", r.UsedBytes,
				"quota_bytes", r.TransferEnableBytes,
				"usage_ratio", fmt.Sprintf("%.2f%%", float64(r.UsedBytes)/float64(r.TransferEnableBytes)*100),
			)
		}
	}

	if disabled > 0 {
		s.logger.Info("node quota: check completed, nodes disabled",
			"checked", len(quotaRows), "disabled", disabled)
	}
	return disabled, nil
}

// disableNode 将节点标记为禁用（is_enabled = false），并记录禁用原因到 metadata。
func (s *NodeTrafficQuotaService) disableNode(ctx context.Context, nodeID uuid.UUID) error {
	// 设置 is_enabled = false，并在 metadata 中记录流量超限标记
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes
		SET is_enabled = false,
		    metadata = metadata || '{"traffic_quota_exceeded": true, "disabled_at": "' || to_char(now(), 'YYYY-MM-DD"T"HH24:MI:SS') || '"}'::jsonb,
		    updated_at = now()
		WHERE id = $1 AND is_enabled = true`, nodeID)
	return err
}

// StartScheduledJobs 启动节点流量限额检查定时任务。
// 每 5 分钟执行一次，启动后立即执行首轮检查。
// ctx 取消后任务退出。
func (s *NodeTrafficQuotaService) StartScheduledJobs(ctx context.Context) {
	s.logger.Info("node traffic quota scheduled job starting", "interval", "5m")
	go s.runQuotaTicker(ctx)
}

// runQuotaTicker 每 5 分钟执行一次 CheckQuotas。
func (s *NodeTrafficQuotaService) runQuotaTicker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	// 启动后立即执行一次
	s.CheckQuotas(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.CheckQuotas(ctx)
		}
	}
}
