package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AnnouncementRepo struct {
	pool *pgxpool.Pool
}

func NewAnnouncementRepo(pool *pgxpool.Pool) *AnnouncementRepo {
	return &AnnouncementRepo{pool: pool}
}

func (r *AnnouncementRepo) Create(ctx context.Context, a *model.Announcement) error {
	query := `
		INSERT INTO announcements (title, content, summary, type, status, target_audience,
		                            target_filter, effective_at, expires_at, pinned, created_by, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, view_count, read_count, created_at, updated_at`
	status := a.Status
	if status == "" {
		status = model.AnnouncementStatusDraft
	}
	audience := a.TargetAudience
	if audience == "" {
		audience = model.AnnouncementAudienceAll
	}
	filter := a.TargetFilter
	if filter == nil {
		filter = json.RawMessage(`{}`)
	}
	meta := a.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, query,
		a.Title, a.Content, a.Summary, a.Type, status, audience,
		filter, a.EffectiveAt, a.ExpiresAt, a.Pinned, a.CreatedBy, meta,
	).Scan(&a.ID, &a.ViewCount, &a.ReadCount, &a.CreatedAt, &a.UpdatedAt)
}

func (r *AnnouncementRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Announcement, error) {
	query := `
		SELECT id, title, content, summary, type, status, target_audience,
		       target_filter, effective_at, expires_at, pinned,
		       view_count, read_count, created_by, published_at, archived_at, metadata,
		       created_at, updated_at, deleted_at
		FROM announcements WHERE id = $1 AND deleted_at IS NULL`
	a := &model.Announcement{}
	var summary *string
	var filter, meta []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.Title, &a.Content, &summary, &a.Type, &a.Status, &a.TargetAudience,
		&filter, &a.EffectiveAt, &a.ExpiresAt, &a.Pinned,
		&a.ViewCount, &a.ReadCount, &a.CreatedBy, &a.PublishedAt, &a.ArchivedAt, &meta,
		&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.Summary = summary
	if filter != nil {
		a.TargetFilter = filter
	}
	if meta != nil {
		a.Metadata = meta
	}
	return a, nil
}

func (r *AnnouncementRepo) List(ctx context.Context, q model.AnnouncementListQuery) ([]*model.Announcement, int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if q.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Type != "" {
		where = append(where, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, q.Type)
		argIdx++
	}
	if q.Search != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR content ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q.Search+"%")
		argIdx++
	}
	if q.Pinned != nil {
		where = append(where, fmt.Sprintf("pinned = $%d", argIdx))
		args = append(args, *q.Pinned)
		argIdx++
	}

	whereSQL := strings.Join(where, " AND ")

	var total int
	countSQL := "SELECT COUNT(*) FROM announcements WHERE " + whereSQL
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
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
		SELECT id, title, content, summary, type, status, target_audience,
		       target_filter, effective_at, expires_at, pinned,
		       view_count, read_count, created_by, published_at, archived_at, metadata,
		       created_at, updated_at
		FROM announcements WHERE ` + whereSQL + `
		ORDER BY pinned DESC, created_at DESC
		LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Announcement
	for rows.Next() {
		a := &model.Announcement{}
		var summary *string
		var filter, meta []byte
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Content, &summary, &a.Type, &a.Status, &a.TargetAudience,
			&filter, &a.EffectiveAt, &a.ExpiresAt, &a.Pinned,
			&a.ViewCount, &a.ReadCount, &a.CreatedBy, &a.PublishedAt, &a.ArchivedAt, &meta,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		a.Summary = summary
		if filter != nil {
			a.TargetFilter = filter
		}
		if meta != nil {
			a.Metadata = meta
		}
		items = append(items, a)
	}
	return items, total, rows.Err()
}

