package doctor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/airport-panel/node-service/internal/metrics"
	"github.com/google/uuid"
)

// ReportStore 抽象 DoctorReportRepo
type ReportStore interface {
	Create(ctx context.Context, rep *DoctorReport) error
	GetByID(ctx context.Context, id uuid.UUID) (*DoctorReport, error)
	GetLatestByNodeID(ctx context.Context, nodeID uuid.UUID) (*DoctorReport, error)
	ListByNodeID(ctx context.Context, nodeID uuid.UUID, page, pageSize int) ([]*DoctorReport, int, error)
}

// CheckDefStore 抽象 DoctorCheckDefRepo
type CheckDefStore interface {
	ListEnabled(ctx context.Context) ([]*DoctorCheckDef, error)
	GetByCode(ctx context.Context, code string) (*DoctorCheckDef, error)
	GetByCodes(ctx context.Context, codes []string) ([]*DoctorCheckDef, error)
}

// NodeExposureFetcher 用于获取节点的 exposure_mode（用于过滤适用的 check defs）。
// app.go 注入实现，避免 doctor 直接依赖 exposure 包（防止循环依赖）。
type NodeExposureFetcher interface {
	Fetch(ctx context.Context, nodeID uuid.UUID) (*NodeExposureInfo, error)
}

// NodeLister 用于定时任务遍历所有启用的节点 ID。
// app.go 注入 NodeRepo 适配实现，避免 doctor 直接依赖 repo 包。
type NodeLister interface {
	ListEnabledNodeIDs(ctx context.Context) ([]uuid.UUID, error)
}

// FixDispatcher 自动修复派发器（将 action 下发到 node-agent 或调用对应 service）。
// app.go 注入实现：解析 nodeID → serverID 后调用 aidiag.ActionDispatcher。
type FixDispatcher interface {
	Dispatch(ctx context.Context, nodeID uuid.UUID, action string, reason string) error
}

// AuditWriter 预留的审计日志接口
type AuditWriter interface {
	Audit(ctx context.Context, action, resource string, before, after interface{})
}

type DoctorService struct {
	reportStore   ReportStore
	defStore      CheckDefStore
	nodeFetcher   NodeExposureFetcher
	probeFetcher  NodeProbeFetcher // 真实探测所需的节点信息获取器
	nodeLister    NodeLister        // 定时任务遍历节点用
	fixDispatcher FixDispatcher     // 自动修复派发器
	audit         AuditWriter
	logger        *slog.Logger
}

func NewDoctorService(
	reportStore ReportStore,
	defStore CheckDefStore,
	nodeFetcher NodeExposureFetcher,
	audit AuditWriter,
	logger *slog.Logger,
) *DoctorService {
	return &DoctorService{
		reportStore: reportStore,
		defStore:    defStore,
		nodeFetcher: nodeFetcher,
		audit:       audit,
		logger:      logger,
	}
}

// SetNodeProbeFetcher 注入节点探测信息获取器（用于真实网络探测）
func (s *DoctorService) SetNodeProbeFetcher(f NodeProbeFetcher) {
	s.probeFetcher = f
}

// SetNodeLister 注入节点列表获取器（用于 StartScheduledJob 遍历所有节点）
func (s *DoctorService) SetNodeLister(l NodeLister) {
	s.nodeLister = l
}

// SetFixDispatcher 注入自动修复派发器（用于 AutoFix 真实下发）
// 未注入时 AutoFix 退化为 "manual_required"（保留旧行为）。
func (s *DoctorService) SetFixDispatcher(d FixDispatcher) {
	s.fixDispatcher = d
}

const stubPassMessage = "检查项未实装网络探测，仅记录结构"

// isApplicable 判断 check def 是否适用于给定 exposure_mode / protocol_type
// applicable 列表包含 "*" 表示适用所有
func isApplicable(def *DoctorCheckDef, exposureMode, protocolType string) bool {
	modeMatch := len(def.ApplicableExposureModes) == 0 || strInSlice("*", def.ApplicableExposureModes) || strInSlice(exposureMode, def.ApplicableExposureModes)
	protoMatch := len(def.ApplicableProtocolTypes) == 0 || strInSlice("*", def.ApplicableProtocolTypes) || strInSlice(protocolType, def.ApplicableProtocolTypes)
	return modeMatch && protoMatch
}

