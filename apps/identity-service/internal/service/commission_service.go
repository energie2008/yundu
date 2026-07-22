package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

var (
	ErrWithdrawDisabled    = errors.New("withdraw is disabled")
	ErrWithdrawMinAmount   = errors.New("amount below minimum withdraw")
	ErrInsufficientBalance = errors.New("insufficient commission balance")
)

// 佣金日志状态
const (
	commissionStatusPending  = 0 // 待结算（订单已完成但未过确认期）
	commissionStatusSettled  = 1 // 已结算（计入邀请人佣金余额）
	commissionStatusCanceled = 2 // 已取消（订单未支付或被撤销）
)

type CommissionService struct {
	withdrawRepo     *repo.CommissionWithdrawRepo
	userRepo         *repo.UserRepo
	commissionRepo   *repo.CommissionLogRepo
	inviteRepo       *repo.InviteCodeRepo
	settingRepo      *repo.SettingRepo
	paymentOrderRepo *repo.PaymentOrderRepo
	logger           *slog.Logger
}

func NewCommissionService(
	withdrawRepo *repo.CommissionWithdrawRepo,
	userRepo *repo.UserRepo,
	commissionRepo *repo.CommissionLogRepo,
	inviteRepo *repo.InviteCodeRepo,
	settingRepo *repo.SettingRepo,
	paymentOrderRepo *repo.PaymentOrderRepo,
) *CommissionService {
	return &CommissionService{
		withdrawRepo:     withdrawRepo,
		userRepo:         userRepo,
		commissionRepo:   commissionRepo,
		inviteRepo:       inviteRepo,
		settingRepo:      settingRepo,
		paymentOrderRepo: paymentOrderRepo,
		logger:           slog.Default(),
	}
}

