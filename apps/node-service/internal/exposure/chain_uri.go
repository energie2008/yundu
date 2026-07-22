package exposure

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/airport-panel/node-service/internal/importer"
	"github.com/airport-panel/subscription/nodespec"
)

// ParseChainURI 把套娃出站 URI 解析为 NodeSpec，供 chain 渲染器消费。
// 支持 socks5://、http(s)://、trojan://、vless://、vmess://、ss://、hysteria2://、tuic://
// 返回 (nil, nil) 表示空 URI（不启用套娃）。
func ParseChainURI(uri string) (*nodespec.NodeSpec, error) {
	if uri == "" {
		return nil, nil
	}
	preview, err := importer.ParseURI(uri)
	if err != nil {
		return nil, fmt.Errorf("parse chain uri: %w", err)
	}
	if preview == nil {
		return nil, fmt.Errorf("parse chain uri: empty preview")
	}
	return uriPreviewToNodeSpec(preview, uri)
}

// uriPreviewToNodeSpec 按协议分发到独立转换函数（职责单一，可独立单测）。
// 每个 xxxPreviewToSpec 只处理一种协议的字段映射，互不影响。
func uriPreviewToNodeSpec(p *importer.URINodePreview, sourceURI string) (*nodespec.NodeSpec, error) {
	protocol := normalizeChainProtocol(p.ProtocolType)

	var converter func(*importer.URINodePreview) (*nodespec.NodeSpec, error)
	switch protocol {
	case "socks":
		converter = socksPreviewToSpec
	case "http":
		converter = httpPreviewToSpec
	case "trojan":
		converter = trojanPreviewToSpec
	case "vless":
		converter = vlessPreviewToSpec
	case "vmess":
		converter = vmessPreviewToSpec
	case "ss":
		converter = ssPreviewToSpec
	case "hysteria2":
		converter = hysteria2PreviewToSpec
	case "tuic":
		converter = tuicPreviewToSpec
	default:
		return nil, fmt.Errorf("unsupported chain protocol: %s (uri=%s)", protocol, redactURI(sourceURI))
	}

	spec, err := converter(p)
	if err != nil {
		return nil, fmt.Errorf("convert %s chain node failed: %w", protocol, err)
	}

	// 通用字段填充
	spec.Address = p.Host
	spec.Port = p.Port
	if spec.Transport.Type == "" {
		spec.Transport.Type = nodespec.Transport(p.TransportType)
	}
	if spec.Transport.Type == "" {
		spec.Transport.Type = nodespec.TransportTCP
	}

	return spec, nil
}

// socksPreviewToSpec 把 socks5 URI 预览转为 NodeSpec。
func socksPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	username := strOrEmpty(p.ConfigJSON["username"])
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolSOCKS5,
		Credentials: nodespec.SOCKS5Credentials{Username: username, Password: password},
	}
	applyTLSFromPreview(spec, p)
	return spec, nil
}

// httpPreviewToSpec 把 http/https URI 预览转为 NodeSpec。
func httpPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	username := strOrEmpty(p.ConfigJSON["username"])
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolHTTP,
		Credentials: nodespec.HTTPCredentials{Username: username, Password: password},
	}
	applyTLSFromPreview(spec, p)
	return spec, nil
}

// trojanPreviewToSpec 把 trojan URI 预览转为 NodeSpec。
func trojanPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolTrojan,
		Credentials: nodespec.TrojanCredentials{Password: password},
	}
	applyTLSFromPreview(spec, p)
	applyTransportFromPreview(spec, p)
	return spec, nil
}

// vlessPreviewToSpec 把 vless URI 预览转为 NodeSpec。
func vlessPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	uuid := p.UUID
	if uuid == "" {
		uuid = strOrEmpty(p.ConfigJSON["uuid"])
	}
	flow := strOrEmpty(p.ConfigJSON["flow"])
	creds := nodespec.VLESSCredentials{
		UUID:       uuid,
		Encryption: "none",
	}
	if flow != "" {
		creds.Flow = nodespec.FlowControl(flow)
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolVLESS,
		Credentials: creds,
	}
	applyTLSFromPreview(spec, p)
	applyTransportFromPreview(spec, p)
	return spec, nil
}

// vmessPreviewToSpec 把 vmess URI 预览转为 NodeSpec。
func vmessPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	uuid := p.UUID
	if uuid == "" {
		uuid = strOrEmpty(p.ConfigJSON["uuid"])
	}
	alterID := 0
	if v, ok := p.ConfigJSON["alter_id"]; ok {
		alterID = anyToInt(v)
	}
	security := strOrEmpty(p.ConfigJSON["security"])
	if security == "" {
		security = "auto"
	}
	spec := &nodespec.NodeSpec{
		Protocol: nodespec.ProtocolVMess,
		Credentials: nodespec.VMessCredentials{
			UUID:     uuid,
			AlterID:  alterID,
			Security: security,
		},
	}
	applyTLSFromPreview(spec, p)
	applyTransportFromPreview(spec, p)
	return spec, nil
}

// ssPreviewToSpec 把 ss URI 预览转为 NodeSpec。
func ssPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	method := strOrEmpty(p.ConfigJSON["method"])
	spec := &nodespec.NodeSpec{
		Protocol: nodespec.ProtocolShadowsocks,
		Credentials: nodespec.ShadowsocksCredentials{
			Password: password,
			Method:   method,
		},
	}
	applyTLSFromPreview(spec, p)
	return spec, nil
}

// hysteria2PreviewToSpec 把 hysteria2 URI 预览转为 NodeSpec。
func hysteria2PreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolHysteria2,
		Credentials: nodespec.Hysteria2Credentials{Password: password},
	}
	applyTLSFromPreview(spec, p)
	return spec, nil
}

