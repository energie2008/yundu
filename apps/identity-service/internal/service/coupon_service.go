package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

// 优惠券校验相关错误（补充 Xboard CouponService 对齐的高级限制）
var (
	ErrCouponPeriodLimit  = errors.New("coupon not valid for this period")
	ErrCouponNotRepeatable = errors.New("coupon is one-time use and already used")
)

// CouponService 优惠券业务逻辑（管理端 CRUD + 下单校验）
type CouponService struct {
	couponRepo       *repo.CouponRepo
	paymentOrderRepo *repo.PaymentOrderRepo
	logger           *slog.Logger
}

func NewCouponService(couponRepo *repo.CouponRepo, logger *slog.Logger) *CouponService {
	return &CouponService{couponRepo: couponRepo, logger: logger}
}

// SetPaymentOrderRepo 注入支付订单仓库（用于 NewUserOnly 校验判断用户是否为新用户）。
// 未注入时 ValidateCoupon 将跳过 NewUserOnly 校验。
func (s *CouponService) SetPaymentOrderRepo(r *repo.PaymentOrderRepo) {
	s.paymentOrderRepo = r
}

// Create 创建优惠券
func (s *CouponService) Create(ctx context.Context, req *model.CreateCouponRequest) (*model.Coupon, error) {
	// 校验优惠码唯一性
	existing, err := s.couponRepo.FindByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCouponCodeExists
	}

	// 校验折扣值合法性
	if req.DiscountType == "percentage" && req.DiscountValue > 100 {
		return nil, errors.New("percentage discount value must be 0-100")
	}

	c := &model.Coupon{
		Code:           req.Code,
		Name:           req.Name,
		DiscountType:   req.DiscountType,
		DiscountValue:  req.DiscountValue,
		MaxUses:        req.MaxUses,
		MinOrderAmount: req.MinOrderAmount,
		PlanID:         req.PlanID,
		LimitUseByUser: req.LimitUseByUser,
		LimitPlanIDs:   req.LimitPlanIDs,
		LimitPeriod:    req.LimitPeriod,
		MaxDiscount:    req.MaxDiscount,
		IsRepeatable:   req.IsRepeatable,
		NewUserOnly:    req.NewUserOnly,
		StartsAt:       req.StartsAt,
		ExpiresAt:      req.ExpiresAt,
		IsActive:       req.IsActive,
	}

	if err := s.couponRepo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetByID 根据 ID 获取优惠券
func (s *CouponService) GetByID(ctx context.Context, id uuid.UUID) (*model.Coupon, error) {
	c, err := s.couponRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrCouponNotFound
	}
	return c, nil
}

// GetByCode 根据优惠码获取优惠券（用于用户端校验时获取优惠券详情）
func (s *CouponService) GetByCode(ctx context.Context, code string) (*model.Coupon, error) {
	c, err := s.couponRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrCouponNotFound
	}
	return c, nil
}

// List 分页查询优惠券
func (s *CouponService) List(ctx context.Context, page, pageSize int, search string, isActive *bool) ([]*model.Coupon, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	return s.couponRepo.List(ctx, page, pageSize, search, isActive)
}

// Update 更新优惠券
func (s *CouponService) Update(ctx context.Context, id uuid.UUID, req *model.UpdateCouponRequest) (*model.Coupon, error) {
	c, err := s.couponRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrCouponNotFound
	}

	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.DiscountType != nil {
		c.DiscountType = *req.DiscountType
	}
	if req.DiscountValue != nil {
		c.DiscountValue = *req.DiscountValue
	}
	// 校验折扣值合法性
	if c.DiscountType == "percentage" && c.DiscountValue > 100 {
		return nil, errors.New("percentage discount value must be 0-100")
	}
	if req.MaxUses != nil {
		c.MaxUses = *req.MaxUses
	}
	if req.MinOrderAmount != nil {
		c.MinOrderAmount = *req.MinOrderAmount
	}
	if req.PlanID != nil {
		c.PlanID = req.PlanID
	}
	if req.LimitUseByUser != nil {
		c.LimitUseByUser = *req.LimitUseByUser
	}
	if req.LimitPlanIDs != nil {
		c.LimitPlanIDs = req.LimitPlanIDs
	}
	// LimitPeriod 为 []string：nil 表示不修改，非 nil（含空切片）表示覆盖
	if req.LimitPeriod != nil {
		c.LimitPeriod = req.LimitPeriod
	}
	if req.MaxDiscount != nil {
		c.MaxDiscount = *req.MaxDiscount
	}
	if req.IsRepeatable != nil {
		c.IsRepeatable = *req.IsRepeatable
	}
	if req.NewUserOnly != nil {
		c.NewUserOnly = *req.NewUserOnly
	}
	if req.StartsAt != nil {
		c.StartsAt = req.StartsAt
	}
	if req.ExpiresAt != nil {
		c.ExpiresAt = req.ExpiresAt
	}
	if req.IsActive != nil {
		c.IsActive = *req.IsActive
	}

	if err := s.couponRepo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Delete 删除优惠券
// 注意：删除优惠券会级联删除 coupon_usages 记录（ON DELETE CASCADE）
func (s *CouponService) Delete(ctx context.Context, id uuid.UUID) error {
	c, err := s.couponRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil {
		return ErrCouponNotFound
	}
	return s.couponRepo.Delete(ctx, id)
}

