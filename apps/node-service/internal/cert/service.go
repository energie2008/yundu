package cert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// AuditWriter 预留的审计日志接口（本批 app.go 传 nil，调用前判 nil）
type AuditWriter interface {
	Audit(ctx context.Context, action, resource string, before, after interface{})
}

// CertificateStore 抽象 CertificateRepo 的数据访问（便于测试注入 mock）
type CertificateStore interface {
	Create(ctx context.Context, c *Certificate) error
	GetByID(ctx context.Context, id uuid.UUID) (*Certificate, error)
	GetByCode(ctx context.Context, code string) (*Certificate, error)
	Update(ctx context.Context, c *Certificate) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	SetRenewPending(ctx context.Context, id uuid.UUID) error
	UpdateRenewal(ctx context.Context, c *Certificate) error
	SetRenewFailed(ctx context.Context, id uuid.UUID, errMsg string) error
	List(ctx context.Context, page, pageSize int, status string, expiresWithinDays int) ([]*Certificate, int, error)
	ListExpiringSoon(ctx context.Context, days int) ([]*Certificate, error)
}

// ProfileStore 抽象 TLSProfileRepo 的数据访问
type ProfileStore interface {
	Create(ctx context.Context, p *TLSProfile) error
	GetByID(ctx context.Context, id uuid.UUID) (*TLSProfile, error)
	GetByCode(ctx context.Context, code string) (*TLSProfile, error)
	Update(ctx context.Context, p *TLSProfile) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountUsageInExposures(ctx context.Context, id uuid.UUID) (int, error)
	List(ctx context.Context, page, pageSize int) ([]*TLSProfile, int, error)
}

// DeployStore 抽象 CertDeployRepo 的数据访问
type DeployStore interface {
	ListByCertificateID(ctx context.Context, certID uuid.UUID) ([]*CertDeployRecord, error)
	Upsert(ctx context.Context, rec *CertDeployRecord) error
}

// NodeSNIReader 扫描启用的节点 SNI 列表，用于自动同步证书 SAN。
// 实现方通常为 *repo.NodeRepo（通过 ListEnabledSNIs 方法满足接口）。
// 未注入时 SyncSANFromNodes 返回 ErrNodeSNIReaderNotInjected。
type NodeSNIReader interface {
	// ListEnabledSNIs 返回所有 is_enabled=true 且 sni 非空的去重 SNI 列表。
	// 可选 serverID 参数：非 nil 时仅扫描该 server 下的节点（用于 per-server 证书）。
	ListEnabledSNIs(ctx context.Context, serverID *uuid.UUID) ([]string, error)
}

// CertBundleSyncStore 阶段 C1: cert_bundles 同步存储接口。
// 实现方通常为 *repo.CapabilityRepo（通过 FindCertBundleIDsByDomain/UpdateCertBundlePEM 满足接口）。
// 未注入时 ACME 续期成功后不同步到 cert_bundles（保持旧行为）。
type CertBundleSyncStore interface {
	FindCertBundleIDsByDomain(ctx context.Context, domain string) ([]uuid.UUID, error)
	UpdateCertBundlePEM(ctx context.Context, id uuid.UUID, certPEM, keyPEM string, notAfter *time.Time) error
}

// CertificateService 封装证书与 TLS Profile 的业务逻辑
type CertificateService struct {
	certRepo     CertificateStore
	profileRepo  ProfileStore
	deployRepo   DeployStore
	audit        AuditWriter
	logger       *slog.Logger
	acmeRegistry *ACMERegistry
	echGen       ECHGenerator
	nodeSNIReader NodeSNIReader
	bundleSyncStore CertBundleSyncStore // 阶段 C1: ACME 续期后同步到 cert_bundles
}

func NewCertificateService(certRepo CertificateStore, profileRepo ProfileStore, deployRepo DeployStore, audit AuditWriter, logger *slog.Logger) *CertificateService {
	return &CertificateService{
		certRepo:    certRepo,
		profileRepo: profileRepo,
		deployRepo:  deployRepo,
		audit:       audit,
		logger:      logger,
	}
}

