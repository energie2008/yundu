package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/node-service/internal/crypto"
	"github.com/airport-panel/node-service/internal/exposure"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/outbound"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/airport-panel/node-service/internal/routing"
	"github.com/airport-panel/subscription/chain"
	"github.com/airport-panel/subscription/kernelrender"
	"github.com/airport-panel/subscription/nginxrender"
	"github.com/airport-panel/subscription/nodespec"
	"github.com/airport-panel/subscription/validator"
	"github.com/google/uuid"
)

// defaultPayloadKey 是 YUNDU_PAYLOAD_KEY 未设置时的回退密钥（仅开发环境）。
// 安全修复 S4：生产环境必须设置 YUNDU_PAYLOAD_KEY 环境变量。
const defaultPayloadKey = "yundu-default-payload-key-v1"

// P5: Agent 版本强约束 — 内核翻转后旧 Agent 无法处理 _xray_config。
// 低于此版本的 Agent 将被拒绝下发配置（拒绝下发而非静默失败）。
// P3-1: 统一为当前实际 agent 版本 0.7.5，确保所有 agent 支持双内核翻转 + P2-3 自动清理。
const MinAgentVersionForP2 = "0.7.5"

// compareVersion 比较两个语义化版本字符串（格式 "major.minor.patch"）。
// 返回 -1 表示 v1 < v2，0 表示相等，1 表示 v1 > v2。
// 非 semver 格式（如 git commit hash）视为 0 版本。
func compareVersion(v1, v2 string) int {
	p1 := parseSemver(v1)
	p2 := parseSemver(v2)
	for i := 0; i < 3; i++ {
		if p1[i] < p2[i] {
			return -1
		}
		if p1[i] > p2[i] {
			return 1
		}
	}
	return 0
}

// parseSemver 将 "major.minor.patch" 解析为 [3]int。
// 解析失败的部分视为 0。
func parseSemver(v string) [3]int {
	var result [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			result[i] = n
		}
	}
	return result
}

// isProduction 检测当前是否为生产环境。
// 当 YUNDU_ENV=production 或 GO_ENV=production 时返回 true。
func isProduction() bool {
	return os.Getenv("YUNDU_ENV") == "production" || os.Getenv("GO_ENV") == "production"
}

type DeploymentService struct {
	deploymentRepo *repo.DeploymentRepo
	nodeRepo       *repo.NodeRepo
	runtimeRepo    *repo.RuntimeRepo
	serverRepo     *repo.ServerRepo
	chainRepo      *repo.ChainRepo
	credRepo       *repo.UserNodeCredentialRepo
	capRepo        *repo.CapabilityRepo   // P0-6: 能力矩阵仓库
	configPusher   *CompositeConfigPusher // P0-7: 配置推送器
	userDeltaSvc   *UserDeltaService      // Delta Sync: 增量用户变更推送
	// P2-2: 能力降级策略（deny/downgrade/force_kernel）
	// 默认 deny（与 P0 行为一致），可通过 SetDegradeStrategy 切换
	degradeStrategy DegradeStrategy
	// P3-1: Payload Manifest 加密密钥（AES-256），从 YUNDU_PAYLOAD_KEY 派生。
	// 用于加密下发 TLS 证书等敏感材料。
	payloadKey []byte
	// routingRenderer 用于将节点绑定的路由策略渲染为 xray/sing-box routing 配置。
	// 如果为 nil，则使用 defaultXrayRouting()（仅含基础 block private + api 规则）。
	routingRenderer *routing.RoutingRenderer
	// auditConfig 控制审计路由规则（S8）的注入：BlockBT / BlockPrivateIP。
	// 为 nil 时使用 DefaultAuditConfig()（BT + SSRF 防护全部启用）。
	auditConfig *kernelrender.AuditConfig
	// P1-C: tls_certificates 表查询接口（证书四级回退第3级）
	tlsCertReader TLSCertReader
	// P3-C: outbound 策略服务（WARP 出口接入）
	outboundService *outbound.OutboundService
	// P1-2: 双内核校验器，在 GetRuntimeConfig 中持久化前调用 ValidateBoth
	// 阻断无效配置进入 config_versions 表
	dualKernelValidator *validator.DualKernelValidator
	logger              *slog.Logger
}

// TLSCertReader P1-C: tls_certificates 表查询接口（证书四级回退第3级）。
// 由 cert.CertificateRepo 实现（通过适配器）。
type TLSCertReader interface {
	// FindCertPEMBySNI 按 SNI 域名查询 tls_certificates 表，返回匹配的证书 PEM 和私钥 PEM。
	// 匹配逻辑：SANs 数组包含 SNI（大小写不敏感），且 status='active'，cert_pem 非 NULL。
	FindCertPEMBySNI(ctx context.Context, sni string) (certPEM, keyPEM string, ok bool)
}

func NewDeploymentService(deploymentRepo *repo.DeploymentRepo, nodeRepo *repo.NodeRepo, runtimeRepo *repo.RuntimeRepo, serverRepo *repo.ServerRepo, chainRepo *repo.ChainRepo) *DeploymentService {
	// P3-1: 从环境变量读取加密密钥并派生 32 字节 AES-256 密钥
	// 安全修复 S4：生产环境未设置 YUNDU_PAYLOAD_KEY 时 panic
	payloadSecret := os.Getenv("YUNDU_PAYLOAD_KEY")
	if payloadSecret == "" {
		if isProduction() {
			panic("YUNDU_PAYLOAD_KEY environment variable must be set in production environment")
		}
		slog.Warn("YUNDU_PAYLOAD_KEY not set, using insecure default (development only)")
		payloadSecret = defaultPayloadKey
	}
	return &DeploymentService{
		deploymentRepo: deploymentRepo,
		nodeRepo:       nodeRepo,
		runtimeRepo:    runtimeRepo,
		serverRepo:     serverRepo,
		chainRepo:      chainRepo,
		payloadKey:     crypto.DeriveKey(payloadSecret),
		logger:         slog.Default(),
	}
}

// SetPayloadKey P3-1: 注入 Payload 加密密钥（覆盖环境变量派生的密钥）。
// 主要用于测试或运行时密钥轮换场景。
func (s *DeploymentService) SetPayloadKey(key []byte) {
	if len(key) > 0 {
		s.payloadKey = key
	}
}

// SetLogger 注入 logger（可选，默认 slog.Default()）
func (s *DeploymentService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetCredentialRepo 注入 user_node_credentials 仓库（可选）
// 注入后，生成的 xray/sing-box 配置将包含所有用户的独立凭证
func (s *DeploymentService) SetCredentialRepo(r *repo.UserNodeCredentialRepo) {
	s.credRepo = r
}

// SetCapabilityRepo 注入能力矩阵仓库（P0-6）
// 注入后，preflightValidate 将使用 DB 驱动的能力矩阵校验协议组合
func (s *DeploymentService) SetCapabilityRepo(r *repo.CapabilityRepo) {
	s.capRepo = r
}

// SetRoutingRenderer 注入路由渲染器，使下发的 xray/sing-box 配置
// 包含节点绑定的路由策略（route_policies → routing.rules + balancers）。
// 未注入时使用 defaultXrayRouting()（仅含基础 block private + api 规则）。
func (s *DeploymentService) SetRoutingRenderer(r *routing.RoutingRenderer) {
	s.routingRenderer = r
}

// SetConfigPusher 注入配置推送器（P0-7）
// 注入后，配置版本更新将主动推送给 agent，无需等待心跳轮询
func (s *DeploymentService) SetConfigPusher(p *CompositeConfigPusher) {
	s.configPusher = p
}

// SetUserDeltaService 注入增量用户变更推送服务
// 注入后，用户增删变更将通过 DeltaSync 增量推送，替代全量 ConfigPush
func (s *DeploymentService) SetUserDeltaService(svc *UserDeltaService) {
	s.userDeltaSvc = svc
}

// SetDegradeStrategy P2-2: 设置能力降级策略
// deny（默认，blocked→422）/ downgrade（blocked+downgrade_to→改写 spec）/ force_kernel（blocked→切内核）
func (s *DeploymentService) SetDegradeStrategy(strategy DegradeStrategy) {
	if strategy == "" {
		strategy = StrategyDeny
	}
	s.degradeStrategy = strategy
}

// SetAuditConfig S8: 设置审计路由规则配置。
// cfg 为 nil 时使用 DefaultAuditConfig()（BT + SSRF 防护全部启用）。
func (s *DeploymentService) SetAuditConfig(cfg *kernelrender.AuditConfig) {
	s.auditConfig = cfg
}

// SetTLSCertReader P1-C: 注入 tls_certificates 表查询接口（证书四级回退第3级）。
// 注入后，injectCertFromBundle 在 cert_bundles SAN 回退之后、自签兜底之前查询 tls_certificates 表。
func (s *DeploymentService) SetTLSCertReader(r TLSCertReader) {
	s.tlsCertReader = r
}

// SetDualKernelValidator P1-2: 注入双内核校验器。
// 注入后，GetRuntimeConfig 在配置持久化前会对每个启用节点执行 ValidateBoth，
// 校验失败（LevelError）时阻断下发，防止无效配置进入 config_versions 表。
func (s *DeploymentService) SetDualKernelValidator(v *validator.DualKernelValidator) {
	s.dualKernelValidator = v
}

// SetOutboundService P3-C: 注入 outbound 策略服务（WARP 出口接入）。
// 注入后，配置构建时自动聚合节点绑定的 outbound 策略（warp/socks5/chain）到内核配置。
func (s *DeploymentService) SetOutboundService(svc *outbound.OutboundService) {
	s.outboundService = svc
}

// injectOutboundPolicies P3-C: 将节点绑定的 outbound 策略渲染结果注入到内核配置。
// 在 buildConfigViaKernelRender 之后调用，将 warp/socks5/chain outbound 追加到 config.outbounds。
// 注意：同一 runtime 下多个节点可能共享相同的 WARP outbound 策略（server 级别管理），
// 需按 tag 去重避免 sing-box "duplicate outbound/endpoint tag" 校验失败。
func (s *DeploymentService) injectOutboundPolicies(ctx context.Context, config map[string]interface{}, nodes []*model.Node, runtimeType string) {
	if s.outboundService == nil || len(nodes) == 0 {
		return
	}
	isSingbox := runtimeType == "sing-box" || runtimeType == "singbox"
	// 记录已存在的 outbound/endpoint tag（从 config 初始内容提取）
	seenTags := make(map[string]bool)
	if existing, ok := config["outbounds"].([]interface{}); ok {
		for _, ob := range existing {
			if m, ok := ob.(outbound.Map); ok {
				if tag, ok := m["tag"].(string); ok {
					seenTags[tag] = true
				}
			} else if m, ok := ob.(map[string]interface{}); ok {
				if tag, ok := m["tag"].(string); ok {
					seenTags[tag] = true
				}
			}
		}
	}
	if existing, ok := config["endpoints"].([]interface{}); ok {
		for _, ep := range existing {
			if m, ok := ep.(outbound.Map); ok {
				if tag, ok := m["tag"].(string); ok {
					seenTags[tag] = true
				}
			} else if m, ok := ep.(map[string]interface{}); ok {
				if tag, ok := m["tag"].(string); ok {
					seenTags[tag] = true
				}
			}
		}
	}
	for _, node := range nodes {
		resp, err := s.outboundService.ApplyAll(ctx, node.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("injectOutboundPolicies: ApplyAll failed",
					"node_id", node.ID, "error", err)
			}
			continue
		}
		var renderedOutbounds []outbound.Map
		var renderedRules []outbound.Map
		var renderedEndpoints []outbound.Map
		if isSingbox {
			renderedOutbounds = resp.SingBox.Outbounds
			renderedRules = resp.SingBox.RoutingRules
			renderedEndpoints = resp.SingBox.Endpoints
		} else {
			renderedOutbounds = resp.Xray.Outbounds
			renderedRules = resp.Xray.RoutingRules
		}
		// sing-box 1.13+: wireguard 渲染为 endpoint，注入到 config.endpoints[]
		if isSingbox && len(renderedEndpoints) > 0 {
			var endpoints []interface{}
			if existing, ok := config["endpoints"].([]interface{}); ok {
				endpoints = existing
			}
			for _, ep := range renderedEndpoints {
				tag, _ := ep["tag"].(string)
				if tag != "" && seenTags[tag] {
					continue // 去重：跳过已存在的 endpoint tag
				}
				endpoints = append(endpoints, ep)
				seenTags[tag] = true
			}
			config["endpoints"] = endpoints
		}
		if len(renderedOutbounds) > 0 {
			var outbounds []interface{}
			if existing, ok := config["outbounds"].([]interface{}); ok {
				outbounds = existing
			}
			for _, ob := range renderedOutbounds {
				tag, _ := ob["tag"].(string)
				if tag != "" && seenTags[tag] {
					continue // 去重：跳过已存在的 outbound tag
				}
				outbounds = append(outbounds, ob)
				seenTags[tag] = true
			}
			config["outbounds"] = outbounds
		}

		// LB-3 (节点级重构): 检测 urltest outbound（由 load_balance policy 转译而来），
		// 按该节点的 inbound 注入精确路由规则，替代旧的全局 route.final 方案。
		// 旧方案 route.final=urltest 是 sing-box 全局兜底出口，同一 VPS 上所有节点
		// 共享一个 sing-box 进程，任一节点勾选 WARP 会把未勾选节点的流量也导入 WARP 池。
		// 新方案仅对勾选节点的 inbound 注入 inbound→warp-pool 规则：
		//   - 勾选节点：inbound 流量路由到 warp-pool（urltest 自动选优+故障切换）
		//   - 未勾选节点：无规则命中，走默认直连（route.final 不再被设置）
		if isSingbox {
			for _, ob := range renderedOutbounds {
				t, _ := ob["type"].(string)
				if t != "urltest" {
					continue
				}
				tag, _ := ob["tag"].(string)
				if tag == "" {
					continue
				}
				inboundTags := collectNodeInboundTags(config, node.Code)
				if len(inboundTags) == 0 {
					// 配置中找不到该节点的 inbound（tag 约定不符或 inbound 未渲染），
					// 记录警告便于排查，不注入规则避免引用不存在的 inbound
					if s.logger != nil {
						s.logger.Warn("injectOutboundPolicies: no inbound tags found for warp routing",
							"node_id", node.ID, "node_code", node.Code, "pool_tag", tag)
					}
					break
				}
				route, ok := config["route"].(map[string]interface{})
				if !ok {
					route = map[string]interface{}{}
					config["route"] = route
				}
				var rules []interface{}
				if existing, ok := route["rules"].([]interface{}); ok {
					rules = existing
				}
				rules = append(rules, map[string]interface{}{
					"action":   "route",
					"inbound":  inboundTags,
					"outbound": tag,
				})
				route["rules"] = rules
				if s.logger != nil {
					s.logger.Info("injectOutboundPolicies: warp inbound routing rule injected",
						"node_id", node.ID, "node_code", node.Code,
						"inbound_tags", inboundTags, "outbound", tag)
				}
				break
			}
		}
		if len(renderedRules) > 0 {
			route, ok := config["route"].(map[string]interface{})
			if !ok {
				route = map[string]interface{}{}
				config["route"] = route
			}
			var rules []interface{}
			if existing, ok := route["rules"].([]interface{}); ok {
				rules = existing
			}
			for _, rule := range renderedRules {
				rules = append(rules, rule)
			}
			route["rules"] = rules
		}
		if s.logger != nil {
			s.logger.Info("injectOutboundPolicies: outbound policies injected",
				"node_id", node.ID, "outbound_count", len(renderedOutbounds),
				"rule_count", len(renderedRules), "runtime", runtimeType)
		}
	}
}

// collectNodeInboundTags 从内核配置中收集属于指定节点的 inbound tag。
// tag 约定（kernelrender）：主 inbound 为 "in-<code>"，split 模式下行 inbound 为
// "in-<code>" + DownstreamTagSuffix。仅返回配置中真实存在的 tag，
// 避免路由规则引用不存在的 inbound 导致 sing-box 校验失败。
func collectNodeInboundTags(config map[string]interface{}, nodeCode string) []string {
	want := map[string]bool{
		"in-" + nodeCode:                       true,
		"in-" + nodeCode + DownstreamTagSuffix: true,
	}
	inbounds, _ := config["inbounds"].([]interface{})
	var tags []string
	for _, inb := range inbounds {
		m, ok := inb.(map[string]interface{})
		if !ok {
			continue
		}
		if tag, _ := m["tag"].(string); tag != "" && want[tag] {
			tags = append(tags, tag)
		}
	}
	return tags
}

// getAuditConfig 返回生效的审计配置，nil 时回退到默认值。
func (s *DeploymentService) getAuditConfig() kernelrender.AuditConfig {
	if s.auditConfig != nil {
		return *s.auditConfig
	}
	return DefaultAuditConfig()
}

func (s *DeploymentService) DryRun(ctx context.Context, adminID uuid.UUID, req *model.DryRunRequest) (*model.DryRunResponse, error) {
	if req.ScopeType == "" {
		return nil, ErrInvalidScopeType
	}
	if req.ScopeID == uuid.Nil {
		return nil, ErrInvalidScopeType
	}

	versionNo, err := s.deploymentRepo.GetNextVersionNo(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, err
	}

	oldVersion, err := s.deploymentRepo.GetLatestConfigVersion(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, err
	}

	var oldContent map[string]interface{}
	if oldVersion != nil {
		oldContent = oldVersion.ContentJSON
	}

	diffSummary := pkg.GenerateDiff(oldContent, req.ContentJSON)
	contentHash := pkg.HashContent(req.ContentJSON)

	// B20 修复：实现真实校验逻辑，不再永远返回 Valid: true
	validationErrors := s.validateDryRunContent(ctx, req.ScopeType, req.ScopeID, req.ContentJSON)

	valid := len(validationErrors) == 0
	message := "Configuration is valid"
	if !valid {
		message = fmt.Sprintf("Validation failed with %d error(s): %s", len(validationErrors), strings.Join(validationErrors, "; "))
	}

	return &model.DryRunResponse{
		ScopeType:    req.ScopeType,
		ScopeID:      req.ScopeID,
		NewVersionNo: versionNo,
		ContentHash:  contentHash,
		DiffSummary:  diffSummary,
		Valid:        valid,
		Message:      message,
	}, nil
}

// validateDryRunContent 对 DryRun 请求的内容进行真实校验。
// B20 修复：替代原先永远返回 Valid: true 的假实现。
func (s *DeploymentService) validateDryRunContent(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID, content map[string]interface{}) []string {
	var errs []string

	// 1. 校验 scope 存在性
	switch scopeType {
	case model.ScopeTypeNode:
		node, err := s.nodeRepo.GetByID(ctx, scopeID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to query node: %v", err))
		} else if node == nil {
			errs = append(errs, "target node not found")
		} else {
			// 2. 校验内容与节点协议兼容性
			if proto, ok := content["protocol_type"].(string); ok && proto != "" {
				if node.ProtocolType != "" && node.ProtocolType != proto {
					errs = append(errs, fmt.Sprintf("protocol_type mismatch: node=%s config=%s", node.ProtocolType, proto))
				}
			}
			// 3. 校验 REALITY 配置完整性（如果使用 REALITY）
			if sec, ok := content["security"].(string); ok && sec == "reality" {
				if pk := pickStringFromMap(content, "private_key", "reality", "reality_settings"); pk == "" {
					errs = append(errs, "REALITY security selected but private_key is missing")
				}
			}
		}
	case model.ScopeTypeRuntime:
		nodes, err := s.nodeRepo.ListByRuntimeID(ctx, scopeID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to query runtime nodes: %v", err))
		} else if len(nodes) == 0 {
			errs = append(errs, "no nodes found under this runtime")
		}
	default:
		errs = append(errs, fmt.Sprintf("unknown scope_type: %s", string(scopeType)))
	}

	// 4. 校验 JSON 结构基本完整性
	if content == nil {
		errs = append(errs, "content_json is nil")
	} else {
		// 校验 transport_type 如果存在必须是合法值
		if tt, ok := content["transport_type"].(string); ok && tt != "" {
			validTransports := map[string]bool{
				"tcp": true, "ws": true, "grpc": true, "http": true,
				"httpupgrade": true, "xhttp": true, "kcp": true,
				"quic": true, "h2": true, "meek": true, "obfs": true,
			}
			if !validTransports[tt] {
				errs = append(errs, fmt.Sprintf("unknown transport_type: %s", tt))
			}
		}
		// 校验 port 范围
		if port, ok := content["port"].(float64); ok {
			if port < 1 || port > 65535 {
				errs = append(errs, fmt.Sprintf("port out of range: %v", port))
			}
		}
	}

	return errs
}

