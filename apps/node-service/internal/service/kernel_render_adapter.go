package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/airport-panel/node-service/internal/exposure"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/airport-panel/subscription/kernelrender"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/google/uuid"
)

// kernel_render_adapter.go 实现 P0-1：用 kernelrender（IR→Compiler）替代 exposure 直拼配置。
//
// 核心逻辑：
//  1. modelNodeToNodeSpecWithCreds: DB model.Node → NodeSpec IR，并注入多用户 Clients
//  2. buildXrayConfigViaKernelRender: 遍历节点 → kernelrender.RenderForKernel → 聚合 inbounds
//  3. buildSingboxConfigViaKernelRender: 同上，sing-box 内核
//
// 多用户凭证映射（XBoard 模型）：所有协议共用 user.uuid
//   - VLESS/VMess/TUIC: UUID = user.uuid
//   - Trojan/SS/Hysteria2/AnyTLS: Password = user.uuid
//   - SS: Method = node.config_json.method

// DownstreamTagSuffix 引用 kernelrender 包的统一常量，用于下行 inbound tag 的展示性命名。
//
// 重要约束（阶段3已退役）：
// P0 阶段基于此常量做 tag 后缀判定是临时兜底逻辑，P1 验证通过（p06测速完美）后已退役。
// 现在下行 inbound 的身份识别由 kernelrender 渲染器注入的显式元数据字段
// `_inbound_role: "downstream"` 承担，tag后缀仅作展示命名，不参与安全逻辑判定。
// isDownstreamInbound 函数保留为防御性 fallback（理论上不应被触发），
// 若触发则记录警告日志，表明渲染器未正确注入 _inbound_role 字段。
const DownstreamTagSuffix = kernelrender.DownstreamTagSuffix

// isDownstreamInbound 判断 inbound tag 是否属于上下行分离节点的下行 inbound。
//
// 阶段3退役说明：
// 本函数 P0 阶段用于直接在剥离逻辑中跳过下行 inbound，是临时兜底。
// P1 正式方案中，kernelrender 已在下行 inbound 注入显式 `_inbound_role: "downstream"` 字段，
// determineInboundExposureMode 优先读取该字段。本函数仅作为防御性 fallback 使用，
// 若被调用说明渲染器版本不一致或存在配置异常，会记录警告。
func isDownstreamInbound(tag string) bool {
	return strings.HasSuffix(tag, DownstreamTagSuffix)
}

// isDownstreamInboundFromMap 从 inbound map 的显式元数据字段判断是否为下行 inbound。
// P1 正式方案：优先使用 _inbound_role 字段（kernelrender 注入的显式标识），
// 避免任何 tag 字符串模式匹配参与安全判定路径。
func isDownstreamInboundFromMap(inbMap map[string]interface{}) bool {
	if inbMap == nil {
		return false
	}
	if role, ok := inbMap["_inbound_role"].(string); ok && role == "downstream" {
		return true
	}
	// 防御性 fallback：理论上不该走到这里，若走到说明渲染器未注入显式字段
	if tag, ok := inbMap["tag"].(string); ok && isDownstreamInbound(tag) {
		slog.Warn("downstream inbound detected via tag suffix fallback; renderer should inject _inbound_role",
			"tag", tag)
		return true
	}
	return false
}

// determineInboundExposureMode 判定单个 inbound 的暴露方式，用于决定是否剥离 TLS。
//
// is_split_mode 字段定位（重要约束）：
//   - 它是前端表单开关，控制"下行暴露方式"下拉框是否显示
//   - 它是 DTO 字段，会持久化到 DB
//   - 它【不】进入渲染逻辑：本函数不读取 IsSplitMode 字段
//   - 后端安全判定只依赖 downstream_exposure_mode 字段是否有值
//   - 任何分支都不得加 `if node.IsSplitMode` 影响剥离判定
//     违反此约束会导致状态源变成两个（tag + is_split_mode），重新制造 p06 式事故
//
// 阶段3退役更新：
// 下行 inbound 身份识别优先使用 kernelrender 注入的显式字段 _inbound_role="downstream"，
// 不再依赖 tag 字符串后缀匹配做安全路径判定。tag 后缀仅作展示命名。
//
// 判定规则：
//  1. 若 inbound 有 _inbound_role="downstream"（显式标识）：返回 node.DownstreamExposureMode
//     - 若 DownstreamExposureMode 独立列有值 → 使用
//     - 否则从 config_json.downstream_exposure_mode 回退
//     - 都没有则回退到 "direct"（下行默认 REALITY 直连不剥离）
//  2. 否则（上行/普通 inbound）：返回上行 exposure mode
//     - 优先 node.ExposureMode 独立列
//     - 回退 config_json.exposure_mode
//     - 最终回退到 isCDNNode 旧逻辑（历史节点兼容性）
func determineInboundExposureMode(node *model.Node, inbMap map[string]interface{}) string {
	if node == nil {
		return "direct"
	}

	// 下行 inbound：优先显式 _inbound_role 字段（P1正式方案），tag后缀仅fallback
	if isDownstreamInboundFromMap(inbMap) {
		if node.DownstreamExposureMode != nil && *node.DownstreamExposureMode != "" {
			return *node.DownstreamExposureMode
		}
		if node.ConfigJSON != nil {
			if v, ok := node.ConfigJSON["downstream_exposure_mode"].(string); ok && v != "" {
				return v
			}
		}
		return "direct"
	}

	// 上行/普通 inbound
	if node.ExposureMode != nil && *node.ExposureMode != "" {
		return *node.ExposureMode
	}
	if node.ConfigJSON != nil {
		if v, ok := node.ConfigJSON["exposure_mode"].(string); ok && v != "" {
			return v
		}
	}
	// 历史节点回退：用旧 isCDNNode 逻辑推断
	if isCDNNode(node) {
		return "cdn_saas"
	}
	return "direct"
}

// shouldStripTLSForInbound 根据 inbound 的暴露方式判断是否需要剥离 TLS。
//
// P1-1 收窄：仅 argo_tunnel 返回 true（cloudflared 明文 HTTP 回源，TLS 已被 CF Edge 终止）。
// CDN 节点（cdn/cdn_saas）的 TLS 剥离改由独立的 shouldStripTLSForNginxVhost 判断，
// 避免"改 CDN 剥离逻辑连带影响隧道节点"的耦合风险。
//
// 调用点应改为：
//
//	if shouldStripTLSForArgoTunnel(em) || shouldStripTLSForNginxVhost(em) { ... }
//
// 旧调用点仍可使用 shouldStripTLSForInbound(em)（仅判断 argo_tunnel），
// 但需确认调用点同时检查 shouldStripTLSForNginxVhost。
func shouldStripTLSForInbound(exposureMode string) bool {
	switch exposureMode {
	case "argo_tunnel":
		// 只有隧道节点需要剥离，因为 TLS 已被 CF Edge 终止
		return true
	default:
		// CDN 节点不由此函数管，由 shouldStripTLSForNginxVhost 独立判断
		return false
	}
}

// shouldStripTLSForArgoTunnel 仅判断 argo_tunnel 模式是否需要剥离 TLS。
// argo_tunnel: cloudflared 明文 HTTP 回源，CF Edge 终止 TLS，xray inbound 必须 security=none。
func shouldStripTLSForArgoTunnel(exposureMode string) bool {
	return exposureMode == "argo_tunnel"
}

// shouldStripTLSForNginxVhost 仅判断 cdn/cdn_saas 模式是否需要剥离 TLS。
// CDN 节点的 TLS 终止发生在 nginx 8445 vhost 层（per-domain ACME 证书），
// nginx 做 TLS termination + path 路由后 proxy_pass http 回源，
// xray inbound 必须 security=none。
// 与 argo_tunnel 的剥离触发条件完全独立，分开维护避免交叉影响。
func shouldStripTLSForNginxVhost(exposureMode string) bool {
	switch exposureMode {
	case "cdn", "cdn_saas":
		return true
	default:
		return false
	}
}

