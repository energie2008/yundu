package outbound

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000015_outbound_policies.sql)

// OutboundPolicy 对应 outbound_policies 表，节点出站策略
type OutboundPolicy struct {
	ID           uuid.UUID `json:"id"`
	NodeID       uuid.UUID `json:"node_id"`
	PolicyType   string    `json:"policy_type"`
	Priority     int       `json:"priority"`
	ConfigJSON   Map       `json:"config_json"`
	RoutingRules []Map     `json:"routing_rules"`
	IsEnabled    bool      `json:"is_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// WarpProfile 对应 warp_profiles 表，WARP 配置档案
type WarpProfile struct {
	ID         uuid.UUID `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	WarpMode   string    `json:"warp_mode"`
	Endpoint   *string   `json:"endpoint,omitempty"`
	LicenseKey *string   `json:"license_key,omitempty"`
	ConfigJSON Map       `json:"config_json"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Map 是 JSONB 字段的通用类型
type Map = map[string]interface{}

// DTO: OutboundPolicy

type CreatePolicyRequest struct {
	PolicyType   string `json:"policy_type" binding:"required,oneof=direct warp socks5 chain blackhole"`
	Priority     *int   `json:"priority"`
	ConfigJSON   Map    `json:"config_json"`
	RoutingRules []Map  `json:"routing_rules"`
	IsEnabled    *bool  `json:"is_enabled"`
}

type UpdatePolicyRequest struct {
	PolicyType   *string `json:"policy_type"`
	Priority     *int    `json:"priority"`
	ConfigJSON   *Map    `json:"config_json"`
	RoutingRules []Map   `json:"routing_rules"`
	IsEnabled    *bool   `json:"is_enabled"`
}

type PolicyResponse struct {
	ID           uuid.UUID `json:"id"`
	NodeID       uuid.UUID `json:"node_id"`
	PolicyType   string    `json:"policy_type"`
	Priority     int       `json:"priority"`
	ConfigJSON   Map       `json:"config_json"`
	RoutingRules []Map     `json:"routing_rules"`
	IsEnabled    bool      `json:"is_enabled"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

func NewPolicyResponse(p *OutboundPolicy) PolicyResponse {
	rules := p.RoutingRules
	if rules == nil {
		rules = []Map{}
	}
	return PolicyResponse{
		ID:           p.ID,
		NodeID:       p.NodeID,
		PolicyType:   p.PolicyType,
		Priority:     p.Priority,
		ConfigJSON:   p.ConfigJSON,
		RoutingRules: rules,
		IsEnabled:    p.IsEnabled,
		CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// DTO: WarpProfile

type CreateWarpProfileRequest struct {
	Code       string `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name       string `json:"name" binding:"required,min=1,max=128"`
	WarpMode   string `json:"warp_mode"`
	Endpoint   string `json:"endpoint"`
	LicenseKey string `json:"license_key"`
	ConfigJSON Map    `json:"config_json"`
	IsDefault  *bool  `json:"is_default"`
}

type WarpProfileResponse struct {
	ID         uuid.UUID `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	WarpMode   string    `json:"warp_mode"`
	Endpoint   *string   `json:"endpoint,omitempty"`
	LicenseKey *string   `json:"license_key,omitempty"`
	ConfigJSON Map       `json:"config_json"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

func NewWarpProfileResponse(w *WarpProfile) WarpProfileResponse {
	return WarpProfileResponse{
		ID:         w.ID,
		Code:       w.Code,
		Name:       w.Name,
		WarpMode:   w.WarpMode,
		Endpoint:   w.Endpoint,
		LicenseKey: w.LicenseKey,
		ConfigJSON: w.ConfigJSON,
		IsDefault:  w.IsDefault,
		CreatedAt:  w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  w.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ApplyAllResponse 用于 apply-all 返回渲染结果
type ApplyAllResponse struct {
	NodeID   uuid.UUID       `json:"node_id"`
	Xray     RenderedRuntime `json:"xray"`
	SingBox  RenderedRuntime `json:"sing_box"`
}

type RenderedRuntime struct {
	Outbounds    []Map `json:"outbounds"`
	RoutingRules []Map `json:"routing_rules"`
}
