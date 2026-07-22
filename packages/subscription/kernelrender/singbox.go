package kernelrender

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/airport-panel/subscription/nodespec"
)

// SingBoxRenderer 生成 sing-box 服务端配置（inbounds + outbounds）
type SingBoxRenderer struct{}

// NewSingBoxRenderer 构造 Sing-box 渲染器
func NewSingBoxRenderer() *SingBoxRenderer { return &SingBoxRenderer{} }

// KernelType 返回内核类型
func (r *SingBoxRenderer) KernelType() KernelType { return KernelSingBox }

// Render 渲染 sing-box 服务端配置
// 适配实际 NodeSpec 结构：Credentials/Security(string)/TLS/Reality/Transport
func (r *SingBoxRenderer) Render(spec *nodespec.NodeSpec) (map[string]interface{}, error) {
	if spec == nil {
		return nil, fmt.Errorf("nodespec is nil")
	}

	// 1. 内核支持矩阵检查
	if err := r.checkKernelSupport(spec); err != nil {
		return nil, err
	}

	// 2. 渲染 inbound
	// P8 端口语义显式分离：
	//   - listen_port: resolveInboundPort() — CDN/Tunnel 用 ServerPort，DIRECT 用 Port
	//   - listen: resolveListenAddress() — CDN/Tunnel 绑 127.0.0.1，DIRECT 绑 0.0.0.0
	inbound := map[string]interface{}{
		"listen_port": resolveInboundPort(spec),
		"listen":      resolveListenAddress(spec),
		"tag":         fmt.Sprintf("in-%s", spec.Code),
	}

	// 协议层（type + users）
	inbound["type"] = r.protocolType(spec)
	if users := r.renderUsers(spec); users != nil {
		inbound["users"] = users
	}

	// 传输层
	transport, err := r.renderTransport(spec)
	if err != nil {
		return nil, err
	}
	if transport != nil {
		inbound["transport"] = transport
	}

	// 安全层（TLS/REALITY）
	if tls := r.renderTLS(spec); tls != nil {
		inbound["tls"] = tls
	}

	// 协议专属字段
	r.addProtocolFields(inbound, spec)

	// 多路复用（Mux）
	if mux := r.renderMultiplex(spec); mux != nil {
		inbound["multiplex"] = mux
	}

	// RawSettings 逃生舱：仅合并 sing_box: 前缀的条目
	if spec.RawConfig != nil {
		if raw, ok := spec.RawConfig["sing_box"].(map[string]interface{}); ok {
			mergeMap(inbound, raw)
		}
	}

	// ECH 场景强制 TLS 1.3（关键坑点固化：ECH 要求 TLS1.3，否则被静默丢弃）
	if isECHEnabled(spec) {
		if tls, ok := inbound["tls"].(map[string]interface{}); ok {
			tls["min_version"] = "1.3" // 强制写入，防止静默失效
		}
	}

	result := map[string]interface{}{
		"inbounds":  []interface{}{inbound},
		"outbounds": r.renderOutbounds(spec),
	}

	// S6/S7/S8: 安全路由规则 — 阻断私有 IP（SSRF 防护）和 BT 协议
	result["route"] = renderSingBoxSecurityRouting()

	// stats 配置：sing-box 不支持像 Xray 那样的顶层 stats 字段，
	// 但可通过 experimental.clash_api 开启流量统计与观测。
	// 此处添加 experimental 段以启用 clash_api（供 Agent 拉取流量统计）。
	result["experimental"] = r.renderExperimental(spec)

	// _limiter 元数据：供 node-agent 解析后初始化 SpeedLimiter/DeviceLimiter。
	// sing-box 不直接支持 per-user 限速配置，限速完全在 Agent 端通过 limiter 包实现。
	// 此元数据作为 "limiter hook"，以 "_" 开头在写入内核配置前由 Agent 剥离
	// （与 _nginx_vhosts 同机制），不污染 sing-box 配置。
	if meta := renderLimiterMeta(spec); meta != nil {
		result["_limiter"] = meta
	}

	return result, nil
}

// renderExperimental 渲染 sing-box experimental 段（clash_api 用于流量统计/观测）。
//
// sing-box 没有类似 Xray 的顶层 stats 字段，流量统计通过 clash_api 实现。
// clash_api 监听 localhost:9090，Agent 可通过该 API 拉取连接数/流量统计，
// 供 SpeedLimiter/DeviceLimiter 做限速/设备数判定。
func (r *SingBoxRenderer) renderExperimental(spec *nodespec.NodeSpec) map[string]interface{} {
	return map[string]interface{}{
		"clash_api": map[string]interface{}{
			"external_controller": "127.0.0.1:9090",
		},
	}
}

// checkKernelSupport 检查 Sing-box 是否支持该协议组合
func (r *SingBoxRenderer) checkKernelSupport(spec *nodespec.NodeSpec) error {
	// Mieru: Sing-box 不支持
	if spec.Protocol == nodespec.ProtocolMieru {
		return &UnsupportedFeatureError{Feature: "mieru", Kernel: KernelSingBox}
	}
	// XHTTP 传输类型：Sing-box 不支持（用 httpupgrade 替代）
	// 注意：sing-box 用 "ws" 不是 "websocket"，用 "httpupgrade" 不是 "xhttp"
	// 但 Sing-box 1.13+ 通过 httpupgrade 兼容模式可支持 xhttp，这里按设计文档标记不支持
	// 实际由 PresetTemplate.KernelCompat 控制是否使用 Sing-box 渲染
	// 这里不做硬性拦截，由上层 validator 决定
	return nil
}

