package model

import (
	"time"

	"github.com/google/uuid"
)

type SubscriptionAccessLog struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	TokenID            *uuid.UUID `json:"token_id,omitempty" db:"token_id"`
	UserID             *uuid.UUID `json:"user_id,omitempty" db:"user_id"`
	ClientType         *string    `json:"client_type,omitempty" db:"client_type"`
	RequestIP          *string    `json:"request_ip,omitempty" db:"request_ip"`
	UserAgent          *string    `json:"user_agent,omitempty" db:"user_agent"`
	ResponseStatus     int        `json:"response_status" db:"response_status"`
	TemplateCode       *string    `json:"template_code,omitempty" db:"template_code"`
	GeneratedNodeCount int        `json:"generated_node_count" db:"generated_node_count"`
	CacheHit           bool       `json:"cache_hit" db:"cache_hit"`
	RequestedAt        time.Time  `json:"requested_at" db:"requested_at"`
}

type ClientStat struct {
	ClientType string `json:"client_type" db:"client_type"`
	Count      int64  `json:"count" db:"count"`
}

type AccessLogStats struct {
	TotalRequests      int64            `json:"total_requests"`
	UniqueIPs          int64            `json:"unique_ips"`
	ClientDistribution map[string]int64 `json:"client_distribution"`
	CacheHitRate       float64          `json:"cache_hit_rate"`
}

type AccessLogOverview struct {
	TotalRequests int64         `json:"total_requests"`
	UniqueUsers   int64         `json:"unique_users"`
	UniqueIPs     int64         `json:"unique_ips"`
	TopClients    []*ClientStat `json:"top_clients"`
}
