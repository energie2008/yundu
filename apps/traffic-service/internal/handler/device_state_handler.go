package handler

import (
	"strings"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/airport-panel/traffic-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DeviceStateHandler 处理 Agent 设备态上报与查询的 HTTP 请求。
//
// 路由（挂在 /api/v1/agent 之下，复用 AgentAuth + HMAC 中间件）：
//   - POST   /devices/report          Agent 上报在线设备 IP
//   - GET    /devices/alive           批量获取用户当前设备数
//   - DELETE /devices/node/:nodeId    节点断连时清除该节点所有设备记录
type DeviceStateHandler struct {
	deviceStateService *service.DeviceStateService
}

// NewDeviceStateHandler 创建设备态 handler。
func NewDeviceStateHandler(deviceStateService *service.DeviceStateService) *DeviceStateHandler {
	return &DeviceStateHandler{
		deviceStateService: deviceStateService,
	}
}

// ReportDevices POST /api/v1/agent/devices/report
//
// 请求体：model.DeviceReportRequest
// Agent 周期性上报某用户在本节点的在线 IP 列表，面板据此聚合跨节点设备态。
func (h *DeviceStateHandler) ReportDevices(c *gin.Context) {
	var req model.DeviceReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	if err := h.deviceStateService.SetDevices(c.Request.Context(), req.UserID, req.NodeID, req.IPs); err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.OK(c, gin.H{
		"status":         "ok",
		"reported_count": len(req.IPs),
	})
}

// GetAliveList GET /api/v1/agent/devices/alive?user_ids=xxx,yyy
//
// 查询多个用户当前在线设备数。user_ids 为逗号分隔的 UUID 列表。
// 返回 model.AliveListResponse。
func (h *DeviceStateHandler) GetAliveList(c *gin.Context) {
	idsParam := c.Query("user_ids")
	if idsParam == "" {
		server.BadRequest(c, "missing user_ids")
		return
	}

	userIDs, errMsg := parseUserIDs(idsParam)
	if errMsg != "" {
		server.BadRequest(c, errMsg)
		return
	}
	if len(userIDs) == 0 {
		server.BadRequest(c, "no valid user_ids")
		return
	}

	alive, err := h.deviceStateService.GetAliveList(c.Request.Context(), userIDs)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	users := make(map[string]int, len(alive))
	for uid, count := range alive {
		users[uid.String()] = count
	}
	server.OK(c, model.AliveListResponse{Users: users})
}

// GetDevicesList GET /api/v1/agent/devices/list?user_ids=xxx,yyy
//
// 查询多个用户的在线设备 IP 列表。user_ids 为逗号分隔的 UUID 列表。
// 返回 model.DevicesListResponse。
func (h *DeviceStateHandler) GetDevicesList(c *gin.Context) {
	idsParam := c.Query("user_ids")
	if idsParam == "" {
		server.BadRequest(c, "missing user_ids")
		return
	}

	userIDs, errMsg := parseUserIDs(idsParam)
	if errMsg != "" {
		server.BadRequest(c, errMsg)
		return
	}
	if len(userIDs) == 0 {
		server.BadRequest(c, "no valid user_ids")
		return
	}

	devices, err := h.deviceStateService.GetUsersDevices(c.Request.Context(), userIDs)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	users := make(map[string][]string, len(devices))
	for uid, ips := range devices {
		users[uid.String()] = ips
	}
	server.OK(c, model.DevicesListResponse{Users: users})
}

// ClearNodeDevices DELETE /api/v1/agent/devices/node/:nodeId
//
// 节点断连（或下线）时清除该节点上报的所有设备记录，避免残留过期记录影响设备数统计。
func (h *DeviceStateHandler) ClearNodeDevices(c *gin.Context) {
	nodeIDStr := c.Param("nodeId")
	nodeID, err := uuid.Parse(nodeIDStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	cleared, err := h.deviceStateService.ClearAllNodeDevicesCounted(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}

	server.OK(c, model.ClearNodeResponse{
		Status:  "ok",
		Cleared: cleared,
	})
}

// RegisterRoutes 将设备态路由注册到指定的 RouterGroup。
// 应挂在 /api/v1/agent 组下（该组已启用 AgentAuth + HMAC 中间件）。
func (h *DeviceStateHandler) RegisterRoutes(rg *gin.RouterGroup) {
	devices := rg.Group("/devices")
	{
		devices.POST("/report", h.ReportDevices)
		devices.GET("/alive", h.GetAliveList)
		devices.GET("/list", h.GetDevicesList)
		devices.DELETE("/node/:nodeId", h.ClearNodeDevices)
	}
}

// parseUserIDs 解析逗号分隔的 UUID 字符串，返回 UUID 列表。
// 返回的 errMsg 非空时表示存在格式错误的 ID。
func parseUserIDs(idsParam string) ([]uuid.UUID, string) {
	parts := strings.Split(idsParam, ",")
	userIDs := make([]uuid.UUID, 0, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, "invalid user id: " + raw
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, ""
}
