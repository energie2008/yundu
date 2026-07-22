package middleware

import (
	"strings"

	"github.com/airport-panel/config/server"
	"github.com/airport-panel/identity-service/internal/pkg"
	"github.com/airport-panel/identity-service/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	CtxUserID     = "user_id"
	CtxAdminID    = "admin_id"
	CtxSessionID  = "session_id"
	CtxIsAdmin    = "is_admin"
	CtxAdminEmail = "admin_email"
)

type AuthMiddleware struct {
	jwtManager  *pkg.JWTManager
	authService *service.AuthService
}

func NewAuthMiddleware(jwtManager *pkg.JWTManager, authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		jwtManager:  jwtManager,
		authService: authService,
	}
}

func (m *AuthMiddleware) UserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			server.Unauthorized(c, "")
			c.Abort()
			return
		}

		claims, err := m.jwtManager.ValidateToken(token, pkg.TokenTypeAccess)
		if err != nil {
			server.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		if claims.IsAdmin {
			server.Unauthorized(c, "invalid token type")
			c.Abort()
			return
		}

		session, err := m.authService.ValidateSession(c.Request.Context(), claims.SessionID)
		if err != nil || session == nil {
			server.Unauthorized(c, "session expired")
			c.Abort()
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxSessionID, claims.SessionID)
		c.Set(CtxIsAdmin, false)
		c.Next()
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

		claims, err := m.jwtManager.ValidateToken(token, pkg.TokenTypeAccess)
		if err != nil {
			server.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}

		if !claims.IsAdmin {
			server.Forbidden(c, "admin access required")
			c.Abort()
			return
		}

		session, err := m.authService.ValidateSession(c.Request.Context(), claims.SessionID)
		if err != nil || session == nil {
			server.Unauthorized(c, "session expired")
			c.Abort()
			return
		}

		admin, user, err := m.authService.GetAdminMe(c.Request.Context(), claims.UserID)
		if err != nil || admin == nil {
			server.Unauthorized(c, "admin not found")
			c.Abort()
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxAdminID, admin.ID)
		c.Set(CtxAdminEmail, user.Email)
		c.Set(CtxSessionID, claims.SessionID)
		c.Set(CtxIsAdmin, true)
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

func GetAdminEmail(c *gin.Context) string {
	if v, exists := c.Get(CtxAdminEmail); exists {
		if email, ok := v.(string); ok {
			return email
		}
	}
	return ""
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
