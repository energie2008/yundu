package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const nodeSelectColumns = `id, code, name, runtime_id, region_id, group_id, node_type, protocol_type, transport_type,
		security_type, address, port, server_port, reality_server_name, sni, alpn, path, host_header, flow, is_enabled, is_visible, allow_udp,
		speed_limit_mbps, device_limit, padding_scheme, rate_time_enable, rate_time_ranges, transfer_enable_bytes,
		traffic_rate, priority, capacity_score, protocol_schema_version, exposure_mode, downstream_exposure_mode, is_split_mode,
		config_json, tags, metadata,
		last_published_version, created_at, updated_at, deleted_at`

func scanNode(scanner pgx.Row) (*model.Node, error) {
	n := &model.Node{}
	err := scanner.Scan(
		&n.ID, &n.Code, &n.Name, &n.RuntimeID, &n.RegionID, &n.GroupID, &n.NodeType, &n.ProtocolType, &n.TransportType,
		&n.SecurityType, &n.Address, &n.Port, &n.ServerPort, &n.RealityServerName, &n.SNI, &n.ALPN, &n.Path, &n.HostHeader, &n.Flow, &n.IsEnabled, &n.IsVisible, &n.AllowUDP,
		&n.SpeedLimitMbps, &n.DeviceLimit, &n.PaddingScheme, &n.RateTimeEnable, &n.RateTimeRanges, &n.TransferEnableBytes,
		&n.TrafficRate, &n.Priority, &n.CapacityScore, &n.ProtocolSchemaVersion, &n.ExposureMode, &n.DownstreamExposureMode, &n.IsSplitMode,
		&n.ConfigJSON, &n.Tags, &n.Metadata,
		&n.LastPublishedVersion, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return n, nil
}

type NodeRepo struct {
	pool *pgxpool.Pool
}

func NewNodeRepo(pool *pgxpool.Pool) *NodeRepo {
	return &NodeRepo{pool: pool}
}

func (r *NodeRepo) Create(ctx context.Context, n *model.Node) error {
	query := `
		INSERT INTO nodes (id, code, name, runtime_id, region_id, group_id, node_type, protocol_type, transport_type,
			security_type, address, port, server_port, reality_server_name, sni, alpn, path, host_header, flow, is_enabled, is_visible, allow_udp,
			speed_limit_mbps, device_limit, padding_scheme, rate_time_enable, rate_time_ranges, transfer_enable_bytes,
			traffic_rate, priority, capacity_score, protocol_schema_version, exposure_mode, downstream_exposure_mode, is_split_mode,
			config_json, tags, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			$22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		n.ID, n.Code, n.Name, n.RuntimeID, n.RegionID, n.GroupID, n.NodeType, n.ProtocolType, n.TransportType,
		n.SecurityType, n.Address, n.Port, n.ServerPort, n.RealityServerName, n.SNI, n.ALPN, n.Path, n.HostHeader, n.Flow, n.IsEnabled, n.IsVisible, n.AllowUDP,
		n.SpeedLimitMbps, n.DeviceLimit, n.PaddingScheme, n.RateTimeEnable, n.RateTimeRanges, n.TransferEnableBytes,
		n.TrafficRate, n.Priority, n.CapacityScore, n.ProtocolSchemaVersion, n.ExposureMode, n.DownstreamExposureMode, n.IsSplitMode,
		n.ConfigJSON, n.Tags, n.Metadata,
	).Scan(&n.CreatedAt, &n.UpdatedAt)
}

func (r *NodeRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Node, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM nodes WHERE id = $1 AND deleted_at IS NULL`, nodeSelectColumns)
	n, err := scanNode(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return n, nil
}

func (r *NodeRepo) GetByCode(ctx context.Context, code string) (*model.Node, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM nodes WHERE code = $1 AND deleted_at IS NULL`, nodeSelectColumns)
	n, err := scanNode(r.pool.QueryRow(ctx, query, code))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return n, nil
}

func (r *NodeRepo) List(ctx context.Context, page, pageSize int, protocolType, regionID, groupID, search string, isEnabled *bool) ([]*model.Node, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	where = append(where, "deleted_at IS NULL")
	if protocolType != "" {
		where = append(where, fmt.Sprintf("protocol_type = $%d", argIdx))
		args = append(args, protocolType)
		argIdx++
	}
	if regionID != "" {
		where = append(where, fmt.Sprintf("region_id = $%d", argIdx))
		rid, err := uuid.Parse(regionID)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, rid)
		argIdx++
	}
	if groupID != "" {
		where = append(where, fmt.Sprintf("group_id = $%d", argIdx))
		gid, err := uuid.Parse(groupID)
		if err != nil {
			return nil, 0, err
		}
		args = append(args, gid)
		argIdx++
	}
	if isEnabled != nil {
		where = append(where, fmt.Sprintf("is_enabled = $%d", argIdx))
		args = append(args, *isEnabled)
		argIdx++
	}
	if search != "" {
		where = append(where, fmt.Sprintf("(code ILIKE $%d OR name ILIKE $%d OR address ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM nodes WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM nodes WHERE %s
		ORDER BY priority ASC, created_at DESC
		LIMIT $%d OFFSET $%d`, nodeSelectColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		n := &model.Node{}
		err := rows.Scan(
			&n.ID, &n.Code, &n.Name, &n.RuntimeID, &n.RegionID, &n.GroupID, &n.NodeType, &n.ProtocolType, &n.TransportType,
			&n.SecurityType, &n.Address, &n.Port, &n.ServerPort, &n.RealityServerName, &n.SNI, &n.ALPN, &n.Path, &n.HostHeader, &n.Flow, &n.IsEnabled, &n.IsVisible, &n.AllowUDP,
			&n.SpeedLimitMbps, &n.DeviceLimit, &n.PaddingScheme, &n.RateTimeEnable, &n.RateTimeRanges, &n.TransferEnableBytes,
			&n.TrafficRate, &n.Priority, &n.CapacityScore, &n.ProtocolSchemaVersion, &n.ExposureMode, &n.DownstreamExposureMode, &n.IsSplitMode,
			&n.ConfigJSON, &n.Tags, &n.Metadata,
			&n.LastPublishedVersion, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, n)
	}
	return nodes, total, rows.Err()
}

