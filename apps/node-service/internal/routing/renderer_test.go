package routing

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeRoutingDataReader 是 RoutingDataReader 的内存实现，用于渲染测试
type fakeRoutingDataReader struct {
	bindings  []*NodeRouteBinding
	policies  map[uuid.UUID]*RoutePolicy
	rules     map[uuid.UUID][]*RoutePolicyRule
	ruleSets  map[uuid.UUID]*RouteRuleSet
	groups    []*OutboundGroup
}

func newFakeRoutingDataReader() *fakeRoutingDataReader {
	return &fakeRoutingDataReader{
		policies: make(map[uuid.UUID]*RoutePolicy),
		rules:    make(map[uuid.UUID][]*RoutePolicyRule),
		ruleSets: make(map[uuid.UUID]*RouteRuleSet),
	}
}

func (f *fakeRoutingDataReader) ListBindingsByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error) {
	return f.bindings, nil
}
func (f *fakeRoutingDataReader) GetPolicyByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error) {
	if p, ok := f.policies[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeRoutingDataReader) ListRulesByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error) {
	return f.rules[policyID], nil
}
func (f *fakeRoutingDataReader) GetRuleSetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	if rs, ok := f.ruleSets[id]; ok {
		return rs, nil
	}
	return nil, nil
}
func (f *fakeRoutingDataReader) ListOutboundGroupsByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error) {
	return f.groups, nil
}

// strPtr 返回字符串指针
func strPtr(s string) *string {
	return &s
}

// ===================== 准备内置规则集数据 =====================

// seedBuiltinRuleSets 填充与 000018_seed_routing.sql 一致的内置规则集
func seedBuiltinRuleSets(reader *fakeRoutingDataReader) {
	ruleSets := []*RouteRuleSet{
		{ID: uuid.New(), Code: "cn-direct", Name: "国内直连", RuleType: "builtin", SourceType: "geoip",
			Content: []string{"geoip:cn", "geosite:cn"}, Status: "active"},
		{ID: uuid.New(), Code: "private-direct", Name: "私有 IP 直连", RuleType: "builtin", SourceType: "inline",
			Content: []string{"geoip:private"}, Status: "active"},
		{ID: uuid.New(), Code: "streaming-unlock", Name: "流媒体域名", RuleType: "builtin", SourceType: "geosite",
			Content: []string{"geosite:netflix", "geosite:hulu", "geosite:disney", "geosite:hbo"}, Status: "active"},
		{ID: uuid.New(), Code: "openai", Name: "OpenAI 域名", RuleType: "builtin", SourceType: "inline",
			Content: []string{"domain_suffix:openai.com", "domain_suffix:anthropic.com", "domain_suffix:gemini.google.com"}, Status: "active"},
		{ID: uuid.New(), Code: "ads-block", Name: "广告屏蔽", RuleType: "builtin", SourceType: "geosite",
			Content: []string{"geosite:category-ads-all"}, Status: "active"},
	}
	for _, rs := range ruleSets {
		reader.ruleSets[rs.ID] = rs
	}
}

// findRuleSetByCode 按 code 查找规则集
func findRuleSetByCode(reader *fakeRoutingDataReader, code string) *RouteRuleSet {
	for _, rs := range reader.ruleSets {
		if rs.Code == code {
			return rs
		}
	}
	return nil
}

// ===================== 测试用例 =====================

