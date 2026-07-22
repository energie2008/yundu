package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminPlanHandler struct {
	planSvc *service.PlanService
}

func NewAdminPlanHandler(planSvc *service.PlanService) *AdminPlanHandler {
	return &AdminPlanHandler{planSvc: planSvc}
}

func (h *AdminPlanHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	plans := admin.Group("/plans")
	{
		plans.GET("", rbac.RequirePermission("plans.read"), h.ListPlans)
		plans.GET("/:id", rbac.RequirePermission("plans.read"), h.GetPlan)
		plans.POST("", rbac.RequirePermission("plans.write"), h.CreatePlan)
		plans.PATCH("/:id", rbac.RequirePermission("plans.write"), h.UpdatePlan)
		plans.DELETE("/:id", rbac.RequirePermission("plans.write"), h.DeletePlan)
		// 节点-套餐绑定管理
		plans.GET("/:id/nodes", rbac.RequirePermission("plans.read"), h.ListPlanNodes)
		plans.PUT("/:id/nodes", rbac.RequirePermission("plans.write"), h.ReplacePlanNodes)
	}
}

// ListPlanNodes 获取套餐已绑定的节点列表（admin 版本）
func (h *AdminPlanHandler) ListPlanNodes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid plan id")
		return
	}
	nodes, err := h.planSvc.ListNodesForPlan(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": nodes, "total": len(nodes)})
}

// ReplacePlanNodes 批量替换套餐的节点绑定
// 请求体: {"node_ids": ["uuid1", "uuid2", ...]}
// node_ids 为空数组表示解绑所有节点
func (h *AdminPlanHandler) ReplacePlanNodes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid plan id")
		return
	}
	var req struct {
		NodeIDs []string `json:"node_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	nodeIDs := make([]uuid.UUID, 0, len(req.NodeIDs))
	for _, s := range req.NodeIDs {
		uid, err := uuid.Parse(s)
		if err != nil {
			server.BadRequest(c, "invalid node id: "+s)
			return
		}
		nodeIDs = append(nodeIDs, uid)
	}
	if err := h.planSvc.ReplacePlanNodes(c.Request.Context(), id, nodeIDs); err != nil {
		code, msg := service.MapPlanErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, gin.H{"plan_id": id, "node_count": len(nodeIDs)})
}

func (h *AdminPlanHandler) ListPlans(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	query := model.PlanListQuery{
		Page:        page,
		PageSize:    pageSize,
		Status:      c.Query("status"),
		BillingType: c.Query("billing_type"),
	}

	items, total, err := h.planSvc.List(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]model.PlanResponse, len(items))
	for i, p := range items {
		pr := model.NewPlanResponse(p)
		prices := make([]model.PlanPrice, 0)
		for period, entry := range p.Prices {
			prices = append(prices, model.PlanPrice{
				PeriodCode: period,
				PriceUSDT:  entry.USDT,
				PriceCNY:   entry.CNY,
			})
		}
		pr.Prices = prices
		resp[i] = pr
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

func (h *AdminPlanHandler) GetPlan(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid plan id")
		return
	}

	p, err := h.planSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapPlanErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	pr := model.NewPlanResponse(p)
	prices := make([]model.PlanPrice, 0)
	for period, entry := range p.Prices {
		prices = append(prices, model.PlanPrice{
			PeriodCode: period,
			PriceUSDT:  entry.USDT,
			PriceCNY:   entry.CNY,
		})
	}
	pr.Prices = prices

	server.OK(c, pr)
}

func (h *AdminPlanHandler) CreatePlan(c *gin.Context) {
	var req model.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	p, err := h.planSvc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapPlanErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	pr := model.NewPlanResponse(p)
	prices := make([]model.PlanPrice, 0)
	for period, entry := range p.Prices {
		prices = append(prices, model.PlanPrice{
			PeriodCode: period,
			PriceUSDT:  entry.USDT,
			PriceCNY:   entry.CNY,
		})
	}
	pr.Prices = prices

	server.Created(c, pr)
}

func (h *AdminPlanHandler) UpdatePlan(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid plan id")
		return
	}

	var req model.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	p, err := h.planSvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := service.MapPlanErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	pr := model.NewPlanResponse(p)
	prices := make([]model.PlanPrice, 0)
	for period, entry := range p.Prices {
		prices = append(prices, model.PlanPrice{
			PeriodCode: period,
			PriceUSDT:  entry.USDT,
			PriceCNY:   entry.CNY,
		})
	}
	pr.Prices = prices

	server.OK(c, pr)
}

func (h *AdminPlanHandler) DeletePlan(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid plan id")
		return
	}

	if err := h.planSvc.Delete(c.Request.Context(), id); err != nil {
		code, msg := service.MapPlanErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}
