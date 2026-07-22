package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type PaginationResponse struct {
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
	Items    interface{} `json:"items"`
}

type CreateServerRequest struct {
	Code      string                 `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name      string                 `json:"name" binding:"required,min=1,max=128"`
	RegionID  *uuid.UUID             `json:"region_id"`
	Provider  *string                `json:"provider"`
	Host      string                 `json:"host" binding:"required,min=1,max=255"`
	IPv4      *string                `json:"ipv4"`
	IPv6      *string                `json:"ipv6"`
	SSHPort   *int                   `json:"ssh_port"`
	OSName    *string                `json:"os_name"`
	OSVersion *string                `json:"os_version"`
	Arch      *string                `json:"arch"`
	Role      ServerRole             `json:"role"`
	Labels    map[string]string      `json:"labels"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type ServerListQuery struct {
	Page     int          `form:"page"`
	PageSize int          `form:"page_size"`
	Status   ServerStatus `form:"status"`
	Search   string       `form:"search"`
}

type ServerSystemMetrics struct {
	CPUPercent      float64 `json:"cpu_percent"`
	MemPercent      float64 `json:"mem_percent"`
	MemTotalMB      int64   `json:"mem_total_mb"`
	MemUsedMB       int64   `json:"mem_used_mb"`
	DiskPercent     float64 `json:"disk_percent"`
	DiskTotalGB     int64   `json:"disk_total_gb"`
	DiskUsedGB      int64   `json:"disk_used_gb"`
	NetworkInKBps   float64 `json:"network_in_kbps"`
	NetworkOutKBps  float64 `json:"network_out_kbps"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	OnlineUsers     int     `json:"online_users"`
}

type ServerResponse struct {
	ID              uuid.UUID            `json:"id"`
	Code            string               `json:"code"`
	Name            string               `json:"name"`
	RegionID        *uuid.UUID           `json:"region_id,omitempty"`
	Provider        *string              `json:"provider,omitempty"`
	Host            string               `json:"host"`
	IPv4            *string              `json:"ipv4,omitempty"`
	IPv6            *string              `json:"ipv6,omitempty"`
	SSHPort         *int                 `json:"ssh_port,omitempty"`
	OSName          *string              `json:"os_name,omitempty"`
	OSVersion       *string              `json:"os_version,omitempty"`
	Arch            *string              `json:"arch,omitempty"`
	Status          ServerStatus         `json:"status"`
	Role            ServerRole           `json:"role"`
	Labels          map[string]string    `json:"labels"`
	LastHeartbeatAt *time.Time           `json:"last_heartbeat_at,omitempty"`
	CreatedAt       string               `json:"created_at"`
	NodeCount       int                  `json:"node_count"`
	Metrics         *ServerSystemMetrics `json:"metrics,omitempty"`
	Runtimes        []RuntimeInfo        `json:"runtimes"`
	// Nodes 关联节点列表（仅在 GetServer 详情接口填充，列表接口为空）
	Nodes []AssociatedNodeInfo `json:"nodes,omitempty"`
}

// AssociatedNodeInfo 服务器关联节点的轻量信息（用于服务器详情页）
type AssociatedNodeInfo struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	ProtocolType string    `json:"protocol_type"`
	Port         int       `json:"port"`
	ServerPort   *int      `json:"server_port,omitempty"`
	Address      string    `json:"address"`
	IsEnabled    bool      `json:"is_enabled"`
	HealthStatus string    `json:"health_status"`
}

// NewAssociatedNodeInfo 从 Node 模型构造关联节点信息
func NewAssociatedNodeInfo(n *Node) AssociatedNodeInfo {
	health := "unknown"
	if n.Metadata != nil {
		if hs, ok := n.Metadata["health_status"].(string); ok && hs != "" {
			health = hs
		}
	}
	if !n.IsEnabled {
		health = "disabled"
	}
	return AssociatedNodeInfo{
		ID:           n.ID,
		Code:         n.Code,
		Name:         n.Name,
		ProtocolType: n.ProtocolType,
		Port:         n.Port,
		ServerPort:   n.ServerPort,
		Address:      n.Address,
		IsEnabled:    n.IsEnabled,
		HealthStatus: health,
	}
}

type RuntimeInfo struct {
	ID              uuid.UUID  `json:"id"`
	RuntimeType     string     `json:"runtime_type"`
	DisplayName     string     `json:"display_name"`
	RuntimeVersion  string     `json:"runtime_version"`
	Status          string     `json:"status"`
	APIPort         int        `json:"api_port,omitempty"`
	MemoryMB        int64      `json:"memory_mb,omitempty"`
	RestartCount    int        `json:"restart_count,omitempty"`
	UptimeSeconds   int64      `json:"uptime_seconds,omitempty"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
}

type ServerDetailResponse struct {
	ServerResponse
	AgentToken string `json:"agent_token,omitempty"`
	InstallCmd string `json:"install_cmd,omitempty"`
}

func NewServerResponse(s *Server) ServerResponse {
	return ServerResponse{
		ID:              s.ID,
		Code:            s.Code,
		Name:            s.Name,
		RegionID:        s.RegionID,
		Provider:        s.Provider,
		Host:            s.Host,
		IPv4:            s.IPv4,
		IPv6:            s.IPv6,
		SSHPort:         s.SSHPort,
		OSName:          s.OSName,
		OSVersion:       s.OSVersion,
		Arch:            s.Arch,
		Status:          s.Status,
		Role:            s.Role,
		Labels:          s.Labels,
		LastHeartbeatAt: s.LastHeartbeatAt,
		CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Runtimes:        []RuntimeInfo{},
	}
}

func normalizeRuntimeTypeLocal(rt string) string {
	switch rt {
	case "xray-core", "xray":
		return "xray"
	case "singbox", "sing-box":
		return "sing-box"
	}
	return rt
}

func runtimeTypeToDisplayName(rt string) string {
	switch normalizeRuntimeTypeLocal(rt) {
	case "xray":
		return "Xray"
	case "sing-box":
		return "Sing-box"
	default:
		return rt
	}
}

func NewServerResponseWithDetails(s *Server, runtimes []*Runtime, nodeCount int) ServerResponse {
	resp := NewServerResponse(s)
	resp.NodeCount = nodeCount

	// 从 server.Metadata["system"] 读取系统metrics
	if sys, ok := s.Metadata["system"].(map[string]interface{}); ok && len(sys) > 0 {
		m := &ServerSystemMetrics{}
		m.CPUPercent = asFloat64(sys["cpu_percent"])
		m.MemPercent = asFloat64(sys["mem_percent"])
		m.MemTotalMB = asInt64(sys["mem_total_mb"])
		m.MemUsedMB = asInt64(sys["mem_used_mb"])
		m.DiskPercent = asFloat64(sys["disk_percent"])
		m.DiskTotalGB = asInt64(sys["disk_total_gb"])
		m.DiskUsedGB = asInt64(sys["disk_used_gb"])
		m.NetworkInKBps = asFloat64(sys["network_in_kbps"])
		m.NetworkOutKBps = asFloat64(sys["network_out_kbps"])
		m.UptimeSeconds = asInt64(sys["uptime_seconds"])
		if onlineUsers, ok := sys["online_users"]; ok {
			m.OnlineUsers = int(asInt64(onlineUsers))
		}
		resp.Metrics = m
	}

	for _, rt := range runtimes {
		normalizedType := normalizeRuntimeTypeLocal(rt.RuntimeType)
		info := RuntimeInfo{
			ID:              rt.ID,
			RuntimeType:     normalizedType,
			DisplayName:     runtimeTypeToDisplayName(normalizedType),
			RuntimeVersion:  "",
			Status:          string(rt.Status),
			LastHeartbeatAt: rt.LastHeartbeatAt,
		}
		if rt.RuntimeVersion != nil {
			info.RuntimeVersion = *rt.RuntimeVersion
		}
		if rt.APIPort != nil {
			info.APIPort = *rt.APIPort
		}
		// 从 runtime.Metadata 读取内存/重启/运行时间等指标
		if rt.Metadata != nil {
			if memMB, ok := rt.Metadata["memory_mb"]; ok {
				info.MemoryMB = asInt64(memMB)
			}
			if restartCount, ok := rt.Metadata["restart_count"]; ok {
				info.RestartCount = int(asInt64(restartCount))
			}
			if uptime, ok := rt.Metadata["uptime_seconds"]; ok {
				info.UptimeSeconds = asInt64(uptime)
			}
		}
		resp.Runtimes = append(resp.Runtimes, info)
	}
	return resp
}

// asFloat64 安全地将 interface{} 转为 float64
func asFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case int32:
		return float64(x)
	}
	return 0
}

