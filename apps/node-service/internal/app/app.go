package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/auth"
	"github.com/airport-panel/config/db"
	"github.com/airport-panel/config/events"
	configredis "github.com/airport-panel/config/redis"
	"github.com/airport-panel/config/server"
	pb "github.com/airport-panel/proto/agent/v1"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/airport-panel/node-service/internal/aidiag"
	"github.com/airport-panel/node-service/internal/cert"
	"github.com/airport-panel/node-service/internal/channelhealth"
	svccfg "github.com/airport-panel/node-service/internal/config"
	"github.com/airport-panel/node-service/internal/doctor"
	"github.com/airport-panel/node-service/internal/experience"
	"github.com/airport-panel/node-service/internal/exposure"
	"github.com/airport-panel/node-service/internal/grpcserver"
	"github.com/airport-panel/node-service/internal/handler"
	"github.com/airport-panel/node-service/internal/importer"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/outbound"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/airport-panel/node-service/internal/protocol"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/airport-panel/node-service/internal/routing"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/airport-panel/node-service/internal/upgrade"
	"github.com/airport-panel/subscription/validator"
)

type uriBulkCreatorAdapter struct {
	nodeService *service.NodeService
}

func (a *uriBulkCreatorAdapter) CreateFromURIPreview(ctx context.Context, req *importer.URINodeCreateRequest) (uuid.UUID, error) {
	uriReq := &service.CreateNodeFromURIRequest{
		Name:          req.Name,
		ProtocolType:  req.ProtocolType,
		TransportType: req.TransportType,
		SecurityType:  req.SecurityType,
		Host:          req.Host,
		Port:          req.Port,
		ConfigJSON:    req.ConfigJSON,
		ServerID:      req.ServerID,
		RuntimeID:     req.RuntimeID,
		Code:          req.Code,
		Region:        req.Region,
		GroupID:       req.GroupID,
		Multiplier:    req.Multiplier,
	}
	node, err := a.nodeService.CreateNodeFromURI(ctx, uriReq)
	if err != nil {
		return uuid.Nil, err
	}
	return node.ID, nil
}

// doctorNodeFetcherAdapter 同时实现 doctor.NodeExposureFetcher 和 doctor.NodeProbeFetcher
// 通过查询 nodeRepo 获取节点的协议/传输/安全类型与对外地址，供 doctor 服务做检查项过滤与真实网络探测。
type doctorNodeFetcherAdapter struct {
	nodeRepo *repo.NodeRepo
}

func (a *doctorNodeFetcherAdapter) getNode(ctx context.Context, nodeID uuid.UUID) (*model.Node, error) {
	return a.nodeRepo.GetByID(ctx, nodeID)
}

// Fetch 实现 doctor.NodeExposureFetcher
func (a *doctorNodeFetcherAdapter) Fetch(ctx context.Context, nodeID uuid.UUID) (*doctor.NodeExposureInfo, error) {
	node, err := a.getNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	mode := "direct"
	if node.Metadata != nil {
		if v, ok := node.Metadata["exposure_mode"].(string); ok && v != "" {
			mode = v
		}
	}
	return &doctor.NodeExposureInfo{
		NodeID:       node.ID,
		ExposureMode: mode,
		ProtocolType: node.ProtocolType,
	}, nil
}

// FetchProbeInfo 实现 doctor.NodeProbeFetcher
func (a *doctorNodeFetcherAdapter) FetchProbeInfo(ctx context.Context, nodeID uuid.UUID) (*doctor.NodeProbeInfo, error) {
	node, err := a.getNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	securityType := ""
	if node.SecurityType != nil {
		securityType = *node.SecurityType
	}
	sni := ""
	if node.SNI != nil {
		sni = *node.SNI
	}
	mode := "direct"
	if node.Metadata != nil {
		if v, ok := node.Metadata["exposure_mode"].(string); ok && v != "" {
			mode = v
		}
	}
	return &doctor.NodeProbeInfo{
		NodeID:        nodeID.String(),
		Host:          node.Address,
		Port:          node.Port,
		ProtocolType:  node.ProtocolType,
		TransportType: node.TransportType,
		SecurityType:  securityType,
		SNI:           sni,
		ExposureMode:  mode,
	}, nil
}

// ListEnabledNodeIDs 实现 doctor.NodeLister：分页遍历所有 is_enabled=true 的节点 ID
func (a *doctorNodeFetcherAdapter) ListEnabledNodeIDs(ctx context.Context) ([]uuid.UUID, error) {
	enabled := true
	var ids []uuid.UUID
	page, pageSize := 1, 200
	for {
		nodes, total, err := a.nodeRepo.List(ctx, page, pageSize, "", "", "", "", &enabled)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		if page*pageSize >= total {
			break
		}
		page++
	}
	return ids, nil
}

// doctorFixDispatcherAdapter 实现 doctor.FixDispatcher
// 解析 nodeID → runtimeID → serverID 后调用 aidiag.ActionDispatcher 真实下发。
type doctorFixDispatcherAdapter struct {
	nodeRepo    *repo.NodeRepo
	runtimeRepo *repo.RuntimeRepo
	dispatcher  aidiag.ActionDispatcher
}

// Dispatch 根据 action 编码调用 aidiag.ActionDispatcher 对应方法
func (a *doctorFixDispatcherAdapter) Dispatch(ctx context.Context, nodeID uuid.UUID, action string, reason string) error {
	serverID, err := a.resolveServerID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("resolve serverID for node %s: %w", nodeID, err)
	}
	switch action {
	case "restart_kernel":
		return a.dispatcher.RestartKernel(ctx, serverID, reason)
	case "reload_config":
		return a.dispatcher.ReloadConfig(ctx, serverID)
	case "renew_cert":
		return a.dispatcher.RenewCert(ctx, serverID, &nodeID)
	default:
		return fmt.Errorf("unsupported doctor fix action %q", action)
	}
}

