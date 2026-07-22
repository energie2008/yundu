package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/middleware"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	tokenResp, user, err := h.authService.Register(c.Request.Context(), &req, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, gin.H{
		"token": tokenResp,
		"user":  model.NewUserResponse(user),
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	userAgent := c.GetHeader("User-Agent")
	ip := c.ClientIP()
	tokenResp, user, err := h.authService.Login(c.Request.Context(), &req, userAgent, ip)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"token": tokenResp,
		"user":  model.NewUserResponse(user),
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
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

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	tokenResp, err := h.authService.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{"token": tokenResp})
}

func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		server.Unauthorized(c, "")
		return
	}
	user, err := h.authService.GetMe(c.Request.Context(), userID)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if user == nil {
		server.NotFound(c, "user not found")
		return
	}
	server.OK(c, model.NewUserResponse(user))
}
