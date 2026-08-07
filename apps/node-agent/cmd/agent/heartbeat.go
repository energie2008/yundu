package main

import (
	"bufio"
	"context"
	"fmt"
	mrand "math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/upgrader"
	pb "github.com/airport-panel/proto/agent/v1"
)

func (a *Agent) sendHeartbeat(ctx context.Context, currentVersion, runtimeStatus, runtimeVersion string, pid int, onlineUsers int) (*pb.HeartbeatAck, error) {
	configVersionNum := int64(0)
	if currentVersion != "" {
		if v, err := strconv.ParseInt(currentVersion, 10, 64); err == nil {
			configVersionNum = v
		}
	}

	kernelType := pb.KernelType_KERNEL_TYPE_XRAY
	if a.cfg.RuntimeType == "sing-box" {
		kernelType = pb.KernelType_KERNEL_TYPE_SINGBOX
	}

	running := runtimeStatus == "running"

	// P1-4: 双核状态上报 —— 构建辅内核 KernelInfo
	var secondaryKernel *pb.KernelInfo
	if a.runtimePlugin != nil && a.useNative {
		if mp, ok := a.runtimePlugin.(interface {
			GetSecondaryKernelStatus() *pb.KernelInfo
		}); ok {
			secondaryKernel = mp.GetSecondaryKernelStatus()
		}
	}

	chanHealth := a.cm.GetHealthStatus()
	var chanState pb.ChannelState
	switch chanHealth.State {
	case "healthy":
		chanState = pb.ChannelState_CHANNEL_STATE_HEALTHY
	case "degraded":
		chanState = pb.ChannelState_CHANNEL_STATE_DEGRADED
	case "unhealthy":
		chanState = pb.ChannelState_CHANNEL_STATE_UNHEALTHY
	default:
		chanState = pb.ChannelState_CHANNEL_STATE_UNKNOWN
	}

	load := collectServerLoad()

	// P2-I: 通过 Heartbeat.nodes 承载服务器级聚合在线人数。
	// proto 的 ChannelHealth 没有 online_users 字段，而 NodeStatus.online_users 已存在，
	// 因此用 nodes[0] 携带聚合值，服务端 gRPC handler 遍历 nodes 求和写入 channel_health_current。
	// 这打通了 gRPC 主路径的在线人数上报（此前仅 HTTP fallback 路径能上报，导致走 gRPC 的节点恒为 0）。
	// 即使 onlineUsers=0 也始终携带 nodes，避免服务端沿用旧值（在线人数归零时也要刷新）。
	nodeStatuses := []*pb.NodeStatus{{OnlineUsers: int64(onlineUsers)}}

	hb := &pb.AgentMessage{
		Seq:       a.nextSeq(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.AgentMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				ConfigVersion: configVersionNum,
				Kernel: &pb.KernelInfo{
					Type:            kernelType,
					Version:         runtimeVersion,
					ConfigVersion:   currentVersion,
					Running:         running,
					SecondaryKernel: secondaryKernel, // P1-4: 辅内核状态
				},
				Channel: &pb.ChannelHealth{
					State: chanState,
				},
				Load:  load,
				Nodes: nodeStatuses,
			},
		},
	}

	respCh := make(chan *pb.PanelMessage, 1)
	a.mu.Lock()
	a.pending[hb.Seq] = &pendingRequest{ch: respCh, ctx: ctx}
	a.mu.Unlock()

	if err := a.cm.Send(hb); err != nil {
		a.mu.Lock()
		delete(a.pending, hb.Seq)
		a.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respCh:
		hbAck := resp.GetHeartbeatAck()
		if hbAck == nil {
			return nil, fmt.Errorf("expected HeartbeatAck")
		}
		return hbAck, nil
	case <-ctx.Done():
		a.mu.Lock()
		delete(a.pending, hb.Seq)
		a.mu.Unlock()
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		a.mu.Lock()
		delete(a.pending, hb.Seq)
		a.mu.Unlock()
		return nil, fmt.Errorf("heartbeat timeout")
	}
}

