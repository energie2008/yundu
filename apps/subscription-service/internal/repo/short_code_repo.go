package repo

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const shortCodeLength = 8

type ShortCodeRepo struct {
	pool *pgxpool.Pool
}

func NewShortCodeRepo(pool *pgxpool.Pool) *ShortCodeRepo {
	return &ShortCodeRepo{pool: pool}
}

func generateShortCode() (string, error) {
	code := make([]byte, shortCodeLength)
	base := big.NewInt(int64(len(base62Chars)))
	for i := 0; i < shortCodeLength; i++ {
		num, err := rand.Int(rand.Reader, base)
		if err != nil {
			return "", err
		}
		code[i] = base62Chars[num.Int64()]
	}
	return string(code), nil
}

func (r *ShortCodeRepo) Create(ctx context.Context, sc *model.SubscriptionShortCode) error {
	var err error
	sc.ShortCode, err = generateShortCode()
	if err != nil {
		return err
	}
	query := `
		INSERT INTO subscription_short_codes (id, short_code, token_id, user_id, description, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		sc.ID, sc.ShortCode, sc.TokenID, sc.UserID, sc.Description, sc.ExpiresAt,
	).Scan(&sc.CreatedAt)
}

func (r *ShortCodeRepo) GetByCode(ctx context.Context, code string) (*model.SubscriptionShortCode, error) {
	query := `
		SELECT id, short_code, token_id, user_id, COALESCE(description, ''), created_at, expires_at
		FROM subscription_short_codes WHERE short_code = $1 AND (expires_at IS NULL OR expires_at > now())`
	sc := &model.SubscriptionShortCode{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&sc.ID, &sc.ShortCode, &sc.TokenID, &sc.UserID, &sc.Description, &sc.CreatedAt, &sc.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return sc, nil
}

func (r *ShortCodeRepo) GetByTokenID(ctx context.Context, tokenID uuid.UUID) (*model.SubscriptionShortCode, error) {
	query := `
		SELECT id, short_code, token_id, user_id, COALESCE(description, ''), created_at, expires_at
		FROM subscription_short_codes WHERE token_id = $1 AND (expires_at IS NULL OR expires_at > now())`
	sc := &model.SubscriptionShortCode{}
	err := r.pool.QueryRow(ctx, query, tokenID).Scan(
		&sc.ID, &sc.ShortCode, &sc.TokenID, &sc.UserID, &sc.Description, &sc.CreatedAt, &sc.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return sc, nil
}

func (r *ShortCodeRepo) RevokeByTokenID(ctx context.Context, tokenID uuid.UUID) error {
	query := `DELETE FROM subscription_short_codes WHERE token_id = $1`
	_, err := r.pool.Exec(ctx, query, tokenID)
	return err
}

func (r *ShortCodeRepo) RevokeByCode(ctx context.Context, code string) error {
	query := `DELETE FROM subscription_short_codes WHERE short_code = $1`
	_, err := r.pool.Exec(ctx, query, code)
	return err
}
