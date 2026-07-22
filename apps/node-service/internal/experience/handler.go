package experience

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
	exp := admin.Group("/experience")
	{
		exp.GET("/scores", rbac.RequirePermission("nodes.read"), h.ListScores)
		exp.GET("/scores/:nodeId", rbac.RequirePermission("nodes.read"), h.GetScore)
		exp.GET("/scores/:nodeId/history", rbac.RequirePermission("nodes.read"), h.ListHistory)
		exp.POST("/recalculate", rbac.RequirePermission("nodes.write"), h.Recalculate)
		exp.GET("/config", rbac.RequirePermission("nodes.read"), h.GetConfig)
		exp.PUT("/config", rbac.RequirePermission("nodes.write"), h.UpdateConfig)
	}
}

// ListScores GET /admin/experience/scores
func (h *AdminHandler) ListScores(c *gin.Context) {
	q := &ScoreListQuery{
		Page:         atoiDefault(c.Query("page"), 1),
		PageSize:     atoiDefault(c.Query("page_size"), 50),
		Grade:        c.Query("grade"),
		OnlyIsolated: c.Query("only_isolated") == "true",
	}
	if nid := c.Query("node_id"); nid != "" {
		if id, err := uuid.Parse(nid); err == nil {
			q.NodeID = &id
		}
	}
	items, total, err := h.svc.ListCurrent(c.Request.Context(), q.NodeID, q.Grade, q.OnlyIsolated, q.Page, q.PageSize)
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

// GetScore GET /admin/experience/scores/:nodeId
func (h *AdminHandler) GetScore(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("nodeId"))
	if err != nil {
		server.BadRequest(c, "invalid node_id")
		return
	}
	items, _, err := h.svc.ListCurrent(c.Request.Context(), &nodeID, "", false, 1, 1)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if len(items) == 0 {
		server.NotFound(c, "experience score not found for this node")
		return
	}
	server.OK(c, items[0])
}

// ListHistory GET /admin/experience/scores/:nodeId/history
func (h *AdminHandler) ListHistory(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("nodeId"))
	if err != nil {
		server.BadRequest(c, "invalid node_id")
		return
	}
	limit := atoiDefault(c.Query("limit"), 200)
	items, err := h.svc.ListHistory(c.Request.Context(), nodeID, limit)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{
		"node_id": nodeID,
		"items":   items,
		"limit":   limit,
	})
}

// Recalculate POST /admin/experience/recalculate
func (h *AdminHandler) Recalculate(c *gin.Context) {
	go func() {
		if err := h.svc.CalculateAll(c.Request.Context()); err != nil {
			_ = err
		}
	}()
	server.OK(c, gin.H{"status": "started", "message": "recalculation started in background"})
}

// GetConfig GET /admin/experience/config
func (h *AdminHandler) GetConfig(c *gin.Context) {
	cfg, err := h.svc.GetConfig(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, cfg)
}

// UpdateConfig PUT /admin/experience/config
func (h *AdminHandler) UpdateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	cfg, err := h.svc.UpdateConfig(c.Request.Context(), &req)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, cfg)
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
