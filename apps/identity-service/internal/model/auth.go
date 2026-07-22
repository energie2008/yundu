package model

import (
	"time"

	"github.com/google/uuid"
)

type SessionType string

const (
	SessionTypeWeb    SessionType = "web"
	SessionTypeMobile SessionType = "mobile"
	SessionTypeAPI    SessionType = "api"
)

type AuthSession struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	UserID            uuid.UUID   `json:"user_id" db:"user_id"`
	SessionType       SessionType `json:"session_type" db:"session_type"`
	TokenID           uuid.UUID   `json:"token_id" db:"token_id"`
	RefreshTokenID    *uuid.UUID  `json:"refresh_token_id,omitempty" db:"refresh_token_id"`
	UserAgent         *string     `json:"user_agent,omitempty" db:"user_agent"`
	IPAddress         *string     `json:"ip_address,omitempty" db:"ip_address"`
	DeviceFingerprint *string     `json:"device_fingerprint,omitempty" db:"device_fingerprint"`
	ExpiresAt         time.Time   `json:"expires_at" db:"expires_at"`
	RevokedAt         *time.Time  `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt         time.Time   `json:"created_at" db:"created_at"`
}