// asInt64 安全地将 interface{} 转为 int64
func asInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	}
	return 0
}

func NewServerDetailResponse(s *Server, agentToken, panelURL string) ServerDetailResponse {
	return ServerDetailResponse{
		ServerResponse: NewServerResponse(s),
		AgentToken:     agentToken,
		InstallCmd:     fmt.Sprintf("curl -sSL %s/install-node-agent.sh | bash -s -- --panel-url=%s --server-code=%s --agent-token=%s", panelURL, panelURL, s.Code, agentToken),
	}
}

type RegisterRuntimeRequest struct {
	RuntimeType         string                 `json:"runtime_type" binding:"required"`
	RuntimeVersion      *string                `json:"runtime_version"`
	ProviderType        RuntimeProviderType    `json:"provider_type"`
	ProviderRef         *string                `json:"provider_ref"`
	ListenHost          *string                `json:"listen_host"`
	APIPort             *int                   `json:"api_port"`
	XrayAPIPort         *int                   `json:"xray_api_port"`
	SingboxClashPort    *int                   `json:"singbox_clash_port"`
	Capabilities        map[string]interface{} `json:"capabilities"`
	ConfigSchemaVersion string                 `json:"config_schema_version"`
	Metadata            map[string]interface{} `json:"metadata"`
	Hostname            string                 `json:"hostname"`
	OS                  string                 `json:"os"`
	Arch                string                 `json:"arch"`
	AgentVersion        string                 `json:"agent_version"`
}

type RuntimeResponse struct {
	ID              uuid.UUID           `json:"id"`
	ServerID        uuid.UUID           `json:"server_id"`
	RuntimeType     string              `json:"runtime_type"`
	RuntimeVersion  *string             `json:"runtime_version,omitempty"`
	ProviderType    RuntimeProviderType `json:"provider_type"`
	Status          RuntimeStatus       `json:"status"`
	LastHeartbeatAt *time.Time          `json:"last_heartbeat_at,omitempty"`
	CreatedAt       string              `json:"created_at"`
}

