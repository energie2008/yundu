// Package dnsproviders - Linode DNS provider
//
// 使用 libdns/linode 实现 DNS-01 challenge。
// 环境变量：LINODE_TOKEN

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/linode"
)

func init() {
	Register(&Provider{
		Name:    "linode",
		Aliases: []string{},
		EnvVars: []string{"LINODE_TOKEN"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			token := env["LINODE_TOKEN"]
			if token == "" {
				token = env["LINODE_API_TOKEN"] // 向后兼容
			}
			return &linode.Provider{
				APIToken: token,
			}, nil
		},
	})
}
