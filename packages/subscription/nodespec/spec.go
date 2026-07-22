package nodespec

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

type Protocol string
type Transport string
type Security string
type TLSCertMode string
type MuxProtocol string
type FlowControl string

const (
	ProtocolVLESS       Protocol = "vless"
	ProtocolVMess       Protocol = "vmess"
	ProtocolTrojan      Protocol = "trojan"
	ProtocolShadowsocks Protocol = "shadowsocks"
	ProtocolHysteria2   Protocol = "hysteria2"
	ProtocolTUIC        Protocol = "tuic"
	ProtocolAnyTLS      Protocol = "anytls"
	ProtocolMieru       Protocol = "mieru"
	ProtocolSOCKS5      Protocol = "socks"
	ProtocolHTTP        Protocol = "http"

	TransportTCP         Transport = "tcp"
	TransportWS          Transport = "ws"
	TransportGRPC        Transport = "grpc"
	TransportHTTP2       Transport = "http2"
	TransportQUIC        Transport = "quic"
	TransportKCP         Transport = "kcp"
	TransportHTTPUpgrade Transport = "httpupgrade"
	TransportXHTTP       Transport = "xhttp"

	SecurityNone    Security = "none"
	SecurityTLS     Security = "tls"
	SecurityReality Security = "reality"

	TLSCertModeNone  TLSCertMode = "none"
	TLSCertModeFile  TLSCertMode = "file"
	TLSCertModePaste TLSCertMode = "paste"
	TLSCertModeACME  TLSCertMode = "acme"

	MuxProtocolNone  MuxProtocol = ""
	MuxProtocolYamux MuxProtocol = "yamux"
	MuxProtocolH2Mux MuxProtocol = "h2mux"
	MuxProtocolSmux  MuxProtocol = "smux"
	MuxProtocolXmux  MuxProtocol = "xmux"

	FlowNone           FlowControl = ""
	FlowXTLSRprxDirect FlowControl = "xtls-rprx-direct"
	FlowXTLSRprxSplice FlowControl = "xtls-rprx-splice"
	FlowXTLSRprxVision FlowControl = "xtls-rprx-vision"
)

var ValidProtocols = map[Protocol]bool{
	ProtocolVLESS: true, ProtocolVMess: true, ProtocolTrojan: true,
	ProtocolShadowsocks: true, ProtocolHysteria2: true, ProtocolTUIC: true,
	ProtocolAnyTLS: true, ProtocolMieru: true,
	ProtocolSOCKS5: true, ProtocolHTTP: true,
}

var ValidTransports = map[Transport]bool{
	TransportTCP: true, TransportWS: true, TransportGRPC: true,
	TransportHTTP2: true, TransportQUIC: true, TransportKCP: true,
	TransportHTTPUpgrade: true, TransportXHTTP: true,
}

var ValidSecurity = map[Security]bool{
	SecurityNone: true, SecurityTLS: true, SecurityReality: true,
}

var ValidTLSCertModes = map[TLSCertMode]bool{
	TLSCertModeNone: true, TLSCertModeFile: true, TLSCertModePaste: true, TLSCertModeACME: true,
}

var ValidMuxProtocols = map[MuxProtocol]bool{
	MuxProtocolNone: true, MuxProtocolYamux: true, MuxProtocolH2Mux: true,
	MuxProtocolSmux: true, MuxProtocolXmux: true,
}

var ValidFlows = map[FlowControl]bool{
	FlowNone: true, FlowXTLSRprxDirect: true, FlowXTLSRprxSplice: true, FlowXTLSRprxVision: true,
}

type VLESSCredentials struct {
	UUID       string      `json:"uuid"`
	Flow       FlowControl `json:"flow,omitempty"`
	Encryption string      `json:"encryption,omitempty"`
}

type VMessCredentials struct {
	UUID     string `json:"uuid"`
	AlterID  int    `json:"alterId,omitempty"`
	Security string `json:"security,omitempty"` // VMess cipher: auto/none/aes-128-gcm/chacha20-poly1305/zero（空值由渲染器默认 auto）
}

type TrojanCredentials struct {
	Password string `json:"password"`
}

type ShadowsocksCredentials struct {
	Password string `json:"password"`
	Method   string `json:"method"`
}

