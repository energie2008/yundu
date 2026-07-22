package renderer

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

type QuantumultRenderer struct{}

func NewQuantumultRenderer() *QuantumultRenderer { return &QuantumultRenderer{} }
func (r *QuantumultRenderer) Name() string        { return "quantumult" }
func (r *QuantumultRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (r *QuantumultRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	var lines []string
	lines = append(lines, "[general]")
	lines = append(lines, "strict_route=0")
	lines = append(lines, "")
	for _, n := range nodes {
		line, err := r.RenderNode(n)
		if err != nil {
			continue
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, "[rewrite_remote]")
	return []byte(strings.Join(lines, "\n")), nil
}

func (r *QuantumultRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	if err := n.Validate(); err != nil {
		return "", err
	}
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		return r.renderVLESS(n, port), nil
	case nodespec.ProtocolTrojan:
		return r.renderTrojan(n, port), nil
	case nodespec.ProtocolShadowsocks:
		return r.renderSS(n, port), nil
	case nodespec.ProtocolVMess:
		return r.renderVMess(n, port), nil
	case nodespec.ProtocolHysteria2:
		return r.renderHysteria2(n, port), nil
	default:
		return "", fmt.Errorf("quantumult: unsupported %s", n.Protocol)
	}
}

func qxEncTag(name string) string {
	return url.QueryEscape(name)
}

func qxTLSParams(n nodespec.NodeSpec) string {
	var parts []string
	overTLS := "0"
	if n.Security == nodespec.SecurityTLS || n.Security == nodespec.SecurityReality {
		overTLS = "1"
	}
	parts = append(parts, "over-tls="+overTLS)
	if n.TLS != nil {
		if n.TLS.SNI != "" {
			parts = append(parts, "sni="+n.TLS.SNI)
		}
		if n.TLS.AllowInsecure {
			parts = append(parts, "tls-verification=0")
		} else {
			parts = append(parts, "tls-verification=1")
		}
		if n.TLS.Fingerprint != "" {
			parts = append(parts, "tls-host="+n.TLS.SNI)
		}
	}
	if n.Reality != nil {
		parts = append(parts, "reality=1")
		parts = append(parts, "pbk="+n.Reality.PublicKey)
		parts = append(parts, "sid="+n.Reality.ShortID)
		if n.Reality.SNI != "" {
			parts = append(parts, "sni="+n.Reality.SNI)
		}
	}
	return strings.Join(parts, ", ")
}

func (r *QuantumultRenderer) renderVLESS(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VLESSCredentials)
	obfs := "none"
	obfsHost := ""
	obfsPath := ""
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		obfsPath = n.Transport.WS.Path
		obfsHost = n.Transport.WS.Host
	}
	tls := n.Security != nodespec.SecurityNone
	line := fmt.Sprintf("vless=%s:%d", n.Address, port)
	q := []string{
		"method=none",
		"password=" + creds.UUID,
		"fast-open=false",
		"udp-relay=" + boolStr(n.AllowUDP),
		"tag=" + qxEncTag(n.Name),
	}
	if obfs != "none" {
		q = append(q, "obfs="+obfs)
		if obfsHost != "" {
			q = append(q, "obfs-host="+obfsHost)
		}
		if obfsPath != "" {
			q = append(q, "obfs-uri="+obfsPath)
		}
	}
	if tls {
		q = append(q, "tls=1")
		if n.TLS != nil && n.TLS.SNI != "" {
			q = append(q, "sni="+n.TLS.SNI)
		}
		if n.TLS != nil && n.TLS.AllowInsecure {
			q = append(q, "tls-verification=0")
		}
		if n.Reality != nil {
			q = append(q, "reality=1")
			q = append(q, "pbk="+n.Reality.PublicKey)
			q = append(q, "sid="+n.Reality.ShortID)
		}
	}
	return line + ", " + strings.Join(q, ", ")
}

func (r *QuantumultRenderer) renderTrojan(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.TrojanCredentials)
	obfs := "none"
	var obfsPath, obfsHost string
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		obfsPath = n.Transport.WS.Path
		obfsHost = n.Transport.WS.Host
	}
	q := []string{
		"password=" + creds.Password,
		"fast-open=false",
		"udp-relay=" + boolStr(n.AllowUDP),
		"tag=" + qxEncTag(n.Name),
	}
	if obfs != "none" {
		q = append(q, "obfs="+obfs)
		if obfsHost != "" {
			q = append(q, "obfs-host="+obfsHost)
		}
		if obfsPath != "" {
			q = append(q, "obfs-uri="+obfsPath)
		}
	}
	if n.TLS != nil {
		q = append(q, "tls=1")
		if n.TLS.SNI != "" { q = append(q, "sni="+n.TLS.SNI) }
		if n.TLS.AllowInsecure { q = append(q, "tls-verification=0") } else { q = append(q, "tls-verification=1") }
	}
	return fmt.Sprintf("trojan=%s:%d, %s", n.Address, port, strings.Join(q, ", "))
}

func (r *QuantumultRenderer) renderSS(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.ShadowsocksCredentials)
	q := []string{
		"method=" + creds.Method,
		"password=" + creds.Password,
		"fast-open=false",
		"udp-relay=" + boolStr(n.AllowUDP),
		"tag=" + qxEncTag(n.Name),
	}
	if n.TLS != nil {
		q = append(q, "tls=1")
		q = append(q, "sni="+n.TLS.SNI)
	}
	return fmt.Sprintf("shadowsocks=%s:%d, %s", n.Address, port, strings.Join(q, ", "))
}

func (r *QuantumultRenderer) renderVMess(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VMessCredentials)
	obfs := "none"
	var obfsPath, obfsHost string
	tls := "none"
	aes128gcm := false
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		obfsPath = n.Transport.WS.Path
		obfsHost = n.Transport.WS.Host
	}
	if n.Security == nodespec.SecurityTLS {
		tls = "tls"
	}
	sni := ""
	if n.TLS != nil {
		sni = n.TLS.SNI
		if n.TLS.AllowInsecure {
			aes128gcm = false
		}
	}
	_ = aes128gcm
	_ = tls
	line := fmt.Sprintf("vmess=%s:%d", n.Address, port)
	q := []string{
		"method=aes-128-gcm",
		"password=" + creds.UUID,
		"udp-relay=" + boolStr(n.AllowUDP),
		"tag=" + qxEncTag(n.Name),
	}
	if obfs != "none" {
		q = append(q, "obfs="+obfs)
		if obfsHost != "" { q = append(q, "obfs-host="+obfsHost) }
		if obfsPath != "" { q = append(q, "obfs-uri="+obfsPath) }
	}
	if n.Security == nodespec.SecurityTLS {
		q = append(q, "tls=1")
		if sni != "" { q = append(q, "sni="+sni) }
	}
	return line + ", " + strings.Join(q, ", ")
}

func (r *QuantumultRenderer) renderHysteria2(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.Hysteria2Credentials)
	sni := ""
	insecure := "0"
	if n.TLS != nil {
		sni = n.TLS.SNI
		if n.TLS.AllowInsecure {
			insecure = "1"
		}
	}
	return fmt.Sprintf("hysteria2=%s:%d, password=%s, sni=%s, tls-verification=%s, tag=%s, udp-relay=1",
		n.Address, port, creds.Password, sni, insecure, qxEncTag(n.Name))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var _ = strconv.Itoa
var _ = tls.VersionTLS13