// SetECHGenerator 注入 ECH 密钥对生成器（可选依赖）
// 未注入时 GenerateECH 会返回 ErrECHBinaryNotFound
func (s *CertificateService) SetECHGenerator(gen ECHGenerator) {
	s.echGen = gen
}

// SetACMERegistry 注入 ACME 客户端注册表（可选依赖）。
// 注入后 TriggerRenew/ObtainCertificate 会调用真实 ACME；未注入时退化为仅置 renew_status=pending。
func (s *CertificateService) SetACMERegistry(reg *ACMERegistry) {
	s.acmeRegistry = reg
}

// SetNodeSNIReader 注入节点 SNI 扫描器（可选依赖）。
// 注入后 SyncSANFromNodes 会自动扫描启用节点的 SNI 合并到证书 SAN；
// 未注入时 SyncSANFromNodes 返回 ErrNodeSNIReaderNotInjected。
func (s *CertificateService) SetNodeSNIReader(r NodeSNIReader) {
	s.nodeSNIReader = r
}

// SetCertBundleSyncStore 注入 cert_bundles 同步存储（可选依赖，阶段 C1）。
// 注入后 ACME 续期成功会自动同步 PEM 到 cert_bundles 表；
// 未注入时续期仅更新 tls_certificates 表（保持旧行为）。
func (s *CertificateService) SetCertBundleSyncStore(store CertBundleSyncStore) {
	s.bundleSyncStore = store
}

// normalizeCertDefaults 填充默认值
func normalizeCertDefaults(c *Certificate) {
	if c.Provider == "" {
		c.Provider = "custom"
	}
	if c.DeployMode == "" {
		c.DeployMode = "agent_push"
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.RenewStatus == "" {
		c.RenewStatus = "unknown"
	}
	if c.RenewDaysBefore == 0 {
		c.RenewDaysBefore = 21
	}
	if c.SANs == nil {
		c.SANs = []string{}
	}
}

func (s *CertificateService) CreateCertificate(ctx context.Context, req *CreateCertificateRequest) (*Certificate, error) {
	existing, err := s.certRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCertAlreadyExists
	}

	provider := req.Provider
	if provider == "" {
		provider = "custom"
	}
	autoRenew := true
	if req.AutoRenew != nil {
		autoRenew = *req.AutoRenew
	}
	renewDays := 21
	if req.RenewDaysBefore != nil {
		renewDays = *req.RenewDaysBefore
	}

	cert := &Certificate{
		ID:                uuid.New(),
		Code:              req.Code,
		Name:              req.Name,
		CertType:          req.CertType,
		CommonName:        req.CommonName,
		SANs:              req.SANs,
		Provider:          provider,
		CertPEM:           req.CertPEM,
		KeyPEMEncrypted:   req.KeyPEMEncrypted,
		CAPEM:             req.CAPEM,
		FingerprintSHA256: req.FingerprintSHA256,
		IssuedAt:          req.IssuedAt,
		ExpiresAt:         req.ExpiresAt,
		AutoRenew:         autoRenew,
		RenewDaysBefore:   renewDays,
		RenewStatus:       "unknown",
		DeployMode:        req.DeployMode,
		ACMEAccountEmail:  req.ACMEAccountEmail,
		ACMEChallengeType: req.ACMEChallengeType,
		ACMEDNSProvider:   req.ACMEDNSProvider,
		CloudflareZoneID:  req.CloudflareZoneID,
		Status:            "active",
	}

	// 加密 ACME 凭证（若前端提交了明文凭证 map）
	if len(req.ACMECredentials) > 0 && req.ACMEDNSProvider != nil && *req.ACMEDNSProvider != "" {
		providerName := *req.ACMEDNSProvider
		if err := ValidateDNSProviderCredentials(providerName, req.ACMECredentials); err != nil {
			return nil, fmt.Errorf("validate dns credentials: %w", err)
		}
		encrypted, err := EncryptCredentials(ACMECredentials{
			Provider: providerName,
			Vars:     req.ACMECredentials,
		})
		if err != nil {
			return nil, fmt.Errorf("encrypt dns credentials: %w", err)
		}
		cert.ACMECredentialsEncrypted = &encrypted
	}

	normalizeCertDefaults(cert)

	if err := s.certRepo.Create(ctx, cert); err != nil {
		return nil, err
	}
	return cert, nil
}

