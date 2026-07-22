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

type TicketHandler struct {
	ticketService *service.TicketService
}

func NewTicketHandler(ticketService *service.TicketService) *TicketHandler {
	return &TicketHandler{ticketService: ticketService}
}

// ListTickets GET /admin/tickets
func (h *TicketHandler) ListTickets(c *gin.Context) {
	var query model.TicketListQuery
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

	items, total, err := h.ticketService.ListTickets(c.Request.Context(), query)
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

// GetTicket GET /admin/tickets/:id
func (h *TicketHandler) GetTicket(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}

	resp, err := h.ticketService.GetTicket(c.Request.Context(), id)
	if err != nil {
		server.NotFound(c, err.Error())
		return
	}
	server.OK(c, resp)
}

// UpdateTicket PATCH /admin/tickets/:id
func (h *TicketHandler) UpdateTicket(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	var req model.UpdateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	resp, err := h.ticketService.UpdateTicket(c.Request.Context(), id, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.OK(c, resp)
}

// AssignTicket POST /admin/tickets/:id/assign
func (h *TicketHandler) AssignTicket(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	adminID := middleware.GetAdminID(c)
	if adminID == uuid.Nil {
		adminID = middleware.GetUserID(c)
	}
	if err := h.ticketService.AssignToAdmin(c.Request.Context(), id, adminID); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, gin.H{"assigned_to": adminID})
}

// AddReply POST /admin/tickets/:id/replies
func (h *TicketHandler) AddReply(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	var req model.CreateTicketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	authorID := middleware.GetUserID(c)
	if authorID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}

	rp, err := h.ticketService.AddReply(c.Request.Context(), id, authorID, model.AuthorTypeAdmin, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewTicketReplyResponse(rp))
}

// ListReplies GET /admin/tickets/:id/replies
func (h *TicketHandler) ListReplies(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}

	items, err := h.ticketService.ListReplies(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": items})
}

// Stats GET /admin/tickets/stats
func (h *TicketHandler) Stats(c *gin.Context) {
	stats, err := h.ticketService.StatsByStatus(c.Request.Context())
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, stats)
}

// AdminCreateTicket POST /admin/tickets （管理员代客创建工单）
// 支持 user_id 或 email 查询参数（参考 XBoard admin ticket create）
func (h *TicketHandler) AdminCreateTicket(c *gin.Context) {
	var req model.CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	userIDStr := c.Query("user_id")
	email := c.Query("email")
	var uid uuid.UUID
	if userIDStr != "" {
		parsed, err := uuid.Parse(userIDStr)
		if err != nil {
			server.ValidationError(c, "invalid user_id query param")
			return
		}
		uid = parsed
	} else if email != "" {
		resolved, err := h.ticketService.GetUserIDByEmail(c.Request.Context(), email)
		if err != nil {
			server.ValidationError(c, "user not found by email: "+err.Error())
			return
		}
		uid = resolved
	} else {
		server.ValidationError(c, "user_id or email query param required")
		return
	}
	t, err := h.ticketService.AdminCreateTicket(c.Request.Context(), uid, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewTicketResponse(t))
}

// ===== 用户端（暂时仅给 admin-web 预留 /me/tickets 路径） =====

// ListMyTickets GET /me/tickets
func (h *TicketHandler) ListMyTickets(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	items, total, err := h.ticketService.ListUserTickets(c.Request.Context(), userID, page, pageSize)
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

// CreateMyTicket POST /me/tickets
func (h *TicketHandler) CreateMyTicket(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Forbidden(c, "unauthorized")
		return
	}
	var req model.CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	t, err := h.ticketService.CreateTicket(c.Request.Context(), userID, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewTicketResponse(t))
}
