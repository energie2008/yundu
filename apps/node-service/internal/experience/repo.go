package experience

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Repo
// ============================================================================

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// UpsertCurrent 更新当前节点体验分；同时插入一条历史快照
func (r *Repo) UpsertCurrent(ctx context.Context, cur *Current, insertHistory bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// UPSERT current
	_, err = tx.Exec(ctx, `
		INSERT INTO node_experience_current
			(node_id, overall_score, latency_score, stability_score, speed_score, success_rate_score,
			 p50_latency_ms, p95_latency_ms, p99_latency_ms, heartbeat_success_rate,
			 channel_failover_count_24h, measured_bandwidth_mbps, connection_success_rate,
			 grade, isolated, calculated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (node_id) DO UPDATE SET
			overall_score = EXCLUDED.overall_score,
			latency_score = EXCLUDED.latency_score,
			stability_score = EXCLUDED.stability_score,
			speed_score = EXCLUDED.speed_score,
			success_rate_score = EXCLUDED.success_rate_score,
			p50_latency_ms = EXCLUDED.p50_latency_ms,
			p95_latency_ms = EXCLUDED.p95_latency_ms,
			p99_latency_ms = EXCLUDED.p99_latency_ms,
			heartbeat_success_rate = EXCLUDED.heartbeat_success_rate,
			channel_failover_count_24h = EXCLUDED.channel_failover_count_24h,
			measured_bandwidth_mbps = EXCLUDED.measured_bandwidth_mbps,
			connection_success_rate = EXCLUDED.connection_success_rate,
			grade = EXCLUDED.grade,
			isolated = EXCLUDED.isolated,
			calculated_at = EXCLUDED.calculated_at
	`,
		cur.NodeID, cur.OverallScore, cur.LatencyScore, cur.StabilityScore,
		cur.SpeedScore, cur.SuccessRateScore,
		cur.P50LatencyMs, cur.P95LatencyMs, cur.P99LatencyMs, cur.HeartbeatSuccessRate,
		cur.ChannelFailoverCount24h, cur.MeasuredBandwidthMbps, cur.ConnectionSuccessRate,
		cur.Grade, cur.Isolated, cur.CalculatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert current: %w", err)
	}

	// INSERT history snapshot
	if insertHistory {
		_, err = tx.Exec(ctx, `
			INSERT INTO node_experience_scores
				(node_id, overall_score, latency_score, stability_score, speed_score, success_rate_score,
				 p50_latency_ms, p95_latency_ms, p99_latency_ms, heartbeat_success_rate,
				 channel_failover_count_24h, measured_bandwidth_mbps, connection_success_rate,
				 grade, isolated, calculated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		`,
			cur.NodeID, cur.OverallScore, cur.LatencyScore, cur.StabilityScore,
			cur.SpeedScore, cur.SuccessRateScore,
			cur.P50LatencyMs, cur.P95LatencyMs, cur.P99LatencyMs, cur.HeartbeatSuccessRate,
			cur.ChannelFailoverCount24h, cur.MeasuredBandwidthMbps, cur.ConnectionSuccessRate,
			cur.Grade, cur.Isolated, cur.CalculatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert history: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ListCurrent 列出所有节点当前体验分
func (r *Repo) ListCurrent(ctx context.Context, nodeID *uuid.UUID, grade string, onlyIsolated bool, page, pageSize int) ([]*Current, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argPos := 1
	if nodeID != nil {
		where += fmt.Sprintf(" AND node_id = $%d", argPos)
		args = append(args, *nodeID)
		argPos++
	}
	if grade != "" {
		where += fmt.Sprintf(" AND grade = $%d", argPos)
		args = append(args, grade)
		argPos++
	}
	if onlyIsolated {
		where += " AND isolated = true"
	}

	var total int
	if err := r.pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM node_experience_current %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := fmt.Sprintf(`
		SELECT node_id, overall_score, latency_score, stability_score, speed_score, success_rate_score,
			   p50_latency_ms, p95_latency_ms, p99_latency_ms, heartbeat_success_rate,
			   channel_failover_count_24h, measured_bandwidth_mbps, connection_success_rate,
			   grade, isolated, calculated_at
		FROM node_experience_current %s
		ORDER BY overall_score DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*Current
	for rows.Next() {
		c := &Current{}
		if err := rows.Scan(
			&c.NodeID, &c.OverallScore, &c.LatencyScore, &c.StabilityScore, &c.SpeedScore, &c.SuccessRateScore,
			&c.P50LatencyMs, &c.P95LatencyMs, &c.P99LatencyMs, &c.HeartbeatSuccessRate,
			&c.ChannelFailoverCount24h, &c.MeasuredBandwidthMbps, &c.ConnectionSuccessRate,
			&c.Grade, &c.Isolated, &c.CalculatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, c)
	}
	return items, total, nil
}

// ListHistory 列出某节点历史评分
func (r *Repo) ListHistory(ctx context.Context, nodeID uuid.UUID, limit int) ([]*Score, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, node_id, overall_score, latency_score, stability_score, speed_score, success_rate_score,
			   p50_latency_ms, p95_latency_ms, p99_latency_ms, heartbeat_success_rate,
			   channel_failover_count_24h, measured_bandwidth_mbps, connection_success_rate,
			   grade, isolated, calculated_at
		FROM node_experience_scores
		WHERE node_id = $1
		ORDER BY calculated_at DESC
		LIMIT $2
	`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*Score
	for rows.Next() {
		s := &Score{}
		if err := rows.Scan(
			&s.ID, &s.NodeID, &s.OverallScore, &s.LatencyScore, &s.StabilityScore, &s.SpeedScore, &s.SuccessRateScore,
			&s.P50LatencyMs, &s.P95LatencyMs, &s.P99LatencyMs, &s.HeartbeatSuccessRate,
			&s.ChannelFailoverCount24h, &s.MeasuredBandwidthMbps, &s.ConnectionSuccessRate,
			&s.Grade, &s.Isolated, &s.CalculatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, nil
}

// GetConfig 读取评分配置
func (r *Repo) GetConfig(ctx context.Context) (*Config, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT weight_latency, weight_stability, weight_speed, weight_success_rate,
			   excellent_threshold, good_threshold, fair_threshold, poor_threshold, isolate_threshold,
			   calc_interval_seconds, probe_interval_seconds, auto_isolate_enabled
		FROM node_experience_config WHERE id = 1
	`)
	cfg := &Config{}
	if err := row.Scan(
		&cfg.WeightLatency, &cfg.WeightStability, &cfg.WeightSpeed, &cfg.WeightSuccessRate,
		&cfg.ExcellentThreshold, &cfg.GoodThreshold, &cfg.FairThreshold, &cfg.PoorThreshold, &cfg.IsolateThreshold,
		&cfg.CalcIntervalSeconds, &cfg.ProbeIntervalSeconds, &cfg.AutoIsolateEnabled,
	); err != nil {
		if err == pgx.ErrNoRows {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	return cfg, nil
}

// UpdateConfig 更新评分配置
func (r *Repo) UpdateConfig(ctx context.Context, cfg *Config) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO node_experience_config (id, weight_latency, weight_stability, weight_speed, weight_success_rate,
			excellent_threshold, good_threshold, fair_threshold, poor_threshold, isolate_threshold,
			calc_interval_seconds, probe_interval_seconds, auto_isolate_enabled, updated_at)
		VALUES (1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
		ON CONFLICT (id) DO UPDATE SET
			weight_latency = EXCLUDED.weight_latency,
			weight_stability = EXCLUDED.weight_stability,
			weight_speed = EXCLUDED.weight_speed,
			weight_success_rate = EXCLUDED.weight_success_rate,
			excellent_threshold = EXCLUDED.excellent_threshold,
			good_threshold = EXCLUDED.good_threshold,
			fair_threshold = EXCLUDED.fair_threshold,
			poor_threshold = EXCLUDED.poor_threshold,
			isolate_threshold = EXCLUDED.isolate_threshold,
			calc_interval_seconds = EXCLUDED.calc_interval_seconds,
			probe_interval_seconds = EXCLUDED.probe_interval_seconds,
			auto_isolate_enabled = EXCLUDED.auto_isolate_enabled,
			updated_at = now()
	`,
		cfg.WeightLatency, cfg.WeightStability, cfg.WeightSpeed, cfg.WeightSuccessRate,
		cfg.ExcellentThreshold, cfg.GoodThreshold, cfg.FairThreshold, cfg.PoorThreshold, cfg.IsolateThreshold,
		cfg.CalcIntervalSeconds, cfg.ProbeIntervalSeconds, cfg.AutoIsolateEnabled,
	)
	return err
}

// DeleteOldHistory 清理过期历史（cron 调用）
func (r *Repo) DeleteOldHistory(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.pool.Exec(ctx, `DELETE FROM node_experience_scores WHERE calculated_at < $1`, before)
	if err != nil {
		return 0, err
	}
	n := res.RowsAffected()
	return n, nil
}
