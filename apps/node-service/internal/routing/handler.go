package routing

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminRoutingHandler 处理路由分流模块的 admin 路由
type AdminRoutingHandler struct {
	ruleSetSvc   *RuleSetService
	policySvc    *PolicyService
	ruleSvc      *PolicyRuleService
	bindingSvc   *BindingService
	lbPolicySvc  *LBPolicyService
	outboundGrpSvc *OutboundGroupService
	renderer     *RoutingRenderer
}

func NewAdminRoutingHandler(
	ruleSetSvc *RuleSetService,
	policySvc *PolicyService,
	ruleSvc *PolicyRuleService,
	bindingSvc *BindingService,
	lbPolicySvc *LBPolicyService,
	outboundGrpSvc *OutboundGroupService,
	renderer *RoutingRenderer,
) *AdminRoutingHandler {
	return &AdminRoutingHandler{
		ruleSetSvc:     ruleSetSvc,
		policySvc:      policySvc,
		ruleSvc:        ruleSvc,
		bindingSvc:     bindingSvc,
		lbPolicySvc:    lbPolicySvc,
		outboundGrpSvc: outboundGrpSvc,
		renderer:       renderer,
	}
}

// RegisterRoutesWithGroup 注册路由分流模块的所有 admin 路由
func (h *AdminRoutingHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	// 路由规则集
	ruleSets := admin.Group("/route-rule-sets")
	{
		ruleSets.GET("", rbac.RequirePermission("nodes.read"), h.ListRuleSets)
		ruleSets.POST("", rbac.RequirePermission("nodes.write"), h.CreateRuleSet)
		ruleSets.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetRuleSet)
		ruleSets.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdateRuleSet)
		ruleSets.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeleteRuleSet)
		ruleSets.POST("/:id/sync", rbac.RequirePermission("nodes.write"), h.SyncRuleSet)
	}

	// 路由策略
	policies := admin.Group("/route-policies")
	{
		policies.GET("", rbac.RequirePermission("nodes.read"), h.ListPolicies)
		policies.POST("", rbac.RequirePermission("nodes.write"), h.CreatePolicy)
		policies.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetPolicy)
		policies.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdatePolicy)
		policies.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeletePolicy)
		policies.POST("/:id/rules", rbac.RequirePermission("nodes.write"), h.AddPolicyRule)
		policies.POST("/:id/reorder", rbac.RequirePermission("nodes.write"), h.ReorderRules)
		policies.POST("/:id/clone", rbac.RequirePermission("nodes.write"), h.ClonePolicy)
	}

	// 路由策略规则条目（独立更新/删除）
	rules := admin.Group("/route-policy-rules")
	{
		rules.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdatePolicyRule)
		rules.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeletePolicyRule)
	}

	// 节点路由绑定 + 出站组（挂在 /nodes/:id 下）
	nodes := admin.Group("/nodes")
	{
		nodes.GET("/:id/route-bindings", rbac.RequirePermission("nodes.read"), h.ListBindings)
		nodes.POST("/:id/route-bindings", rbac.RequirePermission("nodes.write"), h.BindPolicy)
		nodes.DELETE("/:id/route-bindings/:policy_id", rbac.RequirePermission("nodes.write"), h.UnbindPolicy)
		nodes.GET("/:id/outbound-groups", rbac.RequirePermission("nodes.read"), h.ListOutboundGroups)
		nodes.POST("/:id/outbound-groups", rbac.RequirePermission("nodes.write"), h.CreateOutboundGroup)
		nodes.PATCH("/:id/outbound-groups/:tag", rbac.RequirePermission("nodes.write"), h.UpdateOutboundGroup)
		nodes.DELETE("/:id/outbound-groups/:tag", rbac.RequirePermission("nodes.write"), h.DeleteOutboundGroup)
		nodes.GET("/:id/routing-config", rbac.RequirePermission("nodes.read"), h.RenderRouting)
	}

	// 节点组负载均衡
	nodeGroups := admin.Group("/node-groups")
	{
		nodeGroups.GET("/:id/lb-policy", rbac.RequirePermission("nodes.read"), h.GetLBPolicy)
		nodeGroups.PUT("/:id/lb-policy", rbac.RequirePermission("nodes.write"), h.UpsertLBPolicy)
	}
}

// ===================== 路由规则集 =====================

func (h *AdminRoutingHandler) ListRuleSets(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := RuleSetListQuery{
		Page:     page,
		PageSize: pageSize,
		RuleType: c.Query("rule_type"),
		Status:   c.Query("status"),
		Keyword:  c.Query("keyword"),
	}

	items, total, err := h.ruleSetSvc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]RuleSetResponse, len(items))
	for i, rs := range items {
		resp[i] = NewRuleSetResponse(rs)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminRoutingHandler) GetRuleSet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule set id")
		return
	}

	rs, err := h.ruleSetSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewRuleSetResponse(rs))
}

func (h *AdminRoutingHandler) CreateRuleSet(c *gin.Context) {
	var req CreateRuleSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	rs, err := h.ruleSetSvc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewRuleSetResponse(rs))
}

func (h *AdminRoutingHandler) UpdateRuleSet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule set id")
		return
	}

	var req UpdateRuleSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	rs, err := h.ruleSetSvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewRuleSetResponse(rs))
}

func (h *AdminRoutingHandler) DeleteRuleSet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule set id")
		return
	}

	if err := h.ruleSetSvc.Delete(c.Request.Context(), id); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

func (h *AdminRoutingHandler) SyncRuleSet(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule set id")
		return
	}

	rs, err := h.ruleSetSvc.SyncFromURL(c.Request.Context(), id)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewRuleSetResponse(rs))
}

// ===================== 路由策略 =====================