type Hysteria2Credentials struct {
	Password string `json:"password"`
	UpMbps   int    `json:"up_mbps,omitempty"`   // Hysteria2 上行带宽（Mbps），用于 BBR 拥塞控制协商
	DownMbps int    `json:"down_mbps,omitempty"` // Hysteria2 下行带宽（Mbps）
}

type TUICCredentials struct {
	UUID     string `json:"uuid"`
	Password string `json:"password,omitempty"`
}

type AnyTLSCredentials struct {
	Password string `json:"password"`
}

type MieruCredentials struct {
	Password     string `json:"password"`
	Username     string `json:"username,omitempty"`
	Multiplexing bool   `json:"multiplexing,omitempty"`
}

// SOCKS5Credentials 是 SOCKS5 代理的凭证类型。
// 对齐 Xboard General::buildSocks，username/password 组合认证。
type SOCKS5Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HTTPCredentials 是 HTTP/HTTPS 代理的凭证类型。
// 对齐 Xboard General::buildHttp，username/password 组合认证。
type HTTPCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CredentialSpec 是统一的多用户凭证 IR 类型（P0-4）。
// 用于服务端多用户场景：NodeSpec.Clients []CredentialSpec。
// 渲染器优先使用 Clients（多用户），为空时回退到 Credentials（单用户，向后兼容）。
// 字段语义对齐 Xray clients[] 与 sing-box users[]，但保持内核中立。
type CredentialSpec struct {
	Email       string         `json:"email,omitempty"`
	UUID        string         `json:"uuid,omitempty"`
	Password    string         `json:"password,omitempty"`
	Flow        FlowControl    `json:"flow,omitempty"`
	Method      string         `json:"method,omitempty"`    // Shadowsocks
	AlterID     int            `json:"alterId,omitempty"`   // VMess
	Security    string         `json:"security,omitempty"`  // VMess cipher
	UpMbps      int            `json:"up_mbps,omitempty"`   // Hysteria2
	DownMbps    int            `json:"down_mbps,omitempty"` // Hysteria2
	Level       int            `json:"level,omitempty"`
	SpeedLimit  int            `json:"speed_limit,omitempty"`  // per-user speed limit (Mbps)
	DeviceLimit int            `json:"device_limit,omitempty"` // per-user device limit
	IPLimit     int            `json:"ip_limit,omitempty"`     // per-user IP limit (max concurrent IPs)
	Extra       map[string]any `json:"extra,omitempty"`
}

type TLSConfig struct {
	SNI           string      `json:"sni,omitempty"`
	ALPN          []string    `json:"alpn,omitempty"`
	Fingerprint   string      `json:"fingerprint,omitempty"`
	AllowInsecure bool        `json:"allow_insecure,omitempty"`
	CertMode      TLSCertMode `json:"cert_mode,omitempty"`
	CertFile      string      `json:"cert_file,omitempty"`
	KeyFile       string      `json:"key_file,omitempty"`
	CertPEM       string      `json:"cert_pem,omitempty"`
	KeyPEM        string      `json:"key_pem,omitempty"`
	ACMEDomains   []string    `json:"acme_domains,omitempty"`
	ACMEEmail     string      `json:"acme_email,omitempty"`
	ECH           *ECHConfig  `json:"ech,omitempty"`
	// PinSHA256 是证书指纹（SHA-256），用于 Hysteria2 等协议的证书锁定。
	// 对齐 Xboard Hysteria2 URI 中的 pinSHA256 参数。
	PinSHA256 string `json:"pin_sha256,omitempty"`
	// Material 是 PEM-only 证书资源引用（P0-2）。
	// 非 nil 时渲染器优先使用 Material.InlinePEM，禁止输出 certificateFile/keyFile。
	// 为 nil 时回退到 CertFile/CertPEM（向后兼容，迁移期保留）。
	Material *TLSMaterialRef `json:"material,omitempty"`
}

// TLSMaterialMode 标识证书来源模式
type TLSMaterialMode string

