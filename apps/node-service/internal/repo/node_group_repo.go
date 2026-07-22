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

// NodeGroupRepo 处理 node_groups 表的数据访问
type NodeGroupRepo struct {
	pool *pgxpool.Pool
}

func NewNodeGroupRepo(pool *pgxpool.Pool) *NodeGroupRepo {
	return &NodeGroupRepo{pool: pool}
}

func (r *NodeGroupRepo) Create(ctx context.Context, g *model.NodeGroup) error {
	query := `
		INSERT INTO node_groups (id, code, name, description, visibility, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		g.ID, g.Code, g.Name, g.Description, g.Visibility, g.SortOrder,
	).Scan(&g.CreatedAt, &g.UpdatedAt)
}

func (r *NodeGroupRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.NodeGroup, error) {
	query := `
		SELECT id, code, name, description, visibility, sort_order, created_at, updated_at
		FROM node_groups WHERE id = $1`
	g := &model.NodeGroup{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&g.ID, &g.Code, &g.Name, &g.Description, &g.Visibility, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *NodeGroupRepo) GetByCode(ctx context.Context, code string) (*model.NodeGroup, error) {
	query := `
		SELECT id, code, name, description, visibility, sort_order, created_at, updated_at
		FROM node_groups WHERE code = $1`
	g := &model.NodeGroup{}
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&g.ID, &g.Code, &g.Name, &g.Description, &g.Visibility, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *NodeGroupRepo) List(ctx context.Context, page, pageSize int, search string) ([]*model.NodeGroup, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if search != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM node_groups WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, code, name, description, visibility, sort_order, created_at, updated_at
		FROM node_groups WHERE %s
		ORDER BY sort_order ASC, created_at ASC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var groups []*model.NodeGroup
	for rows.Next() {
		g := &model.NodeGroup{}
		if err := rows.Scan(
			&g.ID, &g.Code, &g.Name, &g.Description, &g.Visibility, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		groups = append(groups, g)
	}
	return groups, total, rows.Err()
}

func (r *NodeGroupRepo) Update(ctx context.Context, g *model.NodeGroup) error {
	query := `
		UPDATE node_groups SET
			name = $2, description = $3, visibility = $4, sort_order = $5, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, g.ID, g.Name, g.Description, g.Visibility, g.SortOrder)
	return err
}

func (r *NodeGroupRepo) Delete(ctx context.Context, id uuid.UUID) error {
	// 检查是否有关联节点（多对多关联表）
	var nodeCount int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM node_group_members WHERE group_id = $1`, id).Scan(&nodeCount); err != nil {
		return err
	}
	if nodeCount > 0 {
		return fmt.Errorf("无法删除：仍有 %d 个节点关联此分组，请先解除关联", nodeCount)
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM node_groups WHERE id = $1`, id)
	return err
}

// CountNodes 统计分组下的节点数（基于多对多关联表）
func (r *NodeGroupRepo) CountNodes(ctx context.Context, groupID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM node_group_members ngm
		 JOIN nodes n ON n.id = ngm.node_id AND n.deleted_at IS NULL
		 WHERE ngm.group_id = $1`, groupID).Scan(&count)
	return count, err
}

// ListAll 返回所有分组（不分页，供下拉框使用）
func (r *NodeGroupRepo) ListAll(ctx context.Context) ([]*model.NodeGroup, error) {
	query := `
		SELECT id, code, name, description, visibility, sort_order, created_at, updated_at
		FROM node_groups ORDER BY sort_order ASC, created_at ASC`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*model.NodeGroup
	for rows.Next() {
		g := &model.NodeGroup{}
		if err := rows.Scan(
			&g.ID, &g.Code, &g.Name, &g.Description, &g.Visibility, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// BatchBindNodes 批量将节点绑定到此分组（多对多关联表）
// 一个节点可同时属于多个分组，绑定到新分组不会从原分组移除
// 仅绑定未删除的节点；已存在的关联记录会被 ON CONFLICT 跳过
// 返回实际新增的关联记录数
func (r *NodeGroupRepo) BatchBindNodes(ctx context.Context, groupID uuid.UUID, nodeIDs []uuid.UUID) (int, error) {
	if len(nodeIDs) == 0 {
		return 0, nil
	}
	// 构造批量 INSERT：VALUES ($1, $2), ($1, $3), ...
	var builder strings.Builder
	builder.WriteString(`INSERT INTO node_group_members (node_id, group_id) VALUES `)
	args := make([]interface{}, 0, len(nodeIDs)+1)
	args = append(args, groupID)
	for i, nid := range nodeIDs {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(fmt.Sprintf("($%d, $1)", i+2))
		args = append(args, nid)
	}
	builder.WriteString(` ON CONFLICT (node_id, group_id) DO NOTHING`)
	tag, err := r.pool.Exec(ctx, builder.String(), args...)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// BatchUnbindNodes 批量解绑节点（从关联表删除记录）
// 仅删除 nodeIDs 中当前属于此分组的关联记录
// 返回实际解绑的记录数
func (r *NodeGroupRepo) BatchUnbindNodes(ctx context.Context, groupID uuid.UUID, nodeIDs []uuid.UUID) (int, error) {
	if len(nodeIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM node_group_members
		 WHERE node_id = ANY($1) AND group_id = $2`,
		nodeIDs, groupID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ListNodeIDsByGroup 返回分组下的所有节点 ID 列表（基于多对多关联表）
func (r *NodeGroupRepo) ListNodeIDsByGroup(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ngm.node_id FROM node_group_members ngm
		 JOIN nodes n ON n.id = ngm.node_id AND n.deleted_at IS NULL
		 WHERE ngm.group_id = $1
		 ORDER BY n.name ASC`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListGroupIDsByNode 返回节点所属的所有分组 ID 列表（多对多）
// 用于节点编辑表单回显节点的所有分组
func (r *NodeGroupRepo) ListGroupIDsByNode(ctx context.Context, nodeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT group_id FROM node_group_members WHERE node_id = $1`,
		nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetNodeGroups 设置节点的所属分组集合（全量覆盖）
// 用于节点编辑表单保存：将节点的关联记录整体替换为传入的 groupIDs
// 同时同步 nodes.group_id 字段为传入的第一个分组（作为主分组，用于显示/排序）
func (r *NodeGroupRepo) SetNodeGroups(ctx context.Context, nodeID uuid.UUID, groupIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. 删除旧关联
	if _, err := tx.Exec(ctx,
		`DELETE FROM node_group_members WHERE node_id = $1`, nodeID); err != nil {
		return err
	}

	// 2. 插入新关联
	for _, gid := range groupIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO node_group_members (node_id, group_id) VALUES ($1, $2)
			 ON CONFLICT (node_id, group_id) DO NOTHING`, nodeID, gid); err != nil {
			return err
		}
	}

	// 3. 同步 nodes.group_id（取第一个作为主分组；为空则置 NULL）
	var primaryGroup interface{}
	if len(groupIDs) > 0 {
		primaryGroup = groupIDs[0]
	}
	if _, err := tx.Exec(ctx,
		`UPDATE nodes SET group_id = $2, updated_at = now() WHERE id = $1`,
		nodeID, primaryGroup); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
