// Package delta 提供 Delta Sync 的应用逻辑。
//
// DeltaApplier 接收控制面推送的增量用户变更，通过 RuntimePlugin.UpdateUsers
// 实现零断流热更，无需重启内核、无需全量替换配置。
package delta

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	agentruntime "github.com/airport-panel/node-agent/internal/runtime"
)

// Applier 将 Delta Sync 消息应用到运行时插件。
type Applier struct {
	mu            sync.RWMutex
	plugin        agentruntime.RuntimePlugin
	logger        *slog.Logger
	baseVersion   int64
	lastApplied   time.Time
	applyCount    int64
	failCount     int64
}

// NewApplier 创建 DeltaApplier。
func NewApplier(plugin agentruntime.RuntimePlugin, logger *slog.Logger) *Applier {
	return &Applier{
		plugin: plugin,
		logger: logger,
	}
}

// SetBaseVersion 设置基线版本（全量同步后调用）。
func (a *Applier) SetBaseVersion(version int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.baseVersion = version
	a.logger.Info("delta sync baseline set", "version", version)
}

// BaseVersion 返回当前基线版本。
func (a *Applier) BaseVersion() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.baseVersion
}

// Apply 应用一个 Delta Sync 消息。
// 返回 (ack, needFullSync)：
//   - ack: 应用结果确认
//   - needFullSync: true 表示版本跳跃过大，需要全量同步
func (a *Applier) Apply(ctx context.Context, d *Sync) (*Ack, bool) {
	start := time.Now()

	a.mu.Lock()
	currentBase := a.baseVersion
	a.mu.Unlock()

	// 版本跳跃检测：Delta 版本必须是 baseVersion+1，否则需要全量同步
	if d.ConfigVersion > 0 && currentBase > 0 {
		expected := currentBase + 1
		if d.ConfigVersion != expected {
			a.logger.Warn("delta version jump detected, need full sync",
				"expected", expected,
				"received", d.ConfigVersion,
				"gap", d.ConfigVersion-currentBase,
			)
			a.failCount++
			return &Ack{
				ConfigVersion: d.ConfigVersion,
				Success:       false,
				Error:         fmt.Sprintf("version jump: expected %d, got %d", expected, d.ConfigVersion),
			}, true
		}
	}

	if d.IsEmpty() {
		a.logger.Debug("delta is empty, skipping")
		return &Ack{
			ConfigVersion:   d.ConfigVersion,
			Success:         true,
			ApplyDurationMs: time.Since(start).Milliseconds(),
		}, false
	}

	// 转换为 runtime.User 列表
	var adds []agentruntime.User
	for _, u := range d.AddUsers {
		adds = append(adds, agentruntime.User{
			Email:    u.Email,
			UUID:     u.UUID,
			Level:    u.Level,
			Password: u.Password,
			Extra:    map[string]interface{}{},
		})
		for k, v := range u.Extra {
			adds[len(adds)-1].Extra[k] = v
		}
		if u.InboundTag != "" {
			adds[len(adds)-1].Extra["inbound_tag"] = u.InboundTag
		}
	}

	var dels []string
	dels = append(dels, d.DelUsers...)

	a.logger.Info("applying delta sync",
		"adds", len(adds),
		"dels", len(dels),
		"target_version", d.ConfigVersion,
	)

	// 通过 RuntimePlugin.UpdateUsers 热更用户
	if err := a.plugin.UpdateUsers(ctx, adds, dels); err != nil {
		a.failCount++
		a.logger.Error("delta apply failed", "error", err)
		return &Ack{
			ConfigVersion: d.ConfigVersion,
			Success:       false,
			Error:         err.Error(),
		}, false
	}

	// 更新基线版本
	a.mu.Lock()
	a.baseVersion = d.ConfigVersion
	a.lastApplied = time.Now()
	a.applyCount++
	a.mu.Unlock()

	dur := time.Since(start)
	a.logger.Info("delta sync applied successfully",
		"version", d.ConfigVersion,
		"adds", len(adds),
		"dels", len(dels),
		"duration_ms", dur.Milliseconds(),
	)

	return &Ack{
		ConfigVersion:   d.ConfigVersion,
		Success:         true,
		ApplyDurationMs: dur.Milliseconds(),
	}, false
}

// Stats 返回 Delta 应用统计。
func (a *Applier) Stats() (applyCount, failCount int64, lastApplied time.Time) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.applyCount, a.failCount, a.lastApplied
}
