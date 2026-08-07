// Package dnsproviders - Tencent Cloud / DNSPod DNS provider
//
// 使用 libdns/tencentcloud 实现 DNS-01 challenge。
// 环境变量：TENCENTCLOUD_SECRET_ID, TENCENTCLOUD_SECRET_KEY

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/tencentcloud"
)

func init() {
	Register(&Provider{
		Name:    "tencentcloud",
		Aliases: []string{"dnspod", "tencent"},
		EnvVars: []string{"TENCENTCLOUD_SECRET_ID", "TENCENTCLOUD_SECRET_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &tencentcloud.Provider{
				SecretId:     env["TENCENTCLOUD_SECRET_ID"],
				SecretKey:    env["TENCENTCLOUD_SECRET_KEY"],
				SessionToken: env["TENCENTCLOUD_SESSION_TOKEN"],
				Region:       env["TENCENTCLOUD_REGION"],
			}, nil
		},
	})
}
