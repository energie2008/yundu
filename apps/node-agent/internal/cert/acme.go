// Package cert 实现 node-agent 的证书自动签发能力。
//
// 设计原则：
//   - 每台 VPS 的 agent 各自独立签发自己负责域名的证书
//   - 使用 DNS-01 challenge（Cloudflare DNS API），不需要 80 端口对外开放
//   - 证书存储在统一路径 /etc/yundu/certs/{domain}/
//   - 兼容宝塔环境：优先复用宝塔已签发的证书
//   - 签发失败不阻断 vhost 同步（跳过该域名，等下轮重试）
package cert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"

	"github.com/airport-panel/node-agent/internal/cert/dnsproviders"
)

// DefaultCertDir 证书统一存储根目录
const DefaultCertDir = "/etc/yundu/certs"

// DefaultRenewThreshold 证书续期阈值：剩余 < 15 天时触发续期
const DefaultRenewThreshold = 15 * 24 * time.Hour

// CertMode 证书签发模式（P1-1: 6 种模式）
type CertMode string

const (
	CertModeHTTP       CertMode = "http"       // HTTP-01 challenge（需要 80 端口）
	CertModeDNS        CertMode = "dns"        // DNS-01 challenge（acme.sh + Cloudflare）
	CertModeCertmagic  CertMode = "certmagic"  // certmagic 纯 Go 实现（DNS-01/HTTP-01）
	CertModeSelf       CertMode = "self"       // 自签名证书（测试/内网）
	CertModeFile       CertMode = "file"       // 从文件读取已有证书
	CertModeContent    CertMode = "content"    // 面板推送 PEM（P1-4）
	CertModeNone       CertMode = "none"       // 不签发（直连节点无 nginx）
)

// PEMBundle PEM 格式的证书+私钥（P1-1: 原子热替换的载荷）
type PEMBundle struct {
	CertPEM []byte
	KeyPEM  []byte
	Domain  string
}

// Manager 证书签发管理器（P1-1: 6 种模式 + 原子热替换 + 重启恢复）
//
// 移植自 Xboard-Node 的 Reconfigure/tearDownACME/acmeFingerprint/loadPEMFromStorage
// 四个核心方法（2026-07-12），实现：
//   - 配置变更热重载（Reconfigure）
//   - ACME 后台 goroutine 干净清理（tearDownACME）
//   - 避免不必要 ACME 重签发的配置指纹（acmeFingerprint）
//   - certmagic 存储统一加载入口（loadPEMFromStorage）
type Manager struct {
	mode             CertMode
	certDir          string
	cfToken          string // Cloudflare API Token（DNS-01 challenge 必需）
	acmeShPath       string // acme.sh 路径（缓存查找结果）
	renewThreshold   time.Duration
	logger           Logger
	eventBus         *CertEventBus // P1-2: 证书事件总线

	// certmagic 模式：纯 Go 证书提供者（惰性初始化）
	certmagicProv *certmagicProvider

	// content 模式：面板推送的 PEM（原子热替换，P1-4）
	content atomic.Pointer[PEMBundle]

	// 缓存已签发证书的原子引用（热替换，无文件读取）
	// key: domain, value: *PEMBundle
	cache sync.Map

	// tlsCertCache 缓存已解析的 tls.Certificate（零中断证书热替换）
	// key: domain, value: *atomic.Pointer[tls.Certificate]
	tlsCertCache sync.Map

	// === 移植自 Xboard-Node 的 ACME 生命周期管理字段 ===
	// acmeMu 保护 ACME 生命周期操作（Start/Reconfigure/tearDownACME 互斥）
	acmeMu sync.Mutex
	// acmeStarted 标记 certmagic ACME 是否已启动
	acmeStarted bool
	// acmeFingerprint 当前 ACME 配置指纹，用于检测配置是否实质变化
	acmeFingerprint string
	// acmeCancel 取消 certmagic 后台 goroutine（Reconfigure/tearDownACME 时调用）
	acmeCancel context.CancelFunc
	// magic 当前 certmagic 配置实例（供 loadPEMFromStorage 读取）
	magic *certmagic.Config
	// renewed 原子标记证书续期发生（调用方 Swap 后触发内核热重载）
	renewed atomic.Bool
	// domain 当前 Manager 负责的域名（ACME 模式必需）
	domain string
	// email ACME 账户邮箱
	email string
	// dnsProvider DNS provider 名称（如 "cloudflare"）
	dnsProvider string
	// dnsEnv DNS provider 环境变量（如 CLOUDFLARE_DNS_API_TOKEN）
	dnsEnv map[string]string
	// certFile/keyFile file 模式的证书/私钥路径
	certFile string
	keyFile  string
}

// Logger 简单的日志接口
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// noopLogger 默认空日志
type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// NewManager 创建证书管理器（默认 dns 模式，向后兼容）。
// cfToken 为 Cloudflare API Token，用于 DNS-01 challenge。
// 为空时禁用 ACME 签发（只能复用已有证书）。
func NewManager(cfToken string, logger Logger) *Manager {
	return NewManagerWithMode(CertModeDNS, cfToken, logger)
}

// NewManagerWithMode 创建指定模式的证书管理器（P1-1: 6 种模式）。
func NewManagerWithMode(mode CertMode, cfToken string, logger Logger) *Manager {
	if logger == nil {
		logger = noopLogger{}
	}
	if mode == "" {
		mode = CertModeDNS
	}
	m := &Manager{
		mode:           mode,
		certDir:        DefaultCertDir,
		cfToken:        cfToken,
		renewThreshold: DefaultRenewThreshold,
		logger:         logger,
	}
	// 重启恢复：从磁盘加载上次签发的 PEM 到内存缓存
	m.loadPersistedPEM()
	return m
}

