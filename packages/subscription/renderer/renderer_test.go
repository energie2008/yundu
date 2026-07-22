package renderer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

func testNodes() []nodespec.NodeSpec {
	return []nodespec.NodeSpec{
		{
			ID: "vless-ws-tls-1", Code: "hk1", Name: "HK-VLESS-WS-TLS",
			Protocol: nodespec.ProtocolVLESS, Address: "hk1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/ws", Host: "hk1.example.com"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "hk1.example.com", Fingerprint: "chrome", ALPN: []string{"h2", "http/1.1"}},
			Credentials: nodespec.VLESSCredentials{UUID: "00000000-0000-0000-0000-000000000001"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "vless-reality-1", Code: "jp1", Name: "JP-VLESS-Reality",
			Protocol: nodespec.ProtocolVLESS, Address: "jp1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP},
			Security: nodespec.SecurityReality, Reality: &nodespec.RealityConfig{SNI: "www.microsoft.com", PublicKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", ShortID: "12345678abcdef00", Fingerprint: "chrome"},
			Credentials: nodespec.VLESSCredentials{UUID: "00000000-0000-0000-0000-000000000002", Flow: "xtls-rprx-vision"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "trojan-ws-tls-1", Code: "us1", Name: "US-Trojan-WS",
			Protocol: nodespec.ProtocolTrojan, Address: "us1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/trojan"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "us1.example.com", Fingerprint: "chrome"},
			Credentials: nodespec.TrojanCredentials{Password: "trojan-password-long-enough-123"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "ss-aes-gcm-1", Code: "sg1", Name: "SG-SS-AES256GCM",
			Protocol: nodespec.ProtocolShadowsocks, Address: "sg1.example.com", Port: 8388,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP},
			Security: nodespec.SecurityNone,
			Credentials: nodespec.ShadowsocksCredentials{Password: "ss-pass-12345", Method: "aes-256-gcm"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "hy2-1", Code: "kr1", Name: "KR-Hysteria2",
			Protocol: nodespec.ProtocolHysteria2, Address: "kr1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportQUIC},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "kr1.example.com"},
			Credentials: nodespec.Hysteria2Credentials{Password: "hy2-password-secret"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "tuic-1", Code: "de1", Name: "DE-TUIC-v5",
			Protocol: nodespec.ProtocolTUIC, Address: "de1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportQUIC},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "de1.example.com", ALPN: []string{"h3"}},
			Credentials: nodespec.TUICCredentials{UUID: "00000000-0000-0000-0000-000000000003", Password: "tuic-pass"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "vmess-ws-tls-1", Code: "uk1", Name: "UK-VMess-WS",
			Protocol: nodespec.ProtocolVMess, Address: "uk1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/vmess", Host: "uk1.example.com"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "uk1.example.com"},
			Credentials: nodespec.VMessCredentials{UUID: "00000000-0000-0000-0000-000000000004", AlterID: 0},
			TrafficRate: 1.0, AllowUDP: true,
		},
	}
}

func TestAllRenderersSmoke(t *testing.T) {
	nodes := testNodes()
	renderers := []string{"uri", "clash", "singbox", "quantumult", "shadowrocket", "surge", "loon"}
	for _, name := range renderers {
		r, err := Get(name)
		if err != nil {
			t.Errorf("renderer %s not found: %v", name, err)
			continue
		}
		out, err := r.Render(nodes)
		if err != nil {
			t.Errorf("renderer %s render error: %v", name, err)
			continue
		}
		if len(out) == 0 {
			t.Errorf("renderer %s produced empty output", name)
		}
		t.Logf("[%s] output size: %d bytes", name, len(out))
	}
}

func TestRendererRegistry(t *testing.T) {
	names := List()
	if len(names) != 7 {
		t.Errorf("expected 7 renderers, got %d: %v", len(names), names)
	}
}

func TestGoldenGenerateAndCompare(t *testing.T) {
	tmpDir := t.TempDir()
	nodes := testNodes()
	formats := []string{"uri", "clash", "singbox", "shadowrocket", "surge", "loon"}
	if err := UpdateGoldenFiles(tmpDir, nodes, formats); err != nil {
		t.Fatalf("generate golden files: %v", err)
	}

	fp := filepath.Join(tmpDir, "nodes.golden.json")
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	var suite GoldenSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatalf("parse golden json: %v", err)
	}
	if len(suite.Cases) != len(nodes) {
		t.Errorf("expected %d cases, got %d", len(nodes), len(suite.Cases))
	}
	for _, c := range suite.Cases {
		for f := range c.Expected {
			if c.Expected[f] == "" {
				t.Errorf("[%s/%s] empty output", c.Name, f)
			}
		}
	}
}

func TestRendererContentType(t *testing.T) {
	expected := map[string]string{
		"uri": "text/plain",
		"clash": "text/yaml",
		"singbox": "application/json",
		"quantumult": "text/plain",
		"shadowrocket": "text/plain",
		"surge": "text/plain",
		"loon": "text/plain",
	}
	for name, ct := range expected {
		r, err := Get(name)
		if err != nil {
			t.Errorf("renderer %s: %v", name, err)
			continue
		}
		if !containsSubstr(r.ContentType(), ct) {
			t.Errorf("[%s] expected content-type to contain %s, got %s", name, ct, r.ContentType())
		}
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
