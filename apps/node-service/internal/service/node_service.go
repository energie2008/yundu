package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/airport-panel/config"
	nodecrypto "github.com/airport-panel/node-service/internal/crypto"
	"github.com/airport-panel/node-service/internal/exposure"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/google/uuid"
)

// ConfigRefresher 由 DeploymentService 实现，用于节点变更后自动触发配置刷新。
// 借鉴 xboard NodeSyncService::notifyConfigUpdated 的"保存即下发"模式，
// 让面板保存节点后自动触发 node-agent 配置下发，无需手动点"发布配置"按钮。
type ConfigRefresher interface {
	RefreshNodeConfig(ctx context.Context, nodeID uuid.UUID) (*model.ConfigVersion, error)
}

// CertSyncHook P1-5: 节点 SNI 变更后自动触发证书 SAN 同步的回调钩子。
// 由 CertificateService.SyncSANFromNodes 实现，通过 setter 注入避免循环依赖。
// 注入后节点保存时自动同步 SNI 到证书 SAN；未注入时跳过（保持旧行为）。
type CertSyncHook func(ctx context.Context, node *model.Node) error

// EventPublisher 由 events.Bus 实现，用于跨服务事件发布。
// 采用接口隔离避免 service 层直接依赖 Redis/具体事件总线实现。
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}

type NodeService struct {
	nodeRepo        *repo.NodeRepo
	runtimeRepo     *repo.RuntimeRepo
	healthRepo      *repo.HealthRepo
	chainRepo       *repo.ChainRepo
	groupRepo       *repo.NodeGroupRepo
	configRefresher ConfigRefresher
	eventPublisher  EventPublisher
	portPlanner     *PortPlanner
	logger          *slog.Logger
	// P1-5: 证书 SAN 同步钩子，节点保存后自动触发
	certSyncHook    CertSyncHook
}

func NewNodeService(nodeRepo *repo.NodeRepo, runtimeRepo *repo.RuntimeRepo, healthRepo *repo.HealthRepo, chainRepo *repo.ChainRepo) *NodeService {
	return &NodeService{
		nodeRepo:    nodeRepo,
		runtimeRepo: runtimeRepo,
		healthRepo:  healthRepo,
		chainRepo:   chainRepo,
	}
}

// SetGroupRepo 注入 NodeGroupRepo（用于节点-分组多对多关联维护）
// 采用 setter 注入避免破坏现有构造函数签名
func (s *NodeService) SetGroupRepo(groupRepo *repo.NodeGroupRepo) {
	s.groupRepo = groupRepo
}

// SetConfigRefresher 注入配置刷新器（DeploymentService），用于节点变更后自动触发配置下发。
// 借鉴 xboard NodeSyncService::notifyConfigUpdated 模式，实现"保存即下发"的零 SSH 闭环。
// 采用 setter 注入避免 NodeService ↔ DeploymentService 循环依赖。
func (s *NodeService) SetConfigRefresher(refresher ConfigRefresher) {
	s.configRefresher = refresher
}

// SetEventPublisher 注入事件发布器，用于节点变更后发布 TopicConfigChanged 事件，
// 使 subscription-service 等订阅方能在节点保存后立即失效缓存。
func (s *NodeService) SetEventPublisher(pub EventPublisher) {
	s.eventPublisher = pub
}

// SetPortPlanner 注入端口规划器，用于零 SSH 架构下自动分配 ServerPort。
func (s *NodeService) SetPortPlanner(pp *PortPlanner) {
	s.portPlanner = pp
}

// SetLogger P1-4: 注入 slog 日志器，用于一致性自检等告警日志。
func (s *NodeService) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

// SetCertSyncHook P1-5: 注入证书 SAN 同步钩子。
// 注入后节点保存时自动触发 SyncSANFromNodes，将节点 SNI 同步到证书 SAN。
// 未注入时跳过自动同步（保持旧行为，需手动调用 /certs/:id/sync-san）。
func (s *NodeService) SetCertSyncHook(hook CertSyncHook) {
	s.certSyncHook = hook
}

// publishConfigChanged 发布节点配置变更事件（fire-and-forget，失败只记录日志）
func (s *NodeService) publishConfigChanged(nodeID uuid.UUID, action string) {
	if s.eventPublisher == nil {
		return
	}
	evt := struct {
		NodeID string `json:"node_id"`
		Action string `json:"action"`
	}{
		NodeID: nodeID.String(),
		Action: action,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.eventPublisher.Publish(ctx, "node:config_changed", evt); err != nil {
			log.Printf("warn: publish config_changed event failed for node %s: %v", nodeID, err)
		}
	}()
}

// ListChainIDsForNodes 批量查询节点绑定的路由组ID
func (s *NodeService) ListChainIDsForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if s.chainRepo == nil {
		return make(map[uuid.UUID][]uuid.UUID), nil
	}
	return s.chainRepo.ListChainBindingsForNodes(ctx, nodeIDs)
}

// ReplaceNodeChainBindings 整体覆盖节点的代理链绑定
func (s *NodeService) ReplaceNodeChainBindings(ctx context.Context, nodeID uuid.UUID, chainIDs []uuid.UUID) error {
	if s.chainRepo == nil {
		return nil
	}
	return s.chainRepo.ReplaceNodeChainBindings(ctx, nodeID, chainIDs)
}

// ListServerInfoForRuntimes 批量查询 runtime 对应的 server 简要信息
func (s *NodeService) ListServerInfoForRuntimes(ctx context.Context, runtimeIDs []uuid.UUID) (map[uuid.UUID]*model.ServerBrief, error) {
	return s.nodeRepo.ListServerInfoForRuntimes(ctx, runtimeIDs)
}

// ListNodeGroupBriefs 批量查询 groupID 对应的分组简要信息
func (s *NodeService) ListNodeGroupBriefs(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID]*model.NodeGroupBrief, error) {
	return s.nodeRepo.ListNodeGroupBriefs(ctx, groupIDs)
}

// ListGroupsForNodes 批量查询多个节点的所有所属分组（多对多）
// 返回 map[nodeID][]*NodeGroupBrief，用于节点编辑回显多分组
func (s *NodeService) ListGroupsForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]*model.NodeGroupBrief, error) {
	return s.nodeRepo.ListGroupsForNodes(ctx, nodeIDs)
}

