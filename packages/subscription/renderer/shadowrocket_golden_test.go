package renderer

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

func shadowrocketTestNodes() []nodespec.NodeSpec {
	return []nodespec.NodeSpec{
		{
			ID: "sr-vless-ws-tls", Code: "hk1", Name: "HK-VLESS-WS-TLS",
			Protocol: nodespec.ProtocolVLESS, Address: "hk1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/ws", Host: "hk1.example.com"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "hk1.example.com", Fingerprint: "chrome", ALPN: []string{"h2", "http/1.1"}},
			Credentials: nodespec.VLESSCredentials{UUID: "00000000-0000-0000-0000-000000000001"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "sr-vless-reality", Code: "jp1", Name: "JP-VLESS-Reality",
			Protocol: nodespec.ProtocolVLESS, Address: "jp1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP},
			Security: nodespec.SecurityReality, Reality: &nodespec.RealityConfig{SNI: "www.microsoft.com", PublicKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", ShortID: "12345678abcdef00", Fingerprint: "chrome"},
			Credentials: nodespec.VLESSCredentials{UUID: "00000000-0000-0000-0000-000000000002", Flow: "xtls-rprx-vision"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "sr-trojan-ws-tls", Code: "us1", Name: "US-Trojan-WS",
			Protocol: nodespec.ProtocolTrojan, Address: "us1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/trojan?extra=param", Host: "us1.example.com"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "us1.example.com", Fingerprint: "firefox"},
			Credentials: nodespec.TrojanCredentials{Password: "trojan-password-long-enough-123"},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "sr-vmess-ws-tls", Code: "uk1", Name: "UK-VMess-WS",
			Protocol: nodespec.ProtocolVMess, Address: "uk1.example.com", Port: 443,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportWS, WS: &nodespec.WSConfig{Path: "/vmess?ed=2048", Host: "uk1.example.com"}},
			Security: nodespec.SecurityTLS, TLS: &nodespec.TLSConfig{SNI: "uk1.example.com", Fingerprint: "safari"},
			Credentials: nodespec.VMessCredentials{UUID: "00000000-0000-0000-0000-000000000004", AlterID: 0},
			TrafficRate: 1.0, AllowUDP: true,
		},
		{
			ID: "sr-ss-aes-gcm", Code: "sg1", Name: "SG-SS-AES256GCM",
			Protocol: nodespec.ProtocolShadowsocks, Address: "sg1.example.com", Port: 8388,
			Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP},
			Security: nodespec.SecurityNone,
			Credentials: nodespec.ShadowsocksCredentials{Password: "ss-pass-12345", Method: "aes-256-gcm"},
			TrafficRate: 1.0, AllowUDP: true,
		},
	}
}

func TestShadowrocketGoldenProtocols(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()

	for _, n := range nodes {
		t.Run(n.ID, func(t *testing.T) {
			line, err := r.RenderNode(n)
			if err != nil {
				t.Fatalf("render node %s: %v", n.ID, err)
			}
			t.Logf("[%s] %s", n.Protocol, line)

			validateShadowrocketURI(t, line, n)
		})
	}
}

func TestShadowrocketOutputSanitization(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()
	out, err := r.Render(nodes)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	content := string(out)

	validateShadowrocketOutputFormat(t, content)
}

func TestShadowrocketNoBOM(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()
	out, _ := r.Render(nodes)

	if len(out) >= 3 && out[0] == 0xEF && out[1] == 0xBB && out[2] == 0xBF {
		t.Error("output contains UTF-8 BOM")
	}
}

func TestShadowrocketLineEndings(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()
	out, _ := r.Render(nodes)
	content := string(out)

	if strings.Contains(content, "\r\n") {
		t.Error("output contains CRLF line endings")
	}
	if strings.Contains(content, "\r") {
		t.Error("output contains CR line endings")
	}
}

func TestShadowrocketNoTrailingWhitespace(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()
	out, _ := r.Render(nodes)
	lines := strings.Split(string(out), "\n")

	for i, line := range lines {
		if line == "" {
			continue
		}
		last := line[len(line)-1]
		if last == ' ' || last == '\t' {
			t.Errorf("line %d ends with whitespace: %q", i+1, line)
		}
	}
}

func TestShadowrocketNoConsecutiveBlankLines(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()
	out, _ := r.Render(nodes)
	content := string(out)

	if strings.Contains(content, "\n\n\n") {
		t.Error("output contains consecutive blank lines")
	}
}

