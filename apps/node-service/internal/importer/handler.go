package importer

import (
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminImportHandler struct {
	svc *ImporterService
}

func NewAdminImportHandler(svc *ImporterService) *AdminImportHandler {
	return &AdminImportHandler{svc: svc}
}

func (h *AdminImportHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	imports := admin.Group("/config-import")
	{
		imports.POST("", rbac.RequirePermission("nodes.write"), h.CreateImport)
		imports.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetImport)
		imports.POST("/:id/apply", rbac.RequirePermission("nodes.write"), h.ApplyImport)
	}
	nodes := admin.Group("/nodes")
	{
		nodes.POST("/import-uri", rbac.RequirePermission("nodes.write"), h.PreviewImportURI)
		nodes.POST("/import-uri/confirm", rbac.RequirePermission("nodes.write"), h.ConfirmImportURI)
	}
	admin.GET("/config-imports", rbac.RequirePermission("nodes.read"), h.ListImports)
}

func (h *AdminImportHandler) CreateImport(c *gin.Context) {
	var req CreateImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	// 本批不解析 multipart；如需支持 multipart 可在此分支处理 c.FormFile

	resp, err := h.svc.Parse(c.Request.Context(), req.SourceType, req.Content, nil)
	if err != nil {
		code, msg := MapImporterErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.Created(c, resp)
}

func (h *AdminImportHandler) GetImport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid import job id")
		return
	}

	job, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		code, msg := MapImporterErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, NewImportJobResponse(job))
}

func (h *AdminImportHandler) ApplyImport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid import job id")
		return
	}

	resp, err := h.svc.Apply(c.Request.Context(), id)
	if err != nil {
		code, msg := MapImporterErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}
	server.OK(c, resp)
}

func (h *AdminImportHandler) ListImports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	jobs, total, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	items := make([]ImportJobResponse, len(jobs))
	for i, j := range jobs {
		items[i] = NewImportJobResponse(j)
	}
	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}
