package upgrade

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpgradeTaskRepo 处理 runtime_upgrade_tasks 数据访问
type UpgradeTaskRepo struct {
	pool *pgxpool.Pool
}

func NewUpgradeTaskRepo(pool *pgxpool.Pool) *UpgradeTaskRepo {
	return &UpgradeTaskRepo{pool: pool}
}

const taskColumns = `id, server_id, runtime_id, from_version, to_version, status, scope, batch_id,
	canary_percent, download_url, expected_sha256, started_at, completed_at, error_message, created_at, updated_at`

func scanTask(row pgx.Row, t *RuntimeUpgradeTask) error {
	return row.Scan(
		&t.ID, &t.ServerID, &t.RuntimeID, &t.FromVersion, &t.ToVersion, &t.Status, &t.Scope, &t.BatchID,
		&t.CanaryPercent, &t.DownloadURL, &t.ExpectedSha256, &t.StartedAt, &t.CompletedAt,
		&t.ErrorMessage, &t.CreatedAt, &t.UpdatedAt,
	)
}

// Create 插入一条升级任务，返回 created_at / updated_at
func (r *UpgradeTaskRepo) Create(ctx context.Context, t *RuntimeUpgradeTask) error {
	query := `
		INSERT INTO runtime_upgrade_tasks (id, server_id, runtime_id, from_version, to_version, status, scope,
			batch_id, canary_percent, download_url, expected_sha256, started_at, completed_at, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		t.ID, t.ServerID, t.RuntimeID, t.FromVersion, t.ToVersion, t.Status, t.Scope,
		t.BatchID, t.CanaryPercent, t.DownloadURL, t.ExpectedSha256, t.StartedAt, t.CompletedAt, t.ErrorMessage,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
}

// GetByID 按 ID 查询任务；不存在时返回 (nil, nil)
func (r *UpgradeTaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*RuntimeUpgradeTask, error) {
	query := fmt.Sprintf(`SELECT %s FROM runtime_upgrade_tasks WHERE id = $1`, taskColumns)
	t := &RuntimeUpgradeTask{}
	if err := scanTask(r.pool.QueryRow(ctx, query, id), t); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// ListByServer 返回某服务器的升级历史（最新在前）
func (r *UpgradeTaskRepo) ListByServer(ctx context.Context, serverID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	query := fmt.Sprintf(`SELECT %s FROM runtime_upgrade_tasks WHERE server_id = $1 ORDER BY created_at DESC`, taskColumns)
	rows, err := r.pool.Query(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*RuntimeUpgradeTask
	for rows.Next() {
		t := &RuntimeUpgradeTask{}
		if err := scanTask(rows, t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListByBatch 返回同一 batch_id 下的全部任务
func (r *UpgradeTaskRepo) ListByBatch(ctx context.Context, batchID uuid.UUID) ([]*RuntimeUpgradeTask, error) {
	query := fmt.Sprintf(`SELECT %s FROM runtime_upgrade_tasks WHERE batch_id = $1 ORDER BY created_at ASC`, taskColumns)
	rows, err := r.pool.Query(ctx, query, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*RuntimeUpgradeTask
	for rows.Next() {
		t := &RuntimeUpgradeTask{}
		if err := scanTask(rows, t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateStatus 更新任务状态及错误信息：
//   - 状态为 running 且 started_at 为空时设置 started_at = now()
//   - 状态为终态（succeeded/failed/rolled_back）时设置 completed_at = now()
func (r *UpgradeTaskRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status UpgradeStatus, errMsg *string) error {
	query := `
		UPDATE runtime_upgrade_tasks SET
			status = $2,
			error_message = $3,
			started_at = CASE WHEN $2 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('succeeded','failed','rolled_back') THEN now() ELSE completed_at END,
			updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, status, errMsg)
	return err
}

// HasRunningTask 判断某服务器是否存在尚未结束的升级任务（pending / running）
func (r *UpgradeTaskRepo) HasRunningTask(ctx context.Context, serverID uuid.UUID) (bool, error) {
	query := `SELECT COUNT(*) FROM runtime_upgrade_tasks WHERE server_id = $1 AND status IN ('pending','running')`
	var count int
	if err := r.pool.QueryRow(ctx, query, serverID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