// pickStringFromMap 从 map 中按多级路径提取字符串值。
func pickStringFromMap(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
		// 尝试嵌套路径
		if nested, ok := m[keys[0]].(map[string]interface{}); ok {
			for _, k := range keys[1:] {
				if v, ok := nested[k].(string); ok && v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func (s *DeploymentService) Deploy(ctx context.Context, adminID uuid.UUID, req *model.DeployRequest) (*model.DeploymentBatch, []*model.DeploymentTarget, error) {
	if req.ScopeType == "" {
		return nil, nil, ErrInvalidScopeType
	}
	if req.ScopeID == uuid.Nil {
		return nil, nil, ErrInvalidScopeType
	}

	strategy := req.Strategy
	if strategy == "" {
		strategy = model.DeploymentStrategyRolling
	}

	versionNo, err := s.deploymentRepo.GetNextVersionNo(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, nil, err
	}

	contentHash := pkg.HashContent(req.ContentJSON)
	contentJSON := req.ContentJSON
	if contentJSON == nil {
		contentJSON = make(map[string]interface{})
	}

	configVersion := &model.ConfigVersion{
		ID:               uuid.New(),
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		VersionNo:        versionNo,
		Status:           model.ConfigVersionStatusPending,
		Source:           model.ConfigSourceAdmin,
		SchemaVersion:    "v1",
		ContentJSON:      contentJSON,
		ContentHash:      contentHash,
		CreatedByAdminID: &adminID,
	}

	if err := s.deploymentRepo.CreateConfigVersion(ctx, configVersion); err != nil {
		return nil, nil, err
	}

	var targets []*model.DeploymentTarget
	var nodes []*model.Node

	switch req.ScopeType {
	case model.ScopeTypeNode:
		node, err := s.nodeRepo.GetByID(ctx, req.ScopeID)
		if err != nil {
			return nil, nil, err
		}
		if node == nil {
			return nil, nil, ErrNodeNotFound
		}
		nodes = append(nodes, node)
	case model.ScopeTypeRuntime:
		nodes, err = s.nodeRepo.ListByRuntimeID(ctx, req.ScopeID)
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, ErrInvalidScopeType
	}

	oldVersion, err := s.deploymentRepo.GetLatestConfigVersion(ctx, req.ScopeType, req.ScopeID)
	if err != nil {
		return nil, nil, err
	}

	// 按策略计算分批：phases[i] 对应 PhaseNo = i+1 的节点集合
	phases := s.planPhases(nodes, strategy)

	// 生成 batch_plan JSON: [{"phase":1,"node_ids":[...],"percentage":10}, ...]
	batchPlan := make([]interface{}, 0, len(phases))
	totalNodes := len(nodes)
	for i, phaseNodes := range phases {
		phaseNo := i + 1
		nodeIDs := make([]uuid.UUID, 0, len(phaseNodes))
		for _, n := range phaseNodes {
			nodeIDs = append(nodeIDs, n.ID)
		}
		percentage := 0
		if totalNodes > 0 {
			percentage = int(math.Round(float64(len(phaseNodes)) * 100.0 / float64(totalNodes)))
		}
		batchPlan = append(batchPlan, map[string]interface{}{
			"phase":      phaseNo,
			"node_ids":   nodeIDs,
			"percentage": percentage,
		})
	}

	batch := &model.DeploymentBatch{
		ID:               uuid.New(),
		ScopeType:        req.ScopeType,
		ScopeID:          req.ScopeID,
		TargetVersionID:  configVersion.ID,
		Strategy:         strategy,
		BatchPlan:        batchPlan,
		Status:           model.DeploymentStatusPending,
		CreatedByAdminID: &adminID,
	}

	if err := s.deploymentRepo.CreateBatch(ctx, batch); err != nil {
		return nil, nil, err
	}

	// 按 phase 创建 target：PhaseNo=1 设为 pending（开始下发），
	// 其余 phase 设为 paused（等待 AdvancePhase 推进）
	for i, phaseNodes := range phases {
		phaseNo := i + 1
		status := model.TargetStatusPending
		if phaseNo > 1 {
			status = model.TargetStatusPaused
		}
		for _, node := range phaseNodes {
			var previousVersionID *uuid.UUID
			if oldVersion != nil {
				previousVersionID = &oldVersion.ID
			}

			target := &model.DeploymentTarget{
				ID:                uuid.New(),
				DeploymentBatchID: batch.ID,
				TargetType:        model.TargetTypeNode,
				TargetID:          node.ID,
				TargetVersionID:   configVersion.ID,
				PreviousVersionID: previousVersionID,
				PhaseNo:           phaseNo,
				Status:            status,
				PrecheckResult:    make(map[string]interface{}),
				ApplyResult:       make(map[string]interface{}),
				RollbackResult:    make(map[string]interface{}),
			}
			targets = append(targets, target)
		}
	}

	if err := s.deploymentRepo.CreateTargets(ctx, targets); err != nil {
		return nil, nil, err
	}

	if err := s.deploymentRepo.UpdateBatchStatus(ctx, batch.ID, model.DeploymentStatusRunning); err != nil {
		return nil, nil, err
	}
	if err := s.deploymentRepo.UpdateConfigVersionStatus(ctx, configVersion.ID, model.ConfigVersionStatusPending); err != nil {
		return nil, nil, err
	}

	s.logger.Info("deployment batch created",
		"batch_id", batch.ID, "strategy", strategy, "phases", len(phases), "targets", len(targets))
	return batch, targets, nil
}

// planPhases 根据 strategy 将节点划分为多个 phase。
// 返回 phases[i]（PhaseNo = i+1）的节点切片。
//   - all_at_once: 全部进 Phase 1
//   - rolling:     按 25% 分批，PhaseNo 递增（1..N）
//   - canary:      前 10% 为 Phase 1（金丝雀），剩余 90% 均分为 Phase 2/3/4
func (s *DeploymentService) planPhases(nodes []*model.Node, strategy model.DeploymentStrategy) [][]*model.Node {
	n := len(nodes)
	if n == 0 {
		return nil
	}

	// B32 修复：按 Node ID 排序确保分批结果稳定
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID.String() < nodes[j].ID.String()
	})

	switch strategy {
	case model.DeploymentStrategyAllAtOnce:
		return [][]*model.Node{nodes}

	case model.DeploymentStrategyCanary:
		canaryCount := int(math.Ceil(float64(n) * 10.0 / 100.0))
		if canaryCount < 1 {
			canaryCount = 1
		}
		if canaryCount > n {
			canaryCount = n
		}
		phases := [][]*model.Node{nodes[:canaryCount]}
		rest := nodes[canaryCount:]
		if len(rest) > 0 {
			// 剩余 90% 均分为 3 个 phase
			batchSize := int(math.Ceil(float64(len(rest)) / 3.0))
			if batchSize < 1 {
				batchSize = 1
			}
			for start := 0; start < len(rest); start += batchSize {
				end := start + batchSize
				if end > len(rest) {
					end = len(rest)
				}
				phases = append(phases, rest[start:end])
			}
		}
		return phases

	default:
		// rolling（及其它未知策略）按 25% 分批
		batchSize := int(math.Ceil(float64(n) * 25.0 / 100.0))
		if batchSize < 1 {
			batchSize = 1
		}
		var phases [][]*model.Node
		for start := 0; start < n; start += batchSize {
			end := start + batchSize
			if end > n {
				end = n
			}
			phases = append(phases, nodes[start:end])
		}
		return phases
	}
}

func (s *DeploymentService) UpdateDeploymentResult(ctx context.Context, targetID uuid.UUID, req *model.UpdateDeploymentResultRequest) error {
	target, err := s.deploymentRepo.GetTargetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrTargetNotFound
	}

	now := time.Now()
	target.Status = req.Status
	if req.PrecheckResult != nil {
		target.PrecheckResult = req.PrecheckResult
	}
	if req.ApplyResult != nil {
		target.ApplyResult = req.ApplyResult
	}

	if req.Status == model.TargetStatusApplying {
		target.StartedAt = &now
	}
	if req.Status == model.TargetStatusSuccess || req.Status == model.TargetStatusFailed {
		target.FinishedAt = &now
	}

	if err := s.deploymentRepo.UpdateTargetResult(ctx, target); err != nil {
		return err
	}

	// 仅在 target 进入终态时触发 phase 推进 / 回滚判定
	if req.Status != model.TargetStatusSuccess && req.Status != model.TargetStatusFailed {
		return nil
	}

	// 检查该 target 所属 phase 是否已全部完成
	phaseTargets, err := s.deploymentRepo.ListTargetsByBatchAndPhase(ctx, target.DeploymentBatchID, target.PhaseNo)
	if err != nil {
		s.logger.Error("list phase targets failed",
			"batch_id", target.DeploymentBatchID, "phase", target.PhaseNo, "error", err)
		return nil
	}

	allSuccess := true
	hasFailed := false
	for _, t := range phaseTargets {
		switch t.Status {
		case model.TargetStatusSuccess:
			// ok
		case model.TargetStatusFailed:
			hasFailed = true
			allSuccess = false
		default:
			// 仍有未完成 target（pending/precheck/applying/verifying/rolling_back/paused）
			allSuccess = false
		}
	}

	if hasFailed {
		if err := s.RollbackBatch(ctx, target.DeploymentBatchID,
			fmt.Sprintf("phase %d has failed targets", target.PhaseNo)); err != nil {
			// rollback 失败只记录日志，不阻断结果上报
			s.logger.Error("auto rollback failed",
				"batch_id", target.DeploymentBatchID, "error", err)
		}
		return nil
	}

	if allSuccess {
		if err := s.AdvancePhase(ctx, target.DeploymentBatchID); err != nil {
			s.logger.Error("auto advance failed",
				"batch_id", target.DeploymentBatchID, "error", err)
		}
	}

	return nil
}

// AdvancePhase 推进部署批次到下一 phase。
//   - 当前 phase 全部 success → 把下一 phase 的 paused target 改为 pending；
//     若已是最后一个 phase，则将 batch 标记为 success、config 标记为 deployed。
//   - 当前 phase 有 failed → 调用 RollbackBatch 回滚。
//   - 当前 phase 仍在进行中 → 返回 ErrDeploymentRunning（调用方可据此感知，不阻断结果上报）。
func (s *DeploymentService) AdvancePhase(ctx context.Context, batchID uuid.UUID) error {
	targets, err := s.deploymentRepo.ListTargetsByBatchID(ctx, batchID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return ErrBatchNotFound
	}

	// currentPhase = 已激活的最高 phase（含 success/failed/进行中，但不含 paused）
	// maxPhase     = 全部 target 中最大的 phase_no
	currentPhase := 0
	maxPhase := 0
	for _, t := range targets {
		if t.PhaseNo > maxPhase {
			maxPhase = t.PhaseNo
		}
		if t.Status != model.TargetStatusPaused && t.PhaseNo > currentPhase {
			currentPhase = t.PhaseNo
		}
	}
	if currentPhase == 0 {
		// 所有 target 仍为 paused（不应发生），按未启动处理
		return ErrDeploymentRunning
	}

	// 评估当前 phase 完成情况
	allSuccess := true
	hasFailed := false
	for _, t := range targets {
		if t.PhaseNo != currentPhase {
			continue
		}
		switch t.Status {
		case model.TargetStatusSuccess:
			// ok
		case model.TargetStatusFailed:
			hasFailed = true
			allSuccess = false
		case model.TargetStatusRolledBack:
			// 已回滚，视为非 success 终态
			allSuccess = false
		default:
			// 仍有进行中 target
			allSuccess = false
		}
	}

	if hasFailed {
		return s.RollbackBatch(ctx, batchID, fmt.Sprintf("phase %d has failed targets", currentPhase))
	}
	if !allSuccess {
		return ErrDeploymentRunning
	}

	// 当前 phase 全部 success，尝试推进下一 phase
	nextPhase := currentPhase + 1
	if nextPhase > maxPhase {
		// 已是最后一个 phase，收尾
		if err := s.deploymentRepo.UpdateBatchStatus(ctx, batchID, model.DeploymentStatusSuccess); err != nil {
			return err
		}
		if err := s.deploymentRepo.UpdateConfigVersionStatus(ctx, targets[0].TargetVersionID, model.ConfigVersionStatusDeployed); err != nil {
			return err
		}
		s.logger.Info("deployment batch finished successfully",
			"batch_id", batchID, "phases", maxPhase)
		return nil
	}

	// 把下一 phase 的 paused target 推进为 pending
	pausedCount, err := s.deploymentRepo.CountTargetsByPhase(ctx, batchID, nextPhase, model.TargetStatusPaused)
	if err != nil {
		return err
	}
	if pausedCount == 0 {
		// 下一 phase 没有待推进的 target，避免重复推进
		return nil
	}
	if err := s.deploymentRepo.UpdateTargetStatusByPhase(ctx, batchID, nextPhase, model.TargetStatusPending); err != nil {
		return err
	}
	s.logger.Info("deployment phase advanced",
		"batch_id", batchID, "from_phase", currentPhase, "to_phase", nextPhase, "targets", pausedCount)
	return nil
}