func TestShadowrocketVMessNotBase64(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()

	for _, n := range nodes {
		if n.Protocol != nodespec.ProtocolVMess {
			continue
		}
		line, err := r.RenderNode(n)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "vmess://") {
			rest := strings.TrimPrefix(line, "vmess://")
			u, err := url.Parse(line)
			if err != nil {
				t.Fatalf("parse vmess uri: %v", err)
			}
			if u.User == nil {
				t.Error("vmess uri should have user info (uuid) in non-base64 format")
			}
			t.Logf("vmess non-base64 format verified, uuid in userinfo: %s", u.User.Username())
			_ = rest
		}
	}
}

func TestShadowrocketFingerprintLowercase(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()

	for _, n := range nodes {
		line, err := r.RenderNode(n)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(line)
		if err != nil {
			continue
		}
		q := u.Query()
		fp := q.Get("fp")
		if fp != "" && fp != strings.ToLower(fp) {
			t.Errorf("[%s] fp parameter should be lowercase, got %q", n.ID, fp)
		}
	}
}

func TestShadowrocketWSPathNoQuery(t *testing.T) {
	r := NewShadowrocketRenderer()
	nodes := shadowrocketTestNodes()

	for _, n := range nodes {
		if n.Transport.Type != nodespec.TransportWS || n.Transport.WS == nil {
			continue
		}
		line, err := r.RenderNode(n)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(line)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		path := q.Get("path")
		if strings.Contains(path, "?") {
			t.Errorf("[%s] ws path should not contain query parameters, got %q", n.ID, path)
		}
	}
}

func TestShadowrocketUserinfoFormat(t *testing.T) {
	tests := []struct {
		name     string
		userinfo string
		valid    bool
	}{
		{"empty", "", false},
		{"upload only", "upload=1234", false},
		{"valid complete", "upload=1024; download=2048; total=1073741824; expire=1893456000", true},
		{"no spaces", "upload=1024;download=2048;total=1073741824;expire=1893456000", true},
	}

	userinfoRegex := regexp.MustCompile(`upload=\d+;\s*download=\d+;\s*total=\d+;\s*expire=\d+`)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := userinfoRegex.MatchString(tc.userinfo)
			if got != tc.valid {
				t.Errorf("expected valid=%v, got %v for %q", tc.valid, got, tc.userinfo)
			}
		})
	}
}

func validateShadowrocketURI(t *testing.T, uri string, n nodespec.NodeSpec) {
	t.Helper()

	validSchemes := map[string]bool{
		"ss://":      n.Protocol == nodespec.ProtocolShadowsocks,
		"vmess://":   n.Protocol == nodespec.ProtocolVMess,
		"vless://":   n.Protocol == nodespec.ProtocolVLESS,
		"trojan://":  n.Protocol == nodespec.ProtocolTrojan,
		"hysteria2://": n.Protocol == nodespec.ProtocolHysteria2,
		"tuic://":    n.Protocol == nodespec.ProtocolTUIC,
	}

	var matched bool
	for scheme, shouldMatch := range validSchemes {
		if strings.HasPrefix(uri, scheme) {
			if !shouldMatch {
				t.Errorf("unexpected scheme %s for protocol %s", scheme, n.Protocol)
			}
			matched = true

			if scheme == "vmess://" {
				rest := strings.TrimPrefix(uri, scheme)
				if isBase64Encoded(rest) {
					t.Error("Shadowrocket VMess should use non-base64 URI format for better compatibility")
				}
			}
			break
		}
	}
	if !matched {
		t.Errorf("no matching scheme for protocol %s in uri: %s", n.Protocol, uri)
	}

	if !strings.Contains(uri, n.Address) {
		t.Errorf("uri should contain address %s", n.Address)
	}

	if n.Protocol != nodespec.ProtocolShadowsocks {
		_, err := url.Parse(uri)
		if err != nil {
			t.Logf("warning: uri parse warning: %v", err)
		}
	}
}

func validateShadowrocketOutputFormat(t *testing.T, content string) {
	t.Helper()

	if len(content) == 0 {
		t.Error("empty output")
		return
	}

	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		t.Error("contains BOM")
	}

	if strings.Contains(content, "\r\n") || strings.Contains(content, "\r") {
		t.Error("contains CRLF or CR line endings")
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "" {
			if i < len(lines)-1 && lines[i+1] == "" {
				t.Errorf("consecutive blank lines at line %d", i+1)
			}
			continue
		}
		last := line[len(line)-1]
		if last == ' ' || last == '\t' {
			t.Errorf("line %d trailing whitespace: %q", i+1, line)
		}
	}

	if content[len(content)-1] != '\n' {
		t.Error("output should end with newline")
	}
}

func isBase64Encoded(s string) bool {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "#"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "?"); idx != -1 {
		s = s[:idx]
	}
	base64Chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	for _, c := range s {
		if !strings.ContainsRune(base64Chars, c) {
			return false
		}
	}
	return len(s) > 20
}
