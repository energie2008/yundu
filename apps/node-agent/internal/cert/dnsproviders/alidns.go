// Package dnsproviders - Aliyun (Alidns) DNS provider
//
// 使用 libdns/alidns 实现 DNS-01 challenge。
// 环境变量：ALICLOUD_ACCESS_KEY, ALICLOUD_SECRET_KEY, ALICLOUD_REGION_ID（可选）

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/alidns"
)

func init() {
	Register(&Provider{
		Name:    "alidns",
		Aliases: []string{"aliyun", "alicloud"},
		EnvVars: []string{"ALICLOUD_ACCESS_KEY", "ALICLOUD_SECRET_KEY", "ALICLOUD_REGION_ID"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			accessKey := env["ALICLOUD_ACCESS_KEY"]
			if accessKey == "" {
				accessKey = env["ALICLOUD_ACCESS_KEY_ID"]
			}
			secretKey := env["ALICLOUD_SECRET_KEY"]
			if secretKey == "" {
				secretKey = env["ALICLOUD_ACCESS_KEY_SECRET"]
			}
			return &alidns.Provider{
				CredentialInfo: alidns.CredentialInfo{
					AccessKeyID:     accessKey,
					AccessKeySecret: secretKey,
					RegionID:        env["ALICLOUD_REGION_ID"],
					SecurityToken:   env["ALICLOUD_SECURITY_TOKEN"],
				},
			}, nil
		},
	})
}
