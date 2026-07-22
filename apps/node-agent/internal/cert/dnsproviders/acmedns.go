// Package dnsproviders - ACME-DNS DNS provider
//
// 使用 libdns/acmedns 实现 DNS-01 challenge。
// ACME-DNS 是一个独立的 DNS 服务器，专门用于 ACME DNS-01 验证。
// 环境变量：ACMEDNS_SERVER_URL, ACMEDNS_USERNAME, ACMEDNS_PASSWORD, ACMEDNS_SUBDOMAIN

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/acmedns"
)

func init() {
	Register(&Provider{
		Name:    "acmedns",
		Aliases: []string{"acme-dns"},
		EnvVars: []string{"ACMEDNS_SERVER_URL", "ACMEDNS_USERNAME", "ACMEDNS_PASSWORD", "ACMEDNS_SUBDOMAIN"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &acmedns.Provider{
				Username:  env["ACMEDNS_USERNAME"],
				Password:  env["ACMEDNS_PASSWORD"],
				Subdomain: env["ACMEDNS_SUBDOMAIN"],
				ServerURL: env["ACMEDNS_SERVER_URL"],
			}, nil
		},
	})
}
