package importer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Parser 通用接口
type Parser interface {
	Parse(content string) (*NodeSpec, error)
}

// --- Xray ---

// XrayConfigParser 解析 Xray config.json
// 提取 inbounds[].protocol、inbounds[].port、inbounds[].settings.clients[].id、
// inbounds[].streamSettings.network、streamSettings.security、
// streamSettings.tlsSettings.certificates[].certificateFile
func XrayConfigParser(content string) (*NodeSpec, error) {
	var raw struct {
		Inbounds []struct {
			Protocol       string `json:"protocol"`
			Port           int    `json:"port"`
			Settings       struct {
				Clients []struct {
					ID string `json:"id"`
				} `json:"clients"`
			} `json:"settings"`
			StreamSettings struct {
				Network   string `json:"network"`
				Security  string `json:"security"`
				TLSSettings struct {
					Certificates []struct {
						CertificateFile string `json:"certificateFile"`
					} `json:"certificates"`
				} `json:"tlsSettings"`
				WSSettings struct {
					Path string `json:"path"`
				} `json:"wsSettings"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("xray parse: %w", err)
	}

	spec := &NodeSpec{
		UUIDs:       []string{},
		RawMetadata: map[string]interface{}{},
	}
	if len(raw.Inbounds) == 0 {
		return spec, nil
	}
	// 取第一个 inbound 作为主信息
	in := raw.Inbounds[0]
	spec.ProtocolType = in.Protocol
	spec.ListenPort = in.Port
	spec.TransportType = in.StreamSettings.Network
	if in.StreamSettings.Network == "" {
		spec.TransportType = "tcp"
	}
	spec.SecurityType = in.StreamSettings.Security
	if spec.SecurityType == "" {
		spec.SecurityType = "none"
	}
	for _, c := range in.Settings.Clients {
		if c.ID != "" {
			spec.UUIDs = append(spec.UUIDs, c.ID)
		}
	}
	for _, cert := range in.StreamSettings.TLSSettings.Certificates {
		if cert.CertificateFile != "" {
			spec.CertPath = cert.CertificateFile
			break
		}
	}
	// 收集所有 inbound 的 raw 信息
	inbounds := make([]map[string]interface{}, 0, len(raw.Inbounds))
	for _, ib := range raw.Inbounds {
		b, _ := json.Marshal(ib)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		inbounds = append(inbounds, m)
	}
	spec.RawMetadata["inbounds"] = inbounds
	if in.StreamSettings.WSSettings.Path != "" {
		spec.RawMetadata["ws_path"] = in.StreamSettings.WSSettings.Path
	}
	annotateMissingFields(spec)
	return spec, nil
}

// --- SingBox ---

// SingBoxConfigParser 解析 sing-box config.json
// 提取 inbounds[].type、listen_port、tls.enabled、tls.server_name
func SingBoxConfigParser(content string) (*NodeSpec, error) {
	var raw struct {
		Inbounds []struct {
			Type       string `json:"type"`
			ListenPort int    `json:"listen_port"`
			TLS        struct {
				Enabled    bool   `json:"enabled"`
				ServerName string `json:"server_name"`
			} `json:"tls"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("singbox parse: %w", err)
	}

	spec := &NodeSpec{
		UUIDs:       []string{},
		RawMetadata: map[string]interface{}{},
	}
	if len(raw.Inbounds) == 0 {
		return spec, nil
	}
	in := raw.Inbounds[0]
	spec.ProtocolType = in.Type
	spec.ListenPort = in.ListenPort
	// sing-box 的 hysteria2/tuic 是 UDP
	if in.Type == "hysteria2" || in.Type == "tuic" {
		spec.TransportType = "udp"
	} else {
		spec.TransportType = "tcp"
	}
	if in.TLS.Enabled {
		spec.SecurityType = "tls"
	} else {
		spec.SecurityType = "none"
	}
	if in.TLS.ServerName != "" {
		spec.SNI = in.TLS.ServerName
	}
	inbounds := make([]map[string]interface{}, 0, len(raw.Inbounds))
	for _, ib := range raw.Inbounds {
		b, _ := json.Marshal(ib)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		inbounds = append(inbounds, m)
	}
	spec.RawMetadata["inbounds"] = inbounds
	annotateMissingFields(spec)
	return spec, nil
}

// --- Nginx ---

// nginxConfRegexes 用于提取 server_name / listen / ssl_certificate / location proxy_pass
var (
	nginxServerNameRe = regexp.MustCompile(`(?m)server_name\s+([^;]+);`)
	nginxListenRe     = regexp.MustCompile(`(?m)listen\s+(\d+)`)
	nginxSSLCertRe    = regexp.MustCompile(`(?m)ssl_certificate\s+([^;]+);`)
	nginxLocationRe   = regexp.MustCompile(`(?ms)location\s+([^\s{]+)\s*\{([^}]*?)\}`)
	nginxProxyPassRe  = regexp.MustCompile(`proxy_pass\s+(http[s]?://[^;]+);`)
)

// NginxConfParser 用正则提取 server_name、listen、ssl_certificate、location /path { proxy_pass }
func NginxConfParser(content string) (*NodeSpec, error) {
	spec := &NodeSpec{
		UUIDs:       []string{},
		RawMetadata: map[string]interface{}{},
	}

	if m := nginxServerNameRe.FindStringSubmatch(content); len(m) > 1 {
		spec.SNI = strings.TrimSpace(m[1])
	}
	if m := nginxListenRe.FindStringSubmatch(content); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &spec.ListenPort)
	}
	if m := nginxSSLCertRe.FindStringSubmatch(content); len(m) > 1 {
		spec.CertPath = strings.TrimSpace(m[1])
	}
	// 推断 security_type：有 ssl_certificate 即为 tls
	if spec.CertPath != "" {
		spec.SecurityType = "tls"
	} else {
		spec.SecurityType = "none"
	}

	// location 提取
	locations := []map[string]interface{}{}
	for _, m := range nginxLocationRe.FindAllStringSubmatch(content, -1) {
		if len(m) < 3 {
			continue
		}
		path := strings.TrimSpace(m[1])
		body := m[2]
		entry := map[string]interface{}{"path": path}
		if pm := nginxProxyPassRe.FindStringSubmatch(body); len(pm) > 1 {
			entry["proxy_pass"] = strings.TrimSpace(pm[1])
		}
		locations = append(locations, entry)
	}
	spec.RawMetadata["locations"] = locations
	// nginx 反代通常用于 WS，推断 transport
	if len(locations) > 0 {
		spec.TransportType = "ws"
	}
	// protocol 无法从 nginx 反代配置确定，留空
	spec.ProtocolType = ""
	annotateMissingFields(spec)
	return spec, nil
}

