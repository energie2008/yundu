package service

import (
	"strings"

	"github.com/airport-panel/node-service/internal/model"
)

// config_json_normalizer.go 实现 P0-5：ConfigJSON 净化与老数据迁移。
//
// 设计原则（来自 yundu-final-implementation-playbook.md §8）：
//  1. 写入时归一化：前端保存时只写 IR 语义键（顶层规范字段）
//  2. 迁移器：将历史 config_json 中散落在 nested1/nested2 的字段拍平到顶层
//  3. 兼容读取：top-level > nested1 > nested2 的回退逻辑只存在于迁移器中，
//     不再留在编译器核心路径里（kernelrender 只读 NodeSpec IR）
//
// 规范化后的 config_json 只保留以下 IR 语义键（全部顶层）：
//   凭证类:   uuid, password, method, flow, alterId
//   TLS类:    fingerprint, alpn, allow_insecure, cert_pem, key_pem, cert_bundle_id, tls_mode
//   REALITY:  public_key, private_key, short_id, short_ids, server_name
//   XHTTP:    mode, xhttp_extra
//   传输类:   path, host, seed, quic_security, quic_key
//   CDN类:    cdn_address, cdn_port, cdn_path, cdn_up_path, cdn_down_path
//   服务类:   server_port, server_port_v6

// canonicalKeys 是规范化后 config_json 允许保留的顶层 IR 语义键白名单。
// 任何不在此白名单的顶层键将在归一化时被删除（除非属于已知扩展键）。
var canonicalKeys = map[string]bool{
	// 凭证
	"uuid": true, "password": true, "method": true, "flow": true, "alterId": true,
	// TLS
	"fingerprint": true, "alpn": true, "allow_insecure": true,
	"cert_pem": true, "key_pem": true, "cert_bundle_id": true, "tls_mode": true,
	// REALITY
	"public_key": true, "private_key": true, "short_id": true, "short_ids": true,
	"server_name": true, "reality_fingerprint": true,
	// XHTTP
	"mode": true, "xhttp_extra": true, "no_grpc_header": true,
	"xhttp": true, // 保留嵌套xhttp对象（含extra.downloadSettings/headers等完整结构）
	// 传输
	"path": true, "host": true, "seed": true,
	"quic_security": true, "quic_key": true,
	"service_name": true, // gRPC serviceName（buildTransportConfig 从 node.Path 读取，此处保留供前端回显和 fallback）
	"network": true,      // 传输类型（ws/grpc/xhttp 等），前端回显 fallback；主数据源为 node.transport_type DB 列
	// CDN
	"cdn_address": true, "cdn_port": true, "cdn_path": true,
	"cdn_up_path": true, "cdn_down_path": true, "cdn_host": true,
	// Tunnel（CDN/Tunnel 节点暴露模式，供 agent_handler.go CloudflaredTunnels API 过滤）
	"exposure_mode": true, "cloudflared_token": true, "cloudflared_tunnel_id": true,
	// 服务
	"server_port": true, "server_port_v6": true, "client_port": true,
	// Hysteria2 / TUIC
	"up_mbps": true, "down_mbps": true,
	// 端口跳跃（Hysteria2/TUIC UDP 协议，客户端 mport 参数和服务端 sing-box hop_ports 字段的数据源）
	"port_hopping": true,
	// 其它保留
	"utls_fingerprint": true, // 兼容期保留，P1 删除
	// === 修复：补全遗漏的白名单字段（解决编辑保存后退回原值）===
	"multiplex":              true, // mux/brutal 配置（enabled/protocol/max_connections/padding/brutal.up_mbps/brutal.down_mbps）
	"custom_outbounds":       true, // 自定义出站 JSON
	"custom_routes":          true, // 自定义路由 JSON
	"cert_file":              true, // TLS file 模式证书路径
	"key_file":               true, // TLS file 模式私钥路径
	"username":               true, // SOCKS/HTTP 协议用户名
	"region":                 true, // 区域标记
	"parent_node_id":         true, // 父节点关联
	"preset_id":              true, // 预设 ID
	"priority":               true, // 优先级（config_json 同步保留，回显 fallback）
	"spider_x":               true, // REALITY spider_x 防探测字段
	"reality_utls_enabled":   true, // REALITY uTLS 开关
	"reality_utls_fingerprint": true, // REALITY uTLS 指纹
	"reality_dest":           true, // REALITY dest（server_name:port）
	// P0-3 修复：允许 Raw JSON 编辑 security/tls 字段（保留在 config_json 供回显和 fallback）
	"security":               true, // 安全类型字符串（tls/reality/none），与 DB security_type 列同步
	"security_type":          true, // 同上（别名）
	"tls":                    true, // 0/1/2 数字标记（0=none, 1=tls, 2=reality），兼容历史数据
	// 链式套娃出站 URI（P-Chain）：节点入站流量通过此代理出站，支持 socks5/http/trojan/vless/vmess/ss
	// 保存时由 validateChainOutboundURI 三重校验（正则+长度+解析），渲染时由 ParseChainURI 消费
	"chain_outbound_uri":     true,
}

