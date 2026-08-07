// Package dnsproviders - Netcup DNS provider
//
// 使用 libdns/netcup v1.0.0 实现 DNS-01 challenge。
// 环境变量：NETCUP_CUSTOMER_NUMBER, NETCUP_API_KEY, NETCUP_API_PASSWORD

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/netcup"
)

func init() {
	Register(&Provider{
		Name:    "netcup",
		Aliases: []string{},
		EnvVars: []string{"NETCUP_CUSTOMER_NUMBER", "NETCUP_API_KEY", "NETCUP_API_PASSWORD"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &netcup.Provider{
				CustomerNumber: env["NETCUP_CUSTOMER_NUMBER"],
				APIKey:         env["NETCUP_API_KEY"],
				APIPassword:    env["NETCUP_API_PASSWORD"],
			}, nil
		},
	})
}
