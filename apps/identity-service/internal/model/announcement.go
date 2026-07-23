package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AnnouncementType 公告类型
type AnnouncementType string

const (
	AnnouncementTypeMaintenance AnnouncementType = "maintenance" // 维护
	AnnouncementTypeUpdate      AnnouncementType = "update"      // 更新
	AnnouncementTypeNotice      AnnouncementType = "notice"      // 通知
	AnnouncementTypeAlert       AnnouncementType = "alert"       // 警报
)

// AnnouncementStatus 公告状态
type AnnouncementStatus string

const (
	AnnouncementStatusDraft     AnnouncementStatus = "draft"     // 草稿
	AnnouncementStatusPublished AnnouncementStatus = "published" // 已发布
	AnnouncementStatusArchived  AnnouncementStatus = "archived"  // 已归档
)

// AnnouncementAudience 公告目标群体
type AnnouncementAudience string

const (
	AnnouncementAudienceAll       AnnouncementAudience = "all"       // 全体
	AnnouncementAudienceUser      AnnouncementAudience = "user"      // 普通用户
	AnnouncementAudienceAdmin     AnnouncementAudience = "admin"     // 管理员
	AnnouncementAudienceSpecific  AnnouncementAudience = "specific"  // 特定用户
	AnnouncementAudiencePlanBased AnnouncementAudience = "plan_based" // 按套餐
)

// Announcement 公告
type Announcement struct {
	ID             uuid.UUID            `json:"id" db:"id"`
	Title          string               `json:"title" db:"title"`
	Content        string               `json:"content" db:"content"`
	Summary        *string              `json:"summary,omitempty" db:"summary"`
	Type           AnnouncementType     `json:"type" db:"type"`
	Status         AnnouncementStatus   `json:"status" db:"status"`
	TargetAudience AnnouncementAudience `json:"target_audience" db:"target_audience"`
	TargetFilter   json.RawMessage      `json:"target_filter" db:"target_filter"`
	EffectiveAt    *time.Time           `json:"effective_at,omitempty" db:"effective_at"`
	ExpiresAt      *time.Time           `json:"expires_at,omitempty" db:"expires_at"`
	Pinned         bool                 `json:"pinned" db:"pinned"`
	ViewCount      int                  `json:"view_count" db:"view_count"`
	ReadCount      int                  `json:"read_count" db:"read_count"`
	CreatedBy      *uuid.UUID           `json:"created_by,omitempty" db:"created_by"`
	PublishedAt    *time.Time           `json:"published_at,omitempty" db:"published_at"`
	ArchivedAt     *time.Time           `json:"archived_at,omitempty" db:"archived_at"`
	Metadata       json.RawMessage      `json:"metadata" db:"metadata"`
	CreatedAt      time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time           `json:"deleted_at,omitempty" db:"deleted_at"`

	// IsRead 临时字段：ListPublishedForUser 查询时填充，非数据库列。
	// 用于在用户端列表中标识当前用户是否已读该公告。
	IsRead         bool                 `json:"is_read" db:"-"`
}

// AnnouncementRead 公告已读记录
type AnnouncementRead struct {
	ID             uuid.UUID `json:"id" db:"id"`
	AnnouncementID uuid.UUID `json:"announcement_id" db:"announcement_id"`
	UserID         uuid.UUID `json:"user_id" db:"user_id"`
	ReadAt         time.Time `json:"read_at" db:"read_at"`
}
