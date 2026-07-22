package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/airport-panel/config"
	"github.com/airport-panel/node-service/internal/model"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

type RuntimeService struct {
	runtimeRepo *repo.RuntimeRepo
	serverRepo  *repo.ServerRepo
}

func NewRuntimeService(runtimeRepo *repo.RuntimeRepo, serverRepo *repo.ServerRepo) *RuntimeService {
	return &RuntimeService{
		runtimeRepo: runtimeRepo,
		serverRepo:  serverRepo,
	}
}

func (s *RuntimeService) RegisterRuntime(ctx context.Context, serverCode string, req *model.RegisterRuntimeRequest) (*model.Runtime, error) {
	server, err := s.serverRepo.GetByCode(ctx, serverCode)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	primaryRT, err := s.registerRuntimeInternal(ctx, server, req)
	if err != nil {
		return nil, err
	}

	// 双核自动补齐：node-agent provider 注册时，自动确保配对内核 runtime 也存在
	// node-agent 是双内核架构（xray+sing-box 并行），需要两个 runtime 记录
	providerType := req.ProviderType
	if providerType == "" {
		providerType = model.RuntimeProviderNodeAgent
	}
	if providerType == model.RuntimeProviderNodeAgent {
		s.ensureDualRuntime(ctx, server, req, primaryRT)
	}

	// 更新 server 元数据（OS/Arch/AgentVersion等）
	s.updateServerMetadata(ctx, server, req)

	if err := s.serverRepo.UpdateHeartbeat(ctx, server.ID); err != nil {
		return nil, err
	}

	return primaryRT, nil
}

// ensureDualRuntime 双核架构：主runtime注册后，自动创建/更新配对runtime
func (s *RuntimeService) ensureDualRuntime(ctx context.Context, server *model.Server, req *model.RegisterRuntimeRequest, primaryRT *model.Runtime) {
	primaryType := normalizeRuntimeType(req.RuntimeType)
	secondaryType := ""
	var secondaryAPIPort *int
	switch primaryType {
	case "xray":
		secondaryType = "sing-box"
		secondaryAPIPort = req.SingboxClashPort
	case "sing-box":
		secondaryType = "xray"
		secondaryAPIPort = req.XrayAPIPort
	default:
		return // 非标准内核类型，不自动创建配对
	}

	// 用 providerRef = secondaryType 查找配对 runtime 是否已存在
	secondaryRef := secondaryType
	existing, err := s.runtimeRepo.GetByServerAndProvider(ctx, server.ID, model.RuntimeProviderNodeAgent, &secondaryRef)
	if err != nil {
		slog.Warn("ensureDualRuntime: lookup secondary runtime failed", "server", server.Code, "type", secondaryType, "error", err)
		return
	}
	if existing != nil {
		existing.RuntimeType = secondaryType
		updated := false
		if secondaryAPIPort != nil && (existing.APIPort == nil || *existing.APIPort != *secondaryAPIPort) {
			existing.APIPort = secondaryAPIPort
			updated = true
		}
		if existing.Status != model.RuntimeStatusActive {
			existing.Status = model.RuntimeStatusActive
			updated = true
		}
		// 注意：不要用主runtime的req.RuntimeVersion覆盖secondary runtime的版本
		// 配对runtime的版本应由其自身的注册/心跳请求更新
		if updated {
			if err := s.runtimeRepo.Update(ctx, existing); err != nil {
				slog.Warn("ensureDualRuntime: update secondary runtime failed", "error", err)
			}
		}
		return
	}

	// 创建配对 runtime（版本先留空，等待sing-box/xray自身注册时更新）
	secondaryRT := &model.Runtime{
		ID:                  uuid.New(),
		ServerID:            server.ID,
		RuntimeType:         secondaryType,
		RuntimeVersion:      nil,
		ProviderType:        model.RuntimeProviderNodeAgent,
		ProviderRef:         &secondaryRef,
		ListenHost:          req.ListenHost,
		APIPort:             secondaryAPIPort,
		Status:              model.RuntimeStatusActive,
		Capabilities:        map[string]interface{}{},
		ConfigSchemaVersion: req.ConfigSchemaVersion,
		Metadata:            map[string]interface{}{"auto_created": true, "paired_with": primaryRT.ID.String()},
	}
	if err := s.runtimeRepo.Create(ctx, secondaryRT); err != nil {
		slog.Warn("ensureDualRuntime: create secondary runtime failed",
			"server", server.Code, "type", secondaryType, "error", err)
		return
	}
	slog.Info("dual-kernel runtime auto-created",
		"server", server.Code, "primary", primaryType, "primary_id", primaryRT.ID,
		"secondary", secondaryType, "secondary_id", secondaryRT.ID)
}

// updateServerMetadata 注册时更新服务器元数据
func (s *RuntimeService) updateServerMetadata(ctx context.Context, server *model.Server, req *model.RegisterRuntimeRequest) {
	needsUpdate := false
	if server.Metadata == nil {
		server.Metadata = make(map[string]interface{})
		needsUpdate = true
	}
	if req.Hostname != "" {
		server.Metadata["hostname"] = req.Hostname
		needsUpdate = true
	}
	if req.OS != "" {
		server.Metadata["os"] = req.OS
		needsUpdate = true
	}
	if req.Arch != "" {
		server.Metadata["arch"] = req.Arch
		needsUpdate = true
	}
	if req.AgentVersion != "" {
		server.Metadata["agent_version"] = req.AgentVersion
		needsUpdate = true
	}
	// 保存双内核API端口到server metadata
	if req.XrayAPIPort != nil {
		server.Metadata["xray_api_port"] = *req.XrayAPIPort
		needsUpdate = true
	}
	if req.SingboxClashPort != nil {
		server.Metadata["singbox_clash_port"] = *req.SingboxClashPort
		needsUpdate = true
	}
	if needsUpdate {
		if err := s.serverRepo.Update(ctx, server); err != nil {
			slog.Warn("updateServerMetadata failed", "error", err)
		}
	}
}

