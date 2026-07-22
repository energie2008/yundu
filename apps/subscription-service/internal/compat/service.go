package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// AuditWriter 审计写入接口（预留，本批传 nil）
type AuditWriter interface {
	Write(ctx context.Context, action string, resourceType string, resourceID string, payload interface{})
}

// profileRepoI 客户端档案 repo 接口（便于测试注入 fake）
type profileRepoI interface {
	GetByCode(ctx context.Context, code string) (*ClientProfile, error)
	ListAll(ctx context.Context, page, pageSize int, status, code string) ([]*ClientProfile, int, error)
}

// matrixRepoI 兼容矩阵 repo 接口
type matrixRepoI interface {
	GetByClientFeature(ctx context.Context, clientCode, featureCode string) (*CompatMatrixEntry, error)
	ListByClientCode(ctx context.Context, clientCode string) ([]*CompatMatrixEntry, error)
	ListAll(ctx context.Context, page, pageSize int, clientCode, featureCode string) ([]*CompatMatrixEntry, int, error)
	Upsert(ctx context.Context, entry *CompatMatrixEntry) error
}

// patchRepoI 高级补丁 repo 接口
type patchRepoI interface {
	GetByNodeID(ctx context.Context, nodeID uuid.UUID) ([]*AdvancedPatchProfile, error)
}

// CompatService 兼容服务：处理客户端功能矩阵 / 节点兼容过滤 / 渲染降级
type CompatService struct {
	profileRepo profileRepoI
	matrixRepo  matrixRepoI
	patchRepo   patchRepoI
	audit       AuditWriter
}

func NewCompatService(profileRepo *ClientProfileRepo, matrixRepo *CompatMatrixRepo, patchRepo *AdvancedPatchRepo) *CompatService {
	return &CompatService{
		profileRepo: profileRepo,
		matrixRepo:  matrixRepo,
		patchRepo:   patchRepo,
	}
}

// SetAuditWriter 注入审计写入器（可选）
func (s *CompatService) SetAuditWriter(a AuditWriter) {
	s.audit = a
}

// ===== 客户端档案 =====

// ListClientProfiles 列出客户端档案
func (s *CompatService) ListClientProfiles(ctx context.Context, query ClientProfileListQuery) ([]*ClientProfile, int, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	return s.profileRepo.ListAll(ctx, query.Page, query.PageSize, query.Status, query.Code)
}

// ===== 兼容矩阵 =====

// ListCompatMatrix 列出兼容矩阵
func (s *CompatService) ListCompatMatrix(ctx context.Context, query CompatMatrixListQuery) ([]*CompatMatrixEntry, int, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	}
	return s.matrixRepo.ListAll(ctx, query.Page, query.PageSize, query.ClientCode, query.FeatureCode)
}

// BatchUpdateMatrix 批量 upsert 兼容矩阵
func (s *CompatService) BatchUpdateMatrix(ctx context.Context, req *CompatMatrixBatchUpdateRequest) (int, error) {
	if req == nil || len(req.Entries) == 0 {
		return 0, ErrMatrixEntryInvalid
	}
	updated := 0
	for _, e := range req.Entries {
		if e.ClientCode == "" || e.FeatureCode == "" {
			return updated, ErrMatrixEntryInvalid
		}
		entry := &CompatMatrixEntry{
			ClientCode:            e.ClientCode,
			FeatureCode:           e.FeatureCode,
			Supported:             e.Supported,
			SupportedSinceVersion: e.SupportedSinceVersion,
			Notes:                 e.Notes,
		}
		if err := s.matrixRepo.Upsert(ctx, entry); err != nil {
			return updated, err
		}
		updated++
	}
	if s.audit != nil {
		s.audit.Write(ctx, "batch_update", "client_compat_matrix", "", req)
	}
	return updated, nil
}

// SyncFromSource 从维护库同步矩阵（本批不真同步，留 TODO）
func (s *CompatService) SyncFromSource(ctx context.Context) (*CompatSyncResponse, error) {
	// TODO(task-32): 当 COMPAT_SOURCE_URL 配置后，从远端拉取最新矩阵并 upsert
	// 目前无维护库配置时返回 synced=0
	return &CompatSyncResponse{
		Synced:  0,
		Message: "维护库未配置，请设置 COMPAT_SOURCE_URL",
	}, nil
}

// ===== 客户端能力查询 =====