// resolveServerID 解析 nodeID → runtimeID → serverID
func (a *doctorFixDispatcherAdapter) resolveServerID(ctx context.Context, nodeID uuid.UUID) (uuid.UUID, error) {
	node, err := a.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return uuid.Nil, err
	}
	if node == nil {
		return uuid.Nil, fmt.Errorf("node %s not found", nodeID)
	}
	rt, err := a.runtimeRepo.GetByID(ctx, node.RuntimeID)
	if err != nil {
		return uuid.Nil, err
	}
	if rt == nil {
		return uuid.Nil, fmt.Errorf("runtime %s not found for node %s", node.RuntimeID, nodeID)
	}
	return rt.ServerID, nil
}

// channelSwitcherAdapter 实现 channelhealth.ChannelSwitcher
// 解析 serverID → machineID（Server.Code）后通过 gRPC PushToMachine 下发 MaintenanceCommand。
// 通道切换指令编码在 reason 字段中：reason="channel_switch:<target_channel>:<原始reason>"
// node-agent 收到 ACTION_RESUME + reason 前缀 "channel_switch:" 时执行通道切换逻辑。
type channelSwitcherAdapter struct {
	agentServer *grpcserver.AgentServer
	serverRepo  *repo.ServerRepo
	logger      *slog.Logger
}

// SwitchChannel 实现 channelhealth.ChannelSwitcher
func (a *channelSwitcherAdapter) SwitchChannel(ctx context.Context, serverID uuid.UUID, targetChannel string, reason string) error {
	if a.agentServer == nil {
		return fmt.Errorf("agent server not configured (gRPC server unavailable)")
	}
	srv, err := a.serverRepo.GetByID(ctx, serverID)
	if err != nil {
		return fmt.Errorf("query server %s: %w", serverID, err)
	}
	if srv == nil {
		return fmt.Errorf("server %s not found", serverID)
	}
	if srv.Code == "" {
		return fmt.Errorf("server %s has empty code (machineID)", serverID)
	}
	// 编码通道切换指令到 reason 字段（约定格式：channel_switch:<target>:<reason>）
	switchReason := "channel_switch:" + targetChannel
	if reason != "" {
		switchReason += ":" + reason
	}
	msg := &pb.PanelMessage{
		Payload: &pb.PanelMessage_Maintenance{
			Maintenance: &pb.MaintenanceCommand{
				Action: pb.MaintenanceCommand_ACTION_RESUME,
				Reason: switchReason,
			},
		},
	}
	if err := a.agentServer.PushToMachine(srv.Code, msg); err != nil {
		a.logger.Error("push channel switch command failed",
			"machine_id", srv.Code, "target_channel", targetChannel, "error", err)
		return fmt.Errorf("push to machine %s: %w", srv.Code, err)
	}
	a.logger.Info("channel switch command dispatched",
		"machine_id", srv.Code, "target_channel", targetChannel, "reason", reason)
	return nil
}

// aidiagCompositeCollector 组合 DBLogCollector（DB历史数据）+ gRPC LogStore（实时内核日志）
type aidiagCompositeCollector struct {
	db         *aidiag.DBLogCollector
	logStore   *grpcserver.LogStore
	serverRepo *repo.ServerRepo
	logger     *slog.Logger
}

