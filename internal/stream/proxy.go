// Package stream is a local HTTP proxy that downloads a track in chunks,
// with several parallel range requests, and serves it to the player.
//
//   - the player starts almost immediately: the first chunks of the NEXT track
//     are downloaded while the current one plays (Prefetch);
//   - the rest of the track keeps downloading in the background, so seeking
//     inside the downloaded part is instant;
//   - sources that do not support ranges (live radios) are passed through.
package stream

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options tune the proxy.
type Options struct {
	ChunkSize  int64 // bytes per range request (default 1 MiB)
	Workers    int   // parallel requests per track (default 4)
	Ahead      int   // chunks kept ahead of the player when Full is false (default 8)
	Full       bool  // download the whole track in the background
	MaxEntries int   // tracks kept in memory (default 4)
	UserAgent  string
}

// Proxy serves registered URLs at http://127.0.0.1:<port>/s/<id>.
type Proxy struct {
	opt     Options
	client  *http.Client
	ln      net.Listener
	srv     *http.Server
	mu      sync.Mutex
	entries map[string]*entry
	Logf    func(format string, args ...any)
}

type entry struct {
	id, url  string
	headers  map[string]string
	created  time.Time
	lastUsed time.Time

	mu       sync.Mutex
	cond     *sync.Cond
	probed   bool
	probeErr error
	passthru bool
	size     int64
	ctype    string
	chunks   map[int64][]byte
	inflight map[int64]bool
	failed   map[int64]int
	want     int64 // chunk the player needs next
	workers  int
	closed   bool
	bytes    int64 // downloaded bytes
	started  time.Time
}

