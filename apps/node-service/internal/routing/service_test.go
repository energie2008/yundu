package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// ===================== Fake Stores =====================

// fakeRuleSetStore 是 RuleSetStore 的内存实现
type fakeRuleSetStore struct {
	byID   map[uuid.UUID]*RouteRuleSet
	byCode map[string]*RouteRuleSet
}

func newFakeRuleSetStore() *fakeRuleSetStore {
	return &fakeRuleSetStore{
		byID:   make(map[uuid.UUID]*RouteRuleSet),
		byCode: make(map[string]*RouteRuleSet),
	}
}

func (f *fakeRuleSetStore) Create(ctx context.Context, rs *RouteRuleSet) error {
	if rs.ID == uuid.Nil {
		rs.ID = uuid.New()
	}
	f.byID[rs.ID] = rs
	f.byCode[rs.Code] = rs
	return nil
}
func (f *fakeRuleSetStore) GetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	if rs, ok := f.byID[id]; ok {
		return rs, nil
	}
	return nil, nil
}
func (f *fakeRuleSetStore) GetByCode(ctx context.Context, code string) (*RouteRuleSet, error) {
	if rs, ok := f.byCode[code]; ok {
		return rs, nil
	}
	return nil, nil
}
func (f *fakeRuleSetStore) Update(ctx context.Context, rs *RouteRuleSet) error {
	f.byID[rs.ID] = rs
	f.byCode[rs.Code] = rs
	return nil
}
func (f *fakeRuleSetStore) UpdateSynced(ctx context.Context, id uuid.UUID, content []string) error {
	if rs, ok := f.byID[id]; ok {
		rs.Content = content
	}
	return nil
}
func (f *fakeRuleSetStore) Delete(ctx context.Context, id uuid.UUID) error {
	if rs, ok := f.byID[id]; ok {
		delete(f.byID, id)
		delete(f.byCode, rs.Code)
	}
	return nil
}
func (f *fakeRuleSetStore) List(ctx context.Context, page, pageSize int, q RuleSetListQuery) ([]*RouteRuleSet, int, error) {
	var out []*RouteRuleSet
	for _, rs := range f.byID {
		out = append(out, rs)
	}
	return out, len(out), nil
}

// fakePolicyStore 是 PolicyStore 的内存实现
type fakePolicyStore struct {
	byID   map[uuid.UUID]*RoutePolicy
	byCode map[string]*RoutePolicy
}

func newFakePolicyStore() *fakePolicyStore {
	return &fakePolicyStore{
		byID:   make(map[uuid.UUID]*RoutePolicy),
		byCode: make(map[string]*RoutePolicy),
	}
}

func (f *fakePolicyStore) Create(ctx context.Context, p *RoutePolicy) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	f.byID[p.ID] = p
	f.byCode[p.Code] = p
	return nil
}
func (f *fakePolicyStore) GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakePolicyStore) GetByCode(ctx context.Context, code string) (*RoutePolicy, error) {
	if p, ok := f.byCode[code]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakePolicyStore) Update(ctx context.Context, p *RoutePolicy) error {
	f.byID[p.ID] = p
	f.byCode[p.Code] = p
	return nil
}
func (f *fakePolicyStore) Delete(ctx context.Context, id uuid.UUID) error {
	if p, ok := f.byID[id]; ok {
		delete(f.byID, id)
		delete(f.byCode, p.Code)
	}
	return nil
}
func (f *fakePolicyStore) List(ctx context.Context, page, pageSize int, q PolicyListQuery) ([]*RoutePolicy, int, error) {
	var out []*RoutePolicy
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, len(out), nil
}

// fakePolicyRuleStore 是 PolicyRuleStore 的内存实现
type fakePolicyRuleStore struct {
	byID map[uuid.UUID]*RoutePolicyRule
}

func newFakePolicyRuleStore() *fakePolicyRuleStore {
	return &fakePolicyRuleStore{byID: make(map[uuid.UUID]*RoutePolicyRule)}
}

