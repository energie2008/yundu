package exposure

import (
	"fmt"

	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
)

// BuildSingboxChainOutbounds 构建订阅侧多跳链式代理的 sing-box outbounds。
// 保留原签名供订阅服务消费；内部调用 BuildSingboxOutboundFromNodeSpec（IR 层统一提取）。
func BuildSingboxChainOutbounds(c *chain.ChainSpec) ([]map[string]interface{}, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

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

		outbound, err := BuildSingboxOutboundFromNodeSpec(hopNode, tag, prevTag)
		if err != nil {
			return nil, fmt.Errorf("build relay %s outbound: %w", tag, err)
		}
		outbounds = append(outbounds, outbound)
		prevTag = tag
	}

	landingTag := "landing"
	landingOutbound, err := BuildSingboxOutboundFromNodeSpec(c.LandingNode, landingTag, prevTag)
	if err != nil {
		return nil, fmt.Errorf("build landing outbound: %w", err)
	}
	outbounds = append(outbounds, landingOutbound)

	return outbounds, nil
}

// BuildSingboxChainRoute 构建订阅侧多跳链式代理的 sing-box route。
func BuildSingboxChainRoute(c *chain.ChainSpec) map[string]interface{} {
	landingTag := "landing"
	rules := []interface{}{
		map[string]interface{}{
			"outbound": "block",
			"ip_cidr":  []string{"geoip:private"},
		},
	}
	return map[string]interface{}{
		"rules":               rules,
		"final":               landingTag,
		"auto_detect_interface": true,
	}
}

// BuildSingboxOutboundFromNodeSpec 从 NodeSpec 构建 sing-box outbound（套娃出站构建器）。
// 字段提取统一走 ExtractChainOutboundFields（IR 层），本函数只做 sing-box 专有序列化。
// detourTag 非空时附加 detour（多跳链用）；服务端单跳套娃传 ""，走 route 规则路由。
func BuildSingboxOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, detourTag string) (map[string]interface{}, error) {
	if ns == nil {
		return nil, fmt.Errorf("nil nodespec")
	}
	f, err := ExtractChainOutboundFields(ns)
	if err != nil {
		return nil, err
	}

	// sing-box 协议类型：shadowsocks 用 "shadowsocks"（非 "ss"）
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

// buildSingboxChainTLS 从 IR 字段构建 sing-box tls 配置。
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

// buildSingboxChainTransport 从 IR 字段构建 sing-box transport 配置。
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
