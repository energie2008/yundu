package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	agentconfig "github.com/airport-panel/node-agent/internal/config"
	"github.com/airport-panel/node-agent/internal/audit"
	"github.com/airport-panel/node-agent/internal/cert"
	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/delta"
	"github.com/airport-panel/node-agent/internal/executor"
	"github.com/airport-panel/node-agent/internal/firewall"
	"github.com/airport-panel/node-agent/internal/hotdiff"
	"github.com/airport-panel/node-agent/internal/nginx"
	"github.com/airport-panel/node-agent/internal/pipeline"
	"github.com/airport-panel/node-agent/internal/prober"
	"github.com/airport-panel/node-agent/internal/resource"
	agentruntime "github.com/airport-panel/node-agent/internal/runtime"
	"github.com/airport-panel/node-agent/internal/transport"
	"github.com/airport-panel/node-agent/internal/upgrader"
	"github.com/airport-panel/node-agent/internal/validator"
	"github.com/airport-panel/node-agent/internal/warp"
	pb "github.com/airport-panel/proto/agent/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	ServiceName      = "node-agent"
	AgentVersion     = "0.2.12"
	HeartbeatSeconds = 10
	DefaultGRPCPort  = 9082
)

type pendingRequest struct {
	ch   chan *pb.PanelMessage
	ctx  context.Context
}

type Agent struct {
	cfg          *agentconfig.Config
	logger       *slog.Logger
	runtimeExec  executor.RuntimeExecutor
	cm           *transport.ChannelManager
	httpClient   *client.Client
	logCollector *transport.LogCollector
	firewallMgr  firewall.Manager
	hostname     string
	seq          atomic.Int64
	mu           sync.Mutex
	pending      map[int64]*pendingRequest
	serverID     string
	sessionToken string
	authDone     chan struct{}
	currentVersion string
	// useNative 表示是否使用原生内嵌运行时（YUNDU_NATIVE_RUNTIME=true）
	useNative bool
	// runtimePlugin 原生内嵌运行时插件（useNative=true 时有效）
	runtimePlugin agentruntime.RuntimePlugin
	// pluginAdapter 将 RuntimePlugin 适配为 RuntimeExecutor 接口
	pluginAdapter *agentruntime.PluginAdapter
	// 边缘自治全链路：LKG 回滚状态机 + deploy.lock + 健康探活 + 边缘预检
	pipeline      *pipeline.Pipeline
	healthChecker *pipeline.HealthChecker
	edgeValidator *validator.EdgeValidator
	// lastAppliedConfig P1-5: 上一次成功应用的 xray/sing-box 配置（JSON map），
	// 用于 HotDiff 比较以决定走 AlterInbound / Reload / restart。
	// 应用成功后更新，失败时保持旧值。
	lastAppliedConfig map[string]interface{}
	// P1-7: 30秒防抖重启
	// restartDebounce 计时器：RESTART_REQUIRED 请求在 30s 窗口内合并，
	// 窗口内若有新的 RESTART 请求则重置计时器，避免短时间内频繁重启。
	// 期间仍走 SIGUSR1 Reload（PID 不变）保持服务可用，计时器到期后执行一次真正的全量重启。
	restartDebounceMu      sync.Mutex
	restartDebounceTimer   *time.Timer
	restartDebouncePending bool
	// P2: Delta Sync 增量用户同步
	deltaApplier *delta.Applier
	// P2: Agent 自升级（原生模式替代 BinaryReconciler）
	selfUpgrader *upgrader.SelfUpgrader
	// P1: NginxReconciler 引用，供心跳 SYNC_EXTERNAL_RESOURCES Action 立即触发同步
	nginxReconciler *NginxReconciler
	// P3: Active Prober 主动拨测
	prober *prober.Prober
	// P1-8: DeviceEnforcer 通过 xray gRPC 执行设备数限制
	deviceEnforcer *executor.DeviceEnforcer
	// P0: 流量统计容错基线——非破坏性读取时跟踪上次上报的累计值，
	// 上报成功后更新，失败时保持不变，下次自动包含未上报流量。
	// key: credential (email 或 UUID), value: [2]int64{upload, download}
	// P0++: baseline 持久化到 traffic_baseline.json，避免 Agent 重启（内核未重启）时重复上报
	trafficBaseline     map[string][2]int64
	trafficBaselineMu   sync.Mutex
	trafficBaselinePath string
	// T06: reportTraffic 互斥锁，防止 gracefulShutdownAll 与心跳循环并发调用
	trafficReportMu sync.Mutex
	// P0+: 流量上报持久化缓冲区（借鉴 Xboard-Node RestoreTraffic）
	// 上报失败时保留未上报流量到 pending，Agent 重启后从 traffic_buffer.json 恢复
	trafficBuffer  *agentruntime.TrafficBuffer
	trafficSaveDeb *agentruntime.SaveDebouncer
	// 双内核：缓存最近成功应用的 sing-box 配置，供 debounced restart 后重新启动 sing-box 内核
	// （Reload 从文件加载的配置已剥离 _singbox_config，无法自动恢复 sing-box）
	lastSingboxConfig map[string]interface{}
	// fetchedViaPayload 标记本次applyConfig是否走加密Payload通道（由fetchConfigViaPayload设置）
	// 用于签名校验策略选择：Payload通道AES-GCM已认证，hash不匹配时warn不阻断
	fetchedViaPayload bool

	// === Phase 6 改造追加字段（不影响现有逻辑） ===
	// channelsAvailable 原为 main() 局部变量，提升为字段供 sendHeartbeatOnce 访问。
	channelsAvailable bool
	// restartCh 自升级重启信号通道，原为 main() 局部变量，提升为字段供 runAgent 监听。
	restartCh chan struct{}
	// cancelFn 用于在 gracefulShutdown 时取消所有子 goroutine 的 context。
	// 在 runAgent 中通过 context.WithCancel 创建并赋值。
	cancelFn context.CancelFunc
	// wg 统一管理子 goroutine 生命周期，gracefulShutdown 时等待所有 goroutine 退出。
	wg sync.WaitGroup

	// === 阶段 A2: 证书管理器提升为 Agent 字段 ===
	// certMgr 原为 runAgent 局部变量，提升为字段以便 fetchConfigViaPayload 消费 TLSMaterials。
	// 非空时表示启用了证书自动签发（nginx 节点）；为 nil 表示直连节点无 nginx。
	certMgr *cert.Manager

	// === Machine 模式专用开关（默认false，Node模式行为不变） ===
	// skipOwnHTTPServer 为 true 时不启动自己的 :10000 HTTP Server
	// （Machine 模式由 Orchestrator 统一启动）
	skipOwnHTTPServer bool
	// skipSelfUpgrader 为 true 时不初始化自己的 SelfUpgrader
	// （Machine 模式由 Orchestrator 统一持有）
	skipSelfUpgrader bool
	// skipSharedResources 为 true 时跳过 Nginx/Cert/Cloudflared/Firewall 等宿主机级共享资源初始化
	// （Machine 模式下这些由 Orchestrator 单例持有，避免多节点重复 ACME 请求或冲突）
	skipSharedResources bool
	// metricsRegistry 为 sub-Agent 使用的独立 Prometheus Registry
	// nil 时使用全局 DefaultRegisterer（Node 模式）
	metricsRegistry prometheus.Registerer
}

