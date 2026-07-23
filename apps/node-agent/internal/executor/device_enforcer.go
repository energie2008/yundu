package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	statsCmd "github.com/xtls/xray-core/app/stats/command"
	proxymanCmd "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/serial"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// defaultXrayAPIEndpoint 是 xray 内置 gRPC API 的默认地址。
	// 控制面（kernelrender）已在 xray 配置中启用 api inbound（tag=api, port=10085）。
	defaultXrayAPIEndpoint = "127.0.0.1:10085"
	// defaultEnforceInterval 是设备限制检查的默认间隔。
	defaultEnforceInterval = 15 * time.Second
	// defaultGRPCDialTimeout 是连接 xray gRPC API 的超时时间。
	defaultGRPCDialTimeout = 3 * time.Second
	// statsNamePrefix 是 xray StatsService 中用户在线 IP 统计的名称前缀。
	// xray 内部使用 "user>><email>" 作为 per-user 在线 IP 统计的 key。
	statsOnlineIPPrefix = "user>>"
)

// DeviceEnforcerConfig 配置设备限制执行器。
type DeviceEnforcerConfig struct {
	// APIEndpoint 是 xray gRPC API 地址（默认 127.0.0.1:10085）。
	APIEndpoint string
	// Interval 是设备限制检查间隔（默认 15s）。
	Interval time.Duration
	// InboundTag 是需要执行用户移除操作的 inbound tag。
	// 为空时从配置中自动推断（取第一个非 api 的 inbound）。
	InboundTag string
}

// ReloadFunc 是配置重载回调，用于在用户被拉黑后连接清零时重新加载配置以恢复用户。
type ReloadFunc func(ctx context.Context) error

// DeviceEnforcer 通过 xray gRPC StatsService 查询每用户在线 IP 数，
// 超过 device_limit 时通过 HandlerService 移除用户（拉黑）。
//
// 工作流程：
//  1. 连接 xray gRPC API（127.0.0.1:10085）
//  2. 定期调用 GetAllOnlineUsers 获取在线用户列表
//  3. 对每个用户调用 GetStatsOnlineIpList 获取其在线 IP 列表
//  4. 将权威 IP 数据同步到 DeviceLimiter（替代 OnConnect/OnDisconnect）
//  5. 当 IP 数超过 device_limit 时，通过 AlterInbound + RemoveUserOperation 移除用户
//  6. 被拉黑用户的连接清零后，通过 ReloadFunc 触发配置重载以恢复用户
//
// 注意：步骤 5 移除用户后，用户无法发起新连接；步骤 6 的恢复通过全量重载实现，
// 未来可优化为直接构造 AddUserOperation 实现零断流恢复（需构造 protocol.User）。
type DeviceEnforcer struct {
	cfg       DeviceEnforcerConfig
	provider  DeviceLimiterProvider
	reloadFn  ReloadFunc
	logger    *slog.Logger

	mu                sync.Mutex
	conn              *grpc.ClientConn
	blocked           map[string]bool // 当前被拉黑的 email 集合
	consecutiveEmpty  int             // GetAllOnlineUsers 连续返回空的次数，防止 gRPC 抖动误清
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// NewDeviceEnforcer 创建设备限制执行器。
func NewDeviceEnforcer(provider DeviceLimiterProvider, cfg DeviceEnforcerConfig, reloadFn ReloadFunc, logger *slog.Logger) *DeviceEnforcer {
	if cfg.APIEndpoint == "" {
		cfg.APIEndpoint = defaultXrayAPIEndpoint
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultEnforceInterval
	}
	return &DeviceEnforcer{
		cfg:      cfg,
		provider: provider,
		reloadFn: reloadFn,
		logger:   logger.With("component", "device-enforcer"),
		blocked:  make(map[string]bool),
	}
}

// Start 连接 xray gRPC 并启动设备限制执行循环。
// 连接失败时返回 error（调用方可选择稍后重试或跳过设备限制）。
func (e *DeviceEnforcer) Start(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, defaultGRPCDialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, e.cfg.APIEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("device enforcer: connect to xray gRPC %s: %w", e.cfg.APIEndpoint, err)
	}

	e.mu.Lock()
	e.conn = conn
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()

	e.wg.Add(1)
	go e.enforceLoop()

	e.logger.Info("device enforcer started",
		"endpoint", e.cfg.APIEndpoint, "interval", e.cfg.Interval)
	return nil
}

