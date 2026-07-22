package aidiag

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// 领域模型（对应迁移 000027 中的 diagnosis_sessions / diagnosis_knowledge 表）
// ============================================================================

// DiagnosisSession 一次 AI 诊断会话
type DiagnosisSession struct {
	ID                 uuid.UUID  `json:"id"`
	ServerID           *uuid.UUID `json:"server_id,omitempty"`
	NodeID             *uuid.UUID `json:"node_id,omitempty"`
	Status             string     `json:"status"` // pending / collecting / analyzing / done / failed
	TriggerSource      string     `json:"trigger_source"`
	TimeWindowStart    *time.Time `json:"time_window_start,omitempty"`
	TimeWindowEnd      *time.Time `json:"time_window_end,omitempty"`
	RawLogs            string     `json:"raw_logs,omitempty"`
	RawMetrics         map[string]interface{} `json:"raw_metrics"`
	LLMProvider        string     `json:"llm_provider"`
	LLMModel           string     `json:"llm_model,omitempty"`
	RootCauseCategory  string     `json:"root_cause_category,omitempty"`
	RootCauseDescription string   `json:"root_cause_description,omitempty"`
	Confidence         *float64   `json:"confidence,omitempty"`
	Suggestions        []Suggestion `json:"suggestions"`
	DocLinks           []DocLink  `json:"doc_links"`
	KnowledgeEntryID   *uuid.UUID `json:"knowledge_entry_id,omitempty"`
	AutofixApplied     bool       `json:"autofix_applied"`
	AutofixResult      map[string]interface{} `json:"autofix_result,omitempty"`
	DurationMs         *int       `json:"duration_ms,omitempty"`
	CreatedByAdminID   *uuid.UUID `json:"created_by_admin_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

// Suggestion 修复建议
type Suggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action,omitempty"`        // 自动修复动作编码
	AutoFixable bool   `json:"auto_fixable"`
}

// DocLink 相关文档链接
type DocLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// KnowledgeEntry 知识库条目
type KnowledgeEntry struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	Category         string    `json:"category"`
	RootCausePattern string    `json:"root_cause_pattern"`
	Solution         string    `json:"solution"`
	AutoFixAction    *string   `json:"auto_fix_action,omitempty"`
	DocLinks         []DocLink `json:"doc_links"`
	HitCount         int       `json:"hit_count"`
	IsVerified       bool      `json:"is_verified"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ============================================================================
// LLM 客户端抽象
// ============================================================================

// LLMRequest LLM 调用请求
type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float32
}

// LLMResponse LLM 调用响应
type LLMResponse struct {
	Content      string  // 原始文本
	FinishReason string
	TokensUsed   int
}

// LLMClient LLM 提供方抽象
type LLMClient interface {
	Provider() string
	Chat(ctx context.Context, req *LLMRequest) (*LLMResponse, error)
}

// ============================================================================
// 日志/指标采集器抽象
// ============================================================================

// LogCollector 日志采集器（从 node-agent 或 traffic-service 拉取）
type LogCollector interface {
	CollectLogs(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (string, error)
	CollectMetrics(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (map[string]interface{}, error)
}

// ============================================================================
// DTO
// ============================================================================

// CreateSessionRequest 创建诊断会话请求
type CreateSessionRequest struct {
	NodeID         *uuid.UUID `json:"node_id,omitempty"`
	ServerID       *uuid.UUID `json:"server_id,omitempty"`
	TimeWindowMins int        `json:"time_window_mins,omitempty"` // 默认 30
}

// ListSessionsQuery 列表查询
type ListSessionsQuery struct {
	Page     int        `form:"page"`
	PageSize int        `form:"page_size"`
	NodeID   *uuid.UUID `form:"node_id,omitempty"`
	ServerID *uuid.UUID `form:"server_id,omitempty"`
	Status   string     `form:"status,omitempty"`
}

// ApplyAutofixRequest 应用自动修复
type ApplyAutofixRequest struct {
	SuggestionIndex int `json:"suggestion_index"` // 建议索引
}
