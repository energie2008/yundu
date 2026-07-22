package service

import (
	"strings"
	"testing"

	nodecrypto "github.com/airport-panel/node-service/internal/crypto"
	"github.com/airport-panel/node-service/internal/model"
)

// strPtr 定义在 health_service.go 中，此处复用

// ========== P0-1: GenerateSelfSignedCertPEM 测试 ==========

func TestGenerateSelfSignedCertPEM_DomainSAN(t *testing.T) {
	// 测试域名 SNI 生成证书，验证 DNSNames SAN
	certPEM, keyPEM, err := nodecrypto.GenerateSelfSignedCertPEM("example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCertPEM failed: %v", err)
	}
	if certPEM == "" || keyPEM == "" {
		t.Fatal("certPEM or keyPEM is empty")
	}
	if !strings.HasPrefix(certPEM, "-----BEGIN CERTIFICATE-----") {
		t.Error("certPEM does not start with CERTIFICATE header")
	}
	if !strings.HasPrefix(keyPEM, "-----BEGIN EC PRIVATE KEY-----") {
		t.Error("keyPEM does not start with EC PRIVATE KEY header")
	}
}

func TestGenerateSelfSignedCertPEM_IPSAN(t *testing.T) {
	// 测试 IP SNI 生成证书，验证 IPAddresses SAN
	certPEM, _, err := nodecrypto.GenerateSelfSignedCertPEM("192.168.1.1")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCertPEM with IP failed: %v", err)
	}
	if certPEM == "" {
		t.Fatal("certPEM is empty for IP SAN")
	}
}

func TestGenerateSelfSignedCertPEM_EmptyDomain(t *testing.T) {
	_, _, err := nodecrypto.GenerateSelfSignedCertPEM("")
	if err == nil {
		t.Error("empty domain should return error")
	}
}

// ========== P1-1: TLS 剥离函数拆分测试 ==========

func TestShouldStripTLSForArgoTunnel(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"argo_tunnel", true},
		{"cdn", false},
		{"cdn_saas", false},
		{"direct", false},
		{"reality", false},
		{"", false},
	}
	for _, c := range cases {
		got := shouldStripTLSForArgoTunnel(c.mode)
		if got != c.want {
			t.Errorf("shouldStripTLSForArgoTunnel(%q)=%v, want %v", c.mode, got, c.want)
		}
	}
}

func TestShouldStripTLSForNginxVhost(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"cdn", true},
		{"cdn_saas", true},
		{"argo_tunnel", false},
		{"direct", false},
		{"reality", false},
		{"", false},
	}
	for _, c := range cases {
		got := shouldStripTLSForNginxVhost(c.mode)
		if got != c.want {
			t.Errorf("shouldStripTLSForNginxVhost(%q)=%v, want %v", c.mode, got, c.want)
		}
	}
}

func TestShouldStripTLSForInbound_NarrowedToArgoOnly(t *testing.T) {
	// P1-1: shouldStripTLSForInbound 收窄为仅 argo_tunnel
	cases := []struct {
		mode string
		want bool
	}{
		{"argo_tunnel", true},
		{"cdn", false},   // P1-1: cdn 不再由此函数管
		{"cdn_saas", false}, // P1-1: cdn_saas 不再由此函数管
		{"direct", false},
	}
	for _, c := range cases {
		got := shouldStripTLSForInbound(c.mode)
		if got != c.want {
			t.Errorf("shouldStripTLSForInbound(%q)=%v, want %v", c.mode, got, c.want)
		}
	}
}

// ========== P1-3: PortPlanner 硬性校验测试 ==========

func TestValidatePortNotReserved(t *testing.T) {
	cases := []struct {
		port    int
		wantErr bool
	}{
		{443, true},   // nginx stream 专属
		{80, true},    // 系统保留
		{22, true},    // 系统保留
		{0, true},     // 非法
		{-1, true},    // 非法
		{65536, true}, // 超出范围
		{8446, false}, // 合法
		{9450, false}, // 合法
		{40020, false}, // 合法
	}
	for _, c := range cases {
		err := ValidatePortNotReserved(c.port)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidatePortNotReserved(%d) err=%v, wantErr=%v", c.port, err, c.wantErr)
		}
	}
}

