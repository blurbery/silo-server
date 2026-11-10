package cache

import (
	"testing"
	"time"
)

func TestBoundedTTLCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewBoundedTTLCache[int](3)
	defer c.Close()

	c.Set("a", 1, time.Hour)
	c.Set("b", 2, time.Hour)
	c.Set("c", 3, time.Hour)
	// A read and an overwrite both count as a use, so "b" is now the oldest.
	if _, ok := c.Get("a"); !ok {
		t.Fatal(`Get("a") missed before the cache was full`)
	}
	c.Set("c", 30, time.Hour)
	c.Set("d", 4, time.Hour)

	if _, ok := c.Get("b"); ok {
		t.Fatal(`least recently used "b" was not evicted`)
	}
	for key, want := range map[string]int{"a": 1, "c": 30, "d": 4} {
		if got, ok := c.Get(key); !ok || got != want {
			t.Fatalf("Get(%q) = (%d, %v), want %d", key, got, ok, want)
		}
	}

	// Entries removed by expiry or invalidation must leave the use order too,
	// or a later eviction would pick a key that is already gone and let the
	// cache grow past its limit.
	c.Set("d", 4, -time.Second)
	c.sweep()
	c.Invalidate("a")
	c.InvalidatePrefix("c")
	if got := c.order.Len(); got != 0 {
		t.Fatalf("use order holds %d keys after every entry was removed, want 0", got)
	}
	for _, key := range []string{"e", "f", "g", "h"} {
		c.Set(key, 0, time.Hour)
	}
	if got := len(c.entries); got != 3 {
		t.Fatalf("cache holds %d entries, want its limit of 3", got)
	}
	if _, ok := c.Get("e"); ok {
		t.Fatal(`oldest entry "e" was not evicted after the cache refilled`)
	}
}
