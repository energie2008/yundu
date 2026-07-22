package repo

import (
	"context"
	"time"

	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TrafficRepo struct {
	pool *pgxpool.Pool
}

func NewTrafficRepo(pool *pgxpool.Pool) *TrafficRepo {
	return &TrafficRepo{pool: pool}
}

func (r *TrafficRepo) RecordUsage(ctx context.Context, userID uuid.UUID, nodeID *uuid.UUID, uploadBytes, downloadBytes int64, timestamp time.Time) error {
	date := timestamp.Truncate(24 * time.Hour)

	sub, err := r.GetActiveUserSubscription(ctx, userID)
	if err != nil {
		return err
	}

	var subID *uuid.UUID
	if sub != nil {
		subID = &sub.ID
	}

	// 先尝试 UPDATE（使用 IS NOT DISTINCT FROM 正确处理 NULL node_id）。
	// PostgreSQL UNIQUE 约束中 NULL != NULL，ON CONFLICT 对 NULL 不生效，
	// 因此先 UPDATE 再 INSERT 是最可靠的方式，避免同日重复行。
	updateQuery := `
		UPDATE traffic_usage_daily
		SET upload_bytes = upload_bytes + $4,
		    download_bytes = download_bytes + $5,
		    updated_at = now()
		WHERE usage_date = $1 AND user_id = $2 AND node_id IS NOT DISTINCT FROM $3`
	tag, err := r.pool.Exec(ctx, updateQuery, date, userID, nodeID, uploadBytes, downloadBytes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		if sub != nil {
			_, err = r.pool.Exec(ctx, `
				UPDATE user_plan_subscriptions
				SET traffic_used_bytes = traffic_used_bytes + $2,
				    upload_bytes = upload_bytes + $3,
				    download_bytes = download_bytes + $4,
				    updated_at = now()
				WHERE id = $1`, sub.ID, uploadBytes+downloadBytes, uploadBytes, downloadBytes)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// 没有匹配行，INSERT 新行（ON CONFLICT 处理并发竞争，仅对非 NULL node_id 生效）
	insertQuery := `
		INSERT INTO traffic_usage_daily (id, usage_date, user_id, subscription_id, node_id, upload_bytes, download_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (usage_date, user_id, node_id)
		DO UPDATE SET
			upload_bytes = traffic_usage_daily.upload_bytes + EXCLUDED.upload_bytes,
			download_bytes = traffic_usage_daily.download_bytes + EXCLUDED.download_bytes,
			updated_at = now()`
	_, err = r.pool.Exec(ctx, insertQuery,
		uuid.New(), date, userID, subID, nodeID, uploadBytes, downloadBytes,
	)
	if err != nil {
		return err
	}

	if sub != nil {
		_, err = r.pool.Exec(ctx, `
			UPDATE user_plan_subscriptions
			SET traffic_used_bytes = traffic_used_bytes + $2,
			    upload_bytes = upload_bytes + $3,
			    download_bytes = download_bytes + $4,
			    updated_at = now()
			WHERE id = $1`, sub.ID, uploadBytes+downloadBytes, uploadBytes, downloadBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *TrafficRepo) GetDailyUsage(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) ([]*model.TrafficUsageDaily, error) {
	query := `
		SELECT id, usage_date, user_id, subscription_id, node_id, upload_bytes, download_bytes, total_bytes, unique_devices, created_at, updated_at
		FROM traffic_usage_daily
		WHERE user_id = $1 AND usage_date >= $2 AND usage_date < $3
		ORDER BY usage_date ASC`
	rows, err := r.pool.Query(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []*model.TrafficUsageDaily
	for rows.Next() {
		u := &model.TrafficUsageDaily{}
		err := rows.Scan(
			&u.ID, &u.UsageDate, &u.UserID, &u.SubscriptionID, &u.NodeID,
			&u.UploadBytes, &u.DownloadBytes, &u.TotalBytes, &u.UniqueDevices,
			&u.CreatedAt, &u.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		usages = append(usages, u)
	}
	return usages, rows.Err()
}

func (r *TrafficRepo) GetUserTotalUsage(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (upload, download int64, err error) {
	query := `
		SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		FROM traffic_usage_daily
		WHERE user_id = $1 AND usage_date >= $2 AND usage_date < $3`
	err = r.pool.QueryRow(ctx, query, userID, startDate, endDate).Scan(&upload, &download)
	return
}

func (r *TrafficRepo) GetNodeTotalUsage(ctx context.Context, nodeID uuid.UUID, date time.Time) (upload, download int64, err error) {
	dayStart := date.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	query := `
		SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		FROM traffic_usage_daily
		WHERE node_id = $1 AND usage_date >= $2 AND usage_date < $3`
	err = r.pool.QueryRow(ctx, query, nodeID, dayStart, dayEnd).Scan(&upload, &download)
	return
}

func (r *TrafficRepo) GetTodayTotalUsage(ctx context.Context) (upload, download int64, err error) {
	today := time.Now().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)
	query := `
		SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		FROM traffic_usage_daily
		WHERE usage_date >= $1 AND usage_date < $2`
	err = r.pool.QueryRow(ctx, query, today, tomorrow).Scan(&upload, &download)
	return
}

func (r *TrafficRepo) GetTopNodes(ctx context.Context, date time.Time, limit int) ([]*model.NodeTrafficItem, error) {
	dayStart := date.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	query := `
		SELECT node_id, SUM(upload_bytes) as upload, SUM(download_bytes) as download, SUM(total_bytes) as total
		FROM traffic_usage_daily
		WHERE node_id IS NOT NULL AND usage_date >= $1 AND usage_date < $2
		GROUP BY node_id
		ORDER BY total DESC
		LIMIT $3`
	rows, err := r.pool.Query(ctx, query, dayStart, dayEnd, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.NodeTrafficItem
	for rows.Next() {
		item := &model.NodeTrafficItem{}
		err := rows.Scan(&item.NodeID, &item.UploadBytes, &item.DownloadBytes, &item.TotalBytes)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TrafficRepo) GetActiveUserSubscription(ctx context.Context, userID uuid.UUID) (*model.UserPlanSubscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, started_at, expires_at, traffic_quota_bytes, traffic_used_bytes,
		       COALESCE(upload_bytes, 0), COALESCE(download_bytes, 0), reset_at
		FROM user_plan_subscriptions
		WHERE user_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1`
	s := &model.UserPlanSubscription{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.PlanID, &s.Status, &s.StartedAt, &s.ExpiresAt,
		&s.TrafficQuotaBytes, &s.TrafficUsedBytes, &s.UploadBytes, &s.DownloadBytes, &s.ResetAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *TrafficRepo) CheckQuota(ctx context.Context, userID uuid.UUID) (*model.QuotaCheckResult, error) {
	sub, err := r.GetActiveUserSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &model.QuotaCheckResult{
		IsOverQuota: false,
		IsExpired:   sub == nil,
	}

	if sub != nil {
		result.TrafficQuota = sub.TrafficQuotaBytes
		result.TrafficUsed = sub.TrafficUsedBytes
		if result.TrafficQuota > 0 {
			result.TrafficRemaining = result.TrafficQuota - result.TrafficUsed
			if result.TrafficRemaining < 0 {
				result.TrafficRemaining = 0
				result.IsOverQuota = true
			}
		} else {
			result.TrafficRemaining = -1
		}
		if sub.ExpiresAt != nil && sub.ExpiresAt.Before(time.Now()) {
			result.IsExpired = true
		}
	}

	return result, nil
}

func (r *TrafficRepo) ResetUserTraffic(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_used_bytes = 0, upload_bytes = 0, download_bytes = 0, reset_at = now(), updated_at = now()
		WHERE user_id = $1 AND status = 'active'`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *TrafficRepo) ResetAllMonthlyTraffic(ctx context.Context) error {
	query := `
		UPDATE user_plan_subscriptions
		SET traffic_used_bytes = 0, upload_bytes = 0, download_bytes = 0, reset_at = now(), updated_at = now()
		WHERE status = 'active'`
	_, err := r.pool.Exec(ctx, query)
	return err
}

// MarkExpiredSubscriptions 将所有已过期但仍标记为 active 的订阅置为 expired。
// 返回过期的 user_id 列表（用于后续踢人通知）。
func (r *TrafficRepo) MarkExpiredSubscriptions(ctx context.Context) ([]string, error) {
	query := `
		UPDATE user_plan_subscriptions
		SET status = 'expired', updated_at = now()
		WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at < now()
		RETURNING user_id`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, rows.Err()
}

// ListActiveUserIDs 返回所有 active 订阅对应的 user_id 列表。
func (r *TrafficRepo) ListActiveUserIDs(ctx context.Context) ([]string, error) {
	query := `SELECT user_id FROM user_plan_subscriptions WHERE status = 'active'`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListOverQuotaUserIDs 返回所有 active 且流量已超额的 user_id 列表。
// 仅包含设置了配额（traffic_quota_bytes > 0）的订阅，无限量套餐不会被视作超额。
func (r *TrafficRepo) ListOverQuotaUserIDs(ctx context.Context) ([]string, error) {
	query := `
		SELECT user_id FROM user_plan_subscriptions
		WHERE status = 'active' AND traffic_quota_bytes > 0 AND traffic_used_bytes >= traffic_quota_bytes`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListUsersExceedingUsageRatio 返回所有 active 且流量使用比例超过 ratio（0~1）的用户告警条目。
// 通过 JOIN users 表获取邮箱，用于流量提醒邮件。仅包含设置了配额的订阅。
func (r *TrafficRepo) ListUsersExceedingUsageRatio(ctx context.Context, ratio float64) ([]*model.TrafficUsageAlert, error) {
	if ratio <= 0 {
		ratio = 0.8
	}
	query := `
		SELECT s.user_id, u.email, s.traffic_used_bytes, s.traffic_quota_bytes
		FROM user_plan_subscriptions s
		JOIN users u ON u.id = s.user_id
		WHERE s.status = 'active'
		  AND s.traffic_quota_bytes > 0
		  AND s.traffic_used_bytes::float8 >= s.traffic_quota_bytes::float8 * $1`
	rows, err := r.pool.Query(ctx, query, ratio)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.TrafficUsageAlert
	for rows.Next() {
		a := &model.TrafficUsageAlert{}
		if err := rows.Scan(&a.UserID, &a.Email, &a.UsedBytes, &a.QuotaBytes); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

// RecordDailyStatistics 写入/更新每日流量统计汇总（按 stat_date 唯一）。
// 表 traffic_statistics_daily 由 migration 创建。
func (r *TrafficRepo) RecordDailyStatistics(ctx context.Context, stat *model.DailyStatistic) error {
	date := stat.StatDate.Truncate(24 * time.Hour)
	query := `
		INSERT INTO traffic_statistics_daily (stat_date, upload_bytes, download_bytes, total_bytes, active_users, online_count)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (stat_date) DO UPDATE SET
			upload_bytes = EXCLUDED.upload_bytes,
			download_bytes = EXCLUDED.download_bytes,
			total_bytes = EXCLUDED.total_bytes,
			active_users = EXCLUDED.active_users,
			online_count = EXCLUDED.online_count,
			updated_at = now()`
	_, err := r.pool.Exec(ctx, query,
		date, stat.UploadBytes, stat.DownloadBytes, stat.TotalBytes, stat.ActiveUsers, stat.OnlineCount,
	)
	return err
}

// GetNodeIDByServerCode 通过服务器代码查找该服务器下第一个启用节点的 ID。
// 用于流量上报时将 ServerCode 解析为 node_id，使节点级流量统计生效。
// 查询路径: servers(code) → runtimes(server_id) → nodes(runtime_id)
// 若找不到则返回 nil（不阻断流量记录，仅 node_id 为 NULL）。
func (r *TrafficRepo) GetNodeIDByServerCode(ctx context.Context, serverCode string) (*uuid.UUID, error) {
	query := `
		SELECT n.id
		FROM nodes n
		JOIN runtimes r ON n.runtime_id = r.id
		JOIN servers s ON r.server_id = s.id
		WHERE s.code = $1 AND n.is_enabled = true AND n.deleted_at IS NULL
		ORDER BY n.created_at ASC
		LIMIT 1`
	var nodeID uuid.UUID
	err := r.pool.QueryRow(ctx, query, serverCode).Scan(&nodeID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &nodeID, nil
}
