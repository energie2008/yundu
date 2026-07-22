package pkg

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNonceReused      = errors.New("nonce already used")
	ErrTimestampExpired = errors.New("timestamp out of allowed window")
	ErrNonceCacheFull   = errors.New("nonce cache is full")
)

const (
	DefaultNonceTTL      = 5 * time.Minute
	DefaultMaxEntries    = 10000
	DefaultCleanupPeriod = 60 * time.Second
)

type nonceEntry struct {
	expiry time.Time
}

type NonceCache struct {
	mu       sync.Mutex
	cache    sync.Map
	ttl      time.Duration
	maxEntries int
	count    atomic.Int64
	stopCh   chan struct{}
	stopped  atomic.Bool
}

func NewNonceCache(ttl time.Duration, maxEntries int) *NonceCache {
	if ttl <= 0 {
		ttl = DefaultNonceTTL
	}
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	return &NonceCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		stopCh:     make(chan struct{}),
	}
}

func (nc *NonceCache) CheckAndStore(machineID string, nonce string, timestamp int64) error {
	if machineID == "" || nonce == "" {
		return fmt.Errorf("machineID and nonce are required")
	}

	now := time.Now().Unix()
	if timestamp < now-300 || timestamp > now+300 {
		return ErrTimestampExpired
	}

	key := fmt.Sprintf("%s:%s", machineID, nonce)

	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, loaded := nc.cache.Load(key); loaded {
		return ErrNonceReused
	}

	if nc.count.Load() >= int64(nc.maxEntries) {
		nc.evictExpired()
		if nc.count.Load() >= int64(nc.maxEntries) {
			return ErrNonceCacheFull
		}
	}

	entry := &nonceEntry{
		expiry: time.Now().Add(nc.ttl),
	}
	nc.cache.Store(key, entry)
	nc.count.Add(1)

	return nil
}

func (nc *NonceCache) evictExpired() {
	now := time.Now()
	var expired []string

	nc.cache.Range(func(key, value any) bool {
		entry, ok := value.(*nonceEntry)
		if ok && now.After(entry.expiry) {
			expired = append(expired, key.(string))
		}
		return true
	})

	for _, key := range expired {
		if _, loaded := nc.cache.LoadAndDelete(key); loaded {
			nc.count.Add(-1)
		}
	}
}

func (nc *NonceCache) cleanup() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.evictExpired()
}

func (nc *NonceCache) Start(ctx context.Context) {
	go nc.cleanupLoop(ctx)
}

func (nc *NonceCache) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(DefaultCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			nc.Stop()
			return
		case <-nc.stopCh:
			return
		case <-ticker.C:
			nc.cleanup()
		}
	}
}

func (nc *NonceCache) Stop() {
	if nc.stopped.CompareAndSwap(false, true) {
		close(nc.stopCh)
	}
}

func (nc *NonceCache) Exists(machineID, nonce string) bool {
	key := fmt.Sprintf("%s:%s", machineID, nonce)
	_, ok := nc.cache.Load(key)
	return ok
}

func (nc *NonceCache) Size() int64 {
	return nc.count.Load()
}
