package middleware

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/airport-panel/config/auth"
	"github.com/airport-panel/config/server"
	"github.com/airport-panel/node-service/internal/pkg"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	CtxAgentServerID   = "agent_server_id"
	CtxAgentServerCode = "agent_server_code"

	HeaderServerCode = "X-Server-Code"
	HeaderAgentToken = "X-Agent-Token"
	HeaderNonce      = "X-Nonce"
	HeaderTimestamp  = "X-Timestamp"
	HeaderSignature  = "X-Signature"

	timestampWindowSec = 300
)

type AgentAuthMiddleware struct {
	tokenSalt  string
	hmacSecret string
	nonceCache *pkg.NonceCache
}

func NewAgentAuthMiddleware(tokenSalt, hmacSecret string, nonceCache *pkg.NonceCache) *AgentAuthMiddleware {
	return &AgentAuthMiddleware{
		tokenSalt:  tokenSalt,
		hmacSecret: hmacSecret,
		nonceCache: nonceCache,
	}
}

func (m *AgentAuthMiddleware) AgentAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		serverCode := c.GetHeader(HeaderServerCode)
		token := c.GetHeader(HeaderAgentToken)
		nonce := c.GetHeader(HeaderNonce)
		timestampStr := c.GetHeader(HeaderTimestamp)
		signature := c.GetHeader(HeaderSignature)

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

		if nonce == "" || timestampStr == "" || signature == "" {
			server.Unauthorized(c, "missing nonce/timestamp/signature")
			c.Abort()
			return
		}

		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			server.Unauthorized(c, "invalid timestamp format")
			c.Abort()
			return
		}

		now := time.Now().Unix()
		if timestamp < now-timestampWindowSec || timestamp > now+timestampWindowSec {
			server.Unauthorized(c, "timestamp out of allowed window")
			c.Abort()
			return
		}

		// 读取 request body 用于签名验证（与 agent 端 auth.Sign 算法一致）
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 使用与 agent 端相同的签名算法：auth.VerifySignature(method, path, body, ts, nonce, sig, HMAC_SECRET)
		// 注意：agent 端签名时 path 包含 query string（如 /api/v1/agent/config?version=73），
		// 因此服务端必须使用 URL.RequestURI() 而非 URL.Path，否则 GET 请求会因 path 不匹配而验签失败
		if err := auth.VerifySignature(c.Request.Method, c.Request.URL.RequestURI(), string(bodyBytes), timestampStr, nonce, signature, m.hmacSecret); err != nil {
			server.Unauthorized(c, "invalid signature")
			c.Abort()
			return
		}

		if m.nonceCache != nil {
			if err := m.nonceCache.CheckAndStore(serverCode, nonce, timestamp); err != nil {
				server.Unauthorized(c, err.Error())
				c.Abort()
				return
			}
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

func GetAgentServerID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get(CtxAgentServerID); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

func SetAgentServerID(c *gin.Context, id uuid.UUID) {
	c.Set(CtxAgentServerID, id)
}
