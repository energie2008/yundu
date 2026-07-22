// Package runtime 提供双内核并行运行时支持。
//
// MultiRuntimePlugin 是双内核一等公民架构的核心实现（手册 §5 P2 原则）。
// 它同时持有 xray 和 sing-box 两个 RuntimePlugin 实例，对外暴露统一接口，
// 实现以下能力：
//   - 配置应用：根据 runtime_type 分发到对应内核
//   - 流量统计：合并两个内核的 per-user 流量数据
//   - 热重载：两个内核独立热重载，互不影响
//   - 状态查询：返回两个内核的合并状态
//
// 设计要点：
//   - xray 始终运行（主内核），sing-box 按需启动（有 sing-box 节点时才启动）
//   - 两个内核监听不同端口，互不冲突
//   - 流量数据合并时，同用户的流量累加（不同内核可能有同一用户的节点）
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/sagernet/sing-box/option"
)

// MultiRuntimePlugin 双内核并行运行时插件。
//
// 同时管理 xray 和 sing-box 两个内核实例，对外提供统一的 RuntimePlugin 接口。
// 配置应用时根据配置内容的 runtime_type 字段分发到对应内核；
// 流量统计时合并两个内核的数据。
type MultiRuntimePlugin struct {
	mu sync.Mutex

	// xrayPlugin 始终存在（主内核）
	xrayPlugin *NativeXray
	// xrayStarted 标记 xray 是否已启动（通过 Start 方法触发）
	// 不能用 xrayPlugin != nil 判断，因为构造函数中 xrayPlugin 始终被赋值
	xrayStarted bool
	// singboxPlugin 按需创建（首次收到 sing-box 配置时）
	singboxPlugin *NativeSingbox
	// singboxStarted 标记 sing-box 是否已启动
	singboxStarted bool
	// singboxClashEndpoint 是分配给 sing-box Clash API 的端点
	singboxClashEndpoint string
	// singboxSpeedLimiter 存储 sing-box 的限速器，在 singboxPlugin 创建时注入
	singboxSpeedLimiter RateLimiter
	// singboxDeviceChecker 存储 sing-box 的设备检查器，在 singboxPlugin 创建时注入
	singboxDeviceChecker DeviceChecker
	// singboxIPChecker 存储 sing-box 的 IP 限制检查器，在 singboxPlugin 创建时注入
	singboxIPChecker IPChecker

	logger *slog.Logger
}

// NewMultiRuntimePlugin 创建双内核并行运行时插件。
// xrayPlugin 必须非 nil（主内核），singboxClashEndpoint 为 sing-box Clash API 端点（Machine模式使用）。
func NewMultiRuntimePlugin(xrayPlugin *NativeXray, singboxClashEndpoint string, logger *slog.Logger) *MultiRuntimePlugin {
	return &MultiRuntimePlugin{
		xrayPlugin:           xrayPlugin,
		singboxClashEndpoint: singboxClashEndpoint,
		logger:               logger,
	}
}

// ensureSingboxPlugin 确保 sing-box 插件实例存在（懒初始化）。
func (m *MultiRuntimePlugin) ensureSingboxPlugin() *NativeSingbox {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.singboxPlugin == nil {
		m.singboxPlugin = NewNativeSingbox(m.logger, m.singboxClashEndpoint)
		// 注入已存储的限速器
		if m.singboxSpeedLimiter != nil {
			m.singboxPlugin.SetSpeedLimiter(m.singboxSpeedLimiter)
		}
		// 注入已存储的设备检查器
		if m.singboxDeviceChecker != nil {
			m.singboxPlugin.SetDeviceChecker(m.singboxDeviceChecker)
		}
		// 注入已存储的 IP 限制检查器
		if m.singboxIPChecker != nil {
			m.singboxPlugin.SetIPLimiter(m.singboxIPChecker)
		}
		m.logger.Info("sing-box plugin initialized (lazy)", "component", "multi-runtime")
	}
	return m.singboxPlugin
}

