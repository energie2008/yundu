package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	pb "github.com/airport-panel/proto/agent/v1"
)

// config_pusher.go 实现 P0-7：WS/gRPC 实时推送 sync.config。
//
// 设计：
//  1. ConfigPusher 接口：抽象配置推送能力，由 app.go 注入具体实现
//  2. CompositeConfigPusher：fan-out 到 gRPC AgentServer + WS Handler
//  3. 在 GetRuntimeConfig 创建新版本后调用 PushConfig，将版本号推送给 agent
//  4. agent 端 handleConfigPush 已就绪，收到后通过 HTTP 拉取完整配置
//  5. 保留版本号心跳作为断线兜底（10 秒级延迟）

// ConfigPusher 配置推送接口（P0-7）
// 由 grpcserver.AgentServer 和 handler.WSHandler 实现 PushToMachine 方法
type ConfigPusher interface {
	PushToMachine(machineID string, msg *pb.PanelMessage) error
}

// CompositeConfigPusher 组合推送器（P0-7）
// fan-out 到多个推送通道（gRPC stream + WS connection），任一成功即认为推送成功。
// gRPC 不在线时 WS 兜底，反之亦然。
type CompositeConfigPusher struct {
	pushers []ConfigPusher
	logger  *slog.Logger
}

func NewCompositeConfigPusher(logger *slog.Logger, pushers ...ConfigPusher) *CompositeConfigPusher {
	if logger == nil {
		logger = slog.Default()
	}
	return &CompositeConfigPusher{pushers: pushers, logger: logger}
}

// PushConfig 构建并推送 ConfigPush 消息到指定 machine
func (p *CompositeConfigPusher) PushConfig(ctx context.Context, serverCode string, cv *model.ConfigVersion) error {
	if cv == nil || serverCode == "" {
		return nil
	}

	msg := &pb.PanelMessage{
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.PanelMessage_ConfigPush{
			ConfigPush: &pb.ConfigPush{
				Version:   cv.VersionNo,
				ConfigHash: cv.ContentHash,
				// agent 端 handleConfigPush 仅用 version 字段触发 applyConfig，
				// applyConfig 会通过 HTTP GET /api/v1/agent/config?version=X 拉取完整配置
				// 因此 nodes/global_config 字段暂不填充（最小化推送，避免双通道数据不一致）
			},
		},
	}

	// fan-out：任一通道成功即返回，全部失败则返回最后一个错误
	var lastErr error
	for _, pusher := range p.pushers {
		if err := pusher.PushToMachine(serverCode, msg); err != nil {
			p.logger.Debug("config push failed on one channel",
				"server_code", serverCode, "version", cv.VersionNo, "error", err)
			lastErr = err
			continue
		}
		p.logger.Info("config pushed to agent",
			"server_code", serverCode, "version", cv.VersionNo, "content_hash", cv.ContentHash[:8])
		return nil
	}
	return lastErr
}

// PushUserBan P0-8: 推送用户封禁通知到指定 agent。
// agent 收到后立即拉取最新配置（已封禁用户已从配置中移除），
// 替代 10s 心跳轮询的全量重载，实现 sub-second 级用户封禁生效。
// P1 将升级为 xray gRPC AlterInbound 真增量更新（无需重载）。
func (p *CompositeConfigPusher) PushUserBan(ctx context.Context, serverCode string, userIDs []string, reason string) error {
	if len(userIDs) == 0 || serverCode == "" {
		return nil
	}

	msg := &pb.PanelMessage{
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.PanelMessage_UserBan{
			UserBan: &pb.UserBanNotice{
				UserIds:   userIDs,
				Reason:    reason,
				Timestamp: time.Now().Unix(),
			},
		},
	}

	var lastErr error
	for _, pusher := range p.pushers {
		if err := pusher.PushToMachine(serverCode, msg); err != nil {
			p.logger.Debug("user ban push failed on one channel",
				"server_code", serverCode, "user_count", len(userIDs), "error", err)
			lastErr = err
			continue
		}
		p.logger.Info("user ban pushed to agent",
			"server_code", serverCode, "user_count", len(userIDs), "reason", reason)
		return nil
	}
	return lastErr
}

// PushUserDelta 推送增量用户变更（Delta Sync）到指定 agent。
// 替代全量 ConfigPush，实现用户增删的 sub-second 级零断流热更。
// Agent 收到后通过 RuntimePlugin.UpdateUsers 直接增删用户，无需重载内核。
func (p *CompositeConfigPusher) PushUserDelta(ctx context.Context, serverCode string, delta *pb.DeltaSync) error {
	if delta == nil || serverCode == "" {
		return nil
	}

	msg := &pb.PanelMessage{
		Timestamp: time.Now().UnixMilli(),
		Payload: &pb.PanelMessage_DeltaSync{
			DeltaSync: delta,
		},
	}

	var lastErr error
	for _, pusher := range p.pushers {
		if err := pusher.PushToMachine(serverCode, msg); err != nil {
			p.logger.Debug("user delta push failed on one channel",
				"server_code", serverCode, "config_version", delta.ConfigVersion, "error", err)
			lastErr = err
			continue
		}
		p.logger.Info("user delta pushed to agent",
			"server_code", serverCode,
			"config_version", delta.ConfigVersion,
			"add_users", len(delta.AddUsers),
			"del_users", len(delta.DelUsers))
		return nil
	}
	return lastErr
}

// PushUserBanToAllServers P0-8: 向所有已连接的 agent 推送用户封禁通知。
// 用于用户封禁后通知所有节点立即更新配置。
func (p *CompositeConfigPusher) PushUserBanToAllServers(ctx context.Context, userIDs []string, reason string) {
	// 由于 CompositeConfigPusher 不持有 server 列表，此方法需由调用方遍历 server 列表
	// 逐个调用 PushUserBan。这里保留接口供未来扩展。
	// 当前由 DeploymentService.PushUserBanToAllRuntimes 实现。
}
