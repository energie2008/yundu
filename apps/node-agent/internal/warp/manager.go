package warp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CommandRunner 抽象命令执行（exec.Command），便于在测试中注入 mock。
type CommandRunner interface {
	Run(name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// defaultRunner 使用 os/exec 真实执行命令。
type defaultRunner struct{}

func (defaultRunner) Run(name string, args ...string) (string, string, int, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return stdout.String(), stderr.String(), ee.ExitCode(), nil
		}
		return stdout.String(), stderr.String(), -1, err
	}
	return stdout.String(), stderr.String(), 0, nil
}

// WarpStatus 表示 WARP 侧车的当前状态（上报到面板 runtimes.capabilities）。
type WarpStatus struct {
	Status      string    `json:"warp_status"`        // running / stopped / not_installed
	WarpIP      string    `json:"warp_ip"`            // WARP 出口 IP
	LatencyMs   int       `json:"warp_latency_ms"`    // WARP 链路延迟（毫秒）
	LastChecked time.Time `json:"warp_last_checked_at"`
}

// PanelReporter 上报 WARP 状态到面板。由调用方注入具体实现（可包装 client.Client）。
type PanelReporter interface {
	ReportWarpStatus(ctx context.Context, status *WarpStatus) error
}

var (
	ErrWarpNotInstalled          = errors.New("warp-cli not installed")
	ErrPanelReporterNotConfigured = errors.New("panel reporter not configured")
	ErrUnsupportedRuntimeType    = errors.New("unsupported runtime type for warp outbound")
)

// Manager 抽象 WARP 侧车管理能力，使本地开发可通过 WARP_MODE=mock 注入假实现，
// 而真实节点环境使用 WarpManager（调用 warp-cli）。
// 接口方法签名与现有 WarpManager 结构体的公共方法一致，不破坏已有测试。
type Manager interface {
	DetectWarp() bool
	GetStatus() *WarpStatus
	Install() error
	Connect() error
	Disconnect() error
	Check() (*WarpStatus, error)
	GetSocks5Outbound(runtimeType string) (string, error)
	ReportToPanel(ctx context.Context, status *WarpStatus) error
	SocksAddr() string
}

// WarpManager 管理 warp-cli 侧车：检测、安装、连接、状态采集与配置渲染。
// 实现 Manager 接口；在真实节点环境运行（依赖 warp-cli 二进制）。
type WarpManager struct {
	runner    CommandRunner
	reporter  PanelReporter
	logger    *slog.Logger
	warpBin   string // warp-cli 可执行文件名/路径
	socksAddr string // WARP SOCKS5 监听地址，如 127.0.0.1:40000
	traceURL  string // 用于探测 WARP 出口 IP 的 cloudflare trace 端点
}

// NewWarpManager 构造一个使用真实 exec 的 WarpManager。
func NewWarpManager(reporter PanelReporter, logger *slog.Logger) *WarpManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &WarpManager{
		runner:    defaultRunner{},
		reporter:  reporter,
		logger:    logger,
		warpBin:   "warp-cli",
		socksAddr: "127.0.0.1:40000",
		traceURL:  "https://1.1.1.1/cdn-cgi/trace",
	}
}

// SetRunner 替换命令执行器（测试用）。
func (m *WarpManager) SetRunner(r CommandRunner) {
	m.runner = r
}

// DetectWarp 检测系统是否已安装 warp-cli。
// 优先执行 `which warp-cli`，再尝试 `warp-cli --version`，最后检查 /usr/bin/warp-cli 是否存在。
func (m *WarpManager) DetectWarp() bool {
	if out, _, code, err := m.runner.Run("which", "warp-cli"); err == nil && code == 0 && strings.TrimSpace(out) != "" {
		return true
	}
	if _, _, code, err := m.runner.Run(m.warpBin, "--version"); err == nil && code == 0 {
		return true
	}
	if _, err := os.Stat("/usr/bin/warp-cli"); err == nil {
		return true
	}
	return false
}

// GetStatus 采集 WARP 综合状态：warp_status / warp_ip / warp_latency_ms。
func (m *WarpManager) GetStatus() *WarpStatus {
	status := &WarpStatus{Status: "not_installed", LastChecked: time.Now()}
	if !m.DetectWarp() {
		return status
	}

	stdout, stderr, code, err := m.runner.Run(m.warpBin, "status")
	if err != nil || code != 0 {
		m.logger.Warn("warp-cli status failed",
			"error", err, "stderr", stderr, "exit_code", code)
		status.Status = "stopped"
		status.LastChecked = time.Now()
		return status
	}

	parsed := parseWarpStatus(stdout)
	status.Status = parsed
	if parsed == "running" {
		ip, latency := m.probeWarpEndpoint()
		status.WarpIP = ip
		status.LatencyMs = latency
	}
	status.LastChecked = time.Now()
	return status
}

