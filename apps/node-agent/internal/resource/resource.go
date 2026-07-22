// Package resource 实现声明式 Runtime Resource 抽象（P2-1）。
//
// 设计目标：将 nginx vhost / cloudflared tunnel / xray config / firewall / cert 等
// 可协调的运行时资源统一为 Resource 接口，由 ReconcilerDriver 统一调度。
//
// 核心抽象（四段式协调循环）：
//  1. Observe  — 观察当前实际状态（含持久化的 lastHash / lastAppliedConfig）
//  2. FetchDesired — 拉取期望状态（来自面板 HTTP API）
//  3. Diff     — 比较期望态与观察态，返回 DiffResult
//  4. Apply    — 根据 diff 执行实际变更
//  5. Persist  — 成功 apply 后持久化状态
//
// ReconcilerDriver 为每个 Resource 启动独立 goroutine，周期性执行四段式循环。
// 也可通过 Event 通道触发即时协调（证书续期、用户封禁等事件）。
package resource

import (
	"context"
	"time"
)

// Resource 声明式 Runtime Resource 接口（P2-1）。
// 一个 Resource 代表一个可被协调的运行时资源。
type Resource interface {
	// Kind 资源类型标识，如 "nginx-vhost" / "cloudflared-tunnel" / "xray-config"
	Kind() string

	// Observe 观察当前实际状态（含 lastHash 等持久化状态）。
	Observe(ctx context.Context) (ObservedState, error)

	// FetchDesired 拉取期望状态（来自面板 HTTP API）。
	FetchDesired(ctx context.Context) (DesiredState, error)

	// Diff 比较期望态与观察态，返回 DiffResult。
	Diff(desired DesiredState, observed ObservedState) (DiffResult, error)

	// Apply 根据 diff 执行实际变更。
	Apply(ctx context.Context, diff DiffResult) error

	// Persist 成功 apply 后持久化状态（hash / version / lastAppliedConfig）。
	Persist(ctx context.Context, desired DesiredState) error
}

// ObservedState 观察到的当前状态。
type ObservedState struct {
	Hash   string      // 内容 hash（Nginx/Cloudflared 模式）
	Raw    interface{} // 资源特定状态（如 *client.CDNVhostResponse）
	Empty  bool        // 是否为空（首次启动）
}

// DesiredState 期望状态。
type DesiredState struct {
	Hash string      // 内容 hash
	Raw  interface{} // 资源特定数据（如 *client.CDNVhostResponse / map[string]interface{}）
}

// DiffResult 协调差异结果。
type DiffResult struct {
	HasDrift      bool      // 是否需要 apply
	Level         DiffLevel // 变更级别（复用 hotdiff.DiffLevel 思路）
	ChangedFields []string  // 变更字段列表
	Summary       string    // 人类可读摘要
	Raw           interface{} // 资源特定 diff 明细（如 hotdiff.UserChanges）
}

// DiffLevel 变更级别（与 hotdiff.DiffLevel 对齐，避免循环依赖此处独立定义）。
type DiffLevel string

const (
	LevelNone      DiffLevel = ""                // 无变更
	LevelHotUser   DiffLevel = "HOT_USER_ONLY"   // 仅用户变更 → AlterInbound
	LevelHotRoute  DiffLevel = "HOT_ROUTING_ONLY" // 仅路由变更 → Reload
	LevelHotTLS    DiffLevel = "HOT_TLS_RELOAD"  // TLS 变更 → Reload
	LevelRestart   DiffLevel = "RESTART_REQUIRED" // 需重启
	LevelFullSync  DiffLevel = "FULL_SYNC"       // 全量同步（hash 不同，非字段级 diff）
)

// Event 触发即时协调的事件（可选实现）。
type Event struct {
	Type    string      // 事件类型（如 "cert-renewed" / "user-ban"）
	Source  string      // 事件来源
	Payload interface{} // 事件载荷
}

// EventHandler 可选：Resource 可实现此接口以响应外部事件触发即时协调。
type EventHandler interface {
	OnEvent(ctx context.Context, event Event) (DesiredState, error)
}

// Options Resource 注册选项。
type Options struct {
	Interval time.Duration // 轮询周期（默认 30s）
}

// Driver 统一调度所有 Resource 的驱动器。
type Driver interface {
	Register(r Resource, opts ...Option) error
	Start(ctx context.Context) error
	Stop()
	Trigger(ctx context.Context, kind string, event Event) error // 手动触发某 Resource 的即时协调
}

// Option 注册选项函数。
type Option func(*Options)

// WithInterval 设置轮询周期。
func WithInterval(d time.Duration) Option {
	return func(o *Options) {
		o.Interval = d
	}
}