func main() {
	// main() 函数收缩为 ~80 行（flag 解析 + 配置加载 + 模式分叉）。
	// 主体逻辑见 runAgent() 方法（本文件内）。
	// 文件拆分说明：
	//   - heartbeat.go    心跳循环（sendHeartbeat/sendHeartbeatOnce/processHeartbeatResponse/runHeartbeat/convertHeartbeatResponse）
	//   - traffic.go      流量上报（startTrafficReportLoop/reportTraffic/loadTrafficBaseline/saveTrafficBaseline）
	//   - watchdog.go     进程看门狗（runWatchdog/maybeRestartSingbox）
	//   - util.go         无状态工具函数（resolve*/generateNonce/buildCapabilities/writeAtomic/readCurrentVersion 等）
	//   - control.go      yunductl 本地控制服务
	//   - machine.go      Machine 模式编排器
	// 仍在 main.go 的核心：Agent struct、认证、recvLoop/handlePanelMessage、
	//   applyConfig、applyWithHotDiff、syncLimiters、runAgent、gracefulShutdown、NewAgent。
	mode := flag.String("mode", envOr("YUNDU_MODE", "node"),
		"运行模式: node(默认) | machine(单进程多节点)")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *showVersion {
		fmt.Printf("yundu-agent %s\n", AgentVersion)
		os.Exit(0)
	}

	// 零配置部署：优先尝试 bootstrap 模式（--endpoint + --token），
	// 从面板拉取完整运行时配置；未指定时回退到原有环境变量模式。
	cfg, err := agentconfig.LoadWithBootstrap()
	if err != nil {
		slog.Error("bootstrap failed, exiting", "error", err)
		os.Exit(1)
	}
	logger := config.NewLogger(ServiceName, cfg.LogLevel)

	if cfg.BootstrapEnabled {
		logger.Info("node-agent started in bootstrap mode (zero-config deploy)",
			"panel_url", cfg.PanelURL,
			"server_code", cfg.ServerCode,
			"runtime_type", cfg.RuntimeType,
			"runtime_path", cfg.RuntimePath)
	}

	// 确保必要目录存在
	for _, dir := range []string{cfg.ConfigDir, cfg.LogDir, cfg.CertsDir()} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Error("failed to create directory", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	// P1 fix: upgrade health gate — if a previous upgrade failed to become healthy
	// within the timeout window, auto-rollback to .bak binary before proceeding.
	if upgrader.CheckAndHandleUpgradePending(logger, "") {
		logger.Warn("exiting after auto-rollback; supervisor will restart previous version")
		os.Exit(0)
	}

	// 零SSH化修复：启动时自动停止并禁用独立 yundu-xray.service，
	// 避免与内嵌 xray-core 端口冲突（443/9450 等）。
	// Agent 内嵌 xray-core 是唯一合法的 xray 运行方式，独立 systemd 服务应被接管。
	stopLegacyXrayService(logger)

	hostname, _ := os.Hostname()
	logger.Info("node-agent starting",
		"mode", *mode,
		"server_code", cfg.ServerCode,
		"panel_url", cfg.PanelURL,
		"runtime_type", cfg.RuntimeType,
		"heartbeat_interval_secs", HeartbeatSeconds,
		"hostname", hostname,
		"version", AgentVersion,
	)

	switch *mode {
	case "machine":
		// Machine 模式：单进程托管 N 节点（参见 machine.go）
		ctx, stop := signal.NotifyContext(context.Background(),
			syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer stop()
		orch := NewMachineOrchestrator(logger, cfg)
		if err := orch.Run(ctx); err != nil {
			logger.Error("machine orchestrator error", "error", err)
			os.Exit(1)
		}
	case "node":
		// 标准节点模式（原有逻辑，runAgent 内部自行处理 SIGINT/SIGTERM）
		agent := NewAgent(cfg, logger)
		agent.restartCh = make(chan struct{}, 1)
		if err := agent.Run(context.Background()); err != nil {
			logger.Error("agent error", "error", err)
			os.Exit(1)
		}
	default:
		logger.Error("unknown mode", "mode", *mode, "valid", "node|machine")
		os.Exit(1)
	}
}

func (a *Agent) Send(msg *pb.AgentMessage) error {
	return a.cm.Send(msg)
}

func (a *Agent) authenticate(ctx context.Context) error {
	nonce := generateNonce()
	ts := time.Now().Unix()
	token := resolveToken(a.cfg, a.logger)

	signPayload := fmt.Sprintf("%s%s%d", a.cfg.ServerCode, nonce, ts)
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(signPayload))
	signature := hex.EncodeToString(mac.Sum(nil))

	kernelType := pb.KernelType_KERNEL_TYPE_XRAY
	if a.cfg.RuntimeType == "sing-box" {
		kernelType = pb.KernelType_KERNEL_TYPE_SINGBOX
	}

	authReq := &pb.AgentMessage{
		Seq:       a.nextSeq(),
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.AgentMessage_Auth{
			Auth: &pb.AuthRequest{
				MachineToken:    token,
				MachineId:       a.cfg.ServerCode,
				Nonce:           nonce,
				Timestamp:       ts,
				Signature:       signature,
				PreferredKernel: kernelType,
				AgentVersion:    AgentVersion,
				Hostname:        a.hostname,
			},
		},
	}

	respCh := make(chan *pb.PanelMessage, 1)
	a.mu.Lock()
	a.pending[authReq.Seq] = &pendingRequest{ch: respCh, ctx: ctx}
	a.mu.Unlock()

	if err := a.cm.Send(authReq); err != nil {
		a.mu.Lock()
		delete(a.pending, authReq.Seq)
		a.mu.Unlock()
		return fmt.Errorf("send auth: %w", err)
	}

	select {
	case resp := <-respCh:
		authAck := resp.GetAuthAck()
		if authAck == nil {
			return fmt.Errorf("expected AuthAck, got %T", resp.Payload)
		}
		if !authAck.Ok {
			return fmt.Errorf("auth failed: %s", authAck.Error)
		}
		a.mu.Lock()
		a.serverID = authAck.ServerId
		a.sessionToken = authAck.SessionToken
		a.mu.Unlock()
		close(a.authDone)
		a.logger.Info("authenticated via channel manager", "server_id", authAck.ServerId)

		// P1 fix: auth succeeded — new binary is healthy, clear upgrade-pending sentinel
		// to prevent auto-rollback on next restart.
		if err := upgrader.CommitUpgradeHealthy(""); err != nil {
			a.logger.Warn("failed to clear upgrade-pending marker", "error", err)
		} else {
			a.logger.Debug("upgrade health gate passed (auth ok)")
		}

		return nil
	case <-ctx.Done():
		a.mu.Lock()
		delete(a.pending, authReq.Seq)
		a.mu.Unlock()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		a.mu.Lock()
		delete(a.pending, authReq.Seq)
		a.mu.Unlock()
		return fmt.Errorf("auth timeout")
	}
}

func (a *Agent) registerWithFallback(ctx context.Context) {
	rtStatus, err := a.runtimeExec.Status(ctx)
	rtVersionStr := AgentVersion
	if err == nil && rtStatus != nil && rtStatus.Version != "" && rtStatus.Version != "xray" {
		rtVersionStr = rtStatus.Version
	}
	// 安全截断：确保 runtime_version 不超过数据库 VARCHAR(64) 限制
	// 正常情况下 probeVersion 已提取简短版本号，此为兜底保护
	if len(rtVersionStr) > 64 {
		rtVersionStr = rtVersionStr[:64]
	}

	capabilities := buildCapabilities(a.cfg.RuntimeType)

	xrayPort := parsePortFromEndpoint(a.cfg.XrayAPIEndpoint)
	singboxPort := parsePortFromEndpoint(a.cfg.SingboxClashEndpoint)

	req := &client.RegisterRequest{
		RuntimeType:         a.cfg.RuntimeType,
		RuntimeVersion:      &rtVersionStr,
		ConfigSchemaVersion: "v1",
		Capabilities:        capabilities,
		XrayAPIPort:         xrayPort,
		SingboxClashPort:    singboxPort,
		Metadata: map[string]interface{}{
			"hostname":      a.hostname,
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
			"agent_version": AgentVersion,
		},
		Hostname:     a.hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: AgentVersion,
	}
	resp, err := a.httpClient.Register(ctx, req)
	if err != nil {
		a.logger.Warn("register via HTTP failed, continuing", "error", err)
		return
	}
	a.mu.Lock()
	if a.serverID == "" {
		a.serverID = resp.ServerID
	}
	a.mu.Unlock()
	a.logger.Info("registered with panel", "server_id", resp.ServerID, "node_id", resp.NodeID, "runtime_version", rtVersionStr)

	if err := upgrader.CommitUpgradeHealthy(""); err != nil {
		a.logger.Warn("failed to clear upgrade-pending marker after HTTP register", "error", err)
	} else {
		a.logger.Debug("upgrade health gate passed (HTTP register ok)")
	}
}

func (a *Agent) recvLoop(ctx context.Context) {
	recvCh := a.cm.Recv()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-recvCh:
			if !ok {
				return
			}
			a.handlePanelMessage(ctx, msg)
		}
	}
}

func (a *Agent) handlePanelMessage(ctx context.Context, msg *pb.PanelMessage) {
	if msg.GetPong() != nil {
		return
	}

	if authAck := msg.GetAuthAck(); authAck != nil {
		a.logger.Debug("received auth ack")
		return
	}

	a.mu.Lock()
	for seq, req := range a.pending {
		select {
		case req.ch <- msg:
			delete(a.pending, seq)
			a.mu.Unlock()
			return
		default:
		}
	}
	a.mu.Unlock()

	if hbAck := msg.GetHeartbeatAck(); hbAck != nil {
		a.processHeartbeatResponse(ctx, hbAck, &a.currentVersion)
		return
	}

	if cfgPush := msg.GetConfigPush(); cfgPush != nil {
		a.handleConfigPush(ctx, cfgPush)
		return
	}

	if maintenance := msg.GetMaintenance(); maintenance != nil {
		a.handleMaintenance(ctx, maintenance)
		return
	}

	if userBan := msg.GetUserBan(); userBan != nil {
		a.handleUserBan(ctx, userBan)
		return
	}

	if certRenew := msg.GetCertRenew(); certRenew != nil {
		a.handleCertRenew(ctx, certRenew)
		return
	}

	if deltaSync := msg.GetDeltaSync(); deltaSync != nil {
		a.handleDeltaSyncMessage(ctx, deltaSync)
		return
	}
}

func (a *Agent) handleMaintenance(ctx context.Context, m *pb.MaintenanceCommand) {
	reason := m.GetReason()
	a.logger.Info("received maintenance command",
		"action", m.Action.String(), "reason", reason, "drain_timeout", m.DrainTimeoutSeconds)

	switch m.Action {
	case pb.MaintenanceCommand_ACTION_RESTART:
		go func() {
			configPath := a.cfg.ConfigFilePath()
			a.logger.Info("restarting kernel due to maintenance command", "config", configPath)
			if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
				a.logger.Error("maintenance restart failed", "error", err)
			} else {
				a.logger.Info("kernel restarted successfully by maintenance command")
			}
		}()

	case pb.MaintenanceCommand_ACTION_DRAIN:
		a.logger.Info("drain requested - stopping new connections", "timeout", m.DrainTimeoutSeconds)

	case pb.MaintenanceCommand_ACTION_STOP:
		a.logger.Info("stop requested - stopping kernel")
		go func() {
			if err := a.runtimeExec.Stop(ctx); err != nil {
				a.logger.Error("maintenance stop failed", "error", err)
			}
		}()

	case pb.MaintenanceCommand_ACTION_RESUME:
		if strings.HasPrefix(reason, "channel_switch:") {
			parts := strings.SplitN(reason, ":", 3)
			targetChannel := ""
			if len(parts) >= 2 {
				targetChannel = parts[1]
			}
			if targetChannel != "" {
				a.logger.Info("channel switch requested via maintenance RESUME", "target", targetChannel)
				if err := a.cm.SwitchChannel(targetChannel); err != nil {
					a.logger.Error("channel switch failed", "target", targetChannel, "error", err)
				} else {
					a.logger.Info("channel switched successfully", "target", targetChannel, "active", a.cm.ActiveChannel().Name())
				}
			}
		} else {
			a.logger.Info("resume requested - resuming service")
		}

	case pb.MaintenanceCommand_ACTION_UNSPECIFIED:
		a.logger.Warn("unspecified maintenance action, ignoring")
	}
}

func (a *Agent) handleUserBan(ctx context.Context, ban *pb.UserBanNotice) {
	a.logger.Info("received user ban notice", "count", len(ban.UserIds), "reason", ban.Reason, "timestamp", ban.Timestamp)
	// P0-8: 收到封禁通知后立即触发配置重载（替代等待 10s 心跳）
	// 配置重载后，已封禁用户将从 xray/sing-box inbound clients 中移除
	// P1 将升级为 xray gRPC AlterInbound 真增量更新（无需重载）
	// 使用 "force" 版本号触发非缓存拉取
	a.applyConfig(ctx, "force", &a.currentVersion)
}

func (a *Agent) handleDeltaSyncMessage(ctx context.Context, d *pb.DeltaSync) {
	a.logger.Info("received delta sync message",
		"server_code", d.ServerCode, "kernel", d.Kernel.String(),
		"add_users", len(d.AddUsers), "del_users", len(d.DelUsers),
		"config_version", d.ConfigVersion)

	adds := make([]delta.UserChange, 0, len(d.AddUsers))
	for _, u := range d.AddUsers {
		adds = append(adds, delta.UserChange{
			Email:      u.Email,
			UUID:       u.Uuid,
			InboundTag: u.InboundTag,
			Level:      int(u.Level),
			Password:   u.Password,
			Extra:      u.Extra,
		})
	}

	sync := &delta.Sync{
		ServerCode:    d.ServerCode,
		Kernel:        kernelTypeToString(d.Kernel),
		AddUsers:      adds,
		DelUsers:      d.DelUsers,
		ConfigVersion: d.ConfigVersion,
		Timestamp:     time.Now(),
	}

	ack := a.ApplyDeltaSync(ctx, sync)
	if !ack.Success {
		a.logger.Warn("delta sync apply failed, falling back to full config reload", "error", ack.Error)
		go a.applyConfig(ctx, "force", &a.currentVersion)
	} else {
		a.currentVersion = strconv.FormatInt(d.ConfigVersion, 10)
	}
}

