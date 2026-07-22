// control.go 实现 Agent 本地 Unix Socket 控制服务，作为 yunductl CLI 的入口。
//
// 补齐零SSH最后一公里：运维人员可通过 yunductl（或直接 curl --unix-socket）
// 执行 status / refresh / rollback / restart / diag / nodes / upgrade 操作，
// 无需 SSH 进入节点。
//
// 安全：Unix Socket 文件权限 0600（仅 root 可读写），无网络端口暴露。
// 路径：/run/yundu/agent.sock
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

// ControlSocketPath 是 Agent 本地控制服务的 Unix Socket 路径。
// yunductl CLI 通过此路径与 Agent 通信。
const ControlSocketPath = "/run/yundu/agent.sock"

// runControlServer 启动 Unix Socket 控制服务（yunductl 的 Agent 侧入口）。
//
// 该服务在 Agent.Run 中以独立 goroutine 启动，ctx 取消时自动关闭。
// 所有端点均返回 JSON 格式响应。
//
// 端点列表：
//   - GET  /status   节点状态快照（版本/配置版本/运行时状态/通道）
//   - POST /refresh  强制从面板拉取最新配置
//   - POST /rollback 回滚到 LKG 配置
//   - POST /restart  重启内核（不重启 agent）
//   - GET  /diag     诊断信息（xray gRPC 可达性/配置目录/版本）
//   - GET  /nodes    节点列表（node 模式返回单节点信息）
//   - POST /upgrade  触发自升级检查
func (a *Agent) runControlServer(ctx context.Context) {
	// Windows 不支持 Unix Socket，跳过控制服务启动
	if runtime.GOOS == "windows" {
		a.logger.Debug("control server disabled on windows (no unix socket)")
		return
	}

	if err := os.MkdirAll("/run/yundu", 0700); err != nil {
		a.logger.Warn("control server: failed to create socket dir", "error", err)
		return
	}
	// 清理可能残留的旧 socket 文件
	os.Remove(ControlSocketPath)

	listener, err := net.Listen("unix", ControlSocketPath)
	if err != nil {
		a.logger.Error("control server: failed to listen on unix socket",
			"path", ControlSocketPath, "error", err)
		return
	}
	defer os.Remove(ControlSocketPath)

	// 仅 root 可读写（权限与 SSH 密钥同级，防止普通用户操控 agent）
	if err := os.Chmod(ControlSocketPath, 0600); err != nil {
		a.logger.Warn("control server: failed to set socket permissions", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", a.ctrlStatus)
	mux.HandleFunc("/refresh", a.ctrlRefresh)
	mux.HandleFunc("/rollback", a.ctrlRollback)
	mux.HandleFunc("/restart", a.ctrlRestart)
	mux.HandleFunc("/diag", a.ctrlDiag)
	mux.HandleFunc("/nodes", a.ctrlNodes)
	mux.HandleFunc("/upgrade", a.ctrlUpgrade)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		srv.Close()
		listener.Close()
	}()

	a.logger.Info("control server listening", "path", ControlSocketPath)
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		a.logger.Error("control server stopped with error", "error", err)
	}
}

// ctrlStatus GET /status — 节点状态快照
func (a *Agent) ctrlStatus(w http.ResponseWriter, r *http.Request) {
	status, _ := a.runtimeExec.Status(r.Context())
	running := status != nil && status.Running
	runtimeState := "stopped"
	if running {
		runtimeState = "running"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version":        AgentVersion,
		"config_version": a.currentVersion,
		"runtime_state":  runtimeState,
		"runtime_type":   a.cfg.RuntimeType,
		"use_native":     a.useNative,
		"channel":        a.cm.GetHealthStatus().ActiveChannel,
	})
}

// ctrlRefresh POST /refresh — 强制从面板拉取最新配置
func (a *Agent) ctrlRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	go a.applyConfig(context.Background(), "force", &a.currentVersion)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "refresh triggered"})
}

// ctrlRollback POST /rollback — 回滚到 LKG 配置
func (a *Agent) ctrlRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	configPath := a.cfg.ConfigFilePath()
	if !a.pipeline.HasLKG(a.cfg.RuntimeType) {
		http.Error(w, `{"error":"no LKG available"}`, http.StatusConflict)
		return
	}
	if err := a.pipeline.RestoreLKG(configPath, a.cfg.RuntimeType); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	// 触发 reload 使 LKG 生效
	go a.runtimeExec.Reload(context.Background(), configPath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "rollback triggered, reloading..."})
}

// ctrlRestart POST /restart — 重启内核（不重启 agent）
func (a *Agent) ctrlRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	configPath := a.cfg.ConfigFilePath()
	go func() {
		if err := a.runtimeExec.Stop(context.Background()); err != nil {
			a.logger.Error("ctrl restart: stop failed", "error", err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := a.runtimeExec.Reload(context.Background(), configPath); err != nil {
			a.logger.Error("ctrl restart: reload failed", "error", err)
		}
		a.maybeRestartSingbox(context.Background())
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "restart triggered"})
}

// ctrlDiag GET /diag — 诊断信息（端口/gRPC/证书）
func (a *Agent) ctrlDiag(w http.ResponseWriter, r *http.Request) {
	diag := map[string]interface{}{
		"xray_grpc_10085": testDialTCP("127.0.0.1:10085"),
		"runtime_type":    a.cfg.RuntimeType,
		"config_dir":      a.cfg.ConfigDir,
		"config_version":  a.currentVersion,
		"use_native":      a.useNative,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(diag)
}

// ctrlNodes GET /nodes — Machine 模式节点列表（node 模式返回单节点信息）
func (a *Agent) ctrlNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mode":        "node",
		"server_code": a.cfg.ServerCode,
		"version":     a.currentVersion,
	})
}

// ctrlUpgrade POST /upgrade — 触发自升级检查
func (a *Agent) ctrlUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	if a.selfUpgrader == nil {
		http.Error(w, `{"error":"self-upgrader not available (non-native mode)"}`, http.StatusServiceUnavailable)
		return
	}
	go func() {
		if err := a.selfUpgrader.CheckNow(context.Background()); err != nil {
			a.logger.Warn("ctrl upgrade: check failed", "error", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "upgrade check triggered"})
}

// testDialTCP 测试 TCP 端口可达性（用于 /diag 诊断 xray gRPC API 是否在线）。
func testDialTCP(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
