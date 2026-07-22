package exposure

import (
	"fmt"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
)

type SBInboundUser struct {
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Method   string `json:"method,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Username string `json:"username,omitempty"`
}

type SBTransport struct {
	Type        string            `json:"type,omitempty"`
	Path        string            `json:"path,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ServiceName string            `json:"service_name,omitempty"`
	Host        []string          `json:"host,omitempty"`
}

type SBTLS struct {
	Enabled    bool        `json:"enabled"`
	ServerName string      `json:"server_name,omitempty"`
	ALPN       []string    `json:"alpn,omitempty"`
	Reality    *SBReality  `json:"reality,omitempty"`
	CertificatePath string `json:"certificate_path,omitempty"`
	KeyPath     string     `json:"key_path,omitempty"`
}

type SBReality struct {
	Enabled   bool     `json:"enabled"`
	ShortID   string   `json:"short_id,omitempty"`
	PrivateKey string  `json:"private_key,omitempty"`
	Dest      string   `json:"dest,omitempty"`
	Xver      int      `json:"xver,omitempty"`
	ServerNames []string `json:"server_names,omitempty"`
}

type SBInbound struct {
	Type             string          `json:"type"`
	Tag              string          `json:"tag,omitempty"`
	Listen           string          `json:"listen,omitempty"`
	ListenPort       int             `json:"listen_port"`
	Users            []SBInboundUser `json:"users,omitempty"`
	Transport        *SBTransport    `json:"transport,omitempty"`
	TLS              *SBTLS          `json:"tls,omitempty"`
	Network          string          `json:"network,omitempty"`
	Method           string          `json:"method,omitempty"`
	Password         string          `json:"password,omitempty"`
	Detour           string          `json:"detour,omitempty"`
	InboundFields    map[string]interface{} `json:"-"`
}

type SBOutbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type SBRouteRule struct {
	Outbound    string   `json:"outbound"`
	SourceIPCIDR []string `json:"source_ip_cidr,omitempty"`
	IPCIDR      []string `json:"ip_cidr,omitempty"`
	Domain      []string `json:"domain,omitempty"`
	Inbound     []string `json:"inbound,omitempty"`
}

type SBRoute struct {
	Rules []SBRouteRule `json:"rules"`
}

type SBLog struct {
	Level string `json:"level"`
}

type SBConfig struct {
	Log       SBLog       `json:"log"`
	Inbounds  []SBInbound `json:"inbounds"`
	Outbounds []SBOutbound `json:"outbounds"`
	Route     SBRoute     `json:"route"`
}

func BuildSingboxInbounds(nodes []*model.Node, listenHost string) []map[string]interface{} {
	return BuildSingboxInboundsWithCreds(nodes, listenHost, nil)
}

// BuildSingboxInboundsWithCreds 构建带多用户凭证的 sing-box inbounds。
// creds 为 nil 或某节点无凭证时，回退到 node.config_json 中的单用户配置（向后兼容）。
func BuildSingboxInboundsWithCreds(nodes []*model.Node, listenHost string, creds NodeCredentials) []map[string]interface{} {
	inbounds := make([]map[string]interface{}, 0)
	hasNodes := false

	for _, node := range nodes {
		if !node.IsEnabled {
			continue
		}
		hasNodes = true

		var nodeCreds []*repo.UserNodeCredential
		if creds != nil {
			nodeCreds = creds[node.ID]
		}
		inbound := buildSingboxInboundMap(node, listenHost, nodeCreds)
		inbounds = append(inbounds, inbound)
	}

	if !hasNodes {
		inbounds = append(inbounds, map[string]interface{}{
			"type":        "direct",
			"tag":         "api",
			"listen":      "127.0.0.1",
			"listen_port": 10086,
		})
	}

	return inbounds
}

func buildSingboxInboundMap(node *model.Node, listenHost string, nodeCreds []*repo.UserNodeCredential) map[string]interface{} {
	inbound := make(map[string]interface{})
	inbound["tag"] = node.Code
	inbound["listen"] = listenHost
	inbound["listen_port"] = node.Port

	inboundType := node.ProtocolType
	if inboundType == "shadowsocks" || inboundType == "ss" {
		inbound["type"] = "shadowsocks"
	} else {
		inbound["type"] = inboundType
	}

	users := extractSingBoxUsers(node, nodeCreds)
	if len(users) > 0 {
		inbound["users"] = users
	}

	transport := buildSingBoxTransport(node)
	if transport != nil {
		inbound["transport"] = transport
	}
	tls := buildSingBoxTLS(node)
	if tls != nil {
		inbound["tls"] = tls
	}

	return inbound
}

