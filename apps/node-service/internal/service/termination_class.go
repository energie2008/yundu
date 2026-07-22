package service

import (
	"fmt"

	"github.com/airport-panel/node-service/internal/model"
)

// TerminationClass 按 TLS 终止位置分类节点，用于统一渲染决策。
//
// P2-1: 替代分散在各处的 switch/if exposureMode 判断，
// 将"TLS 在哪里终止"这一架构决策收敛为单一枚举。
//
// 分类矩阵：
//   - cf_edge:     CF 边缘终止 TLS（argo_tunnel），源站接收明文 HTTP
//   - nginx:       nginx 8445 vhost 终止 TLS（cdn/cdn_saas），proxy_pass http 回源
//   - self_tcp:    xray 自身终止 TLS（direct TCP+TLS），nginx stream 仅 SNI 透传
//   - self_udp:    xray 自身终止 TLS（UDP 协议如 hysteria2/tuic），不经过 nginx
//   - reality:     xray REALITY 握手（direct reality），不走传统 TLS 证书
type TerminationClass string

const (
	TerminationCFEdge  TerminationClass = "cf_edge"
	TerminationNginx   TerminationClass = "nginx"
	TerminationSelfTCP TerminationClass = "self_tcp"
	TerminationSelfUDP TerminationClass = "self_udp"
	TerminationReality TerminationClass = "reality"
)

// ClassifyTermination 根据节点的 exposure_mode/protocol/security 判定 TerminationClass。
//
// 判定规则（按优先级）：
//  1. securityType=reality → TerminationReality
//  2. exposureMode=argo_tunnel → TerminationCFEdge（CF 边缘终止）
//  3. exposureMode=cdn/cdn_saas → TerminationNginx（nginx 8445 终止）
//  4. protocolType=hysteria2/tuic → TerminationSelfUDP（UDP 不经 nginx）
//  5. 其他 → TerminationSelfTCP（xray 自终止 TCP TLS）
func ClassifyTermination(node *model.Node) TerminationClass {
	if node == nil {
		return TerminationSelfTCP
	}

	securityType := getSecurityType(node)
	protocolType := node.ProtocolType

	// 1. REALITY 优先判定（不走传统 TLS 证书链路）
	if securityType == "reality" {
		return TerminationReality
	}

	// 2. argo_tunnel → CF 边缘终止
	em := determineExposureMode(node)
	if node.ExposureMode != nil && *node.ExposureMode != "" {
		em = *node.ExposureMode
	}
	if em == "argo_tunnel" {
		return TerminationCFEdge
	}

	// 3. cdn/cdn_saas → nginx 终止
	if em == "cdn" || em == "cdn_saas" {
		return TerminationNginx
	}

	// 4. UDP 协议 → xray 自终止 UDP TLS
	if protocolType == "hysteria2" || protocolType == "tuic" {
		return TerminationSelfUDP
	}

	// 5. 默认 → xray 自终止 TCP TLS
	return TerminationSelfTCP
}

// String 返回 TerminationClass 的可读描述。
func (tc TerminationClass) String() string {
	switch tc {
	case TerminationCFEdge:
		return "CF Edge TLS termination (argo_tunnel)"
	case TerminationNginx:
		return "nginx vhost TLS termination (cdn/cdn_saas)"
	case TerminationSelfTCP:
		return "xray self TLS termination (direct TCP)"
	case TerminationSelfUDP:
		return "xray self TLS termination (direct UDP)"
	case TerminationReality:
		return "xray REALITY handshake (direct reality)"
	default:
		return fmt.Sprintf("unknown termination class: %s", string(tc))
	}
}

// NeedsNginxVhost 判断该 TerminationClass 是否需要生成 nginx HTTP vhost（8445 location 路由）。
// 仅 TerminationNginx（CDN 节点）需要 nginx vhost 做 TLS termination + path 路由。
func (tc TerminationClass) NeedsNginxVhost() bool {
	return tc == TerminationNginx
}

// NeedsStreamSNI 判断该 TerminationClass 是否需要 nginx stream SNI 分流。
// cf_edge (argo_tunnel) 完全绕过 nginx（cloudflared 直连 xray），不需要 stream SNI；
// self_udp（UDP 协议）不经过 nginx stream；
// nginx/self_tcp/reality 都经过 nginx 443 stream（SNI 透传/分流）。
func (tc TerminationClass) NeedsStreamSNI() bool {
	switch tc {
	case TerminationCFEdge, TerminationSelfUDP:
		return false
	default:
		return true
	}
}

// NeedsCertBundle 判断该 TerminationClass 是否需要从 cert_bundles 注入证书 PEM。
// 仅 self_tcp 需要 xray 自身持有证书（nginx/cf_edge 终止 TLS 后明文回源）。
// reality 不走传统证书。self_udp 需要 xray 持有证书。
// cf_edge/nginx 的证书由 nginx 持有（ACME），xray inbound 为 sec=none 不需要 PEM。
func (tc TerminationClass) NeedsCertBundle() bool {
	switch tc {
	case TerminationSelfTCP, TerminationSelfUDP:
		return true
	default:
		return false
	}
}

// NeedsTLSStrip 判断该 TerminationClass 是否需要剥离 xray inbound 的 TLS。
// cf_edge: 剥离（cloudflared 明文 HTTP 回源）
// nginx: 剥离（nginx 终止 TLS 后 proxy_pass http）
// self_tcp/self_udp/reality: 不剥离（xray 自终止 TLS/REALITY）
func (tc TerminationClass) NeedsTLSStrip() bool {
	switch tc {
	case TerminationCFEdge, TerminationNginx:
		return true
	default:
		return false
	}
}
