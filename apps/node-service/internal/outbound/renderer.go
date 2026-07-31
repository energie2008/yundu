package outbound

import "strings"

// RenderOutbounds 把一组 outbound policies 渲染成 xray + sing-box 的 outbounds + routing rules JSON。
// 仅处理 is_enabled=true 的策略，按 priority 升序排列。
// 每种 policy_type 对应一个生成函数：
//   - direct: 直连出站
//   - warp: Cloudflare WARP 出站（socks/wireguard）
//   - socks5: SOCKS5 代理出站
//   - chain: 链式出站（本批简化为 socks5 形式）
//   - blackhole: 阻断出站
func RenderOutbounds(policies []*OutboundPolicy) (*ApplyAllResponse, error) {
	if len(policies) == 0 {
		return &ApplyAllResponse{
			Xray:    RenderedRuntime{Outbounds: []Map{}, RoutingRules: []Map{}},
			SingBox: RenderedRuntime{Outbounds: []Map{}, RoutingRules: []Map{}, Endpoints: []Map{}},
		}, nil
	}

	xrayOutbounds := make([]Map, 0, len(policies))
	singBoxOutbounds := make([]Map, 0, len(policies))
	singBoxEndpoints := make([]Map, 0) // sing-box 1.13+: wireguard 渲染为 endpoint 而非 outbound
	xrayRules := make([]Map, 0)
	singBoxRules := make([]Map, 0)

	// P3-F / LB-2 自动注入已移除（按需分配模式）：
	//   - warp policy 未配置 routing_rules 时不再自动注入流媒体规则
	//   - load_balance policy 未配置 routing_rules 时不再自动注入流媒体规则
	// 节点在 admin-web WARP 管理页显式启用 WARP 仅生成 warp outbound/endpoint，
	// 流量是否走 WARP 由用户显式配置的 routing_rules 决定；未配置则保持 direct。
	// load_balance policy 由 EnableLoadBalance API 显式创建，配合 deployment_service
	// 的 route.final=urltest 实现"全部流量走 WARP 池"的显式策略。

	for _, p := range policies {
		if p == nil || !p.IsEnabled {
			continue
		}

		// xray outbound
		if xb, err := renderXrayOutbound(p); err == nil {
			xrayOutbounds = append(xrayOutbounds, xb)
		}
		// sing-box: warp 有 private_key 时渲染为 endpoint（sing-box 1.13+ 移除了 wireguard outbound），
		// 其余类型渲染为 outbound。urltest/selector 可通过 tag 引用 endpoint。
		isWarpEndpoint := false
		if p.PolicyType == "warp" {
			if pk, _ := p.ConfigJSON["private_key"].(string); pk != "" {
				isWarpEndpoint = true
				if ep, err := renderSingBoxWarpEndpoint(p); err == nil {
					singBoxEndpoints = append(singBoxEndpoints, ep)
				}
			}
		}
		if !isWarpEndpoint {
			if sb, err := renderSingBoxOutbound(p); err == nil {
				singBoxOutbounds = append(singBoxOutbounds, sb)
			}
		}

		// routing rules（同形式：policy 配置里的 routing_rules 直接转译）
		// 按需模式：仅渲染用户显式配置的 routing_rules，不再自动注入默认流媒体规则。
		for _, rule := range p.RoutingRules {
			if r := renderXrayRoutingRule(p, rule); r != nil {
				xrayRules = append(xrayRules, r)
			}
			if r := renderSingBoxRoutingRule(p, rule); r != nil {
				singBoxRules = append(singBoxRules, r)
			}
		}
	}

	// 确保每个 runtime 都有默认 direct（freedom）出站
	if !hasTag(xrayOutbounds, "direct") {
		xrayOutbounds = append(xrayOutbounds, Map{
			"tag":      "direct",
			"protocol": "freedom",
		})
	}
	if !hasTag(singBoxOutbounds, "direct") {
		singBoxOutbounds = append(singBoxOutbounds, Map{
			"type": "direct",
			"tag":  "direct",
		})
	}
	// blackhole 兜底
	if !hasTag(xrayOutbounds, "block") {
		xrayOutbounds = append(xrayOutbounds, Map{
			"tag":      "block",
			"protocol": "blackhole",
		})
	}
	if !hasTag(singBoxOutbounds, "block") {
		singBoxOutbounds = append(singBoxOutbounds, Map{
			"type": "block",
			"tag":  "block",
		})
	}

	return &ApplyAllResponse{
		Xray: RenderedRuntime{
			Outbounds:    xrayOutbounds,
			RoutingRules: xrayRules,
		},
		SingBox: RenderedRuntime{
			Outbounds:    singBoxOutbounds,
			RoutingRules: singBoxRules,
			Endpoints:    singBoxEndpoints,
		},
	}, nil
}