// addProtocolFields 添加协议专属字段（Hysteria2 obfs/masquerade、TUIC congestion_control 等）
func (r *SingBoxRenderer) addProtocolFields(inbound map[string]interface{}, spec *nodespec.NodeSpec) {
	switch spec.Protocol {
	case nodespec.ProtocolHysteria2:
		// obfs
		if spec.Transport.QUIC != nil && spec.Transport.QUIC.Security != "" {
			inbound["obfs"] = map[string]interface{}{
				"type":     spec.Transport.QUIC.Security,
				"password": spec.Transport.QUIC.Key,
			}
		}
		// masquerade (SNI 伪装)
		// sing-box v1.11+ 要求 masquerade 为 URL 格式（file:// http:// https://）
		// 使用 https:// + SNI 作为反向代理目标，将非 Hysteria2 流量代理到 SNI 站点
		if spec.TLS != nil && spec.TLS.SNI != "" {
			inbound["masquerade"] = "https://" + spec.TLS.SNI
		}
		// 带宽（inbound 侧用于 BBR 提示）
		if c, ok := spec.Credentials.(nodespec.Hysteria2Credentials); ok {
			if c.UpMbps > 0 {
				inbound["up_mbps"] = c.UpMbps
			}
			if c.DownMbps > 0 {
				inbound["down_mbps"] = c.DownMbps
			}
		}
		// 端口跳跃（hop_ports）：sing-box hysteria2 inbound 原生支持
		// 当配置 port_hopping.enabled=true 且 port_range 非空时写入 hop_ports 字段
		// sing-box 会监听 port_range 范围内所有 UDP 端口，实现端口跳跃抗封锁
		if spec.Transport.PortHopping != nil && spec.Transport.PortHopping.Enabled && spec.Transport.PortHopping.PortRange != "" {
			inbound["hop_ports"] = spec.Transport.PortHopping.PortRange
		}
	case nodespec.ProtocolTUIC:
		inbound["congestion_control"] = "bbr"
		// 端口跳跃：TUIC v5 也支持 hop_ports（sing-box 1.11+）
		if spec.Transport.PortHopping != nil && spec.Transport.PortHopping.Enabled && spec.Transport.PortHopping.PortRange != "" {
			inbound["hop_ports"] = spec.Transport.PortHopping.PortRange
		}
	case nodespec.ProtocolAnyTLS:
		// padding_scheme: AnyTLS 填充方案（如 "max-0" 无填充）。
		// 为空时不写入，由 sing-box 使用内核默认值。
		if spec.PaddingScheme != "" {
			inbound["padding_scheme"] = spec.PaddingScheme
		}
	}
}

// protocolType 返回 sing-box 的协议类型字符串
// sing-box 字段用 type，Xray 用 protocol
func (r *SingBoxRenderer) protocolType(spec *nodespec.NodeSpec) string {
	switch spec.Protocol {
	case nodespec.ProtocolVLESS:
		return "vless"
	case nodespec.ProtocolVMess:
		return "vmess"
	case nodespec.ProtocolTrojan:
		return "trojan"
	case nodespec.ProtocolShadowsocks:
		return "shadowsocks"
	case nodespec.ProtocolHysteria2:
		return "hysteria2"
	case nodespec.ProtocolTUIC:
		return "tuic"
	case nodespec.ProtocolAnyTLS:
		return "anytls"
	case nodespec.ProtocolSOCKS5:
		return "socks"
	case nodespec.ProtocolHTTP:
		return "http"
	default:
		return string(spec.Protocol)
	}
}

// renderUsers 渲染用户列表
// sing-box 用 users（数组），Xray 用 settings.clients
func (r *SingBoxRenderer) renderUsers(spec *nodespec.NodeSpec) []map[string]interface{} {
	// 多用户路径（P0-4）
	if hasMultiClients(spec) {
		return r.renderUsersMultiClient(spec)
	}
	switch spec.Protocol {
	case nodespec.ProtocolVLESS:
		uuid := extractUUID(spec)
		user := map[string]interface{}{"uuid": uuid}
		// REALITY + Vision flow（仅 TCP 传输层支持，XHTTP/WS 等不支持）
		if flow := extractFlow(spec); flow != "" {
			user["flow"] = flow
		} else if spec.Security == nodespec.SecurityReality && spec.Transport.Type == nodespec.TransportTCP {
			user["flow"] = string(nodespec.FlowXTLSRprxVision)
		}
		return []map[string]interface{}{user}
	case nodespec.ProtocolVMess:
		uuid := extractUUID(spec)
		return []map[string]interface{}{
			{"uuid": uuid, "alterId": 0},
		}
	case nodespec.ProtocolTrojan:
		password := extractPassword(spec)
		return []map[string]interface{}{
			{"password": password},
		}
	case nodespec.ProtocolShadowsocks:
		if c, ok := spec.Credentials.(nodespec.ShadowsocksCredentials); ok {
			return []map[string]interface{}{
				{"password": c.Password, "method": c.Method},
			}
		}
		return nil
	case nodespec.ProtocolHysteria2:
		password := extractPassword(spec)
		user := map[string]interface{}{"password": password}
		if c, ok := spec.Credentials.(nodespec.Hysteria2Credentials); ok {
			if c.UpMbps > 0 {
				user["up_mbps"] = c.UpMbps
			}
			if c.DownMbps > 0 {
				user["down_mbps"] = c.DownMbps
			}
		}
		return []map[string]interface{}{user}
	case nodespec.ProtocolTUIC:
		if c, ok := spec.Credentials.(nodespec.TUICCredentials); ok {
			user := map[string]interface{}{"uuid": c.UUID}
			if c.Password != "" {
				user["password"] = c.Password
			}
			return []map[string]interface{}{user}
		}
		return nil
	case nodespec.ProtocolAnyTLS:
		password := extractPassword(spec)
		return []map[string]interface{}{
			{"password": password},
		}
	case nodespec.ProtocolSOCKS5:
		if c, ok := spec.Credentials.(nodespec.SOCKS5Credentials); ok {
			return []map[string]interface{}{
				{"username": c.Username, "password": c.Password},
			}
		}
		return nil
	case nodespec.ProtocolHTTP:
		if c, ok := spec.Credentials.(nodespec.HTTPCredentials); ok {
			return []map[string]interface{}{
				{"username": c.Username, "password": c.Password},
			}
		}
		return nil
	default:
		return nil
	}
}