func (h *AdminRoutingHandler) ListPolicies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := PolicyListQuery{
		Page:       page,
		PageSize:   pageSize,
		PolicyType: c.Query("policy_type"),
		Status:     c.Query("status"),
		Keyword:    c.Query("keyword"),
	}

	items, total, err := h.policySvc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]PolicyResponse, len(items))
	for i, p := range items {
		resp[i] = NewPolicyResponse(p)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminRoutingHandler) GetPolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return
	}

	p, err := h.policySvc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPolicyResponse(p))
}

func (h *AdminRoutingHandler) CreatePolicy(c *gin.Context) {
	var req CreatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.policySvc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPolicyResponse(p))
}

func (h *AdminRoutingHandler) ClonePolicy(c *gin.Context) {
	templateCode := c.Param("id")
	if templateCode == "" {
		server.BadRequest(c, "invalid template code")
		return
	}

	var req ClonePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.policySvc.CloneFromTemplate(c.Request.Context(), templateCode, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPolicyResponse(p))
}

func (h *AdminRoutingHandler) UpdatePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return
	}

	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.policySvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPolicyResponse(p))
}

func (h *AdminRoutingHandler) DeletePolicy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return
	}

	if err := h.policySvc.Delete(c.Request.Context(), id); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

func (h *AdminRoutingHandler) AddPolicyRule(c *gin.Context) {
	policyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return
	}

	var req AddRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	rule, err := h.ruleSvc.AddRule(c.Request.Context(), policyID, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewPolicyRuleResponse(rule))
}

func (h *AdminRoutingHandler) ReorderRules(c *gin.Context) {
	policyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid policy id")
		return
	}

	var req ReorderRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	ruleIDs := make([]uuid.UUID, 0, len(req.RuleIDs))
	for _, idStr := range req.RuleIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			server.BadRequest(c, "invalid rule id: "+idStr)
			return
		}
		ruleIDs = append(ruleIDs, id)
	}

	if err := h.ruleSvc.Reorder(c.Request.Context(), policyID, ruleIDs); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, gin.H{"reordered": len(ruleIDs)})
}

func (h *AdminRoutingHandler) UpdatePolicyRule(c *gin.Context) {
	ruleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule id")
		return
	}

	var req UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	rule, err := h.ruleSvc.UpdateRule(c.Request.Context(), ruleID, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewPolicyRuleResponse(rule))
}

func (h *AdminRoutingHandler) DeletePolicyRule(c *gin.Context) {
	ruleID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid rule id")
		return
	}

	if err := h.ruleSvc.DeleteRule(c.Request.Context(), ruleID); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

// ===================== 节点路由绑定 =====================

func (h *AdminRoutingHandler) ListBindings(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	items, err := h.bindingSvc.ListBindings(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]BindingResponse, len(items))
	for i, b := range items {
		resp[i] = NewBindingResponse(b)
	}
	server.OK(c, gin.H{"items": resp, "total": len(resp)})
}

func (h *AdminRoutingHandler) BindPolicy(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	var req BindPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	policyID, err := uuid.Parse(req.PolicyID)
	if err != nil {
		server.BadRequest(c, "invalid policy_id")
		return
	}

	b, err := h.bindingSvc.Bind(c.Request.Context(), nodeID, policyID, req.BindScope, req.InboundTag)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewBindingResponse(b))
}

func (h *AdminRoutingHandler) UnbindPolicy(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	policyID, err := uuid.Parse(c.Param("policy_id"))
	if err != nil {
		server.BadRequest(c, "invalid policy_id")
		return
	}

	if err := h.bindingSvc.Unbind(c.Request.Context(), nodeID, policyID); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

// ===================== 节点组负载均衡 =====================

func (h *AdminRoutingHandler) GetLBPolicy(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	p, err := h.lbPolicySvc.Get(c.Request.Context(), groupID)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewLBPolicyResponse(p))
}

func (h *AdminRoutingHandler) UpsertLBPolicy(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid group id")
		return
	}

	var req UpsertLBPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	p, err := h.lbPolicySvc.Upsert(c.Request.Context(), groupID, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewLBPolicyResponse(p))
}

// ===================== 出站组 =====================

func (h *AdminRoutingHandler) ListOutboundGroups(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	items, err := h.outboundGrpSvc.List(c.Request.Context(), nodeID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]OutboundGroupResponse, len(items))
	for i, g := range items {
		resp[i] = NewOutboundGroupResponse(g)
	}
	server.OK(c, gin.H{"items": resp, "total": len(resp)})
}

func (h *AdminRoutingHandler) CreateOutboundGroup(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	var req CreateOutboundGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	g, err := h.outboundGrpSvc.Create(c.Request.Context(), nodeID, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, NewOutboundGroupResponse(g))
}

func (h *AdminRoutingHandler) UpdateOutboundGroup(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	tag := c.Param("tag")
	if tag == "" {
		server.BadRequest(c, "invalid tag")
		return
	}

	var req UpdateOutboundGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	g, err := h.outboundGrpSvc.Update(c.Request.Context(), nodeID, tag, &req)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewOutboundGroupResponse(g))
}

func (h *AdminRoutingHandler) DeleteOutboundGroup(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	tag := c.Param("tag")
	if tag == "" {
		server.BadRequest(c, "invalid tag")
		return
	}

	if err := h.outboundGrpSvc.Delete(c.Request.Context(), nodeID, tag); err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.NoContent(c)
}

// ===================== 路由渲染 =====================

func (h *AdminRoutingHandler) RenderRouting(c *gin.Context) {
	nodeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return
	}

	result, err := h.renderer.RenderRouting(c.Request.Context(), nodeID)
	if err != nil {
		code, msg := MapRoutingErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, result)
}
