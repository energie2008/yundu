package limiter

import (
	"sync"
	"time"
)

// globalStaleThreshold 全局设备态过期阈值。
//
// 面板通过 WebSocket 周期性下发各用户跨节点在线设备数（globalDevices）。
// 若距上次下发超过该阈值，则认为全局态已陈旧（WS 断连或面板不可达），
// 设备数判定退化为仅使用本节点本地 IP 集合，避免使用过期数据错误拒绝连接。
const globalStaleThreshold = 60 * time.Second

// DeviceLimiter 设备数限制器。
//
// 参考 Xboard-Node 的设备限制逻辑，合并两个来源判定某用户是否已达设备数上限：
//
//  1. localDevices：本节点观测到的在线 IP 集合（ip -> 引用计数）。
//     引用计数用于正确处理同一 IP 的多连接复用（断开一条仅减少计数，归零才移除）。
//
//  2. globalDevices：面板下发的跨节点设备总数（含本节点）。
//     通过 WS 推送，带 globalUpdatedAt 时间戳用于判定新鲜度。
//
// 合并策略：取 max(本地去重IP数, 全局设备数)。全局态过期(>60s)时退化为本地判定。
// 超过 deviceLimit 时拒绝新 IP 连接；已存在 IP 重连始终放行。
type DeviceLimiter struct {
	mu              sync.RWMutex
	localDevices    map[string]map[string]int // uuid -> ip -> refcount
	globalDevices   map[string]int            // uuid -> global device count (from panel)
	globalUpdatedAt map[string]time.Time      // uuid -> last global update time
}

// NewDeviceLimiter 创建一个空的设备数限制器。
func NewDeviceLimiter() *DeviceLimiter {
	return &DeviceLimiter{
		localDevices:    make(map[string]map[string]int),
		globalDevices:   make(map[string]int),
		globalUpdatedAt: make(map[string]time.Time),
	}
}

// OnConnect 连接建立时调用，返回是否允许该连接。
//
// 判定逻辑：
//   - deviceLimit <= 0 表示不限制设备数，直接放行。
//   - 若该 IP 已在本地集合中（同一设备重连/多连接），始终放行。
//   - 否则计算合并后的当前设备数，达到 deviceLimit 则拒绝。
//   - 放行后递增该 IP 的引用计数。
//
// 返回 true 表示允许连接，false 表示因超出设备数限制而拒绝。
func (dl *DeviceLimiter) OnConnect(uuid string, ip string, deviceLimit int) bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if deviceLimit > 0 {
		// 同一 IP 已连接：始终放行（重连/多连接复用同一设备配额）。
		if !dl.hasIPLocked(uuid, ip) {
			current := dl.countDevicesLocked(uuid)
			if current >= deviceLimit {
				return false
			}
		}
	}

	// 记录连接：递增引用计数。
	if dl.localDevices[uuid] == nil {
		dl.localDevices[uuid] = make(map[string]int)
	}
	dl.localDevices[uuid][ip]++
	return true
}

// OnDisconnect 连接断开时调用。
//
// 递减该 IP 的引用计数，归零时从本地集合移除该 IP；
// 用户无任何在线 IP 时清理其条目，避免 map 无限增长。
func (dl *DeviceLimiter) OnDisconnect(uuid string, ip string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	ips, ok := dl.localDevices[uuid]
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
		delete(dl.localDevices, uuid)
	}
}