func (s *NodeService) CreateNode(ctx context.Context, req *model.CreateNodeRequest) (*model.Node, error) {
	var warnings []string
	existing, err := s.nodeRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrNodeAlreadyExists
	}

	runtime, err := s.runtimeRepo.GetByID(ctx, req.RuntimeID)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, ErrRuntimeNotFound
	}

	// R8 修复: Path 唯一性校验（同一 runtime 下不允许重复 path）
	if req.Path != nil && *req.Path != "" {
		unique, err := s.nodeRepo.CheckPathUnique(ctx, req.RuntimeID, *req.Path, uuid.Nil)
		if err != nil {
			return nil, fmt.Errorf("check path unique failed: %w", err)
		}
		if !unique {
			return nil, fmt.Errorf("path '%s' already exists in this runtime, path must be unique", *req.Path)
		}
	}

	// 协议级校验：校验协议/传输/安全组合是否合法
	if err := validateProtocolCombo(req); err != nil {
		return nil, fmt.Errorf("protocol validation failed: %w", err)
	}

	nodeType := req.NodeType
	if nodeType == "" {
		nodeType = model.NodeTypeStandard
	}

	trafficRate := 1.00
	if req.TrafficRate != nil {
		trafficRate = *req.TrafficRate
	}

	priority := 100
	if req.Priority != nil {
		priority = *req.Priority
	}

	capacityScore := 100
	if req.CapacityScore != nil {
		capacityScore = *req.CapacityScore
	}

	schemaVersion := req.ProtocolSchemaVersion
	if schemaVersion == "" {
		schemaVersion = "v1"
	}

	if req.ConfigJSON == nil {
		req.ConfigJSON = make(map[string]interface{})
	}
	// 修复前后端不一致：前端发送顶层 cert_bundle_id，后端写入 config_json.cert_bundle_id
	if req.CertBundleID != nil && *req.CertBundleID != "" {
		req.ConfigJSON["cert_bundle_id"] = *req.CertBundleID
	}
	if req.Tags == nil {
		req.Tags = make([]string, 0)
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	if req.ALPN == nil {
		req.ALPN = make([]string, 0)
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}
	isVisible := true
	if req.IsVisible != nil {
		isVisible = *req.IsVisible
	}

	node := &model.Node{
		ID:                     uuid.New(),
		Code:                   req.Code,
		Name:                   req.Name,
		RuntimeID:              req.RuntimeID,
		RegionID:               req.RegionID,
		GroupID:                req.GroupID,
		NodeType:               nodeType,
		ProtocolType:           req.ProtocolType,
		TransportType:          req.TransportType,
		SecurityType:           req.SecurityType,
		Address:                req.Address,
		Port:                   req.Port,
		ServerPort:             req.ServerPort,
		RealityServerName:      req.RealityServerName,
		SNI:                    req.SNI,
		ALPN:                   req.ALPN,
		Path:                   req.Path,
		HostHeader:             req.HostHeader,
		Flow:                   req.Flow,
		IsEnabled:              isEnabled,
		IsVisible:              isVisible,
		AllowUDP:               req.AllowUDP,
		SpeedLimitMbps:         req.SpeedLimitMbps,
		DeviceLimit:            req.DeviceLimit,
		PaddingScheme:          req.PaddingScheme,
		TransferEnableBytes:    req.TransferEnableBytes,
		TrafficRate:            trafficRate,
		Priority:               priority,
		CapacityScore:          capacityScore,
		ProtocolSchemaVersion:  schemaVersion,
		ExposureMode:           req.ExposureMode,
		DownstreamExposureMode: req.DownstreamExposureMode,
		IsSplitMode:            false,
		ConfigJSON:             req.ConfigJSON,
		Tags:                   req.Tags,
		Metadata:               req.Metadata,
		LastPublishedVersion:   0,
	}
	if req.IsSplitMode != nil {
		node.IsSplitMode = *req.IsSplitMode
	}

	if err := s.standardizeNodeFields(ctx, node); err != nil {
		return nil, err
	}

	// P2 校验：自签证书节点禁止使用 CDN/Tunnel 模式
	if err := validateExposureMode(node); err != nil {
		return nil, err
	}

	// Bug-B1: 自动为 REALITY 节点补全 private_key/public_key，
	// 消除 xray_config.go 中的硬编码密钥回退。
	// B39 修复: autoGenerateREALITYKeys 失败不再静默吞掉，以 Error 级别记录
	if err := autoGenerateREALITYKeys(ctx, node); err != nil {
		log.Printf("error: auto-generate REALITY keys failed for new node %s: %v", node.Code, err)
	}

	// P0-2: 规范化 config_json：拍平嵌套结构（reality_settings/tls_settings → 顶层键），
	// 统一 snake_case，删除白名单外的键，确保下游 kernelrender 能从顶层读取所有字段。
	secStr := ""
	if node.SecurityType != nil {
		secStr = *node.SecurityType
	}
	node.ConfigJSON = NormalizeNodeConfigJSON(node.ConfigJSON, node.ProtocolType, node.TransportType, secStr)

	// 根据节点协议/传输/安全特性自动路由到正确的内核 runtime
	if err := s.routeNodeToCorrectRuntime(ctx, node); err != nil {
		return nil, err
	}

	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, err
	}

	healthStatus := &model.NodeHealthStatus{
		NodeID:            node.ID,
		OverallStatus:     "unknown",
		HeartbeatStatus:   "unknown",
		ProbeStatus:       "unknown",
		AvailabilityScore: 0,
		LatencyScore:      0,
		LossScore:         0,
		HandshakeScore:    0,
		ChainScore:        0,
		StabilityScore:    0,
	}
	if err := s.healthRepo.UpsertStatus(ctx, healthStatus); err != nil {
		return nil, err
	}

	// 保存节点绑定的代理链（路由组）列表，整体覆盖语义
	if len(req.ChainIDs) > 0 {
		if err := s.ReplaceNodeChainBindings(ctx, node.ID, req.ChainIDs); err != nil {
			return nil, fmt.Errorf("failed to bind chains: %w", err)
		}
	}

	// 保存节点-分组多对多关联
	// 优先使用 GroupIDs；若为空则回退到 GroupID（向后兼容）
	groupIDsToSet := req.GroupIDs
	if len(groupIDsToSet) == 0 && req.GroupID != nil {
		groupIDsToSet = []uuid.UUID{*req.GroupID}
	}
	if s.groupRepo != nil && len(groupIDsToSet) > 0 {
		if err := s.groupRepo.SetNodeGroups(ctx, node.ID, groupIDsToSet); err != nil {
			return nil, fmt.Errorf("failed to set node groups: %w", err)
		}
	}

	// D9 修复: 自动绑定节点到计划，用户订阅即可看到新节点
	if len(req.PlanIDs) > 0 {
		if err := s.nodeRepo.BindNodeToPlans(ctx, node.ID, req.PlanIDs); err != nil {
			return nil, fmt.Errorf("failed to bind node to plans: %w", err)
		}
	}

	// 自动触发配置刷新（借鉴 xboard NodeSyncService::notifyConfigUpdated）
	// 新建节点后立即触发 node-agent 配置下发，无需手动点"发布配置"按钮
	// 失败不阻断保存操作，仅告警（节点已成功创建，配置可稍后手动发布）
	if s.configRefresher != nil {
		if _, err := s.configRefresher.RefreshNodeConfig(ctx, node.ID); err != nil {
			warnMsg := fmt.Sprintf("配置下发失败（agent可能离线）：%v，节点已保存，agent重连后将自动拉取配置", err)
			log.Printf("warn: auto refresh config failed for new node %s: %v", node.ID, err)
			warnings = append(warnings, warnMsg)
		}
	}

	// P0-4: 将警告写入 node.Metadata，NewNodeResponse 会提取到 Warnings 字段
	if len(warnings) > 0 {
		if node.Metadata == nil {
			node.Metadata = make(map[string]interface{})
		}
		node.Metadata["_warnings"] = warnings
	}

	// P2-1: 发布节点配置变更事件，通知订阅服务等立即失效缓存
	s.publishConfigChanged(node.ID, "created")

	return node, nil
}

func (s *NodeService) GetNode(ctx context.Context, id uuid.UUID) (*model.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}
	return node, nil
}

func (s *NodeService) ListNodes(ctx context.Context, page, pageSize int, protocolType, regionID, groupID, search string, isEnabled *bool) ([]*model.Node, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 20
	}
	return s.nodeRepo.List(ctx, page, pageSize, protocolType, regionID, groupID, search, isEnabled)
}

// ListPlanCodesForNodes 批量查询节点的已绑定套餐 code
func (s *NodeService) ListPlanCodesForNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	return s.nodeRepo.ListPlanCodesForNodes(ctx, nodeIDs)
}

