// Package runtime 提供代理内核的原生内嵌抽象层。
// 支持 Xray（core.Instance）和 sing-box（box.Box 蓝绿）两种原生内嵌运行时，
// 替代原有的 exec.Command 子进程模式，实现零断流热更、内存配置、增量用户管理。
package runtime

import "time"

// User 描述单个代理用户（跨内核统一抽象）。
type User struct {
	// Email 是用户的唯一标识（xray 中为 email，sing-box 中为 name）。
	Email string `json:"email"`
	// UUID 是用户的认证凭证（VLESS/VMess/Trojan 使用）。
	UUID string `json:"uuid"`
	// Level 是用户等级（xray policy level）。
	Level int `json:"level,omitempty"`
	// Password 是 sing-box 协议（Hy2/TUIC/Shadowtls）的密码字段。
	Password string `json:"password,omitempty"`
	// Extra 存放内核专有字段（如 inbound_tag、flow、encryption 等）。
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// TrafficStat 描述单个用户的流量统计。
type TrafficStat struct {
	Email    string `json:"email"`
	UUID     string `json:"uuid,omitempty"` // 用户 UUID（用于流量上报凭证反查）
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

// PluginStatus 描述运行时插件的状态。
type PluginStatus struct {
	// Running 表示内核是否在运行。
	Running bool `json:"running"`
	// Version 是内核版本字符串。
	Version string `json:"version"`
	// Uptime 是内核已运行的秒数。
	Uptime int64 `json:"uptime_seconds"`
	// ConfigHash 是当前运行配置的 SHA-256 hash。
	ConfigHash string `json:"config_hash"`
	// RestartCount 是启动/重启总次数。
	RestartCount int64 `json:"restart_count"`
	// ActiveConns 是活跃连接数（用于 Drain 监控），-1 表示不可用。
	ActiveConns int64 `json:"active_connections"`
	// PID 是子进程模式下的 PID（原生模式为 0）。
	PID int `json:"pid,omitempty"`
	// StartedAt 是启动时间。
	StartedAt time.Time `json:"started_at,omitempty"`
}
