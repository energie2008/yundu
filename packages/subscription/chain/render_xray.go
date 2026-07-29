package chain

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/airport-panel/subscription/nodespec"
)

// renderXrayChain 构建 xray 链式代理的完整 outbounds + routing。
func renderXrayChain(c *ChainSpec) (*ChainRenderResult, error) {
	outbounds := make([]map[string]interface{}, 0)
	outbounds = append(outbounds,
		map[string]interface{}{
			"protocol": "freedom",
			"tag":      "direct",
		},
		map[string]interface{}{
			"protocol": "blackhole",
			"tag":      "block",
		},
	)

	var prevTag string
	for i, hop := range c.Relays {
		tag := hop.Tag
		if tag == "" {
			tag = fmt.Sprintf("relay-%d", i)
		}

		hopNode := &nodespec.NodeSpec{
			ID:          hop.NodeID,
			Code:        tag,
			Name:        tag,
			Protocol:    hop.Protocol,
			Address:     hop.Address,
			Port:        hop.Port,
			Credentials: hop.Credentials,
			Transport:   hop.Transport,
			Security:    hop.Security,
			TLS:         hop.TLS,
			Reality:     hop.Reality,
			AllowUDP:    true,
			TrafficRate: 1.0,
		}

		outbound, err := renderXrayOutboundFromNodeSpec(hopNode, tag, prevTag)
		if err != nil {
			return nil, fmt.Errorf("build relay %s outbound: %w", tag, err)
		}
		outbounds = append(outbounds, outbound)
		prevTag = tag
	}

	landingTag := "landing"
	landingOutbound, err := renderXrayOutboundFromNodeSpec(c.LandingNode, landingTag, prevTag)
	if err != nil {
		return nil, fmt.Errorf("build landing outbound: %w", err)
	}
	outbounds = append(outbounds, landingOutbound)

	routing := map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"outboundTag": "block",
				"ip":          []string{"geoip:private"},
			},
		},
		"final": landingTag,
	}

	return &ChainRenderResult{Outbounds: outbounds, Routing: routing}, nil
}

// renderXrayOutboundFromNodeSpec 从 NodeSpec 构建 xray outbound。
func renderXrayOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, proxyTag string) (map[string]interface{}, error) {
	if ns == nil {
		return nil, fmt.Errorf("nil nodespec")
	}
	f, err := ExtractChainOutboundFields(ns)
	if err != nil {
		return nil, err
	}

	proto := f.Protocol
	if proto == "ss" {
		proto = "shadowsocks"
	}
	ob := map[string]interface{}{
		"tag":      tag,
		"protocol": proto,
	}

	settings, err := buildXrayChainSettings(f)
	if err != nil {
		return nil, err
	}
	ob["settings"] = settings

	if ss := buildXrayChainStreamSettings(f); ss != nil {
		ob["streamSettings"] = ss
	}

	if proxyTag != "" {
		ob["proxySettings"] = map[string]interface{}{"tag": proxyTag}
	}

	return ob, nil
}

func buildXrayChainSettings(f *ChainOutboundFields) (map[string]interface{}, error) {
	switch f.Protocol {
	case "socks", "http":
		server := map[string]interface{}{
			"address": f.Address,
			"port":    f.Port,
		}
		if f.Username != "" || f.Password != "" {
			server["users"] = []interface{}{map[string]interface{}{
				"user": f.Username,
				"pass": f.Password,
			}}
		}
		return map[string]interface{}{"servers": []interface{}{server}}, nil
	case "trojan":
		return map[string]interface{}{"servers": []interface{}{map[string]interface{}{
			"address":  f.Address,
			"port":     f.Port,
			"password": f.Password,
		}}}, nil
	case "vless":
		user := map[string]interface{}{"id": f.UUID, "encryption": f.Encryption}
		if f.Flow != "" {
			user["flow"] = f.Flow
		}
		return map[string]interface{}{"vnext": []interface{}{map[string]interface{}{
			"address": f.Address,
			"port":    f.Port,
			"users":   []interface{}{user},
		}}}, nil
	case "vmess":
		user := map[string]interface{}{
			"id":       f.UUID,
			"alterId":  f.AlterID,
			"security": "auto",
		}
		return map[string]interface{}{"vnext": []interface{}{map[string]interface{}{
			"address": f.Address,
			"port":    f.Port,
			"users":   []interface{}{user},
		}}}, nil
	case "ss":
		return map[string]interface{}{"servers": []interface{}{map[string]interface{}{
			"address":  f.Address,
			"port":     f.Port,
			"method":   f.SSMethod,
			"password": f.Password,
		}}}, nil
	default:
		return nil, fmt.Errorf("xray unsupported chain protocol: %s", f.Protocol)
	}
}

func buildXrayChainStreamSettings(f *ChainOutboundFields) map[string]interface{} {
	if !f.TLSEnabled && (f.Transport == "" || f.Transport == "tcp") {
		return nil
	}

	ss := map[string]interface{}{"network": f.Transport}
	if f.Transport == "" {
		ss["network"] = "tcp"
	}

	if f.TLSEnabled {
		if f.IsReality {
			ss["security"] = "reality"
			realitySettings := map[string]interface{}{}
			if f.SNI != "" {
				realitySettings["serverName"] = f.SNI
			}
			if f.RealityPublicKey != "" {
				realitySettings["publicKey"] = f.RealityPublicKey
			}
			if f.RealityShortID != "" {
				realitySettings["shortId"] = f.RealityShortID
			}
			if f.Fingerprint != "" {
				realitySettings["fingerprint"] = f.Fingerprint
			}
			ss["realitySettings"] = realitySettings
		} else {
			ss["security"] = "tls"
			tlsSettings := map[string]interface{}{}
			if f.SNI != "" {
				tlsSettings["serverName"] = f.SNI
			}
			if len(f.ALPN) > 0 {
				tlsSettings["alpn"] = f.ALPN
			} else {
				tlsSettings["alpn"] = []string{"h2", "http/1.1"}
			}
			if f.Fingerprint != "" {
				tlsSettings["fingerprint"] = f.Fingerprint
			}
			if f.AllowInsecure {
				if pin, err := fetchPinnedCertSHA256(f.Address, f.Port, f.SNI); err == nil {
					tlsSettings["pinnedPeerCertSha256"] = pin
				}
			}
			ss["tlsSettings"] = tlsSettings
		}
	}

	switch f.Transport {
	case "ws":
		wsSettings := map[string]interface{}{"path": f.WSPath}
		if f.WSPath == "" {
			wsSettings["path"] = "/"
		}
		if f.WSHost != "" {
			wsSettings["headers"] = map[string]interface{}{"Host": f.WSHost}
		}
		ss["wsSettings"] = wsSettings
	case "grpc":
		grpcSettings := map[string]interface{}{}
		if f.GRPCServiceName != "" {
			grpcSettings["serviceName"] = f.GRPCServiceName
		}
		ss["grpcSettings"] = grpcSettings
	case "tcp", "":
		ss["tcpSettings"] = map[string]interface{}{
			"header": map[string]interface{}{"type": "none"},
		}
	}

	return ss
}

// fetchPinnedCertSHA256 连接到上游代理服务器，获取其 TLS 证书的 SHA256 指纹。
func fetchPinnedCertSHA256(address string, port int, sni string) (string, error) {
	addr := fmt.Sprintf("%s:%d", address, port)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return "", fmt.Errorf("fetch cert failed: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificates")
	}

	cert := state.PeerCertificates[0]
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:]), nil
}

// DefaultXrayRouting 返回 xray 默认 routing 模板（chain 渲染专用）。
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
