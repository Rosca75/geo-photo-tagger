package main

// thumbnail_cache.go — In-memory LRU cache for generated thumbnails.
//
// Why this exists: decoding a thumbnail (especially HEIC, which runs a WASM
// libheif decoder) is expensive — ~10-30 ms per HEIC file. The UI re-requests
// the same thumbnail on every re-render, hover, and photo re-selection, so
// without a cache each of those re-decodes the file from disk. This cache keeps
// the finished base64 JPEG string so repeat views are effectively free.
//
// The cache is keyed on path + requested size + file mtime + file size. Because
// mtime and size are part of the key, a file that is modified on disk (for
// example after ApplyGPS rewrites its EXIF) automatically misses the stale
// entry and is decoded once more — no manual invalidation is required.
//
// Empty-string results (HEIC decode failures, unsupported formats) are cached
// too, so a file that cannot produce a thumbnail is not retried on every hover.
//
// The implementation is a classic mutex-guarded map + container/list LRU using
// only the standard library — no new dependency.

import (
	"container/list"
	"fmt"
	"sync"
)

// maxCacheEntries bounds the number of cached thumbnails. A 200 px JPEG base64
// string is on the order of ~15 KB, so 512 entries is a few MB at most — a safe
// ceiling for a desktop app while still covering a large folder's worth of
// repeat views.
const maxCacheEntries = 512

// cacheEntry is the value stored in each LRU list element. It keeps its own key
// so eviction (which starts from the list) can delete the matching map entry.
type cacheEntry struct {
	key   string
	value string // base64 JPEG (may be "" for a cached miss)
}

// thumbnailCacheType is a fixed-capacity, thread-safe LRU cache mapping a
// composite string key to a base64 JPEG thumbnail string.
type thumbnailCacheType struct {
	mu       sync.Mutex
	maxItems int
	// ll orders entries most-recently-used (front) to least (back).
	ll *list.List
	// items maps a key to its element in ll for O(1) lookup.
	items map[string]*list.Element
}

// newThumbnailCache builds an empty cache bounded to maxItems entries.
func newThumbnailCache(maxItems int) *thumbnailCacheType {
	return &thumbnailCacheType{
		maxItems: maxItems,
		ll:       list.New(),
		items:    make(map[string]*list.Element, maxItems),
	}
}

// get returns the cached value for key and whether it was present. A hit is
// promoted to most-recently-used so it survives eviction longer.
func (c *thumbnailCacheType) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*cacheEntry).value, true
}

// put inserts or updates the value for key, then evicts the least-recently-used
// entry if the cache is over capacity.
func (c *thumbnailCacheType) put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update-in-place if the key already exists.
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*cacheEntry).value = value
		return
	}

	// Insert as most-recently-used.
	el := c.ll.PushFront(&cacheEntry{key: key, value: value})
	c.items[key] = el

	// Evict from the back while over the capacity limit.
	for c.ll.Len() > c.maxItems {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}

// thumbCache is the process-wide singleton used by GenerateThumbnail.
var thumbCache = newThumbnailCache(maxCacheEntries)

// thumbCacheKey builds the composite cache key for a thumbnail request.
// Including mtime (as UnixNano) and size means any on-disk edit invalidates the
// prior entry naturally, without an explicit invalidation call.
func thumbCacheKey(path string, maxSize int, mtimeUnixNano int64, size int64) string {
	return fmt.Sprintf("%s|%d|%d|%d", path, maxSize, mtimeUnixNano, size)
}
