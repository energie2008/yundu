package importer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type URINodePreview struct {
	Name          string                 `json:"name"`
	ProtocolType  string                 `json:"protocol_type"`
	TransportType string                 `json:"transport_type"`
	SecurityType  string                 `json:"security_type"`
	Host          string                 `json:"host"`
	Port          int                    `json:"port"`
	UUID          string                 `json:"uuid,omitempty"`
	Password      string                 `json:"password,omitempty"`
	ConfigJSON    map[string]interface{} `json:"config_json"`
	Valid         bool                   `json:"valid"`
	Warning       string                 `json:"warning,omitempty"`
}

func ParseURI(uri string) (*URINodePreview, error) {
	nodes, errs := ParseURIs(uri)
	if len(errs) > 0 && len(nodes) == 0 {
		return nil, fmt.Errorf("%s", errs[0])
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid URI found")
	}
	return nodes[0], nil
}

func RenderURI(spec *URINodePreview) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("nil spec")
	}
	user := ""
	if spec.UUID != "" {
		user = spec.UUID
	} else if spec.Password != "" {
		user = spec.Password
	} else if u, _ := spec.ConfigJSON["uuid"].(string); u != "" {
		user = u
	} else if p, _ := spec.ConfigJSON["password"].(string); p != "" {
		user = p
	}
	if user == "" {
		return "", fmt.Errorf("missing credentials (uuid/password)")
	}

	params := url.Values{}
	if spec.TransportType != "" && spec.TransportType != "tcp" {
		params.Set("type", spec.TransportType)
	}
	if spec.SecurityType != "" && spec.SecurityType != "none" {
		params.Set("security", spec.SecurityType)
	}
	if sni, _ := spec.ConfigJSON["sni"].(string); sni != "" {
		params.Set("sni", sni)
	}
	if fp, _ := spec.ConfigJSON["fp"].(string); fp != "" {
		params.Set("fp", fp)
	}
	if wsPath, _ := spec.ConfigJSON["ws_path"].(string); wsPath != "" {
		params.Set("path", wsPath)
	}
	if wsHost, _ := spec.ConfigJSON["ws_host"].(string); wsHost != "" {
		params.Set("host", wsHost)
	}
	if alpn, _ := spec.ConfigJSON["alpn"].(string); alpn != "" {
		params.Set("alpn", alpn)
	}
	if pbk, _ := spec.ConfigJSON["pbk"].(string); pbk != "" {
		params.Set("pbk", pbk)
	}
	if sid, _ := spec.ConfigJSON["sid"].(string); sid != "" {
		params.Set("sid", sid)
	}
	if spx, _ := spec.ConfigJSON["spx"].(string); spx != "" {
		params.Set("spx", spx)
	}
	if flow, _ := spec.ConfigJSON["flow"].(string); flow != "" {
		params.Set("flow", flow)
	}
	if serviceName, _ := spec.ConfigJSON["service_name"].(string); serviceName != "" {
		params.Set("serviceName", serviceName)
	}
	if method, _ := spec.ConfigJSON["method"].(string); method != "" {
		params.Set("method", method)
	}

	u := &url.URL{
		Scheme:   spec.ProtocolType,
		User:     url.User(user),
		Host:     net.JoinHostPort(spec.Host, strconv.Itoa(spec.Port)),
		Fragment: spec.Name,
	}
	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}
	return u.String(), nil
}

type URIImportNode struct {
	Name          string                 `json:"name"`
	ProtocolType  string                 `json:"protocol_type" binding:"required"`
	TransportType string                 `json:"transport_type"`
	SecurityType  string                 `json:"security_type"`
	Host          string                 `json:"host" binding:"required"`
	Port          int                    `json:"port" binding:"required,min=1,max=65535"`
	ConfigJSON    map[string]interface{} `json:"config_json"`
	ServerID      *string                `json:"server_id"`
	RuntimeID     *string                `json:"runtime_id"`
	Code          string                 `json:"code"`
	Region        string                 `json:"region"`
	GroupID       *string                `json:"group_id"`
	Multiplier    float64                `json:"multiplier"`
}