// modelNodeToNodeSpecWithCreds 将 DB 节点转为 NodeSpec IR，并注入多用户凭证（P0-1）。
// creds 为该节点的 per-user 凭证列表；为空时回退到单用户（extractCredentials）。
func modelNodeToNodeSpecWithCreds(n *model.Node, creds []*repo.UserNodeCredential) *nodespec.NodeSpec {
	spec := modelNodeToNodeSpec(n)
	if spec == nil {
		return nil
	}
	if len(creds) == 0 {
		return spec
	}
	// 注入多用户 Clients（XBoard 模型：所有协议共用 user.uuid）
	spec.Clients = buildCredentialSpecs(n, creds)

	// 修复：用第一个用户凭证更新 spec.Credentials，确保 L1 Schema 校验通过。
	// extractCredentials 从 config_json 提取凭证，但凭证模型已迁移到 users.uuid（per-user），
	// config_json 中不再存储 uuid/password，导致 spec.Credentials 为空，preflightValidate 失败。
	// XBoard 模型：所有协议共用 user.uuid，取第一个有效凭证作为代表填充 Credentials。
	if creds[0] != nil && creds[0].CredentialValue != "" {
		injectFirstCredentialForValidation(spec, n, creds[0].CredentialValue)
	}
	return spec
}

// injectFirstCredentialForValidation 用第一个用户凭证更新 spec.Credentials，
// 确保 preflightValidate 的 L1 Schema 校验通过。
// 保留 extractCredentials 提取的辅助字段（如 Flow/Method/AlterID），仅填充缺失的 uuid/password。
func injectFirstCredentialForValidation(spec *nodespec.NodeSpec, n *model.Node, value string) {
	switch strings.ToLower(n.ProtocolType) {
	case "vless":
		flow := nodespec.FlowNone
		if existing, ok := spec.Credentials.(nodespec.VLESSCredentials); ok {
			flow = existing.Flow
		} else if n.Flow != nil && *n.Flow != "" {
			flow = nodespec.FlowControl(*n.Flow)
		}
		spec.Credentials = nodespec.VLESSCredentials{UUID: value, Flow: flow}
	case "vmess":
		alterID := 0
		if existing, ok := spec.Credentials.(nodespec.VMessCredentials); ok {
			alterID = existing.AlterID
		}
		spec.Credentials = nodespec.VMessCredentials{UUID: value, AlterID: alterID}
	case "trojan":
		spec.Credentials = nodespec.TrojanCredentials{Password: value}
	case "shadowsocks", "ss":
		method := ""
		if existing, ok := spec.Credentials.(nodespec.ShadowsocksCredentials); ok {
			method = existing.Method
		}
		if method == "" {
			if v, ok := getStringFromNodeConfig(n, "method"); ok {
				method = v
			}
		}
		spec.Credentials = nodespec.ShadowsocksCredentials{Password: value, Method: method}
	case "hysteria2":
		spec.Credentials = nodespec.Hysteria2Credentials{Password: value}
	case "tuic":
		spec.Credentials = nodespec.TUICCredentials{UUID: value}
	case "anytls":
		spec.Credentials = nodespec.AnyTLSCredentials{Password: value}
	}
}

// buildCredentialSpecs 根据 protocol 将 user.uuid 列表转换为 CredentialSpec 数组。
// 修复：按 email 去重，避免同一用户因多套餐订阅在同一节点重复出现导致 xray 报错 "User already exists"。
func buildCredentialSpecs(n *model.Node, creds []*repo.UserNodeCredential) []nodespec.CredentialSpec {
	method := ""
	if v, ok := getStringFromNodeConfig(n, "method"); ok {
		method = v
	}
	// flow 从 node.Flow 或 config_json 读取
	flow := nodespec.FlowNone
	if n.Flow != nil && *n.Flow != "" {
		flow = nodespec.FlowControl(*n.Flow)
	} else if f, ok := getStringFromNodeConfig(n, "flow"); ok && f != "" {
		flow = nodespec.FlowControl(f)
	}

	// 按 email 去重：同一用户购买多个包含同一节点的套餐时，只保留第一条凭证
	seen := make(map[string]bool, len(creds))
	clients := make([]nodespec.CredentialSpec, 0, len(creds))
	for _, c := range creds {
		if c.CredentialValue == "" {
			continue
		}
		email := c.UserID.String()
		if seen[email] {
			continue // 跳过重复用户
		}
		seen[email] = true

		cs := nodespec.CredentialSpec{
			Email: email,
			Level: 0,
		}
		switch strings.ToLower(n.ProtocolType) {
		case "vless", "vmess", "tuic":
			cs.UUID = c.CredentialValue
			if strings.ToLower(n.ProtocolType) == "vless" && strings.ToLower(n.TransportType) == "tcp" {
				cs.Flow = flow
			}
		case "trojan", "shadowsocks", "ss", "hysteria2", "anytls":
			cs.Password = c.CredentialValue
			if strings.ToLower(n.ProtocolType) == "shadowsocks" || strings.ToLower(n.ProtocolType) == "ss" {
				cs.Method = method
			}
		default:
			// 未知协议：用 UUID 兜底
			cs.UUID = c.CredentialValue
		}
		// P0: 注入 per-user 限速/设备限制（从 subscription/plan JOIN 获取）
		if c.SpeedLimitMbps > 0 {
			cs.SpeedLimit = c.SpeedLimitMbps
		}
		if c.DeviceLimit > 0 {
			cs.DeviceLimit = c.DeviceLimit
		}
		clients = append(clients, cs)
	}
	return clients
}

// buildXrayConfigViaKernelRender 用 kernelrender 生成 Xray 服务端配置（P0-1）。
// 替代 exposure.RenderXrayConfigWithCreds，统一走 IR→Compiler 链路。
// P2-2: 渲染前应用能力降级策略（ApplyDegrade），实际改写 NodeSpec。
func (s *DeploymentService) buildXrayConfigViaKernelRender(
	ctx context.Context,
	nodes []*model.Node,
	listenHost string,
	creds exposure.NodeCredentials,
) (map[string]interface{}, error) {
	return s.buildConfigViaKernelRender(ctx, "xray", kernelrender.KernelXray, nodes, listenHost, creds)
}

// buildSingboxConfigViaKernelRender 用 kernelrender 生成 sing-box 服务端配置（P0-1）。
// 替代 exposure.RenderSingBoxConfigWithCreds，统一走 IR→Compiler 链路。
// P2-2: 渲染前应用能力降级策略（ApplyDegrade），实际改写 NodeSpec。
func (s *DeploymentService) buildSingboxConfigViaKernelRender(
	ctx context.Context,
	nodes []*model.Node,
	listenHost string,
	creds exposure.NodeCredentials,
) (map[string]interface{}, error) {
	return s.buildConfigViaKernelRender(ctx, "sing-box", kernelrender.KernelSingBox, nodes, listenHost, creds)
}