func buildSingBoxInbound(node *model.Node, listenHost string, nodeCreds []*repo.UserNodeCredential) SBInbound {
	inbound := SBInbound{
		Tag:        node.Code,
		Listen:     listenHost,
		ListenPort: node.Port,
	}

	inboundType := node.ProtocolType
	if inboundType == "shadowsocks" || inboundType == "ss" {
		inbound.Type = "shadowsocks"
	} else {
		inbound.Type = inboundType
	}

	inbound.Users = extractSingBoxUsers(node, nodeCreds)

	inbound.Transport = buildSingBoxTransport(node)
	inbound.TLS = buildSingBoxTLS(node)

	return inbound
}

func buildSingBoxTransport(node *model.Node) *SBTransport {
	transportType := node.TransportType
	if transportType == "tcp" {
		return nil
	}

	tr := &SBTransport{}
	switch transportType {
	case "ws":
		tr.Type = "ws"
		if node.Path != nil {
			tr.Path = *node.Path
		} else {
			tr.Path = "/"
		}
		if node.HostHeader != nil {
			tr.Headers = map[string]string{
				"Host": *node.HostHeader,
			}
		}
	case "grpc":
		tr.Type = "grpc"
		if node.Path != nil {
			tr.ServiceName = *node.Path
		}
	case "http", "h2":
		tr.Type = "http"
		if node.HostHeader != nil {
			tr.Host = []string{*node.HostHeader}
		}
		if node.Path != nil {
			tr.Path = *node.Path
		} else {
			tr.Path = "/"
		}
	default:
		tr.Type = transportType
	}
	return tr
}

func buildSingBoxTLS(node *model.Node) *SBTLS {
	security := "none"
	if node.SecurityType != nil && *node.SecurityType != "" {
		security = *node.SecurityType
	}

	if security == "none" {
		return nil
	}

	tls := &SBTLS{
		Enabled: true,
		CertificatePath: "/etc/sing-box/server.crt",
		KeyPath:         "/etc/sing-box/server.key",
	}

	if node.SNI != nil {
		tls.ServerName = *node.SNI
	}
	if len(node.ALPN) > 0 {
		tls.ALPN = node.ALPN
	} else {
		tls.ALPN = []string{"h2", "http/1.1"}
	}

	if security == "reality" {
		reality := &SBReality{
			Enabled: true,
			Xver:    0,
		}
		if node.SNI != nil {
			reality.ServerNames = []string{*node.SNI}
			reality.Dest = *node.SNI + ":443"
		}
		// private_key 三级回退（与 xray_config.go 保持一致）
		if privKey := pickStringNested(node.ConfigJSON, "private_key", "reality", "reality_settings"); privKey != "" {
			reality.PrivateKey = privKey
		}
		// short_id 三级回退
		if shortID := pickStringNested(node.ConfigJSON, "short_id", "reality", "reality_settings"); shortID != "" {
			reality.ShortID = shortID
		}
		// dest 三级回退：顶层 dest > reality_settings.server_name:server_port
		if destOverride := pickStringNested(node.ConfigJSON, "dest", "reality", "reality_settings"); destOverride != "" {
			reality.Dest = destOverride
		} else if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
			if sn, ok := rs["server_name"].(string); ok && sn != "" {
				sp := 443
				if p, ok := rs["server_port"].(float64); ok && p > 0 {
					sp = int(p)
				}
				reality.Dest = fmt.Sprintf("%s:%d", sn, sp)
				reality.ServerNames = []string{sn}
			}
		}
		tls.Reality = reality
		tls.CertificatePath = ""
		tls.KeyPath = ""
	}

	return tls
}

