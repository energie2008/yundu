package runtime

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TrafficBuffer 流量上报容错缓冲区（借鉴 Xboard-Node 的 RestoreTraffic 机制）。
//
// 设计目标：
//   - 上报失败时保留未上报流量，下次合并重试，避免数据丢失
//   - Agent 重启时从持久化文件恢复 pending 流量
//   - 线程安全，支持并发 Add/Pending/Clear
//
// 数据流：
//   每 10s: ConnTracker.Snapshot() → buffer.Add(stats)  // 暂存增量
//   每 60s: merged = buffer.Pending()                   // 合并 pending + 当前
//           if report(merged) succeeds:
//               buffer.Clear()                          // 清空 pending
//           else:
//               保留 pending，下次 60s 合并重试          // 数据不丢失
//   Agent 重启: buffer.Load() → 恢复 pending             // 持久化恢复
type TrafficBuffer struct {
	mu       sync.Mutex
	pending  map[string]TrafficStat // 上报失败时暂存的流量
	filePath string                 // 持久化文件路径
}

// NewTrafficBuffer 创建流量缓冲区，filePath 为持久化文件路径（空则不持久化）。
func NewTrafficBuffer(filePath string) *TrafficBuffer {
	tb := &TrafficBuffer{
		pending:  make(map[string]TrafficStat),
		filePath: filePath,
	}
	tb.Load()
	return tb
}

// Add 将流量统计累加到 pending 缓冲区。
// 同一用户的流量会累加（Upload += , Download +=）。
func (t *TrafficBuffer) Add(stats map[string]TrafficStat) {
	if len(stats) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, stat := range stats {
		existing := t.pending[key]
		existing.Email = stat.Email
		existing.UUID = stat.UUID
		existing.Upload += stat.Upload
		existing.Download += stat.Download
		t.pending[key] = existing
	}
}

// Pending 返回当前 pending 的合并流量（不清空）。
// 调用方应在上报成功后调用 Clear() 清空缓冲区。
func (t *TrafficBuffer) Pending() map[string]TrafficStat {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make(map[string]TrafficStat, len(t.pending))
	for k, v := range t.pending {
		result[k] = v
	}
	return result
}

// Clear 清空 pending 缓冲区（上报成功后调用）。
// 先删除持久化文件，再清空内存，避免"清空内存后删文件失败"导致下次 Load 恢复出幽灵数据。
func (t *TrafficBuffer) Clear() {
	// 先删除持久化文件
	if t.filePath != "" {
		_ = os.Remove(t.filePath)
	}
	// 再清空内存
	t.mu.Lock()
	t.pending = make(map[string]TrafficStat)
	t.mu.Unlock()
}

// Load 从持久化文件恢复 pending 流量（Agent 重启时调用）。
// 文件不存在视为正常情况（上次上报成功后已 Clear）；读取/解析失败告警便于运维排查。
func (t *TrafficBuffer) Load() {
	if t.filePath == "" {
		return
	}
	data, err := os.ReadFile(t.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("traffic buffer: failed to read persist file",
				"path", t.filePath, "error", err)
		}
		return
	}
	var pending map[string]TrafficStat
	if err := json.Unmarshal(data, &pending); err != nil {
		slog.Warn("traffic buffer: failed to parse persist file, pending data may be lost",
			"path", t.filePath, "error", err)
		return
	}
	t.mu.Lock()
	t.pending = pending
	t.mu.Unlock()
	slog.Info("traffic buffer: restored pending from persist file",
		"path", t.filePath, "user_count", len(pending))
}

// Save 将 pending 流量持久化到文件。
// 上报失败时调用，确保 Agent 重启后能恢复未上报流量。
func (t *TrafficBuffer) Save() {
	if t.filePath == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		_ = os.Remove(t.filePath)
		return
	}
	data, err := json.Marshal(t.pending)
	if err != nil {
		return
	}
	// 原子写入：先写临时文件，再 rename
	tmpPath := t.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmpPath, t.filePath)
}

// SaveDebounced 防抖持久化：避免频繁写盘。
// 使用单独的 goroutine 延迟 1 秒执行 Save，期间若有新调用则取消旧定时器。
type SaveDebouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	buffer   *TrafficBuffer
	interval time.Duration
}

// NewSaveDebouncer 创建防抖持久化器。
func NewSaveDebouncer(tb *TrafficBuffer, interval time.Duration) *SaveDebouncer {
	return &SaveDebouncer{
		buffer:   tb,
		interval: interval,
	}
}

// Trigger 触发防抖持久化（非阻塞）。
func (d *SaveDebouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.interval, func() {
		d.buffer.Save()
	})
}

// Stop 停止防抖定时器。
func (d *SaveDebouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

// ResolveBufferPath 解析 traffic_buffer.json 的路径。
// 默认放在 configDir 下，与 version.txt 同目录。
func ResolveBufferPath(configDir string) string {
	return filepath.Join(filepath.Dir(configDir), "traffic_buffer.json")
}