// buildConfigViaKernelRender P2-2 统一渲染入口：应用降级 → 渲染 → 持久化降级事件。
// kernelName: "xray" / "sing-box"（用于能力矩阵查询）
// renderKernel: kernelrender.KernelXray / kernelrender.KernelSingBox
func (s *DeploymentService) buildConfigViaKernelRender(
	ctx context.Context,
	kernelName string,
	renderKernel kernelrender.KernelType,
	nodes []*model.Node,
	listenHost string,
	creds exposure.NodeCredentials,
) (map[string]interface{}, error) {
	inbounds := make([]interface{}, 0)
	hasNodes := false
	var degradeEvents []CapabilityLostEvent
	// P1-3: 收集所有节点渲染器生成的 policy levels（保留 up_mbps/down_mbps 限速字段）
	collectedPolicyLevels := make(map[string]interface{})

	// P-Chain: 链式套娃出站收集（per-node，在节点循环内填充）
	// 套娃 outbound = 节点入站流量经此代理出站；健康降级用 balancer(xray)/urltest(sing-box)
	//
	// 两种输入方式兼容并存：
	//  1. chain_outbound_uri (URI 解析)：自动解析协议参数→构建 outbound + balancer/urltest 降级 + routing
	//     适合快速套娃（socks5://... trojan://...），自动生成健康降级，用户无需手写 JSON
	//  2. custom_outbounds + custom_routes (JSON 直接注入)：原样注入当前内核配置
	//     适合高级用户（xboard 兼容），用户按目标内核格式手填 JSON，无自动降级
	var chainOutbounds []interface{}         // 套娃出站（URI 方式：xray 或 sing-box 格式，按当前内核构建）
	var chainRoutingRules []interface{}      // 套娃路由规则（URI 方式：inboundTag→balancer/outbound）
	var chainBalancers []interface{}         // xray balancer（URI 方式：候选 [chainTag, direct]，leastPing 降级）
	var chainURLTests []interface{}          // sing-box urltest outbound（URI 方式：候选 [chainTag, direct]，30s 探测）
	var chainTags []string                   // 套娃出站 tag 列表（URI 方式：xray observatory subjectSelector 用）
	var customOutbounds []interface{}        // 自定义出站（JSON 方式：原样注入，用户自管内核格式，无降级）
	var customRoutingRules []interface{}     // 自定义路由规则（JSON 方式：原样注入，用户自管格式）
	// P-Chain-Bridge: sing-box 桥接（自签证书/insecure 场景）
	// 当 xray runtime 下 chain URI 含 insecure=1 时，xray 的 pinnedPeerCertSha256 对无 SAN 证书无效。
	// 自动用 sing-box 桥接（支持 insecure:true），xray chain outbound 改为 socks5 指向本地桥接端口。
	// 桥接配置注入 _chain_bridges 字段，node-agent 解析后启动辅助 sing-box 实例。
	var chainBridgeInbounds []interface{}  // sing-box 桥接 inbounds（socks5 监听端口）
	var chainBridgeOutbounds []interface{} // sing-box 桥接 outbounds（原始协议+insecure）
	var chainBridgeRules []interface{}     // sing-box 桥接 route rules（inbound→outbound）

	strategy := s.degradeStrategy
	if strategy == "" {
		strategy = StrategyDeny
	}

	for _, node := range nodes {
		if !node.IsEnabled {
			continue
		}
		hasNodes = true
		// B11: 从 cert_bundles 表注入证书 PEM（TLS 节点渲染需要）
		s.injectCertFromBundle(ctx, node)
		nodeCreds := creds[node.ID]
		spec := modelNodeToNodeSpecWithCreds(node, nodeCreds)
		if spec == nil {
			continue
		}

		// P2-2: 渲染前应用能力降级（改写 spec 原地）
		events, newKernel, err := ApplyDegrade(ctx, strategy, kernelName, spec, s.capRepo, node.ID, node.Code)
		if err != nil {
			// preflight 已校验，此处不应出错；出错则跳过该节点并记录警告
			s.logger.Warn("render-time degrade failed, skipping node",
				"node_code", node.Code, "kernel", kernelName, "error", err)
			continue
		}
		degradeEvents = append(degradeEvents, events...)

		// force_kernel 切换了内核：当前内核无法渲染，跳过
		if newKernel != kernelName {
			s.logger.Info("node force-switched to different kernel, skipping on current kernel",
				"node_code", node.Code, "from", kernelName, "to", newKernel)
			continue
		}

		rendered, err := kernelrender.RenderForKernel(renderKernel, spec)
		if err != nil {
			// 防御性处理：UnsupportedFeatureError 表示当前内核不支持该特性，
			// 跳过该节点（通常该节点已被 ensureXrayRuntimeForNode 切换到 xray runtime，
			// 此处仅作为最后安全网，避免历史数据/竞态条件导致整个配置下发失败）
			var ufErr *kernelrender.UnsupportedFeatureError
			if errors.As(err, &ufErr) {
				slog.Warn("skipping node due to unsupported feature (defensive fallback)",
					"node_code", node.Code, "kernel", kernelName,
					"feature", ufErr.Feature, "hint", ufErr.Hint)
				continue
			}
			return nil, fmt.Errorf("节点 %s 编译失败: %w", node.Code, err)
		}
		// P1-3: 收集渲染器生成的 policy levels（包含 up_mbps/down_mbps 限速字段）
		if kernelName == "xray" {
			if policy, ok := rendered["policy"].(map[string]interface{}); ok {
				if levels, ok := policy["levels"].(map[string]interface{}); ok {
					for lvlKey, lvlVal := range levels {
						if lvlMap, ok := lvlVal.(map[string]interface{}); ok {
							if existing, ok := collectedPolicyLevels[lvlKey].(map[string]interface{}); ok {
								// 合并：保留限速字段，补充统计字段
								for k, v := range lvlMap {
									existing[k] = v
								}
							} else {
								collectedPolicyLevels[lvlKey] = lvlMap
							}
						}
					}
				}
			}
		}
		// 提取 inbounds，保留渲染器 resolveListenAddress 的结果
		if inbList, ok := rendered["inbounds"].([]interface{}); ok {
			for _, inb := range inbList {
				if inbMap, ok := inb.(map[string]interface{}); ok {
					// listen 地址优先级：
					// 1. runtime 显式配置 ListenHost（如 Tunnel 节点需要 "::" 支持 cloudflared [::1] 连接）
					// 2. 渲染器 resolveListenAddress 结果（已正确实现）：
					//    - CDN/Tunnel/Direct-TCP（ServerPort > 0 && ServerPort != Port）→ 127.0.0.1
					//    - DIRECT/UDP 直连（ServerPort == 0 或 ServerPort == Port）→ 0.0.0.0
					// 不再强制覆盖为 "::"，避免 DIRECT 节点经 nginx SNI 代理时暴露内核端口到公网
					if listenHost != "" {
						inbMap["listen"] = listenHost
					}
					// R2: CDN 节点原始 inbound 去 TLS 化（不再创建镜像 inbound）
				// CDN 架构：client → CDN(443/TLS) → nginx(终止TLS, proxy_pass http) → xray(无TLS)
				// nginx 已终止 TLS，xray inbound 不需要 TLS/REALITY，直接 security=none。
				// 之前创建镜像 inbound（port+1000）导致端口冲突和 nginx 端口不匹配，
				// 现在直接修改原始 inbound：去 TLS，listen=127.0.0.1，nginx proxy_pass 指向原始端口。
				// argo_tunnel 节点例外：cloudflared 通过 [::1] 回源，listen 必须为 "::"
				//
				// P1 inbound 级 TLS 剥离：每个 inbound 根据自身暴露方式判定，不再节点级一刀切。
				// 判定逻辑：determineInboundExposureMode(node, inbMap) 返回单个 inbound 的暴露方式，
				// shouldStripTLSForInbound(em) 决定是否剥离。
				// - 上行 inbound（cdn_saas/argo_tunnel）→ 剥离 TLS
				// - 下行 inbound（显式 _inbound_role="downstream"，默认 direct/reality）→ 保留 TLS/REALITY
				// - 纯直连节点 → 不剥离
				//
				// is_split_mode 字段【不】参与此处判定（防止状态源双轨）。
				// 阶段3已完成退役：下行 inbound 身份识别由 _inbound_role 显式字段承担，
				// tag 后缀仅作展示命名，不参与安全判定路径。
				em := determineInboundExposureMode(node, inbMap)
				// P1-1: argo_tunnel 和 cdn/cdn_saas 分别独立判断，避免耦合
				if shouldStripTLSForArgoTunnel(em) || shouldStripTLSForNginxVhost(em) {
					if kernelName == "xray" {
						stripTLSFromXrayInbound(inbMap, node)
					} else {
						stripTLSFromSingboxInbound(inbMap)
					}
				}
				inbounds = append(inbounds, inbMap)
				}
			}
		}

		// P-Chain: 收集套娃出站（per-node，嵌入现有节点循环，复用 node 上下文）。
		// 节点 config_json.chain_outbound_uri 非空 → 解析为 NodeSpec → 按内核构建套娃 outbound + 降级 balancer。
		// 套娃失败仅跳过该节点套娃，不影响节点本身渲染（D7 健康降级：上游失效自动切 direct）。
		if node.ConfigJSON != nil {
			if uri, _ := node.ConfigJSON["chain_outbound_uri"].(string); uri != "" {
				chainSpec, err := exposure.ParseChainURI(uri)
				if err != nil {
					s.logger.Warn("parse chain_outbound_uri failed, skipping chain for node",
						"node_code", node.Code, "error", err)
				} else if chainSpec != nil {
					chainTag := fmt.Sprintf("chain-%s", node.Code)
					inboundTag := fmt.Sprintf("in-%s", node.Code)
					chainSpec.Code = chainTag
					chainTags = append(chainTags, chainTag)

					if kernelName == "xray" {
					// P-Chain-Bridge: 自签证书/insecure 自动 sing-box 桥接
					// xray 26.3.27 移除 allowInsecure，pinnedPeerCertSha256 对无 SAN 证书无效。
					// 当 URI 含 insecure=1 时，自动用 sing-box 桥接（支持 insecure:true），
					// xray chain outbound 改为 socks5 指向本地 sing-box 桥接端口。
					// 零 SSH 方案：面板填入原始 URI（含 insecure=1）→ 自动生成桥接 → 无需手动部署。
					needsBridge := chainSpec.TLS != nil && chainSpec.TLS.AllowInsecure
					if needsBridge {
						sbOb, sbErr := exposure.BuildSingboxOutboundFromNodeSpec(chainSpec, chainTag, "")
						if sbErr != nil {
							s.logger.Warn("build singbox bridge outbound failed, falling back to xray",
								"node_code", node.Code, "error", sbErr)
							needsBridge = false
						} else {
							port := allocChainBridgePort(node.Code)
							bridgeInTag := fmt.Sprintf("bridge-in-%d", port)
							chainBridgeInbounds = append(chainBridgeInbounds, map[string]interface{}{
								"type":        "socks",
								"tag":         bridgeInTag,
								"listen":      "127.0.0.1",
								"listen_port": port,
							})
							chainBridgeOutbounds = append(chainBridgeOutbounds, sbOb)
							chainBridgeRules = append(chainBridgeRules, map[string]interface{}{
								"action":   "route",
								"inbound":  []string{bridgeInTag},
								"outbound": chainTag,
							})
							// xray chain outbound = socks5 指向本地 sing-box 桥接
							ob := map[string]interface{}{
								"tag":      chainTag,
								"protocol": "socks",
								"settings": map[string]interface{}{
									"servers": []interface{}{
										map[string]interface{}{
											"address": "127.0.0.1",
											"port":    port,
										},
									},
								},
							}
							chainOutbounds = append(chainOutbounds, ob)
						// D7-fix: 直接用 outboundTag 路由到 chain，不走 balancer 降级。
						// leastPing + [chain, direct] 会导致永远走 direct（direct 延迟永远最低）。
						// 套娃目的就是走 chain，chain 挂了节点不可用，客户端应切换其他节点。
						chainRoutingRules = append(chainRoutingRules, map[string]interface{}{
							"type":        "field",
							"inboundTag":  []string{inboundTag},
							"outboundTag": chainTag,
						})
						s.logger.Info("chain bridge created (sing-box for insecure TLS)",
							"node_code", node.Code, "bridge_port", port, "chain_tag", chainTag)
						}
					}
					if !needsBridge {
						ob, err := exposure.BuildXrayOutboundFromNodeSpec(chainSpec, chainTag, "")
						if err != nil {
							s.logger.Warn("build xray chain outbound failed",
								"node_code", node.Code, "error", err)
						} else {
						chainOutbounds = append(chainOutbounds, ob)
						// D7-fix: 直接用 outboundTag 路由到 chain，不走 balancer 降级。
						chainRoutingRules = append(chainRoutingRules, map[string]interface{}{
							"type":        "field",
							"inboundTag":  []string{inboundTag},
							"outboundTag": chainTag,
						})
					}
					}
				} else {
					ob, err := exposure.BuildSingboxOutboundFromNodeSpec(chainSpec, chainTag, "")
					if err != nil {
						s.logger.Warn("build singbox chain outbound failed",
							"node_code", node.Code, "error", err)
					} else {
						chainOutbounds = append(chainOutbounds, ob)
						// D7-fix: 直接路由到 chain，不走 urltest 降级。
						// sing-box 1.12+ 规则格式：含 action 字段（与 renderSingBoxRoute 对齐）
						chainRoutingRules = append(chainRoutingRules, map[string]interface{}{
							"action":   "route",
							"inbound":  []string{inboundTag},
							"outbound": chainTag,
						})
					}
				}
				}
			}

			// P-Chain (JSON 方式): custom_outbounds + custom_routes 直接注入（xboard 兼容）。
			// 与 chain_outbound_uri (URI 方式) 并存：两者可同时使用，各自独立注入。
			// custom_outbounds: JSON 数组，原样追加到当前内核 outbounds（用户按目标内核格式填写）
			// custom_routes:    JSON 数组，原样前插到当前内核 routing.rules（用户按目标内核格式填写）
			// 注意：JSON 方式无自动 balancer/urltest 降级，用户需自行管理路由规则和出站 tag。
			if cobs := parseCustomJSONArray(node.ConfigJSON["custom_outbounds"]); len(cobs) > 0 {
				customOutbounds = append(customOutbounds, cobs...)
				s.logger.Info("custom_outbounds injected",
					"node_code", node.Code, "kernel", kernelName, "count", len(cobs))
			}
			if crs := parseCustomJSONArray(node.ConfigJSON["custom_routes"]); len(crs) > 0 {
				customRoutingRules = append(customRoutingRules, crs...)
				s.logger.Info("custom_routes injected",
					"node_code", node.Code, "kernel", kernelName, "count", len(crs))
			}

			// P-Chain (父节点方式): parent_node_id → 查询父节点 → 构建 outbound + routing 规则。
			// 与 chain_outbound_uri/custom_outbounds 并存。
			// 设计参考：Xray 用 proxySettings.tag 串联，Sing-box 用 detour 字段。
			// 多节点场景下用 routing 规则（inboundTag→outboundTag）精确路由到各自 parent outbound，
			// 避免 direct outbound 只能持有一个 proxySettings.tag 的局限。
			// parent outbound 无 balancer/urltest 降级（父节点失效则该节点不可用，符合中转语义）。
			if parentIDStr, _ := node.ConfigJSON["parent_node_id"].(string); parentIDStr != "" {
				parentUUID, err := uuid.Parse(parentIDStr)
				if err != nil {
					s.logger.Warn("invalid parent_node_id UUID, skipping parent chain",
						"node_code", node.Code, "parent_node_id", parentIDStr, "error", err)
				} else {
					parentNode, perr := s.nodeRepo.GetByID(ctx, parentUUID)
					if perr != nil || parentNode == nil {
						s.logger.Warn("parent node not found, skipping parent chain",
							"node_code", node.Code, "parent_node_id", parentIDStr, "error", perr)
					} else {
						parentSpec := modelNodeToNodeSpec(parentNode)
						if parentSpec != nil {
							parentTag := fmt.Sprintf("parent-%s", parentNode.Code)
							parentSpec.Code = parentTag
							inboundTag := fmt.Sprintf("in-%s", node.Code)

							if kernelName == "xray" {
								ob, oerr := exposure.BuildXrayOutboundFromNodeSpec(parentSpec, parentTag, "")
								if oerr != nil {
									s.logger.Warn("build xray parent outbound failed",
										"node_code", node.Code, "parent_code", parentNode.Code, "error", oerr)
								} else {
									chainOutbounds = append(chainOutbounds, ob)
									// routing 规则：节点 inbound → parent outbound（直接路由，无 balancer 降级）
									chainRoutingRules = append(chainRoutingRules, map[string]interface{}{
										"type":        "field",
										"inboundTag":  []string{inboundTag},
										"outboundTag": parentTag,
									})
									s.logger.Info("parent_node chain injected (xray)",
										"node_code", node.Code, "parent_code", parentNode.Code,
										"parent_tag", parentTag, "inbound_tag", inboundTag)
								}
							} else {
								ob, oerr := exposure.BuildSingboxOutboundFromNodeSpec(parentSpec, parentTag, "")
								if oerr != nil {
									s.logger.Warn("build singbox parent outbound failed",
										"node_code", node.Code, "parent_code", parentNode.Code, "error", oerr)
								} else {
									chainOutbounds = append(chainOutbounds, ob)
									// sing-box 1.12+ 规则格式：含 action 字段
									chainRoutingRules = append(chainRoutingRules, map[string]interface{}{
										"action":   "route",
										"inbound":  []string{inboundTag},
										"outbound": parentTag,
									})
									s.logger.Info("parent_node chain injected (sing-box)",
										"node_code", node.Code, "parent_code", parentNode.Code,
										"parent_tag", parentTag, "inbound_tag", inboundTag)
								}
							}
						}
					}
				}
			}
		}
	}

	if !hasNodes {
		return nil, fmt.Errorf("没有启用的节点")
	}

	// P0-4: 聚合后按 tag 去重 inbounds。
	// kernelrender.RenderForKernel 对每个节点都会调用 ensureAPIInbound 注入 tag="api"，
	// Machine 模式多节点聚合后会出现多个相同 tag 的 inbound，导致 xray 启动报错
	// "existing tag found: api"。去重策略：保留第一个出现的 tag，丢弃后续重复条目。
	{
		seen := make(map[string]bool)
		deduped := make([]interface{}, 0, len(inbounds))
		for _, inb := range inbounds {
			m, ok := inb.(map[string]interface{})
			if !ok {
				deduped = append(deduped, inb)
				continue
			}
			tag, _ := m["tag"].(string)
			if tag == "" {
				deduped = append(deduped, inb)
				continue
			}
			if seen[tag] {
				continue
			}
			seen[tag] = true
			deduped = append(deduped, inb)
		}
		inbounds = deduped
	}

	// P2-2: 持久化降级事件到 capability_lost_events 表
	s.persistDegradeEvents(ctx, degradeEvents)

	if kernelName == "xray" {
		// 添加 API inbound（dokodemo-door），供 node-agent 通过 gRPC 访问 StatsService 流量统计。
		// 先检查 per-node 渲染器（kernelrender.ensureAPIInbound）是否已注入了 api inbound，
		// 避免重复注入导致 xray 启动报错 "existing tag found: api"。
		hasAPIInbound := false
		for _, inb := range inbounds {
			if m, ok := inb.(map[string]interface{}); ok {
				if tag, _ := m["tag"].(string); tag == "api" {
					hasAPIInbound = true
					break
				}
			}
		}
		if !hasAPIInbound {
			apiInbound := map[string]interface{}{
				"listen":   "127.0.0.1",
				"port":     10085,
				"protocol": "dokodemo-door",
				"settings": map[string]interface{}{
					"address": "127.0.0.1",
				},
				"tag": "api",
			}
			inbounds = append(inbounds, apiInbound)
		}

		// 渲染路由策略（如果注入了 routingRenderer 且节点有绑定的策略）
		xrayRouting, xrayBalancers := s.renderXrayRouting(ctx, nodes)

		// S8: 注入审计路由规则（BT 阻断 + SSRF 防护），按 AuditConfig 控制启用/禁用
		kernelrender.InjectAuditRules(xrayRouting, s.getAuditConfig())

		// P1-3: 合并 policy levels：以渲染器生成的限速配置为基础，补充统计字段
		// xray 的 policy 系统要求显式列出每个 level，否则 ForLevel() 返回
		// SessionDefault()（Stats.UserUplink=false），导致流量统计被静默丢弃。
		// 同时保留渲染器设置的 up_mbps/down_mbps 限速字段，实现真正的带宽控制。
		policyLevels := make(map[string]interface{})
		// Level 0 默认配置：不限速 + 开启统计
		policyLevels["0"] = map[string]interface{}{
			"bufferSize":        1024,
			"statsUserUplink":   true,
			"statsUserDownlink": true,
			"statsUserOnline":   true,
		}
		// 先合并从渲染器收集到的 levels（包含限速字段）
		for lvlKey, lvlVal := range collectedPolicyLevels {
			if lvlMap, ok := lvlVal.(map[string]interface{}); ok {
				merged := make(map[string]interface{})
				// 默认开启统计
				merged["statsUserUplink"] = true
				merged["statsUserDownlink"] = true
				merged["statsUserOnline"] = true
				merged["bufferSize"] = 1024
				// 合并渲染器设置的字段（如 up_mbps/down_mbps 限速）
				for k, v := range lvlMap {
					merged[k] = v
				}
				policyLevels[lvlKey] = merged
			}
		}
		// 扫描所有 inbound clients，确保每个 level 都在 policy 中（处理 CDN 镜像等新增 clients）
		for _, inb := range inbounds {
			inbMap, ok := inb.(map[string]interface{})
			if !ok {
				continue
			}
			settings, _ := inbMap["settings"].(map[string]interface{})
			if settings == nil {
				continue
			}
			clients, _ := settings["clients"].([]interface{})
			for _, c := range clients {
				client, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				// xray 渲染器设置的 level 是 int（sl.levelForClient 返回 int），
				// 但 JSON 反序列化后可能是 float64，这里兼容 int/int64/float64
				var levelInt int
				switch v := client["level"].(type) {
				case float64:
					levelInt = int(v)
				case int:
					levelInt = v
				case int64:
					levelInt = int(v)
				default:
					continue
				}
				levelStr := fmt.Sprintf("%d", levelInt)
				if _, exists := policyLevels[levelStr]; !exists {
					// 新增 level：默认开启统计（无特殊限速配置时不限速）
					policyLevels[levelStr] = map[string]interface{}{
						"bufferSize":        1024,
						"statsUserUplink":   true,
						"statsUserDownlink": true,
						"statsUserOnline":   true,
					}
				}
			}
		}

		// P-Chain: 插入路由规则
		// chainRoutingRules 前插：带 inboundTag 精确匹配（in-P5 等），不会误伤 api inbound，安全前置
		// customRoutingRules 后插：用户自定义规则可能不含 inboundTag（如无条件 final 规则），
		//   放在默认规则（api + block private）之后作为兜底，避免误伤系统流量
		if len(chainRoutingRules) > 0 || len(customRoutingRules) > 0 {
			rules, _ := xrayRouting["rules"].([]interface{})
			newRules := make([]interface{}, 0, len(rules)+len(chainRoutingRules)+len(customRoutingRules))
			newRules = append(newRules, chainRoutingRules...)  // 前插：套娃路由（inboundTag 精确匹配）
			newRules = append(newRules, rules...)               // 中间：默认规则（api + block private）
			newRules = append(newRules, customRoutingRules...)  // 后插：用户自定义兜底
			xrayRouting["rules"] = newRules
		}

		// P-Chain: 组装 xray outbounds（默认 direct/block/api + 套娃出站 + 自定义出站）
		xrayOutbounds := defaultXrayOutbounds()
		for _, ob := range chainOutbounds {
			if m, ok := ob.(map[string]interface{}); ok {
				xrayOutbounds = append(xrayOutbounds, m)
			}
		}
		// custom_outbounds 原样追加（xboard 兼容：用户按 xray 格式填写，直接注入）
		for _, ob := range customOutbounds {
			if m, ok := ob.(map[string]interface{}); ok {
				xrayOutbounds = append(xrayOutbounds, m)
			}
		}

		config := map[string]interface{}{
			"log": map[string]interface{}{"loglevel": "warning"},
			"api": map[string]interface{}{
				"tag":      "api",
				"services": []string{"HandlerService", "LoggerService", "StatsService"},
			},
			"stats": map[string]interface{}{},
			"policy": map[string]interface{}{
				"levels": policyLevels,
			},
			"inbounds":  inbounds,
			"outbounds": xrayOutbounds,
			"routing":   xrayRouting,
		}
		// P-Chain: 合并 balancers 到 routing.balancers
		// 重要：xray 要求 balancers 在 routing 内部（routing.balancers），不是顶层 config["balancers"]。
		// 之前放顶层导致 xray 报错 "app/router: balancer balancer-P5 not found"（router 找不到 balancer）。
		// xrayBalancers 类型为 []map[string]interface{}，chainBalancers 类型为 []interface{}，统一转为 []interface{}
		allBalancers := make([]interface{}, 0, len(xrayBalancers)+len(chainBalancers))
		for _, b := range xrayBalancers {
			allBalancers = append(allBalancers, b)
		}
		for _, b := range chainBalancers {
			if m, ok := b.(map[string]interface{}); ok {
				allBalancers = append(allBalancers, m)
			}
		}
		if len(allBalancers) > 0 {
			xrayRouting["balancers"] = allBalancers
		}
		// P-Chain: observatory 健康探测（leastPing balancer 依赖此组件）
		// probeURL 使用 google.com 而非 gstatic.com：gstatic 经部分上游出口（如土耳其）TLS 握手后超时，
		// 导致 observatory 误判 chain outbound dead → balancer 降级 direct → 套娃失效。
		if len(chainTags) > 0 {
			config["observatory"] = map[string]interface{}{
				"subjectSelector": chainTags,
				"probeInterval":   "30s",
				"probeURL":        "https://www.google.com/generate_204",
			}
		}
		// P-Chain-Bridge: 注入 sing-box 桥接配置（自签证书/insecure 场景）
		// node-agent 解析 _chain_bridges 字段，启动辅助 sing-box 实例处理自签证书 TLS。
		// xray chain outbound 为 socks5 指向本地桥接端口，sing-box 桥接用 insecure:true 连接上游。
		if len(chainBridgeInbounds) > 0 {
			chainBridgeOutbounds = append(chainBridgeOutbounds,
				map[string]interface{}{"type": "direct", "tag": "direct"})
			config["_chain_bridges"] = map[string]interface{}{
				"log":       map[string]interface{}{"level": "warn"},
				"inbounds":  chainBridgeInbounds,
				"outbounds": chainBridgeOutbounds,
				"route":     map[string]interface{}{"rules": chainBridgeRules},
			}
			s.logger.Info("chain bridges injected",
				"bridge_count", len(chainBridgeInbounds), "kernel", "xray")
		}
		return config, nil
	}

	// sing-box
	sbRoute := s.renderSingBoxRoute(ctx, nodes)

	// P-Chain: 插入路由规则（sing-box 1.12+ action:route 格式）
	// chainRoutingRules 前插：带 inbound 精确匹配，安全前置
	// customRoutingRules 后插：用户自定义规则放默认规则之后作为兜底，避免误伤系统流量
	if len(chainRoutingRules) > 0 || len(customRoutingRules) > 0 {
		rules, _ := sbRoute["rules"].([]interface{})
		newRules := make([]interface{}, 0, len(rules)+len(chainRoutingRules)+len(customRoutingRules))
		newRules = append(newRules, chainRoutingRules...)  // 前插：套娃路由（inbound 精确匹配）
		newRules = append(newRules, rules...)               // 中间：默认规则
		newRules = append(newRules, customRoutingRules...)  // 后插：用户自定义兜底
		sbRoute["rules"] = newRules
	}

	// P-Chain: 组装 sing-box outbounds（默认 direct/block + 套娃出站 + urltest 降级组 + 自定义出站）
	sbOutbounds := []interface{}{
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	}
	for _, ob := range chainOutbounds {
		sbOutbounds = append(sbOutbounds, ob)
	}
	for _, ut := range chainURLTests {
		sbOutbounds = append(sbOutbounds, ut)
	}
	// custom_outbounds 原样追加（xboard 兼容：用户按 sing-box 格式填写，直接注入）
	for _, ob := range customOutbounds {
		sbOutbounds = append(sbOutbounds, ob)
	}

	return map[string]interface{}{
		"log":       map[string]interface{}{"level": "warn"},
		"inbounds":  inbounds,
		"outbounds": sbOutbounds,
		"route":     sbRoute,
	}, nil
}