func NewRuntimeResponse(r *Runtime) RuntimeResponse {
	return RuntimeResponse{
		ID:              r.ID,
		ServerID:        r.ServerID,
		RuntimeType:     r.RuntimeType,
		RuntimeVersion:  r.RuntimeVersion,
		ProviderType:    r.ProviderType,
		Status:          r.Status,
		LastHeartbeatAt: r.LastHeartbeatAt,
		CreatedAt:       r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type CreateNodeRequest struct {
	Code      string     `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name      string     `json:"name" binding:"required,min=1,max=128"`
	RuntimeID uuid.UUID  `json:"runtime_id" binding:"required"`
	RegionID  *uuid.UUID `json:"region_id"`
	GroupID   *uuid.UUID `json:"group_id"`
	// GroupIDs 节点所属的分组列表（多对多）
	// 向后兼容：若为空且 GroupID 非 nil，则使用 GroupID
	// 若同时提供 GroupID 和 GroupIDs，以 GroupIDs 为准
	GroupIDs []uuid.UUID `json:"group_ids"`
	// ChainIDs 保存节点绑定的代理链（路由组）列表，保存时整体覆盖
	ChainIDs              []uuid.UUID            `json:"chain_ids"`
	NodeType              NodeType               `json:"node_type"`
	ProtocolType          string                 `json:"protocol_type" binding:"required"`
	TransportType         string                 `json:"transport_type" binding:"required"`
	SecurityType          *string                `json:"security_type"`
	Address               string                 `json:"address" binding:"required"`
	Port                  int                    `json:"port" binding:"required,min=1,max=65535"`
	ServerPort            *int                   `json:"server_port"`
	RealityServerName     *string                `json:"reality_server_name"`
	SNI                   *string                `json:"sni"`
	ALPN                  []string               `json:"alpn"`
	Path                  *string                `json:"path"`
	HostHeader            *string                `json:"host_header"`
	Flow                  *string                `json:"flow"`
	AllowUDP              bool                   `json:"allow_udp"`
	SpeedLimitMbps        *int                   `json:"speed_limit_mbps"`
	DeviceLimit           *int                   `json:"device_limit"`
	PaddingScheme         *string                `json:"padding_scheme"`
	TrafficRate           *float64               `json:"traffic_rate"`
	Priority              *int                   `json:"priority"`
	CapacityScore         *int                   `json:"capacity_score"`
	ProtocolSchemaVersion string                 `json:"protocol_schema_version"`
	ConfigJSON            map[string]interface{} `json:"config_json"`
	Tags                  []string               `json:"tags"`
	Metadata              map[string]interface{} `json:"metadata"`
	// PlanIDs D9 修复: 创建节点时自动绑定到指定计划，用户订阅即可看到新节点
	PlanIDs []uuid.UUID `json:"plan_ids"`
	// CertBundleID 关联证书包 ID，写入 config_json.cert_bundle_id
	// 修复前后端不一致：前端发送顶层 cert_bundle_id，后端需写入 config_json
	CertBundleID *string `json:"cert_bundle_id"`
	// IsEnabled 新建节点时是否启用（默认 true）
	// 修复：前端 specToNodePayload 发送 is_enabled，但 CreateNodeRequest 缺该字段被 Gin 静默丢弃
	IsEnabled *bool `json:"is_enabled"`
	// IsVisible 新建节点时是否可见（默认 true）
	// 修复：前端 specToNodePayload 发送 is_visible，但 CreateNodeRequest 缺该字段被 Gin 静默丢弃
	IsVisible *bool `json:"is_visible"`
	// TransferEnableBytes 新建节点的流量限额（字节）
	// 修复：前端 specToNodePayload 发送 transfer_enable_bytes，但 CreateNodeRequest 缺该字段被 Gin 静默丢弃
	TransferEnableBytes *int64 `json:"transfer_enable_bytes"`
	// ExposureMode 上行暴露方式: direct/cdn/cdn_saas/argo_tunnel
	ExposureMode *string `json:"exposure_mode"`
	// DownstreamExposureMode 下行暴露方式（XHTTP split mode）: direct/reality/none
	DownstreamExposureMode *string `json:"downstream_exposure_mode"`
	// IsSplitMode 是否启用上下行分离（前端表单开关）
	IsSplitMode *bool `json:"is_split_mode"`
}

type UpdateNodeRequest struct {
	Name *string `json:"name"`
	// Code 节点编码（唯一标识），nil 表示不修改
	Code    *string    `json:"code"`
	// TransportType 传输类型（ws/grpc/xhttp 等），nil 表示不修改
	// 修复：前端切换 transport（如 ws→xhttp）时需更新 DB 列，否则编辑回显仍是旧值
	TransportType *string `json:"transport_type"`
	// ProtocolType 协议类型（vless/trojan/vmess/ss/hysteria2/tuic/anytls 等），nil 表示不修改
	// 修复：前端切换协议时需更新 DB 列，否则编辑回显仍是旧值
	ProtocolType *string `json:"protocol_type"`
	// TransferEnableBytes 节点流量限额（字节），nil 表示不修改
	// 修复：前端设置的流量限额需持久化到 DB，否则编辑回显永远为 0
	TransferEnableBytes *int64 `json:"transfer_enable_bytes"`
	GroupID *uuid.UUID `json:"group_id"`
	// GroupIDs 节点所属的分组列表（多对多，整体覆盖语义）
	// nil 表示不修改，空数组 [] 表示清空所有分组
	// 若同时提供 GroupID 和 GroupIDs，以 GroupIDs 为准
	GroupIDs []uuid.UUID `json:"group_ids"`
	// ChainIDs 用于整体覆盖节点绑定的代理链（路由组）列表
	// nil 表示不修改，空数组 [] 表示清空所有绑定
	ChainIDs          []uuid.UUID            `json:"chain_ids"`
	NodeType          *NodeType              `json:"node_type"`
	Address           *string                `json:"address"`
	Port              *int                   `json:"port"`
	ServerPort        *int                   `json:"server_port"`
	RealityServerName *string                `json:"reality_server_name"`
	SNI               *string                `json:"sni"`
	ALPN              []string               `json:"alpn"`
	Path           *string                `json:"path"`
	HostHeader     *string                `json:"host_header"`
	Flow           *string                `json:"flow"`
	SecurityType   *string                `json:"security_type"`
	IsEnabled      *bool                  `json:"is_enabled"`
	IsVisible      *bool                  `json:"is_visible"`
	AllowUDP       *bool                  `json:"allow_udp"`
	SpeedLimitMbps *int                   `json:"speed_limit_mbps"`
	DeviceLimit    *int                   `json:"device_limit"`
	PaddingScheme  *string                `json:"padding_scheme"`
	TrafficRate    *float64               `json:"traffic_rate"`
	Priority       *int                   `json:"priority"`
	CapacityScore  *int                   `json:"capacity_score"`
	ConfigJSON     map[string]interface{} `json:"config_json"`
	Tags           []string               `json:"tags"`
	Metadata       map[string]interface{} `json:"metadata"`
	// CertBundleID 关联证书包 ID，写入 config_json.cert_bundle_id
	// 修复前后端不一致：前端发送顶层 cert_bundle_id，后端需写入 config_json
	CertBundleID *string `json:"cert_bundle_id"`
	// ExposureMode 上行暴露方式: direct/cdn/cdn_saas/argo_tunnel
	ExposureMode *string `json:"exposure_mode"`
	// DownstreamExposureMode 下行暴露方式（XHTTP split mode）: direct/reality/none
	DownstreamExposureMode *string `json:"downstream_exposure_mode"`
	// IsSplitMode 是否启用上下行分离（前端表单开关）
	IsSplitMode *bool `json:"is_split_mode"`
}

type NodeListQuery struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"page_size"`
	Status       string `form:"status"`
	ProtocolType string `form:"protocol_type"`
	RegionID     string `form:"region_id"`
	GroupID      string `form:"group_id"`
	Search       string `form:"search"`
	IsEnabled    *bool  `form:"is_enabled"`
}

type NodeResponse struct {
	ID                   uuid.UUID              `json:"id"`
	Code                 string                 `json:"code"`
	Name                 string                 `json:"name"`
	RuntimeID            uuid.UUID              `json:"runtime_id"`
	RegionID             *uuid.UUID             `json:"region_id,omitempty"`
	GroupID              *uuid.UUID             `json:"group_id,omitempty"`
	NodeType             NodeType               `json:"node_type"`
	ProtocolType         string                 `json:"protocol_type"`
	TransportType        string                 `json:"transport_type"`
	SecurityType         *string                `json:"security_type,omitempty"`
	Address              string                 `json:"address"`
	Port                 int                    `json:"port"`
	ServerPort           *int                   `json:"server_port,omitempty"`
	RealityServerName    *string                `json:"reality_server_name,omitempty"`
	SNI                  *string                `json:"sni,omitempty"`
	ALPN                 []string               `json:"alpn"`
	Path                 *string                `json:"path,omitempty"`
	HostHeader           *string                `json:"host_header,omitempty"`
	Flow                 *string                `json:"flow,omitempty"`
	IsEnabled            bool                   `json:"is_enabled"`
	IsVisible            bool                   `json:"is_visible"`
	AllowUDP             bool                   `json:"allow_udp"`
	SpeedLimitMbps       *int                   `json:"speed_limit_mbps,omitempty"`
	DeviceLimit          *int                   `json:"device_limit,omitempty"`
	PaddingScheme        *string                `json:"padding_scheme,omitempty"`
	TrafficRate          float64                `json:"traffic_rate"`
	Priority             int                    `json:"priority"`
	CapacityScore        int                    `json:"capacity_score"`
	// TransferEnableBytes 节点流量限额（字节），用于前端编辑回显
	TransferEnableBytes  *int64                 `json:"transfer_enable_bytes,omitempty"`
	Tags                 []string               `json:"tags"`
	ConfigJSON           map[string]interface{} `json:"config"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	LastPublishedVersion int64                  `json:"last_published_version"`
	HealthStatus         *string                `json:"health_status,omitempty"`
	PlanCodes            []string               `json:"plan_codes"`
	// 扩展字段：节点编辑回显所需
	ServerInfo *ServerBrief    `json:"server_info,omitempty"`
	ChainIDs   []uuid.UUID     `json:"chain_ids"`
	GroupInfo  *NodeGroupBrief `json:"group_info,omitempty"`
	// GroupIDs 节点所属的所有分组 ID 列表（多对多，用于编辑回显）
	GroupIDs []uuid.UUID `json:"group_ids"`
	// Groups 节点所属的所有分组简要信息（多对多，用于编辑回显）
	Groups    []NodeGroupBrief `json:"groups"`
	CreatedAt string           `json:"created_at"`
	// P0-4: 保存/下发过程中的警告信息（不阻断操作）
	Warnings []string `json:"warnings,omitempty"`
	// P2-1: 配置下发状态（pending/pushed/applied/failed）
	DispatchStatus string `json:"dispatch_status,omitempty"`
	// P2-1: 下发目标版本号
	DispatchVersion int64 `json:"dispatch_version,omitempty"`
	// P2-1: 下发时间（RFC3339）
	DispatchTime string `json:"dispatch_time,omitempty"`
	// P2-1: 下发失败错误信息
	DispatchError string `json:"dispatch_error,omitempty"`
	// CertBundleID 关联的证书包 ID（从 config_json.cert_bundle_id 提取，用于前端编辑回显）
	CertBundleID string `json:"cert_bundle_id,omitempty"`
	// ExposureMode 上行暴露方式: direct/cdn/cdn_saas/argo_tunnel
	ExposureMode string `json:"exposure_mode,omitempty"`
	// DownstreamExposureMode 下行暴露方式（XHTTP split mode）
	DownstreamExposureMode string `json:"downstream_exposure_mode,omitempty"`
	// IsSplitMode 是否启用上下行分离
	IsSplitMode bool `json:"is_split_mode"`
}

// ServerBrief 节点关联的服务器简要信息（用于编辑回显）
type ServerBrief struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
	Host string    `json:"host"`
}

// NodeGroupBrief 节点所属分组的简要信息（用于编辑回显）
type NodeGroupBrief struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

func NewNodeResponse(n *Node) NodeResponse {
	resp := NodeResponse{
		ID:                     n.ID,
		Code:                   n.Code,
		Name:                   n.Name,
		RuntimeID:              n.RuntimeID,
		RegionID:               n.RegionID,
		GroupID:                n.GroupID,
		NodeType:               n.NodeType,
		ProtocolType:           n.ProtocolType,
		TransportType:          n.TransportType,
		SecurityType:           n.SecurityType,
		Address:                n.Address,
		Port:                   n.Port,
		ServerPort:             n.ServerPort,
		RealityServerName:      n.RealityServerName,
		SNI:                    n.SNI,
		ALPN:                   n.ALPN,
		Path:                   n.Path,
		HostHeader:             n.HostHeader,
		Flow:                   n.Flow,
		IsEnabled:              n.IsEnabled,
		IsVisible:              n.IsVisible,
		AllowUDP:               n.AllowUDP,
		SpeedLimitMbps:         n.SpeedLimitMbps,
		DeviceLimit:            n.DeviceLimit,
		PaddingScheme:          n.PaddingScheme,
		TrafficRate:            n.TrafficRate,
		Priority:               n.Priority,
		CapacityScore:          n.CapacityScore,
		TransferEnableBytes:    n.TransferEnableBytes,
		Tags:                   n.Tags,
		ConfigJSON:             n.ConfigJSON,
		Metadata:               n.Metadata,
		LastPublishedVersion:   n.LastPublishedVersion,
		ChainIDs:               []uuid.UUID{},
		PlanCodes:              []string{},
		CreatedAt:              n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		IsSplitMode:            n.IsSplitMode,
	}
	// 填充 exposure_mode：优先独立列，回退 config_json
	if n.ExposureMode != nil && *n.ExposureMode != "" {
		resp.ExposureMode = *n.ExposureMode
	} else if n.ConfigJSON != nil {
		if em, ok := n.ConfigJSON["exposure_mode"].(string); ok && em != "" {
			resp.ExposureMode = em
		}
	}
	// 填充 downstream_exposure_mode：优先独立列，回退 config_json
	if n.DownstreamExposureMode != nil && *n.DownstreamExposureMode != "" {
		resp.DownstreamExposureMode = *n.DownstreamExposureMode
	}
	// 从 config_json 提取 cert_bundle_id 到顶层（前端编辑回显用）
	if n.ConfigJSON != nil {
		if cbID, ok := n.ConfigJSON["cert_bundle_id"].(string); ok && cbID != "" {
			resp.CertBundleID = cbID
		}
	}
	// P0-4: 从 Metadata._warnings 提取警告到 Warnings 字段（API 响应友好）
	// P2-1: 从 Metadata 提取配置下发状态字段
	if n.Metadata != nil {
		if w, ok := n.Metadata["_warnings"]; ok {
			if ws, ok := w.([]string); ok {
				resp.Warnings = ws
			}
			delete(n.Metadata, "_warnings")
		}
		// P2-1: 提取 _dispatch_* 字段到顶层响应字段
		if s, ok := n.Metadata["_dispatch_status"].(string); ok {
			resp.DispatchStatus = s
		}
		if v, ok := n.Metadata["_dispatch_version"].(float64); ok {
			resp.DispatchVersion = int64(v)
		}
		if t, ok := n.Metadata["_dispatch_time"].(string); ok {
			resp.DispatchTime = t
		}
		if e, ok := n.Metadata["_dispatch_error"].(string); ok {
			resp.DispatchError = e
		}
	}
	return resp
}

func NewNodeResponseWithHealth(n *Node, health *NodeHealthStatus) NodeResponse {
	resp := NewNodeResponse(n)
	if health != nil {
		resp.HealthStatus = &health.OverallStatus
	}
	return resp
}

// NewNodeResponseWithFull 构造含 server_info / chain_ids / group_info 的完整响应
// groupIDs: 节点所属的所有分组 ID（多对多，用于编辑回显）
// groups: 节点所属的所有分组简要信息（多对多，用于编辑回显）
func NewNodeResponseWithFull(n *Node, health *NodeHealthStatus, serverInfo *ServerBrief, chainIDs []uuid.UUID, groupInfo *NodeGroupBrief, groupIDs []uuid.UUID, groups []NodeGroupBrief) NodeResponse {
	resp := NewNodeResponseWithHealth(n, health)
	resp.ServerInfo = serverInfo
	if len(chainIDs) > 0 {
		resp.ChainIDs = chainIDs
	}
	resp.GroupInfo = groupInfo
	// 多对多分组回显
	if groupIDs != nil {
		resp.GroupIDs = groupIDs
	} else {
		resp.GroupIDs = []uuid.UUID{}
	}
	if groups != nil {
		resp.Groups = groups
	} else {
		resp.Groups = []NodeGroupBrief{}
	}
	// 兼容旧字段 GroupIDs：若 GroupID 非 nil 但 GroupIDs 为空，回填
	if n.GroupID != nil && len(resp.GroupIDs) == 0 {
		resp.GroupIDs = []uuid.UUID{*n.GroupID}
	}
	return resp
}

type CreateChainRequest struct {
	Code           string                 `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name           string                 `json:"name" binding:"required,min=1,max=128"`
	ChainMode      ChainMode              `json:"chain_mode"`
	Strategy       ChainStrategy          `json:"chain_strategy"`
	MaxHops        int                    `json:"max_hops"`
	HealthPolicyID *uuid.UUID             `json:"health_policy_id"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type ChainListQuery struct {
	Page     int         `form:"page"`
	PageSize int         `form:"page_size"`
	Status   ChainStatus `form:"status"`
}

type AddHopRequest struct {
	HopIndex             int                    `json:"hop_index" binding:"required,min=0"`
	HopType              HopType                `json:"hop_type" binding:"required"`
	UpstreamNodeID       *uuid.UUID             `json:"upstream_node_id"`
	UpstreamRuntimeID    *uuid.UUID             `json:"upstream_runtime_id"`
	OutboundProtocolType *string                `json:"outbound_protocol_type"`
	OutboundConfigJSON   map[string]interface{} `json:"outbound_config_json"`
}

type BindNodeRequest struct {
	NodeID   uuid.UUID `json:"node_id" binding:"required"`
	BindMode BindMode  `json:"bind_mode"`
	Priority int       `json:"priority"`
}

type ProxyChainResponse struct {
	ID        uuid.UUID     `json:"id"`
	Code      string        `json:"code"`
	Name      string        `json:"name"`
	Status    ChainStatus   `json:"status"`
	ChainMode ChainMode     `json:"chain_mode"`
	Strategy  ChainStrategy `json:"strategy"`
	MaxHops   int           `json:"max_hops"`
	CreatedAt string        `json:"created_at"`
}

func NewProxyChainResponse(c *ProxyChain) ProxyChainResponse {
	return ProxyChainResponse{
		ID:        c.ID,
		Code:      c.Code,
		Name:      c.Name,
		Status:    c.Status,
		ChainMode: c.ChainMode,
		Strategy:  c.Strategy,
		MaxHops:   c.MaxHops,
		CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type AgentHeartbeatRequest struct {
	ServerCode    string                 `json:"server_code" binding:"required"`
	RuntimeRef    *string                `json:"runtime_ref"`
	Timestamp     time.Time              `json:"timestamp"`
	ConfigVersion string                 `json:"config_version_current,omitempty"`
	RTTMs         *int                   `json:"rtt_ms"`
	LossRatio     *float64               `json:"loss_ratio"`
	OnlineUsers   int                    `json:"online_users"`
	CPUPercent    *float64               `json:"cpu_percent"`
	MemPercent    *float64               `json:"mem_percent"`
	DiskPercent   *float64               `json:"disk_percent"`
	Metrics       map[string]interface{} `json:"metrics"`
	ErrorMessage  *string                `json:"error_message"`
	AgentVersion  string                 `json:"agent_version,omitempty"`
	RuntimeStatus string                 `json:"runtime_status,omitempty"`
	RuntimeVersion string                `json:"runtime_version,omitempty"`
	OS            string                 `json:"os,omitempty"`
	Arch          string                 `json:"arch,omitempty"`
	Pid           int                    `json:"pid,omitempty"`
	// 通道健康（node-agent 三通道降级状态）
	ChannelHealth *ChannelHealthReport `json:"channel_health,omitempty"`
}

// ChannelHealthReport 心跳中上报的通道健康数据（与 channelhealth.HeartbeatChannelHealth 对齐，
// 但放在 model 包以避免 dto 依赖 channelhealth 造成循环引用）
type ChannelHealthReport struct {
	ActiveChannel string           `json:"active_channel"` // grpc / ws / http
	ChannelState  string           `json:"channel_state"`  // healthy / degraded / unhealthy / unknown
	RTTMs         *int             `json:"rtt_ms,omitempty"`
	FailCount1h   int              `json:"fail_count_1h,omitempty"`
	OnlineUsers   int              `json:"online_users,omitempty"`
	LastError     *string          `json:"last_error,omitempty"`
	Failover      *ChannelFailover `json:"failover,omitempty"`
}

// ChannelFailover 心跳中携带的降级事件
type ChannelFailover struct {
	FromChannel string `json:"from_channel"`
	ToChannel   string `json:"to_channel"`
	Reason      string `json:"reason"` // heartbeat_timeout / connection_error / auto_recovery / initial_connect
}

type HeartbeatResponse struct {
	Status              string  `json:"status"`
	CurrentTime         int64   `json:"current_time"`
	TargetConfigVersion *string `json:"target_config_version,omitempty"`
	ConfigURL           *string `json:"config_url,omitempty"`
	ConfigSignature     *string `json:"config_signature,omitempty"`
	Action              *string `json:"action,omitempty"`
	// ExtraActions 附加动作列表（与 Action 并行执行）。
	// 用于节点配置变更时同时触发外部资源同步（nginx vhost/证书），
	// 消除 nginx reconciler 30s 轮询延迟，实现"保存即下发"。
	ExtraActions        []string `json:"extra_actions,omitempty"`
	NeedUpgrade         *bool   `json:"need_upgrade,omitempty"`
	UpgradeURL          *string `json:"upgrade_url,omitempty"`
	UpgradeVersion      *string `json:"upgrade_version,omitempty"`
	NeedReboot          *bool   `json:"need_reboot,omitempty"`
}

type AgentConfigResponse struct {
	Version   string                 `json:"version"`
	Config    map[string]interface{} `json:"config"`
	Signature string                 `json:"signature"`
	AppliedAt string                 `json:"applied_at"`
}

type AgentConfigResultRequest struct {
	Version         string `json:"version" binding:"required"`
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	RollbackVersion string `json:"rollback_version,omitempty"`
	DurationMs      int64  `json:"duration_ms"`
}

// DeploymentResultRequest P3-1: Agent 上报部署 ACK/NACK 的请求体。
// Agent 在 precheck / activate / healthcheck 各阶段完成后通过
// POST /api/v1/agent/deployment-result 上报结果。
type DeploymentResultRequest struct {
	Version            string     `json:"version" binding:"required"`
	Success            bool       `json:"success"`
	Message            string     `json:"message"`
	Phase              string     `json:"phase"` // precheck / activate / healthcheck
	DurationMs         int64      `json:"duration_ms"`
	DeploymentTargetID *uuid.UUID `json:"deployment_target_id,omitempty"`
}

type AgentRegisterRequest struct {
	ServerCode          string                 `json:"server_code" binding:"required"`
	RuntimeType         string                 `json:"runtime_type" binding:"required"`
	RuntimeVersion      *string                `json:"runtime_version"`
	ListenHost          *string                `json:"listen_host"`
	APIPort             *int                   `json:"api_port"`
	XrayAPIPort         *int                   `json:"xray_api_port"`
	SingboxClashPort    *int                   `json:"singbox_clash_port"`
	Capabilities        map[string]interface{} `json:"capabilities"`
	ConfigSchemaVersion string                 `json:"config_schema_version"`
	Metadata            map[string]interface{} `json:"metadata"`
	Hostname            string                 `json:"hostname"`
	OS                  string                 `json:"os"`
	Arch                string                 `json:"arch"`
	AgentVersion        string                 `json:"agent_version"`
}

type AgentResponse struct {
	NodeID   uuid.UUID `json:"node_id"`
	ServerID uuid.UUID `json:"server_id"`
}

type NodeHealthResponse struct {
	NodeID             uuid.UUID  `json:"node_id"`
	OverallStatus      string     `json:"overall_status"`
	HeartbeatStatus    string     `json:"heartbeat_status"`
	ProbeStatus        string     `json:"probe_status"`
	AvailabilityScore  int        `json:"availability_score"`
	LatencyScore       int        `json:"latency_score"`
	LossScore          int        `json:"loss_score"`
	CurrentRTTMs       *int       `json:"current_rtt_ms,omitempty"`
	CurrentLossRatio   *float64   `json:"current_loss_ratio,omitempty"`
	CurrentOnlineUsers int        `json:"current_online_users"`
	LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
	UpdatedAt          string     `json:"updated_at"`
}

func NewNodeHealthResponse(h *NodeHealthStatus) NodeHealthResponse {
	return NodeHealthResponse{
		NodeID:             h.NodeID,
		OverallStatus:      h.OverallStatus,
		HeartbeatStatus:    h.HeartbeatStatus,
		ProbeStatus:        h.ProbeStatus,
		AvailabilityScore:  h.AvailabilityScore,
		LatencyScore:       h.LatencyScore,
		LossScore:          h.LossScore,
		CurrentRTTMs:       h.CurrentRTTMs,
		CurrentLossRatio:   h.CurrentLossRatio,
		CurrentOnlineUsers: h.CurrentOnlineUsers,
		LastHeartbeatAt:    h.LastHeartbeatAt,
		UpdatedAt:          h.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type HealthEventListQuery struct {
	Page      int        `form:"page"`
	PageSize  int        `form:"page_size"`
	NodeID    string     `form:"node_id"`
	EventType string     `form:"event_type"`
	Severity  string     `form:"severity"`
	StartTime *time.Time `form:"start_time"`
	EndTime   *time.Time `form:"end_time"`
}

type NodeHealthEventResponse struct {
	ID         uuid.UUID              `json:"id"`
	NodeID     uuid.UUID              `json:"node_id"`
	EventType  string                 `json:"event_type"`
	Severity   HealthSeverity         `json:"severity"`
	FromStatus *string                `json:"from_status,omitempty"`
	ToStatus   *string                `json:"to_status,omitempty"`
	Metrics    map[string]interface{} `json:"metrics"`
	Message    *string                `json:"message,omitempty"`
	OccurredAt string                 `json:"occurred_at"`
}

func NewNodeHealthEventResponse(e *NodeHealthEvent) NodeHealthEventResponse {
	return NodeHealthEventResponse{
		ID:         e.ID,
		NodeID:     e.NodeID,
		EventType:  e.EventType,
		Severity:   e.Severity,
		FromStatus: e.FromStatus,
		ToStatus:   e.ToStatus,
		Metrics:    e.Metrics,
		Message:    e.Message,
		OccurredAt: e.OccurredAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type DryRunRequest struct {
	ScopeType   ScopeType              `json:"scope_type" binding:"required"`
	ScopeID     uuid.UUID              `json:"scope_id" binding:"required"`
	ContentJSON map[string]interface{} `json:"content_json" binding:"required"`
}

type DryRunResponse struct {
	ScopeType    ScopeType `json:"scope_type"`
	ScopeID      uuid.UUID `json:"scope_id"`
	NewVersionNo int64     `json:"new_version_no"`
	ContentHash  string    `json:"content_hash"`
	DiffSummary  string    `json:"diff_summary"`
	Valid        bool      `json:"valid"`
	Message      string    `json:"message,omitempty"`
}

type DeployRequest struct {
	ScopeType   ScopeType              `json:"scope_type" binding:"required"`
	ScopeID     uuid.UUID              `json:"scope_id" binding:"required"`
	Strategy    DeploymentStrategy     `json:"strategy"`
	ContentJSON map[string]interface{} `json:"content_json" binding:"required"`
}

type DeployResponse struct {
	BatchID     uuid.UUID        `json:"batch_id"`
	Status      DeploymentStatus `json:"status"`
	TargetCount int              `json:"target_count"`
	CreatedAt   string           `json:"created_at"`
}

type DeploymentBatchListQuery struct {
	Page      int              `form:"page"`
	PageSize  int              `form:"page_size"`
	Status    DeploymentStatus `form:"status"`
	ScopeType ScopeType        `form:"scope_type"`
}

type UpdateDeploymentResultRequest struct {
	TargetID       uuid.UUID              `json:"target_id" binding:"required"`
	Status         TargetStatus           `json:"status" binding:"required"`
	PrecheckResult map[string]interface{} `json:"precheck_result"`
	ApplyResult    map[string]interface{} `json:"apply_result"`
	ErrorMessage   *string                `json:"error_message"`
}

// ===== NodeGroup（会员分组）DTO =====

type CreateNodeGroupRequest struct {
	Code        string  `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name        string  `json:"name" binding:"required,min=1,max=128"`
	Description *string `json:"description"`
	Visibility  string  `json:"visibility"`
	SortOrder   *int    `json:"sort_order"`
}

type UpdateNodeGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility"`
	SortOrder   *int    `json:"sort_order"`
}

type NodeGroupListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Search   string `form:"search"`
}

