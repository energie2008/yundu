package protocol

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProtocolRegistryRepo 处理 protocol_registry 数据访问
type ProtocolRegistryRepo struct {
	pool *pgxpool.Pool
}

func NewProtocolRegistryRepo(pool *pgxpool.Pool) *ProtocolRegistryRepo {
	return &ProtocolRegistryRepo{pool: pool}
}

const protocolColumns = `id, protocol_type, transport_type, security_type, schema_version, config_schema, description, is_enabled, created_at, updated_at`

func scanProtocol(row pgx.Row, p *ProtocolRegistry) error {
	return row.Scan(
		&p.ID, &p.ProtocolType, &p.TransportType, &p.SecurityType, &p.SchemaVersion,
		&p.ConfigSchema, &p.Description, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *ProtocolRegistryRepo) Create(ctx context.Context, p *ProtocolRegistry) error {
	query := `
		INSERT INTO protocol_registry (protocol_type, transport_type, security_type, schema_version, config_schema, description, is_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.ProtocolType, p.TransportType, p.SecurityType, p.SchemaVersion, p.ConfigSchema, p.Description, p.IsEnabled,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *ProtocolRegistryRepo) GetByID(ctx context.Context, id uuid.UUID) (*ProtocolRegistry, error) {
	query := fmt.Sprintf(`SELECT %s FROM protocol_registry WHERE id = $1`, protocolColumns)
	p := &ProtocolRegistry{}
	if err := scanProtocol(r.pool.QueryRow(ctx, query, id), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// FindByCombo 按 protocol_type + transport_type + security_type 查找（schema_version 默认取最新）
func (r *ProtocolRegistryRepo) FindByCombo(ctx context.Context, protocolType, transportType, securityType string) (*ProtocolRegistry, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM protocol_registry
		WHERE protocol_type = $1 AND transport_type = $2 AND security_type = $3 AND is_enabled = true
		ORDER BY schema_version DESC LIMIT 1`, protocolColumns)
	p := &ProtocolRegistry{}
	if err := scanProtocol(r.pool.QueryRow(ctx, query, protocolType, transportType, securityType), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *ProtocolRegistryRepo) Update(ctx context.Context, p *ProtocolRegistry) error {
	query := `
		UPDATE protocol_registry SET
			config_schema = $2, description = $3, is_enabled = $4, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, p.ID, p.ConfigSchema, p.Description, p.IsEnabled)
	return err
}

func (r *ProtocolRegistryRepo) List(ctx context.Context, page, pageSize int, query ProtocolListQuery) ([]*ProtocolRegistry, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if query.ProtocolType != "" {
		where = append(where, fmt.Sprintf("protocol_type = $%d", argIdx))
		args = append(args, query.ProtocolType)
		argIdx++
	}
	if query.TransportType != "" {
		where = append(where, fmt.Sprintf("transport_type = $%d", argIdx))
		args = append(args, query.TransportType)
		argIdx++
	}
	if query.SecurityType != "" {
		where = append(where, fmt.Sprintf("security_type = $%d", argIdx))
		args = append(args, query.SecurityType)
		argIdx++
	}
	if query.IsEnabled != "" {
		where = append(where, fmt.Sprintf("is_enabled = $%d", argIdx))
		args = append(args, query.IsEnabled == "true" || query.IsEnabled == "1")
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM protocol_registry WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM protocol_registry WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		protocolColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*ProtocolRegistry
	for rows.Next() {
		p := &ProtocolRegistry{}
		if err := scanProtocol(rows, p); err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	return items, total, rows.Err()
}

// ConfigTemplateRepo 处理 config_templates 数据访问
type ConfigTemplateRepo struct {
	pool *pgxpool.Pool
}

func NewConfigTemplateRepo(pool *pgxpool.Pool) *ConfigTemplateRepo {
	return &ConfigTemplateRepo{pool: pool}
}

const templateColumns = `id, code, name, runtime_type, template_type, content, variables_schema, is_default, created_at, updated_at`

func scanTemplate(row pgx.Row, t *ConfigTemplate) error {
	return row.Scan(
		&t.ID, &t.Code, &t.Name, &t.RuntimeType, &t.TemplateType,
		&t.Content, &t.VariablesSchema, &t.IsDefault, &t.CreatedAt, &t.UpdatedAt,
	)
}

func (r *ConfigTemplateRepo) GetByCode(ctx context.Context, code string) (*ConfigTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM config_templates WHERE code = $1`, templateColumns)
	t := &ConfigTemplate{}
	if err := scanTemplate(r.pool.QueryRow(ctx, query, code), t); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *ConfigTemplateRepo) GetByID(ctx context.Context, id uuid.UUID) (*ConfigTemplate, error) {
	query := fmt.Sprintf(`SELECT %s FROM config_templates WHERE id = $1`, templateColumns)
	t := &ConfigTemplate{}
	if err := scanTemplate(r.pool.QueryRow(ctx, query, id), t); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *ConfigTemplateRepo) Update(ctx context.Context, t *ConfigTemplate) error {
	query := `
		UPDATE config_templates SET
			name = $2, content = $3, variables_schema = $4, is_default = $5, updated_at = now()
		WHERE code = $1`
	_, err := r.pool.Exec(ctx, query, t.Code, t.Name, t.Content, t.VariablesSchema, t.IsDefault)
	return err
}

func (r *ConfigTemplateRepo) List(ctx context.Context, page, pageSize int, query TemplateListQuery) ([]*ConfigTemplate, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if query.RuntimeType != "" {
		where = append(where, fmt.Sprintf("runtime_type = $%d", argIdx))
		args = append(args, query.RuntimeType)
		argIdx++
	}
	if query.TemplateType != "" {
		where = append(where, fmt.Sprintf("template_type = $%d", argIdx))
		args = append(args, query.TemplateType)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM config_templates WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := fmt.Sprintf(`SELECT %s FROM config_templates WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		templateColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*ConfigTemplate
	for rows.Next() {
		t := &ConfigTemplate{}
		if err := scanTemplate(rows, t); err != nil {
			return nil, 0, err
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}