func ParseURIs(content string) ([]*URINodePreview, []string) {
	var nodes []*URINodePreview
	var errors []string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		decoded := line
		if strings.HasPrefix(strings.ToLower(line), "vmess://") {
			if b64 := strings.TrimPrefix(line, "vmess://"); b64 != "" {
				if data, err := base64Decode(b64); err == nil {
					decoded = string(data)
					if node, err := parseVmess(decoded); err == nil {
						nodes = append(nodes, node)
					} else {
						errors = append(errors, fmt.Sprintf("vmess parse error: %v", err))
					}
					continue
				}
			}
		}

		lower := strings.ToLower(decoded)
		switch {
		case strings.HasPrefix(lower, "vless://"):
			if node, err := parseStandardURI("vless", decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("vless parse error: %v", err))
			}
		case strings.HasPrefix(lower, "trojan://"):
			if node, err := parseStandardURI("trojan", decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("trojan parse error: %v", err))
			}
		case strings.HasPrefix(lower, "ss://"):
			if node, err := parseSS(decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("ss parse error: %v", err))
			}
		case strings.HasPrefix(lower, "hysteria2://") || strings.HasPrefix(lower, "hy2://"):
			if node, err := parseStandardURI("hysteria2", decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("hysteria2 parse error: %v", err))
			}
		case strings.HasPrefix(lower, "tuic://"):
			if node, err := parseStandardURI("tuic", decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("tuic parse error: %v", err))
			}
		case strings.HasPrefix(lower, "socks5://") || strings.HasPrefix(lower, "socks://") || strings.HasPrefix(lower, "socks5h://"):
			if node, err := parseSOCKS5(decoded); err == nil {
				nodes = append(nodes, node)
			} else {
				errors = append(errors, fmt.Sprintf("socks5 parse error: %v", err))
			}
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			// 仅当含 user info 时视为 HTTP 代理 URI（区分普通网站 URL）
			if u, err := url.Parse(decoded); err == nil && u.User != nil {
				if node, err := parseHTTPProxy(decoded, u); err == nil {
					nodes = append(nodes, node)
				} else {
					errors = append(errors, fmt.Sprintf("http parse error: %v", err))
				}
			} else if err != nil {
				errors = append(errors, fmt.Sprintf("http parse error: %v", err))
			}
		default:
			if strings.Contains(decoded, "://") {
				errors = append(errors, fmt.Sprintf("unsupported URI scheme: %s", decoded[:min(30, len(decoded))]))
			} else {
				errors = append(errors, fmt.Sprintf("unrecognized URI format: %s", decoded[:min(30, len(decoded))]))
			}
		}
	}

	return nodes, errors
}

