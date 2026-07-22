package exposure

// Deprecated: This file contains dual-renderer code that bypasses the IR (NodeSpec) layer.
// All config rendering should go through packages/subscription/kernelrender/ instead.
// The functions below (BuildXrayInbounds, BuildXrayInboundsWithCreds, etc.) are zombie code
// with zero callers and should be removed in a future cleanup.

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
)

const (
	tlsCertPath = "/etc/yundu/config/ssl/cert.pem"
	tlsKeyPath  = "/etc/yundu/config/ssl/private.key"
)

type XrayClient struct {
	ID      string `json:"id"`
	Flow    string `json:"flow,omitempty"`
	Email   string `json:"email,omitempty"`
	Level   int    `json:"level,omitempty"`
}

type XrayVLESSSettings struct {
	Clients    []XrayClient `json:"clients"`
	Decryption string       `json:"decryption"`
}

type XrayVMessSettings struct {
	Clients []XrayVMessClient `json:"clients"`
}

type XrayVMessClient struct {
	ID      string `json:"id"`
	AlterID int    `json:"alterId"`
	Email   string `json:"email,omitempty"`
	Level   int    `json:"level,omitempty"`
}

type XrayTrojanSettings struct {
	Clients []XrayTrojanClient `json:"clients"`
}

type XrayTrojanClient struct {
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
	Level    int    `json:"level,omitempty"`
}

type XraySSSettings struct {
	Method   string          `json:"method,omitempty"`
	Password string          `json:"password,omitempty"`
	Network  string          `json:"network,omitempty"`
	Clients  []XraySSClient  `json:"clients,omitempty"`
}

func isSS2022Method(method string) bool {
	return strings.HasPrefix(method, "2022-blake3-")
}

type XraySSClient struct {
	Password string `json:"password"`
	Method   string `json:"method,omitempty"`
	Email    string `json:"email,omitempty"`
	Level    int    `json:"level,omitempty"`
}

