package kernelrender

import (
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

// TestRenderVLESSRealityVision 测试 VLESS+TCP+REALITY+Vision 的双核渲染
func TestRenderVLESSRealityVision(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:     "p01-vless-reality-vision",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "1.2.3.4",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportTCP,
		},
		Security: nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI:         "rust-lang.org",
			PrivateKey:  "privkey_test_value",
			ShortID:     "abc123",
			Fingerprint: "chrome",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid-1234",
			Flow: nodespec.FlowXTLSRprxVision,
		},
	}

	// Xray 渲染
	xrayCfg, err := RenderForKernel(KernelXray, spec)
	if err != nil {
		t.Fatalf("Xray渲染失败: %v", err)
	}
	inbounds := xrayCfg["inbounds"].([]interface{})
	inbound := inbounds[0].(map[string]interface{})
	if inbound["protocol"] != "vless" {
		t.Errorf("期望protocol=vless, got %v", inbound["protocol"])
	}
	if inbound["port"].(int) != 443 {
		t.Errorf("期望port=443, got %v", inbound["port"])
	}
	ss := inbound["streamSettings"].(map[string]interface{})
	if ss["security"] != "reality" {
		t.Errorf("期望security=reality, got %v", ss["security"])
	}
	reality := ss["realitySettings"].(map[string]interface{})
	if reality["privateKey"] != "privkey_test_value" {
		t.Errorf("REALITY privateKey错误: %v", reality["privateKey"])
	}

	// Sing-box 渲染
	sbCfg, err := RenderForKernel(KernelSingBox, spec)
	if err != nil {
		t.Fatalf("Sing-box渲染失败: %v", err)
	}
	sbInbounds := sbCfg["inbounds"].([]interface{})
	sbInbound := sbInbounds[0].(map[string]interface{})
	if sbInbound["type"] != "vless" {
		t.Errorf("期望type=vless, got %v", sbInbound["type"])
	}
	if sbInbound["listen_port"].(int) != 443 {
		t.Errorf("期望listen_port=443, got %v", sbInbound["listen_port"])
	}
	tls := sbInbound["tls"].(map[string]interface{})
	if tls["enabled"] != true {
		t.Errorf("期望tls.enabled=true, got %v", tls["enabled"])
	}
	realityMap := tls["reality"].(map[string]interface{})
	if realityMap["private_key"] != "privkey_test_value" {
		t.Errorf("REALITY private_key错误: %v", realityMap["private_key"])
	}
}

// TestRenderVLESSWSTLS 测试 VLESS+WS+TLS（CDN 场景）
func TestRenderVLESSWSTLS(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:     "p03-vless-ws-tls",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "one.example.com",
		Port:     10003,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportWS,
			WS: &nodespec.WSConfig{
				Path: "/random-path-123",
				Host: "one.example.com",
			},
		},
		Security: nodespec.SecurityTLS,
		TLS: &nodespec.TLSConfig{
			SNI:         "one.example.com",
			Fingerprint: "chrome",
			CertFile:    "/path/cert.pem",
			KeyFile:     "/path/key.pem",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid-5678",
		},
	}

	// Xray 渲染
	xrayCfg, err := RenderForKernel(KernelXray, spec)
	if err != nil {
		t.Fatalf("Xray渲染失败: %v", err)
	}
	inbound := xrayCfg["inbounds"].([]interface{})[0].(map[string]interface{})
	ss := inbound["streamSettings"].(map[string]interface{})
	if ss["network"] != "ws" {
		t.Errorf("期望network=ws, got %v", ss["network"])
	}
	ws := ss["wsSettings"].(map[string]interface{})
	if ws["path"] != "/random-path-123" {
		t.Errorf("WS path错误: %v", ws["path"])
	}

	// Sing-box 渲染
	sbCfg, err := RenderForKernel(KernelSingBox, spec)
	if err != nil {
		t.Fatalf("Sing-box渲染失败: %v", err)
	}
	sbInbound := sbCfg["inbounds"].([]interface{})[0].(map[string]interface{})
	transport := sbInbound["transport"].(map[string]interface{})
	if transport["type"] != "ws" {
		t.Errorf("期望transport.type=ws, got %v", transport["type"])
	}
}

