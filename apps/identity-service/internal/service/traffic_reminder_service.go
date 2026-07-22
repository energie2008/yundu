package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/airport-panel/identity-service/internal/repo"
)

// TrafficReminderService 负责定时检查套餐即将到期 / 流量即将耗尽的订阅，
// 并通过 NotificationService 发送站内信提醒（模板：plan_expiry / traffic_warning）。
//
// 设计要点：
//   - 仅扫描活跃订阅（status='active'），避免对已过期/已替换订阅产生噪声。
//   - 尊重用户通知偏好（users.notify_expiry / notify_traffic）。
//   - 通过 NotificationService.RecentNotificationExists 做去重，同一模板对同一用户
//     在 dedupWindow 内只发送一次，避免定时任务每次执行都重复推送。
type TrafficReminderService struct {
	subRepo   *repo.SubscriptionRepo
	userRepo  *repo.UserRepo
	notifySvc *NotificationService
	log       *slog.Logger
}

// NewTrafficReminderService 构造流量/到期提醒服务。
func NewTrafficReminderService(
	subRepo *repo.SubscriptionRepo,
	userRepo *repo.UserRepo,
	notifySvc *NotificationService,
	log *slog.Logger,
) *TrafficReminderService {
	if log == nil {
		log = slog.Default()
	}
	return &TrafficReminderService{
		subRepo:   subRepo,
		userRepo:  userRepo,
		notifySvc: notifySvc,
		log:       log,
	}
}

// CheckPlanExpiry 检查套餐即将到期（到期前 3 天）的订阅并发送 plan_expiry 通知。
// withinHours 控制扫描窗口（默认 72 小时 = 3 天）。
func (s *TrafficReminderService) CheckPlanExpiry(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("CheckPlanExpiry panic", "error", r)
		}
	}()
	const withinHours = 72
	subs, err := s.subRepo.ListExpiringSoon(ctx, withinHours)
	if err != nil {
		s.log.Error("plan expiry: list expiring subscriptions failed", "error", err)
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	dedupSince := time.Now().Add(-24 * time.Hour)
	sent := 0
	for _, sub := range subs {
		if sub.ExpiresAt == nil {
			continue
		}
		user, err := s.userRepo.GetByID(ctx, sub.UserID)
		if err != nil || user == nil {
			continue
		}
		// 尊重用户偏好
		if !user.NotifyExpiry {
			continue
		}
		// 去重：24 小时内已发过 plan_expiry 则跳过
		exists, err := s.notifySvc.RecentNotificationExists(ctx, user.ID, "plan_expiry", dedupSince)
		if err != nil {
			s.log.Warn("plan expiry: dedup check failed", "user", user.ID, "error", err)
		}
		if exists {
			continue
		}
		s.notifySvc.NotifyUserAsync(user.ID, "plan_expiry", map[string]interface{}{
			"plan_name":   sub.PlanName,
			"expiry_date": sub.ExpiresAt.Format("2006-01-02 15:04"),
			"expires_at":  sub.ExpiresAt.Format("2006-01-02 15:04"),
			"user_id":     user.ID.String(),
		})
		sent++
	}
	if sent > 0 {
		s.log.Info("plan expiry: notifications dispatched", "count", sent, "candidates", len(subs))
	}
	return nil
}

// CheckTrafficWarning 检查流量即将耗尽（已用 > 80%）的订阅并发送 traffic_warning 通知。
func (s *TrafficReminderService) CheckTrafficWarning(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("CheckTrafficWarning panic", "error", r)
		}
	}()
	const thresholdPct = 80.0
	subs, err := s.subRepo.ListHighTrafficUsage(ctx, thresholdPct)
	if err != nil {
		s.log.Error("traffic warning: list high traffic subscriptions failed", "error", err)
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	dedupSince := time.Now().Add(-24 * time.Hour)
	sent := 0
	for _, sub := range subs {
		if sub.TrafficQuotaBytes <= 0 {
			continue
		}
		user, err := s.userRepo.GetByID(ctx, sub.UserID)
		if err != nil || user == nil {
			continue
		}
		if !user.NotifyTraffic {
			continue
		}
		exists, err := s.notifySvc.RecentNotificationExists(ctx, user.ID, "traffic_warning", dedupSince)
		if err != nil {
			s.log.Warn("traffic warning: dedup check failed", "user", user.ID, "error", err)
		}
		if exists {
			continue
		}
		pct := float64(sub.TrafficUsedBytes) * 100.0 / float64(sub.TrafficQuotaBytes)
		s.notifySvc.NotifyUserAsync(user.ID, "traffic_warning", map[string]interface{}{
			"used":       formatBytes(sub.TrafficUsedBytes),
			"total":      formatBytes(sub.TrafficQuotaBytes),
			"percentage": fmt.Sprintf("%.1f", pct),
			"user_id":    user.ID.String(),
		})
		sent++
	}
	if sent > 0 {
		s.log.Info("traffic warning: notifications dispatched", "count", sent, "candidates", len(subs))
	}
	return nil
}

// formatBytes 将字节数格式化为人类可读字符串（MB / GB）。
func formatBytes(b int64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if b >= gb {
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	}
	if b >= mb {
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	}
	return fmt.Sprintf("%d B", b)
}
