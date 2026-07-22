package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/airport-panel/traffic-service/internal/repo"
)

// MailSender 流量告警邮件发送抽象。
//
// traffic-service 与 identity-service（mail_service 所在）是独立的 Go module，
// 且 identity-service/internal/service 受 Go internal 包规则限制无法被外部导入，
// 因此这里通过接口解耦：调用方注入具体实现（如 SMTP 适配器或跨服务事件适配器），
// traffic-service 本身只负责"找出超量用户并触发告警"。
type MailSender interface {
	// IsEnabled 邮件通道是否可用（未配置 SMTP 时返回 false，告警将仅记录日志）。
	IsEnabled() bool
	// SendTrafficAlert 向指定邮箱发送流量使用告警。
	SendTrafficAlert(ctx context.Context, to string, usedBytes, quotaBytes int64) error
}

// NopMailSender 空实现：不发送任何邮件，仅记录日志。作为默认兜底，保证服务可独立运行。
type NopMailSender struct {
	logger *slog.Logger
}

func NewNopMailSender(logger *slog.Logger) *NopMailSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &NopMailSender{logger: logger}
}

func (n *NopMailSender) IsEnabled() bool { return false }

func (n *NopMailSender) SendTrafficAlert(ctx context.Context, to string, usedBytes, quotaBytes int64) error {
	n.logger.Warn("traffic alert mail skipped: no mail sender configured",
		"to", to, "used", usedBytes, "quota", quotaBytes)
	return nil
}

// TrafficReminderService 流量提醒定时任务。
//
// 对应 Xboard 的 send:remindMail 定时任务：每天 11:30 检查流量使用超过阈值
// （默认 80%）的用户并发送告警邮件，提醒用户及时续费或购买流量包。
type TrafficReminderService struct {
	repo      *repo.TrafficRepo
	mailer    MailSender
	logger    *slog.Logger
	threshold float64 // 触发告警的流量使用比例，默认 0.8
}

// NewTrafficReminderService 创建流量提醒服务。mailer 为 nil 时使用 NopMailSender。
func NewTrafficReminderService(repo *repo.TrafficRepo, mailer MailSender, logger *slog.Logger) *TrafficReminderService {
	if logger == nil {
		logger = slog.Default()
	}
	if mailer == nil {
		mailer = NewNopMailSender(logger)
	}
	return &TrafficReminderService{
		repo:      repo,
		mailer:    mailer,
		logger:    logger,
		threshold: 0.8,
	}
}

// SetThreshold 设置触发告警的流量使用比例（0~1），<=0 时保持默认 0.8。
func (s *TrafficReminderService) SetThreshold(ratio float64) {
	if ratio > 0 && ratio <= 1 {
		s.threshold = ratio
	}
}

// SendTrafficReminders 扫描流量使用超过阈值的用户并发送告警邮件。
// 单个用户发送失败仅记录日志，不影响其余用户。
func (s *TrafficReminderService) SendTrafficReminders(ctx context.Context) error {
	users, err := s.repo.ListUsersExceedingUsageRatio(ctx, s.threshold)
	if err != nil {
		s.logger.Error("traffic reminder: list exceeding users failed", "error", err)
		return err
	}
	if len(users) == 0 {
		return nil
	}
	s.logger.Info("traffic reminder: sending alerts", "count", len(users), "threshold", s.threshold)

	sent := 0
	for _, u := range users {
		if u.Email == "" {
			s.logger.Warn("traffic reminder: user has no email, skip", "user_id", u.UserID)
			continue
		}
		if err := s.mailer.SendTrafficAlert(ctx, u.Email, u.UsedBytes, u.QuotaBytes); err != nil {
			s.logger.Error("traffic reminder: send mail failed",
				"user_id", u.UserID, "email", u.Email, "error", err)
			continue
		}
		sent++
		s.logger.Info("traffic reminder: alert sent",
			"user_id", u.UserID, "email", u.Email,
			"used", formatBytes(u.UsedBytes), "quota", formatBytes(u.QuotaBytes),
			"ratio", fmt.Sprintf("%.0f%%", u.UsageRatio()*100),
		)
	}
	s.logger.Info("traffic reminder: done", "total", len(users), "sent", sent)
	return nil
}

// StartScheduledJobs 启动流量提醒定时任务：每天 11:30 执行一次。
// ctx 取消后任务退出。启动后会等待到下一个 11:30 再首次执行（避免启动即打扰）。
func (s *TrafficReminderService) StartScheduledJobs(ctx context.Context) {
	s.logger.Info("traffic reminder scheduled job starting", "fire_at", "11:30 daily", "threshold", s.threshold)
	go s.runDaily(ctx)
}

// runDaily 计算到下一个 11:30 的等待时间，先等待到 11:30 执行首轮，之后每 24 小时执行一次。
func (s *TrafficReminderService) runDaily(ctx context.Context) {
	for {
		next := nextFireAt(time.Now(), 11, 30)
		wait := time.Until(next)
		if wait <= 0 {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			s.SendTrafficReminders(ctx)
		}
	}
}

// nextFireAt 返回从 from 起下一个 hh:mm 时刻（同天已过则顺延到次日）。
func nextFireAt(from time.Time, hour, minute int) time.Time {
	t := time.Date(from.Year(), from.Month(), from.Day(), hour, minute, 0, 0, from.Location())
	if !t.After(from) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// formatBytes 将字节数格式化为易读字符串（仅用于日志）。
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
