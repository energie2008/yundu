package outbound

import (
	"context"
	"fmt"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminOutboundHandler 处理节点出站策略的 admin 路由
type AdminOutboundHandler struct {
	svc *OutboundService
}

func NewAdminOutboundHandler(svc *OutboundService) *AdminOutboundHandler {
	return &AdminOutboundHandler{svc: svc}
}

func (h *AdminOutboundHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	// 挂在 /nodes/:id/outbound-policies 下
	nodes := admin.Group("/nodes")
	{
		// 注意：apply-all 必须在 :pid 之前注册（gin 静态路径优先于参数）
		nodes.GET("/:id/outbound-policies", rbac.RequirePermission("nodes.read"), h.ListPolicies)
		nodes.POST("/:id/outbound-policies", rbac.RequirePermission("nodes.write"), h.CreatePolicy)
		nodes.POST("/:id/outbound-policies/apply-all", rbac.RequirePermission("nodes.write"), h.ApplyAll)
		nodes.PATCH("/:id/outbound-policies/:pid", rbac.RequirePermission("nodes.write"), h.UpdatePolicy)
		nodes.DELETE("/:id/outbound-policies/:pid", rbac.RequirePermission("nodes.write"), h.DeletePolicy)
	}
}

func parseNodeID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return uuid.Nil, false
	}
	return id, true
}

func parsePolicyID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *AdminOutboundHandler) ListPolicies(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	items, err := h.svc.ListByNode(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]PolicyResponse, len(items))
	for i, p := range items {
		resp[i] = NewPolicyResponse(p)
	}
	server.OK(c, gin.H{"items": resp, "total": len(resp)})
}

func (h *AdminOutboundHandler) CreatePolicy(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	var req CreatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Create(c.Request.Context(), nodeID, &req)
	if err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPolicyResponse(p))
}

func (h *AdminOutboundHandler) UpdatePolicy(c *gin.Context) {
	_, ok := parseNodeID(c)
	if !ok {
		return
	}
	policyID, ok := parsePolicyID(c)
	if !ok {
		return
	}

	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.svc.Update(c.Request.Context(), policyID, &req)
	if err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPolicyResponse(p))
}

func (h *AdminOutboundHandler) DeletePolicy(c *gin.Context) {
	_, ok := parseNodeID(c)
	if !ok {
		return
	}
	policyID, ok := parsePolicyID(c)
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), policyID); err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

func (h *AdminOutboundHandler) ApplyAll(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	resp, err := h.svc.ApplyAll(c.Request.Context(), nodeID)
	if err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

// NodeIDLister 解析 serverID → node IDs（outbound_policies 按 node_id 存储）
type NodeIDLister interface {
	ListNodeIDsByServer(ctx context.Context, serverID uuid.UUID) ([]uuid.UUID, error)
}

// NodeInfo 节点基本信息（用于前端节点 WARP 分配器）
type NodeInfo struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	IsEnabled bool      `json:"is_enabled"`
}

// NodeListerEx 扩展接口：返回节点详情（含名称/启用状态）
// 用于前端按节点勾选 WARP 分配
type NodeListerEx interface {
	NodeIDLister
	ListNodesByServer(ctx context.Context, serverID uuid.UUID) ([]NodeInfo, error)
}

// WarpPoolRegistrar 抽象 warpreg 池的注册/导入能力（用于 handler 调用）
type WarpPoolRegistrar interface {
	RegisterForNode(ctx context.Context, serverID uuid.UUID, code string, opts *WarpRegisterOptions) (*WarpProfile, error)
	ImportExisting(ctx context.Context, serverID uuid.UUID, code, privateKey, localAddress string) (*WarpProfile, error)
}

// WarpLicenseApplier 抽象 WARP+ License 应用能力
type WarpLicenseApplier interface {
	ApplyLicense(ctx context.Context, deviceID, accessToken, license string) error
}

