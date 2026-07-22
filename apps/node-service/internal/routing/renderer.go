package routing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// 主出站 tag（proxy 动作映射到此 tag）
const MainOutboundTag = "proxy"

// RoutingDataReaderAdapter 把 5 个 Repo 组合为 RoutingDataReader 接口的实现
type RoutingDataReaderAdapter struct {
	BindingRepo        *NodeRouteBindingRepo
	PolicyRepo         *RoutePolicyRepo
	RuleRepo           *RoutePolicyRuleRepo
	RuleSetRepo        *RouteRuleSetRepo
	OutboundGroupRepo  *OutboundGroupRepo
}

func (a *RoutingDataReaderAdapter) ListBindingsByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error) {
	return a.BindingRepo.ListByNode(ctx, nodeID)
}

func (a *RoutingDataReaderAdapter) GetPolicyByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error) {
	return a.PolicyRepo.GetByID(ctx, id)
}

func (a *RoutingDataReaderAdapter) ListRulesByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error) {
	return a.RuleRepo.ListByPolicy(ctx, policyID)
}

func (a *RoutingDataReaderAdapter) GetRuleSetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	return a.RuleSetRepo.GetByID(ctx, id)
}

func (a *RoutingDataReaderAdapter) ListOutboundGroupsByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error) {
	return a.OutboundGroupRepo.ListByNode(ctx, nodeID)
}

// ===================== 渲染数据源接口 =====================

// RoutingDataReader 抽象渲染所需的数据读取（便于测试注入 mock）
type RoutingDataReader interface {
	ListBindingsByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error)
	GetPolicyByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error)
	ListRulesByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error)
	GetRuleSetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error)
	ListOutboundGroupsByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error)
}

// ===================== RoutingRenderer =====================

// RoutingRenderer 负责把节点绑定的路由策略渲染为 xray / sing-box 的 routing 配置。
// 与 outbound.RenderOutbounds 配合：outbound 渲染器生成 outbounds，本渲染器生成 routing.rules + balancers。
type RoutingRenderer struct {
	reader RoutingDataReader
	logger *slog.Logger
}

func NewRoutingRenderer(reader RoutingDataReader, logger *slog.Logger) *RoutingRenderer {
	return &RoutingRenderer{reader: reader, logger: logger}
}

// RenderRouting 读取节点绑定的 route_policy，展开 route_policy_rules，
// 按 rule_source 渲染 geoip/geosite/domain/ip_cidr 规则，
// 按 outbound_action 映射到 xray routing.rules 的 outboundTag，
// 注入 outbound_groups balancers。
func (r *RoutingRenderer) RenderRouting(ctx context.Context, nodeID uuid.UUID) (*RenderedRouting, error) {
	// 1. 读取节点绑定的策略
	bindings, err := r.reader.ListBindingsByNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}

	xrayRules := make([]Map, 0)
	singBoxRules := make([]Map, 0)
	xrayBalancers := make([]Map, 0)
	singBoxRuleSets := make([]Map, 0)
	seenRuleSets := make(map[uuid.UUID]bool)

	// 2. 遍历每个绑定的策略，展开规则
	for _, b := range bindings {
		policy, err := r.reader.GetPolicyByID(ctx, b.PolicyID)
		if err != nil {
			return nil, fmt.Errorf("get policy %s: %w", b.PolicyID, err)
		}
		if policy == nil {
			continue // 策略不存在，跳过
		}

		rules, err := r.reader.ListRulesByPolicy(ctx, policy.ID)
		if err != nil {
			return nil, fmt.Errorf("list rules for policy %s: %w", policy.ID, err)
		}

		for _, rule := range rules {
			xrayRule, sbRule, rsIDs, err := r.renderRule(ctx, rule)
			if err != nil {
				return nil, err
			}
			if xrayRule != nil {
				xrayRules = append(xrayRules, xrayRule)
			}
			if sbRule != nil {
				singBoxRules = append(singBoxRules, sbRule)
			}
			// 收集引用的 rule_set（用于 sing-box rule_set 声明）
			for _, rsID := range rsIDs {
				if !seenRuleSets[rsID] {
					seenRuleSets[rsID] = true
					if rs, err := r.reader.GetRuleSetByID(ctx, rsID); err == nil && rs != nil {
						singBoxRuleSets = append(singBoxRuleSets, buildSingBoxRuleSet(rs))
					}
				}
			}
		}
	}

	// 3. 注入 outbound_groups balancers
	groups, err := r.reader.ListOutboundGroupsByNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list outbound groups: %w", err)
	}
	for _, g := range groups {
		if g.Status != "active" {
			continue
		}
		xrayBalancers = append(xrayBalancers, buildXrayBalancer(g))
	}

	return &RenderedRouting{
		NodeID: nodeID,
		Xray: XrayRouting{
			Rules:     xrayRules,
			Balancers: xrayBalancers,
		},
		SingBox: SingBoxRouting{
			Rules:    singBoxRules,
			RuleSets: singBoxRuleSets,
		},
	}, nil
}

