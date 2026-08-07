// Package dnsproviders - Scaleway DNS provider
//
// 使用 libdns/scaleway v0.3.1 实现 DNS-01 challenge。
// 环境变量：SCW_SECRET_KEY, SCW_ORGANIZATION_ID
// Scaleway 的 Access Key（SCW_ACCESS_KEY）通常以组织 ID 为前缀，
// 若未显式提供 SCW_ORGANIZATION_ID，则回退使用 SCW_ACCESS_KEY。

package dnsproviders

import (
	"github.com/caddyserver/certmagic"
	"github.com/libdns/scaleway"
)

func init() {
	Register(&Provider{
		Name:    "scaleway",
		Aliases: []string{"scw"},
		EnvVars: []string{"SCW_SECRET_KEY", "SCW_ORGANIZATION_ID", "SCW_ACCESS_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			orgID := env["SCW_ORGANIZATION_ID"]
			if orgID == "" {
				orgID = env["SCW_ACCESS_KEY"] // 回退兼容
			}
			// Provider 嵌入 Client（有未导出字段），零值构造后方法内部自动初始化
			return &scaleway.Provider{
				SecretKey:      env["SCW_SECRET_KEY"],
				OrganizationID: orgID,
			}, nil
		},
	})
}
