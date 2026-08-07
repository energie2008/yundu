// Package dnsproviders - Namecheap DNS provider
//
// 使用 libdns/namecheap 实现 DNS-01 challenge。
// 环境变量：NAMECHEAP_API_USER, NAMECHEAP_API_KEY

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/namecheap"
)

func init() {
	Register(&Provider{
		Name:    "namecheap",
		Aliases: []string{},
		EnvVars: []string{"NAMECHEAP_API_USER", "NAMECHEAP_API_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &namecheap.Provider{
				APIKey:      env["NAMECHEAP_API_KEY"],
				User:        env["NAMECHEAP_API_USER"],
				APIEndpoint: env["NAMECHEAP_API_ENDPOINT"],
				ClientIP:    env["NAMECHEAP_CLIENT_IP"],
			}, nil
		},
	})
}
