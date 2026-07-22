package main

// binary_reconciler.go 实现 P2-4：BinaryReconciler
//
// 功能：
//   - 定期从面板拉取期望的二进制规格（版本 + 下载 URL + SHA-256 checksum）
//   - 检测到版本差异时：下载 → 校验 checksum → 备份旧二进制 → 原子替换 → 重启
//   - 支持灰度策略（canary/rolling/all_at_once，由面板控制哪些节点先升级）
//   - 支持回滚：升级失败或新版本启动失败时，自动恢复旧二进制
//   - 实现resource.Resource 接口，由 ReconcilerDriver 统一调度
//
// 安全保障：
//   - SHA-256 checksum 校验：下载后验证，不匹配则拒绝安装
//   - 原子替换：tmp 文件 → rename，避免半写入状态
//   - 自动回滚：新二进制启动失败 → 恢复备份 → 重启旧版本
//   - 版本持久化：记录当前版本到 state file，重启后能恢复状态

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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/resource"
)

// BinaryReconciler P2-4: xray/sing-box 二进制远程升级协调器
type BinaryReconciler struct {
	httpClient *client.Client
	logger     *slog.Logger
	// runtimeType: "xray" / "sing-box"
	runtimeType string
	// binPath: 二进制安装路径（如 /usr/local/bin/xray）
	binPath string
	// backupPath: 旧版本备份路径（如 /usr/local/bin/xray.bak）
	backupPath string
	// stateFile: 当前已安装版本状态文件
	stateFile string
	// interval: 轮询周期
	interval time.Duration
	// restartFunc: 升级后重启 runtime 的回调（由 main 注入）
	restartFunc func(ctx context.Context) error

	mu              sync.Mutex
	lastAppliedHash string // = version + checksum 的 hash
	consecutiveNoop int
}

// binaryState 持久化到 stateFile 的状态
type binaryState struct {
	Version     string `json:"version"`
	Checksum    string `json:"checksum"`
	DownloadURL string `json:"download_url"`
	InstalledAt string `json:"installed_at"`
}

// NewBinaryReconciler 创建二进制升级协调器。
// binPath: 二进制路径（如 /usr/local/bin/xray）
// restartFunc: 升级后重启 runtime 的回调
func NewBinaryReconciler(
	httpClient *client.Client,
	logger *slog.Logger,
	runtimeType, binPath, stateFile string,
	restartFunc func(ctx context.Context) error,
) *BinaryReconciler {
	r := &BinaryReconciler{
		httpClient:  httpClient,
		logger:      logger.With("component", "binary-reconciler"),
		runtimeType: runtimeType,
		binPath:     binPath,
		backupPath:  binPath + ".bak",
		stateFile:   stateFile,
		interval:    60 * time.Second, // 比其他 reconciler 慢，避免频繁检查
		restartFunc: restartFunc,
	}
	// 加载上次的状态
	if data, err := os.ReadFile(stateFile); err == nil {
		var st binaryState
		if json.Unmarshal(data, &st) == nil {
			r.lastAppliedHash = hashBinaryState(st)
			r.logger.Info("loaded previous binary state",
				"version", st.Version, "checksum", st.Checksum[:min(8, len(st.Checksum))])
		}
	}
	return r
}

// --- Resource 接口实现（P2-1 + P2-4）---

// Kind 实现 resource.Resource
func (r *BinaryReconciler) Kind() string { return "binary-" + r.runtimeType }

// Observe 实现 resource.Resource：返回当前已安装的二进制状态
func (r *BinaryReconciler) Observe(ctx context.Context) (resource.ObservedState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return resource.ObservedState{
		Hash:  r.lastAppliedHash,
		Empty: r.lastAppliedHash == "",
	}, nil
}