const (
	TLSMaterialModeACME    TLSMaterialMode = "acme"
	TLSMaterialModeFile    TLSMaterialMode = "file"
	TLSMaterialModeContent TLSMaterialMode = "content"
	TLSMaterialModeSelf    TLSMaterialMode = "self"
	TLSMaterialModeNone    TLSMaterialMode = "none"
)

// TLSMaterialRef 是 PEM-only 证书资源引用（P0-2）。
// 证书不再使用路径，而是作为平台一级资源引用进入编译器。
// 这把证书从"文件系统依赖"升级为"版本化配置链路"。
type TLSMaterialRef struct {
	Mode         TLSMaterialMode `json:"mode"`
	CertBundleID string          `json:"cert_bundle_id,omitempty"` // 证书包ID（版本化，对应 cert_bundles 表）
	InlinePEM    *PEMBundle      `json:"inline_pem,omitempty"`     // content 模式时填充
}

// PEMBundle 是 inline PEM 证书包
type PEMBundle struct {
	CertPEM []byte `json:"cert_pem"`
	KeyPEM  []byte `json:"key_pem"`
}

type RealityConfig struct {
	SNI         string   `json:"sni,omitempty"`
	PublicKey   string   `json:"public_key"`
	ShortID     string   `json:"short_id"`
	PrivateKey  string   `json:"private_key,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	SpiderX     string   `json:"spider_x,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	ShortIDs    []string `json:"short_ids,omitempty"`
	// Dest 是 REALITY 的回落目标（host:port 格式，如 "127.0.0.1:8460" 或 "example.com:443"）。
	// 为空时由渲染器回退到 SNI:443（保持向后兼容）。
	// 推荐使用本地反代地址（如 127.0.0.1:8460 → nginx vhost 反代真实站点），
	// 避免直接回落到 SNI 自身造成 xray 反代自己的循环问题。
	Dest string `json:"dest,omitempty"`
}

type ECHConfig struct {
	Enabled     bool   `json:"enabled"`
	PEM         string `json:"pem,omitempty"`
	Key         string `json:"key,omitempty"`
	QueryDomain string `json:"query_domain,omitempty"`
}