func convertHeartbeatResponse(resp *client.HeartbeatResponse) (*pb.HeartbeatAck, []pb.HeartbeatAction) {
	action := pb.HeartbeatAction_HEARTBEAT_ACTION_NONE
	if resp.Action != nil {
		switch *resp.Action {
		case "reload":
			action = pb.HeartbeatAction_HEARTBEAT_ACTION_RELOAD
		case "restart":
			action = pb.HeartbeatAction_HEARTBEAT_ACTION_RESTART
		case "maintenance":
			action = pb.HeartbeatAction_HEARTBEAT_ACTION_MAINTENANCE
		case "upgrade":
			action = pb.HeartbeatAction_HEARTBEAT_ACTION_UPGRADE
		}
	}
	// P1: 解析 extra_actions（附加动作，与主 action 并行执行）
	var extraActions []pb.HeartbeatAction
	for _, a := range resp.ExtraActions {
		switch a {
		case "sync_external_resources":
			extraActions = append(extraActions, pb.HeartbeatAction_HEARTBEAT_ACTION_SYNC_EXTERNAL_RESOURCES)
		}
	}
	latestVersion := int64(0)
	if resp.TargetConfigVersion != nil {
		if v, err := strconv.ParseInt(*resp.TargetConfigVersion, 10, 64); err == nil {
			latestVersion = v
		}
	}
	return &pb.HeartbeatAck{
		Action:              action,
		LatestConfigVersion: latestVersion,
		ServerTime:          resp.CurrentTime,
	}, extraActions
}

func (a *Agent) processHeartbeatResponse(ctx context.Context, hbAck *pb.HeartbeatAck, currentVersion *string) {
	switch hbAck.Action {
	case pb.HeartbeatAction_HEARTBEAT_ACTION_NONE:
		return
	case pb.HeartbeatAction_HEARTBEAT_ACTION_RELOAD:
		targetVersionNum := hbAck.LatestConfigVersion
		targetVersion := strconv.FormatInt(targetVersionNum, 10)
		if targetVersion == *currentVersion {
			a.logger.Debug("config version already current, skip reload", "version", targetVersion)
			return
		}
		a.logger.Info("config reload triggered", "current", *currentVersion, "target", targetVersion)
		// Jitter Pull: 0-3000ms 随机延迟，避免心跳返回 RELOAD 时所有节点同时拉取配置
		jitter := time.Duration(mrand.Intn(3000)) * time.Millisecond
		a.logger.Debug("applying jitter before config pull", "delay", jitter)
		time.Sleep(jitter)
		a.applyConfig(ctx, targetVersion, currentVersion)
		// P1 修复：配置重载后自动触发 nginx vhost 同步。
		// protobuf HeartbeatAck 没有 ExtraActions 字段，WS/gRPC 通道只能携带单个 Action。
		// 因此在 agent 端 RELOAD 后自动触发 nginx 同步，确保所有通道都能即时同步外部资源。
		if a.nginxReconciler != nil {
			go a.nginxReconciler.TriggerSync(ctx)
		}
	case pb.HeartbeatAction_HEARTBEAT_ACTION_RESTART:
		a.logger.Info("runtime restart requested")
		configPath := a.cfg.ConfigFilePath()
		if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
			a.logger.Error("restart failed", "error", err)
		}
	case pb.HeartbeatAction_HEARTBEAT_ACTION_MAINTENANCE:
		a.logger.Info("maintenance mode requested")
	case pb.HeartbeatAction_HEARTBEAT_ACTION_UPGRADE:
		a.logger.Info("agent upgrade available")
		// P2: 原生模式下触发 self-upgrader 立即检查
		if a.selfUpgrader != nil {
			go func() {
				if err := a.selfUpgrader.CheckNow(ctx); err != nil {
					a.logger.Warn("self-upgrade check failed", "error", err)
				}
			}()
		}
	case pb.HeartbeatAction_HEARTBEAT_ACTION_SYNC_EXTERNAL_RESOURCES:
		// P1: 面板通知有外部资源（nginx vhost/证书）变更，立即触发同步
		// 消除 nginx reconciler 独立 30s 轮询的延迟，实现"保存即下发"
		a.logger.Info("external resources sync triggered")
		if a.nginxReconciler != nil {
			go a.nginxReconciler.TriggerSync(ctx)
		}
	}
}

