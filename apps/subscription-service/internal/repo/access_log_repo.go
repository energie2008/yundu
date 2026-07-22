package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccessLogRepo struct {
	pool *pgxpool.Pool
}

func NewAccessLogRepo(pool *pgxpool.Pool) *AccessLogRepo {
	return &AccessLogRepo{pool: pool}
}

func (r *AccessLogRepo) Insert(ctx context.Context, l *model.SubscriptionAccessLog) error {
	return r.Create(ctx, l)
}

func (r *AccessLogRepo) Create(ctx context.Context, l *model.SubscriptionAccessLog) error {
	query := `
		INSERT INTO subscription_access_logs (id, token_id, user_id, client_type, request_ip, user_agent, response_status, template_code, generated_node_count, cache_hit, requested_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		RETURNING requested_at`
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, query,
		l.ID, l.TokenID, l.UserID, l.ClientType, l.RequestIP, l.UserAgent,
		l.ResponseStatus, l.TemplateCode, l.GeneratedNodeCount, l.CacheHit,
	).Scan(&l.RequestedAt)
}

func (r *AccessLogRepo) GetOverview(ctx context.Context, startTime, endTime time.Time) (*model.AccessLogOverview, error) {
	return r.GetStatsOverview(ctx, startTime, endTime)
}

func (r *AccessLogRepo) List(ctx context.Context, userID *uuid.UUID, page, pageSize int) ([]*model.SubscriptionAccessLog, int, error) {
	if userID != nil {
		return r.ListByUserID(ctx, *userID, page, pageSize, nil, nil)
	}
	return r.ListAll(ctx, page, pageSize)
}

func (r *AccessLogRepo) ListByTokenID(ctx context.Context, tokenID uuid.UUID, page, pageSize int) ([]*model.SubscriptionAccessLog, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, fmt.Sprintf("token_id = $%d", argIdx))
	args = append(args, tokenID)
	argIdx++

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscription_access_logs WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, token_id, user_id, client_type, request_ip::text, user_agent, response_status, template_code, generated_node_count, cache_hit, requested_at
		FROM subscription_access_logs WHERE %s
		ORDER BY requested_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*model.SubscriptionAccessLog
	for rows.Next() {
		l := &model.SubscriptionAccessLog{}
		err := rows.Scan(
			&l.ID, &l.TokenID, &l.UserID, &l.ClientType, &l.RequestIP, &l.UserAgent,
			&l.ResponseStatus, &l.TemplateCode, &l.GeneratedNodeCount, &l.CacheHit, &l.RequestedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

func (r *AccessLogRepo) ListByUserID(ctx context.Context, userID uuid.UUID, page, pageSize int, startTime, endTime *time.Time) ([]*model.SubscriptionAccessLog, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
	args = append(args, userID)
	argIdx++

	if startTime != nil {
		where = append(where, fmt.Sprintf("requested_at >= $%d", argIdx))
		args = append(args, *startTime)
		argIdx++
	}
	if endTime != nil {
		where = append(where, fmt.Sprintf("requested_at <= $%d", argIdx))
		args = append(args, *endTime)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscription_access_logs WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, token_id, user_id, client_type, request_ip::text, user_agent, response_status, template_code, generated_node_count, cache_hit, requested_at
		FROM subscription_access_logs WHERE %s
		ORDER BY requested_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*model.SubscriptionAccessLog
	for rows.Next() {
		l := &model.SubscriptionAccessLog{}
		err := rows.Scan(
			&l.ID, &l.TokenID, &l.UserID, &l.ClientType, &l.RequestIP, &l.UserAgent,
			&l.ResponseStatus, &l.TemplateCode, &l.GeneratedNodeCount, &l.CacheHit, &l.RequestedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

func (r *AccessLogRepo) GetStatsByUserID(ctx context.Context, userID uuid.UUID, days int) (*model.AccessLogStats, error) {
	startTime := time.Now().AddDate(0, 0, -days)

	var stats model.AccessLogStats
	var totalRequests, cacheHits int64

	query := `
		SELECT COUNT(*), COUNT(DISTINCT request_ip), COUNT(*) FILTER (WHERE cache_hit = true)
		FROM subscription_access_logs
		WHERE user_id = $1 AND requested_at >= $2`

	err := r.pool.QueryRow(ctx, query, userID, startTime).Scan(&totalRequests, &stats.UniqueIPs, &cacheHits)
	if err != nil {
		return nil, err
	}

	stats.TotalRequests = totalRequests
	if totalRequests > 0 {
		stats.CacheHitRate = float64(cacheHits) / float64(totalRequests) * 100
	} else {
		stats.CacheHitRate = 0
	}

	clientQuery := `
		SELECT client_type, COUNT(*) as count
		FROM subscription_access_logs
		WHERE user_id = $1 AND requested_at >= $2 AND client_type IS NOT NULL
		GROUP BY client_type
		ORDER BY count DESC`

	clientRows, err := r.pool.Query(ctx, clientQuery, userID, startTime)
	if err != nil {
		return nil, err
	}
	defer clientRows.Close()

	stats.ClientDistribution = make(map[string]int64)
	for clientRows.Next() {
		var clientType *string
		var count int64
		if err := clientRows.Scan(&clientType, &count); err != nil {
			return nil, err
		}
		if clientType != nil {
			stats.ClientDistribution[*clientType] = count
		}
	}
	if err := clientRows.Err(); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (r *AccessLogRepo) GetStatsOverview(ctx context.Context, startTime, endTime time.Time) (*model.AccessLogOverview, error) {
	var overview model.AccessLogOverview

	query := `
		SELECT COUNT(*), COUNT(DISTINCT user_id), COUNT(DISTINCT request_ip)
		FROM subscription_access_logs
		WHERE requested_at >= $1 AND requested_at <= $2`

	err := r.pool.QueryRow(ctx, query, startTime, endTime).Scan(&overview.TotalRequests, &overview.UniqueUsers, &overview.UniqueIPs)
	if err != nil {
		return nil, err
	}

	topClientsQuery := `
		SELECT client_type, COUNT(*) as count
		FROM subscription_access_logs
		WHERE requested_at >= $1 AND requested_at <= $2 AND client_type IS NOT NULL
		GROUP BY client_type
		ORDER BY count DESC
		LIMIT 5`

	clientRows, err := r.pool.Query(ctx, topClientsQuery, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer clientRows.Close()

	var topClients []*model.ClientStat
	for clientRows.Next() {
		var clientType string
		var count int64
		if err := clientRows.Scan(&clientType, &count); err != nil {
			return nil, err
		}
		topClients = append(topClients, &model.ClientStat{
			ClientType: clientType,
			Count:      count,
		})
	}
	if err := clientRows.Err(); err != nil {
		return nil, err
	}

	overview.TopClients = topClients
	return &overview, nil
}

func (r *AccessLogRepo) ListAll(ctx context.Context, page, pageSize int) ([]*model.SubscriptionAccessLog, int, error) {
	countQuery := `SELECT COUNT(*) FROM subscription_access_logs`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, token_id, user_id, client_type, request_ip::text, user_agent, response_status, template_code, generated_node_count, cache_hit, requested_at
		FROM subscription_access_logs
		ORDER BY requested_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := r.pool.Query(ctx, query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*model.SubscriptionAccessLog
	for rows.Next() {
		l := &model.SubscriptionAccessLog{}
		err := rows.Scan(
			&l.ID, &l.TokenID, &l.UserID, &l.ClientType, &l.RequestIP, &l.UserAgent,
			&l.ResponseStatus, &l.TemplateCode, &l.GeneratedNodeCount, &l.CacheHit, &l.RequestedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
