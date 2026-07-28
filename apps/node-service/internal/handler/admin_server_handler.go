package handler

import (
	"context"
	"strconv"
	"time"

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
		server.Fail(c, config.ErrorCodeInternal, fmt.Sprintf("failed to restart kernel: %v", err))
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
		server.Fail(c, config.ErrorCodeInternal, fmt.Sprintf("failed to reload config: %v", err))
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
