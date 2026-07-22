package renderer

import (
	"fmt"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
	"gopkg.in/yaml.v3"
)

type ClashRenderer struct {
	isMeta bool
}

func NewClashRenderer() *ClashRenderer { return &ClashRenderer{isMeta: false} }
func NewClashMetaRenderer() *ClashRenderer { return &ClashRenderer{isMeta: true} }

func (r *ClashRenderer) Name() string {
	if r.isMeta {
		return "clashmeta"
	}
	return "clash"
}
func (r *ClashRenderer) ContentType() string { return "text/yaml; charset=utf-8" }

type ClashHysteria2Opts struct {
	Password     string `yaml:"password"`
	Obfs         string `yaml:"obfs,omitempty"`
	ObfsPassword string `yaml:"obfs-password,omitempty"`
}

type ClashTUIC struct {
	Name                string   `yaml:"name"`
	Type                string   `yaml:"type"`
	Server              string   `yaml:"server"`
	Port                int      `yaml:"port"`
	UUID                string   `yaml:"uuid"`
	Password            string   `yaml:"password,omitempty"`
	ALPN                []string `yaml:"alpn,omitempty"`
	Servername          string   `yaml:"sni,omitempty"`
	SkipCertVerify      bool     `yaml:"skip-cert-verify"`
	UDP                 bool     `yaml:"udp,omitempty"`
	CongestionController string  `yaml:"congestion-controller,omitempty"`
	UdpRelayMode        string   `yaml:"udp-relay-mode,omitempty"`
}

