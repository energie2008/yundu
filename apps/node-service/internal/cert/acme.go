package cert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// ACMEUser 实现 lego 的 registration.User 接口，封装 ACME 账户信息。
type ACMEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *ACMEUser) GetEmail() string                        { return u.Email }
func (u *ACMEUser) GetRegistration() *registration.Resource  { return u.Registration }
func (u *ACMEUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// CertificateResult 封装 ACME 签发/续期后的证书结果。
type CertificateResult struct {
	CertPEM   []byte
	KeyPEM    []byte
	IssuerURL string
	NotAfter  time.Time
}

// ACMEClient 封装 lego 客户端，提供证书签发与续期能力。
//
// 支持 http-01 与 dns-01（cloudflare）两种 challenge。
// http-01 需要监听 80 端口（或在反向代理后面），生产环境推荐使用 dns-01。
type ACMEClient struct {
	client        *lego.Client
	challengeType string
	logger        *slog.Logger
}

// NewACMEClient 创建 ACME 客户端并注册账户。
//
// 参数：
//   - email:         ACME 账户邮箱（必填）
//   - dirURL:        ACME directory URL，留空则使用 Let's Encrypt staging
//   - challengeType: "http-01" 或 "dns-01"
//   - dnsProvider:   dns-01 时使用的 DNS 提供商（如 "cloudflare"），http-01 时忽略
//   - credentials:   dns-01 时的显式凭证 map（来自 DB 解密）；nil 时退化为环境变量模式
//   - logger:        日志记录器，nil 时使用 slog.Default()
func NewACMEClient(email, dirURL, challengeType, dnsProvider string, credentials map[string]string, logger *slog.Logger) (*ACMEClient, error) {
	if email == "" {
		return nil, fmt.Errorf("ACME email is required")
	}
	if dirURL == "" {
		dirURL = lego.LEDirectoryStaging
	}
	if challengeType == "" {
		challengeType = "http-01"
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "acme-client")

	// 1. 生成账户私钥（ECDSA P-256，ACME 推荐的轻量级算法）
	accountKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate account key: %w", err)
	}

	// 2. 创建 ACME 用户
	user := &ACMEUser{
		Email: email,
		key:   accountKey,
	}

	// 3. 创建 lego 配置与客户端
	config := lego.NewConfig(user)
	config.CADirURL = dirURL
	config.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create lego client: %w", err)
	}

	// 4. 配置 challenge provider
	switch challengeType {
	case "http-01":
		// http-01 默认监听 80 端口；iface/port 留空使用默认值
		providerServer := http01.NewProviderServer("", "")
		if err := client.Challenge.SetHTTP01Provider(providerServer); err != nil {
			return nil, fmt.Errorf("set http-01 provider: %w", err)
		}
	case "dns-01":
		if err := setupDNSProvider(client, dnsProvider, credentials); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported challenge type: %s (use http-01 or dns-01)", challengeType)
	}

	// 5. 注册账户（TermsOfServiceAgreed=true 表示同意 CA 的服务条款）
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("register ACME account: %w", err)
	}
	user.Registration = reg

	logger.Info("ACME client registered",
		"email", email, "dir_url", dirURL,
		"challenge_type", challengeType, "account_uri", reg.URI)

	return &ACMEClient{
		client:        client,
		challengeType: challengeType,
		logger:        logger,
	}, nil
}

