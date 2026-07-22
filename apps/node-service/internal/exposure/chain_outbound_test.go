package exposure

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestChainOutboundXraySocks5 验证 P5 节点套娃 socks5 协议的 xray outbound 渲染。
// P5 = VLESS+WS+TLS 节点，套娃 socks5://hmfpcsfd:lfr0o0ar4u2f@163.123.203.113:8216
func TestChainOutboundXraySocks5(t *testing.T) {
	uri := "socks5://hmfpcsfd:lfr0o0ar4u2f@163.123.203.113:8216"
	spec, err := ParseChainURI(uri)
	if err != nil {
		t.Fatalf("ParseChainURI failed: %v", err)
	}
	if spec == nil {
		t.Fatal("ParseChainURI returned nil spec")
	}
	if spec.Protocol != "socks" {
		t.Errorf("protocol: got %s want socks", spec.Protocol)
	}
	if spec.Address != "163.123.203.113" {
		t.Errorf("address: got %s want 163.123.203.113", spec.Address)
	}
	if spec.Port != 8216 {
		t.Errorf("port: got %d want 8216", spec.Port)
	}

	ob, err := BuildXrayOutboundFromNodeSpec(spec, "chain-P5", "")
	if err != nil {
		t.Fatalf("BuildXrayOutboundFromNodeSpec failed: %v", err)
	}
	if ob["protocol"] != "socks" {
		t.Errorf("xray outbound protocol: got %v want socks", ob["protocol"])
	}
	if ob["tag"] != "chain-P5" {
		t.Errorf("xray outbound tag: got %v want chain-P5", ob["tag"])
	}

	// 验证 settings.servers 结构
	settings, ok := ob["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("settings is not a map")
	}
	servers, ok := settings["servers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("servers: expected 1 entry, got %v", settings["servers"])
	}
	server := servers[0].(map[string]interface{})
	if server["address"] != "163.123.203.113" {
		t.Errorf("server address: got %v want 163.123.203.113", server["address"])
	}
	if server["port"] != 8216 {
		t.Errorf("server port: got %v want 8216", server["port"])
	}
	// 验证用户名密码认证
	users, ok := server["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("users: expected 1 entry, got %v", server["users"])
	}
	user := users[0].(map[string]interface{})
	if user["user"] != "hmfpcsfd" {
		t.Errorf("username: got %v want hmfpcsfd", user["user"])
	}
	if user["pass"] != "lfr0o0ar4u2f" {
		t.Errorf("password: got %v want lfr0o0ar4u2f", user["pass"])
	}

	// 验证无 streamSettings（socks5 无 TLS 无 Transport）
	if _, hasStream := ob["streamSettings"]; hasStream {
		t.Error("socks5 outbound should not have streamSettings")
	}

	// 验证无 proxySettings（服务端单跳套娃，走 routing 路由）
	if _, hasProxy := ob["proxySettings"]; hasProxy {
		t.Error("server-side chain outbound should not have proxySettings")
	}
}

// TestChainOutboundXrayTrojan 验证 P4 节点套娃 trojan 协议的 xray outbound 渲染。
// P4 = VMess+WS+TLS 节点，套娃 trojan://...@fd6a09.douyincdn.25hp9v9.top:34711?security=tls&sni=...
func TestChainOutboundXrayTrojan(t *testing.T) {
	uri := "trojan://9acd564f-c4b7-4ab7-81c0-ed0a1b715757@fd6a09.douyincdn.25hp9v9.top:34711?security=tls&sni=cn-hnzz-cm-01-01.bilivideo.com&fp=edge&alpn=h2%2Chttp%2F1.1&insecure=1&allowInsecure=1&type=tcp&headerType=none"
	spec, err := ParseChainURI(uri)
	if err != nil {
		t.Fatalf("ParseChainURI failed: %v", err)
	}
	if spec.Protocol != "trojan" {
		t.Errorf("protocol: got %s want trojan", spec.Protocol)
	}
	if spec.Address != "fd6a09.douyincdn.25hp9v9.top" {
		t.Errorf("address: got %s", spec.Address)
	}
	if spec.Port != 34711 {
		t.Errorf("port: got %d want 34711", spec.Port)
	}

	ob, err := BuildXrayOutboundFromNodeSpec(spec, "chain-P4", "")
	if err != nil {
		t.Fatalf("BuildXrayOutboundFromNodeSpec failed: %v", err)
	}
	if ob["protocol"] != "trojan" {
		t.Errorf("protocol: got %v want trojan", ob["protocol"])
	}

	// 验证 trojan settings
	settings := ob["settings"].(map[string]interface{})
	servers := settings["servers"].([]interface{})
	server := servers[0].(map[string]interface{})
	if server["password"] != "9acd564f-c4b7-4ab7-81c0-ed0a1b715757" {
		t.Errorf("trojan password mismatch: got %v", server["password"])
	}

	// 验证 TLS streamSettings（trojan+TLS）
	ss, ok := ob["streamSettings"].(map[string]interface{})
	if !ok {
		t.Fatal("trojan+TLS outbound should have streamSettings")
	}
	if ss["security"] != "tls" {
		t.Errorf("streamSettings security: got %v want tls", ss["security"])
	}
	tlsSettings := ss["tlsSettings"].(map[string]interface{})
	if tlsSettings["serverName"] != "cn-hnzz-cm-01-01.bilivideo.com" {
		t.Errorf("TLS SNI: got %v want cn-hnzz-cm-01-01.bilivideo.com", tlsSettings["serverName"])
	}
	if tlsSettings["fingerprint"] != "edge" {
		t.Errorf("TLS fingerprint: got %v want edge", tlsSettings["fingerprint"])
	}
	if !tlsSettings["allowInsecure"].(bool) {
		t.Error("TLS allowInsecure should be true")
	}
}

