package limiter

import (
	"sync"
)

// IPLimiter IP 限制器。
//
// 提供黑白名单 + IP 数限制功能：
//
//  1. blockedIPs：全局黑名单，任何用户使用该 IP 均被拒绝。
//
//  2. allowedIPs：用户白名单（按 uuid）。若配置了白名单，仅允许列表内 IP 连接。
//
//  3. activeIPs：当前在线 IP 集合（uuid -> ip -> refcount），用于 IP 数限制。
//     引用计数用于正确处理同一 IP 的多连接复用。
//
// 判定优先级：黑名单 > 白名单 > IP 数限制。
type IPLimiter struct {
	mu           sync.RWMutex
	allowedIPs   map[string]map[string]bool // uuid -> 允许的 IP 白名单集合
	blockedIPs   map[string]bool            // 全局黑名单 IP 集合
	activeIPs    map[string]map[string]int  // uuid -> ip -> 引用计数
	maxIPsPerUser map[string]int            // uuid -> 最大 IP 数限制（备用，实际从 OnConnect 参数传入）
}

// NewIPLimiter 创建一个空的 IP 限制器。
func NewIPLimiter() *IPLimiter {
	return &IPLimiter{
		allowedIPs:    make(map[string]map[string]bool),
		blockedIPs:    make(map[string]bool),
		activeIPs:     make(map[string]map[string]int),
		maxIPsPerUser: make(map[string]int),
	}
}

// OnConnect 连接建立时调用，返回是否允许该连接。
//
// 判定逻辑：
//   1. 检查 IP 是否在全局黑名单中，是则拒绝。
//   2. 若该用户配置了白名单且 IP 不在白名单内，拒绝。
//   3. 若 ipLimit > 0 且该 IP 未连接过，检查当前在线 IP 数是否已达上限，是则拒绝。
//   4. 同一 IP 重连/多连接始终放行。
//   5. 放行后递增该 IP 的引用计数。
//
// 返回 true 表示允许连接，false 表示被拒绝。
func (il *IPLimiter) OnConnect(uuid string, ip string, ipLimit int) bool {
	il.mu.Lock()
	defer il.mu.Unlock()

	if il.blockedIPs[ip] {
		return false
	}

	if allowed, hasWhitelist := il.allowedIPs[uuid]; hasWhitelist {
		if len(allowed) > 0 && !allowed[ip] {
			return false
		}
	}

	if ipLimit > 0 {
		if !il.hasIPLocked(uuid, ip) {
			current := il.countIPsLocked(uuid)
			if current >= ipLimit {
				return false
			}
		}
	}

	if il.activeIPs[uuid] == nil {
		il.activeIPs[uuid] = make(map[string]int)
	}
	il.activeIPs[uuid][ip]++
	il.maxIPsPerUser[uuid] = ipLimit
	return true
}

// OnDisconnect 连接断开时调用。
//
// 递减该 IP 的引用计数，归零时从在线集合移除该 IP；
// 用户无任何在线 IP 时清理其条目，避免 map 无限增长。
func (il *IPLimiter) OnDisconnect(uuid string, ip string) {
	il.mu.Lock()
	defer il.mu.Unlock()

	ips, ok := il.activeIPs[uuid]
	if !ok {
		return
	}
	if cnt, ok := ips[ip]; ok {
		cnt--
		if cnt <= 0 {
			delete(ips, ip)
		} else {
			ips[ip] = cnt
		}
	}
	if len(ips) == 0 {
		delete(il.activeIPs, uuid)
	}
}

// BlockIP 将 IP 添加到全局黑名单。
//
// 黑名单优先级最高，已在线的连接不受影响，仅拒绝后续新连接。
func (il *IPLimiter) BlockIP(ip string) {
	il.mu.Lock()
	defer il.mu.Unlock()
	il.blockedIPs[ip] = true
}

// UnblockIP 将 IP 从全局黑名单移除。
func (il *IPLimiter) UnblockIP(ip string) {
	il.mu.Lock()
	defer il.mu.Unlock()
	delete(il.blockedIPs, ip)
}

