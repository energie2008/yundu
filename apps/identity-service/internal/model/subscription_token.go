package model

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionTokenStatus string

const (
	SubscriptionTokenStatusActive  SubscriptionTokenStatus = "active"
	SubscriptionTokenStatusRevoked SubscriptionTokenStatus = "revoked"
	SubscriptionTokenStatusExpired SubscriptionTokenStatus = "expired"
)

type SubscriptionToken struct {
	ID             uuid.UUID               `json:"id" db:"id"`
	UserID         uuid.UUID               `json:"user_id" db:"user_id"`
	TokenHash      string                  `json:"-" db:"token_hash"`
	TokenPreview   string                  `json:"token_preview" db:"token_preview"`
	Status         SubscriptionTokenStatus `json:"status" db:"status"`
	ClientHint     *string                 `json:"client_hint,omitempty" db:"client_hint"`
	AllowIPBind    bool                    `json:"allow_ip_bind" db:"allow_ip_bind"`
	BoundIP        *string                 `json:"bound_ip,omitempty" db:"bound_ip"`
	LastAccessAt   *time.Time              `json:"last_access_at,omitempty" db:"last_access_at"`
	LastAccessIP   *string                 `json:"last_access_ip,omitempty" db:"last_access_ip"`
	ExpiresAt      *time.Time              `json:"expires_at,omitempty" db:"expires_at"`
	RevokedAt      *time.Time              `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt      time.Time               `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at" db:"updated_at"`
}
