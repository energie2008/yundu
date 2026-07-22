package handler

import (
	"strconv"
	"strings"

	"github.com/airport-panel/config/server"
	nodecrypto "github.com/airport-panel/node-service/internal/crypto"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminNodeHandler struct {
	nodeService   *service.NodeService
	healthService *service.HealthService
}

func NewAdminNodeHandler(nodeService *service.NodeService, healthService *service.HealthService) *AdminNodeHandler {
	return &AdminNodeHandler{
		nodeService:   nodeService,
		healthService: healthService,
	}
}

func (h *AdminNodeHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	nodes := admin.Group("/nodes")
	{
		nodes.POST("", rbac.RequirePermission("nodes.write"), h.CreateNode)
		nodes.GET("", rbac.RequirePermission("nodes.read"), h.ListNodes)
		nodes.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetNode)
		nodes.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdateNode)
		nodes.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeleteNode)
		nodes.GET("/:id/health", rbac.RequirePermission("nodes.read"), h.GetNodeHealth)
		// P0-5: 批量迁移历史节点 config_json 到规范化结构
		nodes.POST("/migrate-config-json", rbac.RequirePermission("nodes.write"), h.MigrateConfigJSON)
		// P1-9: REALITY 密钥自动生成
		nodes.POST("/reality/generate", rbac.RequirePermission("nodes.write"), h.GenerateREALITYKeypair)
	}
}

// GenerateREALITYKeypair P1-9: 生成 X25519 REALITY 密钥对
// 替代固定密钥对，每个 REALITY 节点应使用独立密钥对。
// 返回 base64url 编码的 private_key 和 public_key，可直接写入节点 config_json。
func (h *AdminNodeHandler) GenerateREALITYKeypair(c *gin.Context) {
	privateKey, publicKey, err := nodecrypto.GenerateREALITYKeypair()
	if err != nil {
		server.InternalError(c, "生成 REALITY 密钥对失败: "+err.Error())
		return
	}
	server.OK(c, gin.H{
		"private_key": privateKey,
		"public_key":  publicKey,
		"curve":       "x25519",
		"encoding":    "base64url",
	})
}

// MigrateConfigJSON P0-5: 触发批量迁移所有节点的 config_json 到规范化结构
func (h *AdminNodeHandler) MigrateConfigJSON(c *gin.Context) {
	migrated, err := h.nodeService.MigrateAllNodeConfigJSON(c.Request.Context())
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{
		"migrated_count": migrated,
		"message":        "config_json migration completed",
	})
}

func (h *AdminNodeHandler) CreateNode(c *gin.Context) {
	var req model.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, friendlyBindError(err))
		return
	}
	sanitizeCreateNodeRequest(&req)

	node, err := h.nodeService.CreateNode(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapNodeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewNodeResponse(node))
}