type NodeGroupResponse struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Visibility  string    `json:"visibility"`
	SortOrder   int       `json:"sort_order"`
	NodeCount   int       `json:"node_count"`
	UserCount   int       `json:"user_count"`
	CreatedAt   string    `json:"created_at"`
}

func NewNodeGroupResponse(g *NodeGroup, nodeCount, userCount int) NodeGroupResponse {
	return NodeGroupResponse{
		ID:          g.ID,
		Code:        g.Code,
		Name:        g.Name,
		Description: g.Description,
		Visibility:  g.Visibility,
		SortOrder:   g.SortOrder,
		NodeCount:   nodeCount,
		UserCount:   userCount,
		CreatedAt:   g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// BatchBindNodesRequest 批量将节点添加到分组
type BatchBindNodesRequest struct {
	NodeIDs []uuid.UUID `json:"node_ids" binding:"required,min=1"`
}

// BatchUnbindNodesRequest 批量从分组移除节点
type BatchUnbindNodesRequest struct {
	NodeIDs []uuid.UUID `json:"node_ids" binding:"required,min=1"`
}

type PrecheckItem struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Scope   string `json:"scope,omitempty"`
	ScopeID string `json:"scope_id,omitempty"`
}

type PrecheckResult struct {
	Passed     bool           `json:"passed"`
	Errors     []PrecheckItem `json:"errors"`
	Warnings   []PrecheckItem `json:"warnings"`
	Infos      []PrecheckItem `json:"infos"`
	ServerCode string         `json:"server_code,omitempty"`
}
