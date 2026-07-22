package warp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockRunner 内存实现的命令执行器，按 "name args..." 全量匹配返回预设结果。
type mockRunner struct {
	results map[string]mockResult
}

type mockResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func newMockRunner() *mockRunner {
	return &mockRunner{results: map[string]mockResult{}}
}

func (mr *mockRunner) set(name string, args []string, r mockResult) {
	key := mockKey(name, args)
	mr.results[key] = r
}

func mockKey(name string, args []string) string {
	return name + " " + strings.Join(args, " ")
}

func (mr *mockRunner) Run(name string, args ...string) (string, string, int, error) {
	if r, ok := mr.results[mockKey(name, args)]; ok {
		return r.stdout, r.stderr, r.exitCode, r.err
	}
	return "", "", 1, nil
}

// fakeReporter 记录最后一次上报的状态。
type fakeReporter struct {
	lastStatus *WarpStatus
	calls      int
	err        error
}

func (f *fakeReporter) ReportWarpStatus(ctx context.Context, status *WarpStatus) error {
	f.calls++
	f.lastStatus = status
	return f.err
}

func newTestManager(runner CommandRunner, reporter PanelReporter) *WarpManager {
	m := NewWarpManager(reporter, nil)
	m.SetRunner(runner)
	return m
}

