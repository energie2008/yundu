// Package dnsproviders - AWS Route53 DNS provider
//
// 使用 libdns/route53 实现 DNS-01 challenge。
// 环境变量：AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/route53"
)

func init() {
	Register(&Provider{
		Name:    "route53",
		Aliases: []string{"aws"},
		EnvVars: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &route53.Provider{
				AccessKeyId:     env["AWS_ACCESS_KEY_ID"],
				SecretAccessKey: env["AWS_SECRET_ACCESS_KEY"],
				SessionToken:    env["AWS_SESSION_TOKEN"],
				Region:          env["AWS_REGION"],
				Profile:         env["AWS_PROFILE"],
			}, nil
		},
	})
}
