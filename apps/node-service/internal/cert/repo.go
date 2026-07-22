package cert

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CertificateRepo 处理 tls_certificates 数据访问
type CertificateRepo struct {
	pool *pgxpool.Pool
}

func NewCertificateRepo(pool *pgxpool.Pool) *CertificateRepo {
	return &CertificateRepo{pool: pool}
}

const certColumns = `id, code, name, cert_type, common_name, sans, provider, cert_pem, key_pem_encrypted, ca_pem,
	fingerprint_sha256, issued_at, expires_at, auto_renew, renew_days_before, renew_status, renew_last_attempt_at,
	renew_last_error, deploy_mode, acme_account_email, acme_challenge_type, acme_dns_provider, acme_credentials_encrypted,
	cloudflare_zone_id, status, created_by_admin_id, created_at, updated_at`

func scanCertificate(row pgx.Row, c *Certificate) error {
	return row.Scan(
		&c.ID, &c.Code, &c.Name, &c.CertType, &c.CommonName, &c.SANs, &c.Provider, &c.CertPEM, &c.KeyPEMEncrypted,
		&c.CAPEM, &c.FingerprintSHA256, &c.IssuedAt, &c.ExpiresAt, &c.AutoRenew, &c.RenewDaysBefore, &c.RenewStatus,
		&c.RenewLastAttemptAt, &c.RenewLastError, &c.DeployMode, &c.ACMEAccountEmail, &c.ACMEChallengeType,
		&c.ACMEDNSProvider, &c.ACMECredentialsEncrypted, &c.CloudflareZoneID, &c.Status, &c.CreatedByAdminID,
		&c.CreatedAt, &c.UpdatedAt,
	)
}

func (r *CertificateRepo) Create(ctx context.Context, c *Certificate) error {
	query := `
		INSERT INTO tls_certificates (id, code, name, cert_type, common_name, sans, provider, cert_pem, key_pem_encrypted,
			ca_pem, fingerprint_sha256, issued_at, expires_at, auto_renew, renew_days_before, renew_status, renew_last_attempt_at,
			renew_last_error, deploy_mode, acme_account_email, acme_challenge_type, acme_dns_provider, acme_credentials_encrypted,
			cloudflare_zone_id, status, created_by_admin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		c.ID, c.Code, c.Name, c.CertType, c.CommonName, c.SANs, c.Provider, c.CertPEM, c.KeyPEMEncrypted,
		c.CAPEM, c.FingerprintSHA256, c.IssuedAt, c.ExpiresAt, c.AutoRenew, c.RenewDaysBefore, c.RenewStatus,
		c.RenewLastAttemptAt, c.RenewLastError, c.DeployMode, c.ACMEAccountEmail, c.ACMEChallengeType,
		c.ACMEDNSProvider, c.ACMECredentialsEncrypted, c.CloudflareZoneID, c.Status, c.CreatedByAdminID,
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

func (r *CertificateRepo) GetByID(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	query := fmt.Sprintf(`SELECT %s FROM tls_certificates WHERE id = $1`, certColumns)
	c := &Certificate{}
	if err := scanCertificate(r.pool.QueryRow(ctx, query, id), c); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *CertificateRepo) GetByCode(ctx context.Context, code string) (*Certificate, error) {
	query := fmt.Sprintf(`SELECT %s FROM tls_certificates WHERE code = $1`, certColumns)
	c := &Certificate{}
	if err := scanCertificate(r.pool.QueryRow(ctx, query, code), c); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *CertificateRepo) Update(ctx context.Context, c *Certificate) error {
	query := `
		UPDATE tls_certificates SET
			name = $2, common_name = $3, sans = $4, cert_pem = $5, ca_pem = $6, fingerprint_sha256 = $7,
			expires_at = $8, auto_renew = $9, renew_days_before = $10, status = $11,
			acme_account_email = $12, acme_challenge_type = $13, acme_dns_provider = $14,
			acme_credentials_encrypted = $15, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.Name, c.CommonName, c.SANs, c.CertPEM, c.CAPEM, c.FingerprintSHA256,
		c.ExpiresAt, c.AutoRenew, c.RenewDaysBefore, c.Status,
		c.ACMEAccountEmail, c.ACMEChallengeType, c.ACMEDNSProvider, c.ACMECredentialsEncrypted,
	)
	return err
}

func (r *CertificateRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE tls_certificates SET status = 'deleted', updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *CertificateRepo) SetRenewPending(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE tls_certificates SET renew_status = 'pending', renew_last_attempt_at = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, time.Now())
	return err
}

