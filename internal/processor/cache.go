package processor

import (
	"context"
	"sync"
	"time"

	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// ConfigCache provides an in-memory TTL cache for server configurations.
type ConfigCache struct {
	mu     sync.RWMutex
	items  map[string]cacheItem
	ttl    time.Duration
	storer Storer
}

type cacheItem struct {
	config    *store.ServerConfig
	expiresAt time.Time
}

func NewConfigCache(storer Storer, ttl time.Duration) *ConfigCache {
	return &ConfigCache{
		items:  make(map[string]cacheItem),
		ttl:    ttl,
		storer: storer,
	}
}

func (c *ConfigCache) GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error) {
	c.mu.RLock()
	item, ok := c.items[serverID]
	c.mu.RUnlock()

	if ok && time.Now().Before(item.expiresAt) {
		return item.config, nil
	}

	// Cache miss or expired
	cfg, err := c.storer.GetServerConfig(ctx, serverID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.items[serverID] = cacheItem{
		config:    cfg,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return cfg, nil
}
