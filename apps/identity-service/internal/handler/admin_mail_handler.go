package handler

import (
	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminMailHandler 邮件模板管理 Handler（管理员）
type AdminMailHandler struct {
	mailSvc *service.MailService
}

// NewAdminMailHandler 构造 AdminMailHandler
func NewAdminMailHandler(mailSvc *service.MailService) *AdminMailHandler {
	return &AdminMailHandler{mailSvc: mailSvc}
}

// ListTemplates GET /api/v1/admin/mail/templates - 列出所有模板
func (h *AdminMailHandler) ListTemplates(c *gin.Context) {
	templates, err := h.mailSvc.ListTemplates(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}

	resp := make([]model.MailTemplateResponse, 0, len(templates))
	for _, t := range templates {
		resp = append(resp, model.NewMailTemplateResponse(t))
	}

	server.OK(c, gin.H{"items": resp})
}

// UpdateTemplate PUT /api/v1/admin/mail/templates/:id - 更新模板
func (h *AdminMailHandler) UpdateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid template id")
		return
	}

	var req model.UpdateMailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	t, err := h.mailSvc.UpdateTemplate(c.Request.Context(), id, req.Subject, req.Body)
	if err != nil {
		if err == service.ErrTemplateNotFound {
			server.NotFound(c, "mail template not found")
			return
		}
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.NewMailTemplateResponse(t))
}

// SendTestMail POST /api/v1/admin/mail/test - 发送测试邮件
func (h *AdminMailHandler) SendTestMail(c *gin.Context) {
	var req model.SendTestMailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.mailSvc.SendTestMail(c.Request.Context(), req.To, req.Subject, req.Body); err != nil {
		if err == service.ErrMailNotConfigured {
			server.Fail(c, config.CodeForbidden, "mail service is not configured")
			return
		}
		server.Fail(c, config.CodeInternalError, "failed to send test email: "+err.Error())
		return
	}

	server.OK(c, gin.H{"sent": true, "to": req.To})
}

// SendMail POST /api/v1/admin/mail/send - 手动发送邮件（管理员）
func (h *AdminMailHandler) SendMail(c *gin.Context) {
	var req model.AdminSendMailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	// 如果指定了模板名称，使用模板发送；否则直接发送 subject + body
	if req.TemplateName != "" {
		if err := h.mailSvc.SendMail(c.Request.Context(), req.To, req.TemplateName, req.Data); err != nil {
			if err == service.ErrMailNotConfigured || err == service.ErrTemplateDisabled {
				server.Fail(c, config.CodeForbidden, err.Error())
				return
			}
			if err == service.ErrTemplateNotFound {
				server.NotFound(c, "mail template not found")
				return
			}
			server.Fail(c, config.CodeInternalError, "failed to send email: "+err.Error())
			return
		}
	} else {
		if err := h.mailSvc.SendTestMail(c.Request.Context(), req.To, req.Subject, req.Body); err != nil {
			if err == service.ErrMailNotConfigured {
				server.Fail(c, config.CodeForbidden, "mail service is not configured")
				return
			}
			server.Fail(c, config.CodeInternalError, "failed to send email: "+err.Error())
			return
		}
	}

	server.OK(c, gin.H{"sent": true, "to": req.To})
}

// ReloadCache POST /api/v1/admin/mail/templates/reload - 重新加载模板缓存
func (h *AdminMailHandler) ReloadCache(c *gin.Context) {
	if err := h.mailSvc.ReloadCache(c.Request.Context()); err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"reloaded": true})
}