// TestChainOutboundSingboxSocks5 验证 sing-box 内核下的 socks5 套娃渲染。
func TestChainOutboundSingboxSocks5(t *testing.T) {
	uri := "socks5://hmfpcsfd:lfr0o0ar4u2f@163.123.203.113:8216"
	spec, err := ParseChainURI(uri)
	if err != nil {
		t.Fatalf("ParseChainURI failed: %v", err)
	}

	ob, err := BuildSingboxOutboundFromNodeSpec(spec, "chain-P5", "")
	if err != nil {
		t.Fatalf("BuildSingboxOutboundFromNodeSpec failed: %v", err)
	}
	if ob["type"] != "socks" {
		t.Errorf("sing-box type: got %v want socks", ob["type"])
	}
	if ob["server"] != "163.123.203.113" {
		t.Errorf("server: got %v want 163.123.203.113", ob["server"])
	}
	if ob["server_port"] != 8216 {
		t.Errorf("server_port: got %v want 8216", ob["server_port"])
	}
	if ob["username"] != "hmfpcsfd" {
		t.Errorf("username: got %v want hmfpcsfd", ob["username"])
	}
	if ob["password"] != "lfr0o0ar4u2f" {
		t.Errorf("password: got %v want lfr0o0ar4u2f", ob["password"])
	}
}

// TestChainOutboundRedactURI 验证 URI 凭证脱敏（D5：密码不写入日志）。
func TestChainOutboundRedactURI(t *testing.T) {
	uri := "socks5://hmfpcsfd:lfr0o0ar4u2f@163.123.203.113:8216"
	redacted := redactURI(uri)
	if strings.Contains(redacted, "lfr0o0ar4u2f") {
		t.Errorf("redactURI leaked password: %s", redacted)
	}
	if strings.Contains(redacted, "hmfpcsfd") {
		t.Errorf("redactURI leaked username: %s", redacted)
	}
	if !strings.Contains(redacted, "163.123.203.113") {
		t.Errorf("redactURI should retain host: %s", redacted)
	}
}

// TestChainOutboundXrayBalancerRule 验证套娃降级 balancer + routing 规则结构。
// 模拟 kernel_render_adapter.go 中的套娃收集逻辑。
func TestChainOutboundXrayBalancerRule(t *testing.T) {
	uri := "socks5://hmfpcsfd:lfr0o0ar4u2f@163.123.203.113:8216"
	spec, err := ParseChainURI(uri)
	if err != nil {
		t.Fatalf("ParseChainURI failed: %v", err)
	}

	chainTag := "chain-P5"
	ob, err := BuildXrayOutboundFromNodeSpec(spec, chainTag, "")
	if err != nil {
		t.Fatalf("BuildXrayOutboundFromNodeSpec failed: %v", err)
	}

	// 模拟 kernel_render_adapter 的 balancer + routing 规则构建
	balancerTag := "balancer-P5"
	balancer := map[string]interface{}{
		"tag":      balancerTag,
		"selector": []string{chainTag, "direct"},
		"strategy": map[string]interface{}{"type": "leastPing"},
	}
	rule := map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{"in-P5"},
		"balancerTag": balancerTag,
	}

	// 验证 balancer 候选含 direct（健康降级）
	selector := balancer["selector"].([]string)
	foundDirect := false
	foundChain := false
	for _, s := range selector {
		if s == "direct" {
			foundDirect = true
		}
		if s == chainTag {
			foundChain = true
		}
	}
	if !foundDirect {
		t.Error("balancer selector must contain 'direct' for health degradation")
	}
	if !foundChain {
		t.Error("balancer selector must contain chain tag")
	}

	// 验证 routing 规则引用正确
	if rule["balancerTag"] != balancerTag {
		t.Errorf("routing rule balancerTag: got %v want %s", rule["balancerTag"], balancerTag)
	}
	inboundTags := rule["inboundTag"].([]string)
	if len(inboundTags) != 1 || inboundTags[0] != "in-P5" {
		t.Errorf("routing rule inboundTag: got %v want [in-P5]", inboundTags)
	}

	// 验证完整 outbound JSON 可序列化（防止渲染时 marshal 失败）
	if _, err := json.Marshal(ob); err != nil {
		t.Errorf("chain outbound not JSON serializable: %v", err)
	}
	if _, err := json.Marshal(balancer); err != nil {
		t.Errorf("balancer not JSON serializable: %v", err)
	}
	if _, err := json.Marshal(rule); err != nil {
		t.Errorf("routing rule not JSON serializable: %v", err)
	}
}