func kernelTypeToString(kt pb.KernelType) string {
	switch kt {
	case pb.KernelType_KERNEL_TYPE_XRAY:
		return "xray"
	case pb.KernelType_KERNEL_TYPE_SINGBOX:
		return "sing-box"
	default:
		return ""
	}
}

func (a *Agent) handleCertRenew(ctx context.Context, renew *pb.CertRenewNotice) {
	a.logger.Info("received cert renew notice", "node_id", renew.NodeId, "cert_type", renew.CertType)
	if len(renew.CertData) == 0 || len(renew.KeyData) == 0 {
		a.logger.Warn("cert renew notice missing cert/key data, skipping")
		return
	}

	// 确定证书写入路径：
	//   - 优先使用 notice 中指定的 cert_path / key_path
	//   - 否则按 cert_type 子目录组织：{certsDir}/{cert_type}/fullchain.pem
	//     （proto 无 Domain 字段，使用 cert_type 作为子目录，
	//      与 cert.Manager.persistPEM 的 {certDir}/{domain} 约定一致）
	certFile := renew.CertPath
	keyFile := renew.KeyPath
	if certFile == "" || keyFile == "" {
		dir := filepath.Join(a.cfg.CertsDir(), renew.CertType)
		if err := os.MkdirAll(dir, 0755); err != nil {
			a.logger.Error("failed to create cert dir", "dir", dir, "error", err)
			return
		}
		certFile = filepath.Join(dir, "fullchain.pem")
		keyFile = filepath.Join(dir, "privkey.pem")
	} else {
		if err := os.MkdirAll(filepath.Dir(certFile), 0755); err != nil {
			a.logger.Error("failed to create cert dir", "dir", filepath.Dir(certFile), "error", err)
			return
		}
	}

	a.logger.Info("writing new cert files", "cert", certFile, "key", keyFile)
	if err := os.WriteFile(certFile, renew.CertData, 0600); err != nil {
		a.logger.Error("failed to write cert file", "path", certFile, "error", err)
		return
	}
	if err := os.WriteFile(keyFile, renew.KeyData, 0600); err != nil {
		a.logger.Error("failed to write key file", "path", keyFile, "error", err)
		return
	}

	// 触发 xray/sing-box 热重载，使新证书生效
	configPath := a.cfg.ConfigFilePath()
	if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
		a.logger.Error("failed to reload runtime after cert renew", "error", err)
		return
	}
	a.logger.Info("cert renewed and runtime reloaded successfully",
		"cert_type", renew.CertType, "cert", certFile, "key", keyFile)
}

func (a *Agent) handleConfigPush(ctx context.Context, cfgPush *pb.ConfigPush) {
	targetVersion := strconv.FormatInt(cfgPush.Version, 10)
	a.logger.Info("received config push", "version", targetVersion)
	// Jitter Pull: 0-3000ms 随机延迟，避免推送风暴时所有节点同时拉取配置打爆面板
	jitter := time.Duration(mrand.Intn(3000)) * time.Millisecond
	a.logger.Debug("applying jitter before config pull", "delay", jitter)
	time.Sleep(jitter)
	a.applyConfig(ctx, targetVersion, &a.currentVersion)
}

// ApplyDeltaSync P2: 应用增量用户同步消息。
// 通过 HTTP POST /api/v1/agent/delta 接收，调用 RuntimePlugin.UpdateUsers 热更。
func (a *Agent) ApplyDeltaSync(ctx context.Context, d *delta.Sync) *delta.Ack {
	if a.deltaApplier == nil {
		return &delta.Ack{
			Success: false,
			Error:   "delta sync not available (native mode disabled or plugin not initialized)",
		}
	}

	ack, needFullSync := a.deltaApplier.Apply(ctx, d)
	if needFullSync {
		a.logger.Info("delta sync triggered full config fetch due to version jump")
		go a.applyConfig(ctx, "force", &a.currentVersion)
	}

	// P3: Delta 应用后触发快速拨测
	if ack.Success && a.prober != nil {
		go a.prober.ProbeAfterApply(ctx)
	}

	return ack
}

// UpdateProberTargets P3: 从配置中提取监听端口，更新拨测目标列表。
func (a *Agent) UpdateProberTargets(configMap map[string]interface{}) {
	if a.prober == nil {
		return
	}
	targets := []*prober.ProbeTarget{}

	inbounds, ok := configMap["inbounds"].([]interface{})
	if !ok {
		return
	}

	for _, ibRaw := range inbounds {
		ib, ok := ibRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := ib["tag"].(string)
		if tag == "api" || tag == "tproxy" || tag == "redirect" {
			continue
		}
		// 兼容 xray (port) 和 sing-box (listen_port) 两种格式
		port := ib["port"]
		if port == nil {
			port = ib["listen_port"] // sing-box format
		}
		if port == nil {
			continue
		}
		portStr := fmt.Sprintf("%v", port)
		protocol, _ := ib["protocol"].(string)
		if protocol == "" {
			protocol, _ = ib["type"].(string)
		}

		targets = append(targets, &prober.ProbeTarget{
			Name:     tag,
			Addr:     "127.0.0.1:" + portStr,
			Protocol: protocol,
			Tags:     extractProbeTags(ib),
		})
	}

	if len(targets) > 0 {
		a.prober.ReplaceTargets(targets)
		a.logger.Info("prober targets updated", "count", len(targets))
	}
}

func (a *Agent) fetchConfigViaHTTP(ctx context.Context, targetVersion string) (map[string]interface{}, string, error) {
	cfgResp, err := a.httpClient.FetchConfig(ctx, targetVersion)
	if err != nil {
		if errors.Is(err, client.ErrNotModified) {
			return nil, "", err
		}
		return nil, "", err
	}
	return cfgResp.Config, cfgResp.Signature, nil
}

// fetchConfigViaPayload 通过加密 Payload Manifest 拉取配置（P1-9）。
//
// 返回与 fetchConfigViaHTTP 相同格式的 (configMap, signature, error)。
//   - PayloadEncrypted=true 时，Content 是 base64 编码的 JSON 字符串字面量，
//     解码后为 nonce||ciphertext，使用 AES-GCM 解密（密钥由 payloadKey 经
//     SHA-256 派生为 32 字节 AES-256 密钥），解密后得到 PayloadContent JSON，
//     其 config_json 字段即为 xray/sing-box 配置。
//   - PayloadEncrypted=false 时，Content 直接是 PayloadContent 的明文 JSON。
//
// 加密格式与 node-service/internal/crypto/payload.go 的 EncryptPayload/BuildManifest 对应。
func (a *Agent) fetchConfigViaPayload(ctx context.Context, targetVersion string) (map[string]interface{}, string, error) {
	manifest, err := a.httpClient.FetchPayload(ctx, targetVersion)
	if err != nil {
		return nil, "", err
	}

	var contentBytes []byte
	if manifest.PayloadEncrypted {
		// 加密模式：Content 是 base64 编码的 JSON 字符串字面量
		var encoded string
		if err := json.Unmarshal(manifest.Content, &encoded); err != nil {
			return nil, "", fmt.Errorf("decode payload content string: %w", err)
		}
		encrypted, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode payload: %w", err)
		}
		// AES-GCM 解密：密钥由 payloadKey 经 SHA-256 派生（32 字节，AES-256）
		keyHash := sha256.Sum256([]byte(a.httpClient.PayloadKey()))
		block, err := aes.NewCipher(keyHash[:])
		if err != nil {
			return nil, "", fmt.Errorf("create aes cipher: %w", err)
		}
		aesgcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, "", fmt.Errorf("create gcm: %w", err)
		}
		nonceSize := aesgcm.NonceSize()
		if len(encrypted) < nonceSize {
			return nil, "", fmt.Errorf("payload ciphertext too short")
		}
		nonce, ciphertext := encrypted[:nonceSize], encrypted[nonceSize:]
		plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, "", fmt.Errorf("decrypt payload: %w", err)
		}
		contentBytes = plaintext
	} else {
		// 明文模式：Content 直接是 PayloadContent JSON
		contentBytes = manifest.Content
	}

	// 解析 PayloadContent，提取 config_json 和 tls_materials 字段
	// tls_materials: domain -> {cert_pem, key_pem}，由面板签发后内联下发（阶段 A2）
	var payload struct {
		ConfigJSON   map[string]interface{} `json:"config_json"`
		TLSMaterials map[string]struct {
			CertPEM string `json:"cert_pem"`
			KeyPEM  string `json:"key_pem"`
		} `json:"tls_materials,omitempty"`
	}
	if err := json.Unmarshal(contentBytes, &payload); err != nil {
		return nil, "", fmt.Errorf("unmarshal payload content: %w", err)
	}
	if payload.ConfigJSON == nil {
		return nil, "", fmt.Errorf("payload content missing config_json")
	}

	// 阶段 A2: 消费 tls_materials，将面板推送的 PEM 注入 certMgr
	// 这样 nginx 节点无需本地 ACME 签发，直接使用面板下发的证书
	if len(payload.TLSMaterials) > 0 && a.certMgr != nil {
		for domain, mat := range payload.TLSMaterials {
			if mat.CertPEM == "" || mat.KeyPEM == "" || domain == "" {
				continue
			}
			a.certMgr.SetContentPEM([]byte(mat.CertPEM), []byte(mat.KeyPEM), domain)
		}
		a.logger.Info("tls_materials consumed from payload",
			"domains", len(payload.TLSMaterials))
	}

	return payload.ConfigJSON, manifest.SHA256, nil
}

