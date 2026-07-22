package renderer

import (
	"encoding/json"
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

type SingBoxRenderer struct{}

func NewSingBoxRenderer() *SingBoxRenderer     { return &SingBoxRenderer{} }
func (r *SingBoxRenderer) Name() string        { return "singbox" }
func (r *SingBoxRenderer) ContentType() string { return "application/json; charset=utf-8" }

type SBTLS struct {
	Enabled    bool       `json:"enabled"`
	ServerName string     `json:"server_name,omitempty"`
	ALPN       []string   `json:"alpn,omitempty"`
	UTLS       *SBUTLS    `json:"utls,omitempty"`
	Insecure   bool       `json:"insecure,omitempty"`
	Reality    *SBReality `json:"reality,omitempty"`
}

type SBUTLS struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type SBReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
}

type SBTransport struct {
	Type                string            `json:"type,omitempty"`
	Path                string            `json:"path,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	Host                interface{}       `json:"host,omitempty"`
	ServiceName         string            `json:"service_name,omitempty"`
	Method              string            `json:"method,omitempty"`
	MaxEarlyData        int               `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string            `json:"early_data_header_name,omitempty"`
}

type SBMux struct {
	Enabled    bool   `json:"enabled"`
	Protocol   string `json:"protocol,omitempty"`
	MaxStreams int    `json:"max_streams,omitempty"`
	Padding    bool   `json:"padding,omitempty"`
}

type SBOutbound struct {
	Type                string          `json:"type"`
	Tag                 string          `json:"tag"`
	Server              string          `json:"server,omitempty"`
	ServerPort          int             `json:"server_port,omitempty"`
	UUID                string          `json:"uuid,omitempty"`
	Password            string          `json:"password,omitempty"`
	Method              string          `json:"method,omitempty"`
	Security            string          `json:"security,omitempty"`
	AlterID             int             `json:"alter_id,omitempty"`
	Flow                string          `json:"flow,omitempty"`
	PacketEncoding      string          `json:"packet_encoding,omitempty"`
	TLS                 *SBTLS          `json:"tls,omitempty"`
	Transport           *SBTransport    `json:"transport,omitempty"`
	Multiplex           *SBMux          `json:"multiplex,omitempty"`
	Outbounds           []string        `json:"outbounds,omitempty"`
	URL                 string          `json:"url,omitempty"`
	Interval            string          `json:"interval,omitempty"`
	Tolerance           int             `json:"tolerance,omitempty"`
	Include             interface{}     `json:"include,omitempty"`
	Exclude             string          `json:"exclude,omitempty"`
	Default             string          `json:"default,omitempty"`
	InterruptExistConns bool            `json:"interrupt_exist_connections,omitempty"`
	UpMbps              int             `json:"up_mbps,omitempty"`
	DownMbps            int             `json:"down_mbps,omitempty"`
	Obfs                *SBHysteriaObfs `json:"obfs,omitempty"`
	ObfsPassword        string          `json:"obfs_password,omitempty"`
	CongestionControl   string          `json:"congestion_control,omitempty"`
	UdpRelayMode        string          `json:"udp_relay_mode,omitempty"`
	ZeroRTTHandshake    bool            `json:"zero_rtt_handshake,omitempty"`
	Heartbeat           string          `json:"heartbeat,omitempty"`
	UDP                 bool            `json:"udp,omitempty"`
	DomainStrategy      string          `json:"domain_strategy,omitempty"`
	Network             string          `json:"network,omitempty"`
}

type SBHysteriaObfs struct {
	Type     string `json:"type,omitempty"`
	Password string `json:"password,omitempty"`
}

type SBDNS struct {
	Servers []interface{} `json:"servers"`
	Rules   []interface{} `json:"rules,omitempty"`
	Final   string        `json:"final,omitempty"`
}

type SBInbound struct {
	Type                     string   `json:"type"`
	Tag                      string   `json:"tag"`
	ListenOn                 string   `json:"listen,omitempty"`
	ListenPort               int      `json:"listen_port,omitempty"`
	Sniff                    bool     `json:"sniff,omitempty"`
	SniffOverrideDestination bool     `json:"sniff_override_destination,omitempty"`
	DomainStrategy           string   `json:"domain_strategy,omitempty"`
	Address                  []string `json:"address,omitempty"`
	AutoRoute                bool     `json:"auto_route,omitempty"`
	Stack                    string   `json:"stack,omitempty"`
	StrictRoute              bool     `json:"strict_route,omitempty"`
}

type SBRoute struct {
	Rules               []interface{} `json:"rules"`
	Final               string        `json:"final,omitempty"`
	AutoDetectInterface bool          `json:"auto_detect_interface,omitempty"`
	OverridePort        int           `json:"override_android_vpn,omitempty"`
}

