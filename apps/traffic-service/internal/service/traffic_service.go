package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/config/events"
	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/airport-panel/traffic-service/internal/repo"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrNoActiveSubscription = errors.New("no active subscription")
	ErrQuotaExceeded        = errors.New("traffic quota exceeded")
	ErrInvalidReport        = errors.New("invalid traffic report")
)

type TrafficService struct {
	trafficRepo      *repo.TrafficRepo
	sessionRepo      *repo.SessionRepo
	credentialRepo   *repo.UserNodeCredentialRepo
	redisClient      *goredis.Client
	eventBus         *events.Bus
	logger           *slog.Logger
	lastMonthlyReset time.Time
}

func NewTrafficService(trafficRepo *repo.TrafficRepo, sessionRepo *repo.SessionRepo, credentialRepo *repo.UserNodeCredentialRepo, redisClient *goredis.Client) *TrafficService {
	return &TrafficService{
		trafficRepo:    trafficRepo,
		sessionRepo:    sessionRepo,
		credentialRepo: credentialRepo,
		redisClient:    redisClient,
	}
}

// SetEventBus 注入事件总线（用于定时任务发送超额通知等事件）。
func (s *TrafficService) SetEventBus(bus *events.Bus) { s.eventBus = bus }

// SetLogger 注入 slog logger。未注入时使用 slog.Default()。
func (s *TrafficService) SetLogger(logger *slog.Logger) { s.logger = logger }

func (s *TrafficService) ReportTraffic(ctx context.Context, reports []model.TrafficReportItem, serverCode string) error {
	if len(reports) == 0 {
		return ErrInvalidReport
	}

	// 通过 ServerCode 解析 node_id，用于节点级流量统计
	// 一台服务器可能有多个节点，取第一个启用节点作为流量归属
	var nodeID *uuid.UUID
	if serverCode != "" {
		if nid, err := s.trafficRepo.GetNodeIDByServerCode(ctx, serverCode); err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to resolve node_id from server_code", "server_code", serverCode, "error", err)
			}
		} else if nid != nil {
			nodeID = nid
		}
	}

	// 同一上报请求内对同一凭证的本地缓存，避免重复查库。
	// 缓存值含义：nil 表示已查过但未匹配；非 nil 表示反查到的 userID。
	credCache := make(map[string]*uuid.UUID, len(reports))

	for _, r := range reports {
		// 跳过无效上报：既无凭证也无 user_id，或流量为 0
		if r.Credential == "" && r.UserID == uuid.Nil {
			if s.logger != nil {
				s.logger.Warn("skipping invalid traffic report: no credential and no user_id")
			}
			continue
		}
		if r.UploadBytes == 0 && r.DownloadBytes == 0 {
			continue
		}

		ts := r.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}

		// 通过凭证反查 user_id（向后兼容：无凭证或反查失败时降级使用 r.UserID）。
		userID := r.UserID
		if r.Credential != "" && s.credentialRepo != nil {
			if cached, ok := credCache[r.Credential]; ok {
				if cached != nil {
					userID = *cached
				}
			} else {
				cred, err := s.credentialRepo.GetByCredentialValue(ctx, r.Credential)
				if err != nil {
					if s.logger != nil {
						s.logger.Warn("lookup credential failed, fallback to reported user_id",
							"credential", r.Credential, "error", err)
					}
				} else if cred != nil {
					uid := cred.UserID
					credCache[r.Credential] = &uid
					userID = uid
				} else {
					// 凭证不在表中，标记为已查未命中，降级使用 r.UserID。
					credCache[r.Credential] = nil
					if s.logger != nil {
						s.logger.Warn("credential not found, fallback to reported user_id",
							"credential", r.Credential, "user_id", r.UserID)
					}
				}
			}
		}

		// 如果上报记录中没有 NodeID，使用从 ServerCode 解析的 nodeID
		effectiveNodeID := r.NodeID
		if effectiveNodeID == nil {
			effectiveNodeID = nodeID
		}

		if err := s.trafficRepo.RecordUsage(ctx, userID, effectiveNodeID, r.UploadBytes, r.DownloadBytes, ts); err != nil {
			return fmt.Errorf("record usage for user %s: %w", userID, err)
		}

		if s.redisClient != nil {
			onlineKey := fmt.Sprintf("%s%s", repo.OnlineUserKeyPrefix, userID.String())
			s.redisClient.Set(ctx, onlineKey, "1", repo.OnlineTTL)
		}
	}
	return nil
}

