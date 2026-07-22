package channelhealth

import (
	"strconv"
	"time"

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
	channels := admin.Group("/channels")
	{
		channels.GET("/health", rbac.RequirePermission("nodes.read"), h.ListHealth)
		channels.GET("/health/:serverId", rbac.RequirePermission("nodes.read"), h.GetHealth)
		channels.GET("/health/:serverId/snapshots", rbac.RequirePermission("nodes.read"), h.ListSnapshots)
		channels.GET("/failover-events", rbac.RequirePermission("nodes.read"), h.ListFailoverEvents)
		channels.POST("/switch", rbac.RequirePermission("nodes.write"), h.ManualSwitch)
	}
}

// ListHealth GET /admin/channels/health
func (h *AdminHandler) ListHealth(c *gin.Context) {
	q := &ChannelHealthListQuery{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 50),
	}
	if sid := c.Query("server_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			q.ServerID = &id
		}
	}
	q.ChannelState = c.Query("channel_state")

	items, total, err := h.svc.ListCurrent(c.Request.Context(), q.ServerID, q.ChannelState, q.Page, q.PageSize)
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

// GetHealth GET /admin/channels/health/:serverId
func (h *AdminHandler) GetHealth(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("serverId"))
	if err != nil {
		server.BadRequest(c, "invalid server_id")
		return
	}
	items, _, err := h.svc.ListCurrent(c.Request.Context(), &serverID, "", 1, 1)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if len(items) == 0 {
		server.NotFound(c, "channel health not found for this server")
		return
	}
	server.OK(c, items[0])
}

// ListSnapshots GET /admin/channels/health/:serverId/snapshots
func (h *AdminHandler) ListSnapshots(c *gin.Context) {
	serverID, err := uuid.Parse(c.Param("serverId"))
	if err != nil {
		server.BadRequest(c, "invalid server_id")
		return
	}
	limit := atoiDefault(c.Query("limit"), 200)
	items, err := h.svc.ListSnapshots(c.Request.Context(), serverID, limit)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{
		"server_id": serverID,
		"items":     items,
		"limit":     limit,
	})
}

// ListFailoverEvents GET /admin/channels/failover-events
func (h *AdminHandler) ListFailoverEvents(c *gin.Context) {
	q := &FailoverEventListQuery{
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 50),
		Reason:   c.Query("reason"),
	}
	if sid := c.Query("server_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			q.ServerID = &id
		}
	}
	if s := c.Query("start_at"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			q.StartAt = &t
		}
	}
	if e := c.Query("end_at"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			q.EndAt = &t
		}
	}
	items, total, err := h.svc.ListFailoverEvents(c.Request.Context(), q.ServerID, q.Reason, q.StartAt, q.EndAt, q.Page, q.PageSize)
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

// ManualSwitch POST /admin/channels/switch
// 通过 ChannelSwitcher 将切换指令下发到 node-agent（未注入时退化为 queued）
func (h *AdminHandler) ManualSwitch(c *gin.Context) {
	var req ManualSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	resp, err := h.svc.ManualSwitch(c.Request.Context(), req.ServerID, req.TargetChannel, req.Reason)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, resp)
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
