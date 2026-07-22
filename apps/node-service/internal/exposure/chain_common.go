package exposure

import (
	"fmt"

	"github.com/airport-panel/subscription/nodespec"
)

// ChainOutboundFields 是双内核共用的中间表示（IR），
// 从 NodeSpec 提取一次，xray/sing-box 各自序列化成自己的 JSON 结构。
// 借鉴 xboard-node 设计：协议字段映射只写一次，双内核只提供薄适配层，
// 消除 chain_xray.go/chain_singbox.go 各自维护协议分支 switch 的重复。
type ChainOutboundFields struct {
	Protocol        string
	Address         string
	Port            int
	Username        string // socks5/http 认证用户名
	Password        string // socks5/http/trojan/ss 认证密码
	UUID            string // vless/vmess 用户ID
	AlterID         int    // vmess alterId
	SSMethod        string // shadowsocks 加密方式
	Flow            string // vless xtls-rprx-vision
	Encryption      string // vless encryption（默认 none）
	TLSEnabled      bool
	SNI             string
	Fingerprint     string
	ALPN            []string
	AllowInsecure   bool
	IsReality       bool
	RealityPublicKey string
	RealityShortID  string
	Transport       string // tcp/ws/grpc/xhttp
	WSPath          string
	WSHost          string
	GRPCServiceName string
}

// ExtractChainOutboundFields 是唯一的字段提取入口，双内核渲染器都调用此函数。
// 从 NodeSpec 提取与套娃 outbound 相关的通用字段，避免 xray/sing-box 两侧重复提取导致行为分叉。
func ExtractChainOutboundFields(ns *nodespec.NodeSpec) (*ChainOutboundFields, error) {
	if ns == nil {
		return nil, fmt.Errorf("nil nodespec")
	}

	f := &ChainOutboundFields{
		Protocol:  normalizeChainProtocol(string(ns.Protocol)),
		Address:   ns.Address,
		Port:      ns.Port,
		Transport: string(ns.Transport.Type),
	}

	// 按凭证类型提取（统一处理具体类型和 map[string]interface{}）
	if ns.Credentials != nil {
		if err := extractCredentialsFields(f, ns.Credentials); err != nil {
			return nil, err
		}
	}

	// TLS / Reality 字段提取
	extractTLSFields(f, ns)

	// Transport 字段提取
	extractTransportFields(f, ns)

	return f, nil
}

// extractCredentialsFields 从 Credentials interface{} 提取凭证字段。
// 支持 nodespec 的具体凭证类型（值类型）和 map[string]interface{}（URI 解析路径）。
func extractCredentialsFields(f *ChainOutboundFields, creds interface{}) error {
	switch c := creds.(type) {
	case nodespec.SOCKS5Credentials:
		f.Username, f.Password = c.Username, c.Password
	case nodespec.HTTPCredentials:
		f.Username, f.Password = c.Username, c.Password
	case nodespec.VLESSCredentials:
		f.UUID = c.UUID
		f.Flow = string(c.Flow)
		f.Encryption = c.Encryption
		if f.Encryption == "" {
			f.Encryption = "none"
		}
	case nodespec.VMessCredentials:
		f.UUID = c.UUID
		f.AlterID = c.AlterID
	case nodespec.TrojanCredentials:
		f.Password = c.Password
	case nodespec.ShadowsocksCredentials:
		f.SSMethod = c.Method
		f.Password = c.Password
	case nodespec.Hysteria2Credentials:
		f.Password = c.Password
	case nodespec.TUICCredentials:
		f.UUID = c.UUID
		f.Password = c.Password
	case map[string]interface{}:
		return extractCredentialsFromMap(f, c)
	default:
		return fmt.Errorf("unsupported chain credential type: %T", creds)
	}
	return nil
}

// extractCredentialsFromMap 从 map[string]interface{} 提取凭证字段。
// 用于 URI 解析路径（URINodePreview.ConfigJSON 转换为 NodeSpec 时 Credentials 可能是 map）。
func extractCredentialsFromMap(f *ChainOutboundFields, m map[string]interface{}) error {
	if v, ok := m["username"].(string); ok {
		f.Username = v
	}
	if v, ok := m["password"].(string); ok {
		f.Password = v
	}
	if v, ok := m["uuid"].(string); ok {
		f.UUID = v
	}
	if v, ok := m["alterId"]; ok {
		f.AlterID = anyToInt(v)
	} else if v, ok := m["alter_id"]; ok {
		f.AlterID = anyToInt(v)
	}
	if v, ok := m["method"].(string); ok {
		f.SSMethod = v
	}
	if v, ok := m["flow"].(string); ok {
		f.Flow = v
	}
	if v, ok := m["encryption"].(string); ok {
		f.Encryption = v
	}
	if f.Encryption == "" && f.Protocol == "vless" {
		f.Encryption = "none"
	}
	return nil
}

// extractTLSFields 从 NodeSpec 提取 TLS/Reality 字段到 ChainOutboundFields。
func extractTLSFields(f *ChainOutboundFields, ns *nodespec.NodeSpec) {
	if ns.Security == nodespec.SecurityReality && ns.Reality != nil {
		f.IsReality = true
		f.TLSEnabled = true
		f.SNI = ns.Reality.SNI
		f.RealityPublicKey = ns.Reality.PublicKey
		f.RealityShortID = ns.Reality.ShortID
		if ns.Reality.Fingerprint != "" {
			f.Fingerprint = ns.Reality.Fingerprint
		}
		if len(ns.Reality.ALPN) > 0 {
			f.ALPN = ns.Reality.ALPN
		}
		return
	}

	if ns.Security == nodespec.SecurityTLS && ns.TLS != nil {
		f.TLSEnabled = true
		f.SNI = ns.TLS.SNI
		f.Fingerprint = ns.TLS.Fingerprint
		f.ALPN = ns.TLS.ALPN
		f.AllowInsecure = ns.TLS.AllowInsecure
	}
}

// extractTransportFields 从 NodeSpec.Transport 提取传输层字段。
func extractTransportFields(f *ChainOutboundFields, ns *nodespec.NodeSpec) {
	if ns.Transport.WS != nil {
		f.WSPath = ns.Transport.WS.Path
		f.WSHost = ns.Transport.WS.Host
	}
	if ns.Transport.GRPC != nil {
		f.GRPCServiceName = ns.Transport.GRPC.ServiceName
	}
	// HTTP2/HTTPUpgrade/XHTTP 的 path/host 从 Headers 或 RawSettings 兜底
	if ns.Transport.Headers != nil {
		if f.WSHost == "" {
			f.WSHost = ns.Transport.Headers["Host"]
		}
	}
}

// normalizeChainProtocol 把协议标识归一化为内核渲染使用的标准名。
func normalizeChainProtocol(p string) string {
	switch p {
	case "socks5", "socks5h":
		return "socks"
	case "shadowsocks":
		return "ss"
	default:
		return p
	}
}

// anyToInt 把 interface{} 转为 int（兼容 float64/int/int64/json number）。
func anyToInt(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		if n, err := parseIntSafe(val); err == nil {
			return n
		}
	}
	return 0
}

func parseIntSafe(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