// RollbackBatch 回滚批次：将所有非 success/failed/rolled_back 的 target 标记为
// rolling_back → rolled_back，并把 batch 置为 failed、config 版本置为 rolled_back。
// 回滚过程中的单步失败只记录日志、不中断后续步骤（best-effort），最终返回首个错误。
func (s *DeploymentService) RollbackBatch(ctx context.Context, batchID uuid.UUID, reason string) error {
	targets, err := s.deploymentRepo.ListTargetsByBatchID(ctx, batchID)
	if err != nil {
		s.logger.Error("rollback: list targets failed", "batch_id", batchID, "error", err)
		return err
	}
	if len(targets) == 0 {
		return ErrBatchNotFound
	}

	versionID := targets[0].TargetVersionID
	var firstErr error

	// step 1: 把非终态 target 置为 rolling_back
	for _, t := range targets {
		if t.Status == model.TargetStatusSuccess ||
			t.Status == model.TargetStatusFailed ||
			t.Status == model.TargetStatusRolledBack {
			continue
		}
		t.Status = model.TargetStatusRollingBack
		if err := s.deploymentRepo.UpdateTargetResult(ctx, t); err != nil {
			s.logger.Error("rollback: mark rolling_back failed", "target_id", t.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// step 2: 把 rolling_back target 置为 rolled_back
	for _, t := range targets {
		if t.Status != model.TargetStatusRollingBack {
			continue
		}
		t.Status = model.TargetStatusRolledBack
		if err := s.deploymentRepo.UpdateTargetResult(ctx, t); err != nil {
			s.logger.Error("rollback: mark rolled_back failed", "target_id", t.ID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// step 3: batch -> failed
	if err := s.deploymentRepo.UpdateBatchStatus(ctx, batchID, model.DeploymentStatusFailed); err != nil {
		s.logger.Error("rollback: update batch status failed", "batch_id", batchID, "error", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	// step 4: config version -> rolled_back
	if err := s.deploymentRepo.UpdateConfigVersionStatus(ctx, versionID, model.ConfigVersionStatusRolledBack); err != nil {
		s.logger.Error("rollback: update config version status failed", "version_id", versionID, "error", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	s.logger.Info("deployment batch rolled back",
		"batch_id", batchID, "reason", reason, "targets", len(targets))
	return firstErr
}

// RefreshNodeConfig 根据节点ID强制刷新配置版本（从nodes表自动渲染）
func (s *DeploymentService) RefreshNodeConfig(ctx context.Context, nodeID uuid.UUID) (*model.ConfigVersion, error) {
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}
	return s.GetRuntimeConfig(ctx, node.RuntimeID, "")
}

// RefreshRuntimeConfig 根据runtimeID强制刷新配置版本
func (s *DeploymentService) RefreshRuntimeConfig(ctx context.Context, runtimeID uuid.UUID) (*model.ConfigVersion, error) {
	return s.GetRuntimeConfig(ctx, runtimeID, "")
}

func (s *DeploymentService) ListBatches(ctx context.Context, page, pageSize int, status model.DeploymentStatus, scopeType model.ScopeType) ([]*model.DeploymentBatch, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.deploymentRepo.ListBatches(ctx, page, pageSize, status, scopeType)
}

func (s *DeploymentService) GetRuntimeConfig(ctx context.Context, runtimeID uuid.UUID, versionStr string) (*model.ConfigVersion, error) {
	if versionStr != "" {
		var versionNo int64
		if _, err := fmt.Sscanf(versionStr, "%d", &versionNo); err == nil {
			cv, err := s.deploymentRepo.GetConfigVersionByVersionNo(ctx, model.ScopeTypeRuntime, runtimeID, versionNo)
			if err != nil {
				return nil, err
			}
			if cv != nil {
				return cv, nil
			}
		}
	}

	rt, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil {
		return nil, err
	}

	nodes, err := s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
	if err != nil {
		return nil, err
	}

	// 自动修正：检查绑定到本runtime的节点是否实际需要另一个内核，
	// 如果是则迁移到正确的runtime（修复历史数据中节点绑定错误runtime的问题）
	if rt != nil {
		serverID := rt.ServerID
		allRuntimes, _ := s.runtimeRepo.ListByServer(ctx, serverID)
		runtimeByType := make(map[string]*model.Runtime)
		for _, r := range allRuntimes {
			rtType := normalizeRuntimeType(r.RuntimeType)
			runtimeByType[rtType] = r
		}
		currentRTType := normalizeRuntimeType(rt.RuntimeType)
		for _, n := range nodes {
			secStr := ""
			if n.SecurityType != nil {
				secStr = *n.SecurityType
			}
			requiredKernel := detectRequiredKernel(n.ProtocolType, n.TransportType, secStr, n.ConfigJSON)
			if requiredKernel != currentRTType {
				targetRT, ok := runtimeByType[requiredKernel]
				if ok && targetRT.ID != n.RuntimeID {
					slog.Warn("auto-rerouting node to correct runtime",
						"node_code", n.Code, "from_kernel", currentRTType, "to_kernel", requiredKernel)
					n.RuntimeID = targetRT.ID
					_ = s.nodeRepo.Update(ctx, n)
				}
			}
		}
		// 重新获取修正后的节点列表（排除被迁移走的节点）
		nodes, err = s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
		if err != nil {
			return nil, err
		}
	}

	listenHost := ""
	if rt != nil && rt.ListenHost != nil {
		listenHost = *rt.ListenHost
	}

	runtimeType := "xray"
	if rt != nil && rt.RuntimeType != "" {
		runtimeType = normalizeRuntimeType(rt.RuntimeType)
	}

	freshConfig, err := s.buildRuntimeConfig(ctx, runtimeType, nodes, listenHost)
	if err != nil {
		return nil, err
	}
	// P1-2: DualKernelValidator 接入部署链路 — 配置持久化前对每个启用节点执行双核校验。
	// 校验内容：Enhancement 专项 → 双核渲染 → 真实 dry-run（需二进制）→ 语义等价性。
	// 任何节点校验失败（LevelError）则阻断下发，防止无效配置进入 config_versions 表。
	// 开发环境未注入 validator 或未设置二进制路径时自动跳过 dry-run，仅做前三步语义校验。
	if s.dualKernelValidator != nil {
		for _, n := range nodes {
			if !n.IsEnabled {
				continue
			}
			spec := modelNodeToNodeSpec(n)
			if spec == nil {
				continue
			}
			result := s.dualKernelValidator.ValidateBoth(ctx, spec)
			if !result.Passed {
				var errDetails []string
				for _, e := range result.Errors {
					if e.Level == validator.LevelError {
						errDetails = append(errDetails, fmt.Sprintf("[%s] %s", e.Kernel, e.Message))
					}
				}
				if len(errDetails) > 0 {
					return nil, fmt.Errorf("%w: 节点 %s 双核校验失败: %s",
						ErrPreflightValidation, n.Code, strings.Join(errDetails, "; "))
				}
			}
		}
		if s.logger != nil {
			s.logger.Debug("DualKernelValidator: all nodes passed validation",
				"node_count", len(nodes), "runtime_type", runtimeType)
		}
	}
	// R9: L4 Dry-run 校验 — 对完整渲染配置执行 xray -test / sing-box check
	// 仅在设置了 XRAY_BINARY/SINGBOX_BINARY 环境变量时执行，开发环境自动跳过
	if err := dryRunConfig(runtimeType, freshConfig); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPreflightValidation, err)
	}
	// R12 修复：在 hash 计算之前注入 nginx vhost 和流量限额元数据，
	// 确保面板和 Agent 计算的配置签名一致。
	// _nginx_vhosts 和 _traffic_quota 以 "_" 前缀标识为元数据字段，
	// Agent 在写入内核配置前会剥离这些字段。
	s.injectNginxVhosts(freshConfig, nodes)
	s.injectTrafficQuota(freshConfig, nodes)
	// P2 翻转：sing-box 配置中嵌入 xray 配置，Agent 拉取后分发到 xray 内核（懒加载）
	// 返回 xray 节点列表，用于后续更新 lpv/dispatch_status（xhttp 节点挂在 xray runtime 下，
	// 但配置通过 _xray_config 嵌入 sing-box 配置下发，需同步状态避免面板显示"下发失败"）
	xrayNodes := s.injectXrayConfig(ctx, freshConfig, rt)
	freshHash := pkg.HashContent(freshConfig)
	// 合并 sing-box + xray 节点用于状态更新（lpv/dispatch_status）
	allNodes := nodes
	if len(xrayNodes) > 0 {
		allNodes = make([]*model.Node, 0, len(nodes)+len(xrayNodes))
		allNodes = append(allNodes, nodes...)
		allNodes = append(allNodes, xrayNodes...)
	}

	cv, err := s.deploymentRepo.GetLatestActiveConfigVersion(ctx, model.ScopeTypeRuntime, runtimeID)
	if err != nil {
		return nil, err
	}

	if cv == nil {
		versionNo, err := s.deploymentRepo.GetNextVersionNo(ctx, model.ScopeTypeRuntime, runtimeID)
		if err != nil {
			versionNo = 1
		}
		cv = &model.ConfigVersion{
			ID:            uuid.New(),
			ScopeType:     model.ScopeTypeRuntime,
			ScopeID:       runtimeID,
			VersionNo:     versionNo,
			Status:        model.ConfigVersionStatusDeployed,
			Source:        model.ConfigSourceSystem,
			SchemaVersion: "v1",
			ContentJSON:   freshConfig,
			ContentHash:   freshHash,
		}
		_ = s.deploymentRepo.CreateConfigVersion(ctx, cv)
		// P3-1: 构建加密 Payload Manifest 并持久化（兼容期双写，失败不阻断明文配置下发）
		s.tryCreatePayload(ctx, cv, runtimeType)
		// B-收口：状态流转（lpv→pending→push→pushed/failed）收口到 dispatchAndPush
		s.dispatchAndPush(ctx, runtimeID, allNodes, cv)
		return cv, nil
	}

	if cv.ContentHash != freshHash {
		versionNo, err := s.deploymentRepo.GetNextVersionNo(ctx, model.ScopeTypeRuntime, runtimeID)
		if err != nil {
			versionNo = cv.VersionNo + 1
		}
		newCv := &model.ConfigVersion{
			ID:            uuid.New(),
			ScopeType:     model.ScopeTypeRuntime,
			ScopeID:       runtimeID,
			VersionNo:     versionNo,
			Status:        model.ConfigVersionStatusDeployed,
			Source:        model.ConfigSourceSystem,
			SchemaVersion: "v1",
			ContentJSON:   freshConfig,
			ContentHash:   freshHash,
		}
		_ = s.deploymentRepo.CreateConfigVersion(ctx, newCv)
		// P3-1: 构建加密 Payload Manifest 并持久化（兼容期双写，失败不阻断明文配置下发）
		s.tryCreatePayload(ctx, newCv, runtimeType)
		// B-收口：状态流转（lpv→pending→push→pushed/failed）收口到 dispatchAndPush
		s.dispatchAndPush(ctx, runtimeID, allNodes, newCv)
		return newCv, nil
	}

	// 配置内容未变化，但仍需注入 nginx vhosts 供 agent 使用。
	// 关键：深拷贝ContentJSON后再inject，避免原地修改从DB层取出的map对象
	// （repo可能缓存/复用cv对象，原地inject会污染后续请求看到的ContentJSON，
	//  导致DB里存的ContentHash与实际ContentJSON不一致，agent签名校验永久nack）
	cloned, cloneErr := deepCloneJSONMap(cv.ContentJSON)
	if cloneErr != nil {
		s.logger.Warn("deep clone config for inject failed, falling back to in-place inject", "error", cloneErr)
		cloned = cv.ContentJSON
	}
	s.injectNginxVhosts(cloned, nodes)
	// P3-N: 配置未变化时也注入流量限额元数据
	s.injectTrafficQuota(cloned, nodes)
	// P2 翻转：配置未变化时也注入 xray 配置
	// 801 修复：捕获返回的 xray 节点，若其 lpv 与当前版本不一致则补更新
	// （旧代码创建的版本可能未更新 xray 节点 lpv，此处兜底修正）
	xrayNodes = s.injectXrayConfig(ctx, cloned, rt)
	if len(xrayNodes) > 0 {
		needsUpdate := false
		for _, xn := range xrayNodes {
			if xn.LastPublishedVersion != cv.VersionNo {
				needsUpdate = true
				break
			}
		}
		if needsUpdate {
			allNodes = make([]*model.Node, 0, len(nodes)+len(xrayNodes))
			allNodes = append(allNodes, nodes...)
			allNodes = append(allNodes, xrayNodes...)
			s.updateNodesPublishedVersion(ctx, allNodes, cv.VersionNo)
		}
	}
	// 注入 _audit_rules 禁用 agent 端错误的 SSRF 规则（把域名放在 ip 字段导致 xray 报错），
	// SSRF/BT 阻断已由服务端 kernelrender.InjectAuditRules 正确注入（IP CIDR 格式）。
	s.injectAuditRulesCompat(cloned)
	cv.ContentJSON = cloned

	// 801 修复：path3（hash 不变）也更新 dispatch_status
	// 当 agent 重启后 version=0 触发 re-fetch，config hash 未变走 path3，
	// 但 dispatch_status 仍需更新为 pushed，否则旧 failed 状态永远不会被清除
	path3AllNodes := nodes
	if len(xrayNodes) > 0 {
		path3AllNodes = make([]*model.Node, 0, len(nodes)+len(xrayNodes))
		path3AllNodes = append(path3AllNodes, nodes...)
		path3AllNodes = append(path3AllNodes, xrayNodes...)
	}
	s.markNodesDispatchStatus(ctx, path3AllNodes, "pushed", cv.VersionNo, "")

	return cv, nil
}

// pushConfigToRuntime P0-7: 查找 runtime 关联的 server，主动推送配置到 agent。
// 返回推送错误（agent 仍可通过心跳兜底拉取），调用方据此标记 dispatch status。
func (s *DeploymentService) pushConfigToRuntime(ctx context.Context, runtimeID uuid.UUID, cv *model.ConfigVersion) error {
	if s.configPusher == nil || cv == nil {
		return fmt.Errorf("config pusher not available")
	}
	// runtime → server.Code
	rt, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil || rt == nil {
		s.logger.Warn("pushConfig: runtime not found", "runtime_id", runtimeID, "error", err)
		return fmt.Errorf("runtime not found: %w", err)
	}
	server, err := s.serverRepo.GetByID(ctx, rt.ServerID)
	if err != nil || server == nil {
		s.logger.Warn("pushConfig: server not found", "server_id", rt.ServerID, "error", err)
		return fmt.Errorf("server not found: %w", err)
	}
	// P5: Agent 版本强约束 — 旧 Agent 无法处理 _xray_config（P2 内核翻转）
	if agentVer, ok := server.Metadata["agent_version"].(string); ok && agentVer != "" {
		if compareVersion(agentVer, MinAgentVersionForP2) < 0 {
			errMsg := fmt.Sprintf("agent version %s below minimum %s (P2 kernel flip requires _xray_config support)",
				agentVer, MinAgentVersionForP2)
			s.logger.Error("pushConfig: agent version too old, rejecting config push",
				"server_code", server.Code, "agent_version", agentVer, "min_version", MinAgentVersionForP2)
			return fmt.Errorf("%s", errMsg)
		}
	}
	if err := s.configPusher.PushConfig(ctx, server.Code, cv); err != nil {
		s.logger.Warn("pushConfig: push failed, agent will fallback to heartbeat",
			"server_code", server.Code, "version", cv.VersionNo, "error", err)
		return err
	}
	return nil
}

// updateNodesPublishedVersion P1-2: 批量更新 runtime 下所有节点的 last_published_version。
// 配置版本发布成功后调用，使前端"已发布版本"字段实时反映真实状态。
// 失败只记录日志，不阻断配置下发流程（下次刷新会自动纠正）。
func (s *DeploymentService) updateNodesPublishedVersion(ctx context.Context, nodes []*model.Node, versionNo int64) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		if err := s.nodeRepo.UpdatePublishedVersion(ctx, n.ID, versionNo); err != nil {
			s.logger.Warn("update node last_published_version failed",
				"node_id", n.ID, "version", versionNo, "error", err)
		}
	}
}

// markNodesDispatchStatus P2-1: 批量标记 runtime 下节点的配置下发状态。
// 失败只记录日志，不阻断流程。
func (s *DeploymentService) markNodesDispatchStatus(ctx context.Context, nodes []*model.Node, status string, version int64, errMsg string) {
	nodeIDs := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			nodeIDs = append(nodeIDs, n.ID)
		}
	}
	if err := s.nodeRepo.UpdateDispatchStatus(ctx, nodeIDs, status, version, errMsg); err != nil {
		s.logger.Warn("mark nodes dispatch status failed",
			"status", status, "version", version, "error", err)
	}
}

// dispatchAndPush B-收口：封装配置下发的状态流转（lpv更新 → pending → push → pushed/failed）。
// 收口 path1（cv==nil）和 path2（hash变）的重复逻辑，确保"pending→pushed/failed"流转只在一处定义。
// path3（hash不变）不需要 push，仍直接调用 markNodesDispatchStatus("pushed")。
func (s *DeploymentService) dispatchAndPush(ctx context.Context, runtimeID uuid.UUID, allNodes []*model.Node, cv *model.ConfigVersion) {
	s.updateNodesPublishedVersion(ctx, allNodes, cv.VersionNo)
	s.markNodesDispatchStatus(ctx, allNodes, "pending", cv.VersionNo, "")
	if pushErr := s.pushConfigToRuntime(ctx, runtimeID, cv); pushErr != nil {
		s.markNodesDispatchStatus(ctx, allNodes, "failed", cv.VersionNo, pushErr.Error())
	} else {
		s.markNodesDispatchStatus(ctx, allNodes, "pushed", cv.VersionNo, "")
	}
}

// SelfHealFailedDispatch D-自愈：清理长时间停留在 failed 的节点下发状态。
// 复用心跳周期（10s）作为巡检触发点，无需独立 goroutine。
// 仅处理 failed 超 5 分钟的节点：重置为 pushed，下次 agent re-fetch 时自然收敛。
// 根因 #4 根治：避免 agent 重启后 version=0 触发 re-fetch 时旧 failed 状态永远不清除。
func (s *DeploymentService) SelfHealFailedDispatch(ctx context.Context, runtimeID uuid.UUID, latestVersion int64) {
	nodes, err := s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
	if err != nil {
		return
	}
	var stale []*model.Node
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, n := range nodes {
		if n == nil || n.Metadata == nil {
			continue
		}
		if status, _ := n.Metadata["_dispatch_status"].(string); status == "failed" {
			// 以 _dispatch_time 为陈旧判定基准（UpdateDispatchStatus 每次写状态都会刷新），
			// 避免节点其他字段更新刷新 updated_at 导致 failed 永远不被自愈。
			// 旧数据无 _dispatch_time 时回退到 updated_at 兼容。
			ts := n.UpdatedAt
			if raw, ok := n.Metadata["_dispatch_time"].(string); ok {
				if t, perr := time.Parse(time.RFC3339Nano, raw); perr == nil {
					ts = t
				}
			}
			if ts.Before(cutoff) {
				stale = append(stale, n)
			}
		}
	}
	if len(stale) > 0 {
		s.logger.Warn("self-heal: resetting stale failed dispatch status",
			"runtime_id", runtimeID, "count", len(stale), "latest_version", latestVersion)
		s.markNodesDispatchStatus(ctx, stale, "pushed", latestVersion, "")
	}
}

// SelfHealDispatchStatus 三通道统一自愈入口（HTTP/WS/gRPC 心跳均调用）：
//   - 版本一致（lpv==latestVersion）且内核运行中 → failed 刷 applied（可信收敛，不重拉）；
//   - 其余 failed 且超过 5 分钟（以 _dispatch_time 判定）→ 刷 pushed（触发 agent 重拉自愈）。
//
// 覆盖同 server 的 sing-box + xray 全部节点，避免通道不同导致自愈语义分叉。
func (s *DeploymentService) SelfHealDispatchStatus(ctx context.Context, runtimeID uuid.UUID, latestVersion int64, kernelRunning bool) {
	allNodes, err := s.nodesForRuntimeAndServer(ctx, runtimeID)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-5 * time.Minute)
	var appliedNodes, stale []*model.Node
	for _, n := range allNodes {
		if n == nil || n.Metadata == nil {
			continue
		}
		if status, _ := n.Metadata["_dispatch_status"].(string); status != "failed" {
			continue
		}
		ts := n.UpdatedAt
		if raw, ok := n.Metadata["_dispatch_time"].(string); ok {
			if t, perr := time.Parse(time.RFC3339Nano, raw); perr == nil {
				ts = t
			}
		}
		if kernelRunning && n.LastPublishedVersion == latestVersion {
			appliedNodes = append(appliedNodes, n)
			continue
		}
		if ts.Before(cutoff) {
			stale = append(stale, n)
		}
	}
	if len(appliedNodes) > 0 {
		s.markNodesDispatchStatus(ctx, appliedNodes, "applied", latestVersion, "")
		s.logger.Info("dispatch self-heal: stale failed -> applied",
			"runtime_id", runtimeID, "version", latestVersion, "count", len(appliedNodes))
	}
	if len(stale) > 0 {
		s.markNodesDispatchStatus(ctx, stale, "pushed", latestVersion, "")
		s.logger.Warn("dispatch self-heal: stale failed -> pushed (retry)",
			"runtime_id", runtimeID, "count", len(stale), "latest_version", latestVersion)
	}
}

// PushUserDeltaToRuntime 向指定 runtime 关联的 agent 推送增量用户变更。
// 当变更仅涉及用户增删（非结构化配置变更）时，使用 DeltaSync 替代全量 ConfigPush，
// 实现 sub-second 级零断流热更。推送失败不阻断业务（agent 仍可通过心跳兜底全量同步）。
func (s *DeploymentService) PushUserDeltaToRuntime(ctx context.Context, runtimeID uuid.UUID, adds []UserChangeEntry, removes []string, configVersion int64) {
	if s.userDeltaSvc == nil || (len(adds) == 0 && len(removes) == 0) {
		return
	}
	rt, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil || rt == nil {
		s.logger.Warn("PushUserDelta: runtime not found", "runtime_id", runtimeID, "error", err)
		return
	}
	server, err := s.serverRepo.GetByID(ctx, rt.ServerID)
	if err != nil || server == nil {
		s.logger.Warn("PushUserDelta: server not found", "server_id", rt.ServerID, "error", err)
		return
	}
	runtimeType := "xray"
	if rt.RuntimeType != "" {
		runtimeType = rt.RuntimeType
	}
	if err := s.userDeltaSvc.OnUsersChanged(ctx, server.Code, runtimeType, adds, removes, configVersion); err != nil {
		s.logger.Warn("PushUserDelta: delta push failed, agent will fallback to full sync",
			"server_code", server.Code, "config_version", configVersion, "error", err)
	}
}

// ===== P3-1: Payload Manifest 加密封包 =====

// tryCreatePayload P3-1: 为 ConfigVersion 构建加密 Payload Manifest 并持久化。
// 失败只记录日志，不阻断明文配置的下发流程（兼容期双写策略）。
func (s *DeploymentService) tryCreatePayload(ctx context.Context, cv *model.ConfigVersion, kernel string) {
	if cv == nil || len(s.payloadKey) == 0 {
		return
	}
	if _, err := s.CreatePayload(ctx, cv, kernel); err != nil {
		s.logger.Warn("create payload manifest failed, plaintext config still available",
			"config_version_id", cv.ID, "version_no", cv.VersionNo, "error", err)
	}
}

// CreatePayload P3-1: 为指定 ConfigVersion 构建 AES-GCM 加密的 Payload Manifest，
// 并写入 config_payloads 表。kernel 为运行时内核类型（xray / sing-box）。
func (s *DeploymentService) CreatePayload(ctx context.Context, cv *model.ConfigVersion, kernel string) (*model.ConfigPayload, error) {
	if cv == nil {
		return nil, ErrVersionNotFound
	}
	if kernel == "" {
		kernel = "xray"
	}
	if len(s.payloadKey) == 0 {
		return nil, fmt.Errorf("payload key not configured")
	}

	content := &crypto.PayloadContent{
		ConfigJSON: cv.ContentJSON,
	}
	// P1-12: 内联 TLS 证书材料到 Payload，使 Agent 可通过 FetchPayload 获取证书。
	// 仅对 runtime 作用域的配置版本填充（cv.ScopeID 即 runtime ID）。
	if cv.ScopeType == model.ScopeTypeRuntime {
		content.TLSMaterials = s.buildTLSMaterials(ctx, cv.ScopeID)
	}
	manifest, err := crypto.BuildManifest(cv.VersionNo, cv.ID.String(), kernel, content, s.payloadKey)
	if err != nil {
		return nil, fmt.Errorf("build manifest: %w", err)
	}

	payload := &model.ConfigPayload{
		ID:               uuid.New(),
		ConfigVersionID:  cv.ID,
		VersionNo:        manifest.VersionNo,
		SHA256:           manifest.SHA256,
		Kernel:           manifest.Kernel,
		RollbackStrategy: manifest.RollbackStrategy,
		PayloadEncrypted: manifest.PayloadEncrypted,
		Content:          manifest.Content,
	}
	if err := s.deploymentRepo.CreatePayload(ctx, payload); err != nil {
		return nil, fmt.Errorf("persist payload: %w", err)
	}

	s.logger.Info("payload manifest created",
		"config_version_id", cv.ID, "version_no", cv.VersionNo, "kernel", kernel, "sha256", manifest.SHA256)
	return payload, nil
}

// GetPayload P3-1: 按版本号查询加密 Payload Manifest（DB 模型）。
func (s *DeploymentService) GetPayload(ctx context.Context, versionNo int64) (*model.ConfigPayload, error) {
	payload, err := s.deploymentRepo.GetPayloadByVersionNo(ctx, versionNo)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, ErrPayloadNotFound
	}
	return payload, nil
}

// GetPayloadManifest P3-1: 按版本号查询加密 Payload，并组装为可返回给 Agent 的 PayloadManifest。
func (s *DeploymentService) GetPayloadManifest(ctx context.Context, versionNo int64) (*crypto.PayloadManifest, error) {
	payload, err := s.GetPayload(ctx, versionNo)
	if err != nil {
		return nil, err
	}
	return &crypto.PayloadManifest{
		VersionNo:        payload.VersionNo,
		DeploymentID:     payload.ConfigVersionID.String(),
		SHA256:           payload.SHA256,
		Timestamp:        payload.CreatedAt.Unix(),
		Kernel:           payload.Kernel,
		RollbackStrategy: payload.RollbackStrategy,
		PayloadEncrypted: payload.PayloadEncrypted,
		Content:          payload.Content,
	}, nil
}

// RecordDeploymentResult P3-1: 记录 Agent 上报的部署 ACK/NACK 结果。
// serverCode 为上报 Agent 对应的 server code；若结果为 nack 且关联了 deployment_target，
// 则同步更新 target 状态并触发既有回滚判定逻辑。
func (s *DeploymentService) RecordDeploymentResult(ctx context.Context, serverCode string, req *model.DeploymentResultRequest) (*model.DeploymentResult, error) {
	if serverCode == "" {
		return nil, ErrServerNotFound
	}
	var versionNo int64
	if req.Version != "" {
		fmt.Sscanf(req.Version, "%d", &versionNo)
	}
	// P0 修复：向后兼容旧版 agent（v0.4.0-）的 status 字符串字段。
	// 旧 agent 发送 status:"ack"/"nack" 而非 success:true/false，
	// 导致 Success 字段默认 false，面板误记录 nack。
	// 优先使用 Success 字段；若 Success=false，再 fallback 检查 Status 字段。
	status := "ack"
	if !req.Success {
		// fallback: 旧 agent 发送 status:"ack" 时，Success 默认 false 但 Status="ack"
		if req.Status != "ack" {
			status = "nack"
		}
	}
	// 向后兼容：旧 agent 发送 error 字段而非 message
	errMsg := req.Message
	if errMsg == "" && req.Error != "" {
		errMsg = req.Error
	}
	phase := req.Phase
	if phase == "" {
		phase = "activate"
	}
	dr := &model.DeploymentResult{
		ID:              uuid.New(),
		ServerCode:      serverCode,
		VersionNo:       versionNo,
		Status:          status,
		Phase:           phase,
		Error:           errMsg,
		ApplyDurationMs: req.DurationMs,
	}
	if req.DeploymentTargetID != nil {
		dr.DeploymentTargetID = *req.DeploymentTargetID
	}
	if err := s.deploymentRepo.CreateDeploymentResult(ctx, dr); err != nil {
		return nil, fmt.Errorf("record deployment result: %w", err)
	}

	// 若为 nack 且有 target 关联，触发既有 target 状态更新与回滚判定
	// P0: 使用计算后的 status（兼容旧 agent）而非 req.Success
	if status == "nack" && req.DeploymentTargetID != nil && *req.DeploymentTargetID != uuid.Nil {
		updateReq := &model.UpdateDeploymentResultRequest{
			TargetID: *req.DeploymentTargetID,
			Status:   model.TargetStatusFailed,
			ApplyResult: map[string]interface{}{
				"error":       errMsg,
				"duration_ms": req.DurationMs,
				"phase":       phase,
			},
		}
		if err := s.UpdateDeploymentResult(ctx, *req.DeploymentTargetID, updateReq); err != nil {
			s.logger.Warn("record deployment result: update target failed",
				"target_id", *req.DeploymentTargetID, "error", err)
		}
	}

	s.logger.Info("deployment result recorded",
		"server_code", serverCode, "version_no", versionNo, "status", status, "phase", phase)
	return dr, nil
}

// PushUserBanToAllServers P0-8: 向所有已连接的 agent 推送用户封禁通知。
// 遍历所有 server，对每个 server 调用 PushUserBan。
// 推送失败不阻断（agent 仍可通过心跳兜底感知用户封禁）。
func (s *DeploymentService) PushUserBanToAllServers(ctx context.Context, userIDs []string, reason string) {
	if s.configPusher == nil || len(userIDs) == 0 {
		return
	}
	// 列出所有 server，逐个推送
	servers, _, err := s.serverRepo.List(ctx, 1, 500, "", "")
	if err != nil {
		s.logger.Warn("PushUserBan: list servers failed", "error", err)
		return
	}
	for _, srv := range servers {
		if err := s.configPusher.PushUserBan(ctx, srv.Code, userIDs, reason); err != nil {
			s.logger.Debug("PushUserBan: push failed for server",
				"server_code", srv.Code, "error", err)
		}
	}
	s.logger.Info("user ban pushed to all servers",
		"server_count", len(servers), "user_count", len(userIDs), "reason", reason)
}

func (s *DeploymentService) buildRuntimeConfig(ctx context.Context, runtimeType string, nodes []*model.Node, listenHost string) (map[string]interface{}, error) {
	// P0 修复：按 Node ID 排序确保配置渲染顺序稳定。
	// 数据库查询 ListByRuntimeID 未使用 ORDER BY，每次返回行顺序可能不同，
	// 导致 inbounds 数组顺序变化 → JSON 序列化不同 → hash 不同 → 版本循环递增。
	// 排序后所有下游处理（凭证预取、链路构建、内核渲染）都使用一致的节点顺序。
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID.String() < nodes[j].ID.String()
	})

	// 通过 chainRepo 查询节点绑定的路由组（替代已删除的 Node.ChainID 字段）
	var chainID uuid.UUID
	var hasChain bool
	if s.chainRepo != nil && len(nodes) > 0 {
		nodeIDs := make([]uuid.UUID, len(nodes))
		for i, n := range nodes {
			nodeIDs[i] = n.ID
		}
		chainMap, err := s.chainRepo.ListChainBindingsForNodes(ctx, nodeIDs)
		if err == nil {
			for _, cids := range chainMap {
				if len(cids) > 0 {
					chainID = cids[0]
					hasChain = true
					break
				}
			}
		}
	}

	// 预取 per-user 凭证（多用户配置支持，P0-1 改用 fetchNodeCredsForBuild）
	creds := s.fetchNodeCredsForBuild(ctx, nodes)

	// P0-3: 四级校验前置门禁——在构建配置前执行 L1/L2/L3/L3.5 校验，
	// 任何节点校验失败则拒绝生成新 config_versions 记录。
	if err := s.preflightValidate(ctx, runtimeType, nodes, creds); err != nil {
		return nil, err
	}

	// S0-3 VPS 级冲突校验（端口/SNI/DNS）：按 server_id 分组后逐台校验
	if err := s.precheckRuntimeGroupedByServer(ctx, nodes); err != nil {
		return nil, err
	}

	if hasChain {
		chainSpec, err := s.buildChainSpec(ctx, chainID, nodes)
		if err != nil {
			return nil, err
		}
		return s.buildChainRuntimeConfig(ctx, runtimeType, nodes, chainSpec, listenHost, creds)
	}

	// P0-1: 统一走 IR→Compiler 链路（kernelrender），替代 exposure 直拼
	switch runtimeType {
	case "xray", "xray-core":
		config, err := s.buildXrayConfigViaKernelRender(ctx, nodes, listenHost, creds)
		if err != nil {
			return nil, err
		}
		// P3-C: 注入 outbound 策略（WARP/socks5/chain）
		s.injectOutboundPolicies(ctx, config, nodes, runtimeType)
		return config, nil
	case "sing-box", "singbox":
		config, err := s.buildSingboxConfigViaKernelRender(ctx, nodes, listenHost, creds)
		if err != nil {
			return nil, err
		}
		// P3-C: 注入 outbound 策略（WARP/socks5/chain）
		s.injectOutboundPolicies(ctx, config, nodes, runtimeType)
		return config, nil
	default:
		return s.buildDefaultRuntimeConfig(nodes), nil
	}
}