func (h *AdminNodeHandler) ListNodes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	protocolType := c.Query("protocol_type")
	regionID := c.Query("region_id")
	groupID := c.Query("group_id")
	search := c.Query("search")

	var isEnabled *bool
	if isEnabledStr := c.Query("is_enabled"); isEnabledStr != "" {
		b, err := strconv.ParseBool(isEnabledStr)
		if err == nil {
			isEnabled = &b
		}
	}

	nodes, total, err := h.nodeService.ListNodes(c.Request.Context(), page, pageSize, protocolType, regionID, groupID, search, isEnabled)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	ctx := c.Request.Context()
	nodeIDs := make([]uuid.UUID, len(nodes))
	runtimeIDSet := make(map[uuid.UUID]struct{})
	groupIDSet := make(map[uuid.UUID]struct{})
	for i, n := range nodes {
		nodeIDs[i] = n.ID
		runtimeIDSet[n.RuntimeID] = struct{}{}
		if n.GroupID != nil {
			groupIDSet[*n.GroupID] = struct{}{}
		}
	}

	// 批量查询避免 N+1
	planCodesMap, _ := h.nodeService.ListPlanCodesForNodes(ctx, nodeIDs)
	chainIDsMap, _ := h.nodeService.ListChainIDsForNodes(ctx, nodeIDs)

	runtimeIDs := make([]uuid.UUID, 0, len(runtimeIDSet))
	for rid := range runtimeIDSet {
		runtimeIDs = append(runtimeIDs, rid)
	}
	serverInfoMap, _ := h.nodeService.ListServerInfoForRuntimes(ctx, runtimeIDs)

	groupIDs := make([]uuid.UUID, 0, len(groupIDSet))
	for gid := range groupIDSet {
		groupIDs = append(groupIDs, gid)
	}
	groupInfoMap, _ := h.nodeService.ListNodeGroupBriefs(ctx, groupIDs)

	// 批量查询每个节点的所有所属分组（多对多）
	allNodeIDs := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		allNodeIDs = append(allNodeIDs, n.ID)
	}
	nodeGroupsMap, _ := h.nodeService.ListGroupsForNodes(ctx, allNodeIDs)

	items := make([]model.NodeResponse, len(nodes))
	for i, n := range nodes {
		health, _ := h.healthService.GetNodeHealth(ctx, n.ID)
		var serverInfo *model.ServerBrief
		if si, ok := serverInfoMap[n.RuntimeID]; ok {
			serverInfo = si
		}
		var chainIDs []uuid.UUID
		if cids, ok := chainIDsMap[n.ID]; ok {
			chainIDs = cids
		}
		var groupInfo *model.NodeGroupBrief
		if n.GroupID != nil {
			if gi, ok := groupInfoMap[*n.GroupID]; ok {
				groupInfo = gi
			}
		}
		// 多对多分组回显
		var nodeGroupIDs []uuid.UUID
		var nodeGroups []model.NodeGroupBrief
		if gs, ok := nodeGroupsMap[n.ID]; ok {
			for _, g := range gs {
				nodeGroupIDs = append(nodeGroupIDs, g.ID)
				nodeGroups = append(nodeGroups, *g)
			}
		}
		resp := model.NewNodeResponseWithFull(n, health, serverInfo, chainIDs, groupInfo, nodeGroupIDs, nodeGroups)
		if codes, ok := planCodesMap[n.ID]; ok {
			resp.PlanCodes = codes
		} else {
			resp.PlanCodes = []string{}
		}
		items[i] = resp
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

func (h *AdminNodeHandler) GetNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	node, err := h.nodeService.GetNode(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapNodeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	ctx := c.Request.Context()
	health, _ := h.healthService.GetNodeHealth(ctx, node.ID)

	// 查询 server_info
	serverInfoMap, _ := h.nodeService.ListServerInfoForRuntimes(ctx, []uuid.UUID{node.RuntimeID})
	var serverInfo *model.ServerBrief
	if si, ok := serverInfoMap[node.RuntimeID]; ok {
		serverInfo = si
	}

	// 查询 chain_ids
	chainIDsMap, _ := h.nodeService.ListChainIDsForNodes(ctx, []uuid.UUID{node.ID})
	var chainIDs []uuid.UUID
	if cids, ok := chainIDsMap[node.ID]; ok {
		chainIDs = cids
	}

	// 查询 group_info（主分组，向后兼容）
	var groupInfo *model.NodeGroupBrief
	if node.GroupID != nil {
		groupInfoMap, _ := h.nodeService.ListNodeGroupBriefs(ctx, []uuid.UUID{*node.GroupID})
		if gi, ok := groupInfoMap[*node.GroupID]; ok {
			groupInfo = gi
		}
	}

	// 查询节点的所有所属分组（多对多）
	nodeGroupsMap, _ := h.nodeService.ListGroupsForNodes(ctx, []uuid.UUID{node.ID})
	var nodeGroupIDs []uuid.UUID
	var nodeGroups []model.NodeGroupBrief
	if gs, ok := nodeGroupsMap[node.ID]; ok {
		for _, g := range gs {
			nodeGroupIDs = append(nodeGroupIDs, g.ID)
			nodeGroups = append(nodeGroups, *g)
		}
	}

	resp := model.NewNodeResponseWithFull(node, health, serverInfo, chainIDs, groupInfo, nodeGroupIDs, nodeGroups)

	// 填充 plan_codes
	planCodesMap, _ := h.nodeService.ListPlanCodesForNodes(ctx, []uuid.UUID{node.ID})
	if codes, ok := planCodesMap[node.ID]; ok {
		resp.PlanCodes = codes
	} else {
		resp.PlanCodes = []string{}
	}

	server.OK(c, resp)
}

func (h *AdminNodeHandler) UpdateNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	var req model.UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, friendlyBindError(err))
		return
	}
	sanitizeUpdateNodeRequest(&req)

	node, err := h.nodeService.UpdateNode(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := service.MapNodeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	health, _ := h.healthService.GetNodeHealth(c.Request.Context(), node.ID)

	server.OK(c, model.NewNodeResponseWithHealth(node, health))
}

