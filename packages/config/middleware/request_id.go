package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const (
	RequestIDHeader = "X-Request-ID"
	RequestIDKey    = "request_id"
)

func generateRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "000000000000000000000000"
	}
	return hex.EncodeToString(b)
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = generateRequestID()
		}
		c.Set(RequestIDKey, rid)
		c.Writer.Header().Set(RequestIDHeader, rid)
		c.Request = c.Request.WithContext(
			context.WithValue(c.Request.Context(), RequestIDKey, rid),
		)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
