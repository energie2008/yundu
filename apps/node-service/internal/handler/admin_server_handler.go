package handler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/grpcserver"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// KernelRestarter 重启内核的抽象接口（由 aidiag.GRPCDispatcher 实现）
type KernelRestarter interface {
	RestartKernel(ctx context.Context, serverID uuid.UUID, reason string) error
	ReloadConfig(ctx context.Context, serverID uuid.UUID) error
}

type AdminServerHandler struct {
	serverService   *service.ServerService
	runtimeService  *service.RuntimeService
	tokenSalt       string
	panelURL        string
	logStore        *grpcserver.LogStore
	kernelRestarter KernelRestarter
}

func NewAdminServerHandler(serverService *service.ServerService, runtimeService *service.RuntimeService, tokenSalt, panelURL string, logStore *grpcserver.LogStore) *AdminServerHandler {
	return &AdminServerHandler{
		serverService:  serverService,
		runtimeService: runtimeService,
		tokenSalt:      tokenSalt,
		panelURL:       panelURL,
		logStore:       logStore,
	}
}

// SetKernelRestarter 注入内核重启器（在 app.go 中 gRPC server 启动后调用）
func (h *AdminServerHandler) SetKernelRestarter(r KernelRestarter) {
	h.kernelRestarter = r
}

func (h *AdminServerHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	servers := admin.Group("/servers")
	{
		servers.POST("", rbac.RequirePermission("nodes.write"), h.CreateServer)
		servers.GET("", rbac.RequirePermission("nodes.read"), h.ListServers)
		servers.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetServer)
		servers.GET("/:id/token", rbac.RequirePermission("nodes.read"), h.GetServerToken)
		servers.POST("/:id/runtimes", rbac.RequirePermission("nodes.write"), h.RegisterRuntime)
		servers.GET("/:id/runtimes", rbac.RequirePermission("nodes.read"), h.ListRuntimes)
		servers.GET("/:id/logs", rbac.RequirePermission("nodes.read"), h.GetServerLogs)
		servers.POST("/:id/restart-kernel", rbac.RequirePermission("nodes.write"), h.RestartKernel)
		servers.POST("/:id/reload-config", rbac.RequirePermission("nodes.write"), h.ReloadConfig)
		servers.GET("/health-dashboard", rbac.RequirePermission("nodes.read"), h.GetHealthDashboard)
	}
}

func (h *AdminServerHandler) CreateServer(c *gin.Context) {
	var req model.CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	srv, err := h.serverService.CreateServer(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	agentToken := pkg.GenerateAgentToken(srv.Code, h.tokenSalt)
	server.Created(c, model.NewServerDetailResponse(srv, agentToken, h.panelURL))
}

func (h *AdminServerHandler) ListServers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := model.ServerStatus(c.Query("status"))
	search := c.Query("search")

	servers, total, err := h.serverService.ListServers(c.Request.Context(), page, pageSize, status, search)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.ServerResponse, len(servers))
	for i, s := range servers {
		runtimes, _ := h.serverService.ListRuntimesByServer(c.Request.Context(), s.ID)
		nodeCount, _ := h.serverService.CountNodesByServer(c.Request.Context(), s.ID)
		items[i] = model.NewServerResponseWithDetails(s, runtimes, nodeCount)
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *AdminServerHandler) GetServer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	srv, err := h.serverService.GetServer(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	runtimes, _ := h.serverService.ListRuntimesByServer(c.Request.Context(), srv.ID)
	nodeCount, _ := h.serverService.CountNodesByServer(c.Request.Context(), srv.ID)
	resp := model.NewServerResponseWithDetails(srv, runtimes, nodeCount)

	// 填充关联节点列表（仅详情接口返回，列表接口不填以保持轻量）
	if nodes, err := h.serverService.ListNodesByServer(c.Request.Context(), srv.ID); err == nil {
		assocNodes := make([]model.AssociatedNodeInfo, len(nodes))
		for i, n := range nodes {
			assocNodes[i] = model.NewAssociatedNodeInfo(n)
		}
		resp.Nodes = assocNodes
	}

	server.OK(c, resp)
}

