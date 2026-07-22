package repo

import (
	"context"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	// 注意：users 表没有 traffic_used_bytes/traffic_total_bytes/expires_at 列
	// 流量信息在 user_plan_subscriptions 表中，这里只查 users 表存在的列
	query := `
		SELECT id, COALESCE(email, ''), COALESCE(username, ''), status, COALESCE(uuid, ''),
		       group_id, created_at, updated_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	u := &model.User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.Username, &u.Status, &u.UUID,
		&u.GroupID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}
