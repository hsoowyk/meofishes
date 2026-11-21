// Package netlog records the HTTP requests the app makes (developer mode).
// Install wraps http.DefaultTransport, so every client that uses the default
// transport (plugins, lyrics, covers) is recorded.
package netlog

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Entry is one request.
type Entry struct {
	At       time.Time
	Method   string
	URL      string
	Status   int
	Duration time.Duration
	Bytes    int64
	Err      string
	UA       string
}

var (
	mu      sync.Mutex
	ring    []Entry
	max     = 400
	enabled atomic.Bool
	total   atomic.Int64
)

// Enable turns recording on or off.
func Enable(on bool) { enabled.Store(on) }

// Enabled reports whether recording is on.
func Enabled() bool { return enabled.Load() }

// Total is the number of requests since start (recorded or not).
func Total() int64 { return total.Load() }

type transport struct{ base http.RoundTripper }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	total.Add(1)
	if !enabled.Load() {
		return t.base.RoundTrip(r)
	}
	t0 := time.Now()
	resp, err := t.base.RoundTrip(r)
	e := Entry{At: t0, Method: r.Method, URL: r.URL.String(), Duration: time.Since(t0), UA: r.Header.Get("User-Agent")}
	if err != nil {
		e.Err = err.Error()
	} else {
		e.Status = resp.StatusCode
		e.Bytes = resp.ContentLength
	}
	mu.Lock()
	ring = append(ring, e)
	if len(ring) > max {
		ring = ring[len(ring)-max:]
	}
	mu.Unlock()
	return resp, err
}

var once sync.Once

// Install wraps http.DefaultTransport (call once at start).
func Install() {
	once.Do(func() { http.DefaultTransport = transport{base: http.DefaultTransport} })
}

// Wrap returns a transport that records through this package.
func Wrap(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transport{base: base}
}

// Entries returns the recorded requests, newest last.
func Entries() []Entry {
	mu.Lock()
	defer mu.Unlock()
	return append([]Entry(nil), ring...)
}

// Clear forgets the recorded requests.
func Clear() {
	mu.Lock()
	ring = nil
	mu.Unlock()
}
