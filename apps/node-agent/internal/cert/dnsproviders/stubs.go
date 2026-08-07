// Package dnsproviders - 未实现的 DNS provider 存根
//
// P1-2 完成里程碑：22 个目标 provider 中 20 个已实现真实代码，2 个保留为 stub。
//
// 已实现的 provider（各自的 .go 文件）：
//   1.  cloudflare       (libdns/cloudflare)
//   2.  alidns           (libdns/alidns)          — 阿里云
//   3.  acmedns          (libdns/acmedns)
//   4.  tencentcloud     (libdns/tencentcloud)     — 腾讯云/DNSPod
//   5.  route53          (libdns/route53)          — AWS
//   6.  googlecloud      (libdns/googleclouddns)   — GCP
//   7.  digitalocean     (libdns/digitalocean)
//   8.  hetzner          (libdns/hetzner)
//   9.  linode           (libdns/linode)
//  10.  gandi            (libdns/gandi)
//  11.  namecheap         (libdns/namecheap)
//  12.  azure            (libdns/azure)             — Microsoft Azure
//  13.  ovh              (libdns/ovh)
//  14.  rfc2136          (libdns/rfc2136)          — BIND/PowerDNS 动态更新
//  15.  namesilo         (libdns/namesilo)
//  16.  powerdns         (libdns/powerdns)
//  17.  transip          (libdns/transip)
//  18.  loopia           (libdns/loopia)
//  19.  netcup           (libdns/netcup)
//  20.  scaleway         (libdns/scaleway)
//
// 以下 2 个 provider 因 libdns 模块未升级至 v1.x API 保留为 stub：
//  21.  vultr            — github.com/libdns/vultr v1.0.0 仍使用 libdns v0.2.x 旧版 API
//                        （record.ID/Name/Type 字段访问 + libdns.Record 复合字面量），
//                        与本项目 libdns v1.x（Record 为接口）不兼容，编译失败。
//  22.  namedotcom       — github.com/libdns/namedotcom v0.3.3 同样不兼容 libdns v1.x。
//
// 当上游模块升级后，创建对应 .go 文件并将此文件中对应条目的 stubBuild 替换为真实实现。
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
			"dns_provider %q 未编译入二进制（libdns 模块未兼容 v1.x API）；"+
				"待上游 %s 升级后实现",
			providerName, libdnsModule,
		)
	}
}

func init() {
	// 21. Vultr
	// github.com/libdns/vultr v1.0.0 仍使用 libdns v0.2.x 旧版 API，不兼容。
	Register(&Provider{
		Name:    "vultr",
		Aliases: []string{},
		EnvVars: []string{"VULTR_API_KEY"},
		Build:   stubBuild("vultr", "github.com/libdns/vultr"),
	})

	// 22. Name.com (namedotcom)
	// github.com/libdns/namedotcom v0.3.3 仍使用 libdns v0.2.x 旧版 API，不兼容。
	Register(&Provider{
		Name:    "namedotcom",
		Aliases: []string{"namecom"},
		EnvVars: []string{"NAMEDOTCOM_TOKEN", "NAMEDOTCOM_USER"},
		Build:   stubBuild("namedotcom", "github.com/libdns/namedotcom"),
	})
}
