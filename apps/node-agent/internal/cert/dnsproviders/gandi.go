// Package dnsproviders - Gandi DNS provider
//
// 使用 libdns/gandi 实现 DNS-01 challenge。
// 环境变量：GANDI_API_KEY

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/gandi"
)

func init() {
	Register(&Provider{
		Name:    "gandi",
		Aliases: []string{},
		EnvVars: []string{"GANDI_API_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			token := env["GANDI_API_KEY"]
			if token == "" {
				token = env["GANDI_PERSONAL_ACCESS_TOKEN"] // 向后兼容
			}
			return &gandi.Provider{
				BearerToken: token,
			}, nil
		},
	})
}
