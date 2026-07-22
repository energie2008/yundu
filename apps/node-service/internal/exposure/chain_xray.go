package exposure

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/nodespec"
)

// fetchPinnedCertSHA256 连接到上游代理服务器，获取其 TLS 证书的 SHA256 指纹。
// 返回 hex 编码（xray infra/conf 用 hex.DecodeString 解码 pinnedPeerCertSha256）。
// 用于处理上游代理自签证书或 SNI 与证书域名不匹配的场景（伪装域名）。
// 超时 5 秒，失败时返回 error（调用方应跳过 pin，让 xray 用 SNI 正常验证）。
// 注意：xray 26.3.27 已移除 allowInsecure（2026-06-01 后返回 PrintRemovedFeatureError），
// 不能再用 allowInsecure=true 作为兜底。
func fetchPinnedCertSHA256(address string, port int, sni string) (string, error) {
	addr := fmt.Sprintf("%s:%d", address, port)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true, // 跳过验证以获取证书（我们需要证书本身，不需要验证）
	})
	if err != nil {
		return "", fmt.Errorf("fetch cert failed: %w", err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificates")
	}

	// 使用叶子证书（第一个）的 DER 编码计算 SHA256，hex 编码。
	// xray-core infra/conf 用 hex.DecodeString 解码 pinnedPeerCertSha256（transport_internet.go L724），
	// JSON 中为单个 hex string（非数组、非 base64）。
	cert := state.PeerCertificates[0]
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:]), nil
}

// BuildXrayChainOutbounds 构建订阅侧多跳链式代理的完整 xray outbounds + routing。
// 保留原签名供订阅服务消费；内部调用 BuildXrayOutboundFromNodeSpec（IR 层统一提取）。
func BuildXrayChainOutbounds(c *chain.ChainSpec) ([]map[string]interface{}, map[string]interface{}, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}

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

		outbound, err := BuildXrayOutboundFromNodeSpec(hopNode, tag, prevTag)
		if err != nil {
			return nil, nil, fmt.Errorf("build relay %s outbound: %w", tag, err)
		}
		outbounds = append(outbounds, outbound)
		prevTag = tag
	}

	landingTag := "landing"
	landingOutbound, err := BuildXrayOutboundFromNodeSpec(c.LandingNode, landingTag, prevTag)
	if err != nil {
		return nil, nil, fmt.Errorf("build landing outbound: %w", err)
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

	return outbounds, routing, nil
}

// BuildXrayOutboundFromNodeSpec 从 NodeSpec 构建 xray outbound（套娃出站构建器）。
// 字段提取统一走 ExtractChainOutboundFields（IR 层），本函数只做 xray 专有序列化。
// proxyTag 非空时附加 proxySettings（多跳链用）；服务端单跳套娃传 ""，走 routing 路由。
func BuildXrayOutboundFromNodeSpec(ns *nodespec.NodeSpec, tag, proxyTag string) (map[string]interface{}, error) {
	if ns == nil {
		return nil, fmt.Errorf("nil nodespec")
	}
	f, err := ExtractChainOutboundFields(ns)
	if err != nil {
		return nil, err
	}

	ob := map[string]interface{}{
		"tag":      tag,
		"protocol": f.Protocol,
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

// buildXrayChainSettings 按 IR 协议字段序列化 xray outbound settings。
// 借鉴 xboard-node：字段已由 IR 层提取一次，此处只做内核专有结构映射。
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

// buildXrayChainStreamSettings 从 IR 字段构建 xray streamSettings。
// TLS/Reality/Transport 的差异由 IR 层归一化，此处只做 xray 专有结构映射。
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
			// 上游代理自签证书或 SNI 与证书域名不匹配时（如 bilivideo.com 伪装），
			// 优先通过自动连接上游获取证书指纹（SHA256），注入 pinnedPeerCertSha256 跳过域名验证。
			// 这是零SSH方案：面板填写 URI（含 insecure=1）→ 自动计算 pin → TLS 验证通过。
			// xray infra/conf 用 hex.DecodeString 解码 pinnedPeerCertSha256，JSON 中为单个 hex string。
			// 注意：xray 26.3.27 已移除 allowInsecure（2026-06-01 后返回 PrintRemovedFeatureError），
			// 不能再用 allowInsecure=true 作为兜底。pin 获取失败时跳过，让 xray 用 SNI 正常验证。
			// 若 SNI 与证书不匹配，observatory 会标记 dead → balancer 降级 direct（链路降级但不崩溃）。
			if f.AllowInsecure {
				if pin, err := fetchPinnedCertSHA256(f.Address, f.Port, f.SNI); err == nil {
					tlsSettings["pinnedPeerCertSha256"] = pin
				}
				// pin 获取失败时不设置任何 TLS 覆盖（allowInsecure 已被 xray 移除）
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