// allocChainBridgePort 基于节点 code 分配稳定的本地桥接端口（11100-11199）。
// 同一节点每次渲染都分配相同端口，确保 xray socks5 outbound 指向正确。
// 端口范围 11100-11199 与节点服务端口（8xxx/9xxx）和 sing-box Clash API 端口不冲突。
func allocChainBridgePort(nodeCode string) int {
	h := sha256.Sum256([]byte("chain-bridge:" + nodeCode))
	return 11100 + int(h[0])%100
}

// persistDegradeEvents P2-2: 将降级事件批量写入 capability_lost_events 表。
// 写入失败不阻断发布（仅记录警告）。
func (s *DeploymentService) persistDegradeEvents(ctx context.Context, events []CapabilityLostEvent) {
	if len(events) == 0 || s.capRepo == nil {
		return
	}
	for _, ev := range events {
		rec := &repo.CapabilityLostEventRecord{
			RuntimeID:       ev.RuntimeID,
			NodeID:          ev.NodeID,
			NodeCode:        ev.NodeCode,
			Kernel:          ev.Kernel,
			Protocol:        ev.Protocol,
			Transport:       ev.Transport,
			Security:        ev.Security,
			Feature:         ev.Feature,
			OriginalSupport: ev.OriginalSupport,
			DegradeStrategy: string(ev.DegradeStrategy),
			DowngradeTo:     ev.DowngradeTo,
			Message:         ev.Message,
			ConfigVersionNo: ev.ConfigVersionNo,
		}
		if err := s.capRepo.InsertCapabilityLostEvent(ctx, rec); err != nil {
			s.logger.Warn("failed to persist capability lost event",
				"node_code", ev.NodeCode, "error", err)
		}
	}
	s.logger.Info("capability lost events persisted", "count", len(events))
}

