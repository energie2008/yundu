package repo

import (
	"context"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommissionLogRepo struct {
	pool *pgxpool.Pool
}

func NewCommissionLogRepo(pool *pgxpool.Pool) *CommissionLogRepo {
	return &CommissionLogRepo{pool: pool}
}

const commissionLogColumns = `id, inviter_id, invitee_id, order_id, trade_no,
	order_amount, get_amount, commission_balance, status, created_at, updated_at`

func scanCommissionLog(row pgx.Row, cl *model.CommissionLog) error {
	return row.Scan(
		&cl.ID, &cl.InviterID, &cl.InviteeID, &cl.OrderID, &cl.TradeNo,
		&cl.OrderAmount, &cl.GetAmount, &cl.CommissionBalance, &cl.Status,
		&cl.CreatedAt, &cl.UpdatedAt,
	)
}

func (r *CommissionLogRepo) Create(ctx context.Context, log *model.CommissionLog) error {
	query := `
		INSERT INTO commission_logs (id, inviter_id, invitee_id, order_id, trade_no,
			order_amount, get_amount, commission_balance, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		log.ID, log.InviterID, log.InviteeID, log.OrderID, log.TradeNo,
		log.OrderAmount, log.GetAmount, log.CommissionBalance, log.Status,
	).Scan(&log.CreatedAt, &log.UpdatedAt)
}

func (r *CommissionLogRepo) ListPendingBefore(ctx context.Context, before time.Time) ([]*model.CommissionLog, error) {
	query := `SELECT ` + commissionLogColumns + ` FROM commission_logs WHERE status = 0 AND created_at < $1 ORDER BY created_at ASC LIMIT 100`
	rows, err := r.pool.Query(ctx, query, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*model.CommissionLog
	for rows.Next() {
		cl := &model.CommissionLog{}
		if err := scanCommissionLog(rows, cl); err != nil {
			return nil, err
		}
		logs = append(logs, cl)
	}
	return logs, rows.Err()
}

func (r *CommissionLogRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status int) error {
	query := `UPDATE commission_logs SET status = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

// GetByID 根据 ID 查询单条佣金日志，未找到时返回 (nil, nil)。
func (r *CommissionLogRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.CommissionLog, error) {
	query := `SELECT ` + commissionLogColumns + ` FROM commission_logs WHERE id = $1`
	cl := &model.CommissionLog{}
	if err := scanCommissionLog(r.pool.QueryRow(ctx, query, id), cl); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return cl, nil
}

// ListByInviter 分页查询某邀请人的佣金记录明细（按创建时间倒序）。
func (r *CommissionLogRepo) ListByInviter(ctx context.Context, inviterID uuid.UUID, page, pageSize int) ([]*model.CommissionLog, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	var total int
	countQuery := `SELECT COUNT(*) FROM commission_logs WHERE inviter_id = $1`
	if err := r.pool.QueryRow(ctx, countQuery, inviterID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.CommissionLog{}, 0, nil
	}
	offset := (page - 1) * pageSize
	query := `SELECT ` + commissionLogColumns + ` FROM commission_logs WHERE inviter_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, inviterID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	logs := make([]*model.CommissionLog, 0)
	for rows.Next() {
		cl := &model.CommissionLog{}
		if err := scanCommissionLog(rows, cl); err != nil {
			return nil, 0, err
		}
		logs = append(logs, cl)
	}
	return logs, total, rows.Err()
}