func (s *CertificateService) GetCertificate(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	c, err := s.certRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrCertNotFound
	}
	return c, nil
}

func (s *CertificateService) ListCertificates(ctx context.Context, page, pageSize int, query CertificateListQuery) ([]*Certificate, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.certRepo.List(ctx, page, pageSize, query.Status, query.ExpiresWithinDays)
}

func (s *CertificateService) UpdateCertificate(ctx context.Context, id uuid.UUID, req *UpdateCertificateRequest) (*Certificate, error) {
	cert, err := s.certRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, ErrCertNotFound
	}

	before := *cert

	if req.Name != nil {
		cert.Name = *req.Name
	}
	if req.CommonName != nil {
		cert.CommonName = *req.CommonName
	}
	if req.SANs != nil {
		cert.SANs = req.SANs
	}
	if req.CertPEM != nil {
		cert.CertPEM = req.CertPEM
	}
	if req.CAPEM != nil {
		cert.CAPEM = req.CAPEM
	}
	if req.FingerprintSHA256 != nil {
		cert.FingerprintSHA256 = req.FingerprintSHA256
	}
	if req.ExpiresAt != nil {
		cert.ExpiresAt = req.ExpiresAt
	}
	if req.AutoRenew != nil {
		cert.AutoRenew = *req.AutoRenew
	}
	if req.RenewDaysBefore != nil {
		cert.RenewDaysBefore = *req.RenewDaysBefore
	}
	if req.Status != nil {
		cert.Status = *req.Status
	}
	// ACME 字段更新（允许事后修改 ACME 配置）
	if req.ACMEAccountEmail != nil {
		cert.ACMEAccountEmail = req.ACMEAccountEmail
	}
	if req.ACMEChallengeType != nil {
		cert.ACMEChallengeType = req.ACMEChallengeType
	}
	if req.ACMEDNSProvider != nil {
		cert.ACMEDNSProvider = req.ACMEDNSProvider
	}
	// 若提交了新凭证，加密后落库
	if len(req.ACMECredentials) > 0 && cert.ACMEDNSProvider != nil && *cert.ACMEDNSProvider != "" {
		providerName := *cert.ACMEDNSProvider
		if err := ValidateDNSProviderCredentials(providerName, req.ACMECredentials); err != nil {
			return nil, fmt.Errorf("validate dns credentials: %w", err)
		}
		encrypted, err := EncryptCredentials(ACMECredentials{
			Provider: providerName,
			Vars:     req.ACMECredentials,
		})
		if err != nil {
			return nil, fmt.Errorf("encrypt dns credentials: %w", err)
		}
		cert.ACMECredentialsEncrypted = &encrypted
	}

	if err := s.certRepo.Update(ctx, cert); err != nil {
		return nil, err
	}

	if s.audit != nil {
		s.audit.Audit(ctx, "update", "tls_certificate", before, cert)
	}
	return cert, nil
}

func (s *CertificateService) DeleteCertificate(ctx context.Context, id uuid.UUID) error {
	cert, err := s.certRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if cert == nil {
		return ErrCertNotFound
	}
	if err := s.certRepo.SoftDelete(ctx, id); err != nil {
		return err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "delete", "tls_certificate", cert, nil)
	}
	return nil
}

// ListExpiringSoon 返回 expires_at <= now+days*24h AND renew_status != 'success' 的证书
func (s *CertificateService) ListExpiringSoon(ctx context.Context, days int) ([]*Certificate, error) {
	if days <= 0 {
		days = 21
	}
	return s.certRepo.ListExpiringSoon(ctx, days)
}