// UpdateRenewal 更新证书续期成功后的字段：cert_pem、key_pem_encrypted、ca_pem、
// fingerprint_sha256、issued_at、expires_at、renew_status='succeeded'、renew_last_attempt_at。
func (r *CertificateRepo) UpdateRenewal(ctx context.Context, c *Certificate) error {
	query := `
		UPDATE tls_certificates SET
			cert_pem = $2, key_pem_encrypted = $3, ca_pem = $4, fingerprint_sha256 = $5,
			issued_at = $6, expires_at = $7, renew_status = $8, renew_last_attempt_at = $9,
			renew_last_error = NULL, updated_at = now()
		WHERE id = $1`
	now := time.Now()
	_, err := r.pool.Exec(ctx, query,
		c.ID, c.CertPEM, c.KeyPEMEncrypted, c.CAPEM, c.FingerprintSHA256,
		c.IssuedAt, c.ExpiresAt, c.RenewStatus, now,
	)
	return err
}

// SetRenewFailed 标记续期失败并写入错误信息。
func (r *CertificateRepo) SetRenewFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	query := `UPDATE tls_certificates
		SET renew_status = 'failed', renew_last_attempt_at = $2, renew_last_error = $3, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, time.Now(), errMsg)
	return err
}

func (r *CertificateRepo) List(ctx context.Context, page, pageSize int, status string, expiresWithinDays int) ([]*Certificate, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	if expiresWithinDays > 0 {
		where = append(where, fmt.Sprintf("expires_at IS NOT NULL AND expires_at <= $%d", argIdx))
		args = append(args, time.Now().Add(time.Duration(expiresWithinDays)*24*time.Hour))
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM tls_certificates WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s FROM tls_certificates WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		certColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var certs []*Certificate
	for rows.Next() {
		c := &Certificate{}
		if err := scanCertificate(rows, c); err != nil {
			return nil, 0, err
		}
		certs = append(certs, c)
	}
	return certs, total, rows.Err()
}

func (r *CertificateRepo) ListExpiringSoon(ctx context.Context, days int) ([]*Certificate, error) {
	threshold := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	query := fmt.Sprintf(`
		SELECT %s FROM tls_certificates
		WHERE expires_at IS NOT NULL AND expires_at <= $1 AND renew_status != 'success' AND status != 'deleted'
		ORDER BY expires_at ASC`, certColumns)
	rows, err := r.pool.Query(ctx, query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []*Certificate
	for rows.Next() {
		c := &Certificate{}
		if err := scanCertificate(rows, c); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

// CertDeployRepo 处理 cert_deploy_records 数据访问
type CertDeployRepo struct {
	pool *pgxpool.Pool
}

func NewCertDeployRepo(pool *pgxpool.Pool) *CertDeployRepo {
	return &CertDeployRepo{pool: pool}
}

func (r *CertDeployRepo) ListByCertificateID(ctx context.Context, certID uuid.UUID) ([]*CertDeployRecord, error) {
	query := `SELECT id, certificate_id, server_id, deploy_status, deploy_path, deployed_at, error_message, created_at
		FROM cert_deploy_records WHERE certificate_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, certID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*CertDeployRecord
	for rows.Next() {
		rec := &CertDeployRecord{}
		if err := rows.Scan(&rec.ID, &rec.CertificateID, &rec.ServerID, &rec.DeployStatus, &rec.DeployPath,
			&rec.DeployedAt, &rec.ErrorMessage, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *CertDeployRepo) Upsert(ctx context.Context, rec *CertDeployRecord) error {
	query := `
		INSERT INTO cert_deploy_records (id, certificate_id, server_id, deploy_status, deploy_path, deployed_at, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (certificate_id, server_id) DO UPDATE SET
			deploy_status = EXCLUDED.deploy_status, deploy_path = EXCLUDED.deploy_path,
			deployed_at = EXCLUDED.deployed_at, error_message = EXCLUDED.error_message
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		rec.ID, rec.CertificateID, rec.ServerID, rec.DeployStatus, rec.DeployPath, rec.DeployedAt, rec.ErrorMessage,
	).Scan(&rec.CreatedAt)
}

// TLSProfileRepo 处理 tls_profiles 数据访问
type TLSProfileRepo struct {
	pool *pgxpool.Pool
}

func NewTLSProfileRepo(pool *pgxpool.Pool) *TLSProfileRepo {
	return &TLSProfileRepo{pool: pool}
}

const profileColumns = `id, code, name, tls_mode, server_name, certificate_id, allow_insecure, utls_fingerprint, alpn,
	min_version, max_version, ech_enabled, ech_config_encrypted, ech_key_encrypted, reality_public_key, reality_private_key_encrypted,
	reality_short_ids, reality_spider_x, reality_dest, notes, created_at, updated_at`

func scanProfile(row pgx.Row, p *TLSProfile) error {
	return row.Scan(
		&p.ID, &p.Code, &p.Name, &p.TLSMode, &p.ServerName, &p.CertificateID, &p.AllowInsecure, &p.UTLSFingerprint,
		&p.ALPN, &p.MinVersion, &p.MaxVersion, &p.ECHEnabled, &p.ECHConfigEncrypted, &p.ECHKeyEncrypted, &p.RealityPublicKey,
		&p.RealityPrivateKeyEncrypted, &p.RealityShortIDs, &p.RealitySpiderX, &p.RealityDest, &p.Notes,
		&p.CreatedAt, &p.UpdatedAt,
	)
}

func (r *TLSProfileRepo) Create(ctx context.Context, p *TLSProfile) error {
	query := `
		INSERT INTO tls_profiles (id, code, name, tls_mode, server_name, certificate_id, allow_insecure, utls_fingerprint,
			alpn, min_version, max_version, ech_enabled, ech_config_encrypted, ech_key_encrypted, reality_public_key, reality_private_key_encrypted,
			reality_short_ids, reality_spider_x, reality_dest, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		p.ID, p.Code, p.Name, p.TLSMode, p.ServerName, p.CertificateID, p.AllowInsecure, p.UTLSFingerprint,
		p.ALPN, p.MinVersion, p.MaxVersion, p.ECHEnabled, p.ECHConfigEncrypted, p.ECHKeyEncrypted, p.RealityPublicKey,
		p.RealityPrivateKeyEncrypted, p.RealityShortIDs, p.RealitySpiderX, p.RealityDest, p.Notes,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
}

func (r *TLSProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*TLSProfile, error) {
	query := fmt.Sprintf(`SELECT %s FROM tls_profiles WHERE id = $1`, profileColumns)
	p := &TLSProfile{}
	if err := scanProfile(r.pool.QueryRow(ctx, query, id), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *TLSProfileRepo) GetByCode(ctx context.Context, code string) (*TLSProfile, error) {
	query := fmt.Sprintf(`SELECT %s FROM tls_profiles WHERE code = $1`, profileColumns)
	p := &TLSProfile{}
	if err := scanProfile(r.pool.QueryRow(ctx, query, code), p); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *TLSProfileRepo) Update(ctx context.Context, p *TLSProfile) error {
	query := `
		UPDATE tls_profiles SET
			name = $2, tls_mode = $3, server_name = $4, certificate_id = $5, allow_insecure = $6, utls_fingerprint = $7,
			alpn = $8, min_version = $9, max_version = $10, ech_enabled = $11, ech_config_encrypted = $12,
			ech_key_encrypted = $13, notes = $14, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.TLSMode, p.ServerName, p.CertificateID, p.AllowInsecure, p.UTLSFingerprint,
		p.ALPN, p.MinVersion, p.MaxVersion, p.ECHEnabled, p.ECHConfigEncrypted, p.ECHKeyEncrypted, p.Notes,
	)
	return err
}

func (r *TLSProfileRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM tls_profiles WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *TLSProfileRepo) CountUsageInExposures(ctx context.Context, id uuid.UUID) (int, error) {
	// edge_exposures.tls_profile_id 引用本表 (迁移 000010)
	query := `SELECT COUNT(*) FROM edge_exposures WHERE tls_profile_id = $1`
	var count int
	if err := r.pool.QueryRow(ctx, query, id).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *TLSProfileRepo) List(ctx context.Context, page, pageSize int) ([]*TLSProfile, int, error) {
	countQuery := `SELECT COUNT(*) FROM tls_profiles`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s FROM tls_profiles ORDER BY created_at DESC LIMIT $1 OFFSET $2`, profileColumns)
	rows, err := r.pool.Query(ctx, query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var profiles []*TLSProfile
	for rows.Next() {
		p := &TLSProfile{}
		if err := scanProfile(rows, p); err != nil {
			return nil, 0, err
		}
		profiles = append(profiles, p)
	}
	return profiles, total, rows.Err()
}