// SetEventBus 注入证书事件总线（P1-2）
func (m *Manager) SetEventBus(bus *CertEventBus) {
	m.eventBus = bus
}

// SetContentPEM P1-4: content 模式 — 面板推送 PEM 证书。
// 原子替换内存中的 PEM bundle，并发布 CertEventContent 事件。
// 订阅者（xray/nginx reconciler）收到事件后自动热重载。
func (m *Manager) SetContentPEM(certPEM, keyPEM []byte, domain string) {
	bundle := &PEMBundle{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Domain:  domain,
	}
	m.content.Store(bundle)
	m.cache.Store(domain, bundle)

	if tlsCert, err := tls.X509KeyPair(certPEM, keyPEM); err == nil {
		m.setTLSCert(domain, &tlsCert)
	} else {
		m.logger.Warn("failed to parse PEM to tls.Certificate", "domain", domain, "error", err)
	}

	// 持久化到磁盘（重启恢复）
	m.persistPEM(domain, certPEM, keyPEM)

	if m.eventBus != nil {
		m.eventBus.Publish(CertEvent{
			Type:     CertEventContent,
			Domain:   domain,
			BundleID: domain,
			PEM:      bundle,
		})
	}
	m.logger.Info("content PEM updated", "domain", domain, "cert_len", len(certPEM))
}

// setTLSCert 原子存储域名的解析后 tls.Certificate（零中断热替换）。
func (m *Manager) setTLSCert(domain string, cert *tls.Certificate) {
	ap := &atomic.Pointer[tls.Certificate]{}
	ap.Store(cert)
	m.tlsCertCache.Store(domain, ap)
}

// GetTLSCert 原子获取域名的 tls.Certificate（零中断，无锁读取）。
// 返回 nil 表示证书未就绪。
func (m *Manager) GetTLSCert(domain string) *tls.Certificate {
	if val, ok := m.tlsCertCache.Load(domain); ok {
		ap := val.(*atomic.Pointer[tls.Certificate])
		return ap.Load()
	}
	if bundle := m.GetPEMBundle(domain); bundle != nil {
		if tlsCert, err := tls.X509KeyPair(bundle.CertPEM, bundle.KeyPEM); err == nil {
			m.setTLSCert(domain, &tlsCert)
			return &tlsCert
		}
	}
	return nil
}

// GetTLSConfig 返回一个 *tls.Config，其 GetCertificate 回调从原子存储中
// 获取最新证书，支持证书热替换零中断（无需重启 TLS listener）。
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			domain := hello.ServerName
			if domain == "" {
				domain = m.domain
			}
			cert := m.GetTLSCert(domain)
			if cert == nil {
				return nil, fmt.Errorf("no certificate available for domain %q", domain)
			}
			return cert, nil
		},
		MinVersion: tls.VersionTLS12,
	}
}

// GetPEMBundle 获取域名的 PEM bundle（从内存缓存，无文件读取）。
// content 模式从 atomic.Pointer 获取，其他模式从 cache 获取。
// 返回 nil 表示缓存未命中，调用方应调用 EnsureCert 签发。
func (m *Manager) GetPEMBundle(domain string) *PEMBundle {
	if val, ok := m.cache.Load(domain); ok {
		return val.(*PEMBundle)
	}
	return nil
}

// persistPEM 原子写入 PEM 到磁盘（tmp+rename, 0600）
func (m *Manager) persistPEM(domain string, certPEM, keyPEM []byte) {
	dir := filepath.Join(m.certDir, domain)
	if err := os.MkdirAll(dir, 0755); err != nil {
		m.logger.Warn("persist PEM: mkdir failed", "domain", domain, "error", err)
		return
	}
	// 原子写入 cert
	certPath := filepath.Join(dir, "fullchain.pem")
	if err := atomicWrite(certPath, certPEM, 0644); err != nil {
		m.logger.Warn("persist PEM: write cert failed", "domain", domain, "error", err)
		return
	}
	// 原子写入 key
	keyPath := filepath.Join(dir, "privkey.pem")
	if err := atomicWrite(keyPath, keyPEM, 0600); err != nil {
		m.logger.Warn("persist PEM: write key failed", "domain", domain, "error", err)
	}
}

// loadPersistedPEM 重启恢复：从磁盘加载已有 PEM 到内存缓存。
// 扫描 certDir 下所有域名目录，加载 fullchain.pem + privkey.pem。
func (m *Manager) loadPersistedPEM() {
	entries, err := os.ReadDir(m.certDir)
	if err != nil {
		return // 目录不存在或无权限，静默跳过
	}
	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := entry.Name()
		certPath := filepath.Join(m.certDir, domain, "fullchain.pem")
		keyPath := filepath.Join(m.certDir, domain, "privkey.pem")
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			continue
		}
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}
		bundle := &PEMBundle{
			CertPEM: certPEM,
			KeyPEM:  keyPEM,
			Domain:  domain,
		}
		m.cache.Store(domain, bundle)
		loaded++
	}
	if loaded > 0 {
		m.logger.Info("persisted PEM bundles loaded on startup", "count", loaded)
	}
}

