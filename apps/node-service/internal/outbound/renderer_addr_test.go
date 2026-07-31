package outbound

import "testing"

// TestEnsureCIDR 校验无前缀 IP 自动补全 CIDR（修复 vps206 warp local_address 无 /32 导致
// sing-box endpoint 校验失败 "netip.ParsePrefix: no '/'" 的问题）。
func TestEnsureCIDR(t *testing.T) {
	cases := map[string]string{
		"172.16.0.2":                              "172.16.0.2/32",
		"172.16.0.2/32":                           "172.16.0.2/32",
		"2606:4700:110:83dd:196f:c109:5ce2:a1cf":  "2606:4700:110:83dd:196f:c109:5ce2:a1cf/128",
		"2606:4700:110:8d11:bab4:845c:6f70:8d13/128": "2606:4700:110:8d11:bab4:845c:6f70:8d13/128",
		"":                                        "",
	}
	for in, want := range cases {
		if got := ensureCIDR(in); got != want {
			t.Errorf("ensureCIDR(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSplitLocalAddresses_NormalizesCIDR 校验双栈无前缀地址被拆分并补全前缀。
func TestSplitLocalAddresses_NormalizesCIDR(t *testing.T) {
	// vps206 实际数据：无 /32 /128 前缀的双栈地址
	got := splitLocalAddresses("172.16.0.2, 2606:4700:110:83dd:196f:c109:5ce2:a1cf")
	want := []string{"172.16.0.2/32", "2606:4700:110:83dd:196f:c109:5ce2:a1cf/128"}
	if len(got) != len(want) {
		t.Fatalf("splitLocalAddresses len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitLocalAddresses[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
