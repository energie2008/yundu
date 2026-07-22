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

// 工单相关错误
var (
	ErrTicketNotFound     = errors.New("ticket not found")
	ErrTicketClosed        = errors.New("ticket is closed, cannot reply")
	ErrTicketInvalidStatus = errors.New("invalid ticket status")
	ErrTicketInvalidCategory = errors.New("invalid ticket category")
	ErrTicketInvalidPriority = errors.New("invalid ticket priority")
)

// TicketService 工单业务服务
type TicketService struct {
	ticketRepo *repo.TicketRepo
	userRepo   *repo.UserRepo
	notifySvc  *NotificationService
}

func NewTicketService(ticketRepo *repo.TicketRepo, userRepo *repo.UserRepo, notifySvc *NotificationService) *TicketService {
	return &TicketService{ticketRepo: ticketRepo, userRepo: userRepo, notifySvc: notifySvc}
}

// GetUserIDByEmail 通过邮箱查询用户ID（参考 XBoard admin ticket create by email）
func (s *TicketService) GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	if s.userRepo == nil {
		return uuid.Nil, errors.New("user repo not available")
	}
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, errors.New("user not found")
	}
	return u.ID, nil
}

func validTicketStatus(s string) bool {
	switch model.TicketStatus(s) {
	case model.TicketStatusOpen, model.TicketStatusInProgress, model.TicketStatusResolved, model.TicketStatusClosed:
		return true
	}
	return false
}

func validTicketCategory(s string) bool {
	if s == "" {
		return true
	}
	switch model.TicketCategory(s) {
	case model.TicketCategoryConsultation, model.TicketCategoryFault, model.TicketCategoryComplaint,
		model.TicketCategoryRefund, model.TicketCategoryOther:
		return true
	}
	return false
}

func validTicketPriority(s string) bool {
	if s == "" {
		return true
	}
	switch model.TicketPriority(s) {
	case model.TicketPriorityLow, model.TicketPriorityNormal, model.TicketPriorityHigh, model.TicketPriorityUrgent:
		return true
	}
	return false
}

// CreateTicket 用户创建工单（user 端）
func (s *TicketService) CreateTicket(ctx context.Context, userID uuid.UUID, req *model.CreateTicketRequest) (*model.Ticket, error) {
	if !validTicketCategory(req.Category) {
		return nil, ErrTicketInvalidCategory
	}
	if !validTicketPriority(req.Priority) {
		return nil, ErrTicketInvalidPriority
	}

	category := model.TicketCategoryConsultation
	if req.Category != "" {
		category = model.TicketCategory(req.Category)
	}
	priority := model.TicketPriorityNormal
	if req.Priority != "" {
		priority = model.TicketPriority(req.Priority)
	}

	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = strings.TrimSpace(req.Message)
	}

	t := &model.Ticket{
		UserID:      userID,
		Subject:     strings.TrimSpace(req.Subject),
		Description: desc,
		Category:    category,
		Priority:    priority,
		Status:      model.TicketStatusOpen,
	}
	if req.RelatedResourceType != "" {
		rt := req.RelatedResourceType
		t.RelatedResourceType = &rt
	}
	if req.RelatedResourceID != "" {
		if rid, err := uuid.Parse(req.RelatedResourceID); err == nil {
			t.RelatedResourceID = &rid
		}
	}

	if err := s.ticketRepo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// AdminCreateTicket 管理员代客创建工单
func (s *TicketService) AdminCreateTicket(ctx context.Context, userID uuid.UUID, req *model.CreateTicketRequest) (*model.Ticket, error) {
	return s.CreateTicket(ctx, userID, req)
}

// ListTickets 列表（管理员）
func (s *TicketService) ListTickets(ctx context.Context, q model.TicketListQuery) ([]model.TicketResponse, int, error) {
	items, total, err := s.ticketRepo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	// 批量查询用户邮箱（参考 XBoard ticket list join users）
	userIDs := make([]uuid.UUID, 0, len(items))
	for _, t := range items {
		userIDs = append(userIDs, t.UserID)
	}
	userMap := make(map[uuid.UUID]*model.User)
	if s.userRepo != nil && len(userIDs) > 0 {
		userMap, _ = s.userRepo.GetByIDs(ctx, userIDs)
	}
	result := make([]model.TicketResponse, 0, len(items))
	for _, t := range items {
		resp := model.NewTicketResponse(t)
		if u, ok := userMap[t.UserID]; ok {
			resp.User = &model.TicketUserSummary{
				ID:    u.ID,
				Email: u.Email,
			}
		}
		result = append(result, resp)
	}
	return result, total, nil
}

// ListUserTickets 用户视角列表
func (s *TicketService) ListUserTickets(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.TicketResponse, int, error) {
	q := model.TicketListQuery{
		Page:     page,
		PageSize: pageSize,
		UserID:   userID.String(),
	}
	return s.ListTickets(ctx, q)
}

// GetTicket 获取单个
func (s *TicketService) GetTicket(ctx context.Context, id uuid.UUID) (*model.TicketResponse, error) {
	t, err := s.ticketRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTicketNotFound
	}
	resp := model.NewTicketResponse(t)
	// 填充用户邮箱
	if s.userRepo != nil {
		if u, err := s.userRepo.GetByID(ctx, t.UserID); err == nil && u != nil {
			resp.User = &model.TicketUserSummary{
				ID:    u.ID,
				Email: u.Email,
			}
		}
	}
	return &resp, nil
}

