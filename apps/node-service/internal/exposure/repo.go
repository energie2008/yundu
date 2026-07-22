package exposure

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExposureRepo 处理 edge_exposures 数据访问
type ExposureRepo struct {
	pool *pgxpool.Pool
}

func NewExposureRepo(pool *pgxpool.Pool) *ExposureRepo {
	return &ExposureRepo{pool: pool}
}

const exposureColumns = `id, server_id, code, name, exposure_mode, public_hostname, public_port, origin_host, origin_port,
	nginx_enabled, nginx_ws_path, nginx_host_header, nginx_extra_conf, tls_profile_id, cf_tunnel_token_encrypted,
	cf_tunnel_id, cf_tunnel_name, cf_protocol, cf_no_tls_verify, cf_origin_server_name, argo_ws_token_encrypted,
	status, health_check_url, last_health_check_at, last_health_status, metadata, created_at, updated_at`

func scanExposure(row pgx.Row, e *EdgeExposure) error {
	return row.Scan(
		&e.ID, &e.ServerID, &e.Code, &e.Name, &e.ExposureMode, &e.PublicHostname, &e.PublicPort, &e.OriginHost,
		&e.OriginPort, &e.NginxEnabled, &e.NginxWSPath, &e.NginxHostHeader, &e.NginxExtraConf, &e.TLSProfileID,
		&e.CFTunnelTokenEncrypted, &e.CFTunnelID, &e.CFTunnelName, &e.CFProtocol, &e.CFNoTLSVerify,
		&e.CFOriginServerName, &e.ArgoWSTokenEncrypted, &e.Status, &e.HealthCheckURL, &e.LastHealthCheckAt,
		&e.LastHealthStatus, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
}

func (r *ExposureRepo) Create(ctx context.Context, e *EdgeExposure) error {
	query := `
		INSERT INTO edge_exposures (id, server_id, code, name, exposure_mode, public_hostname, public_port, origin_host,
			origin_port, nginx_enabled, nginx_ws_path, nginx_host_header, nginx_extra_conf, tls_profile_id,
			cf_tunnel_token_encrypted, cf_tunnel_id, cf_tunnel_name, cf_protocol, cf_no_tls_verify, cf_origin_server_name,
			argo_ws_token_encrypted, status, health_check_url, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		e.ID, e.ServerID, e.Code, e.Name, e.ExposureMode, e.PublicHostname, e.PublicPort, e.OriginHost, e.OriginPort,
		e.NginxEnabled, e.NginxWSPath, e.NginxHostHeader, e.NginxExtraConf, e.TLSProfileID, e.CFTunnelTokenEncrypted,
		e.CFTunnelID, e.CFTunnelName, e.CFProtocol, e.CFNoTLSVerify, e.CFOriginServerName, e.ArgoWSTokenEncrypted,
		e.Status, e.HealthCheckURL, e.Metadata,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
}

func (r *ExposureRepo) GetByID(ctx context.Context, id uuid.UUID) (*EdgeExposure, error) {
	query := fmt.Sprintf(`SELECT %s FROM edge_exposures WHERE id = $1`, exposureColumns)
	e := &EdgeExposure{}
	if err := scanExposure(r.pool.QueryRow(ctx, query, id), e); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ExposureRepo) GetByCode(ctx context.Context, code string) (*EdgeExposure, error) {
	query := fmt.Sprintf(`SELECT %s FROM edge_exposures WHERE code = $1`, exposureColumns)
	e := &EdgeExposure{}
	if err := scanExposure(r.pool.QueryRow(ctx, query, code), e); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ExposureRepo) GetByServerID(ctx context.Context, serverID uuid.UUID) (*EdgeExposure, error) {
	query := fmt.Sprintf(`SELECT %s FROM edge_exposures WHERE server_id = $1 ORDER BY created_at DESC LIMIT 1`, exposureColumns)
	e := &EdgeExposure{}
	if err := scanExposure(r.pool.QueryRow(ctx, query, serverID), e); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *ExposureRepo) Update(ctx context.Context, e *EdgeExposure) error {
	query := `
		UPDATE edge_exposures SET
			name = $2, exposure_mode = $3, public_hostname = $4, public_port = $5, origin_host = $6, origin_port = $7,
			nginx_enabled = $8, nginx_ws_path = $9, nginx_host_header = $10, nginx_extra_conf = $11, tls_profile_id = $12,
			cf_protocol = $13, cf_no_tls_verify = $14, cf_origin_server_name = $15, status = $16, health_check_url = $17,
			metadata = $18, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query,
		e.ID, e.Name, e.ExposureMode, e.PublicHostname, e.PublicPort, e.OriginHost, e.OriginPort,
		e.NginxEnabled, e.NginxWSPath, e.NginxHostHeader, e.NginxExtraConf, e.TLSProfileID,
		e.CFProtocol, e.CFNoTLSVerify, e.CFOriginServerName, e.Status, e.HealthCheckURL, e.Metadata,
	)
	return err
}

