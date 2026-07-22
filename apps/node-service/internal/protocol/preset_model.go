package protocol

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type KernelCompatLevel string

const (
	CompatBoth         KernelCompatLevel = "both"
	CompatXrayOnly     KernelCompatLevel = "xray_only"
	CompatSingboxOnly  KernelCompatLevel = "singbox_only"
	CompatExperimental KernelCompatLevel = "experimental"
)

type ProtocolPreset struct {
	ID                  uuid.UUID         `json:"id"`
	Code                string            `json:"code"`
	Name                string            `json:"name"`
	Badge               *string           `json:"badge,omitempty"`
	Description         *string           `json:"description,omitempty"`
	ProtocolType        string            `json:"protocol_type"`
	TransportType       string            `json:"transport_type"`
	SecurityType        string            `json:"security_type"`
	MinXrayVersion      *string           `json:"min_xray_version,omitempty"`
	MinSingboxVersion   *string           `json:"min_singbox_version,omitempty"`
	ClientSupport       []string          `json:"client_support"`
	KernelCompat        KernelCompatLevel `json:"kernel_compat"`
	BaseSpec            Map               `json:"base_spec"`
	DefaultConfig       Map               `json:"default_config,omitempty"`
	Recommendations     []string          `json:"recommendations,omitempty"`
	Warnings            []string          `json:"warnings,omitempty"`
	RecommendedPort     int               `json:"recommended_port"`
	Icon                *string           `json:"icon,omitempty"`
	SortOrder           int               `json:"sort_order"`
	IsRecommended       bool              `json:"is_recommended"`
	IsEnabled           bool              `json:"is_enabled"`
	IsBuiltin           bool              `json:"is_builtin"`
	UpdatedFromUpstream *time.Time        `json:"updated_from_upstream,omitempty"`
	DeprecatedAt        *time.Time        `json:"deprecated_at,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

type CreatePresetRequest struct {
	Code                string            `json:"code" binding:"required,min=2,max=64"`
	Name                string            `json:"name" binding:"required,min=1,max=128"`
	Badge               string            `json:"badge"`
	Description         string            `json:"description"`
	ProtocolType        string            `json:"protocol_type" binding:"required,min=1,max=32"`
	TransportType       string            `json:"transport_type" binding:"required,min=1,max=32"`
	SecurityType        string            `json:"security_type" binding:"required,min=1,max=32"`
	MinXrayVersion      string            `json:"min_xray_version"`
	MinSingboxVersion   string            `json:"min_singbox_version"`
	ClientSupport       []string          `json:"client_support"`
	KernelCompat        KernelCompatLevel `json:"kernel_compat"`
	BaseSpec            Map               `json:"base_spec"`
	DefaultConfig       Map               `json:"default_config"`
	Recommendations     []string          `json:"recommendations"`
	Warnings            []string          `json:"warnings"`
	RecommendedPort     int               `json:"recommended_port"`
	Icon                string            `json:"icon"`
	SortOrder           int               `json:"sort_order"`
	IsRecommended       bool              `json:"is_recommended"`
	IsEnabled           *bool             `json:"is_enabled"`
}

// ForkPresetRequest 用于 fork 内置预设为自定义预设
type ForkPresetRequest struct {
	// Name 自定义预设名称，空则使用 "内置预设名 (副本)"
	Name string `json:"name"`
	// Code 自定义预设 code，空则自动生成 "fork-{原code}-{timestamp}"
	Code string `json:"code"`
}

type UpdatePresetRequest struct {
	Name                *string            `json:"name"`
	Badge               *string            `json:"badge"`
	Description         *string            `json:"description"`
	MinXrayVersion      *string            `json:"min_xray_version"`
	MinSingboxVersion   *string            `json:"min_singbox_version"`
	ClientSupport       []string           `json:"client_support"`
	KernelCompat        *KernelCompatLevel `json:"kernel_compat"`
	BaseSpec            Map                `json:"base_spec"`
	DefaultConfig       Map                `json:"default_config"`
	Recommendations     []string           `json:"recommendations"`
	Warnings            []string           `json:"warnings"`
	RecommendedPort     *int               `json:"recommended_port"`
	Icon                *string            `json:"icon"`
	SortOrder           *int               `json:"sort_order"`
	IsRecommended       *bool              `json:"is_recommended"`
	IsEnabled           *bool              `json:"is_enabled"`
	DeprecatedAt        *time.Time         `json:"deprecated_at"`
}

type PresetListQuery struct {
	Page          int    `form:"page"`
	PageSize      int    `form:"page_size"`
	ProtocolType  string `form:"protocol_type"`
	TransportType string `form:"transport_type"`
	SecurityType  string `form:"security_type"`
	KernelCompat  string `form:"kernel_compat"`
	IsEnabled     string `form:"is_enabled"`
	IsRecommended string `form:"is_recommended"`
	IsBuiltin     string `form:"is_builtin"`
}

type PresetResponse struct {
	ID                  string            `json:"id"`
	Code                string            `json:"code"`
	Name                string            `json:"name"`
	Badge               *string           `json:"badge,omitempty"`
	Description         *string           `json:"description,omitempty"`
	Protocol            string            `json:"protocol"`
	ProtocolType        string            `json:"protocol_type"`
	Transport           string            `json:"transport"`
	TransportType       string            `json:"transport_type"`
	Security            string            `json:"security"`
	SecurityType        string            `json:"security_type"`
	MinXrayVersion      *string           `json:"min_xray_version,omitempty"`
	MinSingboxVersion   *string           `json:"min_singbox_version,omitempty"`
	ClientSupport       []string          `json:"client_support"`
	KernelCompat        KernelCompatLevel `json:"kernel_compat"`
	BaseSpec            Map               `json:"base_spec"`
	DefaultConfig       Map               `json:"default_config,omitempty"`
	Recommendations     []string          `json:"recommendations,omitempty"`
	Warnings            []string          `json:"warnings,omitempty"`
	RecommendedPort     int               `json:"recommended_port"`
	Icon                *string           `json:"icon,omitempty"`
	SortOrder           int               `json:"sort_order"`
	IsRecommended       bool              `json:"is_recommended"`
	IsEnabled           bool              `json:"is_enabled"`
	IsBuiltin           bool              `json:"is_builtin"`
	UpdatedFromUpstream *string           `json:"updated_from_upstream,omitempty"`
	DeprecatedAt        *string           `json:"deprecated_at,omitempty"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

func NewPresetResponse(p *ProtocolPreset) PresetResponse {
	resp := PresetResponse{
		ID:                p.ID.String(),
		Code:              p.Code,
		Name:              p.Name,
		Badge:             p.Badge,
		Description:       p.Description,
		Protocol:          p.ProtocolType,
		ProtocolType:      p.ProtocolType,
		Transport:         p.TransportType,
		TransportType:     p.TransportType,
		Security:          p.SecurityType,
		SecurityType:      p.SecurityType,
		MinXrayVersion:    p.MinXrayVersion,
		MinSingboxVersion: p.MinSingboxVersion,
		ClientSupport:     p.ClientSupport,
		KernelCompat:      p.KernelCompat,
		BaseSpec:          p.BaseSpec,
		DefaultConfig:     p.DefaultConfig,
		Recommendations:   p.Recommendations,
		Warnings:          p.Warnings,
		RecommendedPort:   p.RecommendedPort,
		Icon:              p.Icon,
		SortOrder:         p.SortOrder,
		IsRecommended:     p.IsRecommended,
		IsEnabled:         p.IsEnabled,
		IsBuiltin:         p.IsBuiltin,
		CreatedAt:         p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:         p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.UpdatedFromUpstream != nil {
		s := p.UpdatedFromUpstream.Format("2006-01-02T15:04:05Z07:00")
		resp.UpdatedFromUpstream = &s
	}
	if p.DeprecatedAt != nil {
		s := p.DeprecatedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.DeprecatedAt = &s
	}
	if resp.ClientSupport == nil {
		resp.ClientSupport = []string{}
	}
	if resp.BaseSpec == nil {
		resp.BaseSpec = Map{}
	}
	if resp.KernelCompat == "" {
		resp.KernelCompat = CompatBoth
	}
	return resp
}

func PresetFromBuiltin(id string, name string, badge *string, desc string,
	proto, transport, security string,
	minXray, minSingbox *string,
	clientSupport []string, compat KernelCompatLevel,
	baseSpec Map, recommendations, warnings []string,
	port int, sortOrder int, isRecommended bool,
	updatedFromUpstream *time.Time) *ProtocolPreset {
	now := time.Now().UTC()
	return &ProtocolPreset{
		ID:                  uuid.NewSHA1(uuid.NameSpaceOID, []byte("builtin-preset-"+id)),
		Code:                id,
		Name:                name,
		Badge:               badge,
		Description:         &desc,
		ProtocolType:        proto,
		TransportType:       transport,
		SecurityType:        security,
		MinXrayVersion:      minXray,
		MinSingboxVersion:   minSingbox,
		ClientSupport:       clientSupport,
		KernelCompat:        compat,
		BaseSpec:            baseSpec,
		DefaultConfig:       Map{},
		Recommendations:     recommendations,
		Warnings:            warnings,
		RecommendedPort:     port,
		SortOrder:           sortOrder,
		IsRecommended:       isRecommended,
		IsEnabled:           true,
		IsBuiltin:           true,
		UpdatedFromUpstream: updatedFromUpstream,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func (p *ProtocolPreset) ToMap() Map {
	data, _ := json.Marshal(p)
	var m Map
	json.Unmarshal(data, &m)
	return m
}
