package executor

// IPEnforcerConfig 配置 IP 限制执行参数。
//
// IP 限制执行已集成进 DeviceEnforcer，避免重复的 gRPC 连接与 goroutine。
// DeviceEnforcer 已通过 GetStatsOnlineIpList 获取每个用户的在线 IP 列表，
// 在同一循环中追加 IP 限制检查为零成本。
//
// 用法：
//
//	enforcer := NewDeviceEnforcer(provider, cfg, reloadFn, logger)
//	enforcer.SetIPProvider(ipProvider) // 启用 IP 限制执行
//	enforcer.Start(ctx)
type IPEnforcerConfig struct {
	// Enabled 控制是否启用 IP 限制执行。
	// 为 false 时，即使设置了 ipProvider 也不检查 IP 限制。
	Enabled bool
}
