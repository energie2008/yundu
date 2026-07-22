package service

import (
	"github.com/airport-panel/node-service/internal/model"
)

// ExposurePolicy P2-4: 声明式渲染策略表。
//
// 将 exposure_mode → 渲染策略 的映射收敛为单一静态表，
// 替代分散在各处的 switch/if exposureMode 判断。
//
// 所有需要根据 exposure_mode 做渲染决策的代码都应通过
// GetExposurePolicy(exposureMode) 查询此表，而非自行 switch。
//
// 策略字段说明：
//   - ListenAddress:    xray inbound 应监听的地址
//     - "127.0.0.1": 仅本地回环（CDN/nginx/cloudflared 回源场景）
//     - "0.0.0.0":   公网监听（direct 直连场景）
//     - "::":        IPv6 任意地址（argo_tunnel cloudflared 回源需要）
//   - StripTLS:          是否剥离 xray inbound 的 TLS（已废弃，改用 NeedsTLSStrip）
//   - NeedsNginxVhost:   是否需要生成 nginx 8445 HTTP vhost（location 路由）
//   - NeedsStreamSNI:    是否需要 nginx 443 stream SNI 分流
//   - NeedsCertBundle:   是否需要从 cert_bundles 注入证书 PEM 到 xray
//   - CertHeldBy:        证书持有方（"xray" / "nginx" / "cf_edge" / "none"）
type ExposurePolicy struct {
	ListenAddress    string
	StripTLS         bool // 兼容字段，等价于 NeedsTLSStrip
	NeedsNginxVhost  bool
	NeedsStreamSNI   bool
	NeedsCertBundle  bool
	CertHeldBy       string
}

// exposurePolicyMap P2-4: exposure_mode → ExposurePolicy 静态映射表。
//
// 这是整个渲染层暴露策略的单一事实源（Single Source of Truth）。
// 新增 exposure_mode 时只需在此表添加一行，所有渲染决策自动更新。
//
// 映射规则（与 TerminationClass 一致）：
//   - direct:     xray 自终止 TLS，公网监听，证书由 xray 持有
//   - reality:    xray REALITY 握手，公网监听，无传统证书
//   - cdn:        nginx 8445 终止 TLS，本地回环，证书由 nginx 持有
//   - cdn_saas:   同 cdn
//   - argo_tunnel: CF 边缘终止 TLS，IPv6 回环（cloudflared），xray 无 TLS
var exposurePolicyMap = map[string]ExposurePolicy{
	"direct": {
		ListenAddress:   "0.0.0.0",
		StripTLS:        false,
		NeedsNginxVhost: false,
		NeedsStreamSNI:  true,
		NeedsCertBundle: true,
		CertHeldBy:      "xray",
	},
	"reality": {
		ListenAddress:   "0.0.0.0",
		StripTLS:        false,
		NeedsNginxVhost: false,
		NeedsStreamSNI:  true,
		NeedsCertBundle: false,
		CertHeldBy:      "none",
	},
	"cdn": {
		ListenAddress:   "127.0.0.1",
		StripTLS:        true,
		NeedsNginxVhost: true,
		NeedsStreamSNI:  true,
		NeedsCertBundle: false,
		CertHeldBy:      "nginx",
	},
	"cdn_saas": {
		ListenAddress:   "127.0.0.1",
		StripTLS:        true,
		NeedsNginxVhost: true,
		NeedsStreamSNI:  true,
		NeedsCertBundle: false,
		CertHeldBy:      "nginx",
	},
	"argo_tunnel": {
		ListenAddress:   "::",
		StripTLS:        true,
		NeedsNginxVhost: false,
		NeedsStreamSNI:  false,
		NeedsCertBundle: false,
		CertHeldBy:      "cf_edge",
	},
	"none": {
		ListenAddress:   "0.0.0.0",
		StripTLS:        false,
		NeedsNginxVhost: false,
		NeedsStreamSNI:  true,
		NeedsCertBundle: false,
		CertHeldBy:      "none",
	},
}

// defaultExposurePolicy 未知 exposure_mode 的默认策略（等同 direct）。
var defaultExposurePolicy = ExposurePolicy{
	ListenAddress:   "0.0.0.0",
	StripTLS:        false,
	NeedsNginxVhost: false,
	NeedsStreamSNI:  true,
	NeedsCertBundle: true,
	CertHeldBy:      "xray",
}

// GetExposurePolicy P2-4: 根据 exposure_mode 查询渲染策略。
// 未知 mode 回退到 defaultExposurePolicy（等同 direct）。
func GetExposurePolicy(exposureMode string) ExposurePolicy {
	if p, ok := exposurePolicyMap[exposureMode]; ok {
		return p
	}
	return defaultExposurePolicy
}

// GetExposurePolicyForNode P2-4: 根据 node 的 exposure_mode 查询渲染策略。
// 自动解析节点的 exposure_mode（含三级回退逻辑）。
func GetExposurePolicyForNode(node *model.Node) ExposurePolicy {
	if node == nil {
		return defaultExposurePolicy
	}
	return GetExposurePolicy(determineExposureMode(node))
}

// AllExposurePolicies P2-4: 返回所有已注册的 exposure_mode 列表（用于测试和文档）。
func AllExposurePolicies() []string {
	return []string{"direct", "reality", "cdn", "cdn_saas", "argo_tunnel", "none"}
}

// IsKnownExposureMode P2-4: 判断 exposure_mode 是否为已知值。
// 用于节点保存时的输入校验。
func IsKnownExposureMode(mode string) bool {
	_, ok := exposurePolicyMap[mode]
	return ok
}