type SBConfig struct {
	Log          map[string]interface{} `json:"log,omitempty"`
	DNS          *SBDNS                 `json:"dns,omitempty"`
	Inbounds     []SBInbound            `json:"inbounds"`
	Outbounds    []SBOutbound           `json:"outbounds"`
	Route        *SBRoute               `json:"route,omitempty"`
	Experimental map[string]interface{} `json:"experimental,omitempty"`
}

func (r *SingBoxRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	obs := make([]SBOutbound, 0, len(nodes)+10)
	tags := make([]string, 0, len(nodes))

	for _, n := range nodes {
		ob, err := r.outboundFromNode(n)
		if err != nil {
			continue
		}
		obs = append(obs, *ob)
		tags = append(tags, ob.Tag)
	}

	selectorOut := SBOutbound{
		Type:      "selector",
		Tag:       "🚀 节点选择",
		Outbounds: append([]string{"♻️ 自动选择", "direct", "block"}, tags...),
	}
	autoOut := SBOutbound{
		Type:                "urltest",
		Tag:                 "♻️ 自动选择",
		Outbounds:           tags,
		URL:                 "http://www.gstatic.com/generate_204",
		Interval:            "5m",
		Tolerance:           50,
		InterruptExistConns: true,
	}
	directOut := SBOutbound{Type: "direct", Tag: "direct"}
	blockOut := SBOutbound{Type: "block", Tag: "block"}
	dnsOut := SBOutbound{Type: "dns", Tag: "dns-out"}

	groups := []SBOutbound{selectorOut, autoOut}
	groups = append(groups,
		SBOutbound{Type: "selector", Tag: "🐟 漏网之鱼", Outbounds: []string{"🚀 节点选择", "direct"}},
		SBOutbound{Type: "selector", Tag: "📈 网络测试", Outbounds: append([]string{"♻️ 自动选择", "direct"}, tags...)},
		SBOutbound{Type: "selector", Tag: "🐋 广告拦截", Outbounds: []string{"block", "direct"}},
	)

	finalOutbounds := make([]SBOutbound, 0)
	finalOutbounds = append(finalOutbounds, groups...)
	finalOutbounds = append(finalOutbounds, obs...)
	finalOutbounds = append(finalOutbounds, directOut, blockOut, dnsOut)

	dnsServers := []interface{}{
		map[string]interface{}{
			"tag":     "dns_proxy",
			"address": "https://1.1.1.1/dns-query",
			"detour":  "🚀 节点选择",
		},
		map[string]interface{}{
			"tag":     "dns_direct",
			"address": "https://doh.pub/dns-query",
			"detour":  "direct",
		},
		map[string]interface{}{
			"tag":     "block",
			"address": "rcode://success",
		},
		map[string]interface{}{
			"tag":     "local",
			"address": "223.5.5.5",
			"detour":  "direct",
		},
	}
	dnsRules := []interface{}{
		map[string]interface{}{
			"server":        "local",
			"domain_suffix": []string{".cn"},
		},
		map[string]interface{}{
			"server":   "dns_direct",
			"rule_set": "geosite-cn",
		},
		map[string]interface{}{
			"server":   "block",
			"rule_set": "geosite-category-ads-all",
		},
	}
	routeRules := []interface{}{
		map[string]interface{}{
			"protocol": "dns",
			"outbound": "dns-out",
		},
		map[string]interface{}{
			"ip_is_private": true,
			"outbound":      "direct",
		},
		map[string]interface{}{
			"rule_set": "geosite-category-ads-all",
			"outbound": "🐋 广告拦截",
		},
		map[string]interface{}{
			"rule_set": []string{"geosite-cn", "geoip-cn"},
			"outbound": "direct",
		},
		map[string]interface{}{
			"match":    true,
			"outbound": "🐟 漏网之鱼",
		},
	}

	cfg := SBConfig{
		Log: map[string]interface{}{"level": "info"},
		DNS: &SBDNS{
			Servers: dnsServers,
			Rules:   dnsRules,
			Final:   "dns_proxy",
		},
		Inbounds: []SBInbound{
			{Type: "mixed", Tag: "mixed-in", ListenPort: 7890, Sniff: true, SniffOverrideDestination: true, DomainStrategy: "prefer_ipv4"},
			{Type: "tun", Tag: "tun-in", Address: []string{"172.19.0.1/30", "fd00::1/126"}, AutoRoute: true, Stack: "mixed", StrictRoute: true, Sniff: true, SniffOverrideDestination: true, DomainStrategy: "prefer_ipv4"},
		},
		Outbounds: finalOutbounds,
		Route: &SBRoute{
			Rules:               routeRules,
			Final:               "🚀 节点选择",
			AutoDetectInterface: true,
		},
		Experimental: map[string]interface{}{
			"cache_file": map[string]interface{}{"enabled": true},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func (r *SingBoxRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	ob, err := r.outboundFromNode(n)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(ob)
	return string(b), err
}

func (r *SingBoxRenderer) outboundFromNode(n nodespec.NodeSpec) (*SBOutbound, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	// sing-box 不支持 XHTTP+REALITY 组合：
	// sing-box 将 XHTTP 降级为 httpupgrade，但 xray REALITY 只支持 RAW/XHTTP/gRPC
	// 此组合的节点应通过 Xray 内核客户端使用
	if n.Security == nodespec.SecurityReality && n.Transport.Type == nodespec.TransportXHTTP {
		return nil, fmt.Errorf("singbox: XHTTP+REALITY not supported (use Xray client instead)")
	}
	// sing-box 不支持 XHTTP split mode（download_settings）：
	// split mode 是 Xray 专有特性（上行 CDN + 下行 REALITY 直连），
	// sing-box 客户端无法处理 download_settings 字段
	if n.Transport.Type == nodespec.TransportXHTTP && n.Transport.XHTTP != nil && n.Transport.XHTTP.DownloadSettings != nil {
		return nil, fmt.Errorf("singbox: XHTTP split mode (download_settings) not supported (use Xray client instead)")
	}
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}
	ob := &SBOutbound{
		Tag:        n.Name,
		Server:     n.Address,
		ServerPort: port,
	}

	if n.AddressIPv6 != "" {
		ob.DomainStrategy = "prefer_ipv4"
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		ob.Type = "vless"
		ob.PacketEncoding = "xudp"
		if c, ok := n.Credentials.(nodespec.VLESSCredentials); ok {
			ob.UUID = c.UUID
			// Vision flow 仅适用于 raw TCP+TLS，WS/gRPC/HTTPUpgrade/XHTTP 等传输层不支持
		if c.Flow == nodespec.FlowXTLSRprxVision && n.Transport.Type == nodespec.TransportTCP {
				ob.Flow = string(c.Flow)
			}
		}
	case nodespec.ProtocolVMess:
		ob.Type = "vmess"
		ob.Security = "auto"
		if c, ok := n.Credentials.(nodespec.VMessCredentials); ok {
			ob.UUID = c.UUID
			ob.AlterID = c.AlterID
		}
	case nodespec.ProtocolTrojan:
		ob.Type = "trojan"
		if c, ok := n.Credentials.(nodespec.TrojanCredentials); ok {
			ob.Password = c.Password
		}
	case nodespec.ProtocolShadowsocks:
		ob.Type = "shadowsocks"
		if c, ok := n.Credentials.(nodespec.ShadowsocksCredentials); ok {
			ob.Password = c.Password
			ob.Method = c.Method
		}
		// sing-box shadowsocks outbound 不支持 tls 字段，跳过 TLS 设置
		// SS2022+TLS 场景应由传输层（如 WS+TLS）处理，而非 outbound 级 TLS
	case nodespec.ProtocolHysteria2:
		ob.Type = "hysteria2"
		if c, ok := n.Credentials.(nodespec.Hysteria2Credentials); ok {
			ob.Password = c.Password
		}
		ob.UpMbps = 0
		ob.DownMbps = 0
	case nodespec.ProtocolTUIC:
		ob.Type = "tuic"
		ob.CongestionControl = "cubic"
		ob.UdpRelayMode = "native"
		ob.ZeroRTTHandshake = true
		ob.Heartbeat = "10s"
		if c, ok := n.Credentials.(nodespec.TUICCredentials); ok {
			ob.UUID = c.UUID
			ob.Password = c.Password
		}
	case nodespec.ProtocolAnyTLS:
		ob.Type = "anytls"
		if c, ok := n.Credentials.(nodespec.AnyTLSCredentials); ok {
			ob.Password = c.Password
		}
	default:
		return nil, fmt.Errorf("singbox: unsupported protocol %s", n.Protocol)
	}

	if n.Transport.Type != nodespec.TransportTCP {
		tr := &SBTransport{}
		switch n.Transport.Type {
		case nodespec.TransportWS:
			tr.Type = "ws"
			if n.Transport.WS != nil {
				tr.Path = n.Transport.WS.Path
				if n.Transport.WS.Host != "" {
					tr.Headers = map[string]string{"Host": n.Transport.WS.Host}
				}
			}
		case nodespec.TransportGRPC:
			tr.Type = "grpc"
			if n.Transport.GRPC != nil {
				tr.ServiceName = n.Transport.GRPC.ServiceName
			}
		case nodespec.TransportHTTP2:
			tr.Type = "http"
			if n.Transport.HTTP2 != nil {
				tr.Path = n.Transport.HTTP2.Path
				if n.Transport.HTTP2.Host != "" {
					tr.Host = []string{n.Transport.HTTP2.Host}
				}
			}
		case nodespec.TransportXHTTP:
			// 注意：sing-box 不支持 download_settings 字段（split mode 是 Xray 专有特性），
			// 这里只渲染基础 XHTTP 字段，不要"顺手补全" download_settings，
			// 否则会导致 sing-box 客户端解析报错。
			// 有 downloadSettings 的节点应通过 kernelrender/singbox.go 返回 UnsupportedFeatureError，
			// 引导用户使用 Xray 内核。
			tr.Type = "httpupgrade"
			if n.Transport.XHTTP != nil {
				tr.Path = n.Transport.XHTTP.Path
				if n.Transport.XHTTP.Host != "" {
					tr.Host = n.Transport.XHTTP.Host
				}
			}
			ob.Flow = ""
		case nodespec.TransportHTTPUpgrade:
			tr.Type = "httpupgrade"
			if n.Transport.HTTPUpgrade != nil {
				tr.Path = n.Transport.HTTPUpgrade.Path
				if n.Transport.HTTPUpgrade.Host != "" {
					tr.Host = n.Transport.HTTPUpgrade.Host
				}
			}
		}
		ob.Transport = tr
	}

	// Shadowsocks 协议跳过 outbound 级 TLS（sing-box 1.13 shadowsocks 无 tls 字段）
	if n.Protocol == nodespec.ProtocolShadowsocks {
		// 不设置 ob.TLS，由 transport 层处理 TLS
	} else if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		ob.TLS = &SBTLS{
			Enabled:    true,
			ServerName: n.TLS.SNI,
			ALPN:       n.TLS.ALPN,
			Insecure:   n.TLS.AllowInsecure,
		}
		// TUIC 协议不支持 uTLS（sing-box 1.13 TUIC outbound 无 utls 字段）
		if n.TLS.Fingerprint != "" && n.Protocol != nodespec.ProtocolTUIC {
			ob.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: n.TLS.Fingerprint}
		}
	}
	// Shadowsocks 协议同样跳过 REALITY 的 outbound 级 TLS
	if n.Protocol != nodespec.ProtocolShadowsocks && n.Security == nodespec.SecurityReality && n.Reality != nil {
		ob.TLS = &SBTLS{
			Enabled:    true,
			ServerName: n.Reality.SNI,
			Reality: &SBReality{
				Enabled:   true,
				PublicKey: n.Reality.PublicKey,
				ShortID:   n.Reality.ShortID,
			},
		}
		if n.Reality.Fingerprint != "" {
			ob.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: n.Reality.Fingerprint}
		}
	}
	if n.Protocol == nodespec.ProtocolHysteria2 && n.Security == nodespec.SecurityTLS && n.TLS != nil {
		ob.TLS = &SBTLS{
			Enabled:    true,
			ServerName: n.TLS.SNI,
			Insecure:   n.TLS.AllowInsecure,
			ALPN:       n.TLS.ALPN,
		}
		if n.RawConfig != nil {
			if obfsType, ok := n.RawConfig["obfs"].(string); ok && obfsType != "" {
				ob.Obfs = &SBHysteriaObfs{Type: obfsType}
			}
			if obfsPass, ok := n.RawConfig["obfs_password"].(string); ok && obfsPass != "" {
				ob.ObfsPassword = obfsPass
			}
		}
	}
	if n.Protocol == nodespec.ProtocolAnyTLS && n.Security == nodespec.SecurityTLS && n.TLS != nil {
		ob.TLS = &SBTLS{
			Enabled:    true,
			ServerName: n.TLS.SNI,
			Insecure:   n.TLS.AllowInsecure,
			ALPN:       n.TLS.ALPN,
		}
		if n.TLS.Fingerprint != "" {
			ob.TLS.UTLS = &SBUTLS{Enabled: true, Fingerprint: n.TLS.Fingerprint}
		}
	}

	if n.Transport.Mux != nil && n.Transport.Mux.Enabled {
		m := n.Transport.Mux
		protocol := string(m.Protocol)
		if protocol == "" {
			protocol = "h2mux"
		}
		mux := &SBMux{
			Enabled:  true,
			Protocol: protocol,
			Padding:  m.Padding,
		}
		if m.MaxConnections > 0 {
			mux.MaxStreams = 0
		} else if m.MaxStreams > 0 {
			mux.MaxStreams = m.MaxStreams
		}
		ob.Multiplex = mux
	}

	return ob, nil
}
