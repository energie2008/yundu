package lb

import (
	"time"

	"github.com/google/uuid"
)

// Map 是 JSONB 字段的通用类型（map[string]interface{} 的别名）
type Map = map[string]interface{}

// 支持的负载均衡策略常量
const (
	StrategyRoundRobin  = "round_robin" // 轮询
	StrategyWeighted    = "weighted"    // 加权（按 node.priority）
	StrategyLeastConn   = "least_conn"  // 最少在线连接数优先
	StrategyLatency     = "latency"      // 延迟最低优先
	StrategyStickyUser  = "sticky_user"  // 同一用户固定节点（HMAC 哈希）
	StrategyGeoAffinity = "geo_affinity" // 按用户 IP 归属地匹配
)

// StickyBy 取值常量
const (
	StickyByUserID             = "user_id"
	StickyBySubscriptionToken  = "subscription_token"
)

// LBPolicy 对应 node_group_lb_policies 表，节点组负载均衡策略
type LBPolicy struct {
	ID                      uuid.UUID `json:"id"`
	GroupID                 uuid.UUID `json:"group_id"`
	LBStrategy              string    `json:"lb_strategy"` // round_robin/weighted/least_conn/latency/sticky_user/geo_affinity
	WeightField             string    `json:"weight_field"`
	GeoAffinity             bool      `json:"geo_affinity"`
	StickyBy                *string   `json:"sticky_by,omitempty"` // user_id / subscription_token / null
	MinScoreThreshold       int       `json:"min_score_threshold"`
	MaxNodesPerSubscription *int      `json:"max_nodes_per_subscription,omitempty"`
	ExtraConfig             Map       `json:"extra_config"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// NodeCandidate 是参与负载均衡的节点候选。
// 字段来自 nodes + node_groups + node_health_status + regions 联查。
type NodeCandidate struct {
	NodeID            uuid.UUID `json:"node_id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	GroupID           uuid.UUID `json:"group_id"`
	Priority          int       `json:"priority"`            // nodes.priority，越大权重越高
	SortOrder         int       `json:"sort_order"`          // node_groups.sort_order，轮询基准
	RTTms             int       `json:"rtt_ms"`              // node_health_status.current_rtt_ms（<=0 视为未知，排序时靠后）
	OnlineUsers       int       `json:"online_users"`        // node_health_status.current_online_users
	HealthScore       int       `json:"health_score"`        // 节点综合得分 0-100
	Capacity          int       `json:"capacity"`            // nodes.capacity_score
	RegionCountryCode string    `json:"region_country_code"` // regions.country_code
}

// LBRequest 是一次负载均衡请求
type LBRequest struct {
	GroupID            uuid.UUID
	UserID             string // sticky_user 时用于 HMAC
	UserIP             string // geo_affinity 时用于 geoip 匹配
	SubscriptionToken  string // sticky_by=subscription_token 时用于 HMAC
}

// LBResult 是负载均衡结果
type LBResult struct {
	SelectedNodes []NodeCandidate `json:"selected_nodes"` // 排序后的节点列表
	Strategy      string          `json:"strategy"`       // 实际使用的策略
	FilteredOut   []NodeCandidate `json:"filtered_out"`    // 被过滤掉的节点（低于阈值等）
}
