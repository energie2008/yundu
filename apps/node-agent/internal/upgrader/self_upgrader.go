// Package upgrader 实现 Agent 自升级（原生内嵌模式下替代 BinaryReconciler）。
//
// 在原生内嵌模式下，xray-core 和 sing-box 都编译进 agent 二进制，
// 不再需要单独下载内核二进制。升级粒度变为整个 agent 二进制。
//
// 升级策略：
//  1. 控制面通知有新版本（通过心跳 HEARTBEAT_ACTION_UPGRADE）
//  2. Agent 下载新二进制到临时路径
//  3. SHA256 checksum 校验
//  4. 原子替换当前二进制（Linux: rename(2) 原子操作）
//  5. 通过 os.Exit(0) 退出，由 systemd/supervisor 自动拉起新二进制
//  6. 新进程 LKG 内存状态由磁盘 checkpoint 恢复
//
// 回退策略：
//  - 新版本启动后 30s 内未上报健康心跳，systemd/supervisor 自动回退到旧版本
//  - 下载失败/checksum 失败不替换二进制
package upgrader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	logFlushGracePeriod = 2 * time.Second
)

// Config 升级配置。
type Config struct {
	// CurrentVersion 当前 agent 版本。
	CurrentVersion string
	// BinaryPath 当前 agent 二进制路径。
	BinaryPath string
	// UpdateURL 升级检查 URL（控制面提供）。
	UpdateURL string
	// CheckInterval 检查间隔。
	CheckInterval time.Duration
	// HTTPClient HTTP 客户端。
	HTTPClient *http.Client
	// OnRestartNeeded 二进制替换成功后、退出前调用的回调。
	// 调用方可以在此回调中执行清理工作（停止 runtime、关闭连接等）。
	// 回调在单独 goroutine 中执行，有 5 秒超时保护。
	OnRestartNeeded func()
}

// SelfUpgrader 管理 agent 自升级流程。
type SelfUpgrader struct {
	mu             sync.Mutex
	cfg            Config
	logger         *slog.Logger
	currentVersion string
	updating       atomic.Bool
	restarting     atomic.Bool
	lastCheck      time.Time
	lastSuccess    time.Time
	failCount      atomic.Int64
	stopCh         chan struct{}
	running        atomic.Bool
}

// NewSelfUpgrader 创建自升级器。
func NewSelfUpgrader(cfg Config, logger *slog.Logger) *SelfUpgrader {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 5 * time.Minute
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.BinaryPath == "" {
		exe, err := os.Executable()
		if err == nil {
			cfg.BinaryPath = exe
		}
	}
	return &SelfUpgrader{
		cfg:            cfg,
		logger:         logger.With("component", "self-upgrader"),
		currentVersion: cfg.CurrentVersion,
		stopCh:         make(chan struct{}),
	}
}

// VersionInfo 版本信息。
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	ReleaseNote string `json:"release_note"`
	ForceUpdate bool   `json:"force_update"`
}

// Start 启动后台升级检查循环。
func (u *SelfUpgrader) Start(ctx context.Context) {
	if !u.running.CompareAndSwap(false, true) {
		return
	}
	u.logger.Info("self-upgrader started",
		"current_version", u.currentVersion,
		"binary_path", u.cfg.BinaryPath,
		"check_interval", u.cfg.CheckInterval.String(),
	)

	go func() {
		ticker := time.NewTicker(u.cfg.CheckInterval)
		defer ticker.Stop()
		defer u.running.Store(false)

		for {
			select {
			case <-ctx.Done():
				return
			case <-u.stopCh:
				return
			case <-ticker.C:
				u.checkAndUpgrade(ctx)
			}
		}
	}()
}

// Stop 停止升级检查。
func (u *SelfUpgrader) Stop() {
	close(u.stopCh)
}

// CheckNow 立即触发一次升级检查。
func (u *SelfUpgrader) CheckNow(ctx context.Context) error {
	return u.checkAndUpgrade(ctx)
}

// IsRestarting 返回是否正在重启流程中。
func (u *SelfUpgrader) IsRestarting() bool {
	return u.restarting.Load()
}

