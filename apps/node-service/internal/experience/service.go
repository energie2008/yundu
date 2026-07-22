package experience

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Service - 节点体验评分引擎
// ============================================================================

// MetricsProvider 指标提供者（从 health_service / channelhealth / traffic_service 收集）
type MetricsProvider interface {
	CollectAllNodeMetrics(ctx context.Context) ([]NodeMetrics, error)
}

type Service struct {
	repo     *Repo
	provider MetricsProvider
	logger   *slog.Logger
	cfg      *Config
}

func NewService(repo *Repo, provider MetricsProvider, logger *slog.Logger) *Service {
	return &Service{
		repo:     repo,
		provider: provider,
		logger:   logger.With("component", "experience"),
		cfg:      DefaultConfig(),
	}
}

// SetMetricsProvider 运行时注入指标提供者
func (s *Service) SetMetricsProvider(p MetricsProvider) { s.provider = p }

// LoadConfig 从数据库加载配置
func (s *Service) LoadConfig(ctx context.Context) error {
	cfg, err := s.repo.GetConfig(ctx)
	if err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

// GetConfig 获取当前配置
func (s *Service) GetConfig(ctx context.Context) (*Config, error) {
	return s.repo.GetConfig(ctx)
}

// UpdateConfig 更新配置
func (s *Service) UpdateConfig(ctx context.Context, req *UpdateConfigRequest) (*Config, error) {
	cfg := s.cfg
	if req.WeightLatency != nil {
		cfg.WeightLatency = *req.WeightLatency
	}
	if req.WeightStability != nil {
		cfg.WeightStability = *req.WeightStability
	}
	if req.WeightSpeed != nil {
		cfg.WeightSpeed = *req.WeightSpeed
	}
	if req.WeightSuccessRate != nil {
		cfg.WeightSuccessRate = *req.WeightSuccessRate
	}
	if req.ExcellentThreshold != nil {
		cfg.ExcellentThreshold = *req.ExcellentThreshold
	}
	if req.GoodThreshold != nil {
		cfg.GoodThreshold = *req.GoodThreshold
	}
	if req.FairThreshold != nil {
		cfg.FairThreshold = *req.FairThreshold
	}
	if req.PoorThreshold != nil {
		cfg.PoorThreshold = *req.PoorThreshold
	}
	if req.IsolateThreshold != nil {
		cfg.IsolateThreshold = *req.IsolateThreshold
	}
	if req.CalcIntervalSeconds != nil {
		cfg.CalcIntervalSeconds = *req.CalcIntervalSeconds
	}
	if req.ProbeIntervalSeconds != nil {
		cfg.ProbeIntervalSeconds = *req.ProbeIntervalSeconds
	}
	if req.AutoIsolateEnabled != nil {
		cfg.AutoIsolateEnabled = *req.AutoIsolateEnabled
	}
	if err := s.repo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	s.cfg = cfg
	return cfg, nil
}

// CalculateAll 计算所有节点的体验分（定时任务调用）
func (s *Service) CalculateAll(ctx context.Context) error {
	if s.provider == nil {
		return nil
	}
	metrics, err := s.provider.CollectAllNodeMetrics(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, m := range metrics {
		cur := s.calculateScore(&m, now)
		if err := s.repo.UpsertCurrent(ctx, cur, true); err != nil {
			s.logger.Warn("upsert experience score failed", "node_id", m.NodeID, "error", err)
		}
	}
	s.logger.Info("experience scores recalculated", "count", len(metrics))
	return nil
}

// calculateScore 根据原始指标和配置计算体验分
func (s *Service) calculateScore(m *NodeMetrics, now time.Time) *Current {
	cfg := s.cfg

	// 1. 延迟分（基于 p95）
	latencyScore := 100.0
	if m.P95LatencyMs != nil {
		latency := *m.P95LatencyMs
		// <100ms 满分，>2000ms 0 分，线性插值
		if latency <= 100 {
			latencyScore = 100
		} else if latency >= 2000 {
			latencyScore = 0
		} else {
			latencyScore = 100 * (1 - (latency-100)/1900)
		}
	}

	// 2. 稳定性分（基于心跳成功率 - 通道降级次数）
	stabilityScore := 100.0
	if m.HeartbeatSuccessRate != nil {
		stabilityScore = 100 * (*m.HeartbeatSuccessRate)
	}
	if m.ChannelFailoverCount24h != nil {
		// 每次降级扣 5 分，最低 0
		penalty := float64(*m.ChannelFailoverCount24h) * 5
		stabilityScore -= penalty
		if stabilityScore < 0 {
			stabilityScore = 0
		}
	}

	// 3. 速度分（基于实测带宽）
	speedScore := 80.0 // 默认中等（未实测）
	if m.MeasuredBandwidthMbps != nil {
		bw := *m.MeasuredBandwidthMbps
		// >=100Mbps 满分，<1Mbps 0 分
		if bw >= 100 {
			speedScore = 100
		} else if bw <= 1 {
			speedScore = 0
		} else {
			speedScore = 100 * bw / 100
		}
	}

	// 4. 成功率分
	successRateScore := 100.0
	if m.ConnectionSuccessRate != nil {
		successRateScore = 100 * (*m.ConnectionSuccessRate)
	}

	// 加权综合分
	overall := latencyScore*cfg.WeightLatency +
		stabilityScore*cfg.WeightStability +
		speedScore*cfg.WeightSpeed +
		successRateScore*cfg.WeightSuccessRate

	// 分级
	grade := s.gradeOf(overall)
	isolated := cfg.AutoIsolateEnabled && overall < cfg.IsolateThreshold

	return &Current{
		NodeID:                  m.NodeID,
		OverallScore:            round2(overall),
		LatencyScore:            round2(latencyScore),
		StabilityScore:          round2(stabilityScore),
		SpeedScore:              round2(speedScore),
		SuccessRateScore:        round2(successRateScore),
		P50LatencyMs:            m.P50LatencyMs,
		P95LatencyMs:            m.P95LatencyMs,
		P99LatencyMs:            m.P99LatencyMs,
		HeartbeatSuccessRate:    m.HeartbeatSuccessRate,
		ChannelFailoverCount24h: m.ChannelFailoverCount24h,
		MeasuredBandwidthMbps:   m.MeasuredBandwidthMbps,
		ConnectionSuccessRate:   m.ConnectionSuccessRate,
		Grade:                   grade,
		Isolated:                isolated,
		CalculatedAt:            now,
	}
}

func (s *Service) gradeOf(score float64) string {
	cfg := s.cfg
	if score >= cfg.ExcellentThreshold {
		return "excellent"
	}
	if score >= cfg.GoodThreshold {
		return "good"
	}
	if score >= cfg.FairThreshold {
		return "fair"
	}
	if score >= cfg.PoorThreshold {
		return "poor"
	}
	return "critical"
}

// ListCurrent 列出所有节点当前体验分
func (s *Service) ListCurrent(ctx context.Context, nodeID *uuid.UUID, grade string, onlyIsolated bool, page, pageSize int) ([]*Current, int, error) {
	return s.repo.ListCurrent(ctx, nodeID, grade, onlyIsolated, page, pageSize)
}

// ListHistory 列出某节点历史评分
func (s *Service) ListHistory(ctx context.Context, nodeID uuid.UUID, limit int) ([]*Score, error) {
	return s.repo.ListHistory(ctx, nodeID, limit)
}

// StartCalculationLoop 启动定时计算循环（阻塞，应在 goroutine 中调用）
func (s *Service) StartCalculationLoop(ctx context.Context) {
	_ = s.LoadConfig(ctx)
	interval := time.Duration(s.cfg.CalcIntervalSeconds) * time.Second
	if interval < 60*time.Second {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.logger.Info("experience calculation loop started", "interval", interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.CalculateAll(ctx); err != nil {
				s.logger.Error("calculate all experience scores failed", "error", err)
			}
		}
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
