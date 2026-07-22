// Package cert - certmagic 证书提供者
//
// 使用 caddyserver/certmagic 纯 Go 库替代 acme.sh 签发证书。
// 支持 DNS-01（via libdns/cloudflare）和 HTTP-01 验证方式，
// certmagic 内部自动处理证书续期，通过 OnEvent 回调桥接到 CertEventBus。
package cert

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
	"go.uber.org/zap"
)

// certmagicProvider 使用 certmagic 库管理证书（纯 Go 实现，替代 acme.sh）
type certmagicProvider struct {
	cfg      *certmagic.Config
	cache    *certmagic.Cache
	certDir  string // /etc/yundu/certs
	cfToken  string // Cloudflare API Token
	logger   Logger
	eventBus *CertEventBus

	initOnce sync.Once
	initErr  error
}

// newCertmagicProvider 创建 certmagic 证书提供者。
// certDir 为证书存储根目录，cfToken 为 Cloudflare API Token（为空时使用 HTTP-01 验证）。
func newCertmagicProvider(certDir, cfToken string, logger Logger, eventBus *CertEventBus) *certmagicProvider {
	return &certmagicProvider{
		certDir:  certDir,
		cfToken:  cfToken,
		logger:   logger,
		eventBus: eventBus,
	}
}

// init 初始化 certmagic 配置（仅执行一次）。
func (p *certmagicProvider) init() error {
	p.initOnce.Do(func() {
		// 创建 certmagic 缓存（后台自动续期依赖此缓存）
		p.cache = certmagic.NewCache(certmagic.CacheOptions{
			GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
				return p.cfg, nil
			},
			Logger: zap.NewNop(),
		})

		// 构建 ACME Issuer 模板
		issuerTemplate := certmagic.ACMEIssuer{
			CA:     certmagic.LetsEncryptProductionCA,
			Agreed: true,
		}

		// 根据是否提供 CF Token 选择验证方式
		if p.cfToken != "" {
			// DNS-01: 使用 Cloudflare DNS provider
			dnsProvider := &cloudflare.Provider{
				APIToken: p.cfToken,
			}
			issuerTemplate.DNS01Solver = &certmagic.DNS01Solver{
				DNSManager: certmagic.DNSManager{
					DNSProvider:        dnsProvider,
					TTL:                120 * time.Second,
					PropagationDelay:   2 * time.Second,
					PropagationTimeout: 2 * time.Minute,
				},
			}
			// DNS-01 模式下禁用 HTTP 和 TLS-ALPN 挑战
			issuerTemplate.DisableHTTPChallenge = true
			issuerTemplate.DisableTLSALPNChallenge = true
			p.logger.Info("certmagic: 配置 DNS-01 验证（Cloudflare）")
		} else {
			// HTTP-01: 使用 certmagic 内置的 HTTP solver（需要 80 端口可访问）
			p.logger.Info("certmagic: 配置 HTTP-01 验证")
		}

		// 创建 certmagic 配置，设置 OnEvent 回调桥接到 CertEventBus
		cfg := certmagic.Config{
			OnEvent: p.onCertmagicEvent,
		}

		p.cfg = certmagic.New(p.cache, cfg)

		// 创建 ACME Issuer 并注入到配置
		acmeIssuer := certmagic.NewACMEIssuer(p.cfg, issuerTemplate)
		p.cfg.Issuers = []certmagic.Issuer{acmeIssuer}

		p.logger.Info("certmagic: 提供者初始化完成")
	})
	return p.initErr
}

// ensureCert 确保证书存在且有效，返回证书和私钥路径。
// 首次调用会触发 certmagic 签发，后续由 certmagic 自动续期。
func (p *certmagicProvider) ensureCert(domain string) (certPath, keyPath string, err error) {
	if err := p.init(); err != nil {
		return "", "", fmt.Errorf("certmagic 初始化失败: %w", err)
	}

	// 标准证书路径
	certPath = filepath.Join(p.certDir, domain, "fullchain.pem")
	keyPath = filepath.Join(p.certDir, domain, "privkey.pem")

	// 快速检查：已有证书有效则直接返回
	if p.isCertValid(certPath) {
		return certPath, keyPath, nil
	}

	// 调用 certmagic ManageSync 获取证书
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	p.logger.Info("certmagic: 开始获取证书", "domain", domain)
	if err := p.cfg.ManageSync(ctx, []string{domain}); err != nil {
		p.publishFailed(domain, err)
		return "", "", fmt.Errorf("certmagic: 获取证书失败 %s: %w", domain, err)
	}

	// certmagic ManageSync 成功后，证书已保存到 certmagic 的存储中。
	// 使用 CacheManagedCertificate 加载到缓存，然后从磁盘读取 PEM。
	if _, err := p.cfg.CacheManagedCertificate(ctx, domain); err != nil {
		p.logger.Warn("certmagic: 缓存加载证书失败（非致命）", "domain", domain, "error", err)
	}

	// 从标准路径读取 certmagic 持久化后的 PEM 文件
	// certmagic 内部存储路径：{Storage}/{domain}/
	// 我们在 ManageSync 成功后直接从我们的标准路径读取
	certPEM, keyPEM, err := p.loadPEMFromDisk(domain)
	if err != nil {
		return "", "", fmt.Errorf("certmagic: 读取证书 PEM 失败 %s: %w", domain, err)
	}

	// 持久化到标准路径（如果 certmagic 存储路径与标准路径不同）
	wasRenewal := fileExists(certPath)
	if err := p.persistPEM(domain, certPEM, keyPEM); err != nil {
		return "", "", fmt.Errorf("certmagic: 持久化证书失败 %s: %w", domain, err)
	}

	// 发布事件
	eventType := CertEventObtained
	if wasRenewal {
		eventType = CertEventRenewed
	}
	p.publishEvent(eventType, domain, certPEM, keyPEM)

	p.logger.Info("certmagic: 证书获取成功", "domain", domain, "type", string(eventType))
	return certPath, keyPath, nil
}

