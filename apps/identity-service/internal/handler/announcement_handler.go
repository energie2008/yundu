package handler

import (
	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"

	"github.com/google/uuid"
)

type AnnouncementHandler struct {
	announceService *service.AnnouncementService
}

func NewAnnouncementHandler(announceService *service.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{announceService: announceService}
}

// List GET /admin/announcements
func (h *AnnouncementHandler) List(c *gin.Context) {
	var query model.AnnouncementListQuery
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

	items, total, err := h.announceService.List(c.Request.Context(), query)
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

// Get GET /admin/announcements/:id
func (h *AnnouncementHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	resp, err := h.announceService.GetByID(c.Request.Context(), id)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, resp)
}

// Create POST /admin/announcements
func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req model.CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	// announcements.created_by FK references users(id), so prefer UserID
	// (AdminLogin JWT contains both user.ID and admin.ID; user.ID satisfies the FK)
	creatorID := middleware.GetUserID(c)
	if creatorID == uuid.Nil {
		creatorID = middleware.GetAdminID(c)
	}

	a, err := h.announceService.Create(c.Request.Context(), &req, creatorID)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewAnnouncementResponse(a))
}

// Update PATCH /admin/announcements/:id
func (h *AnnouncementHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	var req model.UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	resp, err := h.announceService.Update(c.Request.Context(), id, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, resp)
}

// Publish POST /admin/announcements/:id/publish
func (h *AnnouncementHandler) Publish(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	resp, err := h.announceService.Publish(c.Request.Context(), id)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, resp)
}

// Archive POST /admin/announcements/:id/archive
func (h *AnnouncementHandler) Archive(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	if err := h.announceService.Archive(c.Request.Context(), id); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"archived": true})
}

// Delete DELETE /admin/announcements/:id
func (h *AnnouncementHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	if err := h.announceService.Delete(c.Request.Context(), id); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.NoContent(c)
}

// Stats GET /admin/announcements/stats
func (h *AnnouncementHandler) Stats(c *gin.Context) {
	stats, err := h.announceService.Stats(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, stats)
}

// MarkRead POST /admin/announcements/:id/read
func (h *AnnouncementHandler) MarkRead(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	if err := h.announceService.MarkRead(c.Request.Context(), id, userID); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"read": true})
}


// ListForUser GET /me/announcements — 用户端列表（仅已发布，参考 XBoard notice/fetch）
func (h *AnnouncementHandler) ListForUser(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	page := 1
	pageSize := 20
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			pageSize = n
		}
	}
	items, total, err := h.announceService.ListPublishedForUser(c.Request.Context(), userID, page, pageSize)
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

// GetForUser GET /me/announcements/:id — 用户端详情（自增阅读数）
func (h *AnnouncementHandler) GetForUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid announcement id")
		return
	}
	resp, err := h.announceService.GetByIDAndIncrementView(c.Request.Context(), id)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, resp)
}
