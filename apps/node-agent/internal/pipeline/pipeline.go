package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/airport-panel/node-agent/internal/validator"
)

// PipelineResult 是部署流水线执行结果
type PipelineResult struct {
	Success         bool   `json:"success"`
	Version         string `json:"version"`
	Phase           string `json:"phase"` // precheck / dryrun / activate / healthcheck / rollback
	Error           string `json:"error,omitempty"`
	ApplyDurationMs int64  `json:"apply_duration_ms"`
	RolledBack      bool   `json:"rolled_back"`
}

// DeployCallbacks encapsulates agent-specific deployment callbacks for the unified pipeline (D11).
// Pipeline.Run uses these to interact with the runtime, keeping Pipeline and Agent logic decoupled.
type DeployCallbacks struct {
	DryRun      func(ctx context.Context, configPath string) error
	Apply       func(ctx context.Context, configPath string) error
	HealthCheck func(ctx context.Context, configJSON []byte) error
	OnRollback  func(ctx context.Context, restoredConfigPath string) error
}

// Pipeline 是部署流水线，负责 LKG 自动回滚状态机与 deploy.lock 管理。
//
// 事务模型：备份 → 预检 → 激活 → 测活 → 成功或回滚
//   - deploy.lock 在部署开始时写入，结束时删除；Agent 崩溃后可通过 CheckStaleLock 恢复。
//   - LKG (Last Known Good) 在激活新配置前备份当前已知良好配置，
//     健康检查失败时从 LKG 回滚，保证节点始终可用。
type Pipeline struct {
	validator *validator.EdgeValidator
	logger    *slog.Logger
	configDir string
}

// NewPipeline 创建新的部署流水线
func NewPipeline(v *validator.EdgeValidator, configDir string, logger *slog.Logger) *Pipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		validator: v,
		logger:    logger,
		configDir: configDir,
	}
}

// DeployLockPath 返回 deploy.lock 文件路径
func (p *Pipeline) DeployLockPath() string {
	return filepath.Join(p.configDir, "config", "deploy.lock")
}

// LKGConfigPath 返回 LKG (Last Known Good) 配置文件路径
func (p *Pipeline) LKGConfigPath(runtimeType string) string {
	switch runtimeType {
	case "xray":
		return filepath.Join(p.configDir, "config", "xray.lkg.json")
	case "sing-box":
		return filepath.Join(p.configDir, "config", "sing-box.lkg.json")
	default:
		return filepath.Join(p.configDir, "config", "config.lkg.json")
	}
}

// WriteDeployLock 写入部署锁文件
func (p *Pipeline) WriteDeployLock(version string) error {
	lockPath := p.DeployLockPath()
	lockContent := map[string]interface{}{
		"version":   version,
		"timestamp": time.Now().Unix(),
		"ttl":       300, // 5 分钟 TTL
		"pid":       os.Getpid(),
	}
	data, err := json.Marshal(lockContent)
	if err != nil {
		return err
	}
	return writeAtomic(lockPath, data, 0644)
}

// RemoveDeployLock 删除部署锁文件
func (p *Pipeline) RemoveDeployLock() {
	lockPath := p.DeployLockPath()
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		p.logger.Warn("failed to remove deploy.lock", "error", err)
	}
}

// CheckStaleLock 检查是否有残留的部署锁（Agent 崩溃恢复时调用）
// 返回 (lockExists, lockVersion, error)
func (p *Pipeline) CheckStaleLock() (bool, string, error) {
	lockPath := p.DeployLockPath()
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}

	var lock map[string]interface{}
	if err := json.Unmarshal(data, &lock); err != nil {
		// 锁文件损坏，删除它
		os.Remove(lockPath)
		return false, "", nil
	}

	timestamp, _ := lock["timestamp"].(float64)
	ttl, _ := lock["ttl"].(float64)
	version, _ := lock["version"].(string)

	// 检查是否过期
	lockTime := time.Unix(int64(timestamp), 0)
	if time.Since(lockTime) > time.Duration(int64(ttl))*time.Second {
		p.logger.Warn("stale deploy.lock found (expired), removing", "version", version, "age", time.Since(lockTime))
		os.Remove(lockPath)
		return false, "", nil
	}

	p.logger.Warn("stale deploy.lock found (active)", "version", version)
	return true, version, nil
}