// FetchDesired 实现 resource.Resource：从面板拉取期望的二进制规格
func (r *BinaryReconciler) FetchDesired(ctx context.Context) (resource.DesiredState, error) {
	spec, err := r.httpClient.FetchBinarySpec(ctx)
	if err != nil {
		return resource.DesiredState{}, err
	}
	if spec == nil || spec.Version == "" || spec.DownloadURL == "" {
		// 无升级任务
		return resource.DesiredState{Hash: r.lastAppliedHash}, nil
	}
	// 仅处理与本 runtime 类型匹配的规格
	if spec.RuntimeType != "" && spec.RuntimeType != r.runtimeType {
		return resource.DesiredState{Hash: r.lastAppliedHash}, nil
	}
	desired := binaryState{
		Version:     spec.Version,
		Checksum:    spec.Checksum,
		DownloadURL: spec.DownloadURL,
	}
	hash := hashBinaryState(desired)
	// Force 模式：即使 hash 相同也触发升级
	if spec.Force {
		hash = hash + ":force:" + time.Now().Format(time.RFC3339)
	}
	return resource.DesiredState{
		Hash: hash,
		Raw:  spec,
	}, nil
}

// Diff 实现 resource.Resource
func (r *BinaryReconciler) Diff(desired resource.DesiredState, observed resource.ObservedState) (resource.DiffResult, error) {
	if desired.Hash == observed.Hash {
		return resource.DiffResult{HasDrift: false, Level: resource.LevelNone}, nil
	}
	spec, ok := desired.Raw.(*client.BinarySpec)
	if !ok || spec == nil {
		return resource.DiffResult{HasDrift: false, Level: resource.LevelNone}, nil
	}
	return resource.DiffResult{
		HasDrift: true,
		Level:    resource.LevelRestart,
		Summary:  fmt.Sprintf("binary upgrade: %s → v%s", r.runtimeType, spec.Version),
		Raw:      spec,
	}, nil
}

// Apply 实现 resource.Resource：下载 → 校验 → 备份 → 替换 → 重启
func (r *BinaryReconciler) Apply(ctx context.Context, diff resource.DiffResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	spec, ok := diff.Raw.(*client.BinarySpec)
	if !ok || spec == nil {
		return nil
	}

	r.logger.Info("starting binary upgrade",
		"runtime", r.runtimeType,
		"target_version", spec.Version,
		"download_url", spec.DownloadURL,
		"strategy", spec.Strategy,
		"checksum", spec.Checksum[:min(8, len(spec.Checksum))])

	// 1. 下载到临时文件
	tmpPath := r.binPath + ".tmp"
	if err := r.downloadFile(ctx, spec.DownloadURL, tmpPath); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer os.Remove(tmpPath) // 清理临时文件（如果还在）

	// 2. SHA-256 校验
	// 安全修复 S5：无 checksum 时拒绝安装（防止供应链攻击）
	if spec.Checksum != "" {
		actualChecksum, err := computeFileSHA256(tmpPath)
		if err != nil {
			return fmt.Errorf("计算 checksum 失败: %w", err)
		}
		if !strings.EqualFold(actualChecksum, spec.Checksum) {
			return fmt.Errorf("checksum 校验失败: expected=%s actual=%s", spec.Checksum[:min(8, len(spec.Checksum))], actualChecksum[:min(8, len(actualChecksum))])
		}
		r.logger.Info("checksum verified", "checksum", actualChecksum[:min(8, len(actualChecksum))])
	} else {
		return fmt.Errorf("binary checksum missing — refusing to install unverified binary (supply chain security)")
	}

	// 3. 设置可执行权限
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	// 4. 备份当前二进制（如果存在）
	if _, err := os.Stat(r.binPath); err == nil {
		_ = os.Remove(r.backupPath) // 清理旧备份
		if err := os.Rename(r.binPath, r.backupPath); err != nil {
			return fmt.Errorf("备份旧二进制失败: %w", err)
		}
		r.logger.Info("backed up current binary", "backup", r.backupPath)
	}

	// 5. 原子替换（rename）
	if err := os.Rename(tmpPath, r.binPath); err != nil {
		// 替换失败，尝试恢复备份
		if _, backupErr := os.Stat(r.backupPath); backupErr == nil {
			_ = os.Rename(r.backupPath, r.binPath)
		}
		return fmt.Errorf("替换二进制失败: %w", err)
	}

	// 6. 验证新二进制可执行
	if err := r.verifyBinary(ctx, r.binPath); err != nil {
		r.logger.Error("new binary verification failed, rolling back", "error", err)
		// 回滚
		if _, backupErr := os.Stat(r.backupPath); backupErr == nil {
			_ = os.Remove(r.binPath)
			_ = os.Rename(r.backupPath, r.binPath)
			r.logger.Info("rolled back to previous binary")
		}
		return fmt.Errorf("新二进制验证失败（已回滚）: %w", err)
	}

	// 7. 重启 runtime
	if r.restartFunc != nil {
		r.logger.Info("restarting runtime with new binary")
		if err := r.restartFunc(ctx); err != nil {
			r.logger.Error("restart failed after upgrade, attempting rollback", "error", err)
			// 重启失败也尝试回滚
			if _, backupErr := os.Stat(r.backupPath); backupErr == nil {
				_ = os.Remove(r.binPath)
				_ = os.Rename(r.backupPath, r.binPath)
				if rerr := r.restartFunc(ctx); rerr != nil {
					r.logger.Error("rollback restart also failed", "error", rerr)
				} else {
					r.logger.Info("rollback successful")
				}
			}
			return fmt.Errorf("升级后重启失败（已回滚）: %w", err)
		}
	}

	r.logger.Info("binary upgrade completed successfully",
		"runtime", r.runtimeType, "version", spec.Version)
	return nil
}

