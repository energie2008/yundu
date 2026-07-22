package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/airport-panel/node-service/internal/repo"
)

// AgentBootstrapService 实现 Agent 零配置部署的 Bootstrap API。
//
// 设计目标：让 node-agent 只需 --endpoint / --token / --runtime 三个参数即可启动，
// 其余配置（runtime_bin、config_dir、heartbeat_interval、grpc_addr、节点列表等）
// 均由面板根据 agent_token 自动推断并下发。
//
// token 查找逻辑：agent_token = HMAC-SHA256(salt, serverCode)，是确定性派生值，
// 因此 Bootstrap 端点遍历所有 server 并用 hmac.Equal 校验，找到匹配的 server。
// 该操作仅在 Agent 首次启动时执行一次（之后走心跳/配置轮询），O(n) 遍历可接受。
type AgentBootstrapService struct {
	serverService     *ServerService
	runtimeService    *RuntimeService
	deploymentService *DeploymentService
	nodeRepo          *repo.NodeRepo
	tokenSalt         string
	logger            *slog.Logger
}

// BootstrapConfig 是 Bootstrap API 下发给 Agent 的运行时配置。
type BootstrapConfig struct {
	AgentID           string          `json:"agent_id"`
	RuntimeType       string          `json:"runtime_type"`        // xray / sing-box
	RuntimeBin        string          `json:"runtime_bin"`          // /usr/local/bin/xray
	ConfigDir         string          `json:"config_dir"`           // /etc/yundu
	HeartbeatInterval int             `json:"heartbeat_interval"`   // 15
	GRPCAddr          string          `json:"grpc_addr,omitempty"`  // panel:9082
	NginxEnabled      bool            `json:"nginx_enabled"`
	CertMode          string          `json:"cert_mode"` // acme / file / content
	WARPEnabled       bool            `json:"warp_enabled"`
	Nodes             []BootstrapNode `json:"nodes"`
}

// BootstrapNode 是 Bootstrap 下发的节点摘要，供 Agent 启动时预知需监听的端口。
type BootstrapNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// NewAgentBootstrapService 构造 Bootstrap 服务。
// tokenSalt 用于校验 agent_token；nodeRepo 用于查询 server 下的节点列表。
func NewAgentBootstrapService(
	serverService *ServerService,
	runtimeService *RuntimeService,
	deploymentService *DeploymentService,
	nodeRepo *repo.NodeRepo,
	tokenSalt string,
	logger *slog.Logger,
) *AgentBootstrapService {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentBootstrapService{
		serverService:     serverService,
		runtimeService:    runtimeService,
		deploymentService: deploymentService,
		nodeRepo:          nodeRepo,
		tokenSalt:         tokenSalt,
		logger:            logger.With("component", "agent-bootstrap-service"),
	}
}

