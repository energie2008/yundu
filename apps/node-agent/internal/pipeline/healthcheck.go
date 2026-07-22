package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/airport-panel/node-agent/internal/machine"
)

// HealthChecker 执行部署后深度健康探活。
//
// MVP 阶段：通过 net.Dial 检测 inbound 端口是否可连。
// 未来迭代：通过 Mock Server + 代理穿透验证（真实流量拨测）。
type HealthChecker struct {
	logger *slog.Logger
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(logger *slog.Logger) *HealthChecker {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthChecker{logger: logger}
}

// CheckHealth 检查内核是否健康。
//
// MVP 阶段：通过 net.Dial 检测 inbound 端口是否可连。
// 未来迭代：通过 Mock Server + 代理穿透验证。
//
// 只要至少一个业务端口（跳过内部端口）可连即认为健康。
func (h *HealthChecker) CheckHealth(ctx context.Context, configJSON []byte, waitDuration time.Duration) error {
	// 等待内核完全启动
	select {
	case <-time.After(waitDuration):
	case <-ctx.Done():
		return ctx.Err()
	}

	// 提取 inbound 端口
	ports, err := extractPorts(configJSON)
	if err != nil {
		return fmt.Errorf("extract ports for health check: %w", err)
	}

	if len(ports) == 0 {
		// 没有端口可检查，认为健康
		return nil
	}

	// 检查至少一个端口可连
	dialTimeout := 3 * time.Second
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			continue
		}
		// 跳过内部 API 端口（使用范围判断）
		if machine.IsInternalAPIPort(port) {
			continue
		}

		addr := fmt.Sprintf("127.0.0.1:%d", port)
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err != nil {
			h.logger.Debug("health check: port not reachable", "addr", addr, "error", err)
			continue
		}
		conn.Close()
		h.logger.Info("health check: port reachable", "addr", addr)
		return nil
	}

	return fmt.Errorf("health check failed: none of %d inbound ports are reachable", len(ports))
}

// extractPorts 从配置 JSON 中提取端口列表。
//
// 同时兼容 xray (inbounds[].port) 与 sing-box (inbounds[].listen_port) 格式。
func extractPorts(configJSON []byte) ([]int, error) {
	var cfg map[string]interface{}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, err
	}

	var ports []int

	if inbounds, ok := cfg["inbounds"].([]interface{}); ok {
		for _, ib := range inbounds {
			if m, ok := ib.(map[string]interface{}); ok {
				if port, ok := toInt(m["port"]); ok {
					ports = append(ports, port)
				}
				if port, ok := toInt(m["listen_port"]); ok {
					ports = append(ports, port)
				}
			}
		}
	}

	return ports, nil
}

// toInt 尝试将 interface{} 转换为 int
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}
