package executor

import (
	"context"
	"fmt"
	"log/slog"
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

	mu      sync.Mutex
	conn    *grpc.ClientConn
	blocked map[string]bool // 当前被拉黑的 email 集合
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
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
		// 无在线用户：清除 DeviceLimiter 的本地状态
		e.provider.DeviceLimiter().ClearLocalDevices()
		return nil
	}

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
