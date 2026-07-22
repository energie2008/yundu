package repo

import (
	"context"
	"fmt"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubscribeTemplateRepo 是按名称索引的订阅模板仓储（对齐 Xboard v2_subscribe_templates）。
//
// 与 TemplateRepo（按 code+target_client 索引）互补：本仓储面向渲染器按内核/格式名
// 直接取模板内容的场景，对应 subscribe_template('singbox') helper。
type SubscribeTemplateRepo struct {
	pool *pgxpool.Pool
}

func NewSubscribeTemplateRepo(pool *pgxpool.Pool) *SubscribeTemplateRepo {
	return &SubscribeTemplateRepo{pool: pool}
}

const subscribeTemplateColumns = `id, name, content, is_builtin, enabled, created_at, updated_at`

func scanSubscribeTemplate(row pgx.Row) (*model.SubscribeTemplate, error) {
	t := &model.SubscribeTemplate{}
	err := row.Scan(
		&t.ID, &t.Name, &t.Content, &t.IsBuiltin, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetByName 按名称获取模板（仅返回 enabled 的模板）。
func (r *SubscribeTemplateRepo) GetByName(ctx context.Context, name string) (*model.SubscribeTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM subscribe_templates WHERE name = $1 AND enabled = true`, subscribeTemplateColumns)
	t, err := scanSubscribeTemplate(r.pool.QueryRow(ctx, query, name))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// GetByNameIncludeDisabled 按名称获取模板（含禁用项，供管理端使用）。
func (r *SubscribeTemplateRepo) GetByNameIncludeDisabled(ctx context.Context, name string) (*model.SubscribeTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM subscribe_templates WHERE name = $1`, subscribeTemplateColumns)
	t, err := scanSubscribeTemplate(r.pool.QueryRow(ctx, query, name))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// GetByID 按 ID 获取模板。
func (r *SubscribeTemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.SubscribeTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM subscribe_templates WHERE id = $1`, subscribeTemplateColumns)
	t, err := scanSubscribeTemplate(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// List 列出所有模板（含禁用项，按名称排序）。
func (r *SubscribeTemplateRepo) List(ctx context.Context) ([]*model.SubscribeTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM subscribe_templates ORDER BY name ASC`, subscribeTemplateColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*model.SubscribeTemplate
	for rows.Next() {
		t, err := scanSubscribeTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// UpdateContent 更新模板内容（管理端编辑）。内置模板允许改内容但不改名称。
func (r *SubscribeTemplateRepo) UpdateContent(ctx context.Context, id uuid.UUID, content string, enabled *bool) (*model.SubscribeTemplate, error) {
	query := `
		UPDATE subscribe_templates
		SET content = $2, enabled = COALESCE($3, enabled), updated_at = now()
		WHERE id = $1
		RETURNING ` + subscribeTemplateColumns
	t, err := scanSubscribeTemplate(r.pool.QueryRow(ctx, query, id, content, enabled))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}
