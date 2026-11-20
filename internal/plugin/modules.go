package plugin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meofishes/internal/lyrics"
)

// Host modules are Go libraries a plugin can ask for in its manifest
// ("modules": ["http", "ytdlp"]). Script plugins get them through
// require("name"); declarative plugins use them through "resolver".
var moduleDocs = map[string]string{
	"http":     "HTTP client: get, getJSON, post, postJSON, request (respects the manifest's \"hosts\")",
	"ytdlp":    "yt-dlp bindings: available, search, info, streamUrl, playlist (YouTube, SoundCloud, Bandcamp… ~1800 sites)",
	"lavalink": "Lavalink v4 REST client: connect({host, port, password, secure}) → search, loadTracks, decode, info",
	"lyrics":   "lyrics parsers and matching: parse, cleanTitle, primaryArtist, matchScore",
	"crypto":   "md5, sha1, sha256, hmacSha256, base64Encode, base64Decode, uuid",
	"storage":  "per-plugin persistent key/value storage: get, set, remove, all",
}

// ModuleNames lists the available modules.
func ModuleNames() []string {
	var out []string
	for k := range moduleDocs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ModuleDoc returns the one-line description of a module.
func ModuleDoc(name string) string { return moduleDocs[name] }

// moduleEnv is what a module instance knows about its plugin.
type moduleEnv struct {
	m    *Manifest
	dir  string
	host *Host
}

func (e *moduleEnv) newModule(name string) (map[string]any, error) {
	if !e.m.Wants(name) {
		return nil, fmt.Errorf("module %q is not listed in plugin.json \"modules\"", name)
	}
	switch name {
	case "http":
		return httpModule(e), nil
	case "ytdlp":
		return ytdlpModule(e), nil
	case "lavalink":
		return lavalinkModule(e), nil
	case "lyrics":
		return lyricsModule(), nil
	case "crypto":
		return cryptoModule(), nil
	case "storage":
		return storageModule(e), nil
	}
	return nil, fmt.Errorf("unknown module %q", name)
}

// ---------------------------------------------------------------- http

func (e *moduleEnv) hostAllowed(raw string) error {
	if len(e.m.Hosts) == 0 {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	h := strings.ToLower(u.Hostname())
	for _, pat := range e.m.Hosts {
		pat = strings.ToLower(pat)
		if pat == h || (strings.HasPrefix(pat, "*.") && (strings.HasSuffix(h, pat[1:]) || h == pat[2:])) {
			return nil
		}
	}
	return fmt.Errorf("host %s is not in the plugin's \"hosts\" list", h)
}

// HTTPResponse is returned to scripts.
type HTTPResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	JSON    any               `json:"json"`
}

func (e *moduleEnv) doHTTP(ctx context.Context, method, rawURL string, body any, opts map[string]any) (*HTTPResponse, error) {
	if q, ok := opts["query"].(map[string]any); ok && len(q) > 0 {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		vals := u.Query()
		for k, v := range q {
			vals.Set(k, toString(v))
		}
		u.RawQuery = vals.Encode()
		rawURL = u.String()
	}
	if err := e.hostAllowed(rawURL); err != nil {
		return nil, err
	}
	var rd io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(data)
		contentType = "application/json"
	}
	if t, ok := opts["timeout"].(int64); ok && t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t)*time.Millisecond)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", e.host.UserAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if h, ok := opts["headers"].(map[string]any); ok {
		for k, v := range h {
			req.Header.Set(k, toString(v))
		}
	}
	resp, err := e.host.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	out := &HTTPResponse{Status: resp.StatusCode, Headers: map[string]string{}, Body: string(data)}
	for k := range resp.Header {
		out.Headers[strings.ToLower(k)] = resp.Header.Get(k)
	}
	var js any
	if json.Unmarshal(data, &js) == nil {
		out.JSON = js
	}
	return out, nil
}