// Stop 停止设备限制执行循环并关闭 gRPC 连接。
func (e *DeviceEnforcer) Stop() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()

	e.wg.Wait()

	e.mu.Lock()
	if e.conn != nil {
		e.conn.Close()
		e.conn = nil
	}
	e.mu.Unlock()

	e.logger.Info("device enforcer stopped")
}

// enforceLoop 运行周期性设备限制执行。
func (e *DeviceEnforcer) enforceLoop() {
	defer e.wg.Done()

	// 首次延迟 5s，等待 xray API 就绪
	time.Sleep(5 * time.Second)

	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.enforceOnce(e.ctx); err != nil {
				e.logger.Warn("device enforcement cycle failed", "error", err)
			}
		}
	}
}

// enforceOnce 执行一次设备限制检查周期。
func (e *DeviceEnforcer) enforceOnce(ctx context.Context) error {
	e.mu.Lock()
	conn := e.conn
	e.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("gRPC connection not established")
	}

	statsClient := statsCmd.NewStatsServiceClient(conn)

	// 1. 获取所有在线用户
	onlineResp, err := statsClient.GetAllOnlineUsers(ctx, &statsCmd.GetAllOnlineUsersRequest{})
	if err != nil {
		return fmt.Errorf("GetAllOnlineUsers: %w", err)
	}

	onlineUsers := onlineResp.GetUsers()
	if len(onlineUsers) == 0 {
		// ⚠️ 关键修复：区分"真无用户"和"查询异常"
		// GetAllOnlineUsers 返回空有三种可能：
		//   a) 真的没有在线用户（正常）
		//   b) gRPC 连到了错误的 xray 实例（如孤儿进程，已由 stopOrphanXray 修复）
		//   c) xray statsUserOnline 配置未生效（GetAllOnlineUsers 返回空但 QueryStats 有流量）
		// 用 QueryStats 交叉验证，避免误清在线用户的设备状态
		e.consecutiveEmpty++
		userStatCount, pingErr := e.pingStatsService(ctx, conn)
		if pingErr != nil {
			// gRPC 通路异常，绝不能 ClearLocalDevices，否则会误清在线用户
			e.logger.Warn("gRPC endpoint unreachable, skipping ClearLocalDevices",
				"endpoint", e.cfg.APIEndpoint,
				"consecutive_empty", e.consecutiveEmpty,
				"error", pingErr)
			return fmt.Errorf("pingStatsService: %w", pingErr)
		}

		if userStatCount > 0 {
			// QueryStats 有用户流量但 GetAllOnlineUsers 返回空
			// → statsUserOnline 可能未正确配置，或 GetAllOnlineUsers API 不兼容当前 xray 版本
			// 不清空 local devices，因为实际有用户在线
			e.logger.Warn("GetAllOnlineUsers returned 0 but QueryStats found user traffic",
				"endpoint", e.cfg.APIEndpoint,
				"user_stat_count", userStatCount,
				"consecutive_empty", e.consecutiveEmpty,
				"hint", "check xray policy.levels.*.statsUserOnline and inbound clients email field")
			return nil
		}

		// QueryStats 也无用户流量 → 真无在线用户
		// 连续空 >= 3 次才 ClearLocalDevices，防止 gRPC 抖动导致误清
		if e.consecutiveEmpty >= 3 {
			e.logger.Info("cleared local devices after consecutive empty responses",
				"consecutive_empty", e.consecutiveEmpty)
			e.provider.DeviceLimiter().ClearLocalDevices()
			e.consecutiveEmpty = 0
		} else {
			e.logger.Debug("GetAllOnlineUsers returned 0 users (likely no active connections)",
				"endpoint", e.cfg.APIEndpoint,
				"consecutive_empty", e.consecutiveEmpty)
		}
		return nil
	}

	// 有在线用户，重置连续空计数
	e.consecutiveEmpty = 0

	deviceLimiter := e.provider.DeviceLimiter()
	var blockedCleared []string

	// 2. 查询每个在线用户的 IP 列表并执行限制
	for _, email := range onlineUsers {
		ipResp, err := statsClient.GetStatsOnlineIpList(ctx, &statsCmd.GetStatsRequest{
			Name: statsOnlineIPPrefix + email,
		})
		if err != nil {
			e.logger.Debug("failed to get online IPs for user",
				"email", email, "error", err)
			continue
		}

		ips := ipResp.GetIps() // map[string]int64 (IP -> connection count)
		ipCount := len(ips)

		// 将权威 IP 数据同步到 DeviceLimiter（供面板上报使用）
		ipList := make([]string, 0, ipCount)
		for ip := range ips {
			ipList = append(ipList, ip)
		}
		deviceLimiter.SyncLocalDevices(email, ipList)

		// 检查设备限制
		deviceLimit := e.provider.GetDeviceLimit(email)
		if deviceLimit <= 0 {
			continue
		}

		e.mu.Lock()
		isBlocked := e.blocked[email]
		e.mu.Unlock()

		if ipCount > deviceLimit && !isBlocked {
			// 超过限制：通过 HandlerService 移除用户
			e.logger.Info("user exceeds device limit, blocking",
				"email", email, "ip_count", ipCount, "limit", deviceLimit)

			if err := e.removeUser(ctx, conn, email); err != nil {
				e.logger.Warn("failed to remove user from inbound",
					"email", email, "error", err)
				continue
			}

			e.mu.Lock()
			e.blocked[email] = true
			e.mu.Unlock()
		} else if ipCount == 0 && isBlocked {
			// 被拉黑用户的连接已清零：标记为可恢复
			blockedCleared = append(blockedCleared, email)
		}
	}

	// 3. 恢复被拉黑且连接已清零的用户
	if len(blockedCleared) > 0 && e.reloadFn != nil {
		for _, email := range blockedCleared {
			e.mu.Lock()
			delete(e.blocked, email)
			e.mu.Unlock()
		}
		e.logger.Info("blocked users' connections cleared, reloading config to restore",
			"users", blockedCleared)
		if err := e.reloadFn(ctx); err != nil {
			e.logger.Warn("reload to restore blocked users failed", "error", err)
		}
	}

	return nil
}

