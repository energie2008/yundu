package main

import (
	"context"
	"encoding/json"
	"time"
)

// runWarpStatusReporter P3-E: 每 5 分钟采集 WARP 状态并上报到面板。
// 仅在 warpMgr 已初始化时运行。
func (a *Agent) runWarpStatusReporter(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if a.warpMgr == nil {
				return
			}
			status := a.warpMgr.GetStatus()
			if status.Status != "running" {
				a.logger.Warn("warp status reporter: warp not running, attempting reconnect",
					"status", status.Status)
				if err := a.warpMgr.Connect(); err != nil {
					a.logger.Error("warp reconnect failed", "error", err)
					continue
				}
				status = a.warpMgr.GetStatus()
			}
			if err := a.warpMgr.ReportToPanel(ctx, status); err != nil {
				a.logger.Warn("warp status report to panel failed", "error", err)
			} else {
				a.logger.Info("warp status reported to panel",
					"status", status.Status, "warp_ip", status.WarpIP, "latency_ms", status.LatencyMs)
			}
		case <-ctx.Done():
			return
		}
	}
}

// runWatchdog 进程 watchdog（从 main() 内联 goroutine 提取为方法）。
// 每 30s 检查 sing-box/xray 进程状态，崩溃自动重启。
// P2 翻转：sing-box 为主内核，重启后调用 maybeRestartXray 恢复 xray 内核（如有缓存配置）。
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
					// P2 翻转：watchdog 重启 sing-box 后，恢复缓存的 xray 配置
					a.maybeRestartXray(ctx)
				}
			}
		}
	}
}

// maybeRestartXray 在 watchdog 重启 sing-box 后恢复缓存的 xray 配置。P2 翻转。
// 解决问题：原版本 watchdog 重启 sing-box 后 xray 不会自动恢复，
// 导致双内核架构下 xray 节点（XHTTP）在 sing-box 崩溃后永久下线。
func (a *Agent) maybeRestartXray(ctx context.Context) {
	if a.lastXrayConfig == nil || !a.useNative || a.pluginAdapter == nil {
		return
	}
	xrayBytes, err := json.Marshal(a.lastXrayConfig)
	if err != nil {
		a.logger.Error("watchdog: marshal xray config failed", "error", err)
		return
	}
	if err := a.pluginAdapter.StartNative(ctx, xrayBytes); err != nil {
		a.logger.Error("watchdog: restart xray failed", "error", err)
	} else {
		a.logger.Info("watchdog: xray restarted after sing-box recovery")
	}
}