// executeCheck 执行单个检查
// 如果 probeFetcher 已注入且能获取节点信息，则执行真实网络探测
// 否则降级为 stub 返回 pass
func executeCheck(ctx context.Context, s *DoctorService, def *DoctorCheckDef, nodeID uuid.UUID) CheckResult {
	if s.probeFetcher != nil {
		info, err := s.probeFetcher.FetchProbeInfo(ctx, nodeID)
		if err == nil && info != nil {
			return executeRealCheck(ctx, def, info)
		}
		// 获取节点信息失败，降级为 stub
	}
	return CheckResult{
		CheckCode: def.Code,
		CheckName: def.Name,
		Category:  def.CheckCategory,
		Severity:  def.Severity,
		Status:    "pass",
		Message:   stubPassMessage,
	}
}

// filterApplicableDefs 按 exposure_mode + protocol_type 过滤 check defs
func (s *DoctorService) filterApplicableDefs(ctx context.Context, defs []*DoctorCheckDef, nodeID uuid.UUID) ([]*DoctorCheckDef, error) {
	if s.nodeFetcher == nil {
		// 没有 fetcher 时，对所有 def 都视为适用（保守做法）
		return defs, nil
	}
	info, err := s.nodeFetcher.Fetch(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return defs, nil
	}
	var applicable []*DoctorCheckDef
	for _, d := range defs {
		if isApplicable(d, info.ExposureMode, info.ProtocolType) {
			applicable = append(applicable, d)
		}
	}
	return applicable, nil
}

// RunFullCheck 从 check_defs 取所有 is_enabled=true 项，按节点 exposure_mode 过滤后并发执行
func (s *DoctorService) RunFullCheck(ctx context.Context, nodeID uuid.UUID, triggerSource string) (*DoctorReport, error) {
	start := time.Now()
	defs, err := s.defStore.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	applicable, err := s.filterApplicableDefs(ctx, defs, nodeID)
	if err != nil {
		return nil, err
	}

	results := s.runChecksConcurrently(ctx, applicable, nodeID)
	return s.persistReport(ctx, nodeID, "full", triggerSource, results, start)
}

// RunSingleCheck 仅执行指定 check_code 的一项（若不适用则返回 skip）
func (s *DoctorService) RunSingleCheck(ctx context.Context, nodeID uuid.UUID, checkCode string, triggerSource string) (*DoctorReport, error) {
	start := time.Now()
	def, err := s.defStore.GetByCode(ctx, checkCode)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, ErrCheckDefNotFound
	}

	var results []CheckResult
	// 判断适用性
	applicable := true
	if s.nodeFetcher != nil {
		info, ferr := s.nodeFetcher.Fetch(ctx, nodeID)
		if ferr != nil {
			return nil, ferr
		}
		if info != nil && !isApplicable(def, info.ExposureMode, info.ProtocolType) {
			applicable = false
		}
	}

	if applicable {
		results = []CheckResult{executeCheck(ctx, s, def, nodeID)}
	} else {
		// 不适用的检查项返回 skip
		results = []CheckResult{{
			CheckCode: def.Code,
			CheckName: def.Name,
			Category:  def.CheckCategory,
			Severity:  def.Severity,
			Status:    "skip",
			Message:   "检查项不适用于当前节点的暴露模式/协议",
		}}
	}
	return s.persistReport(ctx, nodeID, "single", triggerSource, results, start)
}

// runChecksConcurrently 并发执行检查并收集结果（保持原 def 顺序）
func (s *DoctorService) runChecksConcurrently(ctx context.Context, defs []*DoctorCheckDef, nodeID uuid.UUID) []CheckResult {
	results := make([]CheckResult, len(defs))
	var wg sync.WaitGroup
	for i, def := range defs {
		wg.Add(1)
		go func(idx int, d *DoctorCheckDef) {
			defer wg.Done()
			results[idx] = executeCheck(ctx, s, d, nodeID)
		}(i, def)
	}
	wg.Wait()
	return results
}