func extractSingBoxUsers(node *model.Node, nodeCreds []*repo.UserNodeCredential) []SBInboundUser {
	// 优先使用 per-user 凭证（多用户模式）
	if len(nodeCreds) > 0 {
		return buildSingBoxUsersFromCreds(node, nodeCreds)
	}

	users := make([]SBInboundUser, 0)

	if node.ConfigJSON == nil {
		return users
	}

	if userList, ok := node.ConfigJSON["clients"].([]interface{}); ok {
		for _, u := range userList {
			if um, ok := u.(map[string]interface{}); ok {
				user := SBInboundUser{}
				switch node.ProtocolType {
				case "vless":
					if uuid, ok := um["id"].(string); ok {
						user.UUID = uuid
					}
					if flow, ok := um["flow"].(string); ok {
						user.Flow = flow
					}
				case "vmess":
					if uuid, ok := um["id"].(string); ok {
						user.UUID = uuid
					}
				case "trojan":
					if password, ok := um["password"].(string); ok {
						user.Password = password
					}
				case "shadowsocks", "ss":
					if password, ok := um["password"].(string); ok {
						user.Password = password
					}
					if method, ok := um["method"].(string); ok {
						user.Method = method
					}
				}
				users = append(users, user)
			}
		}
	}

	if len(users) == 0 {
		user := SBInboundUser{}
		switch node.ProtocolType {
		case "vless":
			if uuid, ok := getStringFromConfig(node.ConfigJSON, "uuid"); ok && uuid != "" {
				user.UUID = uuid
				if node.Flow != nil {
					user.Flow = *node.Flow
				}
				users = append(users, user)
			}
		case "vmess":
			if uuid, ok := getStringFromConfig(node.ConfigJSON, "uuid"); ok && uuid != "" {
				user.UUID = uuid
				users = append(users, user)
			}
		case "trojan":
			if password, ok := getStringFromConfig(node.ConfigJSON, "password"); ok && password != "" {
				user.Password = password
				users = append(users, user)
			}
		case "shadowsocks", "ss":
			password, _ := getStringFromConfig(node.ConfigJSON, "password")
			method, _ := getStringFromConfig(node.ConfigJSON, "method")
			if password != "" && method != "" {
				user.Password = password
				user.Method = method
				users = append(users, user)
			}
		case "tuic":
			// TUIC 使用 UUID 凭证（与 VLESS/VMess 一致）
			if uuid, ok := getStringFromConfig(node.ConfigJSON, "uuid"); ok && uuid != "" {
				user.UUID = uuid
				if password, ok := getStringFromConfig(node.ConfigJSON, "password"); ok {
					user.Password = password
				}
				users = append(users, user)
			}
		}
	}

	return users
}

// buildSingBoxUsersFromCreds 从 per-user 凭证生成 sing-box inbound users
// UUID 凭证 → vless/vmess/tuic 的 UUID 字段
// buildSingBoxUsersFromCreds 从 per-user 凭证生成 sing-box users 数组
// 对齐 XBoard：所有协议都用 user.uuid（CredentialType 统一为 "uuid"）
// VLESS/VMess/TUIC: UUID 字段 = user.uuid
// Trojan/SS/Hysteria2/AnyTLS: Password 字段 = user.uuid
// SS2022 派生在订阅渲染层处理，节点端统一用 uuid
func buildSingBoxUsersFromCreds(node *model.Node, creds []*repo.UserNodeCredential) []SBInboundUser {
	users := make([]SBInboundUser, 0, len(creds))
	proto := node.ProtocolType
	for _, c := range creds {
		if c.CredentialValue == "" {
			continue
		}
		user := SBInboundUser{}
		switch proto {
		case "vless":
			user.UUID = c.CredentialValue
			if node.Flow != nil {
				user.Flow = *node.Flow
			}
		case "vmess":
			user.UUID = c.CredentialValue
		case "tuic":
			user.UUID = c.CredentialValue
		case "trojan":
			// XBoard 模型：Trojan 密码 = user.uuid
			user.Password = c.CredentialValue
		case "shadowsocks", "ss":
			// XBoard 模型：SS 密码 = user.uuid（SS2022 派生在订阅层处理）
			user.Password = c.CredentialValue
			if method, ok := getStringFromConfig(node.ConfigJSON, "method"); ok {
				user.Method = method
			}
		default:
			// 未知协议：按 UUID 处理（XBoard 模型默认）
			user.UUID = c.CredentialValue
		}
		users = append(users, user)
	}
	return users
}