type WSConfig struct {
	Path    string            `json:"path,omitempty"`
	Host    string            `json:"host,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type GRPCConfig struct {
	ServiceName string `json:"service_name,omitempty"`
	IdleTimeout int    `json:"idle_timeout,omitempty"`
	HealthCheck bool   `json:"health_check,omitempty"`
}

type HTTP2Config struct {
	Host    string            `json:"host,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type HTTPUpgradeConfig struct {
	Host    string            `json:"host,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Mode    string            `json:"mode,omitempty"`
}

type XHTTPConfig struct {
	Path                 string               `json:"path,omitempty"`
	Host                 string               `json:"host,omitempty"`
	Mode                 string               `json:"mode,omitempty"`
	Headers              map[string]string    `json:"headers,omitempty"`
	ScMaxEachPostBytes   int                  `json:"sc_max_each_post_bytes,omitempty"`
	ScMinPostsIntervalMs int                  `json:"sc_min_posts_interval_ms,omitempty"`
	XPaddingBytes        string               `json:"x_padding_bytes,omitempty"`
	NoGRPCHeader         bool                 `json:"no_grpc_header,omitempty"`
	DownloadSettings     *XHTTPDownloadConfig `json:"download_settings,omitempty"`
}

type XHTTPDownloadConfig struct {
	Address      string         `json:"address,omitempty" yaml:"address,omitempty"`
	AddressIPv6  string         `json:"address_ipv6,omitempty" yaml:"address_ipv6,omitempty"`
	Port         int            `json:"port,omitempty" yaml:"port,omitempty"`
	ServerPort   int            `json:"server_port,omitempty" yaml:"server_port,omitempty"`
	Network      Transport      `json:"network,omitempty" yaml:"network,omitempty"`
	Security     Security       `json:"security,omitempty" yaml:"security,omitempty"`
	Path         string         `json:"path,omitempty" yaml:"path,omitempty"`
	Host         string         `json:"host,omitempty" yaml:"host,omitempty"`
	Mode         string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	NoGRPCHeader bool           `json:"no_grpc_header,omitempty" yaml:"no_grpc_header,omitempty"`
	Reality      *RealityConfig `json:"reality,omitempty" yaml:"reality,omitempty"`
	TLS          *TLSConfig     `json:"tls,omitempty" yaml:"tls,omitempty"`
}

type KCPConfig struct {
	Seed             string `json:"seed,omitempty"`
	MTU              int    `json:"mtu,omitempty"`
	TTI              int    `json:"tti,omitempty"`
	UplinkCapacity   int    `json:"uplink_capacity,omitempty"`
	DownlinkCapacity int    `json:"downlink_capacity,omitempty"`
	Congestion       bool   `json:"congestion,omitempty"`
	ReadBufferSize   int    `json:"read_buffer_size,omitempty"`
	WriteBufferSize  int    `json:"write_buffer_size,omitempty"`
	HeaderType       string `json:"header_type,omitempty"`
}

type QUICConfig struct {
	Security string            `json:"security,omitempty"`
	Key      string            `json:"key,omitempty"`
	Header   map[string]string `json:"header,omitempty"`
}

type MuxConfig struct {
	Enabled         bool        `json:"enabled"`
	Protocol        MuxProtocol `json:"protocol,omitempty"`
	MaxConnections  int         `json:"max_connections,omitempty"`
	MaxStreams      int         `json:"max_streams,omitempty"`
	Padding         bool        `json:"padding,omitempty"`
	KeepAlivePeriod int         `json:"keep_alive_period,omitempty"`
	// XMUX 专用字段（Xray xhttpSettings.extra.xmux），支持范围值如 "16-32"
	MaxConcurrency   string `json:"max_concurrency,omitempty"`   // XMUX: "16-32"
	CMaxReuseTimes   string `json:"c_max_reuse_times,omitempty"` // XMUX: "64-128"
	HMaxRequestTimes string `json:"h_max_request_times,omitempty"`
	HMaxReusableSecs string `json:"h_max_reusable_secs,omitempty"`
}

// SockoptConfig Xray outbound sockopt 性能参数
type SockoptConfig struct {
	TCPFastOpen  bool   `json:"tcp_fast_open,omitempty"`
	TCPMultipath bool   `json:"tcp_multipath,omitempty"`
	Congestion   string `json:"congestion,omitempty"` // "bbr"
	TCPKeepAlive int    `json:"tcp_keep_alive,omitempty"`
}

type TCPBrutalConfig struct {
	Enabled  bool `json:"enabled"`
	UpMbps   int  `json:"up_mbps,omitempty"`
	DownMbps int  `json:"down_mbps,omitempty"`
}

type PortHoppingConfig struct {
	Enabled     bool   `json:"enabled"`
	PortRange   string `json:"port_range,omitempty"`
	Interval    int    `json:"interval,omitempty"`
	PortMapping string `json:"port_mapping,omitempty"`
}

type TransportConfig struct {
	Type        Transport              `json:"type"`
	WS          *WSConfig              `json:"ws,omitempty"`
	GRPC        *GRPCConfig            `json:"grpc,omitempty"`
	HTTP2       *HTTP2Config           `json:"http2,omitempty"`
	HTTPUpgrade *HTTPUpgradeConfig     `json:"httpupgrade,omitempty"`
	XHTTP       *XHTTPConfig           `json:"xhttp,omitempty"`
	KCP         *KCPConfig             `json:"kcp,omitempty"`
	QUIC        *QUICConfig            `json:"quic,omitempty"`
	Mux         *MuxConfig             `json:"mux,omitempty"`
	TCPBrutal   *TCPBrutalConfig       `json:"tcp_brutal,omitempty"`
	PortHopping *PortHoppingConfig     `json:"port_hopping,omitempty"`
	Sockopt     *SockoptConfig         `json:"sockopt,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	RawSettings map[string]interface{} `json:"raw_settings,omitempty" yaml:"raw_settings,omitempty"`
}

