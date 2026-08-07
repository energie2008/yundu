package cert

import (
	"log/slog"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AdminACMEDefaultsHandler 管理面板全局 ACME 默认账户（TLS 证书页）。
type AdminACMEDefaultsHandler struct {
	store  ACMEDefaultsStore
	logger *slog.Logger
}

func NewAdminACMEDefaultsHandler(store ACMEDefaultsStore, logger *slog.Logger) *AdminACMEDefaultsHandler {
	return &AdminACMEDefaultsHandler{store: store, logger: logger}
}

func (h *AdminACMEDefaultsHandler) RegisterRoutesWithGroup(admin *gin.RouterGroup, rbac *middleware.RBACMiddleware) {
	g := admin.Group("/tls-certificates")
	g.GET("/defaults", rbac.RequirePermission("nodes.read"), h.GetDefaults)
	g.PUT("/defaults", rbac.RequirePermission("nodes.write"), h.PutDefaults)
}

// GetDefaults 返回当前 ACME 默认账户（凭证仅返回是否已配置）。
func (h *AdminACMEDefaultsHandler) GetDefaults(c *gin.Context) {
	d, err := h.store.Load(c.Request.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("acme defaults load failed", "error", err)
		}
		server.InternalError(c, "")
		return
	}
	server.OK(c, defaultsToDTO(d))
}

type putACMEDefaultsRequest struct {
	Email         string             `json:"email"`
	DirURL        string             `json:"dir_url"`
	ChallengeType string             `json:"challenge_type"`
	DNSProvider   string             `json:"dns_provider"`
	Credentials   *map[string]string `json:"credentials"`
}

// PutDefaults 保存 ACME 默认账户。
// credentials 为 nil 时保留已有凭证；空 map 清空凭证；非空时加密存储。
func (h *AdminACMEDefaultsHandler) PutDefaults(c *gin.Context) {
	var req putACMEDefaultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()

	current, err := h.store.Load(ctx)
	if err != nil {
		server.InternalError(c, "")
		return
	}
	if current == nil {
		current = &ACMEDefaults{}
	}

	d := &ACMEDefaults{
		Email:                req.Email,
		DirURL:               req.DirURL,
		ChallengeType:        req.ChallengeType,
		DNSProvider:          req.DNSProvider,
		CredentialsEncrypted: current.CredentialsEncrypted,
		HasCredentials:       current.HasCredentials,
	}

	if req.Credentials != nil {
		if len(*req.Credentials) == 0 {
			d.CredentialsEncrypted = ""
			d.HasCredentials = false
		} else if req.DNSProvider != "" {
			if err := ValidateDNSProviderCredentials(req.DNSProvider, *req.Credentials); err != nil {
				server.BadRequest(c, "validate dns credentials: "+err.Error())
				return
			}
			enc, err := EncryptCredentials(ACMECredentials{
				Provider: req.DNSProvider,
				Vars:     *req.Credentials,
			})
			if err != nil {
				server.InternalError(c, "")
				return
			}
			d.CredentialsEncrypted = enc
			d.HasCredentials = true
		}
	}

	if err := h.store.Save(ctx, d); err != nil {
		if h.logger != nil {
			h.logger.Error("acme defaults save failed", "error", err)
		}
		server.InternalError(c, "")
		return
	}
	if h.logger != nil {
		h.logger.Info("ACME defaults updated",
			"email", d.Email, "challenge_type", d.ChallengeType,
			"dns_provider", d.DNSProvider, "has_credentials", d.HasCredentials)
	}
	server.OK(c, defaultsToDTO(d))
}
