package nodespec

import (
	"testing"
)

// TestVerifyP17PresetsLoaded 验证 17 个 v2 协议预设全部加载且字段完整
func TestVerifyP17PresetsLoaded(t *testing.T) {
	reg, err := LoadPresetRegistry("../presets")
	if err != nil {
		t.Fatalf("LoadPresetRegistry failed: %v", err)
	}

	// 期望的 17 个预设 ID（P01-P17）
	expectedIDs := []string{
		"P01-vless-reality-vision",
		"P02-trojan-tls",
		"P03-vless-ws-tls",
		"P04-trojan-ws-tls",
		"P05-anytls",
		"P06-xhttp-up-cdn-down-reality",
		"P07-xhttp-up-reality-down-cdn",
		"P08-xhttp-tls-cdn",
		"P09-xhttp-stream-up-reality-xmux",
		"P10-vless-httpupgrade-tls",
		"P11-hysteria2",
		"P12-tuic-v5",
		"P13-warp-masque-overlay",
		"P14-vless-ws-tls-ss2022",
		"P15-reality-ss2022",
		"P16-trojan-grpc-tls",
		"P17-xhttp-stream-up-reality-xmux-v4v6",
	}

	for _, id := range expectedIDs {
		p, ok := reg.Get(id)
		if !ok {
			t.Errorf("❌ 预设 %s 未找到", id)
			continue
		}
		// 验证基础字段非空
		if p.Name == "" {
			t.Errorf("❌ %s: name 为空", id)
		}
		if p.Description == "" {
			t.Errorf("❌ %s: description 为空", id)
		}
		if len(p.ClientSupport) == 0 {
			t.Errorf("❌ %s: client_support 为空", id)
		}
		// 验证 base_spec 关键字段
		// P07 上行 REALITY 用 8443，下行 CDN 用 443；其余默认 443
		expectedPort := 443
		if id == "P07-xhttp-up-reality-down-cdn" {
			expectedPort = 8443
		}
		if p.BaseSpec.Port != expectedPort {
			t.Errorf("❌ %s: port=%d, 期望 %d", id, p.BaseSpec.Port, expectedPort)
		}
		if p.BaseSpec.TrafficRate != 1.0 {
			t.Errorf("❌ %s: traffic_rate=%v, 期望 1.0", id, p.BaseSpec.TrafficRate)
		}
		// allow_udp=false 的协议：Trojan(TCP/WS不支持UDP转发)、AnyTLS、Trojan+gRPC
		allowUDPFalse := map[string]bool{
			"P02-trojan-tls":     true,
			"P04-trojan-ws-tls":  true,
			"P05-anytls":         true,
			"P16-trojan-grpc-tls": true,
		}
		if allowUDPFalse[id] {
			if p.BaseSpec.AllowUDP {
				t.Errorf("❌ %s: allow_udp=true, 期望 false", id)
			}
		} else {
			if !p.BaseSpec.AllowUDP {
				t.Errorf("❌ %s: allow_udp=false, 期望 true", id)
			}
		}
		if !p.BaseSpec.IsVisible {
			t.Errorf("❌ %s: is_visible=false, 期望 true", id)
		}
		// v2 字段验证
		if p.DeploymentProfile == "" {
			t.Errorf("❌ %s: deployment_profile 为空", id)
		}
		if p.Enhancement == nil {
			t.Errorf("❌ %s: enhancement 为 nil", id)
		} else {
			if p.Enhancement.UTLS == "" {
				t.Errorf("❌ %s: enhancement.utls 为空", id)
			}
			if p.Enhancement.ECH == "" {
				t.Errorf("❌ %s: enhancement.ech 为空", id)
			}
			if p.Enhancement.Multiplex == "" {
				t.Errorf("❌ %s: enhancement.multiplex 为空", id)
			}
		}
		t.Logf("✅ %s: proto=%s transport=%s security=%s profile=%s utls=%s ech=%s mux=%s",
			id, p.Protocol, p.Transport, p.Security,
			p.DeploymentProfile,
			p.Enhancement.UTLS, p.Enhancement.ECH, p.Enhancement.Multiplex)
	}

	// 验证协议无雷同（用更精确的区分：包含 transport mode / encryption layer / split mode）
	combos := make(map[string]string)
	for _, id := range expectedIDs {
		p, _ := reg.Get(id)
		if p == nil {
			continue
		}
		// P13 是 overlay 层，protocol=vless+tcp+none 是占位，不算雷同
		if id == "P13-warp-masque-overlay" {
			continue
		}
		// 用 deployment_profile + proto + transport + security + xhttp mode 区分
		key := string(p.DeploymentProfile) + "|" + string(p.Protocol) + "+" + string(p.Transport) + "+" + string(p.Security)
		// 对 xhttp 协议进一步用 mode 区分
		if p.Transport == TransportXHTTP && p.BaseSpec.Transport.XHTTP != nil {
			key += "|" + p.BaseSpec.Transport.XHTTP.Mode
			// split mode（有 download_settings）进一步区分
			if p.BaseSpec.Transport.XHTTP.DownloadSettings != nil {
				key += "|split"
			}
		}
		// P14 有 SS2022 encryption layer 区分
		if id == "P14-vless-ws-tls-ss2022" {
			key += "|ss2022"
		}
		// P17 有 v4/v6 双栈分离区分
		if id == "P17-xhttp-stream-up-reality-xmux-v4v6" {
			key += "|v4v6-dualstack"
		}
		if existing, exists := combos[key]; exists {
			t.Errorf("❌ 协议组合雷同: %s 和 %s 都是 %s", existing, id, key)
		}
		combos[key] = id
	}
	t.Logf("✅ 协议组合唯一性检查通过（%d 个唯一组合）", len(combos))
}

