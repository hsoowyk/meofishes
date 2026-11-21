// Package offline keeps songs on disk:
//
//   - downloads: songs the user chose to keep (Download in the menus);
//     they stay until removed and play without internet;
//   - cache: songs kept automatically after they were played in full,
//     limited in size (the oldest are removed first).
//
// Both are looked up by the track (source + id), so a song plays from disk
// whatever plugin resolves it.
package offline

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meofishes/internal/core"
)

// Kinds of entries.
const (
	Download = "download"
	Cache    = "cache"
)

// Entry is one song on disk.
type Entry struct {
	Key   string     `json:"key"`
	Kind  string     `json:"kind"`
	Path  string     `json:"path"`
	Size  int64      `json:"size"`
	Track core.Track `json:"track"`
	At    time.Time  `json:"at"`
	Used  time.Time  `json:"used"`
}

// Store is the index of downloaded and cached songs.
type Store struct {
	DownloadDir string
	CacheDir    string
	MaxCacheMB  int
	UserAgent   string

	mu       sync.Mutex
	index    map[string]*Entry
	file     string
	client   *http.Client
	active   map[string]bool
	OnChange func()
}

// Open loads the index (stored in CacheDir/../offline.json).
func Open(indexFile, downloadDir, cacheDir string) *Store {
	s := &Store{DownloadDir: downloadDir, CacheDir: cacheDir, MaxCacheMB: 1024, file: indexFile,
		index: map[string]*Entry{}, active: map[string]bool{}, client: &http.Client{Timeout: 0}}
	if data, err := os.ReadFile(indexFile); err == nil {
		var list []*Entry
		if json.Unmarshal(data, &list) == nil {
			for _, e := range list {
				if st, err := os.Stat(e.Path); err == nil && !st.IsDir() {
					s.index[e.Key] = e
				}
			}
		}
	}
	return s
}

func (s *Store) changed() {
	s.mu.Lock()
	list := make([]*Entry, 0, len(s.index))
	for _, e := range s.index {
		list = append(list, e)
	}
	s.mu.Unlock()
	data, _ := json.MarshalIndent(list, "", " ")
	_ = os.MkdirAll(filepath.Dir(s.file), 0o755)
	_ = os.WriteFile(s.file, data, 0o644)
	if s.OnChange != nil {
		s.OnChange()
	}
}

// Lookup returns the file of a track, if it is on disk.
func (s *Store) Lookup(t core.Track) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.index[t.Key()]
	if e == nil {
		return "", false
	}
	if _, err := os.Stat(e.Path); err != nil {
		delete(s.index, e.Key)
		return "", false
	}
	e.Used = time.Now()
	return e.Path, true
}

// Kind returns "download", "cache" or "".
func (s *Store) Kind(t core.Track) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.index[t.Key()]; e != nil {
		return e.Kind
	}
	return ""
}

// Busy reports whether a track is being downloaded.
func (s *Store) Busy(t core.Track) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[t.Key()]
}

