package cert

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ACMEDefaults 面板全局 ACME 默认账户（system_settings.acme 组）。
// 证书创建时未显式指定账户时自动继承；DNS 凭证加密存储。
type ACMEDefaults struct {
	Email                string
	DirURL               string
	ChallengeType        string
	DNSProvider          string
	CredentialsEncrypted string
	HasCredentials       bool
}

// ACMEDefaultsStore 抽象 ACME 默认账户存储，便于注入与测试。
type ACMEDefaultsStore interface {
	Load(ctx context.Context) (*ACMEDefaults, error)
	Save(ctx context.Context, d *ACMEDefaults) error
}

// ACMEDefaultsRepo 基于 system_settings 表存储（group=acme）。
type ACMEDefaultsRepo struct {
	pool *pgxpool.Pool
}

func NewACMEDefaultsRepo(pool *pgxpool.Pool) *ACMEDefaultsRepo {
	return &ACMEDefaultsRepo{pool: pool}
}

func (r *ACMEDefaultsRepo) Load(ctx context.Context) (*ACMEDefaults, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT setting_key, value_json #>> '{}' FROM system_settings WHERE setting_group = 'acme'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	d := &ACMEDefaults{}
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "email":
			d.Email = val
		case "dir_url":
			d.DirURL = val
		case "challenge_type":
			d.ChallengeType = val
		case "dns_provider":
			d.DNSProvider = val
		case "dns_credentials":
			d.CredentialsEncrypted = val
			d.HasCredentials = val != ""
		}
	}
	return d, rows.Err()
}

func (r *ACMEDefaultsRepo) Save(ctx context.Context, d *ACMEDefaults) error {
	values := []struct {
		key  string
		val  string
		sec  bool
		desc string
	}{
		{"email", d.Email, false, "ACME 账户邮箱"},
		{"dir_url", d.DirURL, false, "ACME 目录 URL（默认 Let's Encrypt）"},
		{"challenge_type", d.ChallengeType, false, "验证方式（dns-01 / http-01）"},
		{"dns_provider", d.DNSProvider, false, "DNS provider（cloudflare/alidns/dnspod/gandi/namesilo）"},
		{"dns_credentials", d.CredentialsEncrypted, true, "DNS 凭证（加密存储）"},
	}
	for _, v := range values {
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO system_settings (setting_group, setting_key, value_json, is_secret, description)
			VALUES ('acme', $1, to_jsonb($2::text), $3, $4)
			ON CONFLICT (setting_group, setting_key)
			DO UPDATE SET value_json = EXCLUDED.value_json, is_secret = EXCLUDED.is_secret, description = EXCLUDED.description, updated_at = now()`,
			v.key, v.val, v.sec, v.desc); err != nil {
			return err
		}
	}
	return nil
}

// ACMEDefaultsDTO 是管理 API 的对外结构（凭证不回显明文）。
type ACMEDefaultsDTO struct {
	Email          string `json:"email"`
	DirURL         string `json:"dir_url"`
	ChallengeType  string `json:"challenge_type"`
	DNSProvider    string `json:"dns_provider"`
	HasCredentials bool   `json:"has_credentials"`
}

func defaultsToDTO(d *ACMEDefaults) ACMEDefaultsDTO {
	if d == nil {
		return ACMEDefaultsDTO{}
	}
	return ACMEDefaultsDTO{
		Email:          d.Email,
		DirURL:         d.DirURL,
		ChallengeType:  d.ChallengeType,
		DNSProvider:    d.DNSProvider,
		HasCredentials: d.HasCredentials,
	}
}

// parseCredentialsJSON 兼容明文 map 与加密串两种存储形态（历史数据）。
func parseCredentialsJSON(raw string) map[string]string {
	var m map[string]string
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		return m
	}
	return nil
}
