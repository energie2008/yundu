package repo

import (
	"context"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthRepo struct {
	pool *pgxpool.Pool
}

func NewAuthRepo(pool *pgxpool.Pool) *AuthRepo {
	return &AuthRepo{pool: pool}
}

func (r *AuthRepo) CreateSession(ctx context.Context, session *model.AuthSession) error {
	query := `
		INSERT INTO auth_sessions (id, user_id, session_type, token_id, refresh_token_id, user_agent, ip_address, device_fingerprint, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		session.ID, session.UserID, session.SessionType, session.TokenID,
		session.RefreshTokenID, session.UserAgent, session.IPAddress,
		session.DeviceFingerprint, session.ExpiresAt,
	).Scan(&session.CreatedAt)
}

func (r *AuthRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenID uuid.UUID) (*model.AuthSession, error) {
	query := `
		SELECT id, user_id, session_type, token_id, refresh_token_id, user_agent, ip_address::text,
		       device_fingerprint, expires_at, revoked_at, created_at
		FROM auth_sessions WHERE refresh_token_id = $1 AND revoked_at IS NULL AND expires_at > now()`
	s := &model.AuthSession{}
	err := r.pool.QueryRow(ctx, query, refreshTokenID).Scan(
		&s.ID, &s.UserID, &s.SessionType, &s.TokenID, &s.RefreshTokenID,
		&s.UserAgent, &s.IPAddress, &s.DeviceFingerprint, &s.ExpiresAt,
		&s.RevokedAt, &s.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *AuthRepo) GetSessionByID(ctx context.Context, sessionID uuid.UUID) (*model.AuthSession, error) {
	query := `
		SELECT id, user_id, session_type, token_id, refresh_token_id, user_agent, ip_address::text,
		       device_fingerprint, expires_at, revoked_at, created_at
		FROM auth_sessions WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()`
	s := &model.AuthSession{}
	err := r.pool.QueryRow(ctx, query, sessionID).Scan(
		&s.ID, &s.UserID, &s.SessionType, &s.TokenID, &s.RefreshTokenID,
		&s.UserAgent, &s.IPAddress, &s.DeviceFingerprint, &s.ExpiresAt,
		&s.RevokedAt, &s.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *AuthRepo) RevokeSession(ctx context.Context, sessionID uuid.UUID) error {
	query := `UPDATE auth_sessions SET revoked_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, sessionID)
	return err
}

func (r *AuthRepo) RevokeAllSessions(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE auth_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *AuthRepo) DeleteExpiredSessions(ctx context.Context) error {
	query := `DELETE FROM auth_sessions WHERE expires_at < $1 OR (revoked_at IS NOT NULL AND revoked_at < $2)`
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	_, err := r.pool.Exec(ctx, query, time.Now(), cutoff)
	return err
}