// renderRule 渲染单条策略规则为 xray rule + sing-box rule
// 返回：xray rule, sing-box rule, 引用的 rule_set IDs
func (r *RoutingRenderer) renderRule(ctx context.Context, rule *RoutePolicyRule) (Map, Map, []uuid.UUID, error) {
	outboundTag := mapOutboundAction(rule.OutboundAction, rule.OutboundTag)
	if outboundTag == "" {
		return nil, nil, nil, fmt.Errorf("%w: outbound_action %s has no outbound_tag", ErrInvalidOutboundAction, rule.OutboundAction)
	}

	// 收集规则匹配条件
	var xrayDomains, xrayIPs []interface{}
	var sbDomains, sbDomainSuffixes, sbDomainKeywords, sbIPCIDRs []interface{}
	var sbGeoSites, sbGeoIPs []interface{}
	var referencedRuleSets []uuid.UUID

	if rule.RuleSource == "rule_set" && rule.RuleSetID != nil {
		// 从 rule_set 展开规则
		rs, err := r.reader.GetRuleSetByID(ctx, *rule.RuleSetID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("get rule_set %s: %w", *rule.RuleSetID, err)
		}
		if rs != nil {
			referencedRuleSets = append(referencedRuleSets, *rule.RuleSetID)
			for _, entry := range rs.Content {
				kind, value := parseRuleEntry(entry)
				applyEntry(kind, value,
					&xrayDomains, &xrayIPs,
					&sbDomains, &sbDomainSuffixes, &sbDomainKeywords, &sbIPCIDRs,
					&sbGeoSites, &sbGeoIPs,
				)
			}
		}
	} else if rule.RuleSource == "inline" {
		// inline 规则：使用 inline_type + inline_values
		inlineType := ""
		if rule.InlineType != nil {
			inlineType = *rule.InlineType
		}
		for _, value := range rule.InlineValues {
			applyInlineEntry(inlineType, value,
				&xrayDomains, &xrayIPs,
				&sbDomains, &sbDomainSuffixes, &sbDomainKeywords, &sbIPCIDRs,
				&sbGeoSites, &sbGeoIPs,
			)
		}
	}

	// 构建 xray rule
	xrayRule := Map{
		"type":         "field",
		"outboundTag":  outboundTag,
	}
	if len(xrayDomains) > 0 {
		xrayRule["domain"] = xrayDomains
	}
	if len(xrayIPs) > 0 {
		xrayRule["ip"] = xrayIPs
	}

	// 构建 sing-box rule
	sbRule := Map{
		"outbound": outboundTag,
	}
	if len(sbDomains) > 0 {
		sbRule["domain"] = sbDomains
	}
	if len(sbDomainSuffixes) > 0 {
		sbRule["domain_suffix"] = sbDomainSuffixes
	}
	if len(sbDomainKeywords) > 0 {
		sbRule["domain_keyword"] = sbDomainKeywords
	}
	if len(sbIPCIDRs) > 0 {
		sbRule["ip_cidr"] = sbIPCIDRs
	}
	if len(sbGeoSites) > 0 {
		// sing-box 接受 geosite: 前缀
		if existing, ok := sbRule["domain"].([]interface{}); ok {
			sbRule["domain"] = append(existing, sbGeoSites...)
		} else {
			sbRule["domain"] = sbGeoSites
		}
	}
	if len(sbGeoIPs) > 0 {
		if existing, ok := sbRule["ip_cidr"].([]interface{}); ok {
			sbRule["ip_cidr"] = append(existing, sbGeoIPs...)
		} else {
			sbRule["ip_cidr"] = sbGeoIPs
		}
	}

	return xrayRule, sbRule, referencedRuleSets, nil
}

