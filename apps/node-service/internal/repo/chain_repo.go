package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChainRepo struct {
	pool *pgxpool.Pool
}

func NewChainRepo(pool *pgxpool.Pool) *ChainRepo {
	return &ChainRepo{pool: pool}
}

func (r *ChainRepo) Create(ctx context.Context, c *model.ProxyChain) error {
	query := `
		INSERT INTO proxy_chains (id, code, name, status, chain_mode, strategy, max_hops, health_policy_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		c.ID, c.Code, c.Name, c.Status, c.ChainMode, c.Strategy, c.MaxHops, c.HealthPolicyID, c.Metadata,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

func (r *ChainRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ProxyChain, error) {
	query := `
		SELECT id, code, name, status, chain_mode, strategy, max_hops, health_policy_id, metadata, created_at, updated_at
		FROM proxy_chains WHERE id = $1`
	c := &model.ProxyChain{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.Code, &c.Name, &c.Status, &c.ChainMode, &c.Strategy, &c.MaxHops, &c.HealthPolicyID, &c.Metadata, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *ChainRepo) List(ctx context.Context, page, pageSize int, status model.ChainStatus) ([]*model.ProxyChain, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(status))
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM proxy_chains WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, code, name, status, chain_mode, strategy, max_hops, health_policy_id, metadata, created_at, updated_at
		FROM proxy_chains WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var chains []*model.ProxyChain
	for rows.Next() {
		c := &model.ProxyChain{}
		err := rows.Scan(
			&c.ID, &c.Code, &c.Name, &c.Status, &c.ChainMode, &c.Strategy, &c.MaxHops, &c.HealthPolicyID, &c.Metadata, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		chains = append(chains, c)
	}
	return chains, total, rows.Err()
}

func (r *ChainRepo) AddHop(ctx context.Context, hop *model.ProxyChainHop) error {
	query := `
		INSERT INTO proxy_chain_hops (id, chain_id, hop_index, hop_type, upstream_node_id, upstream_runtime_id, outbound_protocol_type, outbound_config_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		hop.ID, hop.ChainID, hop.HopIndex, hop.HopType, hop.UpstreamNodeID, hop.UpstreamRuntimeID, hop.OutboundProtocolType, hop.OutboundConfigJSON,
	).Scan(&hop.CreatedAt)
}

func (r *ChainRepo) RemoveHop(ctx context.Context, chainID uuid.UUID, hopIndex int) error {
	query := `DELETE FROM proxy_chain_hops WHERE chain_id = $1 AND hop_index = $2`
	_, err := r.pool.Exec(ctx, query, chainID, hopIndex)
	return err
}

func (r *ChainRepo) ListHops(ctx context.Context, chainID uuid.UUID) ([]*model.ProxyChainHop, error) {
	query := `
		SELECT id, chain_id, hop_index, hop_type, upstream_node_id, upstream_runtime_id, outbound_protocol_type, outbound_config_json, created_at
		FROM proxy_chain_hops WHERE chain_id = $1 ORDER BY hop_index ASC`
	rows, err := r.pool.Query(ctx, query, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hops []*model.ProxyChainHop
	for rows.Next() {
		h := &model.ProxyChainHop{}
		err := rows.Scan(
			&h.ID, &h.ChainID, &h.HopIndex, &h.HopType, &h.UpstreamNodeID, &h.UpstreamRuntimeID, &h.OutboundProtocolType, &h.OutboundConfigJSON, &h.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		hops = append(hops, h)
	}
	return hops, rows.Err()
}

func (r *ChainRepo) BindNode(ctx context.Context, binding *model.NodeChainBinding) error {
	query := `
		INSERT INTO node_chain_bindings (node_id, chain_id, bind_mode, priority)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id, chain_id) DO UPDATE SET bind_mode = $3, priority = $4
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query, binding.NodeID, binding.ChainID, binding.BindMode, binding.Priority).Scan(&binding.CreatedAt)
}

func (r *ChainRepo) UnbindNode(ctx context.Context, nodeID, chainID uuid.UUID) error {
	query := `DELETE FROM node_chain_bindings WHERE node_id = $1 AND chain_id = $2`
	_, err := r.pool.Exec(ctx, query, nodeID, chainID)
	return err
}

// ReplaceNodeChainBindings 整体覆盖节点的代理链绑定列表
// 事务内删除旧绑定，插入新绑定。chainIDs 为空表示清空所有绑定。
// 优先级按数组顺序赋值（0,1,2...），bind_mode 默认 'route'
func (r *ChainRepo) ReplaceNodeChainBindings(ctx context.Context, nodeID uuid.UUID, chainIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM node_chain_bindings WHERE node_id = $1`, nodeID); err != nil {
		return err
	}

	for i, chainID := range chainIDs {
		bindMode := "route"
		priority := i
		if _, err := tx.Exec(ctx, `
			INSERT INTO node_chain_bindings (node_id, chain_id, bind_mode, priority)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (node_id, chain_id) DO UPDATE SET bind_mode = $3, priority = $4`,
			nodeID, chainID, bindMode, priority); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *ChainRepo) ListNodeBindings(ctx context.Context, chainID uuid.UUID) ([]*model.NodeChainBinding, error) {
	query := `
		SELECT node_id, chain_id, bind_mode, priority, created_at
		FROM node_chain_bindings WHERE chain_id = $1 ORDER BY priority ASC`
	rows, err := r.pool.Query(ctx, query, chainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []*model.NodeChainBinding
	for rows.Next() {
		b := &model.NodeChainBinding{}
		err := rows.Scan(&b.NodeID, &b.ChainID, &b.BindMode, &b.Priority, &b.CreatedAt)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, b)
	}
	return bindings, rows.Err()
}

// ListChainBindingsForNodes 批量查询多个节点已绑定的 chain_id 列表（避免 N+1）
// 返回 map[nodeID][]chainID
func (r *ChainRepo) ListChainBindingsForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(nodeIDs) == 0 {
		return make(map[uuid.UUID][]uuid.UUID), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT node_id, chain_id
		FROM node_chain_bindings
		WHERE node_id = ANY($1)
		ORDER BY node_id, priority ASC`, nodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]uuid.UUID)
	for rows.Next() {
		var nodeID, chainID uuid.UUID
		if err := rows.Scan(&nodeID, &chainID); err != nil {
			return nil, err
		}
		result[nodeID] = append(result[nodeID], chainID)
	}
	return result, rows.Err()
}