// TriggerRenew 触发证书续期。
//
// 如果注入了 ACMEClient 且证书有 ACME 配置（ACMEAccountEmail 非空），
// 则调用 lego 真实续期；否则退化为仅置 renew_status=pending（保留旧行为）。
//
// 续期成功：更新 cert_pem / key_pem / expires_at，renew_status='succeeded'。
// 续期失败：renew_status='failed'，renew_last_error 记录错误信息。
func (s *CertificateService) TriggerRenew(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	cert, err := s.certRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, ErrCertNotFound
	}

	// 没有 ACME 注册表或证书没有 ACME 配置时，保留旧行为（仅置 pending）
	if s.acmeRegistry == nil || cert.ACMEAccountEmail == nil || *cert.ACMEAccountEmail == "" {
		if err := s.certRepo.SetRenewPending(ctx, id); err != nil {
			return nil, err
		}
		cert.RenewStatus = "pending"
		now := time.Now()
		cert.RenewLastAttemptAt = &now
		if s.audit != nil {
			s.audit.Audit(ctx, "renew", "tls_certificate", nil, cert)
		}
		return cert, nil
	}

	// 按 cert 维度选择 ACME client（支持多 DNS provider）
	acmeClient, err := s.acmeRegistry.GetClientForCert(cert)
	if err != nil {
		return nil, fmt.Errorf("select ACME client for cert %s: %w", cert.Code, err)
	}

	if s.logger != nil {
		s.logger.Info("triggering ACME renew",
			"cert_id", cert.ID, "code", cert.Code, "common_name", cert.CommonName)
	}

	var certPEM, keyPEM []byte
	if cert.CertPEM != nil {
		certPEM = []byte(*cert.CertPEM)
	}
	if cert.KeyPEMEncrypted != nil {
		keyPEM = []byte(*cert.KeyPEMEncrypted)
	}

	result, err := acmeClient.RenewCertificate(ctx, certPEM, keyPEM)
	if err != nil {
		errMsg := err.Error()
		if setErr := s.certRepo.SetRenewFailed(ctx, id, errMsg); setErr != nil {
			return nil, fmt.Errorf("renew failed (%v) and persist error: %w", err, setErr)
		}
		cert.RenewStatus = "failed"
		cert.RenewLastError = &errMsg
		now := time.Now()
		cert.RenewLastAttemptAt = &now
		if s.logger != nil {
			s.logger.Error("ACME renew failed",
				"cert_id", cert.ID, "code", cert.Code, "error", err)
		}
		if s.audit != nil {
			s.audit.Audit(ctx, "renew_failed", "tls_certificate", nil, cert)
		}
		return cert, err
	}

	// 续期成功，更新证书数据
	certStr := string(result.CertPEM)
	cert.CertPEM = &certStr
	if len(result.KeyPEM) > 0 {
		keyStr := string(result.KeyPEM)
		cert.KeyPEMEncrypted = &keyStr
	}
	if !result.NotAfter.IsZero() {
		notAfter := result.NotAfter
		cert.ExpiresAt = &notAfter
	}
	issuedAt := time.Now()
	cert.IssuedAt = &issuedAt
	cert.RenewStatus = "succeeded"
	now := time.Now()
	cert.RenewLastAttemptAt = &now
	cert.RenewLastError = nil

	if err := s.certRepo.UpdateRenewal(ctx, cert); err != nil {
		return nil, fmt.Errorf("persist renewed certificate: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("ACME renew succeeded",
			"cert_id", cert.ID, "code", cert.Code,
			"expires_at", result.NotAfter.Format(time.RFC3339))
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "renew", "tls_certificate", nil, cert)
	}

	// 阶段 C1: 续期成功后自动同步 PEM 到 cert_bundles 表
	s.syncRenewalToCertBundles(ctx, cert)

	return cert, nil
}

// syncRenewalToCertBundles 阶段 C1: 将续期后的新 PEM 同步到 cert_bundles 表。
// 遍历证书的 CommonName + SANs 列表，查找匹配的 cert_bundle 并更新 PEM。
// 同步失败仅记录日志，不影响续期结果（cert_bundles 是辅助同步通道）。
func (s *CertificateService) syncRenewalToCertBundles(ctx context.Context, cert *Certificate) {
	if s.bundleSyncStore == nil {
		return // 未注入，保持旧行为
	}
	if cert.CertPEM == nil || *cert.CertPEM == "" {
		return
	}
	certPEM := *cert.CertPEM
	keyPEM := ""
	if cert.KeyPEMEncrypted != nil {
		keyPEM = *cert.KeyPEMEncrypted
	}

	// 收集所有域名（CommonName + SANs）
	domains := make(map[string]struct{})
	if cert.CommonName != "" {
		domains[cert.CommonName] = struct{}{}
	}
	for _, san := range cert.SANs {
		if san != "" {
			domains[san] = struct{}{}
		}
	}

	synced := 0
	for domain := range domains {
		ids, err := s.bundleSyncStore.FindCertBundleIDsByDomain(ctx, domain)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("sync cert_bundles: find by domain failed",
					"domain", domain, "cert_id", cert.ID, "error", err)
			}
			continue
		}
		for _, id := range ids {
			if err := s.bundleSyncStore.UpdateCertBundlePEM(ctx, id, certPEM, keyPEM, cert.ExpiresAt); err != nil {
				if s.logger != nil {
					s.logger.Warn("sync cert_bundles: update PEM failed",
						"bundle_id", id, "domain", domain, "error", err)
				}
				continue
			}
			synced++
		}
	}
	if synced > 0 && s.logger != nil {
		s.logger.Info("cert_bundles synced after renewal",
			"cert_id", cert.ID, "code", cert.Code, "synced_bundles", synced)
	}
}