// GetBootstrapConfig 根据 agent_token 返回 Agent 运行时配置。
//
// 步骤：
//  1. 通过 token 查找对应的 server（遍历 server 列表，HMAC 校验）
//  2. 查找该 server 下的 runtime 记录，确定 runtime_type
//  3. 返回合理的默认值（runtime_bin 根据类型自动推断路径）
//  4. 查找该 server 下的所有节点，返回节点列表
func (s *AgentBootstrapService) GetBootstrapConfig(ctx context.Context, token string) (*BootstrapConfig, error) {
	if token == "" {
		return nil, ErrCodeRequired
	}

	// 1. 通过 token 查找 server
	srv, err := s.findServerByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	s.logger.Info("bootstrap: server matched by token",
		"server_id", srv.ID, "server_code", srv.Code)

	// 2. 查找 runtime 记录，确定 runtime_type
	runtimes, err := s.runtimeService.ListRuntimes(ctx, srv.ID)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	runtimeType := "xray" // 默认 xray
	for _, rt := range runtimes {
		if rt.Status == model.RuntimeStatusActive && rt.RuntimeType != "" {
			runtimeType = rt.RuntimeType
			break
		}
	}
	// 若没有 active runtime，取第一个非空 runtime_type
	if runtimeType == "xray" {
		for _, rt := range runtimes {
			if rt.RuntimeType != "" {
				runtimeType = rt.RuntimeType
				break
			}
		}
	}

	// 3. 推断 runtime_bin 路径
	runtimeBin := inferRuntimeBin(runtimeType)

	// 4. 查找该 server 下的所有节点
	nodes := make([]BootstrapNode, 0)
	for _, rt := range runtimes {
		rtNodes, err := s.nodeRepo.ListByRuntimeID(ctx, rt.ID)
		if err != nil {
			s.logger.Warn("bootstrap: list nodes by runtime failed",
				"runtime_id", rt.ID, "error", err)
			continue
		}
		for _, n := range rtNodes {
			nodes = append(nodes, BootstrapNode{
				ID:       n.ID.String(),
				Name:     n.Name,
				Protocol: n.ProtocolType,
				Port:     n.Port,
			})
		}
	}

	cfg := &BootstrapConfig{
		AgentID:           srv.Code,
		RuntimeType:       runtimeType,
		RuntimeBin:        runtimeBin,
		ConfigDir:         "/etc/yundu",
		HeartbeatInterval: 15,
		GRPCAddr:          "", // 留空则 agent 从 PanelURL 推导
		NginxEnabled:      false,
		CertMode:          "acme",
		WARPEnabled:       false,
		Nodes:             nodes,
	}

	s.logger.Info("bootstrap config generated",
		"server_code", srv.Code,
		"runtime_type", runtimeType,
		"runtime_bin", runtimeBin,
		"node_count", len(nodes))

	return cfg, nil
}

// findServerByToken 遍历所有 server，用 HMAC-SHA256 校验 token 是否匹配。
// 返回匹配的 server；若无一匹配返回 (nil, nil)。
func (s *AgentBootstrapService) findServerByToken(ctx context.Context, token string) (*model.Server, error) {
	// 分页遍历所有 server（status="" 表示不过滤状态）
	page, pageSize := 1, 200
	for {
		servers, total, err := s.serverService.ListServers(ctx, page, pageSize, "", "")
		if err != nil {
			return nil, fmt.Errorf("list servers: %w", err)
		}
		for _, srv := range servers {
			if pkg.ValidateAgentToken(srv.Code, token, s.tokenSalt) {
				return srv, nil
			}
		}
		if page*pageSize >= total {
			break
		}
		page++
	}
	return nil, nil
}

// inferRuntimeBin 根据 runtime_type 推断二进制路径。
func inferRuntimeBin(runtimeType string) string {
	switch runtimeType {
	case "sing-box":
		return "/usr/local/bin/sing-box"
	case "xray":
		fallthrough
	default:
		return "/usr/local/bin/xray"
	}
}

// MachineNodeEntry 表示 Machine 模式下单个节点的摘要信息。
// node-agent 的 MachineOrchestrator 通过此信息为每个节点创建子 Agent。
type MachineNodeEntry struct {
	ServerCode  string `json:"server_code"`  // node.Code，子 Agent 用此作为 server_code 认证
	RuntimeType string `json:"runtime_type"` // xray / sing-box
	RuntimeID   string `json:"runtime_id"`   // runtime.ID (UUID string)
	AgentToken  string `json:"agent_token"`  // T04: HMAC-SHA256(salt, serverCode)，子 Agent 认证用
	ListenPort  int    `json:"listen_port"`  // T04: 节点内部监听端口（ServerPort）
}

// MachineNodesResponse 是 GET /api/v1/agent/machine/nodes 的响应结构。
type MachineNodesResponse struct {
	Nodes []MachineNodeEntry `json:"nodes"`
}

