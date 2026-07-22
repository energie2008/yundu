package renderer

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

type ShadowrocketRenderer struct {
	uri *URIRenderer
}

func NewShadowrocketRenderer() *ShadowrocketRenderer {
	return &ShadowrocketRenderer{uri: NewURIRenderer()}
}
func (r *ShadowrocketRenderer) Name() string        { return "shadowrocket" }
func (r *ShadowrocketRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (r *ShadowrocketRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	var lines []string
	for _, n := range nodes {
		line, err := r.RenderNode(n)
		if err != nil {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []byte(""), nil
	}
	output := strings.Join(lines, "\n") + "\n"
	return normalizeLineEndings(output), nil
}

func (r *ShadowrocketRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	if err := n.Validate(); err != nil {
		return "", err
	}
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		return r.renderVLESSShadowrocket(n, port), nil
	case nodespec.ProtocolVMess:
		return r.renderVMessShadowrocket(n, port), nil
	case nodespec.ProtocolTrojan:
		return r.renderTrojanShadowrocket(n, port), nil
	case nodespec.ProtocolShadowsocks:
		return r.renderSSShadowrocket(n, port), nil
	case nodespec.ProtocolHysteria2:
		return r.renderHysteria2Shadowrocket(n, port), nil
	case nodespec.ProtocolTUIC:
		return r.uri.renderTUIC(n, n.Address, port), nil
	default:
		return "", fmt.Errorf("shadowrocket: unsupported %s", n.Protocol)
	}
}

func normalizeLineEndings(s string) []byte {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}

func StripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func (r *ShadowrocketRenderer) renderVLESSShadowrocket(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VLESSCredentials)
	uuid := creds.UUID
	flow := creds.Flow

	u := &url.URL{
		Scheme: "vless",
		User:   url.User(uuid),
		Host:   net.JoinHostPort(n.Address, strconv.Itoa(port)),
	}
	q := u.Query()
	q.Set("encryption", "none")

	switch n.Transport.Type {
	case nodespec.TransportWS:
		if n.Transport.WS != nil {
			q.Set("type", "ws")
			q.Set("path", cleanWSPath(n.Transport.WS.Path))
			if n.Transport.WS.Host != "" {
				q.Set("host", n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		if n.Transport.GRPC != nil {
			q.Set("type", "grpc")
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
			q.Set("mode", "multi")
		}
	case nodespec.TransportTCP:
		q.Set("type", "tcp")
	case nodespec.TransportQUIC:
		q.Set("type", "quic")
	}

	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		if n.TLS.SNI != "" { q.Set("sni", n.TLS.SNI) }
		if n.TLS.Fingerprint != "" { q.Set("fp", strings.ToLower(n.TLS.Fingerprint)) }
		if len(n.TLS.ALPN) > 0 { q.Set("alpn", strings.Join(n.TLS.ALPN, ",")) }
		if n.TLS.AllowInsecure {
			q.Set("allowInsecure", "1")
		} else {
			q.Set("allowInsecure", "0")
		}
	} else if n.Security == nodespec.SecurityReality && n.Reality != nil {
		q.Set("security", "reality")
		q.Set("pbk", n.Reality.PublicKey)
		q.Set("sid", n.Reality.ShortID)
		if n.Reality.SNI != "" { q.Set("sni", n.Reality.SNI) }
		if n.Reality.Fingerprint != "" { q.Set("fp", strings.ToLower(n.Reality.Fingerprint)) }
		if n.Reality.SpiderX != "" { q.Set("spx", n.Reality.SpiderX) }
		q.Set("allowInsecure", "0")
	} else {
		q.Set("security", "none")
	}

	if flow != "" {
		q.Set("flow", string(flow))
	}
	if n.AllowUDP {
		q.Set("udp", "1")
	}
	u.RawQuery = q.Encode()
	u.Fragment = url.PathEscape(n.Name)
	return u.String()
}

func (r *ShadowrocketRenderer) renderVMessShadowrocket(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VMessCredentials)
	uuid := creds.UUID
	aid := creds.AlterID

	u := &url.URL{
		Scheme: "vmess",
		User:   url.User(uuid),
		Host:   net.JoinHostPort(n.Address, strconv.Itoa(port)),
	}
	q := u.Query()

	switch n.Transport.Type {
	case nodespec.TransportWS:
		if n.Transport.WS != nil {
			q.Set("type", "ws")
			q.Set("path", cleanWSPath(n.Transport.WS.Path))
			if n.Transport.WS.Host != "" {
				q.Set("host", n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		if n.Transport.GRPC != nil {
			q.Set("type", "grpc")
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
		}
	case nodespec.TransportTCP:
		q.Set("type", "tcp")
	}

	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		q.Set("security", "tls")
		if n.TLS.SNI != "" { q.Set("sni", n.TLS.SNI) }
		if n.TLS.Fingerprint != "" { q.Set("fp", strings.ToLower(n.TLS.Fingerprint)) }
		if len(n.TLS.ALPN) > 0 { q.Set("alpn", strings.Join(n.TLS.ALPN, ",")) }
		if n.TLS.AllowInsecure {
			q.Set("allowInsecure", "1")
		}
	} else {
		q.Set("security", "none")
	}

	q.Set("alterId", strconv.Itoa(aid))
	q.Set("v", "2")
	if n.AllowUDP {
		q.Set("udp", "1")
	}
	u.RawQuery = q.Encode()
	u.Fragment = url.PathEscape(n.Name)
	return u.String()
}

func (r *ShadowrocketRenderer) renderSSShadowrocket(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.ShadowsocksCredentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))

	u := &url.URL{
		Scheme: "ss",
		Host:   host,
	}
	u.User = url.UserPassword(creds.Method, creds.Password)
	q := u.Query()
	if n.AllowUDP {
		q.Set("udp", "1")
	}
	u.RawQuery = q.Encode()
	u.Fragment = url.PathEscape(n.Name)

	result := u.String()
	result = strings.Replace(result, "%3A", ":", 1)
	return result
}

func cleanWSPath(path string) string {
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	return path
}

func (r *ShadowrocketRenderer) renderTrojanShadowrocket(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.TrojanCredentials)
	u := &url.URL{
		Scheme: "trojan",
		User:   url.User(creds.Password),
		Host:   net.JoinHostPort(n.Address, strconv.Itoa(port)),
	}
	q := u.Query()
	switch n.Transport.Type {
	case nodespec.TransportWS:
		if n.Transport.WS != nil {
			q.Set("type", "ws")
			q.Set("path", cleanWSPath(n.Transport.WS.Path))
			if n.Transport.WS.Host != "" { q.Set("host", n.Transport.WS.Host) }
		}
	case nodespec.TransportGRPC:
		if n.Transport.GRPC != nil {
			q.Set("type", "grpc")
			q.Set("serviceName", n.Transport.GRPC.ServiceName)
		}
	}
	q.Set("security", "tls")
	if n.TLS != nil {
		if n.TLS.SNI != "" { q.Set("sni", n.TLS.SNI) }
		if n.TLS.Fingerprint != "" { q.Set("fp", strings.ToLower(n.TLS.Fingerprint)) }
		q.Set("allowInsecure", boolVal(n.TLS.AllowInsecure))
	}
	if n.AllowUDP { q.Set("udp", "1") }
	u.RawQuery = q.Encode()
	u.Fragment = url.PathEscape(n.Name)
	return u.String()
}

func (r *ShadowrocketRenderer) renderHysteria2Shadowrocket(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.Hysteria2Credentials)
	u := &url.URL{
		Scheme: "hysteria2",
		User:   url.User(creds.Password),
		Host:   net.JoinHostPort(n.Address, strconv.Itoa(port)),
	}
	q := u.Query()
	q.Set("security", "tls")
	if n.TLS != nil {
		if n.TLS.SNI != "" { q.Set("sni", n.TLS.SNI) }
		q.Set("insecure", boolVal(n.TLS.AllowInsecure))
	}
	u.RawQuery = q.Encode()
	u.Fragment = url.PathEscape(n.Name)
	return u.String()
}

func boolVal(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