// StartRenewalJob 启动定时续期检查（每 6 小时一轮，启动后立即执行一次）。
//
// 查询所有 30 天内过期且有 ACME 配置的证书，逐个调用 TriggerRenew。
// 需通过 SetACMEClient 注入 ACME 客户端；未注入时仅记录 warn 并跳过。
func (s *CertificateService) StartRenewalJob(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	// 启动后立即执行一次（避免冷启动后等 6 小时才首轮）
	s.checkAndRenew(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRenew(ctx)
		}
	}
}

// checkAndRenew 执行一轮续期检查。
func (s *CertificateService) checkAndRenew(ctx context.Context) {
	if s.acmeRegistry == nil {
		if s.logger != nil {
			s.logger.Warn("cert renewal tick skipped: ACME registry not injected")
		}
		return
	}
	expiring, err := s.certRepo.ListExpiringSoon(ctx, 30)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("cert renewal tick: list expiring certs failed", "error", err)
		}
		return
	}
	if len(expiring) == 0 {
		if s.logger != nil {
			s.logger.Info("cert renewal tick: no expiring certs")
		}
		return
	}
	if s.logger != nil {
		s.logger.Info("cert renewal round start", "expiring_count", len(expiring))
	}
	renewed, failed := 0, 0
	for _, c := range expiring {
		// 仅处理有 ACME 配置的证书
		if c.ACMEAccountEmail == nil || *c.ACMEAccountEmail == "" {
			continue
		}
		if _, err := s.TriggerRenew(ctx, c.ID); err != nil {
			failed++
			if s.logger != nil {
				s.logger.Error("cert renewal failed",
					"cert_id", c.ID, "code", c.Code, "error", err)
			}
			continue
		}
		renewed++
	}
	if s.logger != nil {
		s.logger.Info("cert renewal round complete",
			"renewed", renewed, "failed", failed, "expiring_total", len(expiring))
	}
}

// GetDeployStatus 返回某证书在所有 server 的部署记录
func (s *CertificateService) GetDeployStatus(ctx context.Context, id uuid.UUID) (*CertDeployStatusResponse, error) {
	if _, err := s.GetCertificate(ctx, id); err != nil {
		return nil, err
	}
	records, err := s.deployRepo.ListByCertificateID(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]CertDeployStatusItem, 0, len(records))
	for _, r := range records {
		items = append(items, CertDeployStatusItem{
			ServerID:     r.ServerID,
			DeployStatus: r.DeployStatus,
			DeployPath:   r.DeployPath,
			DeployedAt:   r.DeployedAt,
			ErrorMessage: r.ErrorMessage,
		})
	}
	return &CertDeployStatusResponse{CertificateID: id, Records: items}, nil
}

// RecordDeploy 写入/更新部署记录
func (s *CertificateService) RecordDeploy(ctx context.Context, certID, serverID uuid.UUID, status string, deployPath *string, errMsg *string) error {
	rec := &CertDeployRecord{
		ID:            uuid.New(),
		CertificateID: certID,
		ServerID:      serverID,
		DeployStatus:  status,
		DeployPath:    deployPath,
		ErrorMessage:  errMsg,
	}
	if status == "success" {
		now := time.Now()
		rec.DeployedAt = &now
	}
	if err := s.deployRepo.Upsert(ctx, rec); err != nil {
		return err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "deploy", "cert_deploy_record", nil, rec)
	}
	return nil
}

