package repo

import (
	"context"
	"strconv"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentOrderRepo struct {
	pool *pgxpool.Pool
}

func NewPaymentOrderRepo(pool *pgxpool.Pool) *PaymentOrderRepo {
	return &PaymentOrderRepo{pool: pool}
}

const paymentOrderColumns = `id, order_no, user_id, plan_id, plan_name, period_code, amount_usdt::float8,
	COALESCE(amount_cny,0)::float8, COALESCE(exchange_rate,7.2)::float8,
	COALESCE(discount_amount,0)::float8, COALESCE(final_amount,amount_usdt)::float8, COALESCE(coupon_code,''),
	pay_address, pay_currency, payment_method, COALESCE(gateway,''), COALESCE(gateway_trade_no,''),
	COALESCE(payment_uri,''), status, tx_hash, paid_amount, paid_at,
	block_number, expires_at, created_at, updated_at`

func scanPaymentOrder(row pgx.Row, o *model.PaymentOrder) error {
	return row.Scan(
		&o.ID, &o.OrderNo, &o.UserID, &o.PlanID, &o.PlanName, &o.PeriodCode,
		&o.AmountUSDT, &o.AmountCNY, &o.ExchangeRate, &o.DiscountAmount, &o.FinalAmount, &o.CouponCode,
		&o.PayAddress, &o.PayCurrency, &o.PaymentMethod, &o.Gateway, &o.GatewayTradeNo, &o.PaymentURI,
		&o.Status, &o.TxHash,
		&o.PaidAmount, &o.PaidAt, &o.BlockNumber, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt,
	)
}

func (r *PaymentOrderRepo) Create(ctx context.Context, order *model.PaymentOrder) error {
	query := `
		INSERT INTO payment_orders (
			id, order_no, user_id, plan_id, plan_name, period_code, amount_usdt,
			amount_cny, exchange_rate, discount_amount, final_amount, coupon_code,
			pay_address, pay_currency, payment_method, gateway, gateway_trade_no, payment_uri, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		order.ID, order.OrderNo, order.UserID, order.PlanID, order.PlanName,
		order.PeriodCode, order.AmountUSDT, order.AmountCNY, order.ExchangeRate,
		order.DiscountAmount, order.FinalAmount,
		order.CouponCode, order.PayAddress, order.PayCurrency, order.PaymentMethod,
		order.Gateway, order.GatewayTradeNo, order.PaymentURI, order.Status, order.ExpiresAt,
	).Scan(&order.CreatedAt, &order.UpdatedAt)
}

// UpdateGatewayInfo 写入易支付网关返回的流水号、支付跳转地址与二维码内容。
func (r *PaymentOrderRepo) UpdateGatewayInfo(ctx context.Context, id uuid.UUID, gateway, tradeNo, paymentURI, qrcode string) error {
	query := `UPDATE payment_orders SET gateway = $2, gateway_trade_no = $3, payment_uri = $4, pay_address = $5, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, gateway, tradeNo, paymentURI, qrcode)
	return err
}

