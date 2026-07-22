package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/airport-panel/node-agent/internal/client"
	agentruntime "github.com/airport-panel/node-agent/internal/runtime"
)

// startTrafficReportLoop 启动流量统计上报循环。
//
// 60s 定时循环：
//   - 从原生运行时（NativeXray）的 StatsService 读取 per-user 流量（Reset=true 增量）
//   - 通过 POST /api/v1/agent/traffic/report 上报到 traffic-service
//   - 失败仅记录警告，不阻断主流程
//
// 仅在 useNative=true 时生效（子进程模式无 StatsService 接口）。
func (a *Agent) startTrafficReportLoop(ctx context.Context) {
	if !a.useNative {
		a.logger.Info("traffic report loop disabled (non-native runtime mode)")
		return
	}
	a.goTrack(func() {
		trafficTicker := time.NewTicker(60 * time.Second)
		defer trafficTicker.Stop()
		// 首次延迟 15s 启动，等待 xray API 就绪
		time.Sleep(15 * time.Second)
		a.reportTraffic(ctx)
		for {
			select {
			case <-ctx.Done():
				a.logger.Info("traffic report loop stopping")
				return
			case <-trafficTicker.C:
				a.reportTraffic(ctx)
			}
		}
	})
}

// reportTraffic 从原生运行时读取 per-user 流量并上报到 traffic-service。
//
// P0 容错策略：使用非破坏性读取（GetTrafficStatsNoReset），跟踪基线计算增量。
//   - 读取当前累计值（不清零计数器）
//   - delta = current - baseline（首次 baseline=0，即全量上报）
//   - 上报成功后更新 baseline = current
//   - 上报失败时 baseline 不变，下次读取自动包含未上报的流量
//   - 检测计数器重置（current < baseline）时将 current 作为全量增量
func (a *Agent) reportTraffic(ctx context.Context) {
	// T06: 防止 gracefulShutdownAll 与心跳循环并发调用 reportTraffic
	a.trafficReportMu.Lock()
	defer a.trafficReportMu.Unlock()

	if a.runtimePlugin == nil {
		return
	}

	// 非破坏性读取当前累计值
	currentStats, err := a.runtimePlugin.GetTrafficStatsNoReset(ctx)
	if err != nil {
		a.logger.Warn("failed to get traffic stats (no reset)", "error", err)
		return
	}
	if len(currentStats) == 0 {
		a.logger.Debug("no traffic stats to report")
		return
	}

	a.trafficBaselineMu.Lock()
	baseline := a.trafficBaseline
	a.trafficBaselineMu.Unlock()

	if baseline == nil {
		baseline = make(map[string][2]int64)
	}

	// 计算增量并构建上报项
	reports := make([]client.TrafficReportItemReq, 0, len(currentStats))
	newBaseline := make(map[string][2]int64, len(currentStats))
	for email, stat := range currentStats {
		// 优先用 UUID 作为凭证，退化为 email
		credential := stat.UUID
		if credential == "" {
			credential = email
		}

		// 记录新基线（无论上报是否成功，都记录当前值供下次计算）
		// 注意：如果上报失败，我们不会更新 a.trafficBaseline，所以下次仍用旧基线
		newBaseline[credential] = [2]int64{stat.Upload, stat.Download}

		// 计算增量
		var lastUp, lastDown int64
		if last, ok := baseline[credential]; ok {
			lastUp = last[0]
			lastDown = last[1]
		}

		deltaUp := stat.Upload - lastUp
		deltaDown := stat.Download - lastDown

		// 检测计数器重置（内核重启导致累计值归零）
		if deltaUp < 0 {
			deltaUp = stat.Upload
		}
		if deltaDown < 0 {
			deltaDown = stat.Download
		}

		if deltaUp == 0 && deltaDown == 0 {
			continue
		}

		reports = append(reports, client.TrafficReportItemReq{
			Credential:    credential,
			UploadBytes:   deltaUp,
			DownloadBytes: deltaDown,
			Timestamp:     time.Now().Format(time.RFC3339),
		})
	}

	if len(reports) == 0 {
		return
	}

	// P0+: 将本次增量流量存入持久化缓冲区
	// 上报成功后清空，失败时保留并持久化到 traffic_buffer.json
	if a.trafficBuffer != nil {
		bufferStats := make(map[string]agentruntime.TrafficStat, len(reports))
		for _, r := range reports {
			bufferStats[r.Credential] = agentruntime.TrafficStat{
				Email:    r.Credential,
				UUID:     r.Credential,
				Upload:   r.UploadBytes,
				Download: r.DownloadBytes,
			}
		}
		a.trafficBuffer.Add(bufferStats)
	}

	// 合并 pending + 当前增量作为最终上报内容
	var finalReports []client.TrafficReportItemReq
	if a.trafficBuffer != nil {
		pending := a.trafficBuffer.Pending()
		finalReports = make([]client.TrafficReportItemReq, 0, len(pending))
		for cred, stat := range pending {
			finalReports = append(finalReports, client.TrafficReportItemReq{
				Credential:    cred,
				UploadBytes:   stat.Upload,
				DownloadBytes: stat.Download,
				Timestamp:     time.Now().Format(time.RFC3339),
			})
		}
	} else {
		finalReports = reports
	}

	req := &client.TrafficReportRequest{
		ServerCode: a.cfg.ServerCode,
		Reports:    finalReports,
	}
	if err := a.httpClient.ReportTraffic(ctx, req); err != nil {
		a.logger.Warn("failed to report traffic, buffered for retry",
			"error", err, "user_count", len(finalReports))
		// 不更新基线，不清空 buffer，下次自动合并重试
		// 持久化 pending 到文件，防止 Agent 重启丢失
		if a.trafficSaveDeb != nil {
			a.trafficSaveDeb.Trigger()
		}
		return
	}

	// 上报成功：更新基线 + 持久化 baseline + 清空 buffer
	a.trafficBaselineMu.Lock()
	a.trafficBaseline = newBaseline
	a.trafficBaselineMu.Unlock()
	a.saveTrafficBaseline() // P0++: 持久化 baseline，避免 Agent 重启（内核未重启）时重复上报
	if a.trafficBuffer != nil {
		a.trafficBuffer.Clear()
	}

	a.logger.Info("traffic reported", "user_count", len(finalReports))
}