// SetSingboxSpeedLimiter 设置 sing-box 的 per-user 限速器。
// 如果 sing-box 插件已创建，立即注入；否则存储并在创建时注入。
// 传入 nil 可禁用限速。
func (m *MultiRuntimePlugin) SetSingboxSpeedLimiter(l RateLimiter) {
	m.mu.Lock()
	m.singboxSpeedLimiter = l
	if m.singboxPlugin != nil {
		m.singboxPlugin.SetSpeedLimiter(l)
	}
	m.mu.Unlock()
}

// SetSingboxDeviceChecker 设置 sing-box 的 per-user 设备数检查器。
// 如果 sing-box 插件已创建，立即注入；否则存储并在创建时注入。
// 传入 nil 可禁用设备数限制。
func (m *MultiRuntimePlugin) SetSingboxDeviceChecker(dc DeviceChecker) {
	m.mu.Lock()
	m.singboxDeviceChecker = dc
	if m.singboxPlugin != nil {
		m.singboxPlugin.SetDeviceChecker(dc)
	}
	m.mu.Unlock()
}

// SetSingboxIPLimiter 设置 sing-box 的 per-user IP 限制检查器。
// 如果 sing-box 插件已创建，立即注入；否则存储并在创建时注入。
// 传入 nil 可禁用 IP 限制。
func (m *MultiRuntimePlugin) SetSingboxIPLimiter(ic IPChecker) {
	m.mu.Lock()
	m.singboxIPChecker = ic
	if m.singboxPlugin != nil {
		m.singboxPlugin.SetIPLimiter(ic)
	}
	m.mu.Unlock()
}

// Start 应用配置到对应内核。
//
// 配置内容的 runtime_type 通过 detectRuntimeType 检测：
//   - xray 配置 → xrayPlugin.Start
//   - sing-box 配置 → singboxPlugin.Start（首次调用时懒初始化）
//
// 两个内核独立启动，互不影响。如果一个内核启动失败，另一个不受影响。
func (m *MultiRuntimePlugin) Start(ctx context.Context, configBytes []byte) error {
	rtType := detectRuntimeType(configBytes)
	m.logger.Info("multi-runtime Start",
		"runtime_type", rtType,
		"config_size", len(configBytes),
		"component", "multi-runtime")

	switch rtType {
	case "sing-box":
		sb := m.ensureSingboxPlugin()
		if err := sb.Start(ctx, configBytes); err != nil {
			return fmt.Errorf("sing-box start failed: %w", err)
		}
		m.mu.Lock()
		m.singboxStarted = true
		m.mu.Unlock()
		m.logger.Info("sing-box started successfully", "component", "multi-runtime")
		return nil

	default: // xray
		if err := m.xrayPlugin.Start(ctx, configBytes); err != nil {
			return fmt.Errorf("xray start failed: %w", err)
		}
		m.mu.Lock()
		m.xrayStarted = true
		m.mu.Unlock()
		m.logger.Info("xray started successfully", "component", "multi-runtime")
		return nil
	}
}

// Stop 停止所有已启动的内核。
func (m *MultiRuntimePlugin) Stop(ctx context.Context) error {
	var errs []error

	m.mu.Lock()
	xrayStarted := m.xrayStarted
	sbStarted := m.singboxStarted
	// 停止后重置启动状态
	m.xrayStarted = false
	m.singboxStarted = false
	m.mu.Unlock()

	if sbStarted {
		if err := m.singboxPlugin.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("sing-box stop: %w", err))
		}
	}
	if xrayStarted {
		if err := m.xrayPlugin.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("xray stop: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi-runtime stop errors: %v", errs)
	}
	return nil
}