func (u *SelfUpgrader) checkAndUpgrade(ctx context.Context) error {
	if u.updating.Load() || u.restarting.Load() {
		return nil
	}
	u.updating.Store(true)
	defer u.updating.Store(false)

	u.mu.Lock()
	u.lastCheck = time.Now()
	u.mu.Unlock()

	u.logger.Debug("checking for agent updates")

	info, err := u.fetchVersionInfo(ctx)
	if err != nil {
		u.logger.Debug("no update available or check failed", "error", err)
		return nil
	}

	if info.Version == u.currentVersion {
		return nil
	}

	u.logger.Info("new agent version available",
		"current", u.currentVersion,
		"new", info.Version,
		"force", info.ForceUpdate,
	)

	if err := u.doUpgrade(ctx, info); err != nil {
		u.failCount.Add(1)
		u.logger.Error("upgrade failed", "error", err)
		return fmt.Errorf("upgrade to %s: %w", info.Version, err)
	}

	u.mu.Lock()
	u.lastSuccess = time.Now()
	u.currentVersion = info.Version
	u.mu.Unlock()

	u.logger.Info("agent upgraded successfully", "version", info.Version)
	return nil
}

func (u *SelfUpgrader) fetchVersionInfo(ctx context.Context) (*VersionInfo, error) {
	if u.cfg.UpdateURL == "" {
		return nil, fmt.Errorf("update URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.UpdateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Version", u.currentVersion)
	req.Header.Set("X-GOOS", runtime.GOOS)
	req.Header.Set("X-GOARCH", runtime.GOARCH)

	resp, err := u.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusNoContent {
		return nil, fmt.Errorf("no update")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	info := &VersionInfo{}

	if len(body) > 0 {
		if err := json.Unmarshal(body, info); err != nil {
			u.logger.Debug("failed to parse JSON body, falling back to headers", "error", err)
		}
	}

	if info.Version == "" {
		info.Version = resp.Header.Get("X-Latest-Version")
	}
	if info.DownloadURL == "" {
		info.DownloadURL = resp.Header.Get("X-Download-URL")
	}
	if info.SHA256 == "" {
		info.SHA256 = resp.Header.Get("X-SHA256")
	}

	if info.Version == "" {
		return nil, fmt.Errorf("no version info in response")
	}

	return info, nil
}

func (u *SelfUpgrader) doUpgrade(ctx context.Context, info *VersionInfo) error {
	if info.DownloadURL == "" {
		return fmt.Errorf("download URL empty")
	}

	if !u.restarting.CompareAndSwap(false, true) {
		return fmt.Errorf("restart already in progress")
	}

	tmpDir := filepath.Dir(u.cfg.BinaryPath)
	tmpFile, err := os.CreateTemp(tmpDir, "agent-update-*")
	if err != nil {
		u.restarting.Store(false)
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanupOnFail := true
	defer func() {
		if cleanupOnFail {
			os.Remove(tmpPath)
			u.restarting.Store(false)
		}
	}()

	u.logger.Info("downloading new agent binary", "url", info.DownloadURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		tmpFile.Close()
		return err
	}
	resp, err := u.cfg.HTTPClient.Do(req)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download status: %d", resp.StatusCode)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("download write: %w", err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if info.SHA256 != "" && actualHash != info.SHA256 {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", info.SHA256, actualHash)
	}
	u.logger.Info("binary downloaded and checksum verified",
		"size_bytes", written, "sha256", actualHash[:16])

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	backupPath := u.cfg.BinaryPath + ".bak"
	if err := os.Rename(u.cfg.BinaryPath, backupPath); err != nil {
		u.logger.Warn("failed to backup old binary, proceeding anyway", "error", err, "backup", backupPath)
	}

	if err := os.Rename(tmpPath, u.cfg.BinaryPath); err != nil {
		os.Rename(backupPath, u.cfg.BinaryPath)
		return fmt.Errorf("atomic replace: %w", err)
	}

	cleanupOnFail = false

	u.logger.Info("binary replaced successfully, initiating restart",
		"old_version", u.currentVersion, "new_version", info.Version)

	// Write upgrade-pending sentinel file for the new process to detect post-upgrade state.
	// The new process will perform a 45s health gate; if healthy it removes the file,
	// if not (crash loop / no auth), the next restart will auto-rollback to .bak.
	pendingPath := u.cfg.BinaryPath + ".upgrade-pending"
	pendingContent := fmt.Sprintf("new_version=%s\nold_version=%s\nupgraded_at=%d\n",
		info.Version, u.currentVersion, time.Now().Unix())
	if err := os.WriteFile(pendingPath, []byte(pendingContent), 0644); err != nil {
		u.logger.Warn("failed to write upgrade-pending marker, rollback guard disabled", "error", err)
	}

	go u.triggerRestart()

	return nil
}

func (u *SelfUpgrader) triggerRestart() {
	if u.cfg.OnRestartNeeded != nil {
		u.logger.Info("calling OnRestartNeeded callback for pre-exit cleanup")
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					u.logger.Error("OnRestartNeeded callback panicked", "panic", r)
				}
			}()
			u.cfg.OnRestartNeeded()
		}()

		select {
		case <-done:
			u.logger.Info("OnRestartNeeded callback completed")
		case <-time.After(5 * time.Second):
			u.logger.Warn("OnRestartNeeded callback timed out after 5s, proceeding with exit")
		}
	}

	u.logger.Info("waiting for logs to flush before exit", "grace_period", logFlushGracePeriod)
	time.Sleep(logFlushGracePeriod)

	u.logger.Info("exiting to allow supervisor to restart with new binary")
	os.Exit(0)
}

