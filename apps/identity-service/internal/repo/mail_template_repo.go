package repo

import (
	"context"
	"fmt"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MailTemplateRepo 邮件模板仓储
type MailTemplateRepo struct {
	pool *pgxpool.Pool
}

func NewMailTemplateRepo(pool *pgxpool.Pool) *MailTemplateRepo {
	return &MailTemplateRepo{pool: pool}
}

func (r *MailTemplateRepo) GetByName(ctx context.Context, name string) (*model.MailTemplate, error) {
	query := `
		SELECT id, name, subject, body, is_builtin, enabled, created_at, updated_at
		FROM mail_templates WHERE name = $1`
	t := &model.MailTemplate{}
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&t.ID, &t.Name, &t.Subject, &t.Body, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *MailTemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.MailTemplate, error) {
	query := `
		SELECT id, name, subject, body, is_builtin, enabled, created_at, updated_at
		FROM mail_templates WHERE id = $1`
	t := &model.MailTemplate{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.Name, &t.Subject, &t.Body, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *MailTemplateRepo) List(ctx context.Context) ([]*model.MailTemplate, error) {
	query := `
		SELECT id, name, subject, body, is_builtin, enabled, created_at, updated_at
		FROM mail_templates ORDER BY name`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.MailTemplate
	for rows.Next() {
		t := &model.MailTemplate{}
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Subject, &t.Body, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (r *MailTemplateRepo) Update(ctx context.Context, id uuid.UUID, subject, body string) (*model.MailTemplate, error) {
	query := `
		UPDATE mail_templates SET subject = $2, body = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, name, subject, body, is_builtin, enabled, created_at, updated_at`
	t := &model.MailTemplate{}
	err := r.pool.QueryRow(ctx, query, id, subject, body).Scan(
		&t.ID, &t.Name, &t.Subject, &t.Body, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *MailTemplateRepo) SetEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE mail_templates SET enabled = $2, updated_at = now() WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("mail template not found")
	}
	return nil
}

// ListEnabled 获取所有启用的模板（用于缓存加载）
func (r *MailTemplateRepo) ListEnabled(ctx context.Context) ([]*model.MailTemplate, error) {
	query := `
		SELECT id, name, subject, body, is_builtin, enabled, created_at, updated_at
		FROM mail_templates WHERE enabled = true ORDER BY name`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*model.MailTemplate
	for rows.Next() {
		t := &model.MailTemplate{}
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Subject, &t.Body, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}