// parseCustomJSONArray 解析 config_json 中的自定义 JSON 数组字段（custom_outbounds / custom_routes）。
// 兼容两种前端存储格式：
//   - []interface{}：前端已 JSON.parse 为数组对象（正常路径，Nodes.tsx L821-825）
//   - string：前端传入 JSON 字符串（兜底，Textarea 原始值未解析时）
//
// 返回 nil 表示无有效数据（空数组、解析失败、类型不匹配）。
func parseCustomJSONArray(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		if len(val) == 0 {
			return nil
		}
		return val
	case string:
		if val == "" {
			return nil
		}
		var arr []interface{}
		if err := json.Unmarshal([]byte(val), &arr); err != nil {
			return nil
		}
		if len(arr) == 0 {
			return nil
		}
		return arr
	}
	return nil
}

// defaultXrayOutbounds 默认 Xray outbounds（不依赖 exposure 包，P0-1 解耦）
func defaultXrayOutbounds() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"protocol": "freedom",
			"tag":      "direct",
		},
		{
			"protocol": "blackhole",
			"tag":      "block",
		},
		{
			"protocol": "blackhole",
			"tag":      "api",
		},
	}
}

// defaultXrayRouting 默认 Xray routing（不依赖 exposure 包，P0-1 解耦）
// S8: 私有 IP 阻断规则已从硬编码移除，由 InjectAuditRules 按 AuditConfig 动态注入。
func defaultXrayRouting() map[string]interface{} {
	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"inboundTag":  []string{"api"},
				"outboundTag": "api",
			},
		},
	}
}

