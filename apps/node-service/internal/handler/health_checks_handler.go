package handler

import (
	"log/slog"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/gin-gonic/gin"
)

// HealthChecksHandler 把维护指南的 SQL 巡检封装为只读 admin API，
// 让维护者在面板即可完成体检，无需 SSH + psql。
type HealthChecksHandler struct {
	nodeRepo *repo.NodeRepo
	logger   *slog.Logger
}

func NewHealthChecksHandler(nodeRepo *repo.NodeRepo, logger *slog.Logger) *HealthChecksHandler {
	return &HealthChecksHandler{nodeRepo: nodeRepo, logger: logger}
}

func (h *HealthChecksHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	g := admin.Group("/health-checks")
	{
		g.GET("/summary", rbac.RequirePermission("nodes.read"), h.Summary)
	}
}

func (h *HealthChecksHandler) Summary(c *gin.Context) {
	s, err := h.nodeRepo.HealthCheckSummary(c.Request.Context())
	if err != nil {
		h.logger.Warn("health-checks: summary failed", "error", err)
		server.InternalError(c, "")
		return
	}
	server.OK(c, s)
}
