package model

import (
	"time"

	"github.com/google/uuid"
)

type TrafficUsageDaily struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	UsageDate      time.Time  `json:"usage_date" db:"usage_date"`
	UserID         uuid.UUID  `json:"user_id" db:"user_id"`
	SubscriptionID *uuid.UUID `json:"subscription_id,omitempty" db:"subscription_id"`
	NodeID         *uuid.UUID `json:"node_id,omitempty" db:"node_id"`
	UploadBytes    int64      `json:"upload_bytes" db:"upload_bytes"`
	DownloadBytes  int64      `json:"download_bytes" db:"download_bytes"`
	TotalBytes     int64      `json:"total_bytes" db:"total_bytes"`
	UniqueDevices  int        `json:"unique_devices" db:"unique_devices"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
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

type QuotaCheckResult struct {
	IsOverQuota      bool  `json:"is_over_quota"`
	IsExpired        bool  `json:"is_expired"`
	TrafficQuota     int64 `json:"traffic_quota"`
	TrafficUsed      int64 `json:"traffic_used"`
	TrafficRemaining int64 `json:"traffic_remaining"`
}

// TrafficUsageAlert 流量使用告警条目（流量使用超过阈值需提醒的用户）。
type TrafficUsageAlert struct {
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Email      string    `json:"email" db:"email"`
	UsedBytes  int64     `json:"used_bytes" db:"used_bytes"`
	QuotaBytes int64     `json:"quota_bytes" db:"quota_bytes"`
}

// UsageRatio 返回已用流量占配额的比例（0~1+），配额为 0 时返回 0。
func (a TrafficUsageAlert) UsageRatio() float64 {
	if a.QuotaBytes <= 0 {
		return 0
	}
	return float64(a.UsedBytes) / float64(a.QuotaBytes)
}

// DailyStatistic 每日流量统计汇总（写入 traffic_statistics_daily 表）。
type DailyStatistic struct {
	StatDate      time.Time `json:"stat_date" db:"stat_date"`
	UploadBytes   int64     `json:"upload_bytes" db:"upload_bytes"`
	DownloadBytes int64     `json:"download_bytes" db:"download_bytes"`
	TotalBytes    int64     `json:"total_bytes" db:"total_bytes"`
	ActiveUsers   int       `json:"active_users" db:"active_users"`
	OnlineCount   int64     `json:"online_count" db:"online_count"`
}
