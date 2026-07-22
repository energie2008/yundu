package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TicketCategory 工单分类
type TicketCategory string

const (
	TicketCategoryConsultation TicketCategory = "consultation" // 咨询
	TicketCategoryFault        TicketCategory = "fault"        // 故障
	TicketCategoryComplaint    TicketCategory = "complaint"    // 投诉
	TicketCategoryRefund       TicketCategory = "refund"       // 退款
	TicketCategoryOther        TicketCategory = "other"        // 其他
)

// TicketPriority 工单优先级
type TicketPriority string

const (
	TicketPriorityLow    TicketPriority = "low"
	TicketPriorityNormal TicketPriority = "normal"
	TicketPriorityHigh   TicketPriority = "high"
	TicketPriorityUrgent TicketPriority = "urgent"
)

// TicketStatus 工单状态
type TicketStatus string

const (
	TicketStatusOpen       TicketStatus = "open"       // 待处理
	TicketStatusInProgress TicketStatus = "in_progress" // 处理中
	TicketStatusResolved   TicketStatus = "resolved"   // 已解决
	TicketStatusClosed     TicketStatus = "closed"     // 已关闭
)

// AuthorType 回复作者类型
type AuthorType string

const (
	AuthorTypeUser  AuthorType = "user"
	AuthorTypeAdmin AuthorType = "admin"
)

// Ticket 工单主表
type Ticket struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	UserID              uuid.UUID       `json:"user_id" db:"user_id"`
	Subject             string          `json:"subject" db:"subject"`
	Description         string          `json:"description" db:"description"`
	Category            TicketCategory  `json:"category" db:"category"`
	Priority            TicketPriority  `json:"priority" db:"priority"`
	Status              TicketStatus    `json:"status" db:"status"`
	AssignedAdminID     *uuid.UUID      `json:"assigned_admin_id,omitempty" db:"assigned_admin_id"`
	RelatedResourceType *string         `json:"related_resource_type,omitempty" db:"related_resource_type"`
	RelatedResourceID   *uuid.UUID      `json:"related_resource_id,omitempty" db:"related_resource_id"`
	ReplyCount          int             `json:"reply_count" db:"reply_count"`
	LastReplyAt         *time.Time      `json:"last_reply_at,omitempty" db:"last_reply_at"`
	ClosedAt            *time.Time      `json:"closed_at,omitempty" db:"closed_at"`
	Metadata            json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
	DeletedAt           *time.Time      `json:"deleted_at,omitempty" db:"deleted_at"`
}

// TicketReply 工单回复
type TicketReply struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	TicketID   uuid.UUID       `json:"ticket_id" db:"ticket_id"`
	AuthorID   uuid.UUID       `json:"author_id" db:"author_id"`
	AuthorType AuthorType      `json:"author_type" db:"author_type"`
	Content    string          `json:"content" db:"content"`
	Attachments json.RawMessage `json:"attachments" db:"attachments"`
	IsInternal bool            `json:"is_internal" db:"is_internal"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at" db:"updated_at"`
}