func TestIsPortValidForNginxStream(t *testing.T) {
	if !IsPortValidForNginxStream(443) {
		t.Error("443 should be nginx stream port")
	}
	if IsPortValidForNginxStream(8446) {
		t.Error("8446 should not be nginx stream port")
	}
}

func TestIsPortUDP(t *testing.T) {
	if !IsPortUDP("hysteria2") {
		t.Error("hysteria2 should be UDP")
	}
	if !IsPortUDP("tuic") {
		t.Error("tuic should be UDP")
	}
	if IsPortUDP("vless") {
		t.Error("vless should not be UDP")
	}
}

// ========== P1-4: is_split_mode 一致性校验测试 ==========

func TestValidateSplitModeConsistency(t *testing.T) {
	// 正常：is_split_mode=true + downstream_exposure_mode 有值
	n1 := &model.Node{
		Code:                  "test1",
		IsSplitMode:           true,
		DownstreamExposureMode: strPtr("direct"),
		ConfigJSON:            map[string]interface{}{"is_split_mode": true, "downstream_exposure_mode": "direct"},
	}
	if err := validateSplitModeConsistency(n1); err != nil {
		t.Errorf("consistent split mode should pass: %v", err)
	}

	// 异常：is_split_mode=true 但 downstream_exposure_mode 为空
	n2 := &model.Node{
		Code:        "test2",
		IsSplitMode: true,
		ConfigJSON:  map[string]interface{}{"is_split_mode": true},
	}
	if err := validateSplitModeConsistency(n2); err == nil {
		t.Error("is_split_mode=true with empty downstream_exposure_mode should fail")
	}

	// 正常：is_split_mode=false + downstream_exposure_mode 为空
	n3 := &model.Node{
		Code:        "test3",
		IsSplitMode: false,
		ConfigJSON:  map[string]interface{}{"is_split_mode": false},
	}
	if err := validateSplitModeConsistency(n3); err != nil {
		t.Errorf("non-split mode should pass: %v", err)
	}

	// 异常：DB is_split_mode 与 config_json.is_split_mode 不一致
	n4 := &model.Node{
		Code:        "test4",
		IsSplitMode: true,
		ConfigJSON:  map[string]interface{}{"is_split_mode": false},
	}
	if err := validateSplitModeConsistency(n4); err == nil {
		t.Error("DB/config_json is_split_mode mismatch should fail")
	}
}

func TestAutoCorrectSplitModeConsistency(t *testing.T) {
	// 异常状态自动修正
	n := &model.Node{
		Code:        "test",
		IsSplitMode: true,
		ConfigJSON:  map[string]interface{}{"is_split_mode": true},
		// downstream_exposure_mode 为空
	}
	autoCorrectSplitModeConsistency(n)
	if n.IsSplitMode {
		t.Error("auto-correct should set IsSplitMode=false when downstream_exposure_mode is empty")
	}
}

// ========== P1-6: TLS 分离字段校验测试 ==========

func TestValidateTLSSplitFields_ArgoTunnelValid(t *testing.T) {
	// 正常 argo_tunnel: DB=none, config_json=tls
	noneSec := "none"
	n := &model.Node{
		Code:         "argo-node",
		SecurityType: &noneSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode":  "argo_tunnel",
			"security_type":  "tls",
			"security":       "tls",
			"tls":            1,
			"cdn_address":    "argo.example.com",
			"cloudflared_tunnel_id": "test-tunnel",
		},
	}
	if err := validateTLSSplitFields(n); err != nil {
		t.Errorf("valid argo_tunnel should pass: %v", err)
	}
}

func TestValidateTLSSplitFields_ArgoTunnelInvalid(t *testing.T) {
	// 异常 argo_tunnel: DB=tls（应为 none）
	tlsSec := "tls"
	n := &model.Node{
		Code:         "argo-node-bad",
		SecurityType: &tlsSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode":         "argo_tunnel",
			"security_type":         "tls",
			"security":              "tls",
			"tls":                   1,
			"cdn_address":           "argo.example.com",
			"cloudflared_tunnel_id": "test-tunnel",
		},
	}
	if err := validateTLSSplitFields(n); err == nil {
		t.Error("argo_tunnel with DB security_type=tls should fail")
	}
}

