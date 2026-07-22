package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

// 公告相关错误
var (
	ErrAnnouncementNotFound  = errors.New("announcement not found")
	ErrAnnouncementArchived   = errors.New("announcement is archived")
	ErrAnnouncementInvalidType = errors.New("invalid announcement type")
	ErrAnnouncementInvalidStatus = errors.New("invalid announcement status")
	ErrAnnouncementInvalidAudience = errors.New("invalid target audience")
)

// AnnouncementService 公告业务服务
type AnnouncementService struct {
	announceRepo *repo.AnnouncementRepo
	notifySvc    *NotificationService
}

func NewAnnouncementService(announceRepo *repo.AnnouncementRepo, notifySvc *NotificationService) *AnnouncementService {
	return &AnnouncementService{announceRepo: announceRepo, notifySvc: notifySvc}
}

func validAnnouncementType(s string) bool {
	switch model.AnnouncementType(s) {
	case model.AnnouncementTypeMaintenance, model.AnnouncementTypeUpdate,
		model.AnnouncementTypeNotice, model.AnnouncementTypeAlert:
		return true
	}
	return false
}

func validAnnouncementStatus(s string) bool {
	switch model.AnnouncementStatus(s) {
	case model.AnnouncementStatusDraft, model.AnnouncementStatusPublished, model.AnnouncementStatusArchived:
		return true
	}
	return false
}

func validAnnouncementAudience(s string) bool {
	switch model.AnnouncementAudience(s) {
	case model.AnnouncementAudienceAll, model.AnnouncementAudienceUser, model.AnnouncementAudienceAdmin,
		model.AnnouncementAudienceSpecific, model.AnnouncementAudiencePlanBased:
		return true
	}
	return false
}

// Create 创建公告
func (s *AnnouncementService) Create(ctx context.Context, req *model.CreateAnnouncementRequest, creatorID uuid.UUID) (*model.Announcement, error) {
	aType := model.AnnouncementTypeNotice
	if req.Type != "" {
		if !validAnnouncementType(req.Type) {
			return nil, ErrAnnouncementInvalidType
		}
		aType = model.AnnouncementType(req.Type)
	}
	audience := model.AnnouncementAudienceAll
	if req.TargetAudience != "" {
		if !validAnnouncementAudience(req.TargetAudience) {
			return nil, ErrAnnouncementInvalidAudience
		}
		audience = model.AnnouncementAudience(req.TargetAudience)
	}

	a := &model.Announcement{
		Title:          strings.TrimSpace(req.Title),
		Content:        req.Content,
		Type:           aType,
		Status:         model.AnnouncementStatusDraft,
		TargetAudience: audience,
		Pinned:         req.Pinned,
		EffectiveAt:    req.EffectiveAt,
		ExpiresAt:      req.ExpiresAt,
		CreatedBy:      &creatorID,
	}
	if req.Summary != "" {
		s := strings.TrimSpace(req.Summary)
		a.Summary = &s
	}

	if err := s.announceRepo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// List 列表（管理员）
func (s *AnnouncementService) List(ctx context.Context, q model.AnnouncementListQuery) ([]model.AnnouncementResponse, int, error) {
	items, total, err := s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.AnnouncementResponse, 0, len(items))
	for _, a := range items {
		result = append(result, model.NewAnnouncementResponse(a))
	}
	return result, total, nil
}

// GetByID 获取单个（管理员）
func (s *AnnouncementService) GetByID(ctx context.Context, id uuid.UUID) (*model.AnnouncementResponse, error) {
	a, err := s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAnnouncementNotFound
	}
	resp := model.NewAnnouncementResponse(a)
	return &resp, nil
}

// GetByIDAndIncrementView 用户视角查看（自增阅读数）
func (s *AnnouncementService) GetByIDAndIncrementView(ctx context.Context, id uuid.UUID) (*model.AnnouncementResponse, error) {
	a, err := s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAnnouncementNotFound
	}
	_ = s.announceRepo.IncViewCount(ctx, id)
	a.ViewCount++
	resp := model.NewAnnouncementResponse(a)
	return &resp, nil
}

