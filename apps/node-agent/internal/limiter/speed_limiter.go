// Package limiter 实现节点侧的两类限流器：
//
//   - SpeedLimiter  : 基于令牌桶的每用户带宽限速（参考 Xboard-Node limiter/speedtracker.go）
//   - DeviceLimiter : 基于本地 IP 集合 + 面板下发全局设备态的每用户设备数限制
//
// 两者均为并发安全，按用户 UUID 分桶管理，支持运行时动态更新限速/限设备配置。
package limiter

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// minBurst 令牌桶的最小突发容量（64KB）。
// 即使用户限速极低，也保证至少能容纳一个常见 TCP 报文段 + 头部开销，
// 避免因 burst 过小导致限速器长期处于饥饿、无法放行任何数据。
const minBurst = 65536 // 64KB

// SpeedLimiter 每用户一个令牌桶限速器。
//
// 参考 Xboard-Node 的 limiter/speedtracker.go：每个用户（按 UUID 标识）维护
// 一个独立的 rate.Limiter，令牌补充速率 = 限速带宽对应的字节速率。
//
// 设计要点：
//   - speedLimitMbps == 0 表示不限速，GetLimiter 返回 nil，AllowN 直接放行。
//   - burst 取「限速带宽对应的字节速率」与 minBurst(64KB) 中的较大值，
//     既能允许一定突发，又避免低限速下 burst 过大导致长时间超速。
//   - SetLimit 通过 rate.Limiter.SetLimit/SetBurst 原地更新，无需重建桶。
type SpeedLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter // uuid -> limiter
}

// NewSpeedLimiter 创建一个空的限速器集合。
func NewSpeedLimiter() *SpeedLimiter {
	return &SpeedLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

// speedToLimit 将 Mbps 限速转换为令牌桶的填充速率（字节/秒）。
//   bytesPerSec = speedLimitMbps * 1_000_000 / 8
// speedLimitMbps <= 0 时返回 rate.Inf（不限速）。
func speedToLimit(speedLimitMbps int) rate.Limit {
	if speedLimitMbps <= 0 {
		return rate.Inf
	}
	// 使用浮点避免整数除法截断（低速率时差异明显）。
	bytesPerSec := float64(speedLimitMbps) * 1_000_000 / 8
	return rate.Limit(bytesPerSec)
}

// speedToBurst 将 Mbps 限速转换为令牌桶的突发容量（字节）。
// 取「1 秒可发送字节数」与 minBurst(64KB) 的较大值。
// speedLimitMbps <= 0 时返回 0（不限速场景下 burst 无意义）。
// B49 修复：使用浮点运算避免整数除法截断。
func speedToBurst(speedLimitMbps int) int {
	if speedLimitMbps <= 0 {
		return 0
	}
	bytesPerSec := int(float64(speedLimitMbps) * 1_000_000 / 8)
	if bytesPerSec < minBurst {
		return minBurst
	}
	return bytesPerSec
}

// GetLimiter 获取或创建用户的限速器。
//
// speedLimitMbps <= 0 表示不限速，直接返回 nil（调用方据此跳过限速判定）。
// 已存在限速器时返回既有实例（不会因传入新的 speedLimitMbps 而重建，
// 如需更新限速请使用 SetLimit）。
func (sl *SpeedLimiter) GetLimiter(uuid string, speedLimitMbps int) *rate.Limiter {
	if speedLimitMbps <= 0 {
		return nil
	}

	// 快路径：读锁查已有 limiter。
	sl.mu.RLock()
	if l, ok := sl.limiters[uuid]; ok {
		sl.mu.RUnlock()
		return l
	}
	sl.mu.RUnlock()

	// 慢路径：加写锁创建。二次检查避免并发重复创建。
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if l, ok := sl.limiters[uuid]; ok {
		return l
	}
	l := rate.NewLimiter(speedToLimit(speedLimitMbps), speedToBurst(speedLimitMbps))
	sl.limiters[uuid] = l
	return l
}

// SetLimit 更新用户限速（原地更新，无需重建桶）。
//
// speedLimitMbps <= 0 表示不限速：移除该用户的限速器，后续 AllowN 将直接放行。
// 若用户此前无 limiter，则按新限速创建一个。
func (sl *SpeedLimiter) SetLimit(uuid string, speedLimitMbps int) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if speedLimitMbps <= 0 {
		// 不限速：移除限速器。
		delete(sl.limiters, uuid)
		return
	}

	l, ok := sl.limiters[uuid]
	if !ok {
		sl.limiters[uuid] = rate.NewLimiter(speedToLimit(speedLimitMbps), speedToBurst(speedLimitMbps))
		return
	}
	// 原地更新速率与突发容量，保持已积累的令牌状态。
	l.SetLimit(speedToLimit(speedLimitMbps))
	l.SetBurst(speedToBurst(speedLimitMbps))
}

