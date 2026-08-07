package repo

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SubscribeSettings 订阅相关设置（system_settings.subscribe 组）。
// 由 subscription-service 启动时加载，60s 内存缓存，TopicConfigChanged 事件主动刷新。
// 直读 DB（与 node-service ACMEDefaultsRepo 同模式），避免跨服务 API 依赖。
type SubscribeSettings struct {
	SubscribePath    string // 订阅路径前缀，默认 "/sub"；用于 user-web 生成 URL（后端双路由始终注册）
	SubscribeDomain  string // 订阅域名，留空则 user-web 用请求 Host；非空时 user-web 用此域名拼 URL
	SubscribeKey     string // HMAC-SHA256 密钥；空则不下发 X-Subscribe-Signature 头
	ShowInfoToServer bool   // 是否回显节点信息给客户端（保留字段，当前未直接影响渲染）
	ShowMethod       int    // 0=不下发 Subscription-Userinfo 头；1=完整（upload/download/total/expire）；2=完整（预留扩展）
	IsRandSub        bool   // 随机订阅：开启后按 [RandSubStart,RandSubEnd) 随机抽样节点
	RandSubStart     int    // 随机抽样下界（含），默认 0
	RandSubEnd       int    // 随机抽样上界（不含），默认 10
}

// SubscribeSettingsRepo 直读 system_settings.subscribe 组，带 60s 缓存。
// DB 错误时返回默认值不阻断订阅（订阅是核心链路，设置读取失败不应让用户订阅 500）。
type SubscribeSettingsRepo struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	cached *SubscribeSettings
	loaded time.Time
	ttl    time.Duration
}

func NewSubscribeSettingsRepo(pool *pgxpool.Pool) *SubscribeSettingsRepo {
	return &SubscribeSettingsRepo{pool: pool, ttl: 60 * time.Second}
}

// Load 读取并缓存；过期或首次调用走 DB。
func (r *SubscribeSettingsRepo) Load(ctx context.Context) (*SubscribeSettings, error) {
	r.mu.RLock()
	if r.cached != nil && time.Since(r.loaded) < r.ttl {
		s := *r.cached
		r.mu.RUnlock()
		return &s, nil
	}
	r.mu.RUnlock()
	return r.reload(ctx)
}

// Reload 强制刷新（由 TopicConfigChanged 事件触发，让设置变更 60s 内必生效）。
func (r *SubscribeSettingsRepo) Reload(ctx context.Context) (*SubscribeSettings, error) {
	return r.reload(ctx)
}

func (r *SubscribeSettingsRepo) reload(ctx context.Context) (*SubscribeSettings, error) {
	// DB 不可用时返回默认值，不阻断订阅
	if r.pool == nil {
		s := defaultSubscribeSettings()
		r.mu.Lock()
		r.cached = s
		r.loaded = time.Now()
		r.mu.Unlock()
		return s, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT setting_key, value_json #>> '{}' FROM system_settings WHERE setting_group = 'subscribe'`)
	if err != nil {
		s := defaultSubscribeSettings()
		r.mu.Lock()
		r.cached = s
		r.loaded = time.Now()
		r.mu.Unlock()
		return s, nil
	}
	defer rows.Close()

	s := defaultSubscribeSettings()
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		// value_json #>> '{}' 会把 jsonb 字符串值的引号剥掉，直接拿到裸值
		val = strings.TrimSpace(val)
		switch key {
		case "subscribe_path":
			if val != "" {
				// 规范化：去掉首尾空白与尾部斜杠，保留前导 /
				p := strings.TrimRight(val, "/")
				if p != "" && !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				s.SubscribePath = p
			}
		case "subscribe_domain":
			// 去掉可能的协议前缀，只保留 host
			d := strings.TrimSpace(val)
			d = strings.TrimPrefix(d, "https://")
			d = strings.TrimPrefix(d, "http://")
			d = strings.TrimRight(d, "/")
			s.SubscribeDomain = d
		case "subscribe_key":
			s.SubscribeKey = val
		case "show_info_to_server":
			s.ShowInfoToServer = parseBool(val, true)
		case "show_method":
			s.ShowMethod = parseInt(val, 1)
		case "is_rand_sub":
			s.IsRandSub = parseBool(val, false)
		case "rand_sub_start":
			s.RandSubStart = parseInt(val, 0)
		case "rand_sub_end":
			s.RandSubEnd = parseInt(val, 10)
		}
	}

	r.mu.Lock()
	r.cached = s
	r.loaded = time.Now()
	r.mu.Unlock()
	return s, nil
}

func defaultSubscribeSettings() *SubscribeSettings {
	return &SubscribeSettings{
		SubscribePath:    "/sub",
		SubscribeDomain:  "",
		SubscribeKey:     "",
		ShowInfoToServer: true,
		ShowMethod:       1,
		IsRandSub:        false,
		RandSubStart:     0,
		RandSubEnd:       10,
	}
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	}
	return def
}

func parseInt(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}