// loadTrafficBaseline 从持久化文件恢复流量基线（Agent 启动时调用）。
// baseline 持久化解决：Agent 进程重启但内核未重启时，内存 baseline 丢失导致下次全量重复上报。
// 内核重启（累计值归零）时，current < baseline 会触发退化全量（reportTraffic 中 delta<0 分支），安全。
func (a *Agent) loadTrafficBaseline() {
	if a.trafficBaselinePath == "" {
		return
	}
	data, err := os.ReadFile(a.trafficBaselinePath)
	if err != nil {
		if !os.IsNotExist(err) {
			a.logger.Warn("failed to read traffic baseline file", "path", a.trafficBaselinePath, "error", err)
		}
		return
	}
	var baseline map[string][2]int64
	if err := json.Unmarshal(data, &baseline); err != nil {
		a.logger.Warn("failed to parse traffic baseline file, will start with empty baseline",
			"path", a.trafficBaselinePath, "error", err)
		return
	}
	a.trafficBaselineMu.Lock()
	a.trafficBaseline = baseline
	a.trafficBaselineMu.Unlock()
	a.logger.Info("traffic baseline restored", "user_count", len(baseline))
}

// saveTrafficBaseline 持久化流量基线到文件（上报成功后调用）。
// 原子写入：先写 .tmp 再 rename，避免写入中断导致文件损坏。
func (a *Agent) saveTrafficBaseline() {
	if a.trafficBaselinePath == "" {
		return
	}
	a.trafficBaselineMu.Lock()
	baseline := a.trafficBaseline
	a.trafficBaselineMu.Unlock()
	if len(baseline) == 0 {
		_ = os.Remove(a.trafficBaselinePath)
		return
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		a.logger.Warn("failed to marshal traffic baseline", "error", err)
		return
	}
	tmpPath := a.trafficBaselinePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		a.logger.Warn("failed to write traffic baseline tmp file", "path", tmpPath, "error", err)
		return
	}
	if err := os.Rename(tmpPath, a.trafficBaselinePath); err != nil {
		a.logger.Warn("failed to rename traffic baseline file", "path", a.trafficBaselinePath, "error", err)
	}
}
