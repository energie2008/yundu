package nodespec

import "testing"

func validBase() NodeSpec {
	return NodeSpec{
		ID: "node-1", Code: "hk1", Name: "香港节点",
		Protocol: ProtocolVLESS, Address: "1.2.3.4", Port: 443,
		Transport: TransportConfig{Type: TransportWS, WS: &WSConfig{Path: "/ws"}},
		Security:  SecurityTLS, TLS: &TLSConfig{SNI: "example.com", Fingerprint: "chrome"},
		Credentials: VLESSCredentials{UUID: "00000000-0000-0000-0000-000000000001"},
		TrafficRate: 1.0,
	}
}

func TestValidVLESSTLS(t *testing.T) {
	n := validBase()
	if err := n.Validate(); err != nil {
		t.Fatalf("valid vless+ws+tls should pass: %v", err)
	}
}

func TestValidVLESSReality(t *testing.T) {
	n := validBase()
	n.Security = SecurityReality
	n.TLS = nil
	n.Reality = &RealityConfig{PublicKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", ShortID: "aaaa1234bbbb5678"}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid vless+reality should pass: %v", err)
	}
}

func TestRealityWithNonVLESS(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolTrojan
	n.Security = SecurityReality
	n.Credentials = TrojanCredentials{Password: "password123456"}
	if err := n.Validate(); err == nil {
		t.Fatal("reality with trojan should fail")
	}
}

func TestMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		modify func(n *NodeSpec)
	}{
		{"missing id", func(n *NodeSpec) { n.ID = "" }},
		{"missing code", func(n *NodeSpec) { n.Code = "" }},
		{"missing name", func(n *NodeSpec) { n.Name = "" }},
		{"missing address", func(n *NodeSpec) { n.Address = "" }},
		{"port 0", func(n *NodeSpec) { n.Port = 0 }},
		{"port >65535", func(n *NodeSpec) { n.Port = 99999 }},
		{"port negative", func(n *NodeSpec) { n.Port = -1 }},
		{"invalid protocol", func(n *NodeSpec) { n.Protocol = "invalid" }},
		{"invalid transport", func(n *NodeSpec) { n.Transport.Type = "invalid" }},
		{"invalid security", func(n *NodeSpec) { n.Security = "invalid" }},
		{"client port invalid", func(n *NodeSpec) { n.ClientPort = 99999 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := validBase()
			tc.modify(&n)
			if err := n.Validate(); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestInvalidUUID(t *testing.T) {
	n := validBase()
	n.Credentials = VLESSCredentials{UUID: "not-a-uuid"}
	if err := n.Validate(); err == nil {
		t.Fatal("invalid vless uuid should fail")
	}
}

func TestEmptyUUID(t *testing.T) {
	n := validBase()
	n.Credentials = VLESSCredentials{UUID: ""}
	if err := n.Validate(); err == nil {
		t.Fatal("empty vless uuid should fail")
	}
}

func TestVMessValid(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolVMess
	n.Credentials = VMessCredentials{UUID: "00000000-0000-0000-0000-000000000002", AlterID: 0}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid vmess should pass: %v", err)
	}
}

func TestVMessInvalidUUID(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolVMess
	n.Credentials = VMessCredentials{UUID: "bad"}
	if err := n.Validate(); err == nil {
		t.Fatal("invalid vmess uuid should fail")
	}
}

func TestTrojanValid(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolTrojan
	n.Credentials = TrojanCredentials{Password: "longpassword123"}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid trojan should pass: %v", err)
	}
}

func TestTrojanShortPassword(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolTrojan
	n.Credentials = TrojanCredentials{Password: "short"}
	if err := n.Validate(); err == nil {
		t.Fatal("short trojan password should fail")
	}
}

func TestSSValid(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolShadowsocks
	n.Transport = TransportConfig{Type: TransportTCP}
	n.Security = SecurityNone
	n.TLS = nil
	n.Credentials = ShadowsocksCredentials{Password: "pass123456", Method: "chacha20-ietf-poly1305"}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid ss should pass: %v", err)
	}
}

func TestSSInvalidMethod(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolShadowsocks
	n.Transport = TransportConfig{Type: TransportTCP}
	n.Security = SecurityNone
	n.TLS = nil
	n.Credentials = ShadowsocksCredentials{Password: "pass123456", Method: "rc4-md5"}
	if err := n.Validate(); err == nil {
		t.Fatal("unsupported ss method should fail")
	}
}

func TestSSMissingMethod(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolShadowsocks
	n.Transport = TransportConfig{Type: TransportTCP}
	n.Security = SecurityNone
	n.TLS = nil
	n.Credentials = ShadowsocksCredentials{Password: "pass123"}
	if err := n.Validate(); err == nil {
		t.Fatal("ss without method should fail")
	}
}

