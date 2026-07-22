package exposure

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000010_edge_exposures.sql)

// EdgeExposure 对应 edge_exposures 表
type EdgeExposure struct {
	ID                    uuid.UUID  `json:"id"`
	ServerID              uuid.UUID  `json:"server_id"`
	Code                  string     `json:"code"`
	Name                  string     `json:"name"`
	ExposureMode          string     `json:"exposure_mode"`
	PublicHostname        *string    `json:"public_hostname,omitempty"`
	PublicPort            int        `json:"public_port"`
	OriginHost            string     `json:"origin_host"`
	OriginPort            int        `json:"origin_port"`
	NginxEnabled          bool       `json:"nginx_enabled"`
	NginxWSPath           *string    `json:"nginx_ws_path,omitempty"`
	NginxHostHeader       *string    `json:"nginx_host_header,omitempty"`
	NginxExtraConf        *string    `json:"nginx_extra_conf,omitempty"`
	TLSProfileID          *uuid.UUID `json:"tls_profile_id,omitempty"`
	CFTunnelTokenEncrypted *string   `json:"cf_tunnel_token_encrypted,omitempty"`
	CFTunnelID            *string    `json:"cf_tunnel_id,omitempty"`
	CFTunnelName          *string    `json:"cf_tunnel_name,omitempty"`
	CFProtocol            string     `json:"cf_protocol"`
	CFNoTLSVerify         bool       `json:"cf_no_tls_verify"`
	CFOriginServerName    *string    `json:"cf_origin_server_name,omitempty"`
	ArgoWSTokenEncrypted  *string    `json:"argo_ws_token_encrypted,omitempty"`
	Status                string     `json:"status"`
	HealthCheckURL        *string    `json:"health_check_url,omitempty"`
	LastHealthCheckAt     *time.Time `json:"last_health_check_at,omitempty"`
	LastHealthStatus      *string    `json:"last_health_status,omitempty"`
	Metadata              map[string]interface{} `json:"metadata"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// NginxGeneratedConfig 对应 nginx_generated_configs 表
type NginxGeneratedConfig struct {
	ID           uuid.UUID  `json:"id"`
	ExposureID   uuid.UUID  `json:"exposure_id"`
	ConfigContent string    `json:"config_content"`
	ConfigHash   string    `json:"config_hash"`
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt  time.Time  `json:"generated_at"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	DeployStatus string     `json:"deploy_status"`
	DeployError  *string    `json:"deploy_error,omitempty"`
}

// ExposureCompatRule 对应 exposure_compat_rules 表
type ExposureCompatRule struct {
	ID             uuid.UUID `json:"id"`
	ProtocolType   string    `json:"protocol_type"`
	TransportType  *string   `json:"transport_type,omitempty"`
	SecurityType   *string   `json:"security_type,omitempty"`
	ExposureMode   string    `json:"exposure_mode"`
	IsAllowed      bool      `json:"is_allowed"`
	Reason         *string   `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// DTO

type CreateExposureRequest struct {
	ServerID             uuid.UUID              `json:"server_id" binding:"required"`
	Code                 string                `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name                 string                `json:"name" binding:"required,min=1,max=128"`
	ExposureMode         string                `json:"exposure_mode" binding:"required"`
	PublicHostname       *string               `json:"public_hostname"`
	PublicPort           *int                  `json:"public_port"`
	OriginHost           string                `json:"origin_host"`
	OriginPort           int                   `json:"origin_port" binding:"required,min=1,max=65535"`
	NginxEnabled         *bool                 `json:"nginx_enabled"`
	NginxWSPath          *string               `json:"nginx_ws_path"`
	NginxHostHeader      *string               `json:"nginx_host_header"`
	NginxExtraConf       *string               `json:"nginx_extra_conf"`
	TLSProfileID         *uuid.UUID            `json:"tls_profile_id"`
	CFTunnelTokenEncrypted *string             `json:"cf_tunnel_token_encrypted"`
	CFTunnelID           *string               `json:"cf_tunnel_id"`
	CFTunnelName         *string               `json:"cf_tunnel_name"`
	CFProtocol           string                `json:"cf_protocol"`
	CFNoTLSVerify        *bool                 `json:"cf_no_tls_verify"`
	CFOriginServerName   *string               `json:"cf_origin_server_name"`
	ArgoWSTokenEncrypted *string               `json:"argo_ws_token_encrypted"`
	HealthCheckURL       *string               `json:"health_check_url"`
	Metadata             map[string]interface{} `json:"metadata"`
}