func (f *fakePolicyRuleStore) Create(ctx context.Context, rule *RoutePolicyRule) error {
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	f.byID[rule.ID] = rule
	return nil
}
func (f *fakePolicyRuleStore) GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicyRule, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, nil
}
func (f *fakePolicyRuleStore) Update(ctx context.Context, rule *RoutePolicyRule) error {
	f.byID[rule.ID] = rule
	return nil
}
func (f *fakePolicyRuleStore) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.byID, id)
	return nil
}
func (f *fakePolicyRuleStore) ListByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error) {
	var out []*RoutePolicyRule
	for _, r := range f.byID {
		if r.PolicyID == policyID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakePolicyRuleStore) UpdateSortOrder(ctx context.Context, id uuid.UUID, sortOrder int) error {
	if r, ok := f.byID[id]; ok {
		r.SortOrder = sortOrder
	}
	return nil
}

// fakeBindingStore 是 BindingStore 的内存实现
type fakeBindingStore struct {
	bindings map[string]*NodeRouteBinding
}

func newFakeBindingStore() *fakeBindingStore {
	return &fakeBindingStore{bindings: make(map[string]*NodeRouteBinding)}
}

func bindingKey(nodeID, policyID uuid.UUID) string {
	return nodeID.String() + "|" + policyID.String()
}

func (f *fakeBindingStore) Create(ctx context.Context, b *NodeRouteBinding) error {
	key := bindingKey(b.NodeID, b.PolicyID)
	if _, exists := f.bindings[key]; exists {
		return nil // 幂等
	}
	f.bindings[key] = b
	return nil
}
func (f *fakeBindingStore) Delete(ctx context.Context, nodeID, policyID uuid.UUID) error {
	delete(f.bindings, bindingKey(nodeID, policyID))
	return nil
}
func (f *fakeBindingStore) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error) {
	var out []*NodeRouteBinding
	for _, b := range f.bindings {
		if b.NodeID == nodeID {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeBindingStore) Get(ctx context.Context, nodeID, policyID uuid.UUID) (*NodeRouteBinding, error) {
	if b, ok := f.bindings[bindingKey(nodeID, policyID)]; ok {
		return b, nil
	}
	return nil, nil
}

// fakeLBPolicyStore 是 LBPolicyStore 的内存实现
type fakeLBPolicyStore struct {
	byGroup map[uuid.UUID]*NodeGroupLBPolicy
}

func newFakeLBPolicyStore() *fakeLBPolicyStore {
	return &fakeLBPolicyStore{byGroup: make(map[uuid.UUID]*NodeGroupLBPolicy)}
}

func (f *fakeLBPolicyStore) GetByGroupID(ctx context.Context, groupID uuid.UUID) (*NodeGroupLBPolicy, error) {
	if p, ok := f.byGroup[groupID]; ok {
		return p, nil
	}
	return nil, nil
}
func (f *fakeLBPolicyStore) Upsert(ctx context.Context, p *NodeGroupLBPolicy) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	f.byGroup[p.GroupID] = p
	return nil
}

// fakeOutboundGroupStore 是 OutboundGroupStore 的内存实现
type fakeOutboundGroupStore struct {
	items []*OutboundGroup
}

func newFakeOutboundGroupStore() *fakeOutboundGroupStore {
	return &fakeOutboundGroupStore{}
}

func (f *fakeOutboundGroupStore) Create(ctx context.Context, g *OutboundGroup) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	f.items = append(f.items, g)
	return nil
}
func (f *fakeOutboundGroupStore) GetByNodeAndTag(ctx context.Context, nodeID uuid.UUID, tag string) (*OutboundGroup, error) {
	for _, g := range f.items {
		if g.NodeID == nodeID && g.Tag == tag {
			return g, nil
		}
	}
	return nil, nil
}
func (f *fakeOutboundGroupStore) Update(ctx context.Context, g *OutboundGroup) error {
	for i, existing := range f.items {
		if existing.NodeID == g.NodeID && existing.Tag == g.Tag {
			f.items[i] = g
			return nil
		}
	}
	return nil
}
func (f *fakeOutboundGroupStore) Delete(ctx context.Context, nodeID uuid.UUID, tag string) error {
	for i, g := range f.items {
		if g.NodeID == nodeID && g.Tag == tag {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return nil
}
func (f *fakeOutboundGroupStore) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error) {
	var out []*OutboundGroup
	for _, g := range f.items {
		if g.NodeID == nodeID {
			out = append(out, g)
		}
	}
	return out, nil
}

// ===================== 测试用例 =====================

func TestCreateRuleSet_Happy(t *testing.T) {
	store := newFakeRuleSetStore()
	svc := NewRuleSetService(store, nil, nil)

	req := &CreateRuleSetRequest{
		Code:       "my-rules",
		Name:       "自定义规则集",
		RuleType:   "custom",
		SourceType: "inline",
		Content:    []string{"domain_suffix:example.com"},
	}
	rs, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rs.Code != "my-rules" {
		t.Errorf("Code = %q, want my-rules", rs.Code)
	}
	if rs.RuleType != "custom" {
		t.Errorf("RuleType = %q, want custom", rs.RuleType)
	}
	if rs.Status != "active" {
		t.Errorf("Status = %q, want active", rs.Status)
	}
}

func TestCreateRuleSet_DuplicateCode_ReturnsConflict(t *testing.T) {
	store := newFakeRuleSetStore()
	existing := &RouteRuleSet{
		ID:      uuid.New(),
		Code:    "cn-direct",
		Name:    "国内直连",
		RuleType: "builtin",
	}
	store.byID[existing.ID] = existing
	store.byCode[existing.Code] = existing

	svc := NewRuleSetService(store, nil, nil)
	req := &CreateRuleSetRequest{
		Code:       "cn-direct",
		Name:       "重复",
		RuleType:   "custom",
		SourceType: "inline",
	}
	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, ErrRuleSetDuplicateCode) {
		t.Fatalf("expected ErrRuleSetDuplicateCode, got %v", err)
	}
}