// --- TLS Profile ---

func (s *CertificateService) CreateProfile(ctx context.Context, req *CreateTLSProfileRequest) (*TLSProfile, error) {
	existing, err := s.profileRepo.GetByCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrProfileAlreadyExists
	}

	tlsMode := req.TLSMode
	if tlsMode == "" {
		tlsMode = "tls"
	}
	allowInsecure := false
	if req.AllowInsecure != nil {
		allowInsecure = *req.AllowInsecure
	}
	minVer := req.MinVersion
	if minVer == "" {
		minVer = "tls12"
	}
	maxVer := req.MaxVersion
	if maxVer == "" {
		maxVer = "tls13"
	}
	echEnabled := false
	if req.ECHEnabled != nil {
		echEnabled = *req.ECHEnabled
	}
	alpn := req.ALPN
	if alpn == nil {
		alpn = []string{"h2", "http/1.1"}
	}
	shortIDs := req.RealityShortIDs
	if shortIDs == nil {
		shortIDs = []string{}
	}

	profile := &TLSProfile{
		ID:                          uuid.New(),
		Code:                        req.Code,
		Name:                        req.Name,
		TLSMode:                     tlsMode,
		ServerName:                  req.ServerName,
		CertificateID:               req.CertificateID,
		AllowInsecure:               allowInsecure,
		UTLSFingerprint:             req.UTLSFingerprint,
		ALPN:                        alpn,
		MinVersion:                  minVer,
		MaxVersion:                  maxVer,
		ECHEnabled:                  echEnabled,
		ECHConfigEncrypted:          req.ECHConfigEncrypted,
		RealityPublicKey:            req.RealityPublicKey,
		RealityPrivateKeyEncrypted:  req.RealityPrivateKeyEncrypted,
		RealityShortIDs:             shortIDs,
		RealitySpiderX:              req.RealitySpiderX,
		RealityDest:                 req.RealityDest,
		Notes:                       req.Notes,
	}

	if err := s.profileRepo.Create(ctx, profile); err != nil {
		return nil, err
	}

	// ECH 启用时写 audit log（auditWriter 为 nil 时退化为 logger.Info 等价物）
	if echEnabled {
		if s.audit != nil {
			s.audit.Audit(ctx, "enable_ech", "tls_profile", nil, profile)
		} else if s.logger != nil {
			s.logger.Info("ECH enabled on tls profile (audit writer not configured)",
				"profile_id", profile.ID, "code", profile.Code)
		}
	}
	return profile, nil
}

func (s *CertificateService) GetProfile(ctx context.Context, id uuid.UUID) (*TLSProfile, error) {
	p, err := s.profileRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrProfileNotFound
	}
	return p, nil
}

func (s *CertificateService) ListProfiles(ctx context.Context, page, pageSize int) ([]*TLSProfile, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.profileRepo.List(ctx, page, pageSize)
}

func (s *CertificateService) UpdateProfile(ctx context.Context, id uuid.UUID, req *UpdateTLSProfileRequest) (*TLSProfile, error) {
	profile, err := s.profileRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrProfileNotFound
	}

	before := *profile

	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.TLSMode != nil {
		profile.TLSMode = *req.TLSMode
	}
	if req.ServerName != nil {
		profile.ServerName = req.ServerName
	}
	if req.CertificateID != nil {
		profile.CertificateID = req.CertificateID
	}
	if req.AllowInsecure != nil {
		profile.AllowInsecure = *req.AllowInsecure
	}
	if req.UTLSFingerprint != nil {
		profile.UTLSFingerprint = *req.UTLSFingerprint
	}
	if req.ALPN != nil {
		profile.ALPN = req.ALPN
	}
	if req.MinVersion != nil {
		profile.MinVersion = *req.MinVersion
	}
	if req.MaxVersion != nil {
		profile.MaxVersion = *req.MaxVersion
	}
	if req.ECHEnabled != nil {
		profile.ECHEnabled = *req.ECHEnabled
	}
	if req.Notes != nil {
		profile.Notes = req.Notes
	}

	if err := s.profileRepo.Update(ctx, profile); err != nil {
		return nil, err
	}

	if s.audit != nil {
		s.audit.Audit(ctx, "update", "tls_profile", before, profile)
	}
	return profile, nil
}

