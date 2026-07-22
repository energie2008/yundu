package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DoctorReportRepo 处理 node_doctor_reports 数据访问
type DoctorReportRepo struct {
	pool *pgxpool.Pool
}

func NewDoctorReportRepo(pool *pgxpool.Pool) *DoctorReportRepo {
	return &DoctorReportRepo{pool: pool}
}

const reportColumns = `id, node_id, report_type, trigger_source, overall_status, checks, summary_ok, summary_warn, summary_fail, duration_ms, created_at`

func scanReport(row pgx.Row, r *DoctorReport) error {
	return row.Scan(
		&r.ID, &r.NodeID, &r.ReportType, &r.TriggerSource, &r.OverallStatus, &r.Checks,
		&r.SummaryOK, &r.SummaryWarn, &r.SummaryFail, &r.DurationMs, &r.CreatedAt,
	)
}

func (r *DoctorReportRepo) Create(ctx context.Context, rep *DoctorReport) error {
	query := `
		INSERT INTO node_doctor_reports (id, node_id, report_type, trigger_source, overall_status, checks,
			summary_ok, summary_warn, summary_fail, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		rep.ID, rep.NodeID, rep.ReportType, rep.TriggerSource, rep.OverallStatus, rep.Checks,
		rep.SummaryOK, rep.SummaryWarn, rep.SummaryFail, rep.DurationMs,
	).Scan(&rep.CreatedAt)
}

func (r *DoctorReportRepo) GetByID(ctx context.Context, id uuid.UUID) (*DoctorReport, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_doctor_reports WHERE id = $1`, reportColumns)
	rep := &DoctorReport{}
	if err := scanReport(r.pool.QueryRow(ctx, query, id), rep); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rep, nil
}

func (r *DoctorReportRepo) GetLatestByNodeID(ctx context.Context, nodeID uuid.UUID) (*DoctorReport, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_doctor_reports WHERE node_id = $1 ORDER BY created_at DESC LIMIT 1`, reportColumns)
	rep := &DoctorReport{}
	if err := scanReport(r.pool.QueryRow(ctx, query, nodeID), rep); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rep, nil
}

func (r *DoctorReportRepo) ListByNodeID(ctx context.Context, nodeID uuid.UUID, page, pageSize int) ([]*DoctorReport, int, error) {
	countQuery := `SELECT COUNT(*) FROM node_doctor_reports WHERE node_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, nodeID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s FROM node_doctor_reports WHERE node_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, reportColumns)
	rows, err := r.pool.Query(ctx, query, nodeID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*DoctorReport
	for rows.Next() {
		rep := &DoctorReport{}
		if err := scanReport(rows, rep); err != nil {
			return nil, 0, err
		}
		reports = append(reports, rep)
	}
	return reports, total, rows.Err()
}

// DoctorCheckDefRepo 处理 node_doctor_check_defs 数据访问
type DoctorCheckDefRepo struct {
	pool *pgxpool.Pool
}

func NewDoctorCheckDefRepo(pool *pgxpool.Pool) *DoctorCheckDefRepo {
	return &DoctorCheckDefRepo{pool: pool}
}

const defColumns = `id, code, name, description, check_category, severity, applicable_exposure_modes, applicable_protocol_types, auto_fix_available, auto_fix_action, sort_order, is_enabled, created_at`

func scanDef(row pgx.Row, d *DoctorCheckDef) error {
	return row.Scan(
		&d.ID, &d.Code, &d.Name, &d.Description, &d.CheckCategory, &d.Severity,
		&d.ApplicableExposureModes, &d.ApplicableProtocolTypes, &d.AutoFixAvailable,
		&d.AutoFixAction, &d.SortOrder, &d.IsEnabled, &d.CreatedAt,
	)
}

func (r *DoctorCheckDefRepo) ListEnabled(ctx context.Context) ([]*DoctorCheckDef, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_doctor_check_defs WHERE is_enabled = true ORDER BY sort_order ASC`, defColumns)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []*DoctorCheckDef
	for rows.Next() {
		d := &DoctorCheckDef{}
		if err := scanDef(rows, d); err != nil {
			return nil, err
		}
		defs = append(defs, d)
	}
	return defs, rows.Err()
}

func (r *DoctorCheckDefRepo) GetByCode(ctx context.Context, code string) (*DoctorCheckDef, error) {
	query := fmt.Sprintf(`SELECT %s FROM node_doctor_check_defs WHERE code = $1`, defColumns)
	d := &DoctorCheckDef{}
	if err := scanDef(r.pool.QueryRow(ctx, query, code), d); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return d, nil
}

func (r *DoctorCheckDefRepo) GetByCodes(ctx context.Context, codes []string) ([]*DoctorCheckDef, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s FROM node_doctor_check_defs WHERE code = ANY($1) ORDER BY sort_order ASC`, defColumns)
	rows, err := r.pool.Query(ctx, query, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []*DoctorCheckDef
	for rows.Next() {
		d := &DoctorCheckDef{}
		if err := scanDef(rows, d); err != nil {
			return nil, err
		}
		defs = append(defs, d)
	}
	return defs, rows.Err()
}

// strInSlice 工具函数
func strInSlice(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// joinStrs 工具函数（避免多次引用 strings 包）
func joinStrs(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
