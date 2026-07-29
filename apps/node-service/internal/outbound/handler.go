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

// AdminWarpHandler 处理 WARP 档案的 admin 路由
// 架构说明：
//   - warp_profiles.node_id 引用 servers(id) → WARP 账户按 VPS 维度管理
//   - outbound_policies.node_id 引用 nodes(id) → 出站策略按节点维度管理
//   - 注册/导入 WARP 账户时，自动为该 VPS 下所有启用节点创建 warp outbound_policy
//   - 启用负载均衡时，自动为该 VPS 下所有启用节点创建 load_balance outbound_policy
type AdminWarpHandler struct {
	svc         *WarpProfileService
	outboundSvc *OutboundService
	nodeLister  NodeIDLister
}

func NewAdminWarpHandler(svc *WarpProfileService, outboundSvc *OutboundService, nodeLister NodeIDLister) *AdminWarpHandler {
	return &AdminWarpHandler{svc: svc, outboundSvc: outboundSvc, nodeLister: nodeLister}
}

func (h *AdminWarpHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	warp := admin.Group("/warp-profiles")
	{
		warp.GET("", rbac.RequirePermission("nodes.read"), h.ListWarpProfiles)
		warp.POST("", rbac.RequirePermission("nodes.write"), h.CreateWarpProfile)
		warp.DELETE("/:pid", rbac.RequirePermission("nodes.write"), h.DeleteWarpProfile)
	}
	// 全部按服务器维度操作（warp_profiles.node_id 引用 servers 表）
	servers := admin.Group("/servers")
	{
		servers.GET("/:id/warp/status", rbac.RequirePermission("nodes.read"), h.GetServerWarpStatus)
		servers.GET("/:id/warp-profiles", rbac.RequirePermission("nodes.read"), h.ListNodeWarpProfiles)
		servers.POST("/:id/warp/register", rbac.RequirePermission("nodes.write"), h.RegisterWarpForNode)
		servers.POST("/:id/warp/import", rbac.RequirePermission("nodes.write"), h.ImportWarpForNode)
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

func (h *AdminWarpHandler) RegisterWarpForNode(c *gin.Context) {
	serverID, ok := parseNodeID(c) // URL :id 是 servers.id
	if !ok {
		return
	}
	if h.svc.pool == nil {
		server.Fail(c, config.CodeInternalError, "warp registration not configured")
		return
	}
	nodeCode := c.Query("code")
	if nodeCode == "" {
		nodeCode = "node"
	}
	w, err := h.svc.pool.RegisterForNode(c.Request.Context(), serverID, nodeCode)
	if err != nil {
		server.Fail(c, config.CodeInternalError, err.Error())
		return
	}
	// 为该 VPS 下所有启用节点创建 warp outbound_policy
	h.createWarpOutboundForNodes(c.Request.Context(), serverID, w)
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
	// 为该 VPS 下所有启用节点创建 warp outbound_policy
	h.createWarpOutboundForNodes(c.Request.Context(), serverID, w)
	server.Created(c, NewWarpProfileResponse(w))
}

// EnableLoadBalance 为服务器下所有节点创建 load_balance outbound_policy
// URL: POST /servers/:id/warp/enable-load-balance
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
	if len(profiles) < 2 {
		server.BadRequest(c, "at least 2 warp profiles required for load balancing")
		return
	}

	// 收集 outbound tags
	var outboundTags []interface{}
	for _, p := range profiles {
		if p.OutboundTag != nil {
			outboundTags = append(outboundTags, *p.OutboundTag)
		}
	}
	if len(outboundTags) < 2 {
		server.BadRequest(c, "at least 2 warp outbound tags required")
		return
	}

	// 为该 VPS 下所有启用节点创建 load_balance outbound_policy
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
	for _, nid := range nodeIDs {
		_, err := h.outboundSvc.Create(c.Request.Context(), nid, &CreatePolicyRequest{
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
		"enabled":         true,
		"strategy":        strategy,
		"nodes_affected":  created,
		"total_nodes":     len(nodeIDs),
		"outbound_tags":   outboundTags,
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
