package client

import (
	"testing"

	"github.com/airport-panel/subscription-service/internal/model"
)

// Karing/Hiddify 的 UA 会附带 mihomo/clash-verge/ClashMeta/sing-box 等兼容 token，
// 必须优先识别为 Karing/Hiddify（返回 sing-box JSON），而不是被误判为 Clash/Mihomo。
func TestDetectClient_KaringHiddifyPrecedence(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want model.ClientType
	}{
		{
			name: "karing windows composite UA",
			ua:   "Karing/1.2.22.2502 platform/windows;mihomo/1.19.27;clash-verge;FLClash;ClashMeta;v2ray;sing-box 1.13.0;NekoBox/Android/1.4.1 (Prefer ClashMeta Format);HiddifyNext",
			want: model.ClientTypeKaring,
		},
		{
			name: "karing ios composite UA",
			ua:   "Karing/1.2.22.2502 platform/ios;mihomo/1.19.27;clash-verge;FLClash;ClashMeta;v2ray;sing-box 1.13.0;NekoBox/Android/1.4.1 (Prefer ClashMeta Format);HiddifyNext",
			want: model.ClientTypeKaring,
		},
		{
			name: "hiddify next UA",
			ua:   "HiddifyNext/4.1.1 (windows) like ClashMeta v2ray sing-box",
			want: model.ClientTypeHiddifyNext,
		},
		{
			name: "hiddify plain UA",
			ua:   "hiddify/2.0.0 like ClashMeta",
			want: model.ClientTypeHiddify,
		},
		{
			name: "pure karing",
			ua:   "Karing/1.2.3",
			want: model.ClientTypeKaring,
		},
		{
			name: "pure mihomo still detected as mihomo",
			ua:   "mihomo/1.19.27",
			want: model.ClientTypeMihomo,
		},
		{
			name: "clash verge still detected as clash verge",
			ua:   "Clash Verge/1.7.0",
			want: model.ClientTypeClashVerge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectClient(tc.ua)
			if got != tc.want {
				t.Fatalf("DetectClient(%q) = %v, want %v", tc.ua, got, tc.want)
			}
		})
	}
}

// ClientToRenderer 映射校验：Karing/Hiddify -> singbox（而非 clashmeta）。
func TestClientToRenderer_KaringHiddifySingbox(t *testing.T) {
	if got := ClientToRenderer(model.ClientTypeKaring); got != "singbox" {
		t.Fatalf("Karing renderer = %q, want singbox", got)
	}
	if got := ClientToRenderer(model.ClientTypeHiddify); got != "singbox" {
		t.Fatalf("Hiddify renderer = %q, want singbox", got)
	}
	if got := ClientToRenderer(model.ClientTypeHiddifyNext); got != "singbox" {
		t.Fatalf("HiddifyNext renderer = %q, want singbox", got)
	}
}
