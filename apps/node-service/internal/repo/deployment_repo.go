package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/airport-panel/node-service/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeploymentRepo struct {
	pool *pgxpool.Pool
}

func NewDeploymentRepo(pool *pgxpool.Pool) *DeploymentRepo {
	return &DeploymentRepo{pool: pool}
}

func (r *DeploymentRepo) CreateConfigVersion(ctx context.Context, v *model.ConfigVersion) error {
	query := `
		INSERT INTO config_versions (id, scope_type, scope_id, version_no, status, source, schema_version, content_json, content_hash, created_by_admin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		v.ID, v.ScopeType, v.ScopeID, v.VersionNo, v.Status, v.Source, v.SchemaVersion, v.ContentJSON, v.ContentHash, v.CreatedByAdminID,
	).Scan(&v.CreatedAt)
}

func (r *DeploymentRepo) GetLatestConfigVersion(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID) (*model.ConfigVersion, error) {
	query := `
		SELECT id, scope_type, scope_id, version_no, status, source, schema_version, content_json, content_hash, created_by_admin_id, created_at, published_at
		FROM config_versions WHERE scope_type = $1 AND scope_id = $2
		ORDER BY version_no DESC LIMIT 1`
	v := &model.ConfigVersion{}
	err := r.pool.QueryRow(ctx, query, string(scopeType), scopeID).Scan(
		&v.ID, &v.ScopeType, &v.ScopeID, &v.VersionNo, &v.Status, &v.Source, &v.SchemaVersion, &v.ContentJSON, &v.ContentHash,
		&v.CreatedByAdminID, &v.CreatedAt, &v.PublishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func (r *DeploymentRepo) GetConfigVersionByID(ctx context.Context, id uuid.UUID) (*model.ConfigVersion, error) {
	query := `
		SELECT id, scope_type, scope_id, version_no, status, source, schema_version, content_json, content_hash, created_by_admin_id, created_at, published_at
		FROM config_versions WHERE id = $1`
	v := &model.ConfigVersion{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&v.ID, &v.ScopeType, &v.ScopeID, &v.VersionNo, &v.Status, &v.Source, &v.SchemaVersion, &v.ContentJSON, &v.ContentHash,
		&v.CreatedByAdminID, &v.CreatedAt, &v.PublishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func (r *DeploymentRepo) UpdateConfigVersionStatus(ctx context.Context, id uuid.UUID, status model.ConfigVersionStatus) error {
	query := `UPDATE config_versions SET status = $2, published_at = CASE WHEN $2 = 'deployed' THEN now() ELSE published_at END WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

func (r *DeploymentRepo) CreateBatch(ctx context.Context, b *model.DeploymentBatch) error {
	// D4 修复：将 BatchPlan ([]interface{}) 序列化为 JSON []byte，
	// pgx 会自动将 []byte 映射为 PostgreSQL JSONB 类型，避免 "inconsistent types" 错误。
	batchPlanJSON, err := json.Marshal(b.BatchPlan)
	if err != nil {
		return fmt.Errorf("marshal batch_plan: %w", err)
	}
	query := `
		INSERT INTO deployment_batches (id, scope_type, scope_id, target_version_id, strategy, batch_plan, status, created_by_admin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		b.ID, b.ScopeType, b.ScopeID, b.TargetVersionID, b.Strategy, batchPlanJSON, b.Status, b.CreatedByAdminID,
	).Scan(&b.CreatedAt)
}

func (r *DeploymentRepo) UpdateBatchStatus(ctx context.Context, id uuid.UUID, status model.DeploymentStatus) error {
	var query string
	if status == model.DeploymentStatusRunning {
		query = `UPDATE deployment_batches SET status = $2, started_at = now() WHERE id = $1`
	} else if status == model.DeploymentStatusSuccess || status == model.DeploymentStatusFailed || status == model.DeploymentStatusRolledBack {
		query = `UPDATE deployment_batches SET status = $2, finished_at = now() WHERE id = $1`
	} else {
		query = `UPDATE deployment_batches SET status = $2 WHERE id = $1`
	}
	_, err := r.pool.Exec(ctx, query, id, status)
	return err
}

func (r *DeploymentRepo) CreateTargets(ctx context.Context, targets []*model.DeploymentTarget) error {
	batch := &pgx.Batch{}
	query := `
		INSERT INTO deployment_targets (id, deployment_batch_id, target_type, target_id, target_version_id, previous_version_id, phase_no, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	for _, t := range targets {
		batch.Queue(query, t.ID, t.DeploymentBatchID, t.TargetType, t.TargetID, t.TargetVersionID, t.PreviousVersionID, t.PhaseNo, t.Status)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for _, t := range targets {
		if err := br.QueryRow().Scan(&t.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

// DispatchTargetResult 供事务批量推进 deployment_targets。
type DispatchTargetResult struct {
	TargetID    uuid.UUID
	Status      string
	ApplyResult map[string]interface{}
}

// ApplyDispatchAndTargetsTx 单事务原子更新三个存储：
// nodes.metadata 下发状态、config_versions.applied_at、deployment_targets 结果。
// 消除 ReportConfigResult 三写非原子导致的跨表不一致（节点 applied 但批次 pending）。
func (r *DeploymentRepo) ApplyDispatchAndTargetsTx(
	ctx context.Context,
	nodeIDs []uuid.UUID,
	status string,
	version int64,
	errMsg string,
	cvID uuid.UUID,
	markApplied bool,
	targets []DispatchTargetResult,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if len(nodeIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE nodes SET
				metadata = jsonb_set(
					jsonb_set(
						jsonb_set(
							jsonb_set(
								COALESCE(metadata, '{}'::jsonb),
								'{_dispatch_status}', to_jsonb($2::text)
							),
							'{_dispatch_version}', to_jsonb($3::bigint)
						),
						'{_dispatch_time}', to_jsonb($4::timestamptz)
					),
					'{_dispatch_error}', to_jsonb($5::text)
				),
				updated_at = now()
			WHERE id = ANY($1)`, nodeIDs, status, version, now, errMsg); err != nil {
			return fmt.Errorf("tx: update nodes dispatch status: %w", err)
		}
	}
	if markApplied {
		if _, err := tx.Exec(ctx, `UPDATE config_versions SET applied_at = now() WHERE id = $1`, cvID); err != nil {
			return fmt.Errorf("tx: mark config version applied: %w", err)
		}
	}
	for _, t := range targets {
		if _, err := tx.Exec(ctx, `
			UPDATE deployment_targets SET
				status = $2, apply_result = $3,
				finished_at = CASE WHEN $2 IN ('success', 'failed', 'rolled_back') THEN now() ELSE finished_at END
			WHERE id = $1`, t.TargetID, t.Status, t.ApplyResult); err != nil {
			return fmt.Errorf("tx: update deployment target %s: %w", t.TargetID, err)
		}
	}
	return tx.Commit(ctx)
}

func (r *DeploymentRepo) UpdateTargetResult(ctx context.Context, target *model.DeploymentTarget) error {
	query := `
		UPDATE deployment_targets SET
			status = $2, precheck_result = $3, apply_result = $4,
			started_at = CASE WHEN $2 IN ('precheck', 'applying', 'verifying') AND started_at IS NULL THEN now() ELSE started_at END,
			finished_at = CASE WHEN $2 IN ('success', 'failed', 'rolled_back') THEN now() ELSE finished_at END
		WHERE id = $1
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		target.ID, target.Status, target.PrecheckResult, target.ApplyResult,
	).Scan(&target.CreatedAt)
}

func (r *DeploymentRepo) GetTargetByID(ctx context.Context, id uuid.UUID) (*model.DeploymentTarget, error) {
	query := `
		SELECT id, deployment_batch_id, target_type, target_id, target_version_id, previous_version_id, phase_no,
			status, precheck_result, apply_result, rollback_result, started_at, finished_at, created_at
		FROM deployment_targets WHERE id = $1`
	t := &model.DeploymentTarget{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.DeploymentBatchID, &t.TargetType, &t.TargetID, &t.TargetVersionID, &t.PreviousVersionID, &t.PhaseNo,
		&t.Status, &t.PrecheckResult, &t.ApplyResult, &t.RollbackResult, &t.StartedAt, &t.FinishedAt, &t.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *DeploymentRepo) ListTargetsByBatchID(ctx context.Context, batchID uuid.UUID) ([]*model.DeploymentTarget, error) {
	query := `
		SELECT id, deployment_batch_id, target_type, target_id, target_version_id, previous_version_id, phase_no,
			status, precheck_result, apply_result, rollback_result, started_at, finished_at, created_at
		FROM deployment_targets WHERE deployment_batch_id = $1 ORDER BY phase_no ASC, id ASC`
	rows, err := r.pool.Query(ctx, query, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []*model.DeploymentTarget
	for rows.Next() {
		t := &model.DeploymentTarget{}
		err := rows.Scan(
			&t.ID, &t.DeploymentBatchID, &t.TargetType, &t.TargetID, &t.TargetVersionID, &t.PreviousVersionID, &t.PhaseNo,
			&t.Status, &t.PrecheckResult, &t.ApplyResult, &t.RollbackResult, &t.StartedAt, &t.FinishedAt, &t.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (r *DeploymentRepo) ListBatches(ctx context.Context, page, pageSize int, status model.DeploymentStatus, scopeType model.ScopeType) ([]*model.DeploymentBatch, int, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if status != "" {
		where = append(where, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(status))
		argIdx++
	}
	if scopeType != "" {
		where = append(where, fmt.Sprintf("scope_type = $%d", argIdx))
		args = append(args, string(scopeType))
		argIdx++
	}

	whereClause := "1=1"
	if len(where) > 0 {
		whereClause = strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM deployment_batches WHERE %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT id, scope_type, scope_id, target_version_id, strategy, batch_plan, status, started_at, finished_at, created_by_admin_id, created_at
		FROM deployment_batches WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, pageSize, (page-1)*pageSize)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var batches []*model.DeploymentBatch
	for rows.Next() {
		b := &model.DeploymentBatch{}
		err := rows.Scan(
			&b.ID, &b.ScopeType, &b.ScopeID, &b.TargetVersionID, &b.Strategy, &b.BatchPlan,
			&b.Status, &b.StartedAt, &b.FinishedAt, &b.CreatedByAdminID, &b.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		batches = append(batches, b)
	}
	return batches, total, rows.Err()
}

func (r *DeploymentRepo) GetNextVersionNo(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID) (int64, error) {
	query := `SELECT COALESCE(MAX(version_no), 0) + 1 FROM config_versions WHERE scope_type = $1 AND scope_id = $2`
	var versionNo int64
	err := r.pool.QueryRow(ctx, query, string(scopeType), scopeID).Scan(&versionNo)
	if err != nil {
		return 1, err
	}
	return versionNo, nil
}

func (r *DeploymentRepo) GetLatestActiveConfigVersion(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID) (*model.ConfigVersion, error) {
	query := `
		SELECT id, scope_type, scope_id, version_no, status, source, schema_version, content_json, content_hash, created_by_admin_id, created_at, published_at
		FROM config_versions WHERE scope_type = $1 AND scope_id = $2 AND status IN ('deployed', 'active')
		ORDER BY version_no DESC LIMIT 1`
	v := &model.ConfigVersion{}
	err := r.pool.QueryRow(ctx, query, string(scopeType), scopeID).Scan(
		&v.ID, &v.ScopeType, &v.ScopeID, &v.VersionNo, &v.Status, &v.Source, &v.SchemaVersion, &v.ContentJSON, &v.ContentHash,
		&v.CreatedByAdminID, &v.CreatedAt, &v.PublishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func (r *DeploymentRepo) GetConfigVersionByVersionNo(ctx context.Context, scopeType model.ScopeType, scopeID uuid.UUID, versionNo int64) (*model.ConfigVersion, error) {
	query := `
		SELECT id, scope_type, scope_id, version_no, status, source, schema_version, content_json, content_hash, created_by_admin_id, created_at, published_at
		FROM config_versions WHERE scope_type = $1 AND scope_id = $2 AND version_no = $3`
	v := &model.ConfigVersion{}
	err := r.pool.QueryRow(ctx, query, string(scopeType), scopeID, versionNo).Scan(
		&v.ID, &v.ScopeType, &v.ScopeID, &v.VersionNo, &v.Status, &v.Source, &v.SchemaVersion, &v.ContentJSON, &v.ContentHash,
		&v.CreatedByAdminID, &v.CreatedAt, &v.PublishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func (r *DeploymentRepo) UpdateConfigVersionApplied(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE config_versions SET status = 'deployed', published_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// UpdateTargetStatusByPhase 批量更新某 phase 的 target 状态。
// started_at / finished_at 的维护规则与 UpdateTargetResult 保持一致。
func (r *DeploymentRepo) UpdateTargetStatusByPhase(ctx context.Context, batchID uuid.UUID, phase int, toStatus model.TargetStatus) error {
	query := `
		UPDATE deployment_targets SET
			status = $3,
			started_at = CASE WHEN $3 IN ('precheck', 'applying', 'verifying') AND started_at IS NULL THEN now() ELSE started_at END,
			finished_at = CASE WHEN $3 IN ('success', 'failed', 'rolled_back') THEN now() ELSE finished_at END
		WHERE deployment_batch_id = $1 AND phase_no = $2`
	_, err := r.pool.Exec(ctx, query, batchID, phase, string(toStatus))
	return err
}

// ListTargetsByBatchAndPhase 查询某 phase 的 targets（按 id 升序，保证顺序稳定）
func (r *DeploymentRepo) ListTargetsByBatchAndPhase(ctx context.Context, batchID uuid.UUID, phase int) ([]*model.DeploymentTarget, error) {
	query := `
		SELECT id, deployment_batch_id, target_type, target_id, target_version_id, previous_version_id, phase_no,
			status, precheck_result, apply_result, rollback_result, started_at, finished_at, created_at
		FROM deployment_targets WHERE deployment_batch_id = $1 AND phase_no = $2 ORDER BY id ASC`
	rows, err := r.pool.Query(ctx, query, batchID, phase)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []*model.DeploymentTarget
	for rows.Next() {
		t := &model.DeploymentTarget{}
		if err := rows.Scan(
			&t.ID, &t.DeploymentBatchID, &t.TargetType, &t.TargetID, &t.TargetVersionID, &t.PreviousVersionID, &t.PhaseNo,
			&t.Status, &t.PrecheckResult, &t.ApplyResult, &t.RollbackResult, &t.StartedAt, &t.FinishedAt, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// CountTargetsByPhase 统计某 phase 某 status 的 target 数量
func (r *DeploymentRepo) CountTargetsByPhase(ctx context.Context, batchID uuid.UUID, phase int, status model.TargetStatus) (int, error) {
	query := `SELECT COUNT(*) FROM deployment_targets WHERE deployment_batch_id = $1 AND phase_no = $2 AND status = $3`
	var count int
	err := r.pool.QueryRow(ctx, query, batchID, phase, string(status)).Scan(&count)
	return count, err
}

// ListActiveTargetsByNodeAndVersion 查询指定节点+目标版本处于非终态（非 success/failed/rolled_back/paused）的部署目标。
// 用于 agent 回执时反查并推进对应的 deployment_target，避免批次因回执走的是 runtime 维度而永久卡在 running。
// 排除 paused：paused 属于后续 phase，应由 AdvancePhase 逐 phase 推进，而非被当前回执直接激活。
func (r *DeploymentRepo) ListActiveTargetsByNodeAndVersion(ctx context.Context, nodeID, versionID uuid.UUID) ([]*model.DeploymentTarget, error) {
	query := `
		SELECT id, deployment_batch_id, target_type, target_id, target_version_id, previous_version_id, phase_no,
			status, precheck_result, apply_result, rollback_result, started_at, finished_at, created_at
		FROM deployment_targets
		WHERE target_id = $1 AND target_version_id = $2
		  AND status IN ('pending', 'precheck', 'applying', 'verifying')
		ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, nodeID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []*model.DeploymentTarget
	for rows.Next() {
		t := &model.DeploymentTarget{}
		if err := rows.Scan(
			&t.ID, &t.DeploymentBatchID, &t.TargetType, &t.TargetID, &t.TargetVersionID, &t.PreviousVersionID, &t.PhaseNo,
			&t.Status, &t.PrecheckResult, &t.ApplyResult, &t.RollbackResult, &t.StartedAt, &t.FinishedAt, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// ===== P3-1: 加密 Payload Manifest 持久化 =====

// CreatePayload 将加密的 PayloadManifest 写入 config_payloads 表。
// id / 各字段由调用方（service 层）填充，created_at 由数据库生成并回写。
func (r *DeploymentRepo) CreatePayload(ctx context.Context, p *model.ConfigPayload) error {
	query := `
		INSERT INTO config_payloads (id, config_version_id, version_no, sha256, kernel, rollback_strategy, payload_encrypted, content)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		p.ID, p.ConfigVersionID, p.VersionNo, p.SHA256, p.Kernel, p.RollbackStrategy, p.PayloadEncrypted, p.Content,
	).Scan(&p.CreatedAt)
}

// GetPayloadByVersionNo 按版本号查询加密 Payload。
// scope 为兼容字段，当前按 version_no 全局查询（一个 runtime 内 version_no 唯一）。
func (r *DeploymentRepo) GetPayloadByVersionNo(ctx context.Context, versionNo int64) (*model.ConfigPayload, error) {
	query := `
		SELECT id, config_version_id, version_no, sha256, kernel, rollback_strategy, payload_encrypted, content, created_at
		FROM config_payloads WHERE version_no = $1 ORDER BY created_at DESC LIMIT 1`
	p := &model.ConfigPayload{}
	err := r.pool.QueryRow(ctx, query, versionNo).Scan(
		&p.ID, &p.ConfigVersionID, &p.VersionNo, &p.SHA256, &p.Kernel, &p.RollbackStrategy, &p.PayloadEncrypted, &p.Content, &p.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// GetPayloadByConfigVersion 按 config_version_id 查询加密 Payload。
func (r *DeploymentRepo) GetPayloadByConfigVersion(ctx context.Context, configVersionID uuid.UUID) (*model.ConfigPayload, error) {
	query := `
		SELECT id, config_version_id, version_no, sha256, kernel, rollback_strategy, payload_encrypted, content, created_at
		FROM config_payloads WHERE config_version_id = $1 ORDER BY created_at DESC LIMIT 1`
	p := &model.ConfigPayload{}
	err := r.pool.QueryRow(ctx, query, configVersionID).Scan(
		&p.ID, &p.ConfigVersionID, &p.VersionNo, &p.SHA256, &p.Kernel, &p.RollbackStrategy, &p.PayloadEncrypted, &p.Content, &p.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// ===== P3-1: 部署结果上报 =====

// CreateDeploymentResult 将 Agent 上报的 ACK/NACK 写入 deployment_results 表。
// 若 DeploymentTargetID 为 uuid.Nil，则插入 NULL（Agent 拉取模式无 target 关联）。
func (r *DeploymentRepo) CreateDeploymentResult(ctx context.Context, dr *model.DeploymentResult) error {
	var targetID interface{}
	if dr.DeploymentTargetID != uuid.Nil {
		targetID = dr.DeploymentTargetID
	}
	query := `
		INSERT INTO deployment_results (id, deployment_target_id, server_code, version_no, status, phase, error, apply_duration_ms, reported_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, NOW()))
		RETURNING reported_at, created_at`
	return r.pool.QueryRow(ctx, query,
		dr.ID, targetID, dr.ServerCode, dr.VersionNo, dr.Status, dr.Phase, dr.Error, dr.ApplyDurationMs, dr.ReportedAt,
	).Scan(&dr.ReportedAt, &dr.CreatedAt)
}
