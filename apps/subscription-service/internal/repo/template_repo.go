package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TemplateRepo struct {
	pool *pgxpool.Pool
}

func NewTemplateRepo(pool *pgxpool.Pool) *TemplateRepo {
	return &TemplateRepo{pool: pool}
}

func (r *TemplateRepo) Create(ctx context.Context, t *model.SubscriptionTemplate) error {
	query := `
		INSERT INTO subscription_templates (id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
		RETURNING created_at, updated_at`
	if t.SchemaVersion == "" {
		t.SchemaVersion = "v1"
	}
	if t.Status == "" {
		t.Status = model.TemplateStatusActive
	}
	if t.TemplateType == "" {
		t.TemplateType = "subscription"
	}
	return r.pool.QueryRow(ctx, query,
		t.ID, t.Code, t.Name, t.TargetClient, t.TemplateType, t.SchemaVersion, t.Content, t.Status, t.IsDefault, t.CreatedByAdmin,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
}

func (r *TemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.SubscriptionTemplate, error) {
	query := `
		SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
		FROM subscription_templates WHERE id = $1`
	t := &model.SubscriptionTemplate{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
		&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *TemplateRepo) GetByCode(ctx context.Context, code string) (*model.SubscriptionTemplate, error) {
	query := `
		SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
		FROM subscription_templates WHERE code = $1`
	t := &model.SubscriptionTemplate{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
		&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *TemplateRepo) GetDefaultByClientType(ctx context.Context, clientType model.ClientType) (*model.SubscriptionTemplate, error) {
	query := `
		SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
		FROM subscription_templates
		WHERE target_client = $1 AND is_default = true AND status = 'active'
		LIMIT 1`
	t := &model.SubscriptionTemplate{}
	err := r.pool.QueryRow(ctx, query, clientType).Scan(
		&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
		&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			query = `
				SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
				FROM subscription_templates
				WHERE target_client = $1 AND status = 'active'
				ORDER BY created_at ASC LIMIT 1`
			err = r.pool.QueryRow(ctx, query, clientType).Scan(
				&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
				&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
			)
			if err != nil {
				if err == pgx.ErrNoRows {
					return nil, nil
				}
				return nil, err
			}
			return t, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *TemplateRepo) ListByClientType(ctx context.Context, clientType model.ClientType) ([]*model.SubscriptionTemplate, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "status = 'active'")
	if clientType != "" {
		where = append(where, fmt.Sprintf("target_client = $%d", argIdx))
		args = append(args, clientType)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	query := fmt.Sprintf(`
		SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
		FROM subscription_templates WHERE %s
		ORDER BY is_default DESC, created_at DESC`, whereClause)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*model.SubscriptionTemplate
	for rows.Next() {
		t := &model.SubscriptionTemplate{}
		err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
			&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (r *TemplateRepo) ListAll(ctx context.Context, page, pageSize int, clientType model.ClientType) ([]*model.SubscriptionTemplate, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if clientType != "" {
		where = append(where, fmt.Sprintf("target_client = $%d", argIdx))
		args = append(args, clientType)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscription_templates WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, code, name, target_client, template_type, schema_version, content, status, is_default, created_by_admin_id, created_at, updated_at
		FROM subscription_templates WHERE %s
		ORDER BY is_default DESC, created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var templates []*model.SubscriptionTemplate
	for rows.Next() {
		t := &model.SubscriptionTemplate{}
		err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.TargetClient, &t.TemplateType, &t.SchemaVersion, &t.Content, &t.Status,
			&t.IsDefault, &t.CreatedByAdmin, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		templates = append(templates, t)
	}
	return templates, total, rows.Err()
}

func (r *TemplateRepo) SetDefault(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var targetClient model.ClientType
	query := `SELECT target_client FROM subscription_templates WHERE id = $1`
	err = tx.QueryRow(ctx, query, id).Scan(&targetClient)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return err
	}

	resetQuery := `UPDATE subscription_templates SET is_default = false, updated_at = now() WHERE target_client = $1`
	if _, err := tx.Exec(ctx, resetQuery, targetClient); err != nil {
		return err
	}

	setQuery := `UPDATE subscription_templates SET is_default = true, status = 'active', updated_at = now() WHERE id = $1`
	if _, err := tx.Exec(ctx, setQuery, id); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *TemplateRepo) Update(ctx context.Context, t *model.SubscriptionTemplate) error {
	query := `
		UPDATE subscription_templates
		SET code = $2, name = $3, target_client = $4, content = $5, status = $6, is_default = $7, updated_at = now()
		WHERE id = $1
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, query,
		t.ID, t.Code, t.Name, t.TargetClient, t.Content, t.Status, t.IsDefault,
	).Scan(&t.UpdatedAt)
}

func (r *TemplateRepo) IsDefault(t *model.SubscriptionTemplate) bool {
	return t.IsDefault
}
