package spok

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// cacheEntry is one cached spectrogram: the PNG bytes plus the X-Spok-Audio label.
type cacheEntry struct {
	key   string
	png   []byte
	label string
}

// lruCache is a capacity-bounded LRU keyed by a content+options hash.
// A nil *lruCache is a no-op (caching disabled).
type lruCache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

func newLRU(capacity int) *lruCache {
	if capacity <= 0 {
		return nil
	}
	return &lruCache{
		cap:   capacity,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
	}
}

func (c *lruCache) get(key string) (png []byte, label string, ok bool) {
	if c == nil {
		return nil, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ele, hit := c.items[key]
	if !hit {
		return nil, "", false
	}
	c.ll.MoveToFront(ele)
	e := ele.Value.(*cacheEntry)
	return e.png, e.label, true
}

func (c *lruCache) put(key string, png []byte, label string) {
	if c == nil || key == "" {
		return
	}
	// Own a copy so callers can reuse their buffers.
	stored := make([]byte, len(png))
	copy(stored, png)

	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, hit := c.items[key]; hit {
		c.ll.MoveToFront(ele)
		e := ele.Value.(*cacheEntry)
		e.png = stored
		e.label = label
		return
	}
	ele := c.ll.PushFront(&cacheEntry{key: key, png: stored, label: label})
	c.items[key] = ele
	for c.ll.Len() > c.cap {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.items, back.Value.(*cacheEntry).key)
	}
}

func (c *lruCache) len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// makeCacheKey hashes the SHA-256 of the input bytes together with the resolved
// RenderOptions so identical audio rendered under different settings never collide.
func makeCacheKey(contentSum []byte, opts RenderOptions) string {
	h := sha256.New()
	h.Write(contentSum)
	fmt.Fprintf(h, "\n%d|%d|%s|%s|%s|%s|%g|%t|%t",
		opts.Width, opts.Height, opts.Color, opts.Scale, opts.Channel,
		opts.WinFunc, opts.Gain, opts.Legend, opts.Overlay)
	return hex.EncodeToString(h.Sum(nil))
}