func (h *AdminServerHandler) GetServerToken(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	srv, err := h.serverService.GetServer(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	agentToken := pkg.GenerateAgentToken(srv.Code, h.tokenSalt)
	server.OK(c, model.NewServerDetailResponse(srv, agentToken, h.panelURL))
}

func (h *AdminServerHandler) RegisterRuntime(c *gin.Context) {
	idStr := c.Param("id")
	serverID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	var req model.RegisterRuntimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	srv, err := h.serverService.GetServer(c.Request.Context(), serverID)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	runtime, err := h.runtimeService.RegisterRuntimeByServerID(c.Request.Context(), srv.ID, &req)
	if err != nil {
		code, msg := service.MapRuntimeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewRuntimeResponse(runtime))
}

func (h *AdminServerHandler) ListRuntimes(c *gin.Context) {
	idStr := c.Param("id")
	serverID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	runtimes, err := h.runtimeService.ListRuntimes(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.RuntimeResponse, len(runtimes))
	for i, r := range runtimes {
		items[i] = model.NewRuntimeResponse(r)
	}

	server.OK(c, items)
}

// RestartKernel POST /admin/servers/:id/restart-kernel
// 通过 gRPC MaintenanceCommand 向 node-agent 下发 ACTION_RESTART 指令
func (h *AdminServerHandler) RestartKernel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	if h.kernelRestarter == nil {
		server.InternalError(c, "kernel restarter not available (gRPC server not initialized)")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "manual restart from admin panel"
	}
	if req.Reason == "" {
		req.Reason = "manual restart from admin panel"
	}

	if err := h.kernelRestarter.RestartKernel(c.Request.Context(), id, req.Reason); err != nil {
		server.Fail(c, config.CodeInternalError, fmt.Sprintf("failed to restart kernel: %v", err))
		return
	}

	server.OK(c, gin.H{"result": "restart command dispatched", "server_id": id})
}

// ReloadConfig POST /admin/servers/:id/reload-config
// 通过 gRPC MaintenanceCommand 向 node-agent 下发配置重新加载指令
func (h *AdminServerHandler) ReloadConfig(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	if h.kernelRestarter == nil {
		server.InternalError(c, "kernel restarter not available (gRPC server not initialized)")
		return
	}

	if err := h.kernelRestarter.ReloadConfig(c.Request.Context(), id); err != nil {
		server.Fail(c, config.CodeInternalError, fmt.Sprintf("failed to reload config: %v", err))
		return
	}

	server.OK(c, gin.H{"result": "reload config command dispatched", "server_id": id})
}

func (h *AdminServerHandler) GetServerLogs(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return
	}

	srv, err := h.serverService.GetServer(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	level := c.Query("level")
	sinceStr := c.DefaultQuery("since", "")
	var since time.Time
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	if h.logStore == nil {
		server.OK(c, gin.H{"logs": []interface{}{}, "total": 0})
		return
	}

	entries := h.logStore.QueryRaw(srv.Code, since, level, limit)
	logs := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		ts := time.UnixMilli(e.Timestamp).Format("2006-01-02 15:04:05")
		logs = append(logs, gin.H{
			"timestamp": ts,
			"level":     e.Level,
			"source":    e.Source,
			"message":   e.Message,
			"labels":    e.Labels,
		})
	}

	server.OK(c, gin.H{"logs": logs, "total": len(logs)})
}

