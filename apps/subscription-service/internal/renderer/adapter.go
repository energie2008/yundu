package renderer

import (
	"strconv"
	"strings"

	"github.com/airport-panel/subscription-service/internal/model"
	"github.com/airport-panel/subscription/nodespec"
)

func NodeInfoToNodeSpec(n *model.NodeInfo) nodespec.NodeSpec {
	// 订阅地址标准化（零 SSH 架构）：
	// address 字段统一表示客户端连接目标（优选IP/域名），订阅直接输出，不覆盖。
	// cdn_address 仅作为 CDN 路由域名（SNI/Host），不覆盖客户端连接地址。
	// 端口映射：cdn_port/client_port 覆盖 DB Port（CDN/Tunnel 节点客户端口=443）
	address := n.Address
	port := n.Port
	securityType := n.SecurityType
	if n.ConfigJSON != nil {
		// 端口映射：cdn_port 优先，client_port 次之（隧道节点用 client_port 字段）
		for _, portKey := range []string{"cdn_port", "client_port"} {
			if portVal, ok := n.ConfigJSON[portKey]; ok {
				switch v := portVal.(type) {
				case float64:
					if v > 0 {
						port = int(v)
					}
				case int:
					if v > 0 {
						port = v
					}
				}
			}
		}
		// 安全类型覆盖：CDN/Tunnel 节点 DB 列为 "none"（服务端无 TLS），
		// 但客户端需要 TLS（CDN 边缘强制 TLS）。config_json.security_type 优先。
		if st, ok := n.ConfigJSON["security_type"].(string); ok && st != "" {
			securityType = st
		}
	}

	spec := nodespec.NodeSpec{
		ID:          n.ID.String(),
		Code:        n.Code,
		Name:        n.Name,
		Address:     address,
		Port:        port,
		TrafficRate: 1.0,
		AllowUDP:    true,
		Group:       n.GroupName,
	}

	if n.Multiplier > 0 {
		spec.TrafficRate = n.Multiplier
	}

	spec.Protocol = parseProtocol(n.ProtocolType)
	spec.Security = parseSecurity(securityType)
	spec.Transport = parseTransport(n.ProtocolType, n.TransportType, n.ConfigJSON, n.Path, n.HostHeader)

	if spec.Security == nodespec.SecurityTLS {
		spec.TLS = parseTLS(spec.Protocol, n.TransportType, n.ConfigJSON, n.SNI, n.ALPN)
	} else if spec.Security == nodespec.SecurityReality {
		spec.Reality = parseReality(n.ConfigJSON, n.SNI, n.ALPN)
	}

	// 零SSH修复：当 security=reality 且 transport=xhttp 时，自动同步 host=sni
	// 与服务端 xray_config.go:471-473 逻辑保持一致，避免客户端 Host header 与 SNI 不匹配
	// 导致 REALITY 握手失败。同时处理 split mode 下行 downloadSettings 的 host 覆盖。
	if spec.Transport.Type == nodespec.TransportXHTTP && spec.Transport.XHTTP != nil {
		// 上行：security=reality 时，host=sni
		if spec.Security == nodespec.SecurityReality && spec.Reality != nil && spec.Reality.SNI != "" {
			spec.Transport.XHTTP.Host = spec.Reality.SNI
		}
		// 下行（split mode）：downloadSettings.security=reality 时，downloadSettings.host=downloadSettings.reality.SNI
		if ds := spec.Transport.XHTTP.DownloadSettings; ds != nil {
			if ds.Security == nodespec.SecurityReality && ds.Reality != nil && ds.Reality.SNI != "" {
				ds.Host = ds.Reality.SNI
			}
		}
	}

	spec.Credentials = parseCredentials(string(spec.Protocol), n.ConfigJSON, n.Flow)

	return spec
}

func NodeInfosToNodeSpecs(nodes []*model.NodeInfo) []nodespec.NodeSpec {
	specs := make([]nodespec.NodeSpec, 0, len(nodes))
	for _, n := range nodes {
		if n == nil || !n.IsEnabled || !n.IsVisible {
			continue
		}
		spec := NodeInfoToNodeSpec(n)
		specs = append(specs, spec)
	}
	return specs
}