type NodeSpec struct {
	NumericID   uint64          `json:"numeric_id" yaml:"numeric_id"`
	ID          string          `json:"id" yaml:"id"`
	Code        string          `json:"code" yaml:"code"`
	Name        string          `json:"name" yaml:"name"`
	Protocol    Protocol        `json:"protocol" yaml:"protocol"`
	Address     string          `json:"address" yaml:"address"`
	AddressIPv6 string          `json:"address_ipv6,omitempty" yaml:"address_ipv6,omitempty"`
	Port        int             `json:"port" yaml:"port"`
	ClientPort  int             `json:"client_port,omitempty" yaml:"client_port,omitempty"`
	ServerPort  int             `json:"server_port,omitempty" yaml:"server_port,omitempty"`
	Transport   TransportConfig `json:"transport" yaml:"transport"`
	Security    Security        `json:"security" yaml:"security"`
	TLS         *TLSConfig      `json:"tls,omitempty" yaml:"tls,omitempty"`
	Reality     *RealityConfig  `json:"reality,omitempty" yaml:"reality,omitempty"`
	// PaddingScheme 是 AnyTLS 协议的填充方案（如 "max-0" 表示无填充）。
	// 仅 AnyTLS 协议在 sing-box 服务端渲染时使用；为空时由内核使用默认值。
	PaddingScheme  string           `json:"padding_scheme,omitempty" yaml:"padding_scheme,omitempty"`
	Credentials    interface{}      `json:"credentials" yaml:"credentials"`
	Clients        []CredentialSpec `json:"clients,omitempty" yaml:"clients,omitempty"`
	AllowUDP       bool             `json:"allow_udp" yaml:"allow_udp"`
	SpeedLimitMbps int              `json:"speed_limit_mbps,omitempty" yaml:"speed_limit_mbps,omitempty"`
	DeviceLimit    int              `json:"device_limit,omitempty" yaml:"device_limit,omitempty"`
	IPLimit        int              `json:"ip_limit,omitempty" yaml:"ip_limit,omitempty"`
	TrafficRate    float64          `json:"traffic_rate" yaml:"traffic_rate"`
	// TransferEnableBytes 是节点级流量限额（字节），0 表示不限额。
	// 由 P3-N 引入：当节点累计流量超过此值时，traffic-service 会禁用该节点。
	TransferEnableBytes int64                  `json:"transfer_enable_bytes,omitempty" yaml:"transfer_enable_bytes,omitempty"`
	Tags                []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
	Group               string                 `json:"group,omitempty" yaml:"group,omitempty"`
	PermissionGroups    []string               `json:"permission_groups,omitempty" yaml:"permission_groups,omitempty"`
	RouteGroups         []string               `json:"route_groups,omitempty" yaml:"route_groups,omitempty"`
	ServerBindings      []ServerBinding        `json:"server_bindings,omitempty" yaml:"server_bindings,omitempty"`
	ParentNodeID        string                 `json:"parent_node_id,omitempty" yaml:"parent_node_id,omitempty"`
	ParentNumericID     uint64                 `json:"parent_numeric_id,omitempty" yaml:"parent_numeric_id,omitempty"`
	ChainNodes          []string               `json:"chain_nodes,omitempty" yaml:"chain_nodes,omitempty"`
	CustomOutbounds     []interface{}          `json:"custom_outbounds,omitempty" yaml:"custom_outbounds,omitempty"`
	CustomRoutes        []interface{}          `json:"custom_routes,omitempty" yaml:"custom_routes,omitempty"`
	IsVisible           bool                   `json:"is_visible" yaml:"is_visible"`
	Priority            int                    `json:"priority,omitempty" yaml:"priority,omitempty"`
	Region              string                 `json:"region,omitempty"`
	NodeType            string                 `json:"node_type,omitempty"`
	PresetID            string                 `json:"preset_id,omitempty" yaml:"preset_id,omitempty"`
	RawConfig           map[string]interface{} `json:"raw_config,omitempty" yaml:"raw_config,omitempty"`
}

type ServerBinding struct {
	ServerID   string `json:"server_id"`
	ServerSID  int    `json:"server_sid"`
	ServerName string `json:"server_name"`
	AutoManage bool   `json:"auto_manage"`
	ListenPort int    `json:"listen_port,omitempty"`
}

