package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ===================== RouteRuleSetRepo =====================

// RouteRuleSetRepo 处理 route_rule_sets 数据访问
type RouteRuleSetRepo struct {
	pool *pgxpool.Pool
}

func NewRouteRuleSetRepo(pool *pgxpool.Pool) *RouteRuleSetRepo {
	return &RouteRuleSetRepo{pool: pool}
}

const ruleSetColumns = `id, code, name, description, rule_type, source_type, source_url, content, auto_update, last_synced_at, status, created_at, updated_at`

func scanRuleSet(row pgx.Row, r *RouteRuleSet) error {
	return row.Scan(
		&r.ID, &r.Code, &r.Name, &r.Description, &r.RuleType, &r.SourceType,
		&r.SourceURL, &r.Content, &r.AutoUpdate, &r.LastSyncedAt, &r.Status, &r.CreatedAt, &r.UpdatedAt,
	)
}

func (r *RouteRuleSetRepo) Create(ctx context.Context, rs *RouteRuleSet) error {
	query := `
		INSERT INTO route_rule_sets (code, name, description, rule_type, source_type, source_url, content, auto_update, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		rs.Code, rs.Name, rs.Description, rs.RuleType, rs.SourceType, rs.SourceURL,
		rs.Content, rs.AutoUpdate, rs.Status,
	).Scan(&rs.ID, &rs.CreatedAt, &rs.UpdatedAt)
}

func (r *RouteRuleSetRepo) GetByID(ctx context.Context, id uuid.UUID) (*RouteRuleSet, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_rule_sets WHERE id = $1`, ruleSetColumns)
	rs := &RouteRuleSet{}
	if err := scanRuleSet(r.pool.QueryRow(ctx, query, id), rs); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rs, nil
}

func (r *RouteRuleSetRepo) GetByCode(ctx context.Context, code string) (*RouteRuleSet, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_rule_sets WHERE code = $1`, ruleSetColumns)
	rs := &RouteRuleSet{}
	if err := scanRuleSet(r.pool.QueryRow(ctx, query, code), rs); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rs, nil
}

func (r *RouteRuleSetRepo) Update(ctx context.Context, rs *RouteRuleSet) error {
	query := `
		UPDATE route_rule_sets SET
			name = $2, description = $3, content = $4, auto_update = $5, status = $6, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		rs.ID, rs.Name, rs.Description, rs.Content, rs.AutoUpdate, rs.Status,
	)
	return err
}

// UpdateSynced 更新远程规则集同步后的 content 和 last_synced_at
func (r *RouteRuleSetRepo) UpdateSynced(ctx context.Context, id uuid.UUID, content []string) error {
	query := `UPDATE route_rule_sets SET content = $2, last_synced_at = now(), updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, content)
	return err
}

func (r *RouteRuleSetRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM route_rule_sets WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *RouteRuleSetRepo) List(ctx context.Context, page, pageSize int, q RuleSetListQuery) ([]*RouteRuleSet, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if q.RuleType != "" {
		where = append(where, fmt.Sprintf("rule_type = $%d", argIdx))
		args = append(args, q.RuleType)
		argIdx++
	}
	if q.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Keyword != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q.Keyword+"%")
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM route_rule_sets WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM route_rule_sets WHERE %s ORDER BY created_at ASC LIMIT $%d OFFSET $%d`,
		ruleSetColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*RouteRuleSet
	for rows.Next() {
		rs := &RouteRuleSet{}
		if err := scanRuleSet(rows, rs); err != nil {
			return nil, 0, err
		}
		items = append(items, rs)
	}
	return items, total, rows.Err()
}

// ===================== RoutePolicyRepo =====================

// RoutePolicyRepo 处理 route_policies 数据访问
type RoutePolicyRepo struct {
	pool *pgxpool.Pool
}

func NewRoutePolicyRepo(pool *pgxpool.Pool) *RoutePolicyRepo {
	return &RoutePolicyRepo{pool: pool}
}

const policyColumns = `id, code, name, description, policy_type, base_template_code, status, created_at, updated_at`

