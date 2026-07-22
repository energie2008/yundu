package renderer

import (
	"fmt"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

type SurgeRenderer struct{}

func NewSurgeRenderer() *SurgeRenderer { return &SurgeRenderer{} }
func (r *SurgeRenderer) Name() string        { return "surge" }
func (r *SurgeRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (r *SurgeRenderer) Render(nodes []nodespec.NodeSpec) ([]byte, error) {
	var lines []string
	lines = append(lines, "#!MANAGED-CONFIG https://example.com interval=43200 strict=false")
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
		names = append(names, surgeName(n.Name))
	}
	lines = append(lines, "")
	lines = append(lines, "[Proxy Group]")
	lines = append(lines, "Proxy = select, " + strings.Join(append(names, "DIRECT"), ", "))
	lines = append(lines, "Auto = url-test, " + strings.Join(names, ", ") + ", url=http://www.gstatic.com/generate_204, interval=300")
	lines = append(lines, "")
	lines = append(lines, "[Rule]")
	lines = append(lines, "DOMAIN-SUFFIX,local,DIRECT")
	lines = append(lines, "IP-CIDR,127.0.0.0/8,DIRECT")
	lines = append(lines, "GEOIP,CN,DIRECT")
	lines = append(lines, "FINAL,Proxy")
	return []byte(strings.Join(lines, "\n")), nil
}

func surgeName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "=", "_")
	s = strings.ReplaceAll(s, ",", "_")
	return s
}

func (r *SurgeRenderer) RenderNode(n nodespec.NodeSpec) (string, error) {
	if err := n.Validate(); err != nil {
		return "", err
	}
	port := n.Port
	if n.ClientPort > 0 {
		port = n.ClientPort
	}

	switch n.Protocol {
	case nodespec.ProtocolVLESS:
		return r.renderVLESS(n, port)
	case nodespec.ProtocolTrojan:
		return r.renderTrojan(n, port)
	case nodespec.ProtocolShadowsocks:
		return r.renderSS(n, port)
	case nodespec.ProtocolVMess:
		return r.renderVMess(n, port)
	case nodespec.ProtocolHysteria2:
		return r.renderHysteria2(n, port)
	case nodespec.ProtocolTUIC:
		return r.renderTUIC(n, port)
	default:
		return "", fmt.Errorf("surge: unsupported %s", n.Protocol)
	}
}

func (r *SurgeRenderer) renderVLESS(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.VLESSCredentials)
	params := []string{
		creds.UUID,
		fmt.Sprintf("%s:%d", n.Address, port),
	}
	transport := ""
	switch n.Transport.Type {
	case nodespec.TransportWS:
		transport = "ws"
		if n.Transport.WS != nil {
			params = append(params, "ws-path="+n.Transport.WS.Path)
			if n.Transport.WS.Host != "" {
				params = append(params, "ws-headers=Host:"+n.Transport.WS.Host)
			}
		}
	case nodespec.TransportGRPC:
		transport = "grpc"
		if n.Transport.GRPC != nil {
			params = append(params, "grpc-service-name="+n.Transport.GRPC.ServiceName)
		}
	default:
		transport = "tcp"
	}
	params = append(params, "transport="+transport)
	tlsMode := "none"
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		tlsMode = "tls"
		params = append(params, "tls=true")
		if n.TLS.SNI != "" { params = append(params, "sni="+n.TLS.SNI) }
		if n.TLS.Fingerprint != "" { params = append(params, "tls-fingerprint="+n.TLS.Fingerprint) }
		if n.TLS.AllowInsecure { params = append(params, "skip-cert-verify=true") }
	} else if n.Security == nodespec.SecurityReality {
		tlsMode = "reality"
		params = append(params, "tls=true")
		if n.Reality != nil {
			params = append(params, "reality-public-key="+n.Reality.PublicKey)
			params = append(params, "reality-short-id="+n.Reality.ShortID)
			if n.Reality.SNI != "" { params = append(params, "sni="+n.Reality.SNI) }
		}
	}
	_ = tlsMode
	if creds.Flow != "" {
		params = append(params, "flow="+string(creds.Flow))
	}
	if n.AllowUDP { params = append(params, "udp-relay=true") }
	return surgeName(n.Name) + " = vless, " + strings.Join(params, ", "), nil
}

