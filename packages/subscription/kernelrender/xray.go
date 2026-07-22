package kernelrender

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/airport-panel/subscription/nodespec"
)

// isSS2022Method 判断是否为 Shadowsocks 2022 系列加密方法
func isSS2022Method(method string) bool {
	return strings.HasPrefix(method, "2022-blake3-")
}

// XrayRenderer 生成 Xray-core 服务端配置（inbound + outbounds）
type XrayRenderer struct{}

// NewXrayRenderer 构造 Xray 渲染器
func NewXrayRenderer() *XrayRenderer { return &XrayRenderer{} }

// KernelType 返回内核类型
func (r *XrayRenderer) KernelType() KernelType { return KernelXray }

// Render 渲染 Xray 服务端配置
// 适配实际 NodeSpec 结构：Credentials/Security(string)/TLS/Reality/Transport
// Enhancement 状态从 TLS/Reality/Mux 字段推断
func (r *XrayRenderer) Render(spec *nodespec.NodeSpec) (map[string]interface{}, error) {
	if spec == nil {
		return nil, fmt.Errorf("nodespec is nil")
	}

	// 1. 内核支持矩阵检查
	if err := r.checkKernelSupport(spec); err != nil {
		return nil, err
	}

	// 2. 计算限速级别映射（P1-7: 为不同限速值分配独立 xray level）
	speedLevels := computeSpeedLevels(spec)

	// 3. 渲染 inbound
	// P8 端口语义显式分离：
	//   - port: resolveInboundPort() — CDN/Tunnel 用 ServerPort，DIRECT 用 Port
	//   - listen: resolveListenAddress() — CDN/Tunnel 绑 127.0.0.1，DIRECT 绑 0.0.0.0
	inbound := map[string]interface{}{
		"port":     resolveInboundPort(spec),
		"listen":   resolveListenAddress(spec),
		"protocol": string(spec.Protocol),
		"tag":      fmt.Sprintf("in-%s", spec.Code),
		"settings": r.renderSettings(spec, speedLevels),
	}

	// 3. 渲染 streamSettings
	streamSettings, err := r.renderStreamSettings(spec)
	if err != nil {
		return nil, fmt.Errorf("渲染streamSettings失败: %w", err)
	}
	if streamSettings != nil {
		inbound["streamSettings"] = streamSettings
	}

	// 3.5 协议级TLS（Hysteria2/TUIC等QUIC协议在Xray中tls在顶层）
	if tls := r.renderProtocolTLS(spec); tls != nil {
		inbound["tls"] = tls
	}

	// 4. ECH 警告：Xray 不支持 ECH，若用户开启则返回结构化警告（非阻断）
	if isECHEnabled(spec) {
		inbound["__warning"] = &UnsupportedFeatureWarning{Feature: "ech", Kernel: KernelXray}
	}

	// 5. RawSettings 逃生舱：仅合并 xray: 前缀的条目，绝不跨内核污染
	if spec.RawConfig != nil {
		if raw, ok := spec.RawConfig["xray"].(map[string]interface{}); ok {
			mergeMap(inbound, raw)
		}
	}

	result := map[string]interface{}{
		"inbounds":  []interface{}{inbound},
		"outbounds": r.renderOutbounds(spec),
		// stats 空对象启用 Xray 流量统计（配合 policy.system.statsOutboundUplink/Downlink）
		"stats":   map[string]interface{}{},
		"policy":  r.renderPolicy(spec, speedLevels),
		"routing": renderSecurityRouting(DefaultAuditConfig()),
	}

	// XHTTP split mode: 当 DownloadSettings.ServerPort > 0 时生成下行独立 inbound
	if dsInbound := r.renderDownloadInbound(spec, speedLevels); dsInbound != nil {
		result["inbounds"] = append(result["inbounds"].([]interface{}), dsInbound)
	}

	// 强制注入 api inbound（dokodemo-door, 127.0.0.1:10085）
	// NativeXray 依赖此 inbound 连接 gRPC HandlerService/StatsService，
	// 实现 AlterInbound 增量用户管理和 per-user 流量统计。
	// 若缺少 api inbound，AlterInbound 会静默 fallback 到全量重载，丧失零断流能力。
	ensureAPIInbound(result)
	// _limiter 元数据：供 node-agent 解析后初始化 SpeedLimiter/DeviceLimiter。
	// 以 "_" 开头的字段在写入内核配置前由 Agent 剥离（与 _nginx_vhosts 同机制），
	// 不会污染 xray 配置。仅在节点配置了限速/设备限制时生成。
	if meta := renderLimiterMeta(spec); meta != nil {
		result["_limiter"] = meta
	}
	return result, nil
}

// renderPolicy 渲染 Xray policy 配置（限速级别 + 流量统计开关）。
//
// Xray 通过 policy.levels 实现级别管理：level 数字对应用户的 rate limit 级别。
// level 0 = 不限速（默认），level 1+ = 不同限速值对应的级别。
// 每个 level 开启 statsUserUplink/Downlink/Online 以支持流量统计和设备在线追踪（P1-8）。
// 同时开启 system.statsOutboundUplink/Downlink 以支持流量统计。
func (r *XrayRenderer) renderPolicy(spec *nodespec.NodeSpec, sl *speedLevelAssignment) map[string]interface{} {
	return map[string]interface{}{
		"levels": sl.levels,
		"system": map[string]interface{}{
			"statsOutboundUplink":   true,
			"statsOutboundDownlink": true,
		},
	}
}

// speedLevelAssignment 持有限速值到 xray level 的映射，以及对应的 policy.levels 配置。
//
// P1-7: 为不同限速值（节点级 + 每用户）分配独立的 xray level，
// 在 policy.levels 中设置 up_mbps/down_mbps 实现实际限速。
// P1-8: 所有 level 开启 statsUserOnline 以支持设备在线 IP 追踪。
type speedLevelAssignment struct {
	speedToLevel map[int]int            // speed Mbps → level (0 = no limit)
	levels       map[string]interface{} // xray policy.levels config
}

