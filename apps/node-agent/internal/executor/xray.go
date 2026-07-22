package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/airport-panel/node-agent/internal/limiter"
)

type XrayExecutor struct {
	binPath       string
	configDir     string
	logger        *slog.Logger
	cmd           *exec.Cmd
	pid           int
	startedAt     time.Time
	configPath    string
	stopping      atomic.Bool
	restartCount  atomic.Int64
	processExited chan struct{}
	// 限速器集成：SpeedLimiter（令牌桶限速）+ DeviceLimiter（设备数限制）
	limiter *LimiterIntegration
}

func NewXrayExecutor(binPath, configDir string, logger *slog.Logger) *XrayExecutor {
	return &XrayExecutor{
		binPath:   binPath,
		configDir: configDir,
		logger:    logger.With("runtime", "xray"),
		limiter:   NewLimiterIntegration(logger),
	}
}

func (e *XrayExecutor) Validate(configContent string) error {
	if strings.TrimSpace(configContent) == "" {
		return fmt.Errorf("empty config")
	}
	return nil
}

func (e *XrayExecutor) Apply(configPath string, content string) error {
	e.logger.Info("applying xray config", "path", configPath)

	// 从配置内容中解析 _limiter 元数据并更新限速器
	e.limiter.ParseLimiterConfig(content)

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.logger.Error("failed to create config directory", "error", err)
		return err
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		e.logger.Error("failed to write temp config", "error", err)
		return err
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		e.logger.Error("failed to rename config", "error", err)
		return err
	}

	e.configPath = configPath
	e.logger.Info("xray config written successfully")
	return nil
}

func (e *XrayExecutor) DryRun(ctx context.Context, configPath string) error {
	e.logger.Info("dry-running xray config", "path", configPath)

	binPath := e.binPath
	if binPath == "" {
		binPath = "xray"
	}

	cmd := exec.CommandContext(ctx, binPath, "run", "-test", "-format=json", "-config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.logger.Error("xray dry-run failed", "error", err, "output", string(output))
		return fmt.Errorf("xray dry-run failed: %w, output: %s", err, string(output))
	}

	e.logger.Debug("xray dry-run succeeded", "output", string(output))
	return nil
}

func (e *XrayExecutor) Reload(ctx context.Context, configPath string) error {
	// 更新限速器配置：从配置文件中解析 _limiter 元数据
	if data, err := os.ReadFile(configPath); err == nil {
		e.limiter.ParseLimiterConfig(string(data))
	}

	if e.cmd != nil && e.cmd.Process != nil {
		if sig := sigUSR1(); sig != nil {
			e.logger.Info("sending SIGUSR1 to xray for graceful reload", "pid", e.cmd.Process.Pid)
			if err := e.cmd.Process.Signal(sig); err == nil {
				time.Sleep(500 * time.Millisecond)
				return nil
			} else {
				e.logger.Warn("SIGUSR1 failed, falling back to restart", "error", err)
			}
		}
	}

	return e.start(ctx, configPath)
}

func (e *XrayExecutor) start(ctx context.Context, configPath string) error {
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.Stop(ctx)
	}

	binPath := e.binPath
	if binPath == "" {
		binPath = "xray"
	}

	e.cmd = exec.CommandContext(ctx, binPath, "run", "-config", configPath)
	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr

	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start xray: %w", err)
	}

	e.pid = e.cmd.Process.Pid
	e.startedAt = time.Now()
	e.configPath = configPath
	e.stopping.Store(false)
	e.processExited = make(chan struct{})
	go e.watchdog(ctx, configPath)
	e.logger.Info("xray started", "pid", e.pid, "config", configPath)
	return nil
}

