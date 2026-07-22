package compat

import (
	"time"

	"github.com/google/uuid"
)

// ClientProfile 客户端档案领域模型（对应 client_profiles 表）
type ClientProfile struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Platform    string
	MinVersion  *string
	MaxVersion  *string
	Status      string
	Notes       *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CompatMatrixEntry 兼容矩阵条目（对应 client_compat_matrix 表）
type CompatMatrixEntry struct {
	ID                   uuid.UUID
	ClientCode           string
	FeatureCode          string
	Supported            bool
	SupportedSinceVersion *string
	Notes                *string
	CreatedAt            time.Time
}

// AdvancedPatchProfile 高级补丁档案（对应 advanced_patch_profiles 表）
type AdvancedPatchProfile struct {
	ID                  uuid.UUID
	NodeID              uuid.UUID
	RuntimeType         string
	PatchJSON           []byte
	PatchTarget         string
	AllowedKeys         []byte
	SchemaVersion       string
	IsEnabled           bool
	LastValidatedAt     *time.Time
	LastValidationResult []byte
	Notes               *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ===== DTO =====

// ClientProfileResponse 客户端档案响应
type ClientProfileResponse struct {
	ID         uuid.UUID `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	MinVersion string    `json:"min_version,omitempty"`
	MaxVersion string    `json:"max_version,omitempty"`
	Status     string    `json:"status"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

func NewClientProfileResponse(p *ClientProfile) ClientProfileResponse {
	resp := ClientProfileResponse{
		ID:        p.ID,
		Code:      p.Code,
		Name:      p.Name,
		Platform:  p.Platform,
		Status:    p.Status,
		CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.MinVersion != nil {
		resp.MinVersion = *p.MinVersion
	}
	if p.MaxVersion != nil {
		resp.MaxVersion = *p.MaxVersion
	}
	if p.Notes != nil {
		resp.Notes = *p.Notes
	}
	return resp
}

// CompatMatrixEntryResponse 兼容矩阵条目响应
type CompatMatrixEntryResponse struct {
	ID                    uuid.UUID `json:"id"`
	ClientCode            string    `json:"client_code"`
	FeatureCode           string    `json:"feature_code"`
	Supported             bool      `json:"supported"`
	SupportedSinceVersion string    `json:"supported_since_version,omitempty"`
	Notes                 string    `json:"notes,omitempty"`
	CreatedAt             string    `json:"created_at"`
}

func NewCompatMatrixEntryResponse(e *CompatMatrixEntry) CompatMatrixEntryResponse {
	resp := CompatMatrixEntryResponse{
		ID:          e.ID,
		ClientCode:  e.ClientCode,
		FeatureCode: e.FeatureCode,
		Supported:   e.Supported,
		CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if e.SupportedSinceVersion != nil {
		resp.SupportedSinceVersion = *e.SupportedSinceVersion
	}
	if e.Notes != nil {
		resp.Notes = *e.Notes
	}
	return resp
}

// ClientProfileListQuery 客户端档案列表查询参数
type ClientProfileListQuery struct {
	Page     int
	PageSize int
	Status   string
	Code     string
}

// CompatMatrixListQuery 兼容矩阵列表查询参数
type CompatMatrixListQuery struct {
	Page       int
	PageSize   int
	ClientCode string
	FeatureCode string
}

// CompatMatrixUpdateEntry 批量更新条目
type CompatMatrixUpdateEntry struct {
	ClientCode           string  `json:"client_code" binding:"required"`
	FeatureCode          string  `json:"feature_code" binding:"required"`
	Supported            bool    `json:"supported"`
	SupportedSinceVersion *string `json:"supported_since_version"`
	Notes                *string `json:"notes"`
}

// CompatMatrixBatchUpdateRequest 批量更新请求
type CompatMatrixBatchUpdateRequest struct {
	Entries []CompatMatrixUpdateEntry `json:"entries" binding:"required,min=1"`
}

// CompatMatrixBatchUpdateResponse 批量更新响应
type CompatMatrixBatchUpdateResponse struct {
	Updated int `json:"updated"`
}

// CompatSyncResponse 同步响应
type CompatSyncResponse struct {
	Synced  int    `json:"synced"`
	Message string `json:"message"`
}

// AdvancedPatchResponse 高级补丁响应
type AdvancedPatchResponse struct {
	ID            uuid.UUID `json:"id"`
	NodeID        uuid.UUID `json:"node_id"`
	RuntimeType   string    `json:"runtime_type"`
	PatchTarget   string    `json:"patch_target"`
	IsEnabled     bool      `json:"is_enabled"`
	SchemaVersion string    `json:"schema_version"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

func NewAdvancedPatchResponse(p *AdvancedPatchProfile) AdvancedPatchResponse {
	return AdvancedPatchResponse{
		ID:            p.ID,
		NodeID:        p.NodeID,
		RuntimeType:   p.RuntimeType,
		PatchTarget:   p.PatchTarget,
		IsEnabled:     p.IsEnabled,
		SchemaVersion: p.SchemaVersion,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// FeatureCode 常量定义（与种子数据保持一致）
const (
	FeatureUTLS              = "utls"
	FeatureReality           = "reality"
	FeatureVLSSEncryption    = "vless_encryption"
	FeatureECH               = "ech"
	FeatureXHTTP             = "xhttp"
	FeatureWS                = "ws"
	FeatureGRPC              = "grpc"
	FeatureHysteria2         = "hysteria2"
	FeatureTUICv5            = "tuic_v5"
)
