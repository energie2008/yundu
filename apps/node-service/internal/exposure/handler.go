package exposure

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminExposureHandler struct {
	svc *ExposureService
}

func NewAdminExposureHandler(svc *ExposureService) *AdminExposureHandler {
	return &AdminExposureHandler{svc: svc}
}

func (h *AdminExposureHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	// 注意：路径参数为 :id（对应 server_id），与现有 server 路由前缀 /servers 一致
	servers := admin.Group("/servers")
	{
		servers.GET("/:id/exposure", rbac.RequirePermission("nodes.read"), h.GetExposure)
		servers.POST("/:id/exposure", rbac.RequirePermission("nodes.write"), h.CreateExposure)
		servers.PATCH("/:id/exposure", rbac.RequirePermission("nodes.write"), h.UpdateExposure)
		servers.DELETE("/:id/exposure", rbac.RequirePermission("nodes.write"), h.DeleteExposure)
		servers.POST("/:id/exposure/preview", rbac.RequirePermission("nodes.read"), h.Preview)
		servers.POST("/:id/exposure/validate", rbac.RequirePermission("nodes.read"), h.Validate)
		servers.POST("/:id/exposure/apply", rbac.RequirePermission("nodes.write"), h.Apply)
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

func (h *AdminExposureHandler) GetExposure(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewExposureResponse(e))
}

func (h *AdminExposureHandler) CreateExposure(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	var req CreateExposureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	// 路径参数优先级高于 body
	req.ServerID = serverID

	e, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewExposureResponse(e))
}

func (h *AdminExposureHandler) UpdateExposure(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	// 先按 server_id 找到 exposure
	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	var req UpdateExposureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), e.ID, &req)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewExposureResponse(updated))
}

func (h *AdminExposureHandler) DeleteExposure(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), e.ID); err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

func (h *AdminExposureHandler) Preview(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp, err := h.svc.Preview(c.Request.Context(), e.ID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

func (h *AdminExposureHandler) Validate(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	resp, err := h.svc.Validate(c.Request.Context(), e.ID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

func (h *AdminExposureHandler) Apply(c *gin.Context) {
	serverID, ok := parseServerID(c)
	if !ok {
		return
	}

	e, err := h.svc.GetByServerID(c.Request.Context(), serverID)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	// dry_run 查询参数：true 时仅预览不更新状态
	dryRun := c.Query("dry_run") == "true" || c.Query("dry-run") == "true"

	resp, err := h.svc.Apply(c.Request.Context(), e.ID, dryRun)
	if err != nil {
		code, msg := MapExposureErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

// ListExposures 提供独立列表查询（不在 server_id 路径下）
// 注意：该方法注册到 /admin/edge-exposures，本批未在 RegisterRoutesWithGroup 中暴露，
// 保留以便未来需要。当前所有 exposure 操作都挂在 /servers/:id/exposure 下。
func (h *AdminExposureHandler) ListExposures(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	items, total, err := h.svc.List(c.Request.Context(), page, pageSize, status)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	resp := make([]ExposureResponse, len(items))
	for i, e := range items {
		resp[i] = NewExposureResponse(e)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}