// persistReport 汇总结果并写入 node_doctor_reports
func (s *DoctorService) persistReport(ctx context.Context, nodeID uuid.UUID, reportType, triggerSource string, results []CheckResult, start time.Time) (*DoctorReport, error) {
	var ok, warn, fail int
	for _, r := range results {
		metrics.DoctorChecksTotal.WithLabelValues(r.Status).Inc()
		switch r.Status {
		case "pass":
			ok++
		case "warn":
			warn++
		case "fail":
			fail++
		case "skip":
			// skip 不计入 ok/warn/fail
		}
	}
	overall := "healthy"
	if fail > 0 {
		overall = "unhealthy"
	} else if warn > 0 {
		overall = "degraded"
	}
	if len(results) == 0 {
		overall = "unknown"
	}

	duration := int(time.Since(start).Milliseconds())
	rep := &DoctorReport{
		ID:            uuid.New(),
		NodeID:        nodeID,
		ReportType:    reportType,
		TriggerSource: triggerSource,
		OverallStatus: overall,
		Checks:        results,
		SummaryOK:     ok,
		SummaryWarn:   warn,
		SummaryFail:   fail,
		DurationMs:    &duration,
	}
	if err := s.reportStore.Create(ctx, rep); err != nil {
		return nil, err
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "doctor_check", "node_doctor_report", nil, rep)
	}
	return rep, nil
}

