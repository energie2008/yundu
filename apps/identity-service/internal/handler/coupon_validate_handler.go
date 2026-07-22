package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CouponValidateRequest 用户端优惠券校验请求
type CouponValidateRequest struct {
	CouponCode string  `json:"coupon_code" binding:"required"`
	PlanID     string  `json:"plan_id"`
	PeriodCode string  `json:"period_code"`
	AmountCNY  float64 `json:"amount_cny" binding:"min=0"`
}

// CouponValidateResponse 用户端优惠券校验响应
type CouponValidateResponse struct {
	Valid          bool    `json:"valid"`
	CouponCode     string  `json:"coupon_code"`
	DiscountType   string  `json:"discount_type"`
	DiscountAmount float64 `json:"discount_amount"`
	FinalAmount    float64 `json:"final_amount"`
}

// CouponValidateHandler 用户端优惠券校验（下单前预校验折扣金额）
type CouponValidateHandler struct {
	couponSvc *service.CouponService
}

// NewCouponValidateHandler 创建用户端优惠券校验 handler
func NewCouponValidateHandler(couponSvc *service.CouponService) *CouponValidateHandler {
	return &CouponValidateHandler{couponSvc: couponSvc}
}

// Validate 校验优惠券并返回折扣信息
// POST /api/v1/coupons/validate
// Body: { coupon_code, plan_id, period_code, amount_cny }
// Response: { valid, coupon_code, discount_type, discount_amount, final_amount }
func (h *CouponValidateHandler) Validate(c *gin.Context) {
	var req CouponValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	// 解析 plan_id（可选，为空表示不限制套餐）
	var planID uuid.UUID
	if req.PlanID != "" {
		pid, err := uuid.Parse(req.PlanID)
		if err != nil {
			server.BadRequest(c, "invalid plan_id")
			return
		}
		planID = pid
	}

	// 获取优惠券详情（用于返回 discount_type）
	coupon, err := h.couponSvc.GetByCode(c.Request.Context(), req.CouponCode)
	if err != nil || coupon == nil {
		// 优惠券不存在：返回 valid=false
		server.OK(c, CouponValidateResponse{
			Valid:      false,
			CouponCode: req.CouponCode,
		})
		return
	}

	// 调用现有的 CouponService.ValidateCoupon 校验并计算折扣金额
	discount, err := h.couponSvc.ValidateCoupon(
		c.Request.Context(),
		req.CouponCode,
		userID,
		req.AmountCNY,
		planID,
		req.PeriodCode,
	)
	if err != nil {
		// 校验失败（过期/已用完/不满足最低消费等）：返回 valid=false 并附带 discount_type
		server.OK(c, CouponValidateResponse{
			Valid:        false,
			CouponCode:   req.CouponCode,
			DiscountType: coupon.DiscountType,
		})
		return
	}

	finalAmount := req.AmountCNY - discount
	if finalAmount < 0 {
		finalAmount = 0
	}

	server.OK(c, CouponValidateResponse{
		Valid:          true,
		CouponCode:     req.CouponCode,
		DiscountType:   coupon.DiscountType,
		DiscountAmount: discount,
		FinalAmount:    finalAmount,
	})
}