func (s *NodeService) UpdateNode(ctx context.Context, id uuid.UUID, req *model.UpdateNodeRequest) (*model.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}

	// R8 修复: Path 唯一性校验（更新 path 时检查同一 runtime 下是否冲突）
	if req.Path != nil && *req.Path != "" {
		currentPath := ""
		if node.Path != nil {
			currentPath = *node.Path
		}
		if *req.Path != currentPath {
			unique, err := s.nodeRepo.CheckPathUnique(ctx, node.RuntimeID, *req.Path, id)
			if err != nil {
				return nil, fmt.Errorf("check path unique failed: %w", err)
			}
			if !unique {
				return nil, fmt.Errorf("path '%s' already exists in this runtime, path must be unique", *req.Path)
			}
		}
	}

	if req.Name != nil {
		node.Name = *req.Name
	}
	// 节点编码更新：需做唯一性校验，避免与其它节点冲突
	if req.Code != nil && *req.Code != "" && *req.Code != node.Code {
		existing, err := s.nodeRepo.GetByCode(ctx, *req.Code)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != id {
			return nil, ErrNodeAlreadyExists
		}
		node.Code = *req.Code
	}
	if req.GroupID != nil {
		node.GroupID = req.GroupID
	}
	if req.NodeType != nil {
		node.NodeType = *req.NodeType
	}
	if req.Address != nil {
		node.Address = *req.Address
	}
	if req.Port != nil {
		node.Port = *req.Port
	}
	if req.ServerPort != nil {
		node.ServerPort = req.ServerPort
	}
	if req.RealityServerName != nil {
		node.RealityServerName = req.RealityServerName
	}
	if req.SNI != nil {
		node.SNI = req.SNI
	}
	if req.ALPN != nil {
		node.ALPN = req.ALPN
	}
	if req.Path != nil {
		node.Path = req.Path
	}
	if req.HostHeader != nil {
		node.HostHeader = req.HostHeader
	}
	if req.Flow != nil {
		node.Flow = req.Flow
	}
	if req.SecurityType != nil {
		node.SecurityType = req.SecurityType
	}
	// 修复：前端切换 transport（如 ws→xhttp）时更新 DB 列
	if req.TransportType != nil && *req.TransportType != "" {
		node.TransportType = *req.TransportType
	}
	// 修复：前端切换 protocol（如 vless→trojan）时更新 DB 列
	if req.ProtocolType != nil && *req.ProtocolType != "" {
		node.ProtocolType = *req.ProtocolType
	}
	// 修复：前端设置流量限额时持久化到 DB
	if req.TransferEnableBytes != nil {
		node.TransferEnableBytes = req.TransferEnableBytes
	}
	if req.IsEnabled != nil {
		node.IsEnabled = *req.IsEnabled
	}
	if req.IsVisible != nil {
		node.IsVisible = *req.IsVisible
	}
	if req.AllowUDP != nil {
		node.AllowUDP = *req.AllowUDP
	}
	if req.SpeedLimitMbps != nil {
		node.SpeedLimitMbps = req.SpeedLimitMbps
	}
	if req.DeviceLimit != nil {
		node.DeviceLimit = req.DeviceLimit
	}
	if req.PaddingScheme != nil {
		node.PaddingScheme = req.PaddingScheme
	}
	if req.TrafficRate != nil {
		node.TrafficRate = *req.TrafficRate
	}
	if req.Priority != nil {
		node.Priority = *req.Priority
	}
	if req.CapacityScore != nil {
		node.CapacityScore = *req.CapacityScore
	}
	if req.ConfigJSON != nil {
		node.ConfigJSON = req.ConfigJSON
	}
	// 修复前后端不一致：前端发送顶层 cert_bundle_id，后端写入 config_json.cert_bundle_id
	if req.CertBundleID != nil {
		if node.ConfigJSON == nil {
			node.ConfigJSON = make(map[string]interface{})
		}
		if *req.CertBundleID != "" {
			node.ConfigJSON["cert_bundle_id"] = *req.CertBundleID
		} else {
			delete(node.ConfigJSON, "cert_bundle_id")
		}
	}
	if req.Tags != nil {
		node.Tags = req.Tags
	}
	if req.Metadata != nil {
		node.Metadata = req.Metadata
	}
	if req.ExposureMode != nil {
		node.ExposureMode = req.ExposureMode
	}
	if req.DownstreamExposureMode != nil {
		node.DownstreamExposureMode = req.DownstreamExposureMode
	}
	if req.IsSplitMode != nil {
		node.IsSplitMode = *req.IsSplitMode
	}

	if err := s.standardizeNodeFields(ctx, node); err != nil {
		return nil, err
	}

	// P2 校验：自签证书节点禁止使用 CDN/Tunnel 模式
	if err := validateExposureMode(node); err != nil {
		return nil, err
	}

	// P0-2: 规范化 config_json（与 CreateNode 一致）
	secStr := ""
	if node.SecurityType != nil {
		secStr = *node.SecurityType
	}
	node.ConfigJSON = NormalizeNodeConfigJSON(node.ConfigJSON, node.ProtocolType, node.TransportType, secStr)

	// Bug-B1: 更新节点时也自动补全REALITY密钥（主连接+下行downloadSettings）
	if err := autoGenerateREALITYKeys(ctx, node); err != nil {
		log.Printf("error: auto-generate REALITY keys failed for update node %s: %v", node.Code, err)
	}

	// 根据节点协议/传输/安全特性自动路由到正确的内核 runtime
	if err := s.routeNodeToCorrectRuntime(ctx, node); err != nil {
		return nil, err
	}

	if err := s.nodeRepo.Update(ctx, node); err != nil {
		return nil, err
	}

	// 更新节点绑定的代理链（路由组）列表
	// 注意：UpdateNodeRequest.ChainIDs 是 []uuid.UUID 类型
	//   - nil: 不修改（保持现有绑定）
	//   - 空 []: 清空所有绑定
	//   - 非空 []: 整体覆盖为新绑定
	if req.ChainIDs != nil {
		if err := s.ReplaceNodeChainBindings(ctx, id, req.ChainIDs); err != nil {
			return nil, fmt.Errorf("failed to update chain bindings: %w", err)
		}
	}

	// 更新节点-分组多对多关联
	// GroupIDs 语义：
	//   - nil: 不修改（保持现有关联）；若 GroupID 也非 nil，则回退到 GroupID 单值更新
	//   - 空 []: 清空所有分组关联
	//   - 非空 []: 整体覆盖为新关联
	if req.GroupIDs != nil {
		if s.groupRepo != nil {
			if err := s.groupRepo.SetNodeGroups(ctx, id, req.GroupIDs); err != nil {
				return nil, fmt.Errorf("failed to update node groups: %w", err)
			}
			// SetNodeGroups 已同步 nodes.group_id，更新内存中的 node 对象
			if len(req.GroupIDs) > 0 {
				gid := req.GroupIDs[0]
				node.GroupID = &gid
			} else {
				node.GroupID = nil
			}
		}
	} else if req.GroupID != nil && s.groupRepo != nil {
		// 向后兼容：仅提供 GroupID 时，将其追加到关联表（不删除已有关联）
		// 注意：这是单值语义的旧行为，建议前端使用 GroupIDs
		_ = s.groupRepo.SetNodeGroups(ctx, id, []uuid.UUID{*req.GroupID})
		node.GroupID = req.GroupID
	}

	// 自动触发配置刷新（借鉴 xboard NodeSyncService::notifyConfigUpdated）
	// 节点配置变更后立即触发 node-agent 配置下发，无需手动点"发布配置"按钮
	// 失败不阻断更新操作，仅告警（节点已成功更新，配置可稍后手动发布）
	var updateWarnings []string
	if s.configRefresher != nil {
		if _, err := s.configRefresher.RefreshNodeConfig(ctx, id); err != nil {
			warnMsg := fmt.Sprintf("配置下发失败（agent可能离线）：%v，节点已保存，agent重连后将自动拉取配置", err)
			log.Printf("warn: auto refresh config failed for node %s: %v", id, err)
			updateWarnings = append(updateWarnings, warnMsg)
		}
	}

	// P1-5: SyncSANFromNodes 实时触发 — 节点 SNI 变更后自动同步到证书 SAN。
	// 避免节点 SNI 变更后证书 SAN 未更新导致 TLS 握手失败。
	// 失败不阻断节点更新，仅告警（证书 SAN 可稍后手动同步）。
	if s.certSyncHook != nil {
		if err := s.certSyncHook(ctx, node); err != nil {
			warnMsg := fmt.Sprintf("证书 SAN 自动同步失败：%v，可稍后手动同步", err)
			log.Printf("warn: auto sync SAN failed for node %s: %v", id, err)
			updateWarnings = append(updateWarnings, warnMsg)
		}
	}

	if len(updateWarnings) > 0 {
		if node.Metadata == nil {
			node.Metadata = make(map[string]interface{})
		}
		node.Metadata["_warnings"] = updateWarnings
	}

	// P2-1: 发布节点配置变更事件
	s.publishConfigChanged(id, "updated")

	return node, nil
}

func (s *NodeService) UpdateNodePublishedVersion(ctx context.Context, id uuid.UUID, version int64) error {
	return s.nodeRepo.UpdatePublishedVersion(ctx, id, version)
}

func (s *NodeService) DeleteNode(ctx context.Context, id uuid.UUID) error {
	if _, err := s.nodeRepo.GetByID(ctx, id); err != nil {
		return ErrNodeNotFound
	}
	if err := s.nodeRepo.Delete(ctx, id); err != nil {
		return err
	}
	// P2-1: 删除节点后发布配置变更事件
	s.publishConfigChanged(id, "deleted")
	return nil
}

func (s *NodeService) ListNodesByRuntime(ctx context.Context, runtimeID uuid.UUID) ([]*model.Node, error) {
	return s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
}

