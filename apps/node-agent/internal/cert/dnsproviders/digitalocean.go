// Package dnsproviders - DigitalOcean DNS provider
//
// 使用 libdns/digitalocean 实现 DNS-01 challenge。
// 环境变量：DO_AUTH_TOKEN

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/digitalocean"
)

func init() {
	Register(&Provider{
		Name:    "digitalocean",
		Aliases: []string{"do"},
		EnvVars: []string{"DO_AUTH_TOKEN"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			token := env["DO_AUTH_TOKEN"]
			if token == "" {
				token = env["DIGITALOCEAN_TOKEN"] // 向后兼容
			}
			return &digitalocean.Provider{
				APIToken: token,
			}, nil
		},
	})
}
