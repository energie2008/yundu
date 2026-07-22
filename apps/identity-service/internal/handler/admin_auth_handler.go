package handler

import (
	"log/slog"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminAuthHandler struct {
	authService *service.AuthService
}

func NewAdminAuthHandler(authService *service.AuthService) *AdminAuthHandler {
	return &AdminAuthHandler{authService: authService}
}

func (h *AdminAuthHandler) Login(c *gin.Context) {
	var req model.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	userAgent := c.GetHeader("User-Agent")
	ip := c.ClientIP()
	tokenResp, admin, user, err := h.authService.AdminLogin(c.Request.Context(), &req, userAgent, ip)
	if err != nil {
		slog.Error("admin login failed", "email", req.Email, "error", err)
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"token": tokenResp,
		"admin": model.NewAdminResponse(admin, user.Email),
	})
}

func (h *AdminAuthHandler) Logout(c *gin.Context) {
	sessionID := middleware.GetSessionID(c)
	if sessionID == uuid.Nil {
		server.OK(c, nil)
		return
	}
	if err := h.authService.Logout(c.Request.Context(), sessionID); err != nil {
		server.InternalError(c, "")
		return
	}
	server.OK(c, nil)
}

func (h *AdminAuthHandler) GetMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	admin, user, err := h.authService.GetAdminMe(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if admin == nil {
		server.NotFound(c, "admin not found")
		return
	}
	server.OK(c, model.NewAdminResponse(admin, user.Email))
}
