package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminTemplateHandler 管理端订阅模板 handler（按名称索引的 SubscribeTemplate）。
//
// 端点（注册于 /api/v1/admin/subscribe/templates）：
//
//	GET    /subscribe/templates        - 列出所有模板
//	PUT    /subscribe/templates/:id    - 更新模板内容
//	POST   /subscribe/templates/reload - 重新加载缓存
//
// 与 SubscriptionHandler 的 /admin/subscription/templates（按 code 索引）互补：
// 本组面向渲染器按内核/格式名直接取模板内容的场景（对齐 Xboard subscribe_template helper）。
type AdminTemplateHandler struct {
	svc *service.TemplateService
}

func NewAdminTemplateHandler(svc *service.TemplateService) *AdminTemplateHandler {
	return &AdminTemplateHandler{svc: svc}
}

// ListSubscribeTemplates GET /admin/subscribe/templates
// 列出所有订阅模板（含禁用项与内置标记），按名称排序。
func (h *AdminTemplateHandler) ListSubscribeTemplates(c *gin.Context) {
	templates, err := h.svc.ListTemplates(c.Request.Context())
	if err != nil {
		server.InternalError(c, "failed to list subscribe templates")
		return
	}
	server.OK(c, gin.H{
		"total": len(templates),
		"items": templates,
	})
}

// UpdateSubscribeTemplate PUT /admin/subscribe/templates/:id
// 更新模板内容。内置模板允许编辑内容，不可删除或改名。
func (h *AdminTemplateHandler) UpdateSubscribeTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid template id")
		return
	}

	var req model.SubscribeTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	updated, err := h.svc.UpdateTemplate(c.Request.Context(), id, req.Content, req.Enabled)
	if err != nil {
		if err == service.ErrSubscribeTemplateNotFound {
			server.NotFound(c, "subscribe template not found")
			return
		}
		server.InternalError(c, "failed to update subscribe template")
		return
	}
	server.OK(c, updated)
}

// ReloadSubscribeTemplateCache POST /admin/subscribe/templates/reload
// 全量重新加载模板缓存。适用于批量修改模板后刷新。
func (h *AdminTemplateHandler) ReloadSubscribeTemplateCache(c *gin.Context) {
	if err := h.svc.ReloadCache(c.Request.Context()); err != nil {
		server.InternalError(c, "failed to reload subscribe template cache")
		return
	}
	server.OK(c, gin.H{"status": "ok", "message": "subscribe template cache reloaded"})
}

// RegisterAdminRoutes 注册到 admin 组（已包含 AdminAuth 中间件）。
func (h *AdminTemplateHandler) RegisterAdminRoutes(admin *gin.RouterGroup) {
	g := admin.Group("/subscribe/templates")
	{
		g.GET("", h.ListSubscribeTemplates)
		g.PUT("/:id", h.UpdateSubscribeTemplate)
		g.POST("/reload", h.ReloadSubscribeTemplateCache)
	}
}
