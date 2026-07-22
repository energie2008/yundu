package middleware

import (
	"github.com/airport-panel/config/server"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RBACMiddleware struct{}

func NewRBACMiddleware() *RBACMiddleware {
	return &RBACMiddleware{}
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

		permissions := GetPermissions(c)
		if !hasPermission(permissions, permissionCode) {
			server.Forbidden(c, "permission denied: "+permissionCode)
			c.Abort()
			return
		}

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

		permissions := GetPermissions(c)
		if !hasPermission(permissions, "*") && !hasPermission(permissions, "admin:*") {
			server.Forbidden(c, "super admin required")
			c.Abort()
			return
		}

		c.Set("is_super_admin", true)
		c.Next()
	}
}

func hasPermission(permissions []string, required string) bool {
	for _, p := range permissions {
		if p == required || p == "*" {
			return true
		}
	}
	return false
}
