package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error)
}

type RateLimitConfig struct {
	RequestsPerMinute int
	BurstMultiplier   int
	Window            time.Duration
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 30,
		BurstMultiplier:   2,
		Window:            time.Minute,
	}
}

type redisRateLimiter struct {
	client *goredis.Client
}

func NewRedisRateLimiter(client *goredis.Client) RateLimiter {
	return &redisRateLimiter{client: client}
}

func (r *redisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	now := time.Now()
	windowStart := now.Add(-window)
	k := fmt.Sprintf("ratelimit:%s", key)

	pipe := r.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, k, "0", strconv.FormatInt(windowStart.UnixMicro(), 10))
	pipe.ZCard(ctx, k)
	pipe.ZAdd(ctx, k, goredis.Z{
		Score:  float64(now.UnixMicro()),
		Member: now.UnixNano(),
	})
	pipe.Expire(ctx, k, window+time.Second)

	cmders, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, err
	}

	if len(cmders) < 2 {
		return false, 0, time.Time{}, fmt.Errorf("unexpected pipeline response")
	}

	countCmd, ok := cmders[1].(*goredis.IntCmd)
	if !ok {
		return false, 0, time.Time{}, fmt.Errorf("unexpected count cmd type")
	}
	count := int(countCmd.Val())

	remaining := limit - count
	resetAt := now.Add(window)
	allowed := count <= limit

	return allowed, remaining, resetAt, nil
}

type memoryLimiterEntry struct {
	timestamps []time.Time
}

type memoryRateLimiter struct {
	mu     sync.RWMutex
	limits map[string]*memoryLimiterEntry
}

func NewMemoryRateLimiter() RateLimiter {
	rl := &memoryRateLimiter{
		limits: make(map[string]*memoryLimiterEntry),
	}
	go rl.cleanup()
	return rl
}

func (m *memoryRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-window)

	entry, exists := m.limits[key]
	if !exists {
		entry = &memoryLimiterEntry{timestamps: make([]time.Time, 0)}
		m.limits[key] = entry
	}

	valid := make([]time.Time, 0, len(entry.timestamps))
	for _, ts := range entry.timestamps {
		if ts.After(windowStart) {
			valid = append(valid, ts)
		}
	}
	entry.timestamps = valid
	count := len(valid)

	entry.timestamps = append(entry.timestamps, now)
	count++

	remaining := limit - count
	resetAt := now.Add(window)
	allowed := count <= limit

	return allowed, remaining, resetAt, nil
}

func (m *memoryRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for k, entry := range m.limits {
			windowStart := now.Add(-10 * time.Minute)
			hasRecent := false
			for _, ts := range entry.timestamps {
				if ts.After(windowStart) {
					hasRecent = true
					break
				}
			}
			if !hasRecent {
				delete(m.limits, k)
			}
		}
		m.mu.Unlock()
	}
}

type RateLimitMiddleware struct {
	limiter RateLimiter
	config  RateLimitConfig
}

func NewRateLimitMiddleware(limiter RateLimiter, config RateLimitConfig) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
		config:  config,
	}
}

func (m *RateLimitMiddleware) SubscriptionRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := c.Param("token")
		if identifier == "" {
			identifier = c.Param("code")
		}
		if identifier == "" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			if parts := splitAndTrim(xff); len(parts) > 0 {
				clientIP = parts[0]
			}
		}

		key := fmt.Sprintf("sub:%s:%s", identifier, clientIP)
		limit := m.config.RequestsPerMinute
		burstLimit := limit * m.config.BurstMultiplier

		allowed, remaining, resetAt, err := m.limiter.Allow(c.Request.Context(), key, burstLimit, m.config.Window)
		if err != nil {
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(burstLimit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(max(0, remaining)))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(time.Until(resetAt).Seconds())))
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusTooManyRequests, "rate limit exceeded, please retry later")
			c.Abort()
			return
		}

		c.Next()
	}
}

func (m *RateLimitMiddleware) GlobalRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			if parts := splitAndTrim(xff); len(parts) > 0 {
				clientIP = parts[0]
			}
		}

		key := fmt.Sprintf("global:%s", clientIP)
		limit := 120

		allowed, remaining, resetAt, err := m.limiter.Allow(c.Request.Context(), key, limit, time.Minute)
		if err != nil {
			c.Next()
			return
		}

		c.Header("X-RateLimit-Global-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Global-Remaining", strconv.Itoa(max(0, remaining)))

		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(time.Until(resetAt).Seconds())))
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusTooManyRequests, "rate limit exceeded, please retry later")
			c.Abort()
			return
		}

		c.Next()
	}
}

func splitAndTrim(s string) []string {
	var parts []string
	for _, p := range splitString(s, ',') {
		if trimmed := trimSpace(p); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
