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
	ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*WarpProfile, error)
	Delete(ctx context.Context, id uuid.UUID) error
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
	case "load_balance":
		// load_balance 聚合 warp outbound，渲染为 sing-box urltest，由 deployment_service 设置 route.final。
		// 允许 >=1 个 outbound：单 warp 时作为 route.final 引用（非真负载均衡但保证流量走 WARP）；
		// 多 warp 时实现自动选优 + 故障切换。
		if cfg == nil {
			return ErrInvalidPolicyConfig
		}
		outbounds, ok := cfg["outbounds"].([]interface{})
		if !ok || len(outbounds) < 1 {
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
	pool   WarpPoolInterface
}

// WarpRegisterOptions 控制注册行为的可选参数（对齐 3x-ui 一键注册体验）。
// 在 outbound 包定义以避免对 warpreg 包的循环依赖，由 app 层适配器转换为 warpreg.RegisterOptions。
//   - LicenseKey: WARP+ License（团队零信任 Key 或个人 WARP+ Key），非空时注册后自动应用
//   - Endpoint: 优选 Endpoint（如 162.159.193.1:2408），非空时覆盖默认 Endpoint
type WarpRegisterOptions struct {
	LicenseKey string
	Endpoint   string
}

// WarpPoolInterface 抽象 warpreg.Pool，避免循环依赖
type WarpPoolInterface interface {
	RegisterForNode(ctx context.Context, nodeID uuid.UUID, nodeCode string, opts *WarpRegisterOptions) (*WarpProfile, error)
	ImportExisting(ctx context.Context, nodeID uuid.UUID, nodeCode, privateKey, localAddress string) (*WarpProfile, error)
}

func NewWarpProfileService(store WarpProfileStore, logger *slog.Logger) *WarpProfileService {
	return &WarpProfileService{store: store, logger: logger}
}

// SetPool 注入 warpreg.Pool（可选，启用注册功能时需要）
func (s *WarpProfileService) SetPool(pool WarpPoolInterface) {
	s.pool = pool
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

// ListByNode 返回某节点的全部 WARP 档案
func (s *WarpProfileService) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*WarpProfile, error) {
	return s.store.ListByNode(ctx, nodeID)
}

// Delete 删除 WARP 档案
func (s *WarpProfileService) Delete(ctx context.Context, id uuid.UUID) error {
	w, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if w == nil {
		return ErrWarpProfileNotFound
	}
	return s.store.Delete(ctx, id)
}