func TestHysteria2Valid(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolHysteria2
	n.Transport = TransportConfig{Type: TransportQUIC}
	n.Credentials = Hysteria2Credentials{Password: "hysteria-pw-123"}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid hysteria2 should pass: %v", err)
	}
}

func TestHysteria2NoPassword(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolHysteria2
	n.Transport = TransportConfig{Type: TransportQUIC}
	n.Credentials = Hysteria2Credentials{}
	if err := n.Validate(); err == nil {
		t.Fatal("hysteria2 without password should fail")
	}
}

func TestTUICValid(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolTUIC
	n.Transport = TransportConfig{Type: TransportQUIC}
	n.Credentials = TUICCredentials{UUID: "00000000-0000-0000-0000-000000000003", Password: "tuic-pw"}
	if err := n.Validate(); err != nil {
		t.Fatalf("valid tuic should pass: %v", err)
	}
}

func TestTUICInvalidUUID(t *testing.T) {
	n := validBase()
	n.Protocol = ProtocolTUIC
	n.Transport = TransportConfig{Type: TransportQUIC}
	n.Credentials = TUICCredentials{UUID: "bad-uuid"}
	if err := n.Validate(); err == nil {
		t.Fatal("tuic with bad uuid should fail")
	}
}

func TestRealityMissingPublicKey(t *testing.T) {
	n := validBase()
	n.Security = SecurityReality
	n.TLS = nil
	n.Reality = &RealityConfig{ShortID: "aaaa1234bbbb5678"}
	if err := n.Validate(); err == nil {
		t.Fatal("reality without public_key should fail")
	}
}

func TestWSWithoutConfig(t *testing.T) {
	n := validBase()
	n.Transport = TransportConfig{Type: TransportWS}
	if err := n.Validate(); err == nil {
		t.Fatal("ws transport without ws config should fail")
	}
}

func TestGRPCWithoutServiceName(t *testing.T) {
	n := validBase()
	n.Transport = TransportConfig{Type: TransportGRPC}
	if err := n.Validate(); err == nil {
		t.Fatal("grpc transport without service_name should fail")
	}
}

func TestNilCredentials(t *testing.T) {
	n := validBase()
	n.Credentials = nil
	if err := n.Validate(); err == nil {
		t.Fatal("nil credentials should fail")
	}
}

func TestInvalidTLSFingerprint(t *testing.T) {
	n := validBase()
	n.TLS.Fingerprint = "windows"
	if err := n.Validate(); err == nil {
		t.Fatal("invalid fingerprint should fail")
	}
}

func TestMapCredentialsVLESS(t *testing.T) {
	n := validBase()
	n.Credentials = map[string]interface{}{"uuid": "00000000-0000-0000-0000-000000000001"}
	if err := n.Validate(); err != nil {
		t.Fatalf("map credentials vless should pass: %v", err)
	}
}

func TestMapCredentialsInvalidVLESS(t *testing.T) {
	n := validBase()
	n.Credentials = map[string]interface{}{"uuid": "bad"}
	if err := n.Validate(); err == nil {
		t.Fatal("map credentials bad vless uuid should fail")
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"00000000-0000-0000-0000-000000000000", true},
		{"abcdefab-1234-5678-90ab-abcdef012345", true},
		{"not-a-uuid", false},
		{"", false},
		{"ABCDEFAB-1234-5678-90AB-ABCDEF012345", true},
	}
	for _, tc := range tests {
		if got := isValidUUID(tc.s); got != tc.want {
			t.Errorf("isValidUUID(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestTrafficRateDefault(t *testing.T) {
	n := validBase()
	n.TrafficRate = 0
	if err := n.Validate(); err != nil {
		t.Fatalf("0 traffic rate should default: %v", err)
	}
	if n.TrafficRate != 1.0 {
		t.Errorf("traffic rate should default to 1.0, got %v", n.TrafficRate)
	}
}

// TestRenderClientFlow 验证 flow 字段渲染的唯一出口函数。
// 覆盖所有现有 transport 类型，确保 TCP→Vision、其他→空。
func TestRenderClientFlow(t *testing.T) {
	tests := []struct {
		name      string
		transport Transport
		wantFlow  FlowControl
	}{
		{"TCP=Vision", TransportTCP, FlowXTLSRprxVision},
		{"WS=空", TransportWS, FlowNone},
		{"gRPC=空", TransportGRPC, FlowNone},
		{"HTTP2=空", TransportHTTP2, FlowNone},
		{"HTTPUpgrade=空", TransportHTTPUpgrade, FlowNone},
		{"XHTTP=空", TransportXHTTP, FlowNone},
		{"KCP=空", TransportKCP, FlowNone},
		{"QUIC=空", TransportQUIC, FlowNone},
		{"未知类型=空", Transport("unknown"), FlowNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderClientFlow(tt.transport)
			if got != tt.wantFlow {
				t.Errorf("RenderClientFlow(%q) = %q, want %q",
					tt.transport, got, tt.wantFlow)
			}
		})
	}
}
