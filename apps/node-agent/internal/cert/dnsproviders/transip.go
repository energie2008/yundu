// Package dnsproviders - TransIP DNS provider
//
// 使用 libdns/transip v1.1.2 实现 DNS-01 challenge。
// 环境变量：TRANSIP_ACCOUNT_NAME, TRANSIP_PRIVATE_KEY_PATH
// PrivateKey 可接受 key 内容字符串或文件路径（libdns/transip 内部自动判断），
// 此处优先使用 TRANSIP_PRIVATE_KEY（内容），回退到读取 TRANSIP_PRIVATE_KEY_PATH 文件。

package dnsproviders

import (
	"fmt"
	"os"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/transip"
)

func init() {
	Register(&Provider{
		Name:    "transip",
		Aliases: []string{},
		EnvVars: []string{"TRANSIP_ACCOUNT_NAME", "TRANSIP_PRIVATE_KEY_PATH", "TRANSIP_PRIVATE_KEY"},
		Build: func(env map[string]string) (certmagic.DNSProvider, error) {
			key := env["TRANSIP_PRIVATE_KEY"]
			if key == "" {
				// 回退：从文件路径读取私钥内容
				keyPath := env["TRANSIP_PRIVATE_KEY_PATH"]
				if keyPath != "" {
					data, err := os.ReadFile(keyPath)
					if err != nil {
						return nil, fmt.Errorf("读取 TransIP 私钥文件失败 %q: %w", keyPath, err)
					}
					key = string(data)
				}
			}
			return &transip.Provider{
				AuthLogin:  env["TRANSIP_ACCOUNT_NAME"],
				PrivateKey: key,
			}, nil
		},
	})
}