// atomicWrite 原子写入文件（tmp+rename）
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// EnsureCert 确保证书存在且有效，返回证书和私钥路径。
//
// 根据 mode 分发（P1-1: 6 种模式）：
//   - none:    跳过签发（直连节点无 nginx）
//   - content: 从面板推送的 PEM 获取（P1-4）
//   - self:    自签名证书
//   - file:    仅从文件读取（不签发，兼容宝塔已有证书）
//   - dns:     acme.sh DNS-01 challenge（默认）
//   - http:    acme.sh HTTP-01 challenge
//
// 签发/续期成功后：
//  1. 加载 PEM 到内存 cache（原子热替换）
//  2. 发布 CertEvent 到 EventBus（订阅者触发 xray/nginx 热重载）
func (m *Manager) EnsureCert(domain string) (certPath, keyPath string, err error) {
	domain = strings.TrimSpace(domain)
	certPath = filepath.Join(m.certDir, domain, "fullchain.pem")
	keyPath = filepath.Join(m.certDir, domain, "privkey.pem")

	// none 模式：跳过签发
	if m.mode == CertModeNone {
		return "", "", fmt.Errorf("cert mode is 'none', skip issuance for %s", domain)
	}

	// content 模式：从 atomic.Pointer 获取面板推送的 PEM
	if m.mode == CertModeContent {
		bundle := m.content.Load()
		if bundle == nil {
			return "", "", fmt.Errorf("content mode: no PEM pushed for %s", domain)
		}
		m.persistPEM(domain, bundle.CertPEM, bundle.KeyPEM)
		return certPath, keyPath, nil
	}

	// self 模式：自签名证书
	if m.mode == CertModeSelf {
		if m.isCertValid(certPath) {
			m.loadToCache(domain, certPath, keyPath)
			return certPath, keyPath, nil
		}
		if err := m.issueSelfSigned(domain, certPath, keyPath); err != nil {
			return "", "", fmt.Errorf("self-signed cert failed for %s: %w", domain, err)
		}
		m.loadToCache(domain, certPath, keyPath)
		m.publishEvent(CertEventObtained, domain, certPath, keyPath)
		m.logger.Info("self-signed cert issued", "domain", domain)
		return certPath, keyPath, nil
	}

	// file 模式：仅从文件读取（不签发）
	if m.mode == CertModeFile {
		if m.isCertValid(certPath) {
			m.loadToCache(domain, certPath, keyPath)
			return certPath, keyPath, nil
		}
		// 检查宝塔路径（VPS190 兼容）
		btCert := filepath.Join("/www/server/panel/vhost/cert", domain, "fullchain.pem")
		btKey := filepath.Join("/www/server/panel/vhost/cert", domain, "privkey.pem")
		if m.isCertValid(btCert) {
			m.logger.Info("copying cert from bt-panel path", "domain", domain, "src", btCert)
			if err := m.copyCert(btCert, btKey, certPath, keyPath); err == nil {
				m.loadToCache(domain, certPath, keyPath)
				return certPath, keyPath, nil
			}
		}
		return "", "", fmt.Errorf("file mode: no valid cert for %s", domain)
	}

	// certmagic / dns / http 模式：统一使用 certmagic 纯 Go 签发（阶段 A3）
	// dns → DNS-01 challenge（via libdns/cloudflare），http → HTTP-01 challenge
	// certmagic → 别名，等同于 dns（有 cfToken）或 http（无 cfToken）
	// acme.sh 仅作为 dns/http 模式的回退（向后兼容已有 VPS）
	if m.mode == CertModeCertmagic || m.mode == CertModeDNS || m.mode == CertModeHTTP {
		// 1. 统一路径证书存在且有效 → 直接使用
		if m.isCertValid(certPath) {
			m.loadToCache(domain, certPath, keyPath)
			return certPath, keyPath, nil
		}

		// 2. 检查宝塔路径（VPS190 兼容）
		btCert := filepath.Join("/www/server/panel/vhost/cert", domain, "fullchain.pem")
		btKey := filepath.Join("/www/server/panel/vhost/cert", domain, "privkey.pem")
		if m.isCertValid(btCert) {
			m.logger.Info("copying cert from bt-panel path", "domain", domain, "src", btCert)
			if err := m.copyCert(btCert, btKey, certPath, keyPath); err == nil {
				m.loadToCache(domain, certPath, keyPath)
				return certPath, keyPath, nil
			}
			m.logger.Warn("copy bt cert failed, will try certmagic", "domain", domain, "error", err)
		}

		// 3. certmagic 签发（默认引擎）
		wasRenewal := fileExists(certPath)
		if m.certmagicProv == nil {
			m.certmagicProv = newCertmagicProvider(m.certDir, m.cfToken, m.logger, m.eventBus)
		}
		_, _, cmErr := m.certmagicProv.ensureCert(domain)
		if cmErr == nil {
			m.loadToCache(domain, certPath, keyPath)
			eventType := CertEventObtained
			if wasRenewal {
				eventType = CertEventRenewed
			}
			m.publishEvent(eventType, domain, certPath, keyPath)
			m.logger.Info("cert issued successfully", "domain", domain,
				"type", string(eventType), "engine", "certmagic")
			return certPath, keyPath, nil
		}

		// 4. certmagic 失败：dns/http 模式回退到 acme.sh（向后兼容）
		// certmagic 模式不回退 acme.sh（纯 Go 实现，无 acme.sh 依赖），但仍兜底自签证书
		if m.mode == CertModeCertmagic {
			return m.fallbackSelfSigned(domain, certPath, keyPath,
				fmt.Errorf("certmagic issue failed for %s: %w", domain, cmErr))
		}
		m.logger.Warn("certmagic issuance failed, falling back to acme.sh",
			"domain", domain, "error", cmErr)

		if m.cfToken == "" && m.mode == CertModeDNS {
			return m.fallbackSelfSigned(domain, certPath, keyPath,
				fmt.Errorf("cert not found for %s and CF_Token not configured (certmagic: %w)", domain, cmErr))
		}

		acmeSh, err := m.findAcmeSh()
		if err != nil {
			return m.fallbackSelfSigned(domain, certPath, keyPath,
				fmt.Errorf("cert issuance failed for %s: certmagic: %w; acme.sh not found: %v", domain, cmErr, err))
		}

		challenge := "dns_cf"
		if m.mode == CertModeHTTP {
			challenge = "no" // acme.sh standalone HTTP-01
		}
		m.logger.Info("issuing cert via acme.sh fallback", "domain", domain, "mode", m.mode, "acme_sh", acmeSh)
		if err := m.issueViaACME(acmeSh, domain, certPath, keyPath, challenge); err != nil {
			return m.fallbackSelfSigned(domain, certPath, keyPath,
				fmt.Errorf("ACME issue failed for %s: certmagic: %w; acme.sh: %v", domain, cmErr, err))
		}

		// acme.sh 签发成功
		m.loadToCache(domain, certPath, keyPath)
		eventType := CertEventObtained
		if wasRenewal {
			eventType = CertEventRenewed
		}
		m.publishEvent(eventType, domain, certPath, keyPath)
		m.logger.Info("cert issued successfully", "domain", domain,
			"type", string(eventType), "engine", "acme.sh")
		return certPath, keyPath, nil
	}

	return "", "", fmt.Errorf("unsupported cert mode: %s for domain %s", m.mode, domain)
}

