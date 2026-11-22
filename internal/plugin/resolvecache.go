package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// ResolveCache remembers slow answers: the direct stream URL yt-dlp found
// for a page (until the URL expires) and which page a song was matched to
// (for weeks). It is saved to a file so the app starts warm.
type ResolveCache struct {
	mu    sync.Mutex
	path  string
	Items map[string]cacheItem `json:"items"`
	dirty bool
}

type cacheItem struct {
	Value string    `json:"v"`
	Until time.Time `json:"until"`
}

// Cache is the app's resolve cache (nil = no caching).
var Cache *ResolveCache

// OpenResolveCache loads the cache file (missing = empty).
func OpenResolveCache(path string) *ResolveCache {
	c := &ResolveCache{path: path, Items: map[string]cacheItem{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, c)
	}
	if c.Items == nil {
		c.Items = map[string]cacheItem{}
	}
	now := time.Now()
	for k, it := range c.Items {
		if now.After(it.Until) {
			delete(c.Items, k)
		}
	}
	return c
}

// Get returns a value that has not expired.
func (c *ResolveCache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.Items[key]
	if !ok || time.Now().After(it.Until) {
		return "", false
	}
	return it.Value, true
}

// Put stores a value until a time and saves the file soon.
func (c *ResolveCache) Put(key, value string, until time.Time) {
	if c == nil || value == "" || !until.After(time.Now()) {
		return
	}
	c.mu.Lock()
	c.Items[key] = cacheItem{Value: value, Until: until}
	if len(c.Items) > 5000 { // drop the ones that expire first
		var oldest string
		for k, it := range c.Items {
			if oldest == "" || it.Until.Before(c.Items[oldest].Until) {
				oldest = k
			}
		}
		delete(c.Items, oldest)
	}
	first := !c.dirty
	c.dirty = true
	c.mu.Unlock()
	if first {
		time.AfterFunc(2*time.Second, c.save)
	}
}

// Drop forgets a key (a cached URL that stopped working).
func (c *ResolveCache) Drop(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.Items, key)
	c.mu.Unlock()
}

func (c *ResolveCache) save() {
	c.mu.Lock()
	c.dirty = false
	data, err := json.Marshal(c)
	c.mu.Unlock()
	if err != nil || c.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, c.path)
	}
}

// urlExpiry reads the expiry of a signed stream URL (YouTube's "expire"
// parameter), with a safety margin; 0 = unknown.
func urlExpiry(u string) time.Time {
	pu, err := url.Parse(u)
	if err != nil {
		return time.Time{}
	}
	if s := pu.Query().Get("expire"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(n, 0).Add(-20 * time.Minute)
		}
	}
	return time.Time{}
}

// streamAlive checks a cached stream URL with a 1-byte request.
func streamAlive(ctx context.Context, u string) bool {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

// flights runs one call per key at a time; others wait for its answer.
type flights struct {
	mu sync.Mutex
	m  map[string]*flight
}

type flight struct {
	done chan struct{}
	v    string
	err  error
}

var inflight = &flights{m: map[string]*flight{}}

func (f *flights) do(key string, fn func() (string, error)) (string, error) {
	f.mu.Lock()
	if fl, ok := f.m[key]; ok {
		f.mu.Unlock()
		<-fl.done
		return fl.v, fl.err
	}
	fl := &flight{done: make(chan struct{})}
	f.m[key] = fl
	f.mu.Unlock()
	fl.v, fl.err = fn()
	f.mu.Lock()
	delete(f.m, key)
	f.mu.Unlock()
	close(fl.done)
	return fl.v, fl.err
}