// normalizeRuntimeType 规范化 runtime_type 字符串
func normalizeRuntimeType(rt string) string {
	switch rt {
	case "xray-core", "xray":
		return "xray"
	case "singbox", "sing-box":
		return "sing-box"
	}
	return rt
}

func (s *RuntimeService) RegisterRuntimeByServerID(ctx context.Context, serverID uuid.UUID, req *model.RegisterRuntimeRequest) (*model.Runtime, error) {
	server, err := s.serverRepo.GetByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	return s.registerRuntimeInternal(ctx, server, req)
}

func (s *RuntimeService) registerRuntimeInternal(ctx context.Context, server *model.Server, req *model.RegisterRuntimeRequest) (*model.Runtime, error) {
	providerType := req.ProviderType
	if providerType == "" {
		providerType = model.RuntimeProviderNodeAgent
	}

	normalizedType := normalizeRuntimeType(req.RuntimeType)

	existing, err := s.runtimeRepo.GetByServerAndProvider(ctx, server.ID, providerType, req.ProviderRef)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		existing.RuntimeType = normalizedType
		if req.RuntimeVersion != nil {
			existing.RuntimeVersion = req.RuntimeVersion
		}
		if req.ListenHost != nil {
			existing.ListenHost = req.ListenHost
		}
		if req.APIPort != nil {
			existing.APIPort = req.APIPort
		}
		if req.Capabilities != nil {
			existing.Capabilities = req.Capabilities
		}
		if req.Metadata != nil {
			existing.Metadata = req.Metadata
		}
		if req.ConfigSchemaVersion != "" {
			existing.ConfigSchemaVersion = req.ConfigSchemaVersion
		}
		existing.Status = model.RuntimeStatusActive
		if err := s.runtimeRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	if req.Capabilities == nil {
		req.Capabilities = make(map[string]interface{})
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	if req.ConfigSchemaVersion == "" {
		req.ConfigSchemaVersion = "v1"
	}

	runtime := &model.Runtime{
		ID:                  uuid.New(),
		ServerID:            server.ID,
		RuntimeType:         normalizedType,
		RuntimeVersion:      req.RuntimeVersion,
		ProviderType:        providerType,
		ProviderRef:         req.ProviderRef,
		ListenHost:          req.ListenHost,
		APIPort:             req.APIPort,
		Status:              model.RuntimeStatusActive,
		Capabilities:        req.Capabilities,
		ConfigSchemaVersion: req.ConfigSchemaVersion,
		Metadata:            req.Metadata,
	}

	if err := s.runtimeRepo.Create(ctx, runtime); err != nil {
		return nil, err
	}

	return runtime, nil
}

func (s *RuntimeService) ListRuntimes(ctx context.Context, serverID uuid.UUID) ([]*model.Runtime, error) {
	return s.runtimeRepo.ListByServer(ctx, serverID)
}

func (s *RuntimeService) UpdateRuntimeHeartbeat(ctx context.Context, id uuid.UUID) error {
	return s.runtimeRepo.UpdateHeartbeat(ctx, id)
}

func (s *RuntimeService) GetRuntimeByID(ctx context.Context, id uuid.UUID) (*model.Runtime, error) {
	runtime, err := s.runtimeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, ErrRuntimeNotFound
	}
	return runtime, nil
}

func (s *RuntimeService) GetRuntimeByServerAndProvider(ctx context.Context, serverID uuid.UUID, providerType model.RuntimeProviderType, providerRef *string) (*model.Runtime, error) {
	rt, err := s.runtimeRepo.GetByServerAndProvider(ctx, serverID, providerType, providerRef)
	if err != nil {
		return nil, err
	}
	if rt != nil {
		return rt, nil
	}
	if providerRef == nil {
		runtimes, err := s.runtimeRepo.ListByServer(ctx, serverID)
		if err != nil {
			return nil, err
		}
		var xrayRT *model.Runtime
		var firstRT *model.Runtime
		for _, r := range runtimes {
			if r.ProviderType != providerType {
				continue
			}
			if firstRT == nil {
				firstRT = r
			}
			if normalizeRuntimeType(r.RuntimeType) == "xray" {
				xrayRT = r
				break
			}
		}
		if xrayRT != nil {
			return xrayRT, nil
		}
		if firstRT != nil {
			return firstRT, nil
		}
	}
	return nil, ErrRuntimeNotFound
}

func MapRuntimeErrorToCode(err error) (config.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrRuntimeNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrServerNotFound):
		return config.CodeNotFound, err.Error()
	case errors.Is(err, ErrServerIDRequired):
		return config.CodeBadRequest, err.Error()
	case errors.Is(err, ErrRuntimeTypeRequired):
		return config.CodeBadRequest, err.Error()
	default:
		return config.CodeInternalError, ""
	}
}
