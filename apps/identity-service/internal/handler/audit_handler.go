package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	auditService *service.AuditService
}

func NewAuditHandler(auditService *service.AuditService) *AuditHandler {
	return &AuditHandler{auditService: auditService}
}

func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	var query model.AuditLogListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	page := query.Page
	if page < 1 {
		page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if pageSize < 1 {
			pageSize = 20
		}
	}

	logs, total, err := h.auditService.ListAuditLogs(
		c.Request.Context(), page, pageSize,
		query.ActorType, query.ActorID, query.ResourceType, query.ResourceID, query.Action,
	)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    logs,
	})
}
