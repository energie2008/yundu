package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/google/uuid"
)

var (
	ErrPresetNotFound = errors.New("protocol preset not found")
	ErrPresetExists   = errors.New("protocol preset already exists")
	ErrBuiltinPreset  = errors.New("cannot modify built-in preset")
)

type PresetStore interface {
	Create(ctx context.Context, p *ProtocolPreset) error
	GetByID(ctx context.Context, id uuid.UUID) (*ProtocolPreset, error)
	GetByCode(ctx context.Context, code string) (*ProtocolPreset, error)
	Update(ctx context.Context, p *ProtocolPreset) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, page, pageSize int, query PresetListQuery) ([]*ProtocolPreset, int, error)
	ListEnabled(ctx context.Context) ([]*ProtocolPreset, error)
	ListAll(ctx context.Context) ([]*ProtocolPreset, error)
}

type PresetService struct {
	store      PresetStore
	logger     *slog.Logger
	builtin    []*ProtocolPreset
	builtinMap map[string]*ProtocolPreset
}

func NewPresetService(store PresetStore, logger *slog.Logger) *PresetService {
	svc := &PresetService{
		store:      store,
		logger:     logger,
		builtinMap: make(map[string]*ProtocolPreset),
	}
	svc.loadBuiltinPresets()
	return svc
}

func (s *PresetService) loadBuiltinPresets() {
	registry := nodespec.NewPresetRegistry()
	defaults := nodespec.BuildDefaultPresets()
	for _, p := range defaults {
		if err := registry.Register(p); err != nil {
			s.logger.Warn("failed to register builtin preset", "id", p.ID, "error", err)
			continue
		}
	}

	yamlDirs := []string{
		"packages/subscription/presets",
		"/app/presets",
		"./presets",
	}
	for _, dir := range yamlDirs {
		if err := registry.LoadFromDirectory(dir); err != nil {
			s.logger.Debug("failed to load presets from directory", "dir", dir, "error", err)
		}
	}

	for _, p := range registry.List() {
		baseSpecMap := presetBaseSpecToMap(p)
		var badge *string
		if p.Badge != "" {
			b := string(p.Badge)
			badge = &b
		}
		var minXray, minSingbox *string
		if p.MinXrayVersion != "" {
			v := p.MinXrayVersion
			minXray = &v
		}
		if p.MinSingboxVersion != "" {
			v := p.MinSingboxVersion
			minSingbox = &v
		}
		clientSupport := p.ClientSupport
		if clientSupport == nil {
			clientSupport = []string{}
		}
		recommendations := p.Recommendations
		if recommendations == nil {
			recommendations = []string{}
		}
		warnings := p.Warnings
		if warnings == nil {
			warnings = []string{}
		}
		compat := KernelCompatLevel(p.KernelCompat)
		if compat == "" {
			compat = CompatBoth
		}
		port := p.BaseSpec.Port
		if port == 0 {
			port = 443
		}
		isRecommended := p.Badge == nodespec.PresetBadgeRecommended
		sortOrder := 100
		switch p.Badge {
		case nodespec.PresetBadgeRecommended:
			sortOrder = 0
		case nodespec.PresetBadgeNew:
			sortOrder = 10
		case nodespec.PresetBadgeBalanced:
			sortOrder = 20
		case nodespec.PresetBadgeCDN:
			sortOrder = 30
		case nodespec.PresetBadgeExperimental:
			sortOrder = 90
		case nodespec.PresetBadgeDeprecated:
			sortOrder = 200
		}
		updatedTime := p.UpdatedFromUpstream
		bp := PresetFromBuiltin(
			p.ID, p.Name, badge, p.Description,
			string(p.Protocol), string(p.Transport), string(p.Security),
			minXray, minSingbox,
			clientSupport, compat,
			baseSpecMap, recommendations, warnings,
			port, sortOrder, isRecommended,
			&updatedTime,
		)
		s.builtin = append(s.builtin, bp)
		s.builtinMap[p.ID] = bp
	}
	s.logger.Info("loaded builtin protocol presets", "count", len(s.builtin))
}

func presetBaseSpecToMap(p *nodespec.PresetTemplate) Map {
	data, err := json.Marshal(p.BaseSpec)
	if err != nil {
		return Map{}
	}
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return Map{}
	}
	return m
}

