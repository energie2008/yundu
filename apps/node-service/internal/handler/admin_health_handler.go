package handler

import (
	"strconv"
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
)

type AdminHealthHandler struct {
	healthService *service.HealthService
}

func NewAdminHealthHandler(healthService *service.HealthService) *AdminHealthHandler {
	return &AdminHealthHandler{
		healthService: healthService,
	}
}

func (h *AdminHealthHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	health := admin.Group("/health")
	{
		health.GET("/events", rbac.RequirePermission("nodes.read"), h.ListEvents)
	}
}

func (h *AdminHealthHandler) ListEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	nodeID := c.Query("node_id")
	eventType := c.Query("event_type")
	severity := c.Query("severity")

	var startTime, endTime *time.Time
	if st := c.Query("start_time"); st != "" {
		t, err := time.Parse(time.RFC3339, st)
		if err == nil {
			startTime = &t
		}
	}
	if et := c.Query("end_time"); et != "" {
		t, err := time.Parse(time.RFC3339, et)
		if err == nil {
			endTime = &t
		}
	}

	events, total, err := h.healthService.ListHealthEvents(c.Request.Context(), page, pageSize, nodeID, eventType, severity, startTime, endTime)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]model.NodeHealthEventResponse, len(events))
	for i, e := range events {
		items[i] = model.NewNodeHealthEventResponse(e)
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}
