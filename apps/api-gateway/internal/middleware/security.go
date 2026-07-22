package middleware

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// 防暴力破解策略常量：
//   - 连续失败 5 次，锁定 15 分钟
const (
	bruteForceMaxFailures = 5
	bruteForceLockTTL     = 15 * time.Minute
)

// AntiBruteForce 防暴力破解中间件，建议挂在登录类路由上。
//
// 工作方式：
//  1. 请求进入前，读取 Redis 中该 IP 的失败计数；若已达阈值（5 次）则直接返回 429
//     并附带剩余锁定秒数。
//  2. 放行至下游 handler 执行。
//  3. handler 返回后依据响应状态判定结果：
//     - 401（未授权）视为一次失败，INCR 失败计数并在首次失败时设置 15 分钟过期；
//     - 2xx（成功）视为登录成功，清空失败计数；
//     - 其他状态不改变计数。
//
// 该方案无需侵入登录 handler，仅依赖其返回 401 表示凭据错误。
// redisClient 为 nil 时降级为直接放行。
func AntiBruteForce(redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if redisClient == nil {
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := "bf:fail:" + ip
		ctx := c.Request.Context()

		// 1. 进入前检查是否已被锁定
		failures, err := redisClient.Get(ctx, key).Int64()
		if err == nil && failures >= bruteForceMaxFailures {
			ttl, _ := redisClient.TTL(ctx, key).Result()
			secs := int64(ttl.Seconds())
			if secs <= 0 {
				secs = int64(bruteForceLockTTL.Seconds())
			}
			c.Header("Retry-After", strconv.FormatInt(secs, 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "account temporarily locked due to too many failed attempts",
				"retry_after": secs,
			})
			return
		}

		// 2. 放行至下游
		c.Next()

		// 3. 依据响应状态记录成功/失败
		status := c.Writer.Status()
		switch {
		case status == http.StatusUnauthorized:
			n, err := redisClient.Incr(ctx, key).Result()
			if err == nil && n == 1 {
				_ = redisClient.Expire(ctx, key, bruteForceLockTTL).Err()
			}
		case status >= 200 && status < 300:
			// 登录/操作成功，清空失败计数
			_ = redisClient.Del(ctx, key).Err()
		}
	}
}

// maliciousUAReqs 命中即视为恶意 UA 的特征（SQL 注入 / XSS / 路径穿越等）。
var maliciousUAReqs = []*regexp.Regexp{
	// SQL 注入特征
	regexp.MustCompile(`(?i)\bunion\b\s+\bselect\b`),
	regexp.MustCompile(`(?i)\bselect\b\s+.*\bfrom\b`),
	regexp.MustCompile(`(?i)\binsert\b\s+\binto\b`),
	regexp.MustCompile(`(?i)\bdelete\b\s+\bfrom\b`),
	regexp.MustCompile(`(?i)\bdrop\b\s+\b(table|database)\b`),
	regexp.MustCompile(`(?i)\bupdate\b\s+\bset\b`),
	regexp.MustCompile(`(?i)'\s*or\s*'?1'?\s*=\s*'?1`),
	regexp.MustCompile(`(?i)--\s*$`),
	regexp.MustCompile(`(?i);\s*drop\b`),
	// XSS 特征
	regexp.MustCompile(`(?i)<\s*script\b`),
	regexp.MustCompile(`(?i)javascript\s*:`),
	regexp.MustCompile(`(?i)\bon(error|load|click|mouseover)\s*=`),
	regexp.MustCompile(`(?i)<\s*img\b[^>]*\bonerror\s*=`),
	regexp.MustCompile(`(?i)<\s*iframe\b`),
	// 路径穿越
	regexp.MustCompile(`\.\./`),
}

// BlockMaliciousUA 拦截恶意 User-Agent。
//
// 拦截规则：
//   - 空 UA（含纯空白）
//   - 命中 SQL 注入 / XSS / 路径穿越特征正则
//
// 命中后直接返回 403。无外部依赖，可安全全局挂载。
func BlockMaliciousUA() gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := c.Request.UserAgent()
		if strings.TrimSpace(ua) == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "empty user-agent is not allowed"})
			return
		}
		for _, re := range maliciousUAReqs {
			if re.MatchString(ua) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "malicious user-agent detected"})
				return
			}
		}
		c.Next()
	}
}
