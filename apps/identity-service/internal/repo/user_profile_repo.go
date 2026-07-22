package repo

import (
	"context"
	"encoding/json"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserProfileRepo struct {
	pool *pgxpool.Pool
}

func NewUserProfileRepo(pool *pgxpool.Pool) *UserProfileRepo {
	return &UserProfileRepo{pool: pool}
}

func (r *UserProfileRepo) Create(ctx context.Context, profile *model.UserProfile) error {
	query := `
		INSERT INTO user_profiles (user_id, avatar_url, contact_email, phone, country_code, tags, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING updated_at`
	metadata := profile.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	tags := profile.Tags
	if tags == nil {
		tags = []string{}
	}
	return r.pool.QueryRow(ctx, query,
		profile.UserID, profile.AvatarURL, profile.ContactEmail,
		profile.Phone, profile.CountryCode, tags, metadata,
	).Scan(&profile.UpdatedAt)
}

func (r *UserProfileRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	query := `
		SELECT user_id, avatar_url, contact_email, phone, country_code, tags, metadata, updated_at
		FROM user_profiles WHERE user_id = $1`
	p := &model.UserProfile{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&p.UserID, &p.AvatarURL, &p.ContactEmail, &p.Phone,
		&p.CountryCode, &p.Tags, &p.Metadata, &p.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *UserProfileRepo) Update(ctx context.Context, profile *model.UserProfile) error {
	query := `
		UPDATE user_profiles SET
			avatar_url = $2, contact_email = $3, phone = $4, country_code = $5,
			tags = $6, metadata = $7, updated_at = now()
		WHERE user_id = $1`
	metadata := profile.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	tags := profile.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := r.pool.Exec(ctx, query,
		profile.UserID, profile.AvatarURL, profile.ContactEmail,
		profile.Phone, profile.CountryCode, tags, metadata,
	)
	return err
}