// loadToCache 从文件加载 PEM 到内存 cache（原子热替换）
func (m *Manager) loadToCache(domain, certPath, keyPath string) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return
	}
	m.cache.Store(domain, &PEMBundle{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Domain:  domain,
	})
}

// publishEvent 发布证书事件到 EventBus
func (m *Manager) publishEvent(eventType CertEventType, domain, certPath, keyPath string) {
	if m.eventBus == nil {
		return
	}
	certPEM, _ := os.ReadFile(certPath)
	keyPEM, _ := os.ReadFile(keyPath)
	m.eventBus.Publish(CertEvent{
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
func (m *Manager) publishFailed(domain string, err error) {
	if m.eventBus == nil {
		return
	}
	m.eventBus.Publish(CertEvent{
		Type:   CertEventFailed,
		Domain: domain,
		Error:  err,
	})
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fallbackSelfSigned 在 ACME 签发全部失败后兜底生成自签证书。
// 确保节点至少有一张可用证书完成 TLS 握手，避免 nginx 8445 无法启动 TLS termination。
// 注意：自签证书在 CF 公网会被验证为无效（525），仅在直连/允许 allow_insecure 场景可用。
// 下次 reconciler 轮询时会通过 isCertValid 检查（剩余 >15 天）跳过签发，不再覆盖。
func (m *Manager) fallbackSelfSigned(domain, certPath, keyPath string, lastErr error) (string, string, error) {
	m.logger.Warn("all ACME issuance failed, falling back to self-signed cert",
		"domain", domain, "last_error", lastErr)
	if err := m.issueSelfSigned(domain, certPath, keyPath); err != nil {
		combinedErr := fmt.Errorf("cert issuance failed for %s: acme: %w; self-signed: %v", domain, lastErr, err)
		m.publishFailed(domain, combinedErr)
		return "", "", combinedErr
	}
	m.loadToCache(domain, certPath, keyPath)
	m.publishEvent(CertEventObtained, domain, certPath, keyPath)
	m.logger.Warn("self-signed cert issued as fallback (ACME failed, will retry next cycle)",
		"domain", domain, "acme_error", lastErr)
	return certPath, keyPath, nil
}

// issueSelfSigned 生成自签名证书（self 模式）。
// P0-1 修复：补齐 CA 标志位、IP SAN 支持、时钟偏移容错、VerifyHostname 自检。
// 移植自 node-service/crypto/selfsigned.go 的 GenerateSelfSignedCertPEM。
func (m *Manager) issueSelfSigned(domain, certPath, keyPath string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("domain is required for self-signed cert")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ECDSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour), // 时钟偏移容错
		NotAfter:              time.Now().Add(365 * 24 * 10 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{domain}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	parsedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse generated cert for self-check: %w", err)
	}
	if err := parsedCert.VerifyHostname(domain); err != nil {
		return fmt.Errorf("self-signed cert VerifyHostname failed for %q: %w", domain, err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal ECDSA key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := atomicWrite(certPath, certPEM, 0644); err != nil {
		return err
	}
	return atomicWrite(keyPath, keyPEM, 0600)
}

// isCertValid 检查证书文件是否存在且未过期。
func (m *Manager) isCertValid(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}

	// 解析 PEM 块，只取第一个证书
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}

	// 检查过期时间
	remaining := time.Until(cert.NotAfter)
	if remaining < m.renewThreshold {
		m.logger.Info("cert expiring soon, will renew",
			"cert", certPath, "remaining_days", int(remaining.Hours()/24))
		return false
	}

	return true
}

// copyCert 复制证书文件到统一路径。
func (m *Manager) copyCert(srcCert, srcKey, dstCert, dstKey string) error {
	if err := os.MkdirAll(filepath.Dir(dstCert), 0755); err != nil {
		return err
	}
	if err := copyFile(srcCert, dstCert); err != nil {
		return err
	}
	return copyFile(srcKey, dstKey)
}

// findAcmeSh 查找 acme.sh 二进制路径。
// 优先级：~/.acme.sh/acme.sh → /www/server/acme.sh/acme.sh → PATH 中的 acme.sh
func (m *Manager) findAcmeSh() (string, error) {
	if m.acmeShPath != "" {
		return m.acmeShPath, nil
	}

	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".acme.sh", "acme.sh"),
		"/root/.acme.sh/acme.sh",
		"/www/server/acme.sh/acme.sh",
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			if info.Mode().Perm()&0111 != 0 { // 可执行
				m.acmeShPath = p
				return p, nil
			}
		}
	}

	// 尝试 PATH 中查找
	if p, err := exec.LookPath("acme.sh"); err == nil {
		m.acmeShPath = p
		return p, nil
	}

	return "", fmt.Errorf("acme.sh not installed (checked ~/.acme.sh, /www/server/acme.sh, PATH)")
}