type ClashXHTTPOpts struct {
	Host string   `yaml:"host,omitempty"`
	Path string   `yaml:"path,omitempty"`
	Mode string   `yaml:"mode,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	DownloadSettings map[string]interface{} `yaml:"extra,omitempty"`
}

type ClashSmuxOpts struct {
	Enabled        bool   `yaml:"enabled"`
	Protocol       string `yaml:"protocol,omitempty"`
	MaxConnections int    `yaml:"max-connections,omitempty"`
	MaxStreams     int    `yaml:"max-streams,omitempty"`
	Padding        bool   `yaml:"padding,omitempty"`
}

type ClashProxy struct {
	Name            string         `yaml:"name"`
	Type            string         `yaml:"type"`
	Server          string         `yaml:"server"`
	Port            int            `yaml:"port"`
	UUID            string         `yaml:"uuid,omitempty"`
	Password        string         `yaml:"password,omitempty"`
	Cipher          string         `yaml:"cipher,omitempty"`
	AlterID         int            `yaml:"alterId,omitempty"`
	Network         string         `yaml:"network,omitempty"`
	TLS             bool           `yaml:"tls,omitempty"`
	Servername      string         `yaml:"servername,omitempty"`
	SNI             string         `yaml:"sni,omitempty"`
	ALPN            []string       `yaml:"alpn,omitempty"`
	Fingerprint     string         `yaml:"client-fingerprint,omitempty"`
	SkipCertVerify  bool           `yaml:"skip-cert-verify"`
	WSOpts          *ClashWS       `yaml:"ws-opts,omitempty"`
	GRPCOpts        *ClashGRPC     `yaml:"grpc-opts,omitempty"`
	HTTPOpts        *ClashHTTP     `yaml:"http-opts,omitempty"`
	H2Opts          *ClashH2       `yaml:"h2-opts,omitempty"`
	HTTPUpgradeOpts *ClashWSOpts   `yaml:"httpupgrade-opts,omitempty"`
	XHTTPOpts       *ClashXHTTPOpts `yaml:"xhttp-opts,omitempty"`
	SmuxOpts        *ClashSmuxOpts `yaml:"smux,omitempty"`
	Flow            string         `yaml:"flow,omitempty"`
	UDP             bool           `yaml:"udp,omitempty"`
	RealityOpts     *ClashReality  `yaml:"reality-opts,omitempty"`
	ObfsPassword    string         `yaml:"obfs-password,omitempty"`
	Obfs            string         `yaml:"obfs,omitempty"`
	Up              string         `yaml:"up,omitempty"`
	Down            string         `yaml:"down,omitempty"`
	DisableMtuDiscovery bool      `yaml:"disable-mtu-discovery,omitempty"`
	FastOpen        bool           `yaml:"fast-open,omitempty"`
	CongestionController string   `yaml:"congestion-controller,omitempty"`
	UdpRelayMode    string         `yaml:"udp-relay-mode,omitempty"`
	ZeroRttHandshake bool         `yaml:"zero-rtt-handshake,omitempty"`
	HeartbeatInterval string      `yaml:"heartbeat-interval,omitempty"`
	Plugin          string         `yaml:"plugin,omitempty"`
	PluginOpts      interface{}    `yaml:"plugin-opts,omitempty"`
	IPVersion       string         `yaml:"ip-version,omitempty"`
}

type ClashWS struct {
	Path             string            `yaml:"path,omitempty"`
	Headers          map[string]string `yaml:"headers,omitempty"`
	EarlyDataHeader  string            `yaml:"early-data-header-name,omitempty"`
	MaxEarlyData     int               `yaml:"max-early-data,omitempty"`
	V2rayHttpUpgrade bool              `yaml:"v2ray-http-upgrade,omitempty"`
}

type ClashWSOpts struct {
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Host    string            `yaml:"host,omitempty"`
}

type ClashGRPC struct {
	ServiceName string `yaml:"grpc-service-name,omitempty"`
}

type ClashHTTP struct {
	Method  string              `yaml:"method,omitempty"`
	Path    []string            `yaml:"path,omitempty"`
	Headers map[string][]string `yaml:"headers,omitempty"`
}

type ClashH2 struct {
	Host []string `yaml:"host,omitempty"`
	Path string   `yaml:"path,omitempty"`
}

type ClashReality struct {
	PublicKey string `yaml:"public-key,omitempty"`
	ShortID   string `yaml:"short-id,omitempty"`
}

type ClashDNSConfig struct {
	Enable            bool              `yaml:"enable"`
	IPv6              bool              `yaml:"ipv6"`
	Listen            string            `yaml:"listen"`
	UseHosts          bool              `yaml:"use-hosts"`
	DefaultNameserver []string          `yaml:"default-nameserver"`
	Nameserver        []string          `yaml:"nameserver"`
	Fallback          []string          `yaml:"fallback,omitempty"`
	FallbackFilter    *ClashFallbackFilter `yaml:"fallback-filter,omitempty"`
	NameserverPolicy  map[string]string `yaml:"nameserver-policy,omitempty"`
}

type ClashFallbackFilter struct {
	GeoIP  bool     `yaml:"geoip"`
	IPCIDR []string `yaml:"ipcidr,omitempty"`
	GeoCode []string `yaml:"geocode,omitempty"`
}

type ClashTun struct {
	Enable              bool     `yaml:"enable"`
	Stack               string   `yaml:"stack"`
	DNSHijack           []string `yaml:"dns-hijack"`
	AutoRoute           bool     `yaml:"auto-route"`
	AutoDetectInterface bool     `yaml:"auto-detect-interface"`
}

type ClashConfig struct {
	Port                      int                    `yaml:"port,omitempty"`
	SocksPort                 int                    `yaml:"socks-port,omitempty"`
	RedirPort                 int                    `yaml:"redir-port,omitempty"`
	TProxyPort                int                    `yaml:"tproxy-port,omitempty"`
	MixedPort                 int                    `yaml:"mixed-port,omitempty"`
	AllowLan                  bool                   `yaml:"allow-lan"`
	Mode                      string                 `yaml:"mode"`
	UnifiedDelay              bool                   `yaml:"unified-delay"`
	LogLevel                  string                 `yaml:"log-level"`
	ExternalController        string                 `yaml:"external-controller,omitempty"`
	ExternalUI                string                 `yaml:"external-ui,omitempty"`
	Secret                    string                 `yaml:"secret,omitempty"`
	Ipv6                      bool                   `yaml:"ipv6"`
	TCPConcurrent             bool                   `yaml:"tcp-concurrent"`
	FindProcessMode           string                 `yaml:"find-process-mode,omitempty"`
	GlobalClientFingerprint   string                 `yaml:"global-client-fingerprint,omitempty"`
	GeoXUrl                   map[string]string      `yaml:"geox-url,omitempty"`
	DNS                       *ClashDNSConfig        `yaml:"dns,omitempty"`
	Tun                       *ClashTun              `yaml:"tun,omitempty"`
	Proxies                   []ClashProxy           `yaml:"proxies"`
	ProxyGroups               []map[string]interface{} `yaml:"proxy-groups"`
	Rules                     []string               `yaml:"rules"`
	RuleProviders             map[string]interface{} `yaml:"rule-providers,omitempty"`
	ProxyProviders            map[string]interface{} `yaml:"proxy-providers,omitempty"`
	Profile                   map[string]interface{} `yaml:"profile,omitempty"`
}

func hasUSName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "us") || strings.Contains(n, "美国") || strings.Contains(n, "美") || strings.Contains(n, "usa")
}

func hasHKName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "hk") || strings.Contains(n, "香港") || strings.Contains(n, "港")
}

func hasJPName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "jp") || strings.Contains(n, "日本") || strings.Contains(n, "日")
}

func hasSGName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "sg") || strings.Contains(n, "新加坡") || strings.Contains(n, "狮")
}

func hasTWName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "tw") || strings.Contains(n, "台湾") || strings.Contains(n, "台")
}

func hasKRName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "kr") || strings.Contains(n, "韩国") || strings.Contains(n, "韩") || strings.Contains(n, "korea")
}

func filterNames(names []string, fn func(string) bool) []string {
	var result []string
	for _, n := range names {
		if fn(n) {
			result = append(result, n)
		}
	}
	return result
}

func (r *ClashRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	proxies := make([]ClashProxy, 0, len(nodes))
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		p, err := r.proxyFromNode(n)
		if err != nil {
			continue
		}
		proxies = append(proxies, *p)
		names = append(names, n.Name)
	}

	usNodes := filterNames(names, hasUSName)
	hkNodes := filterNames(names, hasHKName)
	jpNodes := filterNames(names, hasJPName)
	sgNodes := filterNames(names, hasSGName)
	twNodes := filterNames(names, hasTWName)
	krNodes := filterNames(names, hasKRName)

	makeURLTestGroup := func(name string, nodes []string, fallback bool) map[string]interface{} {
		g := map[string]interface{}{
			"name":     name,
			"type":     "url-test",
			"url":      "http://www.gstatic.com/generate_204",
			"interval": 300,
			"tolerance": 50,
		}
		proxiesList := nodes
		if fallback && len(proxiesList) == 0 {
			proxiesList = names
		}
		g["proxies"] = proxiesList
		return g
	}

	groups := []map[string]interface{}{
		{
			"name":         "🚀 节点选择",
			"type":         "select",
			"proxies":      append([]string{"♻️ 自动选择", "DIRECT", "🇭🇰 香港节点", "🇯🇵 日本节点", "🇺🇸 美国节点", "🇸🇬 新加坡节点", "🇹🇼 台湾节点"}, names...),
		},
		{
			"name":         "♻️ 自动选择",
			"type":         "url-test",
			"proxies":      names,
			"url":          "http://www.gstatic.com/generate_204",
			"interval":     300,
			"tolerance":    50,
		},
		{
			"name":         "🐟 漏网之鱼",
			"type":         "select",
			"proxies":      append([]string{"🚀 节点选择", "DIRECT"}, names...),
		},
		{
			"name":         "📈 网络测试",
			"type":         "select",
			"proxies":      append([]string{"♻️ 自动选择", "DIRECT"}, names...),
		},
		{
			"name":         "🌍 全球直连",
			"type":         "select",
			"proxies":      []string{"DIRECT", "🚀 节点选择"},
		},
		{
			"name":         "🛑 全球拦截",
			"type":         "select",
			"proxies":      []string{"REJECT", "DIRECT"},
		},
		{
			"name":         "🐋 广告拦截",
			"type":         "select",
			"proxies":      []string{"REJECT", "DIRECT"},
		},
	}

	if len(hkNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇭🇰 香港节点", hkNodes, false))
	}
	if len(jpNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇯🇵 日本节点", jpNodes, false))
	}
	if len(usNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇺🇸 美国节点", usNodes, false))
	}
	if len(sgNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇸🇬 新加坡节点", sgNodes, false))
	}
	if len(twNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇹🇼 台湾节点", twNodes, false))
	}
	if len(krNodes) > 0 {
		groups = append(groups, makeURLTestGroup("🇰🇷 韩国节点", krNodes, false))
	}

	var rules []string
	rules = append(rules,
		"DOMAIN-SUFFIX,local,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT",
		"IP-CIDR,172.16.0.0/12,DIRECT",
		"IP-CIDR,192.168.0.0/16,DIRECT",
		"IP-CIDR,10.0.0.0/8,DIRECT",
		"IP-CIDR,100.64.0.0/10,DIRECT",
		"DOMAIN-SUFFIX,cn,DIRECT",
		"GEOIP,CN,DIRECT",
		"MATCH,🐟 漏网之鱼",
	)

	cfg := ClashConfig{
		MixedPort:               7890,
		AllowLan:                true,
		Mode:                    "rule",
		UnifiedDelay:            true,
		LogLevel:                "info",
		ExternalController:      "127.0.0.1:9090",
		Ipv6:                    true,
		TCPConcurrent:           true,
		FindProcessMode:         "strict",
		GlobalClientFingerprint: "chrome",
		Proxies:                 proxies,
		ProxyGroups:             groups,
		Rules:                   rules,
	}

	if r.isMeta {
		cfg.DNS = &ClashDNSConfig{
			Enable:           true,
			IPv6:             true,
			Listen:           "0.0.0.0:1053",
			UseHosts:         true,
			DefaultNameserver: []string{
				"223.5.5.5",
				"119.29.29.29",
			},
			Nameserver: []string{
				"https://doh.pub/dns-query",
				"https://dns.alidns.com/dns-query",
			},
			Fallback: []string{
				"https://1.1.1.1/dns-query",
				"https://8.8.8.8/dns-query",
			},
			FallbackFilter: &ClashFallbackFilter{
				GeoIP:  true,
				IPCIDR: []string{"240.0.0.0/4"},
				GeoCode: []string{"CN"},
			},
		}
		cfg.GeoXUrl = map[string]string{
			"geoip":   "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat",
			"geosite": "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
			"mmdb":    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
		}
	}

	return yaml.Marshal(cfg)
}

func (r *ClashRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	p, err := r.proxyFromNode(n)
	if err != nil {
		return "", err
	}
	b, err := yaml.Marshal(p)
	return string(b), err
}

func (r *ClashRenderer) proxyFromNode(n nodespec.NodeSpec) (*ClashProxy, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}
	p := &ClashProxy{
		Name:   n.Name,
		Server: n.Address,
		Port:   port,
		UDP:    n.AllowUDP,
	}

	if n.AddressIPv6 != "" {
		p.IPVersion = "dual"
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		p.Type = "vless"
		if c, ok := n.Credentials.(nodespec.VLESSCredentials); ok {
			p.UUID = c.UUID
			if c.Flow == nodespec.FlowXTLSRprxVision && n.Transport.Type != nodespec.TransportXHTTP {
				p.Flow = string(c.Flow)
			}
		}
	case nodespec.ProtocolVMess:
		p.Type = "vmess"
		if c, ok := n.Credentials.(nodespec.VMessCredentials); ok {
			p.UUID = c.UUID
			p.AlterID = c.AlterID
			p.Cipher = "auto"
		}
	case nodespec.ProtocolTrojan:
		p.Type = "trojan"
		if c, ok := n.Credentials.(nodespec.TrojanCredentials); ok {
			p.Password = c.Password
		}
	case nodespec.ProtocolShadowsocks:
		p.Type = "ss"
		if c, ok := n.Credentials.(nodespec.ShadowsocksCredentials); ok {
			p.Password = c.Password
			p.Cipher = c.Method
		}
	case nodespec.ProtocolHysteria2:
		p.Type = "hysteria2"
		if c, ok := n.Credentials.(nodespec.Hysteria2Credentials); ok {
			p.Password = c.Password
		}
		p.Up = "100"
		p.Down = "200"
		p.FastOpen = true
	case nodespec.ProtocolTUIC:
		p.Type = "tuic"
		if c, ok := n.Credentials.(nodespec.TUICCredentials); ok {
			p.UUID = c.UUID
			p.Password = c.Password
		}
		p.UDP = true
		p.CongestionController = "cubic"
		p.UdpRelayMode = "native"
		p.ZeroRttHandshake = true
	case nodespec.ProtocolAnyTLS:
		if !r.isMeta {
			return nil, fmt.Errorf("clash: anytls requires Clash.Meta")
		}
		p.Type = "anytls"
		if c, ok := n.Credentials.(nodespec.AnyTLSCredentials); ok {
			p.Password = c.Password
		}
	default:
		return nil, fmt.Errorf("clash: unsupported protocol %s", n.Protocol)
	}

	if n.Transport.Type != nodespec.TransportTCP {
		switch n.Transport.Type {
		case nodespec.TransportWS:
			p.Network = "ws"
			if n.Transport.WS != nil {
				p.WSOpts = &ClashWS{Path: n.Transport.WS.Path}
				if n.Transport.WS.Host != "" {
					p.WSOpts.Headers = map[string]string{"Host": n.Transport.WS.Host}
				}
			}
		case nodespec.TransportGRPC:
			p.Network = "grpc"
			if n.Transport.GRPC != nil {
				p.GRPCOpts = &ClashGRPC{ServiceName: n.Transport.GRPC.ServiceName}
			}
		case nodespec.TransportHTTP2:
			p.Network = "h2"
			if n.Transport.HTTP2 != nil {
				p.H2Opts = &ClashH2{Path: n.Transport.HTTP2.Path}
				if n.Transport.HTTP2.Host != "" {
					p.H2Opts.Host = []string{n.Transport.HTTP2.Host}
				}
			}
		case nodespec.TransportXHTTP:
			if r.isMeta {
				p.Network = "xhttp"
				if n.Transport.XHTTP != nil {
					xhttpOpts := &ClashXHTTPOpts{
						Path: n.Transport.XHTTP.Path,
						Host: n.Transport.XHTTP.Host,
						Mode: n.Transport.XHTTP.Mode,
					}
					if n.Transport.XHTTP.Host != "" {
						xhttpOpts.Headers = map[string]string{"Host": n.Transport.XHTTP.Host}
					}
					if ds := n.Transport.XHTTP.DownloadSettings; ds != nil && ds.Address != "" {
						download := map[string]interface{}{
							"address": ds.Address,
							"port":    ds.Port,
						}
						if ds.Port == 0 {
							download["port"] = 443
						}
						network := string(ds.Network)
						if network == "" {
							network = "xhttp"
						}
						download["network"] = network
						if ds.Path != "" || ds.Host != "" || ds.Mode != "" {
							xh := map[string]interface{}{}
							if ds.Path != "" {
								xh["path"] = ds.Path
							}
							if ds.Host != "" {
								xh["host"] = ds.Host
							}
							if ds.Mode != "" {
								xh["mode"] = ds.Mode
							}
							download["xhttpSettings"] = xh
						}
						security := string(ds.Security)
						if ds.Reality != nil {
							security = "reality"
							reality := map[string]interface{}{
								"publicKey":  ds.Reality.PublicKey,
								"shortId":    ds.Reality.ShortID,
								"serverName": ds.Reality.SNI,
							}
							if ds.Reality.Fingerprint != "" {
								reality["fingerprint"] = ds.Reality.Fingerprint
							}
							download["realitySettings"] = reality
						} else if ds.TLS != nil {
							security = "tls"
							tls := map[string]interface{}{}
							if ds.TLS.SNI != "" {
								tls["serverName"] = ds.TLS.SNI
							}
							if len(tls) > 0 {
								download["tlsSettings"] = tls
							}
						}
						if security != "" {
							download["security"] = security
						}
						xhttpOpts.DownloadSettings = map[string]interface{}{
							"downloadSettings": download,
						}
					}
					p.XHTTPOpts = xhttpOpts
				}
				p.Flow = ""
			} else {
				p.Network = "ws"
				if n.Transport.XHTTP != nil {
					p.WSOpts = &ClashWS{Path: n.Transport.XHTTP.Path}
					if n.Transport.XHTTP.Host != "" {
						p.WSOpts.Headers = map[string]string{"Host": n.Transport.XHTTP.Host}
					}
				}
			}
		case nodespec.TransportHTTPUpgrade:
			if r.isMeta {
				// Clash Meta 原生支持 httpupgrade 网络类型（>=1.18.0）
				p.Network = "httpupgrade"
				if n.Transport.HTTPUpgrade != nil {
					p.HTTPUpgradeOpts = &ClashWSOpts{
						Path: n.Transport.HTTPUpgrade.Path,
					}
					if n.Transport.HTTPUpgrade.Host != "" {
						p.HTTPUpgradeOpts.Headers = map[string]string{"Host": n.Transport.HTTPUpgrade.Host}
					}
				}
			} else {
				// 原版 Clash 不支持 httpupgrade，回退为 ws + v2ray-http-upgrade
				p.Network = "ws"
				if n.Transport.HTTPUpgrade != nil {
					p.WSOpts = &ClashWS{
						Path: n.Transport.HTTPUpgrade.Path,
						V2rayHttpUpgrade: true,
					}
					if n.Transport.HTTPUpgrade.Host != "" {
						p.WSOpts.Headers = map[string]string{"Host": n.Transport.HTTPUpgrade.Host}
					}
				}
			}
		}
	}

	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		p.TLS = true
		p.SkipCertVerify = n.TLS.AllowInsecure
		if n.Protocol == nodespec.ProtocolTrojan || n.Protocol == nodespec.ProtocolHysteria2 || n.Protocol == nodespec.ProtocolTUIC {
			p.SNI = n.TLS.SNI
		} else {
			p.Servername = n.TLS.SNI
		}
		if len(n.TLS.ALPN) > 0 {
			p.ALPN = n.TLS.ALPN
		}
		if n.TLS.Fingerprint != "" {
			p.Fingerprint = n.TLS.Fingerprint
		}
	}
	if n.Security == nodespec.SecurityReality && n.Reality != nil {
		p.TLS = true
		p.Servername = n.Reality.SNI
		if n.Protocol == nodespec.ProtocolTrojan {
			p.SNI = n.Reality.SNI
		}
		p.SkipCertVerify = false
		p.RealityOpts = &ClashReality{
			PublicKey: n.Reality.PublicKey,
			ShortID:   n.Reality.ShortID,
		}
		if n.Reality.Fingerprint != "" {
			p.Fingerprint = n.Reality.Fingerprint
		}
	}

	if n.Protocol == nodespec.ProtocolHysteria2 && n.Security == nodespec.SecurityTLS && n.TLS != nil {
		p.SNI = n.TLS.SNI
		p.SkipCertVerify = n.TLS.AllowInsecure
		if n.RawConfig != nil {
			if obfsType, ok := n.RawConfig["obfs"].(string); ok && obfsType != "" {
				p.Obfs = obfsType
			}
			if obfsPass, ok := n.RawConfig["obfs_password"].(string); ok && obfsPass != "" {
				p.ObfsPassword = obfsPass
			}
		}
	}
	if n.Protocol == nodespec.ProtocolAnyTLS && n.Security == nodespec.SecurityTLS && n.TLS != nil {
		p.TLS = true
		p.SNI = n.TLS.SNI
		p.SkipCertVerify = n.TLS.AllowInsecure
		if len(n.TLS.ALPN) > 0 {
			p.ALPN = n.TLS.ALPN
		}
		if n.TLS.Fingerprint != "" {
			p.Fingerprint = n.TLS.Fingerprint
		}
	}

	if n.Transport.Mux != nil && n.Transport.Mux.Enabled && r.isMeta {
		smux := &ClashSmuxOpts{
			Enabled: true,
			Padding: n.Transport.Mux.Padding,
		}
		if n.Transport.Mux.Protocol != "" {
			smux.Protocol = string(n.Transport.Mux.Protocol)
		} else {
			smux.Protocol = "h2mux"
		}
		if n.Transport.Mux.MaxConnections > 0 {
			smux.MaxConnections = n.Transport.Mux.MaxConnections
		}
		if n.Transport.Mux.MaxStreams > 0 {
			smux.MaxStreams = n.Transport.Mux.MaxStreams
		}
		p.SmuxOpts = smux
	}

	return p, nil
}

var _ = strings.Join
