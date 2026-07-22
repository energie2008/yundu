package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenRepo struct {
	pool *pgxpool.Pool
}

func NewTokenRepo(pool *pgxpool.Pool) *TokenRepo {
	return &TokenRepo{pool: pool}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (r *TokenRepo) Create(ctx context.Context, t *model.SubscriptionToken) error {
	t.TokenHash = hashToken(t.TokenValue)
	if len(t.TokenValue) > 16 {
		t.TokenPreview = t.TokenValue[:8] + "..." + t.TokenValue[len(t.TokenValue)-4:]
	} else {
		t.TokenPreview = t.TokenValue
	}
	query := `
		INSERT INTO subscription_tokens (id, user_id, token_hash, token_preview, status, client_hint, allow_ip_bind, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		t.ID, t.UserID, t.TokenHash, t.TokenPreview, t.Status, t.ClientHint, t.AllowIPBind, t.ExpiresAt,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
}

func (r *TokenRepo) GetByValue(ctx context.Context, tokenValue string) (*model.SubscriptionToken, error) {
	tokenHash := hashToken(tokenValue)
	query := `
		SELECT id, user_id, token_hash, token_preview, status, client_hint, allow_ip_bind, bound_ip::text,
		       last_access_at, last_access_ip::text, expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE token_hash = $1 AND revoked_at IS NULL`
	t := &model.SubscriptionToken{TokenValue: tokenValue}
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint, &t.AllowIPBind, &t.BoundIP,
		&t.LastAccessAt, &t.LastAccessIP, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *TokenRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.SubscriptionToken, error) {
	query := `
		SELECT id, user_id, token_hash, token_preview, status, client_hint, allow_ip_bind, bound_ip::text,
		       last_access_at, last_access_ip::text, expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE id = $1`
	t := &model.SubscriptionToken{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint, &t.AllowIPBind, &t.BoundIP,
		&t.LastAccessAt, &t.LastAccessIP, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *TokenRepo) ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*model.SubscriptionToken, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, userID)
	argIdx++

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscription_tokens WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, token_hash, token_preview, status, client_hint, allow_ip_bind, bound_ip::text,
		       last_access_at, last_access_ip::text, expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tokens []*model.SubscriptionToken
	for rows.Next() {
		t := &model.SubscriptionToken{}
		err := rows.Scan(
			&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint, &t.AllowIPBind, &t.BoundIP,
			&t.LastAccessAt, &t.LastAccessIP, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		tokens = append(tokens, t)
	}
	return tokens, total, rows.Err()
}

func (r *TokenRepo) ListAll(ctx context.Context, page, pageSize int, status string, userID *uuid.UUID) ([]*model.SubscriptionToken, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "revoked_at IS NULL")
	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if userID != nil {
		where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, *userID)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscription_tokens WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, token_hash, token_preview, status, client_hint, allow_ip_bind, bound_ip::text,
		       last_access_at, last_access_ip::text, expires_at, revoked_at, created_at, updated_at
		FROM subscription_tokens WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tokens []*model.SubscriptionToken
	for rows.Next() {
		t := &model.SubscriptionToken{}
		err := rows.Scan(
			&t.ID, &t.UserID, &t.TokenHash, &t.TokenPreview, &t.Status, &t.ClientHint, &t.AllowIPBind, &t.BoundIP,
			&t.LastAccessAt, &t.LastAccessIP, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		tokens = append(tokens, t)
	}
	return tokens, total, rows.Err()
}

func (r *TokenRepo) RevokeToken(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE subscription_tokens SET status = $2, revoked_at = now(), updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, model.SubscriptionTokenStatusRevoked)
	return err
}

func (r *TokenRepo) UpdateAccess(ctx context.Context, id uuid.UUID, ip string, clientHint *string) error {
	now := time.Now()
	query := `UPDATE subscription_tokens SET last_access_at = $2, last_access_ip = $3, client_hint = COALESCE($4, client_hint), updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, now, ip, clientHint)
	return err
}

func (r *TokenRepo) UpdateExpiredStatus(ctx context.Context) error {
	query := `UPDATE subscription_tokens SET status = $2, updated_at = now() WHERE expires_at < now() AND status = $3 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, query, model.SubscriptionTokenStatusExpired, model.SubscriptionTokenStatusActive)
	return err
}

func (r *TokenRepo) GetActiveUserSubscription(ctx context.Context, userID uuid.UUID) (*model.UserPlanSubscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, started_at, expires_at,
		       traffic_quota_bytes, traffic_used_bytes,
		       COALESCE(upload_bytes, 0), COALESCE(download_bytes, 0),
		       reset_at
		FROM user_plan_subscriptions
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC LIMIT 1`
	s := &model.UserPlanSubscription{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.PlanID, &s.Status, &s.StartedAt, &s.ExpiresAt,
		&s.TrafficQuotaBytes, &s.TrafficUsedBytes, &s.UploadBytes, &s.DownloadBytes, &s.ResetAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *TokenRepo) GetPlanByID(ctx context.Context, planID uuid.UUID) (*model.Plan, error) {
	query := `SELECT id, code, name, traffic_bytes FROM plans WHERE id = $1 AND deleted_at IS NULL`
	p := &model.Plan{}
	err := r.pool.QueryRow(ctx, query, planID).Scan(&p.ID, &p.Code, &p.Name, &p.TrafficBytes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}
