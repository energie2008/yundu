package handler

import (
	"time"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminCertBundleHandler 阶段 C2: cert_bundles 管理 API
type AdminCertBundleHandler struct {
	capRepo *repo.CapabilityRepo
}

func NewAdminCertBundleHandler(capRepo *repo.CapabilityRepo) *AdminCertBundleHandler {
	return &AdminCertBundleHandler{capRepo: capRepo}
}

func (h *AdminCertBundleHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	bundles := admin.Group("/cert-bundles")
	{
		bundles.GET("", rbac.RequirePermission("nodes.read"), h.List)
		bundles.POST("", rbac.RequirePermission("nodes.write"), h.Create)
		bundles.GET("/:id", rbac.RequirePermission("nodes.read"), h.Get)
		bundles.DELETE("/:id", rbac.RequirePermission("nodes.write"), h.Delete)
	}
}

// certBundleResponse 列表响应（不含私钥，安全考虑）
type certBundleResponse struct {
	ID        uuid.UUID  `json:"id"`
	Provider  string     `json:"provider"`
	Mode      string     `json:"mode"`
	SAN       []string   `json:"san"`
	NotAfter  *time.Time `json:"not_after,omitempty"`
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// certBundleDetailResponse 详情响应（含 PEM，仅 GET /:id 返回）
type certBundleDetailResponse struct {
	certBundleResponse
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// createCertBundleRequest 创建请求
type createCertBundleRequest struct {
	Provider string   `json:"provider" binding:"required"`
	Mode     string   `json:"mode" binding:"required"`
	CertPEM  string   `json:"cert_pem" binding:"required"`
	KeyPEM   string   `json:"key_pem" binding:"required"`
	SAN      []string `json:"san"`
}

func (h *AdminCertBundleHandler) List(c *gin.Context) {
	provider := c.Query("provider")
	bundles, err := h.capRepo.ListCertBundles(c.Request.Context(), provider)
	if err != nil {
		server.InternalError(c, "failed to list cert bundles")
		return
	}
	items := make([]certBundleResponse, 0, len(bundles))
	for _, b := range bundles {
		items = append(items, certBundleResponse{
			ID:        b.ID,
			Provider:  b.Provider,
			Mode:      b.Mode,
			SAN:       b.SAN,
			NotAfter:  b.NotAfter,
			Version:   b.Version,
			CreatedAt: b.CreatedAt,
			UpdatedAt: b.UpdatedAt,
		})
	}
	server.OK(c, gin.H{"items": items, "total": len(items)})
}

func (h *AdminCertBundleHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid id")
		return
	}
	b, err := h.capRepo.GetCertBundle(c.Request.Context(), id)
	if err != nil {
		server.InternalError(c, "failed to get cert bundle")
		return
	}
	if b == nil {
		server.NotFound(c, "cert bundle not found")
		return
	}
	server.OK(c, certBundleDetailResponse{
		certBundleResponse: certBundleResponse{
			ID:        b.ID,
			Provider:  b.Provider,
			Mode:      b.Mode,
			SAN:       b.SAN,
			NotAfter:  b.NotAfter,
			Version:   b.Version,
			CreatedAt: b.CreatedAt,
			UpdatedAt: b.UpdatedAt,
		},
		CertPEM: b.CertPEM,
		KeyPEM:  b.KeyPEM,
	})
}

func (h *AdminCertBundleHandler) Create(c *gin.Context) {
	var req createCertBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, err.Error())
		return
	}
	cb := &repo.CertBundle{
		ID:       uuid.New(),
		Provider: req.Provider,
		Mode:     req.Mode,
		CertPEM:  req.CertPEM,
		KeyPEM:   req.KeyPEM,
		SAN:      req.SAN,
		Version:  1,
	}
	if err := h.capRepo.CreateCertBundle(c.Request.Context(), cb); err != nil {
		server.InternalError(c, "failed to create cert bundle")
		return
	}
	server.OK(c, certBundleResponse{
		ID:        cb.ID,
		Provider:  cb.Provider,
		Mode:      cb.Mode,
		SAN:       cb.SAN,
		Version:   cb.Version,
		CreatedAt: cb.CreatedAt,
		UpdatedAt: cb.UpdatedAt,
	})
}

func (h *AdminCertBundleHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.BadRequest(c, "invalid id")
		return
	}
	if err := h.capRepo.DeleteCertBundle(c.Request.Context(), id); err != nil {
		server.InternalError(c, "failed to delete cert bundle")
		return
	}
	server.OK(c, gin.H{"deleted": true})
}
