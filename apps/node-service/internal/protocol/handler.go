package protocol

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminProtocolHandler 处理 protocol_registry 的 admin 路由
type AdminProtocolHandler struct {
	svc *ProtocolService
}

func NewAdminProtocolHandler(svc *ProtocolService) *AdminProtocolHandler {
	return &AdminProtocolHandler{svc: svc}
}

func (h *AdminProtocolHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	registry := admin.Group("/protocol-registry")
	{
		registry.GET("", rbac.RequirePermission("nodes.read"), h.ListProtocols)
		registry.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetProtocol)
		registry.POST("", rbac.RequirePermission("nodes.write"), h.CreateProtocol)
		registry.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdateProtocol)
	}
}

func (h *AdminProtocolHandler) ListProtocols(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := ProtocolListQuery{
		Page:          page,
		PageSize:      pageSize,
		ProtocolType:  c.Query("protocol_type"),
		TransportType: c.Query("transport_type"),
		SecurityType:  c.Query("security_type"),
		IsEnabled:     c.Query("is_enabled"),
	}

	items, total, err := h.svc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]ProtocolResponse, len(items))
	for i, p := range items {
		resp[i] = NewProtocolResponse(p)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminProtocolHandler) GetProtocol(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid protocol id")
		return
	}

	p, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := MapProtocolErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewProtocolResponse(p))
}

func (h *AdminProtocolHandler) CreateProtocol(c *gin.Context) {
	var req CreateProtocolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapProtocolErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewProtocolResponse(p))
}

func (h *AdminProtocolHandler) UpdateProtocol(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid protocol id")
		return
	}

	var req UpdateProtocolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapProtocolErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewProtocolResponse(p))
}

// AdminTemplateHandler 处理 config_templates 的 admin 路由
type AdminTemplateHandler struct {
	svc *TemplateService
}

func NewAdminTemplateHandler(svc *TemplateService) *AdminTemplateHandler {
	return &AdminTemplateHandler{svc: svc}
}

func (h *AdminTemplateHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	templates := admin.Group("/config-templates")
	{
		templates.GET("", rbac.RequirePermission("nodes.read"), h.ListTemplates)
		templates.PUT("/:code", rbac.RequirePermission("nodes.write"), h.UpdateTemplate)
		templates.POST("/:code/render", rbac.RequirePermission("nodes.read"), h.RenderTemplate)
	}
}

func (h *AdminTemplateHandler) ListTemplates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := TemplateListQuery{
		Page:         page,
		PageSize:     pageSize,
		RuntimeType:  c.Query("runtime_type"),
		TemplateType: c.Query("template_type"),
	}

	items, total, err := h.svc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]TemplateResponse, len(items))
	for i, t := range items {
		resp[i] = NewTemplateResponse(t)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminTemplateHandler) UpdateTemplate(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		server.BadRequest(c, "invalid template code")
		return
	}

	var req UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	t, err := h.svc.Update(c.Request.Context(), code, &req)
	if err != nil {
		code, msg := MapProtocolErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewTemplateResponse(t))
}

func (h *AdminTemplateHandler) RenderTemplate(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		server.BadRequest(c, "invalid template code")
		return
	}

	var req RenderTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	resp, err := h.svc.Render(c.Request.Context(), code, req.Variables)
	if err != nil {
		code, msg := MapProtocolErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

type AdminPresetHandler struct {
	svc *PresetService
}

func NewAdminPresetHandler(svc *PresetService) *AdminPresetHandler {
	return &AdminPresetHandler{svc: svc}
}

func (h *AdminPresetHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	presets := admin.Group("/protocol-presets")
	{
		presets.GET("", rbac.RequirePermission("nodes.read"), h.ListPresets)
		presets.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetPreset)
		presets.POST("", rbac.RequirePermission("nodes.write"), h.CreatePreset)
		presets.POST("/:id/fork", rbac.RequirePermission("nodes.write"), h.ForkPreset)
		presets.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdatePreset)
		presets.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeletePreset)
	}
}

func (h *AdminPresetHandler) RegisterPublicRoutes(public *gin.RouterGroup) {
	public.GET("/protocol-presets", h.ListEnabledPresets)
}

func (h *AdminPresetHandler) ListPresets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := PresetListQuery{
		Page:          page,
		PageSize:      pageSize,
		ProtocolType:  c.Query("protocol_type"),
		TransportType: c.Query("transport_type"),
		SecurityType:  c.Query("security_type"),
		IsEnabled:     c.Query("is_enabled"),
		IsRecommended: c.Query("is_recommended"),
	}

	items, total, err := h.svc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]PresetResponse, len(items))
	for i, p := range items {
		resp[i] = NewPresetResponse(p)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminPresetHandler) ListEnabledPresets(c *gin.Context) {
	items, err := h.svc.ListEnabled(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]PresetResponse, len(items))
	for i, p := range items {
		resp[i] = NewPresetResponse(p)
	}
	server.OK(c, resp)
}

func (h *AdminPresetHandler) GetPreset(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid preset id")
		return
	}

	p, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := MapPresetErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPresetResponse(p))
}

func (h *AdminPresetHandler) CreatePreset(c *gin.Context) {
	var req CreatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapPresetErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPresetResponse(p))
}

// ForkPreset 复制内置预设为自定义预设（可编辑）
// 允许用户基于内置预设创建副本，解决内置预设不可编辑的问题
func (h *AdminPresetHandler) ForkPreset(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid preset id")
		return
	}

	var req ForkPresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 允许空 body，使用默认名称
		req = ForkPresetRequest{}
	}

	p, err := h.svc.ForkBuiltin(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapPresetErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPresetResponse(p))
}

func (h *AdminPresetHandler) UpdatePreset(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid preset id")
		return
	}

	var req UpdatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapPresetErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPresetResponse(p))
}

func (h *AdminPresetHandler) DeletePreset(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid preset id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		code, msg := MapPresetErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, nil)
}