// Persist 实现 resource.Resource：持久化版本状态
func (r *BinaryReconciler) Persist(ctx context.Context, desired resource.DesiredState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	spec, ok := desired.Raw.(*client.BinarySpec)
	if !ok || spec == nil {
		return nil
	}
	st := binaryState{
		Version:     spec.Version,
		Checksum:    spec.Checksum,
		DownloadURL: spec.DownloadURL,
		InstalledAt: time.Now().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	if err := os.MkdirAll(filepath.Dir(r.stateFile), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(r.stateFile, data, 0644); err != nil {
		return err
	}
	r.lastAppliedHash = hashBinaryState(st)
	r.consecutiveNoop = 0
	return nil
}

// --- 辅助方法 ---

// downloadFile 下载文件到指定路径
func (r *BinaryReconciler) downloadFile(ctx context.Context, url, destPath string) error {
	// B28 修复: 为二进制下载添加 5 分钟超时与 200MB 体积上限，
	// 防止恶意/异常下载 URL 导致请求无限挂起或磁盘/内存耗尽。
	const maxDownloadSize int64 = 200 * 1024 * 1024 // 200MB
	downloadClient := &http.Client{
		Timeout: 5 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// 限制响应体大小，超过 200MB 视为异常并中止下载
	limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, limitedReader)
	if err != nil {
		return err
	}
	if written > maxDownloadSize {
		return fmt.Errorf("下载内容超过 %d 字节上限，已中止", maxDownloadSize)
	}
	return nil
}

// computeFileSHA256 计算文件的 SHA-256 校验和（hex 编码）
func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyBinary 验证二进制可执行（调用 --version 或 version 子命令）
func (r *BinaryReconciler) verifyBinary(ctx context.Context, path string) error {
	// 尝试执行 version 子命令
	cmd := exec.CommandContext(ctx, path, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 某些二进制可能不支持 version 子命令，尝试 --version
		cmd2 := exec.CommandContext(ctx, path, "--version")
		output2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("binary verification failed: %w (output: %s)", err, string(output))
		}
		r.logger.Info("binary version", "output", strings.TrimSpace(string(output2)))
		return nil
	}
	r.logger.Info("binary version", "output", strings.TrimSpace(string(output)))
	return nil
}

// hashBinaryState 计算二进制状态的内容 hash
func hashBinaryState(st binaryState) string {
	data, _ := json.Marshal(st)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