// renderUsersMultiClient 渲染多用户 users（P0-4）
// 根据 spec.Clients []CredentialSpec 输出 sing-box users 数组
//
// 重要：每个 user 必须设置 "name" 字段（值为用户 email 或 UUID）。
// sing-box 路由器通过 inbound users 的 name 字段填充 metadata.User，
// ConnTracker 依赖 metadata.User 区分用户流量。
// 如果 name 为空，ConnTracker 会跳过该连接的流量统计（conntracker.go:214-216）。
func (r *SingBoxRenderer) renderUsersMultiClient(spec *nodespec.NodeSpec) []map[string]interface{} {
	users := make([]map[string]interface{}, 0, len(spec.Clients))
	// userIdentifier 返回用户标识：优先 email，回退 UUID，再回退 password
	// 此标识同时用作 sing-box 的 name 字段和 ConnTracker 的 userID
	userIdentifier := func(c nodespec.CredentialSpec) string {
		if c.Email != "" {
			return c.Email
		}
		if c.UUID != "" {
			return c.UUID
		}
		return c.Password
	}
	switch spec.Protocol {
	case nodespec.ProtocolVLESS:
		for _, c := range spec.Clients {
			user := map[string]interface{}{
				"uuid": c.UUID,
				"name": userIdentifier(c),
			}
			flow := clientFlowFor(c, spec.Transport.Type)
			if spec.Security == nodespec.SecurityReality && spec.Transport.Type == nodespec.TransportTCP && flow == "" {
				flow = string(nodespec.FlowXTLSRprxVision)
			}
			if flow != "" {
				user["flow"] = flow
			}
			users = append(users, user)
		}
	case nodespec.ProtocolVMess:
		for _, c := range spec.Clients {
			user := map[string]interface{}{
				"uuid":    c.UUID,
				"alterId": c.AlterID,
				"name":    userIdentifier(c),
			}
			users = append(users, user)
		}
	case nodespec.ProtocolTrojan:
		for _, c := range spec.Clients {
			users = append(users, map[string]interface{}{
				"password": c.Password,
				"name":     userIdentifier(c),
			})
		}
	case nodespec.ProtocolShadowsocks:
		// sing-box SS：多用户时每个 user 带 password + method
		for _, c := range spec.Clients {
			users = append(users, map[string]interface{}{
				"password": c.Password,
				"method":   c.Method,
				"name":     userIdentifier(c),
			})
		}
	case nodespec.ProtocolHysteria2:
		// Hysteria2 多用户：每个用户有独立 password，name 用于流量统计
		for _, c := range spec.Clients {
			user := map[string]interface{}{
				"password": c.Password,
				"name":     userIdentifier(c),
			}
			if c.UpMbps > 0 {
				user["up_mbps"] = c.UpMbps
			}
			if c.DownMbps > 0 {
				user["down_mbps"] = c.DownMbps
			}
			users = append(users, user)
		}
	case nodespec.ProtocolTUIC:
		for _, c := range spec.Clients {
			user := map[string]interface{}{
				"uuid": c.UUID,
				"name": userIdentifier(c),
			}
			if c.Password != "" {
				user["password"] = c.Password
			}
			users = append(users, user)
		}
	case nodespec.ProtocolAnyTLS:
		for _, c := range spec.Clients {
			users = append(users, map[string]interface{}{
				"password": c.Password,
				"name":     userIdentifier(c),
			})
		}
	case nodespec.ProtocolSOCKS5:
		for _, c := range spec.Clients {
			user := map[string]interface{}{"name": userIdentifier(c)}
			if c.UUID != "" {
				user["username"] = c.UUID
			}
			if c.Password != "" {
				user["password"] = c.Password
			}
			users = append(users, user)
		}
	case nodespec.ProtocolHTTP:
		for _, c := range spec.Clients {
			user := map[string]interface{}{"name": userIdentifier(c)}
			if c.UUID != "" {
				user["username"] = c.UUID
			}
			if c.Password != "" {
				user["password"] = c.Password
			}
			users = append(users, user)
		}
	}
	return users
}