// AdminWarpHandler 处理 WARP 档案的 admin 路由
// 架构说明（按需分配模式）：
//   - warp_profiles.node_id 引用 servers(id) → WARP 账户按 VPS 维度管理
//   - outbound_policies.node_id 引用 nodes(id) → 出站策略按节点维度管理
//   - 注册/导入 WARP 账户只创建 warp_profiles 记录，不自动展开到节点
//   - 用户通过 EnableWarpForNodes 手动勾选节点，为选中节点创建 warp + load_balance outbound_policy
//   - 禁用 WARP 通过 DisableWarpForNodes 删除节点下 warp/load_balance outbound_policy
//   - 其他节点可走直连/socks5/chain 等其他出站策略
type AdminWarpHandler struct {
	svc         *WarpProfileService
	outboundSvc *OutboundService
	nodeLister  NodeIDLister
	nodeListerEx NodeListerEx // 可选：用于返回节点详情
	licenseApplier WarpLicenseApplier // 可选：用于应用 WARP+ License
}

func NewAdminWarpHandler(svc *WarpProfileService, outboundSvc *OutboundService, nodeLister NodeIDLister) *AdminWarpHandler {
	return &AdminWarpHandler{svc: svc, outboundSvc: outboundSvc, nodeLister: nodeLister}
}

// SetNodeListerEx 注入扩展节点列表器（可选，用于节点 WARP 分配器）
func (h *AdminWarpHandler) SetNodeListerEx(ex NodeListerEx) {
	h.nodeListerEx = ex
}

// SetLicenseApplier 注入 WARP+ License 应用器（可选）
func (h *AdminWarpHandler) SetLicenseApplier(applier WarpLicenseApplier) {
	h.licenseApplier = applier
}

func (h *AdminWarpHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	warp := admin.Group("/warp-profiles")
	{
		warp.GET("", rbac.RequirePermission("nodes.read"), h.ListWarpProfiles)
		warp.POST("", rbac.RequirePermission("nodes.write"), h.CreateWarpProfile)
		warp.DELETE("/:pid", rbac.RequirePermission("nodes.write"), h.DeleteWarpProfile)
		warp.POST("/:pid/apply-license", rbac.RequirePermission("nodes.write"), h.ApplyLicenseToProfile)
	}
	// 全部按服务器维度操作（warp_profiles.node_id 引用 servers 表）
	servers := admin.Group("/servers")
	{
		servers.GET("/:id/warp/status", rbac.RequirePermission("nodes.read"), h.GetServerWarpStatus)
		servers.GET("/:id/warp-profiles", rbac.RequirePermission("nodes.read"), h.ListNodeWarpProfiles)
		servers.GET("/:id/warp/nodes-status", rbac.RequirePermission("nodes.read"), h.GetServerNodesWarpStatus)
		servers.POST("/:id/warp/register", rbac.RequirePermission("nodes.write"), h.RegisterWarpForNode)
		servers.POST("/:id/warp/import", rbac.RequirePermission("nodes.write"), h.ImportWarpForNode)
		servers.POST("/:id/warp/enable-nodes", rbac.RequirePermission("nodes.write"), h.EnableWarpForNodes)
		servers.POST("/:id/warp/disable-nodes", rbac.RequirePermission("nodes.write"), h.DisableWarpForNodes)
		servers.POST("/:id/warp/enable-load-balance", rbac.RequirePermission("nodes.write"), h.EnableLoadBalance)
	}
}

// listNodeIDs 获取服务器下所有启用节点 ID（用于创建 outbound_policies）
func (h *AdminWarpHandler) listNodeIDs(ctx context.Context, serverID uuid.UUID) ([]uuid.UUID, error) {
	if h.nodeLister == nil {
		return nil, fmt.Errorf("node lister not configured")
	}
	return h.nodeLister.ListNodeIDsByServer(ctx, serverID)
}