// TestRenderXHTTPModeAutoRejected 测试 XHTTP mode=auto 被拒绝
func TestRenderXHTTPModeAutoRejected(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:     "p08-xhttp-auto",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "one.example.com",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportXHTTP,
			XHTTP: &nodespec.XHTTPConfig{
				Path: "/xhttp",
				Mode: "auto", // 禁止 auto
			},
		},
		Security: nodespec.SecurityTLS,
		TLS: &nodespec.TLSConfig{
			SNI: "one.example.com",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid",
		},
	}

	// Xray 渲染应失败
	_, err := RenderForKernel(KernelXray, spec)
	if err == nil {
		t.Error("期望Xray渲染拒绝auto模式，但没有返回错误")
	}
	if err != nil && !contains(err.Error(), "auto") {
		t.Errorf("错误信息应包含auto: %v", err)
	}
}

// TestRenderAnyTLSXrayUnsupported 测试 AnyTLS 在 Xray 不支持
func TestRenderAnyTLSXrayUnsupported(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:      "p05-anytls",
		Protocol:  nodespec.ProtocolAnyTLS,
		Address:   "1.2.3.4",
		Port:      443,
		Security:  nodespec.SecurityTLS,
		TLS:       &nodespec.TLSConfig{SNI: "example.com"},
		Credentials: nodespec.AnyTLSCredentials{
			Password: "password123",
		},
	}

	// Xray 渲染应返回 UnsupportedFeatureError
	_, err := RenderForKernel(KernelXray, spec)
	if err == nil {
		t.Fatal("期望Xray渲染返回UnsupportedFeatureError")
	}
	if _, ok := err.(*UnsupportedFeatureError); !ok {
		t.Errorf("期望*UnsupportedFeatureError, got %T: %v", err, err)
	}

	// Sing-box 渲染应成功
	sbCfg, err := RenderForKernel(KernelSingBox, spec)
	if err != nil {
		t.Fatalf("Sing-box渲染AnyTLS应成功: %v", err)
	}
	sbInbound := sbCfg["inbounds"].([]interface{})[0].(map[string]interface{})
	if sbInbound["type"] != "anytls" {
		t.Errorf("期望type=anytls, got %v", sbInbound["type"])
	}
}

// TestRenderTUICXrayUnsupported 测试 TUIC 在 Xray 不支持
func TestRenderTUICXrayUnsupported(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:      "p12-tuic",
		Protocol:  nodespec.ProtocolTUIC,
		Address:   "1.2.3.4",
		Port:      443,
		Security:  nodespec.SecurityTLS,
		TLS:       &nodespec.TLSConfig{SNI: "example.com", ALPN: []string{"h3"}},
		Credentials: nodespec.TUICCredentials{
			UUID:     "test-uuid",
			Password: "password123",
		},
	}

	// Xray 渲染应返回 UnsupportedFeatureError
	_, err := RenderForKernel(KernelXray, spec)
	if err == nil {
		t.Fatal("期望Xray渲染TUIC返回UnsupportedFeatureError")
	}

	// Sing-box 渲染应成功
	_, err = RenderForKernel(KernelSingBox, spec)
	if err != nil {
		t.Fatalf("Sing-box渲染TUIC应成功: %v", err)
	}
}

// TestECHForcesTLS13 测试 ECH 启用时 Sing-box 强制写入 min_version=1.3
func TestECHForcesTLS13(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:      "p02-trojan-ech",
		Protocol:  nodespec.ProtocolTrojan,
		Address:   "1.2.3.4",
		Port:      443,
		Security:  nodespec.SecurityTLS,
		TLS: &nodespec.TLSConfig{
			SNI: "example.com",
			ECH: &nodespec.ECHConfig{
				Enabled: true,
				PEM:     "ech-pem-content",
			},
		},
		Credentials: nodespec.TrojanCredentials{
			Password: "password123",
		},
	}

	// Sing-box 渲染应包含 min_version=1.3
	sbCfg, err := RenderForKernel(KernelSingBox, spec)
	if err != nil {
		t.Fatalf("Sing-box渲染失败: %v", err)
	}
	sbInbound := sbCfg["inbounds"].([]interface{})[0].(map[string]interface{})
	tls := sbInbound["tls"].(map[string]interface{})
	if tls["min_version"] != "1.3" {
		t.Errorf("ECH启用时tls.min_version应为1.3, got %v", tls["min_version"])
	}
	ech := tls["ech"].(map[string]interface{})
	if ech["enabled"] != true {
		t.Errorf("ECH应启用, got %v", ech["enabled"])
	}
}