// renderTLS 渲染 TLS 配置
// sing-box 字段用 snake_case（server_name/allow_insecure/short_id），
// Xray 用 camelCase（serverName/allowInsecure/shortIds）
func (r *SingBoxRenderer) renderTLS(spec *nodespec.NodeSpec) map[string]interface{} {
	if spec.Security == nodespec.SecurityNone || spec.Security == "" {
		return nil
	}

	tls := map[string]interface{}{"enabled": true}

	switch spec.Security {
	case nodespec.SecurityTLS:
		if spec.TLS == nil {
			return tls // 结构不完整时不 panic，交由校验层拦截
		}
		if spec.TLS.SNI != "" {
			tls["server_name"] = spec.TLS.SNI
		}
		if len(spec.TLS.ALPN) > 0 {
			tls["alpn"] = spec.TLS.ALPN
		}
		if spec.TLS.AllowInsecure {
			tls["insecure"] = true
		}
		// uTLS 指纹 — sing-box 入站 TLS 不支持 utls 字段（仅出站支持）
		// uTLS 是客户端伪装技术，服务端无需配置
		// ECH 配置
		if spec.TLS.ECH != nil && spec.TLS.ECH.Enabled {
			tls["ech"] = r.renderECH(spec)
		}
		// 证书配置（P0-2: PEM-only，与 xray renderer 对齐）
		// 优先使用 inline PEM（避免文件路径依赖，确保配置自包含）
		// sing-box InboundTLSOptions 支持 certificate/key ([]string) 或 certificate_path/key_path (string)
		if spec.TLS.CertPEM != "" && spec.TLS.KeyPEM != "" {
			tls["certificate"] = []string{spec.TLS.CertPEM}
			tls["key"] = []string{spec.TLS.KeyPEM}
		} else if spec.TLS.CertFile != "" && spec.TLS.KeyFile != "" {
			// 文件路径模式（回退，用于手动配置 cert_file/key_file 的场景）
			tls["certificate_path"] = spec.TLS.CertFile
			tls["key_path"] = spec.TLS.KeyFile
		}

	case nodespec.SecurityReality:
		if spec.Reality == nil {
			return tls
		}
		if spec.Reality.SNI != "" {
			tls["server_name"] = spec.Reality.SNI
		}
		// handshake.server/server_port：推荐由用户显式配置 dest（host:port）
	// 支持两种填法：
	//   1. 本地反代：127.0.0.1:9454（推荐，回落到本地 nginx vhost 反代真实站点）
	//   2. 伪装域名：oyc.yale.edu:443（直连模式，回落到真实外部站点）
	// 注意：sing-box 渲染器无 error 返回通道，dest 为空时仍用 SNI:443 兜底（向后兼容）
	// 上层 deployment_service.go 的 preflight 校验应在节点保存阶段强制要求 dest 非空
	handshakeServer := spec.Reality.SNI
	handshakePort := 443
	if spec.Reality.Dest != "" {
		if h, p, err := net.SplitHostPort(spec.Reality.Dest); err == nil {
			if port, err := strconv.Atoi(p); err == nil && port > 0 {
				handshakeServer = h
				handshakePort = port
			}
		}
	}
		// Reality 在 Sing-box 下是 tls.reality.enabled=true，不是独立 security 类型
		reality := map[string]interface{}{
			"enabled": true,
			"handshake": map[string]interface{}{
				"server":      handshakeServer,
				"server_port": handshakePort,
			},
			"private_key": spec.Reality.PrivateKey,
		}
		// short_id（sing-box 用数组，支持多个）
		if len(spec.Reality.ShortIDs) > 0 {
			reality["short_id"] = spec.Reality.ShortIDs
		} else if spec.Reality.ShortID != "" {
			reality["short_id"] = []string{spec.Reality.ShortID}
		}
		tls["reality"] = reality
		// uTLS 指纹 — sing-box 入站 TLS 不支持 utls 字段
		// REALITY 的 uTLS 指纹由客户端配置，服务端无需设置
	}

	return tls
}

// renderECH 渲染 ECH 配置（仅 Sing-box 支持）
func (r *SingBoxRenderer) renderECH(spec *nodespec.NodeSpec) map[string]interface{} {
	if spec.TLS == nil || spec.TLS.ECH == nil {
		return nil
	}
	ech := map[string]interface{}{"enabled": true}
	e := spec.TLS.ECH
	if e.PEM != "" {
		ech["key"] = e.PEM
	}
	if e.Key != "" {
		ech["key_path"] = e.Key
	}
	if e.QueryDomain != "" {
		ech["query_domain"] = e.QueryDomain
	}
	return ech
}

// renderTransport 渲染传输层配置
// sing-box 用 type 字段，Xray 用 network 字段
// 返回 (nil, nil) 表示该协议不需要 transport 字段；返回 (nil, err) 表示硬性不支持。
func (r *SingBoxRenderer) renderTransport(spec *nodespec.NodeSpec) (map[string]interface{}, error) {
	switch spec.Transport.Type {
	case nodespec.TransportTCP:
		// TCP 不需要 transport 字段
		return nil, nil
	case nodespec.TransportWS:
		// sing-box 用 "ws"（不是 "websocket"）
		tr := map[string]interface{}{"type": "ws"}
		if spec.Transport.WS != nil {
			if spec.Transport.WS.Path != "" {
				tr["path"] = spec.Transport.WS.Path
			}
			if spec.Transport.WS.Host != "" {
				tr["headers"] = map[string]string{"Host": spec.Transport.WS.Host}
			}
		}
		return tr, nil
	case nodespec.TransportGRPC:
		tr := map[string]interface{}{"type": "grpc"}
		if spec.Transport.GRPC != nil {
			tr["service_name"] = spec.Transport.GRPC.ServiceName
		}
		return tr, nil
	case nodespec.TransportHTTPUpgrade:
		// sing-box 用 "httpupgrade"（不是 "xhttp"）
		tr := map[string]interface{}{"type": "httpupgrade"}
		if spec.Transport.HTTPUpgrade != nil {
			if spec.Transport.HTTPUpgrade.Path != "" {
				tr["path"] = spec.Transport.HTTPUpgrade.Path
			}
			if spec.Transport.HTTPUpgrade.Host != "" {
				tr["host"] = spec.Transport.HTTPUpgrade.Host
			}
		}
		return tr, nil
	case nodespec.TransportXHTTP:
		// sing-box 不原生支持 xhttp 传输。
		// - 有 downloadSettings（split mode）：sing-box 无此概念，返回 UnsupportedFeatureError，
		//   由 DualKernelValidator 优雅降级逻辑捕获并记录 Info，不阻断 Xray 侧。
		// - 无 downloadSettings（基础 XHTTP）：降级为 httpupgrade，保留 path/host 基础字段，
		//   避免硬拒绝导致 sing-box 用户完全无法使用 XHTTP 节点。
		if spec.Transport.XHTTP != nil && spec.Transport.XHTTP.DownloadSettings != nil &&
			spec.Transport.XHTTP.DownloadSettings.Address != "" {
			return nil, &UnsupportedFeatureError{
				Kernel:  KernelSingBox,
				Feature: "xhttp downloadSettings (split mode)",
				Hint:    "use xray kernel for xhttp/split-mode nodes",
			}
		}
		// 无 downloadSettings：降级为 httpupgrade（仅保留基础字段，丢弃 mode/extra 等高级特性）
		tr := map[string]interface{}{"type": "httpupgrade"}
		if spec.Transport.XHTTP != nil {
			if spec.Transport.XHTTP.Path != "" {
				tr["path"] = spec.Transport.XHTTP.Path
			}
			if spec.Transport.XHTTP.Host != "" {
				tr["host"] = spec.Transport.XHTTP.Host
			}
		}
		return tr, nil
	case nodespec.TransportQUIC:
		// QUIC 协议（Hysteria2/TUIC）的传输在 sing-box 中是协议内置的，不需要 transport 字段
		return nil, nil
	default:
		return nil, nil
	}
}

