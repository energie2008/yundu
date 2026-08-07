// Package dnsproviders - Namesilo DNS provider
//
// 使用 libdns/namesilo v1.0.0 实现 DNS-01 challenge。
// 环境变量：NAMESILO_API_KEY

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/namesilo"
)

func init() {
	Register(&Provider{
		Name:    "namesilo",
		Aliases: []string{},
		EnvVars: []string{"NAMESILO_API_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &namesilo.Provider{
				APIToken: env["NAMESILO_API_KEY"],
			}, nil
		},
	})
}
