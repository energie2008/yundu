package handler

import (
	"log/slog"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/service"
	"github.com/gin-gonic/gin"
)

// 阶段3: 零 SSH 完整性 — CF Tunnel 管理 API
//
// 暴露给前端 admin 调用，实现"新建 argo_tunnel 节点时自动创建 CF Tunnel + DNS"。
//
// 端点：
//   POST /api/v1/admin/cf-tunnels
//   Body: {"hostname": "douyincdn88.yundu.space", "tunnel_name": "vps81-douyincdn88"}
//   Resp: {"tunnel_id": "...", "token": "...", "hostname": "...", "dns_record": "..."}
//
// 前端拿到 token 后，在节点保存时写入 config_json.cloudflared_token，
// agent 拉取后自动启动 cloudflared（token 模式），无需 SSH。

// CFTunnelHandler CF Tunnel 管理 API
type CFTunnelHandler struct {
	cfService *service.CFTunnelService
	logger    *slog.Logger
}

// NewCFTunnelHandler 创建 handler
func NewCFTunnelHandler(cfService *service.CFTunnelService, logger *slog.Logger) *CFTunnelHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CFTunnelHandler{
		cfService: cfService,
		logger:    logger,
	}
}

// CreateTunnel POST /api/v1/admin/cf-tunnels
// 自动创建 CF Tunnel + DNS CNAME，返回 token 供节点 config_json 使用
func (h *CFTunnelHandler) CreateTunnel(c *gin.Context) {
	if h.cfService == nil || !h.cfService.IsConfigured() {
		server.Fail(c, config.CodeServiceUnavailable,
			"CF API not configured. Set CF_API_TOKEN and CF_ACCOUNT_ID env vars on the panel server.")
		return
	}

	var req service.CreateTunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	if req.Hostname == "" {
		server.BadRequest(c, "hostname is required")
		return
	}

	h.logger.Info("admin create CF tunnel request",
		"hostname", req.Hostname, "tunnel_name", req.TunnelName)

	result, err := h.cfService.CreateTunnelWithDNS(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("create CF tunnel failed",
			"hostname", req.Hostname, "error", err)
		server.Fail(c, config.CodeInternalError, err.Error())
		return
	}

	h.logger.Info("CF tunnel created via admin API",
		"tunnel_id", result.TunnelID, "hostname", result.Hostname)

	server.OK(c, result)
}

// CheckConfigured GET /api/v1/admin/cf-tunnels/status
// 返回 CF API 配置状态（前端用于显示"自动创建 Tunnel"按钮是否可用）
func (h *CFTunnelHandler) CheckConfigured(c *gin.Context) {
	configured := h.cfService != nil && h.cfService.IsConfigured()
	server.OK(c, gin.H{
		"configured": configured,
		"message": gin.H{
			"hint": "若要启用零 SSH Tunnel 自动创建，请在面板服务器配置环境变量 CF_API_TOKEN（需要 Tunnel:Edit + DNS:Edit 权限）、CF_ACCOUNT_ID、CF_ZONE_ID",
		},
	})
}
