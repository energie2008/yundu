package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	runtimeService    *service.RuntimeService
	healthService     *service.HealthService
	serverService     *service.ServerService
	deploymentService *service.DeploymentService
	// P2-4: 二进制升级规格服务（agent 拉取期望版本用）
	binarySpecService *service.BinarySpecService
	logger            *slog.Logger
}

func NewAgentHandler(runtimeService *service.RuntimeService, healthService *service.HealthService, serverService *service.ServerService, deploymentService *service.DeploymentService) *AgentHandler {
	return &AgentHandler{
		runtimeService:    runtimeService,
		healthService:     healthService,
		serverService:     serverService,
		deploymentService: deploymentService,
		logger:            slog.Default(),
	}
}

// SetBinarySpecService P2-4: 注入二进制升级规格服务
// 注入后 /agent/binary-spec 端点才可用，未注入时返回 204 No Content（无升级任务）。
func (h *AgentHandler) SetBinarySpecService(svc *service.BinarySpecService) {
	h.binarySpecService = svc
}

func (h *AgentHandler) Register(c *gin.Context) {
	serverCode := c.GetHeader("X-Server-Code")
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	var req model.RegisterRuntimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("register: failed to bind JSON",
			"server_code", serverCode, "error", err)
		server.BadRequest(c, err.Error())
		return
	}

	runtime, err := h.runtimeService.RegisterRuntime(c.Request.Context(), serverCode, &req)
	if err != nil {
		h.logger.Error("register: RegisterRuntime failed",
			"server_code", serverCode,
			"runtime_type", req.RuntimeType,
			"provider_type", req.ProviderType,
			"error", err)
		code, msg := service.MapRuntimeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		h.logger.Error("register: GetServerByCode failed",
			"server_code", serverCode, "error", err)
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	middleware.SetAgentServerID(c, serverSrv.ID)

	server.OK(c, model.AgentResponse{
		ServerID: serverSrv.ID,
		NodeID:   runtime.ID,
	})
}