// --- Cloudflared ---

// CloudflaredConfigParser 解析 cloudflared config.yml
// 提取 tunnel.token、ingress[].hostname、ingress[].service
// 使用基于行的简化解析（避免引入 YAML 依赖）
func CloudflaredConfigParser(content string) (*NodeSpec, error) {
	spec := &NodeSpec{
		UUIDs:       []string{},
		RawMetadata: map[string]interface{}{},
	}

	lines := strings.Split(content, "\n")
	type ingressEntry struct {
		Hostname string
		Service  string
	}
	var ingress []ingressEntry
	var current *ingressEntry
	var inIngressBlock bool
	var tunnelToken string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		// tunnel: 下属 token
		if strings.Contains(trimmed, "token:") {
			parts := strings.SplitN(trimmed, "token:", 2)
			if len(parts) == 2 {
				token := strings.TrimSpace(parts[1])
				token = strings.Trim(token, "\"'")
				tunnelToken = token
			}
		}
		// ingress 列表项起始（以 "-" 开头）
		trimmedLeft := strings.TrimLeft(trimmed, " \t")
		if strings.HasPrefix(trimmedLeft, "- ") {
			// 新的 ingress 条目
			if current != nil {
				ingress = append(ingress, *current)
			}
			current = &ingressEntry{}
			inIngressBlock = true
			// 同一行可能包含 hostname 或 service
			rest := strings.TrimSpace(strings.TrimPrefix(trimmedLeft, "- "))
			if kv := splitYAMLKV(rest); kv != nil {
				if kv.key == "hostname" {
					current.Hostname = kv.value
				} else if kv.key == "service" {
					current.Service = kv.value
				}
			}
			continue
		}
		// ingress 条目内的 service/hostname（缩进续行）
		if inIngressBlock && current != nil {
			if kv := splitYAMLKV(trimmedLeft); kv != nil {
				if kv.key == "hostname" {
					current.Hostname = kv.value
				} else if kv.key == "service" {
					current.Service = kv.value
				}
			}
		}
	}
	if current != nil {
		ingress = append(ingress, *current)
	}

	if tunnelToken != "" {
		spec.RawMetadata["tunnel_token"] = tunnelToken
	}
	ingressList := make([]map[string]interface{}, 0, len(ingress))
	for _, e := range ingress {
		ingressList = append(ingressList, map[string]interface{}{
			"hostname": e.Hostname,
			"service":  e.Service,
		})
	}
	spec.RawMetadata["ingress"] = ingressList
	// 从 ingress 推断 SNI（取第一个有 hostname 的条目）
	for _, e := range ingress {
		if e.Hostname != "" {
			spec.SNI = e.Hostname
			break
		}
	}
	// cloudflared tunnel 通常对应 cloudflare_tunnel_fixed 暴露模式；transport 推断为 ws（最常见）
	spec.TransportType = "ws"
	spec.SecurityType = "tls"
	annotateMissingFields(spec)
	return spec, nil
}