func scanPolicy(row pgx.Row, p *RoutePolicy) error {
	return row.Scan(
		&p.ID, &p.Code, &p.Name, &p.Description, &p.PolicyType, &p.BaseTemplateCode,
		&p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *RoutePolicyRepo) Create(ctx context.Context, p *RoutePolicy) error {
	query := `
		INSERT INTO route_policies (code, name, description, policy_type, base_template_code, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.Code, p.Name, p.Description, p.PolicyType, p.BaseTemplateCode, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *RoutePolicyRepo) GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_policies WHERE id = $1`, policyColumns)
	p := &RoutePolicy{}
	if err := scanPolicy(r.pool.QueryRow(ctx, query, id), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *RoutePolicyRepo) GetByCode(ctx context.Context, code string) (*RoutePolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_policies WHERE code = $1`, policyColumns)
	p := &RoutePolicy{}
	if err := scanPolicy(r.pool.QueryRow(ctx, query, code), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *RoutePolicyRepo) Update(ctx context.Context, p *RoutePolicy) error {
	query := `
		UPDATE route_policies SET
			name = $2, description = $3, status = $4, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, p.ID, p.Name, p.Description, p.Status)
	return err
}

func (r *RoutePolicyRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM route_policies WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *RoutePolicyRepo) List(ctx context.Context, page, pageSize int, q PolicyListQuery) ([]*RoutePolicy, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if q.PolicyType != "" {
		where = append(where, fmt.Sprintf("policy_type = $%d", argIdx))
		args = append(args, q.PolicyType)
		argIdx++
	}
	if q.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Keyword != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q.Keyword+"%")
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM route_policies WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM route_policies WHERE %s ORDER BY created_at ASC LIMIT $%d OFFSET $%d`,
		policyColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*RoutePolicy
	for rows.Next() {
		p := &RoutePolicy{}
		if err := scanPolicy(rows, p); err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	return items, total, rows.Err()
}

// ===================== RoutePolicyRuleRepo =====================

// RoutePolicyRuleRepo 处理 route_policy_rules 数据访问
type RoutePolicyRuleRepo struct {
	pool *pgxpool.Pool
}

func NewRoutePolicyRuleRepo(pool *pgxpool.Pool) *RoutePolicyRuleRepo {
	return &RoutePolicyRuleRepo{pool: pool}
}

const policyRuleColumns = `id, policy_id, sort_order, rule_source, rule_set_id, inline_type, inline_values, outbound_action, outbound_tag, notes, created_at`

func scanPolicyRule(row pgx.Row, r *RoutePolicyRule) error {
	return row.Scan(
		&r.ID, &r.PolicyID, &r.SortOrder, &r.RuleSource, &r.RuleSetID,
		&r.InlineType, &r.InlineValues, &r.OutboundAction, &r.OutboundTag, &r.Notes, &r.CreatedAt,
	)
}

func (r *RoutePolicyRuleRepo) Create(ctx context.Context, rule *RoutePolicyRule) error {
	query := `
		INSERT INTO route_policy_rules (policy_id, sort_order, rule_source, rule_set_id, inline_type, inline_values, outbound_action, outbound_tag, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, query,
		rule.PolicyID, rule.SortOrder, rule.RuleSource, rule.RuleSetID,
		rule.InlineType, rule.InlineValues, rule.OutboundAction, rule.OutboundTag, rule.Notes,
	).Scan(&rule.ID, &rule.CreatedAt)
}

func (r *RoutePolicyRuleRepo) GetByID(ctx context.Context, id uuid.UUID) (*RoutePolicyRule, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_policy_rules WHERE id = $1`, policyRuleColumns)
	rule := &RoutePolicyRule{}
	if err := scanPolicyRule(r.pool.QueryRow(ctx, query, id), rule); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rule, nil
}

func (r *RoutePolicyRuleRepo) Update(ctx context.Context, rule *RoutePolicyRule) error {
	query := `
		UPDATE route_policy_rules SET
			rule_source = $2, rule_set_id = $3, inline_type = $4, inline_values = $5,
			outbound_action = $6, outbound_tag = $7, notes = $8
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		rule.ID, rule.RuleSource, rule.RuleSetID, rule.InlineType, rule.InlineValues,
		rule.OutboundAction, rule.OutboundTag, rule.Notes,
	)
	return err
}