func hasTag(outbounds []Map, tag string) bool {
	for _, o := range outbounds {
		if t, ok := o["tag"].(string); ok && t == tag {
			return true
		}
	}
	return false
}

func policyTag(p *OutboundPolicy) string {
	if t, ok := p.ConfigJSON["tag"].(string); ok && t != "" {
		return t
	}
	return p.PolicyType
}

// renderXrayOutbound 生成 xray 单个 outbound 配置
func renderXrayOutbound(p *OutboundPolicy) (Map, error) {
	tag := policyTag(p)
	switch p.PolicyType {
	case "direct":
		return Map{
			"tag":      tag,
			"protocol": "freedom",
			"settings": Map{"domainStrategy": "AsIs"},
		}, nil
	case "blackhole":
		return Map{
			"tag":      tag,
			"protocol": "blackhole",
			"settings": Map{"response": Map{"type": "none"}},
		}, nil
	case "socks5":
		server, _ := p.ConfigJSON["server"].(string)
		port := toInt(p.ConfigJSON["port"])
		return Map{
			"tag":      tag,
			"protocol": "socks",
			"settings": Map{
				"servers": []Map{{
					"address": server,
					"port":    port,
					"users":   buildSocksUsers(p.ConfigJSON),
				}},
			},
		}, nil
	case "warp":
		// WARP 在 xray 中表现为 socks 出站到本地 warp 客户端（wireproxy / warp-cli）
		// Fallback 端口选择：wireproxy 模式默认 40001，warp-cli 模式默认 40000
		server, _ := p.ConfigJSON["server"].(string)
		if server == "" {
			server = "127.0.0.1"
		}
		port := toInt(p.ConfigJSON["port"])
		if port == 0 {
			if wireproxyMode, _ := p.ConfigJSON["wireproxy"].(bool); wireproxyMode {
				port = 40001
			} else {
				port = 40000
			}
		}
		return Map{
			"tag":      tag,
			"protocol": "socks",
			"settings": Map{
				"servers": []Map{{
					"address": server,
					"port":    port,
				}},
			},
		}, nil
	case "chain":
		// 链式出站：本批简化为按 via 指定前置出站
		server, _ := p.ConfigJSON["server"].(string)
		port := toInt(p.ConfigJSON["port"])
		return Map{
			"tag":      tag,
			"protocol": "socks",
			"settings": Map{
				"servers": []Map{{
					"address": server,
					"port":    port,
					"users":   buildSocksUsers(p.ConfigJSON),
				}},
			},
		}, nil
	case "load_balance":
		// xray 无原生 load_balance，用第一个 outbound 作为兜底（xray 侧降级处理）
		// 真正的负载均衡由 sing-box 侧 load_balance outbound 处理
		// xray 内核节点不应使用 load_balance policy（detectRequiredKernel 会把 load_balance 节点路由到 sing-box）
		outbounds, _ := p.ConfigJSON["outbounds"].([]interface{})
		fallback := ""
		if len(outbounds) > 0 {
			if s, ok := outbounds[0].(string); ok {
				fallback = s
			}
		}
		return Map{
			"tag":      tag,
			"protocol": "freedom",
			"settings": Map{
				"domainStrategy": "AsIs",
			},
			"load_balance": Map{
				"fallback_outbound": fallback,
			},
		}, nil
	}
	return nil, ErrRenderFailed
}

