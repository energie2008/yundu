package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	agentconfig "github.com/airport-panel/node-agent/internal/config"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// stopLegacyXrayService 检测并停止独立的 yundu-xray.service / xray.service，
// 避免与 agent 内嵌 xray-core 端口冲突（443/9450 等）。
// 这是零SSH化的关键一环：agent 启动时自动接管 xray，无需手动 SSH 停止旧服务。
func stopLegacyXrayService(logger *slog.Logger) {
	services := []string{"yundu-xray.service", "xray.service"}
	for _, svc := range services {
		// 检测服务是否存在
		check := exec.Command("systemctl", "list-unit-files", svc)
		out, err := check.CombinedOutput()
		if err != nil || !strings.Contains(string(out), svc) {
			continue
		}
		// 服务存在，检查是否 active
		statusCmd := exec.Command("systemctl", "is-active", svc)
		statusOut, _ := statusCmd.Output()
		status := strings.TrimSpace(string(statusOut))
		if status == "active" {
			logger.Warn("detected legacy xray service, stopping to avoid port conflict",
				"service", svc, "status", status)
			// 停止服务
			if err := exec.Command("systemctl", "stop", svc).Run(); err != nil {
				logger.Error("failed to stop legacy xray service", "service", svc, "error", err)
			} else {
				logger.Info("legacy xray service stopped", "service", svc)
			}
		}
		// 禁用服务，防止重启后自动启动
		if err := exec.Command("systemctl", "disable", svc).Run(); err != nil {
			logger.Debug("failed to disable legacy xray service (may already be disabled)",
				"service", svc, "error", err)
		} else {
			logger.Info("legacy xray service disabled", "service", svc)
		}
	}

	// 兜底：清理非 systemd 管理的孤儿 xray 进程
	stopOrphanXray(logger)
}

// stopOrphanXray 清理非 systemd 管理的孤儿 xray 进程。
//
// 背景：stopLegacyXrayService 只能停止 systemd unit（yundu-xray.service / xray.service），
// 但有些 xray 进程可能是：
//   - 手动启动后变成孤儿（PPid=1）
//   - 旧版本 node-agent 子进程模式启动后未正确清理
//   - 测速脚本/xboard 遗留进程
//
// 这些孤儿进程会与 native 内嵌 xray 通过 SO_REUSEPORT 抢占 443/9450/10085 端口，
// 导致流量统计分散、DeviceEnforcer gRPC 查询到错误实例。
//
// 本函数用 pgrep 兜底，只清理加载了 yundu 主配置（/etc/yundu/config/xray.json）的孤儿进程，
// 不影响 xray-cn-transit 等其他独立 xray 进程，也不影响 node-agent 自己的子进程（子进程模式）。
//
// 安全性：所有模式下调用均安全——
//   - native 模式：xray 作为库运行在 node-agent 进程内，pgrep 不会匹配到 node-agent
//   - 子进程模式：xray 是 node-agent 的子进程，isChildProcess 检查会跳过
//   - machine 模式：子 Agent 用独立配置路径（/etc/yundu/nodes/node-N/config/xray.json），
//     pgrep 正则 config/xray\.json 不会匹配子节点配置
func stopOrphanXray(logger *slog.Logger) {
	// pgrep -f 匹配完整命令行，正则 "xray.*config/xray\.json" 只匹配主配置，
	// 不会匹配 xray-cn-transit.json 等其他配置
	out, err := exec.Command("pgrep", "-f", `xray.*config/xray\.json`).Output()
	if err != nil {
		// pgrep 退出码 1 表示无匹配进程，不是错误
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			logger.Debug("no orphan xray process found (pgrep no match)")
			return
		}
		// pgrep 命令本身不存在（非 Linux 或未安装 procps）
		logger.Debug("pgrep not available or failed, skipping orphan xray cleanup", "error", err)
		return
	}

	myPID := os.Getpid()
	for _, pidStr := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// 不要杀自己（native 模式下 xray 运行在 node-agent 进程内，pgrep 不会匹配到 node-agent，
		// 但防御性检查仍然保留）
		if pid == myPID {
			continue
		}

		// 不要杀 node-agent 的子进程（子进程模式下 xray 是 node-agent 的子进程，由 executor 管理）
		if isChildProcess(pid, myPID) {
			logger.Debug("skipping child xray process (managed by executor)", "pid", pid)
			continue
		}

		// 读取命令行用于日志
		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdlineStr := strings.ReplaceAll(string(cmdline), "\x00", " ")

		logger.Warn("detected orphan xray process, sending SIGTERM",
			"pid", pid, "cmdline", cmdlineStr)

		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			logger.Warn("failed to kill orphan xray process",
				"pid", pid, "error", err)
		} else {
			logger.Info("orphan xray process terminated",
				"pid", pid)
		}
	}
}

// isChildProcess 检查 pid 是否是 parentPID 的直接子进程。
// 通过读取 /proc/<pid>/status 中的 PPid 字段判断。
func isChildProcess(pid, parentPID int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ppid, err := strconv.Atoi(fields[1])
				if err == nil && ppid == parentPID {
					return true
				}
			}
			break
		}
	}
	return false
}

func (a *Agent) nextSeq() int64 {
	return a.seq.Add(1)
}

