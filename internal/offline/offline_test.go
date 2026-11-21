package offline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meofishes/internal/core"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte(strings.Repeat("x", 3000)))
	}))
	defer srv.Close()
	s := Open(filepath.Join(dir, "offline.json"), filepath.Join(dir, "dl"), filepath.Join(dir, "cache"))
	a := core.Track{Source: "x", ID: "1", Title: "A/B", Artist: "Z"}
	p, err := s.Fetch(context.Background(), a, srv.URL+"/a", nil, Cache, nil)
	if err != nil || !strings.HasSuffix(p, ".mp3") {
		t.Fatalf("fetch: %v %s", err, p)
	}
	if s.Kind(a) != Cache {
		t.Fatal("kind")
	}
	// promote to a download without downloading again
	p2, err := s.Fetch(context.Background(), a, "http://invalid.invalid/", nil, Download, nil)
	if err != nil || filepath.Dir(p2) != filepath.Join(dir, "dl") || s.Kind(a) != Download {
		t.Fatalf("promote: %v %s %s", err, p2, s.Kind(a))
	}
	// reopen from disk
	s2 := Open(filepath.Join(dir, "offline.json"), filepath.Join(dir, "dl"), filepath.Join(dir, "cache"))
	if _, ok := s2.Lookup(a); !ok {
		t.Fatal("lookup after reopen")
	}
	// cache trimming
	s2.MaxCacheMB = 0
	b := core.Track{Source: "x", ID: "2", Title: "B"}
	if _, err := s2.Save(b, []byte("data"), ".ogg", Cache); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Lookup(b); ok {
		t.Fatal("cache over the limit should be trimmed")
	}
	s2.Remove(a)
	if _, err := os.Stat(p2); err == nil {
		t.Fatal("file not removed")
	}
}
