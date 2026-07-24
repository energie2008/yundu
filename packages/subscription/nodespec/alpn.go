package nodespec

import "strings"

// DeriveALPN 是项目唯一的 ALPN 真相源。
// 根据协议和传输层类型推导 TLS ALPN,消除散落在各处的硬编码规则。
//
// 规则:
//   - Hysteria2/TUIC(QUIC 系)                    → ["h3"]
//   - WS/HTTPUpgrade(HTTP/1.1 Upgrade)           → ["http/1.1"]
//   - 其他(TCP/gRPC/XHTTP/REALITY/AnyTLS/Mieru)   → ["h2","http/1.1"]
//
// 设计原因:
//   - h3: QUIC 协议原生基于 HTTP/3,ALPN 必须为 h3
//   - http/1.1: CF CDN 协商 HTTP/2 后 WS Upgrade 失败(返回 400),
//     WS/HTTPUpgrade 必须 HTTP/1.1
//   - h2,http/1.1: 默认 TLS 双 ALPN,允许客户端协商 h2(多路复用)或 http/1.1(兼容)
//
// 调用方:
//   - node_service.go 保存节点时推导 ALPN 写入 DB
//   - xray_config.go / singbox_config.go 渲染内核配置时推导 ALPN
//   - adapter.go 渲染订阅 URI 时推导 ALPN
func DeriveALPN(protocol, transportType string) []string {
	p := strings.ToLower(strings.TrimSpace(protocol))
	t := strings.ToLower(strings.TrimSpace(transportType))
	switch {
	case p == string(ProtocolHysteria2) || p == string(ProtocolTUIC):
		return []string{"h3"}
	case t == string(TransportWS) || t == string(TransportHTTPUpgrade):
		return []string{"http/1.1"}
	default:
		return []string{"h2", "http/1.1"}
	}
}
