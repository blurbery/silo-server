// Package cache provides a generic in-process TTL cache backed by a
// sync.RWMutex-protected map with automatic background expiry sweeping.
package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// Default sweep interval for the background expiry goroutine.
const defaultSweepInterval = 30 * time.Second

// ---- TTLCache -----------------------------------------------------------

// entry holds a cached value together with its expiration timestamp.
type entry[V comparable] struct {
	value     V
	expiresAt time.Time
	// order is the entry's place in TTLCache.order; nil in an unbounded cache.
	order *list.Element
}

// TTLCache is a generic, concurrency-safe, in-process cache with per-key
// time-to-live semantics. A background goroutine periodically sweeps out
// expired entries. Call Close to stop the sweeper when the cache is no
// longer needed.
//
// A cache from NewTTLCache has no entry limit, so it only suits a key space
// that is small or that its callers bound. NewBoundedTTLCache adds a limit.
type TTLCache[V comparable] struct {
	mu      sync.RWMutex
	entries map[string]entry[V]
	// limit and order are set only on a bounded cache. order lists the keys,
	// most recently used first.
	limit int
	order *list.List
	stop  chan struct{}
	once  sync.Once // ensures Close is idempotent
}

// NewTTLCache creates a new TTLCache and starts the background sweeper
// with the default 30-second interval.
func NewTTLCache[V comparable]() *TTLCache[V] {
	return newTTLCacheWithSweepInterval[V](defaultSweepInterval)
}

// NewBoundedTTLCache creates a TTLCache that holds at most maxEntries
// entries. Storing a new key in a full cache evicts the least recently used
// entry, whether or not it has expired. A Get on a bounded cache takes the
// write lock to record the use. maxEntries must be positive.
func NewBoundedTTLCache[V comparable](maxEntries int) *TTLCache[V] {
	if maxEntries <= 0 {
		panic("cache: NewBoundedTTLCache needs a positive maxEntries")
	}
	c := newTTLCacheWithSweepInterval[V](defaultSweepInterval)
	c.limit = maxEntries
	c.order = list.New()
	return c
}

// newTTLCacheWithSweepInterval creates a TTLCache with a caller-specified
// sweep interval. This is intentionally unexported; tests use it to speed
// up sweeper assertions.
func newTTLCacheWithSweepInterval[V comparable](interval time.Duration) *TTLCache[V] {
	c := &TTLCache[V]{
		entries: make(map[string]entry[V]),
		stop:    make(chan struct{}),
	}
	go c.sweepLoop(interval)
	return c
}

// Get retrieves the value for key. It returns the value and true on a hit,
// or the zero value of V and false on a miss (including expired entries).
func (c *TTLCache[V]) Get(key string) (V, bool) {
	var e entry[V]
	var exists bool
	if c.order == nil {
		c.mu.RLock()
		e, exists = c.entries[key]
		c.mu.RUnlock()
	} else {
		c.mu.Lock()
		if e, exists = c.entries[key]; exists {
			c.order.MoveToFront(e.order)
		}
		c.mu.Unlock()
	}

	if !exists || time.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores value under key with the given TTL. If the key already exists
// it is overwritten and the expiry is reset.
func (c *TTLCache[V]) Set(key string, value V, ttl time.Duration) {
	e := entry[V]{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.order != nil {
		e.order = c.useLocked(key)
	}
	c.entries[key] = e
}

// useLocked returns key's place in the use order of a bounded cache, moved to
// the front, making room for a new key by evicting the least recently used
// one. The caller holds the write lock.
func (c *TTLCache[V]) useLocked(key string) *list.Element {
	if old, exists := c.entries[key]; exists {
		c.order.MoveToFront(old.order)
		return old.order
	}
	if len(c.entries) >= c.limit {
		oldest, _ := c.order.Back().Value.(string)
		c.deleteLocked(oldest)
	}
	return c.order.PushFront(key)
}

// deleteLocked removes key and its place in the use order. The caller holds
// the write lock.
func (c *TTLCache[V]) deleteLocked(key string) {
	e, exists := c.entries[key]
	if !exists {
		return
	}
	if e.order != nil {
		c.order.Remove(e.order)
	}
	delete(c.entries, key)
}

// Invalidate removes a single key from the cache immediately.
func (c *TTLCache[V]) Invalidate(key string) {
	c.mu.Lock()
	c.deleteLocked(key)
	c.mu.Unlock()
}

// InvalidatePrefix removes every key that starts with prefix.
func (c *TTLCache[V]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			c.deleteLocked(k)
		}
	}
	c.mu.Unlock()
}

// Close stops the background sweeper goroutine. It is safe to call Close
// multiple times; only the first call has any effect.
func (c *TTLCache[V]) Close() {
	c.once.Do(func() {
		close(c.stop)
	})
}

// sweepLoop runs in its own goroutine and periodically removes expired
// entries from the map.
func (c *TTLCache[V]) sweepLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.sweep()
		}
	}
}

// sweep deletes all entries whose expiry time has passed.
func (c *TTLCache[V]) sweep() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			c.deleteLocked(k)
		}
	}
	c.mu.Unlock()
}
