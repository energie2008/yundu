package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// 限流策略常量（对齐任务要求）：
//   - 登录接口：每 IP 每分钟 5 次
//   - 注册接口：每 IP 每小时 3 次
//   - API 接口：每用户每秒 10 次（未登录则按 IP）
const (
	loginLimit     = 5
	loginWindow    = 1 * time.Minute
	registerLimit  = 3
	registerWindow = 1 * time.Hour
	apiLimit       = 10
	apiWindow      = 1 * time.Second
)

// incrExpireScript 原子地 INCR 并在首次创建时设置过期，避免竞态下窗口不刷新。
var incrExpireScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// ratePolicy 描述一条限流策略。
type ratePolicy struct {
	keyPrefix string
	limit     int64
	window    time.Duration
}

// RateLimitMiddleware 基于 Redis 的请求频率限制中间件。
//
// 根据请求 path / method 自动选择策略：
//   - 登录接口（POST，path 以 /auth/login 结尾）：每 IP 每分钟 5 次
//   - 注册接口（POST，path 以 /auth/register 结尾）：每 IP 每小时 3 次
//   - 其他 API 接口：每用户每秒 10 次（未登录用户按 IP 计）
//
// redisClient 为 nil 时降级为直接放行（不阻断业务）。
func RateLimitMiddleware(redisClient *redis.Client) gin.HandlerFunc {
	// RATE_LIMIT_ENABLED=false 时完全禁用限流（测试阶段使用）
	if os.Getenv("RATE_LIMIT_ENABLED") == "false" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if redisClient == nil {
			c.Next()
			return
		}

		policy, identifier := resolveRatePolicy(c)
		if identifier == "" {
			c.Next()
			return
		}

		key := fmt.Sprintf("rl:%s:%s", policy.keyPrefix, identifier)
		ctx := c.Request.Context()

		count, err := incrExpireScript.Run(ctx, redisClient, []string{key}, int64(policy.window.Seconds())).Int64()
		if err != nil {
			// Redis 异常时放行，避免基础设施抖动导致服务不可用。
			c.Next()
			return
		}
		if count > policy.limit {
			ttl, _ := redisClient.TTL(ctx, key).Result()
			c.Header("Retry-After", strconv.FormatInt(int64(ttl.Seconds()), 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": int(ttl.Seconds()),
			})
			return
		}
		c.Next()
	}
}

// resolveRatePolicy 依据请求路径/方法与登录态选择限流策略与标识符。
func resolveRatePolicy(c *gin.Context) (ratePolicy, string) {
	path := c.Request.URL.Path
	method := c.Request.Method

	switch {
	case method == http.MethodPost && hasPathSuffix(path, "/auth/login"):
		return ratePolicy{keyPrefix: "login", limit: loginLimit, window: loginWindow}, c.ClientIP()
	case method == http.MethodPost && hasPathSuffix(path, "/auth/register"):
		return ratePolicy{keyPrefix: "register", limit: registerLimit, window: registerWindow}, c.ClientIP()
	default:
		// API 接口：优先按用户 ID 限流，未登录按 IP
		if uid := GetUserID(c); uid != uuid.Nil {
			return ratePolicy{keyPrefix: "api", limit: apiLimit, window: apiWindow}, uid.String()
		}
		return ratePolicy{keyPrefix: "api", limit: apiLimit, window: apiWindow}, c.ClientIP()
	}
}

// hasPathSuffix 判断 path 是否以 suffix 结尾（忽略尾部斜杠）。
func hasPathSuffix(path, suffix string) bool {
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
