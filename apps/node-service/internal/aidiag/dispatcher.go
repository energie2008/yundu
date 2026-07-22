package aidiag

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/airport-panel/node-service/internal/cert"
	"github.com/airport-panel/node-service/internal/grpcserver"
	"github.com/airport-panel/node-service/internal/repo"
	pb "github.com/airport-panel/proto/agent/v1"
)

// ============================================================================
// ActionDispatcher - 自动修复动作派发抽象
// ============================================================================
//
// ApplyAutofix 根据 LLM 建议的 suggestion.Action 将指令派发到具体执行器：
//   - restart_kernel / reload_config: 通过 gRPC MaintenanceCommand 下发到 node-agent
//   - renew_cert: 调用 cert.CertificateService 触发面板侧续期流程
//
// 设计动机：原 ApplyAutofix 仅标记 "queued" 不实际下发，导致自动修复形同虚设。
// 抽象出 ActionDispatcher 接口便于测试注入 mock，生产环境使用 GRPCDispatcher。

// ActionDispatcher 派发自动修复动作
type ActionDispatcher interface {
	RestartKernel(ctx context.Context, serverID uuid.UUID, reason string) error
	ReloadConfig(ctx context.Context, serverID uuid.UUID) error
	RenewCert(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID) error
}

// GRPCDispatcher 基于 gRPC 的 ActionDispatcher 实现
//
//   - RestartKernel/ReloadConfig: 通过 MaintenanceCommand 下发到 node-agent
//   - RenewCert: 调用 cert.CertificateService.TriggerRenew 触发面板侧续期
//     （当 ACMEClient 已注入时执行真实 ACME 续期，否则仅置 renew_status=pending）
type GRPCDispatcher struct {
	agentServer *grpcserver.AgentServer
	serverRepo  *repo.ServerRepo
	certService *cert.CertificateService
	logger      *slog.Logger
}

// NewGRPCDispatcher 构造 gRPC 派发器
// serverRepo 用于将 serverID 解析为 machineID（即 Server.Code，grpcserver.sessions 的 key）
func NewGRPCDispatcher(agentServer *grpcserver.AgentServer, serverRepo *repo.ServerRepo, certService *cert.CertificateService, logger *slog.Logger) *GRPCDispatcher {
	return &GRPCDispatcher{
		agentServer: agentServer,
		serverRepo:  serverRepo,
		certService: certService,
		logger:      logger.With("component", "aidiag-dispatcher"),
	}
}

// resolveMachineID 将 serverID 解析为 grpcserver sessions 中使用的 machineID（Server.Code）
func (d *GRPCDispatcher) resolveMachineID(ctx context.Context, serverID uuid.UUID) (string, error) {
	srv, err := d.serverRepo.GetByID(ctx, serverID)
	if err != nil {
		return "", fmt.Errorf("query server %s: %w", serverID, err)
	}
	if srv == nil {
		return "", fmt.Errorf("server %s not found", serverID)
	}
	if srv.Code == "" {
		return "", fmt.Errorf("server %s has empty code (machineID)", serverID)
	}
	return srv.Code, nil
}

// pushMaintenance 构造 MaintenanceCommand 并下发到指定 server 的 node-agent
func (d *GRPCDispatcher) pushMaintenance(ctx context.Context, serverID uuid.UUID, action pb.MaintenanceCommand_Action, reason string, drainTimeout int64) error {
	if d.agentServer == nil {
		return fmt.Errorf("agent server not configured (gRPC server unavailable)")
	}
	machineID, err := d.resolveMachineID(ctx, serverID)
	if err != nil {
		return err
	}
	msg := &pb.PanelMessage{
		Payload: &pb.PanelMessage_Maintenance{
			Maintenance: &pb.MaintenanceCommand{
				Action:              action,
				Reason:              reason,
				DrainTimeoutSeconds: drainTimeout,
			},
		},
	}
	if err := d.agentServer.PushToMachine(machineID, msg); err != nil {
		d.logger.Error("push maintenance command failed",
			"machine_id", machineID, "action", action, "reason", reason, "error", err)
		return fmt.Errorf("push to machine %s: %w", machineID, err)
	}
	d.logger.Info("maintenance command dispatched",
		"machine_id", machineID, "action", action, "reason", reason, "server_id", serverID)
	return nil
}

// RestartKernel 立即重启内核（ACTION_RESTART）
func (d *GRPCDispatcher) RestartKernel(ctx context.Context, serverID uuid.UUID, reason string) error {
	return d.pushMaintenance(ctx, serverID, pb.MaintenanceCommand_ACTION_RESTART, reason, 0)
}

// ReloadConfig 重载配置
//
// xray/sing-box 内核热重载支持有限，绝大多数配置变更需要重启内核进程才能生效，
// 因此这里复用 ACTION_RESTART 并以 reason 区分语义。
// 后续若 agent 支持 SIGHUP 热重载，可改为下发专用指令。
func (d *GRPCDispatcher) ReloadConfig(ctx context.Context, serverID uuid.UUID) error {
	return d.pushMaintenance(ctx, serverID, pb.MaintenanceCommand_ACTION_RESTART,
		"config reload requested by AI diagnosis", 0)
}

// RenewCert 触发证书续期
//
// 调用 cert.CertificateService.TriggerRenew 触发面板侧续期。
// 当 ACMEClient 已注入时，TriggerRenew 会通过 lego 执行真实 ACME 续期
// 并更新 cert_pem / key_pem / expires_at；否则仅置 renew_status=pending。
// 当前为粗粒度触发：对全平台 30 天内将到期的证书批量续期。
func (d *GRPCDispatcher) RenewCert(ctx context.Context, serverID uuid.UUID, nodeID *uuid.UUID) error {
	if d.certService == nil {
		return fmt.Errorf("cert service not configured")
	}
	expiring, err := d.certService.ListExpiringSoon(ctx, 30)
	if err != nil {
		return fmt.Errorf("list expiring certs: %w", err)
	}
	triggered := 0
	for _, c := range expiring {
		if _, err := d.certService.TriggerRenew(ctx, c.ID); err != nil {
			d.logger.Warn("trigger renew failed",
				"cert_id", c.ID, "code", c.Code, "error", err)
			continue
		}
		triggered++
	}
	d.logger.Info("cert renewal dispatched",
		"server_id", serverID, "triggered_count", triggered, "expiring_total", len(expiring))
	if triggered == 0 {
		return fmt.Errorf("no certs renewed (expiring_total=%d)", len(expiring))
	}
	return nil
}