func TestDeleteRuleSet_Builtin_ReturnsBuiltinCannotDelete(t *testing.T) {
	store := newFakeRuleSetStore()
	builtin := &RouteRuleSet{
		ID:       uuid.New(),
		Code:     "cn-direct",
		Name:     "国内直连",
		RuleType: "builtin",
	}
	store.byID[builtin.ID] = builtin
	store.byCode[builtin.Code] = builtin

	svc := NewRuleSetService(store, nil, nil)
	err := svc.Delete(context.Background(), builtin.ID)
	if !errors.Is(err, ErrBuiltinCannotDelete) {
		t.Fatalf("expected ErrBuiltinCannotDelete, got %v", err)
	}
}

func TestCreatePolicy_Happy(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()
	svc := NewPolicyService(policyStore, ruleStore, ruleSetStore, nil)

	req := &CreatePolicyRequest{
		Code: "my-policy",
		Name: "我的策略",
	}
	p, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.Code != "my-policy" {
		t.Errorf("Code = %q, want my-policy", p.Code)
	}
	if p.PolicyType != "custom" {
		t.Errorf("PolicyType = %q, want custom", p.PolicyType)
	}
}

func TestCloneFromTemplate_Happy(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()

	// 准备模板策略
	tmpl := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "tpl-standard",
		Name:       "标准机场",
		PolicyType: "builtin_template",
		Status:     "active",
	}
	policyStore.byID[tmpl.ID] = tmpl
	policyStore.byCode[tmpl.Code] = tmpl

	// 准备模板规则
	rule1 := &RoutePolicyRule{
		ID:             uuid.New(),
		PolicyID:       tmpl.ID,
		SortOrder:      10,
		RuleSource:     "inline",
		OutboundAction: "proxy",
	}
	ruleStore.byID[rule1.ID] = rule1

	svc := NewPolicyService(policyStore, ruleStore, ruleSetStore, nil)
	req := &ClonePolicyRequest{
		NewCode: "my-clone",
		NewName: "克隆策略",
	}
	p, err := svc.CloneFromTemplate(context.Background(), "tpl-standard", req)
	if err != nil {
		t.Fatalf("CloneFromTemplate failed: %v", err)
	}
	if p.Code != "my-clone" {
		t.Errorf("Code = %q, want my-clone", p.Code)
	}
	if p.PolicyType != "custom" {
		t.Errorf("PolicyType = %q, want custom", p.PolicyType)
	}
	if p.BaseTemplateCode == nil || *p.BaseTemplateCode != "tpl-standard" {
		t.Errorf("BaseTemplateCode = %v, want tpl-standard", p.BaseTemplateCode)
	}

	// 验证规则已复制
	clonedRules, _ := ruleStore.ListByPolicy(context.Background(), p.ID)
	if len(clonedRules) != 1 {
		t.Errorf("cloned rules count = %d, want 1", len(clonedRules))
	}
}

