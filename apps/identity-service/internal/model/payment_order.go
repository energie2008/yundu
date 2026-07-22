package model

import (
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusExpired  = "expired"
	PaymentStatusCanceled = "canceled"

	PaymentMethodUSDTTRC20 = "usdt_trc20"
	PaymentMethodUSDTERC20 = "usdt_erc20"
	PaymentMethodWechat    = "wechat"
	PaymentMethodAlipay    = "alipay"
	PaymentMethodZero      = "zero_amount"
)

// IsFiatPayment 判断是否为法币支付（微信/支付宝）
func IsFiatPayment(method string) bool {
	return method == PaymentMethodWechat || method == PaymentMethodAlipay
}

// IsUSDPayment 判断是否为 USDT 加密货币支付
func IsUSDPayment(method string) bool {
	return method == PaymentMethodUSDTTRC20 || method == PaymentMethodUSDTERC20
}

type CreateOrderRequest struct {
	PlanID        uuid.UUID `json:"plan_id" binding:"required"`
	PeriodCode    string    `json:"period_code" binding:"required,oneof=month quarter half_year year onetime"`
	CouponCode    string    `json:"coupon_code,omitempty"`
	PaymentMethod string    `json:"payment_method,omitempty"`
}

type OrderListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

type OrderResponse struct {
	ID             uuid.UUID  `json:"id"`
	OrderNo        string     `json:"order_no"`
	PlanID         uuid.UUID  `json:"plan_id"`
	PlanName       string     `json:"plan_name"`
	PeriodCode     string     `json:"period_code"`
	AmountUSDT     float64    `json:"amount_usdt"`
	AmountCNY      float64    `json:"amount_cny"`
	ExchangeRate   float64    `json:"exchange_rate"`
	DiscountAmount float64    `json:"discount_amount"`
	FinalAmount    float64    `json:"final_amount"`
	CouponCode     string     `json:"coupon_code,omitempty"`
	PayAddress     string     `json:"pay_address"`
	PayCurrency    string     `json:"pay_currency"`
	PaymentMethod  string     `json:"payment_method"`
	PaymentURI     string     `json:"payment_uri,omitempty"`
	Status         string     `json:"status"`
	TxHash         *string    `json:"tx_hash,omitempty"`
	PaidAmount     *float64   `json:"paid_amount,omitempty"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type PaymentAddressResponse struct {
	PayAddress  string    `json:"pay_address"`
	AmountUSDT  float64   `json:"amount_usdt"`
	ExpiresAt   time.Time `json:"expires_at"`
	TRC20URI    string    `json:"trc20_uri"`
	Currency    string    `json:"currency"`
	OrderNo     string    `json:"order_no"`
}

type Coupon struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	Code            string      `json:"code" db:"code"`
	Name            string      `json:"name" db:"name"`
	DiscountType    string      `json:"discount_type" db:"discount_type"`
	DiscountValue   float64     `json:"discount_value" db:"discount_value"`
	MaxUses         int         `json:"max_uses" db:"max_uses"`
	UsedCount       int         `json:"used_count" db:"used_count"`
	MinOrderAmount  float64     `json:"min_order_amount" db:"min_order_amount"`
	PlanID          *uuid.UUID  `json:"plan_id,omitempty" db:"plan_id"`
	LimitUseByUser  int         `json:"limit_use_by_user" db:"limit_use_by_user"`
	LimitPlanIDs    []uuid.UUID `json:"limit_plan_ids,omitempty" db:"limit_plan_ids"`
	NewUserOnly     bool        `json:"new_user_only" db:"new_user_only"`
	// LimitPeriod 限制可用周期（month/quarter/year 等），空=不限制
	LimitPeriod     []string    `json:"limit_period,omitempty" db:"limit_period"`
	// MaxDiscount 最大折扣金额上限（0=不限制）
	MaxDiscount     float64     `json:"max_discount" db:"max_discount"`
	// IsRepeatable 是否可重复使用（false=一次性券，全局仅可用一次）
	IsRepeatable    bool        `json:"is_repeatable" db:"is_repeatable"`
	StartsAt        *time.Time  `json:"starts_at,omitempty" db:"starts_at"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty" db:"expires_at"`
	IsActive        bool        `json:"is_active" db:"is_active"`
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at" db:"updated_at"`
	Discount        float64     `json:"-" db:"-"`
}