// runHeartbeat 心跳循环（从 main() 内联闭包提取，逻辑完全不变）。
// 每 HeartbeatSeconds 秒发送一次心跳，启动时立即发一次。
func (a *Agent) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(HeartbeatSeconds * time.Second)
	defer ticker.Stop()
	a.sendHeartbeatOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			a.logger.Info("heartbeat goroutine stopping")
			return
		case <-ticker.C:
			a.sendHeartbeatOnce(ctx)
		}
	}
}

// sendHeartbeatOnce 发送单次心跳（原 main() 中的 sendHeartbeat 闭包逻辑）。
func (a *Agent) sendHeartbeatOnce(ctx context.Context) {
	runtimeStatus, err := a.runtimeExec.Status(ctx)
	runtimeStatusStr := "stopped"
	runtimeVersionStr := ""
	pid := 0
	if err == nil && runtimeStatus != nil {
		if runtimeStatus.Running {
			runtimeStatusStr = "running"
		}
		runtimeVersionStr = runtimeStatus.Version
		pid = runtimeStatus.PID
	} else if err != nil {
		a.logger.Error("failed to get runtime status", "error", err)
	}

	chanHealth := a.cm.GetHealthStatus()
	// P2-I: 获取在线用户数。
	// 优先使用 ActiveUserCount()（基于连接生命周期：connect +1 / close -1），
	// 精确反映实时在线状态，不受流量上报周期和基线污染影响。
	// 回退到 GetTrafficStatsNoReset（流量计数法，近似值）用于不支持连接追踪的内核。
	onlineUsers := 0
	if a.runtimePlugin != nil && a.useNative {
		if counter, ok := a.runtimePlugin.(interface{ ActiveUserCount() int }); ok {
			onlineUsers = counter.ActiveUserCount()
		} else if stats, err := a.runtimePlugin.GetTrafficStatsNoReset(context.Background()); err == nil {
			onlineUsers = len(stats)
		}
	}
	var channelHealthReport *client.ChannelHealthReport
	if chanHealth.ActiveChannel != "unknown" {
		channelHealthReport = &client.ChannelHealthReport{
			ActiveChannel: chanHealth.ActiveChannel,
			ChannelState:  chanHealth.State,
			FailCount1h:   chanHealth.FailCount,
			OnlineUsers:   onlineUsers,
		}
	}

	xrayPort := parsePortFromEndpoint(a.cfg.XrayAPIEndpoint)
	singboxPort := parsePortFromEndpoint(a.cfg.SingboxClashEndpoint)

	httpLoad := collectServerLoad()
	cpuPct := float64(httpLoad.CpuPercent)
	memPct := float64(httpLoad.MemPercent)
	diskPct := float64(httpLoad.DiskPercent)

	hbReq := &client.HeartbeatRequest{
		ServerCode:       a.cfg.ServerCode,
		Timestamp:        time.Now(),
		ConfigVersion:    a.currentVersion,
		XrayAPIPort:      xrayPort,
		SingboxClashPort: singboxPort,
		OnlineUsers:      onlineUsers,
		ChannelHealth:    channelHealthReport,
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		AgentVersion:     AgentVersion,
		RuntimeStatus:    runtimeStatusStr,
		RuntimeVersion:   runtimeVersionStr,
		Pid:              pid,
		CPUPercent:       &cpuPct,
		MemPercent:       &memPct,
		DiskPercent:      &diskPct,
		Metrics: map[string]interface{}{
			"cpu_percent":      httpLoad.CpuPercent,
			"mem_percent":      httpLoad.MemPercent,
			"mem_total_mb":     httpLoad.MemTotalMb,
			"mem_used_mb":      httpLoad.MemUsedMb,
			"disk_percent":     httpLoad.DiskPercent,
			"disk_total_gb":    httpLoad.DiskTotalGb,
			"disk_used_gb":     httpLoad.DiskUsedGb,
			"network_in_kbps":  httpLoad.NetworkInKbps,
			"network_out_kbps": httpLoad.NetworkOutKbps,
			"uptime_seconds":   httpLoad.UptimeSeconds,
			"load_1":           httpLoad.Load_1,
			"goroutines":       httpLoad.Goroutines,
		},
	}

	var hbResp *pb.HeartbeatAck
	var extraActions []pb.HeartbeatAction
	if a.channelsAvailable {
		resp, err := a.sendHeartbeat(ctx, a.currentVersion, runtimeStatusStr, runtimeVersionStr, pid, onlineUsers)
		if err != nil {
			a.logger.Warn("protobuf heartbeat failed, using HTTP fallback", "error", err)
			fallbackResp, fbErr := a.httpClient.Heartbeat(ctx, hbReq)
			if fbErr != nil {
				a.logger.Error("fallback heartbeat also failed", "error", fbErr)
				return
			}
			hbResp, extraActions = convertHeartbeatResponse(fallbackResp)
		} else {
			hbResp = resp
		}
	} else {
		fallbackResp, fbErr := a.httpClient.Heartbeat(ctx, hbReq)
		if fbErr != nil {
			a.logger.Error("HTTP heartbeat failed", "error", fbErr)
			return
		}
		hbResp, extraActions = convertHeartbeatResponse(fallbackResp)
	}

	a.logger.Info("heartbeat sent",
		"response_action", hbResp.Action,
		"extra_actions", len(extraActions),
		"channel", chanHealth.ActiveChannel,
		"channel_state", chanHealth.State,
		"online_users", onlineUsers)

	// P1 fix: 每次心跳成功也清除升级 sentinel，防止健康进程被误回滚
	if err := upgrader.CommitUpgradeHealthy(""); err != nil {
		a.logger.Debug("clear upgrade sentinel after heartbeat (ok if not upgrading)", "error", err)
	}

	a.processHeartbeatResponse(ctx, hbResp, &a.currentVersion)
	// P1: 处理附加动作（如 sync_external_resources），与主 action 并行执行
	for _, ea := range extraActions {
		a.processHeartbeatResponse(ctx, &pb.HeartbeatAck{Action: ea, LatestConfigVersion: hbResp.LatestConfigVersion, ServerTime: hbResp.ServerTime}, &a.currentVersion)
	}
}