func (t *TransportConfig) MergeRaw(dst map[string]interface{}) {
	if dst == nil {
		return
	}
	structuredKeys := make(map[string]bool)
	if t.WS != nil {
		if t.WS.Path != "" {
			structuredKeys["path"] = true
		}
		if t.WS.Host != "" {
			structuredKeys["host"] = true
		}
		if len(t.WS.Headers) > 0 {
			structuredKeys["headers"] = true
		}
	}
	if t.GRPC != nil {
		if t.GRPC.ServiceName != "" {
			structuredKeys["service_name"] = true
			structuredKeys["serviceName"] = true
		}
	}
	if t.HTTP2 != nil {
		if t.HTTP2.Path != "" {
			structuredKeys["path"] = true
		}
		if t.HTTP2.Host != "" {
			structuredKeys["host"] = true
		}
		if len(t.HTTP2.Headers) > 0 {
			structuredKeys["headers"] = true
		}
	}
	if t.HTTPUpgrade != nil {
		if t.HTTPUpgrade.Path != "" {
			structuredKeys["path"] = true
		}
		if t.HTTPUpgrade.Host != "" {
			structuredKeys["host"] = true
		}
		if len(t.HTTPUpgrade.Headers) > 0 {
			structuredKeys["headers"] = true
		}
		if t.HTTPUpgrade.Mode != "" {
			structuredKeys["mode"] = true
		}
	}
	if t.XHTTP != nil {
		if t.XHTTP.Path != "" {
			structuredKeys["path"] = true
		}
		if t.XHTTP.Host != "" {
			structuredKeys["host"] = true
		}
		if t.XHTTP.Mode != "" {
			structuredKeys["mode"] = true
		}
		structuredKeys["downloadSettings"] = true
		structuredKeys["xPaddingBytes"] = true
		structuredKeys["noGRPCHeader"] = true
	}
	if t.KCP != nil {
		if t.KCP.Seed != "" {
			structuredKeys["seed"] = true
		}
		if t.KCP.MTU > 0 {
			structuredKeys["mtu"] = true
		}
		if t.KCP.TTI > 0 {
			structuredKeys["tti"] = true
		}
		if t.KCP.UplinkCapacity > 0 {
			structuredKeys["uplinkCapacity"] = true
		}
		if t.KCP.DownlinkCapacity > 0 {
			structuredKeys["downlinkCapacity"] = true
		}
		structuredKeys["congestion"] = true
		if t.KCP.ReadBufferSize > 0 {
			structuredKeys["readBufferSize"] = true
		}
		if t.KCP.WriteBufferSize > 0 {
			structuredKeys["writeBufferSize"] = true
		}
		if t.KCP.HeaderType != "" {
			structuredKeys["header"] = true
		}
	}
	if t.QUIC != nil {
		if t.QUIC.Security != "" {
			structuredKeys["security"] = true
		}
		if t.QUIC.Key != "" {
			structuredKeys["key"] = true
		}
		if len(t.QUIC.Header) > 0 {
			structuredKeys["header"] = true
		}
	}
	structuredKeys["mux"] = true
	structuredKeys["tcp_brutal"] = true
	structuredKeys["port_hopping"] = true

	for k, v := range t.RawSettings {
		if !structuredKeys[k] {
			if _, exists := dst[k]; !exists {
				dst[k] = v
			}
		}
	}
}

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (n *NodeSpec) Validate() error {
	if n.ID == "" {
		return errors.New("id is required")
	}
	if n.Code == "" {
		return errors.New("code is required")
	}
	if n.Name == "" {
		return errors.New("name is required")
	}
	if !ValidProtocols[n.Protocol] {
		return fmt.Errorf("invalid protocol: %s", n.Protocol)
	}
	if !ValidTransports[n.Transport.Type] {
		return fmt.Errorf("invalid transport: %s", n.Transport.Type)
	}
	if !ValidSecurity[n.Security] {
		return fmt.Errorf("invalid security: %s", n.Security)
	}
	if n.Port < 1 || n.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", n.Port)
	}
	if n.ClientPort != 0 && (n.ClientPort < 1 || n.ClientPort > 65535) {
		return fmt.Errorf("invalid client_port: %d", n.ClientPort)
	}
	if n.ServerPort != 0 && (n.ServerPort < 1 || n.ServerPort > 65535) {
		return fmt.Errorf("invalid server_port: %d", n.ServerPort)
	}
	if n.Address == "" {
		return errors.New("address is required")
	}
	if n.TrafficRate <= 0 {
		n.TrafficRate = 1.0
	}

	if n.Security == SecurityTLS && n.TLS != nil {
		if err := validateTLS(n.TLS, n.Protocol); err != nil {
			return err
		}
	}
	if n.Security == SecurityReality {
		if n.Reality == nil {
			return errors.New("reality config is required when security is reality")
		}
		if err := validateReality(n.Reality); err != nil {
			return err
		}
	}
	if n.Security == SecurityReality && n.Protocol != ProtocolVLESS && n.Protocol != ProtocolShadowsocks && n.Protocol != ProtocolAnyTLS {
		return fmt.Errorf("reality is only supported with vless/shadowsocks protocols, got %s", n.Protocol)
	}

	if n.Transport.Mux != nil && n.Transport.Mux.Enabled && n.Transport.Mux.Protocol != "" {
		if !ValidMuxProtocols[n.Transport.Mux.Protocol] {
			return fmt.Errorf("invalid mux protocol: %s", n.Transport.Mux.Protocol)
		}
	}

	switch n.Transport.Type {
	case TransportWS:
		if n.Transport.WS == nil {
			return errors.New("ws config is required for ws transport")
		}
	case TransportGRPC:
		if n.Transport.GRPC == nil || n.Transport.GRPC.ServiceName == "" {
			return errors.New("grpc service_name is required")
		}
	case TransportXHTTP:
		if n.Transport.XHTTP == nil {
			return errors.New("xhttp config is required for xhttp transport")
		}
	case TransportHTTP2, TransportHTTPUpgrade, TransportKCP, TransportQUIC:
	}

	return validateCredentials(n.Protocol, n.Credentials)
}