// NormalizeNodeConfigJSON 将 config_json 归一化为顶层 IR 语义键（P0-5 写入时归一化）。
// 在 CreateNode / UpdateNode 保存前调用，确保 DB 中只存规范化结构。
//
// 归一化规则：
//  1. 拍平 nested fallback：reality{} / reality_settings{} / tls_settings{} / xhttp{} → 顶层
//  2. 字段名统一为 snake_case（public_key 而非 publicKey）
//  3. 删除白名单外的顶层键（目标专有字段如 certificateFile/keyFile 由 TLSMaterialRef 管理）
//  4. 保留 nil 安全：输入 nil 返回空 map
func NormalizeNodeConfigJSON(cfg map[string]interface{}, protocol, transport, security string) map[string]interface{} {
	if cfg == nil {
		return make(map[string]interface{})
	}
	normalized := make(map[string]interface{})

	// Step 1: 拍平 nested fallback 结构，按优先级读取（顶层 > nested）
	// REALITY 字段：顶层 > reality{} > reality_settings{}
	copyFlatKey(normalized, cfg, "public_key", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "private_key", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "short_id", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "server_name", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "reality_fingerprint", "reality", "reality_settings")
	// 修复：spider_x / reality_utls_enabled / reality_utls_fingerprint / reality_dest 拍平
	copyFlatKey(normalized, cfg, "spider_x", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "reality_utls_enabled", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "reality_utls_fingerprint", "reality", "reality_settings")
	copyFlatKey(normalized, cfg, "reality_dest", "reality", "reality_settings")
	// 兼容 camelCase（历史 xboard 数据）
	if v := pickStringFromNested(cfg, "publicKey", "reality"); v != "" && normalized["public_key"] == "" {
		normalized["public_key"] = v
	}
	if v := pickStringFromNested(cfg, "shortId", "reality"); v != "" && normalized["short_id"] == "" {
		normalized["short_id"] = v
	}
	if v := pickStringFromNested(cfg, "serverName", "reality"); v != "" && normalized["server_name"] == "" {
		normalized["server_name"] = v
	}
	// short_ids 数组
	if sids, ok := extractStringSlice(cfg, "short_ids", "reality", "reality_settings"); ok && len(sids) > 0 {
		normalized["short_ids"] = sids
	}

	// TLS 字段：顶层 > tls_settings{} > tls{}
	copyFlatKey(normalized, cfg, "fingerprint", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "allow_insecure", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "cert_pem", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "key_pem", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "cert_bundle_id", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "tls_mode", "tls_settings", "tls")
	// 修复：cert_file / key_file 拍平（TLS file 模式证书路径）
	copyFlatKey(normalized, cfg, "cert_file", "tls_settings", "tls")
	copyFlatKey(normalized, cfg, "key_file", "tls_settings", "tls")
	// 兼容 camelCase
	if v := pickStringFromNested(cfg, "serverName", "tls_settings", "tls"); v != "" {
		normalized["server_name"] = v
	}
	if v := pickStringFromNested(cfg, "allowInsecure", "tls_settings"); v != "" && normalized["allow_insecure"] == "" {
		normalized["allow_insecure"] = v
	}
	// alpn 数组
	if alpn, ok := extractStringSlice(cfg, "alpn", "tls_settings", "tls"); ok && len(alpn) > 0 {
		normalized["alpn"] = alpn
	}
	// utls_fingerprint 兼容
	if v, ok := cfg["utls_fingerprint"].(string); ok && v != "" {
		normalized["utls_fingerprint"] = v
	}

	// XHTTP 字段：顶层 > xhttp{}
	copyFlatKey(normalized, cfg, "mode", "xhttp")
	copyFlatKey(normalized, cfg, "no_grpc_header", "xhttp")
	if xhttpExtra, ok := cfg["xhttp"].(map[string]interface{}); ok {
		if extra, ok := xhttpExtra["extra"].(map[string]interface{}); ok && len(extra) > 0 {
			normalized["xhttp_extra"] = extra
		}
	}

	// Step 2: 直接拷贝白名单内的顶层键
	for key, val := range cfg {
		if !canonicalKeys[key] {
			continue
		}
		// 已在 Step 1 通过 fallback 填充的键不覆盖（顶层优先）
		if _, exists := normalized[key]; exists {
			// 顶层值优先：如果原顶层有值，用顶层值覆盖 fallback
			if strVal, ok := val.(string); ok && strVal != "" {
				normalized[key] = strVal
			} else if val != nil {
				normalized[key] = val
			}
		} else {
			normalized[key] = val
		}
	}

	// Step 3: 协议级语义清理
	normalizeForProtocol(normalized, strings.ToLower(protocol))

	// Step 4: REALITY 短 id 归一化（short_id + short_ids 合并）
	if sids, ok := normalized["short_ids"].([]string); ok && len(sids) > 0 {
		// 保留 short_ids 数组形式（推荐），同时填充 short_id 兼容单值读取
		if _, hasSingle := normalized["short_id"]; !hasSingle {
			normalized["short_id"] = sids[0]
		}
	}

	return normalized
}