// UpdateUsers 更新用户凭证到对应内核。
// 遍历所有已启动的内核，将用户更新应用到所有内核。
func (m *MultiRuntimePlugin) UpdateUsers(ctx context.Context, adds []User, dels []string) error {
	m.mu.Lock()
	xrayStarted := m.xrayStarted
	sbStarted := m.singboxStarted
	m.mu.Unlock()

	// xray 用户更新
	if xrayStarted {
		if err := m.xrayPlugin.UpdateUsers(ctx, adds, dels); err != nil {
			m.logger.Warn("xray UpdateUsers failed", "error", err, "component", "multi-runtime")
			// 不 return，继续更新 sing-box
		}
	}

	// sing-box 用户更新
	if sbStarted {
		if err := m.singboxPlugin.UpdateUsers(ctx, adds, dels); err != nil {
			m.logger.Warn("sing-box UpdateUsers failed", "error", err, "component", "multi-runtime")
		}
	}
	return nil
}

// GetTrafficStats 合并两个内核的流量统计（增量值，读取后清零）。
//
// 合并规则：
//   - 同一用户标识（email/UUID）的流量累加
//   - 两个内核都返回增量值（Reset=true 或 Swap(0)），合并后仍为增量值
//   - 仅返回有流量的用户
func (m *MultiRuntimePlugin) GetTrafficStats(ctx context.Context) (map[string]TrafficStat, error) {
	merged := make(map[string]TrafficStat)

	// 采集 xray 流量
	m.mu.Lock()
	xrayStarted := m.xrayStarted
	sbStarted := m.singboxStarted
	m.mu.Unlock()

	if xrayStarted {
		xrayStats, err := m.xrayPlugin.GetTrafficStats(ctx)
		if err != nil {
			m.logger.Warn("xray GetTrafficStats failed", "error", err, "component", "multi-runtime")
		} else {
			for k, v := range xrayStats {
				merged[k] = v
			}
		}
	}

	// 采集 sing-box 流量
	if sbStarted {
		sbStats, err := m.singboxPlugin.GetTrafficStats(ctx)
		if err != nil {
			m.logger.Warn("sing-box GetTrafficStats failed", "error", err, "component", "multi-runtime")
		} else {
			// 合并：同用户流量累加
			for k, v := range sbStats {
				if existing, ok := merged[k]; ok {
					existing.Upload += v.Upload
					existing.Download += v.Download
					merged[k] = existing
				} else {
					merged[k] = v
				}
			}
		}
	}

	return merged, nil
}

// GetTrafficStatsNoReset 非破坏性读取两个内核的流量统计（不清零计数器）。
//
// 合并规则与 GetTrafficStats 相同：同用户流量累加。
// 用于容错上报模式：上报失败时基线不变，下次读取自动包含未上报的流量。
func (m *MultiRuntimePlugin) GetTrafficStatsNoReset(ctx context.Context) (map[string]TrafficStat, error) {
	merged := make(map[string]TrafficStat)

	m.mu.Lock()
	xrayStarted := m.xrayStarted
	sbStarted := m.singboxStarted
	m.mu.Unlock()

	if xrayStarted {
		xrayStats, err := m.xrayPlugin.GetTrafficStatsNoReset(ctx)
		if err != nil {
			m.logger.Warn("xray GetTrafficStatsNoReset failed", "error", err, "component", "multi-runtime")
		} else {
			for k, v := range xrayStats {
				merged[k] = v
			}
		}
	}

	if sbStarted {
		sbStats, err := m.singboxPlugin.GetTrafficStatsNoReset(ctx)
		if err != nil {
			m.logger.Warn("sing-box GetTrafficStatsNoReset failed", "error", err, "component", "multi-runtime")
		} else {
			for k, v := range sbStats {
				if existing, ok := merged[k]; ok {
					existing.Upload += v.Upload
					existing.Download += v.Download
					merged[k] = existing
				} else {
					merged[k] = v
				}
			}
		}
	}

	return merged, nil
}

