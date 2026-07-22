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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/airport-panel/node-agent/internal/limiter"
)

type SingBoxExecutor struct {
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
	// B19 修复：蓝绿热转管理器，避免配置更新时断流
	blueGreen *SingBoxBlueGreen
	// useBlueGreen 控制是否使用蓝绿热转（首次启动后启用）
	useBlueGreen bool
}

func NewSingBoxExecutor(binPath, configDir string, logger *slog.Logger) *SingBoxExecutor {
	bg := NewSingBoxBlueGreen(binPath, configDir, logger)
	return &SingBoxExecutor{
		binPath:      binPath,
		configDir:    configDir,
		logger:       logger.With("runtime", "sing-box"),
		limiter:      NewLimiterIntegration(logger),
		blueGreen:    bg,
		useBlueGreen: true, // 默认启用蓝绿热转，避免配置更新时断流
	}
}

func (e *SingBoxExecutor) Validate(configContent string) error {
	if strings.TrimSpace(configContent) == "" {
		return fmt.Errorf("empty config")
	}
	return nil
}

func (e *SingBoxExecutor) Apply(configPath string, content string) error {
	e.logger.Info("applying sing-box config", "path", configPath)

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
	e.logger.Info("sing-box config written successfully")
	return nil
}

func (e *SingBoxExecutor) DryRun(ctx context.Context, configPath string) error {
	e.logger.Info("dry-running sing-box config", "path", configPath)

	binPath := e.binPath
	if binPath == "" {
		binPath = "sing-box"
	}

	cmd := exec.CommandContext(ctx, binPath, "check", "-c", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.logger.Error("sing-box dry-run failed", "error", err, "output", string(output))
		return fmt.Errorf("sing-box check failed: %w, output: %s", err, string(output))
	}

	e.logger.Debug("sing-box dry-run succeeded", "output", string(output))
	return nil
}

func (e *SingBoxExecutor) Reload(ctx context.Context, configPath string) error {
	// 更新限速器配置：从配置文件中解析 _limiter 元数据
	if data, err := os.ReadFile(configPath); err == nil {
		e.limiter.ParseLimiterConfig(string(data))
	}

	// B19 修复：默认使用蓝绿热转，避免配置更新时断流
	if e.useBlueGreen && e.blueGreen != nil {
		e.logger.Info("reloading sing-box via blue-green swap", "old_config", e.configPath)
		if err := e.blueGreen.Swap(ctx, configPath); err != nil {
			e.logger.Error("blue-green swap failed, falling back to full restart", "error", err)
			// 蓝绿失败时降级为全量重启：先清理 blueGreen 状态，再走 start
			_ = e.blueGreen.Stop()
			if e.cmd != nil && e.cmd.Process != nil {
				e.logger.Info("restarting sing-box (fallback)", "pid", e.cmd.Process.Pid)
			}
			if err := e.start(ctx, configPath); err != nil {
				return err
			}
			// 降级启动成功后，重新启用蓝绿模式供下次 Reload 使用
			e.useBlueGreen = true
			return nil
		}
		e.configPath = configPath
		e.logger.Info("sing-box blue-green swap succeeded")
		return nil
	}

	// 蓝绿不可用时走普通启动
	if e.cmd != nil && e.cmd.Process != nil {
		e.logger.Info("restarting sing-box", "pid", e.cmd.Process.Pid)
	}

	if err := e.start(ctx, configPath); err != nil {
		return err
	}
	return nil
}

func (e *SingBoxExecutor) start(ctx context.Context, configPath string) error {
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.Stop(ctx)
	}

	binPath := e.binPath
	if binPath == "" {
		binPath = "sing-box"
	}

	e.cmd = exec.CommandContext(ctx, binPath, "run", "-c", configPath)
	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr

	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	e.pid = e.cmd.Process.Pid
	e.startedAt = time.Now()
	e.configPath = configPath
	e.stopping.Store(false)
	e.processExited = make(chan struct{})
	go e.watchdog(ctx, configPath)
	e.logger.Info("sing-box started", "pid", e.pid, "config", configPath)
	return nil
}

// watchdog monitors the sing-box process and restarts it on crash with
// exponential backoff (1s, 2s, 4s, 8s, 16s, 30s cap). If the process
// ran stably for >60s before crashing, the restart counter resets.
func (e *SingBoxExecutor) watchdog(ctx context.Context, configPath string) {
	for {
		if e.cmd == nil || e.cmd.Process == nil {
			// start() failed on a previous restart attempt — retry
			e.restartCount.Add(1)
		} else {
			err := e.cmd.Wait()
			close(e.processExited)

			if e.stopping.Load() {
				e.logger.Info("sing-box stopped intentionally",
					"pid", e.pid, "exit_error", err)
				return
			}

			// Crash detected — adjust restart count
			uptime := time.Since(e.startedAt)
			if uptime > 60*time.Second {
				e.restartCount.Store(0)
			}
			e.restartCount.Add(1)

			e.logger.Error("sing-box process crashed, scheduling restart",
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
			e.logger.Error("sing-box restart failed, will retry",
				"error", err, "restart_count", count)
			continue
		}
		return
	}
}

func (e *SingBoxExecutor) Stop(ctx context.Context) error {
	// 先停止 blueGreen 管理的进程（如果有）
	if e.blueGreen != nil {
		if err := e.blueGreen.Stop(); err != nil {
			e.logger.Warn("blue-green stop returned error", "error", err)
		}
	}

	// 再停止 e.cmd 管理的进程（降级模式下使用）
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}

	e.stopping.Store(true)
	e.logger.Info("stopping sing-box", "pid", e.cmd.Process.Pid)
	if err := e.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		e.cmd.Process.Kill()
	}

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