// SetLogger 注入 slog logger，未注入时使用 slog.Default()。
func (s *CommissionService) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *CommissionService) RequestWithdraw(ctx context.Context, userID uuid.UUID, req *model.CreateWithdrawRequest) (*model.Withdraw, error) {
	withdrawEnabled := true
	minWithdraw := 10.0

	enableSetting, err := s.settingRepo.GetByGroupKey(ctx, "invite", "commission.withdraw_enable")
	if err == nil && enableSetting != nil {
		var enabled bool
		if json.Unmarshal(enableSetting.ValueJSON, &enabled) == nil {
			withdrawEnabled = enabled
		}
	}

	minSetting, err := s.settingRepo.GetByGroupKey(ctx, "invite", "commission.min_withdraw")
	if err == nil && minSetting != nil {
		var minVal float64
		if json.Unmarshal(minSetting.ValueJSON, &minVal) == nil {
			minWithdraw = minVal
		}
	}

	if !withdrawEnabled {
		return nil, ErrWithdrawDisabled
	}

	if req.Amount < minWithdraw {
		return nil, ErrWithdrawMinAmount
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if user.CommissionBalance < req.Amount {
		return nil, ErrInsufficientBalance
	}

	if err := s.userRepo.DeductCommissionBalance(ctx, userID, req.Amount); err != nil {
		return nil, err
	}

	// real_name 列为 NOT NULL DEFAULT ''，统一以非 nil 指针写入（空串占位），
	// 避免 NULL 写入违反 NOT NULL 约束。
	realName := req.RealName
	w := &model.Withdraw{
		ID:       uuid.New(),
		UserID:   userID,
		Amount:   req.Amount,
		Method:   req.Method,
		Account:  req.Account,
		RealName: &realName,
		Status:   model.WithdrawStatusPending,
	}

	if err := s.withdrawRepo.Create(ctx, w); err != nil {
		return nil, err
	}

	return w, nil
}

func (s *CommissionService) ListUserWithdrawals(ctx context.Context, userID uuid.UUID) ([]*model.Withdraw, int, error) {
	return s.withdrawRepo.ListByUser(ctx, userID)
}

func (s *CommissionService) GetSummary(ctx context.Context, userID uuid.UUID) (*model.CommissionSummary, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	withdrawEnabled := true
	minWithdraw := 10.0
	rate := 100

	enableSetting, err := s.settingRepo.GetByGroupKey(ctx, "invite", "commission.withdraw_enable")
	if err == nil && enableSetting != nil {
		var enabled bool
		if json.Unmarshal(enableSetting.ValueJSON, &enabled) == nil {
			withdrawEnabled = enabled
		}
	}

	minSetting, err := s.settingRepo.GetByGroupKey(ctx, "invite", "commission.min_withdraw")
	if err == nil && minSetting != nil {
		var minVal float64
		if json.Unmarshal(minSetting.ValueJSON, &minVal) == nil {
			minWithdraw = minVal
		}
	}

	rateSetting, err := s.settingRepo.GetByGroupKey(ctx, "invite", "commission.rate")
	if err == nil && rateSetting != nil {
		var rateVal int
		if json.Unmarshal(rateSetting.ValueJSON, &rateVal) == nil {
			rate = rateVal
		}
	}

	invitedCount, err := s.userRepo.CountInvitedByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	withdrawnTotal, err := s.withdrawRepo.SumWithdrawnByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	pendingSettlement, err := s.withdrawRepo.GetPendingCommissions(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &model.CommissionSummary{
		AvailableBalance:  user.CommissionBalance,
		TotalEarned:       user.CommissionTotal,
		PendingSettlement: pendingSettlement,
		InvitedCount:      invitedCount,
		WithdrawnTotal:    withdrawnTotal,
		Rate:              rate,
		MinWithdraw:       minWithdraw,
		WithdrawEnabled:   withdrawEnabled,
	}, nil
}

// GetOrCreateInviteCode returns the user's first existing invite code, or creates one if none exists.
func (s *CommissionService) GetOrCreateInviteCode(ctx context.Context, userID uuid.UUID) (string, error) {
	codes, err := s.inviteRepo.ListByUser(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(codes) > 0 {
		return codes[0].Code, nil
	}
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	code := string(b)
	ic := &model.InviteCode{
		ID:     uuid.New(),
		UserID: userID,
		Code:   code,
	}
	if err := s.inviteRepo.Create(ctx, ic); err != nil {
		return "", err
	}
	return code, nil
}

// ListAllWithdrawals lists all withdrawals (admin view), optionally filtered by status.
func (s *CommissionService) ListAllWithdrawals(ctx context.Context, status *int) ([]*model.Withdraw, int, error) {
	return s.withdrawRepo.ListAll(ctx, status)
}

// ProcessWithdrawal approves or rejects a pending withdrawal. On reject, the amount is refunded to the user balance.
func (s *CommissionService) ProcessWithdrawal(ctx context.Context, withdrawID uuid.UUID, adminID uuid.UUID, approve bool, remark string) (*model.Withdraw, error) {
	w, err := s.withdrawRepo.GetByID(ctx, withdrawID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errors.New("withdrawal not found")
	}
	if w.Status != model.WithdrawStatusPending {
		return nil, errors.New("withdrawal already processed")
	}
	newStatus := model.WithdrawStatusRejected
	if approve {
		newStatus = model.WithdrawStatusPaid
	}
	if err := s.withdrawRepo.UpdateStatus(ctx, withdrawID, int(newStatus), adminID, remark); err != nil {
		return nil, err
	}
	if !approve {
		// refund the amount to user's commission balance
		if err := s.userRepo.RefundCommissionBalance(ctx, w.UserID, w.Amount); err != nil {
			return nil, err
		}
	}
	w.Status = newStatus
	handledBy := adminID
	w.HandledBy = &handledBy
	return w, nil
}

// commissionConfirmConfig 仅读取佣金结算所需的配置子集。
type commissionConfirmConfig struct {
	Enabled     bool `json:"enabled"`
	ConfirmDays int  `json:"confirm_days"`
}

// loadConfirmDays 读取佣金确认天数，读不到时返回默认值 3 天。
func (s *CommissionService) loadConfirmDays() (bool, int) {
	cfg := commissionConfirmConfig{Enabled: true, ConfirmDays: 3}
	data, err := s.settingRepo.GetJSON(context.Background(), "invite", "commission")
	if err != nil {
		return cfg.Enabled, cfg.ConfirmDays
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.ConfirmDays <= 0 {
		cfg.ConfirmDays = 3
	}
	return cfg.Enabled, cfg.ConfirmDays
}

// CheckPendingCommissions 检查待结算佣金（订单完成后 N 天自动结算）。
//
// 对应 Xboard 的 check:commission 定时任务：遍历所有超过确认期仍处于 pending
// 状态的佣金日志，确认后把佣金计入邀请人余额并将状态置为 settled。
// 单条失败仅记录日志，不影响其余结算。
func (s *CommissionService) CheckPendingCommissions(ctx context.Context) error {
	enabled, confirmDays := s.loadConfirmDays()
	if !enabled {
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(confirmDays) * 24 * time.Hour)
	pendingLogs, err := s.commissionRepo.ListPendingBefore(ctx, cutoff)
	if err != nil {
		s.logger.Error("scheduled: list pending commissions failed", "error", err)
		return err
	}
	if len(pendingLogs) == 0 {
		return nil
	}
	s.logger.Info("scheduled: checking pending commissions", "count", len(pendingLogs), "confirm_days", confirmDays)
	for _, cl := range pendingLogs {
		if err := s.settleOne(ctx, cl); err != nil {
			s.logger.Error("scheduled: settle commission failed", "commission", cl.ID, "error", err)
			continue
		}
	}
	return nil
}

// SettleCommission 结算单条佣金（手动触发或补偿重试）。
func (s *CommissionService) SettleCommission(ctx context.Context, commissionID uuid.UUID) error {
	cl, err := s.commissionRepo.GetByID(ctx, commissionID)
	if err != nil {
		return err
	}
	if cl == nil {
		return errors.New("commission not found")
	}
	if cl.Status != commissionStatusPending {
		return errors.New("commission is not pending")
	}
	return s.settleOne(ctx, cl)
}

// settleOne 执行单条佣金的结算：校验订单仍为已支付 → 校验邀请人存在 →
// 累加佣金余额/累计 → 更新状态为已结算。
// 订单若已退款/取消，则把佣金标记为 canceled，避免向邀请人错误发放。
func (s *CommissionService) settleOne(ctx context.Context, cl *model.CommissionLog) error {
	// 订单状态校验：若关联订单已不再是已支付，则取消该笔佣金（对齐原 PaymentService 行为）
	if cl.OrderID != nil && s.paymentOrderRepo != nil {
		order, err := s.paymentOrderRepo.GetByID(ctx, *cl.OrderID)
		if err != nil {
			s.logger.Warn("settle: load order failed, skip", "commission", cl.ID, "order", *cl.OrderID, "error", err)
			return err
		}
		if order == nil || order.Status != model.PaymentStatusPaid {
			_ = s.commissionRepo.UpdateStatus(ctx, cl.ID, commissionStatusCanceled)
			s.logger.Info("settle: order not paid, commission canceled", "commission", cl.ID, "order", cl.OrderID)
			return nil
		}
	}

	inviter, err := s.userRepo.GetByID(ctx, cl.InviterID)
	if err != nil {
		return err
	}
	if inviter == nil {
		// 邀请人不存在，直接标记取消，避免反复重试
		_ = s.commissionRepo.UpdateStatus(ctx, cl.ID, commissionStatusCanceled)
		return errors.New("inviter not found")
	}
	newBalance := math.Round((inviter.CommissionBalance+cl.GetAmount)*100) / 100
	newTotal := math.Round((inviter.CommissionTotal+cl.GetAmount)*100) / 100
	if err := s.userRepo.UpdateCommission(ctx, inviter.ID, newBalance, newTotal); err != nil {
		return err
	}
	cl.CommissionBalance = newBalance
	if err := s.commissionRepo.UpdateStatus(ctx, cl.ID, commissionStatusSettled); err != nil {
		return err
	}
	s.logger.Info("commission settled", "commission", cl.ID, "inviter", inviter.ID, "amount", cl.GetAmount, "balance", newBalance)
	return nil
}

// ListCommissionDetails 查询用户的佣金记录明细（分页，按创建时间倒序）。
func (s *CommissionService) ListCommissionDetails(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.CommissionDetailResponse, int, error) {
	logs, total, err := s.commissionRepo.ListByInviter(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.CommissionDetailResponse, 0, len(logs))
	for _, cl := range logs {
		result = append(result, model.NewCommissionDetailResponse(cl))
	}
	return result, total, nil
}

// ListInvitations 查询用户邀请的用户列表（分页，按注册时间倒序）。
func (s *CommissionService) ListInvitations(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.InvitationResponse, int, error) {
	users, total, err := s.userRepo.ListInvitedByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]model.InvitationResponse, 0, len(users))
	for _, u := range users {
		result = append(result, model.NewInvitationResponse(u))
	}
	return result, total, nil
}

// ProcessWithdraw 处理提现申请（定时任务自动放行）。
//
// 与 ProcessWithdrawal(adminID, approve, remark) 的区别：本方法面向定时任务，
// 以系统身份（adminID=uuid.Nil）自动放行处于 pending 的提现申请。
// 拒绝场景请使用 ProcessWithdrawal 手动处理。
func (s *CommissionService) ProcessWithdraw(ctx context.Context, withdrawID uuid.UUID) error {
	w, err := s.withdrawRepo.GetByID(ctx, withdrawID)
	if err != nil {
		return err
	}
	if w == nil {
		return errors.New("withdrawal not found")
	}
	if w.Status != model.WithdrawStatusPending {
		return errors.New("withdrawal already processed")
	}
	// 自动放行：状态置为已打款，不退款
	if err := s.withdrawRepo.UpdateStatus(ctx, withdrawID, int(model.WithdrawStatusPaid), uuid.Nil, "auto-processed by scheduler"); err != nil {
		return err
	}
	s.logger.Info("withdraw auto-processed", "withdraw", withdrawID, "user", w.UserID, "amount", w.Amount)
	return nil
}
