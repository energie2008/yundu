package lb

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

// ===================== LBPolicyRepo =====================

// LBPolicyRepo 处理 node_group_lb_policies 数据访问（与 node-service routing 包同表）
type LBPolicyRepo struct {
	pool *pgxpool.Pool
}

func NewLBPolicyRepo(pool *pgxpool.Pool) *LBPolicyRepo {
	return &LBPolicyRepo{pool: pool}
}

const lbPolicyColumns = `id, group_id, lb_strategy, weight_field, geo_affinity, sticky_by, min_score_threshold, max_nodes_per_subscription, extra_config, created_at, updated_at`

func scanLBPolicy(row pgx.Row, p *LBPolicy) error {
	return row.Scan(
		&p.ID, &p.GroupID, &p.LBStrategy, &p.WeightField, &p.GeoAffinity,
		&p.StickyBy, &p.MinScoreThreshold, &p.MaxNodesPerSubscription,
		&p.ExtraConfig, &p.CreatedAt, &p.UpdatedAt,
	)
}

// GetByGroupID 按节点组 ID 查询负载均衡策略；不存在返回 (nil, nil)
func (r *LBPolicyRepo) GetByGroupID(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_group_lb_policies WHERE group_id = $1`, lbPolicyColumns)
	p := &LBPolicy{}
	if err := scanLBPolicy(r.pool.QueryRow(ctx, query, groupID), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// Upsert 创建或更新节点组负载均衡策略（group_id 唯一）
func (r *LBPolicyRepo) Upsert(ctx context.Context, p *LBPolicy) error {
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

// ===================== NodeCandidateRepo =====================

// NodeCandidateRepo 查询节点候选列表（联表 nodes + node_groups + node_health_status + regions）
type NodeCandidateRepo struct {
	pool *pgxpool.Pool
}

func NewNodeCandidateRepo(pool *pgxpool.Pool) *NodeCandidateRepo {
	return &NodeCandidateRepo{pool: pool}
}

// nodeCandidateColumns 与 NodeCandidate 字段一一对应。
// health_score 在 SQL 内按设计文档权重计算：可用性0.30 + 延迟0.25 + 丢包0.20 + 握手0.15 + 稳定性0.10。
// RTT 为 NULL 时用 0 表示，由调用方按 <=0 视为未知。
const nodeCandidateColumns = `
	n.id, n.code, n.name, n.group_id, n.priority, COALESCE(ng.sort_order, 0),
	COALESCE(h.current_rtt_ms, 0), COALESCE(h.current_online_users, 0),
	(COALESCE(h.availability_score, 0)*0.30 + COALESCE(h.latency_score, 0)*0.25
		+ COALESCE(h.loss_score, 0)*0.20 + COALESCE(h.handshake_score, 0)*0.15
		+ COALESCE(h.stability_score, 0)*0.10)::int AS health_score,
	COALESCE(n.capacity_score, 0), COALESCE(r.country_code, '')
`

func scanNodeCandidate(row pgx.Row, c *NodeCandidate) error {
	return row.Scan(
		&c.NodeID, &c.Code, &c.Name, &c.GroupID, &c.Priority, &c.SortOrder,
		&c.RTTms, &c.OnlineUsers, &c.HealthScore, &c.Capacity, &c.RegionCountryCode,
	)
}

// ListByGroupID 查询节点组下所有启用且可见的节点候选，按 sort_order、priority 排序。
func (r *NodeCandidateRepo) ListByGroupID(ctx context.Context, groupID uuid.UUID) ([]NodeCandidate, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM nodes n
		LEFT JOIN node_groups ng ON ng.id = n.group_id
		LEFT JOIN node_health_status h ON h.node_id = n.id
		LEFT JOIN regions r ON r.id = n.region_id
		WHERE n.group_id = $1 AND n.is_enabled = true AND n.is_visible = true AND n.deleted_at IS NULL
		ORDER BY ng.sort_order ASC, n.priority DESC, n.created_at ASC`, nodeCandidateColumns)

	rows, err := r.pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NodeCandidate
	for rows.Next() {
		var c NodeCandidate
		if err := scanNodeCandidate(rows, &c); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// ===================== NodeDataReaderAdapter =====================

// NodeDataReaderAdapter 组合 LBPolicyRepo + NodeCandidateRepo + redis client，
// 实现 engine.NodeDataReader 接口，便于在生产环境注入真实依赖。
type NodeDataReaderAdapter struct {
	PolicyRepo *LBPolicyRepo
	NodeRepo   *NodeCandidateRepo
	Redis      *goredis.Client
}

// GetPolicy 读取节点组负载均衡策略
func (a *NodeDataReaderAdapter) GetPolicy(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error) {
	return a.PolicyRepo.GetByGroupID(ctx, groupID)
}

// GetGroupNodes 读取节点组下所有候选节点（已按 sort_order / priority 排序）
func (a *NodeDataReaderAdapter) GetGroupNodes(ctx context.Context, groupID uuid.UUID) ([]NodeCandidate, error) {
	return a.NodeRepo.ListByGroupID(ctx, groupID)
}

// IncrCounter 用 Redis INCR 自增计数器，并刷新 24h 过期时间。
// 用于 round_robin 策略轮转起始位置。
func (a *NodeDataReaderAdapter) IncrCounter(ctx context.Context, key string) (int64, error) {
	if a.Redis == nil {
		// 无 redis 时退化为本地自增（保证流程不中断，仅供开发/测试）
		return localIncr(key), nil
	}
	pipe := a.Redis.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("redis incr %s: %w", key, err)
	}
	return incr.Val(), nil
}

// counterKey 构造 round_robin 计数器 redis key：lb:rr:{group_id}
func counterKey(groupID uuid.UUID) string {
	return fmt.Sprintf("lb:rr:%s", groupID.String())
}

// localIncr 是 redis 缺失时的进程内自增兜底（非线程安全，仅用于开发兜底，不应在生产使用）
var localCounters = make(map[string]int64)

func localIncr(key string) int64 {
	localCounters[key]++
	return localCounters[key]
}
