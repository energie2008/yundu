package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CouponRepo struct {
	pool *pgxpool.Pool
}

func NewCouponRepo(pool *pgxpool.Pool) *CouponRepo {
	return &CouponRepo{pool: pool}
}

const couponColumns = `id, code, name, discount_type, discount_value, max_uses, used_count,
	min_order_amount, plan_id, limit_use_by_user, limit_plan_ids, new_user_only,
	limit_period, max_discount, is_repeatable,
	starts_at, expires_at, is_active, created_at, updated_at`

func scanCoupon(row pgx.Row, c *model.Coupon) error {
	var limitPlanIDs []uuid.UUID
	var limitPeriod []string
	err := row.Scan(
		&c.ID, &c.Code, &c.Name, &c.DiscountType, &c.DiscountValue,
		&c.MaxUses, &c.UsedCount, &c.MinOrderAmount, &c.PlanID,
		&c.LimitUseByUser, &limitPlanIDs, &c.NewUserOnly,
		&limitPeriod, &c.MaxDiscount, &c.IsRepeatable,
		&c.StartsAt, &c.ExpiresAt, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return err
	}
	c.LimitPlanIDs = limitPlanIDs
	c.LimitPeriod = limitPeriod
	return nil
}

func (r *CouponRepo) FindByCode(ctx context.Context, code string) (*model.Coupon, error) {
	query := `SELECT ` + couponColumns + ` FROM coupons WHERE code = $1`
	c := &model.Coupon{}
	err := scanCoupon(r.pool.QueryRow(ctx, query, code), c)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *CouponRepo) GetByCode(ctx context.Context, code string) (*model.Coupon, error) {
	return r.FindByCode(ctx, code)
}

// GetByID 根据 ID 查询优惠券
func (r *CouponRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Coupon, error) {
	query := `SELECT ` + couponColumns + ` FROM coupons WHERE id = $1`
	c := &model.Coupon{}
	err := scanCoupon(r.pool.QueryRow(ctx, query, id), c)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// List 分页查询优惠券列表
// search 为空时查询全部；is_active 为 nil 时不作为筛选条件
func (r *CouponRepo) List(ctx context.Context, page, pageSize int, search string, isActive *bool) ([]*model.Coupon, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if search != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if isActive != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *isActive)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM coupons WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM coupons WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		couponColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Coupon
	for rows.Next() {
		c := &model.Coupon{}
		if err := scanCoupon(rows, c); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, rows.Err()
}

// Create 创建优惠券
func (r *CouponRepo) Create(ctx context.Context, c *model.Coupon) error {
	limitPlanIDs := c.LimitPlanIDs
	if limitPlanIDs == nil {
		limitPlanIDs = []uuid.UUID{}
	}
	limitPeriod := c.LimitPeriod
	if limitPeriod == nil {
		limitPeriod = []string{}
	}
	query := `
		INSERT INTO coupons (code, name, discount_type, discount_value, max_uses,
			min_order_amount, plan_id, limit_use_by_user, limit_plan_ids, new_user_only,
			limit_period, max_discount, is_repeatable,
			starts_at, expires_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, used_count, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		c.Code, c.Name, c.DiscountType, c.DiscountValue, c.MaxUses,
		c.MinOrderAmount, c.PlanID, c.LimitUseByUser, limitPlanIDs, c.NewUserOnly,
		limitPeriod, c.MaxDiscount, c.IsRepeatable,
		c.StartsAt, c.ExpiresAt, c.IsActive,
	).Scan(&c.ID, &c.UsedCount, &c.CreatedAt, &c.UpdatedAt)
}

// Update 更新优惠券（除 used_count 外的所有可变字段）
func (r *CouponRepo) Update(ctx context.Context, c *model.Coupon) error {
	limitPlanIDs := c.LimitPlanIDs
	if limitPlanIDs == nil {
		limitPlanIDs = []uuid.UUID{}
	}
	limitPeriod := c.LimitPeriod
	if limitPeriod == nil {
		limitPeriod = []string{}
	}
	query := `
		UPDATE coupons SET
			name = $2, discount_type = $3, discount_value = $4, max_uses = $5,
			min_order_amount = $6, plan_id = $7, limit_use_by_user = $8,
			limit_plan_ids = $9, new_user_only = $10,
			limit_period = $11, max_discount = $12, is_repeatable = $13,
			starts_at = $14, expires_at = $15, is_active = $16, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.Name, c.DiscountType, c.DiscountValue, c.MaxUses,
		c.MinOrderAmount, c.PlanID, c.LimitUseByUser, limitPlanIDs, c.NewUserOnly,
		limitPeriod, c.MaxDiscount, c.IsRepeatable,
		c.StartsAt, c.ExpiresAt, c.IsActive,
	)
	return err
}

// Delete 删除优惠券
func (r *CouponRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM coupons WHERE id = $1`, id)
	return err
}

func (r *CouponRepo) IncrementUsed(ctx context.Context, couponID uuid.UUID) error {
	query := `UPDATE coupons SET used_count = used_count + 1, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, couponID)
	return err
}

func (r *CouponRepo) IncrementUsage(ctx context.Context, couponID uuid.UUID) error {
	return r.IncrementUsed(ctx, couponID)
}

// CreateUsage 记录优惠券使用情况
// 注意：coupon_usages 表的实际列名是 discount_amount（非 discount_applied）
func (r *CouponRepo) CreateUsage(ctx context.Context, usage *model.CouponUsage) error {
	query := `INSERT INTO coupon_usages (id, coupon_id, user_id, order_id, discount_amount)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		usage.ID, usage.CouponID, usage.UserID, usage.OrderID, usage.DiscountApplied,
	).Scan(&usage.CreatedAt)
}

func (r *CouponRepo) RecordUsage(ctx context.Context, couponID, userID, orderID uuid.UUID, discountAmount float64) error {
	usage := &model.CouponUsage{
		ID:              uuid.New(),
		CouponID:        couponID,
		UserID:          userID,
		OrderID:         &orderID,
		DiscountApplied: discountAmount,
	}
	return r.CreateUsage(ctx, usage)
}

func (r *CouponRepo) CountUsageByUser(ctx context.Context, couponID, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM coupon_usages WHERE coupon_id = $1 AND user_id = $2`
	var count int
	err := r.pool.QueryRow(ctx, query, couponID, userID).Scan(&count)
	return count, err
}