func (s *CertificateService) DeleteProfile(ctx context.Context, id uuid.UUID) error {
	profile, err := s.profileRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return ErrProfileNotFound
	}
	// 检查是否被 edge_exposures 引用
	count, err := s.profileRepo.CountUsageInExposures(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProfileInUse
	}
	if err := s.profileRepo.Delete(ctx, id); err != nil {
		return err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "delete", "tls_profile", profile, nil)
	}
	return nil
}

// GenerateECH 为 TLS Profile 生成 ECH 密钥对并持久化
//
// 调用 xray tls ech --generate 生成密钥对：
//   - ConfigPEM 写入 profile.ECHConfigEncrypted（公开，分发到客户端）
//   - KeyPEM    写入 profile.ECHKeyEncrypted（私有，仅服务端使用）
//
// 同时将 ECHEnabled 置为 true。需要先通过 SetECHGenerator 注入生成器。
func (s *CertificateService) GenerateECH(ctx context.Context, id uuid.UUID) (*TLSProfile, error) {
	profile, err := s.profileRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, ErrProfileNotFound
	}

	if s.echGen == nil {
		return nil, ErrECHBinaryNotFound
	}

	pair, err := s.echGen.Generate(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("ECH key pair generation failed", "profile_id", id, "code", profile.Code, "error", err)
		}
		return nil, fmt.Errorf("generate ECH key pair: %w", err)
	}

	before := *profile
	profile.ECHEnabled = true
	cfg := pair.ConfigPEM
	key := pair.KeyPEM
	profile.ECHConfigEncrypted = &cfg
	profile.ECHKeyEncrypted = &key

	if err := s.profileRepo.Update(ctx, profile); err != nil {
		return nil, fmt.Errorf("persist ECH key pair: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("ECH key pair generated and stored",
			"profile_id", id, "code", profile.Code)
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "generate_ech", "tls_profile", before, profile)
	}
	return profile, nil
}

// ObtainCertificate 触发首次 ACME 证书签发（区别于 TriggerRenew 续期）
//
// 前提：
//   - acmeRegistry 已注入
//   - 证书配置了 ACMEAccountEmail
//   - 证书有 CommonName（必填）和可选 SANs
//
// 成功后更新 cert_pem / key_pem / expires_at / renew_status='succeeded'
func (s *CertificateService) ObtainCertificate(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	cert, err := s.certRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, ErrCertNotFound
	}

	if s.acmeRegistry == nil {
		return nil, fmt.Errorf("ACME registry not injected, cannot obtain certificate")
	}
	if cert.ACMEAccountEmail == nil || *cert.ACMEAccountEmail == "" {
		return nil, fmt.Errorf("certificate %s has no ACME account email configured", cert.Code)
	}
	if cert.CommonName == "" {
		return nil, fmt.Errorf("certificate %s has no common_name", cert.Code)
	}

	acmeClient, err := s.acmeRegistry.GetClientForCert(cert)
	if err != nil {
		return nil, fmt.Errorf("select ACME client for cert %s: %w", cert.Code, err)
	}

	domains := append([]string{cert.CommonName}, cert.SANs...)
	if s.logger != nil {
		s.logger.Info("obtaining certificate",
			"cert_id", cert.ID, "code", cert.Code,
			"domains", domains, "common_name", cert.CommonName)
	}

	result, err := acmeClient.ObtainCertificate(ctx, domains)
	if err != nil {
		errMsg := err.Error()
		if setErr := s.certRepo.SetRenewFailed(ctx, id, errMsg); setErr != nil {
			return nil, fmt.Errorf("obtain failed (%v) and persist error: %w", err, setErr)
		}
		cert.RenewStatus = "failed"
		cert.RenewLastError = &errMsg
		now := time.Now()
		cert.RenewLastAttemptAt = &now
		if s.logger != nil {
			s.logger.Error("ACME obtain failed",
				"cert_id", cert.ID, "code", cert.Code, "error", err)
		}
		return cert, err
	}

	// 签发成功，更新证书数据
	certStr := string(result.CertPEM)
	cert.CertPEM = &certStr
	if len(result.KeyPEM) > 0 {
		keyStr := string(result.KeyPEM)
		cert.KeyPEMEncrypted = &keyStr
	}
	if !result.NotAfter.IsZero() {
		notAfter := result.NotAfter
		cert.ExpiresAt = &notAfter
	}
	issuedAt := time.Now()
	cert.IssuedAt = &issuedAt
	cert.RenewStatus = "succeeded"
	now := time.Now()
	cert.RenewLastAttemptAt = &now
	cert.RenewLastError = nil

	if err := s.certRepo.UpdateRenewal(ctx, cert); err != nil {
		return nil, fmt.Errorf("persist obtained certificate: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("ACME certificate obtained",
			"cert_id", cert.ID, "code", cert.Code,
			"expires_at", result.NotAfter.Format(time.RFC3339))
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "obtain", "tls_certificate", nil, cert)
	}
	return cert, nil
}