// GetClientFeatures 返回某客户端某版本支持的功能 code 列表
// 版本比较：若客户端版本 < supported_since_version 则视为不支持
func (s *CompatService) GetClientFeatures(ctx context.Context, clientCode, version string) ([]string, error) {
	if clientCode == "" {
		return nil, ErrClientNotFound
	}
	profile, err := s.profileRepo.GetByCode(ctx, clientCode)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrClientNotFound
	}

	entries, err := s.matrixRepo.ListByClientCode(ctx, clientCode)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		return []string{}, nil
	}

	features := make([]string, 0, len(entries))
	for _, e := range entries {
		if isEntrySupported(e, version) {
			features = append(features, e.FeatureCode)
		}
	}
	return features, nil
}

// isEntrySupported 判断单个条目在某版本下是否支持
// 规则：supported=false → 不支持；supported=true 且 supported_since_version 为空 → 支持；
//       supported=true 且 supported_since_version 不为空 → 当 version >= supported_since_version 时支持
func isEntrySupported(e *CompatMatrixEntry, version string) bool {
	if !e.Supported {
		return false
	}
	if e.SupportedSinceVersion == nil || *e.SupportedSinceVersion == "" {
		return true
	}
	if version == "" {
		// 客户端未提供版本时，按保守策略：不支持需要特定版本的功能
		return false
	}
	return compareVersions(version, *e.SupportedSinceVersion) >= 0
}

// FilterNodeForClient 判断节点是否兼容某客户端
// node 参数为 map[string]interface{}，含 protocol_type/transport_type/security_type/encryption/ech 等字段
func (s *CompatService) FilterNodeForClient(ctx context.Context, node map[string]interface{}, clientCode, version string) (bool, string, error) {
	if node == nil {
		return false, "node is nil", ErrMatrixEntryInvalid
	}
	if clientCode == "" {
		return false, "client_code required", ErrClientNotFound
	}

	profile, err := s.profileRepo.GetByCode(ctx, clientCode)
	if err != nil {
		return false, "", err
	}
	if profile == nil {
		return false, "", ErrClientNotFound
	}

	// 节点状态检查：若节点 status 字段为 hidden 直接不可见
	if status, ok := node["status"].(string); ok && status == "hidden" {
		return false, "node is hidden", nil
	}

	securityType, _ := node["security_type"].(string)
	transportType, _ := node["transport_type"].(string)
	encryption, _ := node["encryption"].(string)
	hasECH := false
	if ech, ok := node["ech"].(map[string]interface{}); ok && ech != nil {
		hasECH = true
	} else if echBool, ok := node["ech"].(bool); ok && echBool {
		hasECH = true
	}

	// 1. REALITY 不支持
	if strings.EqualFold(securityType, "reality") {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureReality) {
			return false, "REALITY not supported by this client", nil
		}
	}
	// 2. VLESS encryption 不支持
	if encryption != "" && encryption != "none" {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureVLSSEncryption) {
			return false, "VLESS encryption not supported by this client", nil
		}
	}
	// 3. ECH 不支持
	if hasECH {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureECH) {
			return false, "ECH not supported by this client", nil
		}
	}
	// 4. XHTTP 不支持
	if strings.EqualFold(transportType, "xhttp") {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureXHTTP) {
			return false, "XHTTP transport not supported by this client", nil
		}
	}
	// 5. WS 检查
	if strings.EqualFold(transportType, "ws") {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureWS) {
			return false, "WS transport not supported by this client", nil
		}
	}

	return true, "", nil
}

// isFeatureSupported 查询某功能在某客户端版本下是否支持（带缓存意识）
func (s *CompatService) isFeatureSupported(ctx context.Context, clientCode, version, featureCode string) bool {
	entry, err := s.matrixRepo.GetByClientFeature(ctx, clientCode, featureCode)
	if err != nil || entry == nil {
		// 未配置的功能默认视为支持（避免过度降级）
		return true
	}
	return isEntrySupported(entry, version)
}