func resolveToken(cfg *agentconfig.Config, logger *slog.Logger) string {
	if cfg.AgentToken != "" {
		return cfg.AgentToken
	}
	if cfg.AgentAPITokenSalt != "" && cfg.ServerCode != "" {
		mac := hmac.New(sha256.New, []byte(cfg.AgentAPITokenSalt))
		mac.Write([]byte(cfg.ServerCode))
		token := hex.EncodeToString(mac.Sum(nil))
		logger.Info("generated agent token using HMAC-SHA256")
		return token
	}
	logger.Warn("no agent token available, authentication may fail")
	return ""
}

func resolveGRPCAddr(cfg *agentconfig.Config, logger *slog.Logger) string {
	if cfg.GRPCAddr != "" {
		return cfg.GRPCAddr
	}
	if cfg.PanelURL == "" {
		return fmt.Sprintf("localhost:%d", DefaultGRPCPort)
	}
	u, err := url.Parse(cfg.PanelURL)
	if err != nil {
		logger.Warn("failed to parse PanelURL for gRPC addr derivation", "error", err)
		return fmt.Sprintf("localhost:%d", DefaultGRPCPort)
	}
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s:%d", host, DefaultGRPCPort)
}

func resolveWSURL(cfg *agentconfig.Config, logger *slog.Logger) string {
	if cfg.PanelURL == "" {
		return "ws://localhost/ws"
	}
	u, err := url.Parse(cfg.PanelURL)
	if err != nil {
		logger.Warn("failed to parse PanelURL for WS addr derivation", "error", err)
		return "ws://localhost/ws"
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	host := u.Host
	if host == "" {
		host = "localhost"
	}
	// Preserve the path prefix (e.g. /agent-api) from PANEL_URL so that
	// reverse-proxied deployments route the WS handshake to node-service.
	base := strings.TrimRight(u.Path, "/")
	return fmt.Sprintf("%s://%s%s/api/v1/agent/ws", scheme, host, base)
}

func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func buildCapabilities(runtimeType string) map[string]interface{} {
	switch runtimeType {
	case "xray":
		return map[string]interface{}{
			"protocols":  []string{"vless", "trojan", "vmess", "shadowsocks"},
			"transports": []string{"tcp", "ws", "grpc", "httpupgrade", "splithttp", "xhttp", "h2"},
			"security":   []string{"tls", "reality", "none"},
			"reload":     true,
			"dry_run":    true,
		}
	case "sing-box":
		return map[string]interface{}{
			"protocols":  []string{"vless", "trojan", "vmess", "shadowsocks", "hysteria2", "tuic", "anytls", "naive"},
			"transports": []string{"tcp", "ws", "grpc", "httpupgrade", "splithttp", "quic"},
			"security":   []string{"tls", "reality", "none"},
			"reload":     true,
			"dry_run":    true,
		}
	default:
		return map[string]interface{}{
			"protocols":  []string{"vless", "trojan", "vmess"},
			"transports": []string{"tcp", "ws"},
			"security":   []string{"tls", "none"},
			"reload":     true,
			"dry_run":    true,
		}
	}
}

// extractProbeTags 从 inbound 配置中提取拨测需要的标签信息（security、method、sni 等）。
func extractProbeTags(ib map[string]interface{}) map[string]string {
	tags := make(map[string]string)

	// 提取 streamSettings.security（tls/reality/none）和 network（传输类型）
	if ss, ok := ib["streamSettings"].(map[string]interface{}); ok {
		if sec, ok := ss["security"].(string); ok && sec != "" {
			tags["security"] = sec
		}
		if network, ok := ss["network"].(string); ok && network != "" {
			tags["transport"] = network
		}
		// 提取 SNI：TLS inbound 从 tlsSettings.serverName，REALITY inbound 从 realitySettings.serverNames[0]
		// 拨测器必须使用正确的 SNI，否则 xray 会以 "tls: unrecognized name" 拒绝连接，
		// 导致 prober 误判为失败并触发 LKG 回滚循环。
		if ts, ok := ss["tlsSettings"].(map[string]interface{}); ok {
			if sn, ok := ts["serverName"].(string); ok && sn != "" {
				tags["sni"] = sn
			}
		}
		if rs, ok := ss["realitySettings"].(map[string]interface{}); ok {
			if sns, ok := rs["serverNames"].([]interface{}); ok && len(sns) > 0 {
				if sn, ok := sns[0].(string); ok && sn != "" {
					tags["sni"] = sn
				}
			}
		}
	}

	// 提取 settings.method（用于 SS 加密方法判断）
	if settings, ok := ib["settings"].(map[string]interface{}); ok {
		if method, ok := settings["method"].(string); ok && method != "" {
			tags["method"] = method
		}
	}

	return tags
}

// extractInboundTag 从 xray/sing-box 配置中提取第一个非 api inbound 的 tag。
func extractInboundTag(configMap map[string]interface{}) string {
	inbounds, ok := configMap["inbounds"].([]interface{})
	if !ok {
		return ""
	}
	for _, ib := range inbounds {
		if m, ok := ib.(map[string]interface{}); ok {
			tag, _ := m["tag"].(string)
			if tag == "" || tag == "api" {
				continue
			}
			return tag
		}
	}
	return ""
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readCurrentVersion(path string, logger *slog.Logger) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeCurrentVersion(path, version string, logger *slog.Logger) {
	if err := writeAtomic(path, []byte(version), 0644); err != nil {
		logger.Error("failed to write version file", "error", err, "path", path)
	}
}
