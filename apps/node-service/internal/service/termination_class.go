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
//   - cf_edge:          CF 边缘终止 TLS（argo_tunnel），源站接收明文 HTTP
//   - nginx:            [P4 deprecated] nginx 8445 vhost 终止 TLS（cdn/cdn_saas），proxy_pass http 回源
//   - nginx_plus_xray:  [P4] CDN 节点 xray 持证书 + nginx proxy_pass https（cdn/cdn_saas）
//   - self_tcp:         xray 自身终止 TLS（direct TCP+TLS），nginx stream 仅 SNI 透传
//   - self_udp:         xray 自身终止 TLS（UDP 协议如 hysteria2/tuic），不经过 nginx
//   - reality:          xray REALITY 握手（direct reality），不走传统 TLS 证书
type TerminationClass string

const (
	TerminationCFEdge        TerminationClass = "cf_edge"
	TerminationNginx         TerminationClass = "nginx"           // [P4 deprecated] CDN 节点改用 nginx_plus_xray
	TerminationNginxPlusXray TerminationClass = "nginx_plus_xray" // [P4] CDN 节点 xray 持证书，nginx proxy_pass https
	TerminationSelfTCP       TerminationClass = "self_tcp"
	TerminationSelfUDP       TerminationClass = "self_udp"
	TerminationReality       TerminationClass = "reality"
)

// ClassifyTermination 根据节点的 exposure_mode/protocol/security 判定 TerminationClass。
//
// 判定规则（按优先级）：
//  1. securityType=reality → TerminationReality
//  2. exposureMode=argo_tunnel → TerminationCFEdge（CF 边缘终止）
//  3. exposureMode=cdn/cdn_saas → TerminationNginxPlusXray（P4: xray 持证书 + nginx proxy_pass https）
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

	// 3. cdn/cdn_saas → nginx + xray 共持证书（P4: xray 持证书，nginx proxy_pass https）
	if em == "cdn" || em == "cdn_saas" {
		return TerminationNginxPlusXray
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
		return "[deprecated] nginx vhost TLS termination (cdn/cdn_saas, P4 前架构)"
	case TerminationNginxPlusXray:
		return "nginx + xray shared TLS (cdn/cdn_saas, P4: xray 持证书 nginx proxy_pass https)"
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
// TerminationNginx（P4 前）和 TerminationNginxPlusXray（P4 后）都需要 nginx vhost。
// 区别：nginx 做 TLS termination + proxy_pass http；nginx_plus_xray 做 proxy_pass https（TLS 由 xray 终止）。
func (tc TerminationClass) NeedsNginxVhost() bool {
	return tc == TerminationNginx || tc == TerminationNginxPlusXray
}

// NeedsStreamSNI 判断该 TerminationClass 是否需要 nginx stream SNI 分流。
//
// P5 改造后：只有 CDN 节点（TerminationNginx/TerminationNginxPlusXray）需要 nginx 443 stream SNI 分流。
// - cf_edge (argo_tunnel) 完全绕过 nginx（cloudflared 直连 xray）
// - self_udp（UDP 协议）不经过 nginx stream
// - self_tcp（trojan+tls/ss/mieru 直连）走 0.0.0.0:port，绕过 nginx stream
// - reality 走 0.0.0.0:port，绕过 nginx stream（消除同 SNI 单后端限制 + 退役 fallback 静态站）
func (tc TerminationClass) NeedsStreamSNI() bool {
	switch tc {
	case TerminationCFEdge, TerminationSelfUDP, TerminationSelfTCP, TerminationReality:
		return false
	default:
		return true // 仅 TerminationNginx(CDN) / TerminationNginxPlusXray(CDN P4) 走 nginx stream
	}
}

// NeedsCertBundle 判断该 TerminationClass 是否需要从 cert_bundles 注入证书 PEM。
// P4 后 nginx_plus_xray 也需要 xray 持有证书（nginx proxy_pass https，TLS 由 xray 终止）。
// cf_edge（argo_tunnel）的 TLS 由 CF 边缘终止，源站 xray sec=none 不需要 PEM。
// nginx（P4 前 deprecated）的 TLS 由 nginx 终止，xray sec=none 不需要 PEM。
// reality 不走传统证书。self_tcp/self_udp 需要 xray 持有证书。
func (tc TerminationClass) NeedsCertBundle() bool {
	switch tc {
	case TerminationSelfTCP, TerminationSelfUDP, TerminationNginxPlusXray:
		return true
	default:
		return false
	}
}

// NeedsTLSStrip 判断该 TerminationClass 是否需要剥离 xray inbound 的 TLS。
// cf_edge: 剥离（cloudflared 明文 HTTP 回源）
// nginx: 剥离（P4 前架构，nginx 终止 TLS 后 proxy_pass http）
// nginx_plus_xray: 不剥离（P4 后架构，xray 持证书，nginx proxy_pass https）
// self_tcp/self_udp/reality: 不剥离（xray 自终止 TLS/REALITY）
func (tc TerminationClass) NeedsTLSStrip() bool {
	switch tc {
	case TerminationCFEdge, TerminationNginx:
		return true
	default:
		return false
	}
}