func (r *SurgeRenderer) renderTrojan(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.TrojanCredentials)
	params := []string{creds.Password, fmt.Sprintf("%s:%d", n.Address, port)}
	transport := "tcp"
	switch n.Transport.Type {
	case nodespec.TransportWS:
		transport = "ws"
		if n.Transport.WS != nil {
			params = append(params, "ws-path="+n.Transport.WS.Path)
			if n.Transport.WS.Host != "" { params = append(params, "ws-headers=Host:"+n.Transport.WS.Host) }
		}
	case nodespec.TransportGRPC:
		transport = "grpc"
		if n.Transport.GRPC != nil {
			params = append(params, "grpc-service-name="+n.Transport.GRPC.ServiceName)
		}
	}
	params = append(params, "transport="+transport, "tls=true")
	if n.TLS != nil {
		if n.TLS.SNI != "" { params = append(params, "sni="+n.TLS.SNI) }
		if n.TLS.AllowInsecure { params = append(params, "skip-cert-verify=true") }
	}
	if n.AllowUDP { params = append(params, "udp-relay=true") }
	return surgeName(n.Name) + " = trojan, " + strings.Join(params, ", "), nil
}

func (r *SurgeRenderer) renderSS(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.ShadowsocksCredentials)
	params := []string{creds.Method, creds.Password, fmt.Sprintf("%s:%d", n.Address, port)}
	if n.AllowUDP { params = append(params, "udp-relay=true") }
	return surgeName(n.Name) + " = ss, " + strings.Join(params, ", "), nil
}

func (r *SurgeRenderer) renderVMess(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.VMessCredentials)
	params := []string{fmt.Sprintf("%s:%d", n.Address, port), "aes-128-gcm", creds.UUID}
	transport := "tcp"
	switch n.Transport.Type {
	case nodespec.TransportWS:
		transport = "ws"
		if n.Transport.WS != nil {
			params = append(params, "ws-path="+n.Transport.WS.Path)
			if n.Transport.WS.Host != "" { params = append(params, "ws-headers=Host:"+n.Transport.WS.Host) }
		}
	}
	params = append(params, "transport="+transport)
	if n.Security == nodespec.SecurityTLS && n.TLS != nil {
		params = append(params, "tls=true")
		if n.TLS.SNI != "" { params = append(params, "sni="+n.TLS.SNI) }
	}
	return surgeName(n.Name) + " = vmess, " + strings.Join(params, ", "), nil
}

func (r *SurgeRenderer) renderHysteria2(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.Hysteria2Credentials)
	params := []string{fmt.Sprintf("%s:%d", n.Address, port), creds.Password}
	if n.TLS != nil {
		if n.TLS.SNI != "" { params = append(params, "sni="+n.TLS.SNI) }
		if n.TLS.AllowInsecure { params = append(params, "skip-cert-verify=true") }
	}
	if n.AllowUDP { params = append(params, "udp-relay=true") }
	return surgeName(n.Name) + " = hysteria2, " + strings.Join(params, ", "), nil
}

func (r *SurgeRenderer) renderTUIC(n nodespec.NodeSpec, port int) (string, error) {
	creds, _ := n.Credentials.(nodespec.TUICCredentials)
	params := []string{fmt.Sprintf("%s:%d", n.Address, port), creds.UUID, creds.Password}
	if n.TLS != nil {
		if n.TLS.SNI != "" { params = append(params, "sni="+n.TLS.SNI) }
		if n.TLS.AllowInsecure { params = append(params, "skip-cert-verify=true") }
	}
	if n.AllowUDP { params = append(params, "udp-relay=true") }
	return surgeName(n.Name) + " = tuic-v5, " + strings.Join(params, ", "), nil
}