// issueViaACME 调用 acme.sh 签发证书。
// challenge 指定验证方式：
//   - "no"     → HTTP-01 standalone（需要 80 端口可访问）
//   - "dns_cf" → DNS-01 + Cloudflare API（需要 CF_Token）
//   - 其他     → 视为 DNS provider name（如 "dns_dp" / "dns_ali"）
func (m *Manager) issueViaACME(acmeSh, domain, certFile, keyFile, challenge string) error {
	certDir := filepath.Dir(certFile)
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return err
	}

	args := []string{
		"--issue",
		"-d", domain,
		"--cert-file", filepath.Join(certDir, "cert.pem"),
		"--key-file", keyFile,
		"--fullchain-file", certFile,
	}
	if challenge == "no" {
		// HTTP-01 standalone 模式：acme.sh 自带 web server 监听 80 端口
		args = append(args, "--standalone", "--listen-v4")
	} else {
		// DNS-01 模式：challenge 作为 DNS provider name
		args = append(args, "--dns", challenge)
	}

	cmd := exec.Command(acmeSh, args...)
	cmd.Env = append(os.Environ(), "CF_Token="+m.cfToken)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("acme.sh failed: %w\n%s", err, out)
	}

	return nil
}

// copyFile 复制单个文件。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// =============================================================================
// 移植自 Xboard-Node 的四个核心方法（2026-07-12）
//
// 参考：https://github.com/cedar2025/Xboard-Node/blob/dev/internal/cert/cert.go
// commit 0a29338e1f102a462363ce3527417029f89bab28
//
// 设计原则：
//   - Reconfigure: 配置变更热重载，返回 PEM 是否变化供调用方决定是否重启内核
//   - tearDownACME: 干净清理 certmagic 后台 goroutine，避免泄漏
//   - acmeFingerprint: 配置指纹，避免不必要的 ACME 重新签发
//   - loadPEMFromStorage: certmagic 存储统一加载入口（初始加载 + 续期刷新共用）
// =============================================================================

// CertReconfigureConfig 是 Reconfigure 的输入配置。
// 封装 ACME 模式所需的全部字段，便于面板推送完整证书配置。
type CertReconfigureConfig struct {
	Mode        CertMode            // 证书模式: http/dns/self/file/content/none
	Domain      string              // ACME 模式必需的域名
	Email       string              // ACME 账户邮箱
	CertDir     string              // 证书存储目录（空则保持原值）
	CertPEM     []byte              // content 模式的证书 PEM
	KeyPEM      []byte              // content 模式的私钥 PEM
	CertFile    string              // file 模式的证书文件路径
	KeyFile     string             // file 模式的私钥文件路径
	DNSProvider string              // DNS-01 provider 名称（如 "cloudflare"）
	DNSEnv      map[string]string   // DNS provider 环境变量
	CFToken     string              // Cloudflare API Token（向后兼容 dns 模式）
}

// Reconfigure 运行时应用新的证书配置（如面板推送）。
// 返回 true 表示 PEM 材料实质变化，调用方应触发内核热重载。
//
// 移植自 Xboard-Node Manager.Reconfigure，适配点：
//   - 保留 YunDu 的 6 模式 + EventBus 通知
//   - 复用现有 certmagicProvider 而非直接操作 certmagic（保留惰性初始化）
//   - 仅在 ACME 模式（http/dns/certmagic）下执行 tearDownACME 逻辑
//
// 此方法是 SetContentPEM 的通用版本：content 模式走 SetContentPEM 快路径，
// 其他模式走 Start 重启路径。
func (m *Manager) Reconfigure(ctx context.Context, newCfg CertReconfigureConfig) (bool, error) {
	m.acmeMu.Lock()
	defer m.acmeMu.Unlock()

	// 继承未设置的字段
	if newCfg.CertDir == "" {
		newCfg.CertDir = m.certDir
	}
	if newCfg.Mode == "" {
		newCfg.Mode = m.mode
	}
	if newCfg.CFToken == "" {
		newCfg.CFToken = m.cfToken
	}

	// 计算新配置的 ACME 指纹
	newFp := acmeFingerprint(newCfg)
	newMode := resolveModeFor(newCfg)

	// 如果 ACME 正在运行且配置实质变化（或切换到非 ACME 模式），先 tearDown
	if m.acmeStarted {
		if newMode != "http" && newMode != "dns" && newMode != "certmagic" || newFp != m.acmeFingerprint {
			m.tearDownACME()
		}
	}

	// 记录旧 PEM（用于变化检测）
	oldTLS := m.TLSCert()

	// 应用新配置
	m.mode = newCfg.Mode
	m.certDir = newCfg.CertDir
	m.cfToken = newCfg.CFToken
	m.domain = newCfg.Domain
	m.email = newCfg.Email
	m.dnsProvider = newCfg.DNSProvider
	m.dnsEnv = newCfg.DNSEnv

	// content 模式快路径：直接 SetContentPEM
	if newMode == "content" && len(newCfg.CertPEM) > 0 && len(newCfg.KeyPEM) > 0 {
		m.SetContentPEM(newCfg.CertPEM, newCfg.KeyPEM, newCfg.Domain)
		newTLS := m.TLSCert()
		return !pemEqual(oldTLS, newTLS), nil
	}

	// 其他模式调用 Start 重新初始化
	if err := m.Start(ctx); err != nil {
		return false, fmt.Errorf("cert reconfigure: %w", err)
	}

	newTLS := m.TLSCert()
	changed := !pemEqual(oldTLS, newTLS)
	if changed {
		m.logger.Info("cert reconfigured, PEM changed",
			"mode", string(newMode),
			"domain", newCfg.Domain,
			"acme_restarted", m.acmeStarted)
	}
	return changed, nil
}

