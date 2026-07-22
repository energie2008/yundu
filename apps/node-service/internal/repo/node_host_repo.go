package repo

// node_host_repo.go 实现 P2-3：节点多 Host 管理
//
// 一个节点可关联多个订阅域名（cdn/direct/tunnel），
// subscription-service 渲染时按 priority 展开为多条连接配置。

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeHost 节点订阅域名
type NodeHost struct {
	ID         int64      `json:"id"`
	NodeID     uuid.UUID  `json:"node_id"`
	Host       string     `json:"host"`
	HostType   string     `json:"host_type"`   // cdn / direct / tunnel
	Port       *int       `json:"port,omitempty"`
	Path       *string    `json:"path,omitempty"`
	SNI        *string    `json:"sni,omitempty"`
	HostHeader *string    `json:"host_header,omitempty"`
	Priority   int        `json:"priority"`
	IsEnabled  bool       `json:"is_enabled"`
	Remark     string     `json:"remark,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// NodeHostRepo 节点 Host 仓库
type NodeHostRepo struct {
	pool *pgxpool.Pool
}

func NewNodeHostRepo(pool *pgxpool.Pool) *NodeHostRepo {
	return &NodeHostRepo{pool: pool}
}

// ListByNode 列出节点的所有 host（按 priority 升序）
func (r *NodeHostRepo) ListByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeHost, error) {
	query := `
		SELECT id, node_id, host, host_type, port, path, sni, host_header,
		       priority, is_enabled, COALESCE(remark,''), created_at, updated_at
		FROM node_hosts
		WHERE node_id = $1
		ORDER BY priority ASC, id ASC`
	return r.queryList(ctx, query, nodeID)
}

// ListEnabledByNode 列出节点的已启用 host（按 priority 升序）
func (r *NodeHostRepo) ListEnabledByNode(ctx context.Context, nodeID uuid.UUID) ([]*NodeHost, error) {
	query := `
		SELECT id, node_id, host, host_type, port, path, sni, host_header,
		       priority, is_enabled, COALESCE(remark,''), created_at, updated_at
		FROM node_hosts
		WHERE node_id = $1 AND is_enabled = TRUE
		ORDER BY priority ASC, id ASC`
	return r.queryList(ctx, query, nodeID)
}

// ListEnabledByNodes 批量查询多节点的已启用 host（subscription-service 用）
// 返回 map[nodeID][]*NodeHost
func (r *NodeHostRepo) ListEnabledByNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]*NodeHost, error) {
	result := make(map[uuid.UUID][]*NodeHost)
	if len(nodeIDs) == 0 {
		return result, nil
	}
	query := `
		SELECT id, node_id, host, host_type, port, path, sni, host_header,
		       priority, is_enabled, COALESCE(remark,''), created_at, updated_at
		FROM node_hosts
		WHERE node_id = ANY($1) AND is_enabled = TRUE
		ORDER BY node_id, priority ASC, id ASC`
	rows, err := r.pool.Query(ctx, query, nodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		h := &NodeHost{}
		if err := rows.Scan(
			&h.ID, &h.NodeID, &h.Host, &h.HostType, &h.Port, &h.Path, &h.SNI, &h.HostHeader,
			&h.Priority, &h.IsEnabled, &h.Remark, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			continue
		}
		result[h.NodeID] = append(result[h.NodeID], h)
	}
	return result, rows.Err()
}

// Create 创建节点 host
func (r *NodeHostRepo) Create(ctx context.Context, h *NodeHost) error {
	query := `
		INSERT INTO node_hosts (node_id, host, host_type, port, path, sni, host_header, priority, is_enabled, remark)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		h.NodeID, h.Host, h.HostType, h.Port, h.Path, h.SNI, h.HostHeader,
		h.Priority, h.IsEnabled, h.Remark,
	).Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
}

// Update 更新节点 host
func (r *NodeHostRepo) Update(ctx context.Context, h *NodeHost) error {
	query := `
		UPDATE node_hosts
		SET host = $2, host_type = $3, port = $4, path = $5, sni = $6, host_header = $7,
		    priority = $8, is_enabled = $9, remark = $10, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, query,
		h.ID, h.Host, h.HostType, h.Port, h.Path, h.SNI, h.HostHeader,
		h.Priority, h.IsEnabled, h.Remark,
	).Scan(&h.UpdatedAt)
}

// Delete 删除节点 host
func (r *NodeHostRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM node_hosts WHERE id = $1", id)
	return err
}

// DeleteByNode 删除节点的所有 host（节点删除时级联）
func (r *NodeHostRepo) DeleteByNode(ctx context.Context, nodeID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM node_hosts WHERE node_id = $1", nodeID)
	return err
}

func (r *NodeHostRepo) queryList(ctx context.Context, query string, args ...interface{}) ([]*NodeHost, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*NodeHost
	for rows.Next() {
		h := &NodeHost{}
		if err := rows.Scan(
			&h.ID, &h.NodeID, &h.Host, &h.HostType, &h.Port, &h.Path, &h.SNI, &h.HostHeader,
			&h.Priority, &h.IsEnabled, &h.Remark, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			continue
		}
		list = append(list, h)
	}
	return list, rows.Err()
}
