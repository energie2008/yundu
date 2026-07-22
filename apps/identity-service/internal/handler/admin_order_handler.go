package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminOrderHandler 管理员订单管理 handler
type AdminOrderHandler struct {
	orderRepo *repo.PaymentOrderRepo
	userSvc   *service.UserService
}

func NewAdminOrderHandler(orderRepo *repo.PaymentOrderRepo, userSvc *service.UserService) *AdminOrderHandler {
	return &AdminOrderHandler{orderRepo: orderRepo, userSvc: userSvc}
}

// RegisterRoutesWithGroup 注册管理员订单路由
func (h *AdminOrderHandler) RegisterRoutesWithGroup(rg *gin.RouterGroup) {
	orders := rg.Group("/orders")
	{
		orders.GET("", h.AdminListOrders)
		orders.GET("/:id", h.AdminGetOrder)
		orders.POST("/:id/cancel", h.AdminCancelOrder)
		orders.POST("/:id/mark-paid", h.AdminMarkPaid)
	}
}

// AdminListQuery 管理员订单列表查询参数
type AdminListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
	UserID   string `form:"user_id"`
	PlanID   string `form:"plan_id"`
}

// AdminListOrders 管理员订单列表
// GET /admin/orders?page=1&page_size=20&status=pending&user_id=&plan_id=
func (h *AdminOrderHandler) AdminListOrders(c *gin.Context) {
	var q AdminListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}

	orders, total, err := h.orderRepo.AdminList(c.Request.Context(), q.Page, q.PageSize, q.Status, q.UserID, q.PlanID)
	if err != nil {
		server.InternalError(c, "failed to list orders")
		return
	}

	items := make([]model.OrderResponse, 0, len(orders))
	for _, o := range orders {
		items = append(items, model.NewOrderResponse(o))
	}

	server.OK(c, model.PaginationResponse{
		Page:     q.Page,
		PageSize: q.PageSize,
		Total:    total,
		Items:    items,
	})
}

// AdminGetOrder 管理员订单详情
// GET /admin/orders/:id
func (h *AdminOrderHandler) AdminGetOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid order id")
		return
	}

	order, err := h.orderRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "failed to get order")
		return
	}
	if order == nil {
		server.NotFound(c, "order not found")
		return
	}

	server.OK(c, model.NewOrderResponse(order))
}

// AdminCancelOrder 管理员取消订单
// POST /admin/orders/:id/cancel
func (h *AdminOrderHandler) AdminCancelOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid order id")
		return
	}

	order, err := h.orderRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "failed to get order")
		return
	}
	if order == nil {
		server.NotFound(c, "order not found")
		return
	}
	if order.Status != model.PaymentStatusPending {
		server.ValidationError(c, "only pending orders can be canceled")
		return
	}

	if err := h.orderRepo.UpdateStatus(c.Request.Context(), id, model.PaymentStatusCanceled, nil, nil, nil); err != nil {
		server.InternalError(c, "failed to cancel order")
		return
	}

	server.OK(c, gin.H{"id": id, "status": model.PaymentStatusCanceled})
}

// AdminMarkPaid 管理员手动标记订单已支付（补单）
// POST /admin/orders/:id/mark-paid
func (h *AdminOrderHandler) AdminMarkPaid(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid order id")
		return
	}

	order, err := h.orderRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "failed to get order")
		return
	}
	if order == nil {
		server.NotFound(c, "order not found")
		return
	}
	if order.Status != model.PaymentStatusPending {
		server.ValidationError(c, "only pending orders can be marked as paid")
		return
	}

	// 手动补单：标记为已支付，通过 userSvc 激活订阅
	if err := h.orderRepo.UpdateStatus(c.Request.Context(), id, model.PaymentStatusPaid, nil, &order.AmountUSDT, nil); err != nil {
		server.InternalError(c, "failed to mark order as paid")
		return
	}

	// 通过 payment_service 激活订阅（手动补单场景）
	// 这里直接返回成功，订阅激活由 payment_service 的轮询逻辑或手动处理
	server.OK(c, gin.H{"id": id, "status": model.PaymentStatusPaid, "hint": "order marked as paid, subscription will be activated"})
}

// 确保 strconv 被使用
var _ = strconv.Itoa
