package casbin

import (
	"container/list"
	"sync"
	"time"
)

type Authorizer interface {
	Enforce(...interface{}) (bool, error)
}

type AuthorizerLoader func() (Authorizer, error)

type cacheEntry struct {
	key        string
	authorizer Authorizer
	expiresAt  time.Time
}

type enforcerLoad struct {
	done       chan struct{}
	authorizer Authorizer
	err        error
}

type Cache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	entries  map[string]*list.Element
	lru      *list.List
	loading  map[string]*enforcerLoad
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

func NewCache(capacity int, ttl time.Duration, now func() time.Time) *Cache {
	if capacity < 1 {
		capacity = 64
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &Cache{
		capacity: capacity, ttl: ttl, now: now,
		entries: make(map[string]*list.Element), lru: list.New(), loading: make(map[string]*enforcerLoad),
	}
}

var defaultCache = NewCache(64, 5*time.Minute, time.Now)

func Get(key string, loader AuthorizerLoader) (Authorizer, error) {
	return defaultCache.Get(key, loader)
}

func Invalidate(key string) {
	defaultCache.Invalidate(key)
}

func Reset() {
	defaultCache.Reset()
}

func Snapshot() CasbinEnforcerCacheMetrics {
	return defaultCache.Metrics()
}

func (c *Cache) Get(key string, loader AuthorizerLoader) (Authorizer, error) {
	c.mu.Lock()
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*cacheEntry)
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
	inFlight := &enforcerLoad{done: make(chan struct{})}
	c.loading[key] = inFlight
	c.mu.Unlock()

	inFlight.authorizer, inFlight.err = loader()

	c.mu.Lock()
	delete(c.loading, key)
	if inFlight.err == nil && inFlight.authorizer != nil {
		c.reloads++
		entry := &cacheEntry{key: key, authorizer: inFlight.authorizer, expiresAt: c.now().Add(c.ttl)}
		c.entries[key] = c.lru.PushFront(entry)
		for c.lru.Len() > c.capacity {
			c.remove(c.lru.Back())
		}
	}
	close(inFlight.done)
	c.mu.Unlock()
	return inFlight.authorizer, inFlight.err
}

func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[key]; element != nil {
		c.remove(element)
	}
}

func (c *Cache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.lru.Init()
	c.loading = make(map[string]*enforcerLoad)
	c.hits = 0
	c.misses = 0
	c.reloads = 0
}

func (c *Cache) Metrics() CasbinEnforcerCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CasbinEnforcerCacheMetrics{Entries: c.lru.Len(), Hits: c.hits, Misses: c.misses, Reloads: c.reloads}
}

func (c *Cache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry)
	delete(c.entries, entry.key)
	c.lru.Remove(element)
}
