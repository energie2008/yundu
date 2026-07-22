package cache

import (
	"fmt"
	"sync"
	"time"
)

type CachedContent struct {
	Content     string
	ContentType string
	UserInfo    string
	ExpiresAt   time.Time
}

type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]*CachedContent
	ttl   time.Duration
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	c := &MemoryCache{
		items: make(map[string]*CachedContent),
		ttl:   ttl,
	}
	go c.cleanup()
	return c
}

func (c *MemoryCache) cacheKey(token, clientType string) string {
	return fmt.Sprintf("%s:%s", token, clientType)
}

func (c *MemoryCache) Get(token, clientType string) (*CachedContent, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.cacheKey(token, clientType)
	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(item.ExpiresAt) {
		return nil, false
	}
	return item, true
}

func (c *MemoryCache) GetStale(token, clientType string) (*CachedContent, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.cacheKey(token, clientType)
	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	return item, true
}

func (c *MemoryCache) Set(token, clientType, content, contentType, userInfo string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.cacheKey(token, clientType)
	c.items[key] = &CachedContent{
		Content:     content,
		ContentType: contentType,
		UserInfo:    userInfo,
		ExpiresAt:   time.Now().Add(c.ttl),
	}
}

func (c *MemoryCache) Invalidate(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := token + ":"
	for k := range c.items {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.items, k)
		}
	}
}

func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*CachedContent)
}

func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		staleThreshold := now.Add(-5 * time.Minute)
		for k, v := range c.items {
			if v.ExpiresAt.Before(staleThreshold) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
