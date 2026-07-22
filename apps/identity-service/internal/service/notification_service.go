package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

// 通知相关错误
var (
	ErrNotificationNotFound     = errors.New("notification not found")
	ErrNotificationTemplateNotFound = errors.New("notification template not found")
	ErrNotificationInvalidCategory = errors.New("invalid notification category")
	ErrNotificationInvalidChannel  = errors.New("invalid notification channel")
	ErrNotificationInvalidPriority = errors.New("invalid notification priority")
)

// NotificationService 通知业务服务
type NotificationService struct {
	notifyRepo   *repo.NotificationRepo
	templateRepo *repo.NotificationTemplateRepo
	log          *slog.Logger
}

func NewNotificationService(notifyRepo *repo.NotificationRepo, templateRepo *repo.NotificationTemplateRepo) *NotificationService {
	return &NotificationService{
		notifyRepo:   notifyRepo,
		templateRepo: templateRepo,
		log:          slog.Default(),
	}
}

// SetLogger 注入 slog logger，未注入时使用 slog.Default()。
func (s *NotificationService) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.log = logger
	}
}

func validNotificationCategory(s string) bool {
	if s == "" {
		return true
	}
	switch model.NotificationCategory(s) {
	case model.NotificationCategoryTrafficExpiry, model.NotificationCategoryPlanExpiry,
		model.NotificationCategoryNodeChange, model.NotificationCategorySystem,
		model.NotificationCategorySubscription, model.NotificationCategoryBilling,
		model.NotificationCategoryTicket, model.NotificationCategoryAnnouncement:
		return true
	}
	return false
}

func validNotificationChannel(s string) bool {
	if s == "" {
		return true
	}
	switch model.NotificationChannel(s) {
	case model.NotificationChannelInApp, model.NotificationChannelEmail,
		model.NotificationChannelTelegram, model.NotificationChannelBark:
		return true
	}
	return false
}

func validNotificationPriority(s string) bool {
	if s == "" {
		return true
	}
	switch model.NotificationPriority(s) {
	case model.NotificationPriorityLow, model.NotificationPriorityNormal,
		model.NotificationPriorityHigh, model.NotificationPriorityUrgent:
		return true
	}
	return false
}

// Create 创建通知
func (s *NotificationService) Create(ctx context.Context, req *model.CreateNotificationRequest) (*model.Notification, error) {
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, errors.New("invalid user_id")
	}
	if !validNotificationCategory(req.Category) {
		return nil, ErrNotificationInvalidCategory
	}
	if !validNotificationChannel(req.Channel) {
		return nil, ErrNotificationInvalidChannel
	}
	if !validNotificationPriority(req.Priority) {
		return nil, ErrNotificationInvalidPriority
	}

	category := model.NotificationCategorySystem
	if req.Category != "" {
		category = model.NotificationCategory(req.Category)
	}
	channel := model.NotificationChannelInApp
	if req.Channel != "" {
		channel = model.NotificationChannel(req.Channel)
	}
	priority := model.NotificationPriorityNormal
	if req.Priority != "" {
		priority = model.NotificationPriority(req.Priority)
	}

	n := &model.Notification{
		UserID:   uid,
		Category: category,
		Title:    strings.TrimSpace(req.Title),
		Content:  req.Content,
		Channel:  channel,
		Priority: priority,
		// 站内信默认直接 sent
		Status: model.NotificationStatusSent,
	}
	if channel == model.NotificationChannelInApp {
		// 站内信立即标记为 sent
	}

	if err := s.notifyRepo.Create(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

// List 列表（管理员视角）
func (s *NotificationService) List(ctx context.Context, q model.NotificationListQuery) ([]model.NotificationResponse, int, error) {
	items, total, err := s.notifyRepo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.NotificationResponse, 0, len(items))
	for _, n := range items {
		result = append(result, model.NewNotificationResponse(n))
	}
	return result, total, nil
}

// ListUserNotifications 用户视角列表（排除已归档，过滤下沉到 SQL）
func (s *NotificationService) ListUserNotifications(ctx context.Context, userID uuid.UUID, page, pageSize int, category string) ([]model.NotificationResponse, int, error) {
	q := model.NotificationListQuery{
		Page:           page,
		PageSize:       pageSize,
		UserID:         userID.String(),
		Category:       category,
		ExcludeArchived: true,
	}
	items, total, err := s.notifyRepo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.NotificationResponse, 0, len(items))
	for _, n := range items {
		result = append(result, model.NewNotificationResponse(n))
	}
	return result, total, nil
}

// MarkRead 单条标记已读
func (s *NotificationService) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	n, err := s.notifyRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if n == nil || n.UserID != userID {
		return ErrNotificationNotFound
	}
	return s.notifyRepo.MarkRead(ctx, id)
}

// MarkAllRead 标记用户全部已读
func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.notifyRepo.MarkAllRead(ctx, userID)
}