// BackupLKG 将当前配置备份为 LKG
func (p *Pipeline) BackupLKG(currentConfigPath, runtimeType string) error {
	lkgPath := p.LKGConfigPath(runtimeType)

	// 如果当前配置文件存在，复制为 LKG
	if _, err := os.Stat(currentConfigPath); err != nil {
		// 当前配置不存在，跳过备份
		return nil
	}

	data, err := os.ReadFile(currentConfigPath)
	if err != nil {
		return fmt.Errorf("read current config for LKG backup: %w", err)
	}

	return writeAtomic(lkgPath, data, 0644)
}

// RestoreLKG 从 LKG 恢复配置
func (p *Pipeline) RestoreLKG(configPath, runtimeType string) error {
	lkgPath := p.LKGConfigPath(runtimeType)

	data, err := os.ReadFile(lkgPath)
	if err != nil {
		return fmt.Errorf("read LKG config: %w", err)
	}

	return writeAtomic(configPath, data, 0644)
}

// HasLKG 检查 LKG 配置是否存在
func (p *Pipeline) HasLKG(runtimeType string) bool {
	lkgPath := p.LKGConfigPath(runtimeType)
	_, err := os.Stat(lkgPath)
	return err == nil
}

// UpdateLKG copies the successfully applied config as LKG.
func (p *Pipeline) UpdateLKG(configPath, runtimeType string) error {
	lkgPath := p.LKGConfigPath(runtimeType)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config for LKG update: %w", err)
	}
	return writeAtomic(lkgPath, data, 0644)
}

