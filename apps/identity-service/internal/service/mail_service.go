package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	// ErrMailNotConfigured SMTP 未配置
	ErrMailNotConfigured = errors.New("mail service is not configured")
	// ErrTemplateNotFound 模板不存在
	ErrTemplateNotFound = errors.New("mail template not found")
	// ErrTemplateDisabled 模板已禁用
	ErrTemplateDisabled = errors.New("mail template is disabled")
)

// MailService 邮件模板服务（参考 Xboard MailService）
// 支持从数据库加载模板、内存缓存、变量占位符渲染
type MailService struct {
	repo   *repo.MailTemplateRepo
	logger *slog.Logger

	// 模板内存缓存
	cache   map[string]*model.MailTemplate
	cacheMu sync.RWMutex

	// SMTP 配置（动态更新）
	smtpCfg SMTPConfig
	smtpMu  sync.RWMutex

	// 站点信息（用于模板变量）
	siteName string
	siteURL  string
	siteMu   sync.RWMutex
}

// NewMailService 构造 MailService
func NewMailService(mailRepo *repo.MailTemplateRepo, logger *slog.Logger) *MailService {
	return &MailService{
		repo:     mailRepo,
		logger:   logger,
		cache:    make(map[string]*model.MailTemplate),
		siteName: "YunDu",
		siteURL:  "http://localhost:3000",
	}
}

// IsEnabled 检查邮件服务是否已启用
func (s *MailService) IsEnabled() bool {
	s.smtpMu.RLock()
	defer s.smtpMu.RUnlock()
	return s.smtpCfg.Enabled
}

// UpdateConfig 更新 SMTP 配置（保持向后兼容）
func (s *MailService) UpdateConfig(cfg *SMTPConfig) {
	s.smtpMu.Lock()
	defer s.smtpMu.Unlock()
	if cfg == nil {
		s.smtpCfg.Enabled = false
		return
	}
	s.smtpCfg = *cfg
}

// UpdateSiteInfo 更新站点名称和 URL
func (s *MailService) UpdateSiteInfo(name, url string) {
	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	if name != "" {
		s.siteName = name
	}
	if url != "" {
		s.siteURL = strings.TrimRight(url, "/")
	}
}

// getSMTPConfig 获取当前 SMTP 配置快照
func (s *MailService) getSMTPConfig() SMTPConfig {
	s.smtpMu.RLock()
	defer s.smtpMu.RUnlock()
	return s.smtpCfg
}

// getSiteInfo 获取站点信息快照
func (s *MailService) getSiteInfo() (string, string) {
	s.siteMu.RLock()
	defer s.siteMu.RUnlock()
	return s.siteName, s.siteURL
}

// GetTemplate 获取模板（优先从缓存读取）
func (s *MailService) GetTemplate(name string) (*model.MailTemplate, error) {
	// 优先从缓存读取
	s.cacheMu.RLock()
	if t, ok := s.cache[name]; ok {
		s.cacheMu.RUnlock()
		return t, nil
	}
	s.cacheMu.RUnlock()

	// 缓存未命中，从数据库加载
	if s.repo == nil {
		return nil, ErrTemplateNotFound
	}
	t, err := s.repo.GetByName(context.Background(), name)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}

	// 写入缓存
	s.cacheMu.Lock()
	s.cache[name] = t
	s.cacheMu.Unlock()

	return t, nil
}

// ListTemplates 列出所有模板
func (s *MailService) ListTemplates(ctx context.Context) ([]*model.MailTemplate, error) {
	if s.repo == nil {
		return nil, errors.New("mail template repo not available")
	}
	return s.repo.List(ctx)
}

// UpdateTemplate 更新模板
func (s *MailService) UpdateTemplate(ctx context.Context, id uuid.UUID, subject, body string) (*model.MailTemplate, error) {
	if s.repo == nil {
		return nil, errors.New("mail template repo not available")
	}
	t, err := s.repo.Update(ctx, id, subject, body)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTemplateNotFound
	}

	// 更新缓存
	s.cacheMu.Lock()
	s.cache[t.Name] = t
	s.cacheMu.Unlock()

	return t, nil
}

