package model

import (
	"time"

	"github.com/google/uuid"
)

type ConfigVersionStatus string

const (
	ConfigVersionStatusDraft     ConfigVersionStatus = "draft"
	ConfigVersionStatusPending   ConfigVersionStatus = "pending"
	ConfigVersionStatusActive    ConfigVersionStatus = "active"
	ConfigVersionStatusDeployed  ConfigVersionStatus = "deployed"
	ConfigVersionStatusRolledBack ConfigVersionStatus = "rolled_back"
)

type ConfigSource string

const (
	ConfigSourceSystem ConfigSource = "system"
	ConfigSourceAdmin  ConfigSource = "admin"
	ConfigSourceAuto   ConfigSource = "auto"
)

type ScopeType string

const (
	ScopeTypeNode    ScopeType = "node"
	ScopeTypeRuntime ScopeType = "runtime"
	ScopeTypeGroup   ScopeType = "group"
	ScopeTypeGlobal  ScopeType = "global"
)

type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusPaused    DeploymentStatus = "paused"
	DeploymentStatusSuccess   DeploymentStatus = "success"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusRolledBack DeploymentStatus = "rolled_back"
)

type DeploymentStrategy string

const (
	DeploymentStrategyRolling  DeploymentStrategy = "rolling"
	DeploymentStrategyBlueGreen DeploymentStrategy = "blue_green"
	DeploymentStrategyCanary   DeploymentStrategy = "canary"
	DeploymentStrategyAllAtOnce DeploymentStrategy = "all_at_once"
)

type TargetType string

const (
	TargetTypeNode    TargetType = "node"
	TargetTypeRuntime TargetType = "runtime"
)

type TargetStatus string

const (
	TargetStatusPending   TargetStatus = "pending"
	TargetStatusPrecheck  TargetStatus = "precheck"
	TargetStatusApplying  TargetStatus = "applying"
	TargetStatusVerifying TargetStatus = "verifying"
	TargetStatusSuccess   TargetStatus = "success"
	TargetStatusFailed    TargetStatus = "failed"
	TargetStatusRollingBack TargetStatus = "rolling_back"
	TargetStatusRolledBack TargetStatus = "rolled_back"
	TargetStatusPaused    TargetStatus = "paused"
)

type ConfigVersion struct {
	ID             uuid.UUID              `json:"id"`
	ScopeType      ScopeType              `json:"scope_type"`
	ScopeID        uuid.UUID              `json:"scope_id"`
	VersionNo      int64                  `json:"version_no"`
	Status         ConfigVersionStatus    `json:"status"`
	Source         ConfigSource           `json:"source"`
	SchemaVersion  string                 `json:"schema_version"`
	ContentJSON    map[string]interface{} `json:"content_json"`
	ContentHash    string                 `json:"content_hash"`
	CreatedByAdminID *uuid.UUID           `json:"created_by_admin_id,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	PublishedAt    *time.Time             `json:"published_at,omitempty"`
}

type DeploymentBatch struct {
	ID               uuid.UUID              `json:"id"`
	ScopeType        ScopeType              `json:"scope_type"`
	ScopeID          uuid.UUID              `json:"scope_id"`
	TargetVersionID  uuid.UUID              `json:"target_version_id"`
	Strategy         DeploymentStrategy     `json:"strategy"`
	BatchPlan        []interface{}          `json:"batch_plan"`
	Status           DeploymentStatus       `json:"status"`
	StartedAt        *time.Time             `json:"started_at,omitempty"`
	FinishedAt       *time.Time             `json:"finished_at,omitempty"`
	CreatedByAdminID *uuid.UUID             `json:"created_by_admin_id,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

type DeploymentTarget struct {
	ID                uuid.UUID              `json:"id"`
	DeploymentBatchID uuid.UUID              `json:"deployment_batch_id"`
	TargetType        TargetType             `json:"target_type"`
	TargetID          uuid.UUID              `json:"target_id"`
	TargetVersionID   uuid.UUID              `json:"target_version_id"`
	PreviousVersionID *uuid.UUID             `json:"previous_version_id,omitempty"`
	PhaseNo           int                    `json:"phase_no"`
	Status            TargetStatus           `json:"status"`
	PrecheckResult    map[string]interface{} `json:"precheck_result"`
	ApplyResult       map[string]interface{} `json:"apply_result"`
	RollbackResult    map[string]interface{} `json:"rollback_result"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	FinishedAt        *time.Time             `json:"finished_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}