// computeSpeedLevels 从 NodeSpec 构建限速级别映射和 policy 配置。
//
// Level 分配：
//   - Level 0: 默认（不限速）
//   - Level 1+: 每个唯一限速值分配一个 level（节点级 + 每用户）
//
// 每个 level 设置 up_mbps/down_mbps 为限速值，并开启统计开关。
func computeSpeedLevels(spec *nodespec.NodeSpec) *speedLevelAssignment {
	a := &speedLevelAssignment{
		speedToLevel: make(map[int]int),
		levels: map[string]interface{}{
			"0": map[string]interface{}{
				"bufferSize":        1024,
				"statsUserUplink":   true,
				"statsUserDownlink": true,
				"statsUserOnline":   true,
			},
		},
	}

	nextLevel := 1
	assignLevel := func(speedMbps int) int {
		if speedMbps <= 0 {
			return 0
		}
		if lvl, ok := a.speedToLevel[speedMbps]; ok {
			return lvl
		}
		lvl := nextLevel
		a.speedToLevel[speedMbps] = lvl
		a.levels[fmt.Sprintf("%d", lvl)] = map[string]interface{}{
			"up_mbps":           speedMbps,
			"down_mbps":         speedMbps,
			"bufferSize":        1024,
			"statsUserUplink":   true,
			"statsUserDownlink": true,
			"statsUserOnline":   true,
		}
		nextLevel++
		return lvl
	}

	// 节点级限速
	assignLevel(spec.SpeedLimitMbps)
	// 每用户限速
	for _, c := range spec.Clients {
		assignLevel(c.SpeedLimit)
	}

	return a
}

// levelForNode 返回节点级限速对应的 xray level。
func (a *speedLevelAssignment) levelForNode(nodeSpeedMbps int) int {
	if nodeSpeedMbps <= 0 {
		return 0
	}
	return a.speedToLevel[nodeSpeedMbps]
}

// levelForClient 返回指定用户的 xray level。
// 优先级：显式 c.Level > 每用户 c.SpeedLimit > 节点级限速。
func (a *speedLevelAssignment) levelForClient(c nodespec.CredentialSpec, nodeSpeedMbps int) int {
	if c.Level > 0 {
		return c.Level
	}
	if c.SpeedLimit > 0 {
		if lvl, ok := a.speedToLevel[c.SpeedLimit]; ok {
			return lvl
		}
	}
	return a.levelForNode(nodeSpeedMbps)
}

// checkKernelSupport 检查 Xray 是否支持该协议组合
func (r *XrayRenderer) checkKernelSupport(spec *nodespec.NodeSpec) error {
	// AnyTLS / Mieru: Xray 不支持
	if spec.Protocol == nodespec.ProtocolAnyTLS {
		return &UnsupportedFeatureError{Feature: "anytls", Kernel: KernelXray}
	}
	if spec.Protocol == nodespec.ProtocolMieru {
		return &UnsupportedFeatureError{Feature: "mieru", Kernel: KernelXray}
	}
	// Hysteria2 / TUIC: Xray 支持 Hysteria2（v1.8.6+），不支持 TUIC
	if spec.Protocol == nodespec.ProtocolTUIC {
		return &UnsupportedFeatureError{Feature: "tuic", Kernel: KernelXray}
	}
	// HTTPUpgrade: Xray 1.8.24+ 支持
	// xhttp: Xray 26.3+ 支持
	return nil
}

// renderSettings 渲染 inbound settings（协议层：clients/users）
// P0-4: 优先使用 spec.Clients（多用户），为空时回退到 spec.Credentials（单用户）
// P1-7: 通过 speedLevelAssignment 为每个 client 分配带限速策略的 xray level
func (r *XrayRenderer) renderSettings(spec *nodespec.NodeSpec, sl *speedLevelAssignment) map[string]interface{} {
	// 多用户路径（P0-4）
	if hasMultiClients(spec) {
		return r.renderSettingsMultiClient(spec, sl)
	}
	switch spec.Protocol {
	case nodespec.ProtocolVLESS:
		uuid := extractUUID(spec)
		client := map[string]interface{}{
			"id":    uuid,
			"level": sl.levelForNode(spec.SpeedLimitMbps),
		}
		// REALITY + Vision flow（仅 TCP 传输层支持 Vision，XHTTP/WS 等不支持）
		if flow := extractFlow(spec); flow != "" {
			client["flow"] = flow
		} else if spec.Security == nodespec.SecurityReality && spec.Transport.Type == nodespec.TransportTCP {
			// REALITY + TCP 默认推荐 vision flow
			client["flow"] = string(nodespec.FlowXTLSRprxVision)
		}
		return map[string]interface{}{
			"clients":    []interface{}{client},
			"decryption": "none",
		}
	case nodespec.ProtocolVMess:
		uuid := extractUUID(spec)
		// VMess cipher：从 spec 读取，fallback 到 "auto"
		security := "auto"
		if c, ok := spec.Credentials.(nodespec.VMessCredentials); ok && c.Security != "" {
			security = c.Security
		}
		client := map[string]interface{}{
			"id":       uuid,
			"alterId":  0,
			"level":    sl.levelForNode(spec.SpeedLimitMbps),
			"security": security,
		}
		return map[string]interface{}{
			"clients": []interface{}{client},
		}
	case nodespec.ProtocolTrojan:
		password := extractPassword(spec)
		client := map[string]interface{}{"password": password}
		if level := sl.levelForNode(spec.SpeedLimitMbps); level > 0 {
			client["level"] = level
		}
		return map[string]interface{}{
			"clients": []interface{}{client},
		}
	case nodespec.ProtocolShadowsocks:
		if c, ok := spec.Credentials.(nodespec.ShadowsocksCredentials); ok {
			return map[string]interface{}{
				"method":   c.Method,
				"password": c.Password,
				"network":  "tcp,udp",
			}
		}
		return map[string]interface{}{}
	case nodespec.ProtocolHysteria2:
		hy2Settings := map[string]interface{}{
			"password": extractPassword(spec),
		}
		if c, ok := spec.Credentials.(nodespec.Hysteria2Credentials); ok {
			if c.UpMbps > 0 {
				hy2Settings["up_mbps"] = c.UpMbps
			}
			if c.DownMbps > 0 {
				hy2Settings["down_mbps"] = c.DownMbps
			}
		}
		if spec.Transport.QUIC != nil {
			if spec.Transport.QUIC.Security != "" {
				hy2Settings["obfs"] = spec.Transport.QUIC.Security
				if spec.Transport.QUIC.Key != "" {
					hy2Settings["obfs_password"] = spec.Transport.QUIC.Key
				}
			}
		}
		return hy2Settings
	case nodespec.ProtocolSOCKS5:
		// Xray SOCKS inbound：auth=password 启用账号密码认证，accounts 列出 user/pass。
		if c, ok := spec.Credentials.(nodespec.SOCKS5Credentials); ok {
			return map[string]interface{}{
				"auth": "password",
				"accounts": []interface{}{
					map[string]interface{}{"user": c.Username, "pass": c.Password},
				},
			}
		}
		return map[string]interface{}{}
	case nodespec.ProtocolHTTP:
		// Xray HTTP inbound：accounts 非空时启用账号密码认证。
		if c, ok := spec.Credentials.(nodespec.HTTPCredentials); ok {
			return map[string]interface{}{
				"accounts": []interface{}{
					map[string]interface{}{"user": c.Username, "pass": c.Password},
				},
			}
		}
		return map[string]interface{}{}
	default:
		return map[string]interface{}{}
	}
}

