package protocol

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000014_protocol_registry.sql)

// ProtocolRegistry 对应 protocol_registry 表，按协议/传输/安全组合注册 schema
type ProtocolRegistry struct {
	ID            uuid.UUID `json:"id"`
	ProtocolType  string    `json:"protocol_type"`
	TransportType string    `json:"transport_type"`
	SecurityType  string    `json:"security_type"`
	SchemaVersion string    `json:"schema_version"`
	ConfigSchema  Map       `json:"config_schema"`
	Description   *string   `json:"description,omitempty"`
	IsEnabled     bool      `json:"is_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ConfigTemplate 对应 config_templates 表，Go template 渲染 xray/sing-box 配置
type ConfigTemplate struct {
	ID              uuid.UUID `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	RuntimeType     string    `json:"runtime_type"`
	TemplateType    string    `json:"template_type"`
	Content         string    `json:"content"`
	VariablesSchema Map       `json:"variables_schema"`
	IsDefault       bool      `json:"is_default"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Map 是 JSONB 字段的通用类型（map[string]interface{} 的别名）
type Map = map[string]interface{}

// DTO: ProtocolRegistry

type CreateProtocolRequest struct {
	ProtocolType  string `json:"protocol_type" binding:"required,min=1,max=32"`
	TransportType string `json:"transport_type" binding:"required,min=1,max=32"`
	SecurityType  string `json:"security_type" binding:"required,min=1,max=32"`
	SchemaVersion string `json:"schema_version"`
	ConfigSchema  Map    `json:"config_schema" binding:"required"`
	Description   *string `json:"description"`
}

type UpdateProtocolRequest struct {
	ConfigSchema *Map   `json:"config_schema"`
	Description  *string `json:"description"`
	IsEnabled    *bool  `json:"is_enabled"`
}

type ProtocolListQuery struct {
	Page          int    `form:"page"`
	PageSize      int    `form:"page_size"`
	ProtocolType  string `form:"protocol_type"`
	TransportType string `form:"transport_type"`
	SecurityType  string `form:"security_type"`
	IsEnabled     string `form:"is_enabled"`
}

type ProtocolResponse struct {
	ID            uuid.UUID `json:"id"`
	ProtocolType  string    `json:"protocol_type"`
	TransportType string    `json:"transport_type"`
	SecurityType  string    `json:"security_type"`
	SchemaVersion string    `json:"schema_version"`
	ConfigSchema  Map       `json:"config_schema"`
	Description   *string   `json:"description,omitempty"`
	IsEnabled     bool      `json:"is_enabled"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

func NewProtocolResponse(p *ProtocolRegistry) ProtocolResponse {
	return ProtocolResponse{
		ID:            p.ID,
		ProtocolType:  p.ProtocolType,
		TransportType: p.TransportType,
		SecurityType:  p.SecurityType,
		SchemaVersion: p.SchemaVersion,
		ConfigSchema:  p.ConfigSchema,
		Description:   p.Description,
		IsEnabled:     p.IsEnabled,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// DTO: ConfigTemplate

type UpdateTemplateRequest struct {
	Name            *string `json:"name"`
	Content         *string `json:"content"`
	VariablesSchema *Map    `json:"variables_schema"`
	IsDefault       *bool   `json:"is_default"`
}

type TemplateListQuery struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"page_size"`
	RuntimeType  string `form:"runtime_type"`
	TemplateType string `form:"template_type"`
}

type TemplateResponse struct {
	ID              uuid.UUID `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	RuntimeType     string    `json:"runtime_type"`
	TemplateType    string    `json:"template_type"`
	Content         string    `json:"content"`
	VariablesSchema Map       `json:"variables_schema"`
	IsDefault       bool      `json:"is_default"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

func NewTemplateResponse(t *ConfigTemplate) TemplateResponse {
	return TemplateResponse{
		ID:              t.ID,
		Code:            t.Code,
		Name:            t.Name,
		RuntimeType:     t.RuntimeType,
		TemplateType:    t.TemplateType,
		Content:         t.Content,
		VariablesSchema: t.VariablesSchema,
		IsDefault:       t.IsDefault,
		CreatedAt:       t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// RenderTemplateRequest 用于 POST /config-templates/:code/render
type RenderTemplateRequest struct {
	Variables Map `json:"variables" binding:"required"`
}

type RenderTemplateResponse struct {
	Code     string `json:"code"`
	Rendered string `json:"rendered"`
}
