package service

import (
	"context"
	"errors"
	"time"

	"github.com/airport-panel/config"
	"github.com/airport-panel/node-service/internal/channelhealth"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

type HealthService struct {
	healthRepo      *repo.HealthRepo
	nodeRepo        *repo.NodeRepo
	serverRepo      *repo.ServerRepo
	runtimeRepo     *repo.RuntimeRepo
	channelHealth   channelHealthRecorder // 可选，避免循环依赖通过接口注入
}

// channelHealthRecorder 抽象 channelhealth.Service.RecordHeartbeat，避免直接依赖
type channelHealthRecorder interface {
	RecordHeartbeat(ctx context.Context, serverID uuid.UUID, runtimeID *uuid.UUID, hb *channelhealth.HeartbeatChannelHealth) error
}

func NewHealthService(healthRepo *repo.HealthRepo, nodeRepo *repo.NodeRepo, serverRepo *repo.ServerRepo, runtimeRepo *repo.RuntimeRepo) *HealthService {
	return &HealthService{
		healthRepo:  healthRepo,
		nodeRepo:    nodeRepo,
		serverRepo:  serverRepo,
		runtimeRepo: runtimeRepo,
	}
}

// SetChannelHealthRecorder 注入通道健康记录器（可选依赖）
func (s *HealthService) SetChannelHealthRecorder(rec channelHealthRecorder) {
	s.channelHealth = rec
}

func (s *HealthService) ReportHeartbeat(ctx context.Context, serverCode string, runtimeRef *string, req *model.AgentHeartbeatRequest) error {
	server, err := s.serverRepo.GetByCode(ctx, serverCode)
	if err != nil {
		return err
	}
	if server == nil {
		return ErrServerNotFound
	}

	now := time.Now()
	if err := s.serverRepo.UpdateHeartbeat(ctx, server.ID); err != nil {
		return err
	}

	// 保存系统metrics到 server.Metadata（CPU/内存/磁盘/网络等）
	if req.Metrics != nil && len(req.Metrics) > 0 {
		if server.Metadata == nil {
			server.Metadata = make(map[string]interface{})
		}
		// 将metrics合并到metadata的"system"键下，避免覆盖其他metadata
		sysMetrics := make(map[string]interface{})
		if existing, ok := server.Metadata["system"].(map[string]interface{}); ok {
			for k, v := range existing {
				sysMetrics[k] = v
			}
		}
		for k, v := range req.Metrics {
			sysMetrics[k] = v
		}
		sysMetrics["updated_at"] = now
		server.Metadata["system"] = sysMetrics

		// 更新OS/Arch信息（如果心跳中有）
		if req.OS != "" {
			server.Metadata["os"] = req.OS
		}
		if req.Arch != "" {
			server.Metadata["arch"] = req.Arch
		}
		if req.Pid > 0 {
			server.Metadata["agent_pid"] = req.Pid
		}
		if req.AgentVersion != "" {
			server.Metadata["agent_version"] = req.AgentVersion
		}
		if err := s.serverRepo.Update(ctx, server); err != nil {
			// metrics更新失败不阻断心跳主流程
			_ = err
		}
	}

	var runtimeID *uuid.UUID
	providerType := model.RuntimeProviderNodeAgent

	// 获取该server下所有node-agent runtimes（双核架构下有xray+sing-box两个）
	allRTs, err := s.runtimeRepo.ListByServer(ctx, server.ID)
	if err != nil {
		return err
	}
	var agentRTs []*model.Runtime
	for _, r := range allRTs {
		if r.ProviderType == providerType {
			agentRTs = append(agentRTs, r)
		}
	}

	// 更新所有agent runtime的心跳时间和状态（双核都活着，因为agent是单进程双内核）
	for _, r := range agentRTs {
		if err := s.runtimeRepo.UpdateHeartbeat(ctx, r.ID); err != nil {
			_ = err
		}
		if r.Status != model.RuntimeStatusActive {
			r.Status = model.RuntimeStatusActive
			if err := s.runtimeRepo.Update(ctx, r); err != nil {
				_ = err
			}
		}
		// 将system metrics写入每个runtime metadata（前端展示kernel内存用）
		if req.Metrics != nil && len(req.Metrics) > 0 {
			needsUpdate := false
			if r.Metadata == nil {
				r.Metadata = make(map[string]interface{})
			}
			if memUsed, ok := req.Metrics["mem_used_mb"]; ok {
				r.Metadata["memory_mb"] = memUsed
				needsUpdate = true
			}
			r.Metadata["last_metrics_at"] = now
			needsUpdate = true
			if needsUpdate {
				_ = s.runtimeRepo.Update(ctx, r)
			}
		}
	}

	// 根据 runtime_ref 找到本次心跳对应的目标 runtime，更新其版本号
	// 双核架构下，agent会分别为xray和sing-box发送心跳（带不同runtime_ref）
	var targetRT *model.Runtime
	if req.RuntimeRef != nil && *req.RuntimeRef != "" {
		ref := *req.RuntimeRef
		for _, r := range agentRTs {
			rRef := ""
			if r.ProviderRef != nil {
				rRef = *r.ProviderRef
			}
			if rRef == ref {
				targetRT = r
				break
			}
		}
	}

	// 主runtime：node-agent双核架构下，xray runtime是主配置通道（承载_singbox_config嵌入）
	// 优先找xray runtime（ProviderRef=nil的那个，或类型为xray的）
	var primaryRT *model.Runtime
	for _, r := range agentRTs {
		if r.ProviderRef == nil || *r.ProviderRef == "" {
			primaryRT = r
			break
		}
	}
	if primaryRT == nil {
		for _, r := range agentRTs {
			if normalizeRuntimeType(r.RuntimeType) == "xray" {
				primaryRT = r
				break
			}
		}
	}
	if primaryRT == nil && len(agentRTs) > 0 {
		primaryRT = agentRTs[0]
	}

	// 确定本次心跳要更新版本的runtime：有runtime_ref时更新对应runtime，否则更新primaryRT
	versionRT := targetRT
	if versionRT == nil {
		versionRT = primaryRT
	}
	if versionRT != nil {
		runtimeID = &versionRT.ID
		if req.RuntimeVersion != "" && (versionRT.RuntimeVersion == nil || *versionRT.RuntimeVersion != req.RuntimeVersion) {
			versionRT.RuntimeVersion = &req.RuntimeVersion
			_ = s.runtimeRepo.Update(ctx, versionRT)
		}
	} else if primaryRT != nil {
		runtimeID = &primaryRT.ID
	}

	// 记录通道健康（如果注入了 recorder）
	if s.channelHealth != nil && req.ChannelHealth != nil {
		hb := convertChannelHealth(req.ChannelHealth)
		if err := s.channelHealth.RecordHeartbeat(ctx, server.ID, runtimeID, hb); err != nil {
			_ = err
		}
	}

	// Machine模式+双核架构：查询该server下所有节点（覆盖xray+sing-box两个runtime的节点），
	// 更新所有节点的last_seen_at和健康状态
	nodes, err := s.nodeRepo.ListByServerID(ctx, server.ID)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		// P2: 更新 nodes.last_seen_at（SQL 层在线状态）
		if err := s.nodeRepo.UpdateLastSeenAt(ctx, node.ID); err != nil {
			// 非致命错误，仅记录日志，不阻断心跳主流程
			_ = err
		}

		currentStatus, err := s.healthRepo.GetStatusByNodeID(ctx, node.ID)
		if err != nil {
			return err
		}

		var previousStatus string
		newStatus := "healthy"
		if currentStatus != nil {
			previousStatus = currentStatus.OverallStatus
		}

		availabilityScore := 100
		latencyScore := 100
		lossScore := 100
		stabilityScore := 100

		if req.LossRatio != nil {
			lossScore = int(100 - (*req.LossRatio * 100))
			if lossScore < 0 {
				lossScore = 0
			}
			if *req.LossRatio > 0.1 {
				newStatus = "degraded"
			}
			if *req.LossRatio > 0.5 {
				newStatus = "offline"
			}
		}

		if req.RTTMs != nil {
			rtt := *req.RTTMs
			if rtt > 500 {
				latencyScore = 50
				if newStatus == "healthy" {
					newStatus = "degraded"
				}
			}
			if rtt > 1000 {
				latencyScore = 0
				newStatus = "offline"
			}
		}

		if req.ErrorMessage != nil && *req.ErrorMessage != "" {
			newStatus = "offline"
			availabilityScore = 0
		}

		healthStatus := &model.NodeHealthStatus{
			NodeID:             node.ID,
			OverallStatus:      newStatus,
			HeartbeatStatus:    "healthy",
			ProbeStatus:        currentStatusValue(currentStatus, "ProbeStatus", "unknown"),
			AvailabilityScore:  availabilityScore,
			LatencyScore:       latencyScore,
			LossScore:          lossScore,
			HandshakeScore:     currentStatusValueInt(currentStatus, "HandshakeScore", 100),
			ChainScore:         currentStatusValueInt(currentStatus, "ChainScore", 100),
			StabilityScore:     stabilityScore,
			CurrentRTTMs:       req.RTTMs,
			CurrentLossRatio:   req.LossRatio,
			CurrentOnlineUsers: req.OnlineUsers,
			CurrentCPUPercent:  req.CPUPercent,
			CurrentMemPercent:  req.MemPercent,
			CurrentDiskPercent: req.DiskPercent,
			LastHeartbeatAt:    &now,
		}

		if newStatus != previousStatus {
			stateChangedAt := now
			healthStatus.LastStateChangedAt = &stateChangedAt

			severity := model.HealthSeverityInfo
			if newStatus == "degraded" {
				severity = model.HealthSeverityWarning
			} else if newStatus == "offline" {
				severity = model.HealthSeverityCritical
			}

			event := &model.NodeHealthEvent{
				ID:         uuid.New(),
				NodeID:     node.ID,
				EventType:  "health_status_change",
				Severity:   severity,
				FromStatus: &previousStatus,
				ToStatus:   &newStatus,
				Metrics:    req.Metrics,
				Message:    req.ErrorMessage,
				OccurredAt: now,
			}
			if event.Metrics == nil {
				event.Metrics = make(map[string]interface{})
			}
			if err := s.healthRepo.RecordEvent(ctx, event); err != nil {
				return err
			}
		}

		if req.ErrorMessage != nil && *req.ErrorMessage != "" {
			healthStatus.LastErrorCode = strPtr("heartbeat_error")
			healthStatus.LastErrorMessage = req.ErrorMessage
		}

		if err := s.healthRepo.UpsertStatus(ctx, healthStatus); err != nil {
			return err
		}
	}

	return nil
}