// SyncSANFromNodes 自动扫描启用节点的 SNI 并合并到证书 SAN。
//
// P9-FIX-2: 修复证书 SAN 缺失问题（如 cdn.dannelblog.na.am 不在证书 SAN 中）。
// 原先证书 SAN 仅由 API 请求传入，节点 SNI 变更后不会自动同步。
//
// 合并策略：
//   - 扫描所有 is_enabled=true 且 sni 非空的节点
//   - 与证书现有 SAN 合并去重（保留用户手动添加的 SAN）
//   - CommonName 自动加入 SAN（如果尚未包含）
//   - 写入 DB 后触发审计日志
//
// serverID 参数：非 nil 时仅扫描该 server 下的节点（per-server 证书场景）；
// 为 nil 时扫描所有节点。返回 (合并后SAN, 新增数量, error)。
func (s *CertificateService) SyncSANFromNodes(ctx context.Context, certID uuid.UUID, serverID *uuid.UUID) (*Certificate, int, error) {
	if s.nodeSNIReader == nil {
		return nil, 0, ErrNodeSNIReaderNotInjected
	}

	cert, err := s.certRepo.GetByID(ctx, certID)
	if err != nil {
		return nil, 0, err
	}
	if cert == nil {
		return nil, 0, ErrCertNotFound
	}

	// 扫描启用节点的 SNI
	nodeSNIs, err := s.nodeSNIReader.ListEnabledSNIs(ctx, serverID)
	if err != nil {
		return nil, 0, fmt.Errorf("scan node SNIs: %w", err)
	}

	// 合并去重：现有 SAN + 节点 SNI + CommonName
	merged := make(map[string]bool, len(cert.SANs)+len(nodeSNIs)+1)
	var newSANs []string

	// 1. 保留现有 SAN（用户手动添加的）
	for _, san := range cert.SANs {
		if san != "" && !merged[san] {
			merged[san] = true
			newSANs = append(newSANs, san)
		}
	}
	// 2. 自动添加 CommonName（证书主体名必须出现在 SAN 中）
	if cert.CommonName != "" && !merged[cert.CommonName] {
		merged[cert.CommonName] = true
		newSANs = append(newSANs, cert.CommonName)
	}
	// 3. 合并节点 SNI
	added := 0
	for _, sni := range nodeSNIs {
		if sni == "" || merged[sni] {
			continue
		}
		merged[sni] = true
		newSANs = append(newSANs, sni)
		added++
	}

	if added == 0 {
		if s.logger != nil {
			s.logger.Info("cert SAN sync: no new SNIs to add",
				"cert_id", cert.ID, "code", cert.Code,
				"current_sans", len(cert.SANs), "node_snis", len(nodeSNIs))
		}
		return cert, 0, nil
	}

	// 更新证书 SAN
	before := *cert
	cert.SANs = newSANs
	if err := s.certRepo.Update(ctx, cert); err != nil {
		return nil, 0, fmt.Errorf("persist merged SANs: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("cert SAN synced from nodes",
			"cert_id", cert.ID, "code", cert.Code,
			"added", added, "total_sans", len(newSANs),
			"node_snis_scanned", len(nodeSNIs))
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "sync_san", "tls_certificate", before, cert)
	}
	return cert, added, nil
}