// MigrateNodeConfigJSON 迁移历史节点的 config_json 到规范化结构（P0-5 一次性迁移）。
// 与 NormalizeNodeConfigJSON 的区别：本函数用于已存在的节点批量迁移，
// 会额外处理已废弃的字段名（如 publicKey→public_key）和已删除的目标专有字段。
func MigrateNodeConfigJSON(node *model.Node) map[string]interface{} {
	protocol := strings.ToLower(node.ProtocolType)
	transport := strings.ToLower(node.TransportType)
	security := getSecurityType(node)
	normalized := NormalizeNodeConfigJSON(node.ConfigJSON, protocol, transport, security)

	// 迁移期额外清理：删除已废弃的目标专有字段
	delete(normalized, "certificateFile")
	delete(normalized, "keyFile")
	delete(normalized, "certificate_file")
	delete(normalized, "key_file")

	// 迁移期：downloadSettings REALITY 校验（项目记忆 P06/P07 坑点）
	// 历史 config_json 可能残留 downloadSettings，迁移时清理（由 kernelrender 独立处理）
	delete(normalized, "downloadSettings")
	delete(normalized, "download_settings")

	return normalized
}

// normalizeForProtocol 根据协议清理不相关字段
func normalizeForProtocol(cfg map[string]interface{}, protocol string) {
	switch protocol {
	case "vless", "vmess":
		// UUID 协议不需要 password/method
		delete(cfg, "password")
		delete(cfg, "method")
	case "trojan", "shadowsocks", "ss", "hysteria2", "anytls":
		// password 协议不需要 uuid/alterId（hysteria2 用 password）
		delete(cfg, "uuid")
		delete(cfg, "alterId")
		if protocol == "trojan" || protocol == "hysteria2" || protocol == "anytls" {
			delete(cfg, "method")
		}
	}
}

// copyFlatKey 从 config_json 按"顶层 > nestedKeys..."顺序读取字符串字段，写入 normalized。
// 顶层优先；顶层无值时按 nestedKeys 顺序回退读取。
func copyFlatKey(normalized, cfg map[string]interface{}, key string, nestedKeys ...string) {
	if v, ok := cfg[key].(string); ok && v != "" {
		normalized[key] = v
		return
	}
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if v, ok := m[key].(string); ok && v != "" {
				normalized[key] = v
				return
			}
		}
	}
}

// pickStringFromNested 从指定 nested 对象中读取字符串字段（camelCase 兼容用）
func pickStringFromNested(cfg map[string]interface{}, key string, nestedKeys ...string) string {
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if v, ok := m[key].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// extractStringSlice 从 config_json 按"顶层 > nestedKeys..."顺序读取字符串切片
func extractStringSlice(cfg map[string]interface{}, key string, nestedKeys ...string) ([]string, bool) {
	if v, ok := cfg[key]; ok {
		if s := anyToStringSlice(v); len(s) > 0 {
			return s, true
		}
	}
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if v, ok := m[key]; ok {
				if s := anyToStringSlice(v); len(s) > 0 {
					return s, true
				}
			}
		}
	}
	return nil, false
}

// anyToStringSlice 将 []interface{} 转为 []string
func anyToStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}