func (s *HealthService) CalculateHealth(ctx context.Context, nodeID uuid.UUID) (*model.NodeHealthStatus, error) {
	status, err := s.healthRepo.GetStatusByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (s *HealthService) GetNodeHealth(ctx context.Context, nodeID uuid.UUID) (*model.NodeHealthStatus, error) {
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}

	status, err := s.healthRepo.GetStatusByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if status == nil {
		status = &model.NodeHealthStatus{
			NodeID:            nodeID,
			OverallStatus:     "unknown",
			HeartbeatStatus:   "unknown",
			ProbeStatus:       "unknown",
			AvailabilityScore: 0,
			LatencyScore:      0,
			LossScore:         0,
			HandshakeScore:    0,
			ChainScore:        0,
			StabilityScore:    0,
			UpdatedAt:         time.Now(),
		}
	}
	return status, nil
}

func (s *HealthService) ListHealthEvents(ctx context.Context, page, pageSize int, nodeID, eventType, severity string, startTime, endTime *time.Time) ([]*model.NodeHealthEvent, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.healthRepo.ListEvents(ctx, page, pageSize, nodeID, eventType, severity, startTime, endTime)
}

func currentStatusValue(s *model.NodeHealthStatus, field, defaultVal string) string {
	if s == nil {
		return defaultVal
	}
	switch field {
	case "ProbeStatus":
		if s.ProbeStatus == "" {
			return defaultVal
		}
		return s.ProbeStatus
	default:
		return defaultVal
	}
}