// TestVerifyForbiddenCombos 验证禁止组合配置正确
func TestVerifyForbiddenCombos(t *testing.T) {
	reg, err := LoadPresetRegistry("../presets")
	if err != nil {
		t.Fatalf("LoadPresetRegistry failed: %v", err)
	}

	// 期望有 forbidden_combos 的预设
	expectedForbidden := map[string][]string{
		"P01-vless-reality-vision":          {"cf_saas", "cf_argo"},
		"P02-trojan-tls":                    {"cf_saas"},
		"P06-xhttp-up-cdn-down-reality":     {"cf_saas", "cf_argo"},
		"P07-xhttp-up-reality-down-cdn":     {"cf_saas", "cf_argo"},
		"P09-xhttp-stream-up-reality-xmux":  {"cf_saas", "cf_argo"},
		"P11-hysteria2":                     {"cf_saas", "cf_argo"},
		"P12-tuic-v5":                       {"cf_saas", "cf_argo"},
		"P15-reality-ss2022":                {"cf_saas", "cf_argo"},
		"P17-xhttp-stream-up-reality-xmux-v4v6": {"cf_saas", "cf_argo"},
	}

	for id, expectedProfiles := range expectedForbidden {
		p, ok := reg.Get(id)
		if !ok {
			t.Errorf("❌ %s 未找到", id)
			continue
		}
		if len(p.ForbiddenCombos) == 0 {
			t.Errorf("❌ %s: forbidden_combos 为空", id)
			continue
		}
		for _, profile := range expectedProfiles {
			if _, exists := p.ForbiddenCombos[DeploymentProfile(profile)]; !exists {
				t.Errorf("❌ %s: forbidden_combos 缺少 %s", id, profile)
			}
		}
		t.Logf("✅ %s: forbidden_combos 包含 %d 项", id, len(p.ForbiddenCombos))
	}
}

// TestVerifyUIWarnings 验证高危字段警告
func TestVerifyUIWarnings(t *testing.T) {
	reg, err := LoadPresetRegistry("../presets")
	if err != nil {
		t.Fatalf("LoadPresetRegistry failed: %v", err)
	}

	// 期望有 ui_warning 的预设
	expectedWarnings := []string{
		"P06-xhttp-up-cdn-down-reality",
		"P07-xhttp-up-reality-down-cdn",
		"P09-xhttp-stream-up-reality-xmux",
		"P13-warp-masque-overlay",
		"P17-xhttp-stream-up-reality-xmux-v4v6",
	}

	for _, id := range expectedWarnings {
		p, ok := reg.Get(id)
		if !ok {
			t.Errorf("❌ %s 未找到", id)
			continue
		}
		if p.UIWarning == "" {
			t.Errorf("❌ %s: ui_warning 为空", id)
			continue
		}
		t.Logf("✅ %s: ui_warning = %q", id, p.UIWarning[:min(40, len(p.UIWarning))]+"...")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
