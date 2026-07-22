package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// capability_repo.go 实现 P0-6：内核能力矩阵 + 证书包 数据访问层。

// KernelCapability 内核能力矩阵条目
type KernelCapability struct {
	ID           int64
	Kernel       string // xray / sing-box
	MinVersion   string
	MaxVersion   string
	Protocol     string
	Transport    string
	Security     string
	Feature      string // * = 通配
	SupportLevel string // native / degradable / blocked
	DowngradeTo  map[string]interface{}
	Message      string
	CreatedAt    time.Time
}

// CertBundle 证书包（PEM-only）
type CertBundle struct {
	ID        uuid.UUID
	Provider  string // acme / self / content / cf-origin
	Mode      string // file / content / acme / self
	CertPEM   string
	KeyPEM    string
	SAN       []string
	NotAfter  *time.Time
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CapabilityRepo 能力矩阵 + 证书包仓库
type CapabilityRepo struct {
	pool *pgxpool.Pool
}

func NewCapabilityRepo(pool *pgxpool.Pool) *CapabilityRepo {
	return &CapabilityRepo{pool: pool}
}

// CheckSupport 查询指定内核对 protocol+transport+security+feature 的支持级别。
// 查询顺序：精确匹配 → 通配 feature(*) → 通配 protocol(*) → 通配 transport(*)。
// 返回首个匹配条目；无匹配时返回 nil（表示无数据，调用方自行判断）。
func (r *CapabilityRepo) CheckSupport(ctx context.Context, kernel, protocol, transport, security, feature string) (*KernelCapability, error) {
	// 按优先级查询：精确 → 通配
	queries := []struct {
		feature, protocol, transport string
	}{
		{feature, protocol, transport},        // 精确匹配
		{"*", protocol, transport},             // 通配 feature
		{feature, protocol, "*"},               // 通配 transport
		{"*", protocol, "*"},                   // 通配 feature+transport
		{feature, "*", transport},              // 通配 protocol
		{"*", "*", transport},                  // 通配 protocol+feature
		{"*", "*", "*"},                        // 全通配
	}

	for _, q := range queries {
		cap, err := r.queryOne(ctx, kernel, q.protocol, q.transport, security, q.feature)
		if err != nil {
			return nil, err
		}
		if cap != nil {
			return cap, nil
		}
	}
	return nil, nil
}

func (r *CapabilityRepo) queryOne(ctx context.Context, kernel, protocol, transport, security, feature string) (*KernelCapability, error) {
	query := `
		SELECT id, kernel, COALESCE(min_version,''), COALESCE(max_version,''),
			protocol, transport, security, feature, support_level, downgrade_to, COALESCE(message,''), created_at
		FROM kernel_capabilities
		WHERE kernel = $1 AND protocol = $2 AND transport = $3 AND security = $4 AND feature = $5
		LIMIT 1`
	cap := &KernelCapability{}
	var downgradeJSON []byte
	err := r.pool.QueryRow(ctx, query, kernel, protocol, transport, security, feature).Scan(
		&cap.ID, &cap.Kernel, &cap.MinVersion, &cap.MaxVersion,
		&cap.Protocol, &cap.Transport, &cap.Security, &cap.Feature, &cap.SupportLevel,
		&downgradeJSON, &cap.Message, &cap.CreatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	if len(downgradeJSON) > 0 {
		_ = json.Unmarshal(downgradeJSON, &cap.DowngradeTo)
	}
	return cap, nil
}

// ListByKernel 列出指定内核的所有能力条目（管理面板用）
func (r *CapabilityRepo) ListByKernel(ctx context.Context, kernel string) ([]*KernelCapability, error) {
	query := `
		SELECT id, kernel, COALESCE(min_version,''), COALESCE(max_version,''),
			protocol, transport, security, feature, support_level, downgrade_to, COALESCE(message,''), created_at
		FROM kernel_capabilities
		WHERE kernel = $1 OR $1 = ''
		ORDER BY protocol, transport, security, feature`
	rows, err := r.pool.Query(ctx, query, kernel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var caps []*KernelCapability
	for rows.Next() {
		cap := &KernelCapability{}
		var downgradeJSON []byte
		if err := rows.Scan(
			&cap.ID, &cap.Kernel, &cap.MinVersion, &cap.MaxVersion,
			&cap.Protocol, &cap.Transport, &cap.Security, &cap.Feature, &cap.SupportLevel,
			&downgradeJSON, &cap.Message, &cap.CreatedAt,
		); err != nil {
			continue
		}
		if len(downgradeJSON) > 0 {
			_ = json.Unmarshal(downgradeJSON, &cap.DowngradeTo)
		}
		caps = append(caps, cap)
	}
	return caps, rows.Err()
}

// CreateCertBundle 创建证书包
func (r *CapabilityRepo) CreateCertBundle(ctx context.Context, cb *CertBundle) error {
	query := `
		INSERT INTO cert_bundles (id, provider, mode, cert_pem, key_pem, san, not_after, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	sanJSON, _ := json.Marshal(cb.SAN)
	return r.pool.QueryRow(ctx, query,
		cb.ID, cb.Provider, cb.Mode, cb.CertPEM, cb.KeyPEM, sanJSON, cb.NotAfter, cb.Version,
	).Scan(&cb.CreatedAt, &cb.UpdatedAt)
}

// GetCertBundle 按 ID 获取证书包
func (r *CapabilityRepo) GetCertBundle(ctx context.Context, id uuid.UUID) (*CertBundle, error) {
	query := `
		SELECT id, provider, mode, cert_pem, key_pem, san, not_after, version, created_at, updated_at
		FROM cert_bundles WHERE id = $1`
	cb := &CertBundle{}
	var sanJSON []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&cb.ID, &cb.Provider, &cb.Mode, &cb.CertPEM, &cb.KeyPEM, &sanJSON,
		&cb.NotAfter, &cb.Version, &cb.CreatedAt, &cb.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	if len(sanJSON) > 0 {
		_ = json.Unmarshal(sanJSON, &cb.SAN)
	}
	return cb, nil
}

// ListCertBundles 列出证书包（可按 provider 过滤）
func (r *CapabilityRepo) ListCertBundles(ctx context.Context, provider string) ([]*CertBundle, error) {
	query := `
		SELECT id, provider, mode, cert_pem, key_pem, san, not_after, version, created_at, updated_at
		FROM cert_bundles
		WHERE $1 = '' OR provider = $1
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []*CertBundle
	for rows.Next() {
		cb := &CertBundle{}
		var sanJSON []byte
		if err := rows.Scan(
			&cb.ID, &cb.Provider, &cb.Mode, &cb.CertPEM, &cb.KeyPEM, &sanJSON,
			&cb.NotAfter, &cb.Version, &cb.CreatedAt, &cb.UpdatedAt,
		); err != nil {
			continue
		}
		if len(sanJSON) > 0 {
			_ = json.Unmarshal(sanJSON, &cb.SAN)
		}
		bundles = append(bundles, cb)
	}
	return bundles, rows.Err()
}

// UpdateCertBundlePEM 阶段 C1: 更新证书包的 PEM 内容并递增版本号。
// 用于 ACME 续期成功后自动同步到 cert_bundles 表，使引用该 bundle 的节点
// 在下次部署时获得新证书。
func (r *CapabilityRepo) UpdateCertBundlePEM(ctx context.Context, id uuid.UUID, certPEM, keyPEM string, notAfter *time.Time) error {
	query := `
		UPDATE cert_bundles
		SET cert_pem = $2, key_pem = $3, not_after = $4, version = version + 1, updated_at = NOW()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, certPEM, keyPEM, notAfter)
	return err
}

// FindCertBundlesByDomain 阶段 C1: 按 SAN 域名查找证书包。
// 用于 ACME 续期后查找匹配的 cert_bundle 进行同步。
func (r *CapabilityRepo) FindCertBundlesByDomain(ctx context.Context, domain string) ([]*CertBundle, error) {
	query := `
		SELECT id, provider, mode, cert_pem, key_pem, san, not_after, version, created_at, updated_at
		FROM cert_bundles
		WHERE $1 = ANY(san::text[])
		ORDER BY version DESC`
	rows, err := r.pool.Query(ctx, query, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bundles []*CertBundle
	for rows.Next() {
		cb := &CertBundle{}
		var sanJSON []byte
		if err := rows.Scan(
			&cb.ID, &cb.Provider, &cb.Mode, &cb.CertPEM, &cb.KeyPEM, &sanJSON,
			&cb.NotAfter, &cb.Version, &cb.CreatedAt, &cb.UpdatedAt,
		); err != nil {
			continue
		}
		if len(sanJSON) > 0 {
			_ = json.Unmarshal(sanJSON, &cb.SAN)
		}
		bundles = append(bundles, cb)
	}
	return bundles, rows.Err()
}

// FindCertBundleIDsByDomain 阶段 C1: 按 SAN 域名查找证书包 ID 列表。
// 满足 cert.CertBundleSyncStore 接口（返回轻量 ID 列表，避免跨包类型依赖）。
func (r *CapabilityRepo) FindCertBundleIDsByDomain(ctx context.Context, domain string) ([]uuid.UUID, error) {
	query := `SELECT id FROM cert_bundles WHERE $1 = ANY(san::text[])`
	rows, err := r.pool.Query(ctx, query, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteCertBundle 阶段 C2: 删除证书包
func (r *CapabilityRepo) DeleteCertBundle(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM cert_bundles WHERE id = $1`, id)
	return err
}

// CapabilityLostEventRecord P2-2: 能力降级事件持久化记录
// 与 service.CapabilityLostEvent 对应（repo 层避免依赖 service 层）
type CapabilityLostEventRecord struct {
	RuntimeID       uuid.UUID
	NodeID          uuid.UUID
	NodeCode        string
	Kernel          string
	Protocol        string
	Transport       string
	Security        string
	Feature         string
	OriginalSupport string
	DegradeStrategy string
	DowngradeTo     map[string]interface{}
	Message         string
	ConfigVersionNo string
}

// InsertCapabilityLostEvent P2-2: 写入能力降级审计事件
func (r *CapabilityRepo) InsertCapabilityLostEvent(ctx context.Context, rec *CapabilityLostEventRecord) error {
	var downgradeJSON []byte
	if len(rec.DowngradeTo) > 0 {
		downgradeJSON, _ = json.Marshal(rec.DowngradeTo)
	}
	query := `
		INSERT INTO capability_lost_events
			(runtime_id, node_id, node_code, kernel, protocol, transport, security, feature,
			 original_support, degrade_strategy, downgrade_to, message, config_version_no)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	_, err := r.pool.Exec(ctx, query,
		nilIfZero(rec.RuntimeID), nilIfZero(rec.NodeID), rec.NodeCode,
		rec.Kernel, rec.Protocol, rec.Transport, rec.Security, rec.Feature,
		rec.OriginalSupport, rec.DegradeStrategy, downgradeJSON, rec.Message, rec.ConfigVersionNo,
	)
	return err
}

// nilIfZero UUID 为零值时返回 nil（避免 PG 存储 00000000-0000-... ）
func nilIfZero(id uuid.UUID) interface{} {
	if id == uuid.Nil {
		return nil
	}
	return id
}
