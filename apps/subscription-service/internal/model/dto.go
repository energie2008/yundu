package model

import (
	"time"

	"github.com/google/uuid"
)

type PaginationResponse struct {
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
	Items    interface{} `json:"items"`
}

type CreateTokenRequest struct {
	UserID    uuid.UUID  `json:"user_id" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type TokenResponse struct {
	ID           uuid.UUID               `json:"id"`
	UserID       uuid.UUID               `json:"user_id"`
	TokenValue   string                  `json:"token_value,omitempty"`
	TokenPreview string                  `json:"token_preview"`
	Status       SubscriptionTokenStatus `json:"status"`
	ExpiresAt    *time.Time              `json:"expires_at,omitempty"`
	LastAccessAt *time.Time              `json:"last_access_at,omitempty"`
	CreatedAt    string                  `json:"created_at"`
}

func NewTokenResponse(t *SubscriptionToken) TokenResponse {
	return TokenResponse{
		ID:           t.ID,
		UserID:       t.UserID,
		TokenPreview: t.TokenPreview,
		Status:       t.Status,
		ExpiresAt:    t.ExpiresAt,
		LastAccessAt: t.LastAccessAt,
		CreatedAt:    t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func NewTokenResponseWithValue(t *SubscriptionToken) TokenResponse {
	resp := NewTokenResponse(t)
	resp.TokenValue = t.TokenValue
	return resp
}

type SubscriptionInfoResponse struct {
	Upload            int64      `json:"upload"`
	Download          int64      `json:"download"`
	Total             int64      `json:"total"`
	Expire            *time.Time `json:"expire,omitempty"`
	TrafficQuotaBytes int64      `json:"traffic_quota_bytes"`
	TrafficUsedBytes  int64      `json:"traffic_used_bytes"`
	IsExpired         bool       `json:"is_expired"`
	IsOverQuota       bool       `json:"is_over_quota"`
}

type CreateTemplateRequest struct {
	Code         string     `json:"code" binding:"required,min=2,max=64"`
	Name         string     `json:"name" binding:"required,min=1,max=128"`
	TargetClient ClientType `json:"target_client" binding:"required"`
	Content      string     `json:"content" binding:"required"`
}

type UpdateTemplateRequest struct {
	ID           uuid.UUID  `json:"-"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	TargetClient ClientType `json:"target_client"`
	Content      string     `json:"content"`
	Status       string     `json:"status"`
}

type ShortCodeCreateRequest struct {
	Token       string `json:"token" binding:"required"`
	Description string `json:"description"`
	ExpiresIn   int    `json:"expires_in"`
}

type TemplateResponse struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	TargetClient  ClientType `json:"target_client"`
	IsDefault     bool       `json:"is_default"`
	Status        string     `json:"status"`
	SchemaVersion string     `json:"schema_version"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

func NewTemplateResponse(t *SubscriptionTemplate, isDefault bool) TemplateResponse {
	return TemplateResponse{
		ID:            t.ID,
		Code:          t.Code,
		Name:          t.Name,
		TargetClient:  t.TargetClient,
		IsDefault:     t.IsDefault,
		Status:        string(t.Status),
		SchemaVersion: t.SchemaVersion,
		CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
