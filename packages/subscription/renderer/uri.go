package renderer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

type Renderer interface {
	Name() string
	ContentType() string
	Render(nodes []nodespec.NodeSpec) ([]byte, error)
	RenderNode(n nodespec.NodeSpec) (string, error)
}

type SubscriptionMeta struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
	SubName  string
}

func SafeBase64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func SafeBase64Decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(s)
	}
	return string(b), err
}

func setIfNotEmpty(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

func addQuery(u *url.URL, k, v string) {
	if v != "" {
		q := u.Query()
		q.Set(k, v)
		u.RawQuery = q.Encode()
	}
}

// setPathOrSlash 设置 path 参数，空值回退到 "/" 避免 WS 握手失败。
// 与 node-service 的 chain_singbox.go 行为保持一致。
func setPathOrSlash(q url.Values, p string) {
	if p != "" {
		q.Set("path", p)
	} else {
		q.Set("path", "/")
	}
}

// isDirectIP 判断 address 是否为 IP 地址（直连节点）。
// Xray v26.2.4+ 已移除 allowInsecure 参数支持，自签名证书无 SAN 时
// TLS 握手会返回 alert 112 unrecognized_name。直连 IP 节点应改用
// pinnedPeerCertSha256 进行证书锁定；CDN 域名节点仍保留 allowInsecure。
func isDirectIP(address string) bool {
	return net.ParseIP(address) != nil
}

type URIRenderer struct{}

func NewURIRenderer() *URIRenderer { return &URIRenderer{} }

func (r *URIRenderer) Name() string        { return "uri" }
func (r *URIRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (r *URIRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	var lines []string
	for _, n := range nodes {
		line, err := r.RenderNode(n)
		if err != nil {
			continue
		}
		lines = append(lines, line)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (r *URIRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	if err := n.Validate(); err != nil {
		return "", err
	}
	addr := n.Address
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		return r.renderVLESS(n, addr, port), nil
	case nodespec.ProtocolVMess:
		return r.renderVMess(n, addr, port), nil
	case nodespec.ProtocolTrojan:
		return r.renderTrojan(n, addr, port), nil
	case nodespec.ProtocolShadowsocks:
		return r.renderSS(n, addr, port), nil
	case nodespec.ProtocolHysteria2:
		return r.renderHysteria2(n, addr, port), nil
	case nodespec.ProtocolTUIC:
		return r.renderTUIC(n, addr, port), nil
	case nodespec.ProtocolAnyTLS:
		return r.renderAnyTLS(n, addr, port), nil
	case nodespec.ProtocolSOCKS5:
		return r.renderSOCKS5(n, addr, port), nil
	case nodespec.ProtocolHTTP:
		return r.renderHTTP(n, addr, port), nil
	default:
		return "", fmt.Errorf("uri renderer does not support protocol: %s", n.Protocol)
	}
}

func (r *URIRenderer) renderVLESS(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.VLESSCredentials)
	uuid := creds.UUID
	flow := creds.Flow

	u := &url.URL{
		Scheme: "vless",
		User:   url.User(uuid),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()

	switch n.Transport.Type {
	case nodespec.TransportWS:
		q.Set("type", "ws")
		if n.Transport.WS != nil {
			setPathOrSlash(q, n.Transport.WS.Path)
			if n.Transport.WS.Host != "" {
				q.Set("host", n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		q.Set("type", "grpc")
		if n.Transport.GRPC != nil {
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
		}
	case nodespec.TransportXHTTP:
		q.Set("type", "xhttp")
		if n.Transport.XHTTP != nil {
			setPathOrSlash(q, n.Transport.XHTTP.Path)
			if n.Transport.XHTTP.Host != "" {
				q.Set("host", n.Transport.XHTTP.Host)
			}
			if n.Transport.XHTTP.Mode != "" {
				q.Set("mode", n.Transport.XHTTP.Mode)
			}
			// downloadSettings（split mode 上下行分离）：base64 编码的 JSON
			// 字段命名与 clash.go 已有实现保持一致
			if ds := n.Transport.XHTTP.DownloadSettings; ds != nil && ds.Address != "" {
				dsMap := map[string]interface{}{
					"address":     ds.Address,
					"port":        ds.Port,
					"network":     "xhttp",
					"security":    string(ds.Security),
					"path":        ds.Path,
					"host":        ds.Host,
					"mode":        ds.Mode,
					"addressIPv6": ds.AddressIPv6,
				}
				// REALITY 子配置（下行 REALITY 直连时必须）
				if ds.Security == nodespec.SecurityReality && ds.Reality != nil {
					dsMap["reality"] = map[string]interface{}{
						"publicKey":   ds.Reality.PublicKey,
						"shortId":     ds.Reality.ShortID,
						"serverName":  ds.Reality.SNI,
						"fingerprint": ds.Reality.Fingerprint,
					}
				}
				// TLS 子配置（下行 TLS CDN 时必须）
				if ds.Security == nodespec.SecurityTLS && ds.TLS != nil {
					dsMap["tls"] = map[string]interface{}{
						"serverName":  ds.TLS.SNI,
						"fingerprint": ds.TLS.Fingerprint,
					}
				}
				dsJSON, _ := json.Marshal(dsMap)
				q.Set("downloadSettings", base64.RawURLEncoding.EncodeToString(dsJSON))
			}
		// XMUX extra（XHTTP 专用多路复用，对应 Xray xhttpSettings.extra.xmux）
		if m := n.Transport.Mux; m != nil && m.Enabled && m.Protocol == nodespec.MuxProtocolXmux {
			extra := map[string]interface{}{}
			xmux := map[string]interface{}{}
			if m.MaxConcurrency != "" {
				xmux["maxConcurrency"] = m.MaxConcurrency
			}
			if m.MaxConnections > 0 {
				xmux["maxConnections"] = m.MaxConnections // B21 修复：maxConnection → maxConnections
			}
			if m.CMaxReuseTimes != "" {
				xmux["cMaxReuseTimes"] = m.CMaxReuseTimes
			}
			if m.HMaxRequestTimes != "" {
				xmux["hMaxRequestTimes"] = m.HMaxRequestTimes
			}
			if m.HMaxReusableSecs != "" {
				xmux["hMaxReusableSecs"] = m.HMaxReusableSecs
			}
			if len(xmux) > 0 {
				extra["xmux"] = xmux
				extraJSON, _ := json.Marshal(extra)
				q.Set("extra", base64.RawURLEncoding.EncodeToString(extraJSON))
			}
		}
		}
		flow = ""
	case nodespec.TransportHTTPUpgrade:
		q.Set("type", "httpupgrade")
		if n.Transport.HTTPUpgrade != nil {
			setPathOrSlash(q, n.Transport.HTTPUpgrade.Path)
			if n.Transport.HTTPUpgrade.Host != "" {
				q.Set("host", n.Transport.HTTPUpgrade.Host)
			}
		}
	case nodespec.TransportKCP:
		q.Set("type", "kcp")
		if n.Transport.KCP != nil {
			q.Set("seed", n.Transport.KCP.Seed)
			if n.Transport.KCP.HeaderType != "" {
				q.Set("headerType", n.Transport.KCP.HeaderType)
			}
		}
	case nodespec.TransportHTTP2:
		q.Set("type", "http")
		if n.Transport.HTTP2 != nil {
			setPathOrSlash(q, n.Transport.HTTP2.Path)
			if n.Transport.HTTP2.Host != "" {
				q.Set("host", n.Transport.HTTP2.Host)
			}
		}
	default:
		q.Set("type", "tcp")
	}

	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		if n.TLS.AllowInsecure {
			// Xray v26.2.4+ 已移除 allowInsecure 支持（2026.8.1 硬禁用）：
			// 有 pin_sha256 时优先用 pinnedPeerCertSha256 证书锁定（覆盖直连 IP 和域名连接两种场景，
			// 例如国内中转+SNI伪装的自签节点通过域名连接但需要证书指纹锁定）；
			// 无 pin_sha256 时回退 allowInsecure（仅 8.1 前可用，CDN 节点走此分支）。
			if n.TLS.PinSHA256 != "" {
				q.Set("pinnedPeerCertSha256", n.TLS.PinSHA256)
			} else {
				q.Set("allowInsecure", "1")
			}
		}
		if n.TLS.ECH != nil && n.TLS.ECH.PEM != "" {
			q.Set("ech", "1")
		}
	} else if n.Security == nodespec.SecurityReality && n.Reality != nil {
		q.Set("security", "reality")
		setIfNotEmpty(q, "sni", n.Reality.SNI)
		q.Set("pbk", n.Reality.PublicKey)
		q.Set("sid", n.Reality.ShortID)
		setIfNotEmpty(q, "fp", n.Reality.Fingerprint)
		setIfNotEmpty(q, "spx", n.Reality.SpiderX)
		if len(n.Reality.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.Reality.ALPN, ","))
		}
	} else {
		q.Set("security", "none")
	}

	if flow != "" && n.Transport.Type == nodespec.TransportTCP {
		q.Set("flow", string(flow))
	}
	// encryption=none 参数始终存在（对齐 Xboard buildVless）
	enc := creds.Encryption
	if enc == "" {
		enc = "none"
	}
	q.Set("encryption", enc)
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

func (r *URIRenderer) renderVMess(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.VMessCredentials)
	j := map[string]interface{}{
		"v":    "2",
		"ps":   n.Name,
		"add":  addr,
		"port": strconv.Itoa(port),
		"id":   creds.UUID,
		"aid":  strconv.Itoa(creds.AlterID),
		"scy":  "auto",
		"net":  string(n.Transport.Type),
		"type": "none",
	}
	switch n.Transport.Type {
	case nodespec.TransportWS:
		if n.Transport.WS != nil {
			if p := n.Transport.WS.Path; p != "" {
				j["path"] = p
			} else {
				j["path"] = "/"
			}
			j["host"] = n.Transport.WS.Host
		}
	}
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		j["tls"] = "tls"
		j["sni"] = n.TLS.SNI
		j["fp"] = n.TLS.Fingerprint
		if len(n.TLS.ALPN) > 0 {
			j["alpn"] = strings.Join(n.TLS.ALPN, ",")
		}
		if n.TLS.AllowInsecure {
			// 有 pin_sha256 时优先用 pinnedPeerCertSha256（覆盖 IP 和域名连接场景）；
			// 无 pin_sha256 时回退 allowInsecure（8.1 前可用）。
			if n.TLS.PinSHA256 != "" {
				j["pinnedPeerCertSha256"] = n.TLS.PinSHA256
			} else {
				j["allowInsecure"] = true
			}
		}
	} else {
		j["tls"] = ""
	}
	b, _ := json.Marshal(j)
	return "vmess://" + base64.StdEncoding.EncodeToString(b)
}

func (r *URIRenderer) renderTrojan(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.TrojanCredentials)
	u := &url.URL{
		Scheme: "trojan",
		User:   url.User(creds.Password),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	switch n.Transport.Type {
	case nodespec.TransportWS:
		q.Set("type", "ws")
		if n.Transport.WS != nil {
			setPathOrSlash(q, n.Transport.WS.Path)
			if n.Transport.WS.Host != "" {
				q.Set("host", n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		q.Set("type", "grpc")
		if n.Transport.GRPC != nil {
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
		}
	case nodespec.TransportXHTTP:
		q.Set("type", "xhttp")
		if n.Transport.XHTTP != nil {
			setPathOrSlash(q, n.Transport.XHTTP.Path)
			if n.Transport.XHTTP.Host != "" {
				q.Set("host", n.Transport.XHTTP.Host)
			}
			if n.Transport.XHTTP.Mode != "" {
				q.Set("mode", n.Transport.XHTTP.Mode)
			}
			// downloadSettings（split mode 上下行分离）：base64 编码的 JSON
			if ds := n.Transport.XHTTP.DownloadSettings; ds != nil && ds.Address != "" {
				dsMap := map[string]interface{}{
					"address":     ds.Address,
					"port":        ds.Port,
					"network":     "xhttp",
					"security":    string(ds.Security),
					"path":        ds.Path,
					"host":        ds.Host,
					"mode":        ds.Mode,
					"addressIPv6": ds.AddressIPv6,
				}
				// REALITY 子配置（下行 REALITY 直连时必须）
				if ds.Security == nodespec.SecurityReality && ds.Reality != nil {
					dsMap["reality"] = map[string]interface{}{
						"publicKey":   ds.Reality.PublicKey,
						"shortId":     ds.Reality.ShortID,
						"serverName":  ds.Reality.SNI,
						"fingerprint": ds.Reality.Fingerprint,
					}
				}
				// TLS 子配置（下行 TLS CDN 时必须）
				if ds.Security == nodespec.SecurityTLS && ds.TLS != nil {
					dsMap["tls"] = map[string]interface{}{
						"serverName":  ds.TLS.SNI,
						"fingerprint": ds.TLS.Fingerprint,
					}
				}
				dsJSON, _ := json.Marshal(dsMap)
				q.Set("downloadSettings", base64.RawURLEncoding.EncodeToString(dsJSON))
			}
		// XMUX extra（XHTTP 专用多路复用，对应 Xray xhttpSettings.extra.xmux）
		if m := n.Transport.Mux; m != nil && m.Enabled && m.Protocol == nodespec.MuxProtocolXmux {
			extra := map[string]interface{}{}
			xmux := map[string]interface{}{}
			if m.MaxConcurrency != "" {
				xmux["maxConcurrency"] = m.MaxConcurrency
			}
			if m.MaxConnections > 0 {
				xmux["maxConnections"] = m.MaxConnections // B21 修复：maxConnection → maxConnections
			}
			if m.CMaxReuseTimes != "" {
				xmux["cMaxReuseTimes"] = m.CMaxReuseTimes
			}
			if m.HMaxRequestTimes != "" {
				xmux["hMaxRequestTimes"] = m.HMaxRequestTimes
			}
			if m.HMaxReusableSecs != "" {
				xmux["hMaxReusableSecs"] = m.HMaxReusableSecs
			}
			if len(xmux) > 0 {
				extra["xmux"] = xmux
				extraJSON, _ := json.Marshal(extra)
				q.Set("extra", base64.RawURLEncoding.EncodeToString(extraJSON))
			}
		}
		}
	case nodespec.TransportHTTPUpgrade:
		q.Set("type", "httpupgrade")
		if n.Transport.HTTPUpgrade != nil {
			setPathOrSlash(q, n.Transport.HTTPUpgrade.Path)
			if n.Transport.HTTPUpgrade.Host != "" {
				q.Set("host", n.Transport.HTTPUpgrade.Host)
			}
		}
	default:
		q.Set("type", "tcp")
	}
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		if n.TLS.AllowInsecure {
			// 有 pin_sha256 时优先用 pinnedPeerCertSha256（覆盖 IP 和域名连接场景）；
			// 无 pin_sha256 时回退 allowInsecure（8.1 前可用）。
			if n.TLS.PinSHA256 != "" {
				q.Set("pinnedPeerCertSha256", n.TLS.PinSHA256)
			} else {
				q.Set("allowInsecure", "1")
			}
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

func (r *URIRenderer) renderSS(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.ShadowsocksCredentials)
	userinfo := base64.StdEncoding.EncodeToString([]byte(creds.Method + ":" + creds.Password))
	u := &url.URL{
		Scheme: "ss",
		User:   url.User(userinfo),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	switch n.Transport.Type {
	case nodespec.TransportWS:
		q.Set("type", "ws")
		if n.Transport.WS != nil {
			setPathOrSlash(q, n.Transport.WS.Path)
			if n.Transport.WS.Host != "" {
				q.Set("host", n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		q.Set("type", "grpc")
		if n.Transport.GRPC != nil {
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
		}
	default:
		q.Set("type", "tcp")
	}
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		if n.TLS.AllowInsecure {
			q.Set("allowInsecure", "1")
		}
	} else if n.Security == nodespec.SecurityReality && n.Reality != nil {
		q.Set("security", "reality")
		setIfNotEmpty(q, "sni", n.Reality.SNI)
		q.Set("pbk", n.Reality.PublicKey)
		q.Set("sid", n.Reality.ShortID)
		setIfNotEmpty(q, "fp", n.Reality.Fingerprint)
		setIfNotEmpty(q, "spx", n.Reality.SpiderX)
		if len(n.Reality.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.Reality.ALPN, ","))
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

func (r *URIRenderer) renderHysteria2(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.Hysteria2Credentials)
	u := &url.URL{
		Scheme: "hysteria2",
		User:   url.User(creds.Password),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		// insecure 参数始终存在（对齐 Xboard buildHysteria）
		if n.TLS.AllowInsecure {
			q.Set("insecure", "1")
		} else {
			q.Set("insecure", "0")
		}
		// pinSHA256 证书指纹（证书锁定）
		setIfNotEmpty(q, "pinSHA256", n.TLS.PinSHA256)
	}
	if n.Transport.QUIC != nil {
		if n.Transport.QUIC.Security != "" {
			q.Set("obfs", n.Transport.QUIC.Security)
		}
		if n.Transport.QUIC.Key != "" {
			q.Set("obfs-password", n.Transport.QUIC.Key)
		}
	}
	// mport 多端口范围（对齐 Xboard server.ports，端口跳跃/多端口）
	if n.Transport.PortHopping != nil && n.Transport.PortHopping.Enabled && n.Transport.PortHopping.PortRange != "" {
		q.Set("mport", n.Transport.PortHopping.PortRange)
	}
	if creds.UpMbps > 0 {
		q.Set("upmbps", strconv.Itoa(creds.UpMbps))
	}
	if creds.DownMbps > 0 {
		q.Set("downmbps", strconv.Itoa(creds.DownMbps))
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

func (r *URIRenderer) renderTUIC(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.TUICCredentials)
	user := creds.UUID
	if creds.Password != "" {
		user = user + ":" + creds.Password
	}
	u := &url.URL{
		Scheme: "tuic",
		User:   url.User(user),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	q.Set("congestion_control", "bbr")
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		if n.TLS.AllowInsecure {
			q.Set("allow_insecure", "1")
		}
	}
	// udp_relay_mode 始终输出 native（对齐 Xboard buildTuic 默认值）
	q.Set("udp_relay_mode", "native")
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

func (r *URIRenderer) renderAnyTLS(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.AnyTLSCredentials)
	u := &url.URL{
		Scheme: "anytls",
		User:   url.User(creds.Password),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	q.Set("type", "tcp")
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		setIfNotEmpty(q, "fp", n.TLS.Fingerprint)
		if len(n.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(n.TLS.ALPN, ","))
		}
		// insecure 参数始终存在（对齐 Xboard buildAnyTLS）
		if n.TLS.AllowInsecure {
			q.Set("insecure", "1")
		} else {
			q.Set("insecure", "0")
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

// renderSOCKS5 生成 SOCKS5 代理 URI。
// 格式：socks://username:password@host:port#name
// 对齐 Xboard General::buildSocks，TLS 场景附加 security/sni/insecure 参数。
func (r *URIRenderer) renderSOCKS5(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.SOCKS5Credentials)
	username := creds.Username
	if username == "" {
		username = creds.Password
	}
	u := &url.URL{
		Scheme: "socks",
		User:   url.UserPassword(username, creds.Password),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		if n.TLS.AllowInsecure {
			q.Set("insecure", "1")
		} else {
			q.Set("insecure", "0")
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}

// renderHTTP 生成 HTTP/HTTPS 代理 URI。
// 格式：http://username:password@host:port#name
// 对齐 Xboard General::buildHttp，TLS 场景附加 security/sni/insecure 参数。
func (r *URIRenderer) renderHTTP(n nodespec.NodeSpec, addr string, port int) string {
	creds, _ := n.Credentials.(nodespec.HTTPCredentials)
	username := creds.Username
	if username == "" {
		username = creds.Password
	}
	scheme := "http"
	if n.Security == nodespec.SecurityTLS {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		User:   url.UserPassword(username, creds.Password),
		Host:   net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	q := u.Query()
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		setIfNotEmpty(q, "sni", n.TLS.SNI)
		if n.TLS.AllowInsecure {
			q.Set("insecure", "1")
		} else {
			q.Set("insecure", "0")
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = n.Name
	return u.String()
}