// renderSingBoxOutbound 生成 sing-box 单个 outbound 配置
func renderSingBoxOutbound(p *OutboundPolicy) (Map, error) {
	tag := policyTag(p)
	switch p.PolicyType {
	case "direct":
		return Map{"type": "direct", "tag": tag}, nil
	case "blackhole":
		return Map{"type": "block", "tag": tag}, nil
	case "socks5":
		server, _ := p.ConfigJSON["server"].(string)
		port := toInt(p.ConfigJSON["port"])
		out := Map{
			"type":   "socks",
			"tag":    tag,
			"server": server,
			"port":   port,
		}
		if u, p := buildSingBoxSocksUser(p.ConfigJSON); u != "" {
			out["username"] = u
			out["password"] = p
		}
		return out, nil
	case "warp":
		// sing-box 1.13+ 移除了 wireguard outbound，有 private_key 的 warp 由
		// renderSingBoxWarpEndpoint 渲染为 endpoint。此处仅处理无 private_key 的
		// socks5 回退（wireproxy / warp-cli 本地代理）。
		//
		// Fallback 端口选择：
		//   - 默认 40000（warp-cli）
		//   - 若 config_json.wireproxy=true，用 40001（wireproxy，~4MB 轻量侧车）
		//   - config_json.port 可显式覆盖端口
		server, _ := p.ConfigJSON["server"].(string)
		if server == "" {
			server = "127.0.0.1"
		}
		port := toInt(p.ConfigJSON["port"])
		if port == 0 {
			// wireproxy 模式：默认 40001；warp-cli 模式：默认 40000
			if wireproxyMode, _ := p.ConfigJSON["wireproxy"].(bool); wireproxyMode {
				port = 40001
			} else {
				port = 40000
			}
		}
		return Map{
			"type":   "socks",
			"tag":    tag,
			"server": server,
			"port":   port,
		}, nil
	case "chain":
		server, _ := p.ConfigJSON["server"].(string)
		port := toInt(p.ConfigJSON["port"])
		out := Map{
			"type":   "socks",
			"tag":    tag,
			"server": server,
			"port":   port,
		}
		if u, pwd := buildSingBoxSocksUser(p.ConfigJSON); u != "" {
			out["username"] = u
			out["password"] = pwd
		}
		return out, nil
	case "load_balance":
		// sing-box 无 load_balance outbound 类型。改用 urltest（延迟自动选优 + 故障切换）。
		// urltest 会周期性测试所有子 outbound/endpoint 的延迟，自动选择最快的，
		// 并在某个 WARP 实例不可用时自动切换到其他实例。
		// 内部 policy_type 仍为 load_balance，renderer 负责转译为 sing-box 的 urltest。
		outbounds, _ := p.ConfigJSON["outbounds"].([]interface{})
		lb := Map{
			"type":      "urltest",
			"tag":       tag,
			"outbounds": outbounds,
		}
		// 健康检查 URL（默认 gstatic 204）
		checkURL, _ := p.ConfigJSON["check_url"].(string)
		if checkURL == "" {
			checkURL = "https://www.gstatic.com/generate_204"
		}
		lb["url"] = checkURL
		// 检查间隔（默认 3 分钟）
		interval, _ := p.ConfigJSON["check_interval"].(string)
		if interval == "" {
			interval = "3m"
		}
		lb["interval"] = interval
		return lb, nil
	}
	return nil, ErrRenderFailed
}

// renderSingBoxWarpEndpoint 生成 sing-box wireguard endpoint 配置（sing-box 1.13+）。
// sing-box 1.13.0 移除了 wireguard outbound，必须使用 endpoint 格式：
//   - 放在 config.endpoints[] 而非 config.outbounds[]
//   - 字段 address（非 local_address）
//   - peer 信息放在 peers[] 数组中（address/port/public_key/allowed_ips）
//
// endpoint 的 tag 可被 urltest/selector outbound 引用，实现负载均衡/故障切换。
func renderSingBoxWarpEndpoint(p *OutboundPolicy) (Map, error) {
	tag := policyTag(p)
	privateKey, _ := p.ConfigJSON["private_key"].(string)

	// 解析 endpoint
	serverAddr, _ := p.ConfigJSON["endpoint"].(string)
	serverPort := 2408
	if addr, port, ok := splitHostPort(serverAddr, 2408); ok {
		serverAddr = addr
		serverPort = port
	}
	if serverAddr == "" {
		serverAddr = "162.159.192.1"
	}

	// Cloudflare WARP 公钥（well-known）
	peerPublicKey, _ := p.ConfigJSON["public_key"].(string)
	if peerPublicKey == "" {
		peerPublicKey = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
	}

	// address（原 local_address）：WARP 分配的本地地址
	localAddr, _ := p.ConfigJSON["local_address"].(string)
	if localAddr == "" {
		localAddr = "172.16.0.2/32"
	}
	addresses := splitLocalAddresses(localAddr)

	mtu := toInt(p.ConfigJSON["mtu"])
	if mtu == 0 {
		mtu = 1280
	}

	// allowed_ips: 检测到 IPv6 地址时启用双栈路由
	allowedIPs := []string{"0.0.0.0/0"}
	for _, addr := range addresses {
		if strings.Contains(addr, ":") {
			allowedIPs = []string{"0.0.0.0/0", "::/0"}
			break
		}
	}

	return Map{
		"type":        "wireguard",
		"tag":         tag,
		"address":     addresses,
		"private_key": privateKey,
		"peers": []Map{{
			"address":     serverAddr,
			"port":        serverPort,
			"public_key":  peerPublicKey,
			"allowed_ips": allowedIPs,
		}},
		"mtu": mtu,
	}, nil
}