func (a *Agent) applyConfig(ctx context.Context, targetVersion string, currentVersion *string) {
	start := time.Now()
	success := false
	errMsg := ""

	// D11: deploy.lock is now managed by Pipeline.Run internally.
	// Deferred result reporting (gRPC + HTTP fallback) stays here.
	defer func() {
		durationMs := time.Since(start).Milliseconds()
		versionNum := int64(0)
		if v, err := strconv.ParseInt(targetVersion, 10, 64); err == nil {
			versionNum = v
		}
		kernelType := a.cfg.RuntimeType

		result := &pb.AgentMessage{
			Seq:       a.nextSeq(),
			Timestamp: time.Now().UnixMilli(),
			Payload: &pb.AgentMessage_ConfigResult{
				ConfigResult: &pb.ConfigResult{
					Version:         versionNum,
					Success:         success,
					Error:           errMsg,
					ApplyDurationMs: durationMs,
					KernelType:      kernelType,
				},
			},
		}
		if err := a.cm.Send(result); err != nil {
			a.logger.Warn("failed to send config result via protobuf, falling back to HTTP", "error", err)
			httpResult := &client.ConfigResult{
				Version:    targetVersion,
				Success:    success,
				Message:    errMsg,
				DurationMs: durationMs,
			}
			if fbErr := a.httpClient.ReportResult(ctx, httpResult); fbErr != nil {
				a.logger.Error("failed to report config result via HTTP", "error", fbErr)
			}
		}
	}()

	a.logger.Info("fetching config", "version", targetVersion)
	// P1-9: 优先使用加密 Payload Manifest 拉取配置，失败时回退到明文 FetchConfig
	a.fetchedViaPayload = false
	configMap, signature, err := a.fetchConfigViaPayload(ctx, targetVersion)
	if err != nil {
		a.logger.Warn("fetch encrypted payload failed, falling back to FetchConfig", "error", err)
		configMap, signature, err = a.fetchConfigViaHTTP(ctx, targetVersion)
		if err != nil {
			if errors.Is(err, client.ErrNotModified) {
				a.logger.Info("config not modified (304), skipping apply", "version", targetVersion)
				success = true
				errMsg = ""
				return
			}
			errMsg = fmt.Sprintf("fetch config failed: %v", err)
			a.logger.Error("fetch config failed", "error", err)
			return
		}
	} else {
		a.fetchedViaPayload = true
	}

	// 签名校验：基于下载下来的原始内容计算hash（在任何字段删除/审计注入之前）。
	// 历史上曾因服务端ContentJSON map被原地污染导致hash永久不一致，将阻断部署。
	// 对Payload加密通道：AES-GCM已提供认证加密，篡改会在解密阶段失败，此处仅做warn记录。
	// 对明文通道：保留strict校验作为纵深防御。
	// 使用fetchedViaPayload标志（由fetchConfigViaPayload设置）判断通道类型。
	{
		rawBytes, marshalErr := json.Marshal(configMap)
		if marshalErr != nil {
			errMsg = fmt.Sprintf("marshal raw config failed: %v", marshalErr)
			a.logger.Error("marshal raw config failed", "error", marshalErr)
			return
		}
		initialHash := sha256.Sum256(rawBytes)
		expectedSig := hex.EncodeToString(initialHash[:])
		if signature != "" && signature != expectedSig {
			if a.fetchedViaPayload {
				a.logger.Warn("config signature mismatch on encrypted channel (AES-GCM authenticated, continuing)",
					"computed", expectedSig, "expected", signature, "config_len", len(rawBytes))
			} else {
				errMsg = fmt.Sprintf("config signature mismatch on plaintext channel: expected %s, got %s", expectedSig[:16], signature[:16])
				a.logger.Error("config signature mismatch, rejecting config",
					"expected", expectedSig, "got", signature)
				a.reportDeploymentResult(ctx, targetVersion, "nack", "signature_verify", errMsg, 0)
				return
			}
		} else if signature != "" {
			a.logger.Debug("config signature verified", "version", targetVersion, "hash", expectedSig[:16])
		}
	}

	// E10: _nginx_vhosts 由 NginxReconciler 独立处理（30s 轮询 /agent/cdn-vhosts），
	// applyConfig 不再直接调用 syncNginxVhosts，避免双路径冲突和重复 nginx reload。
	// 仅需从 configMap 中移除该字段，避免 xray 验证失败。
	delete(configMap, "_nginx_vhosts")

	// 提取 _limiter 字段（kernelrender 注入的限速器元数据）
	// 必须在 marshal 前移除，避免内核验证失败（xray/sing-box 不识别此字段）
	limiterMeta := configMap["_limiter"]
	delete(configMap, "_limiter")

	// 双内核架构：提取 _singbox_config 字段（面板将 sing-box 配置嵌入 xray 配置中）
	// 提取后单独应用到 sing-box 内核，避免 xray 验证失败
	singboxConfig, hasSingboxConfig := configMap["_singbox_config"].(map[string]interface{})
	if hasSingboxConfig {
		delete(configMap, "_singbox_config")
		a.logger.Info("extracted _singbox_config from xray config, will apply to sing-box kernel",
			"sb_inbounds", len(singboxConfig["inbounds"].([]interface{})))
	}

	// P-Chain-Bridge: 提取 _chain_bridges 字段（自签证书/insecure 链式桥接配置）
	// 面板在 xray runtime 下遇到 insecure=1 时自动生成 sing-box 桥接配置，
	// xray chain outbound 为 socks5 指向本地桥接端口，sing-box 桥接用 insecure:true 连接上游。
	// 合并到 singboxConfig 中作为一个 sing-box 实例运行（或单独注入如果没有 _singbox_config）。
	chainBridges, hasChainBridges := configMap["_chain_bridges"].(map[string]interface{})
	if hasChainBridges {
		delete(configMap, "_chain_bridges")
		a.logger.Info("extracted _chain_bridges from xray config, will merge into sing-box kernel")
		singboxConfig = mergeChainBridges(singboxConfig, chainBridges, a.logger)
		hasSingboxConfig = true
	}

	// P2-9: 审计规则动态下发（_audit_rules）
	// 面板可通过 _audit_rules 字段动态下发 BT/SSRF/LAN 阻断规则，
	// 默认注入 BT 阻断 + SSRF 阻断规则，无需硬编码。
	auditRules := audit.ExtractRules(configMap, a.logger)
	if err := audit.ApplyToConfig(configMap, a.cfg.RuntimeType, auditRules, a.logger); err != nil {
		a.logger.Warn("audit rule injection failed", "error", err)
	}
	if hasSingboxConfig {
		if err := audit.ApplyToConfig(singboxConfig, "sing-box", auditRules, a.logger); err != nil {
			a.logger.Warn("sing-box audit rule injection failed", "error", err)
		}
	}

	configContent, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		errMsg = fmt.Sprintf("marshal config failed: %v", err)
		a.logger.Error("marshal indent config failed", "error", err)
		return
	}

	// Quick semantic validation before entering Pipeline.Run
	if err := a.runtimeExec.Validate(string(configContent)); err != nil {
		errMsg = fmt.Sprintf("config validation failed: %v", err)
		a.logger.Error("config validation failed", "error", err)
		return
	}

	// P1-5: HotDiff 分类决定热重载策略 (computed before Pipeline.Run, used in Apply callback)
	diff := hotdiff.ComputeHotDiff(a.lastAppliedConfig, configMap)
	a.logger.Info("hotdiff classified",
		"level", string(diff.Level),
		"changed_fields", diff.ChangedFields,
		"summary", diff.Summary,
		"user_changes", len(diff.UserChanges))

	// Backup version.txt before pipeline overwrites config
	if *currentVersion != "" {
		writeCurrentVersion(a.cfg.VersionFilePath()+".bak", *currentVersion, a.logger)
	}

	configPath := a.cfg.ConfigFilePath()

	// D11: Build DeployCallbacks for the unified Pipeline.Run
	callbacks := pipeline.DeployCallbacks{
		DryRun: func(ctx context.Context, tmpPath string) error {
			return a.runtimeExec.DryRun(ctx, tmpPath)
		},
		Apply: func(ctx context.Context, cfgPath string) error {
			return a.applyWithHotDiff(ctx, cfgPath, diff)
		},
		HealthCheck: func(ctx context.Context, configJSON []byte) error {
			// Phase 1: Process-level check
			time.Sleep(1 * time.Second)
			status, err := a.runtimeExec.Status(ctx)
			if err != nil || !status.Running {
				return fmt.Errorf("runtime not running after reload: %v", err)
			}
			// Phase 2: Port reachability check
			if a.healthChecker != nil {
				if hcErr := a.healthChecker.CheckHealth(ctx, configJSON, 2*time.Second); hcErr != nil {
					return fmt.Errorf("health check failed (port unreachable): %v", hcErr)
				}
			}
			return nil
		},
		OnRollback: func(ctx context.Context, restoredConfigPath string) error {
			// E8 fix: single reload via OnRollback callback (no double reload)
			a.runtimeExec.Rollback()
			return a.runtimeExec.Reload(ctx, restoredConfigPath)
		},
	}

	// D11: Execute unified deployment pipeline
	// Pipeline.Run handles: lock -> precheck -> write temp -> dryrun -> backup -> activate -> apply -> healthcheck -> success/rollback
	result := a.pipeline.Run(ctx, targetVersion, configContent, configPath, a.cfg.RuntimeType, callbacks)

	if !result.Success {
		errMsg = result.Error
		a.logger.Error("pipeline deployment failed",
			"version", targetVersion,
			"phase", result.Phase,
			"rolled_back", result.RolledBack,
			"error", result.Error)
		a.reportDeploymentResult(ctx, targetVersion, "nack", result.Phase, errMsg, result.ApplyDurationMs)
		return
	}

	a.logger.Info("pipeline deployment succeeded, running post-success tasks",
		"version", targetVersion, "duration_ms", result.ApplyDurationMs)

	// === Post-success tasks (executed only after Pipeline.Run succeeds) ===

	// E10: nginx vhost 同步已由 NginxReconciler 独立处理，不再在此调用 syncNginxVhosts

	// 同步防火墙规则：从 xray/sing-box 配置提取 inbound 端口并确保已开放
	a.syncFirewallPorts(configMap)

	// P3: 更新主动拨测目标列表
	a.UpdateProberTargets(configMap)

	// 更新限速器配置
	a.syncLimiters(limiterMeta)

	// P1-8: 启动设备限制执行器（首次成功应用配置后启动一次）
	a.maybeStartDeviceEnforcer(ctx, configMap)

	// 双内核架构：如果有 sing-box 配置，应用到 sing-box 内核
	if hasSingboxConfig && a.useNative && a.pluginAdapter != nil {
		sbConfigBytes, sbErr := json.Marshal(singboxConfig)
		if sbErr != nil {
			a.logger.Error("marshal sing-box config failed", "error", sbErr)
		} else {
			if sbErr := a.pluginAdapter.StartNative(ctx, sbConfigBytes); sbErr != nil {
				a.logger.Error("apply sing-box config failed", "error", sbErr)
			} else {
				a.logger.Info("sing-box config applied successfully via dual-kernel injection",
					"sb_config_size", len(sbConfigBytes))
				// 缓存 sing-box 配置，供 debounced restart 后重新启动 sing-box
				a.lastSingboxConfig = singboxConfig
			}
		}
	}

	// === Version & state updates ===
	writeCurrentVersion(a.cfg.VersionFilePath(), targetVersion, a.logger)
	*currentVersion = targetVersion
	a.currentVersion = targetVersion
	// P1-5: 缓存本次成功应用的配置，供下次 HotDiff 比较
	a.lastAppliedConfig = configMap

	// P2: 全量配置应用成功后，更新 Delta Sync 基线版本
	if a.deltaApplier != nil {
		ver, _ := strconv.ParseInt(targetVersion, 10, 64)
		a.deltaApplier.SetBaseVersion(ver)
	}

	// P3: 配置应用后触发主动拨测，验证全链路正常
	if a.prober != nil {
		go func() {
			healthy, results := a.prober.ProbeAfterApply(ctx)
			if !healthy {
				failedTargets := []string{}
				for _, r := range results {
					if !r.Success {
						failedTargets = append(failedTargets, r.Target)
					}
				}
				a.logger.Warn("post-apply prober detected failures (LKG may be needed)",
					"failed_targets", failedTargets)
				// YUNDU_DISABLE_PROBER=1 时禁用 LKG 回滚（用于 prober 误判场景下的紧急恢复）
				// 例如 CDN 节点 SNI 不匹配导致 prober 拨测失败，但配置实际是正确的
				if os.Getenv("YUNDU_DISABLE_PROBER") == "1" {
					a.logger.Warn("YUNDU_DISABLE_PROBER=1, skipping LKG rollback despite prober failures",
						"fail_count", a.prober.FailCount())
					return
				}
				// 连续失败 3 次触发 LKG 回滚
				if a.prober.FailCount() >= 3 {
					a.logger.Error("prober fail count exceeded threshold, triggering LKG rollback")
					if a.pipeline.HasLKG(a.cfg.RuntimeType) {
						if err := a.pipeline.RestoreLKG(configPath, a.cfg.RuntimeType); err == nil {
							a.runtimeExec.Reload(ctx, configPath)
						}
					}
				}
			}
		}()
	}

	success = true
	errMsg = "config applied successfully"
	a.logger.Info("config applied successfully", "version", targetVersion, "duration_ms", time.Since(start).Milliseconds())
	// 上报 ACK：部署成功
	a.reportDeploymentResult(ctx, targetVersion, "ack", "activate", "", time.Since(start).Milliseconds())
}