// ListPlanCodesForNodes 批量查询多个节点的已绑定套餐 code（避免 N+1 查询）
// 返回 map[nodeID][]{plan_code}
//
// 节点绑定套餐的两种方式（与 subscription-service ListVisibleNodes 对齐）：
//  1. plan_nodes 表直接关联（向后兼容）
//  2. plans.group_id → nodes.group_id 群组关联（标准模式，推荐）
//
// 两种方式取并集（DISTINCT 去重），确保两种绑定方式的节点都能正确显示已绑定套餐。
func (r *NodeRepo) ListPlanCodesForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	if len(nodeIDs) == 0 {
		return make(map[uuid.UUID][]string), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT node_id, code FROM (
			-- 方式1: plan_nodes 表直接关联
			SELECT pn.node_id, p.code
			FROM plan_nodes pn
			JOIN plans p ON p.id = pn.plan_id
			WHERE pn.node_id = ANY($1)
			UNION
			-- 方式2: plans.group_id → nodes.group_id 群组关联
			SELECT n.id AS node_id, p.code
			FROM nodes n
			JOIN plans p ON p.group_id = n.group_id AND p.deleted_at IS NULL
			WHERE n.id = ANY($1) AND n.group_id IS NOT NULL
		) t
		ORDER BY node_id, code`, nodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]string)
	for rows.Next() {
		var nodeID uuid.UUID
		var code string
		if err := rows.Scan(&nodeID, &code); err != nil {
			return nil, err
		}
		result[nodeID] = append(result[nodeID], code)
	}
	return result, rows.Err()
}

// BindNodeToPlans D9 修复: 将节点绑定到多个计划（插入 plan_nodes 表）。
// 使用 ON CONFLICT DO NOTHING 避免重复绑定时报错。
func (r *NodeRepo) BindNodeToPlans(ctx context.Context, nodeID uuid.UUID, planIDs []uuid.UUID) error {
	if len(planIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	query := `INSERT INTO plan_nodes (plan_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	for _, planID := range planIDs {
		batch.Queue(query, planID, nodeID)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range planIDs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ListServerInfoForRuntimes 批量查询多个 runtimeID 对应的 server 简要信息（避免 N+1）
// 返回 map[runtimeID]*ServerBrief
func (r *NodeRepo) ListServerInfoForRuntimes(ctx context.Context, runtimeIDs []uuid.UUID) (map[uuid.UUID]*model.ServerBrief, error) {
	if len(runtimeIDs) == 0 {
		return make(map[uuid.UUID]*model.ServerBrief), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id AS runtime_id, s.id, s.code, s.name, s.host
		FROM runtimes r
		JOIN servers s ON s.id = r.server_id
		WHERE r.id = ANY($1)`, runtimeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID]*model.ServerBrief)
	for rows.Next() {
		var runtimeID uuid.UUID
		b := &model.ServerBrief{}
		if err := rows.Scan(&runtimeID, &b.ID, &b.Code, &b.Name, &b.Host); err != nil {
			return nil, err
		}
		result[runtimeID] = b
	}
	return result, rows.Err()
}

// ListNodeGroupBriefs 批量查询多个 groupID 的分组简要信息（避免 N+1）
// 返回 map[groupID]*NodeGroupBrief
func (r *NodeRepo) ListNodeGroupBriefs(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID]*model.NodeGroupBrief, error) {
	if len(groupIDs) == 0 {
		return make(map[uuid.UUID]*model.NodeGroupBrief), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, code, name FROM node_groups WHERE id = ANY($1)`, groupIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID]*model.NodeGroupBrief)
	for rows.Next() {
		b := &model.NodeGroupBrief{}
		if err := rows.Scan(&b.ID, &b.Code, &b.Name); err != nil {
			return nil, err
		}
		result[b.ID] = b
	}
	return result, rows.Err()
}

// ListGroupsForNodes 批量查询多个节点的所有所属分组（多对多，避免 N+1）
// 返回 map[nodeID][]NodeGroupBrief
// 用于节点列表/详情回显节点的多分组关联
func (r *NodeRepo) ListGroupsForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]*model.NodeGroupBrief, error) {
	result := make(map[uuid.UUID][]*model.NodeGroupBrief)
	if len(nodeIDs) == 0 {
		return result, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT ngm.node_id, ng.id, ng.code, ng.name
		FROM node_group_members ngm
		JOIN node_groups ng ON ng.id = ngm.group_id
		WHERE ngm.node_id = ANY($1)
		ORDER BY ngm.node_id, ng.sort_order ASC, ng.name ASC`, nodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var nodeID uuid.UUID
		b := &model.NodeGroupBrief{}
		if err := rows.Scan(&nodeID, &b.ID, &b.Code, &b.Name); err != nil {
			return nil, err
		}
		result[nodeID] = append(result[nodeID], b)
	}
	return result, rows.Err()
}

func (r *NodeRepo) Update(ctx context.Context, n *model.Node) error {
	query := `
		UPDATE nodes SET
			code = $2, name = $3, runtime_id = $4, region_id = $5, group_id = $6, node_type = $7, protocol_type = $8, transport_type = $9,
			security_type = $10, address = $11, port = $12, server_port = $13, reality_server_name = $14, sni = $15, alpn = $16, path = $17,
			host_header = $18, flow = $19, is_enabled = $20, is_visible = $21, allow_udp = $22, speed_limit_mbps = $23,
			device_limit = $24, padding_scheme = $25, rate_time_enable = $26, rate_time_ranges = $27, transfer_enable_bytes = $28,
			traffic_rate = $29, priority = $30, capacity_score = $31, exposure_mode = $32, downstream_exposure_mode = $33, is_split_mode = $34,
			config_json = $35, tags = $36, metadata = $37,
			updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		n.ID, n.Code, n.Name, n.RuntimeID, n.RegionID, n.GroupID, n.NodeType, n.ProtocolType, n.TransportType,
		n.SecurityType, n.Address, n.Port, n.ServerPort, n.RealityServerName, n.SNI, n.ALPN, n.Path, n.HostHeader, n.Flow,
		n.IsEnabled, n.IsVisible, n.AllowUDP, n.SpeedLimitMbps,
		n.DeviceLimit, n.PaddingScheme, n.RateTimeEnable, n.RateTimeRanges, n.TransferEnableBytes,
		n.TrafficRate, n.Priority, n.CapacityScore, n.ExposureMode, n.DownstreamExposureMode, n.IsSplitMode,
		n.ConfigJSON, n.Tags, n.Metadata,
	)
	return err
}

