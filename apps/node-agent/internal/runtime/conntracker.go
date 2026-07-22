// Package runtime 提供 sing-box 的 per-user 流量统计 ConnTracker。
//
// 本文件移植自 Xboard-Node 的 internal/kernel/singbox/conntracker.go，
// 实现 sing-box 的 adapter.ConnectionTracker 接口，通过包装连接的 Read/Write
// 方法，使用 atomic.Int64 无锁累加每用户的上下行字节数。
//
// 设计要点：
//   - 流量计数使用 atomic.Int64，无锁化设计，在每个数据包的 Read/Write 时直接累加
//   - GetAndReset 使用 Swap(0) 原子读取并清零，实现增量上报
//   - Get 非破坏性读取，用于容错上报（失败后下次仍可读到完整数据）
//   - 集成 SpeedLimiter 实现 per-user 限速：在 Read/Write 时阻塞等待令牌
//   - 集成 DeviceChecker 实现 per-user 设备数限制：在连接建立时检查并拒绝超额连接
//   - O(users) 读取复杂度，与连接数无关
//   - 无需 ClashAPI / V2RayAPI，零 IPC 开销
package runtime

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// RateLimiter 提供 per-user 的阻塞式速率限制接口。
// limiter.SpeedLimiter 实现此接口（通过 Wait 方法）。
// 使用接口而非具体类型，避免 runtime 包直接依赖 limiter 包。
type RateLimiter interface {
	// Wait 阻塞等待直到允许通过 n 字节，或 ctx 被取消。
	// 返回 nil 表示放行，返回 error 表示 ctx 取消（应中止数据转发）。
	Wait(ctx context.Context, userID string, n int) error
}

// DeviceChecker 提供 per-user 的设备数限制检查接口。
// executor 包中的适配器将 DeviceLimiter + 设备限制查询封装为此接口。
type DeviceChecker interface {
	// OnConnect 在新连接建立时调用，返回是否允许该连接。
	// 返回 true 表示允许，false 表示因超出设备数限制而拒绝。
	OnConnect(userID string, ip string) bool
	// OnDisconnect 在连接关闭时调用，释放设备计数。
	OnDisconnect(userID string, ip string)
}

// IPChecker 提供 per-user 的 IP 限制检查接口（黑名单/白名单/IP数限制）。
// executor 包中的适配器将 limiter.IPLimiter + IP限制查询封装为此接口。
// 接口签名与 DeviceChecker 相同，但语义不同：IPChecker 负责黑白名单和IP数限制。
type IPChecker interface {
	// OnConnect 在新连接建立时调用，返回是否允许该连接。
	// 返回 true 表示允许，false 表示因IP黑名单/白名单/IP数超限而拒绝。
	OnConnect(userID string, ip string) bool
	// OnDisconnect 在连接关闭时调用，释放IP引用计数。
	OnDisconnect(userID string, ip string)
}

// userStats 每用户的流量统计（无锁化设计）。
// 流量计数使用 atomic.Int64，在每个数据包的 Read/Write 时直接累加，
// 读取时使用 Swap(0) 原子读取并清零（GetAndReset）或 Load 原子读取（Get）。
type userStats struct {
	upload   atomic.Int64 // 上传字节（用户 → 入站 Read）
	download atomic.Int64 // 下载字节（入站 → 用户 Write）
}

// ConnTracker 实现 sing-box 的 adapter.ConnectionTracker 接口。
//
// 当 sing-box 路由器将连接路由到出站时，会调用 RoutedConnection/RoutedPacketConnection，
// 此处包装原始连接，在 Read/Write 时累加字节数到对应用户的 userStats，
// 并通过 RateLimiter 执行 per-user 限速，通过 DeviceChecker 执行设备数限制。
//
// 线程安全：users map 使用 RWMutex 保护，userStats 内部使用 atomic 无锁操作。
type ConnTracker struct {
	mu            sync.RWMutex
	users         map[string]*userStats // userIdentifier(name 字段) → 统计
	limiter       RateLimiter           // 限速器（nil = 不限速）
	deviceChecker DeviceChecker         // 设备数检查器（nil = 不限制）
	ipLimiter     IPChecker             // IP限制检查器（nil = 不限制）
}