// tuicPreviewToSpec 把 tuic URI 预览转为 NodeSpec。
func tuicPreviewToSpec(p *importer.URINodePreview) (*nodespec.NodeSpec, error) {
	uuid := p.UUID
	if uuid == "" {
		uuid = strOrEmpty(p.ConfigJSON["uuid"])
	}
	password := p.Password
	if password == "" {
		password = strOrEmpty(p.ConfigJSON["password"])
	}
	spec := &nodespec.NodeSpec{
		Protocol:    nodespec.ProtocolTUIC,
		Credentials: nodespec.TUICCredentials{UUID: uuid, Password: password},
	}
	applyTLSFromPreview(spec, p)
	return spec, nil
}

// applyTLSFromPreview 从 URINodePreview.ConfigJSON["tls"] 提取 TLS 配置应用到 NodeSpec。
func applyTLSFromPreview(spec *nodespec.NodeSpec, p *importer.URINodePreview) {
	if p.SecurityType == "tls" {
		spec.Security = nodespec.SecurityTLS
		spec.TLS = &nodespec.TLSConfig{}
		if tlsMap, ok := p.ConfigJSON["tls"].(map[string]interface{}); ok {
			spec.TLS.SNI = strOrEmpty(tlsMap["server_name"], tlsMap["sni"])
			spec.TLS.Fingerprint = strOrEmpty(tlsMap["fingerprint"], tlsMap["fp"])
			spec.TLS.AllowInsecure = boolOrFalse(tlsMap["allow_insecure"], tlsMap["insecure"])
			if alpn := strSliceOrEmpty(tlsMap["alpn"]); len(alpn) > 0 {
				spec.TLS.ALPN = alpn
			}
		} else {
			// 兜底：从顶层 ConfigJSON 读取
			spec.TLS.SNI = strOrEmpty(p.ConfigJSON["sni"])
			spec.TLS.Fingerprint = strOrEmpty(p.ConfigJSON["fp"], p.ConfigJSON["fingerprint"])
			spec.TLS.AllowInsecure = boolOrFalse(p.ConfigJSON["allow_insecure"], p.ConfigJSON["insecure"])
			if alpn := strSliceOrEmpty(p.ConfigJSON["alpn"]); len(alpn) > 0 {
				spec.TLS.ALPN = alpn
			}
		}
		return
	}
	if p.SecurityType == "reality" {
		spec.Security = nodespec.SecurityReality
		spec.Reality = &nodespec.RealityConfig{}
		if rMap, ok := p.ConfigJSON["reality"].(map[string]interface{}); ok {
			spec.Reality.SNI = strOrEmpty(rMap["server_name"], rMap["sni"])
			spec.Reality.PublicKey = strOrEmpty(rMap["public_key"], rMap["pbk"])
			spec.Reality.Fingerprint = strOrEmpty(rMap["fingerprint"], rMap["fp"])
			if sid := strOrEmpty(rMap["short_id"], rMap["sid"]); sid != "" {
				spec.Reality.ShortID = sid
			}
			if sids := strSliceOrEmpty(rMap["short_ids"]); len(sids) > 0 {
				spec.Reality.ShortIDs = sids
			}
		}
		return
	}
	spec.Security = nodespec.SecurityNone
}

// applyTransportFromPreview 从 URINodePreview 提取传输层配置（ws/grpc path/host）。
func applyTransportFromPreview(spec *nodespec.NodeSpec, p *importer.URINodePreview) {
	transportType := nodespec.Transport(p.TransportType)
	if transportType == "" {
		transportType = nodespec.TransportTCP
	}
	spec.Transport.Type = transportType

	switch transportType {
	case nodespec.TransportWS:
		wsPath := strOrEmpty(p.ConfigJSON["ws_path"], p.ConfigJSON["path"])
		wsHost := strOrEmpty(p.ConfigJSON["ws_host"], p.ConfigJSON["host"])
		if wsPath != "" || wsHost != "" {
			spec.Transport.WS = &nodespec.WSConfig{Path: wsPath, Host: wsHost}
		}
	case nodespec.TransportGRPC:
		serviceName := strOrEmpty(p.ConfigJSON["service_name"], p.ConfigJSON["serviceName"])
		if serviceName != "" {
			spec.Transport.GRPC = &nodespec.GRPCConfig{ServiceName: serviceName}
		}
	}
}

// redactURI 日志打印时隐藏 URI 中的凭证部分，防止密码写入日志文件。
// 把 user:pass@ 替换为 ***:***@，保留 scheme/host/port/query 供排错。
func redactURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return "***"
	}
	if u.User != nil {
		u.User = url.UserPassword("***", "***")
	}
	return u.String()
}

// strOrEmpty 从 interface{}（可能是 string）中读取第一个非空字符串。
// 支持多候选键（从嵌套 map 或多个字段名读取）。
func strOrEmpty(vals ...interface{}) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// boolOrFalse 从 interface{} 中读取布尔值（任意一个为 true 即返回 true）。
func boolOrFalse(vals ...interface{}) bool {
	for _, v := range vals {
		switch b := v.(type) {
		case bool:
			if b {
				return true
			}
		case string:
			if strings.ToLower(b) == "true" || b == "1" {
				return true
			}
		}
	}
	return false
}

// strSliceOrEmpty 从 interface{} 中读取字符串切片（兼容 []string / []interface{}）。
func strSliceOrEmpty(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				result = append(result, str)
			}
		}
		return result
	case string:
		// 单个字符串按逗号分割
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	return nil
}
