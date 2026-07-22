package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// GenerateSelfSignedCertPEM 生成 ECDSA P-256 自签名证书（10 年有效期）。
// 返回 certPEM 和 keyPEM（PEM 编码），用于 TLS 节点无有效证书时的自动兜底。
//
// P0-1 修复：
//   - 新增 IP SAN 支持（net.ParseIP 判断 SNI 是域名还是 IP）
//   - 新增 CA 标志位（KeyUsageCertSign / IsCA / BasicConstraintsValid）
//   - 新增 NotBefore 时钟偏移容错（-1h）
//   - 新增生成后 VerifyHostname 自检（失败则返回 error 阻断部署）
func GenerateSelfSignedCertPEM(domain string) (certPEM, keyPEM string, err error) {
	if domain == "" {
		return "", "", fmt.Errorf("domain is required for self-signed cert")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ECDSA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial number: %w", err)
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

	// SAN 逻辑：如果 domain 是 IP 地址则写入 IPAddresses，否则写入 DNSNames
	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{domain}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}

	// P0-1: 生成后 VerifyHostname 自检
	// Go 1.15+ crypto/tls 移除了 CommonName 回退匹配，只认 SAN 扩展。
	// 如果 SAN 写入失败（编码问题等），客户端（sing-box/Xray）会报
	// x509: certificate is not valid for any names。此处提前阻断。
	parsedCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", "", fmt.Errorf("parse generated cert for self-check: %w", err)
	}
	if err := parsedCert.VerifyHostname(domain); err != nil {
		return "", "", fmt.Errorf("self-signed cert VerifyHostname failed for %q: %w", domain, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshal ECDSA key: %w", err)
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certPEMBytes), string(keyPEMBytes), nil
}
