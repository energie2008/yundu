package doctor

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminDoctorHandler struct {
	svc *DoctorService
}

func NewAdminDoctorHandler(svc *DoctorService) *AdminDoctorHandler {
	return &AdminDoctorHandler{svc: svc}
}

func (h *AdminDoctorHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	// 注意：挂在 /nodes/:id 下，与现有 node handler 的 /nodes/:id 不冲突（不同子路径）
	nodes := admin.Group("/nodes")
	{
		nodes.GET("/:id/doctor-reports", rbac.RequirePermission("nodes.read"), h.ListReports)
		nodes.GET("/:id/doctor-reports/latest", rbac.RequirePermission("nodes.read"), h.GetLatestReport)
		nodes.POST("/:id/doctor/check", rbac.RequirePermission("nodes.write"), h.RunCheck)
		nodes.POST("/:id/doctor/autofix", rbac.RequirePermission("nodes.write"), h.AutoFix)
	}
}

func parseNodeID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid node id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *AdminDoctorHandler) ListReports(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	reports, total, err := h.svc.ListReports(c.Request.Context(), nodeID, page, pageSize)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]DoctorReportResponse, len(reports))
	for i, r := range reports {
		items[i] = NewDoctorReportResponse(r)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

func (h *AdminDoctorHandler) GetLatestReport(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	rep, err := h.svc.GetLatestReport(c.Request.Context(), nodeID)
	if err != nil {
		code, msg := MapDoctorErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewDoctorReportResponse(rep))
}

func (h *AdminDoctorHandler) RunCheck(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	var req RunCheckRequest
	// body 为空时执行全量检查
	_ = c.ShouldBindJSON(&req)

	if len(req.CheckCodes) == 0 {
		rep, err := h.svc.RunFullCheck(c.Request.Context(), nodeID, "manual")
		if err != nil {
			code, msg := MapDoctorErrorToCode(err)
			server.Fail(c, code, msg)
			return
		}
		server.OK(c, NewDoctorReportResponse(rep))
		return
	}

	// 指定 check_codes：逐个执行单项检查，合并为一个报告
	results := make([]CheckResult, 0, len(req.CheckCodes))
	for _, code := range req.CheckCodes {
		rep, err := h.svc.RunSingleCheck(c.Request.Context(), nodeID, code, "manual")
		if err != nil {
			code2, msg := MapDoctorErrorToCode(err)
			server.Fail(c, code2, msg)
			return
		}
		results = append(results, rep.Checks...)
	}
	// 合并汇总
	var ok2, warn, fail int
	for _, r := range results {
		switch r.Status {
		case "pass":
			ok2++
		case "warn":
			warn++
		case "fail":
			fail++
		}
	}
	overall := "healthy"
	if fail > 0 {
		overall = "unhealthy"
	} else if warn > 0 {
		overall = "degraded"
	}
	merged := &DoctorReport{
		ID:            uuid.New(),
		NodeID:        nodeID,
		ReportType:    "subset",
		TriggerSource: "manual",
		OverallStatus: overall,
		Checks:        results,
		SummaryOK:     ok2,
		SummaryWarn:   warn,
		SummaryFail:   fail,
	}
	server.OK(c, NewDoctorReportResponse(merged))
}

func (h *AdminDoctorHandler) AutoFix(c *gin.Context) {
	nodeID, ok := parseNodeID(c)
	if !ok {
		return
	}

	var req AutoFixRequest
	_ = c.ShouldBindJSON(&req)

	resp, err := h.svc.AutoFix(c.Request.Context(), nodeID, req.CheckCodes)
	if err != nil {
		code, msg := MapDoctorErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}
