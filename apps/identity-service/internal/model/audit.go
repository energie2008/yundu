package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ActorType string

const (
	ActorTypeUser  ActorType = "user"
	ActorTypeAdmin ActorType = "admin"
	ActorTypeSystem ActorType = "system"
)

type AuditLog struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	ActorType     ActorType       `json:"actor_type" db:"actor_type"`
	ActorID       *uuid.UUID      `json:"actor_id,omitempty" db:"actor_id"`
	ActorDisplay  *string         `json:"actor_display,omitempty" db:"actor_display"`
	Action        string          `json:"action" db:"action"`
	ResourceType  string          `json:"resource_type" db:"resource_type"`
	ResourceID    *uuid.UUID      `json:"resource_id,omitempty" db:"resource_id"`
	RequestID     *string         `json:"request_id,omitempty" db:"request_id"`
	IPAddress     *string         `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent     *string         `json:"user_agent,omitempty" db:"user_agent"`
	BeforeJSON    json.RawMessage `json:"before_json,omitempty" db:"before_json"`
	AfterJSON     json.RawMessage `json:"after_json,omitempty" db:"after_json"`
	Metadata      json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
}
