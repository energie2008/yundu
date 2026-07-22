package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/node-agent/internal/cert"
	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/nginx"
	"github.com/airport-panel/node-agent/internal/resource"
)

// NginxReconciler 是独立的 nginx vhost 协调循环。
//
// 设计原则：
//   - 状态源独立：期望状态来自面板的 /agent/cdn-vhosts 端点，不依赖 xray config_versions 表
//   - 检测-diff-执行分离：每轮独立拉取期望态，计算 hash diff，只在有真实变更时执行
//   - 幂等：重复执行同一份 vhost 内容不导致 nginx reload 抖动
//   - 独立哈希：用独立 hash 文件追踪状态，完全脱离 version.txt 保护逻辑
//   - 证书自动签发：通过 ACME DNS-01 challenge，每台 VPS 独立签发自己负责域名的证书
//
// 这样 nginx vhost 同步不再被 applyConfig 调用链和 version.txt 保护机制"连坐"，
// 实现真正的零 SSH 自动化。
type NginxReconciler struct {
	httpClient client.VhostFetcher
	logger     *slog.Logger
	certMgr    *cert.Manager
	// stateFile 记录上次成功应用的 vhost hash，独立于 version.txt
	stateFile string
	// interval 轮询周期，默认 30 秒
	interval time.Duration
	// nginxEnv nginx 环境类型："bt"（宝塔）、"standard"（标准）或 "none"（无 nginx）
	nginxEnv string

	mu               sync.Mutex
	lastHash         string
	consecutiveNoop  int
	consecutiveError int
}

// NewNginxReconciler 创建 nginx vhost 协调器。
// stateFile 通常为 /etc/yundu/nginx_vhost_state.hash
// certMgr 为证书管理器，可为 nil（禁用 ACME 签发，仅复用已有证书）
func NewNginxReconciler(httpClient client.VhostFetcher, logger *slog.Logger, stateFile, nginxEnv string, certMgr *cert.Manager) *NginxReconciler {
	r := &NginxReconciler{
		httpClient: httpClient,
		logger:     logger.With("component", "nginx-reconciler"),
		certMgr:    certMgr,
		stateFile:  stateFile,
		nginxEnv:   nginxEnv,
		interval:   30 * time.Second,
	}
	// 启动时加载上次的 hash（如果存在）
	if data, err := os.ReadFile(stateFile); err == nil {
		r.lastHash = string(data)
		r.logger.Info("loaded previous nginx vhost hash", "hash", r.lastHash[:min(8, len(r.lastHash))])
	}
	return r
}

