package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepo struct {
	pool *pgxpool.Pool
}

func NewNotificationRepo(pool *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{pool: pool}
}

func (r *NotificationRepo) Create(ctx context.Context, n *model.Notification) error {
	query := `
		INSERT INTO notifications (user_id, category, title, content, channel, status,
		                            priority, metadata, target_resource_type, target_resource_id,
		                            template_code, scheduled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, sent_at, created_at, updated_at`
	status := n.Status
	if status == "" {
		status = model.NotificationStatusPending
	}
	channel := n.Channel
	if channel == "" {
		channel = model.NotificationChannelInApp
	}
	priority := n.Priority
	if priority == "" {
		priority = model.NotificationPriorityNormal
	}
	meta := n.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, query,
		n.UserID, n.Category, n.Title, n.Content, channel, status,
		priority, meta, n.TargetResourceType, n.TargetResourceID,
		n.TemplateCode, n.ScheduledAt,
	).Scan(&n.ID, &n.SentAt, &n.CreatedAt, &n.UpdatedAt)
}

func (r *NotificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	query := `
		SELECT id, user_id, category, title, content, channel, status, priority,
		       metadata, target_resource_type, target_resource_id, template_code,
		       scheduled_at, sent_at, read_at, archived_at, created_at, updated_at
		FROM notifications WHERE id = $1`
	n := &model.Notification{}
	var meta []byte
	var trt *string
	var trid *uuid.UUID
	var tc *string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&n.ID, &n.UserID, &n.Category, &n.Title, &n.Content, &n.Channel, &n.Status, &n.Priority,
		&meta, &trt, &trid, &tc,
		&n.ScheduledAt, &n.SentAt, &n.ReadAt, &n.ArchivedAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	n.Metadata = meta
	n.TargetResourceType = trt
	n.TargetResourceID = trid
	n.TemplateCode = tc
	return n, nil
}

func (r *NotificationRepo) List(ctx context.Context, q model.NotificationListQuery) ([]*model.Notification, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if q.UserID != "" {
		uid, err := uuid.Parse(q.UserID)
		if err == nil {
			where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
			args = append(args, uid)
			argIdx++
		}
	}
	if q.Category != "" {
		where = append(where, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, q.Category)
		argIdx++
	}
	if q.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Channel != "" {
		where = append(where, fmt.Sprintf("channel = $%d", argIdx))
		args = append(args, q.Channel)
		argIdx++
	}
	if q.ExcludeArchived {
		where = append(where, "archived_at IS NULL")
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	listSQL := `
		SELECT id, user_id, category, title, content, channel, status, priority,
		       metadata, target_resource_type, target_resource_id, template_code,
		       scheduled_at, sent_at, read_at, archived_at, created_at, updated_at
		FROM notifications WHERE ` + whereSQL + `
		ORDER BY created_at DESC
		LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Notification
	for rows.Next() {
		n := &model.Notification{}
		var meta []byte
		var trt *string
		var trid *uuid.UUID
		var tc *string
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Category, &n.Title, &n.Content, &n.Channel, &n.Status, &n.Priority,
			&meta, &trt, &trid, &tc,
			&n.ScheduledAt, &n.SentAt, &n.ReadAt, &n.ArchivedAt, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		n.Metadata = meta
		n.TargetResourceType = trt
		n.TargetResourceID = trid
		n.TemplateCode = tc
		items = append(items, n)
	}
	return items, total, rows.Err()
}

func (r *NotificationRepo) MarkRead(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at = now(), status = 'read', updated_at = now() WHERE id = $1 AND read_at IS NULL`, id)
	return err
}

func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE notifications SET read_at = now(), status = 'read', updated_at = now() WHERE user_id = $1 AND read_at IS NULL AND archived_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *NotificationRepo) Archive(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET archived_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

func (r *NotificationRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notifications WHERE id = $1`, id)
	return err
}

// UnreadCount 获取用户未读数
func (r *NotificationRepo) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL AND archived_at IS NULL`, userID).Scan(&count)
	return count, err
}

