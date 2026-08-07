// Package dnsproviders - Azure DNS provider
//
// 使用 libdns/azure v0.5.0 实现 DNS-01 challenge。
// 环境变量：AZURE_SUBSCRIPTION_ID, AZURE_RESOURCE_GROUP, AZURE_TENANT_ID,
//          AZURE_CLIENT_ID, AZURE_CLIENT_SECRET
// TenantId/ClientId/ClientSecret 用于 Service Principal 认证；
// 若使用 Managed Identity，留空这三个字段即可（由 Azure 运行时注入）。

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/azure"
)

func init() {
	Register(&Provider{
		Name:    "azure",
		Aliases: []string{"azuredns"},
		EnvVars: []string{"AZURE_SUBSCRIPTION_ID", "AZURE_RESOURCE_GROUP", "AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			return &azure.Provider{
				SubscriptionId:    env["AZURE_SUBSCRIPTION_ID"],
				ResourceGroupName: env["AZURE_RESOURCE_GROUP"],
				TenantId:          env["AZURE_TENANT_ID"],
				ClientId:          env["AZURE_CLIENT_ID"],
				ClientSecret:      env["AZURE_CLIENT_SECRET"],
			}, nil
		},
	})
}
