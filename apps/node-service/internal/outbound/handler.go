package outbound

import (
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

// AdminWarpHandler 处理 WARP 档案的 admin 路由
type AdminWarpHandler struct {
	svc *WarpProfileService
}

func NewAdminWarpHandler(svc *WarpProfileService) *AdminWarpHandler {
	return &AdminWarpHandler{svc: svc}
}

func (h *AdminWarpHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	warp := admin.Group("/warp-profiles")
	{
		warp.GET("", rbac.RequirePermission("nodes.read"), h.ListWarpProfiles)
		warp.POST("", rbac.RequirePermission("nodes.write"), h.CreateWarpProfile)
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
