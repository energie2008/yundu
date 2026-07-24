package nodespec

import (
	"reflect"
	"testing"
)

func TestDeriveALPN(t *testing.T) {
	tests := []struct {
		name         string
		protocol     string
		transport    string
		wantALPN     []string
	}{
		// QUIC 系协议 → h3
		{"hysteria2+quic", "hysteria2", "quic", []string{"h3"}},
		{"hysteria2+tcp(unusual)", "hysteria2", "tcp", []string{"h3"}},
		{"tuic+quic", "tuic", "quic", []string{"h3"}},
		{"TUIC+TCP(unusual)", "TUIC", "TCP", []string{"h3"}},

		// WS/HTTPUpgrade → http/1.1
		{"vless+ws", "vless", "ws", []string{"http/1.1"}},
		{"trojan+ws", "trojan", "ws", []string{"http/1.1"}},
		{"vless+httpupgrade", "vless", "httpupgrade", []string{"http/1.1"}},
		{"VMess+WS", "VMess", "WS", []string{"http/1.1"}},

		// 默认 → h2,http/1.1
		{"vless+tcp", "vless", "tcp", []string{"h2", "http/1.1"}},
		{"trojan+tcp+tls", "trojan", "tcp", []string{"h2", "http/1.1"}},
		{"vless+grpc", "vless", "grpc", []string{"h2", "http/1.1"}},
		{"vless+xhttp", "vless", "xhttp", []string{"h2", "http/1.1"}},
		{"vless+reality(xhttp)", "vless", "xhttp", []string{"h2", "http/1.1"}},
		{"anytls+tcp", "anytls", "tcp", []string{"h2", "http/1.1"}},
		{"mieru+tcp", "mieru", "tcp", []string{"h2", "http/1.1"}},

		// 边界情况
		{"empty protocol+transport", "", "", []string{"h2", "http/1.1"}},
		{"spaces", " hysteria2 ", " quic ", []string{"h3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveALPN(tt.protocol, tt.transport)
			if !reflect.DeepEqual(got, tt.wantALPN) {
				t.Errorf("DeriveALPN(%q, %q) = %v, want %v",
					tt.protocol, tt.transport, got, tt.wantALPN)
			}
		})
	}
}

func TestDeriveALPNReturnsNewSlice(t *testing.T) {
	// 确保每次调用返回新切片,调用方修改不影响其他调用方
	a := DeriveALPN("vless", "tcp")
	b := DeriveALPN("vless", "tcp")
	a[0] = "modified"
	if b[0] == "modified" {
		t.Error("DeriveALPN should return independent slices")
	}
}
