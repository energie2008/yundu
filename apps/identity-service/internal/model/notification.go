package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NotificationCategory 通知分类
type NotificationCategory string

const (
	NotificationCategoryTrafficExpiry  NotificationCategory = "traffic_expiry"  // 流量到期
	NotificationCategoryPlanExpiry    NotificationCategory = "plan_expiry"     // 套餐到期
	NotificationCategoryNodeChange     NotificationCategory = "node_change"     // 节点变更
	NotificationCategorySystem         NotificationCategory = "system"          // 系统通知
	NotificationCategorySubscription  NotificationCategory = "subscription"     // 订阅
	NotificationCategoryBilling        NotificationCategory = "billing"         // 账单
	NotificationCategoryTicket        NotificationCategory = "ticket"           // 工单
	NotificationCategoryAnnouncement  NotificationCategory = "announcement"    // 公告
)

// NotificationChannel 通知渠道
type NotificationChannel string

const (
	NotificationChannelInApp     NotificationChannel = "in_app"     // 站内信
	NotificationChannelEmail     NotificationChannel = "email"       // 邮件
	NotificationChannelTelegram NotificationChannel = "telegram"    // Telegram
	NotificationChannelBark      NotificationChannel = "bark"        // Bark
)

// NotificationStatus 通知状态
type NotificationStatus string

const (
	NotificationStatusPending   NotificationStatus = "pending"   // 待发送
	NotificationStatusSent      NotificationStatus = "sent"        // 已发送
	NotificationStatusDelivered NotificationStatus = "delivered"  // 已送达
	NotificationStatusFailed    NotificationStatus = "failed"      // 发送失败
	NotificationStatusRead      NotificationStatus = "read"        // 已读
)

// NotificationPriority 通知优先级
type NotificationPriority string

const (
	NotificationPriorityLow    NotificationPriority = "low"
	NotificationPriorityNormal NotificationPriority = "normal"
	NotificationPriorityHigh   NotificationPriority = "high"
	NotificationPriorityUrgent NotificationPriority = "urgent"
)

// Notification 通知
type Notification struct {
	ID                 uuid.UUID            `json:"id" db:"id"`
	UserID             uuid.UUID            `json:"user_id" db:"user_id"`
	Category           NotificationCategory `json:"category" db:"category"`
	Title              string               `json:"title" db:"title"`
	Content            string               `json:"content" db:"content"`
	Channel            NotificationChannel  `json:"channel" db:"channel"`
	Status             NotificationStatus   `json:"status" db:"status"`
	Priority           NotificationPriority `json:"priority" db:"priority"`
	Metadata           json.RawMessage      `json:"metadata" db:"metadata"`
	TargetResourceType *string              `json:"target_resource_type,omitempty" db:"target_resource_type"`
	TargetResourceID   *uuid.UUID           `json:"target_resource_id,omitempty" db:"target_resource_id"`
	TemplateCode       *string              `json:"template_code,omitempty" db:"template_code"`
	ScheduledAt        *time.Time           `json:"scheduled_at,omitempty" db:"scheduled_at"`
	SentAt             *time.Time           `json:"sent_at,omitempty" db:"sent_at"`
	ReadAt             *time.Time           `json:"read_at,omitempty" db:"read_at"`
	ArchivedAt         *time.Time           `json:"archived_at,omitempty" db:"archived_at"`
	CreatedAt         time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at" db:"updated_at"`
}

// NotificationTemplate 通知模板
type NotificationTemplate struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	Code          string          `json:"code" db:"code"`
	Name          string          `json:"name" db:"name"`
	Description   string          `json:"description" db:"description"`
	Category      NotificationCategory `json:"category" db:"category"`
	Channel       NotificationChannel  `json:"channel" db:"channel"`
	TitleTemplate string          `json:"title_template" db:"title_template"`
	BodyTemplate  string          `json:"body_template" db:"body_template"`
	Variables     json.RawMessage `json:"variables" db:"variables"`
	Enabled       bool            `json:"enabled" db:"enabled"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}
