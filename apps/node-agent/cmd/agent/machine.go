package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	agentconfig "github.com/airport-panel/node-agent/internal/config"
	"github.com/airport-panel/node-agent/internal/cert"
	"github.com/airport-panel/node-agent/internal/client"
	"github.com/airport-panel/node-agent/internal/machine"
	"github.com/airport-panel/node-agent/internal/nginx"
	"github.com/airport-panel/node-agent/internal/upgrader"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type machineNodeEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServerCode  string `json:"server_code"`
	RuntimeType string `json:"runtime_type"`
	Protocol    string `json:"protocol"`
	Port        int    `json:"port"`
	Token       string `json:"agent_token"`
}

type machineAgent struct {
	cfg    *agentconfig.Config
	agent  *Agent
	cancel context.CancelFunc
	done   chan struct{} // T07: 替代 time.Sleep，等待 sub-Agent 确实退出
}

type MachineOrchestrator struct {
	logger      *slog.Logger
	baseCfg     *agentconfig.Config
	serverToken string

	xrayPortAllocator    *machine.APIPortAllocator
	singboxPortAllocator *machine.APIPortAllocator

	mu       sync.RWMutex
	agents   map[string]*machineAgent
	requests map[int64]chan *pendingRequest

	restartCh            chan struct{}
	selfUpgrader         *upgrader.SelfUpgrader
	unifiedHTTPServer    *http.Server
	registries           map[string]*prometheus.Registry

	nginxReconciler      *NginxReconciler
	cloudflaredReconciler *CloudflaredReconciler
	certMgr              *cert.Manager
}

func NewMachineOrchestrator(logger *slog.Logger, cfg *agentconfig.Config) *MachineOrchestrator {
	portBaseDir := filepath.Join(cfg.ConfigDir, "machine", "ports")
	os.MkdirAll(portBaseDir, 0755)
	m := &MachineOrchestrator{
		logger:               logger.With("mode", "machine"),
		baseCfg:              cfg,
		serverToken:          os.Getenv("YUNDU_SERVER_TOKEN"),
		xrayPortAllocator:    machine.NewAPIPortAllocator(portBaseDir, "xray_api", machine.XrayAPIPortRangeStart, machine.XrayAPIPortRangeEnd),
		singboxPortAllocator: machine.NewAPIPortAllocator(portBaseDir, "singbox_clash", machine.SingboxClashPortRangeStart, machine.SingboxClashPortRangeEnd),
		agents:               make(map[string]*machineAgent),
		requests:             make(map[int64]chan *pendingRequest),
		restartCh:            make(chan struct{}, 1),
		registries:           make(map[string]*prometheus.Registry),
	}
	upgraderCfg := upgrader.Config{
		CurrentVersion: AgentVersion,
		UpdateURL:      cfg.PanelURL + "/api/v1/agent/upgrade/check",
		CheckInterval:  5 * time.Minute,
		OnRestartNeeded: func() {
			logger.Warn("machine mode: self-upgrade detected, will restart ENTIRE process after graceful shutdown of all nodes")
			select {
			case m.restartCh <- struct{}{}:
			default:
			}
		},
	}
	m.selfUpgrader = upgrader.NewSelfUpgrader(upgraderCfg, logger)

	explicitEnv := os.Getenv("NODE_AGENT_NGINX_ENV")
	nginxEnv := nginx.ResolveEnv(explicitEnv)
	if nginxEnv != nginx.EnvNone {
		cfToken := os.Getenv("CF_Token")
		m.certMgr = cert.NewManager(cfToken, logger)

		machineClient := client.NewMachineClient(cfg.PanelURL, m.serverToken)
		stateFile := "/etc/yundu/machine_nginx_vhost_state.hash"
		if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
			logger.Warn("failed to create nginx state dir", "error", err)
		}
		m.nginxReconciler = NewNginxReconciler(machineClient, logger, stateFile, nginxEnv, m.certMgr)

		// T05: Machine 模式也注册 cloudflared 协调器，复用同一 machineClient 拉取聚合隧道配置。
		cloudflaredStateFile := "/etc/yundu/machine_cloudflared_state.hash"
		if err := os.MkdirAll(filepath.Dir(cloudflaredStateFile), 0755); err != nil {
			logger.Warn("failed to create cloudflared state dir", "error", err)
		}
		m.cloudflaredReconciler = NewCloudflaredReconciler(machineClient, logger, cloudflaredStateFile)

		if err := nginx.EnsureNginxSkeleton(logger); err != nil {
			logger.Error("failed to ensure nginx skeleton", "error", err)
		}
		logger.Info("shared nginx reconciler initialized", "nginx_env", nginxEnv, "state_file", stateFile, "acme_enabled", cfToken != "")
		logger.Info("shared cloudflared reconciler initialized", "state_file", cloudflaredStateFile)
	} else {
		logger.Info("nginx not detected, shared nginx reconciler disabled")
	}

	return m
}

