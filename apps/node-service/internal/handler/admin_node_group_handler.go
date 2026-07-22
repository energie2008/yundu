package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// failServiceError 统一处理 service 层错误返回（兼容历史调用点）
func failServiceError(c *gin.Context, op string, err error) {
	code, msg := service.MapNodeErrorToCode(err)
	server.Fail(c, code, msg)
}

// AdminNodeGroupHandler 会员分组管理
type AdminNodeGroupHandler struct {
	groupService *service.NodeGroupService
}

func NewAdminNodeGroupHandler(groupService *service.NodeGroupService) *AdminNodeGroupHandler {
	return &AdminNodeGroupHandler{groupService: groupService}
}

func (h *AdminNodeGroupHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	groups := admin.Group("/node-groups")
	{
		groups.POST("", rbac.RequirePermission("nodes.write"), h.Create)
		groups.GET("", rbac.RequirePermission("nodes.read"), h.List)
		groups.GET("/all", rbac.RequirePermission("nodes.read"), h.ListAll)
		groups.GET("/:id", rbac.RequirePermission("nodes.read"), h.Get)
		groups.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.Update)
		groups.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.Delete)
		// 批量绑定/解绑节点到分组
		groups.POST("/:id/bind-nodes", rbac.RequirePermission("nodes.write"), h.BindNodes)
		groups.POST("/:id/unbind-nodes", rbac.RequirePermission("nodes.write"), h.UnbindNodes)
		// 列出分组下的节点 ID
		groups.GET("/:id/nodes", rbac.RequirePermission("nodes.read"), h.ListNodes)
	}
}

// BindNodes 批量绑定节点到分组
// 请求体: { "node_ids": ["uuid1", "uuid2", ...] }
// 如果节点已属于其他分组，将被迁移到此分组
func (h *AdminNodeGroupHandler) BindNodes(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	var req model.BatchBindNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if len(req.NodeIDs) == 0 {
		server.BadRequest(c, "node_ids 不能为空")
		return
	}

	affected, err := h.groupService.BatchBindNodes(c.Request.Context(), id, req.NodeIDs)
	if err != nil {
		failServiceError(c, "NodeGroup.BindNodes", err)
		return
	}

	server.OK(c, gin.H{"bound": affected})
}

// UnbindNodes 批量解绑节点（从分组移除）
// 请求体: { "node_ids": ["uuid1", "uuid2", ...] }
func (h *AdminNodeGroupHandler) UnbindNodes(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	var req model.BatchUnbindNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if len(req.NodeIDs) == 0 {
		server.BadRequest(c, "node_ids 不能为空")
		return
	}

	affected, err := h.groupService.BatchUnbindNodes(c.Request.Context(), id, req.NodeIDs)
	if err != nil {
		failServiceError(c, "NodeGroup.UnbindNodes", err)
		return
	}

	server.OK(c, gin.H{"unbound": affected})
}

// ListNodes 列出分组下的所有节点 ID
func (h *AdminNodeGroupHandler) ListNodes(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	nodeIDs, err := h.groupService.ListNodeIDsByGroup(c.Request.Context(), id)
	if err != nil {
		failServiceError(c, "NodeGroup.ListNodes", err)
		return
	}

	server.OK(c, gin.H{"node_ids": nodeIDs})
}

func (h *AdminNodeGroupHandler) Create(c *gin.Context) {
	var req model.CreateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	g, err := h.groupService.Create(c.Request.Context(), &req)
	if err != nil {
		failServiceError(c, "NodeGroup.Create", err)
		return
	}

	nodeCount, _ := h.groupService.CountNodes(c.Request.Context(), g.ID)
	server.Created(c, model.NewNodeGroupResponse(g, nodeCount, 0))
}

func (h *AdminNodeGroupHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	search := c.Query("search")

	groups, total, err := h.groupService.List(c.Request.Context(), page, pageSize, search)
	if err != nil {
		failServiceError(c, "NodeGroup.List", err)
		return
	}

	items := make([]model.NodeGroupResponse, len(groups))
	for i, g := range groups {
		nodeCount, _ := h.groupService.CountNodes(c.Request.Context(), g.ID)
		items[i] = model.NewNodeGroupResponse(g, nodeCount, 0)
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

// ListAll 返回所有分组（不分页，供下拉框使用）
func (h *AdminNodeGroupHandler) ListAll(c *gin.Context) {
	groups, err := h.groupService.ListAll(c.Request.Context())
	if err != nil {
		failServiceError(c, "NodeGroup.ListAll", err)
		return
	}

	items := make([]model.NodeGroupResponse, len(groups))
	for i, g := range groups {
		nodeCount, _ := h.groupService.CountNodes(c.Request.Context(), g.ID)
		items[i] = model.NewNodeGroupResponse(g, nodeCount, 0)
	}

	server.OK(c, gin.H{"items": items})
}

func (h *AdminNodeGroupHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	g, err := h.groupService.Get(c.Request.Context(), id)
	if err != nil {
		failServiceError(c, "NodeGroup.Get", err)
		return
	}

	nodeCount, _ := h.groupService.CountNodes(c.Request.Context(), g.ID)
	server.OK(c, model.NewNodeGroupResponse(g, nodeCount, 0))
}

func (h *AdminNodeGroupHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	var req model.UpdateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	g, err := h.groupService.Update(c.Request.Context(), id, &req)
	if err != nil {
		failServiceError(c, "NodeGroup.Update", err)
		return
	}

	nodeCount, _ := h.groupService.CountNodes(c.Request.Context(), g.ID)
	server.OK(c, model.NewNodeGroupResponse(g, nodeCount, 0))
}

func (h *AdminNodeGroupHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	if err := h.groupService.Delete(c.Request.Context(), id); err != nil {
		failServiceError(c, "NodeGroup.Delete", err)
		return
	}

	server.OK(c, gin.H{"deleted": true})
}