type UpdateExposureRequest struct {
	Name                 *string                `json:"name"`
	ExposureMode         *string                `json:"exposure_mode"`
	PublicHostname       *string                `json:"public_hostname"`
	PublicPort           *int                   `json:"public_port"`
	OriginHost           *string                `json:"origin_host"`
	OriginPort           *int                   `json:"origin_port"`
	NginxEnabled         *bool                  `json:"nginx_enabled"`
	NginxWSPath          *string                `json:"nginx_ws_path"`
	NginxHostHeader      *string                `json:"nginx_host_header"`
	NginxExtraConf       *string                `json:"nginx_extra_conf"`
	TLSProfileID         *uuid.UUID             `json:"tls_profile_id"`
	CFProtocol           *string                `json:"cf_protocol"`
	CFNoTLSVerify        *bool                  `json:"cf_no_tls_verify"`
	CFOriginServerName   *string                `json:"cf_origin_server_name"`
	Status               *string                `json:"status"`
	HealthCheckURL       *string                `json:"health_check_url"`
	Metadata             map[string]interface{} `json:"metadata"`
}

type ExposureListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

type ExposureResponse struct {
	ID             uuid.UUID  `json:"id"`
	ServerID       uuid.UUID  `json:"server_id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	ExposureMode   string     `json:"exposure_mode"`
	PublicHostname *string    `json:"public_hostname,omitempty"`
	PublicPort     int        `json:"public_port"`
	OriginHost     string     `json:"origin_host"`
	OriginPort     int        `json:"origin_port"`
	NginxEnabled   bool       `json:"nginx_enabled"`
	TLSProfileID   *uuid.UUID `json:"tls_profile_id,omitempty"`
	CFTunnelID     *string    `json:"cf_tunnel_id,omitempty"`
	CFProtocol     string     `json:"cf_protocol"`
	Status         string     `json:"status"`
	CreatedAt      string     `json:"created_at"`
}

func NewExposureResponse(e *EdgeExposure) ExposureResponse {
	return ExposureResponse{
		ID:             e.ID,
		ServerID:       e.ServerID,
		Code:           e.Code,
		Name:           e.Name,
		ExposureMode:   e.ExposureMode,
		PublicHostname: e.PublicHostname,
		PublicPort:     e.PublicPort,
		OriginHost:     e.OriginHost,
		OriginPort:     e.OriginPort,
		NginxEnabled:   e.NginxEnabled,
		TLSProfileID:   e.TLSProfileID,
		CFTunnelID:     e.CFTunnelID,
		CFProtocol:     e.CFProtocol,
		Status:         e.Status,
		CreatedAt:      e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// PreviewResponse 用于 POST /admin/servers/:id/exposure/preview
type PreviewResponse struct {
	NginxConf      string `json:"nginx_conf"`
	CloudflaredYML string `json:"cloudflared_yml"`
	Explanation    string `json:"explanation"`
}

// ValidateResponse 用于 POST /admin/servers/:id/exposure/validate
type ValidateResponse struct {
	IsAllowed bool   `json:"is_allowed"`
	Reason    string `json:"reason"`
}

// ApplyResponse 用于 POST /admin/servers/:id/exposure/apply（含 dry_run）
type ApplyResponse struct {
	Exposure       *ExposureResponse `json:"exposure"`
	DryRun         bool              `json:"dry_run"`
	NginxConf      string           `json:"nginx_conf,omitempty"`
	CloudflaredYML string           `json:"cloudflared_yml,omitempty"`
	NginxConfigHash string          `json:"nginx_config_hash,omitempty"`
	ValidateResult *ValidateResponse `json:"validate_result,omitempty"`
	Status         string           `json:"status"` // applying / applied / failed / dry_run
	Message        string           `json:"message,omitempty"`
}

// NodeInfo 用于 renderer / service 中获取节点的协议/传输/安全类型（由 app.go 注入查询回调）
type NodeInfo struct {
	ProtocolType  string
	TransportType string
	SecurityType  string
}