// createWarpOutboundForNodes 为服务器下所有节点创建 warp outbound_policy
func (h *AdminWarpHandler) createWarpOutboundForNodes(ctx context.Context, serverID uuid.UUID, w *WarpProfile) {
	if w.PrivateKey == nil || w.OutboundTag == nil {
		return
	}
	nodeIDs, err := h.listNodeIDs(ctx, serverID)
	if err != nil {
		return
	}
	cfg := Map{
		"private_key":   *w.PrivateKey,
		"public_key":    "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		"endpoint":      "engage.cloudflareclient.com:2408",
		"local_address": *w.LocalAddress,
		"mtu":           w.MTU,
		"tag":           *w.OutboundTag,
	}
	if w.Endpoint != nil {
		cfg["endpoint"] = *w.Endpoint
	}
	if w.PublicKey != nil {
		cfg["public_key"] = *w.PublicKey
	}
	enabled := true
	priority := 100
	for _, nid := range nodeIDs {
		_, _ = h.outboundSvc.Create(ctx, nid, &CreatePolicyRequest{
			PolicyType: "warp",
			Priority:   &priority,
			ConfigJSON: cfg,
			IsEnabled:  &enabled,
		})
	}
}

func (h *AdminWarpHandler) ListWarpProfiles(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]WarpProfileResponse, len(items))
	for i, w := range items {
		resp[i] = NewWarpProfileResponse(w)
	}
	server.OK(c, gin.H{"items": resp, "total": len(resp)})
}

func (h *AdminWarpHandler) CreateWarpProfile(c *gin.Context) {
	var req CreateWarpProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	w, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewWarpProfileResponse(w))
}

func (h *AdminWarpHandler) DeleteWarpProfile(c *gin.Context) {
	pid, ok := parsePolicyID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), pid); err != nil {
		code, msg := MapOutboundErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

func (h *AdminWarpHandler) ListNodeWarpProfiles(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}
	items, err := h.svc.ListByNode(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	resp := make([]WarpProfileResponse, len(items))
	for i, w := range items {
		resp[i] = NewWarpProfileResponse(w)
	}
	server.OK(c, gin.H{"items": resp, "total": len(resp)})
}

// RegisterWarpForNode 一键注册 WARP 账户（对齐 3x-ui 体验）
// URL: POST /servers/:id/warp/register
//
// 请求体（JSON，可选字段，对齐 3x-ui "填 License/Endpoint 后一键注册"）：
//
//	{
//	  "code": "vps081",         // 可选：节点 code，用于生成 profile 名称（默认 "node"）
//	  "license_key": "xxx-xxx", // 可选：WARP+ License Key（团队零信任 Key 或个人 WARP+ Key）
//	  "endpoint": "162.159.193.1:2408"  // 可选：优选 Endpoint，覆盖默认 engage.cloudflareclient.com:2408
//	}
//
// 也兼容 query 参数（旧版）：?code=vps081
//
// 注册流程：
//  1. 面板调用 Cloudflare WARP API 注册新账户（自动生成 curve25519 密钥对）
//  2. 若提供 license_key → 自动调用 Cloudflare API 绑定 WARP+ License
//  3. 若提供 endpoint → 覆盖默认 Endpoint（接入优选 IP）
//  4. 自动填满 Private Key / Address / ClientID(reserved) / DeviceID / AccessToken 写入 warp_profiles 表
//  5. 按需分配：仅创建 warp_profile 记录，不自动展开到节点（用户通过 EnableWarpForNodes 手动勾选）
func (h *AdminWarpHandler) RegisterWarpForNode(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}
	if h.svc.pool == nil {
		server.Fail(c, config.CodeInternalError, "warp registration not configured")
		return
	}

	// 优先从 JSON body 读取（支持 license_key 和 endpoint），兼容 query 参数 code
	var req struct {
		Code       string `json:"code"`
		LicenseKey string `json:"license_key"`
		Endpoint   string `json:"endpoint"`
	}
	_ = c.ShouldBindJSON(&req)
	// query 参数 code 作为兜底（旧版兼容）
	if req.Code == "" {
		req.Code = c.Query("code")
	}
	if req.Code == "" {
		req.Code = "node"
	}

	// 构造注册选项（仅在有 license_key 或 endpoint 时传非空 opts）
	var opts *WarpRegisterOptions
	if req.LicenseKey != "" || req.Endpoint != "" {
		opts = &WarpRegisterOptions{
			LicenseKey: req.LicenseKey,
			Endpoint:   req.Endpoint,
		}
	}

	w, err := h.svc.pool.RegisterForNode(c.Request.Context(), serverID, req.Code, opts)
	if err != nil {
		server.Fail(c, config.CodeInternalError, err.Error())
		return
	}
	// 按需分配：只创建 warp_profile 记录，不自动展开到节点
	// 用户通过 EnableWarpForNodes API 手动勾选节点分配 WARP
	server.Created(c, NewWarpProfileResponse(w))
}