// renderMultiplex 渲染多路复用配置
// sing-box 用 multiplex（inbound 级别），Xray 用 mux（outbound 级别）
func (r *SingBoxRenderer) renderMultiplex(spec *nodespec.NodeSpec) map[string]interface{} {
	if spec.Transport.Mux == nil || !spec.Transport.Mux.Enabled {
		return nil
	}
	m := spec.Transport.Mux
	protocol := string(m.Protocol)
	if protocol == "" {
		protocol = "h2mux"
	}
	result := map[string]interface{}{
		"enabled":  true,
		"protocol": protocol,
		"padding":  m.Padding,
	}
	// 字段互斥：max_connections 与 min/max_streams 二选一，渲染器强制只写一组
	if m.MaxConnections > 0 {
		result["max_connections"] = m.MaxConnections
	} else {
		if m.MaxStreams > 0 {
			result["max_streams"] = m.MaxStreams
		}
	}
	if m.KeepAlivePeriod > 0 {
		result["keep_alive_period"] = m.KeepAlivePeriod
	}
	return result
}

// renderSingBoxSecurityRouting 生成 sing-box 安全路由规则（S6/S7/S8）。
//
// sing-box >= 1.12 使用 action: "reject" 语法；
// AdaptConfigForVersion 会自动将 action 降级为 outbound: "block"（< 1.12）。
//
// 规则：
//  1. SSRF 防护：阻断私有 IP 段
//  2. BT 防护：阻断 BitTorrent 协议
func renderSingBoxSecurityRouting() map[string]interface{} {
	return map[string]interface{}{
		"rules": []interface{}{
			// S6: SSRF 防护 — 阻断私有 IP 段
			map[string]interface{}{
				"action":  "reject",
				"ip_cidr": []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fc00::/7", "fe80::/10"},
			},
			// S7: BT 防护 — 阻断 BitTorrent 协议
			map[string]interface{}{
				"action":   "reject",
				"protocol": []string{"bittorrent"},
			},
		},
	}
}

// renderOutbounds 渲染出站配置
// sing-box 默认 outbounds 必须包含 direct/block/dns-out 共 3 个
func (r *SingBoxRenderer) renderOutbounds(spec *nodespec.NodeSpec) []interface{} {
	outbounds := []interface{}{
		map[string]interface{}{
			"type": "direct",
			"tag":  "direct",
		},
		map[string]interface{}{
			"type": "block",
			"tag":  "block",
		},
		map[string]interface{}{
			"type": "dns",
			"tag":  "dns-out",
		},
	}

	// 如果有 WARP 出口绑定（OutboundBinding），添加 wireguard outbound
	// 这里简化处理，实际 WARP 配置由上层 ChainSpec 处理

	return outbounds
}

// ===== 1.1 版本自适应降级 =====
//
// sing-box 配置格式随版本演进，低版本客户端无法解析高版本字段。
// 本段实现与 Xboard SingBox::adaptConfigForVersion 对齐的版本降级逻辑：
//
//	>= 1.12: 使用 action 语法（route.rules[].action 字段：reject/hijack-dns）
//	<  1.12: 使用旧语法（route.rules[].outbound 字段引用 block/dns 出站）
//	<  1.11: DNS 配置降级（type+server → 旧 address 格式）；恢复废弃入站字段
//	<  1.10: TUN 配置降级（address 数组 → inet4_address/inet6_address）
//
// 模板基准格式为最新版（1.12+ action 语法），渲染后按客户端 UA 版本向下降级。
// 升级（>= 1.12）与降级（< 1.12）互逆，均在 1.12 版本边界切换。

// singBoxVersionRe 匹配 UA 中的 sing-box 版本号。
// 兼容 "sing-box/1.12.3"、"singbox v1.11.0"、"SFI/1.10.0" 等格式。
var singBoxVersionRe = regexp.MustCompile(`(?i)(?:sing-?box|\b(?:sfi|sfa|sfm|sgm|sgt)\b)[\s/]+v?(\d+(?:\.\d+){0,2})`)

