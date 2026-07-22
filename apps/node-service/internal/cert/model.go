package cert

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000009_tls_certificates.sql)

// Certificate 对应 tls_certificates 表
type Certificate struct {
	ID                     uuid.UUID  `json:"id"`
	Code                   string     `json:"code"`
	Name                   string     `json:"name"`
	CertType               string     `json:"cert_type"`
	CommonName             string     `json:"common_name"`
	SANs                   []string   `json:"sans"`
	Provider               string     `json:"provider"`
	CertPEM                *string    `json:"cert_pem,omitempty"`
	KeyPEMEncrypted        *string    `json:"key_pem_encrypted,omitempty"`
	CAPEM                  *string    `json:"ca_pem,omitempty"`
	FingerprintSHA256     *string    `json:"fingerprint_sha256,omitempty"`
	IssuedAt               *time.Time `json:"issued_at,omitempty"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	AutoRenew              bool       `json:"auto_renew"`
	RenewDaysBefore        int        `json:"renew_days_before"`
	RenewStatus            string     `json:"renew_status"`
	RenewLastAttemptAt    *time.Time `json:"renew_last_attempt_at,omitempty"`
	RenewLastError         *string    `json:"renew_last_error,omitempty"`
	DeployMode             string     `json:"deploy_mode"`
	ACMEAccountEmail       *string    `json:"acme_account_email,omitempty"`
	ACMEChallengeType      *string    `json:"acme_challenge_type,omitempty"`
	ACMEDNSProvider        *string    `json:"acme_dns_provider,omitempty"`
	ACMECredentialsEncrypted *string  `json:"acme_credentials_encrypted,omitempty"`
	CloudflareZoneID       *string    `json:"cloudflare_zone_id,omitempty"`
	Status                 string     `json:"status"`
	CreatedByAdminID       *uuid.UUID `json:"created_by_admin_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// TLSProfile 对应 tls_profiles 表
type TLSProfile struct {
	ID                          uuid.UUID  `json:"id"`
	Code                        string     `json:"code"`
	Name                        string     `json:"name"`
	TLSMode                     string     `json:"tls_mode"`
	ServerName                  *string    `json:"server_name,omitempty"`
	CertificateID               *uuid.UUID `json:"certificate_id,omitempty"`
	AllowInsecure               bool       `json:"allow_insecure"`
	UTLSFingerprint             string     `json:"utls_fingerprint"`
	ALPN                        []string   `json:"alpn"`
	MinVersion                  string     `json:"min_version"`
	MaxVersion                  string     `json:"max_version"`
	ECHEnabled                  bool       `json:"ech_enabled"`
	ECHConfigEncrypted          *string    `json:"ech_config_encrypted,omitempty"`
	ECHKeyEncrypted             *string    `json:"ech_key_encrypted,omitempty"`
	RealityPublicKey           *string    `json:"reality_public_key,omitempty"`
	RealityPrivateKeyEncrypted *string    `json:"reality_private_key_encrypted,omitempty"`
	RealityShortIDs             []string  `json:"reality_short_ids"`
	RealitySpiderX             *string    `json:"reality_spider_x,omitempty"`
	RealityDest                 *string    `json:"reality_dest,omitempty"`
	Notes                       *string    `json:"notes,omitempty"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
}

// CertDeployRecord 对应 cert_deploy_records 表
type CertDeployRecord struct {
	ID            uuid.UUID  `json:"id"`
	CertificateID uuid.UUID  `json:"certificate_id"`
	ServerID      uuid.UUID  `json:"server_id"`
	DeployStatus  string     `json:"deploy_status"`
	DeployPath    *string    `json:"deploy_path,omitempty"`
	DeployedAt    *time.Time `json:"deployed_at,omitempty"`
	ErrorMessage  *string    `json:"error_message,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// DTO: Certificate

type CreateCertificateRequest struct {
	Code                string     `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name                string     `json:"name" binding:"required,min=1,max=128"`
	CertType            string     `json:"cert_type" binding:"required"`
	CommonName          string     `json:"common_name" binding:"required,min=1,max=255"`
	SANs                []string   `json:"sans"`
	Provider            string     `json:"provider"`
	CertPEM             *string    `json:"cert_pem"`
	KeyPEMEncrypted     *string    `json:"key_pem_encrypted"`
	CAPEM               *string    `json:"ca_pem"`
	FingerprintSHA256   *string    `json:"fingerprint_sha256"`
	IssuedAt            *time.Time `json:"issued_at"`
	ExpiresAt           *time.Time `json:"expires_at"`
	AutoRenew           *bool      `json:"auto_renew"`
	RenewDaysBefore     *int       `json:"renew_days_before"`
	DeployMode          string     `json:"deploy_mode"`
	ACMEAccountEmail    *string    `json:"acme_account_email"`
	ACMEChallengeType   *string    `json:"acme_challenge_type"`
	ACMEDNSProvider     *string    `json:"acme_dns_provider"`
	ACMECredentials     map[string]string `json:"acme_credentials"` // 明文凭证，service 层加密后落库
	CloudflareZoneID    *string    `json:"cloudflare_zone_id"`
}

type UpdateCertificateRequest struct {
	Name            *string    `json:"name"`
	CommonName      *string    `json:"common_name"`
	SANs            []string   `json:"sans"`
	CertPEM         *string    `json:"cert_pem"`
	CAPEM           *string    `json:"ca_pem"`
	FingerprintSHA256 *string  `json:"fingerprint_sha256"`
	ExpiresAt       *time.Time `json:"expires_at"`
	AutoRenew       *bool      `json:"auto_renew"`
	RenewDaysBefore *int       `json:"renew_days_before"`
	Status          *string    `json:"status"`
	// ACME 字段（允许事后修改）
	ACMEAccountEmail  *string            `json:"acme_account_email"`
	ACMEChallengeType *string            `json:"acme_challenge_type"`
	ACMEDNSProvider   *string            `json:"acme_dns_provider"`
	ACMECredentials   map[string]string  `json:"acme_credentials"` // 明文凭证，service 层加密后落库
}

type CertificateListQuery struct {
	Page               int    `form:"page"`
	PageSize           int    `form:"page_size"`
	Status             string `form:"status"`
	ExpiresWithinDays  int    `form:"expires_within_days"`
}

type CertificateResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Code                string     `json:"code"`
	Name                string     `json:"name"`
	CertType            string     `json:"cert_type"`
	CommonName          string     `json:"common_name"`
	SANs                []string   `json:"sans"`
	Provider            string     `json:"provider"`
	FingerprintSHA256   *string    `json:"fingerprint_sha256,omitempty"`
	IssuedAt            *time.Time `json:"issued_at,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	AutoRenew           bool       `json:"auto_renew"`
	RenewDaysBefore     int        `json:"renew_days_before"`
	RenewStatus         string     `json:"renew_status"`
	RenewLastAttemptAt  *time.Time `json:"renew_last_attempt_at,omitempty"`
	DeployMode          string     `json:"deploy_mode"`
	Status              string     `json:"status"`
	CreatedAt           string     `json:"created_at"`
}

func NewCertificateResponse(c *Certificate) CertificateResponse {
	return CertificateResponse{
		ID:                c.ID,
		Code:              c.Code,
		Name:              c.Name,
		CertType:          c.CertType,
		CommonName:        c.CommonName,
		SANs:              c.SANs,
		Provider:          c.Provider,
		FingerprintSHA256: c.FingerprintSHA256,
		IssuedAt:          c.IssuedAt,
		ExpiresAt:         c.ExpiresAt,
		AutoRenew:         c.AutoRenew,
		RenewDaysBefore:   c.RenewDaysBefore,
		RenewStatus:       c.RenewStatus,
		RenewLastAttemptAt: c.RenewLastAttemptAt,
		DeployMode:        c.DeployMode,
		Status:            c.Status,
		CreatedAt:          c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CertDeployStatusResponse 用于 GET /admin/tls-certificates/:id/deploy-status
type CertDeployStatusItem struct {
	ServerID     uuid.UUID  `json:"server_id"`
	DeployStatus string     `json:"deploy_status"`
	DeployPath   *string    `json:"deploy_path,omitempty"`
	DeployedAt   *time.Time `json:"deployed_at,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
}

type CertDeployStatusResponse struct {
	CertificateID uuid.UUID               `json:"certificate_id"`
	Records       []CertDeployStatusItem  `json:"records"`
}

// DTO: TLSProfile

type CreateTLSProfileRequest struct {
	Code                  string     `json:"code" binding:"required,alphanum,min=2,max=64"`
	Name                  string     `json:"name" binding:"required,min=1,max=128"`
	TLSMode               string     `json:"tls_mode"`
	ServerName            *string    `json:"server_name"`
	CertificateID         *uuid.UUID `json:"certificate_id"`
	AllowInsecure         *bool      `json:"allow_insecure"`
	UTLSFingerprint       string     `json:"utls_fingerprint" binding:"required,min=1,max=32"`
	ALPN                  []string   `json:"alpn"`
	MinVersion            string     `json:"min_version"`
	MaxVersion            string     `json:"max_version"`
	ECHEnabled            *bool      `json:"ech_enabled"`
	ECHConfigEncrypted    *string    `json:"ech_config_encrypted"`
	RealityPublicKey      *string    `json:"reality_public_key"`
	RealityPrivateKeyEncrypted *string `json:"reality_private_key_encrypted"`
	RealityShortIDs       []string   `json:"reality_short_ids"`
	RealitySpiderX        *string    `json:"reality_spider_x"`
	RealityDest           *string    `json:"reality_dest"`
	Notes                 *string    `json:"notes"`
}

type UpdateTLSProfileRequest struct {
	Name                  *string    `json:"name"`
	TLSMode               *string    `json:"tls_mode"`
	ServerName            *string    `json:"server_name"`
	CertificateID         *uuid.UUID `json:"certificate_id"`
	AllowInsecure         *bool      `json:"allow_insecure"`
	UTLSFingerprint       *string    `json:"utls_fingerprint"`
	ALPN                  []string   `json:"alpn"`
	MinVersion            *string    `json:"min_version"`
	MaxVersion            *string    `json:"max_version"`
	ECHEnabled            *bool      `json:"ech_enabled"`
	Notes                 *string    `json:"notes"`
}

type TLSProfileListQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type TLSProfileResponse struct {
	ID              uuid.UUID  `json:"id"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	TLSMode         string     `json:"tls_mode"`
	ServerName      *string    `json:"server_name,omitempty"`
	CertificateID   *uuid.UUID `json:"certificate_id,omitempty"`
	AllowInsecure   bool       `json:"allow_insecure"`
	UTLSFingerprint string     `json:"utls_fingerprint"`
	ALPN            []string   `json:"alpn"`
	MinVersion      string     `json:"min_version"`
	MaxVersion      string     `json:"max_version"`
	ECHEnabled      bool       `json:"ech_enabled"`
	RealityDest     *string    `json:"reality_dest,omitempty"`
	CreatedAt       string     `json:"created_at"`
}

func NewTLSProfileResponse(p *TLSProfile) TLSProfileResponse {
	return TLSProfileResponse{
		ID:              p.ID,
		Code:            p.Code,
		Name:            p.Name,
		TLSMode:         p.TLSMode,
		ServerName:      p.ServerName,
		CertificateID:  p.CertificateID,
		AllowInsecure:  p.AllowInsecure,
		UTLSFingerprint: p.UTLSFingerprint,
		ALPN:            p.ALPN,
		MinVersion:      p.MinVersion,
		MaxVersion:      p.MaxVersion,
		ECHEnabled:      p.ECHEnabled,
		RealityDest:     p.RealityDest,
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// DNSProviderListResponse 用于 GET /admin/tls-certificates/dns-providers
type DNSProviderListResponse struct {
	Providers []DNSProviderMeta `json:"providers"`
}