// renderXrayRouting 渲染 xray routing 配置。
// 如果注入了 routingRenderer 且节点有绑定的路由策略，返回策略渲染结果 + balancers；
// 否则回退到 defaultXrayRouting()。
func (s *DeploymentService) renderXrayRouting(ctx context.Context, nodes []*model.Node) (map[string]interface{}, []map[string]interface{}) {
	if s.routingRenderer == nil || len(nodes) == 0 {
		return defaultXrayRouting(), nil
	}

	// 取第一个节点（同一 Agent 通常只有一个节点，多节点时取第一个的路由策略）
	rendered, err := s.routingRenderer.RenderRouting(ctx, nodes[0].ID)
	if err != nil {
		s.logger.Warn("failed to render routing, falling back to default", "node_id", nodes[0].ID, "error", err)
		return defaultXrayRouting(), nil
	}

	// 如果没有绑定的策略（rules 为空），回退到默认
	if len(rendered.Xray.Rules) == 0 {
		return defaultXrayRouting(), nil
	}

	// 合并默认规则（api + block private）与策略规则
	defaultRules := defaultXrayRouting()["rules"].([]interface{})
	allRules := make([]interface{}, 0, len(defaultRules)+len(rendered.Xray.Rules))
	allRules = append(allRules, defaultRules...)
	for _, r := range rendered.Xray.Rules {
		allRules = append(allRules, r)
	}

	routing := map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          allRules,
	}

	// 转换 balancers
	var balancers []map[string]interface{}
	for _, b := range rendered.Xray.Balancers {
		balancers = append(balancers, b)
	}

	return routing, balancers
}