// GetMachineNodes 根据 server_token 返回该服务器上所有需要 Machine Agent 托管的节点列表。
//
// 用于 Machine 模式（--mode machine）：单个 Agent 进程托管 N 个节点。
// MachineOrchestrator 定期调用此 API 发现新增节点、移除已删除节点。
//
// 认证方式与 Bootstrap 相同：通过 server_token（即 agent_token = HMAC(salt, serverCode)）
// 查找对应的 server，然后返回该 server 下所有 runtime 的节点列表。
func (s *AgentBootstrapService) GetMachineNodes(ctx context.Context, token string) (*MachineNodesResponse, error) {
	if token == "" {
		return nil, ErrCodeRequired
	}

	// 1. 通过 token 查找 server
	srv, err := s.findServerByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	// 2. 查找该 server 下的所有 runtime
	runtimes, err := s.runtimeService.ListRuntimes(ctx, srv.ID)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	// 3. 对每个 runtime，查询其下的所有启用节点
	nodes := make([]MachineNodeEntry, 0)
	for _, rt := range runtimes {
		rtNodes, err := s.nodeRepo.ListByRuntimeID(ctx, rt.ID)
		if err != nil {
			s.logger.Warn("machine nodes: list nodes by runtime failed",
				"runtime_id", rt.ID, "error", err)
			continue
		}
		for _, n := range rtNodes {
			entry := MachineNodeEntry{
				ServerCode:  n.Code,
				RuntimeType: rt.RuntimeType,
				RuntimeID:   rt.ID.String(),
				// T04: 补全 AgentToken（HMAC-SHA256 派生，与 Bootstrap 一致）
				AgentToken: pkg.GenerateAgentToken(srv.Code, s.tokenSalt),
			}
			// T04: 补全 ListenPort（从 node.ServerPort 取值）
			if n.ServerPort != nil {
				entry.ListenPort = *n.ServerPort
			}
			nodes = append(nodes, entry)
		}
	}

	s.logger.Info("machine nodes fetched",
		"server_code", srv.Code,
		"runtime_count", len(runtimes),
		"node_count", len(nodes))

	return &MachineNodesResponse{Nodes: nodes}, nil
}

// MachineCloudflaredTunnelsResponse 是 GET /api/v1/agent/machine/cloudflared-tunnels 的响应结构。
// T05: 返回该服务器上所有节点的 cloudflared 隧道配置聚合，供 Machine 模式统一管理 cloudflared。
type MachineCloudflaredTunnelsResponse struct {
	Tunnels []MachineCloudflaredTunnel `json:"tunnels"`
}

