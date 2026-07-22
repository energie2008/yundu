package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout 给所有请求套一个统一的超时。
// 注意：对于 WebSocket / 长连接类请求应使用 TimeoutByPath 豁免，否则在 ctx.Done()
// 时会强制 504，但此时连接往往已被 handler 接管，写入会失败（不会真正断开）。
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			rid := GetRequestID(c)
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"code":       50300,
					"message":    "request timeout",
					"request_id": rid,
				})
			}
			return
		}
	}
}

// TimeoutByPath 与 Timeout 行为一致，但允许按 URL 前缀覆盖超时时长。
// overrides 的 key 是路径前缀（如 "/api/v1/admin/diagnosis/"），value 是该前缀下的超时。
// value <= 0 表示豁免超时（用于 WebSocket / SSE / 长轮询等场景）。
// 多个前缀命中时，取最长匹配前缀的值，避免误命中较短前缀。
func TimeoutByPath(defaultDuration time.Duration, overrides map[string]time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		d := defaultDuration
		var bestMatch string
		for prefix := range overrides {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			if len(prefix) > len(bestMatch) {
				bestMatch = prefix
			}
		}
		if bestMatch != "" {
			d = overrides[bestMatch]
		}
		if d <= 0 {
			// 豁免：不设超时，直接放行
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			rid := GetRequestID(c)
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"code":       50300,
					"message":    "request timeout",
					"request_id": rid,
				})
			}
			return
		}
	}
}
