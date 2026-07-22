package handler

import (
	"strconv"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type NotificationHandler struct {
	notifyService *service.NotificationService
}

func NewNotificationHandler(notifyService *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notifyService: notifyService}
}

// List GET /admin/notifications
func (h *NotificationHandler) List(c *gin.Context) {
	var query model.NotificationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}

	items, total, err := h.notifyService.List(c.Request.Context(), query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
		Items:    items,
	})
}

// Create POST /admin/notifications
func (h *NotificationHandler) Create(c *gin.Context) {
	var req model.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	n, err := h.notifyService.Create(c.Request.Context(), &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewNotificationResponse(n))
}

// GetByID GET /admin/notifications/:id - 获取单条通知详情
func (h *NotificationHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	resp, err := h.notifyService.GetByID(c.Request.Context(), id)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, resp)
}

// Delete DELETE /admin/notifications/:id - 删除通知
func (h *NotificationHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	if err := h.notifyService.AdminDelete(c.Request.Context(), id); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"deleted": true})
}

// AdminMarkRead POST /admin/notifications/:id/read - 标记已读
func (h *NotificationHandler) AdminMarkRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	if err := h.notifyService.AdminMarkRead(c.Request.Context(), id); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"read": true})
}

// AdminArchive POST /admin/notifications/:id/archive - 归档通知
func (h *NotificationHandler) AdminArchive(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	if err := h.notifyService.AdminArchive(c.Request.Context(), id); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"archived": true})
}

// ===== 模板管理 =====

// ListTemplates GET /admin/notification-templates
func (h *NotificationHandler) ListTemplates(c *gin.Context) {
	items, err := h.notifyService.ListTemplates(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": items})
}

// GetTemplate GET /admin/notification-templates/:code
func (h *NotificationHandler) GetTemplate(c *gin.Context) {
	code := c.Param("code")
	resp, err := h.notifyService.GetTemplateByCode(c.Request.Context(), code)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, resp)
}

// UpsertTemplate PUT /admin/notification-templates/:code
func (h *NotificationHandler) UpsertTemplate(c *gin.Context) {
	code := c.Param("code")
	var t model.NotificationTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	t.Code = code

	resp, err := h.notifyService.UpsertTemplate(c.Request.Context(), &t)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, resp)
}

// SetTemplateEnabled PATCH /admin/notification-templates/:code/enabled
func (h *NotificationHandler) SetTemplateEnabled(c *gin.Context) {
	code := c.Param("code")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if err := h.notifyService.SetTemplateEnabled(c.Request.Context(), code, body.Enabled); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"enabled": body.Enabled})
}

// DeleteTemplate DELETE /admin/notification-templates/:code
func (h *NotificationHandler) DeleteTemplate(c *gin.Context) {
	code := c.Param("code")
	if err := h.notifyService.DeleteTemplate(c.Request.Context(), code); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.NoContent(c)
}

// ===== 用户端（/me/notifications） =====

// ListMyNotifications GET /me/notifications
func (h *NotificationHandler) ListMyNotifications(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	category := c.Query("category")

	items, total, err := h.notifyService.ListUserNotifications(c.Request.Context(), userID, page, pageSize, category)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    items,
	})
}

// MarkMyRead POST /me/notifications/:id/read
func (h *NotificationHandler) MarkMyRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	if err := h.notifyService.MarkRead(c.Request.Context(), id, userID); err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, gin.H{"read": true})
}

// MarkAllMyRead POST /me/notifications/read-all
func (h *NotificationHandler) MarkAllMyRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	count, err := h.notifyService.MarkAllRead(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"read_count": count})
}

// ArchiveMy POST /me/notifications/:id/archive
func (h *NotificationHandler) ArchiveMy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid notification id")
		return
	}
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	if err := h.notifyService.Archive(c.Request.Context(), id, userID); err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, gin.H{"archived": true})
}

// UnreadCount GET /me/notifications/unread-count
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	count, err := h.notifyService.UnreadCount(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"unread_count": count})
}
