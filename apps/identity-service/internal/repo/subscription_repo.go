package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SubscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepo(pool *pgxpool.Pool) *SubscriptionRepo {
	return &SubscriptionRepo{pool: pool}
}

func (r *SubscriptionRepo) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (*model.UserPlanSubscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, started_at, expires_at, renewal_mode,
		       traffic_quota_bytes, traffic_used_bytes,
		       COALESCE(upload_bytes, 0), COALESCE(download_bytes, 0),
		       reset_at, speed_limit_mbps,
		       device_limit, ip_limit, source, metadata, created_at, updated_at, deleted_at
		FROM user_plan_subscriptions
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC LIMIT 1`
	s := &model.UserPlanSubscription{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.PlanID, &s.Status, &s.StartedAt, &s.ExpiresAt, &s.RenewalMode,
		&s.TrafficQuotaBytes, &s.TrafficUsedBytes, &s.UploadBytes, &s.DownloadBytes, &s.ResetAt, &s.SpeedLimitMbps,
		&s.DeviceLimit, &s.IPLimit, &s.Source, &s.Metadata,
		&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *SubscriptionRepo) Create(ctx context.Context, sub *model.UserPlanSubscription) error {
	query := `
		INSERT INTO user_plan_subscriptions (
			id, user_id, plan_id, status, started_at, expires_at, renewal_mode,
			traffic_quota_bytes, traffic_used_bytes, reset_at, speed_limit_mbps,
			device_limit, ip_limit, source, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at, updated_at`
	metadata := sub.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	now := time.Now()
	startedAt := sub.StartedAt
	if startedAt == nil {
		startedAt = &now
	}
	return r.pool.QueryRow(ctx, query,
		sub.ID, sub.UserID, sub.PlanID, sub.Status, startedAt, sub.ExpiresAt, sub.RenewalMode,
		sub.TrafficQuotaBytes, sub.TrafficUsedBytes, sub.ResetAt, sub.SpeedLimitMbps,
		sub.DeviceLimit, sub.IPLimit, sub.Source, metadata,
	).Scan(&sub.CreatedAt, &sub.UpdatedAt)
}

func (r *SubscriptionRepo) ExtendByDays(ctx context.Context, id uuid.UUID, days int) error {
	query := `
		UPDATE user_plan_subscriptions
		SET expires_at = CASE
			WHEN expires_at IS NULL THEN now() + ($2::int * INTERVAL '1 day')
			WHEN expires_at < now() THEN now() + ($2::int * INTERVAL '1 day')
			ELSE expires_at + ($2::int * INTERVAL '1 day')
		END,
		updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, days)
	return err
}

func (r *SubscriptionRepo) MarkReplaced(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE user_plan_subscriptions SET status = 'replaced', updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *SubscriptionRepo) AddTrafficUsed(ctx context.Context, userID uuid.UUID, bytes int64) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_used_bytes = traffic_used_bytes + $2, updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())`
	_, err := r.pool.Exec(ctx, query, userID, bytes)
	return err
}

func (r *SubscriptionRepo) ResetTraffic(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_used_bytes = 0, reset_at = now(), updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *SubscriptionRepo) ResetAllTokensForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE subscription_tokens SET status = 'revoked', revoked_at = now(), updated_at = now() WHERE user_id = $1 AND status = 'active'`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *SubscriptionRepo) SetTrafficBytes(ctx context.Context, userID uuid.UUID, bytes int64) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_used_bytes = GREATEST(0, traffic_used_bytes + $2), updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID, bytes)
	return err
}

func (r *SubscriptionRepo) UpdateQuotaBytes(ctx context.Context, userID uuid.UUID, quotaBytes int64) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_quota_bytes = $2, updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID, quotaBytes)
	return err
}

func (r *SubscriptionRepo) AddQuotaBytes(ctx context.Context, userID uuid.UUID, bytes int64) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_quota_bytes = traffic_quota_bytes + $2, updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID, bytes)
	return err
}