func (s *PresetService) mergePresets(dbPresets []*ProtocolPreset) []*ProtocolPreset {
	result := make([]*ProtocolPreset, 0, len(s.builtin)+len(dbPresets))
	overridden := make(map[string]bool)
	for _, dbp := range dbPresets {
		overridden[dbp.Code] = true
		result = append(result, dbp)
	}
	for _, bp := range s.builtin {
		if !overridden[bp.Code] {
			result = append(result, bp)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (s *PresetService) getByCodeFromMerged(code string, merged []*ProtocolPreset) (*ProtocolPreset, bool) {
	for _, p := range merged {
		if p.Code == code {
			return p, true
		}
	}
	return nil, false
}

func (s *PresetService) getByIDFromMerged(id uuid.UUID, merged []*ProtocolPreset) (*ProtocolPreset, bool) {
	for _, p := range merged {
		if p.ID == id {
			return p, true
		}
	}
	return nil, false
}

func (s *PresetService) GetByID(ctx context.Context, id uuid.UUID) (*ProtocolPreset, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	for _, bp := range s.builtin {
		if bp.ID == id {
			return bp, nil
		}
	}
	return nil, ErrPresetNotFound
}

func (s *PresetService) GetByCode(ctx context.Context, code string) (*ProtocolPreset, error) {
	p, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	if bp, ok := s.builtinMap[code]; ok {
		return bp, nil
	}
	return nil, ErrPresetNotFound
}

func (s *PresetService) List(ctx context.Context, page, pageSize int, query PresetListQuery) ([]*ProtocolPreset, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	dbPresets, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, 0, err
	}

	merged := s.mergePresets(dbPresets)
	filtered := make([]*ProtocolPreset, 0, len(merged))
	for _, p := range merged {
		if query.ProtocolType != "" && p.ProtocolType != query.ProtocolType {
			continue
		}
		if query.TransportType != "" && p.TransportType != query.TransportType {
			continue
		}
		if query.SecurityType != "" && p.SecurityType != query.SecurityType {
			continue
		}
		if query.KernelCompat != "" && string(p.KernelCompat) != query.KernelCompat {
			continue
		}
		if query.IsEnabled != "" {
			want, _ := strconv.ParseBool(query.IsEnabled)
			if p.IsEnabled != want {
				continue
			}
		}
		if query.IsRecommended != "" {
			want, _ := strconv.ParseBool(query.IsRecommended)
			if p.IsRecommended != want {
				continue
			}
		}
		if query.IsBuiltin != "" {
			want, _ := strconv.ParseBool(query.IsBuiltin)
			if p.IsBuiltin != want {
				continue
			}
		}
		if p.DeprecatedAt != nil && p.DeprecatedAt.Before(time.Now()) {
			continue
		}
		filtered = append(filtered, p)
	}

	total := len(filtered)
	start := (page - 1) * pageSize
	if start >= total {
		return []*ProtocolPreset{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return filtered[start:end], total, nil
}

func (s *PresetService) ListEnabled(ctx context.Context) ([]*ProtocolPreset, error) {
	dbPresets, err := s.store.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	merged := s.mergePresets(dbPresets)
	result := make([]*ProtocolPreset, 0, len(merged))
	for _, p := range merged {
		if p.IsEnabled && (p.DeprecatedAt == nil || p.DeprecatedAt.After(time.Now())) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *PresetService) ListAll(ctx context.Context) ([]*ProtocolPreset, error) {
	dbPresets, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return s.mergePresets(dbPresets), nil
}

func (s *PresetService) Create(ctx context.Context, req *CreatePresetRequest) (*ProtocolPreset, error) {
	if bp, ok := s.builtinMap[req.Code]; ok {
		_ = bp
		return nil, ErrPresetExists
	}
	existing, err := s.store.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPresetExists
	}

	port := req.RecommendedPort
	if port == 0 {
		port = 443
	}
	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	defaultConfig := req.DefaultConfig
	if defaultConfig == nil {
		defaultConfig = Map{}
	}
	baseSpec := req.BaseSpec
	if baseSpec == nil {
		baseSpec = Map{}
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	var icon *string
	if req.Icon != "" {
		icon = &req.Icon
	}
	var badge *string
	if req.Badge != "" {
		b := req.Badge
		badge = &b
	}
	var minXray, minSingbox *string
	if req.MinXrayVersion != "" {
		v := req.MinXrayVersion
		minXray = &v
	}
	if req.MinSingboxVersion != "" {
		v := req.MinSingboxVersion
		minSingbox = &v
	}
	clientSupport := req.ClientSupport
	if clientSupport == nil {
		clientSupport = []string{}
	}
	compat := req.KernelCompat
	if compat == "" {
		compat = CompatBoth
	}
	recommendations := req.Recommendations
	if recommendations == nil {
		recommendations = []string{}
	}
	warnings := req.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	// B42 清理：移除空操作条件 if !strings.ContainsAny(code, "-_") {}

	p := &ProtocolPreset{
		ID:                uuid.New(),
		Code:              req.Code,
		Name:              req.Name,
		Badge:             badge,
		Description:       desc,
		ProtocolType:      req.ProtocolType,
		TransportType:     req.TransportType,
		SecurityType:      req.SecurityType,
		MinXrayVersion:    minXray,
		MinSingboxVersion: minSingbox,
		ClientSupport:     clientSupport,
		KernelCompat:      compat,
		BaseSpec:          baseSpec,
		DefaultConfig:     defaultConfig,
		Recommendations:   recommendations,
		Warnings:          warnings,
		RecommendedPort:   port,
		Icon:              icon,
		SortOrder:         req.SortOrder,
		IsRecommended:     req.IsRecommended,
		IsEnabled:         isEnabled,
		IsBuiltin:         false,
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ForkBuiltin 将内置预设复制为可编辑的自定义预设
// 解决内置预设不可编辑（ErrBuiltinPreset）的限制
func (s *PresetService) ForkBuiltin(ctx context.Context, id uuid.UUID, req *ForkPresetRequest) (*ProtocolPreset, error) {
	// 查找内置预设
	var src *ProtocolPreset
	for _, bp := range s.builtin {
		if bp.ID == id {
			src = bp
			break
		}
	}
	if src == nil {
		// 也允许 fork 已有自定义预设
		db, err := s.store.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if db == nil {
			return nil, ErrPresetNotFound
		}
		src = db
	}

	// 生成 code
	code := req.Code
	if code == "" {
		code = "fork-" + src.Code + "-" + strconv.FormatInt(time.Now().Unix(), 36)
	}
	// 检查 code 冲突
	if _, ok := s.builtinMap[code]; ok {
		return nil, ErrPresetExists
	}
	existing, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPresetExists
	}

	name := req.Name
	if name == "" {
		name = src.Name + " (副本)"
	}

	// 复制 BaseSpec / DefaultConfig
	baseSpec := Map{}
	if src.BaseSpec != nil {
		for k, v := range src.BaseSpec {
			baseSpec[k] = v
		}
	}
	defaultConfig := Map{}
	if src.DefaultConfig != nil {
		for k, v := range src.DefaultConfig {
			defaultConfig[k] = v
		}
	}

	clientSupport := src.ClientSupport
	if clientSupport == nil {
		clientSupport = []string{}
	}
	recommendations := src.Recommendations
	if recommendations == nil {
		recommendations = []string{}
	}
	warnings := src.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	p := &ProtocolPreset{
		ID:                uuid.New(),
		Code:              code,
		Name:              name,
		Badge:             src.Badge,
		Description:       src.Description,
		ProtocolType:      src.ProtocolType,
		TransportType:     src.TransportType,
		SecurityType:      src.SecurityType,
		MinXrayVersion:    src.MinXrayVersion,
		MinSingboxVersion: src.MinSingboxVersion,
		ClientSupport:     clientSupport,
		KernelCompat:      src.KernelCompat,
		BaseSpec:          baseSpec,
		DefaultConfig:     defaultConfig,
		Recommendations:   recommendations,
		Warnings:          warnings,
		RecommendedPort:   src.RecommendedPort,
		Icon:              src.Icon,
		SortOrder:         src.SortOrder + 100, // fork 的预设排在原预设之后
		IsRecommended:     false,
		IsEnabled:         true,
		IsBuiltin:         false,
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PresetService) Update(ctx context.Context, id uuid.UUID, req *UpdatePresetRequest) (*ProtocolPreset, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		for _, bp := range s.builtin {
			if bp.ID == id {
				return nil, ErrBuiltinPreset
			}
		}
		return nil, ErrPresetNotFound
	}
	if p.IsBuiltin {
		return nil, ErrBuiltinPreset
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Badge != nil {
		p.Badge = req.Badge
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.MinXrayVersion != nil {
		p.MinXrayVersion = req.MinXrayVersion
	}
	if req.MinSingboxVersion != nil {
		p.MinSingboxVersion = req.MinSingboxVersion
	}
	if req.ClientSupport != nil {
		p.ClientSupport = req.ClientSupport
	}
	if req.KernelCompat != nil {
		p.KernelCompat = *req.KernelCompat
	}
	if req.BaseSpec != nil {
		p.BaseSpec = req.BaseSpec
	}
	if req.DefaultConfig != nil {
		p.DefaultConfig = req.DefaultConfig
	}
	if req.Recommendations != nil {
		p.Recommendations = req.Recommendations
	}
	if req.Warnings != nil {
		p.Warnings = req.Warnings
	}
	if req.RecommendedPort != nil {
		p.RecommendedPort = *req.RecommendedPort
	}
	if req.Icon != nil {
		p.Icon = req.Icon
	}
	if req.SortOrder != nil {
		p.SortOrder = *req.SortOrder
	}
	if req.IsRecommended != nil {
		p.IsRecommended = *req.IsRecommended
	}
	if req.IsEnabled != nil {
		p.IsEnabled = *req.IsEnabled
	}
	if req.DeprecatedAt != nil {
		p.DeprecatedAt = req.DeprecatedAt
	}

	if err := s.store.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PresetService) Delete(ctx context.Context, id uuid.UUID) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPresetNotFound
	}
	if p.IsBuiltin {
		return ErrBuiltinPreset
	}
	return s.store.Delete(ctx, id)
}

func MapPresetErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrPresetNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPresetExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrBuiltinPreset):
		return config.CodeForbidden, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
