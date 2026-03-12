package ragfs

import (
	"sync"
	"time"
)

// cachedMeta holds a cached directory listing or stat result.
type cachedMeta struct {
	entries   []Entry
	entry     *Entry
	expiresAt time.Time
}

// cachedContent holds cached file content.
type cachedContent struct {
	data      []byte
	expiresAt time.Time
	size      int
}

// Cache provides in-memory caching of metadata and file content with TTL
// expiration and LRU eviction for content.
type Cache struct {
	metaTTL    time.Duration
	contentTTL time.Duration
	maxSize    int64

	mu      sync.RWMutex
	meta    map[string]*cachedMeta
	content map[string]*cachedContent
	// currentSize tracks the total byte size of cached content.
	currentSize int64
	// accessOrder tracks LRU order for content eviction.
	accessOrder []string
}

// CacheOption configures the Cache.
type CacheOption func(*Cache)

// WithMetaTTL sets the metadata cache TTL.
func WithMetaTTL(d time.Duration) CacheOption {
	return func(c *Cache) { c.metaTTL = d }
}

// WithContentTTL sets the content cache TTL.
func WithContentTTL(d time.Duration) CacheOption {
	return func(c *Cache) { c.contentTTL = d }
}

// WithMaxSize sets the maximum content cache size in bytes.
func WithMaxSize(n int64) CacheOption {
	return func(c *Cache) { c.maxSize = n }
}

// NewCache creates a cache with the given options. Defaults: 30s metadata TTL,
// 60s content TTL, 100MB max size.
func NewCache(opts ...CacheOption) *Cache {
	c := &Cache{
		metaTTL:    30 * time.Second,
		contentTTL: 60 * time.Second,
		maxSize:    100 * 1024 * 1024, // 100MB
		meta:       make(map[string]*cachedMeta),
		content:    make(map[string]*cachedContent),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GetMeta returns a cached metadata entry for the given path, or nil if not
// cached or expired.
func (c *Cache) GetMeta(path string) *Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.meta[path]
	if !ok || time.Now().After(m.expiresAt) {
		return nil
	}
	return m.entry
}

// GetMetaList returns a cached directory listing for the given path, or nil if
// not cached or expired.
func (c *Cache) GetMetaList(path string) []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.meta[path]
	if !ok || time.Now().After(m.expiresAt) {
		return nil
	}
	return m.entries
}

// PutMeta caches a stat result for the given path.
func (c *Cache) PutMeta(path string, e *Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.meta[path] = &cachedMeta{
		entry:     e,
		expiresAt: time.Now().Add(c.metaTTL),
	}
}

// PutMetaList caches a directory listing for the given path.
func (c *Cache) PutMetaList(path string, entries []Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.meta[path] = &cachedMeta{
		entries:   entries,
		expiresAt: time.Now().Add(c.metaTTL),
	}
}

// GetContent returns cached file content for the given path, or nil if not
// cached or expired.
func (c *Cache) GetContent(path string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cc, ok := c.content[path]
	if !ok || time.Now().After(cc.expiresAt) {
		return nil
	}
	c.touchLRU(path)
	return cc.data
}

// PutContent caches file content for the given path, evicting LRU entries if
// the cache exceeds its maximum size.
func (c *Cache) PutContent(path string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove existing entry size if updating.
	if existing, ok := c.content[path]; ok {
		c.currentSize -= int64(existing.size)
		c.removeLRU(path)
	}

	// Evict LRU entries until we have room.
	for c.currentSize+int64(len(data)) > c.maxSize && len(c.accessOrder) > 0 {
		c.evictOldest()
	}

	c.content[path] = &cachedContent{
		data:      data,
		expiresAt: time.Now().Add(c.contentTTL),
		size:      len(data),
	}
	c.currentSize += int64(len(data))
	c.accessOrder = append(c.accessOrder, path)
}

// Invalidate removes all cached data (metadata and content) for the given path.
func (c *Cache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.meta, path)
	if cc, ok := c.content[path]; ok {
		c.currentSize -= int64(cc.size)
		delete(c.content, path)
		c.removeLRU(path)
	}
}

// InvalidatePrefix removes all cached data for paths that have the given prefix.
// Useful when a directory is modified.
func (c *Cache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.meta {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.meta, k)
		}
	}
	for k, cc := range c.content {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			c.currentSize -= int64(cc.size)
			delete(c.content, k)
			c.removeLRU(k)
		}
	}
}

func (c *Cache) touchLRU(path string) {
	c.removeLRU(path)
	c.accessOrder = append(c.accessOrder, path)
}

func (c *Cache) removeLRU(path string) {
	for i, p := range c.accessOrder {
		if p == path {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			return
		}
	}
}

func (c *Cache) evictOldest() {
	if len(c.accessOrder) == 0 {
		return
	}
	oldest := c.accessOrder[0]
	c.accessOrder = c.accessOrder[1:]
	if cc, ok := c.content[oldest]; ok {
		c.currentSize -= int64(cc.size)
		delete(c.content, oldest)
	}
}
