package upgrade

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminUpgradeHandler 处理 runtime 升级任务的 admin 路由
type AdminUpgradeHandler struct {
	svc *UpgradeService
}

func NewAdminUpgradeHandler(svc *UpgradeService) *AdminUpgradeHandler {
	return &AdminUpgradeHandler{svc: svc}
}

func (h *AdminUpgradeHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	servers := admin.Group("/servers")
	{
		servers.POST("/:id/runtime-upgrade", rbac.RequirePermission("nodes.write"), h.CreateUpgrade)
		servers.GET("/:id/runtime-upgrades", rbac.RequirePermission("nodes.read"), h.ListByServer)
	}

	upgrades := admin.Group("/runtime-upgrades")
	{
		upgrades.POST("/batch", rbac.RequirePermission("nodes.write"), h.CreateBatchUpgrade)
		upgrades.POST("/canary", rbac.RequirePermission("nodes.write"), h.CreateCanaryUpgrade)
		upgrades.GET("/:taskId", rbac.RequirePermission("nodes.read"), h.GetTask)
		upgrades.POST("/:taskId/rollback", rbac.RequirePermission("nodes.write"), h.Rollback)
	}
}

func parseServerID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid server id")
		return uuid.Nil, false
	}
	return id, true
}

func parseTaskID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("taskId"))
	if err != nil {
		server.BadRequest(c, "invalid task id")
		return uuid.Nil, false
	}
	return id, true
}

// CreateUpgrade POST /admin/servers/:id/runtime-upgrade
func (h *AdminUpgradeHandler) CreateUpgrade(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	task, err := h.svc.CreateUpgrade(c.Request.Context(), serverID, &req)
	if err != nil {
		code, msg := MapUpgradeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewResponse(task))
}

// CreateBatchUpgrade POST /admin/runtime-upgrades/batch
func (h *AdminUpgradeHandler) CreateBatchUpgrade(c *gin.Context) {
	var req BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	tasks, err := h.svc.CreateBatchUpgrade(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapUpgradeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	items := make([]Response, len(tasks))
	for i, t := range tasks {
		items[i] = NewResponse(t)
	}
	server.OK(c, gin.H{
		"batch_id": tasks[0].BatchID,
		"items":    items,
		"total":    len(items),
	})
}

// CreateCanaryUpgrade POST /admin/runtime-upgrades/canary
func (h *AdminUpgradeHandler) CreateCanaryUpgrade(c *gin.Context) {
	var req CanaryUpgradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	tasks, err := h.svc.CreateCanaryUpgrade(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapUpgradeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	items := make([]Response, len(tasks))
	for i, t := range tasks {
		items[i] = NewResponse(t)
	}
	server.OK(c, gin.H{
		"items": items,
		"total": len(items),
	})
}

// ListByServer GET /admin/servers/:id/runtime-upgrades
func (h *AdminUpgradeHandler) ListByServer(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tasks, err := h.svc.ListByServer(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]Response, len(tasks))
	for i, t := range tasks {
		items[i] = NewResponse(t)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     len(items),
		"items":     items,
	})
}

// GetTask GET /admin/runtime-upgrades/:taskId
func (h *AdminUpgradeHandler) GetTask(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, err := h.svc.GetByID(c.Request.Context(), taskID)
	if err != nil {
		code, msg := MapUpgradeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewResponse(task))
}

// Rollback POST /admin/runtime-upgrades/:taskId/rollback
func (h *AdminUpgradeHandler) Rollback(c *gin.Context) {
	taskID, ok := parseTaskID(c)
	if !ok {
		return
	}

	task, err := h.svc.Rollback(c.Request.Context(), taskID)
	if err != nil {
		code, msg := MapUpgradeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewResponse(task))
}