// renderXrayRoutingRule 把 policy 的 routing_rule 转为 xray 规则
func renderXrayRoutingRule(p *OutboundPolicy, rule Map) Map {
	out := Map{
		"type":        "field",
		"outboundTag": policyTag(p),
	}
	if domains, ok := rule["domains"].([]interface{}); ok && len(domains) > 0 {
		out["domain"] = domains
	}
	if ip, ok := rule["ip_cidr"].([]interface{}); ok && len(ip) > 0 {
		out["ip"] = ip
	}
	if geoip, ok := rule["geoip"].(string); ok && geoip != "" {
		out["ip"] = appendIfAbsent(out["ip"], "geoip:"+geoip)
	}
	if geosite, ok := rule["geosite"].(string); ok && geosite != "" {
		out["domain"] = appendIfAbsent(out["domain"], "geosite:"+geosite)
	}
	if pt, ok := rule["port"].(int); ok && pt > 0 {
		out["port"] = pt
	}
	return out
}

// renderSingBoxRoutingRule 把 policy 的 routing_rule 转为 sing-box 规则
func renderSingBoxRoutingRule(p *OutboundPolicy, rule Map) Map {
	out := Map{
		"outbound": policyTag(p),
	}
	if domains, ok := rule["domains"].([]interface{}); ok && len(domains) > 0 {
		out["domain"] = domains
	}
	if ip, ok := rule["ip_cidr"].([]interface{}); ok && len(ip) > 0 {
		out["ip_cidr"] = ip
	}
	if geoip, ok := rule["geoip"].(string); ok && geoip != "" {
		out["ip_cidr"] = appendIfAbsent(out["ip_cidr"], "geoip:"+geoip)
	}
	if geosite, ok := rule["geosite"].(string); ok && geosite != "" {
		out["domain"] = appendIfAbsent(out["domain"], "geosite:"+geosite)
	}
	if pt, ok := rule["port"].(int); ok && pt > 0 {
		out["port"] = pt
	}
	return out
}

func appendIfAbsent(current interface{}, item interface{}) []interface{} {
	switch v := current.(type) {
	case []interface{}:
		return append(v, item)
	case nil:
		return []interface{}{item}
	default:
		return []interface{}{current, item}
	}
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// splitHostPort 解析 "host:port" 格式字符串，返回 host 和 port。
// 解析失败时返回 ok=false。
func splitHostPort(addr string, defaultPort int) (string, int, bool) {
	if addr == "" {
		return "", defaultPort, false
	}
	idx := -1
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			idx = i
			break
		}
		if addr[i] < '0' || addr[i] > '9' {
			break
		}
	}
	if idx < 0 {
		return addr, defaultPort, true
	}
	host := addr[:idx]
	port := 0
	for _, c := range addr[idx+1:] {
		if c < '0' || c > '9' {
			return addr, defaultPort, false
		}
		port = port*10 + int(c-'0')
	}
	if port == 0 {
		port = defaultPort
	}
	return host, port, true
}

func buildSocksUsers(cfg Map) []Map {
	user, _ := cfg["username"].(string)
	pwd, _ := cfg["password"].(string)
	if user == "" && pwd == "" {
		return []Map{}
	}
	return []Map{{"user": user, "pass": pwd}}
}

func buildSingBoxSocksUser(cfg Map) (string, string) {
	user, _ := cfg["username"].(string)
	pwd, _ := cfg["password"].(string)
	return user, pwd
}

// defaultWarpRoutingRules WARP 默认路由规则（常见流媒体解锁）。
//
// Deprecated: P3-F/LB-2 自动注入已移除（按需分配模式）。此函数不再被 RenderOutbounds 自动调用，
// 保留供 admin-web WARP 管理页"一键填充流媒体规则"功能显式复用。
// 返回的 rule Map 仅包含匹配条件（domains/ip_cidr），
// outbound tag 由 renderXrayRoutingRule/renderSingBoxRoutingRule 通过 policyTag(p) 自动设置。
func defaultWarpRoutingRules() []Map {
	return []Map{
		// 流媒体域名（domain 匹配，含子域名）
		{
			"domains": []interface{}{
				"netflix.com",
				"nflxvideo.net",
				"nflximg.net",
				"chatgpt.com",
				"openai.com",
				"disneyplus.com",
				"disney-plus.net",
				"hulu.com",
				"hbo.com",
				"hbomax.com",
				"spotify.com",
				"tiktok.com",
				"youtube.com",
				"googlevideo.com",
				"ytimg.com",
				"bbc.co.uk",
				"bbc.com",
				"abema.tv",
				"dazn.com",
				"primevideo.com",
				"amazonaws.com",
			},
		},
	}
}

// splitLocalAddresses 将 local_address 字符串按逗号拆分为多个 CIDR。
// 支持双栈输入如 "172.16.0.2/32, 2606:4700:xxx/128"，输出 ["172.16.0.2/32","2606:4700:xxx/128"]。
// sing-box wireguard outbound 的 local_address 期望每个 CIDR 为独立数组元素，
// 不能把含逗号的整个字符串作为单个元素，否则双栈地址解析失败。
func splitLocalAddresses(addr string) []string {
	parts := strings.Split(addr, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{addr}
	}
	return out
}
