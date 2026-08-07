// Package dnsproviders - OVH DNS provider
//
// 使用 libdns/ovh v1.1.0 实现 DNS-01 challenge。
// 环境变量：OVH_ENDPOINT, OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET, OVH_CONSUMER_KEY
// OVH_ENDPOINT 示例: ovh-eu / ovh-ca / ovh-us

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/ovh"
)

func init() {
	Register(&Provider{
		Name:    "ovh",
		Aliases: []string{},
		EnvVars: []string{"OVH_ENDPOINT", "OVH_APPLICATION_KEY", "OVH_APPLICATION_SECRET", "OVH_CONSUMER_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &ovh.Provider{
				Endpoint:          env["OVH_ENDPOINT"],
				ApplicationKey:    env["OVH_APPLICATION_KEY"],
				ApplicationSecret: env["OVH_APPLICATION_SECRET"],
				ConsumerKey:       env["OVH_CONSUMER_KEY"],
			}, nil
		},
	})
}
