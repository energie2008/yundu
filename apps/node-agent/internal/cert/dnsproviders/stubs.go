// Package dnsproviders - 未实现的 DNS provider 存根
//
// 已实现的 provider（各自的 .go 文件）：
//   - cloudflare / alidns / acmedns
//   - tencentcloud (DNSPod) / route53 (AWS) / googlecloud (GCP)
//   - digitalocean / hetzner / linode / gandi / namecheap
//
// 以下 provider 尚未实现，Build 返回引导用户添加依赖的错误。
// 当用户需要某个 provider 时，在 go.mod 中添加对应的 libdns 模块，
// 然后创建对应的 .go 文件并将此文件中对应条目的 stubBuild 替换为真实实现。
//
// 注意：vultr 和 namedotcom (name.com) 的最新 libdns 包仍基于 libdns v0.2.x
// 旧版 API（libdns.Record 为结构体），与本项目使用的 libdns v1.x（Record 为接口）
// 不兼容，因此暂保留为 stub。
//
// 完整 provider 列表参考：
// https://github.com/cedar2025/Xboard-Node/blob/dev/docs-dns-providers.md

package dnsproviders

import (
	"fmt"

	"github.com/caddyserver/certmagic"
)

// stubBuild 返回一个引导用户添加依赖的错误。
func stubBuild(providerName, libdnsModule string) func(env map[string]string) (certmagic.DNSProvider, error) {
	return func(env map[string]string) (certmagic.DNSProvider, error) {
		return nil, fmt.Errorf(
			"dns_provider %q 未编译入二进制；请运行 `go get %s` 并替换 stubBuild 为真实实现",
			providerName, libdnsModule,
		)
	}
}

func init() {
	// 6. Azure DNS
	Register(&Provider{
		Name:    "azure",
		Aliases: []string{"azuredns"},
		EnvVars: []string{"AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_TENANT_ID", "AZURE_SUBSCRIPTION_ID"},
		Build:   stubBuild("azure", "github.com/libdns/azure"),
	})

	// 12. OVH
	Register(&Provider{
		Name:    "ovh",
		Aliases: []string{},
		EnvVars: []string{"OVH_ENDPOINT", "OVH_APPLICATION_KEY", "OVH_APPLICATION_SECRET", "OVH_CONSUMER_KEY"},
		Build:   stubBuild("ovh", "github.com/libdns/ovh"),
	})

	// 13. RFC 2136
	Register(&Provider{
		Name:    "rfc2136",
		Aliases: []string{"bind"},
		EnvVars: []string{"RFC2136_NAMESERVER", "RFC2136_TSIG_KEY", "RFC2136_TSIG_ALGORITHM", "RFC2136_TSIG_SECRET"},
		Build:   stubBuild("rfc2136", "github.com/libdns/rfc2136"),
	})

	// 14. Vultr
	// 注意：github.com/libdns/vultr v1.0.0 仍使用 libdns v0.2.x 旧版 API，
	// 与本项目 libdns v1.x 不兼容，暂保留为 stub。
	Register(&Provider{
		Name:    "vultr",
		Aliases: []string{},
		EnvVars: []string{"VULTR_API_KEY"},
		Build:   stubBuild("vultr", "github.com/libdns/vultr"),
	})

	// 16. Namesilo
	Register(&Provider{
		Name:    "namesilo",
		Aliases: []string{},
		EnvVars: []string{"NAMESILO_API_KEY"},
		Build:   stubBuild("namesilo", "github.com/libdns/namesilo"),
	})

	// 17. PowerDNS
	Register(&Provider{
		Name:    "powerdns",
		Aliases: []string{"pdns"},
		EnvVars: []string{"PDNS_API_URL", "PDNS_API_KEY"},
		Build:   stubBuild("powerdns", "github.com/libdns/powerdns"),
	})

	// 18. TransIP
	Register(&Provider{
		Name:    "transip",
		Aliases: []string{},
		EnvVars: []string{"TRANSIP_ACCOUNT_NAME", "TRANSIP_PRIVATE_KEY_PATH"},
		Build:   stubBuild("transip", "github.com/libdns/transip"),
	})

	// 19. Loopia
	Register(&Provider{
		Name:    "loopia",
		Aliases: []string{},
		EnvVars: []string{"LOOPIA_API_USER", "LOOPIA_API_PASSWORD"},
		Build:   stubBuild("loopia", "github.com/libdns/loopia"),
	})

	// 20. Netcup
	Register(&Provider{
		Name:    "netcup",
		Aliases: []string{},
		EnvVars: []string{"NETCUP_CUSTOMER_NUMBER", "NETCUP_API_KEY", "NETCUP_API_PASSWORD"},
		Build:   stubBuild("netcup", "github.com/libdns/netcup"),
	})

	// 21. Scaleway
	Register(&Provider{
		Name:    "scaleway",
		Aliases: []string{"scw"},
		EnvVars: []string{"SCW_ACCESS_KEY", "SCW_SECRET_KEY"},
		Build:   stubBuild("scaleway", "github.com/libdns/scaleway"),
	})

	// 22. Name.com (namedotcom)
	// 注意：github.com/libdns/namedotcom v0.3.3 仍使用 libdns v0.2.x 旧版 API，
	// 与本项目 libdns v1.x 不兼容，暂保留为 stub。
	Register(&Provider{
		Name:    "namedotcom",
		Aliases: []string{"namecom"},
		EnvVars: []string{"NAMEDOTCOM_TOKEN", "NAMEDOTCOM_USER"},
		Build:   stubBuild("namedotcom", "github.com/libdns/namedotcom"),
	})
}
