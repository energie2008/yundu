package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminDeploymentHandler struct {
	deploymentService *service.DeploymentService
}

func NewAdminDeploymentHandler(deploymentService *service.DeploymentService) *AdminDeploymentHandler {
	return &AdminDeploymentHandler{
		deploymentService: deploymentService,
	}
}

func (h *AdminDeploymentHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	deployments := admin.Group("/deployments")
	{
		deployments.POST("/dry-run", rbac.RequirePermission("nodes.write"), h.DryRun)
		deployments.POST("/precheck", rbac.RequirePermission("nodes.read"), h.Precheck)
		deployments.POST("", rbac.RequirePermission("nodes.write"), h.Deploy)
		deployments.GET("", rbac.RequirePermission("nodes.read"), h.ListBatches)
		deployments.POST("/targets/:id/result", rbac.RequirePermission("nodes.write"), h.UpdateTargetResult)
		deployments.POST("/refresh", rbac.RequirePermission("nodes.write"), h.RefreshConfig)
		// P0-8: 用户封禁通知推送到所有 agent
		deployments.POST("/user-ban-notify", rbac.RequirePermission("nodes.write"), h.NotifyUserBan)
		// P1-8: 发布与回滚 API
		deployments.POST("/publish", rbac.RequirePermission("nodes.write"), h.Publish)
		deployments.POST("/:id/rollback", rbac.RequirePermission("nodes.write"), h.Rollback)
		deployments.GET("/:id/diff", rbac.RequirePermission("nodes.read"), h.GetBatchDiff)
		deployments.GET("/:id/results", rbac.RequirePermission("nodes.read"), h.GetBatchResults)
	}
}

// NotifyUserBan P0-8: 向所有已连接的 agent 推送用户封禁通知
func (h *AdminDeploymentHandler) NotifyUserBan(c *gin.Context) {
	var req struct {
		UserIDs []string `json:"user_ids" binding:"required"`
		Reason  string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	h.deploymentService.PushUserBanToAllServers(c.Request.Context(), req.UserIDs, req.Reason)
	server.OK(c, gin.H{
		"notified":  true,
		"user_count": len(req.UserIDs),
		"reason":    req.Reason,
	})
}

// RefreshConfig 强制刷新配置版本（从nodes表自动渲染，无需手动提供ContentJSON）
func (h *AdminDeploymentHandler) RefreshConfig(c *gin.Context) {
	var req struct {
		ScopeType string    `json:"scope_type" binding:"required"` // "node" 或 "runtime"
		ScopeID   uuid.UUID `json:"scope_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	var cv *model.ConfigVersion
	var err error
	switch req.ScopeType {
	case "node":
		cv, err = h.deploymentService.RefreshNodeConfig(c.Request.Context(), req.ScopeID)
	case "runtime":
		cv, err = h.deploymentService.RefreshRuntimeConfig(c.Request.Context(), req.ScopeID)
	default:
		server.BadRequest(c, "invalid scope_type, must be 'node' or 'runtime'")
		return
	}

	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"version_no":   cv.VersionNo,
		"content_hash": cv.ContentHash,
		"status":       cv.Status,
		"message":      "配置已刷新，节点将在下次心跳时自动拉取",
	})
}

func (h *AdminDeploymentHandler) Precheck(c *gin.Context) {
	var req struct {
		ScopeType string    `json:"scope_type" binding:"required"`
		ScopeID   uuid.UUID `json:"scope_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	result, err := h.deploymentService.PrecheckDeployment(c.Request.Context(), model.ScopeType(req.ScopeType), req.ScopeID)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, result)
}

func (h *AdminDeploymentHandler) DryRun(c *gin.Context) {
	var req model.DryRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)
	result, err := h.deploymentService.DryRun(c.Request.Context(), adminID, &req)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, result)
}

func (h *AdminDeploymentHandler) Deploy(c *gin.Context) {
	var req model.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)
	batch, targets, err := h.deploymentService.Deploy(c.Request.Context(), adminID, &req)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, gin.H{
		"batch":        batch,
		"targets":      targets,
		"target_count": len(targets),
	})
}

func (h *AdminDeploymentHandler) ListBatches(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := model.DeploymentStatus(c.Query("status"))
	scopeType := model.ScopeType(c.Query("scope_type"))

	batches, total, err := h.deploymentService.ListBatches(c.Request.Context(), page, pageSize, status, scopeType)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    batches,
	})
}

func (h *AdminDeploymentHandler) UpdateTargetResult(c *gin.Context) {
	idStr := c.Param("id")
	targetID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid target id")
		return
	}

	var req model.UpdateDeploymentResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	req.TargetID = targetID

	if err := h.deploymentService.UpdateDeploymentResult(c.Request.Context(), targetID, &req); err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"status": "updated"})
}

// Publish P1-8: 正式发布配置（自动渲染 + 创建部署批次 + 推送）
func (h *AdminDeploymentHandler) Publish(c *gin.Context) {
	var req struct {
		ScopeType string    `json:"scope_type" binding:"required"` // "node" 或 "runtime"
		ScopeID   uuid.UUID `json:"scope_id" binding:"required"`
		Strategy  string    `json:"strategy"` // rolling/canary/all_at_once，默认 rolling
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)
	batch, targets, cv, err := h.deploymentService.Publish(
		c.Request.Context(), adminID,
		model.ScopeType(req.ScopeType), req.ScopeID, model.DeploymentStrategy(req.Strategy),
	)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, gin.H{
		"batch":         batch,
		"targets":       targets,
		"target_count":  len(targets),
		"config_version": cv,
	})
}

// Rollback P1-8: 回滚指定部署批次
func (h *AdminDeploymentHandler) Rollback(c *gin.Context) {
	idStr := c.Param("id")
	batchID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid batch id")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req) // reason 可选，不绑定也不报错

	if err := h.deploymentService.Rollback(c.Request.Context(), batchID, req.Reason); err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"batch_id": batchID,
		"status":   "rolled_back",
		"reason":   req.Reason,
	})
}

// GetBatchDiff P1-8: 获取部署批次的配置 diff
func (h *AdminDeploymentHandler) GetBatchDiff(c *gin.Context) {
	idStr := c.Param("id")
	batchID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid batch id")
		return
	}

	result, err := h.deploymentService.GetBatchDiff(c.Request.Context(), batchID)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, result)
}

// GetBatchResults P1-8: 获取部署批次的所有 target 结果
func (h *AdminDeploymentHandler) GetBatchResults(c *gin.Context) {
	idStr := c.Param("id")
	batchID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid batch id")
		return
	}

	result, err := h.deploymentService.GetBatchResults(c.Request.Context(), batchID)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, result)
}
