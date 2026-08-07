// Package dnsproviders - PowerDNS DNS provider
//
// 使用 libdns/powerdns v0.1.4 实现 DNS-01 challenge。
// 适用于自建 PowerDNS 服务器（通过 HTTP API 管理）。
// 环境变量：PDNS_API_URL, PDNS_API_KEY, PDNS_SERVER_ID（可选，默认 localhost）

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/powerdns"
)

func init() {
	Register(&Provider{
		Name:    "powerdns",
		Aliases: []string{"pdns"},
		EnvVars: []string{"PDNS_API_URL", "PDNS_API_KEY", "PDNS_SERVER_ID"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &powerdns.Provider{
				ServerURL: env["PDNS_API_URL"],
				APIToken:  env["PDNS_API_KEY"],
				ServerID:  env["PDNS_SERVER_ID"], // 留空则使用库默认 "localhost"
			}, nil
		},
	})
}
