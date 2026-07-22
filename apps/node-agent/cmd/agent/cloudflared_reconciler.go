package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/resource"
)

// cloudflaredConfigPath cloudflared 配置文件路径
const cloudflaredConfigPath = "/etc/cloudflared/config.yml"

// CloudflaredReconciler 是独立的 cloudflared 隧道协调循环。
//
// 设计原则（仿照 NginxReconciler）：
//   - 状态源独立：期望状态来自面板的 /agent/cloudflared-tunnels 端点
//   - 检测-diff-执行分离：每轮独立拉取期望态，计算 hash diff，只在有真实变更时执行
//   - 幂等：重复执行同一份隧道配置不导致 cloudflared 重启
//   - 独立哈希：用独立 hash 文件追踪状态，完全脱离 version.txt 保护逻辑
type CloudflaredReconciler struct {
	// T05: 改用 TunnelFetcher 接口，让 Machine 模式（MachineClient）和单节点模式（*Client）都能复用。
	httpClient client.TunnelFetcher
	logger     *slog.Logger
	// stateFile 记录上次成功应用的隧道配置 hash
	stateFile string
	// interval 轮询周期，默认 30 秒
	interval time.Duration

	mu              sync.Mutex
	lastHash        string
	consecutiveNoop int

	cmd    *exec.Cmd  // 当前运行的 cloudflared 进程
	waitCh chan error // cloudflared 进程退出信号
}

// NewCloudflaredReconciler 创建 cloudflared 隧道协调器。
// stateFile 通常为 /etc/yundu/cloudflared_state.hash
// httpClient 参数接受 client.TunnelFetcher 接口（T05），允许 *client.Client 或 *client.MachineClient 注入。
func NewCloudflaredReconciler(httpClient client.TunnelFetcher, logger *slog.Logger, stateFile string) *CloudflaredReconciler {
	r := &CloudflaredReconciler{
		httpClient: httpClient,
		logger:     logger.With("component", "cloudflared-reconciler"),
		stateFile:  stateFile,
		interval:   30 * time.Second,
	}
	// 启动时加载上次的 hash（如果存在）
	if data, err := os.ReadFile(stateFile); err == nil {
		r.lastHash = string(data)
		r.logger.Info("loaded previous cloudflared hash", "hash", r.lastHash[:min(8, len(r.lastHash))])
	}
	return r
}