func (c *aidiagCompositeCollector) CollectLogs(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (string, error) {
	dbLogs, err := c.db.CollectLogs(ctx, serverID, nodeID, start, end)
	if err != nil {
		c.logger.Warn("composite collector: db logs failed", "error", err)
		dbLogs = ""
	}

	srv, err := c.serverRepo.GetByID(ctx, serverID)
	if err != nil || srv == nil || srv.Code == "" {
		return dbLogs, nil
	}

	entries := c.logStore.QueryRaw(srv.Code, start, "", 200)
	if len(entries) == 0 {
		return dbLogs, nil
	}

	var rt strings.Builder
	rt.WriteString(fmt.Sprintf("--- 节点实时内核日志（%s，最近 %d 条）---\n", srv.Code, len(entries)))
	for _, e := range entries {
		ts := time.UnixMilli(e.Timestamp).Format("15:04:05.000")
		rt.WriteString(fmt.Sprintf("  %s [%s] %s: %s\n", ts, e.Level, e.Source, e.Message))
	}
	rt.WriteString("\n")
	return rt.String() + dbLogs, nil
}

func (c *aidiagCompositeCollector) CollectMetrics(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID, start, end time.Time) (map[string]interface{}, error) {
	return c.db.CollectMetrics(ctx, serverID, nodeID, start, end)
}

func Run() {
	cfg := svccfg.Load()
	logger := config.NewLogger(svccfg.ServiceName, cfg.LogLevel)

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		panic(err)
	}
	defer pool.Close()

	// P2-1: 初始化 Redis 客户端和事件总线
	redisClient, err := configredis.NewClient(cfg.Redis)
	var eventBus *events.Bus
	if err != nil {
		logger.Warn("failed to connect to redis, event bus running in local-only mode", "error", err)
		eventBus = events.NewNopBus(logger)
	} else {
		logger.Info("redis connected, event bus enabled", "addr", cfg.Redis.Addr)
		eventBus = events.NewBus(redisClient, logger)
	}

	jwtManager := pkg.NewJWTManager(cfg.JWT.Secret)

	serverRepo := repo.NewServerRepo(pool)
	runtimeRepo := repo.NewRuntimeRepo(pool)
	nodeRepo := repo.NewNodeRepo(pool)
	chainRepo := repo.NewChainRepo(pool)
	healthRepo := repo.NewHealthRepo(pool)
	deploymentRepo := repo.NewDeploymentRepo(pool)
	nodeGroupRepo := repo.NewNodeGroupRepo(pool)
	userNodeCredentialRepo := repo.NewUserNodeCredentialRepo(pool)
	capabilityRepo := repo.NewCapabilityRepo(pool)

	serverService := service.NewServerService(serverRepo, runtimeRepo, nodeRepo)
	runtimeService := service.NewRuntimeService(runtimeRepo, serverRepo)
	nodeService := service.NewNodeService(nodeRepo, runtimeRepo, healthRepo, chainRepo)
	// 注入 NodeGroupRepo，使节点 Create/Update 能同步维护 node_group_members 多对多关联
	nodeService.SetGroupRepo(nodeGroupRepo)
	// P2-1: 注入事件发布器，节点变更后自动发布 TopicConfigChanged 事件
	nodeService.SetEventPublisher(eventBus)
	// 注入端口规划器，用于零 SSH 架构下自动分配 ServerPort
	portPlanner := service.NewPortPlanner(nodeRepo, logger)
	nodeService.SetPortPlanner(portPlanner)
	chainService := service.NewChainService(chainRepo, nodeRepo)
	healthService := service.NewHealthService(healthRepo, nodeRepo, serverRepo, runtimeRepo)
	deploymentService := service.NewDeploymentService(deploymentRepo, nodeRepo, runtimeRepo, serverRepo, chainRepo)
	// 注入 per-user 凭证仓库，使生成的 xray/sing-box 配置包含所有用户的独立 UUID/密码
	deploymentService.SetCredentialRepo(userNodeCredentialRepo)
	// P0-6: 注入能力矩阵仓库，使 preflightValidate 能执行 DB 驱动的 L3 能力校验
	deploymentService.SetCapabilityRepo(capabilityRepo)
	// P2-2: 设置能力降级策略为 downgrade
	// sing-box 节点遇到 XHTTP 等不支持的传输时，自动降级为 HTTPUpgrade 并记录 CapabilityLost 事件
	// 可通过环境变量 CAPABILITY_DEGRADE_STRATEGY 覆盖（deny/downgrade/force_kernel）
	degradeStrategy := service.StrategyDowngrade
	if envStrategy := os.Getenv("CAPABILITY_DEGRADE_STRATEGY"); envStrategy != "" {
		degradeStrategy = service.DegradeStrategy(envStrategy)
	}
	deploymentService.SetDegradeStrategy(degradeStrategy)
	// 注入 ConfigRefresher，使节点 Create/Update 后自动触发配置下发（借鉴 xboard notifyConfigUpdated）
	// 实现"保存即下发"的零 SSH 闭环，无需手动点"发布配置"按钮
	nodeService.SetConfigRefresher(deploymentService)
	// P1-4: 注入 slog 日志器
	nodeService.SetLogger(logger)
	nodeGroupService := service.NewNodeGroupService(nodeGroupRepo)

	authMiddleware := middleware.NewAuthMiddleware(jwtManager)
	rbacMiddleware := middleware.NewRBACMiddleware()
	nonceCache := pkg.NewNonceCache(pkg.DefaultNonceTTL, pkg.DefaultMaxEntries)
	nonceCache.Start(ctx)
	defer nonceCache.Stop()
	agentAuthMiddleware := middleware.NewAgentAuthMiddleware(cfg.AgentAPITokenSalt, cfg.HMACSecret, nonceCache)
	nonceStore := auth.NewMemoryNonceStore()
	defer nonceStore.Stop()
	hmacMiddleware := auth.SignatureGinMiddleware(cfg.HMACSecret, nonceStore)
	_ = hmacMiddleware

	certRepo := cert.NewCertificateRepo(pool)
	tlsProfileRepo := cert.NewTLSProfileRepo(pool)
	certDeployRepo := cert.NewCertDeployRepo(pool)
	certService := cert.NewCertificateService(certRepo, tlsProfileRepo, certDeployRepo, nil, logger)

	// 注入 ECH 密钥对生成器（依赖本地 xray 二进制；未安装时 GenerateECH 返回错误）
	certService.SetECHGenerator(cert.NewXrayECHGenerator(os.Getenv("XRAY_BINARY_PATH")))

	// P9-FIX-2: 注入 NodeSNIReader，使证书 SAN 可自动扫描节点 SNI 同步。
	// 未注入时 SyncSANFromNodes 返回 ErrNodeSNIReaderNotInjected（向后兼容）。
	// *repo.NodeRepo 通过 ListEnabledSNIs 方法自动满足 cert.NodeSNIReader 接口。
	certService.SetNodeSNIReader(nodeRepo)

	// P1-5: 注入证书 SAN 同步钩子到 NodeService。
	// 节点保存后自动扫描该节点 SNI 并同步到匹配的证书 SAN，
	// 避免 SNI 变更后证书未更新导致 TLS 握手失败。
	nodeService.SetCertSyncHook(func(ctx context.Context, n *model.Node) error {
		if n == nil || n.SNI == nil || *n.SNI == "" {
			return nil
		}
		// 查找包含该 SNI 域名的证书并触发 SAN 同步
		// best-effort 策略：查询失败或找不到关联证书则静默跳过
		certs, _, err := certRepo.List(ctx, 1, 200, "", 0)
		if err != nil {
			return nil // 查询失败不阻断节点保存
		}
		sni := *n.SNI
		for _, c := range certs {
			// 检查证书 SANs 是否包含该 SNI 或 CommonName 匹配
			matched := false
			for _, san := range c.SANs {
				if strings.EqualFold(san, sni) {
					matched = true
					break
				}
			}
			if !matched && strings.EqualFold(c.CommonName, sni) {
				matched = true
			}
			if matched {
				if _, _, err := certService.SyncSANFromNodes(ctx, c.ID, nil); err != nil {
					logger.Warn("auto sync SAN failed for cert",
						"cert_id", c.ID, "node_code", n.Code, "sni", sni, "error", err)
				}
			}
		}
		return nil
	})

	// 阶段 C1: 注入 CertBundleSyncStore，使 ACME 续期成功后自动同步 PEM 到 cert_bundles 表。
	// *repo.CapabilityRepo 通过 FindCertBundleIDsByDomain/UpdateCertBundlePEM 满足接口。
	// 未注入时续期仅更新 tls_certificates 表（保持旧行为）。
	certService.SetCertBundleSyncStore(capabilityRepo)

	// 注入 ACME 客户端注册表（如果 ACME_EMAIL 已配置），用于证书自动签发与续期。
	// 支持按证书维度选择 DNS provider（cloudflare/alidns/dnspod/gandi/namesilo）。
	// 未配置 ACME_EMAIL 时跳过（不影响启动，但 ObtainCertificate/TriggerRenew 会退化为 pending）。
	acmeEmail := os.Getenv("ACME_EMAIL")
	if acmeEmail != "" {
		acmeDirURL := os.Getenv("ACME_DIR_URL")
		acmeRegistry := cert.NewACMERegistry(acmeEmail, acmeDirURL, logger)
		certService.SetACMERegistry(acmeRegistry)
		// 启动证书续期定时任务（每 6 小时检查一次）
		go certService.StartRenewalJob(ctx)
		logger.Info("ACME registry injected and renewal job started",
			"email", acmeEmail, "dir_url", acmeDirURL)
	}

	exposureRepo := exposure.NewExposureRepo(pool)
	nginxConfigRepo := exposure.NewNginxConfigRepo(pool)
	compatRuleRepo := exposure.NewCompatRuleRepo(pool)
	exposureService := exposure.NewExposureService(exposureRepo, nginxConfigRepo, compatRuleRepo, nil, nil, nil, logger)

	doctorReportRepo := doctor.NewDoctorReportRepo(pool)
	doctorCheckDefRepo := doctor.NewDoctorCheckDefRepo(pool)
	// doctorNodeFetcherAdapter 同时实现 NodeExposureFetcher / NodeProbeFetcher / NodeLister
	doctorNodeFetcher := &doctorNodeFetcherAdapter{nodeRepo: nodeRepo}
	doctorService := doctor.NewDoctorService(doctorReportRepo, doctorCheckDefRepo, doctorNodeFetcher, nil, logger)
	// 注入真实网络探测所需的节点信息获取器（DNS/TCP/TLS/UDP/Latency/CertExpiry）
	doctorService.SetNodeProbeFetcher(doctorNodeFetcher)
	// 注入节点列表获取器（StartScheduledJob 遍历所有启用节点用）
	doctorService.SetNodeLister(doctorNodeFetcher)
	// 启动 doctor 定时全量检查（每 30 分钟一轮，启动后立即执行首轮）
	go doctorService.StartScheduledJob(ctx)

	// 阶段2.2: 启动 argo_tunnel 节点一致性巡检（每 5 分钟一轮）
	// 防止后续有人手动改 DB 字段绕开渲染逻辑，回到"手工打补丁"的老问题
	argoConsistencyChecker := service.NewArgoTunnelConsistencyChecker(nodeRepo, logger)
	go argoConsistencyChecker.Start(ctx)

	// 阶段3: 零 SSH 完整性 — CF API 自动创建 Tunnel
	// 通过环境变量 CF_API_TOKEN/CF_ACCOUNT_ID/CF_ZONE_ID 配置
	// 未配置时 IsConfigured()=false，handler 返回 CF_API_NOT_CONFIGURED
	cfTunnelService := service.NewCFTunnelService(logger)

	// 启动日志清理定时任务（每天 03:00 清理 30 天前的 node_doctor_reports / cert_deploy_records）
	logCleanupService := service.NewLogCleanupService(pool, logger)
	logCleanupService.StartScheduledJobs(ctx)

	// ===== 通道健康（Phase 8 新增）=====
	channelHealthRepo := channelhealth.NewRepo(pool)
	channelHealthService := channelhealth.NewService(channelHealthRepo, logger)
	// 注入到 healthService（可选依赖）
	healthService.SetChannelHealthRecorder(channelHealthService)

	// ===== AI 诊断（Phase 8 新增）=====
	// 使用 DBLogCollector 采集 channel_health_* 和 node_doctor_reports 数据
	// gRPC 启动后再包装 compositeCollector 追加实时内核日志
	aidiagRepo := aidiag.NewRepo(pool)
	llmClient := aidiag.LLMFromEnv()
	dbCollector := aidiag.NewDBLogCollector(pool)
	aidiagService := aidiag.NewService(aidiagRepo, llmClient, dbCollector, logger)

	// ===== 节点体验评分（Phase 8 新增）=====
	experienceRepo := experience.NewRepo(pool)
	experienceService := experience.NewService(experienceRepo, nil, logger)
	// 启动定时计算循环（5 分钟一次）
	go experienceService.StartCalculationLoop(ctx)

	importJobRepo := importer.NewImportJobRepo(pool)
	importerService := importer.NewImporterService(importJobRepo, nil, nil, logger)
	uriAdapter := &uriBulkCreatorAdapter{nodeService: nodeService}
	importerService.SetURIBulkCreator(uriAdapter)

	protocolRegistryRepo := protocol.NewProtocolRegistryRepo(pool)
	configTemplateRepo := protocol.NewConfigTemplateRepo(pool)
	protocolPresetRepo := protocol.NewProtocolPresetRepo(pool)
	protocolService := protocol.NewProtocolService(protocolRegistryRepo, logger)
	templateService := protocol.NewTemplateService(configTemplateRepo, logger)
	presetService := protocol.NewPresetService(protocolPresetRepo, logger)

	outboundPolicyRepo := outbound.NewOutboundPolicyRepo(pool)
	warpProfileRepo := outbound.NewWarpProfileRepo(pool)
	outboundService := outbound.NewOutboundService(outboundPolicyRepo, logger)
	warpProfileService := outbound.NewWarpProfileService(warpProfileRepo, logger)

	upgradeRepo := upgrade.NewUpgradeTaskRepo(pool)
	upgradeService := upgrade.NewUpgradeService(upgradeRepo, runtimeRepo, nil, logger)

	ruleSetRepo := routing.NewRouteRuleSetRepo(pool)
	routePolicyRepo := routing.NewRoutePolicyRepo(pool)
	policyRuleRepo := routing.NewRoutePolicyRuleRepo(pool)
	bindingRepo := routing.NewNodeRouteBindingRepo(pool)
	lbPolicyRepo := routing.NewNodeGroupLBPolicyRepo(pool)
	outboundGroupRepo := routing.NewOutboundGroupRepo(pool)

	ruleSetService := routing.NewRuleSetService(ruleSetRepo, nil, logger)
	policyService := routing.NewPolicyService(routePolicyRepo, policyRuleRepo, ruleSetRepo, logger)
	policyRuleService := routing.NewPolicyRuleService(policyRuleRepo, routePolicyRepo, ruleSetRepo, logger)
	bindingService := routing.NewBindingService(bindingRepo, routePolicyRepo, logger)
	lbPolicyService := routing.NewLBPolicyService(lbPolicyRepo, logger)
	outboundGroupService := routing.NewOutboundGroupService(outboundGroupRepo, logger)
	routingRenderer := routing.NewRoutingRenderer(
		&routing.RoutingDataReaderAdapter{
			BindingRepo:       bindingRepo,
			PolicyRepo:        routePolicyRepo,
			RuleRepo:          policyRuleRepo,
			RuleSetRepo:       ruleSetRepo,
			OutboundGroupRepo: outboundGroupRepo,
		},
		logger,
	)

	// 将路由渲染器注入 DeploymentService，使下发的 xray/sing-box 配置
	// 包含节点绑定的路由策略（route_policies → routing.rules + balancers）
	deploymentService.SetRoutingRenderer(routingRenderer)

	healthHandler := handler.NewHealthHandler()
	// P2-K: 一键安装脚本 handler，PanelURL 从配置读取，AgentCDNURL 从环境变量读取
	installHandler := handler.NewInstallHandler(cfg.PublicURL, os.Getenv("AGENT_CDN_URL"))
	agentHandler := handler.NewAgentHandler(runtimeService, healthService, serverService, deploymentService)
	// Agent 零配置部署：Bootstrap 服务根据 agent_token 自动下发运行时配置
	agentBootstrapService := service.NewAgentBootstrapService(serverService, runtimeService, deploymentService, nodeRepo, cfg.AgentAPITokenSalt, logger)
	agentBootstrapHandler := handler.NewAgentBootstrapHandler(agentBootstrapService)
	sharedLogStore := grpcserver.NewLogStore()
	wsHandler := handler.NewWSHandler(logger, sharedLogStore, runtimeService, healthService, serverService, deploymentService)
	adminServerHandler := handler.NewAdminServerHandler(serverService, runtimeService, cfg.AgentAPITokenSalt, cfg.PublicURL, sharedLogStore)
	adminNodeHandler := handler.NewAdminNodeHandler(nodeService, healthService)
	// P2-3: 节点多 Host 管理
	nodeHostRepo := repo.NewNodeHostRepo(pool)
	adminNodeHostHandler := handler.NewAdminNodeHostHandler(nodeHostRepo)
	adminNodeGroupHandler := handler.NewAdminNodeGroupHandler(nodeGroupService)
	adminChainHandler := handler.NewAdminChainHandler(chainService)
	adminDeploymentHandler := handler.NewAdminDeploymentHandler(deploymentService)
	adminHealthHandler := handler.NewAdminHealthHandler(healthService)
	adminCertHandler := cert.NewAdminCertHandler(certService)
	adminCertBundleHandler := handler.NewAdminCertBundleHandler(capabilityRepo)
	adminExposureHandler := exposure.NewAdminExposureHandler(exposureService)
	adminDoctorHandler := doctor.NewAdminDoctorHandler(doctorService)
	adminImportHandler := importer.NewAdminImportHandler(importerService)
	adminUpgradeHandler := upgrade.NewAdminUpgradeHandler(upgradeService)
	adminProtocolHandler := protocol.NewAdminProtocolHandler(protocolService)
	adminTemplateHandler := protocol.NewAdminTemplateHandler(templateService)
	adminPresetHandler := protocol.NewAdminPresetHandler(presetService)
	adminOutboundHandler := outbound.NewAdminOutboundHandler(outboundService)
	adminWarpHandler := outbound.NewAdminWarpHandler(warpProfileService)
	adminRoutingHandler := routing.NewAdminRoutingHandler(
		ruleSetService, policyService, policyRuleService,
		bindingService, lbPolicyService, outboundGroupService, routingRenderer,
	)
	// ===== DualKernelValidator（双核校验器）=====
	// Enhancement 专项 → 双核渲染 → 真实 dry-run（需二进制）→ 语义等价性
	// 开发环境不设 XRAY_BINARY/SINGBOX_BINARY 时自动跳过 dry-run，仅做前三步
	xrayBin := os.Getenv("XRAY_BINARY")
	singboxBin := os.Getenv("SINGBOX_BINARY")
	dualKernelValidator := validator.NewDualKernelValidator(xrayBin, singboxBin)
	if xrayBin == "" && singboxBin == "" {
		logger.Info("DualKernelValidator: dry-run skipped (XRAY_BINARY/SINGBOX_BINARY not set), Enhancement+render+semantic still active")
	} else {
		logger.Info("DualKernelValidator: dry-run enabled", "xray_binary", xrayBin, "singbox_binary", singboxBin)
	}

	adminValidationHandler := handler.NewAdminValidationHandler(dualKernelValidator)
	adminChannelHealthHandler := channelhealth.NewAdminHandler(channelHealthService)
	adminAIDiagHandler := aidiag.NewAdminHandler(aidiagService)
	adminExperienceHandler := experience.NewAdminHandler(experienceService)

	opts := server.DefaultOptions(svccfg.ServiceName, cfg.Port)
	opts.Logger = logger
	opts.RegisterRoutes = func(api *gin.RouterGroup) {
		healthHandler.RegisterRoutes(api)

		// P2-K: 一键安装脚本（公开端点，无需认证）
		api.GET("/install.sh", installHandler.ServeInstallScript)

		publicRoutes := api.Group("/public")
		{
			adminPresetHandler.RegisterPublicRoutes(publicRoutes)
		}

		agentRoutes := api.Group("/agent")
		// /agent/health 供 node-agent HTTP 通道 HealthCheck 探测使用，
		// 仅返回 200，不需要 AgentAuth（HealthCheck 仅携带 Bearer+HMAC 头，无 X-Server-Code）。
		agentRoutes.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
		// /agent/bootstrap 供 node-agent 零配置部署首次启动时拉取运行时配置，
		// 不需要 AgentAuth（agent 此时还没有 server_code / HMAC 头），仅通过 token 参数验证。
		agentRoutes.GET("/bootstrap", agentBootstrapHandler.Bootstrap)
		// /agent/machine/nodes 供 Machine 模式 Agent 定期拉取该服务器上所有节点列表，
		// 不需要 AgentAuth（使用 server_token 查询参数认证，与 bootstrap 相同的认证模式）。
		agentRoutes.GET("/machine/nodes", agentBootstrapHandler.MachineNodes)
		// /agent/machine/cdn-vhosts 供 Machine 模式 Agent 定期拉取该服务器上所有节点聚合后的 nginx vhost 配置，
		// 不需要 AgentAuth（使用 server_token 查询参数认证，与 bootstrap 相同的认证模式）。
		agentRoutes.GET("/machine/cdn-vhosts", agentBootstrapHandler.MachineCDNVhosts)
		// T05: /agent/machine/cloudflared-tunnels 供 Machine 模式 Agent 定期拉取该服务器上所有节点的 cloudflared 隧道配置聚合，
		// 不需要 AgentAuth（使用 server_token 查询参数认证，与 bootstrap 相同的认证模式）。
		agentRoutes.GET("/machine/cloudflared-tunnels", agentBootstrapHandler.MachineCloudflaredTunnels)
		// Self-upgrade endpoints (public, token-free, so that agents with broken auth can still recover via upgrade).
		// /agent/upgrade/check  — polled by SelfUpgrader every 5min; returns VersionInfo JSON or 204.
		// /agent/download/node-agent-linux-amd64 — serves the new binary.
		agentRoutes.GET("/upgrade/check", agentHandler.UpgradeCheck)
		agentRoutes.GET("/download/node-agent-linux-amd64", agentHandler.DownloadAgentBinary)
		agentRoutes.Use(agentAuthMiddleware.AgentAuth())
		{
			agentRoutes.POST("/register", agentHandler.Register)
			agentRoutes.POST("/heartbeat", agentHandler.Heartbeat)
			agentRoutes.GET("/config", agentHandler.FetchConfig)
			agentRoutes.GET("/cdn-vhosts", agentHandler.CDNVhosts)
			agentRoutes.POST("/config/result", agentHandler.ReportConfigResult)
			// P3-1: 加密 Payload Manifest 拉取 + 部署结果上报
			agentRoutes.GET("/payload", agentHandler.FetchPayload)
			agentRoutes.POST("/deployment-result", agentHandler.ReportDeploymentResult)
			// D7 修复: Cloudflared 隧道配置拉取（cloudflared reconciler 轮询）
			agentRoutes.GET("/cloudflared-tunnels", agentHandler.CloudflaredTunnels)
			// D8 修复: 设备状态上报 + 全局设备态拉取
			agentRoutes.POST("/devices/report", agentHandler.ReportDevices)
			agentRoutes.GET("/devices/alive", agentHandler.FetchAliveDevices)
			// P2-4: 二进制升级规格拉取
			agentRoutes.GET("/binary-spec", agentHandler.BinarySpec)
			agentRoutes.GET("/ws", wsHandler.HandleWebSocket)
		}

		adminRoutes := api.Group("/admin")
		adminRoutes.Use(authMiddleware.AdminAuth())
		{
			adminServerHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminNodeHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminNodeHostHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminNodeGroupHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminChainHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminDeploymentHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminHealthHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminCertHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminCertBundleHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminExposureHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminDoctorHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminImportHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminUpgradeHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminProtocolHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminTemplateHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminPresetHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminOutboundHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminWarpHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminRoutingHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminValidationHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminChannelHealthHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminAIDiagHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)
			adminExperienceHandler.RegisterRoutesWithGroup(adminRoutes, rbacMiddleware)

			// 阶段3: CF Tunnel 自动创建（零 SSH 完整性）
			// 端点：POST /api/v1/admin/cf-tunnels、GET /api/v1/admin/cf-tunnels/status
			cfTunnelHandler := handler.NewCFTunnelHandler(cfTunnelService, logger)
			adminRoutes.GET("/cf-tunnels/status", cfTunnelHandler.CheckConfigured)
			adminRoutes.POST("/cf-tunnels", rbacMiddleware.RequireSuperAdmin(), cfTunnelHandler.CreateTunnel)
		}
	}

	srv := server.New(opts)

	grpcLogger := logger.With("transport", "grpc")

	grpcHandler := &struct {
		sync.RWMutex
		onMessage func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error)
	}{}

	onGRPCMessage := func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error) {
		grpcLogger.Debug("received gRPC message", "machine_id", machineID, "seq", msg.Seq)
		if ping := msg.GetPing(); ping != nil {
			return &pb.PanelMessage{
				Payload: &pb.PanelMessage_Pong{Pong: &pb.Pong{
					Timestamp:     time.Now().UnixMilli(),
					PingTimestamp: ping.Timestamp,
				}},
			}, nil
		}
		grpcHandler.RLock()
		handler := grpcHandler.onMessage
		grpcHandler.RUnlock()
		if handler != nil {
			return handler(ctx, machineID, msg)
		}
		return nil, nil
	}
	if _, agentSrv, err := grpcserver.StartGRPCServer(&grpcserver.ServerConfig{
		Port:        cfg.GRPCPort,
		TLSCertFile: cfg.TLSCertFile,
		TLSKeyFile:  cfg.TLSKeyFile,
		TokenSalt:   cfg.AgentAPITokenSalt,
		Logger:      grpcLogger,
		OnMessage:   onGRPCMessage,
		LogStore:    sharedLogStore,
		NonceCache:  nonceCache,
	}); err != nil {
		logger.Error("failed to start gRPC server", "error", err)
	} else {
		logStore := agentSrv.LogStore()
		compositeCollector := &aidiagCompositeCollector{
			db:         dbCollector,
			logStore:   logStore,
			serverRepo: serverRepo,
			logger:     logger,
		}
		aidiagService.SetLogCollector(compositeCollector)

		// 注入 AI 诊断自动修复派发器（GRPCDispatcher 依赖 *AgentServer.PushToMachine）
		// 用于 ApplyAutofix 将 restart_kernel/reload_config/renew_cert 真实下发到 node-agent
		dispatcher := aidiag.NewGRPCDispatcher(agentSrv, serverRepo, certService, logger)
		aidiagService.SetActionDispatcher(dispatcher)
		// 注入 doctor 自动修复派发器（解析 nodeID→serverID 后调用 aidiag.ActionDispatcher）
		doctorFixDispatcher := &doctorFixDispatcherAdapter{
			nodeRepo:    nodeRepo,
			runtimeRepo: runtimeRepo,
			dispatcher:  dispatcher,
		}
		doctorService.SetFixDispatcher(doctorFixDispatcher)
		// 注入通道切换派发器（解析 serverID→machineID 后通过 gRPC PushToMachine 下发）
		channelHealthService.SetChannelSwitcher(&channelSwitcherAdapter{
			agentServer: agentSrv,
			serverRepo:  serverRepo,
			logger:      logger,
		})
		// P0-7: 注入 CompositeConfigPusher（gRPC + WS fan-out）到 deploymentService
		// 配置版本更新后主动推送 ConfigPush 到 agent，无需等待心跳轮询
		configPusher := service.NewCompositeConfigPusher(logger, agentSrv, wsHandler)
		deploymentService.SetConfigPusher(configPusher)

		// P0: 创建 UserDeltaService 并注入 DeploymentService（增量用户变更推送）
		userDeltaSvc := service.NewUserDeltaService(configPusher, logger)
		deploymentService.SetUserDeltaService(userDeltaSvc)

		// P0: 创建 UserEventHandler 并订阅用户生命周期事件
		// 打通 ban/unban/traffic_reset/plan_changed/token_revoked → 节点实时推送闭环
		userEventHandler := service.NewUserEventHandler(
			deploymentService,
			runtimeRepo,
			userNodeCredentialRepo,
			userDeltaSvc,
			logger,
		)
		eventBus.Subscribe(events.TopicUserBanned, userEventHandler.HandleUserBanned)
		eventBus.Subscribe(events.TopicUserUnbanned, userEventHandler.HandleUserUnbanned)
		eventBus.Subscribe(events.TopicTrafficReset, userEventHandler.HandleTrafficReset)
		eventBus.Subscribe(events.TopicPlanChanged, userEventHandler.HandlePlanChanged)
		eventBus.Subscribe(events.TopicTokenRevoked, userEventHandler.HandleTokenRevoked)
		logger.Info("user event handler registered for topics: user:banned,user:unbanned,user:traffic_reset,user:plan_changed,token:revoked")

		logger.Info("aidiag+doctor+channelhealth dispatchers injected (gRPC-backed)", "component", "app")

		// 设置 gRPC 心跳处理器（与 ws_handler.handleWSHeartbeat 逻辑对齐）
		grpcHandler.Lock()
		grpcHandler.onMessage = func(ctx context.Context, machineID string, msg *pb.AgentMessage) (*pb.PanelMessage, error) {
			if hb := msg.GetHeartbeat(); hb != nil {
				serverCode := machineID
				currentVersion := strconv.FormatInt(hb.GetConfigVersion(), 10)

				hbReq := &model.AgentHeartbeatRequest{
					ServerCode:    serverCode,
					Timestamp:     time.Now(),
					ConfigVersion: currentVersion,
				}

				// 提取 ServerLoad (CPU/内存/磁盘/网络) 指标
				if load := hb.GetLoad(); load != nil {
					cpu := float64(load.GetCpuPercent())
					mem := float64(load.GetMemPercent())
					disk := float64(load.GetDiskPercent())
					netIn := float64(load.GetNetworkInKbps())
					netOut := float64(load.GetNetworkOutKbps())
					hbReq.CPUPercent = &cpu
					hbReq.MemPercent = &mem
					hbReq.DiskPercent = &disk
					hbReq.Metrics = map[string]interface{}{
						"cpu_percent":      cpu,
						"mem_percent":      mem,
						"mem_total_mb":     load.GetMemTotalMb(),
						"mem_used_mb":      load.GetMemUsedMb(),
						"disk_percent":     disk,
						"disk_total_gb":    load.GetDiskTotalGb(),
						"disk_used_gb":     load.GetDiskUsedGb(),
						"network_in_kbps":  netIn,
						"network_out_kbps": netOut,
						"uptime_seconds":   load.GetUptimeSeconds(),
						"load_1":           load.GetLoad_1(),
						"load_5":           load.GetLoad_5(),
						"load_15":          load.GetLoad_15(),
					}
				}

				// 提取 KernelInfo（版本、运行状态）
				if kernel := hb.GetKernel(); kernel != nil {
					hbReq.RuntimeVersion = kernel.GetVersion()
					if kernel.GetRunning() {
						hbReq.RuntimeStatus = "active"
					} else {
						hbReq.RuntimeStatus = "inactive"
					}
				}

				if ch := hb.GetChannel(); ch != nil {
					chState := "healthy"
					switch ch.GetState() {
					case pb.ChannelState_CHANNEL_STATE_DEGRADED:
						chState = "degraded"
					case pb.ChannelState_CHANNEL_STATE_UNHEALTHY:
						chState = "unhealthy"
					case pb.ChannelState_CHANNEL_STATE_UNKNOWN:
						chState = "unknown"
					}
					rttMs := int(ch.GetRttMs())
					lastErr := ch.GetLastError()
					hbReq.ChannelHealth = &model.ChannelHealthReport{
						ActiveChannel: "grpc",
						ChannelState:  chState,
						RTTMs:         &rttMs,
						LastError:     &lastErr,
					}
				}

				if healthService != nil {
					_ = healthService.ReportHeartbeat(ctx, serverCode, nil, hbReq)
				}

				action := pb.HeartbeatAction_HEARTBEAT_ACTION_NONE
				latestVersion := int64(0)

				if serverService != nil && runtimeService != nil && deploymentService != nil {
					serverSrv, err := serverService.GetServerByCode(ctx, serverCode)
					if err == nil && serverSrv != nil {
						providerType := model.RuntimeProviderNodeAgent
						rt, err := runtimeService.GetRuntimeByServerAndProvider(ctx, serverSrv.ID, providerType, nil)
						if err == nil && rt != nil {
							targetVersion, err := deploymentService.GetRuntimeConfig(ctx, rt.ID, "")
							if err == nil && targetVersion != nil {
								latestVersion = targetVersion.VersionNo
								if hb.GetConfigVersion() < targetVersion.VersionNo {
									action = pb.HeartbeatAction_HEARTBEAT_ACTION_RELOAD
								}
							}
						}
					}
				}

				if kernel := hb.GetKernel(); kernel != nil {
					if !kernel.GetRunning() {
						action = pb.HeartbeatAction_HEARTBEAT_ACTION_RESTART
					}
				}

				return &pb.PanelMessage{
					Seq:       msg.Seq,
					Timestamp: time.Now().UnixMilli(),
					Payload: &pb.PanelMessage_HeartbeatAck{HeartbeatAck: &pb.HeartbeatAck{
						Action:              action,
						LatestConfigVersion: latestVersion,
						ServerTime:          time.Now().Unix(),
					}},
				}, nil
			}
			return nil, nil
		}
		grpcHandler.Unlock()
	}

	// P2-1: 启动事件总线（Redis Pub/Sub 消费）
	busCtx, busCancel := context.WithCancel(context.Background())
	defer busCancel()
	if err := eventBus.Start(busCtx); err != nil {
		logger.Error("failed to start event bus", "error", err)
	}

	// 优雅关闭：捕获 SIGINT/SIGTERM 停止事件总线
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down node-service...")
		eventBus.Stop()
		busCancel()
	}()

	srv.Start()
}
