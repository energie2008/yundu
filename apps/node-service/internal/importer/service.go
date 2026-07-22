package importer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// ImportJobStore 抽象 ImportJobRepo
type ImportJobStore interface {
	Create(ctx context.Context, j *ImportJob) error
	GetByID(ctx context.Context, id uuid.UUID) (*ImportJob, error)
	UpdateParseResult(ctx context.Context, id uuid.UUID, parseResult map[string]interface{}, status string, parseError *string, preview *NodeSpec) error
	MarkApplied(ctx context.Context, id uuid.UUID, nodeID uuid.UUID) error
	List(ctx context.Context, page, pageSize int) ([]*ImportJob, int, error)
}

// NodeCreator 把 preview_node_spec 落地为 nodes 记录。
// app.go 注入实现（封装对 node repo + service 的调用），避免 importer 直接依赖 node 模块。
type NodeCreator interface {
	CreateFromSpec(ctx context.Context, spec *NodeSpec, jobID uuid.UUID) (uuid.UUID, error)
}

type URINodeCreateRequest struct {
	Name          string                 `json:"name"`
	ProtocolType  string                 `json:"protocol_type"`
	TransportType string                 `json:"transport_type"`
	SecurityType  string                 `json:"security_type"`
	Host          string                 `json:"host"`
	Port          int                    `json:"port"`
	ConfigJSON    map[string]interface{} `json:"config_json"`
	ServerID      *uuid.UUID             `json:"server_id"`
	RuntimeID     *uuid.UUID             `json:"runtime_id"`
	Code          string                 `json:"code"`
	Region        string                 `json:"region"`
	GroupID       *uuid.UUID             `json:"group_id"`
	Multiplier    float64                `json:"multiplier"`
}

type URIBulkCreator interface {
	CreateFromURIPreview(ctx context.Context, req *URINodeCreateRequest) (uuid.UUID, error)
}

// AuditWriter 预留的审计日志接口
type AuditWriter interface {
	Audit(ctx context.Context, action, resource string, before, after interface{})
}

type ImporterService struct {
	store          ImportJobStore
	nodeCreator    NodeCreator
	uriBulkCreator URIBulkCreator
	audit          AuditWriter
	logger         *slog.Logger
}

func NewImporterService(store ImportJobStore, nodeCreator NodeCreator, audit AuditWriter, logger *slog.Logger) *ImporterService {
	return &ImporterService{
		store:       store,
		nodeCreator: nodeCreator,
		audit:       audit,
		logger:      logger,
	}
}

func (s *ImporterService) SetURIBulkCreator(creator URIBulkCreator) {
	s.uriBulkCreator = creator
}

// Parse 创建 import_job、解析内容、持久化 parse_result 与 preview_node_spec
func (s *ImporterService) Parse(ctx context.Context, sourceType, content string, createdBy *uuid.UUID) (*ParseResponse, error) {
	parser, err := ParserForSourceType(sourceType)
	if err != nil {
		return nil, err
	}

	job := &ImportJob{
		ID:               uuid.New(),
		SourceType:       sourceType,
		RawContent:       content,
		ParseResult:      map[string]interface{}{},
		ParseStatus:      "pending",
		ApplyStatus:      "pending",
		CreatedByAdminID: createdBy,
	}
	if err := s.store.Create(ctx, job); err != nil {
		return nil, err
	}

	spec, perr := parser.Parse(content)
	if perr != nil {
		errMsg := perr.Error()
		job.ParseStatus = "failed"
		job.ParseError = &errMsg
		_ = s.store.UpdateParseResult(ctx, job.ID, map[string]interface{}{"error": errMsg}, "failed", &errMsg, nil)
		return nil, ErrParseFailed
	}

	rawExtract := map[string]interface{}{
		"source_type":  sourceType,
		"node_spec":    spec,
	}
	parseResult := map[string]interface{}{
		"source_type": sourceType,
		"node_spec":   spec,
	}
	job.ParseResult = parseResult
	job.ParseStatus = "success"
	job.PreviewNodeSpec = spec
	if err := s.store.UpdateParseResult(ctx, job.ID, parseResult, "success", nil, spec); err != nil {
		return nil, err
	}
	_ = rawExtract

	if s.audit != nil {
		s.audit.Audit(ctx, "parse", "config_import_job", nil, job)
	}

	return &ParseResponse{
		JobID:           job.ID,
		ParseResult:     ParseResult{SourceType: sourceType, NodeSpec: spec, RawExtract: rawExtract},
		PreviewNodeSpec: spec,
	}, nil
}

