package main

import (
	"context"
	"encoding/json"
	"time"
)

// runWatchdog 进程 watchdog（从 main() 内联 goroutine 提取为方法）。
// 每 30s 检查 xray/sing-box 进程状态，崩溃自动重启。
// 重启成功后调用 maybeRestartSingbox 恢复 sing-box 内核（如有缓存配置）。
func (a *Agent) runWatchdog(ctx context.Context) {
	watchTicker := time.NewTicker(30 * time.Second)
	defer watchTicker.Stop()
	restartCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-watchTicker.C:
			status, err := a.runtimeExec.Status(ctx)
			if err != nil {
				a.logger.Warn("watchdog: failed to get runtime status", "error", err)
				continue
			}
			if status == nil || !status.Running {
				restartCount++
				configPath := a.cfg.ConfigFilePath()
				a.logger.Error("watchdog: runtime crashed, attempting restart",
					"restart_count", restartCount, "config_path", configPath)
				if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
					a.logger.Error("watchdog: restart failed", "error", err, "restart_count", restartCount)
				} else {
					a.logger.Info("watchdog: runtime restarted successfully", "restart_count", restartCount)
					// ★新增：watchdog 重启 xray 后，恢复缓存的 sing-box 配置
					a.maybeRestartSingbox(ctx)
				}
			}
		}
	}
}

// maybeRestartSingbox 在 watchdog 重启 xray 后恢复缓存的 sing-box 配置。
// 解决问题：原版本 watchdog 重启 xray 后 sing-box 不会自动恢复，
// 导致双内核架构下 sing-box 节点（Hysteria2/TUIC）在 xray 崩溃后永久下线。
func (a *Agent) maybeRestartSingbox(ctx context.Context) {
	if a.lastSingboxConfig == nil || !a.useNative || a.pluginAdapter == nil {
		return
	}
	sbBytes, err := json.Marshal(a.lastSingboxConfig)
	if err != nil {
		a.logger.Error("watchdog: marshal sing-box config failed", "error", err)
		return
	}
	if err := a.pluginAdapter.StartNative(ctx, sbBytes); err != nil {
		a.logger.Error("watchdog: restart sing-box failed", "error", err)
	} else {
		a.logger.Info("watchdog: sing-box restarted after xray recovery")
	}
}