// renderSingBoxRoute 渲染 sing-box route 配置。
// 如果注入了 routingRenderer 且节点有绑定的路由策略，返回策略渲染结果；
// 否则回退到默认（仅 ip_is_private → block）。
func (s *DeploymentService) renderSingBoxRoute(ctx context.Context, nodes []*model.Node) map[string]interface{} {
	defaultRules := []interface{}{
		map[string]interface{}{
			"action":        "route",
			"ip_is_private": true,
			"outbound":      "block",
		},
	}

	if s.routingRenderer == nil || len(nodes) == 0 {
		return map[string]interface{}{"rules": defaultRules}
	}

	rendered, err := s.routingRenderer.RenderRouting(ctx, nodes[0].ID)
	if err != nil {
		s.logger.Warn("failed to render sing-box route, falling back to default", "node_id", nodes[0].ID, "error", err)
		return map[string]interface{}{"rules": defaultRules}
	}

	if len(rendered.SingBox.Rules) == 0 {
		return map[string]interface{}{"rules": defaultRules}
	}

	// 合并默认规则与策略规则
	allRules := make([]interface{}, 0, len(defaultRules)+len(rendered.SingBox.Rules))
	allRules = append(allRules, defaultRules...)
	for _, r := range rendered.SingBox.Rules {
		allRules = append(allRules, r)
	}

	route := map[string]interface{}{
		"rules": allRules,
	}

	// 添加 rule_set 声明
	if len(rendered.SingBox.RuleSets) > 0 {
		route["rule_set"] = rendered.SingBox.RuleSets
	}

	return route
}

// isCDNNode 判断节点是否需要 TLS 剥离（xray inbound security=none）。
//
// 唯一真相源：写入时持久化的 config_json.exposure_mode。
// standardizeNodeFields 在 Create/Update 时已调用 determineExposureMode 并把结果
// 写入 config_json["exposure_mode"]（显式 > tunnel 凭证 > cdn_address > direct）。
// 渲染层只读这个已持久化的值，不再用"host 是域名 + 端口不同"等表象特征重新推断，
// 避免 REALITY 直连节点（host 为伪装域名如 captive.apple.com）被误判为 CDN 而剥离
// REALITY（realitySettings 被清空 → 客户端握手失败 first record not TLS）。
//
// 剥离矩阵（与 determineExposureMode 对齐）：
//   - argo_tunnel：剥离（cloudflared 明文 HTTP 回源，CF 边缘终止 TLS）
//   - cdn / cdn_saas：剥离（nginx 8445 终止 TLS 后 proxy_pass http 回源）【过渡态】
//   - direct：绝不剥离（xray 自身终止 TLS/REALITY，nginx stream 仅 SNI 透传）
func isCDNNode(n *model.Node) bool {
	if n == nil || n.ConfigJSON == nil {
		return false
	}
	// 规则 0：exposure_mode 已显式持久化 → 直接作为判定依据（唯一真相源）
	if em, ok := n.ConfigJSON["exposure_mode"].(string); ok && em != "" {
		switch em {
		case "argo_tunnel", "cdn", "cdn_saas":
			return true
		default: // "direct" 及其他（含未来的 relay 等）一律不剥离，安全兜底
			return false
		}
	}
	// 规则 1：exposure_mode 未持久化（历史节点未重新保存）时的安全回退。
	// cloudflared 凭证是隧道节点的明确标志（cloudflared 明文回源必须剥离），
	// 与 determineExposureMode 的判定 key 对齐（cloudflared_tunnel_id / cloudflared_tunnel_token）。
	if tid, ok := n.ConfigJSON["cloudflared_tunnel_id"].(string); ok && tid != "" {
		return true
	}
	if ttok, ok := n.ConfigJSON["cloudflared_tunnel_token"].(string); ok && ttok != "" {
		return true
	}
	// metadata 中的 argo_tunnel 显式声明同样认。
	if isArgoTunnelNode(n) {
		return true
	}
	// 规则 2：exposure_mode 未持久化但有 cdn_address → 判定为 CDN（剥离）。
	// 这是历史 CDN 节点（vlessws/trojws/xhttp 等）的现状：它们从未重新保存过，
	// config_json 里只有 cdn_address 没有 exposure_mode。必须与 determineExposureMode
	// 的回退规则（有 cdn_address → cdn_saas）对齐，否则这些正常运行的 CDN 节点会在
	// 下次 refresh 时被错误地"不剥离"，导致 nginx proxy_pass http 到带 TLS 的 xray 而全挂。
	// 与旧"host 是域名"猜测的区别：cdn_address 是用户显式配置的 CDN 回源域名，
	// 不会与 REALITY 直连节点的伪装域名（host/server_name）混淆——REALITY 节点不填 cdn_address。
	if cdnAddr, ok := n.ConfigJSON["cdn_address"].(string); ok && strings.TrimSpace(cdnAddr) != "" {
		return true
	}
	// 其余未持久化节点默认不剥离（安全兜底，宁缺毋滥）。
	return false
}

// generateCDNMirrorInbound 为 CDN 节点生成镜像非 TLS inbound（R2 核心实现）。
//
// CDN 架构：client → CDN(443/TLS) → nginx(终止TLS) → xray(内部端口/无TLS)
//
// 镜像 inbound 规则：
//   - port: config_json.cdn_internal_port 或 原始端口 + 1000（如 9445 → 10445）
//   - listen: "127.0.0.1"（仅本地 nginx 可访问）
//   - security: "none"（nginx 已终止 TLS）
//   - 移除 tlsSettings / realitySettings
//   - tag: 原始 tag + "-internal" 后缀
//   - protocol/transport/path/credentials 保持一致
func generateCDNMirrorInbound(original map[string]interface{}, n *model.Node) map[string]interface{} {
	if original == nil {
		return nil
	}

	// 深拷贝原始 inbound（JSON marshal/unmarshal 确保完全独立）
	data, err := json.Marshal(original)
	if err != nil {
		return nil
	}
	var mirror map[string]interface{}
	if err := json.Unmarshal(data, &mirror); err != nil {
		return nil
	}

	// 1. 计算内部端口：config_json.cdn_internal_port > config_json.cdn_port > 原始+1000
	internalPort := 0
	if originalPort, ok := original["port"].(float64); ok {
		internalPort = int(originalPort) + 1000
	}
	if n.ConfigJSON != nil {
		if p, ok := n.ConfigJSON["cdn_internal_port"].(float64); ok && p > 0 && p <= 65535 {
			internalPort = int(p)
		} else if p, ok := n.ConfigJSON["cdn_port"].(float64); ok && p > 0 && p <= 65535 {
			internalPort = int(p)
		}
	}
	if internalPort <= 0 {
		return nil
	}
	mirror["port"] = internalPort

	// 2. 监听地址改为 127.0.0.1（仅本地 nginx 可访问）
	mirror["listen"] = "127.0.0.1"

	// 3. 修改 tag 加 -internal 后缀
	if tag, ok := mirror["tag"].(string); ok && tag != "" {
		mirror["tag"] = tag + "-internal"
	}

	// 4. 剥离 TLS/REALITY 安全配置，设为 none
	if ss, ok := mirror["streamSettings"].(map[string]interface{}); ok {
		ss["security"] = "none"
		delete(ss, "tlsSettings")
		delete(ss, "realitySettings")
	}

	// 5. 移除协议级 TLS（Hysteria2 等 QUIC 协议的顶层 tls 字段）
	delete(mirror, "tls")

	return mirror
}