// buildNginxVhosts 扫描节点的 config_json，为 CDN 节点（含 cdn_address 的节点）
// 生成 nginx WS vhost 片段，并为所有启用节点（CDN + REALITY）生成 stream SNI 分流配置。
// 返回 vhosts map（https_snippet/stream_snippet/listen_port/stream_default_upstream）。
// 无 CDN/REALITY 节点时返回 nil。
//
// 字段结构：
//
//	{
//	  https_snippet: "...nginx server block with SSL certs...",
//	  stream_snippet: "...nginx stream SNI split config...",
//	  listen_port: 8445,
//	  stream_default_upstream: "upstream_cdn_xxx"
//	}
func (s *DeploymentService) buildNginxVhosts(nodes []*model.Node) map[string]interface{} {
	if len(nodes) == 0 {
		return nil
	}

	var wsEntries []*nginxrender.WSVhostEntry
	seenSNI := make(map[string]bool)
	var domains []string
	var streamEntries []StreamUpstreamEntry
	var streamDefaultUpstream string
	seenStreamSNI := make(map[string]bool)

	for _, n := range nodes {
		if n == nil || n.ConfigJSON == nil || n.DeletedAt != nil || !n.IsEnabled {
			continue
		}

		// P5.2: 使用 TerminationClass.NeedsStreamSNI() 做前置过滤，单一真相源。
		// 仅 CDN 节点（TerminationNginx/NginxPlusXray）需要 nginx 443 stream SNI 分流。
		// cf_edge/self_udp/self_tcp/reality 全部绕过 nginx stream：
		// - cf_edge: cloudflared 直连 xray
		// - self_udp: UDP 协议直接监听 0.0.0.0:UDP端口
		// - self_tcp: trojan+tls/ss/mieru 直连 0.0.0.0:port（P5 改造）
		// - reality: xray 0.0.0.0:port + dest=大厂域名（P5 改造，消除同 SNI 限制 + 退役 fallback 静态站）
		tc := ClassifyTermination(n)
		if !tc.NeedsStreamSNI() {
			continue
		}

		cdnAddr, _ := n.ConfigJSON["cdn_address"].(string)
		cdnAddr = strings.TrimSpace(cdnAddr)
		// 回退修复：cdn/cdn_saas 节点（TerminationNginx）config_json.cdn_address 缺失时以 SNI 回退。
		// 历史/导入数据未写入 cdn_address 会导致 WS/gRPC/HTTPUpgrade CDN 节点跳过 nginx 8445 vhost
		// 生成，且 stream 误将 SNI 透传到无 TLS 的本地端口（upstream_tls_*→plainport）→ TLS 握手失败。
		// CDN 节点必须由 nginx 持证书在 8445 终止 TLS，故以 SNI 作为 CDN 域名单一真相源回退。
		if cdnAddr == "" && tc == TerminationNginx {
			if n.SNI != nil {
				cdnAddr = strings.TrimSpace(*n.SNI)
			}
			if cdnAddr == "" {
				if sn, ok := n.ConfigJSON["sni"].(string); ok {
					cdnAddr = strings.TrimSpace(sn)
				}
			}
		}
		isReality := tc == TerminationReality

		if cdnAddr != "" {
			// 回退 P4：CDN 节点 nginx 持证书，nginx 8445 终止 TLS 后 proxy_pass http 回源。
			// 流量路径：CF → nginx 443 stream (SNI 分流) → nginx 8445 (TLS 终止 + location 路由)
			//         → proxy_pass http://127.0.0.1:serverPort → xray inbound（security=none，明文）
			// 同 SNI 多节点通过 path 区分，nginx 8445 按 location 路由到不同 xray inbound 端口。
			serverPort := n.Port
			if sp, ok := n.ConfigJSON["server_port"].(float64); ok && sp > 0 && sp <= 65535 {
				serverPort = int(sp)
			}

			// 域名收集（用于 ACME 证书签发，nginx 持有证书做 TLS termination）
			if !seenSNI[cdnAddr] {
				seenSNI[cdnAddr] = true
				domains = append(domains, cdnAddr)
			}

			// 生成 WSVhostEntry：nginx 8445 按 path 路由到 xray inbound（同 SNI 多 path 自动分组）
			// 覆盖 WS/gRPC/HTTPUpgrade/XHTTP 全协议，由 RenderWSVhostConf 按 ServerName 合并到同一 server 块
			// 注入 per-domain 证书路径，让 nginx 8445 能加载 ssl_certificate 完成 TLS termination
			// 证书路径与 node-agent certmagic 存储约定一致：/etc/yundu/certs/{domain}/fullchain.pem
			// 回退 P4：BackendHTTPS=false 让 nginx proxy_pass http 回源到 xray（security=none）
			nodePath := extractNodePath(n, "")
			if nodePath != "" && serverPort > 0 {
				wsEntries = append(wsEntries, &nginxrender.WSVhostEntry{
					ServerName:    cdnAddr,
					WSPath:        nodePath,
					InternalPort:  serverPort,
					CertPath:      fmt.Sprintf("/etc/yundu/certs/%s/fullchain.pem", cdnAddr),
					KeyPath:       fmt.Sprintf("/etc/yundu/certs/%s/privkey.pem", cdnAddr),
					IsGRPC:        strings.EqualFold(n.TransportType, "grpc"),
					IsHTTPUpgrade: strings.EqualFold(n.TransportType, "httpupgrade"),
					IsXHTTP:       strings.EqualFold(n.TransportType, "xhttp"),
					// 回退 P4：nginx 持证书架构下 nginx 通过 proxy_pass http 回源到 xray（security=none）
					BackendHTTPS: false,
				})
			}

			// stream SNI 路由：CDN 域名统一转发到 nginx 8445（nginx 做 TLS termination + path 路由）
			// 8445 由 cfg.HTTPSListenPort 定义（默认 8445），骨架已支持按需生成 HTTP server block
			if !seenStreamSNI[cdnAddr] {
				seenStreamSNI[cdnAddr] = true
				upstreamID := "upstream_cdn_" + sanitizeSNI(cdnAddr)
				streamEntries = append(streamEntries, StreamUpstreamEntry{
					SNI:        cdnAddr,
					UpstreamID: upstreamID,
					TargetAddr: fmt.Sprintf("127.0.0.1:%d", 8445),
					Mode:       "cdn_stream",
				})
				if streamDefaultUpstream == "" {
					streamDefaultUpstream = upstreamID
				}
			}
		}

		if isReality && cdnAddr == "" {
			realitySNI := extractRealitySNI(n)
			realityServerPort := extractRealityServerPort(n)

			if realitySNI != "" && realityServerPort > 0 {
				listenPort := n.Port
				if n.ServerPort != nil && *n.ServerPort > 0 {
					listenPort = *n.ServerPort
				}
				// 多节点共用同一 REALITY 伪装 SNI（如 mesu.apple.com）是正常的伪装需求。
				// nginx stream map 对同一 SNI 只能路由到单一后端，因此：
				// - 第一个节点：注册到 nginx stream SNI 分流
				// - 后续相同 SNI 的节点：跳过 nginx stream 注册（需改为 direct 类型由 xray 直接监听 443）
				// 不再自动拼接前缀（如 pp09.mesu.apple.com），那会破坏 REALITY 伪装
				if !seenStreamSNI[realitySNI] {
					seenStreamSNI[realitySNI] = true
					if streamDefaultUpstream == "" && listenPort == 443 {
						streamDefaultUpstream = "upstream_reality_" + sanitizeSNI(n.Code)
					}
					streamEntries = append(streamEntries, StreamUpstreamEntry{
						SNI:        realitySNI,
						UpstreamID: "upstream_reality_" + sanitizeSNI(n.Code),
						TargetAddr: fmt.Sprintf("127.0.0.1:%d", realityServerPort),
						Mode:       "reality",
					})
				}
			}
		}

		// TLS 直连节点（trojan+tcp+tls 等）：security=tls 且无 CDN 地址，
		// xray 绑定 127.0.0.1:ServerPort，nginx stream 按 SNI 透传到该端口。
		// 与 REALITY 直连分支（上方）对称，覆盖 security=tls 的直连场景。
		if !isReality && cdnAddr == "" {
			isTLS := false
			if sec, ok := n.ConfigJSON["security"].(string); ok && strings.EqualFold(sec, "tls") {
				isTLS = true
			}
			if n.SecurityType != nil && strings.EqualFold(*n.SecurityType, "tls") {
				isTLS = true
			}
			if isTLS {
				tlsSNI := ""
				if n.SNI != nil {
					tlsSNI = strings.TrimSpace(*n.SNI)
				}
				if tlsSNI == "" {
					if sn, ok := n.ConfigJSON["sni"].(string); ok {
						tlsSNI = strings.TrimSpace(sn)
					}
				}
				if tlsSNI == "" {
					if ts, ok := n.ConfigJSON["tls_settings"].(map[string]interface{}); ok {
						if sn, ok := ts["server_name"].(string); ok && sn != "" {
							tlsSNI = strings.TrimSpace(sn)
						} else if sn, ok := ts["sni"].(string); ok && sn != "" {
							tlsSNI = strings.TrimSpace(sn)
						}
					}
				}
				tlsServerPort := 0
				if n.ServerPort != nil && *n.ServerPort > 0 {
					tlsServerPort = *n.ServerPort
				}
				if tlsServerPort == 0 {
					if sp, ok := n.ConfigJSON["server_port"].(float64); ok && sp > 0 && sp <= 65535 {
						tlsServerPort = int(sp)
					}
				}
				if tlsSNI != "" && tlsServerPort > 0 && !seenStreamSNI[tlsSNI] {
					seenStreamSNI[tlsSNI] = true
					streamEntries = append(streamEntries, StreamUpstreamEntry{
						SNI:        tlsSNI,
						UpstreamID: "upstream_tls_" + sanitizeSNI(n.Code),
						TargetAddr: fmt.Sprintf("127.0.0.1:%d", tlsServerPort),
						Mode:       "tls",
					})
				}
				if tlsSNI != "" && !seenSNI[tlsSNI] {
					seenSNI[tlsSNI] = true
					domains = append(domains, tlsSNI)
				}
			}
		}

		// XHTTP split mode：处理 downloadSettings 下行 REALITY/TLS 的 stream SNI 分流
		if strings.EqualFold(n.TransportType, "xhttp") {
			var ds map[string]interface{}
			if extra, ok := n.ConfigJSON["xhttp"].(map[string]interface{}); ok {
				if d, ok := extra["extra"].(map[string]interface{}); ok {
					if dsRaw, ok := d["downloadSettings"].(map[string]interface{}); ok {
						ds = dsRaw
					}
				}
			}
			if ds == nil {
				if extra, ok := n.ConfigJSON["xhttp_extra"].(map[string]interface{}); ok {
					if dsRaw, ok := extra["downloadSettings"].(map[string]interface{}); ok {
						ds = dsRaw
					}
				}
			}
			if ds != nil {
				dlSec, _ := ds["security"].(string)
				dlSec = strings.ToLower(dlSec)
				dlSNI := ""
				if dlSec == "reality" {
					var rs map[string]interface{}
					if r, ok := ds["realitySettings"].(map[string]interface{}); ok && len(r) > 0 {
						rs = r
					} else if r, ok := ds["reality"].(map[string]interface{}); ok && len(r) > 0 {
						rs = r
					}
					if rs != nil {
						if sn, ok := rs["serverName"].(string); ok && sn != "" {
							dlSNI = sn
						} else if sn, ok := rs["server_name"].(string); ok && sn != "" {
							dlSNI = sn
						} else if sn, ok := rs["sni"].(string); ok && sn != "" {
							dlSNI = sn
						}
					}
				} else if dlSec == "tls" {
					var ts map[string]interface{}
					if t, ok := ds["tlsSettings"].(map[string]interface{}); ok && len(t) > 0 {
						ts = t
					} else if t, ok := ds["tls"].(map[string]interface{}); ok && len(t) > 0 {
						ts = t
					}
					if ts != nil {
						if sn, ok := ts["serverName"].(string); ok && sn != "" {
							dlSNI = sn
						} else if sn, ok := ts["server_name"].(string); ok && sn != "" {
							dlSNI = sn
						} else if sn, ok := ts["sni"].(string); ok && sn != "" {
							dlSNI = sn
						}
					}
				}
				// 兼容裸奔sni字段（旧格式）
				if dlSNI == "" {
					if sn, ok := ds["sni"].(string); ok && sn != "" {
						dlSNI = sn
					} else if sn, ok := ds["serverName"].(string); ok && sn != "" {
						dlSNI = sn
					} else if sn, ok := ds["server_name"].(string); ok && sn != "" {
						dlSNI = sn
					}
				}
				dlPort := 0
				if p, ok := ds["port"].(float64); ok && p > 0 && p <= 65535 {
					dlPort = int(p)
				}
				// 下行监听端口：优先server_port（服务端高位端口），否则用port
				if sp, ok := ds["server_port"].(float64); ok && sp > 0 && sp <= 65535 {
					dlPort = int(sp)
				}
				// 下行 address（CDN 域名或直连 IP），用于判断走 CDN 还是直连
				dlAddr, _ := ds["address"].(string)
				isDlLoopback := dlAddr == "127.0.0.1" || dlAddr == "localhost"
				// 下行 TLS 走 CDN（address 是域名，非回环/空）→ 为下行 address 生成 stream SNI 路由
				// 注意：stream SNI 必须用下行 address（CDN 域名），不是 tlsSettings.serverName（伪装 SNI）
				// 因为客户端连 address → CF CDN → nginx stream，nginx 按 SNI=address 匹配路由
				// P2 TLS分离架构改造 719：不再生成 WSVhostEntry，改为 stream 透传
				if dlSec == "tls" && dlAddr != "" && !isDlLoopback && dlPort > 0 {
					// P2 TLS分离架构改造 719：下行 CDN TLS 也走 stream 透传，不再生成 WSVhostEntry。
					// 客户端连 dlAddr → CF CDN → nginx stream → xray 下行端口（xray 自终止 TLS）
					if !seenStreamSNI[dlAddr] {
						seenStreamSNI[dlAddr] = true
						streamEntries = append(streamEntries, StreamUpstreamEntry{
							SNI:        dlAddr,
							UpstreamID: "upstream_dl_" + sanitizeSNI(n.Code),
							TargetAddr: fmt.Sprintf("127.0.0.1:%d", dlPort),
							Mode:       "dl_stream",
						})
					}
					if !seenSNI[dlAddr] {
						seenSNI[dlAddr] = true
						domains = append(domains, dlAddr)
					}
				}
				// 下行 SNI 和端口存在，且不是回环地址（需要对外暴露）
				// 下行 TLS 走 CDN 时已用 dlAddr 作为 SNI 生成 stream 路由（上方分支），
				// 不再用 dlSNI（伪装 SNI）生成 stream 路由，避免重复
				if dlSNI != "" && dlPort > 0 && !seenStreamSNI[dlSNI] {
					skipStream := false
					if dlSec == "tls" && dlAddr != "" && !isDlLoopback {
						skipStream = true
					}
					if !skipStream {
						seenStreamSNI[dlSNI] = true
						streamEntries = append(streamEntries, StreamUpstreamEntry{
							SNI:        dlSNI,
							UpstreamID: "upstream_dl_" + sanitizeSNI(n.Code) + "_" + sanitizeSNI(dlSNI),
							TargetAddr: fmt.Sprintf("127.0.0.1:%d", dlPort),
							Mode:       dlSec,
						})
					}
				}
			}
		}
	}

	cfg := nginxrender.DefaultConfig()
	httpsSnippet := ""
	streamSnippet := ""

	if len(wsEntries) > 0 {
		// WS/gRPC/HTTPUpgrade/XHTTP 统一由 RenderWSVhostConf 按 server_name 分组渲染，
		// 同一域名的多种传输类型合并到同一 server 块，避免 conflicting server_name
		httpsSnippet = nginxrender.RenderWSVhostConf(wsEntries, cfg, "", "")
	}

	streamSnippet = renderStreamSnippet(streamEntries, streamDefaultUpstream)

	if len(wsEntries) == 0 && len(streamEntries) == 0 {
		return nil
	}

	vhosts := map[string]interface{}{
		"https_snippet":           httpsSnippet,
		"stream_snippet":          streamSnippet,
		"listen_port":             cfg.HTTPSListenPort,
		"domains":                 domains,
		"stream_default_upstream": streamDefaultUpstream,
	}

	s.logger.Info("nginx vhosts built",
		"ws_entries", len(wsEntries),
		"sni_hosts", len(seenSNI),
		"domains", len(domains),
		"stream_upstreams", len(streamEntries))

	return vhosts
}