// NewConnTracker 创建 ConnTracker 实例。
func NewConnTracker() *ConnTracker {
	return &ConnTracker{
		users: make(map[string]*userStats),
	}
}

// SetSpeedLimiter 设置 per-user 限速器。
// 传入 nil 可禁用限速。此方法可在运行时调用（热更新限速配置）。
// 已有连接的 trackedConn 会通过 tracker.limiter 动态读取最新限速器引用。
func (t *ConnTracker) SetSpeedLimiter(l RateLimiter) {
	t.mu.Lock()
	t.limiter = l
	t.mu.Unlock()
}

// SetDeviceChecker 设置 per-user 设备数检查器。
// 传入 nil 可禁用设备数限制。此方法可在运行时调用。
// 设置后，新建立的连接会经过设备数检查，超额连接将被拒绝。
func (t *ConnTracker) SetDeviceChecker(dc DeviceChecker) {
	t.mu.Lock()
	t.deviceChecker = dc
	t.mu.Unlock()
}

// SetIPLimiter 设置 per-user IP 限制检查器。
// 传入 nil 可禁用 IP 限制。此方法可在运行时调用（热更新 IP 黑白名单/IP数限制）。
// 设置后，新建立的连接会经过 IP 检查（黑名单 > 白名单 > IP数限制），被拒绝的连接将被关闭。
func (t *ConnTracker) SetIPLimiter(il IPChecker) {
	t.mu.Lock()
	t.ipLimiter = il
	t.mu.Unlock()
}

// getOrCreateUser 返回（或创建）指定用户的 userStats（调用方无需持锁）。
func (t *ConnTracker) getOrCreateUser(userID string) *userStats {
	// 快速路径：读锁查找
	t.mu.RLock()
	us, ok := t.users[userID]
	t.mu.RUnlock()
	if ok {
		return us
	}

	// 慢速路径：写锁创建
	t.mu.Lock()
	defer t.mu.Unlock()
	// double-check（可能已被其他 goroutine 创建）
	if us, ok := t.users[userID]; ok {
		return us
	}
	us = &userStats{}
	t.users[userID] = us
	return us
}

// addUpload 累加用户上传字节。
func (t *ConnTracker) addUpload(userID string, n int64) {
	us := t.getOrCreateUser(userID)
	us.upload.Add(n)
}

// addDownload 累加用户下载字节。
func (t *ConnTracker) addDownload(userID string, n int64) {
	us := t.getOrCreateUser(userID)
	us.download.Add(n)
}

// GetAndReset 原子读取所有用户流量并清零（用于增量上报）。
// 返回 map[userID] [2]int64{upload, download}，仅包含有流量的用户。
func (t *ConnTracker) GetAndReset() map[string][2]int64 {
	t.mu.RLock()
	result := make(map[string][2]int64, len(t.users))
	for userID, us := range t.users {
		up := us.upload.Swap(0) // 原子读取并清零
		down := us.download.Swap(0)
		if up > 0 || down > 0 {
			result[userID] = [2]int64{up, down}
		}
	}
	t.mu.RUnlock()
	return result
}

// Get 非破坏性读取所有用户流量（不清零）。
//
// 用于容错上报模式：
//   - 先用 Get 读取当前累计值
//   - 计算与上次上报的差值（delta = current - lastReported）
//   - 上报成功后更新 lastReported = current
//   - 上报失败时不更新 lastReported，下次上报自动包含未上报的流量
//
// 返回 map[userID] [2]int64{upload, download}，仅包含有流量的用户。
func (t *ConnTracker) Get() map[string][2]int64 {
	t.mu.RLock()
	result := make(map[string][2]int64, len(t.users))
	for userID, us := range t.users {
		up := us.upload.Load()
		down := us.download.Load()
		if up > 0 || down > 0 {
			result[userID] = [2]int64{up, down}
		}
	}
	t.mu.RUnlock()
	return result
}

// Reset 清空所有用户统计（用于配置重载后重置状态）。
func (t *ConnTracker) Reset() {
	t.mu.Lock()
	t.users = make(map[string]*userStats)
	t.mu.Unlock()
}

