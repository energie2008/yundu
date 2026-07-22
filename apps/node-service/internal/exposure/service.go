package exposure

import (
	"context"
	"log/slog"

	"github.com/airport-panel/node-service/internal/cert"
	"github.com/airport-panel/node-service/internal/metrics"
	"github.com/google/uuid"
)

// NodeInfoFetcher 用于在 preview/validate 时按 server_id 查询节点协议信息。
// app.go 注入实现（封装对 node repo 的查询），避免 exposure 直接依赖 node repo。
type NodeInfoFetcher interface {
	FetchByServerID(ctx context.Context, serverID uuid.UUID) (*NodeInfo, error)
}

// CertFetcher 用于按 tls_profile_id 查询关联的 TLSProfile（暴露给 renderer）
type CertFetcher interface {
	FetchProfile(ctx context.Context, profileID uuid.UUID) (*cert.TLSProfile, error)
}

// ExposureStore 抽象 ExposureRepo 的数据访问（便于测试注入 mock）
type ExposureStore interface {
	Create(ctx context.Context, e *EdgeExposure) error
	GetByID(ctx context.Context, id uuid.UUID) (*EdgeExposure, error)
	GetByCode(ctx context.Context, code string) (*EdgeExposure, error)
	GetByServerID(ctx context.Context, serverID uuid.UUID) (*EdgeExposure, error)
	Update(ctx context.Context, e *EdgeExposure) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, page, pageSize int, status string) ([]*EdgeExposure, int, error)
}

// NginxConfigStore 抽象 NginxConfigRepo
type NginxConfigStore interface {
	GetByExposureAndHash(ctx context.Context, exposureID uuid.UUID, hash string) (*NginxGeneratedConfig, error)
	CreateIfAbsent(ctx context.Context, c *NginxGeneratedConfig) (*NginxGeneratedConfig, error)
}

// CompatRuleStore 抽象 CompatRuleRepo
type CompatRuleStore interface {
	FindMatch(ctx context.Context, protocol, transport, security, exposureMode string) (*ExposureCompatRule, error)
}

// AuditWriter 预留的审计日志接口
type AuditWriter interface {
	Audit(ctx context.Context, action, resource string, before, after interface{})
}

type ExposureService struct {
	store         ExposureStore
	nginxStore    NginxConfigStore
	compatStore   CompatRuleStore
	nodeFetcher   NodeInfoFetcher
	certFetcher   CertFetcher
	audit         AuditWriter
	logger        *slog.Logger
}

func NewExposureService(
	store ExposureStore,
	nginxStore NginxConfigStore,
	compatStore CompatRuleStore,
	nodeFetcher NodeInfoFetcher,
	certFetcher CertFetcher,
	audit AuditWriter,
	logger *slog.Logger,
) *ExposureService {
	return &ExposureService{
		store:       store,
		nginxStore:  nginxStore,
		compatStore: compatStore,
		nodeFetcher: nodeFetcher,
		certFetcher: certFetcher,
		audit:       audit,
		logger:      logger,
	}
}

func (s *ExposureService) Create(ctx context.Context, req *CreateExposureRequest) (*EdgeExposure, error) {
	existing, err := s.store.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrExposureAlreadyExists
	}

	publicPort := 443
	if req.PublicPort != nil {
		publicPort = *req.PublicPort
	}
	originHost := req.OriginHost
	if originHost == "" {
		originHost = "127.0.0.1"
	}
	cfProtocol := req.CFProtocol
	if cfProtocol == "" {
		cfProtocol = "auto"
	}

	nginxEnabled := false
	if req.NginxEnabled != nil {
		nginxEnabled = *req.NginxEnabled
	}
	cfNoTLSVerify := false
	if req.CFNoTLSVerify != nil {
		cfNoTLSVerify = *req.CFNoTLSVerify
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	e := &EdgeExposure{
		ID:                      uuid.New(),
		ServerID:                req.ServerID,
		Code:                    req.Code,
		Name:                    req.Name,
		ExposureMode:            req.ExposureMode,
		PublicHostname:          req.PublicHostname,
		PublicPort:              publicPort,
		OriginHost:              originHost,
		OriginPort:              req.OriginPort,
		NginxEnabled:            nginxEnabled,
		NginxWSPath:             req.NginxWSPath,
		NginxHostHeader:         req.NginxHostHeader,
		NginxExtraConf:          req.NginxExtraConf,
		TLSProfileID:            req.TLSProfileID,
		CFTunnelTokenEncrypted:  req.CFTunnelTokenEncrypted,
		CFTunnelID:              req.CFTunnelID,
		CFTunnelName:            req.CFTunnelName,
		CFProtocol:              cfProtocol,
		CFNoTLSVerify:           cfNoTLSVerify,
		CFOriginServerName:      req.CFOriginServerName,
		ArgoWSTokenEncrypted:    req.ArgoWSTokenEncrypted,
		Status:                  "pending",
		HealthCheckURL:          req.HealthCheckURL,
		Metadata:                metadata,
	}

	if err := s.store.Create(ctx, e); err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "create", "edge_exposure", nil, e)
	}
	return e, nil
}

