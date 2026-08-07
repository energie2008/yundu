package dnsproviders

import (
	"strings"
	"testing"
)

// TestAllProvidersRegistered 验证全部 22 个 DNS provider 都已注册到 registry。
// 这是 P1-2 的验收测试：20 个真实实现 + 2 个 stub（vultr/namedotcom 因 libdns v0.2.x 不兼容）。
func TestAllProvidersRegistered(t *testing.T) {
	expected := []string{
		// 原有 11 个（v1.3 之前已实现）
		"acmedns", "alidns", "cloudflare", "digitalocean", "gandi",
		"googlecloud", "hetzner", "linode", "namecheap", "route53", "tencentcloud",
		// P1-2 新增 9 个真实实现
		"azure", "loopia", "namesilo", "netcup", "ovh",
		"powerdns", "rfc2136", "scaleway", "transip",
		// 保留为 stub（libdns 模块未兼容 v1.x）
		"vultr", "namedotcom",
	}

	names := CanonicalNames()
	if len(names) != len(expected) {
		t.Fatalf("期望 %d 个 provider，实际 %d 个：%v", len(expected), len(names), names)
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	for _, e := range expected {
		if !nameSet[e] {
			t.Errorf("provider %q 未注册", e)
		}
	}
}

// TestStubProvidersReturnError 验证 stub provider 调用 Build 时返回明确的错误引导信息。
func TestStubProvidersReturnError(t *testing.T) {
	stubs := []string{"vultr", "namedotcom"}
	for _, name := range stubs {
		p, ok := Get(name)
		if !ok {
			t.Fatalf("stub provider %q 未注册", name)
		}
		_, err := p.Build(map[string]string{})
		if err == nil {
			t.Errorf("stub provider %q 应返回错误，但返回 nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "未编译入二进制") {
			t.Errorf("stub provider %q 错误消息不含引导信息：%v", name, err)
		}
	}
}

// TestRealProvidersBuildNoPanic 验证真实 provider 的 Build 函数在空 env 下不 panic。
// Build 可能返回 nil error（因为很多 provider 不在 Build 阶段校验 env），
// 关键是不 panic——真正校验在 certmagic 调用 DNS-01 时发生。
func TestRealProvidersBuildNoPanic(t *testing.T) {
	realProviders := []string{
		"acmedns", "alidns", "cloudflare", "digitalocean", "gandi",
		"googlecloud", "hetzner", "linode", "namecheap", "route53", "tencentcloud",
		"azure", "loopia", "namesilo", "netcup", "ovh",
		"powerdns", "rfc2136", "scaleway", "transip",
	}
	for _, name := range realProviders {
		p, ok := Get(name)
		if !ok {
			t.Fatalf("provider %q 未注册", name)
			continue
		}
		// 空 env 不应 panic
		_, _ = p.Build(map[string]string{})
	}
}

// TestAliases 验证关键 provider 的别名注册正确。
func TestAliases(t *testing.T) {
	aliases := map[string][]string{
		"cloudflare":   {"cf"},
		"alidns":       {"aliyun", "alicloud"},
		"tencentcloud": {"dnspod", "tencent"},
		"route53":      {"aws"},
		"digitalocean": {"do"},
		"azure":        {"azuredns"},
		"powerdns":     {"pdns"},
		"rfc2136":      {"bind"},
		"scaleway":     {"scw"},
		"namedotcom":   {"namecom"},
	}
	for canonical, aliasList := range aliases {
		for _, alias := range aliasList {
			p, ok := Get(alias)
			if !ok {
				t.Errorf("别名 %q 未注册（应指向 %q）", alias, canonical)
				continue
			}
			if p.Name != canonical {
				t.Errorf("别名 %q 指向 %q，期望 %q", alias, p.Name, canonical)
			}
		}
	}
}
