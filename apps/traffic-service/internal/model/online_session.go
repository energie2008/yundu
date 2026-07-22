package model

import (
	"time"

	"github.com/google/uuid"
)

type OnlineSessionStatus string

const (
	OnlineSessionStatusOnline  OnlineSessionStatus = "online"
	OnlineSessionStatusOffline OnlineSessionStatus = "offline"
)

type OnlineSession struct {
	ID           uuid.UUID           `json:"id" db:"id"`
	UserID       uuid.UUID           `json:"user_id" db:"user_id"`
	CredentialID *uuid.UUID          `json:"credential_id,omitempty" db:"credential_id"`
	NodeID       *uuid.UUID          `json:"node_id,omitempty" db:"node_id"`
	RuntimeID    *uuid.UUID          `json:"runtime_id,omitempty" db:"runtime_id"`
	ClientIP     *string             `json:"client_ip,omitempty" db:"client_ip"`
	ClientType   *string             `json:"client_type,omitempty" db:"client_type"`
	ConnectedAt  time.Time           `json:"connected_at" db:"connected_at"`
	LastSeenAt   time.Time           `json:"last_seen_at" db:"last_seen_at"`
	DisconnectedAt *time.Time        `json:"disconnected_at,omitempty" db:"disconnected_at"`
	Status       OnlineSessionStatus `json:"status" db:"status"`
}