// renderSettingsMultiClient 渲染多用户 settings（P0-4）
// 根据 spec.Clients []CredentialSpec 输出 Xray clients 数组
// P1-7: 通过 speedLevelAssignment 为每个 client 分配带限速策略的 xray level
func (r *XrayRenderer) renderSettingsMultiClient(spec *nodespec.NodeSpec, sl *speedLevelAssignment) map[string]interface{} {
	switch spec.Protocol {
	case nodespec.ProtocolVLESS:
		clients := make([]interface{}, 0, len(spec.Clients))
		for _, c := range spec.Clients {
			level := sl.levelForClient(c, spec.SpeedLimitMbps)
			client := map[string]interface{}{
				"id":    c.UUID,
				"level": level,
			}
			if c.Email != "" {
				client["email"] = c.Email
			}
			// flow：REALITY+TCP 默认 vision，XHTTP/WS 等传输层不支持 flow
			flow := clientFlowFor(c, spec.Transport.Type)
			if spec.Security == nodespec.SecurityReality && spec.Transport.Type == nodespec.TransportTCP && flow == "" {
				flow = string(nodespec.FlowXTLSRprxVision)
			}
			if flow != "" {
				client["flow"] = flow
			}
			clients = append(clients, client)
		}
		return map[string]interface{}{
			"clients":    clients,
			"decryption": "none",
		}
	case nodespec.ProtocolVMess:
		clients := make([]interface{}, 0, len(spec.Clients))
		for _, c := range spec.Clients {
			security := c.Security
			if security == "" {
				security = "auto"
			}
			alterID := c.AlterID
			level := sl.levelForClient(c, spec.SpeedLimitMbps)
			client := map[string]interface{}{
				"id":       c.UUID,
				"alterId":  alterID,
				"level":    level,
				"security": security,
			}
			if c.Email != "" {
				client["email"] = c.Email
			}
			clients = append(clients, client)
		}
		return map[string]interface{}{
			"clients": clients,
		}
	case nodespec.ProtocolTrojan:
		clients := make([]interface{}, 0, len(spec.Clients))
		for _, c := range spec.Clients {
			client := map[string]interface{}{"password": c.Password}
			if c.Email != "" {
				client["email"] = c.Email
			}
			level := sl.levelForClient(c, spec.SpeedLimitMbps)
			if level > 0 {
				client["level"] = level
			}
			clients = append(clients, client)
		}
		return map[string]interface{}{
			"clients": clients,
		}
	case nodespec.ProtocolShadowsocks:
		// SS2022 多用户：password 在 settings 层，clients 带 password
		if len(spec.Clients) > 0 {
			first := spec.Clients[0]
			method := first.Method
			settings := map[string]interface{}{
				"method":  method,
				"network": "tcp,udp",
			}
			if isSS2022Method(method) {
				// SS2022：主 password + clients[].password
				settings["password"] = first.Password
				clients := make([]interface{}, 0, len(spec.Clients))
				for _, c := range spec.Clients {
					client := map[string]interface{}{"password": c.Password}
					clients = append(clients, client)
				}
				settings["clients"] = clients
			} else {
				// 非 SS2022：单 password
				settings["password"] = first.Password
			}
			return settings
		}
		return map[string]interface{}{}
	case nodespec.ProtocolHysteria2:
		// Hysteria2 单密码协议，多用户共享 password；取首个
		if len(spec.Clients) > 0 {
			first := spec.Clients[0]
			hy2Settings := map[string]interface{}{"password": first.Password}
			if first.UpMbps > 0 {
				hy2Settings["up_mbps"] = first.UpMbps
			}
			if first.DownMbps > 0 {
				hy2Settings["down_mbps"] = first.DownMbps
			}
			if spec.Transport.QUIC != nil {
				if spec.Transport.QUIC.Security != "" {
					hy2Settings["obfs"] = spec.Transport.QUIC.Security
					if spec.Transport.QUIC.Key != "" {
						hy2Settings["obfs_password"] = spec.Transport.QUIC.Key
					}
				}
			}
			return hy2Settings
		}
		return map[string]interface{}{}
	case nodespec.ProtocolSOCKS5:
		// Xray SOCKS inbound（多用户）：auth=password + accounts[]
		accounts := make([]interface{}, 0, len(spec.Clients))
		for _, c := range spec.Clients {
			account := map[string]interface{}{}
			if c.UUID != "" {
				account["user"] = c.UUID
			}
			if c.Password != "" {
				account["pass"] = c.Password
			}
			accounts = append(accounts, account)
		}
		return map[string]interface{}{
			"auth":     "password",
			"accounts": accounts,
		}
	case nodespec.ProtocolHTTP:
		// Xray HTTP inbound（多用户）：accounts[] 非空时启用认证
		httpAccounts := make([]interface{}, 0, len(spec.Clients))
		for _, c := range spec.Clients {
			account := map[string]interface{}{}
			if c.UUID != "" {
				account["user"] = c.UUID
			}
			if c.Password != "" {
				account["pass"] = c.Password
			}
			httpAccounts = append(httpAccounts, account)
		}
		return map[string]interface{}{
			"accounts": httpAccounts,
		}
	default:
		return map[string]interface{}{}
	}
}