// reportDeploymentResult 上报部署结果（ACK/NACK）到面板。
//
// 边缘自治全链路：在部署流水线各阶段结束时调用，
// status="ack" 表示成功，status="nack" 表示失败（已触发回滚）。
// phase 标识失败阶段：precheck / activate / healthcheck。
// 上报失败仅记录警告，不影响本地部署流程。
func (a *Agent) reportDeploymentResult(ctx context.Context, version, status, phase, errMsg string, durationMs int64) {
	req := &client.DeploymentResultRequest{
		Version:    version,
		Status:     status,
		Phase:      phase,
		Error:      errMsg,
		DurationMs: durationMs,
	}
	if err := a.httpClient.ReportDeploymentResult(ctx, req); err != nil {
		a.logger.Warn("failed to report deployment result", "error", err, "status", status)
	}
}

// applyWithHotDiff P1-5+P1-7: 根据 HotDiff 分类结果选择热重载策略。
//   - HOT_USER_ONLY    → AlterInbound（增量用户更新，PID 不变）
//   - HOT_ROUTING_ONLY → Reload（SIGUSR1，PID 不变）
//   - HOT_TLS_RELOAD   → Reload（SIGUSR1，PID 不变）
//   - RESTART_REQUIRED → Reload（SIGUSR1 PID 不变）+ 30s 防抖后真正全量重启
//   - 空（无变更）      → 跳过
func (a *Agent) applyWithHotDiff(ctx context.Context, configPath string, diff hotdiff.DiffDetail) error {
	switch diff.Level {
	case hotdiff.DiffHotUserOnly:
		// 增量用户变更：优先 AlterInbound，失败回退 Reload
		users := make([]executor.AlterUser, 0, len(diff.UserChanges))
		for _, uc := range diff.UserChanges {
			users = append(users, executor.AlterUser{
				InboundTag: uc.InboundTag,
				Email:      uc.Email,
				Op:         executor.AlterUserOp(uc.Op),
				Account:    uc.Account,
			})
		}
		a.logger.Info("applying hot user reload via AlterInbound", "users", len(users))
		if err := a.runtimeExec.AlterInbound(ctx, users); err != nil {
			a.logger.Warn("AlterInbound failed, falling back to Reload", "error", err)
			return a.runtimeExec.Reload(ctx, configPath)
		}
		return nil

	case hotdiff.DiffHotRoutingOnly, hotdiff.DiffHotTLSReload:
		// 路由/TLS：走 Reload（Linux SIGUSR1 PID 不变；Windows 全量重启）
		a.logger.Info("applying reload",
			"level", string(diff.Level), "summary", diff.Summary)
		return a.runtimeExec.Reload(ctx, configPath)

	case hotdiff.DiffRestartNeeded:
		// P1-7: 30秒防抖重启
		// 立即走 SIGUSR1 Reload（PID 不变，保持服务可用），同时启动/重置 30s 防抖计时器。
		// 计时器到期后执行一次真正的全量重启（Stop+Start，PID 变化）。
		// 窗口内若有新的 RESTART 请求，重置计时器以合并重启。
		a.logger.Info("restart required, applying SIGUSR1 reload now + scheduling debounced restart",
			"summary", diff.Summary)
		if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
			return err
		}
		a.scheduleDebouncedRestart(ctx, configPath)
		return nil

	default:
		// 无变更（DiffLevel 为空），跳过 reload
		a.logger.Info("no changes detected, skipping reload")
		return nil
	}
}

// scheduleDebouncedRestart P1-7: 安排一次 30s 防抖重启。
// 若已有待执行的重启计时器，则重置计时器（合并重启请求）。
// 计时器到期后在独立 goroutine 中执行全量重启。
func (a *Agent) scheduleDebouncedRestart(ctx context.Context, configPath string) {
	a.restartDebounceMu.Lock()
	defer a.restartDebounceMu.Unlock()

	if a.restartDebounceTimer != nil {
		// 已有待执行的重启，重置计时器（合并）
		a.restartDebounceTimer.Reset(30 * time.Second)
		a.logger.Info("restart debounce timer reset (merged)", "delay", "30s")
		return
	}

	a.restartDebouncePending = true
	a.restartDebounceTimer = time.AfterFunc(30*time.Second, func() {
		a.restartDebounceMu.Lock()
		a.restartDebounceTimer = nil
		wasPending := a.restartDebouncePending
		a.restartDebouncePending = false
		a.restartDebounceMu.Unlock()

		if !wasPending {
			return
		}

		a.logger.Info("debounced restart firing: performing full restart now")
		// 全量重启：Stop + Reload（Reload 内部会 start）
		if err := a.runtimeExec.Stop(ctx); err != nil {
			a.logger.Error("debounced restart: stop failed", "error", err)
		}
		if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
			a.logger.Error("debounced restart: reload failed", "error", err)
		} else {
			a.logger.Info("debounced restart completed successfully")
		}
		// 双内核：Reload 只恢复了 xray（文件已剥离 _singbox_config），需手动重启 sing-box
		if a.lastSingboxConfig != nil && a.useNative && a.pluginAdapter != nil {
			sbBytes, sbErr := json.Marshal(a.lastSingboxConfig)
			if sbErr != nil {
				a.logger.Error("debounced restart: marshal sing-box config failed", "error", sbErr)
			} else if sbErr := a.pluginAdapter.StartNative(ctx, sbBytes); sbErr != nil {
				a.logger.Error("debounced restart: restart sing-box failed", "error", sbErr)
			} else {
				a.logger.Info("debounced restart: sing-box restarted successfully",
					"sb_config_size", len(sbBytes))
			}
		}
	})
	a.logger.Info("restart debounce timer started", "delay", "30s")
}

// syncNginxVhosts 同步 nginx snippet 到本地 nginx。
// vhostsRaw 是从 xray 配置中提取的 _nginx_vhosts 字段值。
// 失败仅记录警告，不影响 xray 配置应用结果。
func (a *Agent) syncNginxVhosts(ctx context.Context, vhostsRaw interface{}) {
	if vhostsRaw == nil {
		return
	}
	vhosts, ok := vhostsRaw.(map[string]interface{})
	if !ok {
		a.logger.Warn("_nginx_vhosts field has invalid type, skipping nginx sync")
		return
	}

	httpsSnippet, _ := vhosts["https_snippet"].(string)
	streamSnippet, _ := vhosts["stream_snippet"].(string)
	if httpsSnippet == "" && streamSnippet == "" {
		return
	}

	// 优先使用宝塔面板路径（VPS190 实测），可通过环境变量 NODE_AGENT_NGINX_ENV 切换
	syncCfg := nginx.DefaultBtPanelConfig()
	if env := os.Getenv("NODE_AGENT_NGINX_ENV"); env == "standard" {
		syncCfg = nginx.DefaultStandardConfig()
	}

	a.logger.Info("syncing nginx vhosts",
		"has_https", httpsSnippet != "",
		"has_stream", streamSnippet != "",
		"https_path", syncCfg.HTTPSSnippetPath,
		"stream_path", syncCfg.StreamSnippetPath)

	result, err := nginx.Sync(streamSnippet, httpsSnippet, syncCfg)
	if err != nil {
		a.logger.Warn("nginx vhost sync failed (xray config already applied, nginx may need manual sync)",
			"error", err,
			"nginx_test_out", result.NginxTestOut)
		return
	}
	a.logger.Info("nginx vhosts synced successfully",
		"stream_applied", result.StreamApplied,
		"https_applied", result.HTTPSApplied,
		"nginx_reloaded", result.NginxReloaded)
}

// syncFirewallPorts extracts inbound ports from the runtime config and
// ensures they are open in the firewall. Best-effort: failures are logged
// but don't affect config deployment success.
func (a *Agent) syncFirewallPorts(config map[string]interface{}) {
	if a.firewallMgr == nil {
		return // no active firewall, nothing to do
	}

	rules := firewall.ExtractPortsFromConfig(config)
	if len(rules) == 0 {
		return
	}

	a.logger.Info("syncing firewall ports",
		"firewall", a.firewallMgr.Name(), "port_count", len(rules))

	firewall.SyncPorts(a.firewallMgr, rules, a.logger)
}