// Run executes the full deployment transaction: lock -> precheck -> backup -> write -> dry-run -> apply -> health-check -> success/rollback.
//
// This is the single deployment entry point (D11 unified pipeline). applyConfig in main.go
// calls this method for the core transaction, eliminating the dual-path problem.
//
// Transaction guarantees:
//   - deploy.lock written at start, defer-cleaned (recoverable via CheckStaleLock after crash)
//   - New config written to temp file, DryRun validated before atomic replace
//   - Current config backed up as .bak (immediate rollback) and LKG (cross-deploy rollback)
//   - Apply / HealthCheck failure triggers auto-rollback: LKG first, then .bak
//   - On success, LKG updated to new config (always represents last successful deploy)
func (p *Pipeline) Run(
	ctx context.Context,
	version string,
	configJSON []byte,
	configPath string,
	runtimeType string,
	callbacks DeployCallbacks,
) *PipelineResult {
	start := time.Now()
	result := &PipelineResult{Version: version}

	// Phase 0: Write deploy lock
	if err := p.WriteDeployLock(version); err != nil {
		p.logger.Warn("failed to write deploy.lock", "error", err)
	}
	defer p.RemoveDeployLock()

	// Phase 1: Edge pre-check
	result.Phase = "precheck"
	if p.validator != nil {
		edgeResult, err := p.validator.PreCheckEdge(ctx, configJSON, runtimeType)
		if err != nil {
			result.Error = fmt.Sprintf("edge pre-check error: %v", err)
			result.ApplyDurationMs = time.Since(start).Milliseconds()
			return result
		}
		if !edgeResult.Passed {
			result.Error = fmt.Sprintf("edge pre-check failed: %s", joinErrors(edgeResult.Errors))
			result.ApplyDurationMs = time.Since(start).Milliseconds()
			return result
		}
		p.logger.Info("edge pre-check passed", "duration", edgeResult.Duration)
	}

	// Phase 2: Write temp config file
	result.Phase = "activate"
	tmpConfigPath := configPath + ".new"
	if err := writeAtomic(tmpConfigPath, configJSON, 0644); err != nil {
		result.Error = fmt.Sprintf("write temp config failed: %v", err)
		result.ApplyDurationMs = time.Since(start).Milliseconds()
		return result
	}

	// Phase 3: DryRun validation (xray -test / sing-box check on temp file)
	if callbacks.DryRun != nil {
		result.Phase = "dryrun"
		if err := callbacks.DryRun(ctx, tmpConfigPath); err != nil {
			result.Error = fmt.Sprintf("dry-run failed: %v", err)
			p.logger.Error("dry-run failed", "error", err)
			os.Remove(tmpConfigPath)
			result.ApplyDurationMs = time.Since(start).Milliseconds()
			return result
		}
	}

	// Phase 4: Backup LKG + .bak (before overwriting current config)
	result.Phase = "activate"
	backupPath := configPath + ".bak"
	if err := p.BackupLKG(configPath, runtimeType); err != nil {
		p.logger.Warn("LKG backup failed, continuing", "error", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		_ = os.Rename(configPath, backupPath)
	}

	// Phase 5: Activate - atomic replace
	if err := os.Rename(tmpConfigPath, configPath); err != nil {
		result.Error = fmt.Sprintf("rename config failed: %v", err)
		p.logger.Error("rename config failed", "error", err)
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = os.Rename(backupPath, configPath)
		}
		result.ApplyDurationMs = time.Since(start).Milliseconds()
		return result
	}

	// Phase 6: Apply config to runtime (SIGUSR1/restart/AlterInbound)
	if callbacks.Apply != nil {
		if err := callbacks.Apply(ctx, configPath); err != nil {
			result.Error = fmt.Sprintf("apply failed: %v", err)
			p.logger.Error("apply failed", "error", err)
			p.rollback(ctx, configPath, backupPath, runtimeType, callbacks)
			result.Phase = "rollback"
			result.RolledBack = true
			result.ApplyDurationMs = time.Since(start).Milliseconds()
			return result
		}
	}

	// Phase 7: HealthCheck (process alive + port reachable)
	result.Phase = "healthcheck"
	if callbacks.HealthCheck != nil {
		if err := callbacks.HealthCheck(ctx, configJSON); err != nil {
			result.Error = fmt.Sprintf("health check failed: %v", err)
			p.logger.Error("health check failed", "error", err)
			p.rollback(ctx, configPath, backupPath, runtimeType, callbacks)
			result.Phase = "rollback"
			result.RolledBack = true
			result.ApplyDurationMs = time.Since(start).Milliseconds()
			return result
		}
	}

	// Phase 8: Success - update LKG with new config, cleanup .bak
	if err := p.UpdateLKG(configPath, runtimeType); err != nil {
		p.logger.Warn("failed to update LKG on success", "error", err)
	}
	os.Remove(backupPath)

	result.Success = true
	result.Phase = "activate"
	result.ApplyDurationMs = time.Since(start).Milliseconds()
	return result
}

// rollback restores config and reloads runtime on deployment failure.
//
// Rollback strategy (E8 fix - avoid double Reload):
//  1. Try LKG first (last successful config across deploys)
//  2. Fall back to .bak (config before this deploy attempt)
//  3. Call OnRollback callback to reload runtime (single Reload)
func (p *Pipeline) rollback(
	ctx context.Context,
	configPath, backupPath, runtimeType string,
	callbacks DeployCallbacks,
) {
	restored := false
	if p.HasLKG(runtimeType) {
		if err := p.RestoreLKG(configPath, runtimeType); err != nil {
			p.logger.Error("LKG restore failed, trying .bak", "error", err)
		} else {
			restored = true
			p.logger.Info("config restored from LKG")
		}
	}
	if !restored {
		if _, err := os.Stat(backupPath); err == nil {
			if err := os.Rename(backupPath, configPath); err != nil {
				p.logger.Error("failed to restore from .bak", "error", err)
			} else {
				restored = true
				p.logger.Info("config restored from .bak")
			}
		}
	}
	if restored && callbacks.OnRollback != nil {
		if err := callbacks.OnRollback(ctx, configPath); err != nil {
			p.logger.Error("rollback callback failed", "error", err)
		}
	}
}

// joinErrors 将错误列表拼接为单个字符串
func joinErrors(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}

// writeAtomic 原子写入文件
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}
