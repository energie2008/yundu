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

	// migration 000068：sing-box 原生 wireguard 字段
	PrivateKey   *string `json:"private_key,omitempty" db:"private_key"`
	PublicKey    *string `json:"public_key,omitempty" db:"public_key"`
	LocalAddress *string `json:"local_address,omitempty" db:"local_address"`
	MTU          int     `json:"mtu" db:"mtu"`

	// migration 000069：WARP 账户注册元数据（warpreg 模块使用）
	DeviceID     *string    `json:"device_id,omitempty" db:"device_id"`
	AccessToken  *string    `json:"access_token,omitempty" db:"access_token"`
	ClientID     *string    `json:"client_id,omitempty" db:"client_id"`
	IPv4Address  *string    `json:"ipv4_address,omitempty" db:"ipv4_address"`
	IPv6Address  *string    `json:"ipv6_address,omitempty" db:"ipv6_address"`
	Status       string     `json:"status" db:"status"`
	NodeID       *uuid.UUID `json:"node_id,omitempty" db:"node_id"`
	OutboundTag  *string    `json:"outbound_tag,omitempty" db:"outbound_tag"`
	LastRotatedAt     *time.Time `json:"last_rotated_at,omitempty" db:"last_rotated_at"`
	LastHealthCheckAt *time.Time `json:"last_health_check_at,omitempty" db:"last_health_check_at"`
}

// Map 是 JSONB 字段的通用类型
type Map = map[string]interface{}

// DTO: OutboundPolicy

type CreatePolicyRequest struct {
	PolicyType   string `json:"policy_type" binding:"required,oneof=direct warp socks5 chain blackhole load_balance"`
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
	Endpoint   *string    `json:"endpoint,omitempty"`
	LicenseKey *string    `json:"license_key,omitempty"`
	ConfigJSON Map        `json:"config_json"`
	IsDefault  bool       `json:"is_default"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`

	// wireguard 原生字段
	PrivateKey   *string `json:"private_key,omitempty"`
	PublicKey    *string `json:"public_key,omitempty"`
	LocalAddress *string `json:"local_address,omitempty"`
	MTU          int     `json:"mtu"`

	// 账户注册元数据
	DeviceID     *string    `json:"device_id,omitempty"`
	IPv4Address  *string    `json:"ipv4_address,omitempty"`
	IPv6Address  *string    `json:"ipv6_address,omitempty"`
	Status       string     `json:"status"`
	NodeID       *uuid.UUID `json:"node_id,omitempty"`
	OutboundTag  *string    `json:"outbound_tag,omitempty"`
	LastRotatedAt     *string `json:"last_rotated_at,omitempty"`
	LastHealthCheckAt *string `json:"last_health_check_at,omitempty"`
}

func NewWarpProfileResponse(w *WarpProfile) WarpProfileResponse {
	resp := WarpProfileResponse{
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

		PrivateKey:   w.PrivateKey,
		PublicKey:    w.PublicKey,
		LocalAddress: w.LocalAddress,
		MTU:          w.MTU,

		DeviceID:     w.DeviceID,
		IPv4Address:  w.IPv4Address,
		IPv6Address:  w.IPv6Address,
		Status:       w.Status,
		NodeID:       w.NodeID,
		OutboundTag:  w.OutboundTag,
	}
	if w.LastRotatedAt != nil {
		resp.LastRotatedAt = ptrTime(w.LastRotatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if w.LastHealthCheckAt != nil {
		resp.LastHealthCheckAt = ptrTime(w.LastHealthCheckAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	return resp
}

func ptrTime(s string) *string { return &s }

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
