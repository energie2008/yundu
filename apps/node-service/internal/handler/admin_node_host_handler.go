package handler

// admin_node_host_handler.go 实现 P2-3：节点多 Host 管理 API
//
// 路由：
//   POST   /admin/nodes/:id/hosts          创建 host
//   GET    /admin/nodes/:id/hosts          列出节点所有 host
//   PATCH  /admin/nodes/:id/hosts/:hid     更新 host
//   DELETE /admin/nodes/:id/hosts/:hid     删除 host

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminNodeHostHandler struct {
	hostRepo *repo.NodeHostRepo
}

func NewAdminNodeHostHandler(hostRepo *repo.NodeHostRepo) *AdminNodeHostHandler {
	return &AdminNodeHostHandler{hostRepo: hostRepo}
}

func (h *AdminNodeHostHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	nodes := admin.Group("/nodes")
	{
		nodes.POST("/:id/hosts", rbac.RequirePermission("nodes.write"), h.CreateHost)
		nodes.GET("/:id/hosts", rbac.RequirePermission("nodes.read"), h.ListHosts)
		nodes.PATCH("/:id/hosts/:hid", rbac.RequirePermission("nodes.write"), h.UpdateHost)
		nodes.DELETE("/:id/hosts/:hid", rbac.RequirePermission("nodes.write"), h.DeleteHost)
	}
}

// CreateHost 创建节点 host
func (h *AdminNodeHostHandler) CreateHost(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}
	var req struct {
		Host       string  `json:"host" binding:"required"`
		HostType   string  `json:"host_type"`            // cdn/direct/tunnel，默认 cdn
		Port       *int    `json:"port,omitempty"`
		Path       *string `json:"path,omitempty"`
		SNI        *string `json:"sni,omitempty"`
		HostHeader *string `json:"host_header,omitempty"`
		Priority   int     `json:"priority"`             // 默认 0→100
		IsEnabled  *bool   `json:"is_enabled,omitempty"`
		Remark     string  `json:"remark,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if req.HostType == "" {
		req.HostType = "cdn"
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	host := &repo.NodeHost{
		NodeID:     nodeID,
		Host:       req.Host,
		HostType:   req.HostType,
		Port:       req.Port,
		Path:       req.Path,
		SNI:        req.SNI,
		HostHeader: req.HostHeader,
		Priority:   req.Priority,
		IsEnabled:  enabled,
		Remark:     req.Remark,
	}
	if err := h.hostRepo.Create(c.Request.Context(), host); err != nil {
		server.InternalError(c, "创建节点 host 失败: "+err.Error())
		return
	}
	server.Created(c, host)
}

// ListHosts 列出节点所有 host
func (h *AdminNodeHostHandler) ListHosts(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}
	hosts, err := h.hostRepo.ListByNode(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "查询节点 host 失败: "+err.Error())
		return
	}
	server.OK(c, gin.H{"hosts": hosts, "count": len(hosts)})
}

// UpdateHost 更新节点 host
func (h *AdminNodeHostHandler) UpdateHost(c *gin.Context) {
	hostID, err := strconv.ParseInt(c.Param("hid"), 10, 64)
	if err != nil {
		server.BadRequest(c, "invalid host id")
		return
	}
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}
	var req struct {
		Host       *string `json:"host,omitempty"`
		HostType   *string `json:"host_type,omitempty"`
		Port       *int    `json:"port,omitempty"`
		Path       *string `json:"path,omitempty"`
		SNI        *string `json:"sni,omitempty"`
		HostHeader *string `json:"host_header,omitempty"`
		Priority   *int    `json:"priority,omitempty"`
		IsEnabled  *bool   `json:"is_enabled,omitempty"`
		Remark     *string `json:"remark,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	// 先读取现有 host
	hosts, err := h.hostRepo.ListByNode(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "查询节点 host 失败: "+err.Error())
		return
	}
	var existing *repo.NodeHost
	for _, h2 := range hosts {
		if h2.ID == hostID {
			existing = h2
			break
		}
	}
	if existing == nil {
		server.NotFound(c, "host not found")
		return
	}
	// 应用更新
	if req.Host != nil {
		existing.Host = *req.Host
	}
	if req.HostType != nil {
		existing.HostType = *req.HostType
	}
	if req.Port != nil {
		existing.Port = req.Port
	}
	if req.Path != nil {
		existing.Path = req.Path
	}
	if req.SNI != nil {
		existing.SNI = req.SNI
	}
	if req.HostHeader != nil {
		existing.HostHeader = req.HostHeader
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.IsEnabled != nil {
		existing.IsEnabled = *req.IsEnabled
	}
	if req.Remark != nil {
		existing.Remark = *req.Remark
	}
	if err := h.hostRepo.Update(c.Request.Context(), existing); err != nil {
		server.InternalError(c, "更新节点 host 失败: "+err.Error())
		return
	}
	server.OK(c, existing)
}

// DeleteHost 删除节点 host
func (h *AdminNodeHostHandler) DeleteHost(c *gin.Context) {
	hostID, err := strconv.ParseInt(c.Param("hid"), 10, 64)
	if err != nil {
		server.BadRequest(c, "invalid host id")
		return
	}
	if err := h.hostRepo.Delete(c.Request.Context(), hostID); err != nil {
		server.InternalError(c, "删除节点 host 失败: "+err.Error())
		return
	}
	server.OK(c, gin.H{"deleted": true})
}