// ValidateCoupon 下单时校验优惠券并计算折扣金额。
//
// 校验流程（参考 Xboard CouponService::check）：
//  1. 查找优惠券（code 匹配、启用状态、未过期、已生效）
//  2. 检查全局使用次数限制（max_uses）
//  3. 检查一次性券（is_repeatable=false 时全局仅可用一次）
//  4. 检查每用户限用次数（limit_use_by_user）
//  5. 检查限制可用套餐（limit_plan_ids / plan_id）
//  6. 检查限制可用周期（limit_period）
//  7. 检查最低消费金额（min_order_amount）
//  8. 仅限新用户（new_user_only，需要注入 PaymentOrderRepo）
//  9. 计算折扣（percentage/fixed），应用 max_discount 上限，并封顶为订单金额
//
// period 为业务周期标识（如 month/quarter/year），由调用方归一化后传入；
// 空字符串表示不参与周期校验。
func (s *CouponService) ValidateCoupon(ctx context.Context, code string, userID uuid.UUID, amount float64, planID uuid.UUID, period string) (float64, error) {
	coupon, err := s.couponRepo.FindByCode(ctx, code)
	if err != nil {
		return 0, err
	}
	if coupon == nil {
		return 0, ErrCouponNotFound
	}

	// 1. 启用状态 / 生效时间 / 过期时间
	if !coupon.IsActive {
		return 0, ErrCouponInvalid
	}
	now := time.Now()
	if coupon.StartsAt != nil && now.Before(*coupon.StartsAt) {
		return 0, ErrCouponNotStarted
	}
	if coupon.ExpiresAt != nil && now.After(*coupon.ExpiresAt) {
		return 0, ErrCouponExpired
	}

	// 2. 全局使用次数限制（max_uses=0 表示不限）
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		return 0, ErrCouponUsedUp
	}

	// 3. 一次性券：不可重复使用，全局已用过即拒绝
	if !coupon.IsRepeatable && coupon.UsedCount > 0 {
		return 0, ErrCouponNotRepeatable
	}

	// 4. 每用户限用次数（limit_use_by_user=0 表示不限）
	if coupon.LimitUseByUser > 0 {
		usedCount, err := s.couponRepo.CountUsageByUser(ctx, coupon.ID, userID)
		if err != nil {
			s.logger.Warn("validate coupon: count usage by user failed", "coupon", coupon.ID, "user", userID, "error", err)
		} else if usedCount >= coupon.LimitUseByUser {
			return 0, ErrCouponUsedUp
		}
	}

	// 5. 限制可用套餐（plan_id 单选 / limit_plan_ids 多选，空=不限制）
	if planID != uuid.Nil {
		planAllowed := true
		if coupon.PlanID != nil && *coupon.PlanID != planID {
			planAllowed = false
		}
		if !planAllowed && len(coupon.LimitPlanIDs) > 0 {
			planAllowed = containsUUID(coupon.LimitPlanIDs, planID)
		}
		if !planAllowed {
			return 0, ErrCouponPlanLimit
		}
	}

	// 6. 限制可用周期（limit_period 空=不限制）
	if period != "" && len(coupon.LimitPeriod) > 0 {
		if !containsString(coupon.LimitPeriod, period) {
			return 0, ErrCouponPeriodLimit
		}
	}

	// 7. 最低消费金额
	if amount < coupon.MinOrderAmount {
		return 0, ErrCouponMinAmount
	}

	// 8. 仅限新用户（需要 PaymentOrderRepo；未注入时跳过）
	if coupon.NewUserOnly && s.paymentOrderRepo != nil {
		orders, _, err := s.paymentOrderRepo.ListByUser(ctx, userID, 1, 1, "")
		if err == nil && len(orders) > 0 {
			return 0, ErrCouponNewUserOnly
		}
	}

	// 9. 计算折扣
	var discount float64
	switch coupon.DiscountType {
	case "fixed":
		discount = coupon.DiscountValue
	case "percentage":
		discount = amount * coupon.DiscountValue / 100.0
	default: // 兜底按百分比处理
		discount = amount * coupon.DiscountValue / 100.0
	}
	// max_discount 上限（0=不限）
	if coupon.MaxDiscount > 0 && discount > coupon.MaxDiscount {
		discount = coupon.MaxDiscount
	}
	// 折扣不可超过订单金额
	if discount > amount {
		discount = amount
	}
	if discount < 0 {
		discount = 0
	}
	discount = math.Round(discount*100) / 100
	return discount, nil
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func containsUUID(list []uuid.UUID, target uuid.UUID) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// MapCouponErrorToCode 将优惠券业务错误映射为 HTTP 错误码
func MapCouponErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrCouponNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrCouponCodeExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrCouponExpired):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponUsedUp):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponMinAmount):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponInvalid):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponPlanLimit):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponNewUserOnly):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponPeriodLimit):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrCouponNotRepeatable):
		return config.CodeBadRequest, err.Error()
	default:
		// 保留原始错误信息以便排查
		msg := err.Error()
		if msg == "" || strings.Contains(msg, "internal") {
			return config.CodeInternalError, ""
		}
		return config.CodeBadRequest, msg
	}
}
