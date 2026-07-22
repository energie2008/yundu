package inboundgroup

import (
	"testing"

	"github.com/airport-panel/subscription/nodespec"
)

// helper: 创建 TCP+Vision+443 节点（primary candidate）
func tcpSpec(code, uuid string) *nodespec.NodeSpec {
	return &nodespec.NodeSpec{
		Code:        code,
		Protocol:    nodespec.ProtocolVLESS,
		Address:     "1.2.3.4",
		Port:        443,
		Transport:   nodespec.TransportConfig{Type: nodespec.TransportTCP},
		Security:    nodespec.SecurityReality,
		Reality:     &nodespec.RealityConfig{PublicKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", ShortID: "aaaa1234bbbb5678", SNI: "example.com", Fingerprint: "chrome"},
		Credentials: nodespec.VLESSCredentials{UUID: uuid},
		TrafficRate: 1.0,
	}
}

// helper: 创建 XHTTP+443 节点（fallback candidate）
func xhttpSpec(code, path string) *nodespec.NodeSpec {
	return &nodespec.NodeSpec{
		Code:     code,
		Protocol: nodespec.ProtocolVLESS,
		Address:  "1.2.3.4",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type:  nodespec.TransportXHTTP,
			XHTTP: &nodespec.XHTTPConfig{Path: path, Mode: "packet-up"},
		},
		Security:    nodespec.SecurityTLS,
		Credentials: nodespec.VLESSCredentials{UUID: "00000000-0000-0000-0000-0000000000xx"},
		TrafficRate: 1.0,
	}
}

// helper: 创建 WS+443 节点（fallback candidate）
func wsSpec(code, path string) *nodespec.NodeSpec {
	return &nodespec.NodeSpec{
		Code:     code,
		Protocol: nodespec.ProtocolTrojan,
		Address:  "1.2.3.4",
		Port:     443,
		Transport: nodespec.TransportConfig{
			Type: nodespec.TransportWS,
			WS:   &nodespec.WSConfig{Path: path},
		},
		Security:    nodespec.SecurityTLS,
		Credentials: nodespec.TrojanCredentials{Password: "longpassword123"},
		TrafficRate: 1.0,
	}
}

// ========== BuildPrimaryClients 测试 ==========

func TestBuildPrimaryClients_SingleSpec(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
	}
	clients, err := gb.BuildPrimaryClients(specs)
	if err != nil {
		t.Fatalf("single spec should succeed: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0]["id"] != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong UUID: %v", clients[0]["id"])
	}
	if clients[0]["flow"] != "xtls-rprx-vision" {
		t.Errorf("wrong flow: %v", clients[0]["flow"])
	}
}

func TestBuildPrimaryClients_MultiSpecs(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		tcpSpec("p02", "00000000-0000-0000-0000-000000000002"),
		tcpSpec("p03", "00000000-0000-0000-0000-000000000003"),
	}
	clients, err := gb.BuildPrimaryClients(specs)
	if err != nil {
		t.Fatalf("3 specs should succeed: %v", err)
	}
	if len(clients) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(clients))
	}
}

func TestBuildPrimaryClients_DuplicateUUID(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		tcpSpec("p02", "00000000-0000-0000-0000-000000000001"), // 重复 UUID
	}
	_, err := gb.BuildPrimaryClients(specs)
	if err == nil {
		t.Fatal("duplicate UUID should return error")
	}
}

func TestBuildPrimaryClients_NilCredentials(t *testing.T) {
	gb := NewGroupBuilder()
	spec := tcpSpec("p01", "")
	specs := []*nodespec.NodeSpec{spec}
	clients, err := gb.BuildPrimaryClients(specs)
	if err != nil {
		t.Fatalf("empty UUID spec should not error: %v", err)
	}
	if len(clients) != 0 {
		t.Fatalf("empty UUID should produce 0 clients, got %d", len(clients))
	}
}

func TestBuildPrimaryClients_NonTCPIgnored(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		xhttpSpec("p09", "/xhttp"), // 非 TCP，应被忽略
	}
	clients, err := gb.BuildPrimaryClients(specs)
	if err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("non-TCP should be ignored, expected 1 client, got %d", len(clients))
	}
}

// ========== DetectPathConflicts 测试 ==========

func TestDetectPathConflicts_NoConflict(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/xhttp-up"),
		wsSpec("p03", "/ws"),
	}
	if err := gb.DetectPathConflicts(specs); err != nil {
		t.Fatalf("different paths should not conflict: %v", err)
	}
}

func TestDetectPathConflicts_SamePath(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/same"),
		xhttpSpec("p07", "/same"), // 同协议同 path
	}
	err := gb.DetectPathConflicts(specs)
	if err == nil {
		t.Fatal("same path across specs should conflict")
	}
}

func TestDetectPathConflicts_CrossProtocolConflict(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/shared"), // XHTTP
		wsSpec("p03", "/shared"),    // WS 同 path
	}
	err := gb.DetectPathConflicts(specs)
	if err == nil {
		t.Fatal("same path across WS and XHTTP should conflict")
	}
}

func TestDetectPathConflicts_TrailingSlash(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/path"),
		xhttpSpec("p07", "/path/"), // 末尾斜杠差异
	}
	err := gb.DetectPathConflicts(specs)
	if err == nil {
		t.Fatal("/path and /path/ should conflict after normalize")
	}
}

