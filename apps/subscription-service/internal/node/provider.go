package node

import (
	"context"
	"encoding/json"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Provider interface {
	// ListVisibleNodes 拉取订阅可见节点
	// planID 用于按套餐过滤（nil=不限套餐）
	// groupID 用于按会员分组过滤（nil=不限分组，即全部节点）
	ListVisibleNodes(ctx context.Context, planID *uuid.UUID, groupID *uuid.UUID) ([]*model.NodeInfo, error)
}

type DBNodeProvider struct {
	pool *pgxpool.Pool
}

func NewDBNodeProvider(pool *pgxpool.Pool) *DBNodeProvider {
	return &DBNodeProvider{pool: pool}
}

func (p *DBNodeProvider) ListVisibleNodes(ctx context.Context, planID *uuid.UUID, groupID *uuid.UUID) ([]*model.NodeInfo, error) {
	// 节点可见性过滤逻辑（与 identity-service ListNodesForPlan 对齐）：
	// 1. 如果 groupID 非空 → 仅返回该分组下的节点（用户会员分组过滤）
	// 2. 如果 planID 非空 → 通过 plan.group_id 关联 nodes.group_id 过滤
	//    （套餐绑定的是群组，不是直接绑定节点）
	// 3. 如果 planID 和 groupID 都为空 → 返回全部可见节点
	// 4. 向后兼容：如果 plan 没有 group_id，回退到 plan_nodes 表
	query := `
		SELECT
			n.id, n.code, n.name,
			n.protocol_type, n.transport_type, COALESCE(n.security_type, ''),
			n.address, n.port,
			COALESCE(n.sni, ''), COALESCE(n.alpn, '{}'), COALESCE(n.path, ''),
			COALESCE(n.host_header, ''), COALESCE(n.flow, ''),
			n.config_json,
			COALESCE(r.name, ''),
			COALESCE(r.country_code, ''),
			COALESCE(ng.name, ''),
			COALESCE(ng.id, '00000000-0000-0000-0000-000000000000'::uuid),
			COALESCE(n.priority, 0),
			COALESCE(nh.current_rtt_ms, 0),
			n.is_enabled, n.is_visible,
			COALESCE(n.traffic_rate, 1.0),
			COALESCE(nh.availability_score, 100),
			n.created_at
		FROM nodes n
		LEFT JOIN node_groups ng ON n.group_id = ng.id
		LEFT JOIN regions r ON n.region_id = r.id
		LEFT JOIN node_health_status nh ON nh.node_id = n.id
	WHERE n.is_enabled = true
	  AND n.is_visible = true
	  AND n.deleted_at IS NULL
	  AND n.address != ''
	  AND (
		  $2::uuid IS NOT NULL
		  AND (
			  n.group_id = $2
			  OR EXISTS (SELECT 1 FROM node_group_members ngm WHERE ngm.node_id = n.id AND ngm.group_id = $2)
		  )
		  OR $2::uuid IS NULL
		  AND (
			  $1::uuid IS NULL
			  OR EXISTS (SELECT 1 FROM plan_nodes pn WHERE pn.plan_id = $1 AND pn.node_id = n.id)
			  OR n.group_id = (SELECT group_id FROM plans WHERE id = $1 AND deleted_at IS NULL)
		  )
	  )
ORDER BY ng.sort_order NULLS LAST, n.priority DESC, n.created_at ASC`

	rows, err := p.pool.Query(ctx, query, planID, groupID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []*model.NodeInfo{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var nodes []*model.NodeInfo
	for rows.Next() {
		var (
			n          model.NodeInfo
			configJSON []byte
			addr       string
			portVal    int
			sni        string
			alpn       []string
			path       string
			hostHdr    string
			flow       string
		)
		err := rows.Scan(
			&n.ID, &n.Code, &n.Name,
			&n.ProtocolType, &n.TransportType, &n.SecurityType,
			&addr, &portVal,
			&sni, &alpn, &path, &hostHdr, &flow,
			&configJSON,
			&n.Region, &n.CountryCode,
			&n.GroupName, &n.GroupID,
			&n.Priority, &n.LatencyMs,
			&n.IsEnabled, &n.IsVisible,
			&n.Multiplier,
			&n.HealthScore,
			&n.CreatedAt,
		)
		if err != nil {
			continue
		}
		n.Address = addr
		n.Port = portVal
		n.SNI = sni
		n.ALPN = alpn
		n.Path = path
		n.HostHeader = hostHdr
		n.Flow = flow
		n.Status = "healthy"
		if configJSON != nil && len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &n.ConfigJSON)
		}
		if n.ConfigJSON == nil {
			n.ConfigJSON = make(map[string]interface{})
		}
		nodes = append(nodes, &n)
	}
	if nodes == nil {
		nodes = []*model.NodeInfo{}
	}
	if err := rows.Err(); err != nil {
		return nodes, err
	}

	// P2-3: 展开多 Host 节点
	// 查询 node_hosts 表，为每个有 host 的节点生成额外的 NodeInfo 副本
	return p.expandNodeHosts(ctx, nodes), nil
}

// nodeHostRow node_hosts 表的精简结构（P2-3）
type nodeHostRow struct {
	NodeID     uuid.UUID
	Host       string
	Port       *int
	Path       *string
	SNI        *string
	HostHeader *string
	Priority   int
}

// expandNodeHosts P2-3: 查询 node_hosts 表并展开多 Host 节点。
// - 有 host 的节点：原节点保留（作为直连回退），每个 host 生成一个副本（address=host）
// - 无 host 的节点：保持原样（向后兼容）
func (p *DBNodeProvider) expandNodeHosts(ctx context.Context, nodes []*model.NodeInfo) []*model.NodeInfo {
	if len(nodes) == 0 {
		return nodes
	}
	nodeIDs := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	query := `
		SELECT node_id, host, port, path, sni, host_header, priority
		FROM node_hosts
		WHERE node_id = ANY($1) AND is_enabled = TRUE
		ORDER BY node_id, priority ASC, id ASC`
	rows, err := p.pool.Query(ctx, query, nodeIDs)
	if err != nil {
		return nodes // 查询失败时返回原节点列表（降级）
	}
	defer rows.Close()

	hostMap := make(map[uuid.UUID][]nodeHostRow)
	for rows.Next() {
		var h nodeHostRow
		if err := rows.Scan(&h.NodeID, &h.Host, &h.Port, &h.Path, &h.SNI, &h.HostHeader, &h.Priority); err != nil {
			continue
		}
		hostMap[h.NodeID] = append(hostMap[h.NodeID], h)
	}

	if len(hostMap) == 0 {
		return nodes // 无 host 配置，返回原列表
	}

	// 展开节点：原节点 + 每个 host 一个副本
	expanded := make([]*model.NodeInfo, 0, len(nodes)+len(hostMap)*2)
	for _, n := range nodes {
		hosts, ok := hostMap[n.ID]
		if !ok || len(hosts) == 0 {
			expanded = append(expanded, n)
			continue
		}
		// 原节点保留（作为直连回退），标记 Name 加后缀 "(direct)"
		original := *n
		original.Name = n.Name + " (direct)"
		expanded = append(expanded, &original)

		// 每个 host 生成一个副本
		for _, h := range hosts {
			clone := *n
			clone.Address = h.Host
			if h.Port != nil {
				clone.Port = *h.Port
			}
			if h.Path != nil {
				clone.Path = *h.Path
			}
			if h.SNI != nil {
				clone.SNI = *h.SNI
			}
			if h.HostHeader != nil {
				clone.HostHeader = *h.HostHeader
			}
			// 清除 cdn_address 覆盖（host 已直接设置为 Address）
			if clone.ConfigJSON != nil {
				delete(clone.ConfigJSON, "cdn_address")
				delete(clone.ConfigJSON, "cdn_port")
			}
			clone.Name = n.Name + " (" + h.Host + ")"
			expanded = append(expanded, &clone)
		}
	}
	return expanded
}
