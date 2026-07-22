package cert

import (
	"errors"
	"strconv"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminCertHandler struct {
	svc *CertificateService
}

func NewAdminCertHandler(svc *CertificateService) *AdminCertHandler {
	return &AdminCertHandler{svc: svc}
}

func (h *AdminCertHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	certs := admin.Group("/tls-certificates")
	{
		certs.GET("", rbac.RequirePermission("nodes.read"), h.ListCertificates)
		certs.POST("", rbac.RequirePermission("nodes.write"), h.CreateCertificate)
		certs.GET("/dns-providers", rbac.RequirePermission("nodes.read"), h.ListDNSProviders)
		certs.GET("/:id", rbac.RequirePermission("nodes.read"), h.GetCertificate)
		certs.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdateCertificate)
		certs.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeleteCertificate)
		certs.GET("/:id/deploy-status", rbac.RequirePermission("nodes.read"), h.GetDeployStatus)
		certs.POST("/:id/renew", rbac.RequirePermission("nodes.write"), h.TriggerRenew)
		certs.POST("/:id/obtain", rbac.RequirePermission("nodes.write"), h.ObtainCertificate)
		certs.POST("/:id/sync-san", rbac.RequirePermission("nodes.write"), h.SyncSANFromNodes)
	}

	profiles := admin.Group("/tls-profiles")
	{
		profiles.GET("", rbac.RequirePermission("nodes.read"), h.ListProfiles)
		profiles.POST("", rbac.RequirePermission("nodes.write"), h.CreateProfile)
		profiles.PATCH("/:id", rbac.RequirePermission("nodes.write"), h.UpdateProfile)
		profiles.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.DeleteProfile)
		profiles.POST("/:id/generate-ech", rbac.RequirePermission("nodes.write"), h.GenerateECH)
	}
}

func (h *AdminCertHandler) ListCertificates(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	expiresWithin, _ := strconv.Atoi(c.Query("expires_within_days"))
	query := CertificateListQuery{
		Page:              page,
		PageSize:          pageSize,
		Status:            c.Query("status"),
		ExpiresWithinDays: expiresWithin,
	}

	certs, total, err := h.svc.ListCertificates(c.Request.Context(), page, pageSize, query)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]CertificateResponse, len(certs))
	for i, cert := range certs {
		items[i] = NewCertificateResponse(cert)
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

func (h *AdminCertHandler) CreateCertificate(c *gin.Context) {
	var req CreateCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	cert, err := h.svc.CreateCertificate(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		if code == 50000 {
			// 内部错误时记录详情到 gin.Errors，由日志中间件输出
			_ = c.Error(err)
		}
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, NewCertificateResponse(cert))
}

func (h *AdminCertHandler) GetCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.GetCertificate(c.Request.Context(), id)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, NewCertificateResponse(cert))
}

func (h *AdminCertHandler) UpdateCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	var req UpdateCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	cert, err := h.svc.UpdateCertificate(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, NewCertificateResponse(cert))
}

func (h *AdminCertHandler) DeleteCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	if err := h.svc.DeleteCertificate(c.Request.Context(), id); err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.NoContent(c)
}

func (h *AdminCertHandler) GetDeployStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	resp, err := h.svc.GetDeployStatus(c.Request.Context(), id)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, resp)
}

func (h *AdminCertHandler) TriggerRenew(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.TriggerRenew(c.Request.Context(), id)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, NewCertificateResponse(cert))
}

func (h *AdminCertHandler) ListProfiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	profiles, total, err := h.svc.ListProfiles(c.Request.Context(), page, pageSize)
	if err != nil {
		server.InternalError(c, "")
		return
	}

	items := make([]TLSProfileResponse, len(profiles))
	for i, p := range profiles {
		items[i] = NewTLSProfileResponse(p)
	}

	server.OK(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     items,
	})
}

func (h *AdminCertHandler) CreateProfile(c *gin.Context) {
	var req CreateTLSProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	profile, err := h.svc.CreateProfile(c.Request.Context(), &req)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.Created(c, NewTLSProfileResponse(profile))
}

func (h *AdminCertHandler) UpdateProfile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid profile id")
		return
	}

	var req UpdateTLSProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}

	profile, err := h.svc.UpdateProfile(c.Request.Context(), id, &req)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, NewTLSProfileResponse(profile))
}

func (h *AdminCertHandler) DeleteProfile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid profile id")
		return
	}

	if err := h.svc.DeleteProfile(c.Request.Context(), id); err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.NoContent(c)
}

// GenerateECH 调用 xray tls ech --generate 为 TLS Profile 生成 ECH 密钥对
//
// POST /admin/tls-profiles/:id/generate-ech
// 成功返回更新后的 profile（含 ech_config_encrypted，但 ech_key_encrypted 不返回前端）
func (h *AdminCertHandler) GenerateECH(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid profile id")
		return
	}

	profile, err := h.svc.GenerateECH(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrECHBinaryNotFound) {
			server.BadRequest(c, err.Error())
			return
		}
		// ECH 生成失败属于上游错误，用 500 + hint
		server.InternalError(c, "generate ECH failed: "+err.Error())
		return
	}

	server.OK(c, NewTLSProfileResponse(profile))
}

// ListDNSProviders 返回注册表中所有可用的 DNS provider 元信息
//
// GET /admin/tls-certificates/dns-providers
// 前端用此接口渲染证书创建表单中的 DNS provider 选择器和凭证字段表单
func (h *AdminCertHandler) ListDNSProviders(c *gin.Context) {
	server.OK(c, DNSProviderListResponse{Providers: ListDNSProviders()})
}

// ObtainCertificate 触发首次 ACME 证书签发
//
// POST /admin/tls-certificates/:id/obtain
// 区别于 /renew（续期已有证书），此端点用于首次向 Let's Encrypt 申请证书
func (h *AdminCertHandler) ObtainCertificate(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	cert, err := h.svc.ObtainCertificate(c.Request.Context(), id)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, NewCertificateResponse(cert))
}

// SyncSANFromNodes 自动扫描启用节点的 SNI 并合并到证书 SAN
//
// POST /admin/tls-certificates/:id/sync-san?server_id=<optional>
//
// P9-FIX-2: 修复证书 SAN 缺失问题。原先 SAN 仅由 API 请求传入，
// 节点 SNI 变更后不会自动同步，导致 9501 trojan+tls SNI=cdn.dannelblog.na.am
// 不在证书 SAN 中的问题。
//
// 查询参数 server_id（可选）：仅扫描该 server 下的节点 SNI（per-server 证书场景）。
// 不传则扫描所有启用节点。
func (h *AdminCertHandler) SyncSANFromNodes(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid certificate id")
		return
	}

	var serverID *uuid.UUID
	if sid := c.Query("server_id"); sid != "" {
		uid, err := uuid.Parse(sid)
		if err != nil {
			server.BadRequest(c, "invalid server_id")
			return
		}
		serverID = &uid
	}

	cert, added, err := h.svc.SyncSANFromNodes(c.Request.Context(), id, serverID)
	if err != nil {
		code, msg := MapCertErrorToCode(err)
		if code == 50000 {
			_ = c.Error(err)
		}
		server.Fail(c, code, msg)
		return
	}

	server.OK(c, gin.H{
		"certificate":  NewCertificateResponse(cert),
		"added_count":  added,
		"total_sans":   len(cert.SANs),
	})
}
