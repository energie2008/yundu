package service

import (
	"context"
	"log/slog"

	pb "github.com/airport-panel/proto/agent/v1"
)

// UserChangeEntry 描述一个增量用户变更条目（新增/修改用户）。
// 对应 agent 端 delta.UserChange，通过 proto DeltaUserChange 序列化传输。
type UserChangeEntry struct {
	Email      string            // 用户邮箱（同时作为唯一标识符）
	UUID       string            // 用户 UUID（VLESS/VMess 协议凭证）
	InboundTag string            // 目标 inbound tag（空则应用到所有匹配 inbound）
	Level      int               // 用户级别（0=默认）
	Password   string            // 密码（Trojan/Hy2/TUIC 等协议）
	Extra      map[string]string // 协议专有字段（flow/method 等）
}

// UserDeltaService 提供增量用户变更推送能力。
// 当用户增删/套餐变更时，构建轻量 DeltaSync 消息推送给 Agent，
// 替代全量 ConfigPush，实现 sub-second 级零断流热更。
type UserDeltaService struct {
	pusher *CompositeConfigPusher
	logger *slog.Logger
}

// NewUserDeltaService 创建 UserDeltaService 实例。
func NewUserDeltaService(pusher *CompositeConfigPusher, logger *slog.Logger) *UserDeltaService {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserDeltaService{pusher: pusher, logger: logger}
}

// OnUsersChanged 检测到用户变更后，构建 DeltaSync 消息并推送到指定 server 的 Agent。
// adds: 新增/修改的用户列表；removes: 待删除用户 email 列表；
// configVersion: 变更后的目标配置版本号。
// 推送失败不阻断业务（Agent 仍可通过心跳兜底全量同步）。
func (s *UserDeltaService) OnUsersChanged(ctx context.Context, serverCode string, kernel string, adds []UserChangeEntry, removes []string, configVersion int64) error {
	if s.pusher == nil {
		return nil
	}
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	if serverCode == "" {
		return nil
	}

	delta := &pb.DeltaSync{
		ServerCode:    serverCode,
		Kernel:        toKernelType(kernel),
		AddUsers:      toProtoUserChanges(adds),
		DelUsers:      removes,
		ConfigVersion: configVersion,
	}

	if err := s.pusher.PushUserDelta(ctx, serverCode, delta); err != nil {
		s.logger.Warn("OnUsersChanged: delta push failed, agent will fallback to full sync",
			"server_code", serverCode, "config_version", configVersion, "error", err)
		return err
	}
	return nil
}

// toProtoUserChanges 将 service 层 UserChangeEntry 列表转换为 proto DeltaUserChange 列表。
func toProtoUserChanges(entries []UserChangeEntry) []*pb.DeltaUserChange {
	if len(entries) == 0 {
		return nil
	}
	result := make([]*pb.DeltaUserChange, 0, len(entries))
	for _, e := range entries {
		result = append(result, &pb.DeltaUserChange{
			Email:      e.Email,
			Uuid:       e.UUID,
			InboundTag: e.InboundTag,
			Level:      int32(e.Level),
			Password:   e.Password,
			Extra:      e.Extra,
		})
	}
	return result
}

// toKernelType 将字符串内核类型转换为 proto KernelType 枚举。
func toKernelType(kernel string) pb.KernelType {
	switch kernel {
	case "xray", "xray-core":
		return pb.KernelType_KERNEL_TYPE_XRAY
	case "sing-box", "singbox":
		return pb.KernelType_KERNEL_TYPE_SINGBOX
	default:
		return pb.KernelType_KERNEL_TYPE_UNSPECIFIED
	}
}
