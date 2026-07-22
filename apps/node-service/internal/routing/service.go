package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// ===================== Store 接口 =====================

// RuleSetStore 抽象 RouteRuleSetRepo 的数据访问
type RuleSetStore interface {
	Create(ctx context.Context, rs *RouteRuleSet) error
	GetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error)
	GetByCode(ctx context.Context, code string) (*RouteRuleSet, error)
	Update(ctx context.Context, rs *RouteRuleSet) error
	UpdateSynced(ctx context.Context, id uuid.UUID, content []string) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, page, pageSize int, q RuleSetListQuery) ([]*RouteRuleSet, int, error)
}

// PolicyStore 抽象 RoutePolicyRepo 的数据访问
type PolicyStore interface {
	Create(ctx context.Context, p *RoutePolicy) error
	GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error)
	GetByCode(ctx context.Context, code string) (*RoutePolicy, error)
	Update(ctx context.Context, p *RoutePolicy) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, page, pageSize int, q PolicyListQuery) ([]*RoutePolicy, int, error)
}

// PolicyRuleStore 抽象 RoutePolicyRuleRepo 的数据访问
type PolicyRuleStore interface {
	Create(ctx context.Context, rule *RoutePolicyRule) error
	GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicyRule, error)
	Update(ctx context.Context, rule *RoutePolicyRule) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error)
	UpdateSortOrder(ctx context.Context, id uuid.UUID, sortOrder int) error
}

// BindingStore 抽象 NodeRouteBindingRepo 的数据访问
type BindingStore interface {
	Create(ctx context.Context, b *NodeRouteBinding) error
	Delete(ctx context.Context, nodeID, policyID uuid.UUID) error
	ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error)
	Get(ctx context.Context, nodeID, policyID uuid.UUID) (*NodeRouteBinding, error)
}

// LBPolicyStore 抽象 NodeGroupLBPolicyRepo 的数据访问
type LBPolicyStore interface {
	GetByGroupID(ctx context.Context, groupID uuid.UUID) (*NodeGroupLBPolicy, error)
	Upsert(ctx context.Context, p *NodeGroupLBPolicy) error
}

// OutboundGroupStore 抽象 OutboundGroupRepo 的数据访问
type OutboundGroupStore interface {
	Create(ctx context.Context, g *OutboundGroup) error
	GetByNodeAndTag(ctx context.Context, nodeID uuid.UUID, tag string) (*OutboundGroup, error)
	Update(ctx context.Context, g *OutboundGroup) error
	Delete(ctx context.Context, nodeID uuid.UUID, tag string) error
	ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error)
}

// URLFetcher 抽象远程 URL 抓取（便于测试注入 mock）
type URLFetcher interface {
	Fetch(url string) ([]byte, error)
}

// defaultURLFetcher 是基于 net/http 的默认实现
type defaultURLFetcher struct {
	client *http.Client
}

func newDefaultURLFetcher() *defaultURLFetcher {
	return &defaultURLFetcher{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *defaultURLFetcher) Fetch(url string) ([]byte, error) {
	// B35 修复: 限制响应体大小为 1MB，防止恶意远程 URL 返回超大内容导致内存耗尽
	const maxRespSize int64 = 1 * 1024 * 1024 // 1MB
	resp, err := f.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote url returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRespSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxRespSize {
		return nil, fmt.Errorf("remote url response exceeds %d bytes limit", maxRespSize)
	}
	return data, nil
}

// ===================== RuleSetService =====================

// RuleSetService 封装路由规则集的业务逻辑
type RuleSetService struct {
	store   RuleSetStore
	fetcher URLFetcher
	logger  *slog.Logger
}

func NewRuleSetService(store RuleSetStore, fetcher URLFetcher, logger *slog.Logger) *RuleSetService {
	svc := &RuleSetService{store: store, fetcher: fetcher, logger: logger}
	if fetcher == nil {
		svc.fetcher = newDefaultURLFetcher()
	}
	return svc
}

func (s *RuleSetService) List(ctx context.Context, page, pageSize int, q RuleSetListQuery) ([]*RouteRuleSet, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize, q)
}

