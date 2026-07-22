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
	cfg := `{"inbounds":[{"port":10000,"protocol":"vless"},{"port":10001,"protocol":"trojan"}]}`
	ports, err := extractInboundPorts([]byte(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
	if ports[0] != 10000 || ports[1] != 10001 {
		t.Errorf("unexpected ports: %v", ports)
	}
}

func TestExtractInboundPorts_SingBox(t *testing.T) {
	cfg := `{"inbounds":[{"type":"vless","listen_port":20000},{"type":"trojan","listen_port":20001}]}`
	ports, err := extractInboundPorts([]byte(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}
}