// renderProtocolTLS 渲染协议级 TLS 配置（Hysteria2/TUIC 等 QUIC 协议在 Xray 中使用顶层 tls 字段）
func (r *XrayRenderer) renderProtocolTLS(spec *nodespec.NodeSpec) map[string]interface{} {
	if spec.Protocol != nodespec.ProtocolHysteria2 {
		return nil
	}
	if spec.Security != nodespec.SecurityTLS || spec.TLS == nil {
		return nil
	}
	tls := map[string]interface{}{
		"enabled": true,
	}
	if spec.TLS.SNI != "" {
		tls["serverName"] = spec.TLS.SNI
	}
	if len(spec.TLS.ALPN) > 0 {
		tls["alpn"] = spec.TLS.ALPN
	}
	// P0-2: PEM-only 路径——优先 TLSMaterialRef inline PEM
	if spec.TLS.Material != nil && spec.TLS.Material.InlinePEM != nil &&
		len(spec.TLS.Material.InlinePEM.CertPEM) > 0 && len(spec.TLS.Material.InlinePEM.KeyPEM) > 0 {
		tls["certificate"] = string(spec.TLS.Material.InlinePEM.CertPEM)
		tls["key"] = string(spec.TLS.Material.InlinePEM.KeyPEM)
	} else if spec.TLS.CertPEM != "" && spec.TLS.KeyPEM != "" {
		tls["certificate"] = spec.TLS.CertPEM
		tls["key"] = spec.TLS.KeyPEM
	}
	// P0-2: 禁止 certificateFile/keyFile 输出，强制 PEM-only
	return tls
}

