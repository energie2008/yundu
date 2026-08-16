// Package sysmetrics 提供服务器系统指标的真实采集与差值计算。
//
// CPU 使用率与网络速率参考 Komari agent（github.com/komari-monitor/komari-agent）的算法：
//   - CPU：/proc/stat 聚合计数两次采样差值 → (Δtotal-Δidle)/Δtotal，同 gopsutil cpu.Percent 思路；
//   - 网络：/proc/net/dev 物理网卡累计字节差值 / 采样间隔 → KB/s，同 Komari NetworkSpeed 思路；
//   - 计数器回绕（网卡重置/系统重启）时安全归零并重建基线（同 Komari safeCounterDelta）。
//
// 纯标准库实现，解析函数与差值计算跨平台可测试；文件读取函数仅 Linux 有效。
package sysmetrics

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sampler 保持 /proc/stat 与 /proc/net/dev 的上次采样基线，
// 供差值算法计算真实 CPU 使用率与网络速率。心跳串行调用，锁仅作防御。
type Sampler struct {
	mu           sync.Mutex
	cpuTotal     uint64
	cpuIdle      uint64
	cpuSampledAt time.Time
	netRx        uint64
	netTx        uint64
	netSampledAt time.Time
}

// CPUPercentFromDelta 基于两次 /proc/stat 采样计算真实 CPU 使用率。
// 首次采样（无基线）或计数器回绕时返回 ok=false，由调用方沿用 loadavg 近似值。
func (s *Sampler) CPUPercentFromDelta(total, idle uint64, now time.Time) (float32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hasBaseline := !s.cpuSampledAt.IsZero() && total > s.cpuTotal && idle >= s.cpuIdle
	pct := float32(0)
	if hasBaseline {
		totalDelta := total - s.cpuTotal
		idleDelta := idle - s.cpuIdle
		if totalDelta > 0 {
			pct = float32(totalDelta-idleDelta) * 100 / float32(totalDelta)
			if pct < 0 {
				pct = 0
			} else if pct > 100 {
				pct = 100
			}
		} else {
			hasBaseline = false
		}
	}
	s.cpuTotal, s.cpuIdle, s.cpuSampledAt = total, idle, now
	return pct, hasBaseline
}

// NetRatesFromDelta 基于两次 /proc/net/dev 采样计算网络收发速率（KB/s）。
// 首次采样或计数器回绕（网卡重置/重载）时返回 0 并重建基线。
func (s *Sampler) NetRatesFromDelta(rx, tx uint64, now time.Time) (inKBps, outKBps float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.netSampledAt.IsZero() && rx >= s.netRx && tx >= s.netTx {
		if elapsed := now.Sub(s.netSampledAt).Seconds(); elapsed > 0 {
			inKBps = float32(float64(rx-s.netRx) / elapsed / 1024)
			outKBps = float32(float64(tx-s.netTx) / elapsed / 1024)
		}
	}
	s.netRx, s.netTx, s.netSampledAt = rx, tx, now
	return inKBps, outKBps
}

// ReadCPUSamples 读取 /proc/stat 首行 cpu 聚合计数，返回 total（全部 ticks）与
// idle（idle+iowait ticks）。USER_HZ 通常为 100，差值算法下无需换算单位。
func ReadCPUSamples() (total, idle uint64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	return ParseCPUSamples(string(data))
}

// ParseCPUSamples 解析 /proc/stat 内容的 cpu 聚合行。
func ParseCPUSamples(data string) (total, idle uint64, ok bool) {
	for _, line := range strings.Split(data, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// 格式: cpu user nice system idle iowait irq softirq steal ...
		for i := 1; i < len(fields); i++ {
			v, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += v
			if i == 4 || i == 5 { // idle、iowait
				idle += v
			}
		}
		return total, idle, true
	}
	return 0, 0, false
}

// virtualNicPrefixes 虚拟/回环网卡前缀（与 Komari loopbackNames 对齐），
// 汇总流量时排除，避免容器/veth 转发导致字节重复计数、速率虚高。
var virtualNicPrefixes = []string{
	"lo", "docker", "veth", "br", "cni", "podman", "flannel",
	"virbr", "vmbr", "tap", "fwbr", "fwpr",
}

// IsVirtualNic 判断网卡名是否为虚拟/回环接口。
func IsVirtualNic(name string) bool {
	for _, p := range virtualNicPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// ReadNetDevTotals 汇总 /proc/net/dev 中物理网卡的累计收发字节数。
func ReadNetDevTotals() (rx, tx uint64, ok bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	return ParseNetDevTotals(string(data))
}

// ParseNetDevTotals 解析 /proc/net/dev 内容，汇总物理网卡累计收发字节。
func ParseNetDevTotals(data string) (rx, tx uint64, ok bool) {
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue // 头部说明行/空行
		}
		name := strings.TrimSpace(parts[0])
		if name == "" || IsVirtualNic(name) {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		r, err1 := strconv.ParseUint(fields[0], 10, 64) // rx bytes
		t, err2 := strconv.ParseUint(fields[8], 10, 64) // tx bytes
		if err1 != nil || err2 != nil {
			continue
		}
		rx += r
		tx += t
		ok = true
	}
	return rx, tx, ok
}

// ReadTCPEstablished 统计 /proc/net/tcp 与 /proc/net/tcp6 中 ESTABLISHED（st=01）连接数。
func ReadTCPEstablished() int64 {
	var count int64
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		count += CountEstablished(string(data))
	}
	return count
}

// CountEstablished 统计单个 /proc/net/tcp(6) 内容中 ESTABLISHED（st=01）的行数。
func CountEstablished(data string) int64 {
	var count int64
	for i, line := range strings.Split(data, "\n") {
		if i == 0 { // 表头
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[3] == "01" {
			count++
		}
	}
	return count
}
