package experience

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// 领域模型（对应迁移 000027 中的 node_experience_* 表）
// ============================================================================

// Score 节点体验评分（历史快照）
type Score struct {
	ID                      int64      `json:"id"`
	NodeID                  uuid.UUID  `json:"node_id"`
	OverallScore            float64    `json:"overall_score"` // 0~100
	LatencyScore            float64    `json:"latency_score"`
	StabilityScore          float64    `json:"stability_score"`
	SpeedScore              float64    `json:"speed_score"`
	SuccessRateScore        float64    `json:"success_rate_score"`
	P50LatencyMs            *float64   `json:"p50_latency_ms,omitempty"`
	P95LatencyMs            *float64   `json:"p95_latency_ms,omitempty"`
	P99LatencyMs            *float64   `json:"p99_latency_ms,omitempty"`
	HeartbeatSuccessRate    *float64   `json:"heartbeat_success_rate,omitempty"`
	ChannelFailoverCount24h *int       `json:"channel_failover_count_24h,omitempty"`
	MeasuredBandwidthMbps   *float64   `json:"measured_bandwidth_mbps,omitempty"`
	ConnectionSuccessRate   *float64   `json:"connection_success_rate,omitempty"`
	Grade                   string     `json:"grade"`     // excellent/good/fair/poor/critical/unknown
	Isolated                bool       `json:"isolated"`
	CalculatedAt            time.Time  `json:"calculated_at"`
}

// Current 当前节点体验分（每节点一条）
type Current struct {
	NodeID                  uuid.UUID  `json:"node_id"`
	OverallScore            float64    `json:"overall_score"`
	LatencyScore            float64    `json:"latency_score"`
	StabilityScore          float64    `json:"stability_score"`
	SpeedScore              float64    `json:"speed_score"`
	SuccessRateScore        float64    `json:"success_rate_score"`
	P50LatencyMs            *float64   `json:"p50_latency_ms,omitempty"`
	P95LatencyMs            *float64   `json:"p95_latency_ms,omitempty"`
	P99LatencyMs            *float64   `json:"p99_latency_ms,omitempty"`
	HeartbeatSuccessRate    *float64   `json:"heartbeat_success_rate,omitempty"`
	ChannelFailoverCount24h *int       `json:"channel_failover_count_24h,omitempty"`
	MeasuredBandwidthMbps   *float64   `json:"measured_bandwidth_mbps,omitempty"`
	ConnectionSuccessRate   *float64   `json:"connection_success_rate,omitempty"`
	Grade                   string     `json:"grade"`
	Isolated                bool       `json:"isolated"`
	CalculatedAt            time.Time  `json:"calculated_at"`
}

// Config 评分配置（单行表）
type Config struct {
	WeightLatency       float64 `json:"weight_latency"`        // 默认 0.30
	WeightStability     float64 `json:"weight_stability"`      // 默认 0.25
	WeightSpeed         float64 `json:"weight_speed"`          // 默认 0.25
	WeightSuccessRate   float64 `json:"weight_success_rate"`   // 默认 0.20
	ExcellentThreshold  float64 `json:"excellent_threshold"`   // 默认 85
	GoodThreshold       float64 `json:"good_threshold"`        // 默认 70
	FairThreshold       float64 `json:"fair_threshold"`        // 默认 60
	PoorThreshold       float64 `json:"poor_threshold"`       // 默认 40
	IsolateThreshold    float64 `json:"isolate_threshold"`    // 默认 30
	CalcIntervalSeconds int     `json:"calc_interval_seconds"` // 默认 300
	ProbeIntervalSeconds int    `json:"probe_interval_seconds"` // 默认 60
	AutoIsolateEnabled  bool    `json:"auto_isolate_enabled"`  // 默认 true
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		WeightLatency:       0.30,
		WeightStability:     0.25,
		WeightSpeed:         0.25,
		WeightSuccessRate:   0.20,
		ExcellentThreshold:  85,
		GoodThreshold:       70,
		FairThreshold:       60,
		PoorThreshold:       40,
		IsolateThreshold:    30,
		CalcIntervalSeconds: 300,
		ProbeIntervalSeconds: 60,
		AutoIsolateEnabled:  true,
	}
}

// ============================================================================
// 输入指标（从 health_service / channelhealth / traffic_service 收集）
// ============================================================================

// NodeMetrics 单节点的原始指标
type NodeMetrics struct {
	NodeID                  uuid.UUID
	P50LatencyMs            *float64
	P95LatencyMs            *float64
	P99LatencyMs            *float64
	HeartbeatSuccessRate    *float64  // 0.0~1.0
	ChannelFailoverCount24h *int
	MeasuredBandwidthMbps   *float64
	ConnectionSuccessRate   *float64  // 0.0~1.0
}

// ============================================================================
// DTO
// ============================================================================

type ScoreListQuery struct {
	Page      int        `form:"page"`
	PageSize  int        `form:"page_size"`
	NodeID    *uuid.UUID `form:"node_id,omitempty"`
	Grade     string     `form:"grade,omitempty"`
	OnlyIsolated bool    `form:"only_isolated,omitempty"`
}

// UpdateConfigRequest 更新评分配置
type UpdateConfigRequest struct {
	WeightLatency       *float64 `json:"weight_latency,omitempty"`
	WeightStability     *float64 `json:"weight_stability,omitempty"`
	WeightSpeed         *float64 `json:"weight_speed,omitempty"`
	WeightSuccessRate   *float64 `json:"weight_success_rate,omitempty"`
	ExcellentThreshold  *float64 `json:"excellent_threshold,omitempty"`
	GoodThreshold       *float64 `json:"good_threshold,omitempty"`
	FairThreshold       *float64 `json:"fair_threshold,omitempty"`
	PoorThreshold       *float64 `json:"poor_threshold,omitempty"`
	IsolateThreshold    *float64 `json:"isolate_threshold,omitempty"`
	CalcIntervalSeconds *int     `json:"calc_interval_seconds,omitempty"`
	ProbeIntervalSeconds *int    `json:"probe_interval_seconds,omitempty"`
	AutoIsolateEnabled  *bool    `json:"auto_isolate_enabled,omitempty"`
}
