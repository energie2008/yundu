package handler

import (
	"net/http"
	"strconv"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserExtrasHandler struct {
	userSvc       *service.UserService
	ticketSvc     *service.TicketService
	notifySvc     *service.NotificationService
	commissionSvc *service.CommissionService
}

func NewUserExtrasHandler(userSvc *service.UserService, ticketSvc *service.TicketService, notifySvc *service.NotificationService, commissionSvc *service.CommissionService) *UserExtrasHandler {
	return &UserExtrasHandler{userSvc: userSvc, ticketSvc: ticketSvc, notifySvc: notifySvc, commissionSvc: commissionSvc}
}

// GET /plans/:id/nodes  - list nodes for a plan
func (h *UserExtrasHandler) ListPlanNodes(c *gin.Context) {
	idStr := c.Param("id")
	planID, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid plan id")
		return
	}
	nodes, err := h.userSvc.ListPlanNodes(c.Request.Context(), planID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, nodes)
}

// GET /me/nodes - list nodes visible to the current user (based on active subscription plan)
func (h *UserExtrasHandler) ListMyNodes(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	nodes, err := h.userSvc.ListUserNodes(c.Request.Context(), userID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, nodes)
}

// GET /me/tickets/:id - get my ticket detail
func (h *UserExtrasHandler) GetMyTicket(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	idStr := c.Param("id")
	tid, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	t, err := h.ticketSvc.GetUserTicket(c.Request.Context(), userID, tid)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, model.NewTicketResponse(t))
}

// GET /me/tickets/:id/replies - get replies
func (h *UserExtrasHandler) ListMyTicketReplies(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	idStr := c.Param("id")
	tid, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	items, err := h.ticketSvc.ListUserTicketReplies(c.Request.Context(), userID, tid)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"items": items})
}

// POST /me/tickets/:id/replies - add reply
func (h *UserExtrasHandler) AddMyTicketReply(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	idStr := c.Param("id")
	tid, err := uuid.Parse(idStr)
	if err != nil {
		server.ValidationError(c, "invalid ticket id")
		return
	}
	var req model.CreateTicketReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	rp, err := h.ticketSvc.AddUserReply(c.Request.Context(), tid, userID, &req)
	if err != nil {
		server.Fail(c, config.CodeBadRequest, err.Error())
		return
	}
	server.Created(c, model.NewTicketReplyResponse(rp))
}

// GET /me/preferences
func (h *UserExtrasHandler) GetPreferences(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	prefs, err := h.userSvc.GetNotificationPreferences(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, prefs)
}

// PUT /me/preferences
func (h *UserExtrasHandler) UpdatePreferences(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	var req model.NotificationPreferences
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	if err := h.userSvc.UpdateNotificationPreferences(c.Request.Context(), userID, &req); err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, req)
}

// POST /me/commissions/withdraw
func (h *UserExtrasHandler) RequestWithdraw(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	var req model.CreateWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}
	w, err := h.commissionSvc.RequestWithdraw(c.Request.Context(), userID, &req)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, model.NewWithdrawResponse(w))
}

// GET /me/commissions/withdrawals
func (h *UserExtrasHandler) ListMyWithdrawals(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	items, total, err := h.commissionSvc.ListUserWithdrawals(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	resp := make([]model.WithdrawResponse, len(items))
	for i, w := range items {
		resp[i] = model.NewWithdrawResponse(w)
	}
	server.OK(c, model.PaginationResponse{Page: 1, PageSize: len(items), Total: total, Items: resp})
}

// GET /me/commissions/summary
func (h *UserExtrasHandler) CommissionSummary(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	summary, err := h.commissionSvc.GetSummary(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, summary)
}

// GET /me/commissions/details - 佣金明细列表（分页）
func (h *UserExtrasHandler) ListMyCommissionDetails(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
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
	items, total, err := h.commissionSvc.ListCommissionDetails(c.Request.Context(), userID, page, pageSize)
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

// GET /me/invitations - 邀请明细列表（被邀请用户列表，分页）
func (h *UserExtrasHandler) ListMyInvitations(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
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
	items, total, err := h.commissionSvc.ListInvitations(c.Request.Context(), userID, page, pageSize)
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

// POST /me/subscription/token/ensure  - returns a raw token (creates one if none)
func (h *UserExtrasHandler) EnsureSubscriptionToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	ip := c.ClientIP()
	token, rawToken, isNew, err := h.userSvc.EnsureSubscriptionToken(c.Request.Context(), userID, ip)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	resp := model.NewSubscriptionTokenResponse(token)
	resp.Token = rawToken
	server.OK(c, gin.H{"token": resp, "is_new": isNew})
}

// Ensure http package is used
var _ = http.StatusOK

// GET /me/invite-code - get or create invite code for current user
func (h *UserExtrasHandler) GetMyInviteCode(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	code, err := h.commissionSvc.GetOrCreateInviteCode(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, gin.H{"code": code})
}

// GET /me/traffic-logs - get traffic usage logs for current user
func (h *UserExtrasHandler) GetMyTrafficLogs(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}
	logs, err := h.userSvc.GetUserTrafficLogs(c.Request.Context(), userID, days)
	if err != nil {
		server.InternalError(c, err.Error())
		return
	}
	server.OK(c, logs)
}
