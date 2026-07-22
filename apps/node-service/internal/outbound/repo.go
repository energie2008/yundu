package outbound

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboundPolicyRepo 处理 outbound_policies 数据访问
type OutboundPolicyRepo struct {
	pool *pgxpool.Pool
}

func NewOutboundPolicyRepo(pool *pgxpool.Pool) *OutboundPolicyRepo {
	return &OutboundPolicyRepo{pool: pool}
}

const policyColumns = `id, node_id, policy_type, priority, config_json, routing_rules, is_enabled, created_at, updated_at`

func scanPolicy(row pgx.Row, p *OutboundPolicy) error {
	return row.Scan(
		&p.ID, &p.NodeID, &p.PolicyType, &p.Priority, &p.ConfigJSON,
		&p.RoutingRules, &p.IsEnabled, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *OutboundPolicyRepo) Create(ctx context.Context, p *OutboundPolicy) error {
	query := `
		INSERT INTO outbound_policies (node_id, policy_type, priority, config_json, routing_rules, is_enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.NodeID, p.PolicyType, p.Priority, p.ConfigJSON, p.RoutingRules, p.IsEnabled,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *OutboundPolicyRepo) GetByID(ctx context.Context, id uuid.UUID) (*OutboundPolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM outbound_policies WHERE id = $1`, policyColumns)
	p := &OutboundPolicy{}
	if err := scanPolicy(r.pool.QueryRow(ctx, query, id), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *OutboundPolicyRepo) Update(ctx context.Context, p *OutboundPolicy) error {
	query := `
		UPDATE outbound_policies SET
			policy_type = $2, priority = $3, config_json = $4, routing_rules = $5, is_enabled = $6, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.PolicyType, p.Priority, p.ConfigJSON, p.RoutingRules, p.IsEnabled,
	)
	return err
}

func (r *OutboundPolicyRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM outbound_policies WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *OutboundPolicyRepo) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*OutboundPolicy, error) {
	query := fmt.Sprintf(`SELECT %s FROM outbound_policies WHERE node_id = $1 ORDER BY priority ASC, created_at ASC`, policyColumns)
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*OutboundPolicy
	for rows.Next() {
		p := &OutboundPolicy{}
		if err := scanPolicy(rows, p); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// WarpProfileRepo 处理 warp_profiles 数据访问
type WarpProfileRepo struct {
	pool *pgxpool.Pool
}

func NewWarpProfileRepo(pool *pgxpool.Pool) *WarpProfileRepo {
	return &WarpProfileRepo{pool: pool}
}

const warpColumns = `id, code, name, warp_mode, endpoint, license_key, config_json, is_default, created_at, updated_at`

func scanWarp(row pgx.Row, w *WarpProfile) error {
	return row.Scan(
		&w.ID, &w.Code, &w.Name, &w.WarpMode, &w.Endpoint, &w.LicenseKey,
		&w.ConfigJSON, &w.IsDefault, &w.CreatedAt, &w.UpdatedAt,
	)
}

func (r *WarpProfileRepo) Create(ctx context.Context, w *WarpProfile) error {
	query := `
		INSERT INTO warp_profiles (code, name, warp_mode, endpoint, license_key, config_json, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		w.Code, w.Name, w.WarpMode, w.Endpoint, w.LicenseKey, w.ConfigJSON, w.IsDefault,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (r *WarpProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*WarpProfile, error) {
	query := fmt.Sprintf(`SELECT %s FROM warp_profiles WHERE id = $1`, warpColumns)
	w := &WarpProfile{}
	if err := scanWarp(r.pool.QueryRow(ctx, query, id), w); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

func (r *WarpProfileRepo) GetByCode(ctx context.Context, code string) (*WarpProfile, error) {
	query := fmt.Sprintf(`SELECT %s FROM warp_profiles WHERE code = $1`, warpColumns)
	w := &WarpProfile{}
	if err := scanWarp(r.pool.QueryRow(ctx, query, code), w); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

func (r *WarpProfileRepo) List(ctx context.Context) ([]*WarpProfile, error) {
	query := fmt.Sprintf(`SELECT %s FROM warp_profiles ORDER BY is_default DESC, created_at ASC`, warpColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*WarpProfile
	for rows.Next() {
		w := &WarpProfile{}
		if err := scanWarp(rows, w); err != nil {
			return nil, err
		}
		items = append(items, w)
	}
	return items, rows.Err()
}
