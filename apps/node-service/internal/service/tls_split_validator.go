package service

import (
	"fmt"

	"github.com/airport-panel/node-service/internal/model"
)

// validateTLSSplitFields P1-6: 校验 TLS 分离节点（仅 argo_tunnel/CF 隧道）的客户端/服务端字段分离。
//
// TLS 分离架构（DB 字段层面）仅适用于 argo_tunnel 节点：
//   - 服务端（DB 列）：SecurityType = "none"（cloudflared 明文 HTTP 回源，xray 无 TLS）
//   - 客户端（config_json）：security_type = "tls"（客户端到 CF Edge 必须 TLS）
//   - 客户端双写：config_json.security = "tls", config_json.tls = 1
//
// cdn/cdn_saas 节点不做 DB 字段分离（DB=config_json=tls），
// xray inbound 的 TLS 剥离由渲染层 shouldStripTLSForNginxVhost 动态完成，
// 与 argo_tunnel 的剥离触发条件完全独立，避免耦合。
//
// 此函数在节点保存时做后置校验（standardizeNodeFields 已先强制设置正确值），
// 防止字段分离不一致导致：
//   - 客户端无 TLS（security_type=none）→ CF 边缘到客户端明文，安全问题
//   - 服务端有 TLS（SecurityType=tls）→ cloudflared HTTP 回源失败（xray 等待 TLS 握手）
func validateTLSSplitFields(node *model.Node) error {
	if node == nil {
		return nil
	}

	// 仅 TLS 分离节点（argo_tunnel）需要校验，cdn/cdn_saas/direct/reality 跳过
	if !isTLSSplitExposureNode(node) {
		return nil
	}

	if node.ConfigJSON == nil {
		em := determineExposureMode(node)
		return fmt.Errorf("%s 节点 config_json 不能为空", em)
	}

	var errs []string

	// 1. 服务端 DB SecurityType 必须为 "none"
	if node.SecurityType == nil {
		errs = append(errs, "DB security_type 为 nil，应为 none（服务端剥离）")
	} else if *node.SecurityType != "none" {
		errs = append(errs, fmt.Sprintf("DB security_type=%s，应为 none（服务端剥离）", *node.SecurityType))
	}

	// 2. 客户端 config_json.security_type 必须为 "tls"
	cjSecType, _ := node.ConfigJSON["security_type"].(string)
	if cjSecType != "tls" {
		errs = append(errs, fmt.Sprintf("config_json.security_type=%q，应为 tls（客户端）", cjSecType))
	}

	// 3. 客户端 config_json.security 必须为 "tls"（双写一致性）
	cjSecurity, _ := node.ConfigJSON["security"].(string)
	if cjSecurity != "tls" {
		errs = append(errs, fmt.Sprintf("config_json.security=%q，应为 tls（客户端双写）", cjSecurity))
	}

	// 4. 客户端 config_json.tls 必须为 1
	if tlsVal, ok := node.ConfigJSON["tls"]; ok {
		tlsInt := 0
		switch v := tlsVal.(type) {
		case float64:
			tlsInt = int(v)
		case int:
			tlsInt = v
		case int64:
			tlsInt = int(v)
		case bool:
			if v {
				tlsInt = 1
			}
		}
		if tlsInt != 1 {
			errs = append(errs, fmt.Sprintf("config_json.tls=%v，应为 1（客户端）", tlsVal))
		}
	} else {
		errs = append(errs, "config_json.tls 字段缺失，应为 1（客户端）")
	}

	if len(errs) > 0 {
		return fmt.Errorf("TLS 分离字段校验失败: %v", errs)
	}
	return nil
}

// validateNonSplitTLSConsistency P1-6: 校验非 TLS 分离节点（direct/reality/cdn/cdn_saas）的 TLS 一致性。
// 非分离节点不需要 DB 字段分离：
//   - 服务端 SecurityType 应与 config_json.security_type 一致（均为 tls/reality/none）
//   - 不应出现 SecurityType=none 但 config_json.security_type=tls 的分离状态
//   - TLS 分离状态（DB=none + config_json=tls）仅允许 argo_tunnel 节点
//
// 注意：cdn/cdn_saas 节点虽然在渲染层会被 shouldStripTLSForNginxVhost 剥离 xray inbound TLS，
// 但 DB 字段层面仍然是"tls/tls 一致"状态，由渲染层动态决定是否剥离，不走 DB 字段分离。
func validateNonSplitTLSConsistency(node *model.Node) error {
	if node == nil || node.ConfigJSON == nil {
		return nil
	}

	// TLS 分离节点（仅 argo_tunnel）由 validateTLSSplitFields 校验，此处跳过
	if isTLSSplitExposureNode(node) {
		return nil
	}

	dbSec := ""
	if node.SecurityType != nil {
		dbSec = *node.SecurityType
	}
	cjSec, _ := node.ConfigJSON["security_type"].(string)

	// 非分离节点不应出现分离状态
	// SecurityType=none 但 config_json.security_type=tls 是分离状态，仅 argo_tunnel 允许
	if dbSec == "none" && cjSec == "tls" {
		return fmt.Errorf("非 argo_tunnel 节点不应出现 TLS 分离（DB=none 但 config_json=tls），" +
			"仅 argo_tunnel（CF 隧道）节点允许 DB 字段分离")
	}

	return nil
}
