package compat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ===== ClientProfileRepo =====

// ClientProfileRepo 客户端档案数据访问
type ClientProfileRepo struct {
	pool *pgxpool.Pool
}

func NewClientProfileRepo(pool *pgxpool.Pool) *ClientProfileRepo {
	return &ClientProfileRepo{pool: pool}
}

func (r *ClientProfileRepo) GetByCode(ctx context.Context, code string) (*ClientProfile, error) {
	query := `
		SELECT id, code, name, platform, min_version, max_version, status, notes, created_at, updated_at
		FROM client_profiles WHERE code = $1`
	p := &ClientProfile{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&p.ID, &p.Code, &p.Name, &p.Platform, &p.MinVersion, &p.MaxVersion, &p.Status, &p.Notes,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *ClientProfileRepo) ListAll(ctx context.Context, page, pageSize int, status, code string) ([]*ClientProfile, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if code != "" {
		where = append(where, fmt.Sprintf("code = $%d", argIdx))
		args = append(args, code)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM client_profiles WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, code, name, platform, min_version, max_version, status, notes, created_at, updated_at
		FROM client_profiles WHERE %s
		ORDER BY created_at ASC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var profiles []*ClientProfile
	for rows.Next() {
		p := &ClientProfile{}
		err := rows.Scan(
			&p.ID, &p.Code, &p.Name, &p.Platform, &p.MinVersion, &p.MaxVersion, &p.Status, &p.Notes,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		profiles = append(profiles, p)
	}
	return profiles, total, rows.Err()
}

// ===== CompatMatrixRepo =====

// CompatMatrixRepo 兼容矩阵数据访问
type CompatMatrixRepo struct {
	pool *pgxpool.Pool
}

func NewCompatMatrixRepo(pool *pgxpool.Pool) *CompatMatrixRepo {
	return &CompatMatrixRepo{pool: pool}
}

// GetByClientFeature 通过 client_code + feature_code 获取单条记录
func (r *CompatMatrixRepo) GetByClientFeature(ctx context.Context, clientCode, featureCode string) (*CompatMatrixEntry, error) {
	query := `
		SELECT id, client_code, feature_code, supported, supported_since_version, notes, created_at
		FROM client_compat_matrix WHERE client_code = $1 AND feature_code = $2`
	e := &CompatMatrixEntry{}
	err := r.pool.QueryRow(ctx, query, clientCode, featureCode).Scan(
		&e.ID, &e.ClientCode, &e.FeatureCode, &e.Supported, &e.SupportedSinceVersion, &e.Notes, &e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

// ListByClientCode 列出某客户端所有功能条目
func (r *CompatMatrixRepo) ListByClientCode(ctx context.Context, clientCode string) ([]*CompatMatrixEntry, error) {
	if clientCode == "" {
		return r.listAll(ctx)
	}
	query := `
		SELECT id, client_code, feature_code, supported, supported_since_version, notes, created_at
		FROM client_compat_matrix WHERE client_code = $1
		ORDER BY feature_code ASC`
	rows, err := r.pool.Query(ctx, query, clientCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMatrixRows(rows)
}

func (r *CompatMatrixRepo) listAll(ctx context.Context) ([]*CompatMatrixEntry, error) {
	query := `
		SELECT id, client_code, feature_code, supported, supported_since_version, notes, created_at
		FROM client_compat_matrix
		ORDER BY client_code ASC, feature_code ASC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMatrixRows(rows)
}

func scanMatrixRows(rows pgx.Rows) ([]*CompatMatrixEntry, error) {
	var entries []*CompatMatrixEntry
	for rows.Next() {
		e := &CompatMatrixEntry{}
		err := rows.Scan(
			&e.ID, &e.ClientCode, &e.FeatureCode, &e.Supported, &e.SupportedSinceVersion, &e.Notes, &e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ListAll 分页列表（可按 client_code/feature_code 过滤）
func (r *CompatMatrixRepo) ListAll(ctx context.Context, page, pageSize int, clientCode, featureCode string) ([]*CompatMatrixEntry, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if clientCode != "" {
		where = append(where, fmt.Sprintf("client_code = $%d", argIdx))
		args = append(args, clientCode)
		argIdx++
	}
	if featureCode != "" {
		where = append(where, fmt.Sprintf("feature_code = $%d", argIdx))
		args = append(args, featureCode)
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM client_compat_matrix WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, client_code, feature_code, supported, supported_since_version, notes, created_at
		FROM client_compat_matrix WHERE %s
		ORDER BY client_code ASC, feature_code ASC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*CompatMatrixEntry
	for rows.Next() {
		e := &CompatMatrixEntry{}
		err := rows.Scan(
			&e.ID, &e.ClientCode, &e.FeatureCode, &e.Supported, &e.SupportedSinceVersion, &e.Notes, &e.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// Upsert 批量 upsert（client_code + feature_code 唯一）
func (r *CompatMatrixRepo) Upsert(ctx context.Context, entry *CompatMatrixEntry) error {
	query := `
		INSERT INTO client_compat_matrix (client_code, feature_code, supported, supported_since_version, notes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (client_code, feature_code) DO UPDATE
		SET supported = EXCLUDED.supported,
		    supported_since_version = EXCLUDED.supported_since_version,
		    notes = EXCLUDED.notes`
	_, err := r.pool.Exec(ctx, query,
		entry.ClientCode, entry.FeatureCode, entry.Supported, entry.SupportedSinceVersion, entry.Notes,
	)
	return err
}

// ===== AdvancedPatchRepo =====

// AdvancedPatchRepo 高级补丁档案数据访问
type AdvancedPatchRepo struct {
	pool *pgxpool.Pool
}

func NewAdvancedPatchRepo(pool *pgxpool.Pool) *AdvancedPatchRepo {
	return &AdvancedPatchRepo{pool: pool}
}

func (r *AdvancedPatchRepo) GetByNodeID(ctx context.Context, nodeID uuid.UUID) ([]*AdvancedPatchProfile, error) {
	query := `
		SELECT id, node_id, runtime_type, patch_json, patch_target, allowed_keys, schema_version,
		       is_enabled, last_validated_at, last_validation_result, notes, created_at, updated_at
		FROM advanced_patch_profiles WHERE node_id = $1
		ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patches []*AdvancedPatchProfile
	for rows.Next() {
		p := &AdvancedPatchProfile{}
		err := rows.Scan(
			&p.ID, &p.NodeID, &p.RuntimeType, &p.PatchJSON, &p.PatchTarget, &p.AllowedKeys, &p.SchemaVersion,
			&p.IsEnabled, &p.LastValidatedAt, &p.LastValidationResult, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		patches = append(patches, p)
	}
	return patches, rows.Err()
}
