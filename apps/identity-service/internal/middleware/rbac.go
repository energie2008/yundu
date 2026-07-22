package middleware

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RBACMiddleware struct {
	rbacService *service.RBACService
	adminRepo   *repo.AdminRepo
}

func NewRBACMiddleware(rbacService *service.RBACService, adminRepo *repo.AdminRepo) *RBACMiddleware {
	return &RBACMiddleware{
		rbacService: rbacService,
		adminRepo:   adminRepo,
	}
}

func (m *RBACMiddleware) RequirePermission(permissionCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin := GetIsAdmin(c)
		if !isAdmin {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		adminID := GetAdminID(c)
		if adminID == uuid.Nil {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		admin, err := m.adminRepo.GetByID(c.Request.Context(), adminID)
		if err != nil || admin == nil {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		if err := m.rbacService.RequirePermission(c.Request.Context(), adminID, admin.IsSuperAdmin, permissionCode); err != nil {
			server.Forbidden(c, err.Error())
			c.Abort()
			return
		}

		c.Set("is_super_admin", admin.IsSuperAdmin)
		c.Next()
	}
}

func (m *RBACMiddleware) RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin := GetIsAdmin(c)
		if !isAdmin {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		adminID := GetAdminID(c)
		if adminID == uuid.Nil {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		admin, err := m.adminRepo.GetByID(c.Request.Context(), adminID)
		if err != nil || admin == nil {
			server.Forbidden(c, "")
			c.Abort()
			return
		}

		if !admin.IsSuperAdmin {
			server.Forbidden(c, "super admin required")
			c.Abort()
			return
		}

		c.Set("is_super_admin", true)
		c.Next()
	}
}