func parseVmess(data string) (*URINodePreview, error) {
	var vmess struct {
		Name    string `json:"ps"`
		Add     string `json:"add"`
		Port    interface{} `json:"port"`
		ID      string `json:"id"`
		Net     string `json:"net"`
		Type    string `json:"type"`
		Host    string `json:"host"`
		Path    string `json:"path"`
		TLS     string `json:"tls"`
		SNI     string `json:"sni"`
		ALPN    string `json:"alpn"`
		Fp      string `json:"fp"`
		AID     interface{} `json:"aid"`
		Scy     string `json:"scy"`
		V       string `json:"v"`
	}

	if err := json.Unmarshal([]byte(data), &vmess); err != nil {
		return nil, fmt.Errorf("invalid vmess json: %w", err)
	}

	port := parsePort(vmess.Port)
	if port == 0 {
		port = 443
	}

	transport := "tcp"
	if vmess.Net != "" {
		transport = vmess.Net
	}

	security := "none"
	if vmess.TLS == "tls" {
		security = "tls"
	}

	host := vmess.Add
	if host == "" {
		host = vmess.Host
	}

	name := vmess.Name
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	configJSON := map[string]interface{}{
		"uuid":       vmess.ID,
		"alter_id":   parsePort(vmess.AID),
		"security":   firstNonEmpty(vmess.Scy, "auto"),
		"network":    transport,
		"host":       vmess.Host,
		"path":       vmess.Path,
		"sni":        vmess.SNI,
		"alpn":       parseCSV(vmess.ALPN),
		"fp":         vmess.Fp,
		"fingerprint": vmess.Fp,
	}

	if vmess.TLS == "tls" {
		tlsMap := map[string]interface{}{}
		if vmess.SNI != "" {
			tlsMap["server_name"] = vmess.SNI
		}
		if vmess.Fp != "" {
			tlsMap["fingerprint"] = vmess.Fp
		}
		if alpn := parseCSV(vmess.ALPN); len(alpn) > 0 {
			tlsMap["alpn"] = alpn
		}
		configJSON["tls"] = tlsMap
	}
	if vmess.Path != "" {
		configJSON["ws_path"] = vmess.Path
	}
	if vmess.Host != "" {
		configJSON["ws_host"] = vmess.Host
	}

	warning := ""
	if vmess.ID == "" {
		warning = "missing uuid"
	}

	return &URINodePreview{
		Name:          name,
		ProtocolType:  "vmess",
		TransportType: transport,
		SecurityType:  security,
		Host:          host,
		Port:          port,
		UUID:          vmess.ID,
		ConfigJSON:    configJSON,
		Valid:         vmess.ID != "" && host != "",
		Warning:       warning,
	}, nil
}