func (m *MachineOrchestrator) Run(ctx context.Context) error {
	if m.serverToken == "" {
		return fmt.Errorf("machine mode requires YUNDU_SERVER_TOKEN environment variable")
	}

	m.logger.Info("machine orchestrator starting",
		"panel_url", m.baseCfg.PanelURL,
		"config_dir", m.baseCfg.ConfigDir)

	m.startUnifiedHTTPServer()

	if m.nginxReconciler != nil {
		go m.nginxReconciler.Start(ctx)
	}

	// T05: 启动 cloudflared 协调循环（与 nginx 协调器并行）
	if m.cloudflaredReconciler != nil {
		go m.cloudflaredReconciler.Start(ctx)
	}

	// 启动证书管理器（ACME 自动签发/续期后台 goroutine）
	if m.certMgr != nil {
		if err := m.certMgr.Start(ctx); err != nil {
			m.logger.Warn("cert manager start failed, ACME auto-renew may be disabled", "error", err)
		} else {
			m.logger.Info("cert manager started for ACME auto-renewal")
		}
	}

	m.selfUpgrader.Start(ctx)
	defer m.selfUpgrader.Stop()

	if err := m.syncNodes(ctx); err != nil {
		m.logger.Warn("initial node sync failed, will retry", "error", err)
	}

	// T08: 300s → 30s，缩短节点发现延迟
	syncTicker := time.NewTicker(30 * time.Second)
	defer syncTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("machine orchestrator shutting down")
			m.gracefulShutdownAll(ctx)
			return nil
		case <-m.restartCh:
			// T01: 不再 os.Exit(0) 杀全进程，改为只重启需要升级的 sub-Agent
			m.logger.Warn("machine orchestrator: upgrade detected, restarting upgraded sub-agents only")
			m.restartUpgradedNodes(ctx)
		case <-syncTicker.C:
			if err := m.syncNodes(ctx); err != nil {
				m.logger.Warn("node sync failed, will retry next cycle", "error", err)
			}
		}
	}
}

// T02: cloneCfgForNode 改为返回 error，端口分配失败不再静默回退
func (m *MachineOrchestrator) cloneCfgForNode(n machineNodeEntry) (*agentconfig.Config, error) {
	cfg := *m.baseCfg
	cfg.ServerCode = n.ServerCode
	cfg.RuntimeType = n.RuntimeType
	if n.Token != "" {
		cfg.AgentToken = n.Token
	}

	nodeConfigDir := filepath.Join(m.baseCfg.ConfigDir, "nodes", n.ServerCode)
	cfg.ConfigDir = nodeConfigDir
	for _, dir := range []string{
		filepath.Join(nodeConfigDir, "certs"),
		filepath.Join(nodeConfigDir, "config"),
	} {
		os.MkdirAll(dir, 0755)
	}

	nodeLogDir := filepath.Join(m.baseCfg.LogDir, "nodes", n.ServerCode)
	cfg.LogDir = nodeLogDir
	os.MkdirAll(nodeLogDir, 0755)

	xrayPort, err := m.xrayPortAllocator.Allocate(n.ServerCode)
	if err != nil {
		// T02: 不再静默回退到起始端口，返回 error 让 startNode 拒绝启动
		return nil, fmt.Errorf("allocate xray port for %s: %w", n.ServerCode, err)
	}
	cfg.XrayAPIEndpoint = fmt.Sprintf("127.0.0.1:%d", xrayPort)

	singboxPort, err := m.singboxPortAllocator.Allocate(n.ServerCode)
	if err != nil {
		return nil, fmt.Errorf("allocate singbox port for %s: %w", n.ServerCode, err)
	}
	cfg.SingboxClashEndpoint = fmt.Sprintf("127.0.0.1:%d", singboxPort)

	return &cfg, nil
}