// Archive 归档
func (s *NotificationService) Archive(ctx context.Context, id, userID uuid.UUID) error {
	n, err := s.notifyRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if n == nil || n.UserID != userID {
		return ErrNotificationNotFound
	}
	return s.notifyRepo.Archive(ctx, id)
}

// UnreadCount 未读数
func (s *NotificationService) UnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.notifyRepo.UnreadCount(ctx, userID)
}

// ===== 管理端便捷方法（不校验用户归属，由 RBAC 权限保护） =====

// GetByID 获取单条通知详情（管理端）
func (s *NotificationService) GetByID(ctx context.Context, id uuid.UUID) (*model.NotificationResponse, error) {
	n, err := s.notifyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, ErrNotificationNotFound
	}
	resp := model.NewNotificationResponse(n)
	return &resp, nil
}

// AdminDelete 删除通知（管理端）
func (s *NotificationService) AdminDelete(ctx context.Context, id uuid.UUID) error {
	return s.notifyRepo.Delete(ctx, id)
}

// AdminMarkRead 管理端标记单条通知已读（不校验用户归属）
func (s *NotificationService) AdminMarkRead(ctx context.Context, id uuid.UUID) error {
	return s.notifyRepo.MarkRead(ctx, id)
}

// AdminArchive 管理端归档单条通知（不校验用户归属）
func (s *NotificationService) AdminArchive(ctx context.Context, id uuid.UUID) error {
	return s.notifyRepo.Archive(ctx, id)
}

// SendByTemplate 按模板发送通知（系统调用）
func (s *NotificationService) SendByTemplate(ctx context.Context, code string, userID uuid.UUID, vars map[string]interface{}) (*model.Notification, error) {
	t, err := s.templateRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrNotificationTemplateNotFound
	}
	if !t.Enabled {
		return nil, fmt.Errorf("template %s is disabled", code)
	}

	title, body, err := renderTemplate(t.TitleTemplate, t.BodyTemplate, vars)
	if err != nil {
		return nil, err
	}

	n := &model.Notification{
		UserID:       userID,
		Category:     t.Category,
		Title:        title,
		Content:      body,
		Channel:      t.Channel,
		Priority:     model.NotificationPriorityNormal,
		Status:       model.NotificationStatusSent,
		TemplateCode: &t.Code,
	}
	if err := s.notifyRepo.Create(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

// NotifyUser 便捷方法：按模板 code 给指定用户发送站内信通知。
// metadata 会同时作为模板变量和写入通知的 metadata（JSON）。
// 模板不存在或被禁用时静默跳过（仅记录日志），不返回错误，避免阻塞业务主流程。
func (s *NotificationService) NotifyUser(ctx context.Context, userID uuid.UUID, templateCode string, metadata map[string]interface{}) {
	t, err := s.templateRepo.GetByCode(ctx, templateCode)
	if err != nil {
		s.log.Warn("notify: load template failed", "code", templateCode, "error", err)
		return
	}
	if t == nil {
		s.log.Warn("notify: template not found, skip", "code", templateCode)
		return
	}
	if !t.Enabled {
		s.log.Info("notify: template disabled, skip", "code", templateCode)
		return
	}

	title, body, err := renderTemplate(t.TitleTemplate, t.BodyTemplate, metadata)
	if err != nil {
		s.log.Warn("notify: render template failed", "code", templateCode, "error", err)
		return
	}

	metaJSON, _ := json.Marshal(metadata)
	n := &model.Notification{
		UserID:       userID,
		Category:     t.Category,
		Title:        title,
		Content:      body,
		Channel:      t.Channel,
		Priority:     model.NotificationPriorityNormal,
		Status:       model.NotificationStatusSent,
		TemplateCode: &t.Code,
		Metadata:     metaJSON,
	}
	if err := s.notifyRepo.Create(ctx, n); err != nil {
		s.log.Warn("notify: create notification failed", "code", templateCode, "user", userID, "error", err)
		return
	}
}

// NotifyUserAsync 异步发送通知（goroutine），确保不阻塞业务主流程。
// 使用独立 context.Background()，避免请求 context 取消后通知丢失。
func (s *NotificationService) NotifyUserAsync(userID uuid.UUID, templateCode string, metadata map[string]interface{}) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("notify: async NotifyUser panic", "code", templateCode, "user", userID, "panic", r)
			}
		}()
		s.NotifyUser(context.Background(), userID, templateCode, metadata)
	}()
}

