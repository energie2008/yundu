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
	TokenValue     string                  `json:"token_value,omitempty"`
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

type UserPlanSubscription struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	UserID            uuid.UUID  `json:"user_id" db:"user_id"`
	PlanID            uuid.UUID  `json:"plan_id" db:"plan_id"`
	Status            string     `json:"status" db:"status"`
	StartedAt         *time.Time `json:"started_at,omitempty" db:"started_at"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	TrafficQuotaBytes int64      `json:"traffic_quota_bytes" db:"traffic_quota_bytes"`
	TrafficUsedBytes  int64      `json:"traffic_used_bytes" db:"traffic_used_bytes"`
	UploadBytes       int64      `json:"upload_bytes" db:"upload_bytes"`
	DownloadBytes     int64      `json:"download_bytes" db:"download_bytes"`
	ResetAt           *time.Time `json:"reset_at,omitempty" db:"reset_at"`
}

type Plan struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Code         string    `json:"code" db:"code"`
	Name         string    `json:"name" db:"name"`
	TrafficBytes int64     `json:"traffic_bytes" db:"traffic_bytes"`
}

type UserSubscriptionInfo struct {
	UserID            uuid.UUID  `json:"user_id"`
	PlanID            *uuid.UUID `json:"plan_id,omitempty"`
	PlanName          string     `json:"plan_name,omitempty"`
	Upload            int64      `json:"upload"`
	Download          int64      `json:"download"`
	Total             int64      `json:"total"`
	TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`
	TrafficUsedBytes  int64      `json:"traffic_used_bytes"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	IsExpired         bool       `json:"is_expired"`
	IsOverQuota       bool       `json:"is_over_quota"`
	// GroupID 用户会员分组ID，用于按分组过滤可见节点（nil=可见全部节点）
	GroupID           *uuid.UUID `json:"group_id,omitempty"`
}