// DetectSingBoxVersion 从 User-Agent 提取 sing-box 内核版本号。
// 返回形如 "1.12.3" 的字符串；无法识别时返回空串。
// 对齐 Xboard SingBox::getSingBoxCoreVersion 的 UA 提取逻辑。
func DetectSingBoxVersion(ua string) string {
	if ua == "" {
		return ""
	}
	m := singBoxVersionRe.FindStringSubmatch(ua)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// RenderWithVersion 渲染服务端配置并按 sing-box 版本号自适应降级。
// version 为空时跳过降级（返回最新格式），适用于无法识别版本的场景。
func (r *SingBoxRenderer) RenderWithVersion(spec *nodespec.NodeSpec, version string) (map[string]interface{}, error) {
	cfg, err := r.Render(spec)
	if err != nil {
		return nil, err
	}
	if version != "" {
		r.AdaptConfigForVersion(cfg, version)
	}
	return cfg, nil
}

// RenderForUA 从 User-Agent 提取 sing-box 版本并渲染降级后的配置。
func (r *SingBoxRenderer) RenderForUA(spec *nodespec.NodeSpec, userAgent string) (map[string]interface{}, error) {
	return r.RenderWithVersion(spec, DetectSingBoxVersion(userAgent))
}

// AdaptConfigForVersion 按 sing-box 内核版本号自适应调整已渲染的配置。
// 直接原地修改 config，对齐 Xboard adaptConfigForVersion 的四档降级：
//
//	>= 1.12: block/dns 出站 → route.rules[].action（reject/hijack-dns）
//	<  1.12: action 降级回 block/dns 出站（与 >= 1.12 升级互逆）
//	<  1.11: DNS type+server → 旧 address 格式；恢复废弃入站字段
//	<  1.10: tun address 数组 → inet4_address/inet6_address
func (r *SingBoxRenderer) AdaptConfigForVersion(config map[string]interface{}, version string) {
	if config == nil || version == "" {
		return
	}
	v := parseVersion(version)
	if v == nil {
		return
	}

	// >= 1.12.0: 移除 block/dns 出站，升级为 route action 语法
	if versionGE(v, 1, 12, 0) {
		r.upgradeSpecialOutboundsToActions(config)
	}
	// < 1.12.0: route action 降级为旧 block/dns 出站（与上方升级互逆）
	if !versionGE(v, 1, 12, 0) {
		r.downgradeActionsToSpecialOutbounds(config)
	}
	// < 1.11.0: DNS type+server → 旧 address 格式；恢复废弃入站字段
	if !versionGE(v, 1, 11, 0) {
		r.convertDNSServersToLegacy(config)
		r.restoreDeprecatedInboundFields(config)
	}
	// < 1.10.0: tun address 数组 → inet4_address/inet6_address
	if !versionGE(v, 1, 10, 0) {
		r.convertTunAddressToLegacy(config)
	}
}

// upgradeSpecialOutboundsToActions sing-box >= 1.12.0: block/dns 出站升级为 action。
// 移除 type 为 block/dns 的出站，并将引用它们的 route.rules[].outbound 改写为 action。
func (r *SingBoxRenderer) upgradeSpecialOutboundsToActions(config map[string]interface{}) {
	rawOuts, ok := config["outbounds"].([]interface{})
	if !ok {
		return
	}
	removedTags := map[string]string{} // tag -> type
	var kept []interface{}
	for _, o := range rawOuts {
		ob, ok := o.(map[string]interface{})
		if !ok {
			kept = append(kept, o)
			continue
		}
		t, _ := ob["type"].(string)
		if t == "block" || t == "dns" {
			tag, _ := ob["tag"].(string)
			removedTags[tag] = t
			continue
		}
		kept = append(kept, o)
	}
	if len(removedTags) == 0 {
		return
	}
	config["outbounds"] = kept

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return
	}
	rules, ok := route["rules"].([]interface{})
	if !ok {
		return
	}
	for _, rr := range rules {
		rule, ok := rr.(map[string]interface{})
		if !ok {
			continue
		}
		ob, exists := rule["outbound"]
		if !exists {
			continue
		}
		tag, _ := ob.(string)
		t, hit := removedTags[tag]
		if !hit {
			continue
		}
		delete(rule, "outbound")
		if t == "dns" {
			rule["action"] = "hijack-dns"
		} else {
			rule["action"] = "reject"
		}
	}
}

// downgradeActionsToSpecialOutbounds sing-box < 1.12.0: rule action 降级为旧 block/dns 出站。
// 与 upgradeSpecialOutboundsToActions（>= 1.12）互逆。
func (r *SingBoxRenderer) downgradeActionsToSpecialOutbounds(config map[string]interface{}) {
	needsDNSOutbound := false
	needsBlockOutbound := false

	route, ok := config["route"].(map[string]interface{})
	if !ok {
		return
	}
	rules, ok := route["rules"].([]interface{})
	if !ok {
		return
	}
	for _, rr := range rules {
		rule, ok := rr.(map[string]interface{})
		if !ok {
			continue
		}
		action, exists := rule["action"]
		if !exists {
			continue
		}
		switch action {
		case "hijack-dns":
			delete(rule, "action")
			rule["outbound"] = "dns-out"
			needsDNSOutbound = true
		case "reject":
			delete(rule, "action")
			rule["outbound"] = "block"
			needsBlockOutbound = true
		}
	}

	outs, _ := config["outbounds"].([]interface{})
	if needsBlockOutbound {
		outs = append(outs, map[string]interface{}{"type": "block", "tag": "block"})
	}
	if needsDNSOutbound {
		outs = append(outs, map[string]interface{}{"type": "dns", "tag": "dns-out"})
	}
	if needsBlockOutbound || needsDNSOutbound {
		config["outbounds"] = outs
	}
}