// syncLimiters 将 _limiter 元数据传入 executor 更新限速器配置。
//
// 通过类型断言访问 executor 的 LimiterUpdater 接口（XrayExecutor/SingBoxExecutor 均实现）。
// 失败仅记录警告，不影响配置部署结果——限速器不工作不影响节点基本可用性。
func (a *Agent) syncLimiters(limiterMeta interface{}) {
	if limiterMeta == nil {
		a.logger.Debug("no _limiter metadata, skip limiter sync")
		return
	}
	updater, ok := a.runtimeExec.(executor.LimiterUpdater)
	if !ok {
		a.logger.Debug("runtime executor does not support limiter integration, skip")
		return
	}
	updater.UpdateLimitersFromMeta(limiterMeta)
	a.logger.Info("limiters synced from config metadata")

	// ★ Phase 6: 显式 IPLimiter 同步（对接阶段3改动）
	// UpdateLimitersFromMeta 已更新 per-user/node-level IP 限制，
	// 此处额外做 IPLimiterProvider 接口断言与状态日志，确认 IP 限制器已正确注入。
	if ipProvider, ok := a.runtimeExec.(executor.IPLimiterProvider); ok {
		ipLimiter := ipProvider.IPLimiter()
		blockedCount := len(ipLimiter.GetBlockedIPs())
		a.logger.Info("IP limiter synced",
			"blocked_ips", blockedCount,
			"ip_limiter_active", ipLimiter != nil)
	} else {
		a.logger.Debug("runtime executor does not support IPLimiterProvider, IP limiting disabled")
	}
}

// maybeStartDeviceEnforcer 在首次成功应用配置后启动设备限制执行器。
//
// 仅在以下条件满足时启动（且仅启动一次）：
//   - runtime executor 实现 DeviceLimiterProvider 接口（XrayExecutor/PluginAdapter 均实现）
//   - 当前 runtime 类型为 xray（设备限制依赖 xray gRPC StatsService）
//   - 配置中存在 inbound（用于提取 inbound tag）
//
// 启动失败仅记录警告，不影响节点基本可用性。
// 后续配置更新时设备限制值已通过 syncLimiters 更新，无需重启 enforcer。
func (a *Agent) maybeStartDeviceEnforcer(ctx context.Context, configMap map[string]interface{}) {
	if a.deviceEnforcer != nil {
		return // 已启动
	}

	// 仅 xray 支持设备限制执行（依赖 StatsService gRPC API）
	if a.cfg.RuntimeType != "xray" {
		return
	}

	provider, ok := a.runtimeExec.(executor.DeviceLimiterProvider)
	if !ok {
		a.logger.Debug("runtime executor does not support device limiter provider, skip enforcer")
		return
	}

	// 从配置中提取第一个非 api 的 inbound tag
	inboundTag := extractInboundTag(configMap)
	if inboundTag == "" {
		a.logger.Warn("device enforcer: no inbound tag found in config, skip enforcer")
		return
	}

	// 重载回调：被拉黑用户连接清零后，通过全量重载恢复用户
	reloadFn := func(ctx context.Context) error {
		configPath := a.cfg.ConfigFilePath()
		return a.runtimeExec.Reload(ctx, configPath)
	}

	enforcer := executor.NewDeviceEnforcer(provider, executor.DeviceEnforcerConfig{
		APIEndpoint: a.cfg.XrayAPIEndpoint,
		InboundTag:  inboundTag,
	}, reloadFn, a.logger)

	if err := enforcer.Start(ctx); err != nil {
		a.logger.Warn("failed to start device enforcer, device limit enforcement disabled",
			"error", err, "inbound_tag", inboundTag)
		return
	}
	a.deviceEnforcer = enforcer
	a.logger.Info("device enforcer started for device limit enforcement",
		"inbound_tag", inboundTag)
}

// startDeviceReportLoop 启动设备状态上报循环。
//
// 两个独立定时循环：
//   - 30s 上报循环：POST /api/v1/agent/devices/report，上报本节点在线设备 IP
//   - 60s 拉取循环：GET /api/v1/agent/devices/alive，拉取全局设备态并更新 DeviceLimiter
//
// WS sync.devices 事件目前无对应 proto 消息，设备态同步完全通过 HTTP 轮询实现。
// 未来面板新增 WS sync.devices 消息后，可在 handlePanelMessage 中接入实时推送。
func (a *Agent) startDeviceReportLoop(ctx context.Context) {
	// 30s 设备上报循环
	a.goTrack(func() {
		reportTicker := time.NewTicker(30 * time.Second)
		defer reportTicker.Stop()
		// 首次延迟 10s 启动，等待 runtime 就绪
		time.Sleep(10 * time.Second)
		a.reportDevices(ctx)
		for {
			select {
			case <-ctx.Done():
				a.logger.Info("device report loop stopping")
				return
			case <-reportTicker.C:
				a.reportDevices(ctx)
			}
		}
	})

	// 60s 全局设备态拉取循环
	a.goTrack(func() {
		aliveTicker := time.NewTicker(60 * time.Second)
		defer aliveTicker.Stop()
		// 首次延迟 15s 启动，避免与上报循环同时执行
		time.Sleep(15 * time.Second)
		a.fetchAliveDevices(ctx)
		for {
			select {
			case <-ctx.Done():
				a.logger.Info("device alive fetch loop stopping")
				return
			case <-aliveTicker.C:
				a.fetchAliveDevices(ctx)
			}
		}
	})

	// P1 fix: 定期清理 IPLimiter 中陈旧空条目，防止长期运行内存泄漏
	a.goTrack(func() {
		gcTicker := time.NewTicker(5 * time.Minute)
		defer gcTicker.Stop()
		time.Sleep(30 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-gcTicker.C:
				if ipProvider, ok := a.runtimeExec.(executor.IPLimiterProvider); ok {
					ipLimiter := ipProvider.IPLimiter()
					if ipLimiter != nil {
						ipLimiter.CleanupStaleEntries()
					}
				}
			}
		}
	})
}

// reportDevices 上报本节点在线设备 IP 列表到面板。
//
// 从 executor 的 DeviceLimiter 获取本地设备快照（uuid -> []ip），
// 通过 POST /api/v1/agent/devices/report 上报。失败仅记录警告。
func (a *Agent) reportDevices(ctx context.Context) {
	provider, ok := a.runtimeExec.(executor.DeviceLimiterProvider)
	if !ok {
		return // executor 不支持设备限制器
	}
	deviceLimiter := provider.DeviceLimiter()
	if deviceLimiter == nil {
		return
	}
	snapshot := deviceLimiter.GetLocalDevicesSnapshot()
	if len(snapshot) == 0 {
		a.logger.Debug("no local devices to report")
		return
	}
	req := &client.DeviceReportRequest{
		Devices: snapshot,
	}
	if err := a.httpClient.ReportDevices(ctx, req); err != nil {
		a.logger.Warn("failed to report devices", "error", err, "user_count", len(snapshot))
		return
	}
	a.logger.Debug("devices reported", "user_count", len(snapshot))
}

// fetchAliveDevices 拉取面板汇总的跨节点全局设备态并更新 DeviceLimiter。
//
// 通过 GET /api/v1/agent/devices/alive 拉取全局设备数（uuid -> count），
// 传入 DeviceLimiter.UpdateGlobalDevices 更新本地全局设备态。
// DeviceLimiter 内部会根据时间戳判断新鲜度，过期时退化为本地判定。
func (a *Agent) fetchAliveDevices(ctx context.Context) {
	provider, ok := a.runtimeExec.(executor.DeviceLimiterProvider)
	if !ok {
		return // executor 不支持设备限制器
	}
	deviceLimiter := provider.DeviceLimiter()
	if deviceLimiter == nil {
		return
	}
	resp, err := a.httpClient.FetchAliveDevices(ctx)
	if err != nil {
		a.logger.Warn("failed to fetch alive devices", "error", err)
		return
	}
	if len(resp.Devices) == 0 {
		return
	}
	deviceLimiter.UpdateGlobalDevices(resp.Devices)
	a.logger.Debug("global device state updated", "user_count", len(resp.Devices))
}

// NewAgent 创建 Agent 实例（供 machine.go 的 MachineOrchestrator 使用）。
// 仅初始化基本字段，运行时/通道/资源等在 Run 中初始化。
func NewAgent(cfg *agentconfig.Config, logger *slog.Logger) *Agent {
	hostname, _ := os.Hostname()
	return &Agent{
		cfg:      cfg,
		logger:   logger,
		hostname: hostname,
		pending:  make(map[int64]*pendingRequest),
		authDone: make(chan struct{}),
	}
}

func parsePortFromEndpoint(endpoint string) *int {
	if endpoint == "" {
		return nil
	}
	_, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}
	return &port
}

// Run 是 Agent 的主入口，完整生命周期管理。
// machine 模式下由 MachineOrchestrator 为每个节点调用。
// node 模式下由 main() 直接调用。
func (a *Agent) Run(ctx context.Context) error {
	// 委托给 runAgent，保持 main() 中现有逻辑的兼容性
	return a.runAgent(ctx)
}

// goTrack 启动一个被 WaitGroup 跟踪的 goroutine。
// gracefulShutdown 时会通过 cancelFn 取消 ctx，并等待所有被跟踪的 goroutine 退出。
// 用于长生命周期的后台循环（heartbeat/watchdog/traffic/device/control）。
func (a *Agent) goTrack(fn func()) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		fn()
	}()
}