// SetAllowedIPs 设置指定用户的 IP 白名单。
//
// 若 ips 为空，则该用户白名单被清空（不启用白名单模式）。
// 仅白名单内的 IP 可连接（黑名单优先级仍高于白名单）。
func (il *IPLimiter) SetAllowedIPs(uuid string, ips []string) {
	il.mu.Lock()
	defer il.mu.Unlock()

	if len(ips) == 0 {
		delete(il.allowedIPs, uuid)
		return
	}

	allowed := make(map[string]bool, len(ips))
	for _, ip := range ips {
		allowed[ip] = true
	}
	il.allowedIPs[uuid] = allowed
}

// ClearAllowedIPs 清除指定用户的 IP 白名单。
func (il *IPLimiter) ClearAllowedIPs(uuid string) {
	il.mu.Lock()
	defer il.mu.Unlock()
	delete(il.allowedIPs, uuid)
}

// IsIPBlocked 检查 IP 是否在全局黑名单中。
func (il *IPLimiter) IsIPBlocked(ip string) bool {
	il.mu.RLock()
	defer il.mu.RUnlock()
	return il.blockedIPs[ip]
}

// GetIPsForUser 获取指定用户当前在线的 IP 列表（去重）。
func (il *IPLimiter) GetIPsForUser(uuid string) []string {
	il.mu.RLock()
	defer il.mu.RUnlock()

	ips, ok := il.activeIPs[uuid]
	if !ok {
		return nil
	}
	ipList := make([]string, 0, len(ips))
	for ip := range ips {
		ipList = append(ipList, ip)
	}
	return ipList
}

// CleanupStaleEntries 清理空的用户条目以防止内存泄漏。
//
// 遍历白名单、在线 IP、限制配置，移除无任何数据的用户条目。
// 建议定期调用（如每分钟一次）。
func (il *IPLimiter) CleanupStaleEntries() {
	il.mu.Lock()
	defer il.mu.Unlock()

	for uuid := range il.allowedIPs {
		if len(il.allowedIPs[uuid]) == 0 {
			delete(il.allowedIPs, uuid)
		}
	}

	for uuid := range il.activeIPs {
		if len(il.activeIPs[uuid]) == 0 {
			delete(il.activeIPs, uuid)
		}
	}

	for uuid := range il.maxIPsPerUser {
		if _, ok := il.activeIPs[uuid]; !ok {
			delete(il.maxIPsPerUser, uuid)
		}
	}
}

// GetBlockedIPs 获取当前所有被封禁的 IP 列表（快照）。
func (il *IPLimiter) GetBlockedIPs() []string {
	il.mu.RLock()
	defer il.mu.RUnlock()

	ips := make([]string, 0, len(il.blockedIPs))
	for ip := range il.blockedIPs {
		ips = append(ips, ip)
	}
	return ips
}

// HasWhitelist 检查指定用户是否配置了白名单。
func (il *IPLimiter) HasWhitelist(uuid string) bool {
	il.mu.RLock()
	defer il.mu.RUnlock()
	allowed, ok := il.allowedIPs[uuid]
	return ok && len(allowed) > 0
}

// GetActiveIPCount 获取指定用户当前在线 IP 数（去重）。
func (il *IPLimiter) GetActiveIPCount(uuid string) int {
	il.mu.RLock()
	defer il.mu.RUnlock()
	return il.countIPsLocked(uuid)
}

// countIPsLocked 返回指定用户当前在线去重 IP 数（调用方持锁）。
func (il *IPLimiter) countIPsLocked(uuid string) int {
	return len(il.activeIPs[uuid])
}

// hasIPLocked 判断该 IP 是否已在在线集合中（调用方持锁）。
func (il *IPLimiter) hasIPLocked(uuid string, ip string) bool {
	ips, ok := il.activeIPs[uuid]
	if !ok {
		return false
	}
	_, ok = ips[ip]
	return ok
}
