package handler

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AgentUpgradeManifest 是 Agent selfUpgrader 轮询 /agent/upgrade/check 返回的版本规格，
// 与 node-agent 的 upgrader.VersionInfo 对齐。
type AgentUpgradeManifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256,omitempty"`
	ReleaseNote string `json:"release_note,omitempty"`
	ForceUpdate bool   `json:"force_update,omitempty"`
}

// AgentUpgradeHandler 管理面板上的 Agent 版本库（/opt/yundu/agent-upgrade/info.json）。
// 通过 admin API 更新版本规格后，所有节点 selfUpgrader 自动升级，无需 SSH 上传二进制
// （download_url 可直连 GitHub Release 资产）。
type AgentUpgradeHandler struct {
	logger *slog.Logger
	dir    string
}

func NewAgentUpgradeHandler(logger *slog.Logger) *AgentUpgradeHandler {
	return &AgentUpgradeHandler{
		logger: logger,
		dir:    getEnvDefault("AGENT_UPGRADE_DIR", "/opt/yundu/agent-upgrade"),
	}
}

func (h *AgentUpgradeHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	g := admin.Group("/agent-upgrade")
	{
		g.GET("/manifest", rbac.RequirePermission("nodes.read"), h.GetManifest)
		g.PUT("/manifest", rbac.RequirePermission("nodes.write"), h.PutManifest)
		g.DELETE("/manifest", rbac.RequirePermission("nodes.write"), h.DeleteManifest)
	}
}

// GetManifest 返回当前 Agent 升级版本规格；未配置时返回空对象。
func (h *AgentUpgradeHandler) GetManifest(c *gin.Context) {
	info, err := h.readManifest()
	if err != nil {
		h.logger.Warn("agent-upgrade: read manifest failed", "error", err)
		server.InternalError(c, "")
		return
	}
	if info == nil {
		server.OK(c, gin.H{})
		return
	}
	server.OK(c, info)
}

// PutManifest 原子更新 info.json 并触发 Agent 心跳升级通知（下次心跳即返回 upgrade action）。
func (h *AgentUpgradeHandler) PutManifest(c *gin.Context) {
	var req AgentUpgradeManifest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, "invalid manifest: "+err.Error())
		return
	}
	if req.Version == "" || req.DownloadURL == "" {
		server.BadRequest(c, "version and download_url are required")
		return
	}
	if err := h.writeManifest(&req); err != nil {
		h.logger.Error("agent-upgrade: write manifest failed", "error", err)
		server.InternalError(c, "")
		return
	}
	h.logger.Info("agent upgrade manifest updated",
		"version", req.Version, "download_url", req.DownloadURL, "force", req.ForceUpdate)
	server.OK(c, req)
}

// DeleteManifest 删除版本规格，停止 Agent 自动升级通知。
func (h *AgentUpgradeHandler) DeleteManifest(c *gin.Context) {
	path := filepath.Join(h.dir, "info.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		h.logger.Warn("agent-upgrade: delete manifest failed", "error", err)
		server.InternalError(c, "")
		return
	}
	h.logger.Info("agent upgrade manifest deleted")
	server.OK(c, gin.H{"deleted": true})
}

func (h *AgentUpgradeHandler) readManifest() (*AgentUpgradeManifest, error) {
	path := filepath.Join(h.dir, "info.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var info AgentUpgradeManifest
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (h *AgentUpgradeHandler) writeManifest(info *AgentUpgradeManifest) error {
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(h.dir, "info.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