func httpModule(e *moduleEnv) map[string]any {
	ctx := context.Background()
	asMap := func(r *HTTPResponse) map[string]any {
		return map[string]any{"status": r.Status, "headers": r.Headers, "body": r.Body, "json": r.JSON, "ok": r.Status < 400}
	}
	return map[string]any{
		"request": func(opts map[string]any) (map[string]any, error) {
			method, _ := opts["method"].(string)
			if method == "" {
				method = "GET"
			}
			u, _ := opts["url"].(string)
			r, err := e.doHTTP(ctx, strings.ToUpper(method), u, opts["body"], opts)
			if err != nil {
				return nil, err
			}
			return asMap(r), nil
		},
		"get": func(u string, opts map[string]any) (map[string]any, error) {
			r, err := e.doHTTP(ctx, "GET", u, nil, opts)
			if err != nil {
				return nil, err
			}
			return asMap(r), nil
		},
		"getJSON": func(u string, opts map[string]any) (any, error) {
			r, err := e.doHTTP(ctx, "GET", u, nil, opts)
			if err != nil {
				return nil, err
			}
			if r.Status >= 400 {
				return nil, fmt.Errorf("GET %s: http %d", u, r.Status)
			}
			if r.JSON == nil {
				return nil, fmt.Errorf("GET %s: response is not JSON", u)
			}
			return r.JSON, nil
		},
		"post": func(u string, body any, opts map[string]any) (map[string]any, error) {
			r, err := e.doHTTP(ctx, "POST", u, body, opts)
			if err != nil {
				return nil, err
			}
			return asMap(r), nil
		},
		"postJSON": func(u string, body any, opts map[string]any) (any, error) {
			r, err := e.doHTTP(ctx, "POST", u, body, opts)
			if err != nil {
				return nil, err
			}
			if r.Status >= 400 {
				return nil, fmt.Errorf("POST %s: http %d", u, r.Status)
			}
			return r.JSON, nil
		},
		"encode": func(params map[string]any) string {
			v := url.Values{}
			for k, x := range params {
				v.Set(k, toString(x))
			}
			return v.Encode()
		},
	}
}

// ---------------------------------------------------------------- lyrics

func lyricsModule() map[string]any {
	return map[string]any{
		"parse": func(format, content string) (any, error) {
			l, err := lyrics.Parse(format, content)
			if err != nil {
				return nil, err
			}
			var m any
			data, _ := json.Marshal(l)
			_ = json.Unmarshal(data, &m)
			return m, nil
		},
		"cleanTitle":    lyrics.CleanTitle,
		"primaryArtist": lyrics.PrimaryArtist,
		"decodeKRC":     lyrics.DecodeKRC,
		"matchScore": func(q map[string]any, title, artist string, durationMs int64) float64 {
			lq := lyrics.Query{Title: toString(q["title"]), Artist: toString(q["artist"])}
			if d, ok := q["durationMs"].(int64); ok {
				lq.DurationMs = d
			}
			return lyrics.MatchScore(lq, title, artist, durationMs)
		},
	}
}

// ---------------------------------------------------------------- crypto

func cryptoModule() map[string]any {
	return map[string]any{
		"md5":    func(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) },
		"sha1":   func(s string) string { h := sha1.Sum([]byte(s)); return hex.EncodeToString(h[:]) },
		"sha256": func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) },
		"hmacSha256": func(key, msg string) string {
			m := hmac.New(sha256.New, []byte(key))
			m.Write([]byte(msg))
			return hex.EncodeToString(m.Sum(nil))
		},
		"base64Encode": func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"base64Decode": func(s string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(s)
			return string(b), err
		},
		"uuid": func() string {
			b := make([]byte, 16)
			_, _ = rand.Read(b)
			b[6], b[8] = (b[6]&0x0f)|0x40, (b[8]&0x3f)|0x80
			return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
		},
	}
}

// ---------------------------------------------------------------- storage

type kvStore struct {
	mu   sync.Mutex
	path string
	data map[string]any
}

func (s *kvStore) load() {
	if s.data != nil {
		return
	}
	s.data = map[string]any{}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.data)
	}
}

func (s *kvStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.data, "", "  ")
	return os.WriteFile(s.path, b, 0o600)
}

func storageModule(e *moduleEnv) map[string]any {
	if e.host.DataDir == "" {
		return map[string]any{"get": func(string) any { return nil }}
	}
	s := &kvStore{path: filepath.Join(e.host.DataDir, e.m.ID, "storage.json")}
	return map[string]any{
		"get": func(k string) any { s.mu.Lock(); defer s.mu.Unlock(); s.load(); return s.data[k] },
		"set": func(k string, v any) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.load()
			s.data[k] = v
			return s.save()
		},
		"remove": func(k string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.load()
			delete(s.data, k)
			return s.save()
		},
		"all": func() map[string]any { s.mu.Lock(); defer s.mu.Unlock(); s.load(); return s.data },
	}
}

var errNoBinary = errors.New("binary not found")
