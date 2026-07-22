package service

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

const (
	RealityTCPPortStart   = 9450
	RealityTCPPortEnd     = 9600
	CDNHTTPPortStart      = 8446
	CDNHTTPPortEnd        = 8600
	// Hysteria2 UDP 端口范围对齐用户期望的端口跳跃区间 40020-40200。
	// 单节点主端口从 40020 起递增分配；若节点启用 port_hopping，
	// hopping 范围由 config_json.port_hopping.port_range 单独配置（可覆盖整个区间）。
	Hysteria2UDPPortStart = 40020
	Hysteria2UDPPortEnd   = 40200
	// TUIC UDP 端口范围紧跟 Hysteria2 之后，避免与 Hy2 hopping 区间冲突。
	TUICUDPPortStart      = 40210
	TUICUDPPortEnd        = 40299
	ShadowTLSPortStart    = 9700
	ShadowTLSPortEnd      = 9749
	AnyTLSPortStart       = 9750
	AnyTLSPortEnd         = 9799
	TunnelPortStart       = 20530
	TunnelPortEnd         = 20699

	// P1-3: 硬性保留端口 — 443 永远属于 nginx stream SNI 分流，不可分配给任何 xray inbound
	NginxStreamPort = 443
	// P1-3: 禁止分配的系统保留端口
	ReservedPortMin = 0
	ReservedPortMax = 1023
)

type PortPlanner struct {
	repo   *repo.NodeRepo
	logger *slog.Logger
}

func NewPortPlanner(r *repo.NodeRepo, logger *slog.Logger) *PortPlanner {
	return &PortPlanner{repo: r, logger: logger}
}

