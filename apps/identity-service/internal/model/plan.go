package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PlanStatus string

const (
	PlanStatusDraft     PlanStatus = "draft"
	PlanStatusActive    PlanStatus = "active"
	PlanStatusArchived  PlanStatus = "archived"
)

type BillingType string

const (
	BillingTypePeriodic  BillingType = "periodic"
	BillingTypeOneTime   BillingType = "one_time"
	BillingTypeTraffic   BillingType = "traffic"
)

type Plan struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	Code          string          `json:"code" db:"code"`
	Name          string          `json:"name" db:"name"`
	Description   string          `json:"description,omitempty" db:"description"`
	Content       string          `json:"content,omitempty" db:"content"`
	Status        PlanStatus      `json:"status" db:"status"`
	BillingType   BillingType     `json:"billing_type" db:"billing_type"`
	TrafficBytes  int64           `json:"traffic_bytes" db:"traffic_bytes"`
	SpeedLimitMbps *int           `json:"speed_limit_mbps,omitempty" db:"speed_limit_mbps"`
	DeviceLimit   *int            `json:"device_limit,omitempty" db:"device_limit"`
	IPLimit       *int            `json:"ip_limit,omitempty" db:"ip_limit"`
	ResetCycle    *string         `json:"reset_cycle,omitempty" db:"reset_cycle"`
	DurationDays  *int            `json:"duration_days,omitempty" db:"duration_days"`
	CanRenew      bool            `json:"can_renew" db:"can_renew"`
	SortOrder     int             `json:"sort_order" db:"sort_order"`
	// GroupID 关联 node_groups 表，决定用户购买此套餐后可见的节点分组
	GroupID       *uuid.UUID      `json:"group_id,omitempty" db:"group_id"`
	Tags          []string           `json:"tags" db:"tags"`
	Features      []string           `json:"features,omitempty" db:"-"`
	FeatureFlags  json.RawMessage    `json:"feature_flags" db:"feature_flags"`
	NodeCount     int                `json:"node_count" db:"-"`
	Prices        map[string]PlanPriceEntry `json:"prices,omitempty" db:"-"`
	CreatedAt     time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at" db:"updated_at"`
	DeletedAt     *time.Time         `json:"deleted_at,omitempty" db:"deleted_at"`
}

// PlanPriceEntry 同时持有 USDT 和 CNY 价格，供 GetPrices 返回
type PlanPriceEntry struct {
	USDT float64
	CNY  float64
}

type SubscriptionStatus string

const (
	SubscriptionStatusInactive  SubscriptionStatus = "inactive"
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
	SubscriptionStatusSuspended SubscriptionStatus = "suspended"
	SubscriptionStatusReplaced  SubscriptionStatus = "replaced"
)

type RenewalMode string

const (
	RenewalModeManual    RenewalMode = "manual"
	RenewalModeAuto      RenewalMode = "auto"
)

type CreatePlanRequest struct {
	// Code 套餐编码，用于 URL/订阅标识，service 层用正则 ^[a-z0-9]+(-[a-z0-9]+)*$ 校验
	Code           string                 `json:"code" binding:"required,min=2,max=64"`
	Name           string                 `json:"name" binding:"required,min=1,max=128"`
	Description    string                 `json:"description"`
	Content        string                 `json:"content"`
	Status         PlanStatus             `json:"status" binding:"required,oneof=draft active archived"`
	BillingType    BillingType            `json:"billing_type" binding:"required,oneof=periodic one_time traffic"`
	TrafficBytes   int64                  `json:"traffic_bytes" binding:"min=0"`
	SpeedLimitMbps *int                   `json:"speed_limit_mbps"`
	DeviceLimit    *int                   `json:"device_limit"`
	IPLimit        *int                   `json:"ip_limit"`
	ResetCycle     *string                `json:"reset_cycle"`
	DurationDays   *int                   `json:"duration_days"`
	CanRenew       bool                   `json:"can_renew"`
	SortOrder      int                    `json:"sort_order"`
	// GroupID 关联会员分组（可选），决定购买此套餐的用户可见节点范围
	GroupID        *uuid.UUID             `json:"group_id"`
	Tags           []string               `json:"tags"`
	FeatureFlags   map[string]interface{} `json:"feature_flags"`
	Prices         []PlanPrice            `json:"prices"`
}

type UpdatePlanRequest struct {
	Name           *string                `json:"name"`
	Description    *string                `json:"description"`
	Content        *string                `json:"content"`
	Status         *PlanStatus            `json:"status" binding:"omitempty,oneof=draft active archived"`
	BillingType    *BillingType           `json:"billing_type" binding:"omitempty,oneof=periodic one_time traffic"`
	TrafficBytes   *int64                 `json:"traffic_bytes"`
	SpeedLimitMbps *int                   `json:"speed_limit_mbps"`
	DeviceLimit    *int                   `json:"device_limit"`
	IPLimit        *int                   `json:"ip_limit"`
	ResetCycle     *string                `json:"reset_cycle"`
	DurationDays   *int                   `json:"duration_days"`
	CanRenew       *bool                  `json:"can_renew"`
	SortOrder      *int                   `json:"sort_order"`
	// GroupID 用于整体覆盖套餐的会员分组绑定
	// nil 表示不修改，传空字符串或 nil 值表示清空
	GroupID        *uuid.UUID             `json:"group_id"`
	Tags           []string               `json:"tags"`
	FeatureFlags   map[string]interface{} `json:"feature_flags"`
	Prices         []PlanPrice            `json:"prices"`
}

type PlanListQuery struct {
	Page        int    `form:"page"`
	PageSize    int    `form:"page_size"`
	Status      string `form:"status"`
	BillingType string `form:"billing_type"`
}

type UserPlanSubscription struct {
	ID               uuid.UUID          `json:"id" db:"id"`
	UserID           uuid.UUID          `json:"user_id" db:"user_id"`
	PlanID           uuid.UUID          `json:"plan_id" db:"plan_id"`
	PlanName         string             `json:"plan_name,omitempty" db:"plan_name"` // JOIN plans.name 得到，仅用于查询
	Status           SubscriptionStatus `json:"status" db:"status"`
	StartedAt        *time.Time         `json:"started_at,omitempty" db:"started_at"`
	ExpiresAt        *time.Time         `json:"expires_at,omitempty" db:"expires_at"`
	RenewalMode      RenewalMode        `json:"renewal_mode" db:"renewal_mode"`
	TrafficQuotaBytes int64             `json:"traffic_quota_bytes" db:"traffic_quota_bytes"`
	TrafficUsedBytes  int64             `json:"traffic_used_bytes" db:"traffic_used_bytes"`
	UploadBytes      int64              `json:"upload_bytes" db:"upload_bytes"`
	DownloadBytes    int64              `json:"download_bytes" db:"download_bytes"`
	ResetAt          *time.Time         `json:"reset_at,omitempty" db:"reset_at"`
	SpeedLimitMbps   *int               `json:"speed_limit_mbps,omitempty" db:"speed_limit_mbps"`
	DeviceLimit      *int               `json:"device_limit,omitempty" db:"device_limit"`
	IPLimit          *int               `json:"ip_limit,omitempty" db:"ip_limit"`
	Source           string             `json:"source" db:"source"`
	Metadata         json.RawMessage    `json:"metadata" db:"metadata"`
	CreatedAt        time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" db:"updated_at"`
	DeletedAt        *time.Time         `json:"deleted_at,omitempty" db:"deleted_at"`
}