// setupDNSProvider 配置 DNS-01 challenge 的 DNS 提供商（注册表模式）。
//
// 优先级：
//  1. 若 credentials 非空，用显式凭证创建 provider（来自 DB 的 acme_credentials_encrypted）
//  2. 若 credentials 为空，退化为环境变量模式（兼容旧版 cloudflare 配置）
func setupDNSProvider(client *lego.Client, providerName string, credentials map[string]string) error {
	if providerName == "" {
		return fmt.Errorf("dns-01 challenge requires a dns provider (e.g. cloudflare)")
	}
	// 显式凭证路径：走注册表工厂
	if len(credentials) > 0 {
		provider, err := CreateDNSProvider(providerName, credentials)
		if err != nil {
			return fmt.Errorf("create %s dns provider: %w", providerName, err)
		}
		if err := client.Challenge.SetDNS01Provider(provider); err != nil {
			return fmt.Errorf("set dns-01 provider: %w", err)
		}
		return nil
	}
	// 退化路径：无显式凭证时用 NewDNSProvider() 从环境变量读
	// 仅 cloudflare 支持此模式（其他 provider 必须显式传凭证）
	provider, err := CreateDNSProvider(providerName, nil)
	if err != nil {
		return fmt.Errorf("create %s dns provider (env mode): %w", providerName, err)
	}
	if err := client.Challenge.SetDNS01Provider(provider); err != nil {
		return fmt.Errorf("set dns-01 provider: %w", err)
	}
	return nil
}

// ObtainCertificate 申请新证书。
//
// domains 的第一个元素作为 CommonName，其余作为 SAN。
// 返回的 CertificateResult 包含 PEM 格式的证书与私钥。
func (c *ACMEClient) ObtainCertificate(ctx context.Context, domains []string) (*CertificateResult, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("domains is required")
	}
	c.logger.Info("obtaining certificate",
		"domains", domains, "challenge_type", c.challengeType)

	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}
	resource, err := c.client.Certificate.Obtain(request)
	if err != nil {
		return nil, fmt.Errorf("obtain certificate: %w", err)
	}

	result, err := parseCertificateResource(resource)
	if err != nil {
		return nil, err
	}
	c.logger.Info("certificate obtained",
		"domain", resource.Domain, "not_after", result.NotAfter.Format(time.RFC3339))
	return result, nil
}

// RenewCertificate 续期证书。
//
// certPEM 为现有证书的 PEM 字节，keyPEM 为对应私钥的 PEM 字节。
// 如果 keyPEM 为空，lego 会为续期后的证书生成新私钥。
func (c *ACMEClient) RenewCertificate(ctx context.Context, certPEM, keyPEM []byte) (*CertificateResult, error) {
	if len(certPEM) == 0 {
		return nil, fmt.Errorf("cert PEM is required")
	}
	c.logger.Info("renewing certificate", "challenge_type", c.challengeType)

	resource := certificate.Resource{
		Certificate: certPEM,
		PrivateKey:  keyPEM,
	}

	newRes, err := c.client.Certificate.Renew(resource, true, false, "")
	if err != nil {
		return nil, fmt.Errorf("renew certificate: %w", err)
	}

	result, err := parseCertificateResource(newRes)
	if err != nil {
		return nil, err
	}
	c.logger.Info("certificate renewed", "not_after", result.NotAfter.Format(time.RFC3339))
	return result, nil
}

// parseCertificateResource 从 lego 的 certificate.Resource 解析出 CertificateResult。
//
// Certificate 字段可能是单个证书或 bundle（含 issuer），这里取第一个证书的 NotAfter。
func parseCertificateResource(res *certificate.Resource) (*CertificateResult, error) {
	result := &CertificateResult{
		CertPEM:   res.Certificate,
		KeyPEM:    res.PrivateKey,
		IssuerURL: res.CertURL,
	}

	// 解析证书以获取 NotAfter
	if len(res.Certificate) > 0 {
		// PEM bundle 可能包含多个块，取第一个（即签发的叶子证书）
		rest := res.Certificate
		for len(rest) > 0 {
			block, r := pem.Decode(rest)
			if block == nil {
				break
			}
			rest = r
			if block.Type == "CERTIFICATE" {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					result.NotAfter = cert.NotAfter
					break
				}
			}
		}
	}
	return result, nil
}

// ACMERegistry 是 ACME 客户端的注册表与缓存管理器
//
// 替代旧版全局单例 ACMEClient，支持按证书维度选择 DNS provider：
//   - http-01 证书共享同一个 client（不依赖 DNS provider）
//   - dns-01 证书按 provider+credentials 懒创建并缓存 client
//
// 缓存 key 为 provider+credentials 的 SHA-256 哈希，避免重复注册 ACME 账户
type ACMERegistry struct {
	email  string
	dirURL string
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[string]*ACMEClient // key: cacheKey
}

