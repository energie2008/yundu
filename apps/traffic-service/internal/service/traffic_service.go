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

	// P2-I: 从 channel_health_current 表聚合全站在线人数。
	// node-agent 每 10s 心跳上报 online_users（基于连接生命周期计数：connect +1 / close -1），
	// 写入 channel_health_current.online_users。此处 SUM 所有服务器的值，返回全站实时在线人数。
	// 优先使用 channel_health_current（精确）；查询失败时回退到 Redis/session 方式（近似）。
	onlineCount := int64(0)
	if totalOnline, err := s.trafficRepo.GetTotalOnlineUsers(ctx); err == nil {
		onlineCount = totalOnline
	} else if s.redisClient != nil {
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
	go s.runCycleResetTicker(ctx)
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

// runCycleResetTicker 每小时检查一次是否有订阅到达周期重置点，按“购买之日”逐用户重置流量。
// 替代旧的“每月 1 号全量清零”（导致所有用户统一按自然月重置）。
func (s *TrafficService) runCycleResetTicker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	// 启动后立即检查一次（应对服务在重置日启动的场景）
	s.runCycleResetIfNeeded(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCycleResetIfNeeded(ctx)
		}
	}
}

// runCycleResetIfNeeded 遍历周期订阅，对“本期已开始但尚未重置”的用户执行流量重置。
// 锚点 = 订阅 started_at（购买之日）起每 30 天滚动；
// 续费只延长 expires_at，不改变重置日。一次性/不限时套餐不参与周期重置。
// reset_at 为空时只初始化锚点不清零，避免首次部署误清老用户流量。
func (s *TrafficService) runCycleResetIfNeeded(ctx context.Context) {
	subs, err := s.trafficRepo.ListCycleSubscriptions(ctx)
	if err != nil {
		s.logger.Error("scheduled: list cycle subscriptions failed", "error", err)
		return
	}
	now := time.Now()
	resetCount := 0
	for _, sub := range subs {
		anchor := cycleResetAnchor(sub.StartedAt, now)
		if anchor.IsZero() {
			continue // 尚未到达第一个周期起点
		}
		if sub.ResetAt == nil {
			// 首次部署/新订阅：只登记周期起点，不清零当前流量
			if err := s.trafficRepo.SetSubscriptionResetAt(ctx, sub.UserID, anchor); err != nil {
				s.logger.Warn("scheduled: init cycle reset anchor failed",
					"user_id", sub.UserID, "anchor", anchor.Format("2006-01-02 15:04"), "error", err)
			}
			continue
		}
		if sub.ResetAt != nil && !sub.ResetAt.Before(anchor) {
			continue // 本期已重置
		}
		if err := s.ResetTraffic(ctx, sub.UserID); err != nil {
			s.logger.Warn("scheduled: cycle traffic reset failed",
				"user_id", sub.UserID, "anchor", anchor.Format("2006-01-02"), "error", err)
			continue
		}
		resetCount++
		s.logger.Info("scheduled: cycle traffic reset done",
			"user_id", sub.UserID, "anchor", anchor.Format("2006-01-02"))
	}
	if resetCount > 0 {
		s.logger.Info("scheduled: cycle traffic reset completed", "users", resetCount)
	}
}

// cycleResetAnchor 计算 <= now 的最近一个周期起点。
// 锚点 = started_at + 30 天 * k（k 为非负整数），即购买日起每 30 天滚动；
// now 早于 started_at 时返回零值。
func cycleResetAnchor(startedAt, now time.Time) time.Time {
	if now.Before(startedAt) {
		return time.Time{}
	}
	const cycleDays = 30
	const approx = 30 * 24 * time.Hour
	k := int(now.Sub(startedAt) / approx)
	if k < 0 {
		k = 0
	}
	// 用粗略商起步，再按日历天数精确校正（处理月末/DST 边界）
	for startedAt.AddDate(0, 0, cycleDays*(k+1)).After(now) {
		k--
	}
	for !startedAt.AddDate(0, 0, cycleDays*(k+1)).After(now) {
		k++
	}
	return startedAt.AddDate(0, 0, cycleDays*k)
}