func (r *ExposureRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM edge_exposures WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *ExposureRepo) List(ctx context.Context, page, pageSize int, status string) ([]*EdgeExposure, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1
	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}
	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM edge_exposures WHERE %s`, whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s FROM edge_exposures WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		exposureColumns, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var exposures []*EdgeExposure
	for rows.Next() {
		e := &EdgeExposure{}
		if err := scanExposure(rows, e); err != nil {
			return nil, 0, err
		}
		exposures = append(exposures, e)
	}
	return exposures, total, rows.Err()
}

// NginxConfigRepo 处理 nginx_generated_configs 数据访问
type NginxConfigRepo struct {
	pool *pgxpool.Pool
}

func NewNginxConfigRepo(pool *pgxpool.Pool) *NginxConfigRepo {
	return &NginxConfigRepo{pool: pool}
}

func (r *NginxConfigRepo) GetByExposureAndHash(ctx context.Context, exposureID uuid.UUID, hash string) (*NginxGeneratedConfig, error) {
	query := `SELECT id, exposure_id, config_content, config_hash, schema_version, generated_at, deployed_at, deploy_status, deploy_error
		FROM nginx_generated_configs WHERE exposure_id = $1 AND config_hash = $2`
	c := &NginxGeneratedConfig{}
	err := r.pool.QueryRow(ctx, query, exposureID, hash).Scan(
		&c.ID, &c.ExposureID, &c.ConfigContent, &c.ConfigHash, &c.SchemaVersion, &c.GeneratedAt,
		&c.DeployedAt, &c.DeployStatus, &c.DeployError,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// CreateIfAbsent 在 (exposure_id, config_hash) 已存在时不重复插入，返回最终记录
func (r *NginxConfigRepo) CreateIfAbsent(ctx context.Context, c *NginxGeneratedConfig) (*NginxGeneratedConfig, error) {
	existing, err := r.GetByExposureAndHash(ctx, c.ExposureID, c.ConfigHash)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	query := `INSERT INTO nginx_generated_configs (id, exposure_id, config_content, config_hash, schema_version)
		VALUES ($1, $2, $3, $4, $5) RETURNING generated_at`
	if err := r.pool.QueryRow(ctx, query,
		c.ID, c.ExposureID, c.ConfigContent, c.ConfigHash, c.SchemaVersion,
	).Scan(&c.GeneratedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// CompatRuleRepo 处理 exposure_compat_rules 数据访问
type CompatRuleRepo struct {
	pool *pgxpool.Pool
}

func NewCompatRuleRepo(pool *pgxpool.Pool) *CompatRuleRepo {
	return &CompatRuleRepo{pool: pool}
}

// FindMatch 查找匹配的兼容规则（按 protocol/transport/security/exposure_mode 精确匹配）
func (r *CompatRuleRepo) FindMatch(ctx context.Context, protocol, transport, security, exposureMode string) (*ExposureCompatRule, error) {
	query := `SELECT id, protocol_type, transport_type, security_type, exposure_mode, is_allowed, reason, created_at
		FROM exposure_compat_rules
		WHERE protocol_type = $1 AND exposure_mode = $2
			AND (transport_type IS NOT DISTINCT FROM $3 OR transport_type IS NULL)
			AND (security_type IS NOT DISTINCT FROM $4 OR security_type IS NULL)
		ORDER BY (transport_type IS NOT NULL) DESC, (security_type IS NOT NULL) DESC
		LIMIT 1`
	rule := &ExposureCompatRule{}
	err := r.pool.QueryRow(ctx, query, protocol, exposureMode, nullableStr(transport), nullableStr(security)).Scan(
		&rule.ID, &rule.ProtocolType, &rule.TransportType, &rule.SecurityType, &rule.ExposureMode,
		&rule.IsAllowed, &rule.Reason, &rule.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rule, nil
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
