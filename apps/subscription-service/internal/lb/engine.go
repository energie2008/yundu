package lb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"

	"github.com/google/uuid"
)

// NodeDataReader 抽象负载均衡引擎所需的数据访问（便于测试注入 fake）。
// 生产实现为 NodeDataReaderAdapter（组合 LBPolicyRepo + NodeCandidateRepo + redis）。
type NodeDataReader interface {
	// GetPolicy 读取节点组负载均衡策略；返回 nil 表示未配置（使用默认策略）
	GetPolicy(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error)
	// GetGroupNodes 读取节点组下所有候选节点（已按 sort_order / priority 排序）
	GetGroupNodes(ctx context.Context, groupID uuid.UUID) ([]NodeCandidate, error)
	// IncrCounter 用 Redis INCR 自增计数器，返回自增后的值（用于 round_robin 轮转）
	IncrCounter(ctx context.Context, key string) (int64, error)
}

// GeoResolver 将用户 IP 解析为国家代码（geoip）。
// 生产实现可接入 MaxMind/纯真库；为 nil 时 geo_affinity 退化为保持原序。
type GeoResolver interface {
	CountryCode(ip string) string
}

// LBEngine 是节点组负载均衡引擎（任务 27 主体）。
// 输入一个 LBRequest，输出按策略排序、过滤、截断后的节点列表。
type LBEngine struct {
	reader       NodeDataReader
	geo          GeoResolver
	stickySecret []byte
	logger       *slog.Logger
}

// NewLBEngine 构造负载均衡引擎。
func NewLBEngine(reader NodeDataReader, logger *slog.Logger) *LBEngine {
	return &LBEngine{
		reader:       reader,
		stickySecret: []byte("yundu-lb-sticky-default-secret"),
		logger:       logger,
	}
}

// WithGeoResolver 注入 geoip 解析器（用于 geo_affinity 策略）
func (e *LBEngine) WithGeoResolver(geo GeoResolver) *LBEngine {
	e.geo = geo
	return e
}

// WithStickySecret 注入 sticky_user 策略的 HMAC 密钥
func (e *LBEngine) WithStickySecret(secret string) *LBEngine {
	if secret != "" {
		e.stickySecret = []byte(secret)
	}
	return e
}

// SelectNodes 是负载均衡主入口：读取策略与候选 → 过滤 → 排序 → 截断。
func (e *LBEngine) SelectNodes(ctx context.Context, req *LBRequest) (*LBResult, error) {
	policy, err := e.getPolicyOrDefault(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}

	candidates, err := e.reader.GetGroupNodes(ctx, req.GroupID)
	if err != nil {
		return nil, fmt.Errorf("lb get group nodes: %w", err)
	}
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}

	// 1. min_score_threshold 过滤：低于阈值的节点不进入订阅
	kept, filteredOut := applyMinScoreThreshold(candidates, policy.MinScoreThreshold)
	if len(kept) == 0 {
		return nil, ErrNoCandidates
	}

	// 2. 按策略排序
	ordered, err := e.applyStrategy(ctx, policy, kept, req)
	if err != nil {
		return nil, err
	}

	// 3. max_nodes_per_subscription 截断（多余节点并入 FilteredOut）
	if policy.MaxNodesPerSubscription != nil {
		max := *policy.MaxNodesPerSubscription
		if max >= 0 && len(ordered) > max {
			truncated := make([]NodeCandidate, len(ordered)-max)
			copy(truncated, ordered[max:])
			ordered = ordered[:max]
			filteredOut = append(filteredOut, truncated...)
		}
	}

	return &LBResult{
		SelectedNodes: ordered,
		Strategy:      policy.LBStrategy,
		FilteredOut:   filteredOut,
	}, nil
}

// getPolicyOrDefault 读取策略；未配置时返回默认策略（round_robin，阈值 30）
func (e *LBEngine) getPolicyOrDefault(ctx context.Context, groupID uuid.UUID) (*LBPolicy, error) {
	policy, err := e.reader.GetPolicy(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("lb get policy: %w", err)
	}
	if policy != nil {
		return policy, nil
	}
	threshold := 30
	return &LBPolicy{
		GroupID:           groupID,
		LBStrategy:        StrategyRoundRobin,
		WeightField:       "priority",
		MinScoreThreshold: threshold,
	}, nil
}

// applyStrategy 按策略分派到对应实现
func (e *LBEngine) applyStrategy(ctx context.Context, policy *LBPolicy, candidates []NodeCandidate, req *LBRequest) ([]NodeCandidate, error) {
	switch policy.LBStrategy {
	case StrategyRoundRobin:
		return e.selectRoundRobin(ctx, candidates, req), nil
	case StrategyWeighted:
		return selectWeighted(candidates), nil
	case StrategyLeastConn:
		return selectLeastConn(candidates), nil
	case StrategyLatency:
		return selectLatency(candidates), nil
	case StrategyStickyUser:
		return e.selectStickyUser(candidates, req, policy)
	case StrategyGeoAffinity:
		return e.selectGeoAffinity(candidates, req)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidStrategy, policy.LBStrategy)
	}
}