// removeUser 通过 HandlerService.AlterInbound + RemoveUserOperation 从 inbound 中移除用户。
func (e *DeviceEnforcer) removeUser(ctx context.Context, conn *grpc.ClientConn, email string) error {
	if e.cfg.InboundTag == "" {
		return fmt.Errorf("inbound tag not configured")
	}

	handlerClient := proxymanCmd.NewHandlerServiceClient(conn)
	op := &proxymanCmd.RemoveUserOperation{Email: email}
	req := &proxymanCmd.AlterInboundRequest{
		Tag:       e.cfg.InboundTag,
		Operation: serial.ToTypedMessage(op),
	}

	_, err := handlerClient.AlterInbound(ctx, req)
	return err
}

// pingStatsService 通过 QueryStats 查询 "user>>>" 前缀的流量统计，
// 用于在 GetAllOnlineUsers 返回空时交叉验证 gRPC 通路和 xray stats 是否正常工作。
//
// 返回值：
//   - int: 有流量统计记录的用户数（uplink 或 downlink > 0 的用户）
//   - error: gRPC 调用失败时返回 error
//
// 使用场景：GetAllOnlineUsers 返回空时调用此方法，
// 如果返回的 userStatCount > 0，说明实际有用户在线但 GetAllOnlineUsers API 未返回，
// 可能是 statsUserOnline 配置问题或 API 版本不兼容，此时不应 ClearLocalDevices。
func (e *DeviceEnforcer) pingStatsService(ctx context.Context, conn *grpc.ClientConn) (int, error) {
	statsClient := statsCmd.NewStatsServiceClient(conn)

	// 查询所有 "user>>>" 前缀的统计项（per-user uplink/downlink）
	// Pattern 匹配会返回所有以 "user>>" 开头的统计项
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := statsClient.QueryStats(pingCtx, &statsCmd.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  false,
	})
	if err != nil {
		return 0, fmt.Errorf("QueryStats(user>>>): %w", err)
	}

	// 统计有流量记录的不同用户数
	// QueryStats 返回的 Stat 列表中，每个用户有 uplink 和 downlink 两条记录
	// 提取不同的 email 并计数
	userSet := make(map[string]bool)
	for _, stat := range resp.GetStat() {
		name := stat.GetName()
		// name 格式: "user>><email>>>uplink" 或 "user>><email>>>downlink"
		// 提取 email 部分
		if rest, ok := strings.CutPrefix(name, statsOnlineIPPrefix); ok {
			parts := strings.SplitN(rest, ">>>", 2)
			if len(parts) > 0 && parts[0] != "" {
				userSet[parts[0]] = true
			}
		}
	}
	return len(userSet), nil
}
