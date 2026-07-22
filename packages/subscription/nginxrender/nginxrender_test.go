package nginxrender

import (
	"strings"
	"testing"

	"github.com/airport-panel/subscription/inboundgroup"
	"github.com/airport-panel/subscription/nodespec"
)

func TestRenderStreamConf_Basic(t *testing.T) {
	cfg := &SnippetConfig{
		HTTPSListenPort:   8445,
		XHTTPInternalPort: 8446,
		UpstreamName:      "yundu_xhttp_backend",
		SNIHosts:          []string{"cdn1.example.com", "cdn2.example.com"},
	}
	out := RenderStreamConf(nil, cfg)
	if !strings.Contains(out, "cdn1\\.example\\.com") {
		t.Errorf("stream conf should contain escaped SNI host: %s", out)
	}
	if !strings.Contains(out, "yundu_xhttp_backend") {
		t.Errorf("stream conf should contain upstream name: %s", out)
	}
}

func TestRenderStreamConf_EmptyHosts(t *testing.T) {
	cfg := DefaultConfig()
	out := RenderStreamConf(nil, cfg)
	if !strings.Contains(out, "空 snippet") {
		t.Errorf("empty hosts should produce empty snippet note: %s", out)
	}
}

func TestRenderHTTPSConf_Basic(t *testing.T) {
	// 构建一个有 internal inbound 的 group
	group := &inboundgroup.InboundGroup{
		Port: 443,
		Internal: []*inboundgroup.InboundUnit{
			{
				Listen: "@xhttp-internal",
				Tag:    "p09-internal",
				Settings: map[string]interface{}{
					"stream": map[string]interface{}{
						"xhttpSettings": map[string]interface{}{
							"path": "/xhttp-up",
						},
					},
				},
			},
		},
	}
	cfg := &SnippetConfig{
		HTTPSListenPort:   8445,
		XHTTPInternalPort: 8446,
		UpstreamName:      "yundu_xhttp_backend",
		SNIHosts:          []string{"cdn.example.com"},
	}
	out := RenderHTTPSConf([]*inboundgroup.InboundGroup{group}, cfg, "/path/cert.pem", "/path/key.pem")
	if !strings.Contains(out, "listen 8445") {
		t.Errorf("https conf should contain listen 8445: %s", out)
	}
	if !strings.Contains(out, "ssl http2") {
		t.Errorf("https conf should contain http2 directive: %s", out)
	}
	if !strings.Contains(out, "cdn.example.com") {
		t.Errorf("https conf should contain server_name: %s", out)
	}
	if !strings.Contains(out, "grpc_pass grpc://127.0.0.1:8446") {
		t.Errorf("https conf should contain grpc_pass to 127.0.0.1:8446: %s", out)
	}
	if !strings.Contains(out, "location /xhttp-up") {
		t.Errorf("https conf should contain location path: %s", out)
	}
}

func TestRenderUpstreamBlock(t *testing.T) {
	cfg := DefaultConfig()
	out := RenderUpstreamBlock(cfg)
	if !strings.Contains(out, "upstream yundu_xhttp_backend") {
		t.Errorf("upstream block should contain upstream name: %s", out)
	}
	if !strings.Contains(out, "127.0.0.1:8445") {
		t.Errorf("upstream block should contain backend address: %s", out)
	}
}

func TestRenderGroupedStreamConfig(t *testing.T) {
	cfg := &SnippetConfig{
		HTTPSListenPort:   8445,
		XHTTPInternalPort: 8446,
		UpstreamName:      "yundu_xhttp_backend",
		SNIHosts:          []string{"cdn.example.com"},
	}
	out := RenderGroupedStreamConfig(nil, cfg)
	if !strings.Contains(out, "listen 443") {
		t.Errorf("grouped stream conf should contain listen 443: %s", out)
	}
	if !strings.Contains(out, "ssl_preread on") {
		t.Errorf("grouped stream conf should contain ssl_preread: %s", out)
	}
	if !strings.Contains(out, "map $ssl_preread_server_name") {
		t.Errorf("grouped stream conf should contain map directive: %s", out)
	}
}

func TestValidateSnippetConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *SnippetConfig
		wantErr bool
	}{
		{"valid", &SnippetConfig{HTTPSListenPort: 8445, XHTTPInternalPort: 8446, UpstreamName: "test"}, false},
		{"nil", nil, true},
		{"invalid_https_port", &SnippetConfig{HTTPSListenPort: 0, XHTTPInternalPort: 8446, UpstreamName: "test"}, true},
		{"invalid_internal_port", &SnippetConfig{HTTPSListenPort: 8445, XHTTPInternalPort: 0, UpstreamName: "test"}, true},
		{"empty_upstream", &SnippetConfig{HTTPSListenPort: 8445, XHTTPInternalPort: 8446, UpstreamName: ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSnippetConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSnippetConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractSNIHostsFromSpecs(t *testing.T) {
	specs := []*nodespec.NodeSpec{
		{
			Transport: nodespec.TransportConfig{Type: nodespec.TransportXHTTP},
			Security:  nodespec.SecurityTLS,
			TLS:       &nodespec.TLSConfig{SNI: "cdn1.example.com"},
		},
		{
			Transport: nodespec.TransportConfig{Type: nodespec.TransportXHTTP},
			Security:  nodespec.SecurityTLS,
			TLS:       &nodespec.TLSConfig{SNI: "cdn2.example.com"},
		},
		{
			Transport: nodespec.TransportConfig{Type: nodespec.TransportXHTTP},
			Security:  nodespec.SecurityTLS,
			TLS:       &nodespec.TLSConfig{SNI: "cdn1.example.com"}, // 重复
		},
		{
			Transport: nodespec.TransportConfig{Type: nodespec.TransportTCP}, // 非 XHTTP
			Security:  nodespec.SecurityTLS,
			TLS:       &nodespec.TLSConfig{SNI: "ignored.example.com"},
		},
	}
	hosts := ExtractSNIHostsFromSpecs(specs)
	if len(hosts) != 2 {
		t.Fatalf("expected 2 unique SNI hosts, got %d: %v", len(hosts), hosts)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HTTPSListenPort != 8445 {
		t.Errorf("default HTTPS port should be 8445, got %d", cfg.HTTPSListenPort)
	}
	if cfg.XHTTPInternalPort != 8446 {
		t.Errorf("default XHTTP internal port should be 8446, got %d", cfg.XHTTPInternalPort)
	}
	if cfg.UpstreamName == "" {
		t.Error("default upstream name should not be empty")
	}
}