// RemoveLimiter 移除用户限速器（用户下线/封禁时调用，避免内存泄漏）。
func (sl *SpeedLimiter) RemoveLimiter(uuid string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	delete(sl.limiters, uuid)
}

// AllowN 非阻塞检查是否允许通过 n 字节。
//
//   - 用户无 limiter（未设置限速或已移除）或 n <= 0 时直接放行。
//   - 否则消耗 n 个令牌，令牌不足时立即返回 false（不阻塞等待）。
//
// 调用方应在实际转发数据前调用本方法，返回 false 时丢弃/延迟该批数据。
func (sl *SpeedLimiter) AllowN(uuid string, n int) bool {
	if n <= 0 {
		return true
	}
	sl.mu.RLock()
	l, ok := sl.limiters[uuid]
	sl.mu.RUnlock()
	if !ok || l == nil {
		// 无限速器 => 不限速。
		return true
	}
	return l.AllowN(time.Now(), n)
}

// Wait 阻塞等待直到允许通过 n 字节，或 ctx 被取消。
//
// 这是 AllowN 的阻塞版本，适用于 per-connection 级别的限速执行：
//   - 用户无 limiter（未设置限速或已移除）或 n <= 0 时立即返回 nil。
//   - 否则阻塞等待令牌补充，直到足够 n 个令牌可用或 ctx 取消。
//   - ctx 取消时返回 ctx.Err()，调用方应据此中止数据转发。
//
// 在 sing-box 的 trackedConn.Read/Write 中调用此方法实现 per-user 限速：
//   - Read 路径：读取 n 字节后调用 Wait(ctx, uuid, n)，延迟下一次读取
//   - Write 路径：写入前调用 Wait(ctx, uuid, len(b))，延迟实际写入
//
// 注意：当 n 超过令牌桶 burst 容量时，会自动分批等待（每次最多 burst 个），
// 避免WaitN在 n > burst 时返回错误。对于大块数据传输会自动分段等待。
func (sl *SpeedLimiter) Wait(ctx context.Context, uuid string, n int) error {
	if n <= 0 {
		return nil
	}
	sl.mu.RLock()
	l, ok := sl.limiters[uuid]
	sl.mu.RUnlock()
	if !ok || l == nil {
		return nil // 无限速器 => 不限速
	}
	// WaitN 在 n > burst 时会返回错误，需要分批等待
	burst := l.Burst()
	for n > 0 {
		chunk := n
		if chunk > burst {
			chunk = burst
		}
		if err := l.WaitN(ctx, chunk); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}

// GetLimit 返回用户当前限速器（用于观测/调试），不存在或无限速时返回 nil。
func (sl *SpeedLimiter) GetLimit(uuid string) *rate.Limiter {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.limiters[uuid]
}

// Count 返回当前已注册的限速器数量。
func (sl *SpeedLimiter) Count() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return len(sl.limiters)
}
