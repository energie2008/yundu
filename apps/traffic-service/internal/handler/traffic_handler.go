package handler

import (
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/traffic-service/internal/middleware"
	"github.com/airport-panel/traffic-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TrafficHandler struct {
	trafficService *service.TrafficService
}

func NewTrafficHandler(trafficService *service.TrafficService) *TrafficHandler {
	return &TrafficHandler{
		trafficService: trafficService,
	}
}

func parseDateParam(c *gin.Context, key string) *time.Time {
	dateStr := c.Query(key)
	if dateStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}
	return &t
}

func (h *TrafficHandler) GetMyTraffic(c *gin.Context) {
	userID := middleware.GetUserID(c)
	startDate := parseDateParam(c, "start_date")
	endDate := parseDateParam(c, "end_date")

	var start, end time.Time
	if startDate != nil {
		start = *startDate
	}
	if endDate != nil {
		end = *endDate
	}

	resp, err := h.trafficService.GetUserTraffic(c.Request.Context(), userID, start, end)
	if err != nil {
		code, msg := service.MapTrafficErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

func (h *TrafficHandler) GetUserTraffic(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid user id")
		return
	}

	startDate := parseDateParam(c, "start_date")
	endDate := parseDateParam(c, "end_date")

	var start, end time.Time
	if startDate != nil {
		start = *startDate
	}
	if endDate != nil {
		end = *endDate
	}

	resp, err := h.trafficService.GetUserTraffic(c.Request.Context(), userID, start, end)
	if err != nil {
		code, msg := service.MapTrafficErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

func (h *TrafficHandler) GetOverview(c *gin.Context) {
	resp, err := h.trafficService.GetOverview(c.Request.Context())
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.OK(c, resp)
}

func (h *TrafficHandler) CheckUserQuota(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid user id")
		return
	}

	resp, err := h.trafficService.CheckQuota(c.Request.Context(), userID)
	if err != nil {
		code, msg := service.MapTrafficErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

func (h *TrafficHandler) ResetUserTraffic(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid user id")
		return
	}

	if err := h.trafficService.ResetTraffic(c.Request.Context(), userID); err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.OK(c, gin.H{"status": "ok"})
}

func (h *TrafficHandler) RegisterUserRoutes(rg *gin.RouterGroup, auth *middleware.AuthMiddleware) {
	user := rg.Group("/user")
	user.Use(auth.UserAuth())
	{
		user.GET("/traffic", h.GetMyTraffic)
	}
}

func (h *TrafficHandler) RegisterAdminRoutes(rg *gin.RouterGroup, auth *middleware.AuthMiddleware) {
	admin := rg.Group("/admin")
	admin.Use(auth.AdminAuth())
	{
		admin.GET("/traffic/overview", h.GetOverview)
		admin.GET("/traffic/user/:id", h.GetUserTraffic)
		admin.GET("/traffic/user/:id/quota", h.CheckUserQuota)
		admin.POST("/traffic/user/:id/reset", h.ResetUserTraffic)
	}
}