func (e *SingBoxExecutor) Status(ctx context.Context) (*RuntimeStatus, error) {
	defaultVersion := e.probeVersion(ctx)
	status := &RuntimeStatus{
		Version: defaultVersion,
	}

	// 优先检查蓝绿模式下的当前实例
	if e.useBlueGreen && e.blueGreen != nil {
		if pid, startedAt, ok := e.blueGreen.CurrentInstanceInfo(); ok {
			status.Running = true
			status.PID = pid
			status.StartedAt = startedAt
			status.Uptime = int64(time.Since(startedAt).Seconds())
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
		// 蓝绿模式但无运行实例，返回 not running
		status.Running = false
		return status, nil
	}

	// 降级模式：检查 e.cmd
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

func (e *SingBoxExecutor) probeVersion(ctx context.Context) string {
	binPath := e.binPath
	if binPath == "" {
		binPath = "sing-box"
	}
	if out, err := exec.CommandContext(ctx, binPath, "version").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			v := strings.TrimSpace(lines[0])
			if v != "" {
				// 提取简短版本号，避免完整输出超过数据库 VARCHAR(64) 限制
				// sing-box 输出格式: "sing-box version 1.10.0" → "sing-box 1.10.0"
				parts := strings.Fields(v)
				if len(parts) >= 3 && parts[1] == "version" {
					return parts[0] + " " + parts[2]
				}
				if len(parts) >= 2 {
					return parts[0] + " " + parts[1]
				}
				return v
			}
		}
	}
	return "sing-box"
}

func (e *SingBoxExecutor) Rollback() error {
	e.logger.Info("rollback sing-box config")

	backupPath := e.configPath + ".bak"
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Rename(backupPath, e.configPath); err != nil {
			return err
		}
	}
	return nil
}

// AlterInbound P1-5: sing-box 无等价 gRPC API，回退到 Reload（全量重启）。
// sing-box 的 Reload 本身就是 stop+start，PID 会变化。
// 未来 sing-box 若暴露 Clash API 的增量端点，可在此接入。
func (e *SingBoxExecutor) AlterInbound(ctx context.Context, users []AlterUser) error {
	if len(users) == 0 {
		return nil
	}
	e.logger.Info("alter inbound: sing-box fallback to reload (no gRPC API)", "users", len(users))
	if e.configPath == "" {
		return fmt.Errorf("alter inbound: config path not set")
	}
	return e.Reload(ctx, e.configPath)
}

// ===== 限速器访问方法（实现 LimiterUpdater + DeviceLimiterProvider + IPLimiterProvider 接口）=====

// UpdateLimitersFromMeta 从已提取的 _limiter 元数据更新限速器（实现 LimiterUpdater 接口）。
// 供 main.go 在剥离 _limiter 字段后调用。
func (e *SingBoxExecutor) UpdateLimitersFromMeta(meta interface{}) {
	e.limiter.UpdateLimitersFromMeta(meta)
}

// SpeedLimiter 返回底层 SpeedLimiter 实例（实现 DeviceLimiterProvider 接口）。
func (e *SingBoxExecutor) SpeedLimiter() *limiter.SpeedLimiter {
	return e.limiter.SpeedLimiter()
}

// DeviceLimiter 返回底层 DeviceLimiter 实例（实现 DeviceLimiterProvider 接口）。
func (e *SingBoxExecutor) DeviceLimiter() *limiter.DeviceLimiter {
	return e.limiter.DeviceLimiter()
}

// GetDeviceLimit 返回指定用户的设备数上限（实现 DeviceLimiterProvider 接口）。
func (e *SingBoxExecutor) GetDeviceLimit(uuid string) int {
	return e.limiter.GetDeviceLimit(uuid)
}

// IPLimiter 返回底层 IPLimiter 实例（实现 IPLimiterProvider 接口）。
func (e *SingBoxExecutor) IPLimiter() *limiter.IPLimiter {
	return e.limiter.IPLimiter()
}

// GetIPLimit 返回指定用户的 IP 数上限（实现 IPLimiterProvider 接口）。
func (e *SingBoxExecutor) GetIPLimit(uuid string) int {
	return e.limiter.GetIPLimit(uuid)
}