// ExistsByUserTemplateSince 判断指定用户在 since 之后是否已存在某模板的通知（用于提醒类通知去重）。
func (r *NotificationRepo) ExistsByUserTemplateSince(ctx context.Context, userID uuid.UUID, templateCode string, since time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM notifications
		              WHERE user_id = $1 AND template_code = $2 AND created_at >= $3)`, userID, templateCode, since).Scan(&exists)
	return exists, err
}

// ListPendingScheduled 获取待调度发送的通知
func (r *NotificationRepo) ListPendingScheduled(ctx context.Context, limit int) ([]*model.Notification, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, user_id, category, title, content, channel, status, priority,
		       metadata, target_resource_type, target_resource_id, template_code,
		       scheduled_at, sent_at, read_at, archived_at, created_at, updated_at
		FROM notifications
		WHERE status = 'pending' AND scheduled_at IS NOT NULL AND scheduled_at <= $1
		ORDER BY scheduled_at ASC
		LIMIT $2`
	rows, err := r.pool.Query(ctx, query, time.Now(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.Notification
	for rows.Next() {
		n := &model.Notification{}
		var meta []byte
		var trt *string
		var trid *uuid.UUID
		var tc *string
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Category, &n.Title, &n.Content, &n.Channel, &n.Status, &n.Priority,
			&meta, &trt, &trid, &tc,
			&n.ScheduledAt, &n.SentAt, &n.ReadAt, &n.ArchivedAt, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.Metadata = meta
		n.TargetResourceType = trt
		n.TargetResourceID = trid
		n.TemplateCode = tc
		items = append(items, n)
	}
	return items, rows.Err()
}

func (r *NotificationRepo) MarkSent(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET status = 'sent', sent_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

func (r *NotificationRepo) MarkFailed(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET status = 'failed', updated_at = now() WHERE id = $1`, id)
	return err
}

// BatchCreateForAllUsers 为所有未删除的活跃用户批量创建一条相同内容的通知（用于公告广播）。
// 返回受影响的用户数。category/channel/status/priority 由调用方在 n 中给定。
func (r *NotificationRepo) BatchCreateForAllUsers(ctx context.Context, n *model.Notification) (int64, error) {
	meta := n.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	channel := n.Channel
	if channel == "" {
		channel = model.NotificationChannelInApp
	}
	priority := n.Priority
	if priority == "" {
		priority = model.NotificationPriorityNormal
	}
	status := n.Status
	if status == "" {
		status = model.NotificationStatusSent
	}
	query := `
		INSERT INTO notifications (user_id, category, title, content, channel, status,
		                            priority, metadata, target_resource_type, target_resource_id,
		                            template_code, scheduled_at)
		SELECT u.id, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		FROM users u
		WHERE u.deleted_at IS NULL AND u.status IN ('active', 'pending', 'expired')`
	tag, err := r.pool.Exec(ctx, query,
		n.Category, n.Title, n.Content, channel, status, priority, meta,
		n.TargetResourceType, n.TargetResourceID, n.TemplateCode, n.ScheduledAt,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ===== NotificationTemplateRepo =====

type NotificationTemplateRepo struct {
	pool *pgxpool.Pool
}

func NewNotificationTemplateRepo(pool *pgxpool.Pool) *NotificationTemplateRepo {
	return &NotificationTemplateRepo{pool: pool}
}

func (r *NotificationTemplateRepo) List(ctx context.Context) ([]*model.NotificationTemplate, error) {
	query := `
		SELECT id, code, name, description, category, channel, title_template, body_template, variables, enabled, created_at, updated_at
		FROM notification_templates
		ORDER BY category, code`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.NotificationTemplate
	for rows.Next() {
		t := &model.NotificationTemplate{}
		var vars []byte
		if err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Category, &t.Channel, &t.TitleTemplate, &t.BodyTemplate, &vars, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if vars != nil {
			t.Variables = vars
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *NotificationTemplateRepo) GetByCode(ctx context.Context, code string) (*model.NotificationTemplate, error) {
	query := `
		SELECT id, code, name, description, category, channel, title_template, body_template, variables, enabled, created_at, updated_at
		FROM notification_templates WHERE code = $1`
	t := &model.NotificationTemplate{}
	var vars []byte
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.Category, &t.Channel, &t.TitleTemplate, &t.BodyTemplate, &vars, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if vars != nil {
		t.Variables = vars
	}
	return t, nil
}

func (r *NotificationTemplateRepo) Upsert(ctx context.Context, t *model.NotificationTemplate) error {
	query := `
		INSERT INTO notification_templates (code, name, description, category, channel, title_template, body_template, variables, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (code) DO UPDATE SET
		    name = EXCLUDED.name,
		    description = EXCLUDED.description,
		    category = EXCLUDED.category,
		    channel = EXCLUDED.channel,
		    title_template = EXCLUDED.title_template,
		    body_template = EXCLUDED.body_template,
		    variables = EXCLUDED.variables,
		    enabled = EXCLUDED.enabled,
		    updated_at = now()
		RETURNING id, created_at, updated_at`
	vars := t.Variables
	if vars == nil {
		vars = json.RawMessage(`[]`)
	}
	return r.pool.QueryRow(ctx, query,
		t.Code, t.Name, t.Description, t.Category, t.Channel, t.TitleTemplate, t.BodyTemplate, vars, t.Enabled,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *NotificationTemplateRepo) SetEnabled(ctx context.Context, code string, enabled bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE notification_templates SET enabled = $2, updated_at = now() WHERE code = $1`, code, enabled)
	return err
}

func (r *NotificationTemplateRepo) Delete(ctx context.Context, code string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notification_templates WHERE code = $1`, code)
	return err
}