// injectNginxVhosts 将 buildNginxVhosts 的结果注入到 config 的 _nginx_vhosts 字段。
// node-agent 在 applyConfig 时读取此字段并自动同步 nginx 配置。
func (s *DeploymentService) injectNginxVhosts(config map[string]interface{}, nodes []*model.Node) {
	if config == nil {
		return
	}
	vhosts := s.buildNginxVhosts(nodes)
	if vhosts != nil {
		config["_nginx_vhosts"] = vhosts
	}
}

// injectTrafficQuota P3-N: 将节点级流量限额信息注入到 config 的 _traffic_quota 字段。
//
// 结构：
//
//	{
//	  "<node_id>": {
//	    "transfer_enable_bytes": 10737418240,
//	    "node_code": "sg01"
//	  },
//	  ...
//	}
//
// 仅包含设置了流量限额（transfer_enable_bytes > 0）的节点。
// node-agent 可读取此字段感知限额，在接近限额时主动降速或拒绝新连接。
// 注入在 hash 计算之后，不影响配置签名一致性。
func (s *DeploymentService) injectTrafficQuota(config map[string]interface{}, nodes []*model.Node) {
	if config == nil || len(nodes) == 0 {
		return
	}
	quotas := make(map[string]interface{})
	for _, n := range nodes {
		if n == nil || n.TransferEnableBytes == nil || *n.TransferEnableBytes <= 0 {
			continue
		}
		quotas[n.ID.String()] = map[string]interface{}{
			"transfer_enable_bytes": *n.TransferEnableBytes,
			"node_code":             n.Code,
		}
	}
	if len(quotas) > 0 {
		config["_traffic_quota"] = quotas
	}
}

// injectAuditRulesCompat 注入 _audit_rules 字段，禁用 node-agent 端默认审计规则中的
// SSRF block（agent 将域名 "localhost" 等放入 xray "ip" 字段导致 "invalid IP: localhost" 配置校验失败）。
// SSRF/BT 阻断已由服务端 kernelrender.InjectAuditRules 正确注入（使用合法 IP CIDR 格式），
// 此处通过 _audit_rules 显式传递规则来覆盖 agent 端默认值，避免重复/错误注入。
func (s *DeploymentService) injectAuditRulesCompat(config map[string]interface{}) {
	if config == nil {
		return
	}
	config["_audit_rules"] = map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"type": "bt_block", "enabled": false},
			map[string]interface{}{"type": "ssrf_block", "enabled": false},
		},
	}
}

// injectXrayConfig 双内核架构（P2 翻转）：当渲染 sing-box 配置时，查询同 server 下的 xray runtime，
// 如果存在 xray 节点，渲染 xray 配置并嵌入到 sing-box 配置的 _xray_config 字段。
// Agent 拉取 sing-box 配置后，提取 _xray_config 并应用到 xray 内核（懒加载）。
// 这避免了需要修改心跳协议支持多 runtime 配置同步的复杂性。
//
// 返回值：成功注入时返回 xray 节点列表，供调用方更新 last_published_version / dispatch_status。
// 双核内嵌模式下 xhttp 节点挂在 xray runtime 下，配置通过 _xray_config 嵌入 sing-box 配置下发。
// 若不返回此列表，xhttp 节点的 lpv/dispatch_status 永远不更新，面板显示"下发失败"（实际已下发）。
func (s *DeploymentService) injectXrayConfig(ctx context.Context, config map[string]interface{}, sbRT *model.Runtime) []*model.Node {
	if config == nil || sbRT == nil {
		return nil
	}
	// P2 翻转：当前 runtime 必须是 sing-box（主内核），才嵌入 xray 配置
	if normalizeRuntimeType(sbRT.RuntimeType) != "sing-box" {
		return nil
	}
	runtimes, err := s.runtimeRepo.ListByServer(ctx, sbRT.ServerID)
	if err != nil {
		s.logger.Warn("injectXrayConfig: ListByServer failed", "error", err)
		return nil
	}
	var xrayRT *model.Runtime
	for _, rt := range runtimes {
		if isXrayRuntime(rt.RuntimeType) {
			xrayRT = rt
			break
		}
	}
	if xrayRT == nil {
		return nil
	}
	// 查询 xray runtime 下的节点
	xrayNodes, err := s.nodeRepo.ListByRuntimeID(ctx, xrayRT.ID)
	if err != nil {
		s.logger.Warn("injectXrayConfig: ListByRuntimeID for xray failed", "error", err)
		return nil
	}
	if len(xrayNodes) == 0 {
		return nil // xray runtime 无节点
	}
	// 渲染 xray 配置
	xrayListenHost := ""
	if xrayRT.ListenHost != nil {
		xrayListenHost = *xrayRT.ListenHost
	}
	xrayCreds := s.fetchNodeCredsForBuild(ctx, xrayNodes)
	xrayConfig, err := s.buildXrayConfigViaKernelRender(ctx, xrayNodes, xrayListenHost, xrayCreds)
	if err != nil {
		s.logger.Warn("injectXrayConfig: buildXrayConfig failed", "error", err)
		return nil
	}
	// 注入 nginx vhosts 和 traffic quota 到 xray 配置
	s.injectNginxVhosts(xrayConfig, xrayNodes)
	s.injectTrafficQuota(xrayConfig, xrayNodes)
	config["_xray_config"] = xrayConfig
	s.logger.Info("injectXrayConfig: xray config embedded into sing-box config",
		"xray_runtime_id", xrayRT.ID, "xray_node_count", len(xrayNodes))
	return xrayNodes
}

// GetNodesByRuntimeID D7 修复: 返回指定 runtime 下的所有节点。
// 供 CloudflaredTunnels handler 查询隧道类型节点用。
func (s *DeploymentService) GetNodesByRuntimeID(ctx context.Context, runtimeID uuid.UUID) ([]*model.Node, error) {
	return s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
}

// GetCDNVhosts 查询指定 runtime 下的 CDN 节点，返回渲染好的 nginx vhost snippet。
// 供 node-agent 的 nginx reconciler 独立轮询调用，不依赖 xray config_versions 版本管理。
func (s *DeploymentService) GetCDNVhosts(ctx context.Context, runtimeID uuid.UUID) (map[string]interface{}, error) {
	nodes, err := s.nodeRepo.ListByRuntimeID(ctx, runtimeID)
	if err != nil {
		return nil, err
	}
	vhosts := s.buildNginxVhosts(nodes)
	if vhosts == nil {
		return map[string]interface{}{
			"https_snippet":           "",
			"stream_snippet":          "",
			"listen_port":             8445,
			"stream_default_upstream": "",
		}, nil
	}
	return vhosts, nil
}

// BuildNginxVhostsForServer 查询指定 server 下所有启用节点，返回聚合的 nginx vhost snippet。
// 供 Machine 模式下 MachineOrchestrator 拉取整台机器的聚合 nginx 配置使用。
func (s *DeploymentService) BuildNginxVhostsForServer(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	nodes, err := s.nodeRepo.ListByServerID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if err := s.precheckRuntimePerServer(ctx, nodes); err != nil {
		s.logger.Error("BuildNginxVhostsForServer: runtime precheck failed", "server_id", serverID, "error", err)
		return nil, err
	}
	vhosts := s.buildNginxVhosts(nodes)
	if vhosts == nil {
		return map[string]interface{}{
			"https_snippet":           "",
			"stream_snippet":          "",
			"listen_port":             8445,
			"domains":                 []string{},
			"stream_default_upstream": "",
		}, nil
	}
	if _, ok := vhosts["domains"]; !ok {
		vhosts["domains"] = []string{}
	}
	return vhosts, nil
}

func (s *DeploymentService) buildChainSpec(ctx context.Context, chainID uuid.UUID, entryNodes []*model.Node) (*chain.ChainSpec, error) {
	proxyChain, err := s.chainRepo.GetByID(ctx, chainID)
	if err != nil {
		return nil, err
	}
	if proxyChain == nil {
		return nil, ErrChainNotFound
	}

	hops, err := s.chainRepo.ListHops(ctx, chainID)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[uuid.UUID]*model.Node)
	for _, n := range entryNodes {
		nodeMap[n.ID] = n
	}

	allNodes := append([]*model.Node{}, entryNodes...)
	for _, hop := range hops {
		if hop.UpstreamNodeID != nil {
			if _, exists := nodeMap[*hop.UpstreamNodeID]; !exists {
				n, err := s.nodeRepo.GetByID(ctx, *hop.UpstreamNodeID)
				if err != nil {
					return nil, err
				}
				if n != nil {
					allNodes = append(allNodes, n)
					nodeMap[n.ID] = n
				}
			}
		}
	}

	relays := make([]chain.ChainHop, 0)
	var landingNode *nodespec.NodeSpec

	for i, hop := range hops {
		if hop.UpstreamNodeID == nil {
			continue
		}
		n, ok := nodeMap[*hop.UpstreamNodeID]
		if !ok {
			continue
		}

		ns := modelNodeToNodeSpec(n)
		chainHop := chain.ChainHop{
			NodeID:      n.ID.String(),
			Protocol:    nodespec.Protocol(n.ProtocolType),
			Address:     n.Address,
			Port:        n.Port,
			Credentials: extractCredentials(n),
			Transport:   buildTransportConfig(n),
			Security:    nodespec.Security(getSecurityType(n)),
			TLS:         buildTLSConfig(n),
			Reality:     buildRealityConfig(n),
			Tag:         fmt.Sprintf("relay-%d", i),
		}

		if i == len(hops)-1 {
			landingNode = ns
		} else {
			relays = append(relays, chainHop)
		}
	}

	if landingNode == nil && len(relays) > 0 {
		landingNode = modelNodeToNodeSpec(entryNodes[0])
	}

	if landingNode == nil {
		landingNode = modelNodeToNodeSpec(entryNodes[0])
		relays = relays[:0]
	}

	cs := &chain.ChainSpec{
		ID:          proxyChain.ID.String(),
		Name:        proxyChain.Name,
		LandingNode: landingNode,
		Relays:      relays,
	}

	if err := cs.Validate(); err != nil {
		return nil, err
	}

	return cs, nil
}

func (s *DeploymentService) buildChainRuntimeConfig(ctx context.Context, runtimeType string, nodes []*model.Node, cs *chain.ChainSpec, listenHost string, creds exposure.NodeCredentials) (map[string]interface{}, error) {
	switch runtimeType {
	case "xray", "xray-core":
		// R1: 使用 kernelrender 生成 inbounds，替代 exposure.BuildXrayInboundsWithCreds
		fullConfig, err := s.buildXrayConfigViaKernelRender(ctx, nodes, listenHost, creds)
		if err != nil {
			return nil, err
		}
		inbounds, _ := fullConfig["inbounds"].([]interface{})

		// P1-1: 使用 chain.RenderChainForKernel 统一入口，替代 exposure.BuildXrayChainOutbounds
		chainResult, err := chain.RenderChainForKernel(chain.KernelXray, cs)
		if err != nil {
			return nil, err
		}
		chainOutbounds := chainResult.Outbounds
		chainRouting := chainResult.Routing

		baseRouting := chain.DefaultXrayRouting()
		rules, _ := baseRouting["rules"].([]interface{})
		if chainRules, ok := chainRouting["rules"].([]interface{}); ok {
			rules = append(rules, chainRules...)
		}
		if final, ok := chainRouting["final"]; ok {
			baseRouting["final"] = final
		}
		baseRouting["rules"] = rules

		return map[string]interface{}{
			"log": map[string]interface{}{
				"loglevel": "warning",
			},
			"inbounds":  inbounds,
			"outbounds": chainOutbounds,
			"routing":   baseRouting,
		}, nil

	case "sing-box", "singbox":
		// R1: 使用 kernelrender 生成 inbounds，替代 exposure.BuildSingboxInboundsWithCreds
		sbFullConfig, err := s.buildSingboxConfigViaKernelRender(ctx, nodes, listenHost, creds)
		if err != nil {
			return nil, err
		}
		sbInbounds, _ := sbFullConfig["inbounds"].([]interface{})

		// P1-1: 使用 chain.RenderChainForKernel 统一入口，替代 exposure.BuildSingboxChainOutbounds
		chainResult, err := chain.RenderChainForKernel(chain.KernelSingBox, cs)
		if err != nil {
			return nil, err
		}
		chainOutbounds := chainResult.Outbounds
		chainRoute := chainResult.Routing

		return map[string]interface{}{
			"log": map[string]interface{}{
				"level": "warn",
			},
			"inbounds":  sbInbounds,
			"outbounds": chainOutbounds,
			"route":     chainRoute,
		}, nil

	default:
		return s.buildDefaultRuntimeConfig(nodes), nil
	}
}

func getSecurityType(n *model.Node) string {
	if n.SecurityType != nil {
		return *n.SecurityType
	}
	return "none"
}

func extractCredentials(n *model.Node) interface{} {
	if n.ConfigJSON == nil {
		return nil
	}

	switch n.ProtocolType {
	case "vless":
		creds := nodespec.VLESSCredentials{}
		if uuid, ok := n.ConfigJSON["uuid"].(string); ok {
			creds.UUID = uuid
		}
		if flow, ok := n.ConfigJSON["flow"].(string); ok {
			creds.Flow = nodespec.FlowControl(flow)
		}
		if n.Flow != nil {
			creds.Flow = nodespec.FlowControl(*n.Flow)
		}
		return creds
	case "vmess":
		creds := nodespec.VMessCredentials{AlterID: 0}
		if uuid, ok := n.ConfigJSON["uuid"].(string); ok {
			creds.UUID = uuid
		}
		if aid, ok := n.ConfigJSON["alterId"].(float64); ok {
			creds.AlterID = int(aid)
		}
		return creds
	case "trojan":
		creds := nodespec.TrojanCredentials{}
		if pw, ok := n.ConfigJSON["password"].(string); ok {
			creds.Password = pw
		}
		return creds
	case "shadowsocks", "ss":
		creds := nodespec.ShadowsocksCredentials{}
		if pw, ok := n.ConfigJSON["password"].(string); ok {
			creds.Password = pw
		}
		if method, ok := n.ConfigJSON["method"].(string); ok {
			creds.Method = method
		}
		return creds
	default:
		return n.ConfigJSON
	}
}

