package provider

import (
	"context"
	"fmt"
	"sync"
)

// Provider 抽象不同运行时后端（node-agent / 3x-ui / 自定义）的通信逻辑。
// 参见 docs/adr/0004-runtime-provider.md
type Provider interface {
	// Type 返回 provider 类型标识（如 "node-agent"、"3x-ui"、"mock"）
	Type() string
	// RegisterRuntime 向后端注册一个 runtime，返回后端分配的引用 ID
	RegisterRuntime(ctx context.Context, spec RuntimeSpec) (runtimeRef string, err error)
	// PushConfig 推送配置到后端
	PushConfig(ctx context.Context, runtimeRef string, config string) error
	// PullStats 拉取运行时统计（在线人数、流量等）
	PullStats(ctx context.Context, runtimeRef string) (*RuntimeStats, error)
	// Reload 触发后端 reload
	Reload(ctx context.Context, runtimeRef string) error
	// Rollback 回滚到上一个配置版本
	Rollback(ctx context.Context, runtimeRef string) error
	// FetchCapabilities 查询后端支持的能力列表
	FetchCapabilities(ctx context.Context) ([]string, error)
}

// RuntimeSpec 描述注册一个 runtime 所需的最小信息
type RuntimeSpec struct {
	ServerCode   string `json:"server_code"`
	RuntimeType  string `json:"runtime_type"`  // xray / sing-box
	APIEndpoint  string `json:"api_endpoint"`
	Version      string `json:"version"`
}

// RuntimeStats 运行时统计
type RuntimeStats struct {
	OnlineUsers int    `json:"online_users"`
	UpBytes     int64  `json:"up_bytes"`
	DownBytes   int64  `json:"down_bytes"`
	Status      string `json:"status"` // running / stopped / error
	Message     string `json:"message,omitempty"`
}

// 能力常量
const (
	CapConfigPush      = "config_push"       // 支持配置下发
	CapHealthReport    = "health_report"     // 支持主动健康上报
	CapWarpSidecar     = "warp_sidecar"      // 支持 WARP sidecar
	CapDryRun          = "dry_run"            // 支持 dry-run 预检
	CapRuntimeUpgrade  = "runtime_upgrade"   // 支持运行时升级
	CapStatsPull       = "stats_pull"        // 支持统计拉取
)

// Registry 按 provider_type 查找 provider 实例
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Type()] = p
}

func (r *Registry) Get(providerType string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[providerType]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerType)
	}
	return p, nil
}

func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}
