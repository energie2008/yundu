package lb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// UpsertLBPolicyRequest 是创建/更新节点组负载均衡策略的请求 DTO
type UpsertLBPolicyRequest struct {
	LBStrategy              string  `json:"lb_strategy" binding:"required,oneof=round_robin weighted least_conn latency sticky_user geo_affinity"`
	WeightField             string  `json:"weight_field"`
	GeoAffinity             bool    `json:"geo_affinity"`
	StickyBy                *string `json:"sticky_by"`
	MinScoreThreshold       int     `json:"min_score_threshold"`
	MaxNodesPerSubscription *int    `json:"max_nodes_per_subscription"`
	ExtraConfig             Map     `json:"extra_config"`
}

// LBService 封装 LBEngine + LBPolicyRepo，提供订阅生成时的负载均衡入口
// 以及节点组策略的读写管理。
type LBService struct {
	engine     *LBEngine
	policyRepo *LBPolicyRepo
	logger     *slog.Logger
}

func NewLBService(engine *LBEngine, policyRepo *LBPolicyRepo, logger *slog.Logger) *LBService {
	return &LBService{engine: engine, policyRepo: policyRepo, logger: logger}
}

// SelectForSubscription 为订阅生成执行负载均衡，返回排序、过滤、截断后的节点列表。
// 这是订阅生成流程（renderer）调用 LB 引擎的入口。
func (s *LBService) SelectForSubscription(ctx context.Context, groupID uuid.UUID, userID, userIP string) (*LBResult, error) {
	req := &LBRequest{
		GroupID: groupID,
		UserID:  userID,
		UserIP:  userIP,
	}
	res, err := s.engine.SelectNodes(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("lb select for subscription: %w", err)
	}
	return res, nil
}

// GetPolicy 读取节点组负载均衡策略；不存在返回 ErrPolicyNotFound
func (s *LBService) GetPolicy(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error) {
	p, err := s.policyRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get lb policy: %w", err)
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}
	return p, nil
}

// UpsertPolicy 创建或更新节点组负载均衡策略
func (s *LBService) UpsertPolicy(ctx context.Context, groupID uuid.UUID, req *UpsertLBPolicyRequest) (*LBPolicy, error) {
	weightField := req.WeightField
	if weightField == "" {
		weightField = "priority"
	}
	policy := &LBPolicy{
		GroupID:                 groupID,
		LBStrategy:              req.LBStrategy,
		WeightField:             weightField,
		GeoAffinity:             req.GeoAffinity,
		StickyBy:                req.StickyBy,
		MinScoreThreshold:       req.MinScoreThreshold,
		MaxNodesPerSubscription: req.MaxNodesPerSubscription,
		ExtraConfig:             req.ExtraConfig,
	}
	if err := s.policyRepo.Upsert(ctx, policy); err != nil {
		return nil, fmt.Errorf("upsert lb policy: %w", err)
	}
	return policy, nil
}