// Start 根据 mode 初始化证书管理器。
// 对 ACME 模式（http/dns/certmagic）启动 certmagic 后台 goroutine。
// 对 content/self/file 模式仅加载 PEM 到内存。
//
// 移植自 Xboard-Node Manager.Start，适配 YunDu 已有的 EnsureCert 分发逻辑。
func (m *Manager) Start(ctx context.Context) error {
	mode := m.resolveMode()

	switch mode {
	case "none", "":
		return nil
	case "file":
		return m.startFile()
	case "self":
		return m.startSelfSigned()
	case "content":
		return m.startContent()
	case "http":
		return m.startACME(ctx, "http")
	case "dns":
		return m.startACME(ctx, "dns")
	case "certmagic":
		// certmagic 模式：复用现有 certmagicProvider（惰性初始化）
		// 不走统一的 startACME 路径，保持向后兼容
		if m.certmagicProv == nil {
			m.certmagicProv = newCertmagicProvider(m.certDir, m.cfToken, m.logger, m.eventBus)
		}
		if m.domain != "" {
			if _, _, err := m.certmagicProv.ensureCert(m.domain); err != nil {
				return fmt.Errorf("certmagic start: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown cert_mode: %q (supported: http, dns, certmagic, self, file, content, none)", mode)
	}
}

// resolveMode 返回当前有效证书模式（处理 auto_tls 向后兼容）。
// 移植自 Xboard-Node Manager.resolveMode。
func (m *Manager) resolveMode() string {
	mode := strings.ToLower(strings.TrimSpace(string(m.mode)))
	if mode != "" {
		return mode
	}
	// 向后兼容：cfToken 非空时默认 dns
	if m.cfToken != "" {
		return "dns"
	}
	return "none"
}

// resolveModeFor 对任意配置无副作用地判断模式（用于 Reconfigure 的 tearDown 决策）。
// 移植自 Xboard-Node resolveModeFor。
func resolveModeFor(cfg CertReconfigureConfig) string {
	mode := strings.ToLower(strings.TrimSpace(string(cfg.Mode)))
	if mode != "" {
		return mode
	}
	if cfg.CFToken != "" {
		return "dns"
	}
	if len(cfg.CertPEM) > 0 && len(cfg.KeyPEM) > 0 {
		return "content"
	}
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		return "file"
	}
	return "none"
}

// tearDownACME 取消运行中的 certmagic 后台 goroutine 并清理相关状态，
// 以便后续 Reconfigure 可以重新初始化 ACME。
//
// 移植自 Xboard-Node Manager.tearDownACME。
func (m *Manager) tearDownACME() {
	if m.acmeCancel != nil {
		m.acmeCancel()
		m.acmeCancel = nil
	}
	m.magic = nil
	m.certmagicProv = nil // 清除旧 provider，下次 Start 重新创建
	m.acmeStarted = false
	m.acmeFingerprint = ""
	// 清空 content 原子指针（若存在），避免旧 PEM 残留
	m.content.Store(nil)
}

// acmeFingerprint 生成稳定的 ACME 配置指纹字符串。
// 任何影响 ACME 签发的字段变化都会导致指纹变化，从而触发重新签发。
//
// 移植自 Xboard-Node acmeFingerprint，扩展了 dnsProvider/dnsEnv 字段。
func acmeFingerprint(cfg CertReconfigureConfig) string {
	keys := make([]string, 0, len(cfg.DNSEnv))
	for k := range cfg.DNSEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(string(cfg.Mode))))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(cfg.Domain))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(cfg.Email))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(cfg.DNSProvider))
	b.WriteByte('|')
	b.WriteString(strings.TrimSpace(cfg.CFToken))
	b.WriteByte('|')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(cfg.DNSEnv[k])
		b.WriteByte(';')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// loadPEMFromStorage 从 certmagic 存储读取指定域名的 cert + key 到内存缓存。
// 这是初始加载和续期刷新的统一入口。
//
// 移植自 Xboard-Node Manager.loadPEMFromStorage。
func (m *Manager) loadPEMFromStorage(ctx context.Context, storage certmagic.Storage, issuerKey, domain string) error {
	if issuerKey == "" || domain == "" {
		return fmt.Errorf("loadPEMFromStorage: issuerKey and domain are required")
	}
	certKey := certmagic.StorageKeys.SiteCert(issuerKey, domain)
	keyKey := certmagic.StorageKeys.SitePrivateKey(issuerKey, domain)

	certPEM, err := storage.Load(ctx, certKey)
	if err != nil {
		return fmt.Errorf("load cert %q: %w", certKey, err)
	}
	keyPEM, err := storage.Load(ctx, keyKey)
	if err != nil {
		return fmt.Errorf("load key %q: %w", keyKey, err)
	}
	if err := validateKeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("invalid cert/key pair in storage: %w", err)
	}

	// 原子更新内存缓存 + 持久化到标准路径
	bundle := &PEMBundle{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
		Domain:  domain,
	}
	m.cache.Store(domain, bundle)
	m.persistPEM(domain, certPEM, keyPEM)
	return nil
}

// validateKeyPair 验证 PEM 证书/私钥配对是否有效。
// 移植自 Xboard-Node validateKeyPair。
func validateKeyPair(certPEM, keyPEM []byte) error {
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return err
	}
	return nil
}

// TLSCert 返回当前域名的 PEM 材料快照。
// 优先从内存缓存读取，缓存未命中返回空值。
// 移植自 Xboard-Node Manager.TLSCert。
func (m *Manager) TLSCert() kernelTLSCert {
	if m.domain == "" {
		// 未指定域名时尝试从 content 加载
		if bundle := m.content.Load(); bundle != nil {
			return kernelTLSCert{CertPEM: bundle.CertPEM, KeyPEM: bundle.KeyPEM}
		}
		return kernelTLSCert{}
	}
	if bundle, ok := m.cache.Load(m.domain); ok {
		b := bundle.(*PEMBundle)
		return kernelTLSCert{CertPEM: b.CertPEM, KeyPEM: b.KeyPEM}
	}
	return kernelTLSCert{}
}