// MigrateAllNodeConfigJSON P0-5: 批量迁移所有节点的 config_json 到规范化结构。
// 遍历全量节点，对每个节点调用 MigrateNodeConfigJSON 拍平 nested 字段并清理废弃键，
// 若迁移后内容有变化则写回 DB。返回实际迁移的节点数。
func (s *NodeService) MigrateAllNodeConfigJSON(ctx context.Context) (int, error) {
	migrated := 0
	page, pageSize := 1, 200
	for {
		nodes, total, err := s.nodeRepo.List(ctx, page, pageSize, "", "", "", "", nil)
		if err != nil {
			return migrated, fmt.Errorf("list nodes page %d: %w", page, err)
		}
		for _, n := range nodes {
			if n.ConfigJSON == nil {
				continue
			}
			normalized := MigrateNodeConfigJSON(n)
			if configJSONEqual(n.ConfigJSON, normalized) {
				continue
			}
			n.ConfigJSON = normalized
			if err := s.nodeRepo.Update(ctx, n); err != nil {
				return migrated, fmt.Errorf("update node %s: %w", n.ID, err)
			}
			migrated++
		}
		if page*pageSize >= total {
			break
		}
		page++
	}
	return migrated, nil
}

// configJSONEqual 简单比较两个 config_json map 是否等价（用于判断迁移是否产生变化）。
func configJSONEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		// 浅比较：仅比较类型与字符串值，足够判断迁移是否产生变化
		if sA, ok := v.(string); ok {
			if sB, ok := bv.(string); !ok || sA != sB {
				return false
			}
		}
	}
	return true
}

// validateProtocolCombo 校验协议/传输/安全组合是否合法
// 基于 nodespec.ValidProtocols/ValidTransports/ValidSecurity 白名单 + 禁止组合规则
func validateProtocolCombo(req *model.CreateNodeRequest) error {
	protocol := nodespec.Protocol(req.ProtocolType)
	transport := nodespec.Transport(req.TransportType)
	securityStr := ""
	if req.SecurityType != nil {
		securityStr = *req.SecurityType
	}
	security := nodespec.Security(securityStr)

	// 1. 白名单校验
	if !nodespec.ValidProtocols[protocol] {
		return fmt.Errorf("%w: unknown protocol '%s'", ErrInvalidProtocolType, req.ProtocolType)
	}
	if transport != "" && !nodespec.ValidTransports[transport] {
		return fmt.Errorf("%w: unknown transport '%s'", ErrInvalidProtocolType, req.TransportType)
	}
	if securityStr != "" && securityStr != "none" && !nodespec.ValidSecurity[security] {
		return fmt.Errorf("%w: unknown security '%s'", ErrInvalidProtocolType, securityStr)
	}

	// 2. 禁止组合规则（forbidden combos）
	// REALITY 仅支持 vless 协议
	if securityStr == "reality" && protocol != nodespec.ProtocolVLESS {
		return fmt.Errorf("%w: REALITY security only supports vless protocol, got '%s'", ErrConfigValidation, req.ProtocolType)
	}
	// Hysteria2/TUIC 必须用 QUIC 传输 + TLS 安全
	if protocol == nodespec.ProtocolHysteria2 || protocol == nodespec.ProtocolTUIC {
		if transport != "" && transport != nodespec.TransportQUIC && transport != "udp" {
			return fmt.Errorf("%w: %s requires quic/udp transport, got '%s'", ErrConfigValidation, req.ProtocolType, req.TransportType)
		}
	}

	// 3. 端口范围校验
	if req.Port < 1 || req.Port > 65535 {
		return fmt.Errorf("%w: invalid port %d (must be 1-65535)", ErrConfigValidation, req.Port)
	}

	return nil
}

// autoGenerateREALITYKeys 为 REALITY 节点自动补全 private_key / public_key。
// 安全修复 (Bug-B1): 消除 xray_config.go:599 的硬编码回退密钥。
// 当节点安全类型为 reality 且 ConfigJSON 中三级路径(顶层 > reality.private_key > reality_settings.private_key)
// 均无 private_key 时，调用 GenerateREALITYKeypair() 生成独立密钥对，
// 写入顶层 private_key/public_key，避免不同 REALITY 节点共享密钥。
// 若已有密钥则跳过，保持幂等。
func autoGenerateREALITYKeys(ctx context.Context, node *model.Node) error {
	cfg := node.ConfigJSON
	if cfg == nil {
		cfg = make(map[string]interface{})
		node.ConfigJSON = cfg
	}

	secType := ""
	if node.SecurityType != nil {
		secType = *node.SecurityType
	}

	// 三级回退检查 private_key：顶层 > reality.private_key > reality_settings.private_key
	getNested := func(nestedPath ...string) string {
		cur := cfg
		for _, key := range nestedPath[:len(nestedPath)-1] {
			if m, ok := cur[key].(map[string]interface{}); ok {
				cur = m
			} else {
				return ""
			}
		}
		if v, ok := cur[nestedPath[len(nestedPath)-1]].(string); ok {
			return v
		}
		return ""
	}

	// 主连接 REALITY 密钥补全（仅当主security为reality时）
	if secType == "reality" {
		needMainKey := true
		if pk := getNested("private_key"); pk != "" {
			needMainKey = false
		}
		if pk := getNested("reality", "private_key"); pk != "" {
			needMainKey = false
		}
		if pk := getNested("reality_settings", "private_key"); pk != "" {
			needMainKey = false
		}
		if needMainKey {
			privateKey, publicKey, err := nodecrypto.GenerateREALITYKeypair()
			if err != nil {
				return fmt.Errorf("auto-generate REALITY keypair failed for node %s: %w", node.Code, err)
			}
			cfg["private_key"] = privateKey
			cfg["public_key"] = publicKey
		}
	}

	// XHTTP downloadSettings 下行 REALITY 密钥自动补全（P06上行CDN+下行REALITY/P07上行REALITY+下行CDN）
	// 无论主security是什么，只要下行security是reality就补全
	autoGenDownloadREALITYKeys := func() error {
		var ds map[string]interface{}
		// 新格式：xhttp.extra.downloadSettings
		if xh, ok := cfg["xhttp"].(map[string]interface{}); ok {
			if extra, ok := xh["extra"].(map[string]interface{}); ok {
				if d, ok := extra["downloadSettings"].(map[string]interface{}); ok {
					ds = d
				}
			}
		}
		// 旧格式兼容：xhttp_extra.downloadSettings
		if ds == nil {
			if extra, ok := cfg["xhttp_extra"].(map[string]interface{}); ok {
				if d, ok := extra["downloadSettings"].(map[string]interface{}); ok {
					ds = d
				}
			}
		}
		if ds == nil {
			if d, ok := cfg["download_settings"].(map[string]interface{}); ok {
				ds = d
			}
		}
		if ds == nil {
			return nil
		}
		sec, _ := ds["security"].(string)
		if sec != "reality" {
			return nil
		}
		var realityMap map[string]interface{}
		if r, ok := ds["realitySettings"].(map[string]interface{}); ok {
			realityMap = r
		} else if r, ok := ds["reality"].(map[string]interface{}); ok {
			realityMap = r
		}
		hasPrivKey := false
		if realityMap != nil {
			if pk, ok := realityMap["privateKey"].(string); ok && pk != "" {
				hasPrivKey = true
			}
			if pk, ok := realityMap["private_key"].(string); ok && pk != "" {
				hasPrivKey = true
			}
		}
		if hasPrivKey {
			return nil
		}
		dlPrivateKey, dlPublicKey, err := nodecrypto.GenerateREALITYKeypair()
		if err != nil {
			return fmt.Errorf("auto-generate downloadSettings REALITY keypair for node %s: %w", node.Code, err)
		}
		if realityMap == nil {
			realityMap = make(map[string]interface{})
		}
		realityMap["privateKey"] = dlPrivateKey
		realityMap["publicKey"] = dlPublicKey
		ds["realitySettings"] = realityMap
		// 写入标准路径 xhttp.extra.downloadSettings
		xh, ok := cfg["xhttp"].(map[string]interface{})
		if !ok {
			xh = make(map[string]interface{})
			cfg["xhttp"] = xh
		}
		extra, ok := xh["extra"].(map[string]interface{})
		if !ok {
			extra = make(map[string]interface{})
			xh["extra"] = extra
		}
		extra["downloadSettings"] = ds
		return nil
	}
	if err := autoGenDownloadREALITYKeys(); err != nil {
		log.Printf("warning: %v", err)
	}

	return nil
}

