package importer

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000012_node_doctor.sql 中的 config_import_jobs 表)

// ImportJob 对应 config_import_jobs 表
type ImportJob struct {
	ID               uuid.UUID              `json:"id"`
	SourceType       string                 `json:"source_type"`
	RawContent       string                 `json:"raw_content"`
	ParseResult      map[string]interface{} `json:"parse_result"`
	ParseStatus      string                 `json:"parse_status"`
	ParseError       *string                `json:"parse_error,omitempty"`
	PreviewNodeSpec  *NodeSpec              `json:"preview_node_spec,omitempty"`
	ApplyStatus      string                 `json:"apply_status"`
	AppliedNodeID    *uuid.UUID             `json:"applied_node_id,omitempty"`
	CreatedByAdminID *uuid.UUID             `json:"created_by_admin_id,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	AppliedAt        *time.Time             `json:"applied_at,omitempty"`
}

// NodeSpec 结构化预览（多 Parser 合并后的节点规格）
type NodeSpec struct {
	ProtocolType   string                 `json:"protocol_type"`
	TransportType  string                 `json:"transport_type"`
	SecurityType   string                 `json:"security_type"`
	ListenPort     int                    `json:"listen_port"`
	UUIDs          []string               `json:"uuids,omitempty"`
	SNI            string                 `json:"sni,omitempty"`
	CertPath       string                 `json:"cert_path,omitempty"`
	RawMetadata    map[string]interface{} `json:"raw_metadata,omitempty"`
	MissingFields  []string               `json:"missing_fields,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Host           string                 `json:"host,omitempty"`
	Port           int                    `json:"port,omitempty"`
	Password       string                 `json:"password,omitempty"`
	UUID           string                 `json:"uuid,omitempty"`
	ConfigJSON     map[string]interface{} `json:"config_json,omitempty"`
}

// ParseResult 解析结果汇总
type ParseResult struct {
	SourceType  string                 `json:"source_type"`
	NodeSpec    *NodeSpec              `json:"node_spec"`
	Warnings    []string               `json:"warnings,omitempty"`
	RawExtract  map[string]interface{} `json:"raw_extract,omitempty"`
}

// DTO

type CreateImportRequest struct {
	SourceType string `json:"source_type" binding:"required,oneof=xray singbox nginx cloudflared"`
	Content    string `json:"content" binding:"required"`
}

type ImportJobResponse struct {
	ID            uuid.UUID  `json:"id"`
	SourceType    string     `json:"source_type"`
	ParseStatus   string     `json:"parse_status"`
	ParseError    *string     `json:"parse_error,omitempty"`
	PreviewNodeSpec *NodeSpec `json:"preview_node_spec,omitempty"`
	ApplyStatus   string     `json:"apply_status"`
	AppliedNodeID *uuid.UUID `json:"applied_node_id,omitempty"`
	CreatedAt     string     `json:"created_at"`
	AppliedAt     *string    `json:"applied_at,omitempty"`
}

func NewImportJobResponse(j *ImportJob) ImportJobResponse {
	var appliedAt *string
	if j.AppliedAt != nil {
		s := j.AppliedAt.Format("2006-01-02T15:04:05Z07:00")
		appliedAt = &s
	}
	return ImportJobResponse{
		ID:              j.ID,
		SourceType:      j.SourceType,
		ParseStatus:      j.ParseStatus,
		ParseError:       j.ParseError,
		PreviewNodeSpec:  j.PreviewNodeSpec,
		ApplyStatus:      j.ApplyStatus,
		AppliedNodeID:    j.AppliedNodeID,
		CreatedAt:        j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		AppliedAt:        appliedAt,
	}
}

// ParseResponse 用于 POST /admin/config-import 的响应
type ParseResponse struct {
	JobID         uuid.UUID    `json:"job_id"`
	ParseResult   ParseResult  `json:"parse_result"`
	PreviewNodeSpec *NodeSpec  `json:"preview_node_spec"`
}

// ApplyResponse 用于 POST /admin/config-import/:id/apply
type ApplyResponse struct {
	JobID      uuid.UUID  `json:"job_id"`
	ApplyStatus string    `json:"apply_status"`
	AppliedNodeID *uuid.UUID `json:"applied_node_id,omitempty"`
}