// getLimiter 返回当前限速器（可为 nil）。
func (t *ConnTracker) getLimiter() RateLimiter {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.limiter
}

// getDeviceChecker 返回当前设备数检查器（可为 nil）。
func (t *ConnTracker) getDeviceChecker() DeviceChecker {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.deviceChecker
}

// getIPLimiter 返回当前 IP 限制检查器（可为 nil）。
func (t *ConnTracker) getIPLimiter() IPChecker {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ipLimiter
}

// extractSourceIP 从 sing-box InboundContext 中提取源 IP 字符串。
// 如果无法提取（如 FQDN 连接），返回空字符串。
func extractSourceIP(metadata adapter.InboundContext) string {
	if metadata.Source.IsValid() {
		return metadata.Source.Addr.String()
	}
	return ""
}

// RoutedConnection 实现 adapter.ConnectionTracker 接口。
//
// sing-box 路由器在将 TCP 连接路由到出站时调用此方法。
// 此处包装原始 conn：
//   - 检查设备数限制：如果用户已超出设备数限制，拒绝新连接（关闭并返回原 conn）
//   - 在 Read/Write 时累加字节数到 metadata.User 对应的 userStats
//   - 通过 RateLimiter 执行 per-user 限速（Wait 阻塞等待令牌）
//   - Close 时取消限速 Wait 的阻塞 + 通知 DeviceChecker 释放设备计数
func (t *ConnTracker) RoutedConnection(
	ctx context.Context,
	conn net.Conn,
	metadata adapter.InboundContext,
	matchedRule adapter.Rule,
	matchOutbound adapter.Outbound,
) net.Conn {
	userID := metadata.User
	if userID == "" {
		return conn // 无用户标识（如 API inbound），不统计
	}

	// 提取源 IP 用于设备数限制
	sourceIP := extractSourceIP(metadata)

	// 设备数检查：如果超出限制，拒绝新连接
	if dc := t.getDeviceChecker(); dc != nil && sourceIP != "" {
		if !dc.OnConnect(userID, sourceIP) {
			// 设备数超限：关闭连接，返回已关闭的 conn
			// sing-box 路由器会检测到连接已关闭并清理资源
			conn.Close()
			return conn
		}
	}

	// IP 限制检查：黑名单/白名单/IP数超限则拒绝连接
	if il := t.getIPLimiter(); il != nil && sourceIP != "" {
		if !il.OnConnect(userID, sourceIP) {
			// IP 被拒绝：关闭连接，返回已关闭的 conn
			conn.Close()
			return conn
		}
	}

	// 创建可取消的 context，用于在连接关闭时中断限速 Wait
	cancelCtx, cancel := context.WithCancel(context.Background())
	return &trackedConn{
		Conn:     conn,
		tracker:  t,
		userID:   userID,
		sourceIP: sourceIP,
		ctx:      cancelCtx,
		cancel:   cancel,
	}
}

// RoutedPacketConnection 实现 adapter.ConnectionTracker 接口。
//
// sing-box 路由器在将 UDP 连接路由到出站时调用此方法。
// 此处包装原始 packetConn，在 ReadPacket/WritePacket 时累加字节数并执行限速。
// 同样执行设备数检查。
func (t *ConnTracker) RoutedPacketConnection(
	ctx context.Context,
	conn N.PacketConn,
	metadata adapter.InboundContext,
	matchedRule adapter.Rule,
	matchOutbound adapter.Outbound,
) N.PacketConn {
	userID := metadata.User
	if userID == "" {
		return conn
	}

	sourceIP := extractSourceIP(metadata)

	if dc := t.getDeviceChecker(); dc != nil && sourceIP != "" {
		if !dc.OnConnect(userID, sourceIP) {
			conn.Close()
			return conn
		}
	}

	// IP 限制检查：黑名单/白名单/IP数超限则拒绝连接
	if il := t.getIPLimiter(); il != nil && sourceIP != "" {
		if !il.OnConnect(userID, sourceIP) {
			conn.Close()
			return conn
		}
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	return &trackedPacketConn{
		PacketConn: conn,
		tracker:    t,
		userID:     userID,
		sourceIP:   sourceIP,
		ctx:        cancelCtx,
		cancel:     cancel,
	}
}

// trackedConn 包装 net.Conn，在 Read/Write 时累加流量并执行 per-user 限速。
//
// 限速策略：
//   - Read: 读取 n 字节后调用 limiter.Wait(ctx, userID, n) 消耗令牌，
//     阻塞延迟下一次 Read 调用，实现上传限速。
//   - Write: 写入前调用 limiter.Wait(ctx, userID, len(b)) 等待令牌，
//     阻塞延迟实际写入，实现下载限速。
//   - Close: 取消 context + 通知 DeviceChecker 释放设备计数。
//   - 限速器为 nil 时不执行任何限速（零开销）。
type trackedConn struct {
	net.Conn
	tracker  *ConnTracker
	userID   string
	sourceIP string
	ctx      context.Context
	cancel   context.CancelFunc
}

func (c *trackedConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		// 限速：等待令牌（阻塞式），实现上传限速
		if limiter := c.tracker.getLimiter(); limiter != nil {
			if waitErr := limiter.Wait(c.ctx, c.userID, n); waitErr != nil {
				// context 取消（连接已关闭），直接返回
				return n, err
			}
		}
		c.tracker.addUpload(c.userID, int64(n))
	}
	return n, err
}