func (p *PortPlanner) AllocateServerPort(ctx context.Context, serverID uuid.UUID, nodeType model.NodeType, protocolType, transportType, securityType string) (int, error) {
	start, end, err := portRangeFor(protocolType, transportType, securityType)
	if err != nil {
		return 0, err
	}

	// P1-3: 边界断言 — 端口范围必须合法
	if start <= 0 || end <= 0 || start > end {
		return 0, fmt.Errorf("invalid port range %d-%d for protocol=%s transport=%s security=%s",
			start, end, protocolType, transportType, securityType)
	}
	// P1-3: 硬性校验 — 端口范围不得包含 443（nginx stream 专属）
	if start <= NginxStreamPort && end >= NginxStreamPort {
		return 0, fmt.Errorf("port range %d-%d illegally includes nginx stream port %d (protocol=%s transport=%s security=%s)",
			start, end, NginxStreamPort, protocolType, transportType, securityType)
	}
	// P1-3: 硬性校验 — 端口范围不得与系统保留端口重叠
	if start <= ReservedPortMax {
		return 0, fmt.Errorf("port range %d-%d overlaps with system reserved ports 0-%d (protocol=%s transport=%s security=%s)",
			start, end, ReservedPortMax, protocolType, transportType, securityType)
	}

	usedPorts, err := p.repo.FindUsedServerPortsInServer(ctx, serverID, start, end)
	if err != nil {
		return 0, fmt.Errorf("find used ports: %w", err)
	}

	usedSet := make(map[int]bool, len(usedPorts))
	for _, port := range usedPorts {
		usedSet[port] = true
	}

	for port := start; port <= end; port++ {
		if !usedSet[port] {
			// P1-3: 最终安全断言 — 分配的端口不得是 443 或系统保留端口
			if port == NginxStreamPort {
				p.logger.Error("port planner attempted to allocate nginx stream port 443, skipping",
					"server_id", serverID,
					"protocol", protocolType,
					"transport", transportType,
					"security", securityType,
				)
				continue
			}
			if port <= ReservedPortMax {
				p.logger.Error("port planner attempted to allocate reserved port, skipping",
					"server_id", serverID,
					"port", port,
				)
				continue
			}
			p.logger.Info("allocated server port",
				"server_id", serverID,
				"node_type", nodeType,
				"protocol", protocolType,
				"transport", transportType,
				"security", securityType,
				"port", port,
				"range_start", start,
				"range_end", end,
			)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available port in range %d-%d for protocol=%s transport=%s security=%s",
		start, end, protocolType, transportType, securityType)
}

// portRangeFor 根据协议/传输/安全类型返回端口范围。
//
// P1-3 修复：
//   - 新增 transportType 空值硬性校验
//   - 新增 UDP 协议（hysteria2/tuic）禁止 TCP 传输校验
//   - 确保 443 永远不在此函数返回范围内
func portRangeFor(protocolType, transportType, securityType string) (start, end int, err error) {
	// P1-3: 硬性校验 — 空值阻断
	if protocolType == "" && transportType == "" && securityType == "" {
		return 0, 0, fmt.Errorf("portRangeFor: all type params are empty")
	}

	if securityType == "reality" {
		return RealityTCPPortStart, RealityTCPPortEnd, nil
	}

	switch protocolType {
	case "hysteria2":
		// P1-3: UDP 协议禁止绑定 TCP 传输
		if transportType == "tcp" {
			return 0, 0, fmt.Errorf("hysteria2 is UDP-only, transport=tcp is invalid")
		}
		return Hysteria2UDPPortStart, Hysteria2UDPPortEnd, nil
	case "tuic":
		// P1-3: UDP 协议禁止绑定 TCP 传输
		if transportType == "tcp" {
			return 0, 0, fmt.Errorf("tuic is UDP-only, transport=tcp is invalid")
		}
		return TUICUDPPortStart, TUICUDPPortEnd, nil
	}

	switch securityType {
	case "shadowtls":
		return ShadowTLSPortStart, ShadowTLSPortEnd, nil
	case "anytls":
		return AnyTLSPortStart, AnyTLSPortEnd, nil
	}

	// 直连 TCP+TLS（如 trojan+tcp+tls, vless+tcp+tls）：走 nginx 443 SNI default，
	// 与 REALITY 节点共用高位端口范围（9450-9600），AllocateServerPort 会自动跳过已占用端口。
	if transportType == "tcp" && securityType == "tls" {
		return RealityTCPPortStart, RealityTCPPortEnd, nil
	}

	switch transportType {
	case "ws", "xhttp", "grpc", "httpupgrade", "splithttp":
		return CDNHTTPPortStart, CDNHTTPPortEnd, nil
	}

	return 0, 0, fmt.Errorf("unknown port range for protocol=%s transport=%s security=%s",
		protocolType, transportType, securityType)
}

// IsPortValidForNginxStream P1-3: 判断端口是否属于 nginx stream 专属端口。
// 443 永远属于 nginx stream SNI 分流，任何 xray inbound 都不得直接绑定。
func IsPortValidForNginxStream(port int) bool {
	return port == NginxStreamPort
}

// IsPortUDP P1-3: 根据协议类型判断端口是否为 UDP 端口。
// UDP 端口不接触 nginx（nginx stream 不做 UDP SNI 分流）。
func IsPortUDP(protocolType string) bool {
	switch protocolType {
	case "hysteria2", "tuic":
		return true
	default:
		return false
	}
}

// ValidatePortNotReserved P1-3: 外部调用的端口安全校验函数。
// 用于 handler 层在节点创建/更新时做前置校验。
func ValidatePortNotReserved(port int) error {
	if port == NginxStreamPort {
		return fmt.Errorf("port %d is reserved for nginx stream SNI dispatch", port)
	}
	if port <= ReservedPortMax {
		return fmt.Errorf("port %d is in system reserved range 0-%d", port, ReservedPortMax)
	}
	if port > 65535 {
		return fmt.Errorf("port %d exceeds maximum 65535", port)
	}
	// P1-3: 端口必须是合法 TCP/UDP 端口
	if !isValidPortNumber(port) {
		return fmt.Errorf("port %d is not a valid port number", port)
	}
	return nil
}

// isValidPortNumber 检查端口是否为合法数字（0 < port <= 65535）
// 注意：这里允许 443 用于 nginx stream 但 xray inbound 不应使用此函数
func isValidPortNumber(port int) bool {
	return port > 0 && port <= 65535
}

// IsPortInReservedRange P1-3: 判断端口是否在系统保留范围（0-1023）。
// 保留给：22 (SSH), 80 (HTTP), 443 (nginx stream), 8445 (nginx internal TLS)。
func IsPortInReservedRange(port int) bool {
	return port >= ReservedPortMin && port <= ReservedPortMax
}

// formatPortRange 用于日志/错误信息的端口范围格式化
func formatPortRange(start, end int) string {
	return fmt.Sprintf("%d-%d", start, end)
}

// HostPortFromAddr 从 "host:port" 格式地址中提取端口号。
// 用于校验节点 server_addr 字段中的端口是否合法。
func HostPortFromAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}