func currentStatusValueInt(s *model.NodeHealthStatus, field string, defaultVal int) int {
	if s == nil {
		return defaultVal
	}
	switch field {
	case "HandshakeScore":
		return s.HandshakeScore
	case "ChainScore":
		return s.ChainScore
	default:
		return defaultVal
	}
}

func strPtr(s string) *string {
	return &s
}

// convertChannelHealth 将 model.ChannelHealthReport 转换为 channelhealth.HeartbeatChannelHealth
func convertChannelHealth(r *model.ChannelHealthReport) *channelhealth.HeartbeatChannelHealth {
	hb := &channelhealth.HeartbeatChannelHealth{
		ActiveChannel: r.ActiveChannel,
		ChannelState:  r.ChannelState,
		RTTMs:         r.RTTMs,
		FailCount1h:   r.FailCount1h,
		OnlineUsers:   r.OnlineUsers,
		LastError:     r.LastError,
	}
	if r.Failover != nil {
		hb.Failover = &channelhealth.FailoverDetail{
			FromChannel: r.Failover.FromChannel,
			ToChannel:   r.Failover.ToChannel,
			Reason:      r.Failover.Reason,
		}
	}
	return hb
}

func MapHealthErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrHealthStatusNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrNodeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrServerNotFound):
		return config.CodeNotFound, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
