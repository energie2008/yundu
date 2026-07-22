package inboundgroup

import (
	"sync"
)

// RenderMode 渲染模式，按 VPS 粒度切换（非全局开关）。
//
// 灰度节奏（社区最佳实践）：
//  1. 测试 VPS，0 真实用户 → JSON 人工比对+单测+xray run -test
//  2. 1 台生产 VPS，小流量节点 → connect 成功率/下载吞吐/path 分流正确性
//  3. 逐台灰度 → 出问题立即回退该 VPS 的 flag
//  4. 全量切换 → Legacy 代码保留≥1个月后再删除
type RenderMode string

const (
	// RenderModeLegacy 单 inbound 模式（现有行为，每个节点独占一个 inbound）
	RenderModeLegacy RenderMode = "single_inbound"
	// RenderModeGrouped InboundGroup 多 inbound 合并模式（primary+internal）
	RenderModeGrouped RenderMode = "inbound_group"
)

// FeatureFlag 按 VPS 粒度控制渲染模式。
//
// 设计要点：
//   - 按 VPS ID 切换，非全局开关，支持逐台灰度
//   - 默认 Legacy（保守），需要显式启用 Grouped
//   - 出问题立即回退该 VPS 的 flag，不影响其他 VPS
//   - node-agent 不需要跟着回滚版本（靠 IsGroupedConfig 结构特征识别）
type FeatureFlag struct {
	mu          sync.RWMutex
	modeByVPS   map[string]RenderMode
	defaultMode RenderMode
}

// NewFeatureFlag 创建 FeatureFlag，默认模式为 Legacy（保守）。
func NewFeatureFlag() *FeatureFlag {
	return &FeatureFlag{
		modeByVPS:   make(map[string]RenderMode),
		defaultMode: RenderModeLegacy,
	}
}

// GetRenderMode 获取指定 VPS 的渲染模式。
// 未显式设置的 VPS 返回 defaultMode（Legacy）。
func (f *FeatureFlag) GetRenderMode(vpsID string) RenderMode {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if mode, ok := f.modeByVPS[vpsID]; ok {
		return mode
	}
	return f.defaultMode
}

// SetRenderMode 设置指定 VPS 的渲染模式（灰度切换入口）。
func (f *FeatureFlag) SetRenderMode(vpsID string, mode RenderMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modeByVPS[vpsID] = mode
}

// Rollback 回退指定 VPS 到 Legacy 模式（出问题时立即调用）。
func (f *FeatureFlag) Rollback(vpsID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modeByVPS[vpsID] = RenderModeLegacy
}

// IsGrouped 判断指定 VPS 是否使用 grouped 模式。
func (f *FeatureFlag) IsGrouped(vpsID string) bool {
	return f.GetRenderMode(vpsID) == RenderModeGrouped
}

// ListGroupedVPS 列出所有已切换到 grouped 模式的 VPS（用于监控/审计）。
func (f *FeatureFlag) ListGroupedVPS() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var result []string
	for vpsID, mode := range f.modeByVPS {
		if mode == RenderModeGrouped {
			result = append(result, vpsID)
		}
	}
	return result
}