// renderStreamSettings 渲染 streamSettings（传输层 + 安全层）
// QUIC 协议（Hysteria2/TUIC）不使用 streamSettings，返回 nil。
func (r *XrayRenderer) renderStreamSettings(spec *nodespec.NodeSpec) (map[string]interface{}, error) {
	if spec.Transport.Type == nodespec.TransportQUIC {
		return nil, nil
	}
	ss := map[string]interface{}{"network": string(spec.Transport.Type)}

	// ===== 安全层 =====
	switch spec.Security {
	case nodespec.SecurityReality:
		if spec.Reality == nil {
			return nil, fmt.Errorf("REALITY场景缺少reality配置")
		}
		// target（REALITY 回落目标）：必须由用户显式配置，不使用 SNI:443 兜底
		// 支持两种填法：
		//   1. 本地反代：127.0.0.1:9454（推荐，回落到本地 nginx vhost 反代真实站点）
		//   2. 伪装域名：oyc.yale.edu:443（直连模式，回落到真实外部站点）
		// 以编辑保存优先：Dest 为空时返回错误，强制用户填写，避免 xray 反代 SNI 自身造成循环
		realityTarget := spec.Reality.Dest
		if realityTarget == "" {
			return nil, fmt.Errorf("REALITY dest 未配置：请在节点编辑中填写 reality_dest（本地反代如 127.0.0.1:9454 或伪装域名如 oyc.yale.edu:443）")
		}
		reality := map[string]interface{}{
			"target":      realityTarget,
			"privateKey":  spec.Reality.PrivateKey,
			"shortIds":    r.realityShortIDs(spec),
			"serverNames": []string{spec.Reality.SNI},
		}
		// uTLS 指纹
		fp := spec.Reality.Fingerprint
		if fp == "" {
			fp = "chrome" // REALITY 默认 chrome 指纹
		}
		reality["fingerprint"] = fp
		ss["security"] = "reality"
		ss["realitySettings"] = reality

	case nodespec.SecurityTLS:
		if spec.TLS == nil {
			return nil, fmt.Errorf("TLS场景缺少tls配置")
		}
		tlsSettings := map[string]interface{}{}
		if spec.TLS.SNI != "" {
			tlsSettings["serverName"] = spec.TLS.SNI
		}
		// R7 修复：按传输类型修正 ALPN
		// gRPC 必须为 ["h2"]，WS/HTTPUpgrade 为 ["h2","http/1.1"]，其他保持原值
		alpn := spec.TLS.ALPN
		switch spec.Transport.Type {
		case nodespec.TransportGRPC:
			alpn = []string{"h2"}
		case nodespec.TransportWS, nodespec.TransportHTTPUpgrade:
			if len(alpn) == 0 {
				alpn = []string{"h2", "http/1.1"}
			}
		}
		if len(alpn) > 0 {
			tlsSettings["alpn"] = alpn
		}
		// 注：allowInsecure 是客户端设置，不应出现在服务端 inbound TLS 配置中。
		// xray 26.3.27 仍支持 allowInsecure 字段（用于 outbound/客户端侧），但服务端 inbound 不需要。
		// 服务端 inbound 仅需配置证书 + SNI + ALPN + fingerprint。
		// uTLS 指纹
		if spec.TLS.Fingerprint != "" {
			tlsSettings["fingerprint"] = spec.TLS.Fingerprint
		}
		// 证书配置（P0-2: PEM-only，禁止 certificateFile/keyFile 输出）
		if spec.TLS.Material != nil && spec.TLS.Material.InlinePEM != nil &&
			len(spec.TLS.Material.InlinePEM.CertPEM) > 0 && len(spec.TLS.Material.InlinePEM.KeyPEM) > 0 {
			// PEM-only 路径：绝不输出 certificateFile/keyFile
			tlsSettings["certificates"] = []interface{}{
				map[string]interface{}{
					"certificate": []string{string(spec.TLS.Material.InlinePEM.CertPEM)},
					"key":         []string{string(spec.TLS.Material.InlinePEM.KeyPEM)},
				},
			}
		} else if spec.TLS.CertPEM != "" && spec.TLS.KeyPEM != "" {
			tlsSettings["certificates"] = []interface{}{
				map[string]interface{}{
					"certificate": []string{spec.TLS.CertPEM},
					"key":         []string{spec.TLS.KeyPEM},
				},
			}
		}
		ss["security"] = "tls"
		ss["tlsSettings"] = tlsSettings

	default:
		ss["security"] = "none"
	}

	// ===== 传输层 =====
	switch spec.Transport.Type {
	case nodespec.TransportTCP:
		// TCP 无需额外配置，但 REALITY+TCP+Vision 需要 flow（在 settings 中已设置）
		if spec.Security == nodespec.SecurityReality && spec.Transport.TCPBrutal != nil && spec.Transport.TCPBrutal.Enabled {
			ss["tcpSettings"] = map[string]interface{}{
				"header": map[string]interface{}{"type": "none"},
			}
		}
	case nodespec.TransportWS:
		if spec.Transport.WS != nil {
			ws := map[string]interface{}{
				"path": spec.Transport.WS.Path,
			}
			if spec.Transport.WS.Host != "" {
				ws["host"] = spec.Transport.WS.Host
			}
			ss["wsSettings"] = ws
		}
	case nodespec.TransportGRPC:
		if spec.Transport.GRPC != nil {
			ss["grpcSettings"] = map[string]interface{}{
				"serviceName": spec.Transport.GRPC.ServiceName,
			}
		}
	case nodespec.TransportXHTTP:
		if spec.Transport.XHTTP == nil {
			return nil, fmt.Errorf("xhttp传输缺少配置")
		}
		if spec.Transport.XHTTP.Mode == "" || spec.Transport.XHTTP.Mode == "auto" {
			return nil, fmt.Errorf("xhttp_mode禁止为空或auto，必须显式指定packet-up/stream-up/stream-down")
		}
		xhttp := map[string]interface{}{
			"path": spec.Transport.XHTTP.Path,
			"mode": spec.Transport.XHTTP.Mode,
		}
		if spec.Transport.XHTTP.Host != "" {
			xhttp["host"] = spec.Transport.XHTTP.Host
		}
		if spec.Transport.XHTTP.XPaddingBytes != "" {
			xhttp["xPaddingBytes"] = spec.Transport.XHTTP.XPaddingBytes
		}
		if spec.Transport.XHTTP.NoGRPCHeader {
			xhttp["noGRPCHeader"] = true
		}
		// extra 字段：XMUX、Headers、downloadSettings 都放在 xhttpSettings.extra
		// 注意：XMUX 不能放到 outbound 级 CMux，Xray 对 XHTTP 的多路复用走 extra.xmux
		extra := map[string]interface{}{}
		// XMUX 渲染（XHTTP 专用多路复用）
		if m := spec.Transport.Mux; m != nil && m.Enabled && m.Protocol == nodespec.MuxProtocolXmux {
			xmux := map[string]interface{}{}
			if m.MaxConcurrency != "" {
				xmux["maxConcurrency"] = m.MaxConcurrency
			}
			if m.MaxConnections > 0 {
				xmux["maxConnections"] = m.MaxConnections
			}
			if m.CMaxReuseTimes != "" {
				xmux["cMaxReuseTimes"] = m.CMaxReuseTimes
			}
			if m.HMaxRequestTimes != "" {
				xmux["hMaxRequestTimes"] = m.HMaxRequestTimes
			}
			if m.HMaxReusableSecs != "" {
				xmux["hMaxReusableSecs"] = m.HMaxReusableSecs
			}
			if len(xmux) > 0 {
				extra["xmux"] = xmux
			}
		}
		// Headers（如 Referer 伪装）
		if len(spec.Transport.XHTTP.Headers) > 0 {
			extra["headers"] = spec.Transport.XHTTP.Headers
		}
		// downloadSettings 暂时不渲染：xray 26.3.27 存在 downloadSettings 静默失败 bug
		// （配置写入后不生效且无错误日志），项目约束要求从服务端 inbound 配置中移除
		// downloadSettings，直到上游修复。仅影响服务端 inbound 配置，不影响客户端
		// share link 生成（uri/clash 渲染器各自独立处理 downloadSettings）。
		// 上游修复后，取消下方注释即可恢复：
		// if ds := spec.Transport.XHTTP.DownloadSettings; ds != nil && ds.Address != "" {
		// 	extra["downloadSettings"] = r.renderXHTTPDownload(ds)
		// }
		if len(extra) > 0 {
			xhttp["extra"] = extra
		}
		ss["xhttpSettings"] = xhttp
	case nodespec.TransportHTTPUpgrade:
		if spec.Transport.HTTPUpgrade != nil {
			hu := map[string]interface{}{
				"path": spec.Transport.HTTPUpgrade.Path,
			}
			if spec.Transport.HTTPUpgrade.Host != "" {
				hu["host"] = spec.Transport.HTTPUpgrade.Host
			}
			ss["httpupgradeSettings"] = hu
		}
	case nodespec.TransportKCP:
		if spec.Transport.KCP != nil {
			kcp := map[string]interface{}{
				"mtu":              spec.Transport.KCP.MTU,
				"tti":              spec.Transport.KCP.TTI,
				"uplinkCapacity":   spec.Transport.KCP.UplinkCapacity,
				"downlinkCapacity": spec.Transport.KCP.DownlinkCapacity,
			}
			if spec.Transport.KCP.Seed != "" {
				kcp["seed"] = spec.Transport.KCP.Seed
			}
			ss["kcpSettings"] = kcp
		}
	}

	// ===== Mux（Xray 的 mux 是 outbound 级别，这里仅记录，实际在 renderOutbounds 处理）=====
	// 注意：Xray 的 mux 配置在 outbound 的 mux 字段，不在 inbound 的 streamSettings

	return ss, nil
}

