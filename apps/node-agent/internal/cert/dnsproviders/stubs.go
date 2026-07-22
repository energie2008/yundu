// Package dnsproviders - 未实现的 DNS provider 存根
//
// 阶段 B2: 为剩余 18 个 DNS provider 注册元信息（名称/别名/环境变量），
// 但实际 Build 返回错误，引导用户添加对应的 libdns 依赖。
//
// 当用户需要某个 provider 时，在 go.mod 中添加对应的 libdns 模块，
// 然后将此文件中对应条目的 stubBuild 替换为真实实现即可。
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
	// 4. 腾讯云 / DNSPod
	Register(&Provider{
		Name:    "tencentcloud",
		Aliases: []string{"dnspod", "tencent"},
		EnvVars: []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY"},
		Build:   stubBuild("tencentcloud", "github.com/libdns/tencentcloud"),
	})

	// 5. AWS Route53
	Register(&Provider{
		Name:    "route53",
		Aliases: []string{"aws"},
		EnvVars: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"},
		Build:   stubBuild("route53", "github.com/libdns/route53"),
	})

	// 6. Azure DNS
	Register(&Provider{
		Name:    "azure",
		Aliases: []string{"azuredns"},
		EnvVars: []string{"AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_TENANT_ID", "AZURE_SUBSCRIPTION_ID"},
		Build:   stubBuild("azure", "github.com/libdns/azure"),
	})

	// 7. DigitalOcean
	Register(&Provider{
		Name:    "digitalocean",
		Aliases: []string{"do"},
		EnvVars: []string{"DO_AUTH_TOKEN"},
		Build:   stubBuild("digitalocean", "github.com/libdns/digitalocean"),
	})

	// 8. Gandi
	Register(&Provider{
		Name:    "gandi",
		Aliases: []string{},
		EnvVars: []string{"GANDI_API_KEY"},
		Build:   stubBuild("gandi", "github.com/libdns/gandi"),
	})

	// 9. Google Cloud DNS
	Register(&Provider{
		Name:    "googlecloud",
		Aliases: []string{"gcp", "googledns"},
		EnvVars: []string{"GCE_PROJECT", "GOOGLE_APPLICATION_CREDENTIALS"},
		Build:   stubBuild("googlecloud", "github.com/libdns/googleclouddns"),
	})

	// 10. Hetzner
	Register(&Provider{
		Name:    "hetzner",
		Aliases: []string{},
		EnvVars: []string{"HETZNER_API_KEY"},
		Build:   stubBuild("hetzner", "github.com/libdns/hetzner"),
	})

	// 11. Linode
	Register(&Provider{
		Name:    "linode",
		Aliases: []string{},
		EnvVars: []string{"LINODE_TOKEN"},
		Build:   stubBuild("linode", "github.com/libdns/linode"),
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
	Register(&Provider{
		Name:    "vultr",
		Aliases: []string{},
		EnvVars: []string{"VULTR_API_KEY"},
		Build:   stubBuild("vultr", "github.com/libdns/vultr"),
	})

	// 15. Namecheap
	Register(&Provider{
		Name:    "namecheap",
		Aliases: []string{},
		EnvVars: []string{"NAMECHEAP_API_USER", "NAMECHEAP_API_KEY"},
		Build:   stubBuild("namecheap", "github.com/libdns/namecheap"),
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
}
