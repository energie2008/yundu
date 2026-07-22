package handler

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminHandler struct {
	adminService *service.AdminService
	userService  *service.UserService
}

func NewAdminHandler(adminService *service.AdminService, userService *service.UserService) *AdminHandler {
	return &AdminHandler{
		adminService: adminService,
		userService:  userService,
	}
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	var query model.UserListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	users, total, err := h.adminService.ListUsers(c.Request.Context(), page, pageSize, query.Status, query.Search)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    users,
	})
}

func (h *AdminHandler) CreateAdmin(c *gin.Context) {
	var req model.CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	admin, user, err := h.adminService.CreateAdmin(c.Request.Context(), &req)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, model.NewAdminResponse(admin, user.Email))
}

func (h *AdminHandler) ListAdmins(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	admins, total, err := h.adminService.ListAdmins(c.Request.Context(), page, pageSize)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    admins,
	})
}

func (h *AdminHandler) AdminListUsers(c *gin.Context) {
	var query model.AdminUserListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	users, total, err := h.userService.AdminListUsers(c.Request.Context(), page, pageSize, query.Status, query.Search)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.PaginationResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		Items:    users,
	})
}

func (h *AdminHandler) AdminCreateUser(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	var req model.AdminCreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	user, err := h.userService.AdminCreateUser(c.Request.Context(), adminID, adminEmail, &req, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	_, profile, sub, err := h.userService.AdminGetUser(c.Request.Context(), user.ID)
	if err != nil {
		server.Created(c, model.NewUserDetailResponse(user, nil, nil))
		return
	}

	server.Created(c, model.NewUserDetailResponse(user, profile, sub))
}

func (h *AdminHandler) AdminGetUser(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	user, profile, sub, err := h.userService.AdminGetUser(c.Request.Context(), userID)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.NewUserDetailResponse(user, profile, sub))
}

func (h *AdminHandler) AdminUpdateUser(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	var req model.AdminUpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminUpdateUser(c.Request.Context(), adminID, adminEmail, userID, &req, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	user, profile, sub, err := h.userService.AdminGetUser(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	server.OK(c, model.NewUserDetailResponse(user, profile, sub))
}

func (h *AdminHandler) AdminBanUser(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	var req model.BanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminBanUser(c.Request.Context(), adminID, adminEmail, userID, req.Reason, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminUnbanUser(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminUnbanUser(c.Request.Context(), adminID, adminEmail, userID, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminResetPassword(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	newPassword, err := h.userService.AdminResetPassword(c.Request.Context(), adminID, adminEmail, userID, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, model.ResetPasswordResponse{NewPassword: newPassword})
}

func (h *AdminHandler) AdminResetTraffic(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminResetTraffic(c.Request.Context(), adminID, adminEmail, userID, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminAddTraffic(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	var req model.AddTrafficRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminAddTraffic(c.Request.Context(), adminID, adminEmail, userID, req.Bytes, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminExtendSubscription(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	var req model.ExtendSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminExtendSubscription(c.Request.Context(), adminID, adminEmail, userID, req.Days, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminChangePlan(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	var req model.ChangePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminChangePlan(c.Request.Context(), adminID, adminEmail, userID, req.PlanID, req.Immediate, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminResetSubscriptionTokens(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminResetSubscriptionTokens(c.Request.Context(), adminID, adminEmail, userID, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminDeleteUser(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminSoftDeleteUser(c.Request.Context(), adminID, adminEmail, userID, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminBatchBan(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	var req struct {
		UserIDs []uuid.UUID `json:"user_ids" binding:"required,min=1"`
		Reason  string      `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminBatchBan(c.Request.Context(), adminID, adminEmail, req.UserIDs, req.Reason, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminBatchUnban(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	var req model.BatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminBatchUnban(c.Request.Context(), adminID, adminEmail, req.UserIDs, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminBatchResetTraffic(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	var req model.BatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminBatchResetTraffic(c.Request.Context(), adminID, adminEmail, req.UserIDs, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminBatchDelete(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	var req model.BatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userService.AdminBatchDelete(c.Request.Context(), adminID, adminEmail, req.UserIDs, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, nil)
}

func (h *AdminHandler) AdminImpersonate(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	adminEmail := middleware.GetAdminEmail(c)

	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		server.ValidationError(c, "invalid user id")
		return
	}

	ip := c.ClientIP()
	token, err := h.userService.AdminCreateImpersonateToken(c.Request.Context(), adminID, adminEmail, userID, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"impersonate_token": token})
}