func (s *TrafficService) GetUserTraffic(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) (*model.UserTrafficResponse, error) {
	if startDate.IsZero() {
		startDate = time.Now().AddDate(0, -1, 0)
	}
	if endDate.IsZero() {
		endDate = time.Now()
	}

	startDate = startDate.Truncate(24 * time.Hour)
	endDate = endDate.Truncate(24 * time.Hour).Add(24 * time.Hour)

	usages, err := s.trafficRepo.GetDailyUsage(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	totalUpload := int64(0)
	totalDownload := int64(0)
	dailyItems := make([]model.DailyTrafficItem, 0)

	for _, u := range usages {
		totalUpload += u.UploadBytes
		totalDownload += u.DownloadBytes
		dailyItems = append(dailyItems, model.DailyTrafficItem{
			Date:          u.UsageDate.Format("2006-01-02"),
			UploadBytes:   u.UploadBytes,
			DownloadBytes: u.DownloadBytes,
			TotalBytes:    u.TotalBytes,
		})
	}

	quota, err := s.CheckQuota(ctx, userID)
	if err != nil {
		quota = &model.QuotaCheckResult{}
	}

	return &model.UserTrafficResponse{
		UserID:         userID,
		StartDate:      startDate.Format("2006-01-02"),
		EndDate:        endDate.Add(-24 * time.Hour).Format("2006-01-02"),
		TotalUpload:    totalUpload,
		TotalDownload:  totalDownload,
		TotalBytes:     totalUpload + totalDownload,
		DailyBreakdown: dailyItems,
		Quota:          *quota,
	}, nil
}

func (s *TrafficService) CheckQuota(ctx context.Context, userID uuid.UUID) (*model.QuotaCheckResult, error) {
	return s.trafficRepo.CheckQuota(ctx, userID)
}

func (s *TrafficService) ResetTraffic(ctx context.Context, userID uuid.UUID) error {
	if err := s.trafficRepo.ResetUserTraffic(ctx, userID); err != nil {
		return err
	}
	if s.eventBus != nil {
		payload := events.UserEvent{
			UserID: userID.String(),
			Reason: "admin_reset",
		}
		if err := s.eventBus.Publish(ctx, events.TopicTrafficReset, payload); err != nil {
			s.logger.Error("publish traffic reset event failed", "user_id", userID, "error", err)
		}
	}
	return nil
}

func (s *TrafficService) ResetAllTraffic(ctx context.Context) error {
	if err := s.trafficRepo.ResetAllMonthlyTraffic(ctx); err != nil {
		return err
	}
	if s.eventBus != nil {
		payload := events.UserEvent{
			UserID: "*",
			Reason: "monthly_reset",
		}
		if err := s.eventBus.Publish(ctx, events.TopicTrafficReset, payload); err != nil {
			s.logger.Error("publish monthly reset event failed", "error", err)
		}
	}
	return nil
}

func (s *TrafficService) GetOverview(ctx context.Context) (*model.OverviewResponse, error) {
	todayUpload, todayDownload, err := s.trafficRepo.GetTodayTotalUsage(ctx)
	if err != nil {
		return nil, err
	}

	topNodes, err := s.trafficRepo.GetTopNodes(ctx, time.Now(), 10)
	if err != nil {
		topNodes = []*model.NodeTrafficItem{}
	}

	onlineCount := int64(0)
	if s.redisClient != nil {
		pattern := fmt.Sprintf("%s*", repo.OnlineUserKeyPrefix)
		keys, err := s.redisClient.Keys(ctx, pattern).Result()
		if err == nil {
			onlineCount = int64(len(keys))
		}
	} else {
		sessions, err := s.sessionRepo.GetActiveSessions(ctx)
		if err == nil {
			onlineCount = int64(len(sessions))
		}
	}

	return &model.OverviewResponse{
		TodayUpload:   todayUpload,
		TodayDownload: todayDownload,
		TodayTotal:    todayUpload + todayDownload,
		OnlineCount:   onlineCount,
		TopNodes:      topNodesToItems(topNodes),
	}, nil
}

func topNodesToItems(nodes []*model.NodeTrafficItem) []model.NodeTrafficItem {
	items := make([]model.NodeTrafficItem, len(nodes))
	for i, n := range nodes {
		items[i] = *n
	}
	return items
}

func MapTrafficErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrNoActiveSubscription):
		return config.CodeForbidden, "no active subscription"
	case errors.Is(err, ErrQuotaExceeded):
		return config.CodeForbidden, "traffic quota exceeded"
	case errors.Is(err, ErrInvalidReport):
		return config.CodeBadRequest, "invalid traffic report"
	default:
		return config.CodeInternalError, "internal server error"
	}
}

// StartScheduledJobs 启动流量/订阅自动化定时任务。
//
// 共启动 3 个 goroutine：
//  1. 每分钟检查超额订阅 + 过期订阅（启动后立即执行首轮）
//  2. 每 24 小时执行日汇总（简化实现：记录日志）
//  3. 每分钟检查是否需要执行月度流量重置（仅当当前日期为 1 号时触发）
//
// 所有任务共享传入的 ctx，ctx 取消后全部退出。
// 单个任务失败仅记录日志，不影响其他任务和整个 scheduler。
func (s *TrafficService) StartScheduledJobs(ctx context.Context) {
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.logger.Info("traffic scheduled jobs starting")

	go s.runMinuteTicker(ctx)
	go s.runDailyTicker(ctx)
	go s.runMonthlyTicker(ctx)
}

// runMinuteTicker 每分钟执行一次：检查过期订阅 + 检查超额订阅。
func (s *TrafficService) runMinuteTicker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	// 启动后立即执行一次（避免冷启动等待）
	s.checkExpiredSubscriptions(ctx)
	s.checkOverQuotaUsers(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkExpiredSubscriptions(ctx)
			s.checkOverQuotaUsers(ctx)
		}
	}
}

