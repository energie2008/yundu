package doctor

import (
	"time"

	"github.com/google/uuid"
)

// 领域模型 (对应迁移 000012_node_doctor.sql)

// DoctorReport 对应 node_doctor_reports 表
type DoctorReport struct {
	ID            uuid.UUID    `json:"id"`
	NodeID        uuid.UUID    `json:"node_id"`
	ReportType    string       `json:"report_type"`
	TriggerSource string       `json:"trigger_source"`
	OverallStatus string       `json:"overall_status"`
	Checks        []CheckResult `json:"checks"`
	SummaryOK     int          `json:"summary_ok"`
	SummaryWarn   int          `json:"summary_warn"`
	SummaryFail   int          `json:"summary_fail"`
	DurationMs    *int         `json:"duration_ms,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

// DoctorCheckDef 对应 node_doctor_check_defs 表
type DoctorCheckDef struct {
	ID                       uuid.UUID `json:"id"`
	Code                     string    `json:"code"`
	Name                     string    `json:"name"`
	Description              *string   `json:"description,omitempty"`
	CheckCategory            string    `json:"check_category"`
	Severity                 string    `json:"severity"`
	ApplicableExposureModes  []string  `json:"applicable_exposure_modes"`
	ApplicableProtocolTypes  []string  `json:"applicable_protocol_types"`
	AutoFixAvailable         bool      `json:"auto_fix_available"`
	AutoFixAction            *string   `json:"auto_fix_action,omitempty"`
	SortOrder                int       `json:"sort_order"`
	IsEnabled                bool      `json:"is_enabled"`
	CreatedAt                time.Time `json:"created_at"`
}

// CheckResult 是 node_doctor_reports.checks JSONB 数组里的单个检查结果
type CheckResult struct {
	CheckCode    string                 `json:"check_code"`
	CheckName    string                 `json:"check_name"`
	Category     string                 `json:"category"`
	Severity     string                 `json:"severity"`
	Status       string                 `json:"status"`        // pass / warn / fail / skip
	Message      string                 `json:"message"`
	FixStatus    string                 `json:"fix_status,omitempty"` // manual_required / fixed / not_applicable
	Details      map[string]interface{} `json:"details,omitempty"`
}

// DTO

type DoctorReportListQuery struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

type DoctorReportResponse struct {
	ID            uuid.UUID    `json:"id"`
	NodeID        uuid.UUID    `json:"node_id"`
	ReportType    string       `json:"report_type"`
	TriggerSource string       `json:"trigger_source"`
	OverallStatus string       `json:"overall_status"`
	Checks        []CheckResult `json:"checks"`
	SummaryOK     int          `json:"summary_ok"`
	SummaryWarn   int          `json:"summary_warn"`
	SummaryFail   int          `json:"summary_fail"`
	DurationMs    *int         `json:"duration_ms,omitempty"`
	CreatedAt     string       `json:"created_at"`
}

func NewDoctorReportResponse(r *DoctorReport) DoctorReportResponse {
	return DoctorReportResponse{
		ID:            r.ID,
		NodeID:        r.NodeID,
		ReportType:    r.ReportType,
		TriggerSource: r.TriggerSource,
		OverallStatus: r.OverallStatus,
		Checks:        r.Checks,
		SummaryOK:     r.SummaryOK,
		SummaryWarn:   r.SummaryWarn,
		SummaryFail:   r.SummaryFail,
		DurationMs:    r.DurationMs,
		CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// RunCheckRequest 用于 POST /admin/nodes/:id/doctor/check
type RunCheckRequest struct {
	CheckCodes []string `json:"check_codes"`
}

// AutoFixRequest 用于 POST /admin/nodes/:id/doctor/autofix
type AutoFixRequest struct {
	CheckCodes []string `json:"check_codes"`
}

// AutoFixResponse
type AutoFixItem struct {
	CheckCode string `json:"check_code"`
	FixStatus string `json:"fix_status"`
	Message   string `json:"message"`
}

type AutoFixResponse struct {
	NodeID uuid.UUID     `json:"node_id"`
	Items  []AutoFixItem `json:"items"`
}

// NodeExposureInfo 用于按 exposure_mode 过滤 check defs
type NodeExposureInfo struct {
	NodeID        uuid.UUID
	ExposureMode  string
	ProtocolType  string
}