func TestDetectPathConflicts_QueryString(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/path?foo=bar"),
	}
	err := gb.DetectPathConflicts(specs)
	if err == nil {
		t.Fatal("path with query string should be rejected")
	}
}

func TestDetectPathConflicts_Fragment(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/path#section"),
	}
	err := gb.DetectPathConflicts(specs)
	if err == nil {
		t.Fatal("path with fragment should be rejected")
	}
}

// ========== PreflightUUIDCheck 测试 ==========

func TestPreflightUUIDCheck_Match(t *testing.T) {
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		tcpSpec("p02", "00000000-0000-0000-0000-000000000002"),
	}
	renderedInbounds := []interface{}{
		map[string]interface{}{
			"listen": "0.0.0.0",
			"port":   float64(443),
			"settings": map[string]interface{}{
				"clients": []interface{}{
					map[string]interface{}{"id": "00000000-0000-0000-0000-000000000001"},
					map[string]interface{}{"id": "00000000-0000-0000-0000-000000000002"},
				},
			},
		},
	}
	if err := PreflightUUIDCheck(specs, renderedInbounds); err != nil {
		t.Fatalf("counts match should pass: %v", err)
	}
}

func TestPreflightUUIDCheck_Mismatch(t *testing.T) {
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		tcpSpec("p02", "00000000-0000-0000-0000-000000000002"),
	}
	renderedInbounds := []interface{}{
		map[string]interface{}{
			"listen": "0.0.0.0",
			"port":   float64(443),
			"settings": map[string]interface{}{
				"clients": []interface{}{
					map[string]interface{}{"id": "00000000-0000-0000-0000-000000000001"},
					// 少了一个 client
				},
			},
		},
	}
	err := PreflightUUIDCheck(specs, renderedInbounds)
	if err == nil {
		t.Fatal("count mismatch should fail preflight")
	}
}

// ========== IsGroupedConfig 测试 ==========

func TestIsGroupedConfig_AbstractSocket(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{"listen": "@xhttp-internal"},
	}
	if !IsGroupedConfig(inbounds) {
		t.Error("abstract unix socket listen should be detected as grouped")
	}
}

func TestIsGroupedConfig_Fallbacks(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{
			"listen":    "0.0.0.0",
			"port":      float64(443),
			"fallbacks": []interface{}{map[string]interface{}{"dest": "@internal"}},
		},
	}
	if !IsGroupedConfig(inbounds) {
		t.Error("inbound with fallbacks should be detected as grouped")
	}
}

func TestIsGroupedConfig_Legacy(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{
			"listen": "0.0.0.0",
			"port":   float64(443),
		},
	}
	if IsGroupedConfig(inbounds) {
		t.Error("plain inbound without @listen or fallbacks should be legacy")
	}
}

// ========== BuildGroupsFromNodeSpecs 集成测试 ==========

func TestBuildGroupsFromNodeSpecs_PrimaryPlusInternal(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		tcpSpec("p01", "00000000-0000-0000-0000-000000000001"),
		xhttpSpec("p09", "/xhttp-up"),
	}
	groups, err := gb.BuildGroupsFromNodeSpecs(specs)
	if err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.Primary == nil {
		t.Fatal("primary inbound should exist")
	}
	if len(g.Primary.Fallbacks) != 1 {
		t.Fatalf("expected 1 fallback rule, got %d", len(g.Primary.Fallbacks))
	}
	if len(g.Internal) != 1 {
		t.Fatalf("expected 1 internal inbound, got %d", len(g.Internal))
	}
	// fallback dest 应该是 @xxx-internal（abstract unix socket）
	if g.Primary.Fallbacks[0].Dest != "@p09-internal" {
		t.Errorf("wrong fallback dest: %s", g.Primary.Fallbacks[0].Dest)
	}
	// internal inbound listen 应该是 @xxx-internal
	if g.Internal[0].Listen != "@p09-internal" {
		t.Errorf("wrong internal listen: %s", g.Internal[0].Listen)
	}
}

func TestBuildGroupsFromNodeSpecs_StandalonePort(t *testing.T) {
	gb := NewGroupBuilder()
	spec := tcpSpec("p02", "00000000-0000-0000-0000-000000000002")
	spec.Port = 8443 // 非 443，独立端口
	specs := []*nodespec.NodeSpec{spec}
	groups, err := gb.BuildGroupsFromNodeSpecs(specs)
	if err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Port != 8443 {
		t.Errorf("standalone group port should be 8443, got %d", groups[0].Port)
	}
	if len(groups[0].Internal) != 0 {
		t.Errorf("standalone group should have 0 internal, got %d", len(groups[0].Internal))
	}
}

func TestBuildGroupsFromNodeSpecs_PathConflict(t *testing.T) {
	gb := NewGroupBuilder()
	specs := []*nodespec.NodeSpec{
		xhttpSpec("p06", "/conflict"),
		xhttpSpec("p07", "/conflict"),
	}
	_, err := gb.BuildGroupsFromNodeSpecs(specs)
	if err == nil {
		t.Fatal("path conflict should fail build")
	}
}
