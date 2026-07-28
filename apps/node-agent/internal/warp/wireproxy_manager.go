package warp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WireProxyManager 管理 wireproxy 子进程：用 userspace WireGuard 替代 warp-cli 系统服务。
//
// 设计动机（借鉴 SUIWARP/wireproxy 项目）：
//   - warp-cli 系统服务占用 30-50MB，违反 ≤150MB 内存约束
//   - wireproxy 是纯 userspace WireGuard，仅 ~4MB RAM，无内核模块依赖
//   - 作为 sing-box 原生 wireguard outbound 的 fallback：当原生 wireguard 不可用
//     （private_key 缺失或 sing-box wireguard 模块 bug）时，wireproxy 提供 SOCKS5 接口
//
// 凭证来源：环境变量（node-agent.env 配置）
//   - WARP_PRIVATE_KEY（必填）：wgcf 注册的 WireGuard 私钥
//   - WARP_PUBLIC_KEY（可选，默认 CF WARP well-known 公钥）
//   - WARP_ENDPOINT（可选，默认 engage.cloudflareclient.com:2408）
//   - WARP_LOCAL_ADDRESS（可选，默认 172.16.0.2/32）
//   - WARP_MTU（可选，默认 1280）
//   - WIREPROXY_BIN（可选，默认 /opt/yundu/bin/wireproxy）
//   - WIREPROXY_CONF（可选，默认 /etc/yundu/wireproxy.conf）
//   - WIREPROXY_SOCKS_ADDR（可选，默认 127.0.0.1:40001）
//
// 与 WarpManager 的区别：
//   - WarpManager 调用 warp-cli 系统服务（需要 apt install + systemd）
//   - WireProxyManager 管理 wireproxy 子进程（独立二进制，Agent 直接 fork/exec）
//   - WireProxyManager 不需要 Install（wireproxy 二进制预装，避免运行时下载安全风险）
type WireProxyManager struct {
	runner   CommandRunner
	reporter PanelReporter
	logger   *slog.Logger

	binPath  string // wireproxy 二进制路径
	confPath string // wireproxy 配置文件路径
	socksAddr string // SOCKS5 监听地址
	traceURL string // 探测出口 IP 的端点

	// wireproxy 子进程句柄（Connect 时创建，Disconnect 时释放）
	cmd *exec.Cmd
}

// NewWireProxyManager 构造一个管理 wireproxy 子进程的 Manager。
func NewWireProxyManager(reporter PanelReporter, logger *slog.Logger) *WireProxyManager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &WireProxyManager{
		runner:    defaultRunner{},
		reporter:  reporter,
		logger:    logger,
		binPath:   getEnvDefault("WIREPROXY_BIN", "/opt/yundu/bin/wireproxy"),
		confPath:  getEnvDefault("WIREPROXY_CONF", "/etc/yundu/wireproxy.conf"),
		socksAddr: getEnvDefault("WIREPROXY_SOCKS_ADDR", "127.0.0.1:40001"),
		traceURL:  "https://1.1.1.1/cdn-cgi/trace",
	}
	return m
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// SetRunner 替换命令执行器（测试用）。
func (m *WireProxyManager) SetRunner(r CommandRunner) {
	m.runner = r
}

// DetectWarp 检测 wireproxy 二进制是否存在。
// 与 WarpManager 不同：wireproxy 是独立二进制，不是系统包，所以只检查文件可执行性。
func (m *WireProxyManager) DetectWarp() bool {
	info, err := os.Stat(m.binPath)
	if err != nil {
		return false
	}
	// 检查可执行权限
	return !info.IsDir() && info.Mode().Perm()&0111 != 0
}