// realityShortIDs 返回 REALITY shortIds（兼容单个 short_id 和数组 short_ids）
func (r *XrayRenderer) realityShortIDs(spec *nodespec.NodeSpec) []string {
	if spec.Reality == nil {
		return []string{}
	}
	if len(spec.Reality.ShortIDs) > 0 {
		return spec.Reality.ShortIDs
	}
	if spec.Reality.ShortID != "" {
		return []string{spec.Reality.ShortID}
	}
	return []string{""}
}

// renderXHTTPDownload 渲染 XHTTP downloadSettings（split mode 上下行分离）
// 当前未被调用：xray 26.3.27 的 downloadSettings 静默失败 bug 导致该配置不生效，
// 已在 renderStreamSettings 中暂时跳过。上游修复后恢复调用即可。
func (r *XrayRenderer) renderXHTTPDownload(ds *nodespec.XHTTPDownloadConfig) map[string]interface{} {
	download := map[string]interface{}{
		"address": ds.Address,
		"port":    ds.Port,
	}
	if ds.Port == 0 {
		download["port"] = 443
	}
	network := string(ds.Network)
	if network == "" {
		network = "xhttp"
	}
	download["network"] = network

	if ds.Path != "" || ds.Host != "" || ds.Mode != "" || ds.NoGRPCHeader {
		xh := map[string]interface{}{}
		if ds.Path != "" {
			xh["path"] = ds.Path
		}
		if ds.Host != "" {
			xh["host"] = ds.Host
		}
		if ds.Mode != "" {
			xh["mode"] = ds.Mode
		}
		if ds.NoGRPCHeader {
			xh["noGRPCHeader"] = true
		}
		download["xhttpSettings"] = xh
	}

	security := string(ds.Security)
	if ds.Reality != nil {
		security = "reality"
		reality := map[string]interface{}{
			"publicKey":  ds.Reality.PublicKey,
			"shortId":    ds.Reality.ShortID,
			"serverName": ds.Reality.SNI,
		}
		if ds.Reality.Fingerprint != "" {
			reality["fingerprint"] = ds.Reality.Fingerprint
		}
		download["realitySettings"] = reality
	} else if ds.TLS != nil {
		security = "tls"
		tls := map[string]interface{}{}
		if ds.TLS.SNI != "" {
			tls["serverName"] = ds.TLS.SNI
		}
		if ds.TLS.Fingerprint != "" {
			tls["fingerprint"] = ds.TLS.Fingerprint
		}
		if len(ds.TLS.ALPN) > 0 {
			tls["alpn"] = ds.TLS.ALPN
		}
		if len(tls) > 0 {
			download["tlsSettings"] = tls
		}
	}
	if security != "" {
		download["security"] = security
	}
	return download
}

