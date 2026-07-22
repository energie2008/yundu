package runtime

import (
	"context"
)

// RuntimePlugin 是代理内核的统一抽象接口。
// 实现方：NativeXray（基于 xray-core core.Instance）、NativeSingbox（基于 sing-box box.Box 蓝绿）。
//
// 设计原则：
//   - Start/Stop 管理内核生命周期，配置从内存字节流加载，不落盘
//   - UpdateUsers 增量更新用户（零断流热更），AlterInbound/蓝绿热转
//   - Validate 内存校验，不调用外部进程
//   - GetTrafficStats 获取 per-user 流量数据
//   - Status 返回运行时状态
type RuntimePlugin interface {
	// Start 根据传入的编译后 JSON 字节码在内存中拉起内核。
	// configBytes 由 IR 编译器（kernelrender）生成，直接传递给内核，不落盘。
	// 如果已有实例在运行，先优雅停止旧实例再启动新实例。
	Start(ctx context.Context, configBytes []byte) error

	// Stop 优雅关闭内核，排空老连接。
	Stop(ctx context.Context) error

	// UpdateUsers 增量更新用户（无需重启整个实例，零断流）。
	// adds: 新增/修改的用户列表；dels: 待删除用户的 email 列表。
	// 实现方应优先使用内核原生 API（xray AlterInbound / sing-box 蓝绿热转），
	// 失败时可回退到全量重载（Start）。
	UpdateUsers(ctx context.Context, adds []User, dels []string) error

	// GetTrafficStats 拉取 per-user 流量统计数据（uplink/downlink）。
	// 注意：此方法会清零计数器（破坏性读取），仅在上报成功时使用。
	// 上报可能失败时应使用 GetTrafficStatsNoReset + 容错策略。
	GetTrafficStats(ctx context.Context) (map[string]TrafficStat, error)

	// GetTrafficStatsNoReset 非破坏性读取 per-user 流量统计（不清零计数器）。
	// 返回自上次调用以来的增量值。用于容错上报：
	// 上报失败时下次调用仍包含未上报的流量，避免数据丢失。
	GetTrafficStatsNoReset(ctx context.Context) (map[string]TrafficStat, error)

	// Status 返回运行时状态。
	Status(ctx context.Context) (*PluginStatus, error)

	// Validate 在内存中校验配置字节码（不启动内核、不调用外部进程、不落盘）。
	// 用于预检阶段快速验证配置合法性。
	Validate(configBytes []byte) error
}
