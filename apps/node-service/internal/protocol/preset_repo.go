package protocol

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProtocolPresetRepo struct {
	pool *pgxpool.Pool
}

func NewProtocolPresetRepo(pool *pgxpool.Pool) *ProtocolPresetRepo {
	return &ProtocolPresetRepo{pool: pool}
}

const presetColumns = `id, code, name, badge, description, protocol_type, transport_type, security_type,
	min_xray_version, min_singbox_version, client_support, kernel_compat, base_spec, default_config,
	recommendations, warnings, recommended_port, icon, sort_order, is_recommended, is_enabled, is_builtin,
	updated_from_upstream, deprecated_at, created_at, updated_at`

func scanPreset(row pgx.Row, p *ProtocolPreset) error {
	return row.Scan(
		&p.ID, &p.Code, &p.Name, &p.Badge, &p.Description, &p.ProtocolType, &p.TransportType, &p.SecurityType,
		&p.MinXrayVersion, &p.MinSingboxVersion, &p.ClientSupport, &p.KernelCompat, &p.BaseSpec, &p.DefaultConfig,
		&p.Recommendations, &p.Warnings, &p.RecommendedPort, &p.Icon, &p.SortOrder, &p.IsRecommended, &p.IsEnabled, &p.IsBuiltin,
		&p.UpdatedFromUpstream, &p.DeprecatedAt, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *ProtocolPresetRepo) Create(ctx context.Context, p *ProtocolPreset) error {
	query := `
		INSERT INTO protocol_presets (
			code, name, badge, description, protocol_type, transport_type, security_type,
			min_xray_version, min_singbox_version, client_support, kernel_compat, base_spec, default_config,
			recommendations, warnings, recommended_port, icon, sort_order, is_recommended, is_enabled, is_builtin
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.Code, p.Name, p.Badge, p.Description, p.ProtocolType, p.TransportType, p.SecurityType,
		p.MinXrayVersion, p.MinSingboxVersion, p.ClientSupport, p.KernelCompat, p.BaseSpec, p.DefaultConfig,
		p.Recommendations, p.Warnings, p.RecommendedPort, p.Icon, p.SortOrder, p.IsRecommended, p.IsEnabled, p.IsBuiltin,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *ProtocolPresetRepo) GetByID(ctx context.Context, id uuid.UUID) (*ProtocolPreset, error) {
	query := fmt.Sprintf(`SELECT %s FROM protocol_presets WHERE id = $1`, presetColumns)
	p := &ProtocolPreset{}
	if err := scanPreset(r.pool.QueryRow(ctx, query, id), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *ProtocolPresetRepo) GetByCode(ctx context.Context, code string) (*ProtocolPreset, error) {
	query := fmt.Sprintf(`SELECT %s FROM protocol_presets WHERE code = $1`, presetColumns)
	p := &ProtocolPreset{}
	if err := scanPreset(r.pool.QueryRow(ctx, query, code), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *ProtocolPresetRepo) Update(ctx context.Context, p *ProtocolPreset) error {
	query := `
		UPDATE protocol_presets SET
			name = $2, badge = $3, description = $4,
			min_xray_version = $5, min_singbox_version = $6, client_support = $7,
			kernel_compat = $8, base_spec = $9, default_config = $10,
			recommendations = $11, warnings = $12, recommended_port = $13,
			icon = $14, sort_order = $15, is_recommended = $16, is_enabled = $17,
			deprecated_at = $18, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.Badge, p.Description,
		p.MinXrayVersion, p.MinSingboxVersion, p.ClientSupport,
		p.KernelCompat, p.BaseSpec, p.DefaultConfig,
		p.Recommendations, p.Warnings, p.RecommendedPort,
		p.Icon, p.SortOrder, p.IsRecommended, p.IsEnabled,
		p.DeprecatedAt,
	)
	return err
}

func (r *ProtocolPresetRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM protocol_presets WHERE id = $1`, id)
	return err
}

func (r *ProtocolPresetRepo) List(ctx context.Context, page, pageSize int, query PresetListQuery) ([]*ProtocolPreset, int, error) {
	items, err := r.ListAll(ctx)
	if err != nil {
		return nil, 0, err
	}
	return items, len(items), nil
}

func (r *ProtocolPresetRepo) ListAll(ctx context.Context) ([]*ProtocolPreset, error) {
	query := fmt.Sprintf(`SELECT %s FROM protocol_presets ORDER BY sort_order ASC, created_at ASC`, presetColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ProtocolPreset
	for rows.Next() {
		p := &ProtocolPreset{}
		if err := scanPreset(rows, p); err != nil {
			return nil, err
		}
		if p.ClientSupport == nil {
			p.ClientSupport = []string{}
		}
		if p.Recommendations == nil {
			p.Recommendations = []string{}
		}
		if p.Warnings == nil {
			p.Warnings = []string{}
		}
		if p.BaseSpec == nil {
			p.BaseSpec = Map{}
		}
		if p.DefaultConfig == nil {
			p.DefaultConfig = Map{}
		}
		if p.KernelCompat == "" {
			p.KernelCompat = CompatBoth
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *ProtocolPresetRepo) ListEnabled(ctx context.Context) ([]*ProtocolPreset, error) {
	query := fmt.Sprintf(`SELECT %s FROM protocol_presets WHERE is_enabled = true ORDER BY sort_order ASC, created_at ASC`, presetColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ProtocolPreset
	for rows.Next() {
		p := &ProtocolPreset{}
		if err := scanPreset(rows, p); err != nil {
			return nil, err
		}
		if p.ClientSupport == nil {
			p.ClientSupport = []string{}
		}
		if p.Recommendations == nil {
			p.Recommendations = []string{}
		}
		if p.Warnings == nil {
			p.Warnings = []string{}
		}
		if p.BaseSpec == nil {
			p.BaseSpec = Map{}
		}
		if p.DefaultConfig == nil {
			p.DefaultConfig = Map{}
		}
		if p.KernelCompat == "" {
			p.KernelCompat = CompatBoth
		}
		items = append(items, p)
	}
	return items, rows.Err()
}
