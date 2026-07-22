package importer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImportJobRepo 处理 config_import_jobs 数据访问
type ImportJobRepo struct {
	pool *pgxpool.Pool
}

func NewImportJobRepo(pool *pgxpool.Pool) *ImportJobRepo {
	return &ImportJobRepo{pool: pool}
}

const jobColumns = `id, source_type, raw_content, parse_result, parse_status, parse_error, preview_node_spec, apply_status, applied_node_id, created_by_admin_id, created_at, applied_at`

func scanJob(row pgx.Row, j *ImportJob) error {
	return row.Scan(
		&j.ID, &j.SourceType, &j.RawContent, &j.ParseResult, &j.ParseStatus, &j.ParseError,
		&j.PreviewNodeSpec, &j.ApplyStatus, &j.AppliedNodeID, &j.CreatedByAdminID,
		&j.CreatedAt, &j.AppliedAt,
	)
}

func (r *ImportJobRepo) Create(ctx context.Context, j *ImportJob) error {
	query := `
		INSERT INTO config_import_jobs (id, source_type, raw_content, parse_result, parse_status, parse_error,
			preview_node_spec, apply_status, applied_node_id, created_by_admin_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		j.ID, j.SourceType, j.RawContent, j.ParseResult, j.ParseStatus, j.ParseError,
		j.PreviewNodeSpec, j.ApplyStatus, j.AppliedNodeID, j.CreatedByAdminID,
	).Scan(&j.CreatedAt)
}

func (r *ImportJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*ImportJob, error) {
	query := fmt.Sprintf(`SELECT %s FROM config_import_jobs WHERE id = $1`, jobColumns)
	j := &ImportJob{}
	if err := scanJob(r.pool.QueryRow(ctx, query, id), j); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return j, nil
}

func (r *ImportJobRepo) UpdateParseResult(ctx context.Context, id uuid.UUID, parseResult map[string]interface{}, status string, parseError *string, preview *NodeSpec) error {
	query := `UPDATE config_import_jobs SET parse_result = $2, parse_status = $3, parse_error = $4, preview_node_spec = $5 WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, parseResult, status, parseError, preview)
	return err
}

func (r *ImportJobRepo) MarkApplied(ctx context.Context, id uuid.UUID, nodeID uuid.UUID) error {
	query := `UPDATE config_import_jobs SET apply_status = 'applied', applied_node_id = $2, applied_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id, nodeID)
	return err
}

func (r *ImportJobRepo) List(ctx context.Context, page, pageSize int) ([]*ImportJob, int, error) {
	countQuery := `SELECT COUNT(*) FROM config_import_jobs`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s FROM config_import_jobs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, jobColumns)
	rows, err := r.pool.Query(ctx, query, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []*ImportJob
	for rows.Next() {
		j := &ImportJob{}
		if err := scanJob(rows, j); err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}