// parseWarpStatus 解析 `warp-cli status` 输出，归一化为 running / stopped。
// 注意 "disconnecting"/"disconnected" 包含 "connect" 子串，故需先判 dis- 前缀。
func parseWarpStatus(output string) string {
	lowered := strings.ToLower(output)
	if strings.Contains(lowered, "disconnect") {
		return "stopped"
	}
	if strings.Contains(lowered, "connect") {
		return "running"
	}
	return "stopped"
}

// probeWarpEndpoint 通过 cloudflare trace 端点探测 WARP 出口 IP 与延迟。
func (m *WarpManager) probeWarpEndpoint() (string, int) {
	start := time.Now()
	stdout, _, code, err := m.runner.Run("curl", "-s", "--max-time", "5", m.traceURL)
	latency := int(time.Since(start).Milliseconds())
	if err != nil || code != 0 {
		return "", 0
	}
	return parseTraceIP(stdout), latency
}

// parseTraceIP 从 cloudflare trace 响应中解析 ip= 字段。
func parseTraceIP(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimPrefix(line, "ip=")
		}
	}
	return ""
}

// Install 安装 warp-cli：优先 apt，失败则回退到 Cloudflare 官方安装脚本。
func (m *WarpManager) Install() error {
	if _, _, code, err := m.runner.Run("apt-get", "install", "-y", "cloudflare-warp"); err == nil && code == 0 {
		m.logger.Info("warp-cli installed via apt")
		return nil
	}
	m.logger.Info("apt install failed or unavailable, falling back to official script")
	stdout, stderr, code, err := m.runner.Run("bash", "-c",
		"curl -fsSL https://pkg.cloudflareclient.com/install | bash")
	if err != nil || code != 0 {
		return fmt.Errorf("install warp-cli via official script failed: code=%d stderr=%s stdout=%s err=%w",
			code, stderr, stdout, err)
	}
	m.logger.Info("warp-cli installed via official script")
	return nil
}

// Connect 执行 `warp-cli connect`。
func (m *WarpManager) Connect() error {
	_, stderr, code, err := m.runner.Run(m.warpBin, "connect")
	if err != nil || code != 0 {
		return fmt.Errorf("warp-cli connect failed: code=%d stderr=%s err=%w", code, stderr, err)
	}
	return nil
}

// Disconnect 执行 `warp-cli disconnect`。
func (m *WarpManager) Disconnect() error {
	_, stderr, code, err := m.runner.Run(m.warpBin, "disconnect")
	if err != nil || code != 0 {
		return fmt.Errorf("warp-cli disconnect failed: code=%d stderr=%s err=%w", code, stderr, err)
	}
	return nil
}

// Check 综合 connect + ping 测试：连接后探测出口 IP 与延迟。
func (m *WarpManager) Check() (*WarpStatus, error) {
	if !m.DetectWarp() {
		return nil, ErrWarpNotInstalled
	}
	if err := m.Connect(); err != nil {
		return nil, err
	}
	// 等待 WARP 隧道建立
	time.Sleep(2 * time.Second)
	status := m.GetStatus()
	if status.Status != "running" {
		return status, fmt.Errorf("warp not running after connect: status=%s", status.Status)
	}
	return status, nil
}

// GetSocks5Outbound 渲染 WARP SOCKS5 outbound 配置段，用于注入 xray / sing-box 配置。
// runtimeType 取值为 "xray" 或 "sing-box"。
func (m *WarpManager) GetSocks5Outbound(runtimeType string) (string, error) {
	host, portStr, err := net.SplitHostPort(m.socksAddr)
	if err != nil {
		return "", fmt.Errorf("invalid socks addr %q: %w", m.socksAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid socks port %q: %w", portStr, err)
	}

	var out interface{}
	switch runtimeType {
	case "xray":
		out = map[string]interface{}{
			"tag":      "warp-out",
			"protocol": "socks",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"address": host, "port": port},
				},
			},
		}
	case "sing-box":
		out = map[string]interface{}{
			"type":        "socks",
			"tag":         "warp-out",
			"server":      host,
			"server_port": port,
		}
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedRuntimeType, runtimeType)
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal warp outbound: %w", err)
	}
	return string(b), nil
}

// ReportToPanel 通过注入的 PanelReporter 上报 warp_status / warp_ip / warp_latency_ms 到面板。
func (m *WarpManager) ReportToPanel(ctx context.Context, status *WarpStatus) error {
	if m.reporter == nil {
		return ErrPanelReporterNotConfigured
	}
	if status == nil {
		status = m.GetStatus()
	}
	return m.reporter.ReportWarpStatus(ctx, status)
}

// SocksAddr 返回当前 WARP SOCKS5 监听地址。
func (m *WarpManager) SocksAddr() string {
	return m.socksAddr
}