// UpdateTicket 管理员更新工单（状态/优先级/分配）
func (s *TicketService) UpdateTicket(ctx context.Context, id uuid.UUID, req *model.UpdateTicketRequest) (*model.TicketResponse, error) {
	t, err := s.ticketRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTicketNotFound
	}

	fields := map[string]interface{}{}
	if req.Status != nil {
		if !validTicketStatus(*req.Status) {
			return nil, ErrTicketInvalidStatus
		}
		fields["status"] = *req.Status
	}
	if req.Priority != nil {
		if !validTicketPriority(*req.Priority) {
			return nil, ErrTicketInvalidPriority
		}
		fields["priority"] = *req.Priority
	}
	if req.Category != nil {
		if !validTicketCategory(*req.Category) {
			return nil, ErrTicketInvalidCategory
		}
		fields["category"] = *req.Category
	}
	if req.AssignedAdminID != nil {
		aid, err := uuid.Parse(*req.AssignedAdminID)
		if err != nil {
			return nil, ErrTicketInvalidStatus
		}
		fields["assigned_admin_id"] = aid
	}

	// 状态为 closed/resolved 时调用 UpdateStatus 会自动设置 closed_at
	if statusStr, ok := fields["status"].(string); ok {
		if err := s.ticketRepo.UpdateStatus(ctx, id, model.TicketStatus(statusStr)); err != nil {
			return nil, err
		}
		// 移除 status 避免 UpdateFields 重复处理
		delete(fields, "status")
	}

	if err := s.ticketRepo.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}

	t, err = s.ticketRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := model.NewTicketResponse(t)
	return &resp, nil
}

// AssignToAdmin 分配工单给管理员
func (s *TicketService) AssignToAdmin(ctx context.Context, ticketID, adminID uuid.UUID) error {
	return s.ticketRepo.AssignAdmin(ctx, ticketID, adminID)
}

// AddReply 添加回复
func (s *TicketService) AddReply(ctx context.Context, ticketID, authorID uuid.UUID, authorType model.AuthorType, req *model.CreateTicketReplyRequest) (*model.TicketReply, error) {
	t, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTicketNotFound
	}
	if t.Status == model.TicketStatusClosed {
		return nil, ErrTicketClosed
	}

	rp := &model.TicketReply{
		TicketID:   ticketID,
		AuthorID:   authorID,
		AuthorType: authorType,
		Content:    req.Content,
		IsInternal:  req.IsInternal,
	}
	if err := s.ticketRepo.CreateReply(ctx, rp); err != nil {
		return nil, err
	}

	// 更新回复计数与最后回复时间
	_ = s.ticketRepo.IncrementReplyCount(ctx, ticketID)

	// 若管理员回复且工单状态是 open，自动转为 in_progress
	if authorType == model.AuthorTypeAdmin && t.Status == model.TicketStatusOpen {
		_ = s.ticketRepo.UpdateStatus(ctx, ticketID, model.TicketStatusInProgress)
	}

	// 管理员回复（且非内部备注）时，异步通知工单用户
	if authorType == model.AuthorTypeAdmin && !req.IsInternal && s.notifySvc != nil {
		s.notifySvc.NotifyUserAsync(t.UserID, "ticket_replied", map[string]interface{}{
			"ticket_id":      t.ID.String(),
			"ticket_subject": t.Subject,
		})
	}

	return rp, nil
}

// ListReplies 获取工单回复
func (s *TicketService) ListReplies(ctx context.Context, ticketID uuid.UUID) ([]model.TicketReplyResponse, error) {
	items, err := s.ticketRepo.ListReplies(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	result := make([]model.TicketReplyResponse, 0, len(items))
	for _, r := range items {
		result = append(result, model.NewTicketReplyResponse(r))
	}
	return result, nil
}

// StatsByStatus 按状态统计
func (s *TicketService) StatsByStatus(ctx context.Context) (map[string]int, error) {
	// 通过 List 拿全部然后聚合（简化实现）
	stats := map[string]int{
		"open": 0, "in_progress": 0, "resolved": 0, "closed": 0,
	}
	for _, status := range []string{"open", "in_progress", "resolved", "closed"} {
		q := model.TicketListQuery{Status: status, PageSize: 1}
		_, total, err := s.ticketRepo.List(ctx, q)
		if err != nil {
			return nil, err
		}
		stats[status] = total
	}
	return stats, nil
}

// GetUserTicket 获取用户自己的工单（验证所有权）
func (s *TicketService) GetUserTicket(ctx context.Context, userID, ticketID uuid.UUID) (*model.Ticket, error) {
	t, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTicketNotFound
	}
	if t.UserID != userID {
		return nil, ErrTicketNotFound
	}
	return t, nil
}

// ListUserTicketReplies 获取用户工单的回复（过滤内部回复）
func (s *TicketService) ListUserTicketReplies(ctx context.Context, userID, ticketID uuid.UUID) ([]*model.TicketReply, error) {
	t, err := s.GetUserTicket(ctx, userID, ticketID)
	if err != nil {
		return nil, err
	}
	replies, err := s.ticketRepo.ListReplies(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	var filtered []*model.TicketReply
	for _, r := range replies {
		if !r.IsInternal {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// AddUserReply 用户添加工单回复
func (s *TicketService) AddUserReply(ctx context.Context, ticketID, userID uuid.UUID, req *model.CreateTicketReplyRequest) (*model.TicketReply, error) {
	t, err := s.GetUserTicket(ctx, userID, ticketID)
	if err != nil {
		return nil, err
	}
	userReq := &model.CreateTicketReplyRequest{
		Content:    req.Content,
		IsInternal: false,
	}
	return s.AddReply(ctx, t.ID, userID, model.AuthorTypeUser, userReq)
}

// Now 方便测试
func Now() time.Time { return time.Now() }
