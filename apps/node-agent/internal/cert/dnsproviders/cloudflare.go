// Package dnsproviders - Cloudflare DNS provider
//
// 移植自 Xboard-Node internal/cert/dnsproviders/cloudflare.go。
// 使用 libdns/cloudflare 实现 DNS-01 challenge。

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
)

func init() {
	Register(&Provider{
		Name:    "cloudflare",
		Aliases: []string{"cf"},
		EnvVars: []string{"CLOUDFLARE_DNS_API_TOKEN"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			token := env["CLOUDFLARE_DNS_API_TOKEN"]
			if token == "" {
				token = env["CF_Token"] // 向后兼容 YunDu 原有环境变量
			}
			return &cloudflare.Provider{
				APIToken: token,
			}, nil
		},
	})
}
