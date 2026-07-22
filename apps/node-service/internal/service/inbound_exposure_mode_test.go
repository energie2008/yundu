package service

import (
	"testing"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/subscription/kernelrender"
)

// 阶段2.3 单元测试：4 场景覆盖 inbound 级 TLS 剥离判定
// 场景：
//  1. 纯 CDN 节点 → 主 inbound 判定为 cdn_saas，应剥离
//  2. 纯直连节点 → 主 inbound 判定为 direct，不剥离
//  3. split mode 上行（主 inbound tag="in-<code>"）→ cdn_saas，应剥离
//  4. split mode 下行（显式 _inbound_role="downstream"）→ reality/direct，不剥离（p06 事故核心）
//
// 阶段3更新：下行inbound使用 _inbound_role 显式字段标识，tag后缀仅作防御性fallback。
func TestDetermineInboundExposureMode(t *testing.T) {
	mkNode := func(em string, dlEM *string, cfg map[string]interface{}) *model.Node {
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		if em != "" {
			cfg["exposure_mode"] = em
		}
		n := &model.Node{
			Code:          "test-node",
			ProtocolType:  "vless",
			TransportType: "xhttp",
			Port:          443,
			ConfigJSON:    cfg,
		}
		if em != "" {
			n.ExposureMode = &em
		}
		if dlEM != nil {
			n.DownstreamExposureMode = dlEM
			cfg["downstream_exposure_mode"] = *dlEM
		}
		return n
	}

	mainTag := "in-test-node"
	dlTag := "in-test-node" + kernelrender.DownstreamTagSuffix

	mkInb := func(tag string, isDownstream bool) map[string]interface{} {
		m := map[string]interface{}{"tag": tag}
		if isDownstream {
			m["_inbound_role"] = "downstream"
		}
		return m
	}

	cases := []struct {
		name      string
		node      *model.Node
		inbMap    map[string]interface{}
		wantEM    string
		wantStrip bool
	}{
		{
			name:      "1_pure_cdn_main_inbound_should_strip",
			node:      mkNode("cdn_saas", nil, map[string]interface{}{"cdn_address": "cdn.example.com"}),
			inbMap:    mkInb(mainTag, false),
			wantEM:    "cdn_saas",
			wantStrip: true,
		},
		{
			name:      "2_pure_direct_main_inbound_should_not_strip",
			node:      mkNode("direct", nil, map[string]interface{}{"host": "captive.apple.com"}),
			inbMap:    mkInb(mainTag, false),
			wantEM:    "direct",
			wantStrip: false,
		},
		{
			name:      "3_split_upstream_main_inbound_cdn_saas_should_strip",
			node:      mkNode("cdn_saas", strPtr("direct"), nil),
			inbMap:    mkInb(mainTag, false),
			wantEM:    "cdn_saas",
			wantStrip: true,
		},
		{
			name:      "4_split_downstream_dl_inbound_direct_should_NOT_strip_p06_fix",
			node:      mkNode("cdn_saas", strPtr("direct"), nil),
			inbMap:    mkInb(dlTag, true),
			wantEM:    "direct",
			wantStrip: false,
		},
		{
			name:      "5_split_downstream_dl_inbound_reality_should_NOT_strip",
			node:      mkNode("cdn_saas", strPtr("reality"), nil),
			inbMap:    mkInb(dlTag, true),
			wantEM:    "reality",
			wantStrip: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotEM := determineInboundExposureMode(c.node, c.inbMap)
			if gotEM != c.wantEM {
				t.Errorf("determineInboundExposureMode=%q, want %q", gotEM, c.wantEM)
			}
			// P1-1: 剥离判定改为 argo_tunnel 和 cdn/cdn_saas 独立判断
			gotStrip := shouldStripTLSForArgoTunnel(gotEM) || shouldStripTLSForNginxVhost(gotEM)
			if gotStrip != c.wantStrip {
				t.Errorf("strip check for %q = %v, want %v", gotEM, gotStrip, c.wantStrip)
			}
		})
	}
}

// 验证 is_split_mode 约束：IsSplitMode=true 但 downstream_exposure_mode 为空时，
// 下行 inbound 不应该错误地进入剥离路径。
// 但按设计，is_split_mode 不参与判定，只有 downstream_exposure_mode 才是安全判定的真相源。
func TestIsSplitMode_DoesNotAffectStripLogic(t *testing.T) {
	dlTag := "in-test" + kernelrender.DownstreamTagSuffix
	n := &model.Node{
		Code:          "test",
		ExposureMode:  strPtr("cdn_saas"),
		IsSplitMode:   true,
		ConfigJSON:    map[string]interface{}{"exposure_mode": "cdn_saas"},
	}
	inbMap := map[string]interface{}{
		"tag":           dlTag,
		"_inbound_role": "downstream",
	}
	// 即使 IsSplitMode=true，downstream_exposure_mode 为空时，
	// 下行 inbound 判定为 "direct"（回退默认），不剥离——这是安全兜底。
	em := determineInboundExposureMode(n, inbMap)
	if shouldStripTLSForArgoTunnel(em) || shouldStripTLSForNginxVhost(em) {
		t.Errorf("downstream inbound should never strip, got em=%q", em)
	}
}

// P2 校验：自签证书 + cdn_saas 应报错
func TestValidateExposureMode_SelfSignedCDN(t *testing.T) {
	n := &model.Node{
		Code:         "selfsigned-node",
		ExposureMode: strPtr("cdn_saas"),
		ConfigJSON:   map[string]interface{}{"cert_type": "self_signed", "exposure_mode": "cdn_saas"},
	}
	if err := validateExposureMode(n); err == nil {
		t.Error("self_signed + cdn_saas should return error")
	}

	n2 := &model.Node{
		Code:         "selfsigned-direct",
		ExposureMode: strPtr("direct"),
		ConfigJSON:   map[string]interface{}{"cert_type": "self_signed", "exposure_mode": "direct"},
	}
	if err := validateExposureMode(n2); err != nil {
		t.Errorf("self_signed + direct should pass, got: %v", err)
	}
}