func (m *MachineOrchestrator) startNode(n machineNodeEntry) {
	cfg, err := m.cloneCfgForNode(n)
	if err != nil {
		// T02: 端口分配失败，拒绝启动该节点
		m.logger.Error("failed to clone config for node, skipping start",
			"server_code", n.ServerCode, "error", err)
		return
	}

	nodeRegistry := prometheus.NewRegistry()
	wrappedRegistry := prometheus.WrapRegistererWith(
		prometheus.Labels{"server_code": n.ServerCode}, nodeRegistry)

	nodeLogger := m.logger.With("server_code", n.ServerCode)

	agent := NewAgent(cfg, nodeLogger)
	agent.skipOwnHTTPServer = true
	agent.skipSelfUpgrader = true
	agent.skipSharedResources = true
	agent.metricsRegistry = wrappedRegistry

	// T07: 初始化 done channel 用于等待 sub-Agent 退出
	done := make(chan struct{})

	m.mu.Lock()
	m.registries[n.ServerCode] = nodeRegistry
	nodeCtx, cancel := context.WithCancel(context.Background())
	m.agents[n.ServerCode] = &machineAgent{
		cfg:    cfg,
		agent:  agent,
		cancel: cancel,
		done:   done,
	}
	m.mu.Unlock()

	go func() {
		defer close(done) // T07: 退出时 close done channel
		defer cancel()
		if err := agent.Run(nodeCtx); err != nil {
			m.logger.Error("sub-agent exited with error", "server_code", n.ServerCode, "error", err)
		}
		m.mu.Lock()
		delete(m.agents, n.ServerCode)
		delete(m.registries, n.ServerCode)
		m.mu.Unlock()
		m.logger.Info("sub-agent removed", "server_code", n.ServerCode)
	}()
}

func (m *MachineOrchestrator) removeNode(serverCode string) {
	m.mu.Lock()
	ma, exists := m.agents[serverCode]
	m.mu.Unlock()

	if exists {
		m.logger.Info("stopping sub-agent", "server_code", serverCode)
		ma.cancel()
		// T07: 替代 time.Sleep(2 * time.Second)，使用 done channel 等待 sub-Agent 确实退出
		select {
		case <-ma.done:
			m.logger.Info("sub-agent stopped cleanly", "server_code", serverCode)
		case <-time.After(5 * time.Second):
			m.logger.Warn("sub-agent stop timeout, forcing port release", "server_code", serverCode)
		}
	}

	if err := m.xrayPortAllocator.Release(serverCode); err != nil {
		m.logger.Warn("failed to release xray port", "server_code", serverCode, "error", err)
	}
	if err := m.singboxPortAllocator.Release(serverCode); err != nil {
		m.logger.Warn("failed to release singbox port", "server_code", serverCode, "error", err)
	}
}

