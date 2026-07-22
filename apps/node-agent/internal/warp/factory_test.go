package warp

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestNewManager_MockMode(t *testing.T) {
	t.Setenv("WARP_MODE", "mock")
	m := NewManager(nil, nil)
	if _, ok := m.(*MockManager); !ok {
		t.Errorf("WARP_MODE=mock: got %T, want *MockManager", m)
	}
}

func TestNewManager_RealMode(t *testing.T) {
	t.Setenv("WARP_MODE", "real")
	m := NewManager(nil, nil)
	if _, ok := m.(*WarpManager); !ok {
		t.Errorf("WARP_MODE=real: got %T, want *WarpManager", m)
	}
}

func TestNewManager_DefaultIsReal(t *testing.T) {
	t.Setenv("WARP_MODE", "")
	m := NewManager(nil, nil)
	if _, ok := m.(*WarpManager); !ok {
		t.Errorf("WARP_MODE empty: got %T, want *WarpManager (default)", m)
	}
}

func TestNewManager_UnknownFallsBackToReal(t *testing.T) {
	t.Setenv("WARP_MODE", "bogus")
	m := NewManager(nil, nil)
	if _, ok := m.(*WarpManager); !ok {
		t.Errorf("WARP_MODE=bogus: got %T, want *WarpManager (fallback)", m)
	}
}

func TestMockManager_Status(t *testing.T) {
	m := NewMockManager()
	s := m.GetStatus()
	if s.Status != "running" {
		t.Errorf("status = %q, want running", s.Status)
	}
	if s.WarpIP != "104.28.1.1" {
		t.Errorf("warp_ip = %q, want 104.28.1.1", s.WarpIP)
	}
	if s.LatencyMs != 22 {
		t.Errorf("latency = %d, want 22", s.LatencyMs)
	}
}

func TestMockManager_FakeStatusOverride(t *testing.T) {
	m := NewMockManager()
	m.FakeStatus = &WarpStatus{Status: "stopped"}
	if s := m.GetStatus(); s.Status != "stopped" {
		t.Errorf("override status = %q, want stopped", s.Status)
	}
}

func TestMockManager_Detect(t *testing.T) {
	m := NewMockManager()
	if !m.DetectWarp() {
		t.Errorf("DetectWarp() = false, want true (mock always installed)")
	}
}

func TestMockManager_Socks5Outbound_Xray(t *testing.T) {
	m := NewMockManager()
	out, err := m.GetSocks5Outbound("xray")
	if err != nil {
		t.Fatalf("GetSocks5Outbound(xray): %v", err)
	}
	if !strings.Contains(out, `"tag":"warp-out"`) || !strings.Contains(out, `"port":40000`) {
		t.Errorf("xray outbound missing fields: %s", out)
	}
}

func TestMockManager_Socks5Outbound_SingBox(t *testing.T) {
	m := NewMockManager()
	out, err := m.GetSocks5Outbound("sing-box")
	if err != nil {
		t.Fatalf("GetSocks5Outbound(sing-box): %v", err)
	}
	if !strings.Contains(out, `"server_port":40000`) {
		t.Errorf("sing-box outbound missing server_port: %s", out)
	}
}

func TestMockManager_ErrorInjection(t *testing.T) {
	m := NewMockManager()
	m.ConnectErr = context.Canceled
	m.InstallErr = context.DeadlineExceeded
	if err := m.Connect(); err != context.Canceled {
		t.Errorf("Connect err = %v, want context.Canceled", err)
	}
	if err := m.Install(); err != context.DeadlineExceeded {
		t.Errorf("Install err = %v, want context.DeadlineExceeded", err)
	}
}

func TestMockManager_SocksAddr(t *testing.T) {
	m := NewMockManager()
	m.Socks5Addr = "127.0.0.1:40001"
	if got := m.SocksAddr(); got != "127.0.0.1:40001" {
		t.Errorf("SocksAddr = %q, want 127.0.0.1:40001", got)
	}
}

func TestNewManager_NoEnvLeakFromOtherTests(t *testing.T) {
	// 确保未设置 WARP_MODE 时（os.Getenv 返回 ""）走 real 分支
	os.Unsetenv("WARP_MODE")
	m := NewManager(nil, nil)
	if _, ok := m.(*WarpManager); !ok {
		t.Errorf("unset WARP_MODE: got %T, want *WarpManager", m)
	}
}