type XrayWSSettings struct {
	Host    string            `json:"host,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type XrayGRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

type XrayHTTP2Settings struct {
	Host []string `json:"host,omitempty"`
	Path string   `json:"path,omitempty"`
}

type XrayHTTPUpgradeSettings struct {
	Path    string            `json:"path,omitempty"`
	Host    string            `json:"host,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type XrayXHTTPSettings struct {
	Host    string            `json:"host,omitempty"`
	Path    string            `json:"path,omitempty"`
	Mode    string            `json:"mode,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

type XraySplitHTTPSettings struct {
	Host    string `json:"host,omitempty"`
	Path    string `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type XrayTLSSettings struct {
	ServerName   string             `json:"serverName,omitempty"`
	ALPN         []string           `json:"alpn,omitempty"`
	MinVersion   string             `json:"minVersion,omitempty"`
	MaxVersion   string             `json:"maxVersion,omitempty"`
	Certificates []XrayCertificate  `json:"certificates,omitempty"`
	Fingerprint  string             `json:"fingerprint,omitempty"`
	AllowInsecure bool              `json:"allowInsecure,omitempty"`
}

type XrayRealitySettings struct {
	Show         bool     `json:"show,omitempty"`
	Dest         string   `json:"dest,omitempty"`
	Xver         int      `json:"xver,omitempty"`
	ServerNames  []string `json:"serverNames,omitempty"`
	PrivateKey   string   `json:"privateKey,omitempty"`
	MinClientVer string   `json:"minClientVer,omitempty"`
	MaxClientVer string   `json:"maxClientVer,omitempty"`
	MaxTimeDiff  int64    `json:"maxTimeDiff,omitempty"`
	ShortIds     []string `json:"shortIds,omitempty"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
}

type XrayCertificate struct {
	CertFile string `json:"certificateFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
}

type XrayTCPSettings struct {
	Header XrayTCPHeader `json:"header,omitempty"`
}

type XrayTCPHeader struct {
	Type    string                 `json:"type,omitempty"`
	Request map[string]interface{} `json:"request,omitempty"`
}

type XrayStreamSettings struct {
	Network              string                  `json:"network"`
	Security             string                  `json:"security,omitempty"`
	TLSSettings          *XrayTLSSettings        `json:"tlsSettings,omitempty"`
	WSSettings           *XrayWSSettings         `json:"wsSettings,omitempty"`
	GRPCSettings         *XrayGRPCSettings       `json:"grpcSettings,omitempty"`
	HTTPSettings         *XrayHTTP2Settings      `json:"httpSettings,omitempty"`
	HTTPUpgradeSettings  *XrayHTTPUpgradeSettings `json:"httpupgradeSettings,omitempty"`
	XHTTPSettings        *XrayXHTTPSettings      `json:"xhttpSettings,omitempty"`
	SplitHTTPSettings    *XraySplitHTTPSettings  `json:"splithttpSettings,omitempty"`
	TCPSettings          *XrayTCPSettings        `json:"tcpSettings,omitempty"`
	RealitySettings      *XrayRealitySettings    `json:"realitySettings,omitempty"`
}

type XrayInbound struct {
	Listen         string                 `json:"listen,omitempty"`
	Port           int                    `json:"port"`
	Protocol       string                 `json:"protocol"`
	Tag            string                 `json:"tag,omitempty"`
	Settings       interface{}            `json:"settings"`
	StreamSettings *XrayStreamSettings    `json:"streamSettings,omitempty"`
	Sniffing       *XraySniffing          `json:"sniffing,omitempty"`
}

type XraySniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

type XrayOutbound struct {
	Protocol string      `json:"protocol"`
	Tag      string      `json:"tag"`
	Settings interface{} `json:"settings,omitempty"`
}

type XrayRoutingRule struct {
	Type        string   `json:"type"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
	IP          []string `json:"ip,omitempty"`
	Domain      []string `json:"domain,omitempty"`
}

type XrayRouting struct {
	DomainStrategy string           `json:"domainStrategy,omitempty"`
	Rules          []XrayRoutingRule `json:"rules"`
}

type XrayLog struct {
	Loglevel string `json:"loglevel"`
}

type XrayAPI struct {
	Tag      string   `json:"tag"`
	Services []string `json:"services"`
}

type XrayStats struct{}

type XrayConfig struct {
	Log       XrayLog        `json:"log"`
	API       *XrayAPI       `json:"api,omitempty"`
	Stats     *XrayStats     `json:"stats,omitempty"`
	Policy    *XrayPolicy    `json:"policy,omitempty"`
	Inbounds  []XrayInbound  `json:"inbounds"`
	Outbounds []XrayOutbound `json:"outbounds"`
	Routing   XrayRouting    `json:"routing"`
}

type XrayPolicy struct {
	Levels map[string]XrayPolicyLevel `json:"levels,omitempty"`
}

type XrayPolicyLevel struct {
	StatsUserUplink   bool `json:"statsUserUplink,omitempty"`
	StatsUserDownlink bool `json:"statsUserDownlink,omitempty"`
}

var xraySupportedProtocols = map[string]bool{
	"vless":      true,
	"vmess":      true,
	"trojan":     true,
	"shadowsocks": true,
	"ss":         true,
	"socks":      true,
	"http":       true,
}

func isXraySupportedProtocol(proto string) bool {
	return xraySupportedProtocols[strings.ToLower(proto)]
}

// Deprecated: zero callers, use kernelrender instead
func BuildXrayInbounds(nodes []*model.Node) []map[string]interface{} {
	return BuildXrayInboundsWithCreds(nodes, nil)
}

// Deprecated: zero callers, use kernelrender instead
// BuildXrayInboundsWithCreds 构建带多用户凭证的 xray inbounds。
// creds 为 nil 或某节点无凭证时，回退到 node.config_json 中的单用户配置（向后兼容）。
func BuildXrayInboundsWithCreds(nodes []*model.Node, creds NodeCredentials) []map[string]interface{} {
	inbounds := make([]map[string]interface{}, 0)
	hasNodes := false

	for _, node := range nodes {
		if !node.IsEnabled {
			continue
		}
		if !isXraySupportedProtocol(node.ProtocolType) {
			continue
		}
		hasNodes = true

		var nodeCreds []*repo.UserNodeCredential
		if creds != nil {
			nodeCreds = creds[node.ID]
		}
		settings := buildXrayProtocolSettings(node, nodeCreds)
		streamSettings := buildXrayStreamSettings(node)
		// P1 修复：xray listen 协议家族错误硬编码为 "::"（IPv6 any）导致 IPv4 路径不通。
		// 正确做法：直接使用渲染器生成的 listen 值
		//   - CDN/Tunnel/Direct-TCP 节点 → 127.0.0.1（仅本地 nginx 转发，绑 IPv4）
		//   - DIRECT/UDP 直连节点 → 0.0.0.0（双栈接收公网）
		// 详见 kernelrender.resolveListenAddress。
		//
		// 历史：VPS206 实测 xray 9445/9449/8890/9450 等端口只监听 [::]（IPv6 only），
		// 而 nginx 8445 vhost proxy_pass 是 IPv4 127.0.0.1，导致 IPv4 路径必然 502。
		listen := "0.0.0.0" // P1 修复：默认 IPv4+IPv6 双栈（覆盖原硬编码 "::"）
		if isCDNNodeForExposure(node) {
			// CDN 节点必须绑 127.0.0.1（只接受 nginx 8445 vhost 反代，不暴露公网）
			listen = "127.0.0.1"
		}
		// 特殊：Argo Tunnel 节点需要 [::] 让 cloudflared 连 [::1]（文档 §4.5 明确要求）
		if isArgoTunnelNode(node) {
			listen = "::"
		}

		inbound := map[string]interface{}{
			"listen":         listen,
			"port":           node.Port,
			"protocol":       normalizeProtocol(node.ProtocolType),
			"tag":            node.Code,
			"settings":       settings,
			"streamSettings": streamSettings,
			"sniffing": map[string]interface{}{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
			},
		}
		inbounds = append(inbounds, inbound)
	}

	apiInbound := map[string]interface{}{
		"listen":   "127.0.0.1",
		"port":     10085,
		"protocol": "dokodemo-door",
		"tag":      "api",
		"settings": map[string]interface{}{
			"address": "127.0.0.1",
		},
	}
	inbounds = append(inbounds, apiInbound)

	if !hasNodes {
		inbounds = append(inbounds, map[string]interface{}{
			"listen":   "0.0.0.0",
			"port":     10086,
			"protocol": "dokodemo-door",
			"tag":      "placeholder",
			"settings": map[string]interface{}{
				"address": "127.0.0.1",
			},
		})
	}

	return inbounds
}

func normalizeProtocol(proto string) string {
	switch strings.ToLower(proto) {
	case "ss", "shadowsocks":
		return "shadowsocks"
	default:
		return strings.ToLower(proto)
	}
}

// Deprecated: zero callers, use kernelrender instead
func DefaultXrayRouting() map[string]interface{} {
	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"inboundTag":  []string{"api"},
				"outboundTag": "api",
			},
			map[string]interface{}{
				"type":        "field",
				"outboundTag": "block",
				"ip":          []string{"geoip:private"},
			},
		},
	}
}

func buildXrayProtocolSettings(node *model.Node, nodeCreds []*repo.UserNodeCredential) interface{} {
	switch strings.ToLower(node.ProtocolType) {
	case "vless":
		clients := extractXrayClients(node, nodeCreds)
		return XrayVLESSSettings{
			Clients:    clients,
			Decryption: "none",
		}
	case "vmess":
		vmessClients := extractXrayVMessClients(node, nodeCreds)
		return XrayVMessSettings{
			Clients: vmessClients,
		}
	case "trojan":
		trojanClients := extractXrayTrojanClients(node, nodeCreds)
		return XrayTrojanSettings{
			Clients: trojanClients,
		}
	case "shadowsocks", "ss":
		ssClients := extractXraySSClients(node, nodeCreds)
		settings := XraySSSettings{}
		if len(ssClients) > 0 {
			if isSS2022Method(ssClients[0].Method) {
				settings.Method = ssClients[0].Method
				for i := range ssClients {
					ssClients[i].Method = ""
				}
				if pwd, _ := getStringFromConfig(node.ConfigJSON, "password"); pwd != "" && len(ssClients) == 1 {
					settings.Password = pwd
					settings.Clients = nil
				} else {
					settings.Clients = ssClients
				}
			} else {
				settings.Clients = ssClients
			}
		} else {
			method, _ := getStringFromConfig(node.ConfigJSON, "method")
			password, _ := getStringFromConfig(node.ConfigJSON, "password")
			if method != "" && password != "" {
				settings.Method = method
				settings.Password = password
			} else if method != "" {
				settings.Method = method
				settings.Clients = ssClients
			} else {
				settings.Clients = ssClients
			}
		}
		settings.Network = "tcp,udp"
		return settings
	default:
		return map[string]interface{}{}
	}
}

func buildXrayStreamSettings(node *model.Node) *XrayStreamSettings {
	network := strings.ToLower(node.TransportType)
	ss := &XrayStreamSettings{
		Network: network,
	}

	security := "none"
	if node.SecurityType != nil && *node.SecurityType != "" {
		security = strings.ToLower(*node.SecurityType)
	}
	ss.Security = security

	path := "/"
	if node.Path != nil && *node.Path != "" {
		path = *node.Path
	}
	host := ""
	if node.HostHeader != nil {
		host = *node.HostHeader
	}
	sni := ""
	if node.SNI != nil {
		sni = *node.SNI
	}

	switch network {
	case "ws":
		wsSettings := &XrayWSSettings{
			Path: path,
			Host: host,
		}
		ss.WSSettings = wsSettings
	case "grpc":
		grpcSettings := &XrayGRPCSettings{
			ServiceName: strings.TrimPrefix(path, "/"),
			MultiMode:   false,
		}
		if grpcSettings.ServiceName == "" || grpcSettings.ServiceName == "/" {
			grpcSettings.ServiceName = "grpc"
		}
		ss.GRPCSettings = grpcSettings
	case "http", "h2":
		httpSettings := &XrayHTTP2Settings{
			Path: path,
		}
		if host != "" {
			httpSettings.Host = []string{host}
		}
		ss.HTTPSettings = httpSettings
	case "httpupgrade":
		httpUpgSettings := &XrayHTTPUpgradeSettings{
			Path: path,
			Host: host,
		}
		ss.HTTPUpgradeSettings = httpUpgSettings
	case "xhttp":
		xhttpSettings := &XrayXHTTPSettings{
			Path: path,
			Host: host,
		}
		// 当 security=reality 时，xhttpSettings.host 必须与 REALITY serverName (sni) 一致，
		// 否则 REALITY 握手时 SNI 与 HTTP Host header 不匹配会导致连接失败。
		// 零SSH修复：自动覆盖 host，无需手动在面板配置中同步。
		if security == "reality" && sni != "" {
			xhttpSettings.Host = sni
		}
		if mode, ok := getStringFromConfig(node.ConfigJSON, "mode"); ok && mode != "" {
			xhttpSettings.Mode = mode
		} else {
			if security == "reality" {
				xhttpSettings.Mode = "stream-up"
			} else {
				xhttpSettings.Mode = "packet-up"
			}
		}
		// 渲染 extra.xmux（XHTTP 专用多路复用，Xray 26.x 专有）
		// 对应 kernelrender/xray.go 的 renderStreamSettings 行302-336
		extra := buildXHTTPExtra(node.ConfigJSON)
		if len(extra) > 0 {
			xhttpSettings.Extra = extra
		}
		ss.XHTTPSettings = xhttpSettings
	case "splithttp":
		splitSettings := &XraySplitHTTPSettings{
			Path: path,
			Host: host,
		}
		ss.SplitHTTPSettings = splitSettings
	case "tcp":
		tcpSettings := &XrayTCPSettings{
			Header: XrayTCPHeader{
				Type: "none",
			},
		}
		ss.TCPSettings = tcpSettings
	}

	if security == "tls" {
		// 证书路径：优先从 config_json 读取（支持 cert_file/key_file 顶层 + tls_settings/tls 嵌套），回退到默认路径
		// 借鉴 xboard cert_config 的 cert_file/key_file 字段
		certFile := tlsCertPath
		keyFile := tlsKeyPath
		if cf := pickStringNested(node.ConfigJSON, "cert_file", "tls_settings", "tls"); cf != "" {
			certFile = cf
		}
		if kf := pickStringNested(node.ConfigJSON, "key_file", "tls_settings", "tls"); kf != "" {
			keyFile = kf
		}
		tlsSettings := &XrayTLSSettings{
			Certificates: []XrayCertificate{
				{
					CertFile: certFile,
					KeyFile:  keyFile,
				},
			},
			MinVersion: "1.2",
			MaxVersion: "1.3",
		}
		if sni != "" {
			tlsSettings.ServerName = sni
		}
		if len(node.ALPN) > 0 {
			tlsSettings.ALPN = node.ALPN
		} else {
			tlsSettings.ALPN = []string{"h2", "http/1.1"}
		}
		// uTLS 指纹（三级回退：顶层 fingerprint > tls_settings.fingerprint > utls_fingerprint）
		// 借鉴 xboard UTLS_CONFIGURATION 的 fingerprint 字段
		if fp := pickStringNested(node.ConfigJSON, "fingerprint", "tls_settings", "tls"); fp != "" {
			tlsSettings.Fingerprint = fp
		} else if fp, ok := getStringFromConfig(node.ConfigJSON, "utls_fingerprint"); ok && fp != "" {
			tlsSettings.Fingerprint = fp
		}
		// allow_insecure 三级回退（顶层 > tls_settings > tls）
		// 前端可能以 bool(true) 或 number(1) 写入，pickBoolNested 兼容两种类型
		tlsSettings.AllowInsecure = pickBoolNested(node.ConfigJSON, "allow_insecure", "tls_settings", "tls")
		ss.TLSSettings = tlsSettings
	} else if security == "reality" {
		realitySettings := &XrayRealitySettings{
			Show: false,
			Xver: 0,
		}
		// 默认伪装网站统一为 mesu.apple.com（用户规范，禁止使用 rust-lang.org 或 www.microsoft.com）。
		// 当节点未指定 SNI 时，回退到 mesu.apple.com:443。
		dest := "mesu.apple.com:443"
		if sni != "" {
			realitySettings.ServerNames = []string{sni}
			dest = sni + ":443"
		} else {
			realitySettings.ServerNames = []string{"mesu.apple.com"}
		}
		// dest 多级回退：顶层 dest > 顶层 reality_dest > reality_settings.dest > reality_settings.server_name:server_port
		// 注：normalizer 白名单中允许 reality_dest（不允许 dest），所以优先查找 reality_dest
		if destOverride := pickStringNested(node.ConfigJSON, "dest", "reality", "reality_settings"); destOverride != "" {
			dest = destOverride
		} else if realityDest, ok := node.ConfigJSON["reality_dest"].(string); ok && realityDest != "" {
			dest = realityDest
		} else if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
			if sn, ok := rs["server_name"].(string); ok && sn != "" {
				sp := 443
				if p, ok := rs["server_port"].(float64); ok && p > 0 {
					sp = int(p)
				}
				dest = fmt.Sprintf("%s:%d", sn, sp)
				realitySettings.ServerNames = []string{sn}
			}
		}
		realitySettings.Dest = dest
		// private_key 三级回退：顶层 > reality.private_key > reality_settings.private_key
		// 安全修复 S1：移除硬编码私钥回退，缺失时 panic 防止使用共享密钥对
		if privKey := pickStringNested(node.ConfigJSON, "private_key", "reality", "reality_settings"); privKey != "" {
			realitySettings.PrivateKey = privKey
		} else {
			slog.Error("REALITY private_key missing — refusing to use hardcoded fallback",
				"node_id", node.ID, "node_name", node.Name)
			return ss // 返回不含 REALITY 配置，使部署预检失败
		}
		// short_id 三级回退
		// 安全修复 S2：移除硬编码 short_id 回退
		if shortID := pickStringNested(node.ConfigJSON, "short_id", "reality", "reality_settings"); shortID != "" {
			realitySettings.ShortIds = []string{shortID}
		} else {
			// 不设 short_id 等同于允许任意 short_id（xray 默认行为），但记录警告
			slog.Warn("REALITY short_id missing, allowing empty short_id",
				"node_id", node.ID, "node_name", node.Name)
			realitySettings.ShortIds = []string{""}
		}
		// fingerprint 三级回退：reality.fingerprint > reality_settings.fingerprint > 顶层 fingerprint/utls_fingerprint
		if fp := pickStringNested(node.ConfigJSON, "fingerprint", "reality", "reality_settings"); fp != "" {
			realitySettings.Fingerprint = fp
		} else if fp, ok := getStringFromConfig(node.ConfigJSON, "utls_fingerprint"); ok && fp != "" {
			realitySettings.Fingerprint = fp
		} else if fp, ok := getStringFromConfig(node.ConfigJSON, "reality_utls_fingerprint"); ok && fp != "" {
			realitySettings.Fingerprint = fp
		}
		ss.RealitySettings = realitySettings
	}

	return ss
}

// buildXHTTPExtra 从 config_json 构建 xhttpSettings.extra 字段
// 包含 XMUX（多路复用）、Headers、downloadSettings（split mode 上下行分离）
// 对应 kernelrender/xray.go 的 renderStreamSettings extra 渲染逻辑
func buildXHTTPExtra(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	extra := map[string]interface{}{}

	// 1. XMUX 渲染（xhttp.extra.xmux）
	if xhttpMap, ok := cfg["xhttp"].(map[string]interface{}); ok {
		if extraMap, ok := xhttpMap["extra"].(map[string]interface{}); ok {
			// 直接复用 config_json 中的 extra 结构
			if xmux, ok := extraMap["xmux"].(map[string]interface{}); ok && len(xmux) > 0 {
				extra["xmux"] = xmux
			}
			if headers, ok := extraMap["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				extra["headers"] = headers
			}
			if ds, ok := extraMap["downloadSettings"].(map[string]interface{}); ok && len(ds) > 0 {
				extra["downloadSettings"] = ds
			}
		}
	}

	// 2. 兼容顶层 xmux 字段（旧格式 xhttp.xmux）
	if xhttpMap, ok := cfg["xhttp"].(map[string]interface{}); ok {
		if xmux, ok := xhttpMap["xmux"].(map[string]interface{}); ok && len(xmux) > 0 {
			if _, exists := extra["xmux"]; !exists {
				extra["xmux"] = xmux
			}
		}
	}

	if len(extra) == 0 {
		return nil
	}
	return extra
}


func extractXrayClients(node *model.Node, nodeCreds []*repo.UserNodeCredential) []XrayClient {
	// 优先使用 per-user 凭证（多用户模式）
	if len(nodeCreds) > 0 {
		return buildXrayClientsFromCreds(node, nodeCreds)
	}

	clients := make([]XrayClient, 0)

	if node.ConfigJSON == nil {
		return defaultXrayClient(node)
	}

	if clientList, ok := node.ConfigJSON["clients"].([]interface{}); ok {
		for _, c := range clientList {
			if cm, ok := c.(map[string]interface{}); ok {
				client := XrayClient{}
				if id, ok := cm["id"].(string); ok {
					client.ID = id
				}
				if flow, ok := cm["flow"].(string); ok {
					client.Flow = flow
				}
				if email, ok := cm["email"].(string); ok {
					client.Email = email
				}
				if level, ok := cm["level"].(float64); ok {
					client.Level = int(level)
				}
				if client.ID != "" {
					clients = append(clients, client)
				}
			}
		}
	}

	if len(clients) == 0 {
		return defaultXrayClient(node)
	}

	return clients
}

// buildXrayClientsFromCreds 从 per-user 凭证生成 VLESS clients
// 每个 UUID 凭证对应一个 client，flow 从节点配置继承，email 设为 userID 用于流量统计
func buildXrayClientsFromCreds(node *model.Node, creds []*repo.UserNodeCredential) []XrayClient {
	clients := make([]XrayClient, 0, len(creds))
	for _, c := range creds {
		if c.CredentialType != "uuid" || c.CredentialValue == "" {
			continue
		}
		client := XrayClient{
			ID:    c.CredentialValue,
			Email: c.UserID.String(),
		}
		if node.Flow != nil && *node.Flow != "" {
			client.Flow = *node.Flow
		} else if flow, ok := getStringFromConfig(node.ConfigJSON, "flow"); ok && flow != "" {
			client.Flow = flow
		}
		clients = append(clients, client)
	}
	return clients
}

func defaultXrayClient(node *model.Node) []XrayClient {
	clients := make([]XrayClient, 0)
	if node.ConfigJSON == nil {
		return clients
	}
	if uuid, ok := getStringFromConfig(node.ConfigJSON, "uuid"); ok && uuid != "" {
		client := XrayClient{ID: uuid}
		if node.Flow != nil && *node.Flow != "" {
			client.Flow = *node.Flow
		} else if flow, ok := getStringFromConfig(node.ConfigJSON, "flow"); ok && flow != "" {
			client.Flow = flow
		}
		clients = append(clients, client)
	}
	return clients
}

func extractXrayVMessClients(node *model.Node, nodeCreds []*repo.UserNodeCredential) []XrayVMessClient {
	// 优先使用 per-user 凭证（多用户模式）
	if len(nodeCreds) > 0 {
		clients := make([]XrayVMessClient, 0, len(nodeCreds))
		for _, c := range nodeCreds {
			if c.CredentialType != "uuid" || c.CredentialValue == "" {
				continue
			}
			clients = append(clients, XrayVMessClient{
				ID:      c.CredentialValue,
				AlterID: 0,
				Email:   c.UserID.String(),
			})
		}
		return clients
	}

	clients := make([]XrayVMessClient, 0)

	if node.ConfigJSON == nil {
		return clients
	}

	if clientList, ok := node.ConfigJSON["clients"].([]interface{}); ok {
		for _, c := range clientList {
			if cm, ok := c.(map[string]interface{}); ok {
				client := XrayVMessClient{AlterID: 0}
				if id, ok := cm["id"].(string); ok {
					client.ID = id
				}
				if alterId, ok := cm["alterId"].(float64); ok {
					client.AlterID = int(alterId)
				}
				if email, ok := cm["email"].(string); ok {
					client.Email = email
				}
				if client.ID != "" {
					clients = append(clients, client)
				}
			}
		}
	}

	if len(clients) == 0 {
		if uuid, ok := getStringFromConfig(node.ConfigJSON, "uuid"); ok && uuid != "" {
			client := XrayVMessClient{ID: uuid, AlterID: 0}
			if aid, ok := node.ConfigJSON["alterId"].(float64); ok {
				client.AlterID = int(aid)
			}
			clients = append(clients, client)
		}
	}

	return clients
}

func extractXrayTrojanClients(node *model.Node, nodeCreds []*repo.UserNodeCredential) []XrayTrojanClient {
	// 优先使用 per-user 凭证（多用户模式）
	// 对齐 XBoard：Trojan 密码 = user.uuid，所有协议共用 UUID
	if len(nodeCreds) > 0 {
		clients := make([]XrayTrojanClient, 0, len(nodeCreds))
		for _, c := range nodeCreds {
			if c.CredentialValue == "" {
				continue
			}
			clients = append(clients, XrayTrojanClient{
				Password: c.CredentialValue,
				Email:    c.UserID.String(),
			})
		}
		return clients
	}

	clients := make([]XrayTrojanClient, 0)

	if node.ConfigJSON == nil {
		// 安全修复 S3：移除硬编码密码回退
		slog.Error("Trojan config missing (ConfigJSON nil) — refusing to use hardcoded fallback",
			"node_id", node.ID, "node_name", node.Name)
		return clients
	}

	if clientList, ok := node.ConfigJSON["clients"].([]interface{}); ok {
		for _, c := range clientList {
			if cm, ok := c.(map[string]interface{}); ok {
				client := XrayTrojanClient{}
				if password, ok := cm["password"].(string); ok {
					client.Password = password
				}
				if email, ok := cm["email"].(string); ok {
					client.Email = email
				}
				if client.Password != "" {
					clients = append(clients, client)
				}
			}
		}
	}

	if len(clients) == 0 {
		if password, ok := getStringFromConfig(node.ConfigJSON, "password"); ok && password != "" {
			clients = append(clients, XrayTrojanClient{Password: password})
		} else {
			// 安全修复 S3：移除硬编码密码回退，返回空切片使部署预检失败
			slog.Error("Trojan password missing — refusing to use hardcoded fallback",
				"node_id", node.ID, "node_name", node.Name)
			return clients // 空 clients
		}
	}

	return clients
}

func extractXraySSClients(node *model.Node, nodeCreds []*repo.UserNodeCredential) []XraySSClient {
	// 优先使用 per-user 凭证（多用户模式）
	// 对齐 XBoard：SS 密码 = user.uuid（SS2022 派生在订阅渲染层处理，节点端统一用 uuid）
	// SS 多用户：每个用户对应一个 client，method 从节点配置继承
	if len(nodeCreds) > 0 {
		method, _ := getStringFromConfig(node.ConfigJSON, "method")
		clients := make([]XraySSClient, 0, len(nodeCreds))
		for _, c := range nodeCreds {
			if c.CredentialValue == "" {
				continue
			}
			clients = append(clients, XraySSClient{
				Password: c.CredentialValue,
				Method:   method,
				Email:    c.UserID.String(),
			})
		}
		return clients
	}

	clients := make([]XraySSClient, 0)

	if node.ConfigJSON == nil {
		return clients
	}

	if clientList, ok := node.ConfigJSON["clients"].([]interface{}); ok {
		for _, c := range clientList {
			if cm, ok := c.(map[string]interface{}); ok {
				client := XraySSClient{}
				if password, ok := cm["password"].(string); ok {
					client.Password = password
				}
				if method, ok := cm["method"].(string); ok {
					client.Method = method
				}
				if email, ok := cm["email"].(string); ok {
					client.Email = email
				}
				if client.Password != "" && client.Method != "" {
					clients = append(clients, client)
				}
			}
		}
	}

	if len(clients) == 0 {
		password, _ := getStringFromConfig(node.ConfigJSON, "password")
		method, _ := getStringFromConfig(node.ConfigJSON, "method")
		if password != "" && method != "" {
			clients = append(clients, XraySSClient{
				Password: password,
				Method:   method,
			})
		}
	}

	return clients
}

// isCDNNodeForExposure 判断节点是否为 CDN 节点（config_json 中包含 cdn_address）。
// CDN 节点需要 listen 127.0.0.1（仅 nginx 反代可访问）。
func isCDNNodeForExposure(n *model.Node) bool {
	if n == nil || n.ConfigJSON == nil {
		return false
	}
	cdnAddr, ok := n.ConfigJSON["cdn_address"].(string)
	return ok && strings.TrimSpace(cdnAddr) != ""
}

// isArgoTunnelNode 判断节点是否为 Argo Tunnel 节点。
// Argo Tunnel 节点需要 listen [::] 让 cloudflared 连 [::1]。
func isArgoTunnelNode(n *model.Node) bool {
	if n == nil || n.ConfigJSON == nil {
		return false
	}
	pt := strings.ToLower(n.ProtocolType)
	if strings.Contains(pt, "argo") {
		return true
	}
	if token, ok := n.ConfigJSON["cf_tunnel_token"].(string); ok && token != "" {
		return true
	}
	return false
}
