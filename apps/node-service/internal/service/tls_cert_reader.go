package service

import (
	"context"

	"github.com/airport-panel/node-service/internal/cert"
)

// tlsCertReaderAdapter P1-C: 将 cert.CertificateRepo 适配为 TLSCertReader 接口。
type tlsCertReaderAdapter struct {
	repo *cert.CertificateRepo
}

// NewTLSCertReaderAdapter P1-C: 创建 TLSCertReader 适配器。
func NewTLSCertReaderAdapter(repo *cert.CertificateRepo) TLSCertReader {
	return &tlsCertReaderAdapter{repo: repo}
}

func (a *tlsCertReaderAdapter) FindCertPEMBySNI(ctx context.Context, sni string) (string, string, bool) {
	if a.repo == nil || sni == "" {
		return "", "", false
	}
	c, err := a.repo.FindActiveBySNI(ctx, sni)
	if err != nil || c == nil {
		return "", "", false
	}
	certPEM := ""
	if c.CertPEM != nil {
		certPEM = *c.CertPEM
	}
	keyPEM := ""
	if c.KeyPEMEncrypted != nil {
		keyPEM = *c.KeyPEMEncrypted
	}
	if certPEM == "" || keyPEM == "" {
		return "", "", false
	}
	return certPEM, keyPEM, true
}