// List returns the entries of a kind, newest first.
func (s *Store) List(kind string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.index {
		if kind == "" || e.Kind == kind {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Usage is the size on disk of a kind.
func (s *Store) Usage(kind string) (n int, bytes int64) {
	for _, e := range s.List(kind) {
		n++
		bytes += e.Size
	}
	return
}

func fileName(t core.Track, ext string) string {
	h := sha1.Sum([]byte(t.Key()))
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(t.String()))
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	if name == "" {
		name = "track"
	}
	return name + " [" + hex.EncodeToString(h[:4]) + "]" + ext
}

// ExtFor guesses a file extension from a content type or URL.
func ExtFor(ctype, url string) string {
	ct, _, _ := mime.ParseMediaType(ctype)
	switch ct {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a", "audio/aac":
		return ".m4a"
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/opus":
		return ".opus"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "video/mp4":
		return ".mp4"
	}
	u := strings.ToLower(strings.SplitN(url, "?", 2)[0])
	for _, e := range []string{".mp3", ".m4a", ".webm", ".ogg", ".opus", ".flac", ".wav", ".aac", ".mp4"} {
		if strings.HasSuffix(u, e) {
			return e
		}
	}
	if strings.Contains(url, "googlevideo.com") {
		if strings.Contains(url, "mime=audio%2Fmp4") {
			return ".m4a"
		}
		return ".webm"
	}
	return ".audio"
}

// Save stores data for a track (cache entries are trimmed to MaxCacheMB).
func (s *Store) Save(t core.Track, data []byte, ext, kind string) (string, error) {
	dir := s.CacheDir
	if kind == Download {
		dir = s.DownloadDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName(t, ext))
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	s.add(t, path, int64(len(data)), kind)
	return path, nil
}

func (s *Store) add(t core.Track, path string, size int64, kind string) {
	s.mu.Lock()
	if old := s.index[t.Key()]; old != nil && old.Path != path {
		if old.Kind == Download && kind == Cache { // never downgrade a download
			s.mu.Unlock()
			_ = os.Remove(path)
			return
		}
		_ = os.Remove(old.Path)
	}
	s.index[t.Key()] = &Entry{Key: t.Key(), Kind: kind, Path: path, Size: size, Track: t, At: time.Now(), Used: time.Now()}
	s.mu.Unlock()
	if kind == Cache {
		s.Trim()
	}
	s.changed()
}

// Fetch downloads url (a direct audio URL) for a track. progress may be nil.
func (s *Store) Fetch(ctx context.Context, t core.Track, url string, headers map[string]string, kind string,
	progress func(done, total int64)) (string, error) {
	key := t.Key()
	s.mu.Lock()
	if s.active[key] {
		s.mu.Unlock()
		return "", errors.New("already downloading")
	}
	s.active[key] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.active, key); s.mu.Unlock() }()

	if p, ok := s.Lookup(t); ok && (kind == Cache || s.Kind(t) == Download) {
		return p, nil
	}
	// a cached copy becomes a download without downloading again
	if kind == Download && s.Kind(t) == Cache {
		if old, ok := s.Lookup(t); ok {
			if err := os.MkdirAll(s.DownloadDir, 0o755); err == nil {
				dst := filepath.Join(s.DownloadDir, filepath.Base(old))
				if os.Rename(old, dst) == nil {
					st, _ := os.Stat(dst)
					s.mu.Lock()
					delete(s.index, key)
					s.mu.Unlock()
					s.add(t, dst, st.Size(), Download)
					return dst, nil
				}
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	dir := s.CacheDir
	if kind == Download {
		dir = s.DownloadDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName(t, ExtFor(resp.Header.Get("Content-Type"), url)))
	f, err := os.Create(path + ".part")
	if err != nil {
		return "", err
	}
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(path + ".part")
				return "", werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(path + ".part")
			return "", rerr
		}
	}
	f.Close()
	if err := os.Rename(path+".part", path); err != nil {
		return "", err
	}
	s.add(t, path, done, kind)
	return path, nil
}

// Remove deletes a song from disk.
func (s *Store) Remove(t core.Track) {
	s.mu.Lock()
	e := s.index[t.Key()]
	delete(s.index, t.Key())
	s.mu.Unlock()
	if e != nil {
		_ = os.Remove(e.Path)
		s.changed()
	}
}

// Clear removes every entry of a kind.
func (s *Store) Clear(kind string) {
	s.mu.Lock()
	for k, e := range s.index {
		if e.Kind == kind {
			_ = os.Remove(e.Path)
			delete(s.index, k)
		}
	}
	s.mu.Unlock()
	s.changed()
}

// Trim removes the least recently used cache entries above MaxCacheMB.
func (s *Store) Trim() {
	s.mu.Lock()
	var cache []*Entry
	var total int64
	for _, e := range s.index {
		if e.Kind == Cache {
			cache = append(cache, e)
			total += e.Size
		}
	}
	limit := int64(s.MaxCacheMB) << 20
	sort.Slice(cache, func(i, j int) bool { return cache[i].Used.Before(cache[j].Used) })
	for _, e := range cache {
		if total <= limit {
			break
		}
		_ = os.Remove(e.Path)
		delete(s.index, e.Key)
		total -= e.Size
	}
	s.mu.Unlock()
}