// New starts the proxy on a random local port.
func New(opt Options) (*Proxy, error) {
	if opt.ChunkSize <= 0 {
		opt.ChunkSize = 1 << 20
	}
	if opt.Workers <= 0 {
		opt.Workers = 4
	}
	if opt.Ahead <= 0 {
		opt.Ahead = 8
	}
	if opt.MaxEntries <= 0 {
		opt.MaxEntries = 4
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{opt: opt, ln: ln, entries: map[string]*entry{},
		client: &http.Client{Timeout: 0, Transport: &http.Transport{
			MaxIdleConnsPerHost: 16, IdleConnTimeout: 90 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
		}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/s/", p.serve)
	p.srv = &http.Server{Handler: mux}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

// SetOptions changes the tuning for tracks registered afterwards.
func (p *Proxy) SetOptions(o Options) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if o.ChunkSize <= 0 {
		o.ChunkSize = p.opt.ChunkSize
	}
	if o.Workers <= 0 {
		o.Workers = p.opt.Workers
	}
	if o.Ahead <= 0 {
		o.Ahead = p.opt.Ahead
	}
	if o.MaxEntries <= 0 {
		o.MaxEntries = p.opt.MaxEntries
	}
	p.opt = o
}

func (p *Proxy) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

// Close stops the proxy.
func (p *Proxy) Close() {
	_ = p.srv.Close()
	p.mu.Lock()
	for _, e := range p.entries {
		e.close()
	}
	p.mu.Unlock()
}

// Register returns the local URL that serves url.
func (p *Proxy) Register(url string, headers map[string]string) string {
	h := sha1.Sum([]byte(url))
	id := hex.EncodeToString(h[:8])
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.entries[id]; !ok {
		e := &entry{id: id, url: url, headers: headers, created: time.Now(), lastUsed: time.Now(),
			chunks: map[int64][]byte{}, inflight: map[int64]bool{}, failed: map[int64]int{}, size: -1}
		e.cond = sync.NewCond(&e.mu)
		p.entries[id] = e
		p.evict()
	}
	return fmt.Sprintf("http://%s/s/%s", p.ln.Addr().String(), id)
}

// Prefetch starts downloading the beginning of a registered URL.
func (p *Proxy) Prefetch(local string) {
	e := p.lookup(local)
	if e == nil {
		return
	}
	go func() {
		if err := p.probe(e); err != nil {
			p.logf("prefetch %s: %v", e.id, err)
			return
		}
		p.kick(e)
	}()
}

func (p *Proxy) lookup(local string) *entry {
	i := strings.LastIndex(local, "/s/")
	if i < 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.entries[local[i+3:]]
}

// evict drops the least recently used tracks (p.mu held).
func (p *Proxy) evict() {
	if len(p.entries) <= p.opt.MaxEntries {
		return
	}
	es := make([]*entry, 0, len(p.entries))
	for _, e := range p.entries {
		es = append(es, e)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].lastUsed.Before(es[j].lastUsed) })
	for _, e := range es[:len(es)-p.opt.MaxEntries] {
		e.close()
		delete(p.entries, e.id)
	}
}

func (e *entry) close() {
	e.mu.Lock()
	e.closed = true
	e.chunks = map[int64][]byte{}
	e.cond.Broadcast()
	e.mu.Unlock()
}

func (p *Proxy) newReq(ctx context.Context, e *entry, method string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, e.url, nil)
	if err != nil {
		return nil, err
	}
	if p.opt.UserAgent != "" {
		req.Header.Set("User-Agent", p.opt.UserAgent)
	}
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// probe learns the size and whether ranges work (first chunk is kept).
func (p *Proxy) probe(e *entry) error {
	e.mu.Lock()
	for !e.probed && e.inflight[-1] {
		e.cond.Wait()
	}
	if e.probed {
		err := e.probeErr
		e.mu.Unlock()
		return err
	}
	e.inflight[-1] = true
	e.started = time.Now()
	e.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := func() error {
		req, err := p.newReq(ctx, e, "GET")
		if err != nil {
			return err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", p.opt.ChunkSize-1))
		resp, err := p.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		e.mu.Lock()
		e.ctype = resp.Header.Get("Content-Type")
		e.mu.Unlock()
		if resp.StatusCode == http.StatusOK || resp.StatusCode != http.StatusPartialContent {
			if resp.StatusCode >= 400 {
				return fmt.Errorf("http %d", resp.StatusCode)
			}
			e.mu.Lock()
			e.passthru = true
			e.mu.Unlock()
			return nil
		}
		total := parseTotal(resp.Header.Get("Content-Range"))
		if total <= 0 {
			e.mu.Lock()
			e.passthru = true
			e.mu.Unlock()
			return nil
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		e.mu.Lock()
		e.size = total
		e.chunks[0] = data
		e.bytes += int64(len(data))
		e.mu.Unlock()
		return nil
	}()
	e.mu.Lock()
	delete(e.inflight, -1)
	e.probed, e.probeErr = true, err
	e.cond.Broadcast()
	e.mu.Unlock()
	return err
}

func parseTotal(cr string) int64 {
	i := strings.LastIndexByte(cr, '/')
	if i < 0 {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func (p *Proxy) nChunks(e *entry) int64 { return (e.size + p.opt.ChunkSize - 1) / p.opt.ChunkSize }

// next picks the chunk a worker should download (e.mu held), or -1.
func (p *Proxy) next(e *entry) int64 {
	n := p.nChunks(e)
	limit := n
	if !p.opt.Full {
		limit = min(n, e.want+int64(p.opt.Ahead))
	}
	for i := e.want; i < limit; i++ {
		if e.chunks[i] == nil && !e.inflight[i] && e.failed[i] < 3 {
			return i
		}
	}
	if p.opt.Full {
		for i := int64(0); i < e.want && i < n; i++ {
			if e.chunks[i] == nil && !e.inflight[i] && e.failed[i] < 3 {
				return i
			}
		}
	}
	return -1
}

// kick starts workers until the pool is full.
func (p *Proxy) kick(e *entry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.passthru || e.size <= 0 || e.closed {
		return
	}
	for e.workers < p.opt.Workers {
		if p.next(e) < 0 {
			return
		}
		e.workers++
		go p.worker(e)
	}
}

func (p *Proxy) worker(e *entry) {
	for {
		e.mu.Lock()
		i := p.next(e)
		if i < 0 || e.closed {
			e.workers--
			e.mu.Unlock()
			return
		}
		e.inflight[i] = true
		e.mu.Unlock()

		data, err := p.fetch(e, i)

		e.mu.Lock()
		delete(e.inflight, i)
		if err != nil {
			e.failed[i]++
			p.logf("chunk %d of %s: %v", i, e.id, err)
		} else if !e.closed {
			e.chunks[i] = data
			e.bytes += int64(len(data))
		}
		e.cond.Broadcast()
		e.mu.Unlock()
	}
}

func (p *Proxy) fetch(e *entry, i int64) ([]byte, error) {
	start := i * p.opt.ChunkSize
	end := min(start+p.opt.ChunkSize, e.size) - 1
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := p.newReq(ctx, e, "GET")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != end-start+1 {
		return nil, fmt.Errorf("short chunk: %d of %d bytes", len(data), end-start+1)
	}
	return data, nil
}

// serve answers the player, honoring its Range header.
func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	e := p.lookup(r.URL.Path)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	p.mu.Lock()
	e.lastUsed = time.Now()
	p.mu.Unlock()
	if err := p.probe(e); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	e.mu.Lock()
	pass, size, ctype := e.passthru, e.size, e.ctype
	e.mu.Unlock()
	if pass {
		p.passthrough(e, w, r)
		return
	}
	var off int64
	if rg := r.Header.Get("Range"); strings.HasPrefix(rg, "bytes=") {
		a, _, _ := strings.Cut(strings.TrimPrefix(rg, "bytes="), "-")
		off, _ = strconv.ParseInt(a, 10, 64)
	}
	if off < 0 || off >= size {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(size-off, 10))
	if r.Header.Get("Range") != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", off, size-1, size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return
	}
	for off < size {
		i := off / p.opt.ChunkSize
		e.mu.Lock()
		e.want = i
		e.mu.Unlock()
		p.kick(e)
		data, err := p.wait(e, i, r.Context())
		if err != nil {
			return
		}
		from := off - i*p.opt.ChunkSize
		if _, err := w.Write(data[from:]); err != nil {
			return
		}
		off = (i + 1) * p.opt.ChunkSize
	}
}

func (p *Proxy) wait(e *entry, i int64, ctx context.Context) ([]byte, error) {
	stop := context.AfterFunc(ctx, func() { e.mu.Lock(); e.cond.Broadcast(); e.mu.Unlock() })
	defer stop()
	e.mu.Lock()
	defer e.mu.Unlock()
	for {
		if d := e.chunks[i]; d != nil {
			return d, nil
		}
		if e.closed || ctx.Err() != nil {
			return nil, errors.New("closed")
		}
		if e.failed[i] >= 3 {
			return nil, fmt.Errorf("chunk %d failed", i)
		}
		if !e.inflight[i] && e.workers < p.opt.Workers {
			e.mu.Unlock()
			p.kick(e)
			e.mu.Lock()
			continue
		}
		e.cond.Wait()
	}
}

// passthrough pipes the upstream response (no range support / live streams).
func (p *Proxy) passthrough(e *entry, w http.ResponseWriter, r *http.Request) {
	req, err := p.newReq(r.Context(), e, r.Method)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	if v := r.Header.Get("Icy-MetaData"); v != "" {
		req.Header.Set("Icy-MetaData", v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// Stat describes one track in the proxy (developer mode).
type Stat struct {
	ID, URL     string
	Size        int64
	Chunks      int64
	Done        int64
	Inflight    int
	Passthrough bool
	Bytes       int64
	Speed       float64 // bytes/s since the first request
	Err         string
}

// Stats lists the tracks in memory, newest first.
func (p *Proxy) Stats() []Stat {
	p.mu.Lock()
	es := make([]*entry, 0, len(p.entries))
	for _, e := range p.entries {
		es = append(es, e)
	}
	p.mu.Unlock()
	sort.Slice(es, func(i, j int) bool { return es[i].lastUsed.After(es[j].lastUsed) })
	var out []Stat
	for _, e := range es {
		e.mu.Lock()
		s := Stat{ID: e.id, URL: e.url, Size: e.size, Passthrough: e.passthru, Bytes: e.bytes, Inflight: len(e.inflight)}
		if e.size > 0 {
			s.Chunks = p.nChunks(e)
		}
		s.Done = int64(len(e.chunks))
		if e.probeErr != nil {
			s.Err = e.probeErr.Error()
		}
		if d := time.Since(e.started).Seconds(); d > 0 && !e.started.IsZero() {
			s.Speed = float64(e.bytes) / d
		}
		e.mu.Unlock()
		out = append(out, s)
	}
	return out
}