// RenderWithCompat 按兼容性自动降级
// 降级规则（优先级从高到低）：
//   1. VLESS Encryption：客户端不支持 → 移除 encryption 字段，回退 none
//   2. ECH：不支持 → 移除 ech 字段，保留 TLS 其余配置
//   3. XHTTP：不支持 → 换 WS 传输（若节点配置了 fallback transport；否则保留并加 warning）
//   4. REALITY：不支持 → 节点标记为 hidden（返回时加 _hidden: true 字段）
// 返回处理后的 node map（深拷贝输入，不修改原 map）
func (s *CompatService) RenderWithCompat(ctx context.Context, node map[string]interface{}, clientCode, version string) (map[string]interface{}, []string, error) {
	if node == nil {
		return nil, nil, ErrMatrixEntryInvalid
	}

	out, err := deepCopyMap(node)
	if err != nil {
		return nil, nil, err
	}

	var warnings []string

	securityType, _ := out["security_type"].(string)
	transportType, _ := out["transport_type"].(string)
	encryption, _ := out["encryption"].(string)

	// 1. VLESS Encryption 降级
	if encryption != "" && encryption != "none" {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureVLSSEncryption) {
			delete(out, "encryption")
			out["encryption"] = "none"
			warnings = append(warnings, "removed vless encryption (not supported), fallback to none")
		}
	}

	// 2. ECH 降级
	if _, ok := out["ech"]; ok {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureECH) {
			delete(out, "ech")
			warnings = append(warnings, "removed ech config (not supported)")
		}
	}

	// 3. XHTTP 降级到 WS（如果节点配置了 fallback transport）
	if strings.EqualFold(transportType, "xhttp") {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureXHTTP) {
			if fallback, ok := out["fallback_transport"].(string); ok && fallback != "" {
				out["transport_type"] = fallback
				delete(out, "fallback_transport")
				// 同步 path 字段：XHTTP 通常用 path，WS 也用 path，无需改
				warnings = append(warnings, fmt.Sprintf("transport changed from xhttp to %s (fallback)", fallback))
			} else {
				// 没配置 fallback transport：保留并加 warning
				warnings = append(warnings, "xhttp transport not supported by client, kept as-is (no fallback configured)")
			}
		}
	}

	// 4. REALITY 不支持 → 标记 hidden（不直接移除，让调用方决定是否过滤）
	if strings.EqualFold(securityType, "reality") {
		if !s.isFeatureSupported(ctx, clientCode, version, FeatureReality) {
			out["_hidden"] = true
			out["_hidden_reason"] = "REALITY not supported by this client"
			warnings = append(warnings, "marked node as hidden (REALITY not supported)")
		}
	}

	return out, warnings, nil
}

// deepCopyMap 通过 JSON 序列化深拷贝 map
func deepCopyMap(m map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal map: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal map: %w", err)
	}
	return out, nil
}

// compareVersions 简单语义化版本比较（按点分割），返回 -1/0/1
// 不依赖外部库；版本格式如 "1.18.0" / "6.5.0"
func compareVersions(v1, v2 string) int {
	a := parseVersion(v1)
	b := parseVersion(v2)
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		var na, nb int
		if i < len(a) {
			na = a[i]
		}
		if i < len(b) {
			nb = b[i]
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

// parseVersion 解析版本字符串为整数切片；非法字符截断
func parseVersion(v string) []int {
	v = strings.TrimSpace(v)
	if v == "" {
		return []int{0}
	}
	// 移除可能的 v 前缀
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		v = v[1:]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		// 截取数字部分
		numStr := ""
		for _, r := range p {
			if r >= '0' && r <= '9' {
				numStr += string(r)
			} else {
				break
			}
		}
		if numStr == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(numStr)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return out
}

// ===== AdvancedPatch =====

// ListAdvancedPatches 列出某节点的高级补丁档案（仅做最简返回，本批未在路由暴露）
func (s *CompatService) ListAdvancedPatches(ctx context.Context, nodeID uuid.UUID) ([]*AdvancedPatchProfile, error) {
	return s.patchRepo.GetByNodeID(ctx, nodeID)
}

// ToAdvancedPatchResponseList 转换为响应 DTO 列表（避免直接返回模型）
func ToAdvancedPatchResponseList(patches []*AdvancedPatchProfile) []AdvancedPatchResponse {
	out := make([]AdvancedPatchResponse, 0, len(patches))
	for _, p := range patches {
		out = append(out, NewAdvancedPatchResponse(p))
	}
	return out
}

// 兼容 model.ClientType 的 helper（避免直接引用模型包，仅做客户端类型字符串校验）
var supportedClientTypes = map[string]bool{
	"clash-meta":   true,
	"sing-box":     true,
	"v2rayn":       true,
	"v2rayng":      true,
	"shadowrocket": true,
	"stash":        true,
	"hiddify":      true,
	"nekobox":      true,
	"loon":         true,
}

// IsKnownClientCode 判断客户端 code 是否在已知列表
func IsKnownClientCode(code string) bool {
	return supportedClientTypes[code]
}