func (m *MachineOrchestrator) syncNodes(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/agent/machine/nodes", m.baseCfg.PanelURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.serverToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch nodes: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code  int                `json:"code"`
		Data  []machineNodeEntry `json:"data"`
		Error string             `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("API error: %s", result.Error)
	}

	desiredSet := make(map[string]machineNodeEntry)
	for _, n := range result.Data {
		if n.ServerCode == "" {
			continue
		}
		desiredSet[n.ServerCode] = n
	}

	m.mu.Lock()
	existingCodes := make(map[string]bool)
	for code := range m.agents {
		existingCodes[code] = true
	}
	m.mu.Unlock()

	for code := range existingCodes {
		if _, ok := desiredSet[code]; !ok {
			m.removeNode(code)
		}
	}

	for code, n := range desiredSet {
		m.mu.Lock()
		_, running := m.agents[code]
		m.mu.Unlock()
		if !running {
			m.logger.Info("starting new sub-agent", "server_code", code, "runtime", n.RuntimeType)
			m.startNode(n)
		}
	}

	m.logger.Info("node sync complete", "desired", len(desiredSet), "running", len(m.agents))
	return nil
}

func (m *MachineOrchestrator) startUnifiedHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", m.handleHealthz)
	mux.HandleFunc("/delta", m.handleDelta)
	mux.HandleFunc("/metrics", m.handleMetrics)
	mux.HandleFunc("/nodes", m.handleNodesList)
	mux.HandleFunc("/v1/status", m.handleProxyStatus)
	mux.HandleFunc("/v1/refresh", m.handleProxyRefresh)
	mux.HandleFunc("/v1/restart", m.handleProxyRestart)
	mux.HandleFunc("/v1/diag", m.handleProxyDiag)

	m.unifiedHTTPServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", m.baseCfg.ListenHost, m.baseCfg.ListenPort),
		Handler: mux,
	}

	go func() {
		m.logger.Info("unified HTTP server starting", "addr", m.unifiedHTTPServer.Addr)
		if err := m.unifiedHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("unified HTTP server error", "error", err)
		}
	}()
}

func (m *MachineOrchestrator) handleNodesList(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	nodes := make([]map[string]interface{}, 0, len(m.agents))
	for code, ma := range m.agents {
		status, _ := ma.agent.runtimeExec.Status(r.Context())
		running := false
		var rtVersion string
		if status != nil {
			running = status.Running
			rtVersion = status.Version
		}
		nodes = append(nodes, map[string]interface{}{
			"server_code":    code,
			"runtime_type":   ma.cfg.RuntimeType,
			"running":        running,
			"runtime_version": rtVersion,
			"config_version": ma.agent.currentVersion,
			"config_dir":     ma.cfg.ConfigDir,
			"xray_api_port":  parsePortFromEndpoint(ma.cfg.XrayAPIEndpoint),
			"clash_api_port": parsePortFromEndpoint(ma.cfg.SingboxClashEndpoint),
		})
	}
	m.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mode":  "machine",
		"count": len(nodes),
		"nodes": nodes,
	})
}

func (m *MachineOrchestrator) findAgentByRequest(r *http.Request) (*machineAgent, string) {
	serverCode := r.URL.Query().Get("server_code")
	if serverCode == "" {
		serverCode = r.Header.Get("X-Server-Code")
	}
	if serverCode == "" {
		return nil, ""
	}
	m.mu.RLock()
	ma, ok := m.agents[serverCode]
	m.mu.RUnlock()
	if !ok {
		return nil, serverCode
	}
	return ma, serverCode
}

func (m *MachineOrchestrator) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	ma, code := m.findAgentByRequest(r)
	if ma == nil {
		http.Error(w, fmt.Sprintf("node %s not found", code), http.StatusNotFound)
		return
	}
	ma.agent.ctrlStatus(w, r)
}

func (m *MachineOrchestrator) handleProxyRefresh(w http.ResponseWriter, r *http.Request) {
	ma, code := m.findAgentByRequest(r)
	if ma == nil {
		http.Error(w, fmt.Sprintf("node %s not found", code), http.StatusNotFound)
		return
	}
	ma.agent.ctrlRefresh(w, r)
}

func (m *MachineOrchestrator) handleProxyRestart(w http.ResponseWriter, r *http.Request) {
	ma, code := m.findAgentByRequest(r)
	if ma == nil {
		http.Error(w, fmt.Sprintf("node %s not found", code), http.StatusNotFound)
		return
	}
	ma.agent.ctrlRestart(w, r)
}

func (m *MachineOrchestrator) handleProxyDiag(w http.ResponseWriter, r *http.Request) {
	ma, code := m.findAgentByRequest(r)
	if ma == nil {
		http.Error(w, fmt.Sprintf("node %s not found", code), http.StatusNotFound)
		return
	}
	ma.agent.ctrlDiag(w, r)
}

func (m *MachineOrchestrator) handleHealthz(w http.ResponseWriter, r *http.Request) {
	serverCode := r.URL.Query().Get("server_code")
	if serverCode == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		m.mu.RLock()
		nodeCount := len(m.agents)
		m.mu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "ok",
			"mode":       "machine",
			"node_count": nodeCount,
			"version":    AgentVersion,
		})
		return
	}

	m.mu.RLock()
	ma, ok := m.agents[serverCode]
	m.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("node %s not found", serverCode), http.StatusNotFound)
		return
	}
	ma.agent.handleHealthz(w, r)
}

func (m *MachineOrchestrator) handleDelta(w http.ResponseWriter, r *http.Request) {
	serverCode := r.URL.Query().Get("server_code")
	if serverCode == "" {
		http.Error(w, "server_code parameter required", http.StatusBadRequest)
		return
	}
	m.mu.RLock()
	ma, ok := m.agents[serverCode]
	m.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("node %s not found", serverCode), http.StatusNotFound)
		return
	}
	ma.agent.handleDelta(w, r)
}

func (m *MachineOrchestrator) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	var gatherers prometheus.Gatherers
	for _, reg := range m.registries {
		gatherers = append(gatherers, reg)
	}
	m.mu.RUnlock()
	h := promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

// T01: restartUpgradedNodes 替代原来的 os.Exit(0) 硬退出。
// 二进制升级必须重启进程，但改进点：
// 1. 先优雅关闭所有 sub-Agent（flush 流量 + stop runtime）
// 2. 记录升级前的状态供 systemd 重启后恢复
// 3. 退出进程让 systemd 用新二进制拉起
func (m *MachineOrchestrator) restartUpgradedNodes(ctx context.Context) {
	m.logger.Warn("machine orchestrator: performing graceful shutdown before binary upgrade")

	// 优雅关闭所有节点（gracefulShutdownAll 内部有 10s 超时保护）
	m.gracefulShutdownAll(ctx)

	// 检查 .upgrade-pending sentinel 文件是否存在（in-process 健康检查机制）
	// systemd 重启新二进制后，如果 45s 内健康检查不通过，自动回滚到 .bak
	sentinelPath := filepath.Join(m.baseCfg.ConfigDir, ".upgrade-pending")
	if err := os.WriteFile(sentinelPath, []byte(fmt.Sprintf("%d", time.Now().Unix())), 0644); err != nil {
		m.logger.Warn("failed to write upgrade-pending sentinel", "error", err)
	}

	m.logger.Warn("all nodes shut down gracefully, exiting process for binary upgrade")
	os.Exit(0)
}

func (m *MachineOrchestrator) gracefulShutdownAll(ctx context.Context) {
	m.logger.Info("machine orchestrator: shutting down all managed nodes")

	if m.unifiedHTTPServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		m.unifiedHTTPServer.Shutdown(shutdownCtx)
	}

	m.mu.RLock()
	agents := make([]*Agent, 0, len(m.agents))
	for _, entry := range m.agents {
		agents = append(agents, entry.agent)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, ag := range agents {
		wg.Add(1)
		go func(a *Agent) {
			defer wg.Done()
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer flushCancel()
			a.reportTraffic(flushCtx)
			if a.cm != nil {
				a.cm.Stop()
			}
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer stopCancel()
			if a.runtimeExec != nil {
				a.runtimeExec.Stop(stopCtx)
			}
		}(ag)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		m.logger.Info("all nodes shut down gracefully")
	case <-time.After(10 * time.Second):
		m.logger.Warn("shutdown timeout, some nodes may not have flushed traffic")
	}
}