func (r *NodeRepo) UpdateStatus(ctx context.Context, id uuid.UUID, isEnabled bool) error {
	query := `UPDATE nodes SET is_enabled = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, isEnabled)
	return err
}

func (r *NodeRepo) UpdatePublishedVersion(ctx context.Context, id uuid.UUID, version int64) error {
	query := `UPDATE nodes SET last_published_version = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, version)
	return err
}

// UpdateDispatchStatus P2-1: 更新节点配置下发状态到 metadata JSONB 字段。
// status: pending/pushed/applied/failed; version: 目标配置版本号; errMsg: 失败时的错误信息（可选）。
// 使用 jsonb_set 幂等设置 metadata 中的 _dispatch_* 字段，不覆盖其他 metadata 键。
func (r *NodeRepo) UpdateDispatchStatus(ctx context.Context, nodeIDs []uuid.UUID, status string, version int64, errMsg string) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, nid := range nodeIDs {
		query := `
			UPDATE nodes SET
				metadata = jsonb_set(
					jsonb_set(
						jsonb_set(
							jsonb_set(
								COALESCE(metadata, '{}'::jsonb),
								'{_dispatch_status}', to_jsonb($2::text)
							),
							'{_dispatch_version}', to_jsonb($3::bigint)
						),
						'{_dispatch_time}', to_jsonb($4::timestamptz)
					),
					'{_dispatch_error}', to_jsonb($5::text)
				),
				updated_at = now()
			WHERE id = $1`
		_, err := r.pool.Exec(ctx, query, nid, status, version, now, errMsg)
		if err != nil {
			return fmt.Errorf("update dispatch status for node %s: %w", nid, err)
		}
	}
	return nil
}