// runAgent 是当前 main() 函数体的 Agent 方法版本。
// 包含运行时初始化、通道初始化、资源初始化、心跳/流量/设备循环、HTTP server、
// 控制服务器（control.go）、watchdog、优雅退出。
//
// 该方法由 main()（node 模式）和 MachineOrchestrator（machine 模式）调用。
func (a *Agent) runAgent(ctx context.Context) error {
	// ★ Phase 6: 创建可取消 context，gracefulShutdown 时通过 cancelFn 取消所有子 goroutine
	ctx, cancel := context.WithCancel(ctx)
	a.cancelFn = cancel
	defer cancel()

	// ===== 运行时初始化：支持原生内嵌模式（YUNDU_NATIVE_RUNTIME=true）与传统子进程模式 =====
	useNative := os.Getenv("YUNDU_NATIVE_RUNTIME") == "true"

	configFilePath := a.cfg.ConfigFilePath()
	configDir := a.cfg.ConfigDir

	if useNative {
		a.logger.Info("NATIVE MODE ENABLED: kernels will run in-process (no subprocess)",
			"runtime_type", a.cfg.RuntimeType)
		apiEndpoint := a.cfg.XrayAPIEndpoint
		if apiEndpoint == "" {
			apiEndpoint = os.Getenv("YUNDU_XRAY_API_ENDPOINT")
		}
		clashEndpoint := a.cfg.SingboxClashEndpoint
		xrayPlugin := agentruntime.NewNativeXray(a.logger, apiEndpoint)
		multiPlugin := agentruntime.NewMultiRuntimePlugin(xrayPlugin, clashEndpoint, a.logger)
		a.runtimePlugin = multiPlugin
		a.pluginAdapter = agentruntime.NewPluginAdapter(multiPlugin, configDir, configFilePath, a.cfg.RuntimeType, a.logger)
		a.runtimeExec = a.pluginAdapter
		a.useNative = true
	} else {
		a.logger.Info("LEGACY MODE: kernels will run as subprocesses (exec.Command)",
			"runtime_type", a.cfg.RuntimeType)
		registry := executor.NewRegistry()
		xrayExec := executor.NewXrayExecutor(a.cfg.RuntimePath, a.cfg.ConfigDir, a.logger)
		singBoxExec := executor.NewSingBoxExecutor(a.cfg.RuntimePath, a.cfg.ConfigDir, a.logger)
		registry.Register("xray", xrayExec)
		registry.Register("sing-box", singBoxExec)
		var err error
		a.runtimeExec, err = registry.Get(a.cfg.RuntimeType)
		if err != nil {
			return fmt.Errorf("failed to get executor: %w (runtime_type=%s)", err, a.cfg.RuntimeType)
		}
	}

	warpMgr := warp.NewManager(nil, a.logger)
	if warpMgr.DetectWarp() {
		a.logger.Info("warp sidecar detected", "socks_addr", warpMgr.SocksAddr())
	}

	token := resolveToken(a.cfg, a.logger)
	grpcAddr := resolveGRPCAddr(a.cfg, a.logger)
	wsURL := resolveWSURL(a.cfg, a.logger)

	a.logger.Info("resolved channel addresses",
		"grpc", grpcAddr, "ws", wsURL, "http", a.cfg.PanelURL)

	grpcCh := transport.NewGRPCChannel(grpcAddr, token)
	wsCh := transport.NewWSChannel(wsURL, a.cfg.ServerCode, token, a.cfg.HMACSecret)
	httpCh := transport.NewHTTPChannel(a.cfg.PanelURL, token, a.cfg.HMACSecret)
	a.httpClient = client.New(a.cfg.PanelURL, a.cfg.ServerCode, token, a.cfg.HMACSecret, a.logger)

	edgeValidator := validator.NewEdgeValidator(a.logger)
	deployPipeline := pipeline.NewPipeline(edgeValidator, a.cfg.ConfigDir, a.logger)
	a.pipeline = deployPipeline
	a.healthChecker = pipeline.NewHealthChecker(a.logger)

	cm := transport.NewChannelManager(transport.ManagerConfig{
		Channels:       []transport.Channel{grpcCh, wsCh, httpCh},
		HealthInterval: 20 * time.Second,
		FailThreshold:  3,
		UpgradeEvery:   60 * time.Second,
		Logger:         a.logger,
	})
	a.cm = cm

	logCollector := transport.NewLogCollector(a.logger)
	logCollector.Start()
	defer logCollector.Stop()
	a.logCollector = logCollector

	// 认证成功后设置日志发送器
	go func() {
		select {
		case <-a.authDone:
			logCollector.SetSender(a, a.nextSeq)
			logCollector.Info("agent", "log collector sender ready after auth", nil)
		case <-ctx.Done():
		}
	}()

	channelsAvailable := true
	if err := cm.Start(ctx); err != nil {
		a.logger.Error("failed to start channel manager, falling back to HTTP only", "error", err)
		channelsAvailable = false
	} else {
		go a.recvLoop(ctx)
		if err := a.authenticate(ctx); err != nil {
			a.logger.Warn("protobuf authentication failed, will use HTTP fallback", "error", err)
			channelsAvailable = false
		} else {
			a.logger.Info("channel manager authenticated successfully", "active_channel", cm.ActiveChannel().Name())
		}
	}
	a.channelsAvailable = channelsAvailable

	go a.registerWithFallback(ctx)

	var nginxEnv string
	if !a.skipSharedResources {
		// nginx vhost 独立协调循环
		explicitEnv := os.Getenv("NODE_AGENT_NGINX_ENV")
		nginxEnv = nginx.ResolveEnv(explicitEnv)
		nginxStateFile := filepath.Join(filepath.Dir(a.cfg.VersionFilePath()), "nginx_vhost_state.hash")

		cfToken := os.Getenv("CF_Token")
		a.certMgr = cert.NewManager(cfToken, a.logger)
		nginxReconciler := NewNginxReconciler(a.httpClient, a.logger, nginxStateFile, nginxEnv, a.certMgr)
		// P1: 保存引用，供心跳 SYNC_EXTERNAL_RESOURCES Action 立即触发同步
		a.nginxReconciler = nginxReconciler

		cloudflaredStateFile := filepath.Join(filepath.Dir(a.cfg.VersionFilePath()), "cloudflared_state.hash")
		cloudflaredReconciler := NewCloudflaredReconciler(a.httpClient, a.logger, cloudflaredStateFile)

		var binaryReconciler resource.Resource
		if !useNative {
			binaryStateFile := filepath.Join(filepath.Dir(a.cfg.VersionFilePath()), "binary_state.json")
			binaryBinPath := a.cfg.RuntimePath
			if binaryBinPath == "" {
				if p, err := exec.LookPath(a.cfg.RuntimeType); err == nil {
					binaryBinPath = p
				} else {
					binaryBinPath = "/usr/local/bin/" + a.cfg.RuntimeType
				}
			}
			binaryReconciler = NewBinaryReconciler(
				a.httpClient, a.logger,
				a.cfg.RuntimeType, binaryBinPath, binaryStateFile,
				func(ctx context.Context) error {
					return a.runtimeExec.Reload(ctx, a.cfg.ConfigFilePath())
				},
			)
		} else {
			a.logger.Info("native mode: BinaryReconciler disabled (kernels embedded in agent binary)")
		}

		// 确保 nginx 骨架配置存在（include 注入 / stream 块 / 默认证书）
		// 必须在 resourceDriver.Start 之前调用，因为 reconciler 的 Apply 依赖 skeleton 已就绪
		if err := nginx.EnsureNginxSkeleton(a.logger); err != nil {
			a.logger.Error("failed to ensure nginx skeleton before resource driver start", "error", err)
		}

		resourceDriver := resource.NewDriver(a.logger)
		if err := resourceDriver.Register(nginxReconciler); err != nil {
			a.logger.Error("failed to register nginx resource", "error", err)
		}
		if err := resourceDriver.Register(cloudflaredReconciler); err != nil {
			a.logger.Error("failed to register cloudflared resource", "error", err)
		}
		if binaryReconciler != nil {
			if err := resourceDriver.Register(binaryReconciler, resource.WithInterval(60*time.Second)); err != nil {
				a.logger.Error("failed to register binary resource", "error", err)
			}
		}
		if err := resourceDriver.Start(ctx); err != nil {
			a.logger.Error("failed to start resource driver", "error", err)
		}
		defer resourceDriver.Stop()

		a.firewallMgr = firewall.Detect(a.logger)

		if err := firewall.ApplyDefaultRules(a.logger); err != nil {
			a.logger.Warn("failed to apply default firewall rules", "error", err)
		}

		// 启动证书管理器（ACME 自动签发/续期后台 goroutine）
		// 不调用 Start 则 dns/http/certmagic 模式的 ACME 续期循环不会启动
		if a.certMgr != nil {
			if err := a.certMgr.Start(ctx); err != nil {
				a.logger.Warn("cert manager start failed, ACME auto-renew may be disabled", "error", err)
			} else {
				a.logger.Info("cert manager started for ACME auto-renewal")
			}
		}

		a.logger.Info("resource driver started (nginx + cloudflared)",
		"nginx_env", nginxEnv, "explicit_env", explicitEnv,
		"state_file", nginxStateFile, "acme_enabled", cfToken != "")
	} else {
		a.logger.Info("shared resources (nginx/cert/cloudflared/firewall) skipped - orchestrator manages them")
	}

	currentVersion := readCurrentVersion(a.cfg.VersionFilePath(), a.logger)
	a.currentVersion = currentVersion
	a.logger.Info("current config version", "version", currentVersion)

	if useNative {
		bufferPath := agentruntime.ResolveBufferPath(a.cfg.ConfigDir)
		a.trafficBuffer = agentruntime.NewTrafficBuffer(bufferPath)
		a.trafficSaveDeb = agentruntime.NewSaveDebouncer(a.trafficBuffer, 2*time.Second)
		a.trafficBaselinePath = filepath.Join(filepath.Dir(a.cfg.ConfigDir), "traffic_baseline.json")
		a.loadTrafficBaseline()
		a.logger.Info("traffic buffer initialized",
			"path", bufferPath, "baseline_path", a.trafficBaselinePath)
	}

	// 检查残留 deploy.lock
	if exists, lockVersion, err := deployPipeline.CheckStaleLock(); err == nil && exists {
		a.logger.Error("stale deploy.lock detected, last deployment may have crashed",
			"lock_version", lockVersion)
		nackReq := &client.DeploymentResultRequest{
			Version: lockVersion,
			Status:  "nack",
			Phase:   "activate",
			Error:   "agent crashed during deployment (stale deploy.lock detected on startup)",
		}
		if err := a.httpClient.ReportDeploymentResult(ctx, nackReq); err != nil {
			a.logger.Warn("failed to report stale-lock NACK", "error", err)
		}
		deployPipeline.RemoveDeployLock()
	}

	if currentVersion != "" {
		configPath := a.cfg.ConfigFilePath()
		if _, err := os.Stat(configPath); err == nil {
			a.logger.Info("existing config found, ensuring runtime is running", "path", configPath)
			if status, _ := a.runtimeExec.Status(ctx); status == nil || !status.Running {
				if err := a.runtimeExec.Reload(ctx, configPath); err != nil {
					a.logger.Error("failed to start runtime with existing config", "error", err)
				} else {
					a.logger.Info("runtime started with existing config", "version", currentVersion)
				}
			} else {
				a.logger.Info("runtime already running", "pid", status.PID)
			}
		}
	}

	// D3: 认证后强制刷新配置
	go func() {
		select {
		case <-a.authDone:
			a.logger.Info("D3: auth succeeded, force-refreshing config from panel")
			a.applyConfig(ctx, "force", &a.currentVersion)
		case <-ctx.Done():
		}
	}()

	// Delta Sync Applier
	if useNative && a.runtimePlugin != nil {
		a.deltaApplier = delta.NewApplier(a.runtimePlugin, a.logger)
		a.logger.Info("delta sync applier initialized (native mode)")
	}

	// Agent Self-Upgrader (skip in machine mode - orchestrator owns upgrade)
	if useNative && !a.skipSelfUpgrader {
		upgraderCfg := upgrader.Config{
			CurrentVersion: AgentVersion,
			UpdateURL:      a.cfg.PanelURL + "/api/v1/agent/upgrade/check",
			CheckInterval:  5 * time.Minute,
			OnRestartNeeded: func() {
				a.logger.Info("self-upgrade: signaling main goroutine for pre-restart cleanup")
				select {
				case a.restartCh <- struct{}{}:
				default:
				}
			},
		}
		a.selfUpgrader = upgrader.NewSelfUpgrader(upgraderCfg, a.logger)
		a.selfUpgrader.Start(ctx)
		defer a.selfUpgrader.Stop()
		a.logger.Info("agent self-upgrader started (native mode)")
	} else if a.skipSelfUpgrader {
		a.logger.Info("self-upgrader skipped (machine mode - orchestrator manages upgrade)")
	}

	// Active Prober
	// YUNDU_DISABLE_PROBER=1 时完全禁用 prober（不启动周期拨测，也不触发 LKG 回滚）
	// 用于 prober 误判导致配置无法生效的紧急恢复场景
	var activeProber *prober.Prober
	if os.Getenv("YUNDU_DISABLE_PROBER") == "1" {
		a.logger.Warn("YUNDU_DISABLE_PROBER=1, active prober disabled (LKG rollback also disabled)")
	} else {
		proberCfg := prober.Config{
			ProbeInterval:    60 * time.Second,
			PostApplyTimeout: 10 * time.Second,
			ProbeURL:         "http://www.google.com/generate_204",
		}
		activeProber = prober.NewProber(proberCfg, a.logger)
		activeProber.Start(ctx)
		defer activeProber.Stop()
		a.prober = activeProber
		a.logger.Info("active prober started", "interval", proberCfg.ProbeInterval.String())
	}

	// watchdog：定期检查内核状态，崩溃自动重启
	a.goTrack(func() { a.runWatchdog(ctx) })

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 心跳循环（提取为方法，逻辑与原 sendHeartbeat 闭包完全相同）
	a.goTrack(func() { a.runHeartbeat(ctx) })

	// 设备状态上报循环
	a.startDeviceReportLoop(ctx)

	// 流量统计上报循环
	a.startTrafficReportLoop(ctx)

	// ★ 本地控制服务器（yunductl CLI 入口，control.go）- skip in machine mode (orchestrator handles it)
	if !a.skipOwnHTTPServer {
		a.goTrack(func() { a.runControlServer(ctx) })
	}

	var agentHTTPServer *http.Server
	if !a.skipOwnHTTPServer {
		// Agent HTTP Server
		listenAddr := fmt.Sprintf("%s:%d", a.cfg.ListenHost, a.cfg.ListenPort)
		if os.Getenv("LISTEN_HOST") == "" {
			listenAddr = fmt.Sprintf("127.0.0.1:%d", a.cfg.ListenPort)
		}
		agentHTTPMux := http.NewServeMux()
		agentHTTPMux.HandleFunc("/delta", a.handleDelta)
		agentHTTPMux.HandleFunc("/healthz", a.handleHealthz)
		metricsHandler := promhttp.Handler()
		if a.metricsRegistry != nil {
			metricsHandler = promhttp.InstrumentMetricHandler(
				a.metricsRegistry, promhttp.HandlerFor(prometheus.Gatherer(nil), promhttp.HandlerOpts{}),
			)
		}
		agentHTTPMux.Handle("/metrics", metricsHandler)
		agentHTTPMux.HandleFunc("/prober/stats", func(w http.ResponseWriter, r *http.Request) {
			if activeProber == nil {
				http.Error(w, `{"error":"prober not available"}`, http.StatusServiceUnavailable)
				return
			}
			total, success, results := activeProber.Stats()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"total_probes":  total,
				"total_success": success,
				"fail_count":    activeProber.FailCount(),
				"targets":       results,
			})
		})

		agentHTTPServer = &http.Server{
			Addr:              listenAddr,
			Handler:           agentHTTPMux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			a.logger.Info("agent HTTP server starting", "addr", listenAddr,
				"endpoints", []string{"/delta", "/healthz", "/metrics", "/prober/stats"})
			if err := agentHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.logger.Error("agent HTTP server failed", "error", err)
			}
		}()
	} else {
		a.logger.Info("own HTTP server skipped (machine mode - orchestrator provides unified HTTP)")
	}

	a.logger.Info("node-agent started and running")

	// 等待退出信号或自升级重启（或在 machine 模式下等待 context 取消）
	if a.skipOwnHTTPServer {
		select {
		case <-ctx.Done():
			a.logger.Info("context cancelled, exiting sub-agent")
			return nil
		case <-a.restartCh:
			a.logger.Warn("sub-agent received restart signal (should not happen in machine mode)")
			return nil
		}
	}

	select {
	case sig := <-quit:
		a.gracefulShutdown(ctx, agentHTTPServer, "signal: "+sig.String())
		a.logger.Info("node-agent stopped")
		return nil
	case <-a.restartCh:
		a.gracefulShutdown(ctx, agentHTTPServer, "self-upgrade binary replaced")
		a.logger.Info("pre-restart cleanup done, waiting for upgrader to exit")
		// 阻塞等待 upgrader 替换二进制并退出进程
		select {}
	case <-ctx.Done():
		a.gracefulShutdown(ctx, agentHTTPServer, "context cancelled")
		return nil
	}
}