// GetStatus 采集 wireproxy 综合状态。
// 通过检查子进程存活 + SOCKS5 端口探测 + cloudflare trace 判断。
func (m *WireProxyManager) GetStatus() *WarpStatus {
	status := &WarpStatus{Status: "not_installed", LastChecked: time.Now()}
	if !m.DetectWarp() {
		return status
	}

	// 检查子进程存活（cmd != nil 且 ProcessState 为 nil 表示仍在运行）
	if !m.isRunning() {
		// 子进程未启动或已退出，检查 SOCKS5 端口是否被其他 wireproxy 实例占用
		if !m.socksPortListening() {
			status.Status = "stopped"
			status.LastChecked = time.Now()
			return status
		}
		// 端口在监听但子进程不在管理范围内（可能是手动启动的），视为 running
	}

	status.Status = "running"
	ip, latency := m.probeWarpEndpoint()
	status.WarpIP = ip
	status.LatencyMs = latency
	status.LastChecked = time.Now()
	return status
}

// isRunning 检查 wireproxy 子进程是否仍在运行。
func (m *WireProxyManager) isRunning() bool {
	if m.cmd == nil || m.cmd.Process == nil {
		return false
	}
	// ProcessState 非 nil 表示进程已退出
	return m.cmd.ProcessState == nil
}

