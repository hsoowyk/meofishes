package lyrics

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meofishes/internal/core"
)

// DefaultOrder is the provider priority: BetterLyrics, lrc.red, Unison,
// Paxsenix, Kugou, and LRCLIB as a line-synced fallback.
var DefaultOrder = []string{"betterlyrics", "lrcred", "unison", "paxsenix", "kugou", "lrclib"}

// Builtins returns every built-in provider bound to env.
func Builtins(env *Env) map[string]Provider {
	ps := []Provider{BetterLyrics{env}, LrcRed{env}, Unison{env}, Paxsenix{env}, Kugou{env}, LrcLib{env}}
	m := map[string]Provider{}
	for _, p := range ps {
		m[p.ID()] = p
	}
	return m
}

// Attempt records what each provider answered, for the UI and logs.
type Attempt struct {
	Provider string
	Err      error
	Sync     core.SyncType
	Took     time.Duration
}

// Chain asks providers in priority order. All providers are queried in
// parallel, but the result is chosen by priority: the first provider in the
// order that has word-synced lyrics wins; if nobody has word sync, the first
// with line sync; then the first with plain text.
type Chain struct {
	Providers []Provider
	Timeout   time.Duration
	CacheDir  string // empty disables the disk cache
	SkipRead  bool   // ignore cached results (but still refresh the cache)
}

func (c *Chain) cachePath(q Query) string {
	if c.CacheDir == "" {
		return ""
	}
	ids := make([]string, len(c.Providers))
	for i, p := range c.Providers {
		ids[i] = p.ID()
	}
	h := sha1.Sum([]byte(strings.ToLower(fmt.Sprintf("%s|%s|%s|%d|%s", q.Title, q.Artist, q.Album, q.DurationMs/2000, strings.Join(ids, ",")))))
	return filepath.Join(c.CacheDir, hex.EncodeToString(h[:])+".json")
}

// Find returns the best lyrics and the per-provider report.
func (c *Chain) Find(ctx context.Context, q Query) (*core.Lyrics, []Attempt, error) {
	if path := c.cachePath(q); path != "" && !c.SkipRead {
		if data, err := os.ReadFile(path); err == nil {
			var l core.Lyrics
			if json.Unmarshal(data, &l) == nil && len(l.Lines) > 0 {
				return &l, []Attempt{{Provider: l.Provider + " (cache)", Sync: l.Sync}}, nil
			}
		}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type res struct {
		l   *core.Lyrics
		att Attempt
	}
	results := make([]chan res, len(c.Providers))
	for i, p := range c.Providers {
		ch := make(chan res, 1)
		results[i] = ch
		go func(p Provider) {
			start := time.Now()
			l, err := p.Fetch(ctx, q)
			if err == nil && IsPlaceholder(l) { // empty, or "instrumental, enjoy"
				err = ErrNotFound
			}
			a := Attempt{Provider: p.ID(), Err: err, Took: time.Since(start)}
			if err == nil {
				l.Provider = p.Name()
				a.Sync = l.Sync
			}
			ch <- res{l, a}
		}(p)
	}

	var attempts []Attempt
	got := make([]*core.Lyrics, len(c.Providers))
	var best *core.Lyrics
	for i := range c.Providers {
		r := <-results[i]
		attempts = append(attempts, r.att)
		got[i] = r.l
		if r.l != nil && r.l.Sync == core.SyncWord {
			best = r.l // highest-priority word sync: stop waiting for lower priorities
			cancel()
			break
		}
	}
	if best == nil {
		for _, want := range []core.SyncType{core.SyncLine, core.SyncNone} {
			for _, l := range got {
				if l != nil && l.Sync == want {
					best = l
					break
				}
			}
			if best != nil {
				break
			}
		}
	}
	if best == nil {
		return nil, attempts, ErrNotFound
	}
	if path := c.cachePath(q); path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if data, err := json.Marshal(best); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
	}
	return best, attempts, nil
}

// Describe turns attempts into a readable one-line report.
func Describe(as []Attempt) string {
	var parts []string
	for _, a := range as {
		switch {
		case a.Err == nil:
			parts = append(parts, fmt.Sprintf("%s: %s", a.Provider, a.Sync))
		case errors.Is(a.Err, ErrNotConfigured):
			parts = append(parts, a.Provider+": not configured")
		case errors.Is(a.Err, ErrNotFound):
			parts = append(parts, a.Provider+": not found")
		case errors.Is(a.Err, context.Canceled):
			parts = append(parts, a.Provider+": skipped")
		default:
			parts = append(parts, a.Provider+": "+a.Err.Error())
		}
	}
	return strings.Join(parts, " · ")
}