// SetTemplateEnabled 启用/禁用模板
func (s *MailService) SetTemplateEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	if s.repo == nil {
		return errors.New("mail template repo not available")
	}
	if err := s.repo.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}

	// 清除缓存，下次读取时重新加载
	s.cacheMu.Lock()
	for name, t := range s.cache {
		if t.ID == id {
			t.Enabled = enabled
			s.cache[name] = t
		}
	}
	s.cacheMu.Unlock()

	return nil
}

// ReloadCache 重新加载缓存
func (s *MailService) ReloadCache(ctx context.Context) error {
	if s.repo == nil {
		return errors.New("mail template repo not available")
	}
	templates, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	s.cacheMu.Lock()
	s.cache = make(map[string]*model.MailTemplate, len(templates))
	for _, t := range templates {
		s.cache[t.Name] = t
	}
	s.cacheMu.Unlock()

	s.logger.Info("mail template cache reloaded", "count", len(templates))
	return nil
}

// SendMail 发送邮件（使用模板）
func (s *MailService) SendMail(ctx context.Context, to string, templateName string, data map[string]interface{}) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		s.logger.Warn("smtp not configured, skipping email", "to", to, "template", templateName)
		return nil
	}

	// 获取模板
	t, err := s.GetTemplate(templateName)
	if err != nil {
		return fmt.Errorf("failed to get mail template %s: %w", templateName, err)
	}
	if !t.Enabled {
		return ErrTemplateDisabled
	}

	// 注入默认变量
	siteName, siteURL := s.getSiteInfo()
	if data == nil {
		data = make(map[string]interface{})
	}
	if _, ok := data["SiteName"]; !ok {
		data["SiteName"] = siteName
	}
	if _, ok := data["SiteURL"]; !ok {
		data["SiteURL"] = siteURL
	}

	// 渲染主题和正文
	subject, err := s.renderPlaceholders(t.Subject, data)
	if err != nil {
		return fmt.Errorf("failed to render mail subject: %w", err)
	}
	body, err := s.renderPlaceholders(t.Body, data)
	if err != nil {
		return fmt.Errorf("failed to render mail body: %w", err)
	}

	// 通过 SMTP 发送
	sender := NewSMTPSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From)
	if err := sender.Send(ctx, to, subject, body); err != nil {
		s.logger.Error("failed to send email", "to", to, "subject", subject, "error", err)
		return err
	}

	s.logger.Info("email sent successfully", "to", to, "subject", subject, "template", templateName)
	return nil
}

// renderPlaceholders 替换模板中的变量占位符（使用 html/template 保证 HTML 安全）
// 支持变量：{{.UserName}}, {{.VerifyURL}}, {{.OrderID}}, {{.PlanName}},
// {{.Amount}}, {{.TrafficUsed}}, {{.TrafficTotal}}, {{.ExpireDate}},
// {{.SiteName}}, {{.SiteURL}} 等
func (s *MailService) renderPlaceholders(tpl string, data map[string]interface{}) (string, error) {
	if tpl == "" {
		return "", nil
	}

	t, err := template.New("mail").Parse(tpl)
	if err != nil {
		// 如果模板解析失败，直接返回原始字符串（避免因模板语法错误导致邮件发送失败）
		s.logger.Warn("failed to parse mail template, returning raw text", "error", err)
		return tpl, nil
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		s.logger.Warn("failed to execute mail template, returning raw text", "error", err)
		return tpl, nil
	}

	return buf.String(), nil
}

// ============================================================
// 向后兼容方法：保持现有调用方（user_service, payment_service）不变
// 这些方法内部调用 SendMail，使用对应的内置模板
// ============================================================

// SendVerifyEmail 发送邮箱验证邮件
func (s *MailService) SendVerifyEmail(ctx context.Context, to, token string) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		s.logger.Warn("smtp not configured, skipping verify email", "to", to, "token", token)
		return nil
	}

	siteName, siteURL := s.getSiteInfo()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = siteURL
	}
	verifyURL := fmt.Sprintf("%s/auth/verify-email?token=%s", strings.TrimRight(baseURL, "/"), token)

	data := map[string]interface{}{
		"UserName":  to,
		"VerifyURL": verifyURL,
		"SiteName":  siteName,
		"SiteURL":   siteURL,
	}

	return s.SendMail(ctx, to, model.MailTemplateVerifyEmail, data)
}

