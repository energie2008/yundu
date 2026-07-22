package upgrade

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000016_runtime_upgrade.sql)

// UpgradeStatus 升级任务状态
type UpgradeStatus string

const (
	StatusPending    UpgradeStatus = "pending"
	StatusRunning    UpgradeStatus = "running"
	StatusSucceeded  UpgradeStatus = "succeeded"
	StatusFailed     UpgradeStatus = "failed"
	StatusRolledBack UpgradeStatus = "rolled_back"
)

// UpgradeScope 升级范围
type UpgradeScope string

const (
	ScopeSingle UpgradeScope = "single"
	ScopeBatch  UpgradeScope = "batch"
	ScopeCanary UpgradeScope = "canary"
)

// IsTerminal 判断状态是否为终态
func (s UpgradeStatus) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusRolledBack
}

// RuntimeUpgradeTask 对应 runtime_upgrade_tasks 表
type RuntimeUpgradeTask struct {
	ID              uuid.UUID      `json:"id"`
	ServerID        uuid.UUID      `json:"server_id"`
	RuntimeID       uuid.UUID      `json:"runtime_id"`
	FromVersion     string         `json:"from_version"`
	ToVersion       string         `json:"to_version"`
	Status          UpgradeStatus  `json:"status"`
	Scope           UpgradeScope   `json:"scope"`
	BatchID         *uuid.UUID     `json:"batch_id,omitempty"`
	CanaryPercent   *int           `json:"canary_percent,omitempty"`
	DownloadURL     string         `json:"download_url"`
	ExpectedSha256  string         `json:"expected_sha256"`
	StartedAt       *time.Time    `json:"started_at,omitempty"`
	CompletedAt     *time.Time    `json:"completed_at,omitempty"`
	ErrorMessage    *string        `json:"error_message,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// DTO

// CreateRequest 用于 POST /admin/servers/:id/runtime-upgrade（server_id 来自路径）
type CreateRequest struct {
	RuntimeID      uuid.UUID `json:"runtime_id" binding:"required"`
	ToVersion      string    `json:"to_version" binding:"required,min=1"`
	DownloadURL    string    `json:"download_url"`
	ExpectedSha256 string    `json:"expected_sha256"`
}

// BatchItem 批量升级中的单项
type BatchItem struct {
	ServerID       uuid.UUID `json:"server_id" binding:"required"`
	RuntimeID      uuid.UUID `json:"runtime_id" binding:"required"`
	ToVersion      string    `json:"to_version" binding:"required,min=1"`
	DownloadURL    string    `json:"download_url"`
	ExpectedSha256 string    `json:"expected_sha256"`
}

// BatchCreateRequest 用于 POST /admin/runtime-upgrades/batch
type BatchCreateRequest struct {
	Items []BatchItem `json:"items" binding:"required,min=1"`
}

// CanaryTarget 灰度升级的目标节点
type CanaryTarget struct {
	ServerID  uuid.UUID `json:"server_id" binding:"required"`
	RuntimeID uuid.UUID `json:"runtime_id" binding:"required"`
}

// CanaryUpgradeRequest 用于 POST /admin/runtime-upgrades/canary
type CanaryUpgradeRequest struct {
	ToVersion      string         `json:"to_version" binding:"required,min=1"`
	DownloadURL    string         `json:"download_url"`
	ExpectedSha256 string         `json:"expected_sha256"`
	CanaryPercent  int            `json:"canary_percent" binding:"required,min=1,max=100"`
	Targets        []CanaryTarget `json:"targets" binding:"required,min=1"`
}

// ListQuery 用于 GET /admin/servers/:id/runtime-upgrades
type ListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

// Response 升级任务对外响应
type Response struct {
	ID              uuid.UUID      `json:"id"`
	ServerID        uuid.UUID      `json:"server_id"`
	RuntimeID       uuid.UUID      `json:"runtime_id"`
	FromVersion     string         `json:"from_version"`
	ToVersion       string         `json:"to_version"`
	Status          UpgradeStatus  `json:"status"`
	Scope           UpgradeScope   `json:"scope"`
	BatchID         *uuid.UUID     `json:"batch_id,omitempty"`
	CanaryPercent   *int           `json:"canary_percent,omitempty"`
	DownloadURL     string         `json:"download_url"`
	ExpectedSha256  string         `json:"expected_sha256"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	ErrorMessage    *string        `json:"error_message,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

// NewResponse 从领域模型构造响应
func NewResponse(t *RuntimeUpgradeTask) Response {
	return Response{
		ID:             t.ID,
		ServerID:       t.ServerID,
		RuntimeID:      t.RuntimeID,
		FromVersion:     t.FromVersion,
		ToVersion:      t.ToVersion,
		Status:         t.Status,
		Scope:          t.Scope,
		BatchID:        t.BatchID,
		CanaryPercent:  t.CanaryPercent,
		DownloadURL:    t.DownloadURL,
		ExpectedSha256: t.ExpectedSha256,
		StartedAt:      t.StartedAt,
		CompletedAt:    t.CompletedAt,
		ErrorMessage:   t.ErrorMessage,
		CreatedAt:       t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