// isUDPProtocol 判断是否为UDP-only协议（不需要443 SNI分流）
func isUDPProtocol(protocolType string) bool {
	switch protocolType {
	case "hysteria2", "tuic":
		return true
	default:
		return false
	}
}

// standardizeNodeFields 在Create/Update前强制标准化节点字段，保证零SSH架构端口规则。
// 这是从源头堵住"垃圾进垃圾出"问题的关键防线：配置离开面板前已经是正确的。
func (s *NodeService) standardizeNodeFields(ctx context.Context, node *model.Node) error {
	secStr := ""
	if node.SecurityType != nil {
		secStr = *node.SecurityType
	}

	// 1. TCP协议强制Port=443
	if !isUDPProtocol(node.ProtocolType) {
		node.Port = 443
	}

	// 2. 自动分配ServerPort
	if !isUDPProtocol(node.ProtocolType) {
		// TCP协议：必须有ServerPort
		if node.ServerPort == nil || *node.ServerPort == 0 {
			if s.portPlanner == nil {
				return fmt.Errorf("port planner not initialized")
			}
			rt, err := s.runtimeRepo.GetByID(ctx, node.RuntimeID)
			if err != nil {
				return fmt.Errorf("get runtime for port allocation: %w", err)
			}
			if rt == nil {
				return fmt.Errorf("runtime %s not found", node.RuntimeID)
			}
			port, err := s.portPlanner.AllocateServerPort(ctx, rt.ServerID, node.NodeType, node.ProtocolType, node.TransportType, secStr)
			if err != nil {
				return fmt.Errorf("auto-allocate server_port: %w", err)
			}
			node.ServerPort = &port
		}
	} else {
		// UDP协议：不需要nginx转发，但也要分配高位端口（若未指定）
		if node.ServerPort == nil || *node.ServerPort == 0 {
			if node.Port <= 0 || node.Port == 443 {
				if s.portPlanner == nil {
					return fmt.Errorf("port planner not initialized")
				}
				rt, err := s.runtimeRepo.GetByID(ctx, node.RuntimeID)
				if err != nil {
					return fmt.Errorf("get runtime for port allocation: %w", err)
				}
				if rt == nil {
					return fmt.Errorf("runtime %s not found", node.RuntimeID)
				}
				port, err := s.portPlanner.AllocateServerPort(ctx, rt.ServerID, node.NodeType, node.ProtocolType, node.TransportType, secStr)
				if err != nil {
					return fmt.Errorf("auto-allocate server_port: %w", err)
				}
				node.Port = port
				node.ServerPort = &port
			} else {
				node.ServerPort = &node.Port
			}
		}
	}

	// 3. REALITY节点必须有RealityServerName/SNI
	if secStr == "reality" {
		if node.RealityServerName == nil || *node.RealityServerName == "" {
			if node.ConfigJSON != nil {
				if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
					if sn, ok := rs["server_name"].(string); ok && sn != "" {
						node.RealityServerName = &sn
					}
				}
				if node.RealityServerName == nil || *node.RealityServerName == "" {
					if sn, ok := node.ConfigJSON["reality_server_name"].(string); ok && sn != "" {
						node.RealityServerName = &sn
					}
				}
				if node.RealityServerName == nil || *node.RealityServerName == "" {
					if node.SNI != nil && *node.SNI != "" {
						node.RealityServerName = node.SNI
					}
				}
			}
		}
		if node.RealityServerName == nil || *node.RealityServerName == "" {
			return fmt.Errorf("REALITY节点必须设置 reality_server_name（伪装SNI域名），用于nginx stream SNI分流")
		}
		node.SNI = node.RealityServerName
	}

	// 4. 同步ServerPort到ConfigJSON（向后兼容渲染器）
	if node.ConfigJSON == nil {
		node.ConfigJSON = make(map[string]interface{})
	}
	if node.ServerPort != nil && *node.ServerPort > 0 {
		node.ConfigJSON["server_port"] = *node.ServerPort
	}

	// 4.5 XHTTP split mode (downloadSettings) 自动分配下行监听端口
	if node.TransportType == "xhttp" && node.ConfigJSON != nil {
		if err := s.allocateDownloadServerPort(ctx, node); err != nil {
			return fmt.Errorf("allocate download server_port: %w", err)
		}
	}

	// 5. 同步RealityServerName到ConfigJSON
	if node.RealityServerName != nil && *node.RealityServerName != "" {
		node.ConfigJSON["reality_server_name"] = *node.RealityServerName
		if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
			rs["server_name"] = *node.RealityServerName
		} else {
			node.ConfigJSON["reality_settings"] = map[string]interface{}{
				"server_name": *node.RealityServerName,
			}
		}
	}

	// 6. exposure_mode 三级判定（标准化零 SSH 架构核心逻辑）
	// 优先级：前端显式设置(node.ExposureMode) > config_json.exposure_mode > tunnel 凭证 > cdn_address > direct
	// 同步写入独立列 node.ExposureMode 和 config_json.exposure_mode 保持双写一致
	em := determineExposureMode(node)
	if node.ExposureMode != nil && *node.ExposureMode != "" {
		em = *node.ExposureMode
	}
	if em != "" {
		node.ConfigJSON["exposure_mode"] = em
		emCopy := em
		node.ExposureMode = &emCopy
	}

	// 6.5 同步 downstream_exposure_mode 和 is_split_mode
	// is_split_mode 字段定位（重要约束）：
	// - 它是前端表单开关，控制"下行暴露方式"下拉框是否显示
	// - 它是 DTO 字段，会持久化到 DB 独立列
	// - 它【不】进入渲染逻辑：determineInboundExposureMode 不读取此字段
	// - 后端安全判定只依赖 downstream_exposure_mode 字段是否有值
	// - 任何分支都不得加 if node.IsSplitMode 影响剥离判定
	if node.DownstreamExposureMode != nil && *node.DownstreamExposureMode != "" {
		node.ConfigJSON["downstream_exposure_mode"] = *node.DownstreamExposureMode
		dlCopy := *node.DownstreamExposureMode
		node.DownstreamExposureMode = &dlCopy
	} else if v, ok := node.ConfigJSON["downstream_exposure_mode"].(string); ok && v != "" {
		node.DownstreamExposureMode = &v
	} else {
		node.DownstreamExposureMode = nil
	}
	// 自动推断 is_split_mode：downstream_exposure_mode 有值则为 true
	if node.DownstreamExposureMode != nil && *node.DownstreamExposureMode != "" {
		node.IsSplitMode = true
		node.ConfigJSON["is_split_mode"] = true
	} else {
		node.IsSplitMode = false
		node.ConfigJSON["is_split_mode"] = false
	}

	// P1-4: is_split_mode / DownstreamExposureMode 一致性自检
	// 防止出现状态双轨：is_split_mode=true 但 downstream_exposure_mode 为空，
	// 或 is_split_mode=false 但 downstream_exposure_mode 有值的异常状态。
	// 异常时记录告警并自动修正为以 downstream_exposure_mode 为准。
	if err := validateSplitModeConsistency(node); err != nil {
		if s.logger != nil {
			s.logger.Warn("is_split_mode / downstream_exposure_mode inconsistency detected and auto-corrected",
				"node_code", node.Code,
				"error", err.Error(),
			)
		}
	}

	// 7. TLS 安全层标准化
	//
	// 架构说明（两个独立函数，分开维护避免耦合风险）：
	//
	// 1) argo_tunnel（CF 隧道）→ DB 字段分离
	//    - 客户端 → CF Edge (TLS终止) → cloudflared (HTTP明文) → xray (security=none)
	//    - DB SecurityType="none", config_json.security_type/security="tls", tls=1
	//    - 只有隧道节点需要 DB 字段层面的分离
	//
	// 2) cdn/cdn_saas（CDN 回源到 nginx）→ 非分离，TLS 剥离在渲染层
	//    - 客户端 → CDN → nginx 443 stream → nginx 8445 (TLS终止) → HTTP明文 → xray
	//    - DB SecurityType="tls", config_json.security_type/security="tls", tls=1（前后一致）
	//    - xray inbound 的 TLS 剥离由渲染层 shouldStripTLSForNginxVhost 动态完成，
	//      不通过 DB 字段分离实现（nginx 8445 是否有 vhost 才是决定因素）
	//
	// 3) direct/reality → xray 自终止，无需特殊处理
	//
	// "先纠正后校验"：统一字段值后再用验证器检查一致性

	// 7.0 argo_tunnel 专属：DB 字段 TLS 分离（DB=none, config_json=tls）
	if isTLSSplitExposureNode(node) {
		noneSec := "none"
		node.SecurityType = &noneSec
		node.ConfigJSON["security_type"] = "tls"
		node.ConfigJSON["security"] = "tls"
		node.ConfigJSON["tls"] = 1
		// 隧道节点由 CF Edge 管理证书，不绑定 cert_bundle
		delete(node.ConfigJSON, "cert_bundle_id")
		// client_port 强制为 443（CF 入口端口）
		node.Port = 443
	}

	// 7.0' cdn/cdn_saas 专属：DB 字段保持 tls，确保客户端 TLS 开启
	// 注意：此处不做 DB 字段分离，只做字段一致性纠正（确保 DB 和 config_json 都是 tls）
	if isCDNExposureNode(node) {
		tlsSec := "tls"
		node.SecurityType = &tlsSec
		node.ConfigJSON["security_type"] = "tls"
		node.ConfigJSON["security"] = "tls"
		node.ConfigJSON["tls"] = 1
		// CDN 节点由 nginx ACME 管理证书，不绑定 cert_bundle
		delete(node.ConfigJSON, "cert_bundle_id")
		// client_port 强制为 443（CDN 入口端口）
		node.Port = 443
	}

	// P1-6: security=tls 客户端/服务端字段一致性校验
	// - argo_tunnel（CF隧道）：DB=none, config_json=tls（字段分离），由 validateTLSSplitFields 校验
	// - cdn/cdn_saas/direct/reality：DB和config_json一致（不分离），由 validateNonSplitTLSConsistency 校验
	// 违反则返回错误阻断保存。
	if err := validateTLSSplitFields(node); err != nil {
		return fmt.Errorf("P1-6 TLS 分离校验: %w", err)
	}
	if err := validateNonSplitTLSConsistency(node); err != nil {
		return fmt.Errorf("P1-6 TLS 一致性校验: %w", err)
	}

	// 7.1 argo_tunnel 专属处理：WS/HTTPUpgrade ALPN + hostname 一致性校验
	if isArgoTunnelExposureNode(node) {
		// WS/HTTPUpgrade 传输 ALPN 必须 http/1.1（CF CDN 协商 HTTP/2 后 WS Upgrade 失败）
		if strings.EqualFold(node.TransportType, "ws") || strings.EqualFold(node.TransportType, "httpupgrade") {
			node.ConfigJSON["alpn"] = []string{"http/1.1"}
			node.ALPN = []string{"http/1.1"}
		}
		// cdn_address 自动同步 host_header（作为 CDN 路由域名）
		if node.HostHeader != nil && *node.HostHeader != "" {
			node.ConfigJSON["cdn_address"] = *node.HostHeader
		}
		// hostname 一致性校验：
		// cloudflared ingress 的 hostname 必须与 cdn_address/SNI 严格一致，
		// 否则 cloudflared 路由失败返回 502。
		// 校验维度：SNI、HostHeader、config_json.cdn_address 三者必须一致
		cdnAddr, _ := node.ConfigJSON["cdn_address"].(string)
		sniVal := ""
		if node.SNI != nil {
			sniVal = *node.SNI
		}
		hostVal := ""
		if node.HostHeader != nil {
			hostVal = *node.HostHeader
		}
		var refHost string
		switch {
		case cdnAddr != "":
			refHost = cdnAddr
		case sniVal != "":
			refHost = sniVal
		case hostVal != "":
			refHost = hostVal
		}
		if refHost != "" {
			if cdnAddr != "" && cdnAddr != refHost {
				return fmt.Errorf("argo_tunnel hostname 不一致: cdn_address=%s 但 SNI/HostHeader=%s，cloudflared ingress 会 502", cdnAddr, refHost)
			}
			if sniVal != "" && sniVal != refHost {
				return fmt.Errorf("argo_tunnel hostname 不一致: SNI=%s 但 cdn_address=%s，cloudflared ingress 会 502", sniVal, refHost)
			}
			if hostVal != "" && hostVal != refHost {
				return fmt.Errorf("argo_tunnel hostname 不一致: HostHeader=%s 但 cdn_address=%s，cloudflared ingress 会 502", hostVal, refHost)
			}
			if cdnAddr == "" {
				node.ConfigJSON["cdn_address"] = refHost
			}
			if sniVal == "" {
				sniCopy := refHost
				node.SNI = &sniCopy
			}
			if hostVal == "" {
				hostCopy := refHost
				node.HostHeader = &hostCopy
			}
		}
	}

	// 7.2 cdn/cdn_saas 专属处理：WS/HTTPUpgrade 传输 ALPN 必须 http/1.1
	// （nginx 8445 需 HTTP/1.1 支持 WebSocket Upgrade）
	// 注意：XHTTP 不强制覆盖 ALPN，packet-up/stream-up 模式支持 h2+http/1.1 协商，
	//   强制 http/1.1 会导致部分客户端 TLS 协商失败连不上。保留用户在面板配置的值。
	if isCDNExposureNode(node) {
		if strings.EqualFold(node.TransportType, "ws") || strings.EqualFold(node.TransportType, "httpupgrade") {
			node.ConfigJSON["alpn"] = []string{"http/1.1"}
			node.ALPN = []string{"http/1.1"}
		}
		// cdn_address 自动同步 host_header/SNI
		if node.HostHeader != nil && *node.HostHeader != "" {
			node.ConfigJSON["cdn_address"] = *node.HostHeader
		}
	}

	// 链式套娃出站 URI 校验（D5 三重防线：正则 + 长度 + 解析）
	// 在保存前拦截非法 URI，防止渲染时 ParseChainURI 失败导致整个配置下发中断
	if node.ConfigJSON != nil {
		if uri, ok := node.ConfigJSON["chain_outbound_uri"].(string); ok && uri != "" {
			if err := validateChainOutboundURI(uri); err != nil {
				return fmt.Errorf("invalid chain_outbound_uri: %w", err)
			}
		}
	}

	return nil
}