func (r *AnnouncementRepo) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	setParts := []string{"updated_at = now()"}
	args := []interface{}{id}
	argIdx := 2
	allowed := map[string]bool{
		"title": true, "content": true, "summary": true, "type": true,
		"status": true, "target_audience": true, "pinned": true,
	}
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		setParts = append(setParts, fmt.Sprintf("%s = $%d", k, argIdx))
		args = append(args, v)
		argIdx++
	}
	if len(setParts) == 1 {
		return nil
	}
	// 若状态变为 published，自动设置 published_at
	statusVal, hasStatus := fields["status"]
	if hasStatus {
		if s, ok := statusVal.(string); ok && s == string(model.AnnouncementStatusPublished) {
			setParts = append(setParts, "published_at = COALESCE(published_at, now())")
		}
		if s, ok := statusVal.(model.AnnouncementStatus); ok && s == model.AnnouncementStatusPublished {
			setParts = append(setParts, "published_at = COALESCE(published_at, now())")
		}
	}
	query := fmt.Sprintf("UPDATE announcements SET %s WHERE id = $1 AND deleted_at IS NULL", strings.Join(setParts, ", "))
	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

func (r *AnnouncementRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE announcements SET deleted_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *AnnouncementRepo) IncViewCount(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE announcements SET view_count = view_count + 1 WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

// 已读记录

func (r *AnnouncementRepo) MarkRead(ctx context.Context, announcementID, userID uuid.UUID) error {
	query := `
		INSERT INTO announcement_reads (announcement_id, user_id) VALUES ($1, $2)
		ON CONFLICT (announcement_id, user_id) DO NOTHING`
	_, err := r.pool.Exec(ctx, query, announcementID, userID)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE announcements SET read_count = (SELECT COUNT(*) FROM announcement_reads WHERE announcement_id = $1) WHERE id = $1`, announcementID)
	return err
}

func (r *AnnouncementRepo) IsRead(ctx context.Context, announcementID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM announcement_reads WHERE announcement_id = $1 AND user_id = $2)`, announcementID, userID).Scan(&exists)
	return exists, err
}

func (r *AnnouncementRepo) ListPublishedForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*model.Announcement, int, error) {
	var total int
	countQuery := `
		SELECT COUNT(*) FROM announcements
		WHERE deleted_at IS NULL AND status = 'published'
		  AND (effective_at IS NULL OR effective_at <= now())
		  AND (expires_at IS NULL OR expires_at > now())`
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	listQuery := `
		SELECT a.id, a.title, a.content, a.summary, a.type, a.status, a.target_audience,
		       a.target_filter, a.effective_at, a.expires_at, a.pinned,
		       a.view_count, a.read_count, a.created_by, a.published_at, a.archived_at, a.metadata,
		       a.created_at, a.updated_at,
		       EXISTS(SELECT 1 FROM announcement_reads r WHERE r.announcement_id = a.id AND r.user_id = $1) AS is_read
		FROM announcements a
		WHERE a.deleted_at IS NULL AND a.status = 'published'
		  AND (a.effective_at IS NULL OR a.effective_at <= now())
		  AND (a.expires_at IS NULL OR a.expires_at > now())
		ORDER BY a.pinned DESC, a.published_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, listQuery, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Announcement
	for rows.Next() {
		a := &model.Announcement{}
		var summary *string
		var filter, meta []byte
		var isRead bool
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Content, &summary, &a.Type, &a.Status, &a.TargetAudience,
			&filter, &a.EffectiveAt, &a.ExpiresAt, &a.Pinned,
			&a.ViewCount, &a.ReadCount, &a.CreatedBy, &a.PublishedAt, &a.ArchivedAt, &meta,
			&a.CreatedAt, &a.UpdatedAt, &isRead,
		); err != nil {
			return nil, 0, err
		}
		a.Summary = summary
		if filter != nil {
			a.TargetFilter = filter
		}
		if meta != nil {
			a.Metadata = meta
		}
		items = append(items, a)
	}
	return items, total, rows.Err()
}