// Start 启动独立协调循环（阻塞，应在独立 goroutine 中调用）。
// 启动时立即执行一次，之后按 interval 周期执行。
// context 取消时停止 cloudflared 进程并退出。
func (r *CloudflaredReconciler) Start(ctx context.Context) {
	r.logger.Info("cloudflared reconciler started", "interval", r.interval, "state_file", r.stateFile)

	// 启动时先跑一次
	r.reconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("cloudflared reconciler stopping, stopping tunnel process")
			r.stopProcess()
			r.logger.Info("cloudflared reconciler stopped")
			return
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

// reconcileOnce 执行一次协调：拉取期望态 → 计算 diff → 有变更才写入+重启。
func (r *CloudflaredReconciler) reconcileOnce(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. 从面板拉取 cloudflared 隧道期望状态（独立接口）
	cfg, err := r.httpClient.FetchCloudflaredTunnels(ctx)
	if err != nil {
		r.logger.Warn("failed to fetch cloudflared tunnels", "error", err)
		return err
	}

	// 2. 计算 hash（用内容 hash 而非版本号，避免和 version.txt 混用）
	newHash := hashCloudflaredConfig(cfg)
	if newHash == r.lastHash {
		r.consecutiveNoop++
		// 每 5 分钟（约 10 轮）打一次心跳日志，证明 loop 活着
		if r.consecutiveNoop%10 == 0 {
			r.logger.Info("no drift detected, cloudflared tunnels in sync",
				"hash", newHash[:min(8, len(newHash))])
		}
		return nil
	}

	// 3. 检测到变更，执行同步
	r.logger.Info("drift detected, applying cloudflared tunnels",
		"old_hash", r.lastHash[:min(8, len(r.lastHash))],
		"new_hash", newHash[:min(8, len(newHash))],
		"tunnels", len(cfg.Tunnels))

	// 3a. 无隧道配置 → 停止 cloudflared 进程
	if len(cfg.Tunnels) == 0 {
		r.stopProcess()
		r.lastHash = newHash
		r.consecutiveNoop = 0
		r.persistHash(newHash)
		r.logger.Info("no tunnels configured, cloudflared stopped")
		return nil
	}

	// 3b. 检查 cloudflared 二进制是否存在
	//     不存在时记录警告并跳过（不报错），仍更新 hash 避免重复拉取刷屏
	if _, err := exec.LookPath("cloudflared"); err != nil {
		r.logger.Warn("cloudflared binary not found, skipping process management",
			"error", err)
		r.lastHash = newHash
		r.consecutiveNoop = 0
		r.persistHash(newHash)
		return nil
	}

	// 3c. 取第一个有效隧道（当前实现单隧道）
	tunnel := cfg.Tunnels[0]

	// 3d. 写入 config.yml（在停止旧进程前写入，减少 downtime）
	if err := r.writeConfigYAML(tunnel); err != nil {
		r.logger.Error("failed to write cloudflared config.yml", "error", err)
		// 不更新 hash，下轮重试
		return err
	}

	// 3e. 重启 cloudflared 进程（先停后启）
	r.stopProcess()
	if err := r.startProcess(tunnel); err != nil {
		r.logger.Error("failed to start cloudflared", "error", err)
		// 不更新 hash，下轮重试
		return err
	}

	// 4. 成功：更新 hash 并持久化
	r.lastHash = newHash
	r.consecutiveNoop = 0
	r.persistHash(newHash)

	r.logger.Info("cloudflared tunnels applied successfully",
		"tunnel_id", tunnel.TunnelID,
		"has_token", tunnel.Token != "",
		"ingress_rules", len(tunnel.Ingress))

	return nil
}

// writeConfigYAML 生成并写入 cloudflared config.yml。
// 手动拼接 YAML（结构简单），避免引入 yaml.v3 依赖。
func (r *CloudflaredReconciler) writeConfigYAML(tunnel client.CloudflaredTunnel) error {
	var sb strings.Builder

	// TunnelID 优先从面板下发的 tunnel.TunnelID 读取；
	// 若面板未下发（节点 config_json 无 cloudflared_tunnel_id），
	// 自动从本地 /etc/cloudflared/credentials.json 读取，
	// 保证单节点模式下 cloudflared 进程能以 "tunnel: <id>" 启动。
	tunnelID := tunnel.TunnelID
	if tunnelID == "" {
		if id, err := readLocalTunnelID(); err == nil && id != "" {
			tunnelID = id
			r.logger.Info("tunnel_id missing from panel, read from local credentials.json",
				"tunnel_id", id)
		} else if err != nil && !os.IsNotExist(err) {
			r.logger.Warn("failed to read local tunnel_id from credentials.json", "error", err)
		}
	}

	if tunnelID != "" {
		sb.WriteString(fmt.Sprintf("tunnel: %s\n", tunnelID))
		sb.WriteString(fmt.Sprintf("credentials-file: /etc/cloudflared/credentials.json\n"))
	}
	// 保留协议与 no-autoupdate 等基础字段
	sb.WriteString("protocol: http2\n")
	sb.WriteString("no-autoupdate: true\n")
	if len(tunnel.Ingress) > 0 {
		sb.WriteString("ingress:\n")
		for _, rule := range tunnel.Ingress {
			if rule.Hostname != "" {
				sb.WriteString(fmt.Sprintf("  - hostname: %s\n", rule.Hostname))
				sb.WriteString(fmt.Sprintf("    service: %s\n", rule.Service))
			}
		}
		// catch-all 规则（cloudflared 要求最后一条是 terminal rule）
		sb.WriteString("  - service: http_status:404\n")
	}
	return writeAtomic(cloudflaredConfigPath, []byte(sb.String()), 0644)
}

// readLocalTunnelID 从 /etc/cloudflared/credentials.json 读取 TunnelID。
// 单节点模式下，面板可能不下发 cloudflared_tunnel_id（依赖本地 credentials），
// 此函数提供 fallback，避免 config.yml 缺少 tunnel: 字段导致 cloudflared 启动失败。
func readLocalTunnelID() (string, error) {
	data, err := os.ReadFile("/etc/cloudflared/credentials.json")
	if err != nil {
		return "", err
	}
	var creds struct {
		TunnelID   string `json:"TunnelID"`
		AccountTag string `json:"AccountTag"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", err
	}
	return creds.TunnelID, nil
}

// startProcess 启动 cloudflared 进程。
// 优先使用 token 模式（cloudflared tunnel run --token <token>），
// 否则使用 config 模式（cloudflared tunnel --config <path> run）。
func (r *CloudflaredReconciler) startProcess(tunnel client.CloudflaredTunnel) error {
	var cmd *exec.Cmd
	mode := "config"
	if tunnel.Token != "" {
		cmd = exec.Command("cloudflared", "tunnel", "run", "--token", tunnel.Token)
		mode = "token"
	} else {
		cmd = exec.Command("cloudflared", "tunnel", "--config", cloudflaredConfigPath, "run")
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cloudflared: %w", err)
	}

	r.cmd = cmd
	r.waitCh = make(chan error, 1)

	// 转发进程输出到日志
	go r.pipeLogs(stdout, "stdout")
	go r.pipeLogs(stderr, "stderr")

	// 等待进程退出（捕获 channel 引用，避免 stopProcess 清空 r.waitCh 后发送到 nil channel）
	waitCh := r.waitCh
	go func() {
		err := cmd.Wait()
		r.logger.Info("cloudflared process exited", "pid", cmd.Process.Pid, "error", err)
		waitCh <- err
	}()

	r.logger.Info("cloudflared started", "pid", cmd.Process.Pid, "mode", mode)
	return nil
}

// stopProcess 停止 cloudflared 进程（SIGTERM → 10s 超时 → SIGKILL）。
func (r *CloudflaredReconciler) stopProcess() {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}

	r.logger.Info("stopping cloudflared process", "pid", r.cmd.Process.Pid)
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		r.logger.Warn("failed to send SIGTERM to cloudflared, killing", "error", err)
		_ = r.cmd.Process.Kill()
	}

	select {
	case <-r.waitCh:
		r.logger.Info("cloudflared process stopped cleanly")
	case <-time.After(10 * time.Second):
		r.logger.Warn("cloudflared did not exit after SIGTERM, killing")
		_ = r.cmd.Process.Kill()
		<-r.waitCh
	}

	r.cmd = nil
	r.waitCh = nil
}

// pipeLogs 读取进程输出并转发到日志。
func (r *CloudflaredReconciler) pipeLogs(rc io.ReadCloser, stream string) {
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		r.logger.Info("cloudflared "+stream, "line", scanner.Text())
	}
}

// persistHash 持久化 hash 到 state file。
func (r *CloudflaredReconciler) persistHash(hash string) {
	_ = os.MkdirAll(filepath.Dir(r.stateFile), 0755)
	if err := os.WriteFile(r.stateFile, []byte(hash), 0644); err != nil {
		r.logger.Warn("failed to persist cloudflared hash", "error", err)
	}
}

// hashCloudflaredConfig 计算隧道配置的内容 hash（SHA-256）。
// 确保内容任意字节变化都能检测到。
func hashCloudflaredConfig(cfg *client.CloudflaredTunnelConfig) string {
	data, _ := json.Marshal(cfg)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --- Resource 接口实现（P2-1）---

// Kind 实现 resource.Resource
func (r *CloudflaredReconciler) Kind() string { return "cloudflared-tunnel" }

// Observe 实现 resource.Resource
func (r *CloudflaredReconciler) Observe(ctx context.Context) (resource.ObservedState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return resource.ObservedState{
		Hash:  r.lastHash,
		Empty: r.lastHash == "",
	}, nil
}

// FetchDesired 实现 resource.Resource
func (r *CloudflaredReconciler) FetchDesired(ctx context.Context) (resource.DesiredState, error) {
	cfg, err := r.httpClient.FetchCloudflaredTunnels(ctx)
	if err != nil {
		return resource.DesiredState{}, err
	}
	return resource.DesiredState{
		Hash: hashCloudflaredConfig(cfg),
		Raw:  cfg,
	}, nil
}

// Diff 实现 resource.Resource
func (r *CloudflaredReconciler) Diff(desired resource.DesiredState, observed resource.ObservedState) (resource.DiffResult, error) {
	if desired.Hash == observed.Hash {
		return resource.DiffResult{HasDrift: false, Level: resource.LevelNone}, nil
	}
	return resource.DiffResult{
		HasDrift: true,
		Level:    resource.LevelFullSync,
		Summary:  "cloudflared tunnel hash mismatch",
		Raw:      desired.Raw,
	}, nil
}

// Apply 实现 resource.Resource
func (r *CloudflaredReconciler) Apply(ctx context.Context, diff resource.DiffResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, ok := diff.Raw.(*client.CloudflaredTunnelConfig)
	if !ok || cfg == nil {
		return nil
	}

	// 无隧道 → 停止进程
	if len(cfg.Tunnels) == 0 {
		r.stopProcess()
		return nil
	}

	// 检查二进制
	if _, err := exec.LookPath("cloudflared"); err != nil {
		r.logger.Warn("cloudflared binary not found, skipping")
		return nil
	}

	tunnel := cfg.Tunnels[0]
	if err := r.writeConfigYAML(tunnel); err != nil {
		return err
	}
	r.stopProcess()
	if err := r.startProcess(tunnel); err != nil {
		return err
	}
	return nil
}

// Persist 实现 resource.Resource
func (r *CloudflaredReconciler) Persist(ctx context.Context, desired resource.DesiredState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastHash = desired.Hash
	r.consecutiveNoop = 0
	r.persistHash(desired.Hash)
	return nil
}
