package middleware

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	pkgmw "github.com/airport-panel/config/middleware"
	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	window   time.Duration
	whitelistPaths []string
}

type visitor struct {
	count    int
	lastSeen time.Time
}

func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
		whitelistPaths: []string{
			"/healthz",
			"/readyz",
			"/metrics",
			"/sub/",
			"/api/v1/agent/", // agent 请求不限流（心跳/注册/配置拉取/资源reconcile）
		},
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) isWhitelisted(path string) bool {
	for _, prefix := range rl.whitelistPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]
	if !exists || now.Sub(v.lastSeen) > rl.window {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: now}
		return true
	}
	v.lastSeen = now
	if v.count >= rl.rate {
		return false
	}
	v.count++
	return true
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	// RATE_LIMIT_ENABLED=false 时完全禁用限流（测试阶段使用）
	if os.Getenv("RATE_LIMIT_ENABLED") == "false" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if rl.isWhitelisted(c.Request.URL.Path) {
			c.Next()
			return
		}

		ip := c.ClientIP()
		if !rl.Allow(ip) {
			rid := pkgmw.GetRequestID(c)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":       42900,
				"message":    "rate limit exceeded",
				"request_id": rid,
			})
			return
		}
		c.Next()
	}
}