func (r *SubscriptionRepo) UpdateExpiresAt(ctx context.Context, userID uuid.UUID, expiresAt time.Time) error {
	query := `
		UPDATE user_plan_subscriptions
		SET expires_at = $2, updated_at = now()
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID, expiresAt)
	return err
}

// ListExpiringSoon 返回在未来 withinHours 小时内到期、且尚未过期的活跃订阅（用于套餐到期提醒）。
// 排除 traffic_quota_bytes 为 NULL 且 expires_at 为 NULL 的无限期订阅。
func (r *SubscriptionRepo) ListExpiringSoon(ctx context.Context, withinHours int) ([]*model.UserPlanSubscription, error) {
	if withinHours <= 0 {
		withinHours = 72
	}
	query := `
		SELECT ups.id, ups.user_id, ups.plan_id, p.name as plan_name, ups.status,
		       COALESCE(ups.started_at, ups.created_at) as started_at, ups.expires_at,
		       COALESCE(ups.traffic_quota_bytes, p.traffic_bytes) as traffic_quota_bytes,
		       COALESCE(ups.traffic_used_bytes, 0) as traffic_used_bytes,
		       COALESCE(ups.speed_limit_mbps, p.speed_limit_mbps, 0) as speed_limit_mbps,
		       COALESCE(ups.device_limit, p.device_limit, 0) as device_limit,
		       ups.reset_at
		FROM user_plan_subscriptions ups
		JOIN plans p ON p.id = ups.plan_id
		WHERE ups.status = 'active' AND ups.deleted_at IS NULL
		  AND ups.expires_at IS NOT NULL
		  AND ups.expires_at > now()
		  AND ups.expires_at <= now() + ($1 || ' hours')::interval
		ORDER BY ups.expires_at ASC
		LIMIT 500`
	rows, err := r.pool.Query(ctx, query, withinHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*model.UserPlanSubscription
	for rows.Next() {
		s := &model.UserPlanSubscription{}
		var speedLimit, deviceLimit *int
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.PlanID, &s.PlanName, &s.Status,
			&s.StartedAt, &s.ExpiresAt,
			&s.TrafficQuotaBytes, &s.TrafficUsedBytes,
			&speedLimit, &deviceLimit, &s.ResetAt,
		); err != nil {
			return nil, err
		}
		if speedLimit != nil {
			s.SpeedLimitMbps = speedLimit
		}
		if deviceLimit != nil {
			s.DeviceLimit = deviceLimit
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// ListHighTrafficUsage 返回已用流量占比超过 thresholdPct（0-100）的活跃订阅（用于流量耗尽提醒）。
// 仅返回有流量配额(>0)的订阅。
func (r *SubscriptionRepo) ListHighTrafficUsage(ctx context.Context, thresholdPct float64) ([]*model.UserPlanSubscription, error) {
	if thresholdPct <= 0 {
		thresholdPct = 80
	}
	query := `
		SELECT ups.id, ups.user_id, ups.plan_id, p.name as plan_name, ups.status,
		       COALESCE(ups.started_at, ups.created_at) as started_at, ups.expires_at,
		       COALESCE(ups.traffic_quota_bytes, p.traffic_bytes) as traffic_quota_bytes,
		       COALESCE(ups.traffic_used_bytes, 0) as traffic_used_bytes,
		       COALESCE(ups.speed_limit_mbps, p.speed_limit_mbps, 0) as speed_limit_mbps,
		       COALESCE(ups.device_limit, p.device_limit, 0) as device_limit,
		       ups.reset_at
		FROM user_plan_subscriptions ups
		JOIN plans p ON p.id = ups.plan_id
		WHERE ups.status = 'active' AND ups.deleted_at IS NULL
		  AND COALESCE(ups.traffic_quota_bytes, p.traffic_bytes) > 0
		  AND COALESCE(ups.traffic_used_bytes, 0) * 100.0 / COALESCE(ups.traffic_quota_bytes, p.traffic_bytes) >= $1
		ORDER BY ups.traffic_used_bytes DESC
		LIMIT 500`
	rows, err := r.pool.Query(ctx, query, thresholdPct)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*model.UserPlanSubscription
	for rows.Next() {
		s := &model.UserPlanSubscription{}
		var speedLimit, deviceLimit *int
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.PlanID, &s.PlanName, &s.Status,
			&s.StartedAt, &s.ExpiresAt,
			&s.TrafficQuotaBytes, &s.TrafficUsedBytes,
			&speedLimit, &deviceLimit, &s.ResetAt,
		); err != nil {
			return nil, err
		}
		if speedLimit != nil {
			s.SpeedLimitMbps = speedLimit
		}
		if deviceLimit != nil {
			s.DeviceLimit = deviceLimit
		}
		items = append(items, s)
	}
	return items, rows.Err()
}
