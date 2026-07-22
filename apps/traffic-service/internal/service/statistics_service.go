package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/airport-panel/traffic-service/internal/model"
	"github.com/airport-panel/traffic-service/internal/repo"
)

// StatisticsService 流量统计汇总定时任务。
//
// 对应 Xboard 的 traffic:statistics 定时任务：每日汇总流量使用情况，
// 将当日上传/下载字节数、活跃用户数、在线用户数写入 traffic_statistics_daily 表，
// 供后台仪表盘与趋势分析使用。
type StatisticsService struct {
	repo   *repo.TrafficRepo
	logger *slog.Logger
}

// NewStatisticsService 创建统计汇总服务。
func NewStatisticsService(repo *repo.TrafficRepo, logger *slog.Logger) *StatisticsService {
	if logger == nil {
		logger = slog.Default()
	}
	return &StatisticsService{repo: repo, logger: logger}
}

// DailyStatistics 每日统计汇总（入库到 traffic_statistics_daily 表）。
//
// 汇总维度：
//   - upload_bytes / download_bytes：当日全站上传/下载字节数（GetTodayTotalUsage）
//   - total_bytes：上传+下载
//   - active_users：当前活跃订阅用户数（ListActiveUserIDs 计数）
//   - online_count：在线用户数（暂记 0，可由 online 设备上报或 Redis 在线集合补全）
//
// 统计日期取当天（按服务器本地时区截断到日）。失败仅记录日志并返回错误。
func (s *StatisticsService) DailyStatistics(ctx context.Context) error {
	upload, download, err := s.repo.GetTodayTotalUsage(ctx)
	if err != nil {
		s.logger.Error("statistics: get today total usage failed", "error", err)
		return err
	}

	activeUsers := 0
	if ids, err := s.repo.ListActiveUserIDs(ctx); err != nil {
		s.logger.Warn("statistics: list active users failed, fallback to 0", "error", err)
	} else {
		activeUsers = len(ids)
	}

	stat := &model.DailyStatistic{
		StatDate:      time.Now(),
		UploadBytes:   upload,
		DownloadBytes: download,
		TotalBytes:    upload + download,
		ActiveUsers:   activeUsers,
		OnlineCount:   0, // TODO: 接入在线设备/Redis 在线集合后补全
	}

	if err := s.repo.RecordDailyStatistics(ctx, stat); err != nil {
		s.logger.Error("statistics: record daily statistics failed", "error", err)
		return err
	}
	s.logger.Info("statistics: daily summary recorded",
		"date", stat.StatDate.Format("2006-01-02"),
		"upload", stat.UploadBytes, "download", stat.DownloadBytes,
		"total", stat.TotalBytes, "active_users", stat.ActiveUsers,
	)
	return nil
}

// StartScheduledJobs 启动每日统计汇总定时任务（每天 23:55 执行，归档当日数据）。
// ctx 取消后任务退出。
func (s *StatisticsService) StartScheduledJobs(ctx context.Context) {
	s.logger.Info("statistics scheduled job starting", "fire_at", "23:55 daily")
	go func() {
		for {
			next := nextFireAt(time.Now(), 23, 55)
			wait := time.Until(next)
			if wait <= 0 {
				wait = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				s.DailyStatistics(ctx)
			}
		}
	}()
}
