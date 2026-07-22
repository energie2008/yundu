package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditRepo struct {
	pool *pgxpool.Pool
}

func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

func (r *AuditRepo) Create(ctx context.Context, log *model.AuditLog) error {
	query := `
		INSERT INTO audit_logs (id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		                        request_id, ip_address, user_agent, before_json, after_json, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		log.ID, log.ActorType, log.ActorID, log.ActorDisplay, log.Action,
		log.ResourceType, log.ResourceID, log.RequestID, log.IPAddress,
		log.UserAgent, log.BeforeJSON, log.AfterJSON, log.Metadata,
	).Scan(&log.CreatedAt)
}

func (r *AuditRepo) List(ctx context.Context, page, pageSize int, actorType, actorID, resourceType, resourceID, action string) ([]*model.AuditLog, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if actorType != "" {
		where = append(where, fmt.Sprintf("actor_type = $%d", argIdx))
		args = append(args, actorType)
		argIdx++
	}
	if actorID != "" {
		aid, err := uuid.Parse(actorID)
		if err == nil {
			where = append(where, fmt.Sprintf("actor_id = $%d", argIdx))
			args = append(args, aid)
			argIdx++
		}
	}
	if resourceType != "" {
		where = append(where, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, resourceType)
		argIdx++
	}
	if resourceID != "" {
		rid, err := uuid.Parse(resourceID)
		if err == nil {
			where = append(where, fmt.Sprintf("resource_id = $%d", argIdx))
			args = append(args, rid)
			argIdx++
		}
	}
	if action != "" {
		where = append(where, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, action)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, actor_type, actor_id, actor_display, action, resource_type, resource_id,
		       request_id, ip_address::text, user_agent, before_json, after_json, metadata, created_at
		FROM audit_logs %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*model.AuditLog
	for rows.Next() {
		l := &model.AuditLog{}
		err := rows.Scan(
			&l.ID, &l.ActorType, &l.ActorID, &l.ActorDisplay, &l.Action,
			&l.ResourceType, &l.ResourceID, &l.RequestID, &l.IPAddress,
			&l.UserAgent, &l.BeforeJSON, &l.AfterJSON, &l.Metadata, &l.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
