package renderer

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

type LoonRenderer struct{}

func NewLoonRenderer() *LoonRenderer { return &LoonRenderer{} }
func (r *LoonRenderer) Name() string        { return "loon" }
func (r *LoonRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (r *LoonRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	var lines []string
	lines = append(lines, "#!MANAGED-CONFIG interval=43200")
	lines = append(lines, "[General]")
	lines = append(lines, "")
	lines = append(lines, "[Proxy]")
	var names []string
	for _, n := range nodes {
		line, err := r.RenderNode(n)
		if err != nil {
			continue
		}
		lines = append(lines, line)
		names = append(names, loonName(n.Name))
	}
	lines = append(lines, "")
	lines = append(lines, "[Proxy Group]")
	lines = append(lines, "Proxy = select, " + strings.Join(append(names, "DIRECT"), ","))
	lines = append(lines, "Auto = url-test, " + strings.Join(names, ",") + ", url=http://www.gstatic.com/generate_204, interval=300")
	return []byte(strings.Join(lines, "\n")), nil
}

func loonName(s string) string {
	return strings.ReplaceAll(s, ",", "_")
}

func (r *LoonRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
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
	case nodespec.ProtocolVMess:
		return r.renderVMess(n, port), nil
	case nodespec.ProtocolTrojan:
		return r.renderTrojan(n, port), nil
	case nodespec.ProtocolShadowsocks:
		return r.renderSS(n, port), nil
	case nodespec.ProtocolHysteria2:
		return r.renderHysteria2(n, port), nil
	default:
		return "", fmt.Errorf("loon: unsupported %s", n.Protocol)
	}
}

func (r *LoonRenderer) renderVLESS(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VLESSCredentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))
	params := []string{creds.UUID, host}
	obfs := "none"
	var pathV, hostV string
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		pathV = n.Transport.WS.Path
		hostV = n.Transport.WS.Host
	}
	params = append(params, "transport="+obfs)
	if pathV != "" { params = append(params, "path="+pathV) }
	if hostV != "" { params = append(params, "host="+hostV) }
	overTLS := "false"
	if n.Security == nodespec.SecurityTLS || n.Security == nodespec.SecurityReality {
		overTLS = "true"
	}
	params = append(params, "over-tls="+overTLS)
	if n.TLS != nil {
		if n.TLS.SNI != "" { params = append(params, "tls-name="+n.TLS.SNI) }
		params = append(params, "skip-cert-verify="+boolVal(n.TLS.AllowInsecure))
	}
	if n.Security == nodespec.SecurityReality && n.Reality != nil {
		params = append(params, "reality-public-key="+n.Reality.PublicKey)
		params = append(params, "reality-short-id="+n.Reality.ShortID)
	}
	if n.AllowUDP { params = append(params, "udp=true") }
	return loonName(n.Name) + " = VLESS, " + strings.Join(params, ", ")
}

func (r *LoonRenderer) renderVMess(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.VMessCredentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))
	params := []string{creds.UUID, host, "aes-128-gcm"}
	obfs := "none"
	var pathV, hostV string
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		pathV = n.Transport.WS.Path
		hostV = n.Transport.WS.Host
	}
	params = append(params, "transport="+obfs)
	if pathV != "" { params = append(params, "path="+pathV) }
	if hostV != "" { params = append(params, "host="+hostV) }
	overTLS := "false"
	if n.Security == nodespec.SecurityTLS { overTLS = "true" }
	params = append(params, "over-tls="+overTLS)
	if n.TLS != nil && n.TLS.SNI != "" { params = append(params, "tls-name="+n.TLS.SNI) }
	return loonName(n.Name) + " = VMess, " + strings.Join(params, ", ")
}

func (r *LoonRenderer) renderTrojan(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.TrojanCredentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))
	params := []string{creds.Password, host}
	obfs := "none"
	var pathV, hostV string
	if n.Transport.Type == nodespec.TransportWS && n.Transport.WS != nil {
		obfs = "ws"
		pathV = n.Transport.WS.Path
		hostV = n.Transport.WS.Host
	}
	params = append(params, "transport="+obfs)
	if pathV != "" { params = append(params, "path="+pathV) }
	if hostV != "" { params = append(params, "host="+hostV) }
	params = append(params, "over-tls=true")
	if n.TLS != nil {
		if n.TLS.SNI != "" { params = append(params, "tls-name="+n.TLS.SNI) }
		params = append(params, "skip-cert-verify="+boolVal(n.TLS.AllowInsecure))
	}
	return loonName(n.Name) + " = Trojan, " + strings.Join(params, ", ")
}

func (r *LoonRenderer) renderSS(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.ShadowsocksCredentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))
	params := []string{creds.Method, creds.Password, host}
	return loonName(n.Name) + " = Shadowsocks, " + strings.Join(params, ", ")
}

func (r *LoonRenderer) renderHysteria2(n nodespec.NodeSpec, port int) string {
	creds, _ := n.Credentials.(nodespec.Hysteria2Credentials)
	host := net.JoinHostPort(n.Address, strconv.Itoa(port))
	params := []string{host, creds.Password}
	sni := ""
	if n.TLS != nil {
		sni = n.TLS.SNI
		params = append(params, "skip-cert-verify="+boolVal(n.TLS.AllowInsecure))
	}
	if sni != "" { params = append(params, "sni="+sni) }
	return loonName(n.Name) + " = Hysteria2, " + strings.Join(params, ", ")
}

var _ = url.QueryEscape
