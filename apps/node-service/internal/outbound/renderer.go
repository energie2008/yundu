package outbound

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
			SingBox: RenderedRuntime{Outbounds: []Map{}, RoutingRules: []Map{}},
		}, nil
	}

	xrayOutbounds := make([]Map, 0, len(policies))
	singBoxOutbounds := make([]Map, 0, len(policies))
	xrayRules := make([]Map, 0)
	singBoxRules := make([]Map, 0)

	for _, p := range policies {
		if p == nil || !p.IsEnabled {
			continue
		}

		// xray outbound
		if xb, err := renderXrayOutbound(p); err == nil {
			xrayOutbounds = append(xrayOutbounds, xb)
		}
		// sing-box outbound
		if sb, err := renderSingBoxOutbound(p); err == nil {
			singBoxOutbounds = append(singBoxOutbounds, sb)
		}

		// routing rules（同形式：policy 配置里的 routing_rules 直接转译）
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
					"address":  server,
					"port":     port,
					"users":   buildSocksUsers(p.ConfigJSON),
				}},
			},
		}, nil
	case "warp":
		// WARP 在 xray 中表现为 socks 出站到本地 warp 客户端
		server, _ := p.ConfigJSON["server"].(string)
		if server == "" {
			server = "127.0.0.1"
		}
		port := toInt(p.ConfigJSON["port"])
		if port == 0 {
			port = 40000
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
			"type":    "socks",
			"tag":     tag,
			"server":  server,
			"port":    port,
		}
		if u, p := buildSingBoxSocksUser(p.ConfigJSON); u != "" {
			out["username"] = u
			out["password"] = p
		}
		return out, nil
	case "warp":
		// sing-box 原生支持 WARP（type=warp），但不同版本字段不同，这里用 socks 形式兜底
		server, _ := p.ConfigJSON["server"].(string)
		if server == "" {
			server = "127.0.0.1"
		}
		port := toInt(p.ConfigJSON["port"])
		if port == 0 {
			port = 40000
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
	}
	return nil, ErrRenderFailed
}

// renderXrayRoutingRule 把 policy 的 routing_rule 转为 xray 规则
func renderXrayRoutingRule(p *OutboundPolicy, rule Map) Map {
	out := Map{
		"type":         "field",
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
