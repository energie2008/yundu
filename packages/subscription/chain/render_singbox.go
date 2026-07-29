package chain

import (
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

// renderSingboxChain 构建 sing-box 链式代理的完整 outbounds + routing。
func renderSingboxChain(c *ChainSpec) (*ChainRenderResult, error) {
	outbounds := make([]map[string]interface{}, 0)
	outbounds = append(outbounds,
		map[string]interface{}{
			"type": "direct",
			"tag":  "direct",
		},
		map[string]interface{}{
			"type": "block",
			"tag":  "block",
		},
		map[string]interface{}{
			"type": "dns",
			"tag":  "dns-out",
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

		outbound, err := renderSingboxOutboundFromNodeSpec(hopNode, tag, prevTag)
		if err != nil {
			return nil, fmt.Errorf("build relay %s outbound: %w", tag, err)
		}
		outbounds = append(outbounds, outbound)
		prevTag = tag
	}

	landingTag := "landing"
	landingOutbound, err := renderSingboxOutboundFromNodeSpec(c.LandingNode, landingTag, prevTag)
	if err != nil {
		return nil, fmt.Errorf("build landing outbound: %w", err)
	}
	outbounds = append(outbounds, landingOutbound)

	routing := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"outbound": "block",
				"ip_cidr":  []string{"geoip:private"},
			},
		},
		"final":               landingTag,
		"auto_detect_interface": true,
	}

	return &ChainRenderResult{Outbounds: outbounds, Routing: routing}, nil
}

// renderSingboxOutboundFromNodeSpec 从 NodeSpec 构建 sing-box outbound。
func renderSingboxOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, detourTag string) (map[string]interface{}, error) {
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
		"type":        proto,
		"tag":         tag,
		"server":      f.Address,
		"server_port": f.Port,
	}

	switch f.Protocol {
	case "socks", "http":
		if f.Username != "" {
			ob["username"] = f.Username
		}
		if f.Password != "" {
			ob["password"] = f.Password
		}
	case "trojan":
		ob["password"] = f.Password
	case "vless":
		ob["uuid"] = f.UUID
		if f.Flow != "" {
			ob["flow"] = f.Flow
		}
		ob["packet_encoding"] = "xudp"
	case "vmess":
		ob["uuid"] = f.UUID
		ob["alter_id"] = f.AlterID
		ob["security"] = "auto"
	case "ss":
		ob["method"] = f.SSMethod
		ob["password"] = f.Password
	default:
		return nil, fmt.Errorf("sing-box unsupported chain protocol: %s", f.Protocol)
	}

	if tls := buildSingboxChainTLS(f); tls != nil {
		ob["tls"] = tls
	}

	if transport := buildSingboxChainTransport(f); transport != nil {
		ob["transport"] = transport
	}

	if detourTag != "" {
		ob["detour"] = detourTag
	}

	return ob, nil
}

func buildSingboxChainTLS(f *ChainOutboundFields) map[string]interface{} {
	if !f.TLSEnabled {
		return nil
	}

	tls := map[string]interface{}{"enabled": true}

	if f.IsReality {
		utls := map[string]interface{}{"enabled": true}
		if f.Fingerprint != "" {
			utls["fingerprint"] = f.Fingerprint
		} else {
			utls["fingerprint"] = "chrome"
		}
		tls["utls"] = utls

		reality := map[string]interface{}{"enabled": true}
		if f.RealityPublicKey != "" {
			reality["public_key"] = f.RealityPublicKey
		}
		if f.RealityShortID != "" {
			reality["short_id"] = f.RealityShortID
		}
		tls["reality"] = reality

		if f.SNI != "" {
			tls["server_name"] = f.SNI
		}
	} else {
		if f.SNI != "" {
			tls["server_name"] = f.SNI
		}
		if len(f.ALPN) > 0 {
			tls["alpn"] = f.ALPN
		}
		if f.Fingerprint != "" {
			tls["utls"] = map[string]interface{}{
				"enabled":     true,
				"fingerprint": f.Fingerprint,
			}
		}
		if f.AllowInsecure {
			tls["insecure"] = true
		}
	}

	return tls
}

func buildSingboxChainTransport(f *ChainOutboundFields) map[string]interface{} {
	if f.Transport == "" || f.Transport == "tcp" {
		return nil
	}

	switch f.Transport {
	case "ws":
		ws := map[string]interface{}{}
		if f.WSPath != "" {
			ws["path"] = f.WSPath
		} else {
			ws["path"] = "/"
		}
		if f.WSHost != "" {
			ws["headers"] = map[string]interface{}{"Host": f.WSHost}
		}
		return map[string]interface{}{"type": "ws", "ws": ws}
	case "grpc":
		grpcOpts := map[string]interface{}{}
		if f.GRPCServiceName != "" {
			grpcOpts["service_name"] = f.GRPCServiceName
		}
		return map[string]interface{}{"type": "grpc", "grpc": grpcOpts}
	default:
		return map[string]interface{}{"type": f.Transport}
	}
}