func (s *ExposureService) GetByID(ctx context.Context, id uuid.UUID) (*EdgeExposure, error) {
	e, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrExposureNotFound
	}
	return e, nil
}

func (s *ExposureService) GetByServerID(ctx context.Context, serverID uuid.UUID) (*EdgeExposure, error) {
	e, err := s.store.GetByServerID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrExposureNotFound
	}
	return e, nil
}

func (s *ExposureService) List(ctx context.Context, page, pageSize int, status string) ([]*EdgeExposure, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize, status)
}

func (s *ExposureService) Update(ctx context.Context, id uuid.UUID, req *UpdateExposureRequest) (*EdgeExposure, error) {
	e, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrExposureNotFound
	}

	before := *e

	if req.Name != nil {
		e.Name = *req.Name
	}
	if req.ExposureMode != nil {
		e.ExposureMode = *req.ExposureMode
	}
	if req.PublicHostname != nil {
		e.PublicHostname = req.PublicHostname
	}
	if req.PublicPort != nil {
		e.PublicPort = *req.PublicPort
	}
	if req.OriginHost != nil {
		e.OriginHost = *req.OriginHost
	}
	if req.OriginPort != nil {
		e.OriginPort = *req.OriginPort
	}
	if req.NginxEnabled != nil {
		e.NginxEnabled = *req.NginxEnabled
	}
	if req.NginxWSPath != nil {
		e.NginxWSPath = req.NginxWSPath
	}
	if req.NginxHostHeader != nil {
		e.NginxHostHeader = req.NginxHostHeader
	}
	if req.NginxExtraConf != nil {
		e.NginxExtraConf = req.NginxExtraConf
	}
	if req.TLSProfileID != nil {
		e.TLSProfileID = req.TLSProfileID
	}
	if req.CFProtocol != nil {
		e.CFProtocol = *req.CFProtocol
	}
	if req.CFNoTLSVerify != nil {
		e.CFNoTLSVerify = *req.CFNoTLSVerify
	}
	if req.CFOriginServerName != nil {
		e.CFOriginServerName = req.CFOriginServerName
	}
	if req.Status != nil {
		e.Status = *req.Status
	}
	if req.HealthCheckURL != nil {
		e.HealthCheckURL = req.HealthCheckURL
	}
	if req.Metadata != nil {
		e.Metadata = req.Metadata
	}

	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "update", "edge_exposure", before, e)
	}
	return e, nil
}

func (s *ExposureService) Delete(ctx context.Context, id uuid.UUID) error {
	e, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrExposureNotFound
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "delete", "edge_exposure", e, nil)
	}
	return nil
}

// Preview 渲染 nginx_conf + cloudflared_yml + explanation
func (s *ExposureService) Preview(ctx context.Context, id uuid.UUID) (*PreviewResponse, error) {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var profile *cert.TLSProfile
	if e.TLSProfileID != nil && s.certFetcher != nil {
		profile, err = s.certFetcher.FetchProfile(ctx, *e.TLSProfileID)
		if err != nil {
			return nil, err
		}
	}

	nginxConf, err := RenderNginxServerBlock(e, profile)
	if err != nil {
		return nil, err
	}
	cfYML, err := RenderCloudflaredYAML(e)
	if err != nil {
		return nil, err
	}

	// 持久化渲染结果（同 exposure_id + hash 已存在则不重复插入）
	if s.nginxStore != nil {
		confRecord := &NginxGeneratedConfig{
			ID:            uuid.New(),
			ExposureID:    e.ID,
			ConfigContent: nginxConf,
			ConfigHash:    HashNginxConf(nginxConf),
			SchemaVersion: "v1",
		}
		if _, err := s.nginxStore.CreateIfAbsent(ctx, confRecord); err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to persist nginx config", "error", err, "exposure_id", e.ID)
			}
		}
	}

	return &PreviewResponse{
		NginxConf:      nginxConf,
		CloudflaredYML: cfYML,
		Explanation:    Explanation(e),
	}, nil
}

// Validate 调用 exposure_compat_rules 预检
func (s *ExposureService) Validate(ctx context.Context, id uuid.UUID) (*ValidateResponse, error) {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.nodeFetcher == nil {
		return nil, ErrNodeInfoMissing
	}
	info, err := s.nodeFetcher.FetchByServerID(ctx, e.ServerID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, ErrNodeInfoMissing
	}

	rule, err := s.compatStore.FindMatch(ctx, info.ProtocolType, info.TransportType, info.SecurityType, e.ExposureMode)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		// 没有匹配规则：默认允许（无约束）
		return &ValidateResponse{IsAllowed: true, Reason: ""}, nil
	}
	reason := ""
	if rule.Reason != nil {
		reason = *rule.Reason
	}
	return &ValidateResponse{IsAllowed: rule.IsAllowed, Reason: reason}, nil
}

