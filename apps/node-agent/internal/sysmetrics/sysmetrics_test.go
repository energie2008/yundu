package sysmetrics

import (
	"math"
	"testing"
	"time"
)

func TestParseCPUSamples(t *testing.T) {
	// 真实 /proc/stat 形态：cpu user nice system idle iowait irq softirq steal
	data := "cpu  100 20 30 4000 50 10 5 0\n" +
		"cpu0 50 10 15 2000 25 5 2 0\n" +
		"intr 123456\n"
	total, idle, ok := ParseCPUSamples(data)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// total = 100+20+30+4000+50+10+5+0 = 4215; idle = 4000+50 = 4050
	if total != 4215 {
		t.Fatalf("total = %d, want 4215", total)
	}
	if idle != 4050 {
		t.Fatalf("idle = %d, want 4050", idle)
	}
}

func TestParseCPUSamplesNoCpuLine(t *testing.T) {
	if _, _, ok := ParseCPUSamples("intr 1\n"); ok {
		t.Fatal("expected ok=false when no cpu line")
	}
}

func TestCPUPercentFromDelta(t *testing.T) {
	var s Sampler
	now := time.Now()

	// 首次采样无基线：ok=false，不产出伪造值
	if _, ok := s.CPUPercentFromDelta(4215, 4050, now); ok {
		t.Fatal("first sample should return ok=false")
	}

	// 第二次采样：total delta=1000，idle delta=900 → 10% 使用率
	pct, ok := s.CPUPercentFromDelta(5215, 4950, now.Add(10*time.Second))
	if !ok {
		t.Fatal("second sample should return ok=true")
	}
	if math.Abs(float64(pct)-10.0) > 0.01 {
		t.Fatalf("cpu percent = %f, want 10", pct)
	}

	// 计数器回绕（total 变小）：ok=false，基线被重建
	if _, ok := s.CPUPercentFromDelta(100, 90, now.Add(20*time.Second)); ok {
		t.Fatal("counter wrap should return ok=false")
	}
	// 重建后的下一次采样恢复正常
	if _, ok := s.CPUPercentFromDelta(200, 180, now.Add(30*time.Second)); !ok {
		t.Fatal("sample after rebuild should return ok=true")
	}
}

func TestCPUPercentBounds(t *testing.T) {
	var s Sampler
	now := time.Now()
	s.CPUPercentFromDelta(1000, 1000, now)
	// idle 不增（全忙）：使用率 100%
	pct, ok := s.CPUPercentFromDelta(2000, 1000, now.Add(time.Second))
	if !ok || pct != 100 {
		t.Fatalf("full busy: pct=%f ok=%v, want 100/true", pct, ok)
	}
}

func TestParseNetDevTotals(t *testing.T) {
	// 真实 /proc/net/dev 形态，含回环/虚拟网卡（应排除）与物理网卡
	data := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo: 999999 1000 0 0 0 0 0 0  999999 1000 0 0 0 0 0 0\n" +
		"  eth0: 1000000 2000 0 0 0 0 0 0  500000 1500 0 0 0 0 0 0\n" +
		"  eth1: 2000000 3000 0 0 0 0 0 0  800000 2500 0 0 0 0 0 0\n" +
		"veth123@if45: 777777 100 0 0 0 0 0 0  777777 100 0 0 0 0 0 0\n" +
		"docker0: 555555 100 0 0 0 0 0 0  555555 100 0 0 0 0 0 0\n"
	rx, tx, ok := ParseNetDevTotals(data)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if rx != 3000000 {
		t.Fatalf("rx = %d, want 3000000 (eth0+eth1 only)", rx)
	}
	if tx != 1300000 {
		t.Fatalf("tx = %d, want 1300000 (eth0+eth1 only)", tx)
	}
}

func TestParseNetDevEmpty(t *testing.T) {
	if _, _, ok := ParseNetDevTotals(""); ok {
		t.Fatal("empty input should return ok=false")
	}
}

func TestNetRatesFromDelta(t *testing.T) {
	var s Sampler
	now := time.Now()

	// 首次采样无基线：速率 0
	in, out := s.NetRatesFromDelta(0, 0, now)
	if in != 0 || out != 0 {
		t.Fatalf("first sample should be zero rates, got %f/%f", in, out)
	}

	// 10 秒收 1024000 字节、发 512000 字节 → 100 KB/s、50 KB/s
	in, out = s.NetRatesFromDelta(1024000, 512000, now.Add(10*time.Second))
	if math.Abs(float64(in)-100.0) > 0.01 {
		t.Fatalf("in = %f KB/s, want 100", in)
	}
	if math.Abs(float64(out)-50.0) > 0.01 {
		t.Fatalf("out = %f KB/s, want 50", out)
	}

	// 计数器回绕（网卡重置）：速率为 0 且基线重建
	in, out = s.NetRatesFromDelta(100, 100, now.Add(20*time.Second))
	if in != 0 || out != 0 {
		t.Fatalf("counter wrap should yield 0, got %f/%f", in, out)
	}
}

func TestCountEstablished(t *testing.T) {
	// 真实 /proc/net/tcp 形态：st=01 ESTABLISHED / st=0A LISTEN / st=06 TIME_WAIT
	data := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1234 1\n" +
		"   1: 8B0AA8C0:8AE2 6E17A8C0:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 5678 1\n" +
		"   2: 8B0AA8C0:8AE3 6E17A8C0:01BB 06 00000000:00000000 00:00000000 00000000  1000        0 9012 1\n" +
		"   3: 8B0AA8C0:8AE4 6E17A8C0:01BB 01 00000000:00000000 00:00000000 00000000  1000        0 3456 1\n"
	if got := CountEstablished(data); got != 2 {
		t.Fatalf("established = %d, want 2", got)
	}
}

func TestIsVirtualNic(t *testing.T) {
	virtual := []string{"lo", "docker0", "vethabc@if12", "br-123", "cni0", "podman1", "flannel.1", "virbr0", "tap0"}
	for _, n := range virtual {
		if !IsVirtualNic(n) {
			t.Fatalf("%s should be virtual", n)
		}
	}
	physical := []string{"eth0", "ens3", "enp0s3", "wlan0"}
	for _, n := range physical {
		if IsVirtualNic(n) {
			t.Fatalf("%s should be physical", n)
		}
	}
}