// selectRoundRobin 轮询：用 Redis INCR 计数器轮转起始位置。
// 候选已按 sort_order 排序，每次请求起始位置 +1，形成轮转。
func (e *LBEngine) selectRoundRobin(ctx context.Context, candidates []NodeCandidate, req *LBRequest) []NodeCandidate {
	n := len(candidates)
	key := counterKey(req.GroupID)
	counter, err := e.reader.IncrCounter(ctx, key)
	if err != nil {
		e.logger.Warn("round_robin incr counter failed, fallback to start 0", "group_id", req.GroupID, "error", err)
		counter = 0
	}
	start := 0
	if counter > 0 {
		start = int((counter - 1) % int64(n))
	}
	result := make([]NodeCandidate, 0, n)
	for i := 0; i < n; i++ {
		result = append(result, candidates[(start+i)%n])
	}
	return result
}

// selectWeighted 加权随机：按 node.priority 加权洗牌（无放回）。
// priority 越大被选到排前的概率越高，生成完整排序。
func selectWeighted(candidates []NodeCandidate) []NodeCandidate {
	pool := make([]NodeCandidate, len(candidates))
	copy(pool, candidates)
	result := make([]NodeCandidate, 0, len(candidates))
	for len(pool) > 0 {
		total := 0
		for _, c := range pool {
			w := c.Priority
			if w < 0 {
				w = 0
			}
			total += w
		}
		idx := 0
		if total <= 0 {
			// 所有权重都为 0 时退化为均匀随机
			idx = rand.IntN(len(pool))
		} else {
			pick := rand.IntN(total)
			acc := 0
			for i, c := range pool {
				w := c.Priority
				if w < 0 {
					w = 0
				}
				acc += w
				if pick < acc {
					idx = i
					break
				}
			}
		}
		result = append(result, pool[idx])
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return result
}

// selectLeastConn 最少连接：按 current_online_users 升序排序（相同时保持原序）。
func selectLeastConn(candidates []NodeCandidate) []NodeCandidate {
	out := make([]NodeCandidate, len(candidates))
	copy(out, candidates)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].OnlineUsers < out[j].OnlineUsers
	})
	return out
}

// selectLatency 延迟优先：按 current_rtt_ms 升序排序。
// RTT <= 0 视为未知，排到最后；相同时保持原序。
func selectLatency(candidates []NodeCandidate) []NodeCandidate {
	out := make([]NodeCandidate, len(candidates))
	copy(out, candidates)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].RTTms, out[j].RTTms
		// 未知（<=0）排最后
		if a <= 0 && b <= 0 {
			return false
		}
		if a <= 0 {
			return false
		}
		if b <= 0 {
			return true
		}
		return a < b
	})
	return out
}

// selectStickyUser 粘性用户：HMAC(user_id + group_id) mod node_count 取固定节点排首位。
// 同一 (user, group) 每次拿到相同首节点，其余按原序轮转拼接。
func (e *LBEngine) selectStickyUser(candidates []NodeCandidate, req *LBRequest, policy *LBPolicy) ([]NodeCandidate, error) {
	identity := req.UserID
	if policy.StickyBy != nil && *policy.StickyBy == StickyBySubscriptionToken {
		identity = req.SubscriptionToken
	}
	if identity == "" {
		return nil, ErrStickyMissingUserID
	}

	mac := hmac.New(sha256.New, e.stickySecret)
	mac.Write([]byte(identity))
	mac.Write([]byte(req.GroupID.String()))
	sum := mac.Sum(nil)
	// 取前 8 字节作为无符号整数，mod 节点数得到固定索引
	idx := int(binary.BigEndian.Uint64(sum[:8]) % uint64(len(candidates)))

	result := make([]NodeCandidate, 0, len(candidates))
	result = append(result, candidates[idx])
	result = append(result, candidates[idx+1:]...)
	result = append(result, candidates[:idx]...)
	return result, nil
}

// selectGeoAffinity 地域亲和：按用户 IP 归属地匹配 region.country_code 优先排序。
// 解析不到国家代码时保持原序；匹配的节点排前，其余按原序跟在后。
func (e *LBEngine) selectGeoAffinity(candidates []NodeCandidate, req *LBRequest) ([]NodeCandidate, error) {
	if req.UserIP == "" {
		return nil, ErrGeoMissingUserIP
	}
	country := ""
	if e.geo != nil {
		country = e.geo.CountryCode(req.UserIP)
	}
	if country == "" {
		// 无法解析 geoip 时保持原序（不阻塞订阅生成）
		return candidates, nil
	}
	matched := make([]NodeCandidate, 0, len(candidates))
	others := make([]NodeCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.RegionCountryCode == country {
			matched = append(matched, c)
		} else {
			others = append(others, c)
		}
	}
	return append(matched, others...), nil
}

// applyMinScoreThreshold 过滤 HealthScore 低于阈值的节点。
// 返回 (通过, 被过滤)；被过滤的保留原相对顺序。
func applyMinScoreThreshold(candidates []NodeCandidate, threshold int) (kept, filtered []NodeCandidate) {
	kept = make([]NodeCandidate, 0, len(candidates))
	filtered = make([]NodeCandidate, 0)
	for _, c := range candidates {
		if c.HealthScore < threshold {
			filtered = append(filtered, c)
		} else {
			kept = append(kept, c)
		}
	}
	return kept, filtered
}