// collectServerLoad 收集服务器系统负载指标（CPU/内存/磁盘/网络/uptime/loadavg/goroutines）
// 纯标准库实现，读取 /proc 文件系统，不依赖外部包
func collectServerLoad() *pb.ServerLoad {
	load := &pb.ServerLoad{}

	// 读取 /proc/loadavg 获取系统负载
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			var load1, load5, load15 float32
			fmt.Sscanf(fields[0], "%f", &load1)
			fmt.Sscanf(fields[1], "%f", &load5)
			fmt.Sscanf(fields[2], "%f", &load15)
			load.Load_1 = load1
			load.Load_5 = load5
			load.Load_15 = load15
			// 近似 CPU 使用率: load1 / CPU 核心数 * 100
			numCPU := float32(runtime.NumCPU())
			if numCPU > 0 {
				cpuPct := load1 * 100 / numCPU
				if cpuPct > 100 {
					cpuPct = 100
				}
				load.CpuPercent = cpuPct
			}
		}
	}

	// 读取 /proc/meminfo 获取内存信息
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		var memTotal, memAvailable int64
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %d", &memTotal)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %d", &memAvailable)
			}
		}
		memTotalMB := memTotal / 1024
		memUsedMB := (memTotal - memAvailable) / 1024
		load.MemTotalMb = memTotalMB
		load.MemUsedMb = memUsedMB
		if memTotal > 0 {
			load.MemPercent = float32(memUsedMB) * 100 / float32(memTotalMB)
		}
	}

	// 磁盘使用率（根分区）
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		diskTotalGB := int64(stat.Blocks) * int64(stat.Bsize) / (1024 * 1024 * 1024)
		diskFreeGB := int64(stat.Bfree) * int64(stat.Bsize) / (1024 * 1024 * 1024)
		diskUsedGB := diskTotalGB - diskFreeGB
		load.DiskTotalGb = diskTotalGB
		load.DiskUsedGb = diskUsedGB
		if diskTotalGB > 0 {
			load.DiskPercent = float32(diskUsedGB) * 100 / float32(diskTotalGB)
		}
	}

	// uptime（秒）
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		var uptime float64
		fmt.Sscanf(string(data), "%f", &uptime)
		load.UptimeSeconds = int64(uptime)
	}

	// goroutines
	load.Goroutines = int64(runtime.NumGoroutine())

	return load
}
