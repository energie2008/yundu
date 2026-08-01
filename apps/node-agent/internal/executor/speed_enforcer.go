package executor

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/node-agent/internal/tcshape"
	statsCmd "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultSpeedEnforceInterval = 15 * time.Second
	defaultSpeedDialTimeout     = 5 * time.Second
)

// SpeedEnforcerConfig 配置 xray 直连节点的每用户带宽整形。
type SpeedEnforcerConfig struct {
	APIEndpoint  string        // xray gRPC API（用于获取每用户在线 IP）
	InboundPorts []int         // 直连 xray inbound 端口（listen != 127.0.0.1）
	StateDir     string        // tc 状态文件目录
	Dev          string        // 主网卡（空=自动探测）
	Ifb          string        // IFB 网卡（空=ifb0）
	ServerIP     string        // 本机 IP（空=自动探测）
	Interval     time.Duration // 检查间隔（默认 15s）
}

// SpeedEnforcer 通过 xray StatsService 获取每用户在线 IP，
// 用 tc/iptables 对直连 inbound 执行每用户带宽整形（下载=egress HTB，上传=IFB）。
//
// xray-core 没有 per-user 带宽字段，native 进程内也没有连接级拦截点；
// 因此对直连节点按用户来源 IP 整形。CDN/Tunnel 节点（127.0.0.1 回源）
// 无法按用户 IP 区分，不参与 tc 整形。
type SpeedEnforcer struct {
	cfg      SpeedEnforcerConfig
	provider DeviceLimiterProvider
	shaper   *tcshape.Manager
	logger   *slog.Logger

	mu     sync.Mutex
	conn   *grpc.ClientConn
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSpeedEnforcer 创建带宽整形执行器。
func NewSpeedEnforcer(provider DeviceLimiterProvider, cfg SpeedEnforcerConfig, logger *slog.Logger) (*SpeedEnforcer, error) {
	if logger == nil {
		logger = slog.Default()
	}
	shaper, err := tcshape.NewManager(tcshape.Options{
		Dev:      cfg.Dev,
		Ifb:      cfg.Ifb,
		ServerIP: cfg.ServerIP,
		StateDir: cfg.StateDir,
		Logger:   logger,
	})
	if err != nil {
		return nil, fmt.Errorf("speed enforcer: init tc manager: %w", err)
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultSpeedEnforceInterval
	}
	return &SpeedEnforcer{
		cfg:      cfg,
		provider: provider,
		shaper:   shaper,
		logger:   logger.With("component", "speed-enforcer"),
	}, nil
}

// Start 建立 xray gRPC 连接并启动整形循环。
func (e *SpeedEnforcer) Start(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, defaultSpeedDialTimeout)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, e.cfg.APIEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("speed enforcer: connect to xray gRPC %s: %w", e.cfg.APIEndpoint, err)
	}

	e.mu.Lock()
	e.conn = conn
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()

	e.wg.Add(1)
	go e.enforceLoop()
	e.logger.Info("speed enforcer started",
		"endpoint", e.cfg.APIEndpoint,
		"ports", fmt.Sprint(e.cfg.InboundPorts),
		"dev", e.shaper.Dev(),
		"server_ip", e.shaper.ServerIP(),
		"interval", e.cfg.Interval)
	return nil
}

// Stop 停止整形循环并清理已应用的 tc/iptables 规则。
func (e *SpeedEnforcer) Stop() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()
	e.wg.Wait()

	e.mu.Lock()
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
	}
	e.mu.Unlock()

	clearCtx, clearCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer clearCancel()
	if err := e.shaper.Clear(clearCtx); err != nil {
		e.logger.Warn("speed enforcer: cleanup tc rules failed", "error", err)
	}
	e.logger.Info("speed enforcer stopped")
}

func (e *SpeedEnforcer) enforceLoop() {
	defer e.wg.Done()
	time.Sleep(5 * time.Second)
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.enforceOnce(e.ctx); err != nil {
				e.logger.Warn("speed enforcement cycle failed", "error", err)
			}
		}
	}
}

func (e *SpeedEnforcer) enforceOnce(ctx context.Context) error {
	if len(e.cfg.InboundPorts) == 0 {
		return nil
	}

	online, err := e.onlineIPs(ctx)
	if err != nil {
		// 降级：使用 DeviceLimiter 快照（由 DeviceEnforcer 从 StatsService 同步）
		online = e.provider.DeviceLimiter().GetLocalDevicesSnapshot()
	}
	if len(online) == 0 {
		// 无在线用户时清空规则，避免残留
		return e.shaper.Apply(ctx, tcshape.Plan{})
	}

	type acc struct{ up, down int }
	byIP := make(map[string]*acc)
	for userID, ips := range online {
		limitMbps := e.provider.GetSpeedLimit(userID)
		if limitMbps <= 0 || len(ips) == 0 {
			continue
		}
		// 用户的总带宽按当前在线设备数均分（避免多设备叠加超限）
		shareKbps := (limitMbps*1000 + len(ips) - 1) / len(ips)
		for _, ip := range ips {
			if ip == "" || strings.Contains(ip, ":") {
				continue // 暂不处理 IPv6
			}
			if a, ok := byIP[ip]; ok {
				a.up += shareKbps
				a.down += shareKbps
			} else {
				byIP[ip] = &acc{up: shareKbps, down: shareKbps}
			}
		}
	}

	ports := append([]int{}, e.cfg.InboundPorts...)
	sort.Ints(ports)
	plan := tcshape.Plan{Rules: make([]tcshape.Rule, 0, len(byIP))}
	for ip, a := range byIP {
		if a.up <= 0 && a.down <= 0 {
			continue
		}
		plan.Rules = append(plan.Rules, tcshape.Rule{
			IP:       ip,
			UpKbps:   a.up,
			DownKbps: a.down,
			Ports:    ports,
		})
	}
	sort.Slice(plan.Rules, func(i, j int) bool { return plan.Rules[i].IP < plan.Rules[j].IP })
	return e.shaper.Apply(ctx, plan)
}

// onlineIPs 通过 xray StatsService 获取每用户当前在线 IP 列表。
func (e *SpeedEnforcer) onlineIPs(ctx context.Context) (map[string][]string, error) {
	e.mu.Lock()
	conn := e.conn
	e.mu.Unlock()
	if conn == nil {
		return nil, fmt.Errorf("gRPC connection not established")
	}

	statsClient := statsCmd.NewStatsServiceClient(conn)
	onlineResp, err := statsClient.GetAllOnlineUsers(ctx, &statsCmd.GetAllOnlineUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetAllOnlineUsers: %w", err)
	}

	out := make(map[string][]string, len(onlineResp.GetUsers()))
	for _, email := range onlineResp.GetUsers() {
		ipResp, err := statsClient.GetStatsOnlineIpList(ctx, &statsCmd.GetStatsRequest{
			Name: statsOnlineIPPrefix + email,
		})
		if err != nil {
			continue
		}
		ips := make([]string, 0, len(ipResp.GetIps()))
		for ip := range ipResp.GetIps() {
			ips = append(ips, ip)
		}
		if len(ips) > 0 {
			out[email] = ips
		}
	}
	return out, nil
}