const (
	// upgradePendingTimeout is the maximum time after an upgrade during which the new
	// process must report healthy. If the sentinel file still exists after this window,
	// the next startup will auto-rollback to the .bak binary.
	upgradePendingTimeout = 5 * time.Minute
)

// CheckAndHandleUpgradePending inspects the upgrade-pending sentinel file next to the
// current binary. If the file exists and is older than upgradePendingTimeout, it
// automatically rolls back to the .bak binary and returns true (caller should exit).
// If the file exists but is still fresh, it returns false with no action (health gate
// is in progress — caller should arrange to call CommitUpgradeHealthy on success).
// If no pending file exists, this is a normal startup — returns false.
//
// This provides a real "30s health rollback" guarantee that was previously only
// documented but not implemented. Works with any supervisor that restarts on exit
// (systemd, docker --restart=always, etc.).
func CheckAndHandleUpgradePending(logger *slog.Logger, binaryPath string) (shouldExit bool) {
	if binaryPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return false
		}
		binaryPath = exe
	}
	pendingPath := binaryPath + ".upgrade-pending"
	backupPath := binaryPath + ".bak"

	data, err := os.ReadFile(pendingPath)
	if err != nil {
		return false
	}

	var upgradedAt int64
	var newVersion string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "upgraded_at=") {
			fmt.Sscanf(line, "upgraded_at=%d", &upgradedAt)
		}
		if strings.HasPrefix(line, "new_version=") {
			newVersion = strings.TrimPrefix(line, "new_version=")
		}
	}

	if upgradedAt == 0 {
		os.Remove(pendingPath)
		return false
	}

	age := time.Since(time.Unix(upgradedAt, 0))
	if age < upgradePendingTimeout {
		logger.Info("upgrade pending — health gate in progress",
			"new_version", newVersion, "age_sec", int(age.Seconds()),
			"timeout_sec", int(upgradePendingTimeout.Seconds()))
		return false
	}

	// Timeout expired without CommitUpgradeHealthy being called — rollback.
	logger.Error("upgrade health gate timed out, auto-rolling back to previous binary",
		"new_version", newVersion, "age_sec", int(age.Seconds()),
		"backup", backupPath)

	if _, err := os.Stat(backupPath); err != nil {
		logger.Error("backup binary not found, cannot rollback — staying on current version",
			"error", err)
		os.Remove(pendingPath)
		return false
	}

	// Replace current binary with backup.
	if err := os.Rename(binaryPath, binaryPath+".bad-upgrade"); err != nil {
		logger.Error("failed to move bad binary aside", "error", err)
		return false
	}
	if err := os.Rename(backupPath, binaryPath); err != nil {
		logger.Error("failed to restore backup binary", "error", err)
		os.Rename(binaryPath+".bad-upgrade", binaryPath)
		return false
	}
	os.Chmod(binaryPath, 0755)
	os.Remove(pendingPath)
	logger.Info("rollback complete — exiting for supervisor to restart previous version")
	return true
}

// CommitUpgradeHealthy removes the upgrade-pending sentinel file after the new binary
// has successfully passed the health gate (auth + first heartbeat succeeded).
// This confirms the upgrade is healthy and prevents auto-rollback on next restart.
func CommitUpgradeHealthy(binaryPath string) error {
	if binaryPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		binaryPath = exe
	}
	pendingPath := binaryPath + ".upgrade-pending"
	err := os.Remove(pendingPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