type CouponUsage struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	CouponID        uuid.UUID  `json:"coupon_id" db:"coupon_id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	OrderID         *uuid.UUID `json:"order_id,omitempty" db:"order_id"`
	DiscountApplied float64    `json:"discount_applied" db:"discount_applied"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

// =====================================================
// Coupon DTO（管理端 CRUD）
// ====================================================

// CouponDiscountType 优惠类型：percentage=比例(0-100)，fixed=固定金额
type CouponDiscountType string

const (
	CouponDiscountPercentage CouponDiscountType = "percentage"
	CouponDiscountFixed      CouponDiscountType = "fixed"
)

// CreateCouponRequest 创建优惠券请求
type CreateCouponRequest struct {
	Code           string     `json:"code" binding:"required,min=2,max=32"`
	Name           string     `json:"name" binding:"required,min=1,max=128"`
	DiscountType   string     `json:"discount_type" binding:"required,oneof=percentage fixed"`
	DiscountValue  float64    `json:"discount_value" binding:"required,min=0"`
	MaxUses        int        `json:"max_uses" binding:"min=0"`
	MinOrderAmount float64    `json:"min_order_amount" binding:"min=0"`
	// PlanID 限定特定套餐可用，nil 表示不限制
	PlanID         *uuid.UUID `json:"plan_id"`
	// LimitUseByUser 每用户可用次数，0=不限
	LimitUseByUser int        `json:"limit_use_by_user" binding:"min=0"`
	// LimitPlanIDs 限定多个套餐可用，空表示不限制
	LimitPlanIDs   []uuid.UUID `json:"limit_plan_ids"`
	// LimitPeriod 限定可用周期（month/quarter/year 等），空表示不限制
	LimitPeriod    []string    `json:"limit_period"`
	// MaxDiscount 最大折扣金额上限（0=不限制）
	MaxDiscount    float64     `json:"max_discount"`
	// IsRepeatable 是否可重复使用（false=一次性券），默认 true
	IsRepeatable   bool        `json:"is_repeatable"`
	// NewUserOnly 仅限新用户使用
	NewUserOnly    bool       `json:"new_user_only"`
	// StartsAt 生效时间，nil 表示立即生效
	StartsAt       *time.Time `json:"starts_at"`
	// ExpiresAt 过期时间，nil 表示永不过期
	ExpiresAt      *time.Time `json:"expires_at"`
	// IsActive 是否启用
	IsActive       bool       `json:"is_active"`
}

// UpdateCouponRequest 更新优惠券请求
// 所有字段为 nil 表示不修改
type UpdateCouponRequest struct {
	Name           *string     `json:"name"`
	DiscountType   *string     `json:"discount_type" binding:"omitempty,oneof=percentage fixed"`
	DiscountValue  *float64    `json:"discount_value"`
	MaxUses        *int        `json:"max_uses"`
	MinOrderAmount *float64    `json:"min_order_amount"`
	PlanID         *uuid.UUID  `json:"plan_id"`
	LimitUseByUser *int        `json:"limit_use_by_user"`
	LimitPlanIDs   []uuid.UUID `json:"limit_plan_ids"`
	// LimitPeriod 限定可用周期，nil 表示不修改，非 nil 则覆盖
	LimitPeriod    []string    `json:"limit_period"`
	// MaxDiscount 最大折扣金额上限，nil 表示不修改
	MaxDiscount    *float64    `json:"max_discount"`
	// IsRepeatable 是否可重复使用，nil 表示不修改
	IsRepeatable   *bool       `json:"is_repeatable"`
	NewUserOnly    *bool       `json:"new_user_only"`
	StartsAt       *time.Time  `json:"starts_at"`
	ExpiresAt      *time.Time  `json:"expires_at"`
	IsActive       *bool       `json:"is_active"`
}