// chainURISchemeRe 套娃 URI 协议前缀正则（白名单）。
// 仅允许已知代理协议 scheme，拒绝 javascript:/file:/ 等危险前缀。
var chainURISchemeRe = regexp.MustCompile(`^(socks5h?|https?|trojan|vless|vmess|ss|hysteria2|hy2|tuic)://`)

// validateChainOutboundURI 三重校验套娃出站 URI（D5 补强）。
//  1. 长度限制（≤2048，防超长输入打满 parser）
//  2. scheme 正则白名单（防恶意前缀）
//  3. ParseChainURI 实际解析（防格式错误，凭证脱敏后返回 error）
func validateChainOutboundURI(uri string) error {
	if uri == "" {
		return nil
	}
	if len(uri) > 2048 {
		return fmt.Errorf("URI 长度超限（%d > 2048）", len(uri))
	}
	if !chainURISchemeRe.MatchString(strings.ToLower(uri)) {
		return fmt.Errorf("URI 协议前缀不支持")
	}
	if _, err := exposure.ParseChainURI(uri); err != nil {
		return fmt.Errorf("URI 解析失败: %w", err)
	}
	return nil
}

// validateExposureMode 校验节点暴露方式与证书类型的兼容性（P2 校验）。
// 硬约束：自签证书（cert_type=self_signed）节点禁止使用 cdn/cdn_saas/argo_tunnel 模式。
// 原因：自签证书仅在 nginx 本地回源场景（direct 直连模式下 xray 自身终止 TLS）安全可用；
// cdn/argo_tunnel 模式下 nginx/xray 之间是 HTTP 明文，不验证证书，理论上可行，但
// 为避免混淆和未来扩展，强制要求自签证书只能走 direct 直连。
func validateExposureMode(node *model.Node) error {
	if node == nil || node.ConfigJSON == nil {
		return nil
	}
	certType, _ := node.ConfigJSON["cert_type"].(string)
	if certType != "self_signed" {
		return nil
	}
	em := determineExposureMode(node)
	if node.ExposureMode != nil && *node.ExposureMode != "" {
		em = *node.ExposureMode
	}
	switch em {
	case "cdn", "cdn_saas", "argo_tunnel":
		return fmt.Errorf("自签证书节点 %s 只能使用 direct 模式（当前=%s），CDN/Tunnel 模式需使用可信 CA 签发的证书", node.Code, em)
	}
	return nil
}