func TestValidateNonSplitTLSConsistency(t *testing.T) {
	// 异常：非分离节点（direct）出现分离状态
	noneSec := "none"
	n := &model.Node{
		Code:         "direct-node",
		SecurityType: &noneSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode": "direct",
			"security_type": "tls",
		},
	}
	if err := validateNonSplitTLSConsistency(n); err == nil {
		t.Error("non-split node with split (DB=none, config=tls) should fail")
	}

	// 正常：direct 节点无分离
	tlsSec := "tls"
	n2 := &model.Node{
		Code:         "direct-node-ok",
		SecurityType: &tlsSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode": "direct",
			"security_type": "tls",
		},
	}
	if err := validateNonSplitTLSConsistency(n2); err != nil {
		t.Errorf("direct node with consistent TLS should pass: %v", err)
	}

	// 正常：cdn_saas 节点 DB=tls, config=tls（不分离，TLS 剥离在渲染层完成）
	n3 := &model.Node{
		Code:         "cdn-node-ok",
		SecurityType: &tlsSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode": "cdn_saas",
			"security_type": "tls",
			"security":      "tls",
			"tls":           1,
			"cdn_address":   "cdn.example.com",
		},
	}
	if err := validateNonSplitTLSConsistency(n3); err != nil {
		t.Errorf("cdn_saas node with consistent TLS (DB=tls, config=tls) should pass: %v", err)
	}

	// 异常：cdn 节点出现分离状态（DB=none, config=tls）——CDN 节点不允许 DB 字段分离
	n4 := &model.Node{
		Code:         "cdn-node-bad",
		SecurityType: &noneSec,
		ConfigJSON: map[string]interface{}{
			"exposure_mode": "cdn",
			"security_type": "tls",
			"security":      "tls",
			"tls":           1,
			"cdn_address":   "cdn.example.com",
		},
	}
	if err := validateNonSplitTLSConsistency(n4); err == nil {
		t.Error("cdn node with DB=none/config=tls split should fail (only argo_tunnel allowed)")
	}
}

// ========== P2-1: TerminationClass 测试 ==========

func TestClassifyTermination(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
		want TerminationClass
	}{
		{
			name: "argo_tunnel",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"exposure_mode":         "argo_tunnel",
					"cloudflared_tunnel_id": "test",
				},
			},
			want: TerminationCFEdge,
		},
		{
			name: "cdn_saas",
			node: &model.Node{
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "cdn_saas",
					"cdn_address":   "cdn.example.com",
				},
			},
			want: TerminationNginx,
		},
		{
			name: "direct_tls",
			node: &model.Node{
				SecurityType: strPtr("tls"),
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "direct",
				},
			},
			want: TerminationSelfTCP,
		},
		{
			name: "hysteria2_udp",
			node: &model.Node{
				ProtocolType: "hysteria2",
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "direct",
				},
			},
			want: TerminationSelfUDP,
		},
		{
			name: "reality",
			node: &model.Node{
				SecurityType: strPtr("reality"),
				ConfigJSON: map[string]interface{}{
					"exposure_mode": "direct",
				},
			},
			want: TerminationReality,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyTermination(c.node)
			if got != c.want {
				t.Errorf("ClassifyTermination=%s, want %s", got, c.want)
			}
		})
	}
}

func TestTerminationClassNeedsCertBundle(t *testing.T) {
	if !TerminationSelfTCP.NeedsCertBundle() {
		t.Error("self_tcp should need cert bundle")
	}
	if !TerminationSelfUDP.NeedsCertBundle() {
		t.Error("self_udp should need cert bundle")
	}
	if TerminationCFEdge.NeedsCertBundle() {
		t.Error("cf_edge should not need cert bundle")
	}
	if TerminationNginx.NeedsCertBundle() {
		t.Error("nginx should not need cert bundle")
	}
	if TerminationReality.NeedsCertBundle() {
		t.Error("reality should not need cert bundle")
	}
}

func TestTerminationClassNeedsTLSStrip(t *testing.T) {
	if !TerminationCFEdge.NeedsTLSStrip() {
		t.Error("cf_edge should need TLS strip")
	}
	if !TerminationNginx.NeedsTLSStrip() {
		t.Error("nginx should need TLS strip")
	}
	if TerminationSelfTCP.NeedsTLSStrip() {
		t.Error("self_tcp should not need TLS strip")
	}
	if TerminationReality.NeedsTLSStrip() {
		t.Error("reality should not need TLS strip")
	}
}

// ========== P2-4: ExposurePolicy 映射表测试 ==========