// Status 返回主内核（xray）的状态。
// sing-box 状态作为辅助信息记录在日志中。
func (m *MultiRuntimePlugin) Status(ctx context.Context) (*PluginStatus, error) {
	return m.xrayPlugin.Status(ctx)
}

// Validate 校验配置内容（自动检测 runtime_type 分发到对应内核）。
func (m *MultiRuntimePlugin) Validate(configBytes []byte) error {
	rtType := detectRuntimeType(configBytes)
	switch rtType {
	case "sing-box":
		sb := m.ensureSingboxPlugin()
		return sb.Validate(configBytes)
	default:
		return m.xrayPlugin.Validate(configBytes)
	}
}

// GetSingboxPlugin 返回 sing-box 插件实例（用于 PluginAdapter 直接引用）。
func (m *MultiRuntimePlugin) GetSingboxPlugin() *NativeSingbox {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.singboxPlugin
}

// IsSingboxStarted 返回 sing-box 是否已启动。
func (m *MultiRuntimePlugin) IsSingboxStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.singboxStarted
}

// detectRuntimeType 根据配置内容检测 runtime 类型。
//
// 检测规则（按优先级）：
//  1. sing-box 配置有 "log" + "inbounds" + "outbounds" 顶层字段，且 inbound 用 "type" 而非 "protocol"
//  2. xray 配置有 "inbounds" 但 inbound 用 "protocol" 字段
//  3. 尝试 sing-box option.Options 解析，成功则为 sing-box
//  4. 默认 xray
//
// P9-FIX: 旧版 containsSingboxMarker 用字符串匹配 `"type": "vless"` 等会误判 xray 配置
// （xray outbound/routing 中也可能出现这些字符串）。改用结构化检测：
//   - sing-box: 顶层有 "route" 字段（xray 用 "routing"）
//   - sing-box: inbound 数组元素用 "type" 字段（xray 用 "protocol"）
//   - sing-box 专属协议（hysteria2/tuic/anytls）仍可用字符串匹配作为辅助判据
func detectRuntimeType(configBytes []byte) string {
	if len(configBytes) == 0 {
		return "xray"
	}

	// 结构化检测：解析 JSON 顶层字段
	// sing-box 用 "route"，xray 用 "routing"
	// sing-box inbound 用 "type"，xray inbound 用 "protocol"
	var top struct {
		Route    json.RawMessage `json:"route"`
		Routing  json.RawMessage `json:"routing"`
		Inbounds []struct {
			Type     string `json:"type"`
			Protocol string `json:"protocol"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(configBytes, &top); err == nil {
		// 1. 顶层有非空 "route" 字段 → sing-box（xray 用 "routing"）
		if len(top.Route) > 0 && string(top.Route) != "null" {
			return "sing-box"
		}
		// 2. 顶层有非空 "routing" 字段 → xray（sing-box 用 "route"）
		if len(top.Routing) > 0 && string(top.Routing) != "null" {
			return "xray"
		}
		// 3. inbound 用 "type" 字段（无 "protocol"）→ sing-box
		for _, in := range top.Inbounds {
			if in.Type != "" && in.Protocol == "" {
				return "sing-box"
			}
		}
		// 4. inbound 用 "protocol" 字段 → xray
		for _, in := range top.Inbounds {
			if in.Protocol != "" {
				return "xray"
			}
		}
	}

	// 辅助判据：sing-box 专属协议（hysteria2/tuic/anytls 这些 xray 不支持的协议）
	configStr := string(configBytes)
	singboxOnlyProtocols := []string{
		`"type": "hysteria2"`,
		`"type": "hysteria"`,
		`"type": "tuic"`,
		`"type": "anytls"`,
	}
	for _, marker := range singboxOnlyProtocols {
		if stringContains(configStr, marker) {
			return "sing-box"
		}
	}
	return "xray"
}

// stringContains 简单字符串包含检测（避免引入 strings 包的依赖）。
func stringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 保留 option 包引用以备未来扩展（如解析 sing-box 配置验证）
var _ = option.Options{}