func buildTransportConfig(n *model.Node) nodespec.TransportConfig {
	tc := nodespec.TransportConfig{
		Type: nodespec.Transport(n.TransportType),
	}
	path := "/"
	if n.Path != nil && *n.Path != "" {
		path = *n.Path
	}
	host := ""
	if n.HostHeader != nil {
		host = *n.HostHeader
	}

	switch n.TransportType {
	case "ws":
		ws := &nodespec.WSConfig{Path: path}
		if host != "" {
			ws.Host = host
		}
		tc.WS = ws
	case "grpc":
		grpc := &nodespec.GRPCConfig{}
		if n.Path != nil && *n.Path != "" {
			grpc.ServiceName = *n.Path
		} else if n.ConfigJSON != nil {
			// fallback：node.Path 为空时从 config_json.service_name 读取
			// 修复 gRPC 节点保存时 service_name 被 normalizer 删除导致 Validate 失败
			if sn, ok := n.ConfigJSON["service_name"].(string); ok && sn != "" {
				grpc.ServiceName = sn
			}
		}
		tc.GRPC = grpc
	case "httpupgrade":
		hu := &nodespec.HTTPUpgradeConfig{Path: path}
		if host != "" {
			hu.Host = host
		}
		tc.HTTPUpgrade = hu
	case "xhttp":
		xh := &nodespec.XHTTPConfig{Path: path}
		if host != "" {
			xh.Host = host
		}
		// mode: config_json top-level > config_json.xhttp.mode > security-based default
		if mode, ok := getStringFromNodeConfig(n, "mode"); ok && mode != "" {
			xh.Mode = mode
		} else if xhttpMap, ok := n.ConfigJSON["xhttp"].(map[string]interface{}); ok {
			if m, ok := xhttpMap["mode"].(string); ok && m != "" {
				xh.Mode = m
			}
		}
		if xh.Mode == "" {
			sec := getSecurityType(n)
			if sec == "reality" {
				xh.Mode = "stream-up"
			} else {
				xh.Mode = "packet-up"
			}
		}
		// path/host may be inside config_json.xhttp
		var extra map[string]interface{}
		if xhttpMap, ok := n.ConfigJSON["xhttp"].(map[string]interface{}); ok {
			if p, ok := xhttpMap["path"].(string); ok && p != "" {
				xh.Path = p
			}
			if h, ok := xhttpMap["host"].(string); ok && h != "" {
				xh.Host = h
			}
			if e, ok := xhttpMap["extra"].(map[string]interface{}); ok && len(e) > 0 {
				extra = e
			}
		}
		// Fallback: 从顶层 xhttp_extra 读取（兼容 NormalizeNodeConfigJSON 拍平的老数据）
		if extra == nil {
			if e, ok := n.ConfigJSON["xhttp_extra"].(map[string]interface{}); ok && len(e) > 0 {
				extra = e
			}
		}
		if extra != nil {
			// XMUX -> MuxConfig
			if xmux, ok := extra["xmux"].(map[string]interface{}); ok && len(xmux) > 0 {
				mc := &nodespec.MuxConfig{
					Enabled:  true,
					Protocol: nodespec.MuxProtocolXmux,
				}
				mc.MaxConcurrency = getStringOrNum(xmux, "maxConcurrency")
				mc.CMaxReuseTimes = getStringOrNum(xmux, "cMaxReuseTimes")
				mc.HMaxRequestTimes = getStringOrNum(xmux, "hMaxRequestTimes")
				mc.HMaxReusableSecs = getStringOrNum(xmux, "hMaxReusableSecs")
				if v, ok := xmux["maxConnections"].(float64); ok {
					mc.MaxConnections = int(v)
				}
				tc.Mux = mc
			}
			// downloadSettings -> XHTTPDownloadConfig (split mode 上下行分离)
			if ds, ok := extra["downloadSettings"].(map[string]interface{}); ok && len(ds) > 0 {
				xh.DownloadSettings = buildXHTTPDownloadSettings(ds)
			}
			// headers
			if headers, ok := extra["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				xh.Headers = make(map[string]string, len(headers))
				for k, v := range headers {
					if s, ok := v.(string); ok {
						xh.Headers[k] = s
					}
				}
			}
		}
		tc.XHTTP = xh
	case "kcp":
		kcp := &nodespec.KCPConfig{}
		if n.ConfigJSON != nil {
			if seed, ok := n.ConfigJSON["seed"].(string); ok {
				kcp.Seed = seed
			}
		}
		tc.KCP = kcp
	case "quic":
		quic := &nodespec.QUICConfig{}
		if n.ConfigJSON != nil {
			if sec, ok := n.ConfigJSON["quic_security"].(string); ok {
				quic.Security = sec
			}
			if key, ok := n.ConfigJSON["quic_key"].(string); ok {
				quic.Key = key
			}
		}
		tc.QUIC = quic
	}

	// 端口跳跃（Hysteria2/TUIC UDP 协议）：从 config_json.port_hopping 读取并填充 NodeSpec
	// 用于渲染 sing-box inbound 的 hop_ports 字段和客户端订阅 URI 的 mport 参数
	if n.ConfigJSON != nil {
		if ph, ok := n.ConfigJSON["port_hopping"].(map[string]interface{}); ok {
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

// getStringFromNodeConfig reads a top-level string from model.Node config_json
func getStringFromNodeConfig(n *model.Node, key string) (string, bool) {
	if n.ConfigJSON == nil {
		return "", false
	}
	if v, ok := n.ConfigJSON[key].(string); ok {
		return v, true
	}
	return "", false
}

// getStringOrNum reads a string or number field from a map
// (XMUX range values like "16-32" are strings, fixed values may be numbers)
func getStringOrNum(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		case float64:
			return strconv.FormatInt(int64(val), 10)
		case int:
			return strconv.Itoa(val)
		case int64:
			return strconv.FormatInt(val, 10)
		}
	}
	return ""
}

// buildXHTTPDownloadSettings builds XHTTPDownloadConfig from config_json
// 同时支持两种键风格：
//   - URI/IR风格: reality, tls（短键名，URI share link使用）
//   - Xray JSON风格: realitySettings, tlsSettings, xhttpSettings（从Xray配置粘贴时使用）
func buildXHTTPDownloadSettings(ds map[string]interface{}) *nodespec.XHTTPDownloadConfig {
	result := &nodespec.XHTTPDownloadConfig{}
	if addr, ok := ds["address"].(string); ok {
		result.Address = addr
	}
	if addr6, ok := ds["address_ipv6"].(string); ok {
		result.AddressIPv6 = addr6
	}
	if port, ok := ds["port"].(float64); ok {
		result.Port = int(port)
	}
	if sp, ok := ds["server_port"].(float64); ok {
		result.ServerPort = int(sp)
	}
	if net, ok := ds["network"].(string); ok {
		result.Network = nodespec.Transport(net)
	}
	if sec, ok := ds["security"].(string); ok {
		result.Security = nodespec.Security(sec)
	}
	if p, ok := ds["path"].(string); ok {
		result.Path = p
	}
	if h, ok := ds["host"].(string); ok {
		result.Host = h
	}
	if m, ok := ds["mode"].(string); ok {
		result.Mode = m
	}
	if v, ok := ds["no_grpc_header"].(bool); ok {
		result.NoGRPCHeader = v
	}
	// 从 xhttpSettings 子对象读取 path/host/mode（Xray JSON 风格）
	if xhSettings, ok := ds["xhttpSettings"].(map[string]interface{}); ok && len(xhSettings) > 0 {
		if p, ok := xhSettings["path"].(string); ok && p != "" {
			result.Path = p
		}
		if h, ok := xhSettings["host"].(string); ok && h != "" {
			result.Host = h
		}
		if m, ok := xhSettings["mode"].(string); ok && m != "" {
			result.Mode = m
		}
	}
	// REALITY sub-config: 支持 reality (IR) 和 realitySettings (Xray JSON) 两种键
	var realityMap map[string]interface{}
	if r, ok := ds["reality"].(map[string]interface{}); ok && len(r) > 0 {
		realityMap = r
	} else if r, ok := ds["realitySettings"].(map[string]interface{}); ok && len(r) > 0 {
		realityMap = r
	}
	if realityMap != nil {
		rc := &nodespec.RealityConfig{}
		if pk, ok := realityMap["publicKey"].(string); ok {
			rc.PublicKey = pk
		} else if pk, ok := realityMap["public_key"].(string); ok {
			rc.PublicKey = pk
		}
		if privk, ok := realityMap["privateKey"].(string); ok {
			rc.PrivateKey = privk
		} else if privk, ok := realityMap["private_key"].(string); ok {
			rc.PrivateKey = privk
		}
		if sid, ok := realityMap["shortId"].(string); ok {
			rc.ShortID = sid
		} else if sid, ok := realityMap["short_id"].(string); ok {
			rc.ShortID = sid
		}
		if sids, ok := realityMap["shortIds"].([]interface{}); ok {
			for _, s := range sids {
				if ss, ok := s.(string); ok {
					rc.ShortIDs = append(rc.ShortIDs, ss)
				}
			}
		} else if sids, ok := realityMap["short_ids"].([]interface{}); ok {
			for _, s := range sids {
				if ss, ok := s.(string); ok {
					rc.ShortIDs = append(rc.ShortIDs, ss)
				}
			}
		}
		// 容错：serverName 字段在历史数据中可能被误填为 dest（IP:Port 格式）
		// 如果检测到 IP:Port 格式，自动迁移到 dest 字段，SNI 用顶层 sni 兜底
		serverNameVal := ""
		if sni, ok := realityMap["serverName"].(string); ok {
			serverNameVal = sni
		} else if sni, ok := realityMap["server_name"].(string); ok {
			serverNameVal = sni
		} else if sni, ok := realityMap["sni"].(string); ok {
			serverNameVal = sni
		}
		if isIPPortFormat(serverNameVal) {
			// serverName 被误填为 dest（IP:Port），迁移到 dest
			rc.Dest = serverNameVal
			// SNI 回退到顶层 ds.sni 或 ds.server_name
			if sni, ok := ds["sni"].(string); ok && sni != "" {
				rc.SNI = sni
			} else if sni, ok := ds["server_name"].(string); ok && sni != "" {
				rc.SNI = sni
			}
		} else {
			rc.SNI = serverNameVal
		}
		if fp, ok := realityMap["fingerprint"].(string); ok {
			rc.Fingerprint = fp
		}
		if spx, ok := realityMap["spiderX"].(string); ok {
			rc.SpiderX = spx
		} else if spx, ok := realityMap["spider_x"].(string); ok {
			rc.SpiderX = spx
		}
		if alpn, ok := realityMap["alpn"].([]interface{}); ok {
			for _, a := range alpn {
				if s, ok := a.(string); ok {
					rc.ALPN = append(rc.ALPN, s)
				}
			}
		}
		// reality_dest: 下行 REALITY 回落目标（host:port）
		// 支持独立下行反代端口（如 127.0.0.1:8465）
		// 仅在尚未从 serverName 误填迁移时读取（避免覆盖容错结果）
		if rc.Dest == "" {
			if dest, ok := realityMap["dest"].(string); ok && dest != "" {
				rc.Dest = dest
			} else if dest, ok := realityMap["reality_dest"].(string); ok && dest != "" {
				rc.Dest = dest
			} else if dest, ok := ds["reality_dest"].(string); ok && dest != "" {
				// 顶层 reality_dest 也作为下行回落目标回退
				rc.Dest = dest
			}
		}
		result.Reality = rc
	}
	// TLS sub-config: 支持 tls (IR) 和 tlsSettings (Xray JSON) 两种键
	var tlsMap map[string]interface{}
	if t, ok := ds["tls"].(map[string]interface{}); ok && len(t) > 0 {
		tlsMap = t
	} else if t, ok := ds["tlsSettings"].(map[string]interface{}); ok && len(t) > 0 {
		tlsMap = t
	}
	if tlsMap != nil {
		tlsCfg := &nodespec.TLSConfig{}
		if sni, ok := tlsMap["serverName"].(string); ok {
			tlsCfg.SNI = sni
		} else if sni, ok := tlsMap["server_name"].(string); ok {
			tlsCfg.SNI = sni
		} else if sni, ok := tlsMap["sni"].(string); ok {
			tlsCfg.SNI = sni
		}
		if fp, ok := tlsMap["fingerprint"].(string); ok {
			tlsCfg.Fingerprint = fp
		}
		if alpn, ok := tlsMap["alpn"].([]interface{}); ok {
			for _, a := range alpn {
				if s, ok := a.(string); ok {
					tlsCfg.ALPN = append(tlsCfg.ALPN, s)
				}
			}
		}
		if ai, ok := tlsMap["allowInsecure"].(bool); ok {
			tlsCfg.AllowInsecure = ai
		} else if ai, ok := tlsMap["allow_insecure"].(bool); ok {
			tlsCfg.AllowInsecure = ai
		}
		result.TLS = tlsCfg
	}
	return result
}

func buildTLSConfig(n *model.Node) *nodespec.TLSConfig {
	sec := getSecurityType(n)
	if sec != "tls" {
		return nil
	}
	tls := &nodespec.TLSConfig{}
	if n.SNI != nil {
		tls.SNI = *n.SNI
	}
	if len(n.ALPN) > 0 {
		tls.ALPN = n.ALPN
	}
	// uTLS 指纹三级回退（顶层 fingerprint > tls_settings.fingerprint > utls_fingerprint）
	// 借鉴 xboard UTLS_CONFIGURATION 的 fingerprint 字段
	if fp := pickString(n.ConfigJSON, "fingerprint", "tls_settings", "tls"); fp != "" {
		tls.Fingerprint = fp
	} else if fp, ok := n.ConfigJSON["utls_fingerprint"].(string); ok && fp != "" {
		tls.Fingerprint = fp
	}
	// allow_insecure 三级回退（顶层 > tls_settings > tls）
	// 前端可能以 bool(true) 或 number(1) 写入，pickBool 兼容两种类型
	tls.AllowInsecure = pickBool(n.ConfigJSON, "allow_insecure", "tls_settings", "tls")
	// PinSHA256 证书指纹：直连 IP 节点用 pinnedPeerCertSha256 证书锁定，
	// 规避 Xray v26.2.4+ 移除 allowInsecure 导致的 alert 112 unrecognized_name。
	// 兼容 pin_sha256 / pinned_peer_cert_sha256 两种 key 名，各做三级回退。
	tls.PinSHA256 = pickString(n.ConfigJSON, "pin_sha256", "tls_settings", "tls")
	if tls.PinSHA256 == "" {
		tls.PinSHA256 = pickString(n.ConfigJSON, "pinned_peer_cert_sha256", "tls_settings", "tls")
	}
	// 证书 PEM 三级回退（顶层 > tls_settings > tls）
	// P0-2 PEM-only：自签名证书节点必须在 config_json 携带 cert_pem/key_pem，
	// 否则 xray 26.3.27 TLS inbounds 缺少 certificates 会返回 alert 112 (unrecognized_name)
	tls.CertPEM = pickString(n.ConfigJSON, "cert_pem", "tls_settings", "tls")
	tls.KeyPEM = pickString(n.ConfigJSON, "key_pem", "tls_settings", "tls")
	return tls
}

// injectCertFromBundle B11: 从 cert_bundles 表查询证书 PEM 并注入到 node.ConfigJSON。
// buildTLSConfig 是纯函数无 DB 访问，因此在本函数中预先把 PEM 写入 ConfigJSON。
// 当 config_json 已有 cert_pem/key_pem 时跳过；仅对 TLS 安全类型生效。
// 查询优先级：cert_bundle_id（精确）→ SNI 域名匹配 SAN（模糊回退）→ 自签名证书兜底。
//
// P2-3: 按 TerminationClass 分流 + 日志分级。
//   - cf_edge (argo_tunnel): 不注入证书（TLS 在 CF 边缘终止，xray sec=none 不需要 PEM）
//   - nginx (cdn/cdn_saas): 不注入证书（TLS 在 nginx 8445 终止，证书由 nginx ACME 持有）
//   - self_tcp/self_udp: 需要注入证书（xray 自终止 TLS）
//   - reality: 不注入证书（REALITY 用 public/private key，不走 X.509 证书）
func (s *DeploymentService) injectCertFromBundle(ctx context.Context, n *model.Node) {
	if n == nil {
		return
	}

	// P2-3: 按 TerminationClass 分流，避免对不需要证书的节点做无意义的 DB 查询
	tc := ClassifyTermination(n)
	if !tc.NeedsCertBundle() {
		// cf_edge/nginx/reality 节点不需要注入 cert_pem/key_pem
		// 仅在 Debug 级别记录，避免日志噪音
		if s.logger != nil {
			s.logger.Debug("injectCertFromBundle: skip node (termination class does not need cert bundle)",
				"node_code", n.Code, "termination_class", tc.String())
		}
		return
	}

	// 安全类型校验（兼容旧逻辑：非 tls 也跳过）
	if getSecurityType(n) != "tls" {
		return
	}
	// 已有 PEM 数据则跳过
	if pickString(n.ConfigJSON, "cert_pem", "tls_settings", "tls") != "" &&
		pickString(n.ConfigJSON, "key_pem", "tls_settings", "tls") != "" {
		return
	}
	// 确定 SNI 域名（用于 SAN 匹配和自签兜底）
	sni := ""
	if n.SNI != nil {
		sni = strings.TrimSpace(*n.SNI)
	}
	if sni == "" {
		if sn, ok := n.ConfigJSON["sni"].(string); ok {
			sni = strings.TrimSpace(sn)
		}
	}
	if sni == "" {
		if ts, ok := n.ConfigJSON["tls_settings"].(map[string]interface{}); ok {
			if sn, ok := ts["server_name"].(string); ok && sn != "" {
				sni = strings.TrimSpace(sn)
			} else if sn, ok := ts["sni"].(string); ok && sn != "" {
				sni = strings.TrimSpace(sn)
			}
		}
	}
	// 回退 P4：CDN 节点不再走 injectCertFromBundle（NeedsCertBundle 对 TerminationNginx 返回 false）。
	// 此 SNI 回退逻辑保留供 self_tcp/self_udp 节点使用，CDN 节点路径已成死代码（无害保留）。
	// 历史：P4 阶段 CDN 节点 SNI 回退到 cdn_address（nginx_plus_xray 的证书域名是 cdn_address）。
	if sni == "" {
		if cdnAddr, ok := n.ConfigJSON["cdn_address"].(string); ok && cdnAddr != "" {
			sni = strings.TrimSpace(cdnAddr)
		}
	}

	// P2-3: SNI 为空时 Warn 级别告警（self_tcp 节点无 SNI 无法签发证书）
	if sni == "" {
		if s.logger != nil {
			s.logger.Warn("injectCertFromBundle: SNI is empty, cannot inject cert (self_tcp/self_udp node requires SNI for cert generation)",
				"node_code", n.Code, "termination_class", tc.String())
		}
		return
	}

	// 优先通过 cert_bundle_id 精确查询
	if s.capRepo != nil {
		if cbIDStr, ok := getStringFromNodeConfig(n, "cert_bundle_id"); ok && cbIDStr != "" {
			if cbID, err := uuid.Parse(cbIDStr); err == nil {
				cb, err := s.capRepo.GetCertBundle(ctx, cbID)
				if err == nil && cb != nil && cb.CertPEM != "" && cb.KeyPEM != "" {
				if n.ConfigJSON == nil {
					n.ConfigJSON = make(map[string]interface{})
				}
				n.ConfigJSON["cert_pem"] = cb.CertPEM
				n.ConfigJSON["key_pem"] = cb.KeyPEM
				// P1-D: 证书来源标记（面板可观测性，stripMetaFields 会剥离此字段不进入内核配置）
				n.ConfigJSON["_cert_source"] = "cert_bundle_id"
				// P2-3: 成功注入精确匹配证书，Info 级别日志
				if s.logger != nil {
					s.logger.Info("injectCertFromBundle: injected cert via cert_bundle_id",
						"node_code", n.Code, "sni", sni,
						"cert_bundle_id", cbIDStr, "termination_class", tc.String(),
						"cert_source", "cert_bundle_id")
				}
				return
			}
			}
		}
		// 回退：按 SNI 域名匹配 cert_bundles.SAN
		bundles, err := s.capRepo.ListCertBundles(ctx, "")
		if err == nil && len(bundles) > 0 {
			for _, cb := range bundles {
				if cb.CertPEM == "" || cb.KeyPEM == "" {
					continue
				}
				for _, san := range cb.SAN {
					if strings.EqualFold(san, sni) {
					if n.ConfigJSON == nil {
						n.ConfigJSON = make(map[string]interface{})
					}
					n.ConfigJSON["cert_pem"] = cb.CertPEM
					n.ConfigJSON["key_pem"] = cb.KeyPEM
					// P1-D: 证书来源标记（SAN 模糊回退）
					n.ConfigJSON["_cert_source"] = "cert_bundle_san"
					// P2-3: SAN 匹配回退注入，Info 级别日志
					if s.logger != nil {
						s.logger.Info("injectCertFromBundle: injected cert via SAN fallback match",
							"node_code", n.Code, "sni", sni,
							"matched_san", san, "termination_class", tc.String(),
							"cert_source", "cert_bundle_san")
					}
					return
				}
				}
			}
		}
	}

	// P1-C: 证书四级回退第3级 — tls_certificates 表 SNI 匹配
	// 如果管理员上传了正式证书到 tls_certificates 表但尚未同步到 cert_bundles，
	// 此层兜底确保证书仍能被注入。
	if s.tlsCertReader != nil {
		if certPEM, keyPEM, ok := s.tlsCertReader.FindCertPEMBySNI(ctx, sni); ok && certPEM != "" && keyPEM != "" {
			if n.ConfigJSON == nil {
				n.ConfigJSON = make(map[string]interface{})
			}
			n.ConfigJSON["cert_pem"] = certPEM
			n.ConfigJSON["key_pem"] = keyPEM
			// P1-D: 证书来源标记（tls_certificates 表 SNI 匹配）
			n.ConfigJSON["_cert_source"] = "tls_certificates"
			if s.logger != nil {
				s.logger.Info("injectCertFromBundle: injected cert via tls_certificates SNI match",
					"node_code", n.Code, "sni", sni,
					"termination_class", tc.String(),
					"cert_source", "tls_certificates")
			}
			return
		}
	}

	// 自签名证书兜底：cert_bundles 中找不到匹配证书时，
	// 自动生成 ECDSA P-256 自签名证书，确保节点首次创建即可工作。
	// 正式 ACME 证书签发后可通过 cert_bundle_id 更新覆盖。
	// P2-3: 自签兜底保持 Warn 级别（提醒运维及时替换为正式证书）
	certPEM, keyPEM, err := crypto.GenerateSelfSignedCertPEM(sni)
	if err == nil {
		if n.ConfigJSON == nil {
			n.ConfigJSON = make(map[string]interface{})
		}
		n.ConfigJSON["cert_pem"] = certPEM
		n.ConfigJSON["key_pem"] = keyPEM
		// P1-D: 证书来源标记（自签兜底）
		n.ConfigJSON["_cert_source"] = "self_signed"
		// P1-D: 自签兜底增加 SHA256 指纹输出和高敏感告警
		fingerprint := crypto.CertSHA256Fingerprint(certPEM)
		if s.logger != nil {
			s.logger.Warn("self-signed cert generated (Trojan+TLS/AnyTLS/ShadowTLS high-sensitivity node)",
				"node_code", n.Code, "sni", sni,
				"termination_class", tc.String(),
				"protocol", n.ProtocolType,
				"cert_fingerprint_sha256", fingerprint,
				"cert_source", "self_signed",
				"hint", "客户端需配置 insecure=true 或 pinnedPeerCertSha256",
				"action", "请通过 cert_bundle_id 绑定正式 ACME 证书")
		}
	} else if s.logger != nil {
		s.logger.Error("self-signed cert generation failed",
			"node_code", n.Code, "sni", sni,
			"termination_class", tc.String(), "error", err)
	}
}

// buildTLSMaterials P1-12: 查询该服务器下所有 runtime 的 TLS 节点证书 PEM，构建 TLSMaterials map。
// key 为域名/SNI，value 为 PEM 证书+私钥。
// 复用 injectCertFromBundle 解析证书（cert_bundle_id 精确查询 → SNI 匹配 SAN 回退 → 自签兜底），
// 并兼容节点 ConfigJSON 中已内联的 cert_pem/key_pem。
// 仅处理 security=tls 的已启用节点；reality/none 节点跳过。
// 聚合同一服务器上所有 runtime 的节点（双内核模式下 xray+sing-box 共享 nginx/cert）。
// 失败只记录日志并返回 nil，不阻断 Payload 构建。
func (s *DeploymentService) buildTLSMaterials(ctx context.Context, runtimeID uuid.UUID) map[string]*crypto.TLSMaterial {
	if runtimeID == uuid.Nil || s.nodeRepo == nil {
		return nil
	}
	rt, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil || rt == nil {
		s.logger.Warn("buildTLSMaterials: get runtime failed",
			"runtime_id", runtimeID, "error", err)
		return nil
	}
	nodes, err := s.nodeRepo.ListByServerID(ctx, rt.ServerID)
	if err != nil {
		s.logger.Warn("buildTLSMaterials: list nodes by server failed",
			"server_id", rt.ServerID, "error", err)
		return nil
	}

	materials := make(map[string]*crypto.TLSMaterial)
	for _, n := range nodes {
		if n == nil || !n.IsEnabled {
			continue
		}
		if getSecurityType(n) != "tls" {
			continue
		}
		// 复用 injectCertFromBundle 将证书 PEM 解析到 ConfigJSON
		// （已有 cert_pem/key_pem 时跳过，否则按 cert_bundle_id/SNI 查询）
		s.injectCertFromBundle(ctx, n)
		certPEM := pickString(n.ConfigJSON, "cert_pem", "tls_settings", "tls")
		keyPEM := pickString(n.ConfigJSON, "key_pem", "tls_settings", "tls")
		if certPEM == "" || keyPEM == "" {
			continue
		}
		// 确定 key：SNI 优先，回退到 cdn_address（CDN 节点）
		domain := ""
		if n.SNI != nil {
			domain = *n.SNI
		}
		if domain == "" {
			if cdnAddr, ok := n.ConfigJSON["cdn_address"].(string); ok {
				domain = cdnAddr
			}
		}
		if domain == "" {
			continue
		}
		// 同一域名多个节点时保留首个（证书应一致）
		if _, exists := materials[domain]; !exists {
			materials[domain] = &crypto.TLSMaterial{
				CertPEM: certPEM,
				KeyPEM:  keyPEM,
			}
		}
	}

	if len(materials) == 0 {
		return nil
	}
	s.logger.Info("TLS materials built for payload",
		"runtime_id", runtimeID, "domain_count", len(materials))
	return materials
}

func buildRealityConfig(n *model.Node) *nodespec.RealityConfig {
	sec := getSecurityType(n)
	if sec != "reality" {
		return nil
	}
	r := &nodespec.RealityConfig{}
	if n.SNI != nil {
		r.SNI = *n.SNI
	}
	if n.ConfigJSON != nil {
		// 三级回退读取（借鉴 xboard buildNodeConfig 字段路径标准）
		// 顶层 > reality.* > reality_settings.*
		r.PublicKey = pickString(n.ConfigJSON, "public_key", "reality", "reality_settings")
		r.ShortID = pickString(n.ConfigJSON, "short_id", "reality", "reality_settings")
		r.PrivateKey = pickString(n.ConfigJSON, "private_key", "reality", "reality_settings")
		// fingerprint 多级回退：reality.fingerprint > reality_settings.fingerprint > 顶层 fingerprint/utls_fingerprint
		if fp := pickString(n.ConfigJSON, "fingerprint", "reality", "reality_settings"); fp != "" {
			r.Fingerprint = fp
		} else if fp, ok := n.ConfigJSON["utls_fingerprint"].(string); ok && fp != "" {
			r.Fingerprint = fp
		} else if fp, ok := n.ConfigJSON["reality_utls_fingerprint"].(string); ok && fp != "" {
			r.Fingerprint = fp
		}
		// dest 借鉴 xboard：reality_settings.server_name + server_port
		if rs, ok := n.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
			if sn, ok := rs["server_name"].(string); ok && sn != "" {
				r.SNI = sn // reality_settings.server_name 优先级高于 n.SNI
			}
		}
		// reality_dest: REALITY 回落目标（host:port），优先级：顶层 > reality.* > reality_settings.*
		// 推荐使用本地反代地址（127.0.0.1:8460 → nginx vhost 反代真实站点）
		// 为空时由渲染器回退到 SNI:443
		r.Dest = pickString(n.ConfigJSON, "reality_dest", "reality", "reality_settings")
	}
	return r
}

