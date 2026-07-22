// Package nginx 提供 node-agent 的 nginx 配置同步能力。
//
// detect.go 实现 nginx 环境自动探测，免配置适配宝塔/标准 nginx/无 nginx 环境。
// 探测结果用于 NginxReconciler 选择正确的 SyncConfig 路径。
package nginx

import (
	"os"
	"os/exec"
)

// EnvNone 表示未检测到 nginx，reconciler 应跳过同步
const EnvNone = "none"

// EnvBtPanel 表示宝塔面板环境（/www/server/panel/vhost/nginx/）
const EnvBtPanel = "bt"

// EnvStandard 表示标准 nginx 环境（/etc/nginx/）
const EnvStandard = "standard"

// DetectEnv 自动探测当前 VPS 的 nginx 环境类型。
//
// 探测顺序：
//  1. 检查 nginx 二进制是否在 PATH 中（exec.LookPath）
//  2. 检查宝塔面板特征路径 /www/server/panel/vhost/nginx/ 是否存在
//  3. 检查标准 nginx 特征路径 /etc/nginx/ 是否存在
//  4. 都不匹配则返回 EnvNone（reconciler 将跳过同步，不报错）
//
// 返回值：EnvBtPanel / EnvStandard / EnvNone
func DetectEnv() string {
	// 1. 检查 nginx 二进制是否存在
	if _, err := exec.LookPath("nginx"); err != nil {
		// nginx 不在 PATH 中，可能是未安装或非 nginx 节点（如纯直连节点）
		return EnvNone
	}

	// 2. 优先检测宝塔环境（特征路径明确）
	if info, err := os.Stat("/www/server/panel/vhost/nginx"); err == nil && info.IsDir() {
		return EnvBtPanel
	}

	// 3. 检测标准 nginx 环境
	if info, err := os.Stat("/etc/nginx"); err == nil && info.IsDir() {
		return EnvStandard
	}

	// 4. nginx 存在但路径不匹配已知模式，回退到标准环境
	// （让用户通过 NODE_AGENT_NGINX_ENV 环境变量覆盖）
	return EnvStandard
}

// ResolveEnv 解析 nginx 环境类型，优先使用环境变量覆盖，否则自动探测。
//
// 环境变量 NODE_AGENT_NGINX_ENV 可选值：bt / standard / none
// 未设置时调用 DetectEnv() 自动探测。
func ResolveEnv(explicit string) string {
	switch explicit {
	case EnvBtPanel, EnvStandard, EnvNone:
		return explicit
	case "":
		return DetectEnv()
	default:
		// 未知值，回退到自动探测
		return DetectEnv()
	}
}