// GetHealthDashboard GET /admin/servers/health-dashboard
// P3-3: 节点健康仪表盘 — 聚合所有服务器的健康状态，包括：
// agent 版本、在线状态、内核状态、配置版本、心跳时间、系统指标。
// 前端用于展示全局健康概览和异常告警。
func (h *AdminServerHandler) GetHealthDashboard(c *gin.Context) {
	ctx := c.Request.Context()

	// 获取所有服务器（不分页）
	servers, _, err := h.serverService.ListServers(ctx, 1, 1000, "", "")
	if err != nil {
		server.InternalError(c, "")
		return
	}

	type ServerHealth struct {
		ID              uuid.UUID  `json:"id"`
		Code            string     `json:"code"`
		Name            string     `json:"name"`
		Host            string     `json:"host"`
		Status          string     `json:"status"`
		Online          bool       `json:"online"`
		AgentVersion    string     `json:"agent_version"`
		LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
		NodeCount       int        `json:"node_count"`
		OnlineUsers     int        `json:"online_users"`
		CPUPercent      float64    `json:"cpu_percent"`
		MemPercent      float64    `json:"mem_percent"`
		DiskPercent     float64    `json:"disk_percent"`
		UptimeSeconds   int64      `json:"uptime_seconds"`
		Runtimes        []gin.H    `json:"runtimes"`
		// 告警标记
		Alerts []string `json:"alerts,omitempty"`
	}

	dashboard := make([]ServerHealth, 0, len(servers))
	now := time.Now()

	for _, s := range servers {
		runtimes, _ := h.serverService.ListRuntimesByServer(ctx, s.ID)
		nodeCount, _ := h.serverService.CountNodesByServer(ctx, s.ID)

		sh := ServerHealth{
			ID:        s.ID,
			Code:      s.Code,
			Name:      s.Name,
			Host:      s.Host,
			Status:    string(s.Status),
			NodeCount: nodeCount,
			Alerts:    []string{},
		}

		// 判断在线状态：心跳在 60s 内视为在线
		if s.LastHeartbeatAt != nil {
			sh.LastHeartbeatAt = s.LastHeartbeatAt
			if now.Sub(*s.LastHeartbeatAt) <= 60*time.Second {
				sh.Online = true
			} else {
				sh.Online = false
				sh.Alerts = append(sh.Alerts, "heartbeat_stale")
			}
		} else {
			sh.Online = false
			sh.Alerts = append(sh.Alerts, "no_heartbeat")
		}

		// 提取 agent 版本和系统指标（从 server.Metadata["system"] 读取，与 NewServerResponseWithDetails 一致）
		if s.Metadata != nil {
			if v, ok := s.Metadata["agent_version"].(string); ok {
				sh.AgentVersion = v
			}
			if sys, ok := s.Metadata["system"].(map[string]interface{}); ok && sys != nil {
				sh.CPUPercent = toFloat64(sys["cpu_percent"])
				sh.MemPercent = toFloat64(sys["mem_percent"])
				sh.DiskPercent = toFloat64(sys["disk_percent"])
				sh.UptimeSeconds = toInt64(sys["uptime_seconds"])
				if ou, ok := sys["online_users"]; ok {
					sh.OnlineUsers = int(toInt64(ou))
				}

				// 磁盘告警
				if sh.DiskPercent > 85 {
					sh.Alerts = append(sh.Alerts, "disk_high")
				}
				// CPU 告警
				if sh.CPUPercent > 90 {
					sh.Alerts = append(sh.Alerts, "cpu_high")
				}
			}
		}

		// 内核状态（从 runtime.Metadata 读取 restart_count / uptime_seconds）
		rtList := make([]gin.H, 0, len(runtimes))
		for _, rt := range runtimes {
			var restartCount int
			var uptimeSeconds int64
			if rt.Metadata != nil {
				if rc, ok := rt.Metadata["restart_count"]; ok {
					restartCount = int(toInt64(rc))
				}
				if ut, ok := rt.Metadata["uptime_seconds"]; ok {
					uptimeSeconds = toInt64(ut)
				}
			}
			rtInfo := gin.H{
				"type":           rt.RuntimeType,
				"status":         rt.Status,
				"restart_count":  restartCount,
				"uptime_seconds": uptimeSeconds,
			}
			if rt.LastHeartbeatAt != nil {
				rtInfo["last_heartbeat_at"] = rt.LastHeartbeatAt
			}
			rtList = append(rtList, rtInfo)

			// 内核异常告警
			if restartCount > 5 {
				sh.Alerts = append(sh.Alerts, "kernel_restart_frequent")
			}
		}
		sh.Runtimes = rtList

		// 服务器未激活告警
		if s.Status == "offline" || s.Status == "retired" {
			sh.Alerts = append(sh.Alerts, "server_"+string(s.Status))
		}

		if len(sh.Alerts) == 0 {
			sh.Alerts = nil
		}

		dashboard = append(dashboard, sh)
	}

	// 汇总统计
	totalServers := len(dashboard)
	onlineCount := 0
	alertCount := 0
	for _, d := range dashboard {
		if d.Online {
			onlineCount++
		}
		if len(d.Alerts) > 0 {
			alertCount++
		}
	}

	server.OK(c, gin.H{
		"summary": gin.H{
			"total_servers": totalServers,
			"online":        onlineCount,
			"offline":       totalServers - onlineCount,
			"alert_count":   alertCount,
		},
		"servers": dashboard,
	})
}

// toFloat64 安全地将 interface{} 转为 float64（兼容 JSON number / int / int64）。
// 用于从 server.Metadata["system"] 读取系统指标。
func toFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	}
	return 0
}

// toInt64 安全地将 interface{} 转为 int64（兼容 JSON number / int / int64 / string）。
// 用于从 runtime.Metadata 读取 restart_count / uptime_seconds 等指标。
func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case int32:
		return int64(x)
	case string:
		var n int64
		fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}
