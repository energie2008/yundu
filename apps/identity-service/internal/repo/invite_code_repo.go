package repo

import (
	"context"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InviteCodeRepo struct {
	pool *pgxpool.Pool
}

func NewInviteCodeRepo(pool *pgxpool.Pool) *InviteCodeRepo {
	return &InviteCodeRepo{pool: pool}
}

const inviteCodeColumns = `id, user_id, code, pv, created_at`

func scanInviteCode(row pgx.Row, ic *model.InviteCode) error {
	return row.Scan(&ic.ID, &ic.UserID, &ic.Code, &ic.PV, &ic.CreatedAt)
}

func (r *InviteCodeRepo) Create(ctx context.Context, code *model.InviteCode) error {
	query := `INSERT INTO invite_codes (id, user_id, code) VALUES ($1, $2, $3) RETURNING pv, created_at`
	return r.pool.QueryRow(ctx, query, code.ID, code.UserID, code.Code).Scan(&code.PV, &code.CreatedAt)
}

func (r *InviteCodeRepo) GetByCode(ctx context.Context, code string) (*model.InviteCode, error) {
	query := `SELECT ` + inviteCodeColumns + ` FROM invite_codes WHERE code = $1`
	ic := &model.InviteCode{}
	err := scanInviteCode(r.pool.QueryRow(ctx, query, code), ic)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ic, nil
}

func (r *InviteCodeRepo) IncrementPV(ctx context.Context, code string) error {
	query := `UPDATE invite_codes SET pv = pv + 1 WHERE code = $1`
	_, err := r.pool.Exec(ctx, query, code)
	return err
}

func (r *InviteCodeRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.InviteCode, error) {
	query := `SELECT ` + inviteCodeColumns + ` FROM invite_codes WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []*model.InviteCode
	for rows.Next() {
		ic := &model.InviteCode{}
		if err := scanInviteCode(rows, ic); err != nil {
			return nil, err
		}
		codes = append(codes, ic)
	}
	return codes, rows.Err()
}