func parseProtocol(p string) nodespec.Protocol {
	switch strings.ToLower(p) {
	case "vless":
		return nodespec.ProtocolVLESS
	case "vmess":
		return nodespec.ProtocolVMess
	case "trojan":
		return nodespec.ProtocolTrojan
	case "shadowsocks", "ss":
		return nodespec.ProtocolShadowsocks
	case "hysteria2", "hy2":
		return nodespec.ProtocolHysteria2
	case "tuic":
		return nodespec.ProtocolTUIC
	case "anytls":
		return nodespec.ProtocolAnyTLS
	case "socks", "socks5":
		return nodespec.ProtocolSOCKS5
	case "http":
		return nodespec.ProtocolHTTP
	default:
		return nodespec.Protocol(strings.ToLower(p))
	}
}

func parseSecurity(s string) nodespec.Security {
	switch strings.ToLower(s) {
	case "tls":
		return nodespec.SecurityTLS
	case "reality":
		return nodespec.SecurityReality
	default:
		return nodespec.SecurityNone
	}
}

func parseTransport(protoStr, t string, cfg map[string]interface{}, dbPath, dbHost string) nodespec.TransportConfig {
	tc := nodespec.TransportConfig{}
	switch strings.ToLower(t) {
	case "ws":
		tc.Type = nodespec.TransportWS
		tc.WS = parseWSConfig(cfg, dbPath, dbHost)
	case "grpc":
		tc.Type = nodespec.TransportGRPC
		tc.GRPC = parseGRPCConfig(cfg)
	case "xhttp":
		tc.Type = nodespec.TransportXHTTP
		tc.XHTTP = parseXHTTPConfig(cfg, dbPath, dbHost)
		// 解析 xhttp.extra.xmux（XHTTP 专用多路复用，对应 Xray xhttpSettings.extra.xmux）
		if mux := parseXMuxConfig(cfg); mux != nil {
			tc.Mux = mux
		}
	case "quic":
		tc.Type = nodespec.TransportQUIC
		tc.QUIC = parseQUICConfig(cfg)
	case "kcp":
		tc.Type = nodespec.TransportKCP
	case "http2":
		tc.Type = nodespec.TransportHTTP2
	case "httpupgrade":
		tc.Type = nodespec.TransportHTTPUpgrade
		tc.HTTPUpgrade = parseHTTPUpgradeConfig(cfg, dbPath, dbHost)
	default:
		tc.Type = nodespec.TransportTCP
	}
	proto := strings.ToLower(protoStr)
	if (proto == "hysteria2" || proto == "hy2" || proto == "tuic") && tc.Type == nodespec.TransportTCP {
		tc.Type = nodespec.TransportQUIC
		tc.QUIC = parseQUICConfig(cfg)
	}
	// 端口跳跃：从 config_json.port_hopping 解析，填充 TransportConfig.PortHopping
	// 用于客户端订阅 URI 的 mport 参数渲染（见 renderer/uri.go）
	if proto == "hysteria2" || proto == "hy2" || proto == "tuic" {
		if ph, ok := cfg["port_hopping"].(map[string]interface{}); ok {
			phCfg := &nodespec.PortHoppingConfig{}
			if e, ok := ph["enabled"].(bool); ok {
				phCfg.Enabled = e
			}
			if pr, ok := ph["port_range"].(string); ok {
				phCfg.PortRange = pr
			}
			if iv, ok := ph["interval"].(float64); ok {
				phCfg.Interval = int(iv)
			}
			if phCfg.Enabled && phCfg.PortRange != "" {
				tc.PortHopping = phCfg
			}
		}
	}
	return tc
}

