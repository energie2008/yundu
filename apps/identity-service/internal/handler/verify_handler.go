package handler

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
)

// VerifyHandler 邮箱验证相关 Handler
// 提供 POST 接口用于邮箱验证、忘记密码、重置密码
// 邮件发送使用模板系统（通过 UserService → MailService → SMTPSender）
type VerifyHandler struct {
	userSvc *service.UserService
}

// NewVerifyHandler 构造 VerifyHandler
func NewVerifyHandler(userSvc *service.UserService) *VerifyHandler {
	return &VerifyHandler{userSvc: userSvc}
}

// VerifyEmail POST /api/v1/auth/verify-email - 邮箱验证
// 接收 token 参数，验证邮箱地址
func (h *VerifyHandler) VerifyEmail(c *gin.Context) {
	var req model.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.userSvc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"verified": true,
		"message":  "email verified successfully",
	})
}

// ForgotPassword POST /api/v1/auth/forgot-password - 忘记密码（发送重置邮件）
// 接收 email 参数，生成重置令牌并通过邮件模板发送重置链接
func (h *VerifyHandler) ForgotPassword(c *gin.Context) {
	var req model.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.userSvc.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	// 出于安全考虑，无论邮箱是否存在都返回相同消息
	server.OK(c, gin.H{
		"message": "if the email exists, a reset link has been sent",
	})
}

// ResetPassword POST /api/v1/auth/reset-password - 重置密码
// 接收 token 和 new_password 参数，验证令牌并重置密码
func (h *VerifyHandler) ResetPassword(c *gin.Context) {
	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	if err := h.userSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"message": "password reset successful",
	})
}

// SendEmailCode POST /api/v1/auth/send-email-code - 发送注册验证码
// 接收 email 参数，生成 6 位数字验证码并通过 SMTP 发送（参考 Xboard CommController::sendEmailVerify）。
// 防刷：60s 冷却 + 每邮箱每日 5 次上限。已注册邮箱静默跳过发送但仍返回成功（防邮箱探测）。
// 无论邮箱是否已注册、发送是否真正执行，统一返回相同消息以避免枚举攻击。
func (h *VerifyHandler) SendEmailCode(c *gin.Context) {
	var req model.SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.ValidationError(c, err.Error())
		return
	}

	ip := c.ClientIP()
	if err := h.userSvc.SendEmailCode(c.Request.Context(), req.Email, ip); err != nil {
		code, msg := service.MapErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	// 出于安全考虑，无论邮箱是否已注册都返回相同消息
	server.OK(c, gin.H{
		"message": "if the email is valid, a verification code has been sent",
	})
}