func (c *trackedConn) Write(b []byte) (int, error) {
	// 限速：写入前等待令牌（阻塞式），实现下载限速
	if limiter := c.tracker.getLimiter(); limiter != nil {
		if err := limiter.Wait(c.ctx, c.userID, len(b)); err != nil {
			// context 取消（连接已关闭），返回 0 写入
			return 0, err
		}
	}
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.tracker.addDownload(c.userID, int64(n))
	}
	return n, err
}

// Close 关闭连接并取消限速 context，释放所有阻塞的 Wait 调用，
// 同时通知 DeviceChecker 释放设备计数，通知 IPLimiter 释放 IP 引用计数。
func (c *trackedConn) Close() error {
	c.cancel()
	if dc := c.tracker.getDeviceChecker(); dc != nil && c.sourceIP != "" {
		dc.OnDisconnect(c.userID, c.sourceIP)
	}
	if il := c.tracker.getIPLimiter(); il != nil && c.sourceIP != "" {
		il.OnDisconnect(c.userID, c.sourceIP)
	}
	return c.Conn.Close()
}

// trackedPacketConn 包装 N.PacketConn，在 ReadPacket/WritePacket 时累加流量并执行限速。
//
// sing-box 的 N.PacketConn 接口使用 *buf.Buffer 而非 []byte：
//   ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error)
//   WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error
// 字节数通过 buffer.Len() 获取。
type trackedPacketConn struct {
	N.PacketConn
	tracker  *ConnTracker
	userID   string
	sourceIP string
	ctx      context.Context
	cancel   context.CancelFunc
}

func (c *trackedPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	destination, err := c.PacketConn.ReadPacket(buffer)
	if err == nil {
		if n := buffer.Len(); n > 0 {
			// 限速：等待令牌
			if limiter := c.tracker.getLimiter(); limiter != nil {
				limiter.Wait(c.ctx, c.userID, n)
			}
			c.tracker.addUpload(c.userID, int64(n))
		}
	}
	return destination, err
}

func (c *trackedPacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	n := buffer.Len()
	// 限速：写入前等待令牌
	if limiter := c.tracker.getLimiter(); limiter != nil {
		if err := limiter.Wait(c.ctx, c.userID, n); err != nil {
			return err
		}
	}
	err := c.PacketConn.WritePacket(buffer, destination)
	if err == nil && n > 0 {
		c.tracker.addDownload(c.userID, int64(n))
	}
	return err
}

// Close 关闭连接并取消限速 context，释放设备计数和 IP 引用计数。
func (c *trackedPacketConn) Close() error {
	c.cancel()
	if dc := c.tracker.getDeviceChecker(); dc != nil && c.sourceIP != "" {
		dc.OnDisconnect(c.userID, c.sourceIP)
	}
	if il := c.tracker.getIPLimiter(); il != nil && c.sourceIP != "" {
		il.OnDisconnect(c.userID, c.sourceIP)
	}
	return c.PacketConn.Close()
}