func parseStandardURI(protocol, uri string) (*URINodePreview, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	portStr := u.Port()
	port := 0
	switch protocol {
	case "vless", "trojan":
		port = 443
	case "hysteria2":
		port = 443
	case "tuic":
		port = 443
	}
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	user := ""
	if u.User != nil {
		user = u.User.Username()
	}

	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	transport := "tcp"
	security := "none"
	params := u.Query()

	if t := params.Get("type"); t != "" {
		transport = t
	}

	if s := params.Get("security"); s != "" {
		security = s
	} else if protocol == "hysteria2" || protocol == "tuic" {
		security = "tls"
	} else {
		if port == 443 {
			security = "tls"
		}
	}

	configJSON := map[string]interface{}{
		"password": user,
	}

	for _, key := range []string{"sni", "host", "path", "serviceName", "fp", "alpn", "pbk", "sid", "spx", "allowInsecure", "insecure", "fingerprint", "headerType", "type", "flow", "encryption", "obfs", "obfs-password", "up", "down", "congestion_control", "udp_relay_mode", "zero_rtt_handshake", "password"} {
		if v := params.Get(key); v != "" {
			switch key {
			case "alpn":
				configJSON[key] = parseCSV(v)
			case "allowInsecure", "insecure":
				configJSON["allow_insecure"] = v == "1" || strings.ToLower(v) == "true"
			case "fp", "fingerprint":
				configJSON["fp"] = v
			case "path":
				configJSON["path"] = v
				configJSON["ws_path"] = v
			case "host":
				configJSON["host"] = v
				configJSON["ws_host"] = v
			case "pbk":
				// reality public key - store in both places
				configJSON["pbk"] = v
			case "sid":
				configJSON["sid"] = v
			case "spx":
				configJSON["spx"] = v
			case "headerType":
				configJSON["tcp_header_type"] = v
			case "serviceName":
				configJSON["service_name"] = v
			case "zero_rtt_handshake":
				configJSON["zero_rtt_handshake"] = v == "1" || strings.ToLower(v) == "true"
			default:
				configJSON[key] = v
			}
		}
	}
	if protocol == "vless" {
		configJSON["uuid"] = user
		if flow := params.Get("flow"); flow != "" {
			configJSON["flow"] = flow
		}
		encryption := params.Get("encryption")
		if encryption == "" {
			encryption = "none"
		}
		configJSON["encryption"] = encryption
	}
	if protocol == "trojan" {
		configJSON["password"] = user
	}
	if protocol == "hysteria2" {
		configJSON["password"] = user
		if obfs := params.Get("obfs"); obfs != "" {
			configJSON["obfs"] = map[string]interface{}{
				"type":     obfs,
				"password": params.Get("obfs-password"),
			}
		}
		up := params.Get("up")
		down := params.Get("down")
		if up != "" {
			if upInt, err := strconv.Atoi(up); err == nil {
				configJSON["up_mbps"] = upInt
			} else {
				configJSON["up_mbps"] = up
			}
		}
		if down != "" {
			if downInt, err := strconv.Atoi(down); err == nil {
				configJSON["down_mbps"] = downInt
			} else {
				configJSON["down_mbps"] = down
			}
		}
		if _, ok := configJSON["alpn"]; !ok {
			configJSON["alpn"] = []string{"h3"}
		}
	}
	if protocol == "tuic" {
		configJSON["uuid"] = user
		if pwd, ok := u.User.Password(); ok && pwd != "" {
			configJSON["password"] = pwd
		} else if pwd := params.Get("password"); pwd != "" {
			configJSON["password"] = pwd
		}
		if cong := params.Get("congestion_control"); cong != "" {
			configJSON["congestion_control"] = cong
		}
		udpRelayMode := params.Get("udp_relay_mode")
		if udpRelayMode != "" {
			configJSON["udp_relay_mode"] = udpRelayMode
		}
		if zrt := params.Get("zero_rtt_handshake"); zrt != "" {
			configJSON["zero_rtt_handshake"] = zrt == "1" || strings.ToLower(zrt) == "true"
		} else {
			configJSON["zero_rtt_handshake"] = true
		}
		if _, ok := configJSON["alpn"]; !ok {
			configJSON["alpn"] = []string{"h3"}
		}
	}

	warning := ""
	if host == "" {
		warning = "missing host"
	}
	valid := host != ""
	if protocol == "vless" && user == "" {
		warning = "missing uuid"
	}
	if protocol == "trojan" && user == "" {
		warning = "missing password"
	}

	if protocol == "hysteria2" || protocol == "tuic" {
		transport = "udp"
	}

	sni := configJSON["sni"]
	fp := configJSON["fp"]
	if fp == nil {
		fp = configJSON["fingerprint"]
	}
	if security == "tls" {
		tlsMap := map[string]interface{}{}
		if sni != nil {
			tlsMap["server_name"] = sni
		}
		if fp != nil {
			tlsMap["fingerprint"] = fp
		}
		if alpn, ok := configJSON["alpn"]; ok {
			tlsMap["alpn"] = alpn
		}
		if ai, ok := configJSON["allow_insecure"]; ok {
			tlsMap["allow_insecure"] = ai
		}
		if len(tlsMap) > 0 {
			configJSON["tls"] = tlsMap
		}
	}
	if security == "reality" {
		realityMap := map[string]interface{}{}
		if pbk, ok := configJSON["pbk"]; ok {
			realityMap["public_key"] = pbk
		}
		if sid, ok := configJSON["sid"]; ok {
			realityMap["short_ids"] = []string{sid.(string)}
		}
		if spx, ok := configJSON["spx"]; ok {
			realityMap["spider_x"] = spx
		}
		if sni != nil {
			realityMap["server_name"] = sni
		}
		if fp != nil {
			realityMap["fingerprint"] = fp
		}
		configJSON["reality"] = realityMap
	}
	if p, ok := configJSON["path"]; ok && p != nil && p != "" {
		configJSON["ws_path"] = p
	}
	if h, ok := configJSON["host"]; ok && h != nil && h != "" {
		configJSON["ws_host"] = h
	}

	uuidField := ""
	passwordField := ""
	switch protocol {
	case "vless":
		uuidField = user
	case "trojan", "hysteria2":
		passwordField = user
	case "tuic":
		uuidField = user
		if pwd, ok := u.User.Password(); ok && pwd != "" {
			passwordField = pwd
		} else if pwd := params.Get("password"); pwd != "" {
			passwordField = pwd
		}
	}

	return &URINodePreview{
		Name:          name,
		ProtocolType:  protocol,
		TransportType: transport,
		SecurityType:  security,
		Host:          host,
		Port:          port,
		UUID:          uuidField,
		Password:      passwordField,
		ConfigJSON:    configJSON,
		Valid:         valid,
		Warning:       warning,
	}, nil
}