func (a *Agent) handleDelta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var d delta.Sync
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid delta payload: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	ack := a.ApplyDeltaSync(r.Context(), &d)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ack)
}

func (a *Agent) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var running bool
	if a.runtimeExec != nil {
		status, _ := a.runtimeExec.Status(r.Context())
		if status != nil {
			running = status.Running
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         map[string]bool{"running": running},
		"version":        AgentVersion,
		"config_version": a.currentVersion,
		"runtime_type":   a.cfg.RuntimeType,
		"use_native":     a.useNative,
	})
}

// gracefulShutdown 优雅退出。
// ★关键改造（相比原 main() 内的 gracefulShutdown 闭包）：
//  1. 退出前最后一次流量上报（避免最后一个计费周期流量丢失）
//  2. 超时从 5s 提升到 8s（流量上报需要更多时间）
//  3. 通过 cancelFn 取消 ctx，通知所有被 goTrack 跟踪的 goroutine 退出
//  4. 等待 WaitGroup 完成（带 8s 超时），确保 goroutine 不会泄漏
func (a *Agent) gracefulShutdown(ctx context.Context, httpSrv *http.Server, reason string) {
	a.logger.Info("initiating graceful shutdown", "reason", reason)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()

	// 1. 退出前最后一次流量上报（避免最后一个周期流量丢失）
	if a.useNative {
		a.logger.Info("shutdown: flushing final traffic report")
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		a.reportTraffic(flushCtx)
		flushCancel()
	}

	// 2. 取消 ctx 通知所有被 goTrack 跟踪的 goroutine 退出
	if a.cancelFn != nil {
		a.cancelFn()
	}

	// 3. 关闭 HTTP Server
	_ = httpSrv.Shutdown(shutdownCtx)

	// 4. 停止 Channel Manager
	a.cm.Stop()

	// 5. 停止 DeviceEnforcer
	if a.deviceEnforcer != nil {
		a.deviceEnforcer.Stop()
	}

	// 6. 停止 Runtime（xray/sing-box）
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	_ = a.runtimeExec.Stop(stopCtx)

	// 7. 等待所有被跟踪的 goroutine 退出（带超时，避免卡死）
	waitDone := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		a.logger.Info("all tracked goroutines exited cleanly")
	case <-time.After(6 * time.Second):
		a.logger.Warn("graceful shutdown: timed out waiting for goroutines to exit")
	}

	a.logger.Info("graceful shutdown complete")
}

// mergeChainBridges 将 _chain_bridges 配置合并到 _singbox_config 中。
// 如果 singboxConfig 为 nil（没有 sing-box runtime 节点），直接用 chainBridges 作为 sing-box 配置。
// 否则合并 inbounds/outbounds/rules 到现有 singboxConfig 中。
// 这样所有 sing-box 桥接和 sing-box runtime 节点共用一个 sing-box 实例。
func mergeChainBridges(singboxConfig, chainBridges map[string]interface{}, logger *slog.Logger) map[string]interface{} {
	if chainBridges == nil {
		return singboxConfig
	}
	if singboxConfig == nil {
		logger.Info("chain bridges used as standalone sing-box config (no _singbox_config)")
		return chainBridges
	}
	// 合并 inbounds
	if cbInbs, ok := chainBridges["inbounds"].([]interface{}); ok {
		sbInbs, _ := singboxConfig["inbounds"].([]interface{})
		singboxConfig["inbounds"] = append(sbInbs, cbInbs...)
	}
	// 合并 outbounds（chainBridges 已含 direct outbound，去重避免重复 tag）
	if cbObs, ok := chainBridges["outbounds"].([]interface{}); ok {
		sbObs, _ := singboxConfig["outbounds"].([]interface{})
		seen := make(map[string]bool)
		for _, ob := range sbObs {
			if m, ok := ob.(map[string]interface{}); ok {
				if tag, _ := m["tag"].(string); tag != "" {
					seen[tag] = true
				}
			}
		}
		for _, ob := range cbObs {
			if m, ok := ob.(map[string]interface{}); ok {
				if tag, _ := m["tag"].(string); tag != "" && seen[tag] {
					continue // 跳过重复 tag（如 direct）
				}
				sbObs = append(sbObs, ob)
			}
		}
		singboxConfig["outbounds"] = sbObs
	}
	// 合并 route.rules
	if cbRoute, ok := chainBridges["route"].(map[string]interface{}); ok {
		if cbRules, ok := cbRoute["rules"].([]interface{}); ok && len(cbRules) > 0 {
			sbRoute, _ := singboxConfig["route"].(map[string]interface{})
			if sbRoute == nil {
				sbRoute = map[string]interface{}{}
				singboxConfig["route"] = sbRoute
			}
			sbRules, _ := sbRoute["rules"].([]interface{})
			// bridge rules 前插（优先匹配）：sing-box route rules 按顺序匹配，
			// 若默认配置含 catch-all → direct 规则，追加在后会导致 bridge inbound 流量走 direct（VPS IP），
			// 永远到不了 trojan outbound。必须前插确保 bridge 规则优先匹配。
			mergedRules := make([]interface{}, 0, len(cbRules)+len(sbRules))
			mergedRules = append(mergedRules, cbRules...)
			mergedRules = append(mergedRules, sbRules...)
			sbRoute["rules"] = mergedRules
		}
	}
	logger.Info("chain bridges merged into sing-box config")
	return singboxConfig
}