// Update 更新公告
func (s *AnnouncementService) Update(ctx context.Context, id uuid.UUID, req *model.UpdateAnnouncementRequest) (*model.AnnouncementResponse, error) {
	a, err := s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAnnouncementNotFound
	}

	fields := map[string]interface{}{}
	if req.Title != nil {
		fields["title"] = *req.Title
	}
	if req.Content != nil {
		fields["content"] = *req.Content
	}
	if req.Summary != nil {
		fields["summary"] = *req.Summary
	}
	if req.Type != nil {
		if !validAnnouncementType(*req.Type) {
			return nil, ErrAnnouncementInvalidType
		}
		fields["type"] = *req.Type
	}
	if req.Status != nil {
		if !validAnnouncementStatus(*req.Status) {
			return nil, ErrAnnouncementInvalidStatus
		}
		fields["status"] = *req.Status
	}
	if req.Pinned != nil {
		fields["pinned"] = *req.Pinned
	}

	if err := s.announceRepo.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}

	a, err = s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := model.NewAnnouncementResponse(a)
	return &resp, nil
}

// Publish 发布公告
func (s *AnnouncementService) Publish(ctx context.Context, id uuid.UUID) (*model.AnnouncementResponse, error) {
	a, err := s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAnnouncementNotFound
	}
	if a.Status == model.AnnouncementStatusArchived {
		return nil, ErrAnnouncementArchived
	}
	fields := map[string]interface{}{"status": string(model.AnnouncementStatusPublished)}
	if err := s.announceRepo.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}
	a, err = s.announceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 公告发布后异步广播站内信通知给所有用户
	if s.notifySvc != nil {
		summary := ""
		if a.Summary != nil {
			summary = *a.Summary
		}
		s.notifySvc.BroadcastByTemplateAsync("announcement_published", map[string]interface{}{
			"announcement_title":   a.Title,
			"announcement_summary": summary,
		})
	}

	resp := model.NewAnnouncementResponse(a)
	return &resp, nil
}

// Archive 归档
func (s *AnnouncementService) Archive(ctx context.Context, id uuid.UUID) error {
	fields := map[string]interface{}{"status": string(model.AnnouncementStatusArchived)}
	return s.announceRepo.UpdateFields(ctx, id, fields)
}

// Delete 软删除
func (s *AnnouncementService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.announceRepo.Delete(ctx, id)
}

// MarkRead 标记已读
func (s *AnnouncementService) MarkRead(ctx context.Context, announcementID, userID uuid.UUID) error {
	return s.announceRepo.MarkRead(ctx, announcementID, userID)
}

// ListPublishedForUser 列出用户可见的已发布公告
func (s *AnnouncementService) ListPublishedForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.AnnouncementResponse, int, error) {
	items, total, err := s.announceRepo.ListPublishedForUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.AnnouncementResponse, 0, len(items))
	for _, a := range items {
		result = append(result, model.NewAnnouncementResponse(a))
	}
	return result, total, nil
}

// Stats 简单统计
type AnnouncementStats struct {
	Total     int `json:"total"`
	Published int `json:"published"`
	Draft     int `json:"draft"`
	Archived  int `json:"archived"`
	Pinned    int `json:"pinned"`
}

func (s *AnnouncementService) Stats(ctx context.Context) (*AnnouncementStats, error) {
	stats := &AnnouncementStats{}
	q := model.AnnouncementListQuery{PageSize: 1}
	// total
	_, total, err := s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	stats.Total = total

	q.Status = "published"
	_, stats.Published, err = s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}

	q.Status = "draft"
	_, stats.Draft, err = s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}

	q.Status = "archived"
	_, stats.Archived, err = s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}

	q.Status = ""
	pinnedTrue := true
	q.Pinned = &pinnedTrue
	_, stats.Pinned, err = s.announceRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// suppress unused time import warning if future date utilities needed
var _ = time.Now