func (r *PaymentOrderRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.PaymentOrder, error) {
	query := `SELECT ` + paymentOrderColumns + ` FROM payment_orders WHERE id = $1`
	o := &model.PaymentOrder{}
	err := scanPaymentOrder(r.pool.QueryRow(ctx, query, id), o)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func (r *PaymentOrderRepo) GetByOrderNo(ctx context.Context, orderNo string) (*model.PaymentOrder, error) {
	query := `SELECT ` + paymentOrderColumns + ` FROM payment_orders WHERE order_no = $1`
	o := &model.PaymentOrder{}
	err := scanPaymentOrder(r.pool.QueryRow(ctx, query, orderNo), o)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func (r *PaymentOrderRepo) GetByTxHash(ctx context.Context, txHash string) (*model.PaymentOrder, error) {
	query := `SELECT ` + paymentOrderColumns + ` FROM payment_orders WHERE tx_hash = $1`
	o := &model.PaymentOrder{}
	err := scanPaymentOrder(r.pool.QueryRow(ctx, query, txHash), o)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func (r *PaymentOrderRepo) ListByUser(ctx context.Context, userID uuid.UUID, page, pageSize int, statusFilter string) ([]*model.PaymentOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	where := `WHERE user_id = $1`
	args := []interface{}{userID}
	if statusFilter != "" {
		where += ` AND status = $2`
		args = append(args, statusFilter)
	}
	countQuery := `SELECT COUNT(*) FROM payment_orders ` + where
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	dataQuery := `SELECT ` + paymentOrderColumns + ` FROM payment_orders ` + where +
		` ORDER BY created_at DESC LIMIT $` + itoa(len(args)+1) + ` OFFSET $` + itoa(len(args)+2)
	args = append(args, pageSize, offset)
	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var orders []*model.PaymentOrder
	for rows.Next() {
		o := &model.PaymentOrder{}
		if err := scanPaymentOrder(rows, o); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, rows.Err()
}

func (r *PaymentOrderRepo) ListPending(ctx context.Context, page, pageSize int) ([]*model.PaymentOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	countQuery := `SELECT COUNT(*) FROM payment_orders WHERE status = 'pending'`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT ` + paymentOrderColumns + ` FROM payment_orders WHERE status = 'pending'
		ORDER BY created_at ASC LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var orders []*model.PaymentOrder
	for rows.Next() {
		o := &model.PaymentOrder{}
		if err := scanPaymentOrder(rows, o); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, rows.Err()
}

// AdminList 管理员订单列表（支持多条件筛选）
func (r *PaymentOrderRepo) AdminList(ctx context.Context, page, pageSize int, status, userID, planID string) ([]*model.PaymentOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	where := `WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		where += ` AND status = $` + itoa(argIdx)
		args = append(args, status)
		argIdx++
	}
	if userID != "" {
		where += ` AND user_id = $` + itoa(argIdx)
		args = append(args, userID)
		argIdx++
	}
	if planID != "" {
		where += ` AND plan_id = $` + itoa(argIdx)
		args = append(args, planID)
		argIdx++
	}

	countQuery := `SELECT COUNT(*) FROM payment_orders ` + where
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQuery := `SELECT ` + paymentOrderColumns + ` FROM payment_orders ` + where +
		` ORDER BY created_at DESC LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)
	args = append(args, pageSize, offset)
	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var orders []*model.PaymentOrder
	for rows.Next() {
		o := &model.PaymentOrder{}
		if err := scanPaymentOrder(rows, o); err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, rows.Err()
}

func (r *PaymentOrderRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, txHash *string, paidAmount *float64, paidAt *time.Time) (bool, error) {
	query := `UPDATE payment_orders SET status = $2, updated_at = now()`
	args := []interface{}{id, status}
	if txHash != nil {
		args = append(args, *txHash)
		query += `, tx_hash = $` + itoa(len(args))
	}
	if paidAmount != nil {
		args = append(args, *paidAmount)
		query += `, paid_amount = $` + itoa(len(args))
	}
	if paidAt != nil {
		args = append(args, *paidAt)
		query += `, paid_at = $` + itoa(len(args))
	}
	query += ` WHERE id = $1`
	ct, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// MarkPaidIfPending 仅当订单仍为 pending 时标记为已支付。
// 用于链上轮询匹配，避免同一笔交易或并发轮询对已处理订单重复激活。
func (r *PaymentOrderRepo) MarkPaidIfPending(ctx context.Context, id uuid.UUID, txHash string, paidAmount float64, paidAt time.Time) (bool, error) {
	query := `UPDATE payment_orders SET status = 'paid', tx_hash = $2, paid_amount = $3, paid_at = $4, updated_at = now() WHERE id = $1 AND status = 'pending'`
	ct, err := r.pool.Exec(ctx, query, id, txHash, paidAmount, paidAt)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func (r *PaymentOrderRepo) UpdateBlockNumber(ctx context.Context, id uuid.UUID, blockNumber *int64) error {
	if blockNumber == nil {
		return nil
	}
	query := `UPDATE payment_orders SET block_number = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, *blockNumber)
	return err
}

func (r *PaymentOrderRepo) MarkExpired(ctx context.Context, before time.Time) (int64, error) {
	query := `UPDATE payment_orders SET status = 'expired', updated_at = now()
		WHERE status = 'pending' AND expires_at < $1`
	tag, err := r.pool.Exec(ctx, query, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

// RevenueStats 营收统计（参考 xboard 仪表盘）：基于 status='paid' 且 paid_at 落在对应时间窗内的订单，
// 按 final_amount（USDT）与 amount_cny（CNY）聚合，并统计当日/当月成交订单数。
type RevenueStats struct {
	TodayRevenueUSDT float64 `json:"today_revenue_usdt"`
	TodayRevenueCNY  float64 `json:"today_revenue_cny"`
	TodayOrderCount  int     `json:"today_order_count"`
	MonthRevenueUSDT float64 `json:"month_revenue_usdt"`
	MonthRevenueCNY  float64 `json:"month_revenue_cny"`
	MonthOrderCount  int     `json:"month_order_count"`
}

// GetRevenueStats 汇总当日与当月的营收。时间窗以数据库本地时区的 date_trunc 计算，
// 避免应用侧时区换算偏差；未成交（非 paid）订单不计入。
func (r *PaymentOrderRepo) GetRevenueStats(ctx context.Context) (*RevenueStats, error) {
	query := `
		SELECT
			COALESCE(SUM(COALESCE(final_amount, amount_usdt)) FILTER (WHERE paid_at >= date_trunc('day', now())), 0)::float8   AS today_usdt,
			COALESCE(SUM(COALESCE(amount_cny, 0))            FILTER (WHERE paid_at >= date_trunc('day', now())), 0)::float8   AS today_cny,
			COUNT(*)                                         FILTER (WHERE paid_at >= date_trunc('day', now()))               AS today_count,
			COALESCE(SUM(COALESCE(final_amount, amount_usdt)) FILTER (WHERE paid_at >= date_trunc('month', now())), 0)::float8 AS month_usdt,
			COALESCE(SUM(COALESCE(amount_cny, 0))            FILTER (WHERE paid_at >= date_trunc('month', now())), 0)::float8 AS month_cny,
			COUNT(*)                                         FILTER (WHERE paid_at >= date_trunc('month', now()))             AS month_count
		FROM payment_orders
		WHERE status = 'paid' AND paid_at IS NOT NULL`
	s := &RevenueStats{}
	err := r.pool.QueryRow(ctx, query).Scan(
		&s.TodayRevenueUSDT, &s.TodayRevenueCNY, &s.TodayOrderCount,
		&s.MonthRevenueUSDT, &s.MonthRevenueCNY, &s.MonthOrderCount,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}