// mapOutboundAction 将 outbound_action 映射到 outboundTag
// proxy→主出站tag, direct→"direct", blackhole→"blackhole", warp→"warp-out",
// tag→outbound_tag 字段值, balancer→outbound_tag 字段值
func mapOutboundAction(action string, outboundTag *string) string {
	switch action {
	case "proxy":
		return MainOutboundTag
	case "direct":
		return "direct"
	case "blackhole":
		return "blackhole"
	case "warp":
		return "warp-out"
	case "tag", "balancer":
		if outboundTag != nil && *outboundTag != "" {
			return *outboundTag
		}
		return ""
	}
	return ""
}

// parseRuleEntry 解析规则集条目，如 "geoip:cn" → ("geoip", "cn")
func parseRuleEntry(entry string) (kind, value string) {
	idx := strings.Index(entry, ":")
	if idx > 0 {
		return entry[:idx], entry[idx+1:]
	}
	return "domain", entry
}

// applyEntry 将规则集条目应用到 xray/sing-box 规则字段
func applyEntry(kind, value string,
	xrayDomains, xrayIPs *[]interface{},
	sbDomains, sbDomainSuffixes, sbDomainKeywords, sbIPCIDRs *[]interface{},
	sbGeoSites, sbGeoIPs *[]interface{},
) {
	switch kind {
	case "geoip":
		*xrayIPs = append(*xrayIPs, "geoip:"+value)
		*sbGeoIPs = append(*sbGeoIPs, "geoip:"+value)
	case "geosite":
		*xrayDomains = append(*xrayDomains, "geosite:"+value)
		*sbGeoSites = append(*sbGeoSites, "geosite:"+value)
	case "domain":
		*xrayDomains = append(*xrayDomains, "domain:"+value)
		*sbDomains = append(*sbDomains, value)
	case "domain_suffix":
		*xrayDomains = append(*xrayDomains, "domain:"+value)
		*sbDomainSuffixes = append(*sbDomainSuffixes, value)
	case "domain_keyword":
		*xrayDomains = append(*xrayDomains, value)
		*sbDomainKeywords = append(*sbDomainKeywords, value)
	case "ip_cidr":
		*xrayIPs = append(*xrayIPs, value)
		*sbIPCIDRs = append(*sbIPCIDRs, value)
	}
}

// applyInlineEntry 将 inline 规则条目应用到 xray/sing-box 规则字段
func applyInlineEntry(inlineType, value string,
	xrayDomains, xrayIPs *[]interface{},
	sbDomains, sbDomainSuffixes, sbDomainKeywords, sbIPCIDRs *[]interface{},
	sbGeoSites, sbGeoIPs *[]interface{},
) {
	switch inlineType {
	case "geoip":
		*xrayIPs = append(*xrayIPs, "geoip:"+value)
		*sbGeoIPs = append(*sbGeoIPs, "geoip:"+value)
	case "geosite":
		*xrayDomains = append(*xrayDomains, "geosite:"+value)
		*sbGeoSites = append(*sbGeoSites, "geosite:"+value)
	case "domain":
		*xrayDomains = append(*xrayDomains, "domain:"+value)
		*sbDomains = append(*sbDomains, value)
	case "domain_suffix":
		*xrayDomains = append(*xrayDomains, "domain:"+value)
		*sbDomainSuffixes = append(*sbDomainSuffixes, value)
	case "domain_keyword":
		*xrayDomains = append(*xrayDomains, value)
		*sbDomainKeywords = append(*sbDomainKeywords, value)
	case "ip_cidr":
		*xrayIPs = append(*xrayIPs, value)
		*sbIPCIDRs = append(*sbIPCIDRs, value)
	}
}

// buildXrayBalancer 把 outbound_group 渲染为 xray balancer 配置
func buildXrayBalancer(g *OutboundGroup) Map {
	selector := make([]string, 0, len(g.Members))
	for _, m := range g.Members {
		if tag, ok := m["tag"].(string); ok && tag != "" {
			selector = append(selector, tag)
		}
	}
	return Map{
		"tag":      g.Tag,
		"selector": selector,
		"strategy": Map{
			"type": g.LBStrategy,
		},
		"probeUrl":      g.ProbeURL,
		"probeInterval": fmt.Sprintf("%ds", g.ProbeIntervalSeconds),
	}
}

// buildSingBoxRuleSet 把 route_rule_set 渲染为 sing-box rule_set 声明
func buildSingBoxRuleSet(rs *RouteRuleSet) Map {
	return Map{
		"tag":    rs.Code,
		"type":   "remote",
		"format": "binary",
	}
}