// SendResetPassword 发送密码重置邮件
func (s *MailService) SendResetPassword(ctx context.Context, to, token string) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		s.logger.Warn("smtp not configured, skipping reset password email", "to", to, "token", token)
		return nil
	}

	siteName, siteURL := s.getSiteInfo()
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = siteURL
	}
	resetURL := fmt.Sprintf("%s/auth/reset-password?token=%s", strings.TrimRight(baseURL, "/"), token)

	data := map[string]interface{}{
		"UserName":  to,
		"ResetURL":  resetURL,
		"SiteName":  siteName,
		"SiteURL":   siteURL,
	}

	return s.SendMail(ctx, to, model.MailTemplateResetPassword, data)
}

// SendPaymentReceived 发送支付成功通知邮件
func (s *MailService) SendPaymentReceived(ctx context.Context, to, orderNo string, amount float64) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		s.logger.Warn("smtp not configured, skipping payment received email", "to", to, "order", orderNo)
		return nil
	}

	siteName, siteURL := s.getSiteInfo()
	data := map[string]interface{}{
		"UserName": to,
		"OrderID":  orderNo,
		"Amount":   fmt.Sprintf("%.2f", amount),
		"SiteName": siteName,
		"SiteURL":  siteURL,
	}

	return s.SendMail(ctx, to, model.MailTemplatePaymentSuccess, data)
}

// SendTicketReply 发送工单回复通知邮件
func (s *MailService) SendTicketReply(ctx context.Context, to, userName, ticketSubject, replyContent string) error {
	siteName, siteURL := s.getSiteInfo()
	data := map[string]interface{}{
		"UserName":      userName,
		"TicketSubject": ticketSubject,
		"ReplyContent":  replyContent,
		"SiteName":      siteName,
		"SiteURL":       siteURL,
	}
	return s.SendMail(ctx, to, model.MailTemplateTicketReply, data)
}

// SendSubscriptionExpired 发送订阅到期提醒邮件
func (s *MailService) SendSubscriptionExpired(ctx context.Context, to, userName, planName, expireDate string) error {
	siteName, siteURL := s.getSiteInfo()
	data := map[string]interface{}{
		"UserName":  userName,
		"PlanName":  planName,
		"ExpireDate": expireDate,
		"SiteName":  siteName,
		"SiteURL":   siteURL,
	}
	return s.SendMail(ctx, to, model.MailTemplateSubscriptionExpired, data)
}

// SendTrafficWarning 发送流量告警邮件（80% 阈值）
func (s *MailService) SendTrafficWarning(ctx context.Context, to, userName, trafficUsed, trafficTotal string) error {
	siteName, siteURL := s.getSiteInfo()
	data := map[string]interface{}{
		"UserName":     userName,
		"TrafficUsed":  trafficUsed,
		"TrafficTotal": trafficTotal,
		"SiteName":     siteName,
		"SiteURL":      siteURL,
	}
	return s.SendMail(ctx, to, model.MailTemplateTrafficWarning, data)
}

// SendVerifyCode 发送注册验证码邮件。
// 与其它提醒类邮件不同：此处 SMTP 未启用时返回 ErrMailNotConfigured（而非静默跳过），
// 因为注册验证码是强校验流程，发不出验证码就必须让上层拒绝注册。
func (s *MailService) SendVerifyCode(ctx context.Context, to, code string) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		return ErrMailNotConfigured
	}
	siteName, siteURL := s.getSiteInfo()
	data := map[string]interface{}{
		"UserName": to,
		"Code":     code,
		"SiteName": siteName,
		"SiteURL":  siteURL,
	}
	return s.SendMail(ctx, to, model.MailTemplateVerifyCode, data)
}

// SendTestMail 发送测试邮件（管理员使用）
func (s *MailService) SendTestMail(ctx context.Context, to, subject, body string) error {
	cfg := s.getSMTPConfig()
	if !cfg.Enabled {
		return ErrMailNotConfigured
	}

	if subject == "" {
		subject = "YunDu - 测试邮件"
	}
	if body == "" {
		siteName, siteURL := s.getSiteInfo()
		body = fmt.Sprintf(`<html><body><h2>测试邮件</h2><p>这是一封来自 %s 的测试邮件。</p><p>站点地址：<a href="%s">%s</a></p></body></html>`, siteName, siteURL, siteURL)
	}

	sender := NewSMTPSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From)
	return sender.Send(ctx, to, subject, body)
}
