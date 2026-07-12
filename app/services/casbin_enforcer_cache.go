package services

import (
	"container/list"
	"sync"
	"time"
)

type casbinAuthorizer interface {
	Enforce(...interface{}) (bool, error)
}

type casbinAuthorizerLoader func() (casbinAuthorizer, error)

type casbinEnforcerCacheEntry struct {
	key        string
	authorizer casbinAuthorizer
	expiresAt  time.Time
}

type casbinEnforcerLoad struct {
	done       chan struct{}
	authorizer casbinAuthorizer
	err        error
}

type casbinEnforcerCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	entries  map[string]*list.Element
	lru      *list.List
	loading  map[string]*casbinEnforcerLoad
	hits     uint64
	misses   uint64
	reloads  uint64
}

type CasbinEnforcerCacheMetrics struct {
	Entries int
	Hits    uint64
	Misses  uint64
	Reloads uint64
}

func newCasbinEnforcerCache(capacity int, ttl time.Duration, now func() time.Time) *casbinEnforcerCache {
	if capacity < 1 {
		capacity = 64
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &casbinEnforcerCache{
		capacity: capacity, ttl: ttl, now: now,
		entries: make(map[string]*list.Element), lru: list.New(), loading: make(map[string]*casbinEnforcerLoad),
	}
}

var defaultCasbinEnforcerCache = newCasbinEnforcerCache(64, 5*time.Minute, time.Now)

func (c *casbinEnforcerCache) Get(key string, loader casbinAuthorizerLoader) (casbinAuthorizer, error) {
	c.mu.Lock()
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*casbinEnforcerCacheEntry)
		if c.now().Before(entry.expiresAt) {
			c.hits++
			c.lru.MoveToFront(element)
			c.mu.Unlock()
			return entry.authorizer, nil
		}
		c.remove(element)
	}
	if inFlight, ok := c.loading[key]; ok {
		c.hits++
		c.mu.Unlock()
		<-inFlight.done
		return inFlight.authorizer, inFlight.err
	}
	c.misses++
	inFlight := &casbinEnforcerLoad{done: make(chan struct{})}
	c.loading[key] = inFlight
	c.mu.Unlock()

	inFlight.authorizer, inFlight.err = loader()

	c.mu.Lock()
	delete(c.loading, key)
	if inFlight.err == nil && inFlight.authorizer != nil {
		c.reloads++
		entry := &casbinEnforcerCacheEntry{key: key, authorizer: inFlight.authorizer, expiresAt: c.now().Add(c.ttl)}
		c.entries[key] = c.lru.PushFront(entry)
		for c.lru.Len() > c.capacity {
			c.remove(c.lru.Back())
		}
	}
	close(inFlight.done)
	c.mu.Unlock()
	return inFlight.authorizer, inFlight.err
}

func (c *casbinEnforcerCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[key]; element != nil {
		c.remove(element)
	}
}

func (c *casbinEnforcerCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.lru.Init()
	c.loading = make(map[string]*casbinEnforcerLoad)
	c.hits = 0
	c.misses = 0
	c.reloads = 0
}

func (c *casbinEnforcerCache) Metrics() CasbinEnforcerCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CasbinEnforcerCacheMetrics{Entries: c.lru.Len(), Hits: c.hits, Misses: c.misses, Reloads: c.reloads}
}

func (c *casbinEnforcerCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*casbinEnforcerCacheEntry)
	delete(c.entries, entry.key)
	c.lru.Remove(element)
}

func InvalidateCasbinEnforcer(connection string) {
	defaultCasbinEnforcerCache.Invalidate(connection)
}

func ResetCasbinEnforcerCacheForTest() {
	defaultCasbinEnforcerCache.Reset()
}

func CasbinEnforcerCacheSnapshot() CasbinEnforcerCacheMetrics {
	return defaultCasbinEnforcerCache.Metrics()
}