// UpdateGlobalDevices 更新面板下发的跨节点设备态。
//
// devices 为 uuid -> 全局设备总数的全量快照。仅更新出现的用户，
// 不清除未出现的用户（面板可能仅推送有变化的用户）；如需全量替换请在调用前 ClearGlobalDevices。
// 每个被更新的用户刷新其 globalUpdatedAt 时间戳。
func (dl *DeviceLimiter) UpdateGlobalDevices(devices map[string]int) {
	if len(devices) == 0 {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	now := time.Now()
	for uuid, count := range devices {
		dl.globalDevices[uuid] = count
		dl.globalUpdatedAt[uuid] = now
	}
}

// ClearGlobalDevices 清除全局设备态（WS 断开时调用）。
//
// 清空后，后续设备数判定将退化为仅使用本地 IP 集合，
// 直到面板重新建立 WS 连接并下发新的全局态。
func (dl *DeviceLimiter) ClearGlobalDevices() {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.globalDevices = make(map[string]int)
	dl.globalUpdatedAt = make(map[string]time.Time)
}

// GetLocalDeviceCount 获取本地（本节点）去重后的设备数。
func (dl *DeviceLimiter) GetLocalDeviceCount(uuid string) int {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	return dl.getLocalDeviceCountLocked(uuid)
}

// GetMergedDeviceCount 获取合并后的设备数（本地与全局取较大值，全局过期则退化为本地）。
// 供观测/调试使用；OnConnect 内部使用相同的 countDevicesLocked 判定。
func (dl *DeviceLimiter) GetMergedDeviceCount(uuid string) int {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	return dl.countDevicesLocked(uuid)
}

// ClearLocalDevices 清空所有本地设备记录。
//
// 由 DeviceEnforcer 在每个轮询周期开始前调用，确保 DeviceLimiter 的本地状态
// 与 xray StatsService 的权威数据同步（移除已离线用户的陈旧记录）。
// 随后通过 SyncLocalDevices 逐个恢复在线用户的 IP 集合。
func (dl *DeviceLimiter) ClearLocalDevices() {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.localDevices = make(map[string]map[string]int)
}

// SyncLocalDevices 用权威数据替换指定用户的本地 IP 集合。
//
// 由 DeviceEnforcer 调用：从 xray StatsService 获取每用户在线 IP 列表后，
// 通过本方法同步到 DeviceLimiter，替代 OnConnect/OnDisconnect 的事件驱动模式。
// 这解决了 node-agent 无法拦截 xray 子进程连接事件的问题——
// OnConnect/OnDisconnect 在 xray 外部进程模式下不会被调用，
// 因此通过 StatsService 轮询 + SyncLocalDevices 提供权威的本地设备态。
func (dl *DeviceLimiter) SyncLocalDevices(uuid string, ips []string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if len(ips) == 0 {
		delete(dl.localDevices, uuid)
		return
	}

	newIPs := make(map[string]int, len(ips))
	for _, ip := range ips {
		newIPs[ip] = 1
	}
	dl.localDevices[uuid] = newIPs
}

// GetLocalDevicesSnapshot 返回当前本地所有在线设备的快照（uuid -> []ip）。
//
// 供 Agent 定期上报设备状态到面板使用。返回去重后的 IP 列表（不含引用计数）。
// 快照是深拷贝，调用方可安全使用而不持锁。
func (dl *DeviceLimiter) GetLocalDevicesSnapshot() map[string][]string {
	dl.mu.RLock()
	defer dl.mu.RUnlock()
	result := make(map[string][]string, len(dl.localDevices))
	for uuid, ips := range dl.localDevices {
		ipList := make([]string, 0, len(ips))
		for ip := range ips {
			ipList = append(ipList, ip)
		}
		result[uuid] = ipList
	}
	return result
}

// countDevicesLocked 计算合并后的当前设备数（调用方持锁）。
//
// 合并策略：
//   - localCount = 本节点去重 IP 数。
//   - 若该用户存在新鲜的全局态（距上次更新 <= globalStaleThreshold），
//     取 max(localCount, globalCount)；全局态包含本节点设备，取 max 避免重复计数又保守。
//   - 否则（全局态过期或缺失）退化为 localCount。
func (dl *DeviceLimiter) countDevicesLocked(uuid string) int {
	localCount := dl.getLocalDeviceCountLocked(uuid)

	if updated, ok := dl.globalUpdatedAt[uuid]; ok {
		if time.Since(updated) <= globalStaleThreshold {
			globalCount := dl.globalDevices[uuid]
			if globalCount > localCount {
				return globalCount
			}
			return localCount
		}
	}
	// 全局态过期或缺失：退化为本地判定。
	return localCount
}

// getLocalDeviceCountLocked 返回本节点去重 IP 数（调用方持锁）。
func (dl *DeviceLimiter) getLocalDeviceCountLocked(uuid string) int {
	return len(dl.localDevices[uuid])
}

// hasIPLocked 判断该 IP 是否已在本地集合中（调用方持锁）。
func (dl *DeviceLimiter) hasIPLocked(uuid string, ip string) bool {
	ips, ok := dl.localDevices[uuid]
	if !ok {
		return false
	}
	_, ok = ips[ip]
	return ok
}
