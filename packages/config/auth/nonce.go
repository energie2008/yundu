package auth

import (
	"context"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type NonceStore interface {
	CheckAndStore(ctx context.Context, nonce string, ttl time.Duration) (bool, error)
}

type RedisNonceStore struct {
	client *goredis.Client
}

func NewRedisNonceStore(client *goredis.Client) *RedisNonceStore {
	return &RedisNonceStore{client: client}
}

func (s *RedisNonceStore) CheckAndStore(ctx context.Context, nonce string, ttl time.Duration) (bool, error) {
	key := "nonce:" + nonce
	ok, err := s.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

type MemoryNonceStore struct {
	mu     sync.RWMutex
	nonces map[string]time.Time
	stop   chan struct{}
	stopOnce sync.Once
}

func NewMemoryNonceStore() *MemoryNonceStore {
	store := &MemoryNonceStore{
		nonces: make(map[string]time.Time),
		stop:   make(chan struct{}),
	}
	go store.cleanupLoop()
	return store
}

func (s *MemoryNonceStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stop:
			return
		}
	}
}

func (s *MemoryNonceStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, k)
		}
	}
}

// Stop 幂等地停止后台清理 goroutine。重复调用不会 panic。
// 使用 sync.Once 保证 close(stop) 只执行一次, 避免 "close of closed channel"。
func (s *MemoryNonceStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}

func (s *MemoryNonceStore) CheckAndStore(ctx context.Context, nonce string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if exp, exists := s.nonces[nonce]; exists && now.Before(exp) {
		return false, nil
	}
	s.nonces[nonce] = now.Add(ttl)
	return true, nil
}