// Start 启动独立协调循环（阻塞，应在独立 goroutine 中调用）。
// 启动时立即执行一次，之后按 interval 周期执行。
func (r *NginxReconciler) Start(ctx context.Context) {
	r.logger.Info("nginx reconciler started", "interval", r.interval, "state_file", r.stateFile)

	// 启动时确保 nginx 骨架配置存在
	if err := nginx.EnsureNginxSkeleton(r.logger); err != nil {
		r.logger.Error("failed to ensure nginx skeleton", "error", err)
	}

	// 启动时先跑一次
	r.reconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("nginx reconciler stopped")
			return
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

// TriggerSync 立即触发一次同步（非阻塞，在独立 goroutine 中执行）。
// 供心跳 SYNC_EXTERNAL_RESOURCES Action 调用，消除 30s 轮询延迟。
// 使用 tryLock 机制避免与正在执行的 reconcileOnce 冲突；若正在执行则跳过本次触发。
func (r *NginxReconciler) TriggerSync(ctx context.Context) {
	go func() {
		if !r.mu.TryLock() {
			r.logger.Debug("nginx reconciler busy, skip triggered sync")
			return
		}
		defer r.mu.Unlock()
		r.logger.Info("triggered nginx vhost sync (heartbeat action)")
		r.reconcileOnceLocked(ctx)
	}()
}
// reconcileOnce 执行一次协调：拉取期望态 → 计算 diff → 有变更才写入+reload。
// 调用方负责加锁（与 TriggerSync 共用 reconcileOnceLocked）。
func (r *NginxReconciler) reconcileOnce(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconcileOnceLocked(ctx)
}

// reconcileOnceLocked 是 reconcileOnce 的已加锁版本，供 TriggerSync 复用。
// 调用前必须已持有 r.mu。
func (r *NginxReconciler) reconcileOnceLocked(ctx context.Context) {
	// 1. 从面板拉取 CDN vhost 期望状态（独立接口，不走 xray config）
	vhosts, err := r.httpClient.FetchCDNVhosts(ctx)
	if err != nil {
		r.consecutiveError++
		if r.consecutiveError <= 3 || r.consecutiveError%10 == 0 {
			r.logger.Warn("failed to fetch cdn vhosts",
				"error", err, "consecutive_errors", r.consecutiveError)
		}
		return
	}
	r.consecutiveError = 0

	// 2. 计算 hash（用内容 hash 而非版本号，避免和 version.txt 混用）
	newHash := hashVhosts(vhosts)
	if newHash == r.lastHash {
		r.consecutiveNoop++
		// 每 5 分钟（约 10 轮）打一次心跳日志，证明 loop 活着
		if r.consecutiveNoop%10 == 0 {
			r.logger.Info("no drift detected, nginx vhosts in sync",
				"hash", newHash[:min(8, len(newHash))])
		}
		return
	}

	// 3. 检测到变更，执行同步
	r.logger.Info("drift detected, applying nginx vhosts",
		"old_hash", r.lastHash[:min(8, len(r.lastHash))],
		"new_hash", newHash[:min(8, len(newHash))],
		"https_len", len(vhosts.HTTPSSnippet),
		"stream_len", len(vhosts.StreamSnippet),
		"domains", len(vhosts.Domains))

	// 3.5 证书预签发：在写入 nginx snippet 前，确保所有域名证书已就绪
	//     签发失败的域名会被记录，但不阻断其他域名的 vhost 同步
	//     下一轮 reconciler 会重试签发失败的域名
	//     P2 TLS分离架构改造 719：证书签发不再依赖 HTTPSSnippet（CDN 节点改为 stream 透传后无 HTTP server block）
	//     只要 domains 非空就触发签发，覆盖 CDN/direct/argo_tunnel 所有场景
	if r.certMgr != nil && len(vhosts.Domains) > 0 {
		r.ensureCerts(vhosts.Domains)
	}

	// 4. 选择 nginx 环境配置（宝塔 / 标准骨架 / 无 nginx）
	//    EnvNone 时跳过同步（直连节点无 nginx，不应报错）
	var syncCfg *nginx.SyncConfig
	switch r.nginxEnv {
	case nginx.EnvBtPanel:
		syncCfg = nginx.DefaultBtPanelConfig()
	case nginx.EnvStandard:
		syncCfg = nginx.DefaultSkeletonSyncConfig()
	case nginx.EnvNone:
		// 无 nginx 环境（如纯直连节点），跳过同步但更新 hash 避免重复拉取
		r.lastHash = newHash
		r.consecutiveNoop = 0
		_ = os.MkdirAll(filepath.Dir(r.stateFile), 0755)
		_ = os.WriteFile(r.stateFile, []byte(newHash), 0644)
		r.logger.Info("nginx not detected, skipping vhost sync",
			"hash", newHash[:min(8, len(newHash))])
		return
	default:
		syncCfg = nginx.DefaultBtPanelConfig()
	}

	// 5. 原子写入 snippet（nginx -t 校验 + rename + reload）
	result, err := nginx.Sync(vhosts.StreamSnippet, vhosts.HTTPSSnippet, syncCfg)
	if err != nil {
		r.logger.Error("nginx sync failed, will retry next cycle",
			"error", err,
			"stream_applied", result.StreamApplied,
			"https_applied", result.HTTPSApplied,
			"nginx_reloaded", result.NginxReloaded,
			"nginx_test_out", result.NginxTestOut)
		// 不更新 hash，下轮重试
		return
	}

	// 6. 成功：更新 hash 并持久化
	r.lastHash = newHash
	r.consecutiveNoop = 0
	_ = os.MkdirAll(filepath.Dir(r.stateFile), 0755)
	if err := os.WriteFile(r.stateFile, []byte(newHash), 0644); err != nil {
		r.logger.Warn("failed to persist nginx vhost hash", "error", err)
	}

	r.logger.Info("nginx vhosts applied successfully",
		"stream_applied", result.StreamApplied,
		"https_applied", result.HTTPSApplied,
		"nginx_reloaded", result.NginxReloaded)
}

// hashVhosts 计算 CDN vhost 响应的内容 hash。
// 使用 SHA-256，确保内容任意字节变化都能检测到。
func hashVhosts(v *client.CDNVhostResponse) string {
	// 序列化为确定性 JSON（字段固定顺序）
	data, _ := json.Marshal(struct {
		HTTPS   string   `json:"https_snippet"`
		Stream  string   `json:"stream_snippet"`
		Port    int      `json:"listen_port"`
		Domains []string `json:"domains"`
	}{
		HTTPS:   v.HTTPSSnippet,
		Stream:  v.StreamSnippet,
		Port:    v.ListenPort,
		Domains: v.Domains,
	})
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ensureCerts 确保所有域名证书已签发且有效。
// 签发失败的域名会被记录警告，但不阻断后续 vhost 同步。
// nginx.Sync 写入 snippet 时，如果证书文件不存在，nginx -t 会失败并回滚，
// 所以必须先确保证书就绪。
func (r *NginxReconciler) ensureCerts(domains []string) {
	successCount := 0
	failedDomains := make([]string, 0, len(domains))

	for _, domain := range domains {
		if domain == "" {
			continue
		}
		_, _, err := r.certMgr.EnsureCert(domain)
		if err != nil {
			r.logger.Warn("cert ensure failed, vhost for this domain may fail",
				"domain", domain, "error", err)
			failedDomains = append(failedDomains, domain)
			continue
		}
		successCount++
	}

	if len(failedDomains) > 0 {
		r.logger.Warn("some domains cert issuance failed",
			"success", successCount,
			"failed", len(failedDomains),
			"failed_domains", strings.Join(failedDomains, ","))
	} else if successCount > 0 {
		r.logger.Info("all certs ready",
			"domains", successCount)
	}
}

// --- Resource 接口实现（P2-1）---
// 以下方法将 NginxReconciler 适配为 resource.Resource 接口，
// 可由 resource.Driver 统一调度。原有的 Start/reconcileOnce 方法保留向后兼容。

// Kind 实现 resource.Resource
func (r *NginxReconciler) Kind() string { return "nginx-vhost" }

// Observe 实现 resource.Resource：返回当前持久化的 hash 状态
func (r *NginxReconciler) Observe(ctx context.Context) (resource.ObservedState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return resource.ObservedState{
		Hash:  r.lastHash,
		Empty: r.lastHash == "",
	}, nil
}

// FetchDesired 实现 resource.Resource：从面板拉取 CDN vhost 期望状态
func (r *NginxReconciler) FetchDesired(ctx context.Context) (resource.DesiredState, error) {
	vhosts, err := r.httpClient.FetchCDNVhosts(ctx)
	if err != nil {
		return resource.DesiredState{}, err
	}
	return resource.DesiredState{
		Hash: hashVhosts(vhosts),
		Raw:  vhosts,
	}, nil
}

// Diff 实现 resource.Resource：比较期望态与观察态的 hash
func (r *NginxReconciler) Diff(desired resource.DesiredState, observed resource.ObservedState) (resource.DiffResult, error) {
	if desired.Hash == observed.Hash {
		return resource.DiffResult{HasDrift: false, Level: resource.LevelNone}, nil
	}
	return resource.DiffResult{
		HasDrift: true,
		Level:    resource.LevelFullSync,
		Summary:  "nginx vhost hash mismatch",
		Raw:      desired.Raw,
	}, nil
}

// Apply 实现 resource.Resource：写入 nginx snippet + reload
func (r *NginxReconciler) Apply(ctx context.Context, diff resource.DiffResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	vhosts, ok := diff.Raw.(*client.CDNVhostResponse)
	if !ok || vhosts == nil {
		return nil
	}

	// 证书预签发
	// P2 TLS分离架构改造 719：证书签发不再依赖 HTTPSSnippet（CDN 节点改为 stream 透传后无 HTTP server block）
	if r.certMgr != nil && len(vhosts.Domains) > 0 {
		r.ensureCerts(vhosts.Domains)
	}

	// 无 nginx 环境跳过
	if r.nginxEnv == nginx.EnvNone {
		r.logger.Info("nginx not detected, skipping vhost sync")
		return nil
	}

	var syncCfg *nginx.SyncConfig
	switch r.nginxEnv {
	case nginx.EnvBtPanel:
		syncCfg = nginx.DefaultBtPanelConfig()
	default:
		syncCfg = nginx.DefaultSkeletonSyncConfig()
	}

	result, err := nginx.Sync(vhosts.StreamSnippet, vhosts.HTTPSSnippet, syncCfg)
	if err != nil {
		r.logger.Error("nginx sync failed",
			"error", err,
			"stream_applied", result.StreamApplied,
			"https_applied", result.HTTPSApplied,
			"nginx_reloaded", result.NginxReloaded)
		return err
	}

	r.logger.Info("nginx vhosts applied successfully",
		"stream_applied", result.StreamApplied,
		"https_applied", result.HTTPSApplied,
		"nginx_reloaded", result.NginxReloaded)
	return nil
}

// Persist 实现 resource.Resource：持久化 hash 到 stateFile
func (r *NginxReconciler) Persist(ctx context.Context, desired resource.DesiredState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastHash = desired.Hash
	r.consecutiveNoop = 0
	_ = os.MkdirAll(filepath.Dir(r.stateFile), 0755)
	return os.WriteFile(r.stateFile, []byte(desired.Hash), 0644)
}