func TestDeletePolicy_BuiltinTemplate_ReturnsCannotDelete(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()

	tmpl := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "tpl-standard",
		Name:       "标准机场",
		PolicyType: "builtin_template",
	}
	policyStore.byID[tmpl.ID] = tmpl
	policyStore.byCode[tmpl.Code] = tmpl

	svc := NewPolicyService(policyStore, ruleStore, ruleSetStore, nil)
	err := svc.Delete(context.Background(), tmpl.ID)
	if !errors.Is(err, ErrTemplateCannotDelete) {
		t.Fatalf("expected ErrTemplateCannotDelete, got %v", err)
	}
}

func TestAddPolicyRule_Happy(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()

	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "my-policy",
		Name:       "我的策略",
		PolicyType: "custom",
	}
	policyStore.byID[policy.ID] = policy
	policyStore.byCode[policy.Code] = policy

	svc := NewPolicyRuleService(ruleStore, policyStore, ruleSetStore, nil)
	req := &AddRuleRequest{
		RuleSource:     "inline",
		InlineType:     "domain_suffix",
		InlineValues:   []string{"openai.com"},
		OutboundAction: "warp",
	}
	rule, err := svc.AddRule(context.Background(), policy.ID, req)
	if err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	if rule.SortOrder != 10 {
		t.Errorf("SortOrder = %d, want 10", rule.SortOrder)
	}
	if rule.OutboundAction != "warp" {
		t.Errorf("OutboundAction = %q, want warp", rule.OutboundAction)
	}
}

func TestAddPolicyRule_TagAction_MissingOutboundTag_ReturnsInvalid(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()

	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "my-policy",
		Name:       "我的策略",
		PolicyType: "custom",
	}
	policyStore.byID[policy.ID] = policy

	svc := NewPolicyRuleService(ruleStore, policyStore, ruleSetStore, nil)
	req := &AddRuleRequest{
		RuleSource:     "inline",
		OutboundAction: "tag",
		// 故意不设 OutboundTag
	}
	_, err := svc.AddRule(context.Background(), policy.ID, req)
	if !errors.Is(err, ErrInvalidOutboundAction) {
		t.Fatalf("expected ErrInvalidOutboundAction, got %v", err)
	}
}