// restoreDeprecatedInboundFields sing-box < 1.11.0: 恢复废弃的入站字段。
func (r *SingBoxRenderer) restoreDeprecatedInboundFields(config map[string]interface{}) {
	ins, ok := config["inbounds"].([]interface{})
	if !ok {
		return
	}
	for _, in := range ins {
		inbound, ok := in.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := inbound["type"].(string); t == "tun" {
			inbound["endpoint_independent_nat"] = true
		}
		if sniff, _ := inbound["sniff"].(bool); sniff {
			inbound["sniff_override_destination"] = true
		}
	}
}

// convertDNSServersToLegacy sing-box < 1.11.0: 将新 DNS server type+server 格式转换为旧 address 格式。
// 对齐 Xboard convertDnsServersToLegacy 的 type→address 映射。
func (r *SingBoxRenderer) convertDNSServersToLegacy(config map[string]interface{}) {
	dns, ok := config["dns"].(map[string]interface{})
	if !ok {
		return
	}
	servers, ok := dns["servers"].([]interface{})
	if !ok {
		return
	}
	for _, s := range servers {
		server, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		stype, exists := server["type"]
		if !exists {
			continue
		}
		t, _ := stype.(string)
		host, _ := server["server"].(string)
		switch t {
		case "https":
			server["address"] = "https://" + host + "/dns-query"
		case "tls":
			server["address"] = "tls://" + host
		case "tcp":
			server["address"] = "tcp://" + host
		case "quic":
			server["address"] = "quic://" + host
		case "udp":
			server["address"] = host
		case "block":
			server["address"] = "rcode://refused"
		case "rcode":
			rc, _ := server["rcode"].(string)
			if rc == "" {
				rc = "success"
			}
			server["address"] = "rcode://" + rc
			delete(server, "rcode")
		default:
			server["address"] = host
		}
		delete(server, "type")
		delete(server, "server")
	}
}

// convertTunAddressToLegacy sing-box < 1.10.0: 将 tun address 数组转换为 inet4_address/inet6_address。
func (r *SingBoxRenderer) convertTunAddressToLegacy(config map[string]interface{}) {
	ins, ok := config["inbounds"].([]interface{})
	if !ok {
		return
	}
	for _, in := range ins {
		inbound, ok := in.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := inbound["type"].(string); t != "tun" {
			continue
		}
		addrs, ok := inbound["address"].([]interface{})
		if !ok {
			continue
		}
		for _, a := range addrs {
			addr, _ := a.(string)
			if addr == "" {
				continue
			}
			if strings.Contains(addr, ":") {
				inbound["inet6_address"] = addr
			} else {
				inbound["inet4_address"] = addr
			}
		}
		delete(inbound, "address")
	}
}

// ===== 1.2 include/exclude/fallback 正则匹配 =====
//
// 订阅 URL 支持参数过滤节点（对齐 Xboard buildOutbounds 的 include/exclude/fallback 逻辑）：
//
//	?include=正则  - 只包含 tag 匹配的节点
//	?exclude=正则  - 排除 tag 匹配的节点
//	?fallback=正则 - 当 include 无匹配时的回退（可为多候选，逗号分隔）
//
// 裸模式（无定界符）按 ~...~ui（大小写不敏感、UTF-8）匹配；带定界符的模式原样使用。
// 非法正则不会 panic，匹配返回 false。

// patternCache 缓存已编译的正则，避免重复编译。
// B33: 原实现是无界 map 且无并发保护，恶意或大量不同 pattern 会导致内存无限增长。
//      改为有界缓存（sync.Map + 原子计数器），达到 maxPatternCacheSize 上限时清空重建。
const maxPatternCacheSize = 100

var (
	patternCache      sync.Map // map[string]*regexp.Regexp
	patternCacheCount atomic.Int64
)

// matchesPattern 判断 subject 是否匹配用户提供的 pattern。
// 对齐 Xboard SingBox::matchesPattern：裸模式自动包裹 ~...~ui 定界符。
func matchesPattern(pattern, subject string) bool {
	if pattern == "" {
		return true
	}
	re := compilePattern(pattern)
	if re == nil {
		return false
	}
	return re.MatchString(subject)
}

// compilePattern 编译用户模式，带缓存。非法模式返回 nil。
// B33: 以 trim 后的 pattern 作为缓存 key，避免 " foo" 与 "foo " 这类仅首尾空白
//      差异的模式产生不同 key 却编译出等价正则、重复占用缓存；裸模式转义也统一使用
//      trimmed，保证 key 与编译结果一致。缓存条目达到 maxPatternCacheSize 时
//      整体清空并重新计数（simple but effective，防止无限增长）。
func compilePattern(pattern string) *regexp.Regexp {
	trimmed := strings.TrimSpace(pattern)
	key := trimmed
	if cached, ok := patternCache.Load(key); ok {
		return cached.(*regexp.Regexp)
	}
	var compiled *regexp.Regexp
	if looksDelimited(trimmed) {
		compiled, _ = regexp.Compile(trimmed)
	} else {
		// 裸模式：转义 ~ 后包裹 ~...~ui（大小写不敏感 + UTF-8）
		escaped := strings.ReplaceAll(trimmed, "~", `\~`)
		compiled, _ = regexp.Compile("~" + escaped + "~ui")
	}
	actual, loaded := patternCache.LoadOrStore(key, compiled)
	if !loaded {
		// 新增条目：检查是否超出上限，达到上限则整体清空重建
		if patternCacheCount.Add(1) >= maxPatternCacheSize {
			patternCache.Range(func(k, v any) bool {
				patternCache.Delete(k)
				return true
			})
			patternCacheCount.Store(0)
		}
	}
	return actual.(*regexp.Regexp)
}

