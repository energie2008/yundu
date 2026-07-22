package aidiag

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// Admin Handler
// ============================================================================

type AdminHandler struct {
	svc *Service
}

func NewAdminHandler(svc *Service) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	diag := admin.Group("/diagnosis")
	{
		diag.POST("/sessions", rbac.RequirePermission("nodes.write"), h.CreateSession)
		diag.GET("/sessions", rbac.RequirePermission("nodes.read"), h.ListSessions)
		diag.GET("/sessions/:id", rbac.RequirePermission("nodes.read"), h.GetSession)
		diag.POST("/sessions/:id/autofix", rbac.RequirePermission("nodes.write"), h.ApplyAutofix)
		diag.GET("/knowledge", rbac.RequirePermission("nodes.read"), h.ListKnowledge)
	}
}

// CreateSession POST /admin/diagnosis/sessions
func (h *AdminHandler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	// 从上下文获取管理员 ID（可选）
	var adminID *uuid.UUID
	if v, exists := c.Get("admin_id"); exists {
		if id, ok := v.(uuid.UUID); ok {
			adminID = &id
		}
	}

	session, err := h.svc.CreateSession(c.Request.Context(), &req, adminID)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, session)
}

// ListSessions GET /admin/diagnosis/sessions
func (h *AdminHandler) ListSessions(c *gin.Context) {
	q := &ListSessionsQuery{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 20),
		Status:   c.Query("status"),
	}
	if nid := c.Query("node_id"); nid != "" {
		if id, err := uuid.Parse(nid); err == nil {
			q.NodeID = &id
		}
	}
	if sid := c.Query("server_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			q.ServerID = &id
		}
	}
	items, total, err := h.svc.ListSessions(c.Request.Context(), q)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{
		"page":      q.Page,
		"page_size": q.PageSize,
		"total":     total,
		"items":     items,
	})
}

// GetSession GET /admin/diagnosis/sessions/:id
func (h *AdminHandler) GetSession(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid session id")
		return
	}
	session, err := h.svc.GetSession(c.Request.Context(), id)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, session)
}

// ApplyAutofix POST /admin/diagnosis/sessions/:id/autofix
func (h *AdminHandler) ApplyAutofix(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid session id")
		return
	}
	var req ApplyAutofixRequest
	_ = c.ShouldBindJSON(&req)
	if req.SuggestionIndex < 0 {
		req.SuggestionIndex = 0
	}
	session, err := h.svc.ApplyAutofix(c.Request.Context(), id, req.SuggestionIndex)
	if err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	server.OK(c, session)
}

// ListKnowledge GET /admin/diagnosis/knowledge
func (h *AdminHandler) ListKnowledge(c *gin.Context) {
	category := c.Query("category")
	onlyVerified := c.Query("verified") == "true"
	page := atoiDefault(c.Query("page"), 1)
	pageSize := atoiDefault(c.Query("page_size"), 20)
	items, total, err := h.svc.ListKnowledge(c.Request.Context(), category, onlyVerified, page, pageSize)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