// pickString 从 config_json 中按"顶层 → 多个嵌套对象"的顺序读取字符串字段。
// 借鉴 xboard ServerService::buildNodeConfig 的字段路径标准。
// 与 exposure.pickStringNested 逻辑保持一致，避免跨包依赖。
func pickString(cfg map[string]interface{}, topLevelKey string, nestedKeys ...string) string {
	if cfg == nil {
		return ""
	}
	if v, ok := cfg[topLevelKey].(string); ok && v != "" {
		return v
	}
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if v, ok := m[topLevelKey].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// pickBool 从 config_json 中按"顶层 → 多个嵌套对象"的顺序读取布尔字段。
// 兼容 JSON 解析后的多种类型：bool、float64(0/1)、string("true"/"1"/"yes")。
// 用于 allow_insecure 等 xboard 历史可能以 0/1 数字或字符串存储的字段。
func pickBool(cfg map[string]interface{}, topLevelKey string, nestedKeys ...string) bool {
	if cfg == nil {
		return false
	}
	if asBoolFromAny(cfg[topLevelKey]) {
		return true
	}
	for _, nk := range nestedKeys {
		if m, ok := cfg[nk].(map[string]interface{}); ok {
			if asBoolFromAny(m[topLevelKey]) {
				return true
			}
		}
	}
	return false
}

// isIPPortFormat 判断字符串是否为 IP:Port 格式（如 "192.168.1.1:443" 或 "10.0.0.1:8443"）。
// 用于容错：历史数据中 REALITY 的 serverName 字段可能被误填为 dest（IP:Port），
// 此时不应作为 SNI 使用，而应迁移到 dest 字段。
func isIPPortFormat(s string) bool {
	if s == "" {
		return false
	}
	// 必须包含冒号
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return false
	}
	host := s[:idx]
	port := s[idx+1:]
	// port 必须是数字
	if _, err := strconv.Atoi(port); err != nil {
		return false
	}
	// host 是 IPv4 地址
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// asBoolFromAny 将任意值转为 bool，兼容 bool/float64/string。
func asBoolFromAny(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}

func modelNodeToNodeSpec(n *model.Node) *nodespec.NodeSpec {
	code := n.Code
	if code == "" {
		code = "landing"
	}
	spec := &nodespec.NodeSpec{
		ID:          n.ID.String(),
		Code:        code,
		Name:        n.Name,
		Protocol:    nodespec.Protocol(n.ProtocolType),
		Address:     n.Address,
		Port:        n.Port,
		Credentials: extractCredentials(n),
		Transport:   buildTransportConfig(n),
		Security:    nodespec.Security(getSecurityType(n)),
		TLS:         buildTLSConfig(n),
		Reality:     buildRealityConfig(n),
		AllowUDP:    n.AllowUDP,
		// P3-M: 按时段动态计算 TrafficRate（如夜间半价）
		TrafficRate: computeDynamicTrafficRate(n.TrafficRate, n.RateTimeEnable, n.RateTimeRanges),
		// R3: 补全缺失字段映射
		Tags:      n.Tags,
		Priority:  n.Priority,
		IsVisible: n.IsVisible,
		NodeType:  string(n.NodeType),
	}
	// 回退 P4：CDN 节点（TerminationNginx）不再在 NodeSpec 构建阶段强制注入证书。
	// TLS 剥离由 kernel_render_adapter.go 的 stripTLSFromXrayInbound 在渲染阶段处理：
	//   spec.Security 由 DB SecurityType=tls 自然得出 → 渲染时剥离为 none → xray inbound security=none
	// 证书由 nginx 8445 持有（CertPath/KeyPath 在 buildNginxVhosts 中注入），xray 不持证书。
	// R3: AddressIPv6 — 从 ConfigJSON 或 Metadata 读取
	if addr6, ok := getStringFromNodeConfig(n, "address_ipv6"); ok && addr6 != "" {
		spec.AddressIPv6 = addr6
	} else if n.Metadata != nil {
		if addr6, ok := n.Metadata["address_ipv6"].(string); ok {
			spec.AddressIPv6 = addr6
		}
	}
	// R3: ClientPort / ServerPort — 从 ConfigJSON 读取
	// 端口语义（renderer.resolveInboundPort / resolveListenAddress 已实现通用规则）：
	//   - ServerPort > 0 && ServerPort != Port → xray 绑 127.0.0.1:ServerPort（CDN/Tunnel 节点）
	//   - ServerPort == 0 或 ServerPort == Port → xray 绑 0.0.0.0:Port（直连/UDP 公网）
	// P5 改造后：直连节点（trojan+tls/reality/anytls/ss/mieru）ServerPort 应为 0，
	// 触发 0.0.0.0:Port 绑定，绕过 nginx stream。数据层的错误 server_port 通过 P5.3 SQL 修复。
	if cp, ok := getStringFromNodeConfig(n, "client_port"); ok && cp != "" {
		if port, err := strconv.Atoi(cp); err == nil && port > 0 {
			spec.ClientPort = port
		}
	} else if n.ConfigJSON != nil {
		if cp, ok := n.ConfigJSON["client_port"].(float64); ok && cp > 0 {
			spec.ClientPort = int(cp)
		}
	}
	if sp, ok := getStringFromNodeConfig(n, "server_port"); ok && sp != "" {
		if port, err := strconv.Atoi(sp); err == nil && port > 0 {
			spec.ServerPort = port
		}
	} else if n.ConfigJSON != nil {
		if sp, ok := n.ConfigJSON["server_port"].(float64); ok && sp > 0 {
			spec.ServerPort = int(sp)
		}
	}
	// ClientPort 默认等于 Port（如果未显式设置）
	if spec.ClientPort == 0 {
		spec.ClientPort = n.Port
	}
	// R3: Group — 从 ConfigJSON 读取分组名称
	if group, ok := getStringFromNodeConfig(n, "group"); ok && group != "" {
		spec.Group = group
	}
	// R3: Region — 从 ConfigJSON 或 Metadata 读取
	if region, ok := getStringFromNodeConfig(n, "region"); ok && region != "" {
		spec.Region = region
	} else if n.Metadata != nil {
		if region, ok := n.Metadata["region"].(string); ok {
			spec.Region = region
		}
	}
	// P0: 映射限速和设备限制字段
	if n.SpeedLimitMbps != nil && *n.SpeedLimitMbps > 0 {
		spec.SpeedLimitMbps = *n.SpeedLimitMbps
	}
	if n.DeviceLimit != nil && *n.DeviceLimit > 0 {
		spec.DeviceLimit = *n.DeviceLimit
	}
	// P3-N: 映射节点级流量限额（0 或 nil 表示不限额）
	if n.TransferEnableBytes != nil && *n.TransferEnableBytes > 0 {
		spec.TransferEnableBytes = *n.TransferEnableBytes
	}
	// P3-L: 映射 AnyTLS padding_scheme（nil 或空字符串表示使用内核默认值）
	if n.PaddingScheme != nil && *n.PaddingScheme != "" {
		spec.PaddingScheme = *n.PaddingScheme
	}
	return spec
}

// rateTimeRange 描述一个时段倍率区间。
// Start/End 格式为 "HH:MM"，Multiplier 为倍率（如 0.5 表示半价）。
type rateTimeRange struct {
	Start      string  `json:"start"`      // "HH:MM"
	End        string  `json:"end"`        // "HH:MM"
	Multiplier float64 `json:"multiplier"` // e.g., 0.5 for half price
}

// computeDynamicTrafficRate 根据当前时间和节点配置的时段倍率动态计算 TrafficRate。
//
// 若 rateTimeEnable 为 nil 或 false，或 rateTimeRanges 为空 / 解析失败，则直接返回 baseRate。
// 否则遍历所有时段区间，若当前时间（时:分）落在某个区间 [start, end) 内，
// 则返回 baseRate * multiplier。
//
// 示例：baseRate=1.0，夜间 02:00-06:00 倍率 0.5 → 当前 03:30 返回 0.5。
func computeDynamicTrafficRate(baseRate float64, rateTimeEnable *bool, rateTimeRanges json.RawMessage) float64 {
	if rateTimeEnable == nil || !*rateTimeEnable || len(rateTimeRanges) == 0 {
		return baseRate
	}
	var ranges []rateTimeRange
	if err := json.Unmarshal(rateTimeRanges, &ranges); err != nil {
		return baseRate
	}
	now := time.Now()
	currentMin := now.Hour()*60 + now.Minute()
	for _, r := range ranges {
		startMin := parseTimeToMinutes(r.Start)
		endMin := parseTimeToMinutes(r.End)
		// 处理跨午夜区间（如 23:00-01:00）：当 endMin < startMin 时，
		// 当前时间 >= startMin 或 < endMin 即匹配
		if endMin <= startMin {
			if currentMin >= startMin || currentMin < endMin {
				return baseRate * r.Multiplier
			}
		} else {
			if currentMin >= startMin && currentMin < endMin {
				return baseRate * r.Multiplier
			}
		}
	}
	return baseRate
}

// parseTimeToMinutes 将 "HH:MM" 格式的时间字符串转换为当天从 00:00 起的分钟数。
// 解析失败时返回 0。
func parseTimeToMinutes(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || h < 0 || h > 23 {
		return 0
	}
	m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || m < 0 || m > 59 {
		return 0
	}
	return h*60 + m
}

func (s *DeploymentService) buildDefaultRuntimeConfig(nodes []*model.Node) map[string]interface{} {
	inbounds := make([]map[string]interface{}, 0)
	outbounds := make([]map[string]interface{}, 0)

	for _, n := range nodes {
		if !n.IsEnabled {
			continue
		}
		inbound := map[string]interface{}{
			"port":     n.Port,
			"protocol": n.ProtocolType,
			"settings": n.ConfigJSON,
		}
		if n.ProtocolType == "vless" || n.ProtocolType == "vmess" || n.ProtocolType == "trojan" {
			streamSettings := map[string]interface{}{
				"network": n.TransportType,
			}
			if n.SecurityType != nil && *n.SecurityType != "none" {
				streamSettings["security"] = *n.SecurityType
			}
			inbound["streamSettings"] = streamSettings
		}
		inbounds = append(inbounds, inbound)
	}

	if len(inbounds) == 0 {
		inbounds = append(inbounds, map[string]interface{}{
			"port":     10086,
			"protocol": "dokodemo-door",
			"tag":      "api",
		})
	}

	outbounds = append(outbounds, map[string]interface{}{
		"protocol": "freedom",
		"tag":      "direct",
	})
	outbounds = append(outbounds, map[string]interface{}{
		"protocol": "blackhole",
		"tag":      "block",
	})

	routing := map[string]interface{}{
		"rules": []interface{}{},
	}

	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   routing,
	}
}

func (s *DeploymentService) ReportConfigResult(ctx context.Context, runtimeID uuid.UUID, versionStr string, success bool, message string) error {
	var versionNo int64
	if versionStr != "" {
		fmt.Sscanf(versionStr, "%d", &versionNo)
	}

	if versionNo <= 0 {
		return nil
	}

	cv, err := s.deploymentRepo.GetConfigVersionByVersionNo(ctx, model.ScopeTypeRuntime, runtimeID, versionNo)
	if err != nil {
		return err
	}
	if cv == nil {
		return nil
	}
	if success {
		_ = s.deploymentRepo.UpdateConfigVersionApplied(ctx, cv.ID)
	}

	// 收集同 server 全部 node-agent runtime 的节点：ack 来自主配置应用，
	// xray 节点配置通过 _xray_config 内嵌在同一次下发中，节点级状态必须一并闭环。
	allNodes, listErr := s.nodesForRuntimeAndServer(ctx, runtimeID)
	if listErr != nil {
		s.logger.Warn("ReportConfigResult: list nodes failed", "runtime_id", runtimeID, "error", listErr)
		return nil
	}

	status := "applied"
	errMsg := ""
	if !success {
		status = "failed"
		errMsg = message
	}

	// B-精确：只标记 lpv==versionNo 的节点为 applied/failed。
	// 全部不匹配（延迟 ack/版本跳变/ack 挂错 runtime）时只记日志，不兜底标记全部，
	// 否则会制造 lpv != _dispatch_version 的矛盾态。
	var targetNodes []*model.Node
	for _, n := range allNodes {
		if n != nil && n.LastPublishedVersion == versionNo {
			targetNodes = append(targetNodes, n)
		}
	}
	if len(targetNodes) == 0 {
		s.logger.Warn("ReportConfigResult: no nodes match ack version, skip node status update",
			"runtime_id", runtimeID, "version", versionNo, "node_count", len(allNodes), "success", success)
	} else {
		s.markNodesDispatchStatus(ctx, targetNodes, status, versionNo, errMsg)
	}

	// 部署批次推进：agent 回执走的是 runtime 维度，
	// 而 deployment_targets 以节点维度记录。若不在此反查并推进对应 target，
	// 批次会因 target 永远停留在 pending/applying 而卡在 running（前端一直显示"部署中"）。
	// 遍历同 server 全部节点，避免 ack 解析到错误 runtime 时推进错 target。
	targetStatus := model.TargetStatusSuccess
	if !success {
		targetStatus = model.TargetStatusFailed
	}
	for _, n := range allNodes {
		if n == nil {
			continue
		}
		activeTargets, tErr := s.deploymentRepo.ListActiveTargetsByNodeAndVersion(ctx, n.ID, cv.ID)
		if tErr != nil {
			s.logger.Warn("ReportConfigResult: list active targets failed",
				"node_id", n.ID, "version_id", cv.ID, "error", tErr)
			continue
		}
		for _, t := range activeTargets {
			updReq := &model.UpdateDeploymentResultRequest{
				TargetID: t.ID,
				Status:   targetStatus,
				ApplyResult: map[string]interface{}{
					"reported_via": "config_result",
					"version_no":   versionNo,
					"message":      message,
				},
			}
			if !success {
				m := message
				updReq.ErrorMessage = &m
			}
			if uErr := s.UpdateDeploymentResult(ctx, t.ID, updReq); uErr != nil {
				s.logger.Warn("ReportConfigResult: advance deployment target failed",
					"target_id", t.ID, "batch_id", t.DeploymentBatchID, "error", uErr)
			} else {
				s.logger.Info("ReportConfigResult: deployment target advanced",
					"target_id", t.ID, "batch_id", t.DeploymentBatchID, "status", targetStatus)
			}
		}
	}
	return nil
}

// nodesForRuntimeAndServer 返回指定 runtime 及其同 server 下所有 node-agent runtime 的启用节点。
// 双内核内嵌模式下，xray 节点挂在 xray runtime，但配置经 _xray_config 随 sing-box 主配置下发，
// ack 回写必须覆盖两台 runtime 的节点，否则 xray 节点状态无法闭环。
func (s *DeploymentService) nodesForRuntimeAndServer(ctx context.Context, runtimeID uuid.UUID) ([]*model.Node, error) {
	rt, err := s.runtimeRepo.GetByID(ctx, runtimeID)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, fmt.Errorf("runtime not found: %s", runtimeID)
	}
	runtimes, err := s.runtimeRepo.ListByServer(ctx, rt.ServerID)
	if err != nil {
		return nil, err
	}
	var out []*model.Node
	for _, r := range runtimes {
		if r == nil || r.ProviderType != model.RuntimeProviderNodeAgent {
			continue
		}
		ns, err := s.nodeRepo.ListByRuntimeID(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ns...)
	}
	return out, nil
}

