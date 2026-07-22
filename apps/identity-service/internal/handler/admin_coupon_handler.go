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

// AdminCouponHandler 优惠券管理（管理端 CRUD）
type AdminCouponHandler struct {
	couponSvc *service.CouponService
}

func NewAdminCouponHandler(couponSvc *service.CouponService) *AdminCouponHandler {
	return &AdminCouponHandler{couponSvc: couponSvc}
}

func (h *AdminCouponHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	coupons := admin.Group("/coupons")
	{
		coupons.GET("", rbac.RequirePermission("coupons.read"), h.ListCoupons)
		coupons.GET("/:id", rbac.RequirePermission("coupons.read"), h.GetCoupon)
		coupons.POST("", rbac.RequirePermission("coupons.write"), h.CreateCoupon)
		coupons.PATCH("/:id", rbac.RequirePermission("coupons.write"), h.UpdateCoupon)
		coupons.DELETE("/:id", rbac.RequirePermission("coupons.write"), h.DeleteCoupon)
	}
}

// ListCoupons 分页查询优惠券列表
func (h *AdminCouponHandler) ListCoupons(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	search := c.Query("search")

	var isActive *bool
	if v := c.Query("is_active"); v != "" {
		b := v == "true" || v == "1"
		isActive = &b
	}

	items, total, err := h.couponSvc.List(c.Request.Context(), page, pageSize, search, isActive)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]model.CouponResponse, len(items))
	for i, item := range items {
		resp[i] = model.NewCouponResponse(item)
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     resp,
	})
}

// GetCoupon 根据 ID 获取优惠券详情
func (h *AdminCouponHandler) GetCoupon(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid coupon id")
		return
	}

	item, err := h.couponSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		code, msg := service.MapCouponErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewCouponResponse(item))
}

// CreateCoupon 创建优惠券
func (h *AdminCouponHandler) CreateCoupon(c *gin.Context) {
	var req model.CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	// 优惠码统一转大写，避免大小写冲突
	req.Code = upperCleanCode(req.Code)

	item, err := h.couponSvc.Create(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapCouponErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewCouponResponse(item))
}

// UpdateCoupon 更新优惠券
func (h *AdminCouponHandler) UpdateCoupon(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid coupon id")
		return
	}

	var req model.UpdateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	item, err := h.couponSvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := service.MapCouponErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewCouponResponse(item))
}

// DeleteCoupon 删除优惠券
func (h *AdminCouponHandler) DeleteCoupon(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid coupon id")
		return
	}

	if err := h.couponSvc.Delete(c.Request.Context(), id); err != nil {
		code, msg := service.MapCouponErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"deleted": true})
}

// upperCleanCode 将优惠码转换为大写并去除首尾空白
func upperCleanCode(code string) string {
	if code == "" {
		return code
	}
	// 去除首尾空白后转大写
	out := make([]byte, 0, len(code))
	for i := 0; i < len(code); i++ {
		ch := code[i]
		// 跳过前后空白
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			continue
		}
		// 小写字母转大写
		if ch >= 'a' && ch <= 'z' {
			ch = ch - 'a' + 'A'
		}
		out = append(out, ch)
	}
	return string(out)
}