// determineExposureMode 自动判定节点的暴露方式。
// 优先级：
//  1. 前端显式设置 config_json.exposure_mode（保留用户选择）
//  2. 有 cloudflared_tunnel_id/token → argo_tunnel
//  3. 有 cdn_address（域名）→ cdn_saas
//  4. 其他 → direct（直连，xray 直接监听 443）
func determineExposureMode(n *model.Node) string {
	if n == nil || n.ConfigJSON == nil {
		return "direct"
	}
	// 1. 前端显式设置
	if em, ok := n.ConfigJSON["exposure_mode"].(string); ok && em != "" {
		return em
	}
	// 2. tunnel 凭证
	if tid, ok := n.ConfigJSON["cloudflared_tunnel_id"].(string); ok && tid != "" {
		return "argo_tunnel"
	}
	if ttok, ok := n.ConfigJSON["cloudflared_tunnel_token"].(string); ok && ttok != "" {
		return "argo_tunnel"
	}
	// 3. cdn_address（域名，非 IP）
	if cdnAddr, ok := n.ConfigJSON["cdn_address"].(string); ok && cdnAddr != "" {
		return "cdn_saas"
	}
	// 4. 默认直连
	return "direct"
}

// isArgoTunnelExposureNode 判断节点是否为 argo_tunnel 暴露方式（需要 TLS 分离架构）
// 隧道节点：cloudflared HTTP 回源 + CF 边缘 TLS
func isArgoTunnelExposureNode(n *model.Node) bool {
	if n == nil || n.ConfigJSON == nil {
		return false
	}
	return determineExposureMode(n) == "argo_tunnel"
}

// isCDNExposureNode 判断节点是否为 cdn/cdn_saas 暴露方式（需要 TLS 分离架构）
// CDN 节点：nginx 8445 终止 TLS + proxy_pass HTTP 回源 + CF 边缘 TLS
func isCDNExposureNode(n *model.Node) bool {
	if n == nil || n.ConfigJSON == nil {
		return false
	}
	em := determineExposureMode(n)
	return em == "cdn" || em == "cdn_saas"
}

// isTLSSplitExposureNode 判断节点是否需要 DB 字段层面的 TLS 分离架构。
// 仅 argo_tunnel（CF 隧道）需要 DB 字段分离：
//   - DB security_type = "none"（cloudflared 明文 HTTP 回源，xray 无 TLS）
//   - config_json.security_type = "tls"（客户端到 CF Edge 必须 TLS）
//
// cdn/cdn_saas 节点不做 DB 字段分离（DB security_type 保持 "tls"），
// TLS 剥离纯粹在渲染层由 shouldStripTLSForNginxVhost 动态完成
// （nginx 8445 终止 TLS 后 proxy_pass 明文回源 xray）。
// 两类节点的剥离触发条件完全独立，避免"改 CDN 逻辑连带影响隧道节点"的耦合风险。
func isTLSSplitExposureNode(n *model.Node) bool {
	return isArgoTunnelExposureNode(n)
}

// allocateDownloadServerPort 为 XHTTP split mode (上下行分离) 节点自动分配下行监听端口。
// 规则：
//   - 仅当 transport=xhttp 且存在 downloadSettings 时触发
//   - 若 downloadSettings.security 与主 security 不同（即上下行走不同TLS/Reality路径），
//     则在对应端口段中分配一个空闲高位端口
//   - 分配的端口写回 config_json.xhttp.extra.downloadSettings.server_port
//   - 若 downloadSettings.server_port 已存在且端口未被占用则保留
func (s *NodeService) allocateDownloadServerPort(ctx context.Context, node *model.Node) error {
	if node.ConfigJSON == nil {
		return nil
	}

	// 提取 downloadSettings map（优先 xhttp.extra.downloadSettings，兼容顶层 download_settings）
	var dsMap map[string]interface{}
	if xhttpMap, ok := node.ConfigJSON["xhttp"].(map[string]interface{}); ok {
		if extraMap, ok := xhttpMap["extra"].(map[string]interface{}); ok {
			if ds, ok := extraMap["downloadSettings"].(map[string]interface{}); ok && len(ds) > 0 {
				dsMap = ds
			}
		}
	}
	if dsMap == nil {
		if ds, ok := node.ConfigJSON["download_settings"].(map[string]interface{}); ok && len(ds) > 0 {
			dsMap = ds
		}
	}
	if dsMap == nil {
		return nil
	}

	// 提取下行安全类型
	dlSec := ""
	if s, ok := dsMap["security"].(string); ok {
		dlSec = s
	}

	// 主安全类型
	mainSec := ""
	if node.SecurityType != nil {
		mainSec = *node.SecurityType
	}
	if mainSec == "" {
		mainSec = inferSecurityFromConfig(node.ConfigJSON)
	}

	// 如果下行安全类型与主安全类型相同，不需要双inbound（同安全不需要独立端口）
	// 只有当安全类型不同时（如上行TLS+CDN，下行Reality直连）才需要分配下行端口
	if dlSec == "" || dlSec == mainSec {
		return nil
	}

	// 如果已设置server_port且端口在有效范围内，保留
	if existingPort, ok := dsMap["server_port"].(float64); ok && existingPort > 0 {
		return nil
	}

	// 需要分配下行端口
	if s.portPlanner == nil {
		return nil // port planner 未初始化，跳过（precheck 阶段会报错）
	}
	rt, err := s.runtimeRepo.GetByID(ctx, node.RuntimeID)
	if err != nil {
		return fmt.Errorf("get runtime for download port allocation: %w", err)
	}
	if rt == nil {
		return fmt.Errorf("runtime %s not found", node.RuntimeID)
	}

	// 下行传输类型（默认xhttp）
	dlTransport := "xhttp"
	if t, ok := dsMap["network"].(string); ok && t != "" {
		dlTransport = t
	}

	dlPort, err := s.portPlanner.AllocateServerPort(ctx, rt.ServerID, node.NodeType, node.ProtocolType, dlTransport, dlSec)
	if err != nil {
		return fmt.Errorf("auto-allocate download server_port (security=%s): %w", dlSec, err)
	}
	dsMap["server_port"] = dlPort

	return nil
}

// inferSecurityFromConfig 从config_json推断安全类型
func inferSecurityFromConfig(cfg map[string]interface{}) string {
	if cfg == nil {
		return ""
	}
	if s, ok := cfg["security"].(string); ok && s != "" {
		return s
	}
	if _, ok := cfg["reality_settings"]; ok {
		return "reality"
	}
	if _, ok := cfg["reality"]; ok {
		return "reality"
	}
	if _, ok := cfg["tls_settings"]; ok {
		return "tls"
	}
	if _, ok := cfg["tls"]; ok {
		return "tls"
	}
	return "none"
}

// CreateNodeFromURIRequest 从 URI 创建节点的请求参数
type CreateNodeFromURIRequest struct {
	Name          string                 `json:"name"`
	ProtocolType  string                 `json:"protocol_type"`
	TransportType string                 `json:"transport_type"`
	SecurityType  string                 `json:"security_type"`
	Host          string                 `json:"host"`
	Port          int                    `json:"port"`
	ConfigJSON    map[string]interface{} `json:"config_json"`
	ServerID      *uuid.UUID             `json:"server_id"`
	RuntimeID     *uuid.UUID             `json:"runtime_id"`
	Code          string                 `json:"code"`
	Region        string                 `json:"region"`
	GroupID       *uuid.UUID             `json:"group_id"`
	Multiplier    float64                `json:"multiplier"`
}

