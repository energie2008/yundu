package executor

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/airport-panel/node-agent/internal/limiter"
)

// limiterUserMeta 是渲染器注入的 _limiter.users 数组中单个用户的限速/设备限制/IP限制元数据。
type limiterUserMeta struct {
	Email          string `json:"email"`
	UUID           string `json:"uuid"`
	SpeedLimitMbps int    `json:"speed_limit_mbps"`
	DeviceLimit    int    `json:"device_limit"`
	IPLimit        int    `json:"ip_limit"`
}

// limiterConfigMeta 是渲染器注入的 _limiter 元数据结构（xray/sing-box 共享）。
//
// 由 kernelrender.renderLimiterMeta 生成，嵌入配置 JSON 的 "_limiter" 字段。
// Agent 在 Apply 前剥离该字段，并据此初始化 SpeedLimiter/DeviceLimiter/IPLimiter。
type limiterConfigMeta struct {
	NodeSpeedLimitMbps int               `json:"node_speed_limit_mbps"`
	NodeDeviceLimit    int               `json:"node_device_limit"`
	NodeIPLimit        int               `json:"node_ip_limit"`
	Users              []limiterUserMeta `json:"users"`
}

// LimiterIntegration 封装 SpeedLimiter/DeviceLimiter 的集成逻辑，
// 供 XrayExecutor/SingBoxExecutor 嵌入复用。
//
// 设计要点：
//   - SpeedLimiter 按用户 UUID（或 email）分桶管理令牌桶限速。
//   - DeviceLimiter 按用户 UUID 管理本地 IP 集合 + 面板下发全局设备态。
//   - userDeviceLimits 存储每用户的设备数上限（从 _limiter 元数据解析），
//     供 OnConnect 调用时查询。
//   - ParseLimiterConfig 从配置 JSON 中解析 _limiter 元数据并更新所有限速器状态。
type LimiterIntegration struct {
	speedLimiter     *limiter.SpeedLimiter
	deviceLimiter    *limiter.DeviceLimiter
	ipLimiter        *limiter.IPLimiter
	userDeviceLimits sync.Map // uuid/email -> int (device limit)
	userIPLimits     sync.Map // uuid/email -> int (ip limit)
	nodeIPLimit      int      // 节点级 IP 数限制默认值（用户未配置 per-user 限制时生效）
	logger           *slog.Logger
}

// NewLimiterIntegration 创建限速器集成实例，初始化空的 SpeedLimiter/DeviceLimiter/IPLimiter。
func NewLimiterIntegration(logger *slog.Logger) *LimiterIntegration {
	return &LimiterIntegration{
		speedLimiter:  limiter.NewSpeedLimiter(),
		deviceLimiter: limiter.NewDeviceLimiter(),
		ipLimiter:     limiter.NewIPLimiter(),
		logger:        logger.With("component", "limiter-integration"),
	}
}

// ParseLimiterConfig 从配置 JSON 字符串中解析 _limiter 元数据并更新限速器。
//
// 调用时机：
//   - Apply() 写入配置后
//   - Reload() 重新加载配置后
//   - main.go applyConfig 剥离 _limiter 后通过 UpdateLimitersFromMeta 调用
//
// 若配置中无 _limiter 字段，清除所有限速器状态（节点未配置限速）。
func (li *LimiterIntegration) ParseLimiterConfig(content string) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		li.logger.Debug("parse limiter config: not valid JSON", "error", err)
		return
	}
	rawMeta, ok := config["_limiter"]
	if !ok {
		li.logger.Debug("no _limiter metadata in config, limiters unchanged")
		return
	}
	li.parseLimiterMeta(rawMeta)
}

// UpdateLimitersFromMeta 从已提取的 _limiter 元数据（interface{}）更新限速器。
//
// 供 main.go 在剥离 _limiter 字段后调用：
//
//	limiterMeta := configMap["_limiter"]
//	delete(configMap, "_limiter")
//	// ... apply config ...
//	if updater, ok := runtimeExec.(LimiterUpdater); ok {
//	    updater.UpdateLimitersFromMeta(limiterMeta)
//	}
func (li *LimiterIntegration) UpdateLimitersFromMeta(meta interface{}) {
	if meta == nil {
		return
	}
	rawMeta, err := json.Marshal(meta)
	if err != nil {
		li.logger.Warn("failed to marshal limiter meta", "error", err)
		return
	}
	li.parseLimiterMeta(rawMeta)
}