func (h *AgentHandler) Heartbeat(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	runtimeRef := c.GetHeader("X-Runtime-Ref")
	var runtimeRefPtr *string
	if runtimeRef != "" {
		runtimeRefPtr = &runtimeRef
	}

	var req model.AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if serverSrv == nil {
		server.NotFound(c, "server not found")
		return
	}

	middleware.SetAgentServerID(c, serverSrv.ID)

	if err := h.healthService.ReportHeartbeat(c.Request.Context(), serverCode, runtimeRefPtr, &req); err != nil {
		code, msg := service.MapHealthErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	providerType := model.RuntimeProviderNodeAgent
	rt, err := h.runtimeService.GetRuntimeByServerAndProvider(c.Request.Context(), serverSrv.ID, providerType, runtimeRefPtr)
	if err != nil {
		h.logger.Error("heartbeat: GetRuntimeByServerAndProvider failed",
			"server_code", serverCode,
			"server_id", serverSrv.ID,
			"provider_type", providerType,
			"runtime_ref_ptr", runtimeRefPtr,
			"config_version_current", req.ConfigVersion,
			"error", err)
		server.OK(c, model.HeartbeatResponse{
			Status:      "ok",
			CurrentTime: time.Now().Unix(),
		})
		return
	}
	if rt == nil {
		h.logger.Warn("heartbeat: runtime is nil after lookup",
			"server_code", serverCode,
			"server_id", serverSrv.ID,
			"provider_type", providerType,
			"runtime_ref_ptr", runtimeRefPtr)
	} else {
		h.logger.Info("heartbeat: runtime found",
			"server_code", serverCode,
			"server_id", serverSrv.ID,
			"runtime_id", rt.ID,
			"config_version_current", req.ConfigVersion)
	}

	resp := model.HeartbeatResponse{
		Status:      "ok",
		CurrentTime: time.Now().Unix(),
	}

	if rt != nil && h.deploymentService != nil {
		targetVersion, err := h.deploymentService.GetRuntimeConfig(c.Request.Context(), rt.ID, "")
		if err != nil {
			h.logger.Error("heartbeat: GetRuntimeConfig failed",
				"server_code", serverCode,
				"runtime_id", rt.ID,
				"error", err)
		} else if targetVersion == nil {
			h.logger.Warn("heartbeat: GetRuntimeConfig returned nil",
				"server_code", serverCode,
				"runtime_id", rt.ID)
		} else {
			targetVersionStr := strconv.FormatInt(targetVersion.VersionNo, 10)
			currentVersion := req.ConfigVersion

			needsReload := false
			if currentVersion == "" || currentVersion == "none" {
				needsReload = true
			} else {
				var currentVer int64
				if _, err := fmt.Sscanf(currentVersion, "%d", &currentVer); err == nil {
					if currentVer < targetVersion.VersionNo {
						needsReload = true
					}
				} else {
					needsReload = true
				}
			}

			h.logger.Info("heartbeat: config version check",
				"server_code", serverCode,
				"runtime_id", rt.ID,
				"target_version", targetVersionStr,
				"current_version", currentVersion,
				"needs_reload", needsReload)

			if needsReload {
				configURL := fmt.Sprintf("/api/v1/agent/config?version=%s", targetVersionStr)
				// 实时重算signature，避免cv.ContentJSON被injectNginxVhosts原地修改后ContentHash过时
				signature := pkg.HashContent(targetVersion.ContentJSON)
				action := "reload"
				resp.TargetConfigVersion = &targetVersionStr
				resp.ConfigURL = &configURL
				resp.ConfigSignature = &signature
				resp.Action = &action
				// P1: 节点配置变更时同时触发外部资源同步（nginx vhost/证书），
				// 消除 nginx reconciler 30s 轮询延迟，实现"保存即下发"。
				// agent 收到 sync_external_resources 会立即调用 NginxReconciler.TriggerSync
				resp.ExtraActions = []string{"sync_external_resources"}
			}
		}
	}

	// Inject upgrade action if a newer agent version is available (immediate trigger vs 5min poll).
	// Old agents may not send agent_version via HTTP fallback (JSON tag was "-"), so we always
	// notify when info.json exists — SelfUpgrader on the agent side will do the real version
	// comparison and skip if already up-to-date.
	if info := loadUpgradeInfo(); info != nil {
		ver := req.AgentVersion
		if ver == "" || ver != info.Version {
			action := "upgrade"
			resp.Action = &action
			if ver != "" {
				h.logger.Info("heartbeat: notifying agent of new version",
					"server_code", serverCode,
					"agent_version", ver,
					"target_version", info.Version)
			} else {
				h.logger.Debug("heartbeat: broadcasting upgrade action (agent version unknown/old)",
					"server_code", serverCode,
					"target_version", info.Version)
			}
		}
	}

	server.OK(c, resp)
}

func (h *AgentHandler) FetchConfig(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	runtimeRef := c.GetHeader("X-Runtime-Ref")
	var runtimeRefPtr *string
	if runtimeRef != "" {
		runtimeRefPtr = &runtimeRef
	}

	versionParam := c.Query("version")

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if serverSrv == nil {
		server.NotFound(c, "server not found")
		return
	}

	providerType := model.RuntimeProviderNodeAgent
	rt, err := h.runtimeService.GetRuntimeByServerAndProvider(c.Request.Context(), serverSrv.ID, providerType, runtimeRefPtr)
	if err != nil {
		code, msg := service.MapRuntimeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if rt == nil {
		server.NotFound(c, "runtime not found")
		return
	}

	cv, err := h.deploymentService.GetRuntimeConfig(c.Request.Context(), rt.ID, versionParam)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if cv == nil {
		server.NotFound(c, "config version not found")
		return
	}

	// P3-1: format=payload 时返回加密 PayloadManifest（兼容期双写，默认仍返回明文配置）
	if c.Query("format") == "payload" {
		manifest, err := h.deploymentService.GetPayloadManifest(c.Request.Context(), cv.VersionNo)
		if err != nil {
			code, msg := service.MapDeploymentErrorToCode(err)
			server.Fail(c, code, msg)
			return
		}
		// 实时重算SHA256：cv.ContentJSON是map引用类型，可能被之前的injectNginxVhosts/
		// injectTrafficQuota等调用原地修改，DB中存的SHA256可能与当前内存中的config不一致，
		// 导致agent端签名校验永久nack。实时重算保证下发的hash与agent解密后拿到的config一致。
		if currentHash := pkg.HashContent(cv.ContentJSON); currentHash != "" {
			manifest.SHA256 = currentHash
		}
		server.OK(c, manifest)
		return
	}

	versionStr := strconv.FormatInt(cv.VersionNo, 10)
	// 实时重算signature，不信任DB中的ContentHash（原因同上，map引用可能被原地污染）
	signature := pkg.HashContent(cv.ContentJSON)

	appliedAt := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	if cv.PublishedAt != nil {
		appliedAt = cv.PublishedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	etag := fmt.Sprintf("\"v%d-%s\"", cv.VersionNo, signature)
	if match := c.GetHeader("If-None-Match"); match != "" {
		if strings.HasPrefix(match, "W/") {
			match = match[2:]
		}
		match = strings.Trim(match, "\"")
		if match == fmt.Sprintf("v%d-%s", cv.VersionNo, signature) {
			c.Header("ETag", etag)
			c.Status(http.StatusNotModified)
			return
		}
	}
	c.Header("ETag", etag)
	c.Header("Cache-Control", "no-cache")

	server.OK(c, model.AgentConfigResponse{
		Version:   versionStr,
		Config:    cv.ContentJSON,
		Signature: signature,
		AppliedAt: appliedAt,
	})
}

// CDNVhosts 返回当前 server 的 CDN 节点 nginx vhost snippet。
// 供 node-agent 的 nginx reconciler 独立轮询调用，不依赖 xray config_versions 版本管理。
// node-agent 通过此端点拉取渲染好的 https_snippet/stream_snippet，用独立 hash 追踪变更，
// 完全脱离 version.txt 保护逻辑，实现 nginx vhost 的零 SSH 自动化同步。
// 聚合整台服务器上所有 runtime 的 TLS 节点（双内核模式下 xray+sing-box 共享 nginx）。
func (h *AgentHandler) CDNVhosts(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if serverSrv == nil {
		server.NotFound(c, "server not found")
		return
	}

	vhosts, err := h.deploymentService.BuildNginxVhostsForServer(c.Request.Context(), serverSrv.ID)
	if err != nil {
		h.logger.Error("cdn-vhosts: BuildNginxVhostsForServer failed",
			"server_code", serverCode, "server_id", serverSrv.ID, "error", err)
		server.InternalError(c, "")
		return
	}

	server.OK(c, vhosts)
}

func (h *AgentHandler) ReportConfigResult(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	runtimeRef := c.GetHeader("X-Runtime-Ref")
	var runtimeRefPtr *string
	if runtimeRef != "" {
		runtimeRefPtr = &runtimeRef
	}

	var req model.AgentConfigResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if serverSrv == nil {
		server.NotFound(c, "server not found")
		return
	}

	providerType := model.RuntimeProviderNodeAgent
	rt, err := h.runtimeService.GetRuntimeByServerAndProvider(c.Request.Context(), serverSrv.ID, providerType, runtimeRefPtr)
	if err != nil {
		code, msg := service.MapRuntimeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if rt == nil {
		server.NotFound(c, "runtime not found")
		return
	}

	if err := h.deploymentService.ReportConfigResult(c.Request.Context(), rt.ID, req.Version, req.Success, req.Message); err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"status": "ok"})
}

// FetchPayload P3-1: 返回加密的 PayloadManifest JSON。
// 端点 GET /api/v1/agent/payload?version=N
// Agent 通过此端点拉取加密 Payload，使用共享密钥本地 AES-GCM 解密，
// 实现 TLS 证书等敏感材料的端到端加密下发。
func (h *AgentHandler) FetchPayload(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	versionParam := c.Query("version")
	if versionParam == "" {
		server.BadRequest(c, "version query parameter is required")
		return
	}
	var versionNo int64
	if _, err := fmt.Sscanf(versionParam, "%d", &versionNo); err != nil || versionNo <= 0 {
		server.BadRequest(c, "invalid version parameter")
		return
	}

	manifest, err := h.deploymentService.GetPayloadManifest(c.Request.Context(), versionNo)
	if err != nil {
		h.logger.Error("fetch-payload: GetPayloadManifest failed",
			"server_code", serverCode, "version_no", versionNo, "error", err)
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, manifest)
}

// ReportDeploymentResult P3-1: 接收 Agent 上报的部署 ACK/NACK。
// 端点 POST /api/v1/agent/deployment-result
// Agent 在 precheck / activate / healthcheck 各阶段完成后上报结果，
// 控制面据此推进部署 phase 或触发回滚。
func (h *AgentHandler) ReportDeploymentResult(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	var req model.DeploymentResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	dr, err := h.deploymentService.RecordDeploymentResult(c.Request.Context(), serverCode, &req)
	if err != nil {
		code, msg := service.MapDeploymentErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"status":      dr.Status,
		"version_no":  dr.VersionNo,
		"reported_at": dr.ReportedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ===== D7/D8 修复: Cloudflared Tunnel + Device API =====

// CloudflaredTunnels D7 修复: 返回当前 server 的 cloudflared 隧道配置。
// 端点 GET /api/v1/agent/cloudflared-tunnels
// Agent 的 cloudflared reconciler 定期轮询此端点，获取期望的隧道配置并调和。
func (h *AgentHandler) CloudflaredTunnels(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	runtimeRef := c.GetHeader("X-Runtime-Ref")
	var runtimeRefPtr *string
	if runtimeRef != "" {
		runtimeRefPtr = &runtimeRef
	}

	serverSrv, err := h.serverService.GetServerByCode(c.Request.Context(), serverCode)
	if err != nil {
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if serverSrv == nil {
		server.NotFound(c, "server not found")
		return
	}

	providerType := model.RuntimeProviderNodeAgent
	rt, err := h.runtimeService.GetRuntimeByServerAndProvider(c.Request.Context(), serverSrv.ID, providerType, runtimeRefPtr)
	if err != nil {
		code, msg := service.MapRuntimeErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	if rt == nil {
		server.NotFound(c, "runtime not found")
		return
	}

	// 查询该 runtime 下的所有节点，筛选出 cloudflared 隧道类型节点
	nodes, err := h.deploymentService.GetNodesByRuntimeID(c.Request.Context(), rt.ID)
	if err != nil {
		h.logger.Error("cloudflared-tunnels: GetNodesByRuntimeID failed",
			"server_code", serverCode, "runtime_id", rt.ID, "error", err)
		server.InternalError(c, "")
		return
	}

	type IngressRule struct {
		Hostname     string                 `json:"hostname"`
		Service      string                 `json:"service"`
		OriginRequest map[string]interface{} `json:"originRequest,omitempty"`
	}
	type Tunnel struct {
		Token    string                 `json:"token,omitempty"`
		TunnelID string                 `json:"tunnel_id,omitempty"`
		Ingress  []IngressRule          `json:"ingress,omitempty"`
		Config   map[string]interface{} `json:"config,omitempty"`
	}

	// 同一台 VPS 只能运行一个 cloudflared 进程，因此同一 runtime 下的所有 argo_tunnel 节点
	// 必须合并为一个 Tunnel 对象（共享同一 TunnelID/Token + 合并所有 ingress 规则）。
	// 之前的实现是每个节点单独构造一个 Tunnel，reconciler 只取 Tunnels[0]，
	// 导致多节点场景下 config.yml 只包含第一个节点的 ingress，其余节点无法访问。
	merged := Tunnel{}
	ingressSeen := make(map[string]bool) // hostname 去重，避免同一域名重复写入
	nodeCount := 0
	for _, node := range nodes {
		if node.ConfigJSON == nil {
			continue
		}
		// 只为 CF Tunnel (Argo) 节点下发 cloudflared 配置。
		// cdn_saas 节点走 CF CDN → nginx 443 SSL → xray，不需要 cloudflared。
		// direct 节点走直连，也不需要 cloudflared。
		exposureMode, _ := node.ConfigJSON["exposure_mode"].(string)
		if exposureMode != "argo_tunnel" {
			// 也检查 metadata（兼容旧数据）
			if node.Metadata != nil {
				if em, ok := node.Metadata["exposure_mode"].(string); ok && em == "argo_tunnel" {
					exposureMode = em
				}
			}
		}
		if exposureMode != "argo_tunnel" {
			continue
		}
		nodeCount++

		// Token/TunnelID：同一 VPS 所有 argo_tunnel 节点共用，取第一个非空值即可
		if merged.Token == "" {
			if token, ok := node.ConfigJSON["cloudflared_token"].(string); ok && token != "" {
				merged.Token = token
			}
		}
		if merged.TunnelID == "" {
			if tunnelID, ok := node.ConfigJSON["cloudflared_tunnel_id"].(string); ok && tunnelID != "" {
				merged.TunnelID = tunnelID
			}
		}
		// 构建 ingress 规则
		// 注意：service 必须指向 xray inbound 实际监听端口（ServerPort，VPS 本机），
		// 不能用 node.Port（client_port=443，CDN 对外端口），
		// 否则 cloudflared 会尝试连 443，但 xray 实际监听 ServerPort，导致 502
		if sni := node.SNI; sni != nil && *sni != "" {
			hostname := *sni
			// 同一 hostname 去重（多节点共用同一 SNI 时只写一条 ingress，
			// cloudflared 不允许同一 hostname 多条规则）
			if ingressSeen[hostname] {
				continue
			}
			ingressSeen[hostname] = true
			listenPort := node.Port
			if node.ServerPort != nil && *node.ServerPort > 0 {
				listenPort = *node.ServerPort
			}
			// cloudflared token 模式：明文 HTTP 回源（CF Dashboard 远程配置覆盖本地 config.yml，
		// originRequest 参数不生效，因此必须保持 xray security=none 的 TLS 剥离方案）
		// service 必须用 http://127.0.0.1:<port> 而非 http://localhost:<port>
		// （localhost 解析优先 IPv6 [::1]，若 xray 仅监听 IPv4 127.0.0.1 会被拒）
		service := "http://127.0.0.1:" + strconv.Itoa(listenPort)
		merged.Ingress = append(merged.Ingress, IngressRule{
			Hostname: hostname,
			Service:  service,
		})
		}
	}

	// 只要有 ingress 规则或 token/tunnel_id，就下发隧道配置。
	// 注意：单节点模式下不需要 token/tunnel_id（cloudflared 已通过 credentials-file 配置），
	// 仅靠 ingress 规则即可驱动 reconciler 更新 /etc/cloudflared/config.yml
	tunnels := make([]Tunnel, 0, 1)
	if len(merged.Ingress) > 0 || merged.Token != "" || merged.TunnelID != "" {
		tunnels = append(tunnels, merged)
	}

	h.logger.Debug("cloudflared-tunnels response",
		"server_code", serverCode, "tunnel_count", len(tunnels),
		"argo_node_count", nodeCount, "ingress_rules", len(merged.Ingress))
	server.OK(c, gin.H{"tunnels": tunnels})
}

// ReportDevices D8 修复: 接收 Agent 上报的本节点在线设备 IP 列表。
// 端点 POST /api/v1/agent/devices/report
func (h *AgentHandler) ReportDevices(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	var req struct {
		Devices map[string][]string `json:"devices"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	// 存储到全局设备状态存储
	globalDeviceStore.Update(serverCode, req.Devices)

	h.logger.Debug("devices reported",
		"server_code", serverCode, "user_count", len(req.Devices))
	server.OK(c, gin.H{"status": "ok"})
}

// FetchAliveDevices D8 修复: 返回面板汇总的跨节点全局设备态。
// 端点 GET /api/v1/agent/devices/alive
func (h *AgentHandler) FetchAliveDevices(c *gin.Context) {
	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	// 获取全局设备数（所有节点汇总）
	devices := globalDeviceStore.GetGlobalDeviceCounts()

	h.logger.Debug("alive devices response",
		"server_code", serverCode, "device_count", len(devices))
	server.OK(c, gin.H{"devices": devices})
}

// ===== Binary Spec =====

// BinarySpec P2-4: 返回当前 server 的期望二进制规格。
// 端点 GET /api/v1/agent/binary-spec
func (h *AgentHandler) BinarySpec(c *gin.Context) {
	if h.binarySpecService == nil {
		// 无升级任务，返回 204 No Content
		c.Status(204)
		return
	}

	serverCode := middleware.GetAgentServerCode(c)
	if serverCode == "" {
		server.BadRequest(c, "missing X-Server-Code header")
		return
	}

	spec := h.binarySpecService.GetTarget(serverCode)
	if spec == nil {
		c.Status(204)
		return
	}
	server.OK(c, spec)
}

// ===== Agent Self-Upgrade Endpoints =====

const (
	agentDownloadDir = "/opt/yundu/agent-upgrade"
)

type upgradeCheckResponse struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	ReleaseNote string `json:"release_note"`
	ForceUpdate bool   `json:"force_update"`
}

var (
	upgradeInfoCacheMu sync.RWMutex
	upgradeInfoCache   *upgradeCheckResponse
	upgradeInfoTime    time.Time
)

// loadUpgradeInfo reads /opt/yundu/agent-upgrade/info.json (if exists) and caches it for 10s.
// File format: {"version":"v20260713","download_url":"https://.../node-agent","sha256":"hex","release_note":"...","force_update":true}
// If info.json doesn't exist, returns 204 No Content (no upgrade available).
func loadUpgradeInfo() *upgradeCheckResponse {
	upgradeInfoCacheMu.RLock()
	if upgradeInfoCache != nil && time.Since(upgradeInfoTime) < 10*time.Second {
		defer upgradeInfoCacheMu.RUnlock()
		return upgradeInfoCache
	}
	upgradeInfoCacheMu.RUnlock()

	upgradeInfoCacheMu.Lock()
	defer upgradeInfoCacheMu.Unlock()

	infoPath := filepath.Join(agentDownloadDir, "info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		upgradeInfoCache = nil
		return nil
	}
	var info upgradeCheckResponse
	if err := json.Unmarshal(data, &info); err != nil {
		slog.Error("failed to parse upgrade info.json", "error", err)
		upgradeInfoCache = nil
		return nil
	}
	if info.Version == "" || info.DownloadURL == "" {
		upgradeInfoCache = nil
		return nil
	}
	upgradeInfoCache = &info
	upgradeInfoTime = time.Now()
	return &info
}

// UpgradeCheck is the SelfUpgrader polling endpoint.
// GET /api/v1/agent/upgrade/check
// This endpoint is public (no AgentAuth) because old agents that need upgrading may not
// have valid auth tokens yet in edge cases. The endpoint returns minimal info.
func (h *AgentHandler) UpgradeCheck(c *gin.Context) {
	info := loadUpgradeInfo()
	if info == nil {
		c.Status(http.StatusNoContent)
		return
	}
	agentVer := c.GetHeader("X-Agent-Version")
	if agentVer == info.Version {
		c.Status(http.StatusNotModified)
		return
	}
	c.JSON(http.StatusOK, info)
}

// DownloadAgentBinary serves the node-agent binary for self-upgrade.
// GET /api/v1/agent/download/node-agent-linux-amd64
func (h *AgentHandler) DownloadAgentBinary(c *gin.Context) {
	binPath := filepath.Join(agentDownloadDir, "node-agent-linux-amd64")
	f, err := os.Open(binPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	// Compute sha256 on first request per file (cache by mtime+size)
	hash := cachedSHA256(binPath, stat)

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(stat.Size(), 10))
	c.Header("X-SHA256", hash)
	c.Header("Content-Disposition", "attachment; filename=node-agent-linux-amd64")
	c.Status(http.StatusOK)
	io.Copy(c.Writer, f)
}

var (
	shaCacheMu  sync.RWMutex
	shaCache    = make(map[string]shaCacheEntry)
)

type shaCacheEntry struct {
	mtime time.Time
	size  int64
	hash  string
}

func cachedSHA256(path string, stat os.FileInfo) string {
	key := path
	shaCacheMu.RLock()
	if ent, ok := shaCache[key]; ok && ent.mtime.Equal(stat.ModTime()) && ent.size == stat.Size() {
		defer shaCacheMu.RUnlock()
		return ent.hash
	}
	shaCacheMu.RUnlock()

	shaCacheMu.Lock()
	defer shaCacheMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	hash := hex.EncodeToString(h.Sum(nil))
	shaCache[key] = shaCacheEntry{mtime: stat.ModTime(), size: stat.Size(), hash: hash}
	return hash
}
