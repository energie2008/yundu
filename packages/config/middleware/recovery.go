package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/airport-panel/config/observability"
	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				rid := GetRequestID(c)
				serviceName := c.GetString("service_name")
				if serviceName == "" {
					serviceName = "unknown"
				}
				observability.RecordPanic(serviceName)
				logger.Error("panic recovered",
					"request_id", rid,
					"service", serviceName,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"error", err,
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":       50000,
					"message":    "internal server error",
					"request_id": rid,
				})
			}
		}()
		c.Next()
	}
}