// Apply 应用 exposure 配置：渲染 nginx/cloudflared 配置并持久化，更新状态并写审计日志
//
// dryRun=true: 仅渲染并返回预览，不更新状态、不写审计（用于变更前预检）
// dryRun=false: 执行 validate → render → persist nginx config → status(pending→applying→applied) → audit_log
//
// 状态跟踪：pending → applying → applied（成功）/ failed（渲染或持久化失败）
// 审计日志记录 before/after + nginx_config_hash，便于追溯变更
func (s *ExposureService) Apply(ctx context.Context, id uuid.UUID, dryRun bool) (*ApplyResponse, error) {
	e, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *e

	// 1. 渲染配置（dry-run 与真实 apply 都需要）
	var profile *cert.TLSProfile
	if e.TLSProfileID != nil && s.certFetcher != nil {
		profile, err = s.certFetcher.FetchProfile(ctx, *e.TLSProfileID)
		if err != nil {
			return nil, err
		}
	}
	nginxConf, err := RenderNginxServerBlock(e, profile)
	if err != nil {
		return nil, err
	}
	cfYML, err := RenderCloudflaredYAML(e)
	if err != nil {
		return nil, err
	}
	configHash := HashNginxConf(nginxConf)

	// 2. dry-run：仅返回预览，不更新状态、不写审计
	if dryRun {
		return &ApplyResponse{
			Exposure:        exposureResponsePtr(e),
			DryRun:          true,
			NginxConf:       nginxConf,
			CloudflaredYML:  cfYML,
			NginxConfigHash: configHash,
			Status:          "dry_run",
			Message:         "dry-run 预览，未更新状态",
		}, nil
	}

	// 3. 真实 apply：先 validate（compat rules 预检）
	var validateResult *ValidateResponse
	if s.nodeFetcher != nil && s.compatStore != nil {
		info, ferr := s.nodeFetcher.FetchByServerID(ctx, e.ServerID)
		if ferr == nil && info != nil {
			rule, ferr := s.compatStore.FindMatch(ctx, info.ProtocolType, info.TransportType, info.SecurityType, e.ExposureMode)
			if ferr == nil && rule != nil {
				reason := ""
				if rule.Reason != nil {
					reason = *rule.Reason
				}
				validateResult = &ValidateResponse{IsAllowed: rule.IsAllowed, Reason: reason}
				if !rule.IsAllowed {
					// 兼容性检查不通过：标记 failed 并写审计
					e.Status = "failed"
					metrics.ExposureAppliesTotal.WithLabelValues("failed").Inc()
					_ = s.store.Update(ctx, e)
					if s.audit != nil {
						s.audit.Audit(ctx, "apply_failed", "edge_exposure", before, e)
					}
					return &ApplyResponse{
						Exposure:       exposureResponsePtr(e),
						DryRun:         false,
						ValidateResult: validateResult,
						Status:         "failed",
						Message:        "兼容性检查不通过: " + reason,
					}, ErrCompatRuleDenied
				}
			}
		}
	}

	// 4. 状态 → applying
	e.Status = "applying"
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	// 5. 持久化 nginx 配置（同 hash 已存在则不重复插入）
	if s.nginxStore != nil {
		confRecord := &NginxGeneratedConfig{
			ID:            uuid.New(),
			ExposureID:    e.ID,
			ConfigContent: nginxConf,
			ConfigHash:    configHash,
			SchemaVersion: "v1",
		}
		if _, err := s.nginxStore.CreateIfAbsent(ctx, confRecord); err != nil {
			// 持久化失败：标记 failed 并写审计
			e.Status = "failed"
			metrics.ExposureAppliesTotal.WithLabelValues("failed").Inc()
			_ = s.store.Update(ctx, e)
			if s.audit != nil {
				s.audit.Audit(ctx, "apply_failed", "edge_exposure", before, e)
			}
			return &ApplyResponse{
				Exposure:       exposureResponsePtr(e),
				DryRun:         false,
				NginxConfigHash: configHash,
				ValidateResult: validateResult,
				Status:         "failed",
				Message:        "nginx 配置持久化失败: " + err.Error(),
			}, err
		}
	}

	// 6. 状态 → applied
	e.Status = "applied"
	metrics.ExposureAppliesTotal.WithLabelValues("applied").Inc()
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	// 7. 写审计日志（含 config_hash 便于追溯）
	if s.audit != nil {
		s.audit.Audit(ctx, "apply", "edge_exposure", before, map[string]interface{}{
			"exposure":        e,
			"nginx_config_hash": configHash,
			"validate_result": validateResult,
		})
	}
	if s.logger != nil {
		s.logger.Info("exposure applied",
			"exposure_id", e.ID, "server_id", e.ServerID, "config_hash", configHash)
	}

	return &ApplyResponse{
		Exposure:        exposureResponsePtr(e),
		DryRun:          false,
		NginxConf:       nginxConf,
		CloudflaredYML:  cfYML,
		NginxConfigHash: configHash,
		ValidateResult: validateResult,
		Status:         "applied",
		Message:         "配置已渲染并持久化，状态已更新为 applied",
	}, nil
}

// exposureResponsePtr 辅助函数：将 EdgeExposure 转为 ExposureResponse 指针
func exposureResponsePtr(e *EdgeExposure) *ExposureResponse {
	resp := NewExposureResponse(e)
	return &resp
}