func (s *NodeService) CreateNodeFromURI(ctx context.Context, req *CreateNodeFromURIRequest) (*model.Node, error) {
	var runtimeID uuid.UUID
	if req.RuntimeID != nil {
		runtimeID = *req.RuntimeID
	} else if req.ServerID != nil {
		runtimes, err := s.runtimeRepo.ListByServer(ctx, *req.ServerID)
		if err != nil {
			return nil, err
		}
		if len(runtimes) == 0 {
			return nil, ErrRuntimeNotFound
		}
		runtimeID = runtimes[0].ID
	} else {
		return nil, ErrRuntimeNotFound
	}

	runtime, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, ErrRuntimeNotFound
	}

	code := req.Code
	if code == "" {
		code = fmt.Sprintf("%s-%s-%d", req.ProtocolType, req.Host, req.Port)
	}

	existing, err := s.nodeRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		suffix := 1
		for {
			newCode := fmt.Sprintf("%s-%d", code, suffix)
			existing2, _ := s.nodeRepo.GetByCode(ctx, newCode)
			if existing2 == nil {
				code = newCode
				break
			}
			suffix++
			if suffix > 100 {
				return nil, ErrNodeAlreadyExists
			}
		}
	}

	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s:%d", req.Host, req.Port)
	}

	securityType := req.SecurityType
	if securityType == "" {
		securityType = "none"
	}
	secTypePtr := &securityType

	trafficRate := 1.0
	if req.Multiplier > 0 {
		trafficRate = req.Multiplier
	}

	transportType := req.TransportType
	if transportType == "" {
		transportType = "tcp"
	}

	configJSON := req.ConfigJSON
	if configJSON == nil {
		configJSON = make(map[string]interface{})
	}

	node := &model.Node{
		ID:                    uuid.New(),
		Code:                  code,
		Name:                  name,
		RuntimeID:             runtimeID,
		RegionID:              nil,
		GroupID:               req.GroupID,
		NodeType:              model.NodeTypeStandard,
		ProtocolType:          req.ProtocolType,
		TransportType:         transportType,
		SecurityType:          secTypePtr,
		Address:               req.Host,
		Port:                  req.Port,
		SNI:                   nil,
		ALPN:                  []string{},
		Path:                  nil,
		HostHeader:            nil,
		Flow:                  nil,
		IsEnabled:             true,
		IsVisible:             true,
		AllowUDP:              true,
			SpeedLimitMbps:        nil,
			TrafficRate:           trafficRate,
			Priority:              100,
			CapacityScore:         100,
			ProtocolSchemaVersion: "v1",
			ConfigJSON:            configJSON,
			Tags:                  []string{},
			Metadata:              make(map[string]interface{}),
			LastPublishedVersion:  0,
		}

		// Bug-B1: 自动为 REALITY 节点补全 private_key/public_key。
		// B39 修复: autoGenerateREALITYKeys 失败不再静默吞掉，以 Error 级别记录
		if err := autoGenerateREALITYKeys(ctx, node); err != nil {
			log.Printf("error: auto-generate REALITY keys failed for node from URI %s: %v", node.Code, err)
		}

		if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, err
	}

	healthStatus := &model.NodeHealthStatus{
		NodeID:            node.ID,
		OverallStatus:     "unknown",
		HeartbeatStatus:   "unknown",
		ProbeStatus:       "unknown",
		AvailabilityScore: 0,
		LatencyScore:      0,
		LossScore:         0,
		HandshakeScore:    0,
		ChainScore:        0,
		StabilityScore:    0,
	}
	if err := s.healthRepo.UpsertStatus(ctx, healthStatus); err != nil {
		return nil, err
	}

	return node, nil
}

func MapNodeErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrNodeAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrNodeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrRuntimeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrInvalidProtocolType):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrConfigValidation):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrNodeGroupAlreadyExists):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrNodeGroupNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrNodeGroupInUse):
		return config.CodeConflict, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}

// extractXHTTPDownloadSettings 从 config_json 中提取 XHTTP downloadSettings map。
// 兼容多种存储路径：xhttp.extra.downloadSettings > xhttp.downloadSettings > download_settings
func extractXHTTPDownloadSettings(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	// 优先路径：xhttp.extra.downloadSettings（normalizer 保留嵌套结构）
	if xhttp, ok := cfg["xhttp"].(map[string]interface{}); ok {
		if extra, ok := xhttp["extra"].(map[string]interface{}); ok {
			if ds, ok := extra["downloadSettings"].(map[string]interface{}); ok {
				return ds
			}
		}
		if ds, ok := xhttp["downloadSettings"].(map[string]interface{}); ok {
			return ds
		}
	}
	// 兼容：顶层 download_settings
	if ds, ok := cfg["download_settings"].(map[string]interface{}); ok {
		return ds
	}
	return nil
}

// routeNodeToCorrectRuntime 根据节点协议/传输/安全特性，将节点路由到正确的内核 runtime。
//
// 路由规则：
//   - Hysteria2 / TUIC / AnyTLS / Naive → 仅 sing-box 支持，必须路由到 sing-box runtime
//   - XHTTP split mode (downloadSettings) → 仅 xray 支持，必须路由到 xray runtime
//   - ECH (TLS encrypted_client_hello) → 仅 xray 支持
//   - 其他协议（VLESS/Trojan/VMess/SS over TCP/WS/gRPC/HTTP2/XHTTP）→ xray 为默认
//
// 如果同 server 下缺少目标类型 runtime，返回明确错误提示。
func (s *NodeService) routeNodeToCorrectRuntime(ctx context.Context, node *model.Node) error {
	currentRT, err := s.runtimeRepo.GetByID(ctx, node.RuntimeID)
	if err != nil {
		return fmt.Errorf("查询当前runtime失败: %w", err)
	}
	if currentRT == nil {
		return fmt.Errorf("节点绑定的runtime不存在: %s", node.RuntimeID)
	}

	// 判定节点需要的目标内核类型
	secStr := ""
	if node.SecurityType != nil {
		secStr = *node.SecurityType
	}
	requiredKernel := detectRequiredKernel(node.ProtocolType, node.TransportType, secStr, node.ConfigJSON)

	// 当前 runtime 类型已经是所需类型，无需切换
	if runtimeMatches(currentRT.RuntimeType, requiredKernel) {
		return nil
	}

	// 查找同 server 下匹配的 runtime
	runtimes, err := s.runtimeRepo.ListByServer(ctx, currentRT.ServerID)
	if err != nil {
		return fmt.Errorf("查询server下runtime列表失败: %w", err)
	}
	var targetRT *model.Runtime
	for _, rt := range runtimes {
		if runtimeMatches(rt.RuntimeType, requiredKernel) {
			targetRT = rt
			break
		}
	}
	if targetRT == nil {
		return fmt.Errorf("该节点需要%s内核，但服务器上没有%s类型的运行时。请先在该服务器创建%s运行时",
			requiredKernel, requiredKernel, requiredKernel)
	}

	slog.Info("auto-routing node to correct runtime",
		"node_code", node.Code, "protocol", node.ProtocolType,
		"transport", node.TransportType,
		"from_runtime", currentRT.ID, "from_type", currentRT.RuntimeType,
		"to_runtime", targetRT.ID, "to_type", targetRT.RuntimeType,
		"required_kernel", requiredKernel)
	node.RuntimeID = targetRT.ID
	return nil
}

// detectRequiredKernel 根据节点协议/传输/安全配置，判断需要哪个内核
func detectRequiredKernel(protocol, transport, security string, cfg map[string]interface{}) string {
	// P0: sing-box only 协议
	switch protocol {
	case "hysteria2", "tuic", "anytls", "naive":
		return "sing-box"
	}

	// XHTTP split mode（downloadSettings）→ xray only
	if transport == "xhttp" {
		if ds := extractXHTTPDownloadSettings(cfg); ds != nil && len(ds) > 0 {
			return "xray"
		}
	}

	// ECH (Encrypted Client Hello) → xray only
	if isECHInConfig(cfg) {
		return "xray"
	}

	// 默认：xray（主内核）
	return "xray"
}

// isECHInConfig 直接从 config_json 检测 ECH 是否启用
func isECHInConfig(cfg map[string]interface{}) bool {
	if cfg == nil {
		return false
	}
	tls, ok := cfg["tls"].(map[string]interface{})
	if !ok {
		return false
	}
	ech, ok := tls["ech"].(map[string]interface{})
	if !ok {
		return false
	}
	if enabled, ok := ech["enabled"].(bool); ok {
		return enabled
	}
	// 如果有 ech 配置但没有 enabled 字段，视为启用
	return len(ech) > 0
}

// runtimeMatches 判断 runtime_type 是否匹配目标内核类型
func runtimeMatches(runtimeType, targetKernel string) bool {
	return normalizeRuntimeType(runtimeType) == targetKernel
}

// isXrayRuntime 判断 runtime_type 是否为 xray 类型
func isXrayRuntime(rt string) bool {
	return normalizeRuntimeType(rt) == "xray"
}
