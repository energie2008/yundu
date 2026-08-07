// Package dnsproviders - Google Cloud DNS provider
//
// 使用 libdns/googleclouddns 实现 DNS-01 challenge。
// 环境变量：GCE_PROJECT, GOOGLE_APPLICATION_CREDENTIALS

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/googleclouddns"
)

func init() {
	Register(&Provider{
		Name:    "googlecloud",
		Aliases: []string{"gcp", "googledns"},
		EnvVars: []string{"GCE_PROJECT", "GOOGLE_APPLICATION_CREDENTIALS"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &googleclouddns.Provider{
				Project:            env["GCE_PROJECT"],
				ServiceAccountJSON: env["GOOGLE_APPLICATION_CREDENTIALS"],
			}, nil
		},
	})
}
