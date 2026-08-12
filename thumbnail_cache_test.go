package main

// thumbnail_cache_test.go — Unit tests for the in-memory LRU thumbnail cache.

import "testing"

// TestThumbnailCacheHit verifies a stored value is returned on lookup.
func TestThumbnailCacheHit(t *testing.T) {
	c := newThumbnailCache(4)
	c.put("a", "va")

	if v, ok := c.get("a"); !ok || v != "va" {
		t.Fatalf("expected hit va, got %q ok=%v", v, ok)
	}
	if _, ok := c.get("missing"); ok {
		t.Fatalf("expected miss for absent key")
	}
}

// TestThumbnailCacheEmptyValue confirms empty-string (decode-miss) values are
// cached and distinguishable from an absent key.
func TestThumbnailCacheEmptyValue(t *testing.T) {
	c := newThumbnailCache(4)
	c.put("heic", "")

	if v, ok := c.get("heic"); !ok || v != "" {
		t.Fatalf("expected cached empty value, got %q ok=%v", v, ok)
	}
}

// TestThumbnailCacheEviction checks that exceeding capacity evicts the
// least-recently-used entry, and that a get() promotes an entry so it survives.
func TestThumbnailCacheEviction(t *testing.T) {
	c := newThumbnailCache(2)
	c.put("a", "1")
	c.put("b", "2")

	// Access "a" so it becomes most-recently-used; "b" is now the LRU.
	if _, ok := c.get("a"); !ok {
		t.Fatalf("expected a to be present")
	}

	// Inserting "c" should evict "b" (least recently used), not "a".
	c.put("c", "3")

	if _, ok := c.get("b"); ok {
		t.Fatalf("expected b to be evicted")
	}
	if _, ok := c.get("a"); !ok {
		t.Fatalf("expected a to survive eviction")
	}
	if _, ok := c.get("c"); !ok {
		t.Fatalf("expected c to be present")
	}
}

// TestThumbCacheKeyDistinct ensures the composite key changes when any
// component (size, mtime, file size) changes.
func TestThumbCacheKeyDistinct(t *testing.T) {
	base := thumbCacheKey("/p.heic", 200, 100, 500)
	if base == thumbCacheKey("/p.heic", 64, 100, 500) {
		t.Fatalf("key should differ on maxSize")
	}
	if base == thumbCacheKey("/p.heic", 200, 101, 500) {
		t.Fatalf("key should differ on mtime")
	}
	if base == thumbCacheKey("/p.heic", 200, 100, 501) {
		t.Fatalf("key should differ on file size")
	}
}