// CouponResponse 优惠券响应
type CouponResponse struct {
	ID             uuid.UUID   `json:"id"`
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	DiscountType   string      `json:"discount_type"`
	DiscountValue  float64     `json:"discount_value"`
	MaxUses        int         `json:"max_uses"`
	UsedCount      int         `json:"used_count"`
	MinOrderAmount float64     `json:"min_order_amount"`
	PlanID         *uuid.UUID  `json:"plan_id,omitempty"`
	LimitUseByUser int         `json:"limit_use_by_user"`
	LimitPlanIDs   []uuid.UUID `json:"limit_plan_ids,omitempty"`
	LimitPeriod    []string    `json:"limit_period,omitempty"`
	MaxDiscount    float64     `json:"max_discount"`
	IsRepeatable   bool        `json:"is_repeatable"`
	NewUserOnly    bool        `json:"new_user_only"`
	StartsAt       *time.Time  `json:"starts_at,omitempty"`
	ExpiresAt      *time.Time  `json:"expires_at,omitempty"`
	IsActive       bool        `json:"is_active"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// NewCouponResponse 将 Coupon 转换为 CouponResponse
func NewCouponResponse(c *Coupon) CouponResponse {
	limitPlanIDs := c.LimitPlanIDs
	if limitPlanIDs == nil {
		limitPlanIDs = []uuid.UUID{}
	}
	limitPeriod := c.LimitPeriod
	if limitPeriod == nil {
		limitPeriod = []string{}
	}
	return CouponResponse{
		ID:             c.ID,
		Code:           c.Code,
		Name:           c.Name,
		DiscountType:   c.DiscountType,
		DiscountValue:  c.DiscountValue,
		MaxUses:        c.MaxUses,
		UsedCount:      c.UsedCount,
		MinOrderAmount: c.MinOrderAmount,
		PlanID:         c.PlanID,
		LimitUseByUser: c.LimitUseByUser,
		LimitPlanIDs:   limitPlanIDs,
		LimitPeriod:    limitPeriod,
		MaxDiscount:    c.MaxDiscount,
		IsRepeatable:   c.IsRepeatable,
		NewUserOnly:    c.NewUserOnly,
		StartsAt:       c.StartsAt,
		ExpiresAt:      c.ExpiresAt,
		IsActive:       c.IsActive,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

type CommissionLog struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	InviterID         uuid.UUID  `json:"inviter_id" db:"inviter_id"`
	InviteeID         uuid.UUID  `json:"invitee_id" db:"invitee_id"`
	OrderID           *uuid.UUID `json:"order_id,omitempty" db:"order_id"`
	TradeNo           *string    `json:"trade_no,omitempty" db:"trade_no"`
	OrderAmount       float64    `json:"order_amount" db:"order_amount"`
	GetAmount         float64    `json:"get_amount" db:"get_amount"`
	CommissionBalance float64    `json:"commission_balance" db:"commission_balance"`
	Status            int        `json:"status" db:"status"`
	PaidAt            *time.Time `json:"paid_at,omitempty" db:"paid_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

type InviteCode struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Code      string    `json:"code" db:"code"`
	PV        int       `json:"pv" db:"pv"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type PeriodDays struct {
	Code string
	Days int
}

var PeriodDaysMap = map[string]int{
	"month":     30,
	"quarter":   90,
	"half_year": 180,
	"year":      365,
	"onetime":   365,
}

func NewOrderResponse(o *PaymentOrder) OrderResponse {
	amountCNY := o.AmountCNY
	if amountCNY == 0 && o.ExchangeRate > 0 {
		amountCNY = math.Round(o.AmountUSDT*o.ExchangeRate*100) / 100
	}
	return OrderResponse{
		ID:             o.ID,
		OrderNo:        o.OrderNo,
		PlanID:         o.PlanID,
		PlanName:       o.PlanName,
		PeriodCode:     o.PeriodCode,
		AmountUSDT:     o.AmountUSDT,
		AmountCNY:      amountCNY,
		ExchangeRate:   o.ExchangeRate,
		DiscountAmount: o.DiscountAmount,
		FinalAmount:    o.FinalAmount,
		CouponCode:     o.CouponCode,
		PayAddress:     o.PayAddress,
		PayCurrency:    o.PayCurrency,
		PaymentMethod:  o.PaymentMethod,
		PaymentURI:     o.PaymentURI,
		Status:         o.Status,
		TxHash:         o.TxHash,
		PaidAmount:     o.PaidAmount,
		PaidAt:         o.PaidAt,
		ExpiresAt:      o.ExpiresAt,
		CreatedAt:      o.CreatedAt,
	}
}
