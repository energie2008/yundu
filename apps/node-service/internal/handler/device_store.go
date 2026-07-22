package handler

import (
	"sync"
	"time"
)

// globalDeviceStore 全局设备状态存储（内存版）。
//
// D8 修复: Agent 通过 POST /api/v1/agent/devices/report 上报本节点在线设备 IP，
// 面板汇总后通过 GET /api/v1/agent/devices/alive 返回跨节点全局设备数。
//
// 使用内存存储 + TTL 过期机制，30 秒内未上报的节点数据自动清除。
// 未来可迁移到 Redis 实现多实例共享。
var globalDeviceStore = NewDeviceStore()

const deviceTTL = 30 * time.Second

// deviceEntry 单个节点的设备上报记录
type deviceEntry struct {
	devices   map[string][]string // uuid -> []ip
	updatedAt time.Time
}

// DeviceStore 线程安全的全局设备状态存储
type DeviceStore struct {
	mu      sync.RWMutex
	entries map[string]*deviceEntry // server_code -> entry
}

func NewDeviceStore() *DeviceStore {
	return &DeviceStore{
		entries: make(map[string]*deviceEntry),
	}
}

// Update 更新指定 server 的设备上报数据
func (ds *DeviceStore) Update(serverCode string, devices map[string][]string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.entries[serverCode] = &deviceEntry{
		devices:   devices,
		updatedAt: time.Now(),
	}
}

// GetGlobalDeviceCounts 汇总所有活跃节点的设备数，返回 uuid -> 全局在线设备总数
func (ds *DeviceStore) GetGlobalDeviceCounts() map[string]int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	now := time.Now()
	counts := make(map[string]int)
	for _, entry := range ds.entries {
		// 过期数据跳过
		if now.Sub(entry.updatedAt) > deviceTTL {
			continue
		}
		for uuid := range entry.devices {
			counts[uuid]++
		}
	}
	return counts
}