// Get 获取 import job 详情
func (s *ImporterService) Get(ctx context.Context, id uuid.UUID) (*ImportJob, error) {
	j, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, ErrImportJobNotFound
	}
	return j, nil
}

// List 列出 import jobs
func (s *ImporterService) List(ctx context.Context, page, pageSize int) ([]*ImportJob, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.store.List(ctx, page, pageSize)
}

// Apply 把 preview_node_spec 写入 nodes/tls_profiles/edge_exposures
// TODO: 本批仅写 nodes 一条最小记录，证书/暴露方式后续补全
func (s *ImporterService) Apply(ctx context.Context, id uuid.UUID) (*ApplyResponse, error) {
	job, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrImportJobNotFound
	}
	if job.ApplyStatus == "applied" {
		return nil, ErrAlreadyApplied
	}
	if job.PreviewNodeSpec == nil {
		return nil, ErrNoPreviewSpec
	}
	if s.nodeCreator == nil {
		return nil, ErrNoPreviewSpec
	}

	nodeID, err := s.nodeCreator.CreateFromSpec(ctx, job.PreviewNodeSpec, id)
	if err != nil {
		return nil, err
	}
	if err := s.store.MarkApplied(ctx, id, nodeID); err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "apply", "config_import_job", job, map[string]interface{}{"applied_node_id": nodeID})
	}
	return &ApplyResponse{
		JobID:          id,
		ApplyStatus:    "applied",
		AppliedNodeID:  &nodeID,
	}, nil
}

type ImportURIPreviewRequest struct {
	Content    string     `json:"content"`
	ServerID   *uuid.UUID `json:"server_id"`
	RuntimeID  *uuid.UUID `json:"runtime_id"`
	Region     string     `json:"region"`
	GroupID    *uuid.UUID `json:"group_id"`
	Multiplier float64    `json:"multiplier"`
}

type ImportURIPreviewResponse struct {
	Nodes   []*URINodePreview `json:"nodes"`
	Errors  []string          `json:"errors"`
	Total   int               `json:"total"`
	Success int               `json:"success"`
	Failed  int               `json:"failed"`
}

type ImportURIConfirmRequest struct {
	Nodes []*URINodeCreateRequest `json:"nodes" binding:"required"`
}

type ImportURIConfirmResult struct {
	NodeID    uuid.UUID `json:"node_id"`
	Name      string    `json:"name"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

type ImportURIConfirmResponse struct {
	Results []*ImportURIConfirmResult `json:"results"`
	Total   int                       `json:"total"`
	Success int                       `json:"success"`
	Failed  int                       `json:"failed"`
}

func (s *ImporterService) ImportURIs(ctx context.Context, req *ImportURIPreviewRequest) (*ImportURIPreviewResponse, error) {
	nodes, errs := ParseURIs(req.Content)

	successCount := 0
	for _, n := range nodes {
		if n.Valid {
			successCount++
		}
	}

	return &ImportURIPreviewResponse{
		Nodes:   nodes,
		Errors:  errs,
		Total:   len(nodes) + len(errs),
		Success: successCount,
		Failed:  len(nodes) - successCount + len(errs),
	}, nil
}

func (s *ImporterService) ConfirmImportURIs(ctx context.Context, req *ImportURIConfirmRequest) (*ImportURIConfirmResponse, error) {
	if s.uriBulkCreator == nil {
		return nil, fmt.Errorf("uri bulk creator not configured")
	}

	results := make([]*ImportURIConfirmResult, 0, len(req.Nodes))
	successCount := 0
	failedCount := 0

	for _, nodeReq := range req.Nodes {
		result := &ImportURIConfirmResult{
			Name:    nodeReq.Name,
			Success: false,
		}

		if nodeReq.Name == "" {
			nodeReq.Name = fmt.Sprintf("%s:%d", nodeReq.Host, nodeReq.Port)
		}

		nodeID, err := s.uriBulkCreator.CreateFromURIPreview(ctx, nodeReq)
		if err != nil {
			result.Error = err.Error()
			failedCount++
		} else {
			result.NodeID = nodeID
			result.Success = true
			successCount++
		}
		results = append(results, result)
	}

	return &ImportURIConfirmResponse{
		Results: results,
		Total:   len(req.Nodes),
		Success: successCount,
		Failed:  failedCount,
	}, nil
}
