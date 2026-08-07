// Package dnsproviders - RFC 2136 dynamic DNS provider
//
// 使用 libdns/rfc2136 v1.0.1 实现 DNS-01 challenge。
// 适用于自建 BIND/PowerDNS 等支持 RFC 2136 动态更新的 DNS 服务器。
// 环境变量：RFC2136_NAMESERVER, RFC2136_TSIG_KEY, RFC2136_TSIG_ALGORITHM, RFC2136_TSIG_SECRET

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/rfc2136"
)

func init() {
	Register(&Provider{
		Name:    "rfc2136",
		Aliases: []string{"bind"},
		EnvVars: []string{"RFC2136_NAMESERVER", "RFC2136_TSIG_KEY", "RFC2136_TSIG_ALGORITHM", "RFC2136_TSIG_SECRET"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &rfc2136.Provider{
				KeyName: env["RFC2136_TSIG_KEY"],
				KeyAlg:  env["RFC2136_TSIG_ALGORITHM"],
				Key:     env["RFC2136_TSIG_SECRET"],
				Server:  env["RFC2136_NAMESERVER"],
			}, nil
		},
	})
}