func (h *AdminNodeHandler) DeleteNode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	if err := h.nodeService.DeleteNode(c.Request.Context(), id); err != nil {
		code, msg := service.MapNodeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"deleted": true})
}

func (h *AdminNodeHandler) GetNodeHealth(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	health, err := h.healthService.GetNodeHealth(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapHealthErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewNodeHealthResponse(health))
}

// friendlyBindError 将 google/uuid 包返回的晦涩错误（如 "invalid UUID length: 4"）
// 转换为面向管理员的中文提示，并保留原始错误用于排查。
// 主要场景：前端未校验 UUID 字段格式直接提交，导致后端 uuid.UUID 字段解析失败。
func friendlyBindError(err error) string {
	raw := err.Error()
	// google/uuid 包的错误特征："invalid UUID length: N" 或 "invalid UUID format: ..."
	if strings.Contains(raw, "invalid UUID length") || strings.Contains(raw, "invalid UUID format") {
		// 提取字段名（Gin binding 错误通常包含 "Key: 'CreateNodeRequest.RuntimeID' "）
		// 注意：必须检查所有 UUID 字段，否则默认归咎于 runtime_id 会误导排查
		field := "runtime_id"
		switch {
		case strings.Contains(raw, "RuntimeID"):
			field = "runtime_id"
		case strings.Contains(raw, "RegionID"):
			field = "region_id"
		case strings.Contains(raw, "GroupIDs"):
			field = "group_ids"
		case strings.Contains(raw, "GroupID"):
			field = "group_id"
		case strings.Contains(raw, "ChainIDs"):
			field = "chain_ids"
		}
		return "参数 " + field + " 不是有效的 UUID 格式（期望 36 字符，如 a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d）。请检查所选服务器是否已注册 runtime，或刷新服务器列表后重试。"
	}
	return raw
}

func trimStr(s *string) {
	if s != nil {
		v := strings.TrimSpace(*s)
		*s = v
	}
}

func sanitizeCreateNodeRequest(req *model.CreateNodeRequest) {
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	req.ProtocolType = strings.TrimSpace(req.ProtocolType)
	req.TransportType = strings.TrimSpace(req.TransportType)
	req.ProtocolSchemaVersion = strings.TrimSpace(req.ProtocolSchemaVersion)
	trimStr(req.SecurityType)
	trimStr(req.RealityServerName)
	trimStr(req.SNI)
	trimStr(req.Path)
	trimStr(req.HostHeader)
	trimStr(req.Flow)
	trimStr(req.PaddingScheme)
	for i := range req.ALPN {
		req.ALPN[i] = strings.TrimSpace(req.ALPN[i])
	}
	for i := range req.Tags {
		req.Tags[i] = strings.TrimSpace(req.Tags[i])
	}
}

func sanitizeUpdateNodeRequest(req *model.UpdateNodeRequest) {
	trimStr(req.Code)
	trimStr(req.Name)
	trimStr(req.Address)
	trimStr(req.ProtocolType)
	trimStr(req.TransportType)
	trimStr(req.SecurityType)
	trimStr(req.RealityServerName)
	trimStr(req.SNI)
	trimStr(req.Path)
	trimStr(req.HostHeader)
	trimStr(req.Flow)
	trimStr(req.PaddingScheme)
	for i := range req.ALPN {
		req.ALPN[i] = strings.TrimSpace(req.ALPN[i])
	}
}