func (r *NodeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE nodes SET deleted_at = now(), is_enabled = false, updated_at = now() WHERE id = $1 AND deleted_at IS NULL`
	ct, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("node not found or already deleted")
	}
	return nil
}

func (r *NodeRepo) CountByServerID(ctx context.Context, serverID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM nodes n JOIN runtimes r ON n.runtime_id = r.id WHERE r.server_id = $1 AND n.deleted_at IS NULL`
	var count int
	err := r.pool.QueryRow(ctx, query, serverID).Scan(&count)
	return count, err
}

func (r *NodeRepo) ListByServerID(ctx context.Context, serverID uuid.UUID) ([]*model.Node, error) {
	query := `
		SELECT n.id, n.code, n.name, n.runtime_id, n.region_id, n.group_id, n.node_type, n.protocol_type, n.transport_type,
			n.security_type, n.address, n.port, n.server_port, n.reality_server_name, n.sni, n.alpn, n.path, n.host_header, n.flow, n.is_enabled, n.is_visible, n.allow_udp,
			n.speed_limit_mbps, n.device_limit, n.padding_scheme, n.rate_time_enable, n.rate_time_ranges, n.transfer_enable_bytes,
			n.traffic_rate, n.priority, n.capacity_score, n.protocol_schema_version, n.exposure_mode, n.downstream_exposure_mode, n.is_split_mode,
			n.config_json, n.tags, n.metadata,
			n.last_published_version, n.created_at, n.updated_at, n.deleted_at
		FROM nodes n
		JOIN runtimes r ON n.runtime_id = r.id
		WHERE r.server_id = $1 AND n.deleted_at IS NULL AND n.is_enabled = true
		ORDER BY n.priority ASC`
	rows, err := r.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		n := &model.Node{}
		err := rows.Scan(
			&n.ID, &n.Code, &n.Name, &n.RuntimeID, &n.RegionID, &n.GroupID, &n.NodeType, &n.ProtocolType, &n.TransportType,
			&n.SecurityType, &n.Address, &n.Port, &n.ServerPort, &n.RealityServerName, &n.SNI, &n.ALPN, &n.Path, &n.HostHeader, &n.Flow, &n.IsEnabled, &n.IsVisible, &n.AllowUDP,
			&n.SpeedLimitMbps, &n.DeviceLimit, &n.PaddingScheme, &n.RateTimeEnable, &n.RateTimeRanges, &n.TransferEnableBytes,
			&n.TrafficRate, &n.Priority, &n.CapacityScore, &n.ProtocolSchemaVersion, &n.ExposureMode, &n.DownstreamExposureMode, &n.IsSplitMode,
			&n.ConfigJSON, &n.Tags, &n.Metadata,
			&n.LastPublishedVersion, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (r *NodeRepo) ListByRuntimeID(ctx context.Context, runtimeID uuid.UUID) ([]*model.Node, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM nodes WHERE runtime_id = $1 AND deleted_at IS NULL AND is_enabled = true
		ORDER BY priority ASC`, nodeSelectColumns)
	rows, err := r.pool.Query(ctx, query, runtimeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.Node
	for rows.Next() {
		n := &model.Node{}
		err := rows.Scan(
			&n.ID, &n.Code, &n.Name, &n.RuntimeID, &n.RegionID, &n.GroupID, &n.NodeType, &n.ProtocolType, &n.TransportType,
			&n.SecurityType, &n.Address, &n.Port, &n.ServerPort, &n.RealityServerName, &n.SNI, &n.ALPN, &n.Path, &n.HostHeader, &n.Flow, &n.IsEnabled, &n.IsVisible, &n.AllowUDP,
			&n.SpeedLimitMbps, &n.DeviceLimit, &n.PaddingScheme, &n.RateTimeEnable, &n.RateTimeRanges, &n.TransferEnableBytes,
			&n.TrafficRate, &n.Priority, &n.CapacityScore, &n.ProtocolSchemaVersion, &n.ExposureMode, &n.DownstreamExposureMode, &n.IsSplitMode,
			&n.ConfigJSON, &n.Tags, &n.Metadata,
			&n.LastPublishedVersion, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// CheckPathUnique R8 修复: 检查同一 runtime 下 path 是否唯一。
// excludeNodeID 用于更新场景（排除自身），创建场景传 uuid.Nil。
// path 为空时不校验（TCP/TLS 等无 path 协议）。
// 返回 true 表示 path 唯一（可用），false 表示已存在冲突。
func (r *NodeRepo) CheckPathUnique(ctx context.Context, runtimeID uuid.UUID, path string, excludeNodeID uuid.UUID) (bool, error) {
	if path == "" {
		return true, nil
	}
	query := `
		SELECT COUNT(*) FROM nodes
		WHERE runtime_id = $1 AND path = $2 AND deleted_at IS NULL AND id != $3`
	var count int
	err := r.pool.QueryRow(ctx, query, runtimeID, path, excludeNodeID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// UpdateLastSeenAt updates the last_seen_at timestamp for a node.
// Called during agent heartbeat to track node online status at the SQL level.
func (r *NodeRepo) UpdateLastSeenAt(ctx context.Context, nodeID uuid.UUID) error {
	query := `UPDATE nodes SET last_seen_at = now(), updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, nodeID)
	return err
}

func (r *NodeRepo) FindUsedServerPortsInServer(ctx context.Context, serverID uuid.UUID, start, end int) ([]int, error) {
	query := `
		SELECT COALESCE(n.server_port, (n.config_json->>'server_port')::int)
		FROM nodes n
		JOIN runtimes r ON n.runtime_id = r.id
		WHERE r.server_id = $1
		  AND n.deleted_at IS NULL
		  AND COALESCE(n.server_port, (n.config_json->>'server_port')::int) BETWEEN $2 AND $3
		ORDER BY 1`
	rows, err := r.pool.Query(ctx, query, serverID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ports []int
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, rows.Err()
}

// ListEnabledSNIs 返回所有 is_enabled=true 且 sni 非空的去重 SNI 列表。
// 用于 cert.SyncSANFromNodes 自动同步证书 SAN。
//
// serverID 非 nil 时仅扫描该 server 下的节点（通过 runtime_id → runtimes.server_id JOIN）；
// 为 nil 时扫描所有启用节点。
//
// 仅返回 security_type=tls 且 exposure_mode 非 argo_tunnel/cdn_saas 的节点 SNI：
//   - reality 节点 SNI 是伪装域名（rust-lang.org/swscan.apple.com 等不属于自己的域名），
//     不能申请证书；reality 协议用 public/private key 握手，不依赖 X.509 证书
//   - argo_tunnel/cdn_saas 节点 TLS 在 CF 边缘终止，源站接收明文 HTTP，不需要本地证书
func (r *NodeRepo) ListEnabledSNIs(ctx context.Context, serverID *uuid.UUID) ([]string, error) {
	// 仅扫描 security_type=tls 的节点 SNI 用于证书 SAN 同步。
	// 排除 reality 节点：reality 的 SNI 是伪装域名（如 rust-lang.org/swscan.apple.com），
	// 不属于自己，不能申请证书；reality 协议本身不依赖 X.509 证书（用 public/private key 握手）。
	// 排除 argo_tunnel/cdn_saas 节点：TLS 在 CF 边缘终止，源站接收明文 HTTP，不需要本地证书。
	query := `
		SELECT DISTINCT n.sni
		FROM nodes n
		WHERE n.deleted_at IS NULL
		  AND n.is_enabled = true
		  AND n.sni IS NOT NULL
		  AND n.sni != ''
		  AND n.security_type = 'tls'
		  AND COALESCE(n.exposure_mode, '') NOT IN ('argo_tunnel', 'cdn_saas')`
	var args []interface{}
	argIdx := 1
	if serverID != nil && *serverID != uuid.Nil {
		query = `
			SELECT DISTINCT n.sni
			FROM nodes n
			JOIN runtimes rt ON n.runtime_id = rt.id
			WHERE n.deleted_at IS NULL
			  AND n.is_enabled = true
			  AND n.sni IS NOT NULL
			  AND n.sni != ''
			  AND n.security_type = 'tls'
			  AND COALESCE(n.exposure_mode, '') NOT IN ('argo_tunnel', 'cdn_saas')
			  AND rt.server_id = $1`
		args = append(args, *serverID)
		argIdx++
		_ = argIdx
	}
	query += ` ORDER BY n.sni`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snis []string
	for rows.Next() {
		var sni string
		if err := rows.Scan(&sni); err != nil {
			return nil, err
		}
		snis = append(snis, sni)
	}
	return snis, rows.Err()
}
