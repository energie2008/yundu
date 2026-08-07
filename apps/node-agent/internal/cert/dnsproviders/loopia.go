// Package dnsproviders - Loopia DNS provider
//
// 使用 libdns/loopia v1.0.1 实现 DNS-01 challenge。
// 环境变量：LOOPIA_API_USER, LOOPIA_API_PASSWORD, LOOPIA_CUSTOMER_NUMBER（可选）

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/loopia"
)

func init() {
	Register(&Provider{
		Name:    "loopia",
		Aliases: []string{},
		EnvVars: []string{"LOOPIA_API_USER", "LOOPIA_API_PASSWORD", "LOOPIA_CUSTOMER_NUMBER"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &loopia.Provider{
				Username: env["LOOPIA_API_USER"],
				Password: env["LOOPIA_API_PASSWORD"],
				Customer: env["LOOPIA_CUSTOMER_NUMBER"],
			}, nil
		},
	})
}