func validateTLS(tls *TLSConfig, proto Protocol) error {
	if tls.CertMode != "" && !ValidTLSCertModes[tls.CertMode] {
		return fmt.Errorf("invalid tls cert_mode: %s", tls.CertMode)
	}
	if tls.Fingerprint != "" {
		validFP := map[string]bool{
			"chrome": true, "firefox": true, "safari": true, "edge": true,
			"random": true, "randomized": true, "ios": true, "android": true,
		}
		if !validFP[tls.Fingerprint] {
			return fmt.Errorf("invalid tls fingerprint: %s", tls.Fingerprint)
		}
	}
	return nil
}

func validateReality(r *RealityConfig) error {
	if r.PublicKey == "" {
		return errors.New("reality public_key is required")
	}
	if r.ShortID == "" && len(r.ShortIDs) == 0 {
		return errors.New("reality short_id or short_ids is required")
	}
	if len(r.PublicKey) != 43 && len(r.PublicKey) != 44 {
		return fmt.Errorf("reality public_key appears invalid (expected base64, got len %d)", len(r.PublicKey))
	}
	return nil
}

func validateCredentials(proto Protocol, creds interface{}) error {
	if creds == nil {
		return errors.New("credentials are required")
	}
	switch c := creds.(type) {
	case VLESSCredentials:
		if c.UUID == "" {
			return errors.New("vless uuid is required")
		}
		if !isValidUUID(c.UUID) {
			return fmt.Errorf("invalid vless uuid format: %s", c.UUID)
		}
		if c.Flow != "" && !ValidFlows[c.Flow] {
			return fmt.Errorf("invalid vless flow: %s", c.Flow)
		}
	case *VLESSCredentials:
		if c == nil {
			return errors.New("vless credentials required")
		}
		return validateCredentials(proto, *c)
	case VMessCredentials:
		if c.UUID == "" {
			return errors.New("vmess uuid is required")
		}
		if !isValidUUID(c.UUID) {
			return fmt.Errorf("invalid vmess uuid format: %s", c.UUID)
		}
	case *VMessCredentials:
		if c == nil {
			return errors.New("vmess credentials required")
		}
		return validateCredentials(proto, *c)
	case TrojanCredentials:
		if c.Password == "" {
			return errors.New("trojan password is required")
		}
		if len(c.Password) < 6 {
			return errors.New("trojan password must be at least 6 characters")
		}
	case *TrojanCredentials:
		if c == nil {
			return errors.New("trojan credentials required")
		}
		return validateCredentials(proto, *c)
	case ShadowsocksCredentials:
		if c.Password == "" {
			return errors.New("shadowsocks password is required")
		}
		if c.Method == "" {
			return errors.New("shadowsocks method is required")
		}
		validMethods := map[string]bool{
			"aes-128-gcm": true, "aes-256-gcm": true, "chacha20-ietf-poly1305": true,
			"xchacha20-ietf-poly1305": true, "2022-blake3-aes-128-gcm": true,
			"2022-blake3-aes-256-gcm": true, "2022-blake3-chacha20-poly1305": true,
		}
		if !validMethods[c.Method] {
			return fmt.Errorf("unsupported shadowsocks method: %s", c.Method)
		}
	case *ShadowsocksCredentials:
		if c == nil {
			return errors.New("shadowsocks credentials required")
		}
		return validateCredentials(proto, *c)
	case Hysteria2Credentials:
		if c.Password == "" {
			return errors.New("hysteria2 password is required")
		}
	case TUICCredentials:
		if c.UUID == "" {
			return errors.New("tuic uuid is required")
		}
		if !isValidUUID(c.UUID) {
			return fmt.Errorf("invalid tuic uuid: %s", c.UUID)
		}
	case AnyTLSCredentials:
		if c.Password == "" {
			return errors.New("anytls password is required")
		}
	case MieruCredentials:
		if c.Password == "" {
			return errors.New("mieru password is required")
		}
	case SOCKS5Credentials:
		if c.Password == "" {
			return errors.New("socks5 password is required")
		}
	case HTTPCredentials:
		if c.Password == "" {
			return errors.New("http password is required")
		}
	case map[string]interface{}:
		return validateCredentialsMap(proto, c)
	default:
		return fmt.Errorf("unsupported credentials type for %s: %T", proto, creds)
	}
	return nil
}