func TestReorderRules_Happy(t *testing.T) {
	policyStore := newFakePolicyStore()
	ruleStore := newFakePolicyRuleStore()
	ruleSetStore := newFakeRuleSetStore()

	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "reorder-test",
		Name:       "重排测试",
		PolicyType: "custom",
	}
	policyStore.byID[policy.ID] = policy

	rule1 := &RoutePolicyRule{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 10}
	rule2 := &RoutePolicyRule{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 20}
	rule3 := &RoutePolicyRule{ID: uuid.New(), PolicyID: policy.ID, SortOrder: 30}
	ruleStore.byID[rule1.ID] = rule1
	ruleStore.byID[rule2.ID] = rule2
	ruleStore.byID[rule3.ID] = rule3

	svc := NewPolicyRuleService(ruleStore, policyStore, ruleSetStore, nil)
	// 反序排列
	err := svc.Reorder(context.Background(), policy.ID, []uuid.UUID{rule3.ID, rule2.ID, rule1.ID})
	if err != nil {
		t.Fatalf("Reorder failed: %v", err)
	}

	if rule3.SortOrder != 10 {
		t.Errorf("rule3 SortOrder = %d, want 10", rule3.SortOrder)
	}
	if rule2.SortOrder != 20 {
		t.Errorf("rule2 SortOrder = %d, want 20", rule2.SortOrder)
	}
	if rule1.SortOrder != 30 {
		t.Errorf("rule1 SortOrder = %d, want 30", rule1.SortOrder)
	}
}

func TestBindPolicy_Happy(t *testing.T) {
	bindingStore := newFakeBindingStore()
	policyStore := newFakePolicyStore()

	policy := &RoutePolicy{
		ID:         uuid.New(),
		Code:       "bind-test",
		Name:       "绑定测试",
		PolicyType: "custom",
	}
	policyStore.byID[policy.ID] = policy

	nodeID := uuid.New()
	svc := NewBindingService(bindingStore, policyStore, nil)
	b, err := svc.Bind(context.Background(), nodeID, policy.ID, "all", "")
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}
	if b.NodeID != nodeID {
		t.Errorf("NodeID = %v, want %v", b.NodeID, nodeID)
	}
	if b.BindScope != "all" {
		t.Errorf("BindScope = %q, want all", b.BindScope)
	}
}

func TestUpsertLBPolicy_Happy(t *testing.T) {
	store := newFakeLBPolicyStore()
	svc := NewLBPolicyService(store, nil)

	groupID := uuid.New()
	threshold := 50
	req := &UpsertLBPolicyRequest{
		LBStrategy:        "latency",
		MinScoreThreshold: &threshold,
		GeoAffinity:       boolPtr(true),
	}
	p, err := svc.Upsert(context.Background(), groupID, req)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if p.LBStrategy != "latency" {
		t.Errorf("LBStrategy = %q, want latency", p.LBStrategy)
	}
	if p.MinScoreThreshold != 50 {
		t.Errorf("MinScoreThreshold = %d, want 50", p.MinScoreThreshold)
	}
	if !p.GeoAffinity {
		t.Errorf("GeoAffinity = false, want true")
	}

	// 再次 Upsert（更新）
	req2 := &UpsertLBPolicyRequest{
		LBStrategy: "round_robin",
	}
	p2, err := svc.Upsert(context.Background(), groupID, req2)
	if err != nil {
		t.Fatalf("Upsert (update) failed: %v", err)
	}
	if p2.LBStrategy != "round_robin" {
		t.Errorf("LBStrategy = %q, want round_robin", p2.LBStrategy)
	}
}

func TestCreateOutboundGroup_DuplicateTag_ReturnsConflict(t *testing.T) {
	store := newFakeOutboundGroupStore()
	nodeID := uuid.New()
	existing := &OutboundGroup{
		ID:     uuid.New(),
		NodeID: nodeID,
		Tag:    "lb-hk",
	}
	store.items = append(store.items, existing)

	svc := NewOutboundGroupService(store, nil)
	req := &CreateOutboundGroupRequest{
		Tag:    "lb-hk",
	}
	_, err := svc.Create(context.Background(), nodeID, req)
	if !errors.Is(err, ErrOutboundGroupDuplicate) {
		t.Fatalf("expected ErrOutboundGroupDuplicate, got %v", err)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
