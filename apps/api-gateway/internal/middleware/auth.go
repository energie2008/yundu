package middleware

import (
	"errors"
	"strings"

	"github.com/airport-panel/config"
	pkgmw "github.com/airport-panel/config/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	CtxUserID      = "user_id"
	CtxAdminID     = "admin_id"
	CtxIsAdmin     = "is_admin"
	CtxPermissions = "permissions"
)

type GatewayClaims struct {
	UserID      uuid.UUID `json:"user_id"`
	TokenType   string    `json:"token_type"`
	SessionID   uuid.UUID `json:"session_id"`
	IsAdmin     bool     `json:"is_admin"`
	Permissions []string `json:"permissions,omitempty"`
	AdminID     uuid.UUID `json:"admin_id,omitempty"`
	jwt.RegisteredClaims
}

type AuthMiddleware struct {
	secret []byte
}

func NewAuthMiddleware(jwtSecret string) *AuthMiddleware {
	return &AuthMiddleware{secret: []byte(jwtSecret)}
}

func (m *AuthMiddleware) parseToken(tokenString string) (*GatewayClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &GatewayClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*GatewayClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.TokenType != "access" {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}

func extractBearerToken(c *gin.Context) string {
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

func unauthorized(c *gin.Context, message string) {
	rid := pkgmw.GetRequestID(c)
	if message == "" {
		message = "unauthorized"
	}
	c.AbortWithStatusJSON(config.CodeUnauthorized.HTTPStatus(), gin.H{
		"code":       config.CodeUnauthorized,
		"message":    message,
		"request_id": rid,
	})
}

func forbidden(c *gin.Context, message string) {
	rid := pkgmw.GetRequestID(c)
	if message == "" {
		message = "forbidden"
	}
	c.AbortWithStatusJSON(config.CodeForbidden.HTTPStatus(), gin.H{
		"code":       config.CodeForbidden,
		"message":    message,
		"request_id": rid,
	})
}

func (m *AuthMiddleware) UserAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c)
		if tokenStr == "" {
			unauthorized(c, "")
			return
		}
		claims, err := m.parseToken(tokenStr)
		if err != nil {
			unauthorized(c, "invalid or expired token")
			return
		}
		if claims.IsAdmin {
			unauthorized(c, "invalid token type")
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxIsAdmin, false)
		c.Next()
	}
}

func (m *AuthMiddleware) AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c)
		if tokenStr == "" {
			unauthorized(c, "")
			return
		}
		claims, err := m.parseToken(tokenStr)
		if err != nil {
			unauthorized(c, "invalid or expired token")
			return
		}
		if !claims.IsAdmin {
			forbidden(c, "admin access required")
			return
		}
		adminID := claims.AdminID
		if adminID == uuid.Nil {
			adminID = claims.UserID
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxAdminID, adminID)
		c.Set(CtxIsAdmin, true)
		// permissions 直接来自 identity-service 签发的 JWT：
		//   - super_admin: ["*"]（hasPermission 会短路放行）
		//   - 普通 admin:  实际权限 code 列表
		perms := claims.Permissions
		if perms == nil {
			perms = []string{}
		}
		c.Set(CtxPermissions, perms)
		c.Next()
	}
}

// hasPermission 检查权限列表是否包含 required 或通配符 "*"。
func hasPermission(permissions []string, required string) bool {
	for _, p := range permissions {
		if p == required || p == "*" {
			return true
		}
	}
	return false
}

func RequirePermission(permissionCode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, exists := c.Get(CtxIsAdmin)
		isAdmin, ok := v.(bool)
		if !exists || !ok || !isAdmin {
			forbidden(c, "")
			return
		}
		rawPerms, _ := c.Get(CtxPermissions)
		perms, _ := rawPerms.([]string)
		if !hasPermission(perms, permissionCode) {
			forbidden(c, "permission denied: "+permissionCode)
			return
		}
		c.Next()
	}
}

func GetUserID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get(CtxUserID); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}