// TestRenderXHTTPDownloadSettings 测试 XHTTP split mode downloadSettings 在服务端不渲染
// xray 26.3.27 存在 downloadSettings 静默失败 bug，服务端 inbound 配置应跳过该字段。
// 客户端 share link 仍正常生成 downloadSettings（由 uri/clash 渲染器独立处理）。
func TestRenderXHTTPDownloadSettings(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:     "p07-xhttp-split",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "primary.example.com",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportXHTTP,
			XHTTP: &nodespec.XHTTPConfig{
				Path: "/xhb4cc53b6",
				Mode: "stream-up", // 上行使用 stream-up
				DownloadSettings: &nodespec.XHTTPDownloadConfig{
					Address: "download.example.com",
					Port:    8443,
					Network: nodespec.TransportXHTTP,
					Path:    "/xhb4cc53b6",
					Host:    "download.example.com",
					Mode:    "packet-up", // 下行使用 packet-up
					Reality: &nodespec.RealityConfig{
						SNI:         "rust-lang.org",
						PublicKey:   "nS2ld_0Xn_GntyX-HqW11DqFbHn72FJviEwJoZ2vUx0",
						ShortID:     "abc123",
						Fingerprint: "chrome",
					},
				},
			},
		},
		Security: nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI:         "rust-lang.org",
			PrivateKey:  "cHAWz_DP00iHGudE9Uq-8txkbwiZGCTAV1GvDQ8Z7U4",
			ShortID:     "abc123",
			Fingerprint: "chrome",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid-1234",
		},
	}

	cfg, err := RenderForKernel(KernelXray, spec)
	if err != nil {
		t.Fatalf("Xray渲染失败: %v", err)
	}

	inbounds := cfg["inbounds"].([]interface{})
	inbound := inbounds[0].(map[string]interface{})
	stream := inbound["streamSettings"].(map[string]interface{})
	xhttp := stream["xhttpSettings"].(map[string]interface{})

	// 验证基础 xhttp 字段
	if xhttp["mode"] != "stream-up" {
		t.Errorf("期望 xhttp.mode=stream-up, got %v", xhttp["mode"])
	}

	// downloadSettings 在服务端 inbound 配置中不应渲染（xray 26.3.27 静默失败 bug）
	if extra, ok := xhttp["extra"].(map[string]interface{}); ok {
		if _, exists := extra["downloadSettings"]; exists {
			t.Errorf("downloadSettings 不应出现在服务端 inbound 配置中（xray 26.3.27 bug），got %v", extra["downloadSettings"])
		}
	}
}

// TestRenderXHTTPDownloadSettingsPortDefault 测试 downloadSettings 在服务端不渲染
// （原测试验证 port 默认 443，因 xray 26.3.27 downloadSettings 静默失败 bug 已跳过渲染）
func TestRenderXHTTPDownloadSettingsPortDefault(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Code:     "p17-xhttp-default-port",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "primary.example.com",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportXHTTP,
			XHTTP: &nodespec.XHTTPConfig{
				Path: "/xhb4cc53b6",
				Mode: "stream-up",
				DownloadSettings: &nodespec.XHTTPDownloadConfig{
					Address: "download.example.com",
					// Port 不设置，原应默认 443（现已跳过渲染）
					Network: nodespec.TransportXHTTP,
				},
			},
		},
		Security: nodespec.SecurityTLS,
		TLS:      &nodespec.TLSConfig{SNI: "primary.example.com"},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid-1234",
		},
	}

	cfg, err := RenderForKernel(KernelXray, spec)
	if err != nil {
		t.Fatalf("Xray渲染失败: %v", err)
	}

	inbound := cfg["inbounds"].([]interface{})[0].(map[string]interface{})
	stream := inbound["streamSettings"].(map[string]interface{})
	xhttp := stream["xhttpSettings"].(map[string]interface{})

	// downloadSettings 在服务端 inbound 配置中不应渲染（xray 26.3.27 静默失败 bug）
	if extra, ok := xhttp["extra"].(map[string]interface{}); ok {
		if _, exists := extra["downloadSettings"]; exists {
			t.Errorf("downloadSettings 不应出现在服务端 inbound 配置中（xray 26.3.27 bug），got %v", extra["downloadSettings"])
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
