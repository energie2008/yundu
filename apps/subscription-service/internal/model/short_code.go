package model

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionShortCode struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	ShortCode   string     `json:"short_code" db:"short_code"`
	TokenID     uuid.UUID  `json:"token_id" db:"token_id"`
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	Description string     `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}
