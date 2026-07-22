package service

import (
	"testing"

	"github.com/airport-panel/node-service/internal/model"
)

// P0 修复验证：isCDNNode 必须以持久化的 exposure_mode 为唯一真相源，
// REALITY 直连节点（exposure_mode=direct）绝不剥离，CDN/tunnel 才剥离。
func TestIsCDNNode_ExposureModeDriven(t *testing.T) {
	mk := func(em string, extra map[string]interface{}) *model.Node {
		cfg := map[string]interface{}{}
		for k, v := range extra {
			cfg[k] = v
		}
		if em != "" {
			cfg["exposure_mode"] = em
		}
		sp := 9452
		return &model.Node{
			ProtocolType:  "vless",
			TransportType: "xhttp",
			Port:          443,
			ServerPort:    &sp,
			ConfigJSON:    cfg,
		}
	}

	cases := []struct {
		name string
		node *model.Node
		want bool
	}{
		// REALITY 直连：host 为伪装域名 + ServerPort != Port，旧逻辑会误判为 CDN
		{"direct_reality_not_strip", mk("direct", map[string]interface{}{"host": "captive.apple.com"}), false},
		{"direct_plain_not_strip", mk("direct", nil), false},
		// CDN 节点：显式 cdn_saas / cdn
		{"cdn_saas_strip", mk("cdn_saas", map[string]interface{}{"cdn_address": "y3.dannelblog.na.am"}), true},
		{"cdn_alias_strip", mk("cdn", map[string]interface{}{"cdn_address": "y3.dannelblog.na.am"}), true},
		// argo_tunnel 显式
		{"argo_tunnel_strip", mk("argo_tunnel", nil), true},
		// 未持久化但带 tunnel 凭证（历史节点回退）
		{"legacy_tunnel_cred_strip", mk("", map[string]interface{}{"cloudflared_tunnel_id": "abc"}), true},
		// 未持久化但有 cdn_address（历史 CDN 节点，维持现状剥离，防回归）
		{"legacy_cdn_address_strip", mk("", map[string]interface{}{"cdn_address": "y3.dannelblog.na.am"}), true},
		// 未持久化只有伪装域名 host、无 cdn_address（REALITY direct 历史形态）→ 不剥
		{"legacy_reality_host_not_strip", mk("", map[string]interface{}{"host": "captive.apple.com"}), false},
		// 未持久化无任何标志 → 默认不剥（安全兜底）
		{"legacy_unknown_not_strip", mk("", nil), false},
	}
	for _, c := range cases {
		if got := isCDNNode(c.node); got != c.want {
			t.Errorf("%s: isCDNNode=%v, want %v", c.name, got, c.want)
		}
	}
}
