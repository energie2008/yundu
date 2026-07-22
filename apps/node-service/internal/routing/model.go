package routing

import (
	"time"

	"github.com/google/uuid"
)

// Map 是 JSONB 字段的通用类型（map[string]interface{} 的别名）
type Map = map[string]interface{}

// 领域模型 (对应迁移 000017_lb_routing.sql)

// RouteRuleSet 对应 route_rule_sets 表，可复用的路由规则集合
type RouteRuleSet struct {
	ID           uuid.UUID  `json:"id"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	RuleType     string     `json:"rule_type"`     // builtin / custom
	SourceType   string     `json:"source_type"`   // inline / geosite / geoip / remote_url
	SourceURL    *string    `json:"source_url,omitempty"`
	Content      []string   `json:"content"`       // 规则条目数组，如 ["geoip:cn","geosite:cn"]
	AutoUpdate   bool       `json:"auto_update"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RoutePolicy 对应 route_policies 表，把"哪些流量"映射到"哪个出站"
type RoutePolicy struct {
	ID               uuid.UUID `json:"id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	PolicyType       string    `json:"policy_type"` // builtin_template / custom
	BaseTemplateCode *string   `json:"base_template_code,omitempty"`
	Status           string    `json:"status"`
	Rules            []RoutePolicyRule `json:"rules,omitempty"` // 详情接口填充
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RoutePolicyRule 对应 route_policy_rules 表，策略内的有序规则条目
type RoutePolicyRule struct {
	ID             uuid.UUID  `json:"id"`
	PolicyID       uuid.UUID  `json:"policy_id"`
	SortOrder      int        `json:"sort_order"`
	RuleSource     string     `json:"rule_source"`     // rule_set / inline
	RuleSetID      *uuid.UUID `json:"rule_set_id,omitempty"`
	InlineType     *string    `json:"inline_type,omitempty"`     // domain/domain_suffix/domain_keyword/geosite/geoip/ip_cidr/port/protocol
	InlineValues   []string   `json:"inline_values"`             // inline 规则的值数组
	OutboundAction string     `json:"outbound_action"`           // proxy/direct/blackhole/warp/tag/balancer
	OutboundTag    *string    `json:"outbound_tag,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// NodeRouteBinding 对应 node_route_bindings 表，节点绑定路由策略
type NodeRouteBinding struct {
	NodeID     uuid.UUID `json:"node_id"`
	PolicyID   uuid.UUID `json:"policy_id"`
	BindScope  string    `json:"bind_scope"`  // all / inbound_tag
	InboundTag *string   `json:"inbound_tag,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// NodeGroupLBPolicy 对应 node_group_lb_policies 表，节点组负载均衡策略
type NodeGroupLBPolicy struct {
	ID                      uuid.UUID `json:"id"`
	GroupID                 uuid.UUID `json:"group_id"`
	LBStrategy              string    `json:"lb_strategy"` // round_robin/weighted/least_conn/latency/random/sticky_user/geo_affinity
	WeightField             string    `json:"weight_field"`
	GeoAffinity             bool      `json:"geo_affinity"`
	StickyBy                *string   `json:"sticky_by,omitempty"` // user_id / subscription_token / null
	MinScoreThreshold       int      `json:"min_score_threshold"`
	MaxNodesPerSubscription *int     `json:"max_nodes_per_subscription,omitempty"`
	ExtraConfig             Map      `json:"extra_config"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// OutboundGroup 对应 outbound_groups 表，出站均衡器（对应 xray balancer）
type OutboundGroup struct {
	ID                  uuid.UUID `json:"id"`
	NodeID              uuid.UUID `json:"node_id"`
	Tag                 string    `json:"tag"`
	LBStrategy          string    `json:"lb_strategy"` // leastPing / random / leastLoad
	ProbeURL            string    `json:"probe_url"`
	ProbeIntervalSeconds int      `json:"probe_interval_seconds"`
	Members             []Map    `json:"members"` // [{"tag": "hk-isp1", "weight": 1}]
	Status              string    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// OutboundGroupMember 是 outbound_groups.members 数组中的成员
type OutboundGroupMember struct {
	Tag    string `json:"tag"`
	Weight int    `json:"weight"`
}

// ===================== DTO: RouteRuleSet =====================

type CreateRuleSetRequest struct {
	Code        string   `json:"code" binding:"required,min=1,max=64"`
	Name        string   `json:"name" binding:"required,min=1,max=128"`
	Description string   `json:"description"`
	RuleType    string   `json:"rule_type" binding:"required,oneof=builtin custom"`
	SourceType  string   `json:"source_type" binding:"required,oneof=inline geosite geoip remote_url"`
	SourceURL   string   `json:"source_url"`
	Content     []string `json:"content"`
	AutoUpdate  *bool    `json:"auto_update"`
}

type UpdateRuleSetRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Content     []string `json:"content"`
	AutoUpdate  *bool    `json:"auto_update"`
	Status      *string  `json:"status"`
}

type RuleSetListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	RuleType string `form:"rule_type"`
	Status   string `form:"status"`
	Keyword  string `form:"keyword"`
}

type RuleSetResponse struct {
	ID           uuid.UUID  `json:"id"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	RuleType     string     `json:"rule_type"`
	SourceType   string     `json:"source_type"`
	SourceURL    *string    `json:"source_url,omitempty"`
	Content      []string   `json:"content"`
	AutoUpdate   bool       `json:"auto_update"`
	LastSyncedAt *string    `json:"last_synced_at,omitempty"`
	Status       string     `json:"status"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

func NewRuleSetResponse(r *RouteRuleSet) RuleSetResponse {
	content := r.Content
	if content == nil {
		content = []string{}
	}
	resp := RuleSetResponse{
		ID:          r.ID,
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		RuleType:    r.RuleType,
		SourceType:  r.SourceType,
		SourceURL:   r.SourceURL,
		Content:     content,
		AutoUpdate:  r.AutoUpdate,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if r.LastSyncedAt != nil {
		s := r.LastSyncedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastSyncedAt = &s
	}
	return resp
}

// ===================== DTO: RoutePolicy =====================

type CreatePolicyRequest struct {
	Code             string `json:"code" binding:"required,min=1,max=64"`
	Name             string `json:"name" binding:"required,min=1,max=128"`
	Description      string `json:"description"`
	PolicyType       string `json:"policy_type"` // custom（默认）
	BaseTemplateCode string `json:"base_template_code"`
}

type ClonePolicyRequest struct {
	NewCode string `json:"new_code" binding:"required,min=1,max=64"`
	NewName string `json:"new_name" binding:"required,min=1,max=128"`
}

type UpdatePolicyRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type PolicyListQuery struct {
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
	PolicyType string `form:"policy_type"`
	Status     string `form:"status"`
	Keyword    string `form:"keyword"`
}

type PolicyResponse struct {
	ID               uuid.UUID          `json:"id"`
	Code             string             `json:"code"`
	Name             string             `json:"name"`
	Description      *string            `json:"description,omitempty"`
	PolicyType       string             `json:"policy_type"`
	BaseTemplateCode *string            `json:"base_template_code,omitempty"`
	Status           string             `json:"status"`
	Rules            []PolicyRuleResponse `json:"rules"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
}

func NewPolicyResponse(p *RoutePolicy) PolicyResponse {
	rules := make([]PolicyRuleResponse, 0, len(p.Rules))
	for _, r := range p.Rules {
		rules = append(rules, NewPolicyRuleResponse(&r))
	}
	return PolicyResponse{
		ID:               p.ID,
		Code:             p.Code,
		Name:             p.Name,
		Description:      p.Description,
		PolicyType:       p.PolicyType,
		BaseTemplateCode: p.BaseTemplateCode,
		Status:           p.Status,
		Rules:            rules,
		CreatedAt:        p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ===================== DTO: RoutePolicyRule =====================

type AddRuleRequest struct {
	RuleSource     string   `json:"rule_source" binding:"required,oneof=rule_set inline"`
	RuleSetID      string   `json:"rule_set_id"`
	InlineType     string   `json:"inline_type"`
	InlineValues   []string `json:"inline_values"`
	OutboundAction string   `json:"outbound_action" binding:"required,oneof=proxy direct blackhole warp tag balancer"`
	OutboundTag    string   `json:"outbound_tag"`
	Notes          string   `json:"notes"`
}

type UpdateRuleRequest struct {
	RuleSource     *string  `json:"rule_source"`
	RuleSetID      *string  `json:"rule_set_id"`
	InlineType     *string  `json:"inline_type"`
	InlineValues   []string `json:"inline_values"`
	OutboundAction *string  `json:"outbound_action"`
	OutboundTag    *string  `json:"outbound_tag"`
	Notes          *string  `json:"notes"`
}

type ReorderRulesRequest struct {
	RuleIDs []string `json:"rule_ids" binding:"required,min=1"`
}

type PolicyRuleResponse struct {
	ID             uuid.UUID  `json:"id"`
	PolicyID       uuid.UUID  `json:"policy_id"`
	SortOrder      int        `json:"sort_order"`
	RuleSource     string     `json:"rule_source"`
	RuleSetID      *uuid.UUID `json:"rule_set_id,omitempty"`
	InlineType     *string    `json:"inline_type,omitempty"`
	InlineValues   []string   `json:"inline_values"`
	OutboundAction string     `json:"outbound_action"`
	OutboundTag    *string    `json:"outbound_tag,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	CreatedAt      string     `json:"created_at"`
}

func NewPolicyRuleResponse(r *RoutePolicyRule) PolicyRuleResponse {
	values := r.InlineValues
	if values == nil {
		values = []string{}
	}
	return PolicyRuleResponse{
		ID:             r.ID,
		PolicyID:       r.PolicyID,
		SortOrder:      r.SortOrder,
		RuleSource:     r.RuleSource,
		RuleSetID:      r.RuleSetID,
		InlineType:     r.InlineType,
		InlineValues:   values,
		OutboundAction: r.OutboundAction,
		OutboundTag:    r.OutboundTag,
		Notes:          r.Notes,
		CreatedAt:      r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ===================== DTO: NodeRouteBinding =====================

type BindPolicyRequest struct {
	PolicyID   string `json:"policy_id" binding:"required"`
	BindScope  string `json:"bind_scope"`
	InboundTag string `json:"inbound_tag"`
}

type BindingResponse struct {
	NodeID     uuid.UUID `json:"node_id"`
	PolicyID   uuid.UUID `json:"policy_id"`
	BindScope  string    `json:"bind_scope"`
	InboundTag *string   `json:"inbound_tag,omitempty"`
	CreatedAt  string    `json:"created_at"`
}

func NewBindingResponse(b *NodeRouteBinding) BindingResponse {
	return BindingResponse{
		NodeID:     b.NodeID,
		PolicyID:   b.PolicyID,
		BindScope:  b.BindScope,
		InboundTag: b.InboundTag,
		CreatedAt:  b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ===================== DTO: NodeGroupLBPolicy =====================

type UpsertLBPolicyRequest struct {
	LBStrategy              string `json:"lb_strategy"`
	WeightField             string `json:"weight_field"`
	GeoAffinity             *bool  `json:"geo_affinity"`
	StickyBy                string `json:"sticky_by"`
	MinScoreThreshold       *int   `json:"min_score_threshold"`
	MaxNodesPerSubscription *int   `json:"max_nodes_per_subscription"`
	ExtraConfig             Map    `json:"extra_config"`
}

type LBPolicyResponse struct {
	ID                      uuid.UUID `json:"id"`
	GroupID                 uuid.UUID `json:"group_id"`
	LBStrategy              string    `json:"lb_strategy"`
	WeightField             string    `json:"weight_field"`
	GeoAffinity             bool      `json:"geo_affinity"`
	StickyBy                *string    `json:"sticky_by,omitempty"`
	MinScoreThreshold       int       `json:"min_score_threshold"`
	MaxNodesPerSubscription *int      `json:"max_nodes_per_subscription,omitempty"`
	ExtraConfig             Map       `json:"extra_config"`
	CreatedAt               string    `json:"created_at"`
	UpdatedAt               string    `json:"updated_at"`
}

func NewLBPolicyResponse(p *NodeGroupLBPolicy) LBPolicyResponse {
	return LBPolicyResponse{
		ID:                      p.ID,
		GroupID:                 p.GroupID,
		LBStrategy:              p.LBStrategy,
		WeightField:             p.WeightField,
		GeoAffinity:             p.GeoAffinity,
		StickyBy:                p.StickyBy,
		MinScoreThreshold:       p.MinScoreThreshold,
		MaxNodesPerSubscription: p.MaxNodesPerSubscription,
		ExtraConfig:             p.ExtraConfig,
		CreatedAt:               p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:               p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ===================== DTO: OutboundGroup =====================

type CreateOutboundGroupRequest struct {
	Tag                  string `json:"tag" binding:"required,min=1,max=64"`
	LBStrategy           string `json:"lb_strategy"`
	ProbeURL             string `json:"probe_url"`
	ProbeIntervalSeconds *int  `json:"probe_interval_seconds"`
	Members              []Map  `json:"members"`
}

type UpdateOutboundGroupRequest struct {
	LBStrategy           *string `json:"lb_strategy"`
	ProbeURL             *string `json:"probe_url"`
	ProbeIntervalSeconds *int    `json:"probe_interval_seconds"`
	Members              []Map   `json:"members"`
	Status               *string `json:"status"`
}

type OutboundGroupResponse struct {
	ID                   uuid.UUID `json:"id"`
	NodeID               uuid.UUID `json:"node_id"`
	Tag                  string    `json:"tag"`
	LBStrategy           string    `json:"lb_strategy"`
	ProbeURL             string    `json:"probe_url"`
	ProbeIntervalSeconds int       `json:"probe_interval_seconds"`
	Members              []Map     `json:"members"`
	Status               string    `json:"status"`
	CreatedAt            string    `json:"created_at"`
	UpdatedAt            string    `json:"updated_at"`
}

func NewOutboundGroupResponse(g *OutboundGroup) OutboundGroupResponse {
	members := g.Members
	if members == nil {
		members = []Map{}
	}
	return OutboundGroupResponse{
		ID:                   g.ID,
		NodeID:               g.NodeID,
		Tag:                  g.Tag,
		LBStrategy:           g.LBStrategy,
		ProbeURL:             g.ProbeURL,
		ProbeIntervalSeconds: g.ProbeIntervalSeconds,
		Members:              members,
		Status:               g.Status,
		CreatedAt:            g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           g.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ===================== 渲染结果 =====================

// RenderedRouting 是路由分流渲染结果（xray + sing-box）
type RenderedRouting struct {
	NodeID  uuid.UUID      `json:"node_id"`
	Xray    XrayRouting    `json:"xray"`
	SingBox SingBoxRouting `json:"sing_box"`
}

// XrayRouting 是 xray 格式的 routing 配置
type XrayRouting struct {
	Rules     []Map `json:"rules"`
	Balancers []Map `json:"balancers"`
}

// SingBoxRouting 是 sing-box 格式的 route 配置
type SingBoxRouting struct {
	Rules    []Map `json:"rules"`
	RuleSets []Map `json:"rule_set"`
}
