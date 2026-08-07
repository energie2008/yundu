// Package dnsproviders - Hetzner DNS provider
//
// 使用 libdns/hetzner 实现 DNS-01 challenge。
// 环境变量：HETZNER_API_KEY

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/hetzner"
)

func init() {
	Register(&Provider{
		Name:    "hetzner",
		Aliases: []string{},
		EnvVars: []string{"HETZNER_API_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			token := env["HETZNER_API_KEY"]
			if token == "" {
				token = env["HETZNER_DNS_TOKEN"] // 向后兼容
			}
			return &hetzner.Provider{
				AuthAPIToken: token,
			}, nil
		},
	})
}