// NewACMERegistry 创建 ACME 客户端注册表
func NewACMERegistry(email, dirURL string, logger *slog.Logger) *ACMERegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &ACMERegistry{
		email:   email,
		dirURL:  dirURL,
		logger:  logger.With("component", "acme-registry"),
		clients: make(map[string]*ACMEClient),
	}
}

// GetClientForCert 根据证书的 ACME 配置选择/创建对应的 ACMEClient
//
// 行为：
//   - cert.ACMEChallengeType 为空或 "http-01"：返回 http-01 client（共享）
//   - cert.ACMEChallengeType == "dns-01"：按 provider+credentials 创建/缓存 client
//   - cert.ACMECredentialsEncrypted 非空时解密获取显式凭证
//   - cert.ACMECredentialsEncrypted 为空时退化为环境变量模式（仅 cloudflare）
func (r *ACMERegistry) GetClientForCert(cert *Certificate) (*ACMEClient, error) {
	if r.email == "" {
		return nil, fmt.Errorf("ACME email not configured in registry")
	}

	challengeType := "http-01"
	if cert.ACMEChallengeType != nil && *cert.ACMEChallengeType != "" {
		challengeType = *cert.ACMEChallengeType
	}

	// http-01 模式：固定 cacheKey，全局共享一个 client
	if challengeType == "http-01" {
		return r.getOrCreate("http-01", "", nil)
	}

	// dns-01 模式：需要 provider name
	providerName := ""
	if cert.ACMEDNSProvider != nil {
		providerName = *cert.ACMEDNSProvider
	}
	if providerName == "" {
		return nil, fmt.Errorf("dns-01 challenge requires acme_dns_provider on certificate %s", cert.Code)
	}

	// 解密凭证（为空则退化为环境变量模式）
	var creds map[string]string
	if cert.ACMECredentialsEncrypted != nil && *cert.ACMECredentialsEncrypted != "" {
		ac, err := DecryptCredentials(*cert.ACMECredentialsEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt credentials for cert %s: %w", cert.Code, err)
		}
		creds = ac.Vars
	}

	cacheKey := buildDNSCacheKey(providerName, creds)
	return r.getOrCreate(challengeType, providerName, creds, cacheKey)
}

// getOrCreate 从缓存获取或创建新的 ACMEClient
func (r *ACMERegistry) getOrCreate(challengeType, dnsProvider string, creds map[string]string, explicitKey ...string) (*ACMEClient, error) {
	cacheKey := challengeType + ":" + dnsProvider
	if len(explicitKey) > 0 {
		cacheKey = explicitKey[0]
	}

	// 快路径：读锁查缓存
	r.mu.RLock()
	if c, ok := r.clients[cacheKey]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	// 慢路径：写锁创建
	r.mu.Lock()
	defer r.mu.Unlock()
	// double-check
	if c, ok := r.clients[cacheKey]; ok {
		return c, nil
	}

	client, err := NewACMEClient(r.email, r.dirURL, challengeType, dnsProvider, creds, r.logger)
	if err != nil {
		return nil, fmt.Errorf("create ACME client [%s/%s]: %w", challengeType, dnsProvider, err)
	}
	r.clients[cacheKey] = client
	return client, nil
}

// buildDNSCacheKey 为 dns-01 client 生成唯一缓存键
// key = "dns-01:" + provider + ":" + sha256(credentials_json)
func buildDNSCacheKey(provider string, creds map[string]string) string {
	if len(creds) == 0 {
		return "dns-01:" + provider + ":env"
	}
	// 简单序列化：按 key 排序拼接
	keys := make([]string, 0, len(creds))
	for k := range creds {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var buf []byte
	for _, k := range keys {
		buf = append(buf, []byte(k)...)
		buf = append(buf, ':')
		buf = append(buf, []byte(creds[k])...)
		buf = append(buf, '|')
	}
	h := sha256.Sum256(buf)
	return "dns-01:" + provider + ":" + hex.EncodeToString(h[:8])
}
