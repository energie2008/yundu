package compat

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"
)

// AdminCompatHandler 兼容矩阵管理 handler
type AdminCompatHandler struct {
	svc *CompatService
}

func NewAdminCompatHandler(svc *CompatService) *AdminCompatHandler {
	return &AdminCompatHandler{svc: svc}
}

// ListClientProfiles GET /admin/client-profiles
func (h *AdminCompatHandler) ListClientProfiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	code := c.Query("code")

	profiles, total, err := h.svc.ListClientProfiles(c.Request.Context(), ClientProfileListQuery{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
		Code:     code,
	})
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]ClientProfileResponse, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, NewClientProfileResponse(p))
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

// ListCompatMatrix GET /admin/client-compat-matrix
func (h *AdminCompatHandler) ListCompatMatrix(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	clientCode := c.Query("client_code")
	featureCode := c.Query("feature_code")

	entries, total, err := h.svc.ListCompatMatrix(c.Request.Context(), CompatMatrixListQuery{
		Page:        page,
		PageSize:    pageSize,
		ClientCode:  clientCode,
		FeatureCode: featureCode,
	})
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]CompatMatrixEntryResponse, 0, len(entries))
	for _, e := range entries {
		items = append(items, NewCompatMatrixEntryResponse(e))
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

// BatchUpdateCompatMatrix PATCH /admin/client-compat-matrix
func (h *AdminCompatHandler) BatchUpdateCompatMatrix(c *gin.Context) {
	var req CompatMatrixBatchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	updated, err := h.svc.BatchUpdateMatrix(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapCompatErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, CompatMatrixBatchUpdateResponse{Updated: updated})
}

// SyncCompatMatrix POST /admin/client-compat-matrix/sync
func (h *AdminCompatHandler) SyncCompatMatrix(c *gin.Context) {
	resp, err := h.svc.SyncFromSource(c.Request.Context())
	if err != nil {
		code, msg := MapCompatErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

// RegisterAdminRoutes 注册到 admin 组（已包含 AdminAuth 中间件）
func (h *AdminCompatHandler) RegisterAdminRoutes(admin *gin.RouterGroup) {
	admin.GET("/client-profiles", h.ListClientProfiles)
	admin.GET("/client-compat-matrix", h.ListCompatMatrix)
	admin.PATCH("/client-compat-matrix", h.BatchUpdateCompatMatrix)
	admin.POST("/client-compat-matrix/sync", h.SyncCompatMatrix)
}