// MachineCloudflaredTunnel 单条 cloudflared 隧道配置（Machine 模式聚合）。
type MachineCloudflaredTunnel struct {
	Token    string                 `json:"token,omitempty"`
	TunnelID string                 `json:"tunnel_id,omitempty"`
	Ingress  []MachineIngressRule   `json:"ingress,omitempty"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

// MachineIngressRule cloudflared ingress 路由规则（Machine 模式聚合）。
type MachineIngressRule struct {
	Hostname      string                 `json:"hostname"`
	Service       string                 `json:"service"`
	OriginRequest map[string]interface{} `json:"originRequest,omitempty"`
}

// MachineCloudflaredTunnels 根据 server_token 返回该服务器上所有节点的 cloudflared 隧道配置聚合。
//
// T05: 用于 Machine 模式（--mode machine）：MachineOrchestrator 的 cloudflared reconciler
// 定期调用此 API 同步整台机器的 cloudflared 隧道配置（argo_tunnel / cdn_saas 类型节点）。
// 认证方式与 MachineCDNVhosts 相同：通过 server_token 查询参数验证身份。
func (s *AgentBootstrapService) MachineCloudflaredTunnels(ctx context.Context, token string) (*MachineCloudflaredTunnelsResponse, error) {
	if token == "" {
		return nil, ErrCodeRequired
	}

	// 1. 通过 token 查找 server
	srv, err := s.findServerByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	// 2. 查找该 server 下的所有 runtime
	runtimes, err := s.runtimeService.ListRuntimes(ctx, srv.ID)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	// 3. 遍历每个 runtime 的节点，筛选出 argo_tunnel / cdn_saas 类型节点
	tunnels := make([]MachineCloudflaredTunnel, 0)
	for _, rt := range runtimes {
		rtNodes, err := s.nodeRepo.ListByRuntimeID(ctx, rt.ID)
		if err != nil {
			s.logger.Warn("machine cloudflared-tunnels: list nodes by runtime failed",
				"runtime_id", rt.ID, "error", err)
			continue
		}
		for _, n := range rtNodes {
			if n.ConfigJSON == nil {
				continue
			}
			exposureMode, _ := n.ConfigJSON["exposure_mode"].(string)
			if exposureMode != "argo_tunnel" && exposureMode != "cdn_saas" {
				if n.Metadata != nil {
					if em, ok := n.Metadata["exposure_mode"].(string); ok {
						exposureMode = em
					}
				}
			}
			if exposureMode != "argo_tunnel" && exposureMode != "cdn_saas" {
				continue
			}

			tunnel := MachineCloudflaredTunnel{}
			if token, ok := n.ConfigJSON["cloudflared_token"].(string); ok {
				tunnel.Token = token
			}
			if tunnelID, ok := n.ConfigJSON["cloudflared_tunnel_id"].(string); ok {
				tunnel.TunnelID = tunnelID
			}
			// 构建 ingress 规则：SNI → 节点本地端口
		if sni := n.SNI; sni != nil && *sni != "" {
			port := n.Port
			if port == 0 {
				port = 443
			}
			// cloudflared token 模式：明文 HTTP 回源，xray 必须 security=none（TLS 剥离方案）
			// service 用 http://127.0.0.1:<port> 而非 http://localhost:<port>（IPv6 解析问题）
			service := "http://127.0.0.1:" + strconv.Itoa(port)
			tunnel.Ingress = append(tunnel.Ingress, MachineIngressRule{
				Hostname: *sni,
				Service:  service,
			})
		}
			if tunnel.Token != "" || tunnel.TunnelID != "" {
				tunnels = append(tunnels, tunnel)
			}
		}
	}

	s.logger.Info("machine cloudflared-tunnels fetched",
		"server_code", srv.Code,
		"tunnel_count", len(tunnels))

	return &MachineCloudflaredTunnelsResponse{Tunnels: tunnels}, nil
}

// MachineCDNVhostsResponse 是 GET /api/v1/agent/machine/cdn-vhosts 的响应结构。
// 返回该服务器上所有节点聚合后的 nginx vhost 配置片段，供 Machine 模式统一管理 nginx。
type MachineCDNVhostsResponse struct {
	HTTPSnippet     string   `json:"https_snippet"`
	StreamSnippet   string   `json:"stream_snippet"`
	ListenPort      int      `json:"listen_port"`
	Domains         []string `json:"domains"`
	DefaultUpstream string   `json:"default_upstream,omitempty"`
}

// MachineCDNVhosts 根据 server_token 返回该服务器上所有节点聚合后的 nginx vhost 配置。
//
// 用于 Machine 模式（--mode machine）：单个 Agent 进程托管 N 个节点，
// nginx 配置需要聚合整台机器上所有 CDN+REALITY 节点的 SNI 分流规则。
// MachineOrchestrator 定期调用此 API 同步 nginx 配置。
func (s *AgentBootstrapService) MachineCDNVhosts(ctx context.Context, token string) (*MachineCDNVhostsResponse, error) {
	if token == "" {
		return nil, ErrCodeRequired
	}

	srv, err := s.findServerByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, ErrServerNotFound
	}

	vhosts, err := s.deploymentService.BuildNginxVhostsForServer(ctx, srv.ID)
	if err != nil {
		return nil, fmt.Errorf("build nginx vhosts for server: %w", err)
	}

	resp := &MachineCDNVhostsResponse{
		HTTPSnippet:     getStringFromMap(vhosts, "https_snippet"),
		StreamSnippet:   getStringFromMap(vhosts, "stream_snippet"),
		ListenPort:      getIntFromMap(vhosts, "listen_port"),
		Domains:         getStringSliceFromMap(vhosts, "domains"),
		DefaultUpstream: getStringFromMap(vhosts, "stream_default_upstream"),
	}

	s.logger.Info("machine cdn vhosts fetched",
		"server_code", srv.Code,
		"domains", len(resp.Domains))

	return resp, nil
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntFromMap(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	return 8445
}

func getStringSliceFromMap(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]string); ok {
		return v
	}
	if v, ok := m[key].([]interface{}); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return []string{}
}
