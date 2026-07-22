package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthRepo struct {
	pool *pgxpool.Pool
}

func NewHealthRepo(pool *pgxpool.Pool) *HealthRepo {
	return &HealthRepo{pool: pool}
}

func (r *HealthRepo) UpsertStatus(ctx context.Context, status *model.NodeHealthStatus) error {
	query := `
		INSERT INTO node_health_status (
			node_id, overall_status, heartbeat_status, probe_status,
			availability_score, latency_score, loss_score, handshake_score, chain_score, stability_score,
			current_rtt_ms, current_loss_ratio, current_online_users, current_cpu_percent, current_mem_percent, current_disk_percent,
			last_heartbeat_at, last_probe_at, last_state_changed_at, last_error_code, last_error_message, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, now())
		ON CONFLICT (node_id) DO UPDATE SET
			overall_status = EXCLUDED.overall_status,
			heartbeat_status = EXCLUDED.heartbeat_status,
			probe_status = EXCLUDED.probe_status,
			availability_score = EXCLUDED.availability_score,
			latency_score = EXCLUDED.latency_score,
			loss_score = EXCLUDED.loss_score,
			handshake_score = EXCLUDED.handshake_score,
			chain_score = EXCLUDED.chain_score,
			stability_score = EXCLUDED.stability_score,
			current_rtt_ms = EXCLUDED.current_rtt_ms,
			current_loss_ratio = EXCLUDED.current_loss_ratio,
			current_online_users = EXCLUDED.current_online_users,
			current_cpu_percent = EXCLUDED.current_cpu_percent,
			current_mem_percent = EXCLUDED.current_mem_percent,
			current_disk_percent = EXCLUDED.current_disk_percent,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			last_probe_at = EXCLUDED.last_probe_at,
			last_state_changed_at = EXCLUDED.last_state_changed_at,
			last_error_code = EXCLUDED.last_error_code,
			last_error_message = EXCLUDED.last_error_message,
			updated_at = now()
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, query,
		status.NodeID, status.OverallStatus, status.HeartbeatStatus, status.ProbeStatus,
		status.AvailabilityScore, status.LatencyScore, status.LossScore, status.HandshakeScore, status.ChainScore, status.StabilityScore,
		status.CurrentRTTMs, status.CurrentLossRatio, status.CurrentOnlineUsers, status.CurrentCPUPercent, status.CurrentMemPercent, status.CurrentDiskPercent,
		status.LastHeartbeatAt, status.LastProbeAt, status.LastStateChangedAt, status.LastErrorCode, status.LastErrorMessage,
	).Scan(&status.UpdatedAt)
}

func (r *HealthRepo) GetStatusByNodeID(ctx context.Context, nodeID uuid.UUID) (*model.NodeHealthStatus, error) {
	query := `
		SELECT node_id, overall_status, heartbeat_status, probe_status,
			availability_score, latency_score, loss_score, handshake_score, chain_score, stability_score,
			current_rtt_ms, current_loss_ratio, current_online_users, current_cpu_percent, current_mem_percent, current_disk_percent,
			last_heartbeat_at, last_probe_at, last_state_changed_at, last_error_code, last_error_message, updated_at
		FROM node_health_status WHERE node_id = $1`
	s := &model.NodeHealthStatus{}
	err := r.pool.QueryRow(ctx, query, nodeID).Scan(
		&s.NodeID, &s.OverallStatus, &s.HeartbeatStatus, &s.ProbeStatus,
		&s.AvailabilityScore, &s.LatencyScore, &s.LossScore, &s.HandshakeScore, &s.ChainScore, &s.StabilityScore,
		&s.CurrentRTTMs, &s.CurrentLossRatio, &s.CurrentOnlineUsers, &s.CurrentCPUPercent, &s.CurrentMemPercent, &s.CurrentDiskPercent,
		&s.LastHeartbeatAt, &s.LastProbeAt, &s.LastStateChangedAt, &s.LastErrorCode, &s.LastErrorMessage, &s.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *HealthRepo) RecordEvent(ctx context.Context, event *model.NodeHealthEvent) error {
	query := `
		INSERT INTO node_health_events (id, node_id, event_type, severity, from_status, to_status, metrics, message, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING occurred_at`
	now := time.Now()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	return r.pool.QueryRow(ctx, query,
		event.ID, event.NodeID, event.EventType, event.Severity, event.FromStatus, event.ToStatus, event.Metrics, event.Message, event.OccurredAt,
	).Scan(&event.OccurredAt)
}

func (r *HealthRepo) ListEvents(ctx context.Context, page, pageSize int, nodeID, eventType, severity string, startTime, endTime *time.Time) ([]*model.NodeHealthEvent, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if nodeID != "" {
		where = append(where, fmt.Sprintf("node_id = $%d", argIdx))
		nid, err := uuid.Parse(nodeID)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, nid)
		argIdx++
	}
	if eventType != "" {
		where = append(where, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, eventType)
		argIdx++
	}
	if severity != "" {
		where = append(where, fmt.Sprintf("severity = $%d", argIdx))
		args = append(args, severity)
		argIdx++
	}
	if startTime != nil {
		where = append(where, fmt.Sprintf("occurred_at >= $%d", argIdx))
		args = append(args, *startTime)
		argIdx++
	}
	if endTime != nil {
		where = append(where, fmt.Sprintf("occurred_at <= $%d", argIdx))
		args = append(args, *endTime)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM node_health_events WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, node_id, event_type, severity, from_status, to_status, metrics, message, occurred_at
		FROM node_health_events WHERE %s
		ORDER BY occurred_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []*model.NodeHealthEvent
	for rows.Next() {
		e := &model.NodeHealthEvent{}
		err := rows.Scan(
			&e.ID, &e.NodeID, &e.EventType, &e.Severity, &e.FromStatus, &e.ToStatus, &e.Metrics, &e.Message, &e.OccurredAt,
		)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}