// renderDownloadInbound 为 XHTTP split mode (上下行分离) 渲染下行独立 inbound。
//
// 架构说明：
//   - 上行 inbound (主inbound): 监听 server_port，接收CDN/nginx转发的流量（TLS在CDN处终止）
//   - 下行 inbound: 监听 downloadSettings.server_port，直接接收客户端REALITY/TLS连接
//   - 两个inbound共享同一组users/clients凭证
//
// 当 DownloadSettings.ServerPort > 0 且安全类型与主inbound不同时生成。
func (r *XrayRenderer) renderDownloadInbound(spec *nodespec.NodeSpec, sl *speedLevelAssignment) map[string]interface{} {
	if spec.Transport.XHTTP == nil || spec.Transport.XHTTP.DownloadSettings == nil {
		return nil
	}
	ds := spec.Transport.XHTTP.DownloadSettings
	if ds.ServerPort <= 0 {
		return nil
	}
	// 仅当安全类型不同时需要独立inbound（相同安全类型走同一inbound）
	if ds.Security == spec.Security {
		return nil
	}

	// 下行inbound的协议与主inbound相同（VLESS/VMess等）
	// _inbound_role 是 P1 正式方案的显式元数据：node-service 通过此字段识别下行inbound，
	// 不再依赖tag字符串模式匹配（tag后缀仅作展示命名，不参与安全判定）。
	dlInbound := map[string]interface{}{
		"port":          ds.ServerPort,
		"listen":        "127.0.0.1",
		"protocol":      string(spec.Protocol),
		"tag":           fmt.Sprintf("in-%s%s", spec.Code, DownstreamTagSuffix),
		"_inbound_role": "downstream",
		"settings":      r.renderSettings(spec, sl),
	}

	// 构建下行streamSettings
	dlSS := map[string]interface{}{
		"network": "xhttp",
	}

	// 下行安全层
	switch ds.Security {
	case nodespec.SecurityReality:
		if ds.Reality == nil {
			return nil
		}
		// 下行Reality私钥：优先使用downloadSettings自身的private_key，
		// 若未设置则复用主Reality配置的privateKey（适用于主节点也是Reality的场景）
		privateKey := ds.Reality.PrivateKey
		if privateKey == "" && spec.Reality != nil {
			privateKey = spec.Reality.PrivateKey
		}
		if privateKey == "" {
			return nil // 无法获取Reality私钥，跳过下行inbound生成
		}
		shortIDs := []string{ds.Reality.ShortID}
		if len(ds.Reality.ShortIDs) > 0 {
			shortIDs = ds.Reality.ShortIDs
		}
		// 下行 REALITY target：必须由用户显式配置，不使用 SNI:443 兜底
		// 支持两种填法：
		//   1. 本地反代：127.0.0.1:9454（推荐，与主 REALITY 共用或独立端口）
		//   2. 伪装域名：oyc.yale.edu:443（直连模式）
		// 以编辑保存优先：Dest 为空时记录日志并返回 nil（跳过下行 inbound 生成）
		// 上层 preflight_validator.go 会在节点保存阶段强制要求 dest 非空
		dlRealityTarget := ds.Reality.Dest
		if dlRealityTarget == "" && spec.Reality != nil {
			// 回退到主 Reality.Dest（仅当主 Reality 显式配置了 dest 时）
			dlRealityTarget = spec.Reality.Dest
		}
		if dlRealityTarget == "" {
			slog.Warn("下行 REALITY dest 未配置，跳过下行 inbound 生成",
				"node_code", spec.Code,
				"hint", "请在节点编辑的 download_settings 中填写 dest（本地反代如 127.0.0.1:9454 或伪装域名如 oyc.yale.edu:443）")
			return nil
		}
		reality := map[string]interface{}{
			"target":      dlRealityTarget,
			"privateKey":  privateKey,
			"shortIds":    shortIDs,
			"serverNames": []string{ds.Reality.SNI},
		}
		fp := ds.Reality.Fingerprint
		if fp == "" {
			fp = "chrome"
		}
		reality["fingerprint"] = fp
		dlSS["security"] = "reality"
		dlSS["realitySettings"] = reality
	case nodespec.SecurityTLS:
		tlsSettings := map[string]interface{}{}
		if ds.TLS != nil {
			if ds.TLS.SNI != "" {
				tlsSettings["serverName"] = ds.TLS.SNI
			}
			if len(ds.TLS.ALPN) > 0 {
				tlsSettings["alpn"] = ds.TLS.ALPN
			}
			if ds.TLS.Fingerprint != "" {
				tlsSettings["fingerprint"] = ds.TLS.Fingerprint
			}
			// 证书配置：复用主inbound的证书（同服务器共用证书）
			if spec.TLS != nil && spec.TLS.Material != nil && spec.TLS.Material.InlinePEM != nil &&
				len(spec.TLS.Material.InlinePEM.CertPEM) > 0 && len(spec.TLS.Material.InlinePEM.KeyPEM) > 0 {
				tlsSettings["certificates"] = []interface{}{
					map[string]interface{}{
						"certificate": []string{string(spec.TLS.Material.InlinePEM.CertPEM)},
						"key":         []string{string(spec.TLS.Material.InlinePEM.KeyPEM)},
					},
				}
			} else if spec.TLS != nil && spec.TLS.CertPEM != "" && spec.TLS.KeyPEM != "" {
				tlsSettings["certificates"] = []interface{}{
					map[string]interface{}{
						"certificate": []string{spec.TLS.CertPEM},
						"key":         []string{spec.TLS.KeyPEM},
					},
				}
			}
		}
		dlSS["security"] = "tls"
		dlSS["tlsSettings"] = tlsSettings
	default:
		dlSS["security"] = "none"
	}

	// 下行xhttpSettings
	mode := ds.Mode
	if mode == "" {
		mode = "stream-up"
	}
	xh := map[string]interface{}{
		"path": ds.Path,
		"mode": mode,
	}
	if ds.Host != "" {
		xh["host"] = ds.Host
	}
	if ds.NoGRPCHeader {
		xh["noGRPCHeader"] = true
	}
	dlSS["xhttpSettings"] = xh

	dlInbound["streamSettings"] = dlSS
	return dlInbound
}

// renderOutbounds 渲染出站配置
func (r *XrayRenderer) renderOutbounds(spec *nodespec.NodeSpec) []interface{} {
	direct := map[string]interface{}{
		"protocol": "freedom",
		"tag":      "direct",
	}
	// sockopt 性能参数（tcp_fast_open/bbr 等）
	if spec.Transport.Sockopt != nil {
		sockopt := map[string]interface{}{}
		if spec.Transport.Sockopt.TCPFastOpen {
			sockopt["tcpFastOpen"] = true
		}
		if spec.Transport.Sockopt.TCPMultipath {
			sockopt["tcpMultipath"] = true
		}
		if spec.Transport.Sockopt.Congestion != "" {
			sockopt["tcpCongestion"] = spec.Transport.Sockopt.Congestion
		}
		if spec.Transport.Sockopt.TCPKeepAlive > 0 {
			sockopt["tcpKeepAliveInterval"] = spec.Transport.Sockopt.TCPKeepAlive
		}
		if len(sockopt) > 0 {
			direct["sockopt"] = sockopt
		}
	}
	outbounds := []interface{}{
		direct,
		map[string]interface{}{
			"protocol": "blackhole",
			"tag":      "block",
		},
	}

	// Mux outbound（仅非 XHTTP 协议走传统 outbound 级 CMux；
	// XHTTP 的多路复用走 xhttpSettings.extra.xmux，已在 renderStreamSettings 处理）
	if shouldUseCMux(spec) {
		muxOut := map[string]interface{}{
			"protocol": "freedom",
			"tag":      "mux-out",
			"mux": map[string]interface{}{
				"enabled":     true,
				"concurrency": 8,
			},
		}
		outbounds = append([]interface{}{muxOut}, outbounds...)
	}

	return outbounds
}

