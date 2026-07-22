package repo

import (
	"context"
	"errors"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CommissionWithdrawRepo struct {
	pool *pgxpool.Pool
}

func NewCommissionWithdrawRepo(pool *pgxpool.Pool) *CommissionWithdrawRepo {
	return &CommissionWithdrawRepo{pool: pool}
}

const withdrawColumns = `id, user_id, amount, method, account, real_name, status, remark, handled_by, handled_at, created_at, updated_at`

func scanWithdraw(row pgx.Row, w *model.Withdraw) error {
	return row.Scan(
		&w.ID, &w.UserID, &w.Amount, &w.Method, &w.Account, &w.RealName,
		&w.Status, &w.Remark, &w.HandledBy, &w.HandledAt, &w.CreatedAt, &w.UpdatedAt,
	)
}

func (r *CommissionWithdrawRepo) Create(ctx context.Context, w *model.Withdraw) error {
	query := `
		INSERT INTO commission_withdrawals (id, user_id, amount, method, account, real_name, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		w.ID, w.UserID, w.Amount, w.Method, w.Account, w.RealName, w.Status,
	).Scan(&w.CreatedAt, &w.UpdatedAt)
}

func (r *CommissionWithdrawRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.Withdraw, int, error) {
	countQuery := `SELECT COUNT(*) FROM commission_withdrawals WHERE user_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + withdrawColumns + ` FROM commission_withdrawals WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Withdraw
	for rows.Next() {
		w := &model.Withdraw{}
		if err := scanWithdraw(rows, w); err != nil {
			return nil, 0, err
		}
		items = append(items, w)
	}
	return items, total, rows.Err()
}

func (r *CommissionWithdrawRepo) SumWithdrawnByUser(ctx context.Context, userID uuid.UUID) (float64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM commission_withdrawals WHERE user_id = $1 AND status = 1`
	var sum float64
	err := r.pool.QueryRow(ctx, query, userID).Scan(&sum)
	return sum, err
}

func (r *CommissionWithdrawRepo) GetPendingCommissions(ctx context.Context, userID uuid.UUID) (float64, error) {
	query := `SELECT COALESCE(SUM(get_amount), 0) FROM commission_logs WHERE inviter_id = $1 AND status = 0`
	var sum float64
	err := r.pool.QueryRow(ctx, query, userID).Scan(&sum)
	return sum, err
}

// ListAll lists all withdrawals, optionally filtered by status. Returns (items, total, error).
func (r *CommissionWithdrawRepo) ListAll(ctx context.Context, status *int) ([]*model.Withdraw, int, error) {
	baseWhere := ""
	args := []interface{}{}
	if status != nil {
		baseWhere = " WHERE status = $1"
		args = append(args, *status)
	}
	countQuery := "SELECT COUNT(*) FROM commission_withdrawals" + baseWhere
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT " + withdrawColumns + " FROM commission_withdrawals" + baseWhere + " ORDER BY created_at DESC LIMIT 500"
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []*model.Withdraw
	for rows.Next() {
		w := &model.Withdraw{}
		if err := scanWithdraw(rows, w); err != nil {
			return nil, 0, err
		}
		items = append(items, w)
	}
	return items, total, rows.Err()
}

// GetByID returns a single withdrawal by id, or nil if not found.
func (r *CommissionWithdrawRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Withdraw, error) {
	query := "SELECT " + withdrawColumns + " FROM commission_withdrawals WHERE id = $1"
	w := &model.Withdraw{}
	if err := scanWithdraw(r.pool.QueryRow(ctx, query, id), w); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

// UpdateStatus updates a withdrawal's status, handler, and remark. Only valid for pending (status=0) rows.
func (r *CommissionWithdrawRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status int, handledBy uuid.UUID, remark string) error {
	query := `UPDATE commission_withdrawals
		SET status = $2, handled_by = $3, handled_at = now(),
		    remark = $4, updated_at = now()
		WHERE id = $1 AND status = 0`
	tag, err := r.pool.Exec(ctx, query, id, status, handledBy, remark)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("withdrawal not found or already processed")
	}
	return nil
}