func validateCredentialsMap(proto Protocol, m map[string]interface{}) error {
	getStr := func(k string) string {
		v, _ := m[k].(string)
		return v
	}
	switch proto {
	case ProtocolVLESS:
		uuid := getStr("uuid")
		if uuid == "" {
			return errors.New("vless uuid is required")
		}
		if !isValidUUID(uuid) {
			return fmt.Errorf("invalid vless uuid: %s", uuid)
		}
	case ProtocolTrojan, ProtocolHysteria2, ProtocolAnyTLS, ProtocolMieru:
		if getStr("password") == "" {
			return fmt.Errorf("%s password is required", proto)
		}
	case ProtocolSOCKS5, ProtocolHTTP:
		if getStr("password") == "" {
			return fmt.Errorf("%s password is required", proto)
		}
	case ProtocolVMess:
		uuid := getStr("uuid")
		if uuid == "" || !isValidUUID(uuid) {
			return errors.New("vmess uuid required and must be valid")
		}
	case ProtocolShadowsocks:
		if getStr("password") == "" || getStr("method") == "" {
			return errors.New("ss password and method required")
		}
	case ProtocolTUIC:
		uuid := getStr("uuid")
		if uuid == "" || !isValidUUID(uuid) {
			return errors.New("tuic uuid required")
		}
	}
	return nil
}

func isValidUUID(s string) bool {
	s = strings.ToLower(s)
	return uuidRegex.MatchString(s)
}

func isAddress(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	if len(s) > 253 {
		return false
	}
	if !strings.Contains(s, ".") {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

var _ = strconv.Itoa
var _ = net.ParseIP

// RenderClientFlow 是渲染客户端 flow 字段的唯一出口。
// 所有渲染路径（uri/clash/singbox/xray/inboundgroup）必须通过此函数获取 flow 值，
// 杜绝分散在各渲染分支里各自判断导致的不一致。
//
// 规则：
//   - TransportTCP (Vision 直连) → "xtls-rprx-vision"
//   - 其他传输类型（XHTTP/WS/gRPC/HTTPUpgrade/KCP/QUIC/H2）→ ""
//
// 未来新增 transport 类型只需在 switch 中加一行 case。
func RenderClientFlow(transportType Transport) FlowControl {
	switch transportType {
	case TransportTCP:
		return FlowXTLSRprxVision
	default:
		return FlowNone
	}
}
