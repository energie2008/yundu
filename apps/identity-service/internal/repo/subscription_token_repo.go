package repo

import (
	"context"
	"net"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubscriptionTokenRepo struct {
	pool *pgxpool.Pool
}

func NewSubscriptionTokenRepo(pool *pgxpool.Pool) *SubscriptionTokenRepo {
	return &SubscriptionTokenRepo{pool: pool}
}

func (r *SubscriptionTokenRepo) Create(ctx context.Context, token *model.SubscriptionToken) error {
	query := `
		INSERT INTO subscription_tokens (
			id, user_id, token_hash, token_preview, status, client_hint,
			allow_ip_bind, bound_ip, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		token.ID, token.UserID, token.TokenHash, token.TokenPreview,
		token.Status, token.ClientHint, token.AllowIPBind, token.BoundIP, token.ExpiresAt,
	).Scan(&token.CreatedAt, &token.UpdatedAt)
}

func (r *SubscriptionTokenRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.SubscriptionToken, error) {
	query := `
		SELECT id, user_id, token_hash, token_preview, status, client_hint,
		       allow_ip_bind, bound_ip::text, last_access_at, last_access_ip::text,
		       expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE id = $1`
	t := &model.SubscriptionToken{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint,
		&t.AllowIPBind, &t.BoundIP, &t.LastAccessAt, &t.LastAccessIP,
		&t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *SubscriptionTokenRepo) GetByTokenHash(ctx context.Context, hash string) (*model.SubscriptionToken, error) {
	query := `
		SELECT id, user_id, token_hash, token_preview, status, client_hint,
		       allow_ip_bind, bound_ip::text, last_access_at, last_access_ip::text,
		       expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE token_hash = $1`
	t := &model.SubscriptionToken{}
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint,
		&t.AllowIPBind, &t.BoundIP, &t.LastAccessAt, &t.LastAccessIP,
		&t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *SubscriptionTokenRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*model.SubscriptionToken, error) {
	query := `
		SELECT id, user_id, token_hash, token_preview, status, client_hint,
		       allow_ip_bind, bound_ip::text, last_access_at, last_access_ip::text,
		       expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens
		WHERE user_id = $1 AND status != 'revoked'
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*model.SubscriptionToken
	for rows.Next() {
		t := &model.SubscriptionToken{}
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint,
			&t.AllowIPBind, &t.BoundIP, &t.LastAccessAt, &t.LastAccessIP,
			&t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *SubscriptionTokenRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE subscription_tokens SET status = 'revoked', revoked_at = now(), updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *SubscriptionTokenRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE subscription_tokens SET status = 'revoked', revoked_at = now(), updated_at = now() WHERE user_id = $1 AND status = 'active'`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *SubscriptionTokenRepo) UpdateAccess(ctx context.Context, id uuid.UUID, ip net.IP) error {
	ipStr := ""
	if ip != nil {
		ipStr = ip.String()
	}
	now := time.Now()
	query := `UPDATE subscription_tokens SET last_access_at = $2, last_access_ip = $3, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, now, ipStr)
	return err
}