// watchdog monitors the xray process and restarts it on crash with
// exponential backoff (1s, 2s, 4s, 8s, 16s, 30s cap). If the process
// ran stably for >60s before crashing, the restart counter resets.
func (e *XrayExecutor) watchdog(ctx context.Context, configPath string) {
	for {
		if e.cmd == nil || e.cmd.Process == nil {
			// start() failed on a previous restart attempt — retry
			e.restartCount.Add(1)
		} else {
			err := e.cmd.Wait()
			close(e.processExited)

			if e.stopping.Load() {
				e.logger.Info("xray stopped intentionally",
					"pid", e.pid, "exit_error", err)
				return
			}

			// Crash detected — adjust restart count
			uptime := time.Since(e.startedAt)
			if uptime > 60*time.Second {
				e.restartCount.Store(0)
			}
			e.restartCount.Add(1)

			e.logger.Error("xray process crashed, scheduling restart",
				"pid", e.pid, "exit_error", err,
				"uptime", uptime.String(),
				"restart_count", e.restartCount.Load())
		}

		count := e.restartCount.Load()
		backoff := backoffDuration(count)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		if err := e.start(ctx, configPath); err != nil {
			e.logger.Error("xray restart failed, will retry",
				"error", err, "restart_count", count)
			continue // loop back and retry after another backoff
		}
		return // start() succeeded and spawned a new watchdog goroutine
	}
}

func (e *XrayExecutor) Stop(ctx context.Context) error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}

	e.stopping.Store(true)
	e.logger.Info("stopping xray", "pid", e.cmd.Process.Pid)
	if err := e.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		e.cmd.Process.Kill()
	}

	// Wait for the watchdog goroutine to detect the exit and close processExited.
	// This avoids calling cmd.Wait() twice (once in Stop, once in watchdog).
	select {
	case <-e.processExited:
	case <-time.After(10 * time.Second):
		e.cmd.Process.Kill()
		<-e.processExited
	}

	e.cmd = nil
	e.pid = 0
	return nil
}

func (e *XrayExecutor) Status(ctx context.Context) (*RuntimeStatus, error) {
	defaultVersion := e.probeVersion(ctx)
	status := &RuntimeStatus{
		Version: defaultVersion,
	}

	if e.cmd == nil || e.cmd.Process == nil {
		status.Running = false
		return status, nil
	}

	proc, err := os.FindProcess(e.cmd.Process.Pid)
	if err != nil {
		status.Running = false
		return status, nil
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		status.Running = false
		e.cmd = nil
		e.pid = 0
		return status, nil
	}

	status.Running = true
	status.PID = e.cmd.Process.Pid
	status.StartedAt = e.startedAt
	status.Uptime = int64(time.Since(e.startedAt).Seconds())
	status.RestartCount = e.restartCount.Load()

	if e.configPath != "" {
		if data, err := os.ReadFile(e.configPath); err == nil {
			hash := sha256.Sum256(data)
			status.ConfigHash = hex.EncodeToString(hash[:])
		}
	}

	if v := e.probeVersion(ctx); v != "" {
		status.Version = v
	}

	return status, nil
}

func (e *XrayExecutor) probeVersion(ctx context.Context) string {
	binPath := e.binPath
	if binPath == "" {
		binPath = "xray"
	}
	if out, err := exec.CommandContext(ctx, binPath, "version").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			v := strings.TrimSpace(lines[0])
			if v != "" {
				// 提取简短版本号（如 "Xray 26.3.27 (...)" → "xray 26.3.27"）
				// 避免完整输出超过数据库 VARCHAR(64) 限制
				return extractShortVersion(v)
			}
		}
	}
	return "xray"
}

// extractShortVersion 从 "Xray 26.3.27 (Xray, Penetrates Everything.) ..." 中提取 "xray 26.3.27"
// 避免完整版本字符串超过数据库字段长度限制
func extractShortVersion(fullVersion string) string {
	parts := strings.SplitN(fullVersion, " ", 3)
	if len(parts) >= 2 {
		return strings.ToLower(parts[0]) + " " + parts[1]
	}
	return fullVersion
}

func (e *XrayExecutor) Rollback() error {
	e.logger.Info("rollback xray config")

	backupPath := e.configPath + ".bak"
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Rename(backupPath, e.configPath); err != nil {
			return err
		}
	}
	return nil
}