func TestRenderRouting_StandardTemplate(t *testing.T) {
	reader := newFakeRoutingDataReader()
	seedBuiltinRuleSets(reader)

	prv := findRuleSetByCode(reader, "private-direct")
	ads := findRuleSetByCode(reader, "ads-block")
	cn := findRuleSetByCode(reader, "cn-direct")

	// 标准机场策略
	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "my-standard",
		Name:       "标准机场",
		PolicyType: "custom",
		Status:     "active",
	}
	reader.policies[policy.ID] = policy

	// 规则条目（与 tpl-standard 一致）
	reader.rules[policy.ID] = []*RoutePolicyRule{
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 10, RuleSource: "rule_set", RuleSetID: &prv.ID, OutboundAction: "direct"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 20, RuleSource: "rule_set", RuleSetID: &ads.ID, OutboundAction: "blackhole"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 30, RuleSource: "rule_set", RuleSetID: &cn.ID, OutboundAction: "direct"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 99, RuleSource: "inline", InlineValues: []string{}, OutboundAction: "proxy"},
	}

	nodeID := uuid.New()
	reader.bindings = []*NodeRouteBinding{
		{NodeID: nodeID, PolicyID: policy.ID, BindScope: "all"},
	}

	renderer := NewRoutingRenderer(reader, nil)
	result, err := renderer.RenderRouting(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("RenderRouting failed: %v", err)
	}

	// 断言：xray routing.rules 数量 = 4
	if len(result.Xray.Rules) != 4 {
		t.Fatalf("xray rules count = %d, want 4", len(result.Xray.Rules))
	}

	// 断言 outboundTag 映射正确
	outboundTags := make(map[string]bool)
	for _, rule := range result.Xray.Rules {
		if tag, ok := rule["outboundTag"].(string); ok {
			outboundTags[tag] = true
		}
	}
	if !outboundTags["direct"] {
		t.Errorf("expected outboundTag 'direct' in xray rules")
	}
	if !outboundTags["blackhole"] {
		t.Errorf("expected outboundTag 'blackhole' in xray rules")
	}
	if !outboundTags["proxy"] {
		t.Errorf("expected outboundTag 'proxy' (main outbound tag) in xray rules")
	}

	// 断言规则1（private-direct → direct）包含 geoip:private
	rule0 := result.Xray.Rules[0]
	if tag, _ := rule0["outboundTag"].(string); tag != "direct" {
		t.Errorf("rule0 outboundTag = %q, want direct", tag)
	}
	ips, _ := rule0["ip"].([]interface{})
	if len(ips) != 1 || ips[0] != "geoip:private" {
		t.Errorf("rule0 ip = %v, want [geoip:private]", ips)
	}

	// 断言规则2（ads-block → blackhole）包含 geosite:category-ads-all
	rule1 := result.Xray.Rules[1]
	if tag, _ := rule1["outboundTag"].(string); tag != "blackhole" {
		t.Errorf("rule1 outboundTag = %q, want blackhole", tag)
	}
	domains, _ := rule1["domain"].([]interface{})
	if len(domains) != 1 || domains[0] != "geosite:category-ads-all" {
		t.Errorf("rule1 domain = %v, want [geosite:category-ads-all]", domains)
	}

	// 断言规则3（cn-direct → direct）同时包含 geoip:cn 和 geosite:cn
	rule2 := result.Xray.Rules[2]
	if tag, _ := rule2["outboundTag"].(string); tag != "direct" {
		t.Errorf("rule2 outboundTag = %q, want direct", tag)
	}
	ips2, _ := rule2["ip"].([]interface{})
	if len(ips2) != 1 || ips2[0] != "geoip:cn" {
		t.Errorf("rule2 ip = %v, want [geoip:cn]", ips2)
	}
	domains2, _ := rule2["domain"].([]interface{})
	if len(domains2) != 1 || domains2[0] != "geosite:cn" {
		t.Errorf("rule2 domain = %v, want [geosite:cn]", domains2)
	}

	// 断言规则4（catch-all proxy）只有 outboundTag，无 domain/ip
	rule3 := result.Xray.Rules[3]
	if tag, _ := rule3["outboundTag"].(string); tag != "proxy" {
		t.Errorf("rule3 outboundTag = %q, want proxy", tag)
	}
	if _, hasDomain := rule3["domain"]; hasDomain {
		t.Errorf("rule3 should not have domain field")
	}
	if _, hasIP := rule3["ip"]; hasIP {
		t.Errorf("rule3 should not have ip field")
	}

	// 断言 sing-box rules 也有 4 条
	if len(result.SingBox.Rules) != 4 {
		t.Errorf("sing-box rules count = %d, want 4", len(result.SingBox.Rules))
	}
}

