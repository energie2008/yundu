package service

import "github.com/airport-panel/subscription/kernelrender"

// DefaultAuditConfig 返回默认审计配置（BT + SSRF 防护全部启用）。
// 审计路由规则（S8）用于阻断 BitTorrent 协议流量和 SSRF 攻击，
// 是节点安全审计的基本要求。
func DefaultAuditConfig() kernelrender.AuditConfig {
	return kernelrender.DefaultAuditConfig()
}
