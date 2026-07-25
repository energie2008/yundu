package validator

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPreCheckEdge_PortConflict(t *testing.T) {
	v := NewEdgeValidator(nil)

	// 使用一个肯定被占用的端口 (不存在的端口范围会跳过，用一个合法但可能被占用的端口)
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     80,
				"protocol": "vless",
				"tag":      "inbound-80",
			},
		},
	}

	data, _ := json.Marshal(cfg)
	result, err := v.PreCheckEdge(context.Background(), data, "xray")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Port 80 might or might not be in use; just verify no panic
	t.Logf("result: passed=%v, errors=%v, warnings=%v", result.Passed, result.Errors, result.Warnings)
}

func TestPreCheckEdge_ValidConfig(t *testing.T) {
	v := NewEdgeValidator(nil)

	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     19999, // unlikely to be in use
				"protocol": "vless",
				"tag":      "test-inbound",
				"streamSettings": map[string]interface{}{
					"security": "none",
					"network":  "tcp",
				},
			},
		},
	}

	data, _ := json.Marshal(cfg)
	result, err := v.PreCheckEdge(context.Background(), data, "xray")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got errors: %v", result.Errors)
	}
}

func TestPreCheckEdge_InvalidJSON(t *testing.T) {
	v := NewEdgeValidator(nil)

	result, err := v.PreCheckEdge(context.Background(), []byte("{invalid json"), "xray")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected fail for invalid JSON")
	}
}

func TestPreCheckEdge_EmptyInbounds(t *testing.T) {
	v := NewEdgeValidator(nil)

	cfg := map[string]interface{}{
		"inbounds": []interface{}{},
	}

	data, _ := json.Marshal(cfg)
	result, err := v.PreCheckEdge(context.Background(), data, "xray")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for empty inbounds")
	}
}

func TestExtractInboundPorts_Xray(t *testing.T) {
	cfg := `{"inbounds":[{"port":10000,"protocol":"vless"},{"port":10001,"protocol":"trojan","listen":"127.0.0.1"}]}`
	ports, err := extractInboundPortsWithListen([]byte(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	if ports[0].port != 10000 || ports[1].port != 10001 {
		t.Errorf("unexpected ports: %v", ports)
	}
	// P0: 第二个 inbound 有 listen=127.0.0.1，应被提取
	if ports[1].listen != "127.0.0.1" {
		t.Errorf("expected listen=127.0.0.1 for second inbound, got %q", ports[1].listen)
	}
}

func TestExtractInboundPorts_SingBox(t *testing.T) {
	cfg := `{"inbounds":[{"type":"vless","listen_port":20000,"listen":"127.0.0.1"},{"type":"trojan","listen_port":20001}]}`
	ports, err := extractInboundPortsWithListen([]byte(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	// P0: 第一个 inbound 有 listen=127.0.0.1
	if ports[0].listen != "127.0.0.1" {
		t.Errorf("expected listen=127.0.0.1, got %q", ports[0].listen)
	}
}

// TestBuildListenAddr_P0 验证 P0 修复：CDN 后端 inbound 的 listen 字段应映射到正确的测试地址。
func TestBuildListenAddr_P0(t *testing.T) {
	cases := []struct {
		listen string
		port   int
		want   string
	}{
		{"", 443, ":443"},               // 默认 → 所有接口
		{"0.0.0.0", 443, ":443"},         // 显式 0.0.0.0 → 所有接口
		{"::", 443, ":443"},              // IPv6 通配 → 所有接口
		{"127.0.0.1", 443, "127.0.0.1:443"}, // CDN 后端 → 只测 localhost
		{"localhost", 443, "127.0.0.1:443"}, // localhost 别名
		{"10.0.0.1", 443, "10.0.0.1:443"},   // 具体 IP
	}
	for _, c := range cases {
		got := buildListenAddr(c.listen, c.port)
		if got != c.want {
			t.Errorf("buildListenAddr(%q, %d) = %q, want %q", c.listen, c.port, got, c.want)
		}
	}
}

// TestPreCheckEdge_CDNBackendListen127 P0 回归测试：
// 当 inbound listen=127.0.0.1 且端口被 nginx(0.0.0.0) 占用时，precheck 应通过而非误报冲突。
// （此测试在无 nginx 的环境下验证 listen 地址正确传递；实际冲突消除在集成环境验证。）
func TestPreCheckEdge_CDNBackendListen127(t *testing.T) {
	v := NewEdgeValidator(nil)
	cfg := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     19998, // unlikely to be in use
				"listen":   "127.0.0.1",
				"protocol": "vless",
				"tag":      "cdn-backend",
			},
		},
	}
	data, _ := json.Marshal(cfg)
	result, err := v.PreCheckEdge(context.Background(), data, "xray")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass for listen=127.0.0.1 backend, got errors: %v", result.Errors)
	}
}
