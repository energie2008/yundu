package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	statsCmd "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// XrayStatsReader 通过 xray 内置 gRPC StatsService 读取 per-user 流量统计。
//
// 适用场景：子进程模式（useNative=false）下，node-agent 无法通过 runtimePlugin
// 读取流量统计（runtimePlugin 为 nil）。但子进程 xray 仍监听 10085 gRPC API
// （kernelrender 总是渲染 api inbound），因此可通过独立的 gRPC 客户端读取。
//
// 与 NativeXray.GetTrafficStatsNoReset 的区别：
//   - NativeXray 持有 xray core.Instance，gRPC 连接在 Start 时建立
//   - XrayStatsReader 不持有 xray 实例，gRPC 连接 lazy 初始化（xray 子进程可能尚未启动）
//   - XrayStatsReader 从配置文件读取 email→UUID 映射（NativeXray 从内存 configBytes 读取）
//
// 返回累计值（Reset=false），调用方（traffic.go reportTraffic）负责跟踪基线计算增量。
type XrayStatsReader struct {
	apiEndpoint string
	configPath  string // xray 配置文件路径，用于提取 email→UUID 映射
	logger      *slog.Logger

	mu   sync.Mutex
	conn *grpc.ClientConn
}

// NewXrayStatsReader 创建独立的 xray 流量统计读取器。
// apiEndpoint 为空时使用 defaultXrayAPIEndpoint（127.0.0.1:10085）。
// configPath 为 xray 配置文件路径（agent 写入的 config.json）。
func NewXrayStatsReader(apiEndpoint, configPath string, logger *slog.Logger) *XrayStatsReader {
	if apiEndpoint == "" {
		apiEndpoint = defaultXrayAPIEndpoint
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &XrayStatsReader{
		apiEndpoint: apiEndpoint,
		configPath:  configPath,
		logger:      logger.With("component", "xray-stats-reader"),
	}
}

// getConn 获取或建立 gRPC 连接（lazy 初始化 + 断线重连）。
// 调用方持锁。
func (r *XrayStatsReader) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	if r.conn != nil {
		// 快速检查连接是否仍可用（非阻塞）
		// grpc.ClientConn 内部有连接状态管理，连接断开时会自动重连
		// 这里不强制检查状态，依赖 gRPC 内部重连机制
		return r.conn, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, grpcDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, r.apiEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// 不使用 grpc.WithBlock()，允许非阻塞建立连接
		// 首次调用时 xray 可能尚未启动，返回的 conn 会在后台重连
	)
	if err != nil {
		return nil, fmt.Errorf("xray-stats-reader: dial gRPC %s: %w", r.apiEndpoint, err)
	}
	r.conn = conn
	r.logger.Info("xray stats reader connected to gRPC API", "endpoint", r.apiEndpoint)
	return conn, nil
}

// GetTrafficStatsNoReset 非破坏性读取 per-user 流量统计（不清零计数器）。
//
// 通过 QueryStats(Reset=false) 读取当前累计值（自 xray 启动以来的单调递增计数器）。
// 返回累计值，调用方需自行跟踪基线计算增量：
//   - delta = current - lastReported
//   - 上报成功后更新 lastReported = current
//   - 上报失败时 lastReported 不变，下次自动包含未上报流量
//
// xray 重启时计数器归零，调用方应检测 current < lastReported 并将 current 作为全量增量。
//
// 若 gRPC 连接不可用（xray 未启动或已退出），返回 error，调用方应跳过本次上报。
// QueryStats 调用带 10s 超时，防止 xray 未启动时阻塞上报循环。
func (r *XrayStatsReader) GetTrafficStatsNoReset(ctx context.Context) (map[string]TrafficStat, error) {
	r.mu.Lock()
	conn, err := r.getConn(ctx)
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}

	// 给 QueryStats 调用加超时，防止 xray 未启动时 gRPC 阻塞
	queryCtx, queryCancel := context.WithTimeout(ctx, 10*time.Second)
	defer queryCancel()

	statsClient := statsCmd.NewStatsServiceClient(conn)
	resp, err := statsClient.QueryStats(queryCtx, &statsCmd.QueryStatsRequest{Reset_: false})
	if err != nil {
		// gRPC 调用失败：可能是 xray 已退出或连接断开
		// 关闭旧连接，下次调用时重连
		r.mu.Lock()
		if r.conn != nil {
			r.conn.Close()
			r.conn = nil
		}
		r.mu.Unlock()
		return nil, fmt.Errorf("xray-stats-reader: query stats: %w", err)
	}

	// 从配置文件读取 email→UUID 映射
	configBytes, _ := os.ReadFile(r.configPath)
	emailToUUID := extractEmailUUIDMap(configBytes)

	rawStats := resp.GetStat()
	if len(rawStats) == 0 {
		r.logger.Debug("QueryStats returned empty", "endpoint", r.apiEndpoint)
	}

	// 解析 "user>>><email>>>traffic>>>uplink|downlink" 格式
	stats := make(map[string]TrafficStat)
	skippedZero := 0
	skippedFormat := 0
	for _, stat := range rawStats {
		name := stat.GetName()
		value := stat.GetValue()
		if value == 0 {
			skippedZero++
			continue
		}
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			skippedFormat++
			continue
		}
		email := parts[1]
		direction := parts[3]

		ts, ok := stats[email]
		if !ok {
			ts = TrafficStat{Email: email}
			if uid, found := emailToUUID[email]; found {
				ts.UUID = uid
			}
		}
		switch direction {
		case "uplink":
			ts.Upload += value
		case "downlink":
			ts.Download += value
		}
		stats[email] = ts
	}

	r.logger.Debug("QueryStats parsed result",
		"total_raw", len(rawStats),
		"skipped_zero", skippedZero,
		"skipped_format", skippedFormat,
		"valid_stats", len(stats))

	return stats, nil
}

// Close 关闭 gRPC 连接（Agent 退出时调用）。
func (r *XrayStatsReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}