// parseLimiterMeta 解析 _limiter 元数据的 JSON 字节并更新限速器。
func (li *LimiterIntegration) parseLimiterMeta(rawMeta []byte) {
	var meta limiterConfigMeta
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		li.logger.Warn("failed to parse _limiter metadata", "error", err)
		return
	}

	// 存储节点级 IP 限制默认值（用户未配置 per-user 限制时生效）
	li.nodeIPLimit = meta.NodeIPLimit

	for _, u := range meta.Users {
		key := u.UUID
		if key == "" {
			key = u.Email
		}
		if key == "" {
			continue
		}

		// 更新限速器：speed_limit_mbps <= 0 时移除限速器（不限速）
		li.speedLimiter.SetLimit(key, u.SpeedLimitMbps)

		// 更新设备数限制
		if u.DeviceLimit > 0 {
			li.userDeviceLimits.Store(key, u.DeviceLimit)
		} else {
			li.userDeviceLimits.Delete(key)
		}

		// 更新 IP 数限制（per-user 优先于 node-level）
		if u.IPLimit > 0 {
			li.userIPLimits.Store(key, u.IPLimit)
		} else {
			li.userIPLimits.Delete(key)
		}
	}

	li.logger.Info("limiter config parsed",
		"user_count", len(meta.Users),
		"speed_limiters", li.speedLimiter.Count(),
		"node_speed_limit_mbps", meta.NodeSpeedLimitMbps,
		"node_device_limit", meta.NodeDeviceLimit,
		"node_ip_limit", meta.NodeIPLimit)
}

// GetDeviceLimit 返回指定用户（uuid/email）的设备数上限。
// 返回 0 表示不限制设备数。
func (li *LimiterIntegration) GetDeviceLimit(key string) int {
	if v, ok := li.userDeviceLimits.Load(key); ok {
		if limit, ok := v.(int); ok {
			return limit
		}
	}
	return 0
}

// GetIPLimit 返回指定用户（uuid/email）的 IP 数上限。
// 优先返回 per-user 限制；若未配置则回退到节点级默认值（nodeIPLimit）。
// 返回 0 表示不限制 IP 数。
func (li *LimiterIntegration) GetIPLimit(key string) int {
	if v, ok := li.userIPLimits.Load(key); ok {
		if limit, ok := v.(int); ok && limit > 0 {
			return limit
		}
	}
	return li.nodeIPLimit
}

// SpeedLimiter 返回底层 SpeedLimiter 实例（供外部调用 AllowN 等）。
func (li *LimiterIntegration) SpeedLimiter() *limiter.SpeedLimiter {
	return li.speedLimiter
}

// DeviceLimiter 返回底层 DeviceLimiter 实例（供外部调用 OnConnect/OnDisconnect 等）。
func (li *LimiterIntegration) DeviceLimiter() *limiter.DeviceLimiter {
	return li.deviceLimiter
}

// IPLimiter 返回底层 IPLimiter 实例（供外部调用 BlockIP/SetAllowedIPs 等）。
func (li *LimiterIntegration) IPLimiter() *limiter.IPLimiter {
	return li.ipLimiter
}

// LimiterUpdater 是支持限速器更新的执行器接口。
// XrayExecutor 和 SingBoxExecutor 均实现此接口。
// main.go 通过类型断言访问，避免污染 RuntimeExecutor 主接口。
type LimiterUpdater interface {
	UpdateLimitersFromMeta(meta interface{})
}

// DeviceLimiterProvider 提供对 DeviceLimiter 和 SpeedLimiter 的访问。
// main.go 的设备状态上报循环通过类型断言访问此接口。
type DeviceLimiterProvider interface {
	DeviceLimiter() *limiter.DeviceLimiter
	SpeedLimiter() *limiter.SpeedLimiter
	GetDeviceLimit(uuid string) int
}

// IPLimiterProvider 提供对 IPLimiter 的访问。
// main.go 的 syncLimiters 循环通过类型断言访问此接口，同步 IP 黑白名单/IP数限制。
type IPLimiterProvider interface {
	IPLimiter() *limiter.IPLimiter
	GetIPLimit(uuid string) int
}

