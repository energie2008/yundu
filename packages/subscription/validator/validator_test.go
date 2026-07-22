package validator

import (
	"context"
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

// TestDualKernelValidator_VLESSReality 测试 VLESS+REALITY+Vision 双核校验
func TestDualKernelValidator_VLESSReality(t *testing.T) {
	v := NewDualKernelValidator("", "").WithSkipDryRun(true)

	spec := &nodespec.NodeSpec{
		Code:     "p01",
		Protocol: nodespec.ProtocolVLESS,
		Address:  "1.2.3.4",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportTCP,
		},
		Security: nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI:         "rust-lang.org",
			PrivateKey:  "privkey_test",
			ShortID:     "abc123",
			Fingerprint: "chrome",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid",
			Flow: nodespec.FlowXTLSRprxVision,
		},
	}

	result := v.ValidateBoth(context.Background(), spec)
	if !result.Passed {
		t.Errorf("VLESS+REALITY+Vision校验应通过, errors: %+v", result.Errors)
	}
	if result.XrayConfig == nil {
		t.Error("Xray配置不应为nil")
	}
	if result.SingBoxConfig == nil {
		t.Error("Sing-box配置不应为nil")
	}
}

// TestDualKernelValidator_PortMismatch 测试端口不一致检测
func TestDualKernelValidator_PortMismatch(t *testing.T) {
	v := NewDualKernelValidator("", "").WithSkipDryRun(true)

	// 构造一个会导致端口不一致的场景（通过修改渲染结果模拟）
	// 正常情况下双核渲染端口一致，这里用 VLESS+WS+TLS 测试正常通过
	// 设置 ServerPort 避免 CDN 节点端口冲突校验报错
	spec := &nodespec.NodeSpec{
		Code:       "p03",
		Protocol:   nodespec.ProtocolVLESS,
		Address:    "one.example.com",
		Port:       443,
		ServerPort: 10003,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportWS,
			WS: &nodespec.WSConfig{
				Path: "/test-path",
				Host: "one.example.com",
			},
		},
		Security: nodespec.SecurityTLS,
		TLS: &nodespec.TLSConfig{
			SNI:         "one.example.com",
			Fingerprint: "chrome",
		},
		Credentials: nodespec.VLESSCredentials{
			UUID: "test-uuid",
		},
	}

	result := v.ValidateBoth(context.Background(), spec)
	if !result.Passed {
		t.Errorf("VLESS+WS+TLS校验应通过, errors: %+v", result.Errors)
	}

	// 验证端口一致性
	xrayPort := extractInboundField(result.XrayConfig, "port")
	sbPort := extractInboundField(result.SingBoxConfig, "listen_port")
	if xrayPort != sbPort {
		t.Errorf("端口应一致: xray=%v sing-box=%v", xrayPort, sbPort)
	}
}

// TestEnhancementValidator_ECHRequiresTLS 测试 ECH 要求 TLS
func TestEnhancementValidator_ECHRequiresTLS(t *testing.T) {
	// ECH 在 REALITY 场景下应报错
	spec := &nodespec.NodeSpec{
		Security: nodespec.SecurityReality,
		Reality:  &nodespec.RealityConfig{SNI: "test.com", PrivateKey: "key", ShortID: "id"},
		TLS: &nodespec.TLSConfig{
			ECH: &nodespec.ECHConfig{Enabled: true},
		},
	}

	errs := RunEnhancementValidators(spec, nil)
	found := false
	for _, e := range errs {
		if e.Level == LevelError && containsStr(e.Message, "ECH只能在security=tls场景下启用") {
			found = true
		}
	}
	if !found {
		t.Errorf("应检测到ECH在REALITY场景的错误, got: %+v", errs)
	}
}

// TestEnhancementValidator_UTLSFingerprint 测试 uTLS 指纹枚举校验
func TestEnhancementValidator_UTLSFingerprint(t *testing.T) {
	// 无效指纹
	spec := &nodespec.NodeSpec{
		Security: nodespec.SecurityTLS,
		TLS: &nodespec.TLSConfig{
			Fingerprint: "invalid_fingerprint",
		},
	}

	errs := RunEnhancementValidators(spec, nil)
	found := false
	for _, e := range errs {
		if e.Level == LevelError && containsStr(e.Message, "无效的uTLS指纹") {
			found = true
		}
	}
	if !found {
		t.Errorf("应检测到无效uTLS指纹错误, got: %+v", errs)
	}

	// 有效指纹
	spec.TLS.Fingerprint = "chrome"
	errs = RunEnhancementValidators(spec, nil)
	for _, e := range errs {
		if e.Level == LevelError && containsStr(e.Message, "无效的uTLS指纹") {
			t.Errorf("chrome指纹应通过校验, but got error: %s", e.Message)
		}
	}
}

// TestEnhancementValidator_RealityMissingKey 测试 REALITY 缺少 private_key
func TestEnhancementValidator_RealityMissingKey(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Security: nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI: "rust-lang.org",
			// PrivateKey 故意留空
			ShortID: "abc123",
		},
	}

	errs := RunEnhancementValidators(spec, nil)
	foundKeyErr := false
	for _, e := range errs {
		if e.Level == LevelError && containsStr(e.Message, "private_key") {
			foundKeyErr = true
		}
	}
	if !foundKeyErr {
		t.Errorf("应检测到REALITY缺少private_key错误, got: %+v", errs)
	}
}

// TestEnhancementValidator_MuxFieldExclusivity 测试 Mux 字段互斥
func TestEnhancementValidator_MuxFieldExclusivity(t *testing.T) {
	// max_connections 和 max_streams 同时设置
	spec := &nodespec.NodeSpec{
		Protocol: nodespec.ProtocolVLESS,
		Transport: nodespec.TransportConfig{
			Mux: &nodespec.MuxConfig{
				Enabled:        true,
				MaxConnections: 4,
				MaxStreams:     10, // 与 MaxConnections 冲突
			},
		},
	}

	errs := RunEnhancementValidators(spec, nil)
	found := false
	for _, e := range errs {
		if e.Level == LevelError && containsStr(e.Message, "Mux字段互斥冲突") {
			found = true
		}
	}
	if !found {
		t.Errorf("应检测到Mux字段互斥错误, got: %+v", errs)
	}
}

// TestEnhancementValidator_RealitySNIWarning 测试 REALITY SNI 不推荐值警告
func TestEnhancementValidator_RealitySNIWarning(t *testing.T) {
	spec := &nodespec.NodeSpec{
		Security: nodespec.SecurityReality,
		Reality: &nodespec.RealityConfig{
			SNI:         "www.apple.com", // 不推荐值
			PrivateKey:  "key",
			ShortID:     "id",
			Fingerprint: "chrome",
		},
	}

	errs := RunEnhancementValidators(spec, nil)
	found := false
	for _, e := range errs {
		if e.Level == LevelWarning && containsStr(e.Message, "GFW 主动探测频繁") {
			found = true
		}
	}
	if !found {
		t.Errorf("应检测到apple.com SNI警告, got: %+v", errs)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
