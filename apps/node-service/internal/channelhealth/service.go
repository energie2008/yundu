package channelhealth

import (
	"context"
	"log/slog"
	"time"

	"github.com/airport-panel/node-service/internal/metrics"
	"github.com/google/uuid"
)

// ============================================================================
// Service
// ============================================================================

// ChannelSwitcher 通道切换派发器（将切换指令下发到 node-agent）。
// app.go 注入实现：解析 serverID → machineID 后通过 grpcserver.PushToMachine 下发。
type ChannelSwitcher interface {
	SwitchChannel(ctx context.Context, serverID uuid.UUID, targetChannel string, reason string) error
}

type Service struct {
	repo     *Repo
	switcher ChannelSwitcher
	logger   *slog.Logger
}

func NewService(repo *Repo, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger.With("component", "channelhealth")}
}

// SetChannelSwitcher 注入通道切换派发器（用于 ManualSwitch 真实下发）
// 未注入时 ManualSwitch 退化为 "queued"（保留旧行为）。
func (s *Service) SetChannelSwitcher(sw ChannelSwitcher) {
	s.switcher = sw
}

// RecordHeartbeat 处理一次心跳上报的通道健康数据
// serverID: 服务器 ID；runtimeID: 可选 runtime ID；hb: 通道健康数据
func (s *Service) RecordHeartbeat(ctx context.Context, serverID uuid.UUID, runtimeID *uuid.UUID, hb *HeartbeatChannelHealth) error {
	if hb == nil {
		return nil
	}

	// 更新通道健康状态指标（1=healthy, 0=degraded, -1=unhealthy）
	metrics.ChannelHealthState.WithLabelValues(serverID.String(), hb.ActiveChannel, hb.ChannelState).
		Set(metrics.ChannelStateValue(hb.ChannelState))

	now := time.Now()
	cur := &ChannelHealthCurrent{
		ServerID:      serverID,
		RuntimeID:     runtimeID,
		ActiveChannel: hb.ActiveChannel,
		ChannelState:  hb.ChannelState,
		RTTMs:         hb.RTTMs,
		FailCount1h:   hb.FailCount1h,
		OnlineUsers:   hb.OnlineUsers,
		LastError:     hb.LastError,
	}

	snapshot := &ChannelHealthSnapshot{
		ServerID:      serverID,
		RuntimeID:     runtimeID,
		ActiveChannel: hb.ActiveChannel,
		ChannelState:  hb.ChannelState,
		RTTMs:         hb.RTTMs,
		FailCount1h:   hb.FailCount1h,
		OnlineUsers:   hb.OnlineUsers,
		LastError:     hb.LastError,
		ReportedAt:    now,
	}

	var failover *FailoverEvent
	if hb.Failover != nil {
		failoverAt := now
		cur.LastFailoverAt = &failoverAt
		cur.LastFailoverFrom = &hb.Failover.FromChannel
		cur.LastFailoverTo = &hb.Failover.ToChannel
		cur.LastFailoverReason = &hb.Failover.Reason

		failover = &FailoverEvent{
			ServerID:     serverID,
			RuntimeID:    runtimeID,
			FromChannel:  hb.Failover.FromChannel,
			ToChannel:    hb.Failover.ToChannel,
			Reason:       hb.Failover.Reason,
			OccurredAt:   failoverAt,
		}
		s.logger.Info("channel failover detected",
			"server_id", serverID,
			"from", hb.Failover.FromChannel,
			"to", hb.Failover.ToChannel,
			"reason", hb.Failover.Reason,
		)
	}

	return s.repo.UpsertCurrent(ctx, cur, snapshot, failover)
}

// ListCurrent 列出所有服务器当前通道健康
func (s *Service) ListCurrent(ctx context.Context, serverID *uuid.UUID, channelState string, page, pageSize int) ([]*ChannelHealthListItem, int, error) {
	return s.repo.ListCurrent(ctx, serverID, channelState, page, pageSize)
}

// ListFailoverEvents 列出降级事件
func (s *Service) ListFailoverEvents(ctx context.Context, serverID *uuid.UUID, reason string, startAt, endAt *time.Time, page, pageSize int) ([]*FailoverEvent, int, error) {
	return s.repo.ListFailoverEvents(ctx, serverID, reason, startAt, endAt, page, pageSize)
}

// ListSnapshots 列出某服务器的通道健康快照时间序列
func (s *Service) ListSnapshots(ctx context.Context, serverID uuid.UUID, limit int) ([]*ChannelHealthSnapshot, error) {
	return s.repo.ListSnapshots(ctx, serverID, limit)
}

// CleanupOldSnapshots 清理过期快照
func (s *Service) CleanupOldSnapshots(ctx context.Context, retention time.Duration) (int64, error) {
	before := time.Now().Add(-retention)
	return s.repo.DeleteOldSnapshots(ctx, before)
}

// ManualSwitch 手动切换通道
//
// 若注入了 ChannelSwitcher，则通过 gRPC 下发切换指令到 node-agent；
// 未注入时退化为 "queued"（保留旧行为）。
// 返回 ManualSwitchResponse 含 status（dispatched/queued/failed）与 message。
func (s *Service) ManualSwitch(ctx context.Context, serverID uuid.UUID, targetChannel string, reason string) (*ManualSwitchResponse, error) {
	resp := &ManualSwitchResponse{
		ServerID:      serverID,
		TargetChannel: targetChannel,
	}

	if s.switcher == nil {
		resp.Status = "queued"
		resp.Message = "switch request queued (ChannelSwitcher not injected)"
		s.logger.Warn("manual switch queued: switcher not configured",
			"server_id", serverID, "target_channel", targetChannel)
		return resp, nil
	}

	if err := s.switcher.SwitchChannel(ctx, serverID, targetChannel, reason); err != nil {
		resp.Status = "failed"
		resp.Message = "切换指令下发失败: " + err.Error()
		s.logger.Error("manual switch failed",
			"server_id", serverID, "target_channel", targetChannel, "error", err)
		return resp, err
	}

	resp.Status = "dispatched"
	resp.Message = "通道切换指令已下发到 node-agent"
	s.logger.Info("manual switch dispatched",
		"server_id", serverID, "target_channel", targetChannel, "reason", reason)
	return resp, nil
}