func parseQUICConfig(cfg map[string]interface{}) *nodespec.QUICConfig {
	qc := &nodespec.QUICConfig{}
	if obfs := getString(cfg, "obfs", "obfs_type"); obfs != "" {
		qc.Security = obfs
	}
	if obfsPwd := getString(cfg, "obfs_password", "obfs-password"); obfsPwd != "" {
		qc.Key = obfsPwd
	}
	return qc
}

func getNestedMap(cfg map[string]interface{}, key string) map[string]interface{} {
	if v, ok := cfg[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func getNestedString(cfg map[string]interface{}, parentKey string, keys ...string) string {
	parent := getNestedMap(cfg, parentKey)
	if parent == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := parent[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseWSConfig(cfg map[string]interface{}, dbPath, dbHost string) *nodespec.WSConfig {
	ws := &nodespec.WSConfig{}
	nestedPath := getNestedString(cfg, "ws", "path", "ws_path")
	nestedHost := getNestedString(cfg, "ws", "host", "ws_host")
	ws.Path = firstNonEmpty(dbPath, nestedPath)
	if ws.Path == "" {
		ws.Path = "/"
	}
	ws.Host = firstNonEmpty(dbHost, nestedHost)
	return ws
}

func parseHTTPUpgradeConfig(cfg map[string]interface{}, dbPath, dbHost string) *nodespec.HTTPUpgradeConfig {
	hu := &nodespec.HTTPUpgradeConfig{}
	nestedPath := getNestedString(cfg, "httpupgrade", "path")
	nestedHost := getNestedString(cfg, "httpupgrade", "host")
	nestedMode := getNestedString(cfg, "httpupgrade", "mode")
	hu.Path = firstNonEmpty(dbPath, nestedPath)
	if hu.Path == "" {
		hu.Path = "/"
	}
	hu.Host = firstNonEmpty(dbHost, nestedHost)
	hu.Mode = nestedMode
	return hu
}

// getStringOrNum 从 map 中获取字符串或数字值，统一返回字符串
// 用于 XMUX 字段，这些字段可能是数字（16, 256）或范围字符串（"16-32", "1800-3600"）
func getStringOrNum(cfg map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			switch val := v.(type) {
			case string:
				if val != "" {
					return val
				}
			case float64:
				// JSON 数字在 Go 中解析为 float64
				if val == float64(int(val)) {
					return strconv.Itoa(int(val))
				}
				return strconv.FormatFloat(val, 'f', -1, 64)
			case int:
				return strconv.Itoa(val)
			case int64:
				return strconv.FormatInt(val, 10)
			}
		}
	}
	return ""
}

// parseXMuxConfig 解析 XHTTP 的 XMUX 配置（xhttp.extra.xmux）
// 对应 Xray xhttpSettings.extra.xmux 结构体
// 支持 maxConcurrency/cMaxReuseTimes/hMaxRequestTimes/hMaxReusableSecs 范围值（如 "16-32"）
// 以及 maxConnection 数字字段
func parseXMuxConfig(cfg map[string]interface{}) *nodespec.MuxConfig {
	xhttpMap := getNestedMap(cfg, "xhttp")
	if xhttpMap == nil {
		return nil
	}
	extraMap := getNestedMap(xhttpMap, "extra")
	if extraMap == nil {
		return nil
	}
	xmuxMap := getNestedMap(extraMap, "xmux")
	if xmuxMap == nil {
		return nil
	}
	mux := &nodespec.MuxConfig{
		Enabled:  true,
		Protocol: nodespec.MuxProtocolXmux,
	}
	// 范围值字段（可能是数字或字符串，如 16、"16-32"、"1800-3600"）
	mux.MaxConcurrency = getStringOrNum(xmuxMap, "maxConcurrency", "max_concurrency")
	mux.CMaxReuseTimes = getStringOrNum(xmuxMap, "cMaxReuseTimes", "c_max_reuse_times")
	mux.HMaxRequestTimes = getStringOrNum(xmuxMap, "hMaxRequestTimes", "h_max_request_times")
	mux.HMaxReusableSecs = getStringOrNum(xmuxMap, "hMaxReusableSecs", "h_max_reusable_secs")
	// 数字字段（B21 修复：同时支持 maxConnections 和 maxConnection 两种键名）
	if v := getInt(xmuxMap, "maxConnections", "maxConnection", "max_connections"); v > 0 {
		mux.MaxConnections = v
	}
	// 如果没有任何 XMUX 专用字段，返回 nil
	if mux.MaxConcurrency == "" && mux.CMaxReuseTimes == "" &&
		mux.HMaxRequestTimes == "" && mux.HMaxReusableSecs == "" &&
		mux.MaxConnections == 0 {
		return nil
	}
	return mux
}

func parseXHTTPConfig(cfg map[string]interface{}, dbPath, dbHost string) *nodespec.XHTTPConfig {
	xhttp := &nodespec.XHTTPConfig{}
	nestedPath := getNestedString(cfg, "xhttp", "path", "xhttp_path")
	nestedHost := getNestedString(cfg, "xhttp", "host", "xhttp_host")
	nestedMode := getNestedString(cfg, "xhttp", "mode", "xhttp_mode")
	xhttp.Path = firstNonEmpty(dbPath, nestedPath)
	if xhttp.Path == "" {
		xhttp.Path = "/"
	}
	xhttp.Host = firstNonEmpty(dbHost, nestedHost)
	xhttp.Mode = nestedMode
	if xhttp.Mode == "" {
		if mode := getString(cfg, "xhttp_mode", "mode"); mode != "" {
			xhttp.Mode = mode
		}
	}
	// 解析 download_settings（XHTTP split mode）：
	// 优先从正确路径 xhttp.extra.downloadSettings 读取，兼容顶层 download_settings 老数据
	var dsMap map[string]interface{}
	xhttpMap := getNestedMap(cfg, "xhttp")
	if xhttpMap != nil {
		extraMap := getNestedMap(xhttpMap, "extra")
		if extraMap != nil {
			if ds := getNestedMap(extraMap, "downloadSettings"); ds != nil {
				dsMap = ds
			}
		}
	}
	if dsMap == nil {
		dsMap = getNestedMap(cfg, "download_settings")
	}
	if dsMap != nil {
		ds := &nodespec.XHTTPDownloadConfig{}
		ds.Address = getString(dsMap, "address", "addr")
		ds.AddressIPv6 = getString(dsMap, "address_ipv6", "addressIPv6")
		if port := getInt(dsMap, "port"); port > 0 {
			ds.Port = port
		}
		if net := getString(dsMap, "network"); net != "" {
			ds.Network = nodespec.Transport(net)
		} else {
			ds.Network = nodespec.TransportXHTTP
		}
		if sec := getString(dsMap, "security"); sec != "" {
			ds.Security = nodespec.Security(sec)
		} else {
			ds.Security = nodespec.SecurityTLS
		}
		ds.Path = getString(dsMap, "path")
		ds.Host = getString(dsMap, "host")
		ds.Mode = getString(dsMap, "mode")
		if ds.Mode == "" {
			ds.Mode = "stream-down"
		}
		// 从 xhttpSettings 子对象读取 path/host/mode（Xray JSON 风格）
		if xhSettings := getNestedMap(dsMap, "xhttpSettings"); xhSettings != nil {
			if p := getString(xhSettings, "path"); p != "" {
				ds.Path = p
			}
			if h := getString(xhSettings, "host"); h != "" {
				ds.Host = h
			}
			if m := getString(xhSettings, "mode"); m != "" {
				ds.Mode = m
			}
		}
		// REALITY 子配置：支持 reality 和 realitySettings 两种键
		var realityMap map[string]interface{}
		if r := getNestedMap(dsMap, "reality"); r != nil {
			realityMap = r
		} else if r := getNestedMap(dsMap, "realitySettings"); r != nil {
			realityMap = r
		}
		if ds.Security == nodespec.SecurityReality || realityMap != nil {
			ds.Reality = &nodespec.RealityConfig{}
			if realityMap != nil {
				ds.Reality.PublicKey = getString(realityMap, "public_key", "publicKey", "pbk")
				ds.Reality.ShortID = getString(realityMap, "short_id", "shortId", "sid")
				ds.Reality.SNI = getString(realityMap, "server_name", "serverName", "sni")
				ds.Reality.Fingerprint = getString(realityMap, "fingerprint", "fp")
			}
			// 顶层回退
			if ds.Reality.PublicKey == "" {
				ds.Reality.PublicKey = getString(dsMap, "public_key", "publicKey", "pbk")
			}
			if ds.Reality.ShortID == "" {
				ds.Reality.ShortID = getString(dsMap, "short_id", "shortId", "sid")
			}
			if ds.Reality.SNI == "" {
				ds.Reality.SNI = getString(dsMap, "server_name", "serverName", "sni")
			}
			if ds.Reality.Fingerprint == "" {
				ds.Reality.Fingerprint = getString(dsMap, "fingerprint", "fp")
			}
			if ds.Reality.Fingerprint == "" {
				ds.Reality.Fingerprint = "chrome"
			}
		}
		// TLS 子配置：支持 tls 和 tlsSettings 两种键
		var tlsMap map[string]interface{}
		if t := getNestedMap(dsMap, "tls"); t != nil {
			tlsMap = t
		} else if t := getNestedMap(dsMap, "tlsSettings"); t != nil {
			tlsMap = t
		}
		if ds.Security == nodespec.SecurityTLS || tlsMap != nil {
			ds.TLS = &nodespec.TLSConfig{}
			if tlsMap != nil {
				ds.TLS.SNI = getString(tlsMap, "server_name", "serverName", "sni")
				ds.TLS.Fingerprint = getString(tlsMap, "fingerprint", "fp")
				ds.TLS.ALPN = getStrings(tlsMap, "alpn")
				ds.TLS.AllowInsecure = getBool(tlsMap, "allow_insecure", "allowInsecure")
			}
			if ds.TLS.SNI == "" {
				ds.TLS.SNI = getString(dsMap, "sni", "server_name", "serverName")
			}
			if ds.TLS.Fingerprint == "" {
				ds.TLS.Fingerprint = getString(dsMap, "fingerprint", "fp")
			}
			if ds.TLS.Fingerprint == "" {
				ds.TLS.Fingerprint = "chrome"
			}
		}
		xhttp.DownloadSettings = ds
	}
	return xhttp
}

func parseGRPCConfig(cfg map[string]interface{}) *nodespec.GRPCConfig {
	grpc := &nodespec.GRPCConfig{}
	svc := getNestedString(cfg, "grpc", "service_name", "serviceName", "grpc_service_name")
	if svc == "" {
		svc = getString(cfg, "grpc_service_name", "serviceName", "service_name")
	}
	if svc == "" {
		svc = getNestedString(cfg, "grpc", "serviceName")
	}
	grpc.ServiceName = svc
	return grpc
}

func parseTLS(proto nodespec.Protocol, transportType string, cfg map[string]interface{}, dbSNI string, dbALPN []string) *nodespec.TLSConfig {
	tls := &nodespec.TLSConfig{}
	nestedSNI := getNestedString(cfg, "tls", "server_name", "servername", "sni")
	// 隧道/CDN 节点的 sni 可能存储在 config_json 顶层（非嵌套在 tls 子 map 中）
	topSNI := getString(cfg, "sni")
	tls.SNI = firstNonEmpty(dbSNI, nestedSNI, topSNI)
	fp := getNestedString(cfg, "tls", "fingerprint", "fp", "client_fingerprint")
	if fp == "" {
		fp = getString(cfg, "client_fingerprint", "fingerprint", "fp", "utls_fingerprint")
	}
	if fp == "" {
		fp = "chrome"
	}
	tls.Fingerprint = fp

	tlsMap := getNestedMap(cfg, "tls")
	if tlsMap != nil {
		tls.AllowInsecure = getBool(tlsMap, "allow_insecure", "skip_cert_verify", "allowInsecure", "allow_insecure")
	}
	if !tls.AllowInsecure {
		tls.AllowInsecure = getBool(cfg, "allow_insecure", "skip_cert_verify", "allowInsecure")
	}

	// PinSHA256：直连 IP 节点的自签名证书指纹（Xray v26.2.4+ 移除 allowInsecure 后必需）
	tls.PinSHA256 = getNestedString(cfg, "tls", "pin_sha256", "pinned_peer_cert_sha256")
	if tls.PinSHA256 == "" {
		tls.PinSHA256 = getString(cfg, "pin_sha256", "pinned_peer_cert_sha256")
	}

	if len(dbALPN) > 0 {
		tls.ALPN = dbALPN
	} else {
		nestedALPN := getNestedStrings(cfg, "tls", "alpn")
		if len(nestedALPN) > 0 {
			tls.ALPN = nestedALPN
		} else {
			switch proto {
			case nodespec.ProtocolHysteria2, nodespec.ProtocolTUIC:
				tls.ALPN = []string{"h3"}
			default:
				// WS/HTTPUpgrade 传输依赖 HTTP/1.1 Upgrade 机制，
				// ALPN 包含 h2 会导致 CF CDN 协商 HTTP/2 后 WS Upgrade 失败（返回 400）
				tt := strings.ToLower(transportType)
				if tt == "ws" || tt == "httpupgrade" {
					tls.ALPN = []string{"http/1.1"}
				} else {
					tls.ALPN = []string{"h2", "http/1.1"}
				}
			}
		}
	}

	return tls
}

func parseReality(cfg map[string]interface{}, dbSNI string, dbALPN []string) *nodespec.RealityConfig {
	reality := &nodespec.RealityConfig{}
	realityMap := getNestedMap(cfg, "reality")
	// 兼容前端写入 reality_settings 嵌套路径（与 node-service buildRealityConfig 三级回退对齐）
	if realityMap == nil {
		realityMap = getNestedMap(cfg, "reality_settings")
	}
	if realityMap != nil {
		reality.PublicKey = getString(realityMap, "public_key", "publicKey", "pbk")
		reality.ShortID = getString(realityMap, "short_id", "shortId", "sid")
		reality.SpiderX = getString(realityMap, "spider_x", "spiderX", "spx")
		sni := getString(realityMap, "server_name", "servername", "sni")
		reality.SNI = firstNonEmpty(dbSNI, sni)
		if alpn := getStrings(realityMap, "alpn"); len(alpn) > 0 {
			reality.ALPN = alpn
		}
	}
	// 顶层键回退（NormalizeNodeConfigJSON 拍平后 reality_settings.public_key → public_key）
	if reality.PublicKey == "" {
		reality.PublicKey = getString(cfg, "reality_public_key", "public_key", "publicKey", "pbk")
	}
	if reality.ShortID == "" {
		reality.ShortID = getString(cfg, "reality_short_id", "short_id", "shortId", "sid")
	}
	if reality.SpiderX == "" {
		reality.SpiderX = getString(cfg, "spider_x", "spiderX", "spx")
	}
	if reality.SNI == "" {
		sni := getString(cfg, "server_name", "servername", "sni")
		reality.SNI = firstNonEmpty(dbSNI, sni, "rust-lang.org")
	}
	fp := getString(cfg, "client_fingerprint", "fingerprint", "fp")
	if fp == "" {
		fp = getNestedString(cfg, "reality", "fingerprint", "fp", "client_fingerprint")
	}
	if fp == "" {
		fp = getNestedString(cfg, "reality_settings", "fingerprint", "fp", "client_fingerprint")
	}
	if fp == "" {
		fp = "chrome"
	}
	reality.Fingerprint = fp
	if len(dbALPN) > 0 {
		reality.ALPN = dbALPN
	} else if len(reality.ALPN) == 0 {
		reality.ALPN = []string{"h2", "http/1.1"}
	}
	return reality
}

func parseCredentials(proto string, cfg map[string]interface{}, dbFlow string) interface{} {
	switch strings.ToLower(proto) {
	case "vless":
		uuid := getString(cfg, "uuid")
		flow := dbFlow
		if flow == "" {
			flow = getString(cfg, "flow")
		}
		enc := getString(cfg, "encryption", "decryption")
		return nodespec.VLESSCredentials{
			UUID:       uuid,
			Flow:       nodespec.FlowControl(flow),
			Encryption: enc,
		}
	case "vmess":
		alterID := 0
		if aid := getInt(cfg, "alterId", "aid", "alter_id"); aid > 0 {
			alterID = aid
		}
		return nodespec.VMessCredentials{
			UUID:    getString(cfg, "uuid"),
			AlterID: alterID,
		}
	case "trojan":
		return nodespec.TrojanCredentials{
			Password: getString(cfg, "password"),
		}
	case "shadowsocks", "ss":
		method := getString(cfg, "cipher", "method")
		if method == "" {
			method = getNestedString(cfg, "ss2022", "method")
		}
		password := getString(cfg, "password")
		if password == "" {
			password = getNestedString(cfg, "ss2022", "password")
		}
		return nodespec.ShadowsocksCredentials{
			Password: password,
			Method:   method,
		}
	case "hysteria2", "hy2":
		upMbps := getInt(cfg, "up_mbps", "upmbps", "up")
		downMbps := getInt(cfg, "down_mbps", "downmbps", "down")
		if upMbps == 0 {
			upMbps = 500
		}
		if downMbps == 0 {
			downMbps = 500
		}
		return nodespec.Hysteria2Credentials{
			Password: getString(cfg, "password"),
			UpMbps:   upMbps,
			DownMbps: downMbps,
		}
	case "tuic":
		return nodespec.TUICCredentials{
			UUID:     getString(cfg, "uuid"),
			Password: getString(cfg, "password"),
		}
	case "anytls":
		return nodespec.AnyTLSCredentials{
			Password: getString(cfg, "password"),
		}
	case "socks", "socks5":
		// SOCKS5 代理：username/password 组合认证（对齐 Xboard General::buildSocks）
		return nodespec.SOCKS5Credentials{
			Username: getString(cfg, "username", "user"),
			Password: getString(cfg, "password", "pass"),
		}
	case "http":
		// HTTP/HTTPS 代理：username/password 组合认证（对齐 Xboard General::buildHttp）
		return nodespec.HTTPCredentials{
			Username: getString(cfg, "username", "user"),
			Password: getString(cfg, "password", "pass"),
		}
	default:
		return map[string]interface{}{}
	}
}

func getBool(cfg map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			switch val := v.(type) {
			case bool:
				return val
			case string:
				return val == "1" || strings.EqualFold(val, "true")
			case int:
				return val == 1
			case float64:
				return val == 1
			}
		}
	}
	return false
}

func getInt(cfg map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			switch val := v.(type) {
			case int:
				return val
			case float64:
				return int(val)
			case string:
				return 0
			}
		}
	}
	return 0
}

func getString(cfg map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			switch val := v.(type) {
			case string:
				if val != "" {
					return val
				}
			}
		}
	}
	return ""
}

func getStrings(cfg map[string]interface{}, keys ...string) []string {
	for _, k := range keys {
		if v, ok := cfg[k]; ok {
			switch val := v.(type) {
			case []interface{}:
				var result []string
				for _, item := range val {
					if s, ok := item.(string); ok && s != "" {
						result = append(result, s)
					}
				}
				if len(result) > 0 {
					return result
				}
			case string:
				if val != "" {
					parts := strings.Split(val, ",")
					var result []string
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" {
							result = append(result, p)
						}
					}
					if len(result) > 0 {
						return result
					}
				}
			}
		}
	}
	return nil
}

func getNestedStrings(cfg map[string]interface{}, parentKey string, keys ...string) []string {
	parent := getNestedMap(cfg, parentKey)
	if parent == nil {
		return nil
	}
	return getStrings(parent, keys...)
}