// generateSingboxCDNMirrorInbound 为 CDN 节点生成 sing-box 格式的镜像非 TLS inbound（R2 扩展）。
//
// sing-box CDN 架构与 xray 相同：client → CDN(443/TLS) → nginx(终止TLS) → sing-box(内部端口/无TLS)
//
// sing-box 镜像 inbound 与 xray 的差异：
//   - 端口字段: listen_port（sing-box）vs port（xray）
//   - TLS 配置: 顶层 tls 对象（sing-box）vs streamSettings.security（xray）
//   - 协议字段: type（sing-box）vs protocol（xray）
//   - 传输字段: 顶层 transport 对象（sing-box）vs streamSettings 内嵌（xray）
//
// 镜像规则：
//   - listen_port: config_json.cdn_internal_port 或 原始端口 + 1000
//   - listen: "127.0.0.1"（仅本地 nginx 可访问）
//   - tls: 移除（nginx 已终止 TLS）
//   - tag: 原始 tag + "-internal" 后缀
//   - type/transport/users 保持一致
func generateSingboxCDNMirrorInbound(original map[string]interface{}, n *model.Node) map[string]interface{} {
	if original == nil {
		return nil
	}

	// 深拷贝原始 inbound
	data, err := json.Marshal(original)
	if err != nil {
		return nil
	}
	var mirror map[string]interface{}
	if err := json.Unmarshal(data, &mirror); err != nil {
		return nil
	}

	// 1. 计算内部端口：config_json.cdn_internal_port > config_json.cdn_port > 原始+1000
	internalPort := 0
	if originalPort, ok := original["listen_port"].(float64); ok {
		internalPort = int(originalPort) + 1000
	}
	if n.ConfigJSON != nil {
		if p, ok := n.ConfigJSON["cdn_internal_port"].(float64); ok && p > 0 && p <= 65535 {
			internalPort = int(p)
		} else if p, ok := n.ConfigJSON["cdn_port"].(float64); ok && p > 0 && p <= 65535 {
			internalPort = int(p)
		}
	}
	if internalPort <= 0 {
		return nil
	}
	mirror["listen_port"] = internalPort

	// 2. 监听地址改为 127.0.0.1（仅本地 nginx 可访问）
	mirror["listen"] = "127.0.0.1"

	// 3. 修改 tag 加 -internal 后缀
	if tag, ok := mirror["tag"].(string); ok && tag != "" {
		mirror["tag"] = tag + "-internal"
	}

	// 4. 剥离 TLS/REALITY 安全配置
	// sing-box 的 TLS 是顶层 tls 对象（而非 xray 的 streamSettings.security）
	delete(mirror, "tls")

	return mirror
}

// stripTLSFromXrayInbound 将 CDN 节点的 xray inbound 去 TLS 化。
//
// CDN 架构：client → CDN(443/TLS) → nginx(终止TLS, proxy_pass http) → xray(无TLS)
// nginx 已经终止 TLS，xray inbound 不需要 TLS/REALITY，直接设为 security=none。
// 这样 nginx proxy_pass http://127.0.0.1:ServerPort 能正确连接无 TLS 的 xray inbound。
//
// argo_tunnel 节点安全约束（P0 修正）：
//   - cloudflared 与 xray 同机运行，通过本地回环回源（HTTP，绕过 nginx）
//   - xray inbound listen 必须为 "127.0.0.1"（仅本机回环）
//   - 禁止监听 "::" 或 "0.0.0.0"：TLS 已剥离为明文，监听公网接口会让攻击者
//     通过 VPS IPv6/IPv4 直连明文代理端口，绕过 CF 边缘防护，架空隐藏源站 IP
//   - 配合 iptables 对 server_port 公网访问二次拦截（网络层兜底）
//   - cloudflared config.yml 的 service 必须用 http://127.0.0.1:<port>，
//     不用 localhost（localhost 解析优先 IPv6 [::1]，若 xray 仅监听 IPv4 会被拒）
//
// 与 generateCDNMirrorInbound 的区别：
//   - generateCDNMirrorInbound 创建新的镜像 inbound（port+1000），导致端口冲突
//   - stripTLSFromXrayInbound 直接修改原始 inbound，不创建新 inbound，避免端口冲突
func stripTLSFromXrayInbound(inbMap map[string]interface{}, n *model.Node) {
	if inbMap == nil {
		return
	}
	// listen 地址：
	// - argo_tunnel 节点：cloudflared 本地回源，必须 127.0.0.1（不监听公网）
	// - 其他 CDN 节点：nginx 本地反代，必须 127.0.0.1
	// 不再用 "::"（P0 安全修正：避免明文端口暴露公网）
	// 阶段2.4 listen 兜底校验：若上层传入非 127.0.0.1，强制覆盖并记录告警
	if cur, _ := inbMap["listen"].(string); cur != "127.0.0.1" {
		slog.Warn("CDN/Tunnel inbound listen not 127.0.0.1, force overriding",
			"node_code", func() string {
				if n != nil {
					return n.Code
				}
				return "?"
			}(),
			"old_listen", cur,
			"new_listen", "127.0.0.1",
		)
	}
	inbMap["listen"] = "127.0.0.1"
	// 剥离 TLS/REALITY 安全配置
	if ss, ok := inbMap["streamSettings"].(map[string]interface{}); ok {
		ss["security"] = "none"
		delete(ss, "tlsSettings")
		delete(ss, "realitySettings")
		delete(ss, "certificates")
	}
	// 移除协议级 TLS（Hysteria2 等 QUIC 协议的顶层 tls 字段）
	delete(inbMap, "tls")
}

// isArgoTunnelNode 判断节点是否为 argo_tunnel 类型（cloudflared 直连 xray）。
// 判定优先级：config_json.exposure_mode == "argo_tunnel" > metadata.exposure_mode == "argo_tunnel"
func isArgoTunnelNode(n *model.Node) bool {
	if n == nil {
		return false
	}
	if n.ConfigJSON != nil {
		if em, ok := n.ConfigJSON["exposure_mode"].(string); ok && em == "argo_tunnel" {
			return true
		}
	}
	if n.Metadata != nil {
		if em, ok := n.Metadata["exposure_mode"].(string); ok && em == "argo_tunnel" {
			return true
		}
	}
	return false
}

// stripTLSFromSingboxInbound 将 CDN 节点的 sing-box inbound 去 TLS 化。
func stripTLSFromSingboxInbound(inbMap map[string]interface{}) {
	if inbMap == nil {
		return
	}
	inbMap["listen"] = "127.0.0.1"
	delete(inbMap, "tls")
}

// fetchNodeCredsForBuild 预取节点多用户凭证（P0-1 辅助函数）
// 保持 exposure.NodeCredentials 类型兼容，等 P0-1 完全删除 exposure 后可改为本地类型
func (s *DeploymentService) fetchNodeCredsForBuild(ctx context.Context, nodes []*model.Node) exposure.NodeCredentials {
	if s.credRepo == nil || len(nodes) == 0 {
		return nil
	}
	nodeIDs := make([]uuid.UUID, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}
	creds, err := exposure.FetchNodeCredentials(ctx, s.credRepo, nodeIDs)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to fetch user node credentials, falling back to single-user config", "error", err)
		}
		return nil
	}
	return creds
}