func (r *RoutePolicyRuleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM route_policy_rules WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// ListByPolicy 按策略 ID 查询规则（按 sort_order 升序）
func (r *RoutePolicyRuleRepo) ListByPolicy(ctx context.Context, policyID uuid.UUID) ([]*RoutePolicyRule, error) {
	query := fmt.Sprintf(`SELECT %s FROM route_policy_rules WHERE policy_id = $1 ORDER BY sort_order ASC`, policyRuleColumns)
	rows, err := r.pool.Query(ctx, query, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*RoutePolicyRule
	for rows.Next() {
		rule := &RoutePolicyRule{}
		if err := scanPolicyRule(rows, rule); err != nil {
			return nil, err
		}
		items = append(items, rule)
	}
	return items, rows.Err()
}

// UpdateSortOrder 更新单条规则的排序值
func (r *RoutePolicyRuleRepo) UpdateSortOrder(ctx context.Context, id uuid.UUID, sortOrder int) error {
	query := `UPDATE route_policy_rules SET sort_order = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, sortOrder)
	return err
}

// ===================== NodeRouteBindingRepo =====================

// NodeRouteBindingRepo 处理 node_route_bindings 数据访问
type NodeRouteBindingRepo struct {
	pool *pgxpool.Pool
}

func NewNodeRouteBindingRepo(pool *pgxpool.Pool) *NodeRouteBindingRepo {
	return &NodeRouteBindingRepo{pool: pool}
}

const bindingColumns = `node_id, policy_id, bind_scope, inbound_tag, created_at`

func scanBinding(row pgx.Row, b *NodeRouteBinding) error {
	return row.Scan(&b.NodeID, &b.PolicyID, &b.BindScope, &b.InboundTag, &b.CreatedAt)
}

func (r *NodeRouteBindingRepo) Create(ctx context.Context, b *NodeRouteBinding) error {
	query := `
		INSERT INTO node_route_bindings (node_id, policy_id, bind_scope, inbound_tag)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id, policy_id) DO NOTHING
		RETURNING created_at`
	err := r.pool.QueryRow(ctx, query, b.NodeID, b.PolicyID, b.BindScope, b.InboundTag).Scan(&b.CreatedAt)
	if err == pgx.ErrNoRows {
		// 已存在（ON CONFLICT DO NOTHING），返回 nil 表示幂等成功
		return nil
	}
	return err
}

func (r *NodeRouteBindingRepo) Delete(ctx context.Context, nodeID, policyID uuid.UUID) error {
	query := `DELETE FROM node_route_bindings WHERE node_id = $1 AND policy_id = $2`
	_, err := r.pool.Exec(ctx, query, nodeID, policyID)
	return err
}

// ListByNode 按节点 ID 查询绑定
func (r *NodeRouteBindingRepo) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeRouteBinding, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_route_bindings WHERE node_id = $1 ORDER BY created_at ASC`, bindingColumns)
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*NodeRouteBinding
	for rows.Next() {
		b := &NodeRouteBinding{}
		if err := scanBinding(rows, b); err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

// Get 获取单条绑定（用于检查是否存在）
func (r *NodeRouteBindingRepo) Get(ctx context.Context, nodeID, policyID uuid.UUID) (*NodeRouteBinding, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_route_bindings WHERE node_id = $1 AND policy_id = $2`, bindingColumns)
	b := &NodeRouteBinding{}
	if err := scanBinding(r.pool.QueryRow(ctx, query, nodeID, policyID), b); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// ===================== NodeGroupLBPolicyRepo =====================

// NodeGroupLBPolicyRepo 处理 node_group_lb_policies 数据访问
type NodeGroupLBPolicyRepo struct {
	pool *pgxpool.Pool
}

func NewNodeGroupLBPolicyRepo(pool *pgxpool.Pool) *NodeGroupLBPolicyRepo {
	return &NodeGroupLBPolicyRepo{pool: pool}
}

const lbPolicyColumns = `id, group_id, lb_strategy, weight_field, geo_affinity, sticky_by, min_score_threshold, max_nodes_per_subscription, extra_config, created_at, updated_at`

func scanLBPolicy(row pgx.Row, p *NodeGroupLBPolicy) error {
	return row.Scan(
		&p.ID, &p.GroupID, &p.LBStrategy, &p.WeightField, &p.GeoAffinity,
		&p.StickyBy, &p.MinScoreThreshold, &p.MaxNodesPerSubscription,
		&p.ExtraConfig, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *NodeGroupLBPolicyRepo) GetByGroupID(ctx context.Context, groupID uuid.UUID) (*NodeGroupLBPolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_group_lb_policies WHERE group_id = $1`, lbPolicyColumns)
	p := &NodeGroupLBPolicy{}
	if err := scanLBPolicy(r.pool.QueryRow(ctx, query, groupID), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// Upsert 创建或更新节点组负载均衡策略（group_id 唯一）
func (r *NodeGroupLBPolicyRepo) Upsert(ctx context.Context, p *NodeGroupLBPolicy) error {
	query := `
		INSERT INTO node_group_lb_policies (group_id, lb_strategy, weight_field, geo_affinity, sticky_by, min_score_threshold, max_nodes_per_subscription, extra_config)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (group_id) DO UPDATE SET
			lb_strategy = EXCLUDED.lb_strategy,
			weight_field = EXCLUDED.weight_field,
			geo_affinity = EXCLUDED.geo_affinity,
			sticky_by = EXCLUDED.sticky_by,
			min_score_threshold = EXCLUDED.min_score_threshold,
			max_nodes_per_subscription = EXCLUDED.max_nodes_per_subscription,
			extra_config = EXCLUDED.extra_config,
			updated_at = now()
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.GroupID, p.LBStrategy, p.WeightField, p.GeoAffinity,
		p.StickyBy, p.MinScoreThreshold, p.MaxNodesPerSubscription, p.ExtraConfig,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// ===================== OutboundGroupRepo =====================

// OutboundGroupRepo 处理 outbound_groups 数据访问
type OutboundGroupRepo struct {
	pool *pgxpool.Pool
}

func NewOutboundGroupRepo(pool *pgxpool.Pool) *OutboundGroupRepo {
	return &OutboundGroupRepo{pool: pool}
}

const outboundGroupColumns = `id, node_id, tag, lb_strategy, probe_url, probe_interval_seconds, members, status, created_at, updated_at`

func scanOutboundGroup(row pgx.Row, g *OutboundGroup) error {
	return row.Scan(
		&g.ID, &g.NodeID, &g.Tag, &g.LBStrategy, &g.ProbeURL,
		&g.ProbeIntervalSeconds, &g.Members, &g.Status, &g.CreatedAt, &g.UpdatedAt,
	)
}

func (r *OutboundGroupRepo) Create(ctx context.Context, g *OutboundGroup) error {
	query := `
		INSERT INTO outbound_groups (node_id, tag, lb_strategy, probe_url, probe_interval_seconds, members, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		g.NodeID, g.Tag, g.LBStrategy, g.ProbeURL, g.ProbeIntervalSeconds, g.Members, g.Status,
	).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func (r *OutboundGroupRepo) GetByNodeAndTag(ctx context.Context, nodeID uuid.UUID, tag string) (*OutboundGroup, error) {
	query := fmt.Sprintf(`SELECT %s FROM outbound_groups WHERE node_id = $1 AND tag = $2`, outboundGroupColumns)
	g := &OutboundGroup{}
	if err := scanOutboundGroup(r.pool.QueryRow(ctx, query, nodeID, tag), g); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *OutboundGroupRepo) Update(ctx context.Context, g *OutboundGroup) error {
	query := `
		UPDATE outbound_groups SET
			lb_strategy = $3, probe_url = $4, probe_interval_seconds = $5, members = $6, status = $7, updated_at = now()
		WHERE node_id = $1 AND tag = $2`
	_, err := r.pool.Exec(ctx, query,
		g.NodeID, g.Tag, g.LBStrategy, g.ProbeURL, g.ProbeIntervalSeconds, g.Members, g.Status,
	)
	return err
}

func (r *OutboundGroupRepo) Delete(ctx context.Context, nodeID uuid.UUID, tag string) error {
	query := `DELETE FROM outbound_groups WHERE node_id = $1 AND tag = $2`
	_, err := r.pool.Exec(ctx, query, nodeID, tag)
	return err
}

// ListByNode 按节点 ID 查询出站组
func (r *OutboundGroupRepo) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundGroup, error) {
	query := fmt.Sprintf(`SELECT %s FROM outbound_groups WHERE node_id = $1 ORDER BY created_at ASC`, outboundGroupColumns)
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*OutboundGroup
	for rows.Next() {
		g := &OutboundGroup{}
		if err := scanOutboundGroup(rows, g); err != nil {
			return nil, err
		}
		items = append(items, g)
	}
	return items, rows.Err()
}
