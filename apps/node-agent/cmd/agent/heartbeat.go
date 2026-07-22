package main

import (
	"context"
	"fmt"
	mrand "math/rand"
	"runtime"
	"strconv"
	"time"

	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/upgrader"
	pb "github.com/airport-panel/proto/agent/v1"
)

func (a *Agent) sendHeartbeat(ctx context.Context, currentVersion, runtimeStatus, runtimeVersion string, pid int) (*pb.HeartbeatAck, error) {
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

	hb := &pb.AgentMessage{
		Seq:       a.nextSeq(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.AgentMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				ConfigVersion: configVersionNum,
				Kernel: &pb.KernelInfo{
					Type:          kernelType,
					Version:       runtimeVersion,
					ConfigVersion: currentVersion,
					Running:       running,
				},
				Channel: &pb.ChannelHealth{
					State: chanState,
				},
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
	var channelHealthReport *client.ChannelHealthReport
	if chanHealth.ActiveChannel != "unknown" {
		channelHealthReport = &client.ChannelHealthReport{
			ActiveChannel: chanHealth.ActiveChannel,
			ChannelState:  chanHealth.State,
			FailCount1h:   chanHealth.FailCount,
		}
	}

	xrayPort := parsePortFromEndpoint(a.cfg.XrayAPIEndpoint)
	singboxPort := parsePortFromEndpoint(a.cfg.SingboxClashEndpoint)

	hbReq := &client.HeartbeatRequest{
		ServerCode:      a.cfg.ServerCode,
		Timestamp:       time.Now(),
		ConfigVersion:   a.currentVersion,
		XrayAPIPort:     xrayPort,
		SingboxClashPort: singboxPort,
		ChannelHealth:   channelHealthReport,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		AgentVersion:    AgentVersion,
		RuntimeStatus:   runtimeStatusStr,
		RuntimeVersion:  runtimeVersionStr,
		Pid:             pid,
	}

	var hbResp *pb.HeartbeatAck
	var extraActions []pb.HeartbeatAction
	if a.channelsAvailable {
		resp, err := a.sendHeartbeat(ctx, a.currentVersion, runtimeStatusStr, runtimeVersionStr, pid)
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
		"channel_state", chanHealth.State)

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
