package model

import (
	"time"

	"github.com/google/uuid"
)

// MailTemplate 邮件模板（参考 Xboard v2_mail_templates）
// 支持 6 种内置模板：verify_email, reset_password, payment_success,
// ticket_reply, subscription_expired, traffic_warning
type MailTemplate struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`            // 模板标识
	Subject   string    `json:"subject" db:"subject"`      // 邮件主题（支持变量占位符）
	Body      string    `json:"body" db:"body"`            // 邮件正文（HTML，支持变量占位符）
	IsBuiltin bool      `json:"is_builtin" db:"is_builtin"` // 内置模板不可删除
	Enabled   bool      `json:"enabled" db:"enabled"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// MailTemplateResponse 邮件模板响应 DTO
type MailTemplateResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	IsBuiltin bool      `json:"is_builtin"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewMailTemplateResponse 构造响应
func NewMailTemplateResponse(t *MailTemplate) MailTemplateResponse {
	return MailTemplateResponse{
		ID:        t.ID,
		Name:      t.Name,
		Subject:   t.Subject,
		Body:      t.Body,
		IsBuiltin: t.IsBuiltin,
		Enabled:   t.Enabled,
		UpdatedAt: t.UpdatedAt,
	}
}

// UpdateMailTemplateRequest 更新邮件模板请求
type UpdateMailTemplateRequest struct {
	Subject string `json:"subject" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

// SendTestMailRequest 发送测试邮件请求
type SendTestMailRequest struct {
	To      string `json:"to" binding:"required,email"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// AdminSendMailRequest 管理员手动发送邮件请求
type AdminSendMailRequest struct {
	To           string                 `json:"to" binding:"required,email"`
	Subject      string                 `json:"subject" binding:"required"`
	Body         string                 `json:"body" binding:"required"`
	TemplateName string                 `json:"template_name,omitempty"`
	Data         map[string]interface{} `json:"data,omitempty"`
}

// VerifyEmailRequest 邮箱验证请求（POST）
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// 邮件模板名称常量
const (
	MailTemplateVerifyEmail          = "verify_email"
	MailTemplateResetPassword        = "reset_password"
	MailTemplatePaymentSuccess       = "payment_success"
	MailTemplateTicketReply          = "ticket_reply"
	MailTemplateSubscriptionExpired  = "subscription_expired"
	MailTemplateTrafficWarning       = "traffic_warning"
	MailTemplateVerifyCode           = "verify_code"
)