// socksPortListening 检查 SOCKS5 端口是否在监听。
func (m *WireProxyManager) socksPortListening() bool {
	conn, err := net.DialTimeout("tcp", m.socksAddr, 1*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// probeWarpEndpoint 通过 cloudflare trace 端点探测 WARP 出口 IP 与延迟。
// 经由 wireproxy SOCKS5 代理发起请求。
func (m *WireProxyManager) probeWarpEndpoint() (string, int) {
	start := time.Now()
	// 通过 SOCKS5 代理访问 trace 端点，验证 WARP 出口 IP
	stdout, _, code, err := m.runner.Run("curl", "-s", "--max-time", "5",
		"--socks5-hostname", m.socksAddr, m.traceURL)
	latency := int(time.Since(start).Milliseconds())
	if err != nil || code != 0 {
		return "", 0
	}
	return parseTraceIP(stdout), latency
}

// Install wireproxy 不支持自动安装（避免运行时下载二进制的安全风险）。
// 需通过运维脚本预装：脚本从 GitHub release 下载 wireproxy 到 m.binPath。
func (m *WireProxyManager) Install() error {
	return fmt.Errorf("wireproxy auto-install not supported; please pre-install wireproxy binary to %s (download from https://github.com/pufferffish/wireproxy/releases)", m.binPath)
}

// Connect 生成 wireproxy 配置并启动子进程。
// 凭证从环境变量读取，若 WARP_PRIVATE_KEY 缺失则返回错误。
func (m *WireProxyManager) Connect() error {
	if !m.DetectWarp() {
		return fmt.Errorf("wireproxy binary not found at %s: %w", m.binPath, ErrWarpNotInstalled)
	}

	privateKey := os.Getenv("WARP_PRIVATE_KEY")
	if privateKey == "" {
		return fmt.Errorf("WARP_PRIVATE_KEY environment variable not set; wireproxy requires wgcf credentials")
	}

	// 若子进程已在运行，跳过
	if m.isRunning() {
		m.logger.Debug("wireproxy already running, skip connect")
		return nil
	}

	// 生成 wireproxy 配置
	if err := m.writeConfig(privateKey); err != nil {
		return fmt.Errorf("write wireproxy config failed: %w", err)
	}

	// 启动 wireproxy 子进程（前台模式，由 Agent 管理生命周期）
	// 注意：不设置 Setpgid（Linux 专属，跨平台兼容），wireproxy 继承 Agent 进程组，
	// Agent 正常退出时通过 Disconnect() 清理；Agent 崩溃时 wireproxy 可能残留，
	// 下次启动 Connect() 前会通过 waitForSocksPort 检测端口冲突。
	cmd := exec.Command(m.binPath, "-c", m.confPath)
	cmd.Stdout = os.Stderr // wireproxy 日志输出到 stderr，Agent 通过 journalctl 统一采集
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start wireproxy failed: %w", err)
	}
	m.cmd = cmd
	m.logger.Info("wireproxy started",
		"pid", cmd.Process.Pid,
		"socks_addr", m.socksAddr,
		"bin", m.binPath)

	// 等待 SOCKS5 端口就绪（最多 3 秒）
	if !m.waitForSocksPort(3 * time.Second) {
		return fmt.Errorf("wireproxy started but SOCKS5 port %s not listening after 3s", m.socksAddr)
	}
	return nil
}

// writeConfig 生成 wireproxy 配置文件（INI 格式）。
// 支持双栈：当 WARP_LOCAL_ADDRESS 包含 IPv6 地址（含 ":"）时，
// AllowedIPs 自动设为 "0.0.0.0/0, ::/0"，否则仅 "0.0.0.0/0"（纯 IPv4）。
func (m *WireProxyManager) writeConfig(privateKey string) error {
	publicKey := getEnvDefault("WARP_PUBLIC_KEY", "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=")
	endpoint := getEnvDefault("WARP_ENDPOINT", "engage.cloudflareclient.com:2408")
	localAddr := getEnvDefault("WARP_LOCAL_ADDRESS", "172.16.0.2/32")
	mtu := getEnvDefault("WARP_MTU", "1280")

	// 双栈检测：localAddr 包含 ":" 表示有 IPv6 地址，AllowedIPs 需要包含 ::/0
	allowedIPs := "0.0.0.0/0"
	if strings.Contains(localAddr, ":") {
		allowedIPs = "0.0.0.0/0, ::/0"
	}

	conf := fmt.Sprintf(`# Auto-generated by node-agent (WireProxyManager)
# Do not edit manually — regenerated on each Connect()
[Interface]
PrivateKey = %s
Address = %s
MTU = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s

[Socks5]
BindAddress = %s
`, privateKey, localAddr, mtu, publicKey, endpoint, allowedIPs, m.socksAddr)

	return os.WriteFile(m.confPath, []byte(conf), 0600)
}

// waitForSocksPort 轮询等待 SOCKS5 端口就绪。
func (m *WireProxyManager) waitForSocksPort(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.socksPortListening() {
			return true
		}
		// 检查子进程是否意外退出
		if !m.isRunning() {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// Disconnect 终止 wireproxy 子进程。
func (m *WireProxyManager) Disconnect() error {
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	// 先尝试 SIGTERM 优雅退出
	if err := m.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		m.logger.Warn("send SIGTERM to wireproxy failed, trying SIGKILL", "error", err)
		_ = m.cmd.Process.Kill()
	}
	// 等待进程退出（最多 2 秒）
	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()
	select {
	case <-done:
		m.logger.Info("wireproxy stopped")
	case <-time.After(2 * time.Second):
		m.logger.Warn("wireproxy did not exit after SIGTERM, sending SIGKILL")
		_ = m.cmd.Process.Kill()
		<-done
	}
	m.cmd = nil
	return nil
}

// Check 综合 connect + ping 测试：启动后探测出口 IP 与延迟。
func (m *WireProxyManager) Check() (*WarpStatus, error) {
	if !m.DetectWarp() {
		return nil, ErrWarpNotInstalled
	}
	if err := m.Connect(); err != nil {
		return nil, err
	}
	status := m.GetStatus()
	if status.Status != "running" {
		return status, fmt.Errorf("wireproxy not running after connect: status=%s", status.Status)
	}
	return status, nil
}

// GetSocks5Outbound 渲染 WARP SOCKS5 outbound 配置段，用于注入 xray / sing-box 配置。
// runtimeType 取值为 "xray" 或 "sing-box"。
func (m *WireProxyManager) GetSocks5Outbound(runtimeType string) (string, error) {
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
		return "", fmt.Errorf("marshal wireproxy outbound: %w", err)
	}
	return string(b), nil
}

// ReportToPanel 通过注入的 PanelReporter 上报 warp_status / warp_ip / warp_latency_ms 到面板。
func (m *WireProxyManager) ReportToPanel(ctx context.Context, status *WarpStatus) error {
	if m.reporter == nil {
		return ErrPanelReporterNotConfigured
	}
	if status == nil {
		status = m.GetStatus()
	}
	return m.reporter.ReportWarpStatus(ctx, status)
}

// SocksAddr 返回 wireproxy SOCKS5 监听地址。
func (m *WireProxyManager) SocksAddr() string {
	return m.socksAddr
}

// 编译期断言：WireProxyManager 实现 Manager 接口。
var _ Manager = (*WireProxyManager)(nil)
