package handler

import (
	"log/slog"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
)

// AgentBootstrapHandler 处理 Agent 零配置部署的 Bootstrap 请求。
//
// 端点 GET /api/v1/agent/bootstrap?token=xxx
// 该端点不需要认证中间件（因为 agent 还没有 server_code / HMAC 头），
// 仅通过 token 参数验证身份。Agent 首次启动时调用一次，获取完整运行时配置。
type AgentBootstrapHandler struct {
	bootstrapService *service.AgentBootstrapService
	logger           *slog.Logger
}

// NewAgentBootstrapHandler 构造 Bootstrap handler。
func NewAgentBootstrapHandler(bootstrapService *service.AgentBootstrapService) *AgentBootstrapHandler {
	return &AgentBootstrapHandler{
		bootstrapService: bootstrapService,
		logger:           slog.Default().With("component", "agent-bootstrap-handler"),
	}
}

// Bootstrap 处理 GET /api/v1/agent/bootstrap?token=xxx 请求。
//
// 逻辑：
//  1. 从 query param 获取 token
//  2. 调用 AgentBootstrapService.GetBootstrapConfig
//  3. 返回 BootstrapConfig JSON
func (h *AgentBootstrapHandler) Bootstrap(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		server.BadRequest(c, "missing token query parameter")
		return
	}

	cfg, err := h.bootstrapService.GetBootstrapConfig(c.Request.Context(), token)
	if err != nil {
		h.logger.Warn("bootstrap failed", "error", err)
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, cfg)
}

// MachineNodes 处理 GET /api/v1/agent/machine/nodes?server_token=xxx 请求。
//
// 用于 Machine 模式（--mode machine）：返回该服务器上所有需要 Agent 托管的节点列表。
// MachineOrchestrator 定期调用此 API 发现新增节点、移除已删除节点。
//
// 认证方式与 Bootstrap 相同：通过 server_token 查询参数验证身份，
// 不需要 AgentAuth 中间件（因为 Machine Agent 使用的是 server 级 token，不是单个节点的 token）。
func (h *AgentBootstrapHandler) MachineNodes(c *gin.Context) {
	token := c.Query("server_token")
	if token == "" {
		server.BadRequest(c, "missing server_token query parameter")
		return
	}

	resp, err := h.bootstrapService.GetMachineNodes(c.Request.Context(), token)
	if err != nil {
		h.logger.Warn("machine nodes failed", "error", err)
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

// MachineCDNVhosts 处理 GET /api/v1/agent/machine/cdn-vhosts?server_token=xxx 请求。
//
// 用于 Machine 模式（--mode machine）：返回该服务器上所有节点聚合后的 nginx vhost 配置。
// MachineOrchestrator 的 nginx reconciler 定期调用此 API 同步整台机器的 nginx 配置。
//
// 认证方式与 Bootstrap 相同：通过 server_token 查询参数验证身份，
// 不需要 AgentAuth 中间件（因为 Machine Agent 使用的是 server 级 token，不是单个节点的 token）。
func (h *AgentBootstrapHandler) MachineCDNVhosts(c *gin.Context) {
	token := c.Query("server_token")
	if token == "" {
		server.BadRequest(c, "missing server_token query parameter")
		return
	}

	resp, err := h.bootstrapService.MachineCDNVhosts(c.Request.Context(), token)
	if err != nil {
		h.logger.Warn("machine cdn vhosts failed", "error", err)
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

// MachineCloudflaredTunnels 处理 GET /api/v1/agent/machine/cloudflared-tunnels?server_token=xxx 请求。
//
// T05: 用于 Machine 模式（--mode machine）：返回该服务器上所有节点的 cloudflared 隧道配置聚合。
// MachineOrchestrator 的 cloudflared reconciler 定期调用此 API 同步整台机器的隧道配置。
//
// 认证方式与 MachineCDNVhosts 相同：通过 server_token 查询参数验证身份。
func (h *AgentBootstrapHandler) MachineCloudflaredTunnels(c *gin.Context) {
	token := c.Query("server_token")
	if token == "" {
		server.BadRequest(c, "missing server_token query parameter")
		return
	}

	resp, err := h.bootstrapService.MachineCloudflaredTunnels(c.Request.Context(), token)
	if err != nil {
		h.logger.Warn("machine cloudflared-tunnels failed", "error", err)
		code, msg := service.MapServerErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}