// runDailyTicker 每 24 小时执行一次日汇总（简化实现：记录日志）。
func (s *TrafficService) runDailyTicker(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	// 启动后立即执行一次首轮
	s.runDailySummary(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDailySummary(ctx)
		}
	}
}

// runMonthlyTicker 每分钟检查当前日期是否为 1 号，若是且本月尚未重置则执行月度流量重置。
func (s *TrafficService) runMonthlyTicker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	// 启动后立即检查一次（应对服务在 1 号启动的场景）
	s.runMonthlyResetIfNeeded(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runMonthlyResetIfNeeded(ctx)
		}
	}
}

// checkExpiredSubscriptions 将所有已过期但仍标记为 active 的订阅置为 expired，
// 并为每个过期用户发布 TopicUserBanned 事件（reason="subscription_expired"），
// 触发 node-service 实时踢人。
func (s *TrafficService) checkExpiredSubscriptions(ctx context.Context) {
	userIDs, err := s.trafficRepo.MarkExpiredSubscriptions(ctx)
	if err != nil {
		s.logger.Error("scheduled: mark expired subscriptions failed", "error", err)
		return
	}
	if len(userIDs) == 0 {
		return
	}
	s.logger.Info("scheduled: subscriptions marked as expired", "count", len(userIDs))
	for _, uid := range userIDs {
		if s.eventBus != nil {
			payload := events.UserEvent{
				UserID: uid,
				Reason: "subscription_expired",
			}
			if err := s.eventBus.Publish(ctx, events.TopicUserBanned, payload); err != nil {
				s.logger.Error("scheduled: publish expired event failed", "user_id", uid, "error", err)
			}
		}
		s.logger.Warn("scheduled: subscription expired, notified via event bus", "user_id", uid)
	}
}

// checkOverQuotaUsers 检查超额订阅（调用 CheckQuota），超额时通过事件总线通知。
func (s *TrafficService) checkOverQuotaUsers(ctx context.Context) {
	userIDs, err := s.trafficRepo.ListOverQuotaUserIDs(ctx)
	if err != nil {
		s.logger.Error("scheduled: list over-quota users failed", "error", err)
		return
	}
	if len(userIDs) == 0 {
		return
	}
	for _, uid := range userIDs {
		userID, err := uuid.Parse(uid)
		if err != nil {
			s.logger.Warn("scheduled: invalid user_id from over-quota list", "user_id", uid, "error", err)
			continue
		}
		result, err := s.CheckQuota(ctx, userID)
		if err != nil {
			s.logger.Error("scheduled: check quota failed", "user_id", uid, "error", err)
			continue
		}
		if !result.IsOverQuota {
			continue
		}
		// 通过事件总线通知：复用 TopicUserBanned，触发 app.go 中订阅者清除在线会话
		if s.eventBus != nil {
			payload := events.UserEvent{
				UserID: uid,
				Reason: "traffic_over_quota",
			}
			if err := s.eventBus.Publish(ctx, events.TopicUserBanned, payload); err != nil {
				s.logger.Error("scheduled: publish over-quota event failed", "user_id", uid, "error", err)
			}
		}
		s.logger.Warn("scheduled: user over quota, notified via event bus",
			"user_id", uid,
			"used", result.TrafficUsed,
			"quota", result.TrafficQuota,
		)
	}
}

// runDailySummary 执行日汇总（简化实现：记录当日流量与活跃订阅数日志）。
func (s *TrafficService) runDailySummary(ctx context.Context) {
	upload, download, err := s.trafficRepo.GetTodayTotalUsage(ctx)
	if err != nil {
		s.logger.Error("scheduled: daily summary get today usage failed", "error", err)
		return
	}
	activeIDs, _ := s.trafficRepo.ListActiveUserIDs(ctx)
	s.logger.Info("scheduled: daily traffic summary",
		"date", time.Now().Format("2006-01-02"),
		"upload_bytes", upload,
		"download_bytes", download,
		"total_bytes", upload+download,
		"active_subscriptions", len(activeIDs),
	)
}

// runMonthlyResetIfNeeded 检查当前日期是否为 1 号，若是且本月尚未重置则执行 ResetAllTraffic。
// 通过 lastMonthlyReset 字段保证每月只执行一次（避免重启或多次 tick 重复重置）。
func (s *TrafficService) runMonthlyResetIfNeeded(ctx context.Context) {
	now := time.Now()
	if now.Day() != 1 {
		return
	}
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	if !s.lastMonthlyReset.IsZero() && !s.lastMonthlyReset.Before(currentMonth) {
		return
	}
	s.logger.Info("scheduled: monthly traffic reset started", "month", currentMonth.Format("2006-01"))
	if err := s.ResetAllTraffic(ctx); err != nil {
		s.logger.Error("scheduled: monthly traffic reset failed", "error", err)
		return
	}
	s.lastMonthlyReset = currentMonth
	s.logger.Info("scheduled: monthly traffic reset completed", "month", currentMonth.Format("2006-01"))
}
