package channelhealth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// UpsertCurrent 更新/插入当前通道健康状态；同时插入一条 snapshot
// 如果 hb.Failover != nil，再插入一条 failover_event
func (r *Repo) UpsertCurrent(ctx context.Context, cur *ChannelHealthCurrent, snapshot *ChannelHealthSnapshot, failover *FailoverEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. UPSERT channel_health_current
	_, err = tx.Exec(ctx, `
		INSERT INTO channel_health_current
			(server_id, runtime_id, active_channel, channel_state, rtt_ms, fail_count_1h, online_users,
			 failover_count_1h, failover_count_24h, last_error, last_failover_at, last_failover_from,
			 last_failover_to, last_failover_reason, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now())
		ON CONFLICT (server_id) DO UPDATE SET
			runtime_id = EXCLUDED.runtime_id,
			active_channel = EXCLUDED.active_channel,
			channel_state = EXCLUDED.channel_state,
			rtt_ms = EXCLUDED.rtt_ms,
			fail_count_1h = EXCLUDED.fail_count_1h,
			online_users = EXCLUDED.online_users,
			last_error = EXCLUDED.last_error,
			last_failover_at = EXCLUDED.last_failover_at,
			last_failover_from = EXCLUDED.last_failover_from,
			last_failover_to = EXCLUDED.last_failover_to,
			last_failover_reason = EXCLUDED.last_failover_reason,
			updated_at = now()
	`,
		cur.ServerID, cur.RuntimeID, cur.ActiveChannel, cur.ChannelState, cur.RTTMs,
		cur.FailCount1h, cur.OnlineUsers, cur.FailoverCount1h, cur.FailoverCount24h,
		cur.LastError, cur.LastFailoverAt, cur.LastFailoverFrom, cur.LastFailoverTo,
		cur.LastFailoverReason,
	)
	if err != nil {
		return fmt.Errorf("upsert channel_health_current: %w", err)
	}

	// 2. INSERT channel_health_snapshots
	if snapshot != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO channel_health_snapshots
				(server_id, runtime_id, active_channel, channel_state, rtt_ms, fail_count_1h, online_users, last_error, reported_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		`,
			snapshot.ServerID, snapshot.RuntimeID, snapshot.ActiveChannel, snapshot.ChannelState,
			snapshot.RTTMs, snapshot.FailCount1h, snapshot.OnlineUsers, snapshot.LastError,
		)
		if err != nil {
			return fmt.Errorf("insert channel_health_snapshots: %w", err)
		}
	}

	// 3. INSERT channel_failover_events（如果发生降级）
	if failover != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO channel_failover_events
				(server_id, runtime_id, from_channel, to_channel, reason, detail, occurred_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`,
			failover.ServerID, failover.RuntimeID, failover.FromChannel, failover.ToChannel,
			failover.Reason, failover.Detail, failover.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("insert channel_failover_events: %w", err)
		}
	}

	// 4. 重算 failover_count_1h / 24h（基于 events 表）
	_, err = tx.Exec(ctx, `
		UPDATE channel_health_current ch
		SET
			failover_count_1h = (
				SELECT COUNT(*) FROM channel_failover_events fe
				WHERE fe.server_id = ch.server_id AND fe.occurred_at > now() - interval '1 hour'
			),
			failover_count_24h = (
				SELECT COUNT(*) FROM channel_failover_events fe
				WHERE fe.server_id = ch.server_id AND fe.occurred_at > now() - interval '24 hours'
			)
		WHERE ch.server_id = $1
	`, cur.ServerID)
	if err != nil {
		return fmt.Errorf("recompute failover counts: %w", err)
	}

	return tx.Commit(ctx)
}

