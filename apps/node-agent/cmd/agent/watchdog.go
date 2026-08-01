package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const xrayCacheFileName = "xray-cache.json"

// xrayCachePath 返回 xray 配置磁盘缓存路径（与主配置文件同目录）。
// native 模式下 _xray_config 只存在于内存，进程重启后需从缓存恢复辅内核。
func (a *Agent) xrayCachePath() string {
	return filepath.Join(filepath.Dir(a.cfg.ConfigFilePath()), xrayCacheFileName)
}

// saveXrayConfigCache 原子持久化最近一次成功应用的 xray 配置。
func (a *Agent) saveXrayConfigCache(cfg map[string]interface{}) {
	if cfg == nil || !a.useNative {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	path := a.xrayCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		a.logger.Warn("save xray cache failed", "error", err)
	}
}

// restoreXrayFromCache Agent 进程重启后从磁盘缓存恢复 xray 辅内核。
// 解决：版本与面板一致时不触发 reload，内存中的 xray 配置不会重新获取，
// 导致 xray 节点在 Agent 自升级/重启后永久下线。
func (a *Agent) restoreXrayFromCache(ctx context.Context) {
	if a.lastXrayConfig != nil || !a.useNative || a.pluginAdapter == nil {
		return
	}
	data, err := os.ReadFile(a.xrayCachePath())
	if err != nil {
		if !os.IsNotExist(err) {
			a.logger.Warn("read xray cache failed", "error", err)
		}
		return
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil || len(cfg) == 0 {
		a.logger.Warn("parse xray cache failed", "error", err)
		return
	}
	xrayBytes, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	if err := a.pluginAdapter.StartNative(ctx, xrayBytes); err != nil {
		a.logger.Warn("restore xray from cache failed", "error", err)
		return
	}
	a.lastXrayConfig = cfg
	a.logger.Info("xray restored from cache", "xray_config_size", len(xrayBytes))
}

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