// SingboxDeviceChecker 将 DeviceLimiter + 设备限制查询适配为 runtime.DeviceChecker 接口。
// 通过结构化类型（Go duck typing）自动满足 runtime.DeviceChecker 接口，
// 无需显式导入 runtime 包，避免循环依赖。
//
// 工作原理：
//   - OnConnect: 查询用户的设备数限制，调用 DeviceLimiter.OnConnect 检查是否允许
//   - OnDisconnect: 调用 DeviceLimiter.OnDisconnect 释放设备计数
//
// 在 PluginAdapter 构造时创建，注入到 MultiRuntimePlugin → NativeSingbox → ConnTracker。
// 设备数限制通过 LimiterIntegration.userDeviceLimits 查询，由 syncLimiters 热更新。
type SingboxDeviceChecker struct {
	deviceLimiter  *limiter.DeviceLimiter
	getDeviceLimit func(uuid string) int
}

// NewSingboxDeviceChecker 创建 sing-box 设备数检查器适配器。
// getDeviceLimit 返回用户的设备数上限，0 表示不限制。
func NewSingboxDeviceChecker(dl *limiter.DeviceLimiter, getLimit func(string) int) *SingboxDeviceChecker {
	return &SingboxDeviceChecker{
		deviceLimiter:  dl,
		getDeviceLimit: getLimit,
	}
}

// OnConnect 在新连接建立时调用，返回是否允许该连接。
// 设备数限制 <= 0 时不限制，直接放行。
func (c *SingboxDeviceChecker) OnConnect(userID string, ip string) bool {
	limit := c.getDeviceLimit(userID)
	return c.deviceLimiter.OnConnect(userID, ip, limit)
}

// OnDisconnect 在连接关闭时调用，释放设备计数。
func (c *SingboxDeviceChecker) OnDisconnect(userID string, ip string) {
	c.deviceLimiter.OnDisconnect(userID, ip)
}

// SingboxIPChecker 将 IPLimiter + IP数限制查询适配为 runtime.IPChecker 接口。
// 通过结构化类型（Go duck typing）自动满足 runtime.IPChecker 接口，
// 无需显式导入 runtime 包，避免循环依赖。
//
// 工作原理：
//   - OnConnect: 查询用户的 IP 数限制，调用 IPLimiter.OnConnect 检查是否允许
//     （判定优先级：黑名单 > 白名单 > IP数限制）
//   - OnDisconnect: 调用 IPLimiter.OnDisconnect 释放 IP 引用计数
//
// 在 PluginAdapter 构造时创建，注入到 MultiRuntimePlugin → NativeSingbox → ConnTracker。
// IP 数限制通过 LimiterIntegration.userIPLimits 查询，由 syncLimiters 热更新。
// IP 黑白名单通过 IPLimiter.BlockIP/SetAllowedIPs 管理（由面板下发或 main.go syncLimiters 同步）。
type SingboxIPChecker struct {
	ipLimiter    *limiter.IPLimiter
	getIPLimit   func(uuid string) int
}

// NewSingboxIPChecker 创建 sing-box IP 限制检查器适配器。
// getIPLimit 返回用户的 IP 数上限，0 表示不限制。
func NewSingboxIPChecker(il *limiter.IPLimiter, getLimit func(string) int) *SingboxIPChecker {
	return &SingboxIPChecker{
		ipLimiter:    il,
		getIPLimit:   getLimit,
	}
}

// OnConnect 在新连接建立时调用，返回是否允许该连接。
// 判定优先级：黑名单 > 白名单 > IP数限制。
// IP 数限制 <= 0 时不限制 IP 数（仅黑名单/白名单生效）。
func (c *SingboxIPChecker) OnConnect(userID string, ip string) bool {
	limit := c.getIPLimit(userID)
	return c.ipLimiter.OnConnect(userID, ip, limit)
}

// OnDisconnect 在连接关闭时调用，释放 IP 引用计数。
func (c *SingboxIPChecker) OnDisconnect(userID string, ip string) {
	c.ipLimiter.OnDisconnect(userID, ip)
}
