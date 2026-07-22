package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func AccessLog(logger *slog.Logger, serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		rid := GetRequestID(c)

		attrs := []any{
			"service", serviceName,
			"request_id", rid,
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", float64(latency.Microseconds()) / 1000.0,
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
			"bytes_out", c.Writer.Size(),
		}

		if query != "" {
			attrs = append(attrs, "query", query)
		}

		if len(c.Errors) > 0 {
			attrs = append(attrs, "error", c.Errors.String())
			logger.Error("request completed", attrs...)
			return
		}

		if status >= 500 {
			logger.Error("request completed", attrs...)
		} else if status >= 400 {
			logger.Warn("request completed", attrs...)
		} else {
			logger.Info("request completed", attrs...)
		}
	}
}