// BroadcastByTemplate 按模板向所有活跃用户广播通知（用于公告发布）。
// 返回受影响的用户数。模板不存在/禁用时返回 (0, nil)。
func (s *NotificationService) BroadcastByTemplate(ctx context.Context, templateCode string, vars map[string]interface{}) (int64, error) {
	t, err := s.templateRepo.GetByCode(ctx, templateCode)
	if err != nil {
		return 0, err
	}
	if t == nil {
		s.log.Warn("broadcast: template not found, skip", "code", templateCode)
		return 0, nil
	}
	if !t.Enabled {
		s.log.Info("broadcast: template disabled, skip", "code", templateCode)
		return 0, nil
	}
	title, body, err := renderTemplate(t.TitleTemplate, t.BodyTemplate, vars)
	if err != nil {
		return 0, err
	}
	metaJSON, _ := json.Marshal(vars)
	n := &model.Notification{
		Category:     t.Category,
		Title:        title,
		Content:      body,
		Channel:      t.Channel,
		Priority:     model.NotificationPriorityNormal,
		Status:       model.NotificationStatusSent,
		TemplateCode: &t.Code,
		Metadata:     metaJSON,
	}
	affected, err := s.notifyRepo.BatchCreateForAllUsers(ctx, n)
	if err != nil {
		return 0, err
	}
	s.log.Info("broadcast: notification sent to users", "code", templateCode, "affected", affected)
	return affected, nil
}

// BroadcastByTemplateAsync 异步广播通知（goroutine），不阻塞业务主流程。
func (s *NotificationService) BroadcastByTemplateAsync(templateCode string, vars map[string]interface{}) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("broadcast: async panic", "code", templateCode, "panic", r)
			}
		}()
		_, _ = s.BroadcastByTemplate(context.Background(), templateCode, vars)
	}()
}

// ProcessScheduledNotifications 处理已到调度时间的 pending 通知（定时任务调用）。
// 将到点的 scheduled 通知标记为 sent（站内信即对用户可见）。
func (s *NotificationService) ProcessScheduledNotifications(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("ProcessScheduledNotifications panic", "error", r)
		}
	}()
	items, err := s.notifyRepo.ListPendingScheduled(ctx, 100)
	if err != nil {
		s.log.Error("scheduled: list pending notifications failed", "error", err)
		return err
	}
	if len(items) == 0 {
		return nil
	}
	s.log.Info("scheduled: processing pending notifications", "count", len(items))
	for _, n := range items {
		if err := s.notifyRepo.MarkSent(ctx, n.ID); err != nil {
			s.log.Warn("scheduled: mark sent failed", "id", n.ID, "error", err)
			continue
		}
	}
	return nil
}

// RecentNotificationExists 判断指定用户在 since 之后是否已收到过某模板的通知（用于提醒去重）。
func (s *NotificationService) RecentNotificationExists(ctx context.Context, userID uuid.UUID, templateCode string, since time.Time) (bool, error) {
	return s.notifyRepo.ExistsByUserTemplateSince(ctx, userID, templateCode, since)
}

// renderTemplate 渲染 Go 模板
func renderTemplate(titleTpl, bodyTpl string, vars map[string]interface{}) (string, string, error) {
	title, err := renderOne("title", titleTpl, vars)
	if err != nil {
		return "", "", err
	}
	body, err := renderOne("body", bodyTpl, vars)
	if err != nil {
		return "", "", err
	}
	return title, body, nil
}

func renderOne(name, tpl string, vars map[string]interface{}) (string, error) {
	if tpl == "" {
		return "", nil
	}
	t, err := template.New(name).Parse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, vars); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// ===== 模板管理 =====

// ListTemplates 列出所有模板
func (s *NotificationService) ListTemplates(ctx context.Context) ([]model.NotificationTemplateResponse, error) {
	items, err := s.templateRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]model.NotificationTemplateResponse, 0, len(items))
	for _, t := range items {
		result = append(result, model.NewNotificationTemplateResponse(t))
	}
	return result, nil
}

// GetTemplateByCode 按 code 获取模板
func (s *NotificationService) GetTemplateByCode(ctx context.Context, code string) (*model.NotificationTemplateResponse, error) {
	t, err := s.templateRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrNotificationTemplateNotFound
	}
	resp := model.NewNotificationTemplateResponse(t)
	return &resp, nil
}

// UpsertTemplate 创建或更新模板
func (s *NotificationService) UpsertTemplate(ctx context.Context, t *model.NotificationTemplate) (*model.NotificationTemplateResponse, error) {
	if t.Code == "" {
		return nil, errors.New("template code is required")
	}
	if t.Name == "" {
		return nil, errors.New("template name is required")
	}
	if !validNotificationCategory(string(t.Category)) {
		return nil, ErrNotificationInvalidCategory
	}
	if !validNotificationChannel(string(t.Channel)) {
		return nil, ErrNotificationInvalidChannel
	}

	if err := s.templateRepo.Upsert(ctx, t); err != nil {
		return nil, err
	}
	resp := model.NewNotificationTemplateResponse(t)
	return &resp, nil
}

// SetTemplateEnabled 启停模板
func (s *NotificationService) SetTemplateEnabled(ctx context.Context, code string, enabled bool) error {
	return s.templateRepo.SetEnabled(ctx, code, enabled)
}

// DeleteTemplate 删除模板
func (s *NotificationService) DeleteTemplate(ctx context.Context, code string) error {
	return s.templateRepo.Delete(ctx, code)
}