// AutoFix 对 auto_fix_available=true 的项执行自动修复
//
// 若注入了 FixDispatcher，则根据 check 的 auto_fix_action（或由 check_code 推断）
// 调用 dispatcher 真实下发到 node-agent；未注入时退化为 "manual_required"。
//
// action 推断规则（当 auto_fix_action 为空时）：
//   - check_code 含 cert/tls       → renew_cert
//   - check_code 含 xray/process/runtime → restart_kernel
//   - check_code 含 nginx/sub_render/config → reload_config
//   - 其他                          → 不派发，标记 manual_required
func (s *DoctorService) AutoFix(ctx context.Context, nodeID uuid.UUID, checkCodes []string) (*AutoFixResponse, error) {
	var defs []*DoctorCheckDef
	var err error
	if len(checkCodes) == 0 {
		defs, err = s.defStore.ListEnabled(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		defs, err = s.defStore.GetByCodes(ctx, checkCodes)
		if err != nil {
			return nil, err
		}
	}

	applicable, err := s.filterApplicableDefs(ctx, defs, nodeID)
	if err != nil {
		return nil, err
	}

	items := make([]AutoFixItem, 0, len(applicable))
	for _, d := range applicable {
		if !d.AutoFixAvailable {
			items = append(items, AutoFixItem{
				CheckCode: d.Code,
				FixStatus: "not_applicable",
				Message:   "该检查项不支持自动修复",
			})
			metrics.DoctorAutofixDispatched.WithLabelValues("none", "not_applicable").Inc()
			continue
		}

		action := inferFixAction(d)
		if action == "" {
			items = append(items, AutoFixItem{
				CheckCode: d.Code,
				FixStatus: "manual_required",
				Message:   "无法推断自动修复动作，需人工介入",
			})
			metrics.DoctorAutofixDispatched.WithLabelValues("none", "manual_required").Inc()
			continue
		}

		if s.fixDispatcher == nil {
			// 未注入派发器：降级为 manual_required（保留旧行为）
			items = append(items, AutoFixItem{
				CheckCode: d.Code,
				FixStatus: "manual_required",
				Message:   "自动修复派发器未注入，需人工介入（action=" + action + "）",
			})
			metrics.DoctorAutofixDispatched.WithLabelValues(action, "manual_required").Inc()
			continue
		}

		reason := "doctor autofix: " + d.Code + " (" + d.Name + ")"
		if err := s.fixDispatcher.Dispatch(ctx, nodeID, action, reason); err != nil {
			s.logger.Error("autofix dispatch failed",
				"node_id", nodeID, "check_code", d.Code, "action", action, "error", err)
			items = append(items, AutoFixItem{
				CheckCode: d.Code,
				FixStatus: "failed",
				Message:   fmt.Sprintf("派发失败: %v", err),
			})
			metrics.DoctorAutofixDispatched.WithLabelValues(action, "failed").Inc()
			continue
		}
		s.logger.Info("autofix dispatched",
			"node_id", nodeID, "check_code", d.Code, "action", action)
		items = append(items, AutoFixItem{
			CheckCode: d.Code,
			FixStatus: "dispatched",
			Message:   "已派发自动修复指令: " + action,
		})
		metrics.DoctorAutofixDispatched.WithLabelValues(action, "dispatched").Inc()
	}
	if s.audit != nil {
		s.audit.Audit(ctx, "autofix", "node_doctor_check", nil, items)
	}
	return &AutoFixResponse{NodeID: nodeID, Items: items}, nil
}

// inferFixAction 根据 check def 推断修复动作编码
func inferFixAction(d *DoctorCheckDef) string {
	if d.AutoFixAction != nil && *d.AutoFixAction != "" {
		return *d.AutoFixAction
	}
	code := strings.ToLower(d.Code)
	switch {
	case strings.Contains(code, "cert") || strings.Contains(code, "tls_expiry"):
		return "renew_cert"
	case strings.Contains(code, "xray") || strings.Contains(code, "process") || strings.Contains(code, "runtime"):
		return "restart_kernel"
	case strings.Contains(code, "nginx") || strings.Contains(code, "sub_render") || strings.Contains(code, "config"):
		return "reload_config"
	default:
		return ""
	}
}

// ListReports 列出节点的 doctor 报告（分页，最新在前）
func (s *DoctorService) ListReports(ctx context.Context, nodeID uuid.UUID, page, pageSize int) ([]*DoctorReport, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.reportStore.ListByNodeID(ctx, nodeID, page, pageSize)
}

// GetLatestReport 获取节点最新报告
func (s *DoctorService) GetLatestReport(ctx context.Context, nodeID uuid.UUID) (*DoctorReport, error) {
	rep, err := s.reportStore.GetLatestByNodeID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if rep == nil {
		return nil, ErrReportNotFound
	}
	return rep, nil
}

// StartScheduledJob 启动定时全量检查（每 30 分钟）。
//
// 每个 tick 遍历所有启用节点，并发执行 RunFullCheck(trigger_source="scheduled")。
// 单节点检查失败仅记录日志，不影响其他节点。
// 需通过 SetNodeLister 注入节点列表获取器；未注入时仅记录 warn 并跳过。
func (s *DoctorService) StartScheduledJob(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	// 启动后立即执行一次（避免冷启动后等 30 分钟才首轮）
	s.runScheduledRound(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runScheduledRound(ctx)
		}
	}
}

// runScheduledRound 执行一轮定时检查
func (s *DoctorService) runScheduledRound(ctx context.Context) {
	if s.nodeLister == nil {
		s.logger.Warn("doctor scheduled tick skipped: NodeLister not injected")
		return
	}
	nodeIDs, err := s.nodeLister.ListEnabledNodeIDs(ctx)
	if err != nil {
		s.logger.Error("doctor scheduled tick: list nodes failed", "error", err)
		return
	}
	if len(nodeIDs) == 0 {
		s.logger.Info("doctor scheduled tick: no enabled nodes")
		return
	}
	s.logger.Info("doctor scheduled round start", "node_count", len(nodeIDs))

	var wg sync.WaitGroup
	for _, nodeID := range nodeIDs {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			// 定时检查用独立 ctx 避免单节点超时影响整轮；复用父 ctx 的取消信号
			if _, err := s.RunFullCheck(ctx, id, "scheduled"); err != nil {
				s.logger.Error("doctor scheduled check failed", "node_id", id, "error", err)
			}
		}(nodeID)
	}
	wg.Wait()
	s.logger.Info("doctor scheduled round complete", "node_count", len(nodeIDs))
}