// shouldUseCMux 判断是否使用传统 outbound 级 CMux。
// XHTTP 协议永远走 xhttpSettings.extra.xmux，不走 outbound CMux。
func shouldUseCMux(spec *nodespec.NodeSpec) bool {
	if spec.Transport.Mux == nil || !spec.Transport.Mux.Enabled {
		return false
	}
	if spec.Transport.Type == nodespec.TransportXHTTP {
		// XHTTP + XMUX 走 extra.xmux；XHTTP + 其他 mux protocol 也不走 outbound CMux（避免冲突）
		return false
	}
	return true
}

// ===== Enhancement 状态推断辅助函数 =====

// AuditConfig 控制审计路由规则（S8）的注入。
// BlockBT 为 true 时阻断 BitTorrent 协议流量；
// BlockPrivateIP 为 true 时阻断私有 IP 段访问（SSRF 防护）。
type AuditConfig struct {
	BlockBT        bool
	BlockPrivateIP bool
}

// DefaultAuditConfig 返回默认审计配置（BT + SSRF 防护全部启用）。
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{BlockBT: true, BlockPrivateIP: true}
}

// InjectAuditRules 向已有的 xray routing 配置中注入审计路由规则（S8）。
// 根据 AuditConfig 的配置，可选注入 BT 阻断规则和 SSRF 防护规则。
// 审计规则插入到 rules 列表头部，确保优先匹配。
// 要求 outbounds 中已包含 tag="block" 的 blackhole 出站。
func InjectAuditRules(routing map[string]interface{}, cfg AuditConfig) {
	if !cfg.BlockBT && !cfg.BlockPrivateIP {
		return
	}
	rules, _ := routing["rules"].([]interface{})
	auditRules := make([]interface{}, 0, 2)
	if cfg.BlockPrivateIP {
		auditRules = append(auditRules, map[string]interface{}{
			"type":        "field",
			"ip":          []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "198.18.0.0/15", "fc00::/7"},
			"outboundTag": "block",
		})
	}
	if cfg.BlockBT {
		auditRules = append(auditRules, map[string]interface{}{
			"type":        "field",
			"protocol":    []string{"bittorrent"},
			"outboundTag": "block",
		})
	}
	// 插入到 rules 头部，确保审计规则优先匹配
	routing["rules"] = append(auditRules, rules...)
}

// renderSecurityRouting 生成安全路由规则（S6/S7/S8）。
//
// 根据 AuditConfig 配置注入安全规则到 xray routing：
//  1. SSRF 防护：阻断私有 IP 段（10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10, 198.18.0.0/15, fc00::/7）
//  2. BT 防护：阻断 BitTorrent 协议流量
//
// 这些规则确保用户无法通过代理访问内网资源（SSRF）或进行 P2P 下载（BT），
// 是节点安全审计的基本要求。
func renderSecurityRouting(cfg AuditConfig) map[string]interface{} {
	routing := map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          []interface{}{},
	}
	InjectAuditRules(routing, cfg)
	return routing
}

// isECHEnabled 判断是否启用了 ECH
func isECHEnabled(spec *nodespec.NodeSpec) bool {
	return spec.TLS != nil && spec.TLS.ECH != nil && spec.TLS.ECH.Enabled
}

// isMuxEnabled 判断是否启用了 Mux
func isMuxEnabled(spec *nodespec.NodeSpec) bool {
	return spec.Transport.Mux != nil && spec.Transport.Mux.Enabled
}

// isUTLSEnabled 判断是否启用了 uTLS 指纹
func isUTLSEnabled(spec *nodespec.NodeSpec) bool {
	if spec.TLS != nil && spec.TLS.Fingerprint != "" {
		return true
	}
	if spec.Reality != nil && spec.Reality.Fingerprint != "" {
		return true
	}
	return false
}

// mergeMap 将 src 合并到 dst（src 的键覆盖 dst 的同名键）
func mergeMap(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

// ensureAPIInbound 确保 xray 配置中包含 api inbound（dokodemo-door, 127.0.0.1:10085）。
//
// NativeXray 依赖此 inbound 连接 gRPC HandlerService/StatsService，实现：
//   - AlterInbound 增量用户管理（零断流添加/删除用户）
//   - StatsService per-user 流量统计（QueryStats）
//
// 若配置中已有 tag="api" 的 inbound，不重复注入。
// 同时注入顶层 api 配置块（HandlerService + StatsService）和路由规则。
func ensureAPIInbound(cfg map[string]interface{}) {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return
	}

	// 检查是否已有 api inbound
	for _, ib := range inbounds {
		if m, ok := ib.(map[string]interface{}); ok {
			if tag, _ := m["tag"].(string); tag == "api" {
				return // 已存在，不重复注入
			}
		}
	}

	// 注入 api inbound（dokodemo-door，监听 127.0.0.1:10085）
	apiInbound := map[string]interface{}{
		"tag":      "api",
		"listen":   "127.0.0.1",
		"port":     10085,
		"protocol": "dokodemo-door",
		"settings": map[string]interface{}{"address": "127.0.0.1"},
	}
	cfg["inbounds"] = append(inbounds, apiInbound)

	// 注入顶层 api 配置块（启用 HandlerService + StatsService）
	cfg["api"] = map[string]interface{}{
		"tag":      "api",
		"services": []string{"HandlerService", "StatsService"},
	}

	// 注入路由规则：api inbound → api 服务（outboundTag="api" 指向顶层 api 配置块）
	// 插入到规则头部，确保 api 流量优先路由
	routing, ok := cfg["routing"].(map[string]interface{})
	if !ok {
		routing = map[string]interface{}{}
		cfg["routing"] = routing
	}
	rules, _ := routing["rules"].([]interface{})
	apiRule := map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{"api"},
		"outboundTag": "api",
	}
	routing["rules"] = append([]interface{}{apiRule}, rules...)
}