// yamlKV 表示一个 key: value 对
type yamlKV struct {
	key   string
	value string
}

// splitYAMLKV 解析 "key: value" 形式的行，返回 key/value（去除引号与空白）
func splitYAMLKV(s string) *yamlKV {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return nil
	}
	key := strings.TrimSpace(s[:idx])
	val := strings.TrimSpace(s[idx+1:])
	val = strings.Trim(val, "\"'")
	if key == "" {
		return nil
	}
	return &yamlKV{key: key, value: val}
}

// --- Builder ---

// BuildNodeSpec 合并多个 Parser 的 NodeSpec 结果
func BuildNodeSpec(specs []*NodeSpec) *NodeSpec {
	merged := &NodeSpec{
		UUIDs:       []string{},
		RawMetadata: map[string]interface{}{},
	}
	for _, s := range specs {
		if s == nil {
			continue
		}
		if merged.ProtocolType == "" && s.ProtocolType != "" {
			merged.ProtocolType = s.ProtocolType
		}
		if merged.TransportType == "" && s.TransportType != "" {
			merged.TransportType = s.TransportType
		}
		if merged.SecurityType == "" && s.SecurityType != "" {
			merged.SecurityType = s.SecurityType
		}
		if merged.ListenPort == 0 && s.ListenPort != 0 {
			merged.ListenPort = s.ListenPort
		}
		merged.UUIDs = append(merged.UUIDs, s.UUIDs...)
		if merged.SNI == "" && s.SNI != "" {
			merged.SNI = s.SNI
		}
		if merged.CertPath == "" && s.CertPath != "" {
			merged.CertPath = s.CertPath
		}
		for k, v := range s.RawMetadata {
			merged.RawMetadata[k] = v
		}
	}
	annotateMissingFields(merged)
	return merged
}

// annotateMissingFields 标注缺失字段（UUID 缺失、证书 PEM 未提供等）
func annotateMissingFields(spec *NodeSpec) {
	var missing []string
	if spec.ProtocolType == "" {
		missing = append(missing, "protocol_type")
	}
	if spec.TransportType == "" {
		missing = append(missing, "transport_type")
	}
	if spec.SecurityType == "" {
		missing = append(missing, "security_type")
	}
	if spec.ListenPort == 0 {
		missing = append(missing, "listen_port")
	}
	if len(spec.UUIDs) == 0 && spec.ProtocolType != "" && needsUUID(spec.ProtocolType) {
		missing = append(missing, "uuids")
	}
	if (spec.SecurityType == "tls" || spec.SecurityType == "reality") && spec.CertPath == "" {
		missing = append(missing, "cert_pem")
	}
	spec.MissingFields = missing
}

// needsUUID 判断协议是否需要 UUID（vless/vmess/trojan 需要）
func needsUUID(protocol string) bool {
	switch protocol {
	case "vless", "vmess", "trojan":
		return true
	}
	return false
}

// ParserForSourceType 根据源类型返回对应 Parser
func ParserForSourceType(sourceType string) (Parser, error) {
	switch sourceType {
	case "xray":
		return parserFunc(XrayConfigParser), nil
	case "singbox":
		return parserFunc(SingBoxConfigParser), nil
	case "nginx":
		return parserFunc(NginxConfParser), nil
	case "cloudflared":
		return parserFunc(CloudflaredConfigParser), nil
	default:
		return nil, ErrUnsupportedSource
	}
}

// parserFunc 把函数包装成 Parser 接口实现
type parserFunc func(content string) (*NodeSpec, error)

func (f parserFunc) Parse(content string) (*NodeSpec, error) { return f(content) }
