package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogCleanupService 日志清理定时任务。
//
// 对应 Xboard 的日志清理类定时任务：定期删除超过保留期的日志型数据，避免
// node_doctor_reports（节点体检报告）、cert_deploy_records（证书部署记录）等
// 日志表无限膨胀。默认保留 30 天。
type LogCleanupService struct {
	pool         *pgxpool.Pool
	logger       *slog.Logger
	retentionDays int
	// tables 需要清理的日志表列表，每张表须含 created_at 列。
	tables []string
}

// NewLogCleanupService 创建日志清理服务。pool 为数据库连接池。
func NewLogCleanupService(pool *pgxpool.Pool, logger *slog.Logger) *LogCleanupService {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogCleanupService{
		pool:          pool,
		logger:        logger,
		retentionDays: 30,
		// 默认清理节点体检报告与证书部署记录两类日志型数据。
		tables:        []string{"node_doctor_reports", "cert_deploy_records"},
	}
}

// SetRetentionDays 设置保留天数（<=0 时保持默认 30 天）。
func (s *LogCleanupService) SetRetentionDays(days int) {
	if days > 0 {
		s.retentionDays = days
	}
}

// SetTables 覆盖需要清理的表列表（每张表须含 created_at 列）。
func (s *LogCleanupService) SetTables(tables []string) {
	if len(tables) > 0 {
		s.tables = tables
	}
}

// CleanupOldLogs 清理超过保留期的日志记录。
//
// 对配置的每张日志表执行 `DELETE FROM <table> WHERE created_at < $1`，
// 单张表失败仅记录日志，不影响其余表的清理。
func (s *LogCleanupService) CleanupOldLogs(ctx context.Context) error {
	if s.pool == nil {
		s.logger.Warn("log cleanup: database pool is nil, skip")
		return nil
	}
	cutoff := time.Now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	s.logger.Info("log cleanup: starting",
		"tables", s.tables, "retention_days", s.retentionDays, "cutoff", cutoff.Format(time.RFC3339))

	var totalDeleted int64
	for _, table := range s.tables {
		// 表名来自内部配置常量，非用户输入，拼接安全。
		query := "DELETE FROM " + table + " WHERE created_at < $1"
		tag, err := s.pool.Exec(ctx, query, cutoff)
		if err != nil {
			s.logger.Error("log cleanup: delete failed", "table", table, "error", err)
			continue
		}
		deleted := tag.RowsAffected()
		totalDeleted += deleted
		s.logger.Info("log cleanup: table done", "table", table, "deleted", deleted)
	}
	s.logger.Info("log cleanup: finished", "total_deleted", totalDeleted)
	return nil
}

// StartScheduledJobs 启动日志清理定时任务（每天 03:00 执行）。
// ctx 取消后任务退出。
func (s *LogCleanupService) StartScheduledJobs(ctx context.Context) {
	s.logger.Info("log cleanup scheduled job starting", "fire_at", "03:00 daily", "retention_days", s.retentionDays)
	go func() {
		for {
			next := nextCleanupFireAt(time.Now(), 3, 0)
			wait := time.Until(next)
			if wait <= 0 {
				wait = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				s.CleanupOldLogs(ctx)
			}
		}
	}()
}

// nextCleanupFireAt 返回从 from 起下一个 hh:mm 时刻（同天已过则顺延到次日）。
func nextCleanupFireAt(from time.Time, hour, minute int) time.Time {
	t := time.Date(from.Year(), from.Month(), from.Day(), hour, minute, 0, 0, from.Location())
	if !t.After(from) {
		t = t.Add(24 * time.Hour)
	}
	return t
}
