package handler

import (
	"github.com/gin-gonic/gin"
)

func RegisterHealthRoutes(r *gin.RouterGroup) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "component": "api-gateway"})
	})
}