// HasCert 返回当前是否已有可用证书材料。
// 移植自 Xboard-Node Manager.HasCert。
func (m *Manager) HasCert() bool {
	t := m.TLSCert()
	return len(t.CertPEM) > 0 && len(t.KeyPEM) > 0
}

// CertRenewed 原子读取并清除续期标志。
// 返回 true 表示自上次调用以来发生过续期，调用方应触发内核热重载。
// 移植自 Xboard-Node Manager.CertRenewed。
func (m *Manager) CertRenewed() bool { return m.renewed.Swap(false) }

// pemEqual 比较两个 kernelTLSCert 的 PEM 内容是否相同。
// 移植自 Xboard-Node pemEqual。
func pemEqual(a, b kernelTLSCert) bool {
	return string(a.CertPEM) == string(b.CertPEM) && string(a.KeyPEM) == string(b.KeyPEM)
}

// kernelTLSCert 是内核消费的 TLS 证书材料快照。
// 与 Xboard-Node 的 kernel.TLSCert 对应。
type kernelTLSCert struct {
	CertPEM []byte
	KeyPEM  []byte
}

// startFile file 模式初始化：从文件读取 PEM 到内存。
// 移植自 Xboard-Node Manager.startFile。
func (m *Manager) startFile() error {
	certPEM, err := os.ReadFile(m.certFile)
	if err != nil {
		return fmt.Errorf("cert file: %w", err)
	}
	keyPEM, err := os.ReadFile(m.keyFile)
	if err != nil {
		return fmt.Errorf("key file: %w", err)
	}
	if err := validateKeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("invalid certificate pair: %w", err)
	}
	bundle := &PEMBundle{CertPEM: certPEM, KeyPEM: keyPEM, Domain: m.domain}
	m.cache.Store(m.domain, bundle)
	m.persistPEM(m.domain, certPEM, keyPEM)
	m.logger.Info("TLS certificate loaded from files",
		"domain", m.domain, "cert", m.certFile, "key", m.keyFile)
	return nil
}

// startSelfSigned self 模式初始化：生成自签名证书。
// 阶段 D: 升级为 ECDSA P-256 + 10 年有效期（替代 RSA 2048/365天）。
// 移植自 Xboard-Node Manager.startSelfSigned。
func (m *Manager) startSelfSigned() error {
	// 优先复用已持久化的自签名证书
	if m.loadPersistedPEMDomain(m.domain) {
		return nil
	}

	domain := m.domain
	if domain == "" {
		domain = "localhost"
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ECDSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: domain},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * 10 * time.Hour), // 10 年
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{domain}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	parsedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse generated cert for self-check: %w", err)
	}
	if err := parsedCert.VerifyHostname(domain); err != nil {
		return fmt.Errorf("self-signed cert VerifyHostname failed for %q: %w", domain, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal ECDSA key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	bundle := &PEMBundle{CertPEM: certPEM, KeyPEM: keyPEM, Domain: domain}
	m.cache.Store(domain, bundle)
	m.persistPEM(domain, certPEM, keyPEM)
	m.logger.Info("self-signed certificate generated", "domain", domain, "algo", "ECDSA P-256", "valid_days", 365*10)
	return nil
}

// startContent content 模式初始化：从已推送的 PEM 加载到内存。
// 移植自 Xboard-Node Manager.startContent。
func (m *Manager) startContent() error {
	bundle := m.content.Load()
	if bundle == nil || len(bundle.CertPEM) == 0 || len(bundle.KeyPEM) == 0 {
		// 无新推送内容，尝试从磁盘恢复
		if m.loadPersistedPEMDomain(m.domain) {
			return nil
		}
		return fmt.Errorf("cert_mode 'content' requires panel-pushed PEM (use SetContentPEM first)")
	}
	if err := validateKeyPair(bundle.CertPEM, bundle.KeyPEM); err != nil {
		return fmt.Errorf("invalid certificate content: %w", err)
	}
	m.persistPEM(m.domain, bundle.CertPEM, bundle.KeyPEM)
	m.logger.Info("TLS certificate loaded from panel content", "domain", m.domain)
	return nil
}

// startACME 启动 certmagic ACME 签发（http/dns 模式统一入口）。
//
// 移植自 Xboard-Node Manager.startACME，适配 YunDu 的 EventBus 通知机制。
// 与现有 certmagicProvider 不同的是，此路径直接操作 certmagic，
// 支持 Reconfigure/tearDownACME 生命周期管理。
func (m *Manager) startACME(ctx context.Context, mode string) error {
	if m.acmeStarted {
		return nil // 幂等：Reconfigure 已处理 tearDown
	}

	if m.domain == "" {
		return fmt.Errorf("cert.domain is required for ACME modes (http/dns)")
	}
	if err := os.MkdirAll(m.certDir, 0755); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}

	storage := &certmagic.FileStorage{Path: m.certDir}

	var magic *certmagic.Config
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(_ certmagic.Certificate) (*certmagic.Config, error) {
			return magic, nil
		},
	})

	magic = certmagic.New(cache, certmagic.Config{
		Storage: storage,
		OnEvent: func(evtCtx context.Context, event string, data map[string]any) error {
			// 仅响应续期事件；初始加载在 ObtainCertSync 后显式执行
			if event != "cert_obtained" {
				return nil
			}
			renewal, _ := data["renewal"].(bool)
			if !renewal {
				return nil
			}
			issuerKey, _ := data["issuer"].(string)
			if issuerKey == "" {
				return nil
			}
			if err := m.loadPEMFromStorage(evtCtx, storage, issuerKey, m.domain); err != nil {
				m.logger.Error("failed to reload cert after renewal", "domain", m.domain, "error", err)
				return nil
			}
			m.renewed.Store(true)
			// 发布事件到 EventBus（xray/nginx 订阅者触发热重载）
			if m.eventBus != nil {
				bundle := &PEMBundle{CertPEM: m.TLSCert().CertPEM, KeyPEM: m.TLSCert().KeyPEM, Domain: m.domain}
				m.eventBus.Publish(CertEvent{
					Type:     CertEventRenewed,
					Domain:   m.domain,
					BundleID: m.domain,
					PEM:      bundle,
				})
			}
			m.logger.Info("TLS certificate reloaded after renewal", "domain", m.domain)
			return nil
		},
	})

	issuer := certmagic.ACMEIssuer{
		CA:    certmagic.LetsEncryptProductionCA,
		Email: m.email,
	}

	if mode == "dns" {
		// DNS-01 模式：构建 DNS solver
		solver, err := m.buildDNSSolver()
		if err != nil {
			return fmt.Errorf("build dns solver: %w", err)
		}
		issuer.DNS01Solver = solver
		issuer.DisableHTTPChallenge = true
		issuer.DisableTLSALPNChallenge = true
	} else {
		// HTTP-01 模式
		issuer.AltHTTPPort = 80
		issuer.DisableTLSALPNChallenge = true
	}

	magic.Issuers = []certmagic.Issuer{certmagic.NewACMEIssuer(magic, issuer)}
	m.magic = magic

	// 派生 context，以便 Reconfigure/tearDownACME 可取消后台 goroutine
	acmeCtx, acmeCancel := context.WithCancel(ctx)

	if err := magic.ObtainCertSync(acmeCtx, m.domain); err != nil {
		acmeCancel()
		return fmt.Errorf("obtain certificate: %w", err)
	}

	issuerKey := magic.Issuers[0].IssuerKey()
	if err := m.loadPEMFromStorage(acmeCtx, storage, issuerKey, m.domain); err != nil {
		acmeCancel()
		return fmt.Errorf("load cert from storage: %w", err)
	}

	if err := magic.ManageAsync(acmeCtx, []string{m.domain}); err != nil {
		acmeCancel()
		return fmt.Errorf("start cert manager: %w", err)
	}

	m.acmeCancel = acmeCancel
	m.acmeFingerprint = acmeFingerprint(CertReconfigureConfig{
		Mode:        CertMode(mode),
		Domain:      m.domain,
		Email:       m.email,
		DNSProvider: m.dnsProvider,
		DNSEnv:      m.dnsEnv,
		CFToken:     m.cfToken,
	})
	m.acmeStarted = true

	// 发布首次签发事件
	if m.eventBus != nil {
		t := m.TLSCert()
		m.eventBus.Publish(CertEvent{
			Type:     CertEventObtained,
			Domain:   m.domain,
			BundleID: m.domain,
			PEM:      &PEMBundle{CertPEM: t.CertPEM, KeyPEM: t.KeyPEM, Domain: m.domain},
		})
	}
	m.logger.Info("ACME certificate obtained",
		"mode", mode, "domain", m.domain, "fingerprint", m.acmeFingerprint[:12])
	return nil
}

