package middleware

import (
	"strings"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	CtxUserID    = "user_id"
	CtxAdminID   = "admin_id"
	CtxSessionID = "session_id"
	CtxIsAdmin   = "is_admin"
	CtxPermissions = "permissions"
)

type AuthMiddleware struct {
	jwtManager *pkg.JWTManager
}

func NewAuthMiddleware(jwtManager *pkg.JWTManager) *AuthMiddleware {
	return &AuthMiddleware{
		jwtManager: jwtManager,
	}
}

func (m *AuthMiddleware) AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			server.Unauthorized(c, "")
			c.Abort()
			return
		}

		claims, err := m.jwtManager.ValidateAdminToken(token)
		if err != nil {
			server.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		adminID := claims.AdminID
		if adminID == uuid.Nil {
			adminID = claims.UserID
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxAdminID, adminID)
		c.Set(CtxSessionID, claims.SessionID)
		c.Set(CtxIsAdmin, true)
		// permissions 直接来自 identity-service 签发的 JWT：
		//   - super_admin: ["*"]（由 identity-service 注入，RequirePermission 中间件会短路放行）
		//   - 普通 admin:  实际权限 code 列表（可能为空 slice，表示无任何权限）
		// 这里不再做 ["*"] 兜底，避免越权。
		perms := claims.Permissions
		if perms == nil {
			perms = []string{}
		}
		c.Set(CtxPermissions, perms)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func GetUserID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get(CtxUserID); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

func GetAdminID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get(CtxAdminID); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

func GetSessionID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get(CtxSessionID); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

func GetIsAdmin(c *gin.Context) bool {
	if v, exists := c.Get(CtxIsAdmin); exists {
		if isAdmin, ok := v.(bool); ok {
			return isAdmin
		}
	}
	return false
}

func GetPermissions(c *gin.Context) []string {
	if v, exists := c.Get(CtxPermissions); exists {
		if perms, ok := v.([]string); ok {
			return perms
		}
	}
	return nil
}