func TestRenderRouting_StreamingTemplate(t *testing.T) {
	reader := newFakeRoutingDataReader()
	seedBuiltinRuleSets(reader)

	prv := findRuleSetByCode(reader, "private-direct")
	ads := findRuleSetByCode(reader, "ads-block")
	stm := findRuleSetByCode(reader, "streaming-unlock")
	oai := findRuleSetByCode(reader, "openai")
	cn := findRuleSetByCode(reader, "cn-direct")

	// 流媒体解锁策略
	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "my-streaming",
		Name:       "流媒体解锁",
		PolicyType: "custom",
		Status:     "active",
	}
	reader.policies[policy.ID] = policy

	// 规则条目（与 tpl-streaming 一致）
	streamingOutTag := "streaming-out"
	reader.rules[policy.ID] = []*RoutePolicyRule{
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 10, RuleSource: "rule_set", RuleSetID: &prv.ID, OutboundAction: "direct"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 20, RuleSource: "rule_set", RuleSetID: &ads.ID, OutboundAction: "blackhole"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 30, RuleSource: "rule_set", RuleSetID: &oai.ID, OutboundAction: "warp"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 40, RuleSource: "rule_set", RuleSetID: &stm.ID, OutboundAction: "tag", OutboundTag: &streamingOutTag},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 50, RuleSource: "rule_set", RuleSetID: &cn.ID, OutboundAction: "direct"},
		{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 99, RuleSource: "inline", InlineValues: []string{}, OutboundAction: "proxy"},
	}

	nodeID := uuid.New()
	reader.bindings = []*NodeRouteBinding{
		{NodeID: nodeID, PolicyID: policy.ID, BindScope: "all"},
	}

	// 注入一个出站组 balancer
	reader.groups = []*OutboundGroup{
		{
			ID:     uuid.New(),
			NodeID: nodeID,
			Tag:    "lb-hk",
			LBStrategy: "leastPing",
			ProbeURL: "https://www.google.com/generate_204",
			ProbeIntervalSeconds: 60,
			Members: []Map{{"tag": "hk-isp1", "weight": 1}, {"tag": "hk-isp2", "weight": 1}},
			Status: "active",
		},
	}

	renderer := NewRoutingRenderer(reader, nil)
	result, err := renderer.RenderRouting(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("RenderRouting failed: %v", err)
	}

	// 断言：xray routing.rules 数量 = 6
	if len(result.Xray.Rules) != 6 {
		t.Fatalf("xray rules count = %d, want 6", len(result.Xray.Rules))
	}

	// 断言所有 outboundTag 都存在
	outboundTags := make(map[string]bool)
	for _, rule := range result.Xray.Rules {
		if tag, ok := rule["outboundTag"].(string); ok {
			outboundTags[tag] = true
		}
	}
	expectedTags := []string{"direct", "blackhole", "warp-out", "streaming-out", "proxy"}
	for _, expected := range expectedTags {
		if !outboundTags[expected] {
			t.Errorf("expected outboundTag %q in xray rules, got %v", expected, outboundTags)
		}
	}

	// 断言 warp-out 出站 tag 存在（OpenAI → warp）
	warpFound := false
	for _, rule := range result.Xray.Rules {
		if tag, _ := rule["outboundTag"].(string); tag == "warp-out" {
			warpFound = true
			// warp-out 规则应包含 OpenAI 域名
			domains, _ := rule["domain"].([]interface{})
			if len(domains) != 3 {
				t.Errorf("warp-out rule domain count = %d, want 3", len(domains))
			}
		}
	}
	if !warpFound {
		t.Errorf("warp-out outboundTag not found in xray rules")
	}

	// 断言 streaming-out tag 存在（流媒体 → tag: streaming-out）
	streamingFound := false
	for _, rule := range result.Xray.Rules {
		if tag, _ := rule["outboundTag"].(string); tag == "streaming-out" {
			streamingFound = true
			// streaming-out 规则应包含 geosite:netflix 等
			domains, _ := rule["domain"].([]interface{})
			if len(domains) != 4 {
				t.Errorf("streaming-out rule domain count = %d, want 4", len(domains))
			}
		}
	}
	if !streamingFound {
		t.Errorf("streaming-out outboundTag not found in xray rules")
	}

	// 断言 balancers 注入正确
	if len(result.Xray.Balancers) != 1 {
		t.Fatalf("xray balancers count = %d, want 1", len(result.Xray.Balancers))
	}
	balancer := result.Xray.Balancers[0]
	if tag, _ := balancer["tag"].(string); tag != "lb-hk" {
		t.Errorf("balancer tag = %q, want lb-hk", tag)
	}
	selector, _ := balancer["selector"].([]string)
	if len(selector) != 2 {
		t.Errorf("balancer selector count = %d, want 2", len(selector))
	}

	// 断言 sing-box rules 也有 6 条
	if len(result.SingBox.Rules) != 6 {
		t.Errorf("sing-box rules count = %d, want 6", len(result.SingBox.Rules))
	}

	// 断言 sing-box rule_set 声明（引用的规则集）
	if len(result.SingBox.RuleSets) == 0 {
		t.Errorf("sing-box rule_set should not be empty")
	}

	// 断言 sing-box 规则也包含 warp-out 和 streaming-out
	sbOutboundTags := make(map[string]bool)
	for _, rule := range result.SingBox.Rules {
		if tag, ok := rule["outbound"].(string); ok {
			sbOutboundTags[tag] = true
		}
	}
	if !sbOutboundTags["warp-out"] {
		t.Errorf("sing-box rules missing warp-out outbound")
	}
	if !sbOutboundTags["streaming-out"] {
		t.Errorf("sing-box rules missing streaming-out outbound")
	}
}

func TestRenderRouting_NoBindings_ReturnsEmpty(t *testing.T) {
	reader := newFakeRoutingDataReader()
	renderer := NewRoutingRenderer(reader, nil)

	result, err := renderer.RenderRouting(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("RenderRouting failed: %v", err)
	}
	if len(result.Xray.Rules) != 0 {
		t.Errorf("xray rules count = %d, want 0", len(result.Xray.Rules))
	}
	if len(result.Xray.Balancers) != 0 {
		t.Errorf("xray balancers count = %d, want 0", len(result.Xray.Balancers))
	}
}