func TestGetExposurePolicy(t *testing.T) {
	cases := []struct {
		mode          string
		wantStrip     bool
		wantVhost     bool
		wantStreamSNI bool
		wantCert      bool
	}{
		{"direct", false, false, true, true},
		{"reality", false, false, true, false},
		{"cdn", true, true, true, false},
		{"cdn_saas", true, true, true, false},
		{"argo_tunnel", true, false, false, false},
		{"none", false, false, true, false},
		{"unknown_mode", false, false, true, true}, // 回退到 direct
	}

	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			p := GetExposurePolicy(c.mode)
			if p.StripTLS != c.wantStrip {
				t.Errorf("StripTLS=%v, want %v", p.StripTLS, c.wantStrip)
			}
			if p.NeedsNginxVhost != c.wantVhost {
				t.Errorf("NeedsNginxVhost=%v, want %v", p.NeedsNginxVhost, c.wantVhost)
			}
			if p.NeedsStreamSNI != c.wantStreamSNI {
				t.Errorf("NeedsStreamSNI=%v, want %v", p.NeedsStreamSNI, c.wantStreamSNI)
			}
			if p.NeedsCertBundle != c.wantCert {
				t.Errorf("NeedsCertBundle=%v, want %v", p.NeedsCertBundle, c.wantCert)
			}
		})
	}
}

func TestExposurePolicyConsistencyWithTerminationClass(t *testing.T) {
	// P2-4: ExposurePolicy 的 StripTLS/NeedsNginxVhost/NeedsStreamSNI 应与 TerminationClass 一致
	for _, mode := range AllExposurePolicies() {
		p := GetExposurePolicy(mode)
		// 构造一个对应 mode 的 node 来分类
		node := &model.Node{
			ConfigJSON: map[string]interface{}{
				"exposure_mode": mode,
			},
		}
		// 为各模式设置合理的 security_type
		switch mode {
		case "argo_tunnel":
			node.ConfigJSON["cloudflared_tunnel_id"] = "test"
			noneSec := "none"
			node.SecurityType = &noneSec
		case "cdn", "cdn_saas":
			node.ConfigJSON["cdn_address"] = "cdn.example.com"
			tlsSec := "tls"
			node.SecurityType = &tlsSec
		case "reality":
			node.SecurityType = strPtr("reality")
		case "direct":
			tlsSec := "tls"
			node.SecurityType = &tlsSec
		case "none":
			// none 模式无 TLS，不参与 cert bundle 一致性校验
			noneSec := "none"
			node.SecurityType = &noneSec
		}

		tc := ClassifyTermination(node)

		// StripTLS 一致性
		if p.StripTLS != tc.NeedsTLSStrip() {
			t.Errorf("mode=%s: policy StripTLS=%v but TC NeedsTLSStrip=%v", mode, p.StripTLS, tc.NeedsTLSStrip())
		}
		// NeedsNginxVhost 一致性
		if p.NeedsNginxVhost != tc.NeedsNginxVhost() {
			t.Errorf("mode=%s: policy NeedsNginxVhost=%v but TC NeedsNginxVhost=%v", mode, p.NeedsNginxVhost, tc.NeedsNginxVhost())
		}
		// NeedsStreamSNI 一致性
		if p.NeedsStreamSNI != tc.NeedsStreamSNI() {
			t.Errorf("mode=%s: policy NeedsStreamSNI=%v but TC NeedsStreamSNI=%v", mode, p.NeedsStreamSNI, tc.NeedsStreamSNI())
		}
		// NeedsCertBundle 一致性（none 模式无 TLS，跳过此校验）
		if mode != "none" {
			if p.NeedsCertBundle != tc.NeedsCertBundle() {
				t.Errorf("mode=%s: policy NeedsCertBundle=%v but TC NeedsCertBundle=%v", mode, p.NeedsCertBundle, tc.NeedsCertBundle())
			}
		}
	}
}

func TestIsKnownExposureMode(t *testing.T) {
	known := []string{"direct", "reality", "cdn", "cdn_saas", "argo_tunnel", "none"}
	for _, m := range known {
		if !IsKnownExposureMode(m) {
			t.Errorf("%s should be known exposure mode", m)
		}
	}
	if IsKnownExposureMode("invalid_mode") {
		t.Error("invalid_mode should not be known")
	}
}
