package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/traffic-service/internal/middleware"
	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/airport-panel/traffic-service/internal/service"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	trafficService *service.TrafficService
}

func NewAgentHandler(trafficService *service.TrafficService) *AgentHandler {
	return &AgentHandler{
		trafficService: trafficService,
	}
}

func (h *AgentHandler) ReportTraffic(c *gin.Context) {
	var req model.TrafficReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	// 从 AgentAuth 中间件获取 ServerCode，用于解析 node_id
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		serverCode = req.ServerCode
	}

	if err := h.trafficService.ReportTraffic(c.Request.Context(), req.Reports, serverCode); err != nil {
		code, msg := service.MapTrafficErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"status": "ok", "reported_count": len(req.Reports)})
}

func (h *AgentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	agent := rg.Group("/agent")
	{
		agent.POST("/traffic/report", h.ReportTraffic)
	}
}
