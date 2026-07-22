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

type TicketRepo struct {
	pool *pgxpool.Pool
}

func NewTicketRepo(pool *pgxpool.Pool) *TicketRepo {
	return &TicketRepo{pool: pool}
}

func (r *TicketRepo) Create(ctx context.Context, t *model.Ticket) error {
	query := `
		INSERT INTO tickets (user_id, subject, description, category, priority, status,
		                     related_resource_type, related_resource_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`
	meta := t.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, query,
		t.UserID, t.Subject, t.Description, t.Category, t.Priority, model.TicketStatusOpen,
		t.RelatedResourceType, t.RelatedResourceID, meta,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *TicketRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Ticket, error) {
	query := `
		SELECT id, user_id, subject, description, category, priority, status,
		       assigned_admin_id, related_resource_type, related_resource_id,
		       reply_count, last_reply_at, closed_at, metadata,
		       created_at, updated_at, deleted_at
		FROM tickets WHERE id = $1 AND deleted_at IS NULL`
	t := &model.Ticket{}
	var relatedType *string
	var meta []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Category, &t.Priority, &t.Status,
		&t.AssignedAdminID, &relatedType, &t.RelatedResourceID,
		&t.ReplyCount, &t.LastReplyAt, &t.ClosedAt, &meta,
		&t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.RelatedResourceType = relatedType
	if meta != nil {
		t.Metadata = meta
	}
	return t, nil
}

func (r *TicketRepo) List(ctx context.Context, q model.TicketListQuery) ([]*model.Ticket, int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIdx := 1

	if q.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, q.Status)
		argIdx++
	}
	if q.Category != "" {
		where = append(where, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, q.Category)
		argIdx++
	}
	if q.Priority != "" {
		where = append(where, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, q.Priority)
		argIdx++
	}
	if q.UserID != "" {
		uid, err := uuid.Parse(q.UserID)
		if err == nil {
			where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
			args = append(args, uid)
			argIdx++
		}
	}
	if q.Email != "" {
		where = append(where, fmt.Sprintf("user_id IN (SELECT id FROM users WHERE email ILIKE $%d)", argIdx))
		args = append(args, "%"+q.Email+"%")
		argIdx++
	}
	if q.AssignedTo != "" {
		aid, err := uuid.Parse(q.AssignedTo)
		if err == nil {
			where = append(where, fmt.Sprintf("assigned_admin_id = $%d", argIdx))
			args = append(args, aid)
			argIdx++
		}
	}
	if q.Search != "" {
		where = append(where, fmt.Sprintf("(subject ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+q.Search+"%")
		argIdx++
	}

	whereSQL := strings.Join(where, " AND ")

	// count
	var total int
	countSQL := "SELECT COUNT(*) FROM tickets WHERE " + whereSQL
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// list
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
		SELECT id, user_id, subject, description, category, priority, status,
		       assigned_admin_id, related_resource_type, related_resource_id,
		       reply_count, last_reply_at, closed_at, metadata,
		       created_at, updated_at
		FROM tickets WHERE ` + whereSQL + `
		ORDER BY (CASE WHEN status IN ('open','in_progress') THEN 0 ELSE 1 END), priority DESC, created_at DESC
		LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*model.Ticket
	for rows.Next() {
		t := &model.Ticket{}
		var relatedType *string
		var meta []byte
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Category, &t.Priority, &t.Status,
			&t.AssignedAdminID, &relatedType, &t.RelatedResourceID,
			&t.ReplyCount, &t.LastReplyAt, &t.ClosedAt, &meta,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		t.RelatedResourceType = relatedType
		if meta != nil {
			t.Metadata = meta
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}

func (r *TicketRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.TicketStatus) error {
	query := `UPDATE tickets SET status = $2, updated_at = now()`
	if status == model.TicketStatusClosed || status == model.TicketStatusResolved {
		query += `, closed_at = now()`
	} else {
		query += `, closed_at = NULL`
	}
	query += ` WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

func (r *TicketRepo) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	setParts := []string{"updated_at = now()"}
	args := []interface{}{id}
	argIdx := 2
	allowed := map[string]bool{
		"status": true, "priority": true, "assigned_admin_id": true,
		"category": true, "subject": true, "description": true,
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
	query := fmt.Sprintf("UPDATE tickets SET %s WHERE id = $1 AND deleted_at IS NULL", strings.Join(setParts, ", "))
	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

func (r *TicketRepo) AssignAdmin(ctx context.Context, id, adminID uuid.UUID) error {
	query := `UPDATE tickets SET assigned_admin_id = $2, status = CASE WHEN status = 'open' THEN 'in_progress' ELSE status END, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, adminID)
	return err
}

// TicketReply operations

func (r *TicketRepo) CreateReply(ctx context.Context, rp *model.TicketReply) error {
	query := `
		INSERT INTO ticket_replies (ticket_id, author_id, author_type, content, attachments, is_internal)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	att := rp.Attachments
	if att == nil {
		att = json.RawMessage(`[]`)
	}
	return r.pool.QueryRow(ctx, query,
		rp.TicketID, rp.AuthorID, rp.AuthorType, rp.Content, att, rp.IsInternal,
	).Scan(&rp.ID, &rp.CreatedAt, &rp.UpdatedAt)
}

func (r *TicketRepo) IncrementReplyCount(ctx context.Context, ticketID uuid.UUID) error {
	query := `
		UPDATE tickets
		SET reply_count = reply_count + 1,
		    last_reply_at = now(),
		    updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, ticketID)
	return err
}

func (r *TicketRepo) ListReplies(ctx context.Context, ticketID uuid.UUID) ([]*model.TicketReply, error) {
	query := `
		SELECT id, ticket_id, author_id, author_type, content, attachments, is_internal, created_at, updated_at
		FROM ticket_replies
		WHERE ticket_id = $1
		ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.TicketReply
	for rows.Next() {
		rp := &model.TicketReply{}
		var att []byte
		if err := rows.Scan(
			&rp.ID, &rp.TicketID, &rp.AuthorID, &rp.AuthorType, &rp.Content, &att, &rp.IsInternal, &rp.CreatedAt, &rp.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if att != nil {
			rp.Attachments = att
		}
		items = append(items, rp)
	}
	return items, rows.Err()
}