func parseSS(uri string) (*URINodePreview, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var method, password, host, name string
	var port int

	if u.User != nil {
		userInfo := u.User.String()
		if decoded, err := base64Decode(userInfo); err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				method = parts[0]
				password = parts[1]
			} else {
				method = string(decoded)
			}
		} else {
			method = u.User.Username()
			password, _ = u.User.Password()
		}
		host = u.Hostname()
		portStr := u.Port()
		if portStr != "" {
			port, _ = strconv.Atoi(portStr)
		}
		if port == 0 {
			port = 8388
		}
		name = u.Fragment
	} else {
		if data, err := base64Decode(strings.TrimPrefix(uri, "ss://")); err == nil {
			parts := strings.SplitN(string(data), "@", 2)
			if len(parts) == 2 {
				userInfo := parts[0]
				up := strings.SplitN(userInfo, ":", 2)
				if len(up) == 2 {
					method = up[0]
					password = up[1]
				}
				hostPort := parts[1]
				if idx := strings.Index(hostPort, "#"); idx > 0 {
					name, _ = url.QueryUnescape(hostPort[idx+1:])
					hostPort = hostPort[:idx]
				}
				if idx := strings.Index(hostPort, "?"); idx > 0 {
					hostPort = hostPort[:idx]
				}
				h, p, err := net.SplitHostPort(hostPort)
				if err == nil {
					host = h
					port, _ = strconv.Atoi(p)
				}
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	if port == 0 {
		port = 8388
	}

	security := "none"
	params := u.Query()
	if tls := params.Get("security"); tls != "" {
		security = tls
	}

	plugin := params.Get("plugin")
	transport := "tcp"
	if plugin != "" && strings.Contains(plugin, "obfs") {
		if strings.Contains(plugin, "ws") {
			transport = "ws"
		}
	}

	configJSON := map[string]interface{}{
		"method":   method,
		"password": password,
	}
	for _, key := range []string{"plugin", "obfs", "obfs-host"} {
		if v := params.Get(key); v != "" {
			configJSON[key] = v
		}
	}

	warning := ""
	if method == "" || password == "" {
		warning = "missing method or password"
	}

	return &URINodePreview{
		Name:          name,
		ProtocolType:  "ss",
		TransportType: transport,
		SecurityType:  security,
		Host:          host,
		Port:          port,
		Password:      password,
		ConfigJSON:    configJSON,
		Valid:         host != "" && method != "" && password != "",
		Warning:       warning,
	}, nil
}

// parseSOCKS5 解析 socks5://user:pass@host:port URI。
// 支持无认证（无 user info）和用户名/密码认证两种模式。
// ProtocolType 返回 "socks5"（由 chain_uri.go 归一化为 nodespec.ProtocolSOCKS5="socks"）。
func parseSOCKS5(uri string) (*URINodePreview, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	portStr := u.Port()
	port := 1080
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	username := ""
	password := ""
	if u.User != nil {
		username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}

	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	// socks5+TLS 支持（security=tls 参数）
	security := "none"
	transport := "tcp"
	params := u.Query()
	if s := params.Get("security"); s != "" {
		security = s
	}
	if t := params.Get("type"); t != "" {
		transport = t
	}

	configJSON := map[string]interface{}{
		"username": username,
		"password": password,
	}
	if sni := params.Get("sni"); sni != "" {
		configJSON["sni"] = sni
	}
	if fp := params.Get("fp"); fp != "" {
		configJSON["fp"] = fp
	}
	if alpn := params.Get("alpn"); alpn != "" {
		configJSON["alpn"] = parseCSV(alpn)
	}
	if ai := params.Get("allowInsecure"); ai != "" || params.Get("insecure") != "" {
		configJSON["allow_insecure"] = ai == "1" || strings.ToLower(ai) == "true" || strings.ToLower(params.Get("insecure")) == "true"
	}

	// TLS 结构化（与 parseStandardURI 对齐）
	if security == "tls" {
		tlsMap := map[string]interface{}{}
		if sni, ok := configJSON["sni"]; ok {
			tlsMap["server_name"] = sni
		}
		if fp, ok := configJSON["fp"]; ok {
			tlsMap["fingerprint"] = fp
		}
		if alpn, ok := configJSON["alpn"]; ok {
			tlsMap["alpn"] = alpn
		}
		if ai, ok := configJSON["allow_insecure"]; ok {
			tlsMap["allow_insecure"] = ai
		}
		if len(tlsMap) > 0 {
			configJSON["tls"] = tlsMap
		}
	}

	warning := ""
	if host == "" {
		warning = "missing host"
	}

	return &URINodePreview{
		Name:          name,
		ProtocolType:  "socks5",
		TransportType: transport,
		SecurityType:  security,
		Host:          host,
		Port:          port,
		Password:      password,
		ConfigJSON:    configJSON,
		Valid:         host != "",
		Warning:       warning,
	}, nil
}

// parseHTTPProxy 解析 http://user:pass@host:port 或 https://user:pass@host:port URI。
// 仅当含 user info 时视为 HTTP 代理 URI（由调用方判定）。
// ProtocolType 返回 "http"（https:// 时 SecurityType="tls"）。
func parseHTTPProxy(uri string, u *url.URL) (*URINodePreview, error) {
	host := u.Hostname()
	portStr := u.Port()
	port := 8080
	if u.Scheme == "https" {
		port = 443
	}
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	username := ""
	password := ""
	if u.User != nil {
		username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}

	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	security := "none"
	if u.Scheme == "https" {
		security = "tls"
	}
	transport := "tcp"
	params := u.Query()
	if s := params.Get("security"); s != "" {
		security = s
	}
	if t := params.Get("type"); t != "" {
		transport = t
	}

	configJSON := map[string]interface{}{
		"username": username,
		"password": password,
	}
	if sni := params.Get("sni"); sni != "" {
		configJSON["sni"] = sni
	}
	if fp := params.Get("fp"); fp != "" {
		configJSON["fp"] = fp
	}
	if alpn := params.Get("alpn"); alpn != "" {
		configJSON["alpn"] = parseCSV(alpn)
	}

	if security == "tls" {
		tlsMap := map[string]interface{}{}
		if sni, ok := configJSON["sni"]; ok {
			tlsMap["server_name"] = sni
		}
		if fp, ok := configJSON["fp"]; ok {
			tlsMap["fingerprint"] = fp
		}
		if alpn, ok := configJSON["alpn"]; ok {
			tlsMap["alpn"] = alpn
		}
		if len(tlsMap) > 0 {
			configJSON["tls"] = tlsMap
		}
	}

	warning := ""
	if host == "" {
		warning = "missing host"
	}

	return &URINodePreview{
		Name:          name,
		ProtocolType:  "http",
		TransportType: transport,
		SecurityType:  security,
		Host:          host,
		Port:          port,
		Password:      password,
		ConfigJSON:    configJSON,
		Valid:         host != "",
		Warning:       warning,
	}, nil
}

func base64Decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.StdEncoding.DecodeString(s)
}

func parsePort(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		if p, err := strconv.Atoi(val); err == nil {
			return p
		}
	}
	return 0
}

func parseCSV(s string) []string {
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