// looksDelimited 判断 pattern 是否为已定界正则（如 /foo/i、#bar#u）。
func looksDelimited(p string) bool {
	if len(p) < 2 {
		return false
	}
	first := p[0]
	switch first {
	case '/', '#', '~', '@', '%':
	default:
		return false
	}
	// 末尾需为相同定界符 + 可选修饰符
	lastDelim := strings.LastIndexByte(p[1:], first)
	if lastDelim < 0 {
		return false
	}
	return true
}

// FilterNodeSpecs 按 include/exclude/fallback 过滤节点列表，返回保留的节点。
// include/exclude/fallback 均为可选（空串表示不限制）。
// 当 include 非空但无任何匹配时，尝试用 fallback 回退。
func FilterNodeSpecs(nodes []nodespec.NodeSpec, include, exclude, fallback string) []nodespec.NodeSpec {
	if include == "" && exclude == "" {
		return nodes
	}
	allTags := make([]string, len(nodes))
	for i := range nodes {
		allTags[i] = nodeTag(&nodes[i])
	}

	result := make([]nodespec.NodeSpec, 0, len(nodes))
	for i := range nodes {
		tag := allTags[i]
		if include != "" && !matchesPattern(include, tag) {
			continue
		}
		if exclude != "" && matchesPattern(exclude, tag) {
			continue
		}
		result = append(result, nodes[i])
	}

	// include 有值但无匹配 → 回退
	if len(result) == 0 && include != "" && fallback != "" {
		result = resolveFallbackNodes(nodes, allTags, fallback)
	}
	return result
}

// FilterNodeSpecPtrs 与 FilterNodeSpecs 相同，但作用于指针切片（避免拷贝大结构体）。
func FilterNodeSpecPtrs(nodes []*nodespec.NodeSpec, include, exclude, fallback string) []*nodespec.NodeSpec {
	if include == "" && exclude == "" {
		return nodes
	}
	allTags := make([]string, len(nodes))
	for i := range nodes {
		allTags[i] = nodeTag(nodes[i])
	}
	result := make([]*nodespec.NodeSpec, 0, len(nodes))
	for i := range nodes {
		tag := allTags[i]
		if include != "" && !matchesPattern(include, tag) {
			continue
		}
		if exclude != "" && matchesPattern(exclude, tag) {
			continue
		}
		result = append(result, nodes[i])
	}
	if len(result) == 0 && include != "" && fallback != "" {
		result = make([]*nodespec.NodeSpec, 0, len(nodes))
		candidates := strings.Split(fallback, ",")
		matched := resolveFallbackTags(allTags, candidates)
		for _, tag := range matched {
			for i := range nodes {
				if allTags[i] == tag {
					result = append(result, nodes[i])
				}
			}
		}
	}
	return result
}

// resolveFallbackNodes 将 fallback 解析为节点列表。
func resolveFallbackNodes(nodes []nodespec.NodeSpec, allTags []string, fallback string) []nodespec.NodeSpec {
	candidates := strings.Split(fallback, ",")
	matched := resolveFallbackTags(allTags, candidates)
	if len(matched) == 0 {
		return nil
	}
	result := make([]nodespec.NodeSpec, 0, len(matched))
	for _, tag := range matched {
		for i := range nodes {
			if allTags[i] == tag {
				result = append(result, nodes[i])
			}
		}
	}
	return result
}

// resolveFallbackTags 将 fallback 候选列表解析为可用 tag 列表。
// 每个候选可为：内置 tag（direct/block）、精确 tag、或正则模式。
// 返回首个非空解析结果（对齐 Xboard resolveFallback 的 first-hit-wins）。
func resolveFallbackTags(allTags []string, candidates []string) []string {
	tagSet := make(map[string]struct{}, len(allTags))
	for _, t := range allTags {
		tagSet[t] = struct{}{}
	}
	for _, candidate := range candidates {
		c := strings.TrimSpace(candidate)
		if c == "" {
			continue
		}
		// 内置出站或精确 tag
		if _, ok := tagSet[c]; ok {
			return []string{c}
		}
		if c == "direct" || c == "block" || c == "dns-out" {
			return []string{c}
		}
		// 正则模式匹配
		var matched []string
		for _, t := range allTags {
			if matchesPattern(c, t) {
				matched = append(matched, t)
			}
		}
		if len(matched) > 0 {
			return matched
		}
	}
	return nil
}

// nodeTag 返回节点的 tag（优先 Name，其次 Code）。
func nodeTag(spec *nodespec.NodeSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Name != "" {
		return spec.Name
	}
	return spec.Code
}

// ===== 版本号解析与比较辅助 =====

// parsedVersion 是已解析的语义化版本（major.minor.patch）。
type parsedVersion struct {
	major, minor, patch int
}

// parseVersion 解析形如 "1.12.3" / "1.12" / "1" 的版本号。
func parseVersion(v string) *parsedVersion {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return nil
	}
	pv := &parsedVersion{}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	pv.major, pv.minor, pv.patch = nums[0], nums[1], nums[2]
	return pv
}

// versionGE 判断 v 是否 >= major.minor.patch。
func versionGE(v *parsedVersion, major, minor, patch int) bool {
	if v.major != major {
		return v.major > major
	}
	if v.minor != minor {
		return v.minor > minor
	}
	return v.patch >= patch
}