// AlterInbound P1-5: 对运行中的 xray 执行增量用户变更（不重启进程，PID 不变）。
//
// 实现策略（渐进式）：
//  1. 若 users 为空，直接返回 nil
//  2. 回退到 SIGUSR1 graceful reload（xray 接收信号后重新加载整个 config.json，PID 不变）
//     - Linux: SIGUSR1 可用，PID 不变 ✓
//     - Windows: SIGUSR1 不可用，会走 start() 全量重启（PID 变化）
//
// 未来增强（P2+）：接入 xray gRPC HandlerService（127.0.0.1:10085），
// 调用 AddUserOperation/RemoveUserOperation 实现真增量更新（无全量重载，无断连）。
// 当前 node-service 已在生成的 xray 配置中启用 api inbound（tag=api, port=10085），
// gRPC endpoint 已就绪，只需在 node-agent 侧补 gRPC client 即可。
//
// 参数 users: 用户变更列表（added/removed/modified）
// 返回: 成功返回 nil；SIGUSR1 失败返回 error（调用方可决定是否走全量 restart）
func (e *XrayExecutor) AlterInbound(ctx context.Context, users []AlterUser) error {
	if len(users) == 0 {
		return nil
	}

	added, removed, modified := 0, 0, 0
	for _, u := range users {
		switch u.Op {
		case AlterUserAdded:
			added++
		case AlterUserRemoved:
			removed++
		case AlterUserModified:
			modified++
		}
	}
	e.logger.Info("alter inbound (hot user reload)",
		"total", len(users), "added", added, "removed", removed, "modified", modified)

	// 当前实现：走 SIGUSR1 graceful reload（PID 不变）
	// 配置文件已由 applyConfig 写入新内容，SIGUSR1 让 xray 重新加载
	if e.configPath == "" {
		return fmt.Errorf("alter inbound: config path not set")
	}
	return e.Reload(ctx, e.configPath)
}

func findProcessPID(name string) (int, error) {
	if runtime.GOOS == "windows" {
		return 0, fmt.Errorf("findProcessPID not supported on windows")
	}
	out, err := exec.Command("pgrep", "-x", name).Output()
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(out))
	lines := strings.Split(pidStr, "\n")
	if len(lines) > 0 {
		pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err == nil {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("process not found")
}

// ===== 限速器访问方法（实现 LimiterUpdater + DeviceLimiterProvider + IPLimiterProvider 接口）=====

// UpdateLimitersFromMeta 从已提取的 _limiter 元数据更新限速器（实现 LimiterUpdater 接口）。
// 供 main.go 在剥离 _limiter 字段后调用。
func (e *XrayExecutor) UpdateLimitersFromMeta(meta interface{}) {
	e.limiter.UpdateLimitersFromMeta(meta)
}

// SpeedLimiter 返回底层 SpeedLimiter 实例（实现 DeviceLimiterProvider 接口）。
func (e *XrayExecutor) SpeedLimiter() *limiter.SpeedLimiter {
	return e.limiter.SpeedLimiter()
}

// DeviceLimiter 返回底层 DeviceLimiter 实例（实现 DeviceLimiterProvider 接口）。
func (e *XrayExecutor) DeviceLimiter() *limiter.DeviceLimiter {
	return e.limiter.DeviceLimiter()
}

// GetDeviceLimit 返回指定用户的设备数上限（实现 DeviceLimiterProvider 接口）。
func (e *XrayExecutor) GetDeviceLimit(uuid string) int {
	return e.limiter.GetDeviceLimit(uuid)
}

// IPLimiter 返回底层 IPLimiter 实例（实现 IPLimiterProvider 接口）。
func (e *XrayExecutor) IPLimiter() *limiter.IPLimiter {
	return e.limiter.IPLimiter()
}

// GetIPLimit 返回指定用户的 IP 数上限（实现 IPLimiterProvider 接口）。
func (e *XrayExecutor) GetIPLimit(uuid string) int {
	return e.limiter.GetIPLimit(uuid)
}