func TestParseWarpStatus_Connected(t *testing.T) {
	cases := map[string]string{
		"Status update: Connected":                       "running",
		"connecting":                                     "running",
		"Status update: Connecting":                      "running",
		"Status update: Disconnected":                    "stopped",
		"disconnected":                                  "stopped",
		"Disconnecting":                                  "stopped",
		"some unknown output":                            "stopped",
	}
	for input, want := range cases {
		if got := parseWarpStatus(input); got != want {
			t.Errorf("parseWarpStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDetectWarp_InstalledViaWhich(t *testing.T) {
	runner := newMockRunner()
	// which warp-cli 返回路径
	runner.set("which", []string{"warp-cli"}, mockResult{stdout: "/usr/bin/warp-cli\n", exitCode: 0})

	m := newTestManager(runner, nil)
	if !m.DetectWarp() {
		t.Errorf("DetectWarp() = false, want true (which found warp-cli)")
	}
}

func TestGetStatus_NotInstalled(t *testing.T) {
	runner := newMockRunner()
	// which 失败 + warp-cli --version 失败 → 视为未安装
	runner.set("which", []string{"warp-cli"}, mockResult{exitCode: 1})
	runner.set("warp-cli", []string{"--version"}, mockResult{exitCode: 1})

	m := newTestManager(runner, nil)
	status := m.GetStatus()
	if status.Status != "not_installed" {
		t.Errorf("Status = %q, want not_installed", status.Status)
	}
	if status.WarpIP != "" {
		t.Errorf("WarpIP = %q, want empty", status.WarpIP)
	}
	if status.LatencyMs != 0 {
		t.Errorf("LatencyMs = %d, want 0", status.LatencyMs)
	}
}

func TestGetStatus_RunningWithIPAndLatency(t *testing.T) {
	runner := newMockRunner()
	runner.set("which", []string{"warp-cli"}, mockResult{stdout: "/usr/bin/warp-cli\n", exitCode: 0})
	runner.set("warp-cli", []string{"status"}, mockResult{stdout: "Status update: Connected\n", exitCode: 0})
	runner.set("curl", []string{"-s", "--max-time", "5", "https://1.1.1.1/cdn-cgi/trace"},
		mockResult{stdout: "fl=123f\nh=1.1.1.1\nip=203.0.113.42\nts=1\nvisit_scheme=https\nuag=\n", exitCode: 0})

	m := newTestManager(runner, nil)
	status := m.GetStatus()
	if status.Status != "running" {
		t.Errorf("Status = %q, want running", status.Status)
	}
	if status.WarpIP != "203.0.113.42" {
		t.Errorf("WarpIP = %q, want 203.0.113.42", status.WarpIP)
	}
	if status.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", status.LatencyMs)
	}
	if status.LastChecked.IsZero() {
		t.Errorf("LastChecked not set")
	}
}

func TestParseTraceIP(t *testing.T) {
	input := "fl=abc\nh=1.1.1.1\nip=198.51.100.7\nts=1700000000\n"
	if got := parseTraceIP(input); got != "198.51.100.7" {
		t.Errorf("parseTraceIP() = %q, want 198.51.100.7", got)
	}
	if got := parseTraceIP("no ip here"); got != "" {
		t.Errorf("parseTraceIP() = %q, want empty", got)
	}
}

func TestGetSocks5Outbound_Xray(t *testing.T) {
	m := newTestManager(newMockRunner(), nil)
	out, err := m.GetSocks5Outbound("xray")
	if err != nil {
		t.Fatalf("GetSocks5Outbound(xray) error: %v", err)
	}
	if !strings.Contains(out, `"tag":"warp-out"`) {
		t.Errorf("xray outbound missing tag: %s", out)
	}
	if !strings.Contains(out, `"protocol":"socks"`) {
		t.Errorf("xray outbound missing protocol: %s", out)
	}
	if !strings.Contains(out, `"address":"127.0.0.1"`) {
		t.Errorf("xray outbound missing address: %s", out)
	}
	if !strings.Contains(out, `"port":40000`) {
		t.Errorf("xray outbound missing port: %s", out)
	}
}

func TestGetSocks5Outbound_SingBox(t *testing.T) {
	m := newTestManager(newMockRunner(), nil)
	out, err := m.GetSocks5Outbound("sing-box")
	if err != nil {
		t.Fatalf("GetSocks5Outbound(sing-box) error: %v", err)
	}
	if !strings.Contains(out, `"type":"socks"`) {
		t.Errorf("sing-box outbound missing type: %s", out)
	}
	if !strings.Contains(out, `"server":"127.0.0.1"`) {
		t.Errorf("sing-box outbound missing server: %s", out)
	}
	if !strings.Contains(out, `"server_port":40000`) {
		t.Errorf("sing-box outbound missing server_port: %s", out)
	}
}

func TestGetSocks5Outbound_UnsupportedRuntime(t *testing.T) {
	m := newTestManager(newMockRunner(), nil)
	_, err := m.GetSocks5Outbound("unknown")
	if !errors.Is(err, ErrUnsupportedRuntimeType) {
		t.Errorf("expected ErrUnsupportedRuntimeType, got %v", err)
	}
}

func TestReportToPanel(t *testing.T) {
	reporter := &fakeReporter{}
	m := newTestManager(newMockRunner(), reporter)

	status := &WarpStatus{Status: "running", WarpIP: "203.0.113.1", LatencyMs: 12}
	if err := m.ReportToPanel(context.Background(), status); err != nil {
		t.Fatalf("ReportToPanel error: %v", err)
	}
	if reporter.calls != 1 {
		t.Errorf("reporter calls = %d, want 1", reporter.calls)
	}
	if reporter.lastStatus.Status != "running" {
		t.Errorf("reported status = %q, want running", reporter.lastStatus.Status)
	}
	if reporter.lastStatus.WarpIP != "203.0.113.1" {
		t.Errorf("reported warp_ip = %q, want 203.0.113.1", reporter.lastStatus.WarpIP)
	}
}

func TestReportToPanel_NoReporter(t *testing.T) {
	m := newTestManager(newMockRunner(), nil)
	err := m.ReportToPanel(context.Background(), &WarpStatus{Status: "running"})
	if !errors.Is(err, ErrPanelReporterNotConfigured) {
		t.Errorf("expected ErrPanelReporterNotConfigured, got %v", err)
	}
}

func TestConnect_Disconnect(t *testing.T) {
	runner := newMockRunner()
	runner.set("warp-cli", []string{"connect"}, mockResult{exitCode: 0})
	runner.set("warp-cli", []string{"disconnect"}, mockResult{exitCode: 0})
	m := newTestManager(runner, nil)

	if err := m.Connect(); err != nil {
		t.Errorf("Connect() error: %v", err)
	}
	if err := m.Disconnect(); err != nil {
		t.Errorf("Disconnect() error: %v", err)
	}
}

func TestConnect_Failed(t *testing.T) {
	runner := newMockRunner()
	runner.set("warp-cli", []string{"connect"}, mockResult{exitCode: 2, stderr: "busy"})
	m := newTestManager(runner, nil)
	if err := m.Connect(); err == nil {
		t.Errorf("Connect() expected error, got nil")
	}
}