// ListCurrent 返回所有服务器的当前通道状态
func (r *Repo) ListCurrent(ctx context.Context, serverID *uuid.UUID, channelState string, page, pageSize int) ([]*ChannelHealthListItem, int, error) {
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
	if serverID != nil {
		where += fmt.Sprintf(" AND ch.server_id = $%d", argPos)
		args = append(args, *serverID)
		argPos++
	}
	if channelState != "" {
		where += fmt.Sprintf(" AND ch.channel_state = $%d", argPos)
		args = append(args, channelState)
		argPos++
	}

	// count
	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM channel_health_current ch %s`, where)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := fmt.Sprintf(`
		SELECT ch.server_id, s.code, s.name,
			   ch.active_channel, ch.channel_state, ch.rtt_ms, ch.fail_count_1h, ch.online_users,
			   ch.failover_count_1h, ch.failover_count_24h,
			   ch.last_failover_at, ch.last_failover_from, ch.last_failover_to, ch.last_failover_reason,
			   ch.updated_at
		FROM channel_health_current ch
		JOIN servers s ON s.id = ch.server_id
		%s
		ORDER BY ch.updated_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*ChannelHealthListItem
	for rows.Next() {
		it := &ChannelHealthListItem{}
		if err := rows.Scan(
			&it.ServerID, &it.ServerCode, &it.ServerName,
			&it.ActiveChannel, &it.ChannelState, &it.RTTMs, &it.FailCount1h, &it.OnlineUsers,
			&it.FailoverCount1h, &it.FailoverCount24h,
			&it.LastFailoverAt, &it.LastFailoverFrom, &it.LastFailoverTo, &it.LastFailoverReason,
			&it.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, nil
}

// ListFailoverEvents 返回降级事件
func (r *Repo) ListFailoverEvents(ctx context.Context, serverID *uuid.UUID, reason string, startAt, endAt *time.Time, page, pageSize int) ([]*FailoverEvent, int, error) {
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
	if serverID != nil {
		where += fmt.Sprintf(" AND fe.server_id = $%d", argPos)
		args = append(args, *serverID)
		argPos++
	}
	if reason != "" {
		where += fmt.Sprintf(" AND fe.reason = $%d", argPos)
		args = append(args, reason)
		argPos++
	}
	if startAt != nil {
		where += fmt.Sprintf(" AND fe.occurred_at >= $%d", argPos)
		args = append(args, *startAt)
		argPos++
	}
	if endAt != nil {
		where += fmt.Sprintf(" AND fe.occurred_at <= $%d", argPos)
		args = append(args, *endAt)
		argPos++
	}

	var total int
	countSQL := fmt.Sprintf(`SELECT COUNT(*) FROM channel_failover_events fe %s`, where)
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := fmt.Sprintf(`
		SELECT fe.id, fe.server_id, fe.runtime_id, fe.from_channel, fe.to_channel, fe.reason, fe.detail, fe.occurred_at
		FROM channel_failover_events fe
		%s
		ORDER BY fe.occurred_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argPos, argPos+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*FailoverEvent
	for rows.Next() {
		it := &FailoverEvent{}
		if err := rows.Scan(
			&it.ID, &it.ServerID, &it.RuntimeID, &it.FromChannel, &it.ToChannel,
			&it.Reason, &it.Detail, &it.OccurredAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	return items, total, nil
}

// ListSnapshots 返回某服务器的通道健康快照时间序列
func (r *Repo) ListSnapshots(ctx context.Context, serverID uuid.UUID, limit int) ([]*ChannelHealthSnapshot, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, server_id, runtime_id, active_channel, channel_state, rtt_ms,
			   fail_count_1h, online_users, last_error, reported_at
		FROM channel_health_snapshots
		WHERE server_id = $1
		ORDER BY reported_at DESC
		LIMIT $2
	`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ChannelHealthSnapshot
	for rows.Next() {
		it := &ChannelHealthSnapshot{}
		if err := rows.Scan(
			&it.ID, &it.ServerID, &it.RuntimeID, &it.ActiveChannel, &it.ChannelState,
			&it.RTTMs, &it.FailCount1h, &it.OnlineUsers, &it.LastError, &it.ReportedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// DeleteOldSnapshots 清理超过 N 天的快照（cron 调用）
func (r *Repo) DeleteOldSnapshots(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.pool.Exec(ctx, `DELETE FROM channel_health_snapshots WHERE reported_at < $1`, before)
	if err != nil {
		return 0, err
	}
	n := res.RowsAffected()
	return n, nil
}