// persistPEM 持久化 PEM 到标准路径（/etc/yundu/certs/{domain}/）
func (p *certmagicProvider) persistPEM(domain string, certPEM, keyPEM []byte) error {
	dir := filepath.Join(p.certDir, domain)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	certPath := filepath.Join(dir, "fullchain.pem")
	if err := atomicWrite(certPath, certPEM, 0644); err != nil {
		return err
	}
	keyPath := filepath.Join(dir, "privkey.pem")
	return atomicWrite(keyPath, keyPEM, 0600)
}

// onCertmagicEvent certmagic 事件回调，桥接到 CertEventBus。
// certmagic 事件:
//   - "cert_obtained" + renewal=false → CertEventObtained
//   - "cert_obtained" + renewal=true  → CertEventRenewed
//   - "cert_failed"                  → CertEventFailed
func (p *certmagicProvider) onCertmagicEvent(ctx context.Context, event string, data map[string]any) error {
	if p.eventBus == nil {
		return nil
	}

	domain, _ := data["identifier"].(string)
	if domain == "" {
		return nil
	}

	switch event {
	case "cert_obtained":
		renewal, _ := data["renewal"].(bool)

		// 从磁盘加载 PEM 文件
		certPEM, keyPEM, err := p.loadPEMFromDisk(domain)
		if err != nil {
			p.logger.Warn("certmagic: 事件回调中加载证书失败", "domain", domain, "error", err)
			return nil
		}

		// 持久化到标准路径
		if err := p.persistPEM(domain, certPEM, keyPEM); err != nil {
			p.logger.Warn("certmagic: 事件回调中持久化证书失败", "domain", domain, "error", err)
		}

		// 映射事件类型
		eventType := CertEventObtained
		if renewal {
			eventType = CertEventRenewed
		}
		p.publishEvent(eventType, domain, certPEM, keyPEM)

		p.logger.Info("certmagic: 证书事件", "event", event, "domain", domain, "renewal", renewal)

	case "cert_failed":
		var certErr error
		if e, ok := data["error"].(error); ok {
			certErr = e
		}
		p.publishFailed(domain, certErr)
		p.logger.Error("certmagic: 证书获取/续期失败", "domain", domain, "error", certErr)
	}

	return nil
}

// publishEvent 发布证书事件到 CertEventBus
func (p *certmagicProvider) publishEvent(eventType CertEventType, domain string, certPEM, keyPEM []byte) {
	if p.eventBus == nil {
		return
	}
	p.eventBus.Publish(CertEvent{
		Type:     eventType,
		Domain:   domain,
		BundleID: domain,
		PEM: &PEMBundle{
			CertPEM: certPEM,
			KeyPEM:  keyPEM,
			Domain:  domain,
		},
	})
}

// publishFailed 发布签发失败事件
func (p *certmagicProvider) publishFailed(domain string, err error) {
	if p.eventBus == nil {
		return
	}
	p.eventBus.Publish(CertEvent{
		Type:   CertEventFailed,
		Domain: domain,
		Error:  err,
	})
}

// isCertValid 检查证书文件是否存在且未过期（剩余 > DefaultRenewThreshold）
func (p *certmagicProvider) isCertValid(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	remaining := time.Until(cert.NotAfter)
	if remaining < DefaultRenewThreshold {
		p.logger.Info("certmagic: 证书即将过期，将续期",
			"cert", certPath, "remaining_days", int(remaining.Hours()/24))
		return false
	}
	return true
}

// loadPEMFromDisk 从标准路径读取 certmagic 签发后保存的 PEM 文件。
// certmagic 默认将证书存储在其 FileStorage 路径下，
// 本方法尝试从标准路径和 certmagic 默认存储路径两个位置读取。
func (p *certmagicProvider) loadPEMFromDisk(domain string) (certPEM, keyPEM []byte, err error) {
	// 首先尝试标准路径
	certPath := filepath.Join(p.certDir, domain, "fullchain.pem")
	keyPath := filepath.Join(p.certDir, domain, "privkey.pem")

	certPEM, err = os.ReadFile(certPath)
	if err == nil {
		keyPEM, err = os.ReadFile(keyPath)
		if err == nil {
			return certPEM, keyPEM, nil
		}
	}

	// 尝试 certmagic 默认存储路径（$HOME/.local/share/certmagic/）
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		cmCertPath := filepath.Join(homeDir, ".local", "share", "certmagic", "certificates", domain, domain+".crt")
		cmKeyPath := filepath.Join(homeDir, ".local", "share", "certmagic", "certificates", domain, domain+".key")
		certPEM, err = os.ReadFile(cmCertPath)
		if err == nil {
			keyPEM, err = os.ReadFile(cmKeyPath)
			if err == nil {
				return certPEM, keyPEM, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("certmagic: 无法从磁盘读取证书 %s（标准路径和 certmagic 默认路径均失败）", domain)
}