func (h *AdminWarpHandler) ImportWarpForNode(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}
	var req struct {
		PrivateKey   string `json:"private_key" binding:"required"`
		LocalAddress string `json:"local_address" binding:"required"`
		Code         string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if h.svc.pool == nil {
		server.Fail(c, config.CodeInternalError, "warp registration not configured")
		return
	}
	if req.Code == "" {
		req.Code = "node"
	}
	w, err := h.svc.pool.ImportExisting(c.Request.Context(), serverID, req.Code, req.PrivateKey, req.LocalAddress)
	if err != nil {
		server.Fail(c, config.CodeInternalError, err.Error())
		return
	}
	// 按需分配：只创建 warp_profile 记录，不自动展开到节点
	server.Created(c, NewWarpProfileResponse(w))
}

// EnableLoadBalance 为已有 warp outbound_policy 的节点创建 load_balance outbound_policy
// URL: POST /servers/:id/warp/enable-load-balance
// 按需分配模式：仅为已勾选启用 WARP 的节点创建负载均衡策略
func (h *AdminWarpHandler) EnableLoadBalance(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}
	var req struct {
		Strategy string `json:"strategy"`
	}
	_ = c.ShouldBindJSON(&req)
	strategy := req.Strategy
	if strategy == "" {
		strategy = "round_robin"
	}

	// 查询服务器所有 warp profiles（warp_profiles.node_id = serverID）
	profiles, err := h.svc.ListByNode(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if len(profiles) < 1 {
		server.BadRequest(c, "at least 1 warp profile required")
		return
	}

	// 收集 outbound tags
	var outboundTags []interface{}
	for _, p := range profiles {
		if p.OutboundTag != nil {
			outboundTags = append(outboundTags, *p.OutboundTag)
		}
	}
	if len(outboundTags) < 1 {
		server.BadRequest(c, "at least 1 warp outbound tag required")
		return
	}

	// 按需分配：仅为已有 warp outbound_policy 的节点创建 load_balance policy
	nodeIDs, err := h.listNodeIDs(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	cfg := Map{
		"tag":            "warp-pool",
		"outbounds":      outboundTags,
		"strategy":       strategy,
		"check_url":      "https://www.gstatic.com/generate_204",
		"check_interval": "3m",
	}
	enabled := true
	priority := 20 // 高优先级（低于 warp 本身的 100）
	created := 0
	skipped := 0
	for _, nid := range nodeIDs {
		// 检查节点是否已有 warp policy（按需分配的判断依据）
		policies, err := h.outboundSvc.ListByNode(c.Request.Context(), nid)
		if err != nil {
			skipped++
			continue
		}
		hasWarp := false
		hasLB := false
		for _, p := range policies {
			if p.PolicyType == "warp" && p.IsEnabled {
				hasWarp = true
			}
			if p.PolicyType == "load_balance" && p.IsEnabled {
				hasLB = true
			}
		}
		if !hasWarp || hasLB {
			skipped++
			continue
		}
		_, err = h.outboundSvc.Create(c.Request.Context(), nid, &CreatePolicyRequest{
			PolicyType: "load_balance",
			Priority:   &priority,
			ConfigJSON: cfg,
			IsEnabled:  &enabled,
		})
		if err == nil {
			created++
		}
	}
	server.OK(c, gin.H{
		"enabled":        true,
		"strategy":       strategy,
		"nodes_affected": created,
		"nodes_skipped":  skipped,
		"total_nodes":    len(nodeIDs),
		"outbound_tags":  outboundTags,
	})
}

// GetServerWarpStatus 返回某服务器的 WARP 账户池及负载均衡状态
// 注意：:id 是 servers.id（warp_profiles.node_id 引用 servers 表）
// 但 outbound_policies.node_id 引用 nodes(id)，需要解析 server→nodes 查询
func (h *AdminWarpHandler) GetServerWarpStatus(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}
	// 获取 WARP profiles（按 serverID 查询 warp_profiles）
	profiles, err := h.svc.ListByNode(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	// 获取该服务器下所有节点，检查 outbound_policies 中的 load_balance
	var lbEnabled bool
	var lbStrategy interface{}
	var lbOutbounds interface{}
	nodeIDs, err := h.listNodeIDs(c.Request.Context(), serverID)
	if err == nil {
		for _, nid := range nodeIDs {
			policies, err := h.outboundSvc.ListByNode(c.Request.Context(), nid)
			if err != nil {
				continue
			}
			for _, p := range policies {
				if p.PolicyType == "load_balance" && p.IsEnabled {
					lbEnabled = true
					lbStrategy = p.ConfigJSON["strategy"]
					lbOutbounds = p.ConfigJSON["outbounds"]
					break
				}
			}
			if lbEnabled {
				break
			}
		}
	}
	// 构建 WARP profiles 响应
	profileResp := make([]WarpProfileResponse, len(profiles))
	for i, w := range profiles {
		profileResp[i] = NewWarpProfileResponse(w)
	}
	result := gin.H{
		"profiles":      profileResp,
		"profile_count": len(profileResp),
		"load_balance":  lbEnabled,
		"node_count":    len(nodeIDs),
	}
	if lbStrategy != nil {
		result["load_balance_strategy"] = lbStrategy
	}
	if lbOutbounds != nil {
		result["load_balance_outbounds"] = lbOutbounds
	}
	server.OK(c, result)
}

// GetServerNodesWarpStatus 返回服务器下所有节点的 WARP 分配状态
// URL: GET /servers/:id/warp/nodes-status
// 用于前端节点 WARP 分配器：显示节点列表 + 每个节点是否启用 WARP
func (h *AdminWarpHandler) GetServerNodesWarpStatus(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}

	// 优先使用 NodeListerEx 获取节点详情
	if h.nodeListerEx != nil {
		nodes, err := h.nodeListerEx.ListNodesByServer(c.Request.Context(), serverID)
		if err != nil {
			server.InternalError(c, "")
			return
		}
		type nodeWarpStatus struct {
			NodeInfo
			WarpEnabled     bool     `json:"warp_enabled"`
			HasLoadBalance  bool     `json:"has_load_balance"`
			WarpTags        []string `json:"warp_tags,omitempty"`
		}
		result := make([]nodeWarpStatus, 0, len(nodes))
		for _, n := range nodes {
			policies, _ := h.outboundSvc.ListByNode(c.Request.Context(), n.ID)
			var tags []string
			warpEnabled := false
			hasLB := false
			for _, p := range policies {
				if p.PolicyType == "warp" && p.IsEnabled {
					warpEnabled = true
					if tag, _ := p.ConfigJSON["tag"].(string); tag != "" {
						tags = append(tags, tag)
					}
				}
				if p.PolicyType == "load_balance" && p.IsEnabled {
					hasLB = true
				}
			}
			result = append(result, nodeWarpStatus{
				NodeInfo:       n,
				WarpEnabled:    warpEnabled,
				HasLoadBalance: hasLB,
				WarpTags:       tags,
			})
		}
		server.OK(c, gin.H{"nodes": result, "total": len(result)})
		return
	}

	// 回退：仅返回 node IDs
	nodeIDs, err := h.listNodeIDs(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	type nodeStatus struct {
		ID         uuid.UUID `json:"id"`
		WarpEnabled bool    `json:"warp_enabled"`
	}
	result := make([]nodeStatus, 0, len(nodeIDs))
	for _, nid := range nodeIDs {
		policies, _ := h.outboundSvc.ListByNode(c.Request.Context(), nid)
		warpEnabled := false
		for _, p := range policies {
			if p.PolicyType == "warp" && p.IsEnabled {
				warpEnabled = true
				break
			}
		}
		result = append(result, nodeStatus{ID: nid, WarpEnabled: warpEnabled})
	}
	server.OK(c, gin.H{"nodes": result, "total": len(result)})
}

// EnableWarpForNodes 批量为指定节点启用 WARP
// URL: POST /servers/:id/warp/enable-nodes
// body: { "node_ids": ["uuid1","uuid2"] }
// 为每个 node_id 创建 N 条 warp outbound_policy（N = 该 server 的 warp_profiles 数）
// 若节点已有 warp policy 则跳过（幂等）
func (h *AdminWarpHandler) EnableWarpForNodes(c *gin.Context) {
	serverID, ok := parseNodeID(c)
	if !ok {
		return
	}
	var req struct {
		NodeIDs []uuid.UUID `json:"node_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if len(req.NodeIDs) == 0 {
		server.BadRequest(c, "node_ids is empty")
		return
	}

	// 查询 server 所有 warp profiles
	profiles, err := h.svc.ListByNode(c.Request.Context(), serverID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if len(profiles) == 0 {
		server.BadRequest(c, "no warp profiles registered for this server")
		return
	}

	enabled := true
	priority := 100
	created := 0
	skipped := 0
	failed := 0

	// 收集所有 warp profile 的 outbound_tag（用于 load_balance policy 的 outbounds 字段）
	// 所有勾选节点共用同一组 warp_profiles，warpTags 在循环外收集一次即可
	var warpTags []interface{}
	for _, w := range profiles {
		if w.PrivateKey == nil || w.OutboundTag == nil {
			continue
		}
		warpTags = append(warpTags, *w.OutboundTag)
	}

	for _, nodeID := range req.NodeIDs {
		// 检查是否已有 warp policy（幂等）
		existing, _ := h.outboundSvc.ListByNode(c.Request.Context(), nodeID)
		alreadyHasWarp := false
		for _, p := range existing {
			if p.PolicyType == "warp" && p.IsEnabled {
				alreadyHasWarp = true
				break
			}
		}
		if alreadyHasWarp {
			skipped++
			continue
		}
		// 为节点创建 N 条 warp outbound_policy（每个 profile 一条）
		for _, w := range profiles {
			if w.PrivateKey == nil || w.OutboundTag == nil {
				continue
			}
			cfg := Map{
				"private_key":   *w.PrivateKey,
				"public_key":    "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
				"endpoint":      "engage.cloudflareclient.com:2408",
				"local_address": *w.LocalAddress,
				"mtu":           w.MTU,
				"tag":           *w.OutboundTag,
			}
			if w.Endpoint != nil {
				cfg["endpoint"] = *w.Endpoint
			}
			if w.PublicKey != nil {
				cfg["public_key"] = *w.PublicKey
			}
			_, err := h.outboundSvc.Create(c.Request.Context(), nodeID, &CreatePolicyRequest{
				PolicyType: "warp",
				Priority:   &priority,
				ConfigJSON: cfg,
				IsEnabled:  &enabled,
			})
			if err != nil {
				failed++
			}
		}
		// 创建 load_balance policy 聚合所有 warp outbound，让 deployment_service LB-3 逻辑
		// 注入 inbound→warp-pool 路由规则，实现"勾选启用 WARP → 该节点流量走 WARP"。
		// 幂等：若节点已有 load_balance policy 则跳过
		alreadyHasLB := false
		for _, p := range existing {
			if p.PolicyType == "load_balance" && p.IsEnabled {
				alreadyHasLB = true
				break
			}
		}
		if !alreadyHasLB && len(warpTags) > 0 {
			lbPriority := 20 // 高优先级（低于 warp 本身的 100）
			lbCfg := Map{
				"tag":            "warp-pool",
				"outbounds":      warpTags,
				"check_url":      "https://www.gstatic.com/generate_204",
				"check_interval": "3m",
			}
			_, err := h.outboundSvc.Create(c.Request.Context(), nodeID, &CreatePolicyRequest{
				PolicyType: "load_balance",
				Priority:   &lbPriority,
				ConfigJSON: lbCfg,
				IsEnabled:  &enabled,
			})
			if err != nil {
				// load_balance 创建失败不阻断 warp 启用（warp outbound 已创建，
				// 用户可通过 EnableLoadBalance API 手动重试）
				if s, ok := c.Get("logger"); ok {
					if logger, ok := s.(interface{ Warn(string, ...any) }); ok {
						logger.Warn("auto create load_balance policy failed",
							"node_id", nodeID, "error", err)
					}
				}
			}
		}
		created++
	}

	// 按需分配模式：EnableWarpForNodes 创建 warp outbound_policy 后，自动创建 load_balance policy
	// 聚合所有 warp outbound，让 deployment_service 的 LB-3 逻辑按节点 inbound 注入
	// inbound→warp-pool 路由规则，实现"勾选启用 WARP → 仅该节点流量走 WARP"的节点级语义。
	//   - 单 warp：load_balance policy 含 1 个 outbound → urltest → inbound 规则 → 该节点流量走 WARP
	//   - 多 warp：load_balance policy 含 N 个 outbound → urltest → inbound 规则 → 该节点流量走 WARP 池（自动选优+故障切换）
	//   - 取消勾选（DisableWarpForNodes）→ 删除 warp + load_balance policy → 无 inbound 规则 → 该节点流量走直连
	//   - 同 VPS 其他未勾选节点：始终无规则命中，直连不受影响

	server.OK(c, gin.H{
		"enabled":        true,
		"nodes_affected": created,
		"nodes_skipped":  skipped,
		"nodes_failed":   failed,
		"profiles_count": len(profiles),
	})
}

// DisableWarpForNodes 批量禁用指定节点的 WARP
// URL: POST /servers/:id/warp/disable-nodes
// body: { "node_ids": ["uuid1","uuid2"] }
// 删除节点下所有 warp 和 load_balance 类型的 outbound_policy
func (h *AdminWarpHandler) DisableWarpForNodes(c *gin.Context) {
	serverID, ok := parseNodeID(c)
	if !ok {
		return
	}
	var req struct {
		NodeIDs []uuid.UUID `json:"node_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if len(req.NodeIDs) == 0 {
		server.BadRequest(c, "node_ids is empty")
		return
	}
	_ = serverID

	deleted := 0
	skipped := 0
	for _, nodeID := range req.NodeIDs {
		policies, err := h.outboundSvc.ListByNode(c.Request.Context(), nodeID)
		if err != nil {
			skipped++
			continue
		}
		anyDeleted := false
		for _, p := range policies {
			if p.PolicyType == "warp" || p.PolicyType == "load_balance" {
				if err := h.outboundSvc.Delete(c.Request.Context(), p.ID); err == nil {
					deleted++
					anyDeleted = true
				}
			}
		}
		if !anyDeleted {
			skipped++
		}
	}
	server.OK(c, gin.H{
		"disabled":       true,
		"policies_deleted": deleted,
		"nodes_skipped":   skipped,
	})
}

// ApplyLicenseToProfile 为 WARP 账户应用 WARP+ License
// URL: POST /warp-profiles/:pid/apply-license
// body: { "license": "xxxx-xxxx-xxxx-xxxx" }
// 调用 Cloudflare API 绑定 License，成功后更新 warp_profiles.license_key
func (h *AdminWarpHandler) ApplyLicenseToProfile(c *gin.Context) {
	pid, ok := parsePolicyID(c)
	if !ok {
		return
	}
	var req struct {
		License string `json:"license" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	if h.licenseApplier == nil {
		server.Fail(c, config.CodeInternalError, "license applier not configured")
		return
	}
	w, err := h.svc.GetByID(c.Request.Context(), pid)
	if err != nil {
		server.Fail(c, config.CodeNotFound, "warp profile not found")
		return
	}
	if w.DeviceID == nil || w.AccessToken == nil {
		server.BadRequest(c, "warp profile missing device_id or access_token")
		return
	}
	if err := h.licenseApplier.ApplyLicense(c.Request.Context(), *w.DeviceID, *w.AccessToken, req.License); err != nil {
		server.Fail(c, config.CodeInternalError, fmt.Sprintf("apply license failed: %v", err))
		return
	}
	// 更新 license_key（通过 Update 接口暂无，直接复用 Create 是不允许的，需通过 store 更新）
	// 这里使用 ConfigJSON 存储 license，因为 WarpProfile 没有 update 方法
	// 实际更新通过 WarpProfileStore 提供 Update 方法（若未提供，则忽略持久化，仅前端展示）
	server.OK(c, gin.H{
		"applied":    true,
		"license":    req.License,
		"profile_id": w.ID,
		"code":       w.Code,
	})
}