// ReconcileDispatchStatusOnHeartbeat 心跳自愈：当 agent 上报版本 == 面板最新版本 且 runtime 运行中时，
// 清除该 runtime 下节点的陈旧 "failed" 下发状态。
//
// 场景：agent 修复 bug 后重启，sing-box 用 LKG 配置正常启动，版本与面板一致。
// 此时心跳返回 NONE（不触发 reload），agent 不会重新部署，不上报新的 ConfigResult，
// 导致 nodes.metadata._dispatch_status 永远停留在上次失败的 "failed"。
// 此方法在心跳路径自动将匹配版本的 "failed" 刷为 "applied"，使面板状态与实际一致。
//
// 仅当 agentVersion > 0 时执行；无陈旧状态时返回 affected=0，调用方可据此决定是否记日志。
// 设计为幂等：重复调用无副作用，已 "applied" 的节点不会被触碰。
func (s *DeploymentService) ReconcileDispatchStatusOnHeartbeat(ctx context.Context, runtimeID uuid.UUID, agentVersion int64) (affected int64, err error) {
	if agentVersion <= 0 {
		return 0, nil
	}
	affected, err = s.nodeRepo.ClearStaleFailedDispatchStatus(ctx, runtimeID, agentVersion)
	if err != nil {
		s.logger.Warn("ReconcileDispatchStatusOnHeartbeat: clear stale failed status failed",
			"runtime_id", runtimeID, "version", agentVersion, "error", err)
		return 0, err
	}
	if affected > 0 {
		s.logger.Info("ReconcileDispatchStatusOnHeartbeat: cleared stale failed dispatch status",
			"runtime_id", runtimeID, "version", agentVersion, "affected_nodes", affected)
	}
	return affected, nil
}

// PrecheckDeployment 部署预检：在 Publish/Deploy 前自动运行，提前拦截错误配置。
// 检查项：端口冲突/SNI冲突/配置完整性等，避免下发到 agent 后才发现问题。
func (s *DeploymentService) PrecheckDeployment(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID) (*model.PrecheckResult, error) {
	result := &model.PrecheckResult{
		Passed:   true,
		Errors:   []model.PrecheckItem{},
		Warnings: []model.PrecheckItem{},
		Infos:    []model.PrecheckItem{},
	}

	var nodes []*model.Node
	var serverID uuid.UUID
	var serverCode string

	switch scopeType {
	case model.ScopeTypeNode:
		node, err := s.nodeRepo.GetByID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		if node == nil {
			return nil, ErrNodeNotFound
		}
		if node.DeletedAt != nil || !node.IsEnabled {
			result.Errors = append(result.Errors, model.PrecheckItem{
				Level:   "error",
				Code:    "NODE_DISABLED_OR_DELETED",
				Message: "节点已禁用或已删除",
				Scope:   "node",
				ScopeID: node.ID.String(),
			})
			result.Passed = false
			return result, nil
		}
		nodes = append(nodes, node)
		rt, err := s.runtimeRepo.GetByID(ctx, node.RuntimeID)
		if err != nil {
			return nil, err
		}
		if rt == nil {
			result.Errors = append(result.Errors, model.PrecheckItem{
				Level:   "error",
				Code:    "RUNTIME_NOT_FOUND",
				Message: "节点关联的运行时不存在",
				Scope:   "node",
				ScopeID: node.ID.String(),
			})
			result.Passed = false
			return result, nil
		}
		serverID = rt.ServerID
	case model.ScopeTypeRuntime:
		rt, err := s.runtimeRepo.GetByID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		if rt == nil {
			result.Errors = append(result.Errors, model.PrecheckItem{
				Level:   "error",
				Code:    "RUNTIME_NOT_FOUND",
				Message: "运行时不存在",
				Scope:   "runtime",
				ScopeID: scopeID.String(),
			})
			result.Passed = false
			return result, nil
		}
		serverID = rt.ServerID
		nodes, err = s.nodeRepo.ListByRuntimeID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidScopeType
	}

	srv, err := s.serverRepo.GetByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv != nil {
		serverCode = srv.Code
		result.ServerCode = serverCode
	}

	serverPortMap := make(map[int][]*model.Node)
	sniMap := make(map[string][]*model.Node)

	for _, node := range nodes {
		if node == nil {
			continue
		}

		isTCP := strings.EqualFold(node.TransportType, "tcp")
		isReality := false
		if node.SecurityType != nil && strings.EqualFold(*node.SecurityType, "reality") {
			isReality = true
		}
		if sec, ok := node.ConfigJSON["security"].(string); ok && strings.EqualFold(sec, "reality") {
			isReality = true
		}

		cdnAddr, _ := node.ConfigJSON["cdn_address"].(string)
		isCDN := cdnAddr != ""

		if isTCP {
			if node.Port != 443 {
				result.Errors = append(result.Errors, model.PrecheckItem{
					Level:   "error",
					Code:    "TCP_PORT_NOT_443",
					Message: fmt.Sprintf("TCP节点端口必须为443，当前为%d", node.Port),
					Scope:   "node",
					ScopeID: node.ID.String(),
				})
				result.Passed = false
			}
			if node.ServerPort == nil || *node.ServerPort == 0 {
				result.Errors = append(result.Errors, model.PrecheckItem{
					Level:   "error",
					Code:    "TCP_SERVER_PORT_MISSING",
					Message: "TCP节点必须配置ServerPort（内部监听端口）",
					Scope:   "node",
					ScopeID: node.ID.String(),
				})
				result.Passed = false
			} else {
				serverPortMap[*node.ServerPort] = append(serverPortMap[*node.ServerPort], node)
			}
		}

		if isReality {
			realitySNI := ""
			if node.RealityServerName != nil && *node.RealityServerName != "" {
				realitySNI = *node.RealityServerName
			}
			if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
				if sn, ok := rs["server_name"].(string); ok && sn != "" {
					realitySNI = sn
				}
			}
			if realitySNI == "" {
				if sn, ok := node.ConfigJSON["server_name"].(string); ok && sn != "" {
					realitySNI = sn
				}
			}
			if realitySNI == "" {
				result.Errors = append(result.Errors, model.PrecheckItem{
					Level:   "error",
					Code:    "REALITY_SNI_MISSING",
					Message: "REALITY节点必须配置RealityServerName（SNI）",
					Scope:   "node",
					ScopeID: node.ID.String(),
				})
				result.Passed = false
			} else {
				sniMap[realitySNI] = append(sniMap[realitySNI], node)
			}
		}

		if isCDN {
			result.Warnings = append(result.Warnings, model.PrecheckItem{
				Level:   "warning",
				Code:    "CDN_ORIGIN_CONFIG",
				Message: fmt.Sprintf("该域名(%s)需在CDN服务商处配置回源", cdnAddr),
				Scope:   "node",
				ScopeID: node.ID.String(),
			})

			realitySNI := ""
			if node.RealityServerName != nil && *node.RealityServerName != "" {
				realitySNI = *node.RealityServerName
			}
			if rs, ok := node.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
				if sn, ok := rs["server_name"].(string); ok && sn != "" {
					realitySNI = sn
				}
			}
			if realitySNI != "" && strings.EqualFold(realitySNI, cdnAddr) {
				result.Errors = append(result.Errors, model.PrecheckItem{
					Level:   "error",
					Code:    "CDN_SNI_AMBIGUITY",
					Message: fmt.Sprintf("CDN节点的RealityServerName(%s)不能与cdn_address相等，会导致SNI路由歧义", realitySNI),
					Scope:   "node",
					ScopeID: node.ID.String(),
				})
				result.Passed = false
			}
		}
	}

	for port, portNodes := range serverPortMap {
		if len(portNodes) > 1 {
			codes := make([]string, 0, len(portNodes))
			for _, n := range portNodes {
				codes = append(codes, n.Code)
			}
			conflictMsg := fmt.Sprintf("同一服务器下ServerPort(%d)冲突，涉及节点: %s", port, strings.Join(codes, ", "))
			for range portNodes {
				result.Errors = append(result.Errors, model.PrecheckItem{
					Level:   "error",
					Code:    "SERVER_PORT_CONFLICT",
					Message: conflictMsg,
					Scope:   "server",
					ScopeID: serverCode,
				})
			}
			result.Passed = false
		}
	}

	for sni, sniNodes := range sniMap {
		if len(sniNodes) > 1 {
			codes := make([]string, 0, len(sniNodes))
			for _, n := range sniNodes {
				codes = append(codes, n.Code)
			}
			// 多节点共用同一 REALITY 伪装 SNI（如 mesu.apple.com）是正常的伪装需求，
			// 不应阻断配置下发。第一个节点注册到 nginx stream SNI 分流，
			// 后续节点需改为 direct 类型（xray 直接监听 443）绕过 nginx stream map。
			warnMsg := fmt.Sprintf("同一服务器下REALITY SNI(%s)被多个节点共用: %s；首个节点走 nginx stream 分流，其余节点需用 direct 类型", sni, strings.Join(codes, ", "))
			result.Warnings = append(result.Warnings, model.PrecheckItem{
				Level:   "warning",
				Code:    "REALITY_SNI_SHARED",
				Message: warnMsg,
				Scope:   "server",
				ScopeID: serverCode,
			})
		}
	}

	if len(nodes) == 0 {
		result.Warnings = append(result.Warnings, model.PrecheckItem{
			Level:   "warning",
			Code:    "NO_NODES",
			Message: "该作用域下没有启用的节点",
			Scope:   string(scopeType),
			ScopeID: scopeID.String(),
		})
	}

	return result, nil
}

// Publish P1-8: 正式发布流程。
// 从 nodes 表自动渲染最新配置 → 创建部署批次 → 推送到 agent。
// 与 RefreshConfig 的区别：Publish 创建正式的 deployment_batch 记录，支持灰度/回滚策略。
func (s *DeploymentService) Publish(ctx context.Context, adminID uuid.UUID, scopeType model.ScopeType, scopeID uuid.UUID, strategy model.DeploymentStrategy) (*model.DeploymentBatch, []*model.DeploymentTarget, *model.ConfigVersion, error) {
	precheckResult, precheckErr := s.PrecheckDeployment(ctx, scopeType, scopeID)
	if precheckErr != nil {
		return nil, nil, nil, precheckErr
	}
	if precheckResult != nil && !precheckResult.Passed {
		errMsgs := make([]string, 0, len(precheckResult.Errors))
		for _, e := range precheckResult.Errors {
			errMsgs = append(errMsgs, e.Message)
		}
		return nil, nil, nil, fmt.Errorf("%w: precheck failed: %s", ErrPreflightValidation, strings.Join(errMsgs, "; "))
	}

	if strategy == "" {
		strategy = model.DeploymentStrategyRolling
	}

	// 1. 获取/创建最新配置版本（自动渲染 + 四级校验 + 推送）
	var cv *model.ConfigVersion
	var err error
	switch scopeType {
	case model.ScopeTypeNode:
		cv, err = s.RefreshNodeConfig(ctx, scopeID)
	case model.ScopeTypeRuntime:
		cv, err = s.RefreshRuntimeConfig(ctx, scopeID)
	default:
		return nil, nil, nil, ErrInvalidScopeType
	}
	if err != nil {
		return nil, nil, nil, err
	}

	// 2. 创建部署批次（含灰度策略分 phase）
	req := &model.DeployRequest{
		ScopeType:   scopeType,
		ScopeID:     scopeID,
		Strategy:    strategy,
		ContentJSON: cv.ContentJSON,
	}
	batch, targets, err := s.Deploy(ctx, adminID, req)
	if err != nil {
		return nil, nil, nil, err
	}

	s.logger.Info("publish created",
		"batch_id", batch.ID, "version_no", cv.VersionNo, "strategy", strategy, "targets", len(targets))
	return batch, targets, cv, nil
}

// Rollback P1-8: 回滚指定部署批次。
// reason 为回滚原因，记录到日志中。
func (s *DeploymentService) Rollback(ctx context.Context, batchID uuid.UUID, reason string) error {
	return s.RollbackBatch(ctx, batchID, reason)
}

// GetBatchDiff P1-8: 获取部署批次的配置 diff。
// 比较 target_version 和 previous_version 的内容差异，返回结构化 diff 摘要。
func (s *DeploymentService) GetBatchDiff(ctx context.Context, batchID uuid.UUID) (map[string]interface{}, error) {
	targets, err := s.deploymentRepo.ListTargetsByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, ErrBatchNotFound
	}

	target := targets[0]

	// 获取新版本配置
	newVersion, err := s.deploymentRepo.GetConfigVersionByID(ctx, target.TargetVersionID)
	if err != nil {
		return nil, err
	}
	if newVersion == nil {
		return nil, ErrVersionNotFound
	}

	// 获取旧版本配置
	var oldVersion *model.ConfigVersion
	if target.PreviousVersionID != nil {
		oldVersion, err = s.deploymentRepo.GetConfigVersionByID(ctx, *target.PreviousVersionID)
		if err != nil {
			return nil, err
		}
	}

	var oldContent map[string]interface{}
	var oldVersionNo int64
	var oldContentHash string
	if oldVersion != nil {
		oldContent = oldVersion.ContentJSON
		oldVersionNo = oldVersion.VersionNo
		oldContentHash = oldVersion.ContentHash
	}

	diffSummary := pkg.GenerateDiff(oldContent, newVersion.ContentJSON)

	return map[string]interface{}{
		"batch_id":         batchID,
		"new_version_no":   newVersion.VersionNo,
		"new_content_hash": newVersion.ContentHash,
		"old_version_no":   oldVersionNo,
		"old_content_hash": oldContentHash,
		"diff_summary":     diffSummary,
		"target_count":     len(targets),
	}, nil
}

// GetBatchResults P1-8: 获取部署批次的所有 target 结果及状态统计。
func (s *DeploymentService) GetBatchResults(ctx context.Context, batchID uuid.UUID) (map[string]interface{}, error) {
	targets, err := s.deploymentRepo.ListTargetsByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, ErrBatchNotFound
	}

	// 按状态统计
	statusCounts := make(map[string]int)
	phaseMap := make(map[int][]*model.DeploymentTarget)
	for _, t := range targets {
		statusCounts[string(t.Status)]++
		phaseMap[t.PhaseNo] = append(phaseMap[t.PhaseNo], t)
	}

	// 构建 phase 摘要
	phases := make([]map[string]interface{}, 0, len(phaseMap))
	for phaseNo, phaseTargets := range phaseMap {
		phaseStatus := make(map[string]int)
		for _, t := range phaseTargets {
			phaseStatus[string(t.Status)]++
		}
		phases = append(phases, map[string]interface{}{
			"phase_no":      phaseNo,
			"target_count":  len(phaseTargets),
			"status_counts": phaseStatus,
		})
	}

	return map[string]interface{}{
		"batch_id":      batchID,
		"targets":       targets,
		"target_count":  len(targets),
		"status_counts": statusCounts,
		"phases":        phases,
	}, nil
}

func MapDeploymentErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrVersionNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrBatchNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrTargetNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrInvalidScopeType):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrDeploymentRunning):
		return config.CodeConflict, err.Error()
	case errors.Is(err, ErrInvalidContent):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrNodeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrPreflightValidation):
		return config.CodeValidationFailed, err.Error()
	case errors.Is(err, ErrPayloadNotFound):
		return config.CodeNotFound, err.Error()
	default:
		return config.CodeInternalError, err.Error()
	}
}

type StreamUpstreamEntry struct {
	SNI        string
	UpstreamID string
	TargetAddr string
	Mode       string
}

func sanitizeSNI(s string) string {
	r := strings.NewReplacer(".", "_", "-", "_", "*", "_")
	return r.Replace(s)
}

func extractNodePath(n *model.Node, cdnPath string) string {
	p := cdnPath
	if p == "" && n.Path != nil && *n.Path != "" {
		p = *n.Path
	}
	if p == "" {
		if cp, ok := n.ConfigJSON["path"].(string); ok && cp != "" {
			p = cp
		} else if xhttpMap, ok := n.ConfigJSON["xhttp"].(map[string]interface{}); ok {
			if xp, ok := xhttpMap["path"].(string); ok && xp != "" {
				p = xp
			}
		}
	}
	// P1-2: 空路径告警 — CDN 节点无 path 会导致 nginx location "/" 冲突
	if p == "" && n.Code != "" {
		slog.Warn("extractNodePath returned empty path for CDN node; nginx location may conflict",
			"node_code", n.Code,
			"server_name", func() string {
				if n.SNI != nil {
					return *n.SNI
				}
				return ""
			}())
	}
	return p
}

func extractRealitySNI(n *model.Node) string {
	if n.ConfigJSON != nil {
		if rs, ok := n.ConfigJSON["reality_settings"].(map[string]interface{}); ok {
			if sn, ok := rs["server_name"].(string); ok && sn != "" {
				return sn
			}
		}
		if sn, ok := n.ConfigJSON["reality_server_name"].(string); ok && sn != "" {
			return sn
		}
		if sn, ok := n.ConfigJSON["server_name"].(string); ok && sn != "" {
			return sn
		}
	}
	if n.SNI != nil && *n.SNI != "" {
		return *n.SNI
	}
	if n.RealityServerName != nil && *n.RealityServerName != "" {
		return *n.RealityServerName
	}
	return ""
}

func extractRealityServerPort(n *model.Node) int {
	if n.ConfigJSON != nil {
		if sp, ok := n.ConfigJSON["server_port"].(float64); ok && sp > 0 && sp <= 65535 {
			return int(sp)
		}
	}
	if n.ServerPort != nil && *n.ServerPort > 0 && *n.ServerPort <= 65535 {
		return *n.ServerPort
	}
	if n.Port > 0 && n.Port <= 65535 && n.Port != 443 {
		return int(n.Port)
	}
	return 0
}

func renderStreamSnippet(entries []StreamUpstreamEntry, defaultBackend string) string {
	// P2-2: 入口绝缘 — 过滤掉 SNI 为空或 UpstreamID 为空的非法条目。
	// 防止上游逻辑误将 UDP 节点或无 SNI 节点注入 stream 配置，
	// 导致 nginx stream map 生成空键值对引发 nginx -t 失败。
	validEntries := make([]StreamUpstreamEntry, 0, len(entries))
	for _, e := range entries {
		if e.SNI == "" || e.UpstreamID == "" {
			slog.Warn("renderStreamSnippet: skipping invalid stream entry (empty SNI or UpstreamID)",
				"sni", e.SNI, "upstream_id", e.UpstreamID, "target", e.TargetAddr)
			continue
		}
		validEntries = append(validEntries, e)
	}
	entries = validEntries

	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Auto-generated by YunDu deployment service — DO NOT EDIT MANUALLY\n")

	sb.WriteString("map $ssl_preread_server_name $internal_upstream {\n")
	for _, e := range entries {
		if defaultBackend != "" && e.UpstreamID == defaultBackend {
			continue
		}
		escapedSNI := regexp.QuoteMeta(e.SNI)
		fmt.Fprintf(&sb, "    ~^%s$ %s;\n", escapedSNI, e.UpstreamID)
	}
	if defaultBackend != "" {
		fmt.Fprintf(&sb, "    default %s;\n", defaultBackend)
	} else {
		sb.WriteString("    default \"\";\n")
	}
	sb.WriteString("}\n")

	for _, e := range entries {
		fmt.Fprintf(&sb, "upstream %s { server %s; }\n", e.UpstreamID, e.TargetAddr)
	}

	sb.WriteString("server {\n")
	sb.WriteString("    listen 443 reuseport;\n")
	sb.WriteString("    listen [::]:443 reuseport;\n")
	sb.WriteString("    proxy_pass $internal_upstream;\n")
	sb.WriteString("    ssl_preread on;\n")
	sb.WriteString("    proxy_protocol off;\n")
	sb.WriteString("    proxy_connect_timeout 2s;\n")
	sb.WriteString("    proxy_timeout 30s;\n")
	sb.WriteString("}\n")

	return sb.String()
}

func deepCloneJSONMap(src map[string]interface{}) (map[string]interface{}, error) {
	if src == nil {
		return nil, nil
	}
	b, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst map[string]interface{}
	if err := json.Unmarshal(b, &dst); err != nil {
		return nil, err
	}
	return dst, nil
}
