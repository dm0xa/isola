package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cacheEntry is a single in-memory value with an optional expiry.
type cacheEntry struct {
	val    string
	expire time.Time // zero time = no expiry
}

// MemCache is an in-memory key/value store with TTL support, used for
// session routing, control tokens and share metadata.
type MemCache struct {
	mu     sync.RWMutex
	data   map[string]*cacheEntry
	stopCh chan struct{}
}

// NewMemCache creates a ready-to-use in-memory cache.
func NewMemCache() *MemCache {
	c := &MemCache{
		data:   make(map[string]*cacheEntry),
		stopCh: make(chan struct{}),
	}
	go c.reapLoop()
	return c
}

func (c *MemCache) reapLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.mu.Lock()
			now := time.Now()
			for k, v := range c.data {
				if !v.expire.IsZero() && now.After(v.expire) {
					delete(c.data, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *MemCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := &cacheEntry{val: value}
	if ttl > 0 {
		e.expire = time.Now().Add(ttl)
	}
	c.data[key] = e
	return nil
}

func (c *MemCache) Get(_ context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok {
		return "", errKeyNotFound
	}
	if !e.expire.IsZero() && time.Now().After(e.expire) {
		return "", errKeyNotFound
	}
	return e.val, nil
}

func (c *MemCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.data, k)
	}
	return nil
}

func (c *MemCache) Incr(_ context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		e = &cacheEntry{val: "0"}
		c.data[key] = e
	}
	v, err := strconv.ParseInt(e.val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("memcache: key %q is not an integer: %w", key, err)
	}
	v++
	e.val = strconv.FormatInt(v, 10)
	return v, nil
}

func (c *MemCache) Decr(_ context.Context, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		e = &cacheEntry{val: "0"}
		c.data[key] = e
	}
	v, err := strconv.ParseInt(e.val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("memcache: key %q is not an integer: %w", key, err)
	}
	v--
	e.val = strconv.FormatInt(v, 10)
	return v, nil
}

func (c *MemCache) Expire(_ context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return errKeyNotFound
	}
	e.expire = time.Now().Add(ttl)
	return nil
}

func (c *MemCache) Persist(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return errKeyNotFound
	}
	e.expire = time.Time{}
	return nil
}

func (c *MemCache) Ping(_ context.Context) error {
	return nil
}

func (c *MemCache) Close() error {
	close(c.stopCh)
	return nil
}

func (c *MemCache) HSet(_ context.Context, key string, values ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.data[key]
	if e == nil || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		e = &cacheEntry{val: "{}"}
		c.data[key] = e
	}
	m := make(map[string]string)
	if err := json.Unmarshal([]byte(e.val), &m); err != nil {
		m = make(map[string]string)
	}
	for i := 0; i+1 < len(values); i += 2 {
		m[values[i]] = values[i+1]
	}
	b, _ := json.Marshal(m)
	e.val = string(b)
	return nil
}

func (c *MemCache) HGetAll(_ context.Context, key string) (map[string]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		return nil, nil
	}
	m := make(map[string]string)
	if err := json.Unmarshal([]byte(e.val), &m); err != nil {
		return nil, nil
	}
	return m, nil
}

func (c *MemCache) SAdd(_ context.Context, key string, members ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.data[key]
	if e == nil || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		e = &cacheEntry{val: ""}
		c.data[key] = e
	}
	seen := make(map[string]struct{})
	for _, m := range splitNonEmpty(e.val) {
		seen[m] = struct{}{}
	}
	for _, m := range members {
		seen[m] = struct{}{}
	}
	parts := make([]string, 0, len(seen))
	for m := range seen {
		parts = append(parts, m)
	}
	e.val = strings.Join(parts, ",")
	return nil
}

func (c *MemCache) SMembers(_ context.Context, key string) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[key]
	if !ok || (!e.expire.IsZero() && time.Now().After(e.expire)) {
		return nil, nil
	}
	return splitNonEmpty(e.val), nil
}

func (c *MemCache) SRem(_ context.Context, key string, members ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	for _, m := range splitNonEmpty(e.val) {
		seen[m] = struct{}{}
	}
	for _, m := range members {
		delete(seen, m)
	}
	parts := make([]string, 0, len(seen))
	for m := range seen {
		parts = append(parts, m)
	}
	e.val = strings.Join(parts, ",")
	return nil
}

// GetInt is a convenience helper that calls Get and parses to int.
func GetInt(cache *MemCache, ctx context.Context, key string) (int, error) {
	val, err := cache.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// errKeyNotFound mirrors redis.Nil for Get operations.
var errKeyNotFound = fmt.Errorf("memcache: key not found")

// IsKeyNotFound reports whether err indicates a missing key.
func IsKeyNotFound(err error) bool {
	return err == errKeyNotFound
}

// SnapshotEntries returns all unexpired cache entries for persistence.
func (c *MemCache) SnapshotEntries() map[string]persistedCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	out := make(map[string]persistedCacheEntry, len(c.data))
	for k, e := range c.data {
		if !e.expire.IsZero() && now.After(e.expire) {
			continue
		}
		out[k] = persistedCacheEntry{Value: e.val, Expire: e.expire}
	}
	return out
}

// RestoreEntry re-inserts a cache entry from a snapshot file.
func (c *MemCache) RestoreEntry(key, val string, expire time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = &cacheEntry{val: val, expire: expire}
}