// buildDNSSolver 构建 certmagic DNS01Solver。
// 当前仅支持 Cloudflare（通过 cfToken），阶段 B 将扩展为 21 个 provider。
func (m *Manager) buildDNSSolver() (*certmagic.DNS01Solver, error) {
	provider, err := m.newDNSProvider()
	if err != nil {
		return nil, err
	}
	return &certmagic.DNS01Solver{
		DNSManager: certmagic.DNSManager{
			DNSProvider: provider,
		},
	}, nil
}

// newDNSProvider 创建 DNS provider 实例。
// 阶段 B 将从 dnsproviders 注册表查找；当前仅支持 Cloudflare。
func (m *Manager) newDNSProvider() (certmagic.DNSProvider, error) {
	name := strings.TrimSpace(m.dnsProvider)
	if name == "" && m.cfToken != "" {
		name = "cloudflare" // 默认 Cloudflare（向后兼容）
	}
	if name == "" {
		return nil, fmt.Errorf("dns_provider is required for cert_mode=dns")
	}

	// 阶段 B 占位：尝试从 dnsproviders 注册表查找
	if p, ok := dnsproviders.Get(name); ok {
		env := m.dnsEnv
		if env == nil {
			env = map[string]string{}
		}
		// 向后兼容：cfToken 映射到 CLOUDFLARE_DNS_API_TOKEN
		if name == "cloudflare" && m.cfToken != "" && env["CLOUDFLARE_DNS_API_TOKEN"] == "" {
			env["CLOUDFLARE_DNS_API_TOKEN"] = m.cfToken
		}
		return p.Build(env)
	}

	return nil, fmt.Errorf("unsupported dns_provider: %q (current: cloudflare; stage B will add 21 providers)", name)
}

// loadPersistedPEMDomain 从磁盘加载指定域名的 PEM 到内存缓存。
// 返回 true 表示加载成功。
func (m *Manager) loadPersistedPEMDomain(domain string) bool {
	if domain == "" {
		return false
	}
	certPath := filepath.Join(m.certDir, domain, "fullchain.pem")
	keyPath := filepath.Join(m.certDir, domain, "privkey.pem")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return false
	}
	if err := validateKeyPair(certPEM, keyPEM); err != nil {
		m.logger.Warn("persisted cert invalid, will regenerate",
			"domain", domain, "error", err)
		return false
	}
	m.cache.Store(domain, &PEMBundle{CertPEM: certPEM, KeyPEM: keyPEM, Domain: domain})
	m.logger.Info("loaded persisted certificate from disk",
		"domain", domain, "dir", m.certDir)
	return true
}
