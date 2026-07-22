package middleware

import (
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/traffic-service/internal/pkg"
	"github.com/gin-gonic/gin"
)

const (
	CtxAgentServerCode = "agent_server_code"
)

type AgentAuthMiddleware struct {
	tokenSalt string
}

func NewAgentAuthMiddleware(tokenSalt string) *AgentAuthMiddleware {
	return &AgentAuthMiddleware{
		tokenSalt: tokenSalt,
	}
}

func (m *AgentAuthMiddleware) AgentAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		serverCode := c.GetHeader("X-Server-Code")
		token := c.GetHeader("X-Agent-Token")

		if serverCode == "" || token == "" {
			server.Unauthorized(c, "missing agent credentials")
			c.Abort()
			return
		}

		if !pkg.ValidateAgentToken(serverCode, token, m.tokenSalt) {
			server.Unauthorized(c, "invalid agent token")
			c.Abort()
			return
		}

		c.Set(CtxAgentServerCode, serverCode)
		c.Next()
	}
}

func GetAgentServerCode(c *gin.Context) string {
	if v, exists := c.Get(CtxAgentServerCode); exists {
		if code, ok := v.(string); ok {
			return code
		}
	}
	return ""
}