func (s *RuleSetService) GetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	rs, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rs == nil {
		return nil, ErrRuleSetNotFound
	}
	return rs, nil
}

// Create 创建路由规则集
func (s *RuleSetService) Create(ctx context.Context, req *CreateRuleSetRequest) (*RouteRuleSet, error) {
	existing, err := s.store.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrRuleSetDuplicateCode
	}

	content := req.Content
	if content == nil {
		content = []string{}
	}
	autoUpdate := false
	if req.AutoUpdate != nil {
		autoUpdate = *req.AutoUpdate
	}

	var description *string
	if req.Description != "" {
		description = &req.Description
	}
	var sourceURL *string
	if req.SourceURL != "" {
		sourceURL = &req.SourceURL
	}

	rs := &RouteRuleSet{
		Code:        req.Code,
		Name:        req.Name,
		Description: description,
		RuleType:    req.RuleType,
		SourceType:  req.SourceType,
		SourceURL:   sourceURL,
		Content:     content,
		AutoUpdate:  autoUpdate,
		Status:      "active",
	}

	if err := s.store.Create(ctx, rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Update 更新规则集基本信息
func (s *RuleSetService) Update(ctx context.Context, id uuid.UUID, req *UpdateRuleSetRequest) (*RouteRuleSet, error) {
	rs, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rs == nil {
		return nil, ErrRuleSetNotFound
	}

	if req.Name != nil {
		rs.Name = *req.Name
	}
	if req.Description != nil {
		rs.Description = req.Description
	}
	if req.Content != nil {
		rs.Content = req.Content
	}
	if req.AutoUpdate != nil {
		rs.AutoUpdate = *req.AutoUpdate
	}
	if req.Status != nil {
		rs.Status = *req.Status
	}

	if err := s.store.Update(ctx, rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Delete 删除规则集（内置规则集不可删除）
func (s *RuleSetService) Delete(ctx context.Context, id uuid.UUID) error {
	rs, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if rs == nil {
		return ErrRuleSetNotFound
	}
	if rs.RuleType == "builtin" {
		return ErrBuiltinCannotDelete
	}
	return s.store.Delete(ctx, id)
}

// SyncFromURL 从远程 URL 同步规则集内容
func (s *RuleSetService) SyncFromURL(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	rs, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rs == nil {
		return nil, ErrRuleSetNotFound
	}
	if rs.SourceURL == nil || *rs.SourceURL == "" {
		return nil, ErrRuleSetNoSourceURL
	}

	// B35 修复: 内容安全校验——仅允许 http/https 协议，防止 SSRF 与本地文件读取
	parsedURL, err := url.Parse(*rs.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid source url: %v", ErrRuleSetSyncFailed, err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("%w: unsupported url scheme '%s' (only http/https allowed)", ErrRuleSetSyncFailed, parsedURL.Scheme)
	}

	data, err := s.fetcher.Fetch(*rs.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRuleSetSyncFailed, err)
	}

	// B35 修复: 基本内容校验——必须非空
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: remote content is empty", ErrRuleSetSyncFailed)
	}

	// 解析为字符串数组
	var content []string
	if err := json.Unmarshal(data, &content); err != nil {
		// 非 JSON 数组，尝试按行分割
		lines := splitLines(string(data))
		if len(lines) == 0 {
			return nil, fmt.Errorf("%w: invalid content format", ErrRuleSetSyncFailed)
		}
		content = lines
	}

	if err := s.store.UpdateSynced(ctx, id, content); err != nil {
		return nil, err
	}

	// 重新读取返回最新数据
	return s.store.GetByID(ctx, id)
}

// splitLines 按换行符分割文本（远程规则集可能是纯文本格式）
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i, c := range text {
		if c == '\n' {
			line := text[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(text) {
		line := text[start:]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// ===================== PolicyService =====================

// PolicyService 封装路由策略的业务逻辑
type PolicyService struct {
	store      PolicyStore
	ruleStore  PolicyRuleStore
	ruleSetStore RuleSetStore
	logger     *slog.Logger
}

func NewPolicyService(store PolicyStore, ruleStore PolicyRuleStore, ruleSetStore RuleSetStore, logger *slog.Logger) *PolicyService {
	return &PolicyService{store: store, ruleStore: ruleStore, ruleSetStore: ruleSetStore, logger: logger}
}

func (s *PolicyService) List(ctx context.Context, page, pageSize int, q PolicyListQuery) ([]*RoutePolicy, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize, q)
}

// GetByID 获取策略详情（含规则列表）
func (s *PolicyService) GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}
	rules, err := s.ruleStore.ListByPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Rules = make([]RoutePolicyRule, len(rules))
	for i, r := range rules {
		p.Rules[i] = *r
	}
	return p, nil
}

// Create 创建路由策略
func (s *PolicyService) Create(ctx context.Context, req *CreatePolicyRequest) (*RoutePolicy, error) {
	existing, err := s.store.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPolicyDuplicateCode
	}

	policyType := req.PolicyType
	if policyType == "" {
		policyType = "custom"
	}

	var description *string
	if req.Description != "" {
		description = &req.Description
	}
	var baseTemplateCode *string
	if req.BaseTemplateCode != "" {
		baseTemplateCode = &req.BaseTemplateCode
	}

	p := &RoutePolicy{
		Code:             req.Code,
		Name:             req.Name,
		Description:      description,
		PolicyType:       policyType,
		BaseTemplateCode: baseTemplateCode,
		Status:           "active",
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// CloneFromTemplate 从内置模板克隆一个自定义策略（含规则条目）
func (s *PolicyService) CloneFromTemplate(ctx context.Context, templateCode string, req *ClonePolicyRequest) (*RoutePolicy, error) {
	tmpl, err := s.store.GetByCode(ctx, templateCode)
	if err != nil {
		return nil, err
	}
	if tmpl == nil {
		return nil, ErrTemplateNotFound
	}

	// 检查新 code 不重复
	existing, err := s.store.GetByCode(ctx, req.NewCode)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPolicyDuplicateCode
	}

	// 创建新策略
	p := &RoutePolicy{
		Code:             req.NewCode,
		Name:             req.NewName,
		Description:      tmpl.Description,
		PolicyType:       "custom",
		BaseTemplateCode: &tmpl.Code,
		Status:           "active",
	}
	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}

	// 复制模板的规则条目
	rules, err := s.ruleStore.ListByPolicy(ctx, tmpl.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range rules {
		newRule := &RoutePolicyRule{
			PolicyID:       p.ID,
			SortOrder:      r.SortOrder,
			RuleSource:     r.RuleSource,
			RuleSetID:      r.RuleSetID,
			InlineType:     r.InlineType,
			InlineValues:   r.InlineValues,
			OutboundAction: r.OutboundAction,
			OutboundTag:    r.OutboundTag,
			Notes:          r.Notes,
		}
		if err := s.ruleStore.Create(ctx, newRule); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// Update 更新策略基本信息
func (s *PolicyService) Update(ctx context.Context, id uuid.UUID, req *UpdatePolicyRequest) (*RoutePolicy, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.Status != nil {
		p.Status = *req.Status
	}

	if err := s.store.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete 删除策略（内置模板不可删除）
func (s *PolicyService) Delete(ctx context.Context, id uuid.UUID) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPolicyNotFound
	}
	if p.PolicyType == "builtin_template" {
		return ErrTemplateCannotDelete
	}
	return s.store.Delete(ctx, id)
}

// ===================== PolicyRuleService =====================

// PolicyRuleService 封装策略规则条目的业务逻辑
type PolicyRuleService struct {
	store RuleSetStore
	ruleStore PolicyRuleStore
	policyStore PolicyStore
	logger     *slog.Logger
}

func NewPolicyRuleService(ruleStore PolicyRuleStore, policyStore PolicyStore, ruleSetStore RuleSetStore, logger *slog.Logger) *PolicyRuleService {
	return &PolicyRuleService{ruleStore: ruleStore, policyStore: policyStore, store: ruleSetStore, logger: logger}
}

// AddRule 向策略添加规则条目
func (s *PolicyRuleService) AddRule(ctx context.Context, policyID uuid.UUID, req *AddRuleRequest) (*RoutePolicyRule, error) {
	// 校验策略存在
	p, err := s.policyStore.GetByID(ctx, policyID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}

	// 校验 rule_source 与 rule_set_id 一致性
	var ruleSetID *uuid.UUID
	if req.RuleSource == "rule_set" {
		if req.RuleSetID == "" {
			return nil, ErrInvalidRuleSource
		}
		rsID, err := uuid.Parse(req.RuleSetID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid rule_set_id", ErrInvalidRuleSource)
		}
		ruleSetID = &rsID
	}

	// 校验 outbound_action 与 outbound_tag 一致性
	var outboundTag *string
	if req.OutboundAction == "tag" || req.OutboundAction == "balancer" {
		if req.OutboundTag == "" {
			return nil, ErrInvalidOutboundAction
		}
		outboundTag = &req.OutboundTag
	}

	var inlineType *string
	if req.InlineType != "" {
		inlineType = &req.InlineType
	}
	values := req.InlineValues
	if values == nil {
		values = []string{}
	}
	var notes *string
	if req.Notes != "" {
		notes = &req.Notes
	}

	// 获取当前最大 sort_order，新规则排在末尾
	existingRules, err := s.ruleStore.ListByPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	maxSort := 0
	for _, r := range existingRules {
		if r.SortOrder > maxSort {
			maxSort = r.SortOrder
		}
	}
	// 新规则 sort_order = 最大值 + 10（预留间隔）
	newSort := maxSort + 10
	if len(existingRules) == 0 {
		newSort = 10
	}

	rule := &RoutePolicyRule{
		PolicyID:       policyID,
		SortOrder:      newSort,
		RuleSource:     req.RuleSource,
		RuleSetID:      ruleSetID,
		InlineType:     inlineType,
		InlineValues:   values,
		OutboundAction: req.OutboundAction,
		OutboundTag:    outboundTag,
		Notes:          notes,
	}

	if err := s.ruleStore.Create(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// UpdateRule 更新规则条目
func (s *PolicyRuleService) UpdateRule(ctx context.Context, ruleID uuid.UUID, req *UpdateRuleRequest) (*RoutePolicyRule, error) {
	rule, err := s.ruleStore.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrPolicyRuleNotFound
	}

	if req.RuleSource != nil {
		rule.RuleSource = *req.RuleSource
	}
	if req.RuleSetID != nil {
		if *req.RuleSetID == "" {
			rule.RuleSetID = nil
		} else {
			rsID, err := uuid.Parse(*req.RuleSetID)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid rule_set_id", ErrInvalidRuleSource)
			}
			rule.RuleSetID = &rsID
		}
	}
	if req.InlineType != nil {
		rule.InlineType = req.InlineType
	}
	if req.InlineValues != nil {
		rule.InlineValues = req.InlineValues
	}
	if req.OutboundAction != nil {
		rule.OutboundAction = *req.OutboundAction
	}
	if req.OutboundTag != nil {
		if *req.OutboundTag == "" {
			rule.OutboundTag = nil
		} else {
			rule.OutboundTag = req.OutboundTag
		}
	}
	if req.Notes != nil {
		rule.Notes = req.Notes
	}

	// 校验 outbound_action 与 outbound_tag 一致性
	if (rule.OutboundAction == "tag" || rule.OutboundAction == "balancer") && rule.OutboundTag == nil {
		return nil, ErrInvalidOutboundAction
	}
	// 校验 rule_source 与 rule_set_id 一致性
	if rule.RuleSource == "rule_set" && rule.RuleSetID == nil {
		return nil, ErrInvalidRuleSource
	}

	if err := s.ruleStore.Update(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// DeleteRule 删除规则条目
func (s *PolicyRuleService) DeleteRule(ctx context.Context, ruleID uuid.UUID) error {
	rule, err := s.ruleStore.GetByID(ctx, ruleID)
	if err != nil {
		return err
	}
	if rule == nil {
		return ErrPolicyRuleNotFound
	}
	return s.ruleStore.Delete(ctx, ruleID)
}

// Reorder 批量重排序规则（按传入的 rule_ids 顺序重新分配 sort_order）
func (s *PolicyRuleService) Reorder(ctx context.Context, policyID uuid.UUID, ruleIDs []uuid.UUID) error {
	// 校验策略存在
	p, err := s.policyStore.GetByID(ctx, policyID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPolicyNotFound
	}

	for i, id := range ruleIDs {
		sortOrder := (i + 1) * 10
		if err := s.ruleStore.UpdateSortOrder(ctx, id, sortOrder); err != nil {
			return err
		}
	}
	return nil
}

// ===================== BindingService =====================

// BindingService 封装节点路由绑定的业务逻辑
type BindingService struct {
	store       BindingStore
	policyStore PolicyStore
	logger      *slog.Logger
}

func NewBindingService(store BindingStore, policyStore PolicyStore, logger *slog.Logger) *BindingService {
	return &BindingService{store: store, policyStore: policyStore, logger: logger}
}

// ListBindings 查询节点绑定的策略
func (s *BindingService) ListBindings(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error) {
	return s.store.ListByNode(ctx, nodeID)
}

// Bind 绑定策略到节点
func (s *BindingService) Bind(ctx context.Context, nodeID, policyID uuid.UUID, bindScope, inboundTag string) (*NodeRouteBinding, error) {
	// 校验策略存在
	p, err := s.policyStore.GetByID(ctx, policyID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPolicyNotFound
	}

	if bindScope == "" {
		bindScope = "all"
	}

	var tag *string
	if inboundTag != "" {
		tag = &inboundTag
	}

	b := &NodeRouteBinding{
		NodeID:     nodeID,
		PolicyID:   policyID,
		BindScope:  bindScope,
		InboundTag: tag,
	}

	if err := s.store.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// Unbind 解绑
func (s *BindingService) Unbind(ctx context.Context, nodeID, policyID uuid.UUID) error {
	existing, err := s.store.Get(ctx, nodeID, policyID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrBindingNotFound
	}
	return s.store.Delete(ctx, nodeID, policyID)
}

// ===================== LBPolicyService =====================

// LBPolicyService 封装节点组负载均衡策略的业务逻辑
type LBPolicyService struct {
	store  LBPolicyStore
	logger *slog.Logger
}

func NewLBPolicyService(store LBPolicyStore, logger *slog.Logger) *LBPolicyService {
	return &LBPolicyService{store: store, logger: logger}
}

// Get 查询节点组负载均衡策略
func (s *LBPolicyService) Get(ctx context.Context, groupID uuid.UUID) (*NodeGroupLBPolicy, error) {
	p, err := s.store.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrLBPolicyNotFound
	}
	return p, nil
}

// Upsert 创建或更新节点组负载均衡策略
func (s *LBPolicyService) Upsert(ctx context.Context, groupID uuid.UUID, req *UpsertLBPolicyRequest) (*NodeGroupLBPolicy, error) {
	lbStrategy := req.LBStrategy
	if lbStrategy == "" {
		lbStrategy = "round_robin"
	}
	weightField := req.WeightField
	if weightField == "" {
		weightField = "priority"
	}
	geoAffinity := false
	if req.GeoAffinity != nil {
		geoAffinity = *req.GeoAffinity
	}
	minScore := 30
	if req.MinScoreThreshold != nil {
		minScore = *req.MinScoreThreshold
	}

	var stickyBy *string
	if req.StickyBy != "" {
		stickyBy = &req.StickyBy
	}

	extraConfig := req.ExtraConfig
	if extraConfig == nil {
		extraConfig = Map{}
	}

	p := &NodeGroupLBPolicy{
		GroupID:                 groupID,
		LBStrategy:              lbStrategy,
		WeightField:             weightField,
		GeoAffinity:             geoAffinity,
		StickyBy:                stickyBy,
		MinScoreThreshold:       minScore,
		MaxNodesPerSubscription: req.MaxNodesPerSubscription,
		ExtraConfig:             extraConfig,
	}

	if err := s.store.Upsert(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ===================== OutboundGroupService =====================

// OutboundGroupService 封装出站策略组的业务逻辑
type OutboundGroupService struct {
	store  OutboundGroupStore
	logger *slog.Logger
}

func NewOutboundGroupService(store OutboundGroupStore, logger *slog.Logger) *OutboundGroupService {
	return &OutboundGroupService{store: store, logger: logger}
}

// List 查询节点的出站组
func (s *OutboundGroupService) List(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error) {
	return s.store.ListByNode(ctx, nodeID)
}

// Create 创建出站组
func (s *OutboundGroupService) Create(ctx context.Context, nodeID uuid.UUID, req *CreateOutboundGroupRequest) (*OutboundGroup, error) {
	existing, err := s.store.GetByNodeAndTag(ctx, nodeID, req.Tag)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrOutboundGroupDuplicate
	}

	lbStrategy := req.LBStrategy
	if lbStrategy == "" {
		lbStrategy = "leastPing"
	}
	probeURL := req.ProbeURL
	if probeURL == "" {
		probeURL = "https://www.google.com/generate_204"
	}
	probeInterval := 60
	if req.ProbeIntervalSeconds != nil {
		probeInterval = *req.ProbeIntervalSeconds
	}
	members := req.Members
	if members == nil {
		members = []Map{}
	}

	g := &OutboundGroup{
		NodeID:               nodeID,
		Tag:                  req.Tag,
		LBStrategy:           lbStrategy,
		ProbeURL:             probeURL,
		ProbeIntervalSeconds: probeInterval,
		Members:              members,
		Status:               "active",
	}

	if err := s.store.Create(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// Update 更新出站组（按 node_id + tag 定位）
func (s *OutboundGroupService) Update(ctx context.Context, nodeID uuid.UUID, tag string, req *UpdateOutboundGroupRequest) (*OutboundGroup, error) {
	g, err := s.store.GetByNodeAndTag(ctx, nodeID, tag)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrOutboundGroupNotFound
	}

	if req.LBStrategy != nil {
		g.LBStrategy = *req.LBStrategy
	}
	if req.ProbeURL != nil {
		g.ProbeURL = *req.ProbeURL
	}
	if req.ProbeIntervalSeconds != nil {
		g.ProbeIntervalSeconds = *req.ProbeIntervalSeconds
	}
	if req.Members != nil {
		g.Members = req.Members
	}
	if req.Status != nil {
		g.Status = *req.Status
	}

	if err := s.store.Update(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// Delete 删除出站组
func (s *OutboundGroupService) Delete(ctx context.Context, nodeID uuid.UUID, tag string) error {
	existing, err := s.store.GetByNodeAndTag(ctx, nodeID, tag)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrOutboundGroupNotFound
	}
	return s.store.Delete(ctx, nodeID, tag)
}
