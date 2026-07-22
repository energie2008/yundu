package outbound

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// PolicyStore 抽象 OutboundPolicyRepo 的数据访问（便于测试注入 mock）
type PolicyStore interface {
	Create(ctx context.Context, p *OutboundPolicy) error
	GetByID(ctx context.Context, id uuid.UUID) (*OutboundPolicy, error)
	Update(ctx context.Context, p *OutboundPolicy) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundPolicy, error)
}

// WarpProfileStore 抽象 WarpProfileRepo 的数据访问
type WarpProfileStore interface {
	Create(ctx context.Context, w *WarpProfile) error
	GetByID(ctx context.Context, id uuid.UUID) (*WarpProfile, error)
	GetByCode(ctx context.Context, code string) (*WarpProfile, error)
	List(ctx context.Context) ([]*WarpProfile, error)
}

// OutboundService 封装出站策略的业务逻辑
type OutboundService struct {
	store  PolicyStore
	logger *slog.Logger
}

func NewOutboundService(store PolicyStore, logger *slog.Logger) *OutboundService {
	return &OutboundService{store: store, logger: logger}
}

// ListByNode 返回某节点的全部出站策略（按 priority 升序）
func (s *OutboundService) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundPolicy, error) {
	return s.store.ListByNode(ctx, nodeID)
}

// Create 为节点创建出站策略。config_json 校验：warp/socks5/chain 至少要有 server+port
func (s *OutboundService) Create(ctx context.Context, nodeID uuid.UUID, req *CreatePolicyRequest) (*OutboundPolicy, error) {
	if err := validatePolicyConfig(req.PolicyType, req.ConfigJSON); err != nil {
		return nil, err
	}

	priority := 100
	if req.Priority != nil {
		priority = *req.Priority
	}
	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	config := req.ConfigJSON
	if config == nil {
		config = Map{}
	}
	rules := req.RoutingRules
	if rules == nil {
		rules = []Map{}
	}

	p := &OutboundPolicy{
		NodeID:       nodeID,
		PolicyType:   req.PolicyType,
		Priority:     priority,
		ConfigJSON:   config,
		RoutingRules: rules,
		IsEnabled:    isEnabled,
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update 更新出站策略
func (s *OutboundService) Update(ctx context.Context, policyID uuid.UUID, req *UpdatePolicyRequest) (*OutboundPolicy, error) {
	p, err := s.store.GetByID(ctx, policyID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}

	policyType := p.PolicyType
	if req.PolicyType != nil {
		policyType = *req.PolicyType
	}
	if req.ConfigJSON != nil {
		if err := validatePolicyConfig(policyType, *req.ConfigJSON); err != nil {
			return nil, err
		}
	}

	if req.PolicyType != nil {
		p.PolicyType = *req.PolicyType
	}
	if req.Priority != nil {
		p.Priority = *req.Priority
	}
	if req.ConfigJSON != nil {
		p.ConfigJSON = *req.ConfigJSON
	}
	if req.RoutingRules != nil {
		p.RoutingRules = req.RoutingRules
	}
	if req.IsEnabled != nil {
		p.IsEnabled = *req.IsEnabled
	}

	if err := s.store.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete 删除出站策略
func (s *OutboundService) Delete(ctx context.Context, policyID uuid.UUID) error {
	p, err := s.store.GetByID(ctx, policyID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPolicyNotFound
	}
	return s.store.Delete(ctx, policyID)
}

// ApplyAll 渲染节点所有出站策略为 xray/sing-box 配置
func (s *OutboundService) ApplyAll(ctx context.Context, nodeID uuid.UUID) (*ApplyAllResponse, error) {
	policies, err := s.store.ListByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return RenderOutbounds(policies)
}

// validatePolicyConfig 校验不同 policy_type 的 config 必填字段
func validatePolicyConfig(policyType string, cfg Map) error {
	switch policyType {
	case "direct", "blackhole":
		return nil
	case "warp":
		// WARP 可选 server/port（默认本地），允许空
		return nil
	case "socks5", "chain":
		if cfg == nil {
			return ErrInvalidPolicyConfig
		}
		server, _ := cfg["server"].(string)
		port := toInt(cfg["port"])
		if server == "" || port <= 0 {
			return ErrInvalidPolicyConfig
		}
		return nil
	}
	return ErrInvalidPolicyConfig
}

// WarpProfileService 封装 WARP 档案的业务逻辑
type WarpProfileService struct {
	store  WarpProfileStore
	logger *slog.Logger
}

func NewWarpProfileService(store WarpProfileStore, logger *slog.Logger) *WarpProfileService {
	return &WarpProfileService{store: store, logger: logger}
}

func (s *WarpProfileService) List(ctx context.Context) ([]*WarpProfile, error) {
	return s.store.List(ctx)
}

func (s *WarpProfileService) GetByID(ctx context.Context, id uuid.UUID) (*WarpProfile, error) {
	w, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrWarpProfileNotFound
	}
	return w, nil
}

func (s *WarpProfileService) Create(ctx context.Context, req *CreateWarpProfileRequest) (*WarpProfile, error) {
	existing, err := s.store.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrWarpProfileExists
	}

	warpMode := req.WarpMode
	if warpMode == "" {
		warpMode = "warp"
	}
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}
	config := req.ConfigJSON
	if config == nil {
		config = Map{}
	}

	var endpoint, licenseKey *string
	if req.Endpoint != "" {
		endpoint = &req.Endpoint
	}
	if req.LicenseKey != "" {
		licenseKey = &req.LicenseKey
	}

	w := &WarpProfile{
		Code:       req.Code,
		Name:       req.Name,
		WarpMode:   warpMode,
		Endpoint:   endpoint,
		LicenseKey: licenseKey,
		ConfigJSON: config,
		IsDefault:  isDefault,
	}

	if err := s.store.Create(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}
