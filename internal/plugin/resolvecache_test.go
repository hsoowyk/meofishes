package plugin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolve.json")
	c := OpenResolveCache(path)
	c.Put("a", "x", time.Now().Add(time.Hour))
	c.Put("old", "y", time.Now().Add(-time.Hour)) // already expired: ignored
	if v, ok := c.Get("a"); !ok || v != "x" {
		t.Fatal("get")
	}
	if _, ok := c.Get("old"); ok {
		t.Fatal("expired value returned")
	}
	c.save()
	if v, ok := OpenResolveCache(path).Get("a"); !ok || v != "x" {
		t.Fatal("not saved")
	}
	exp := time.Now().Add(6 * time.Hour).Unix()
	if e := urlExpiry(fmt.Sprintf("https://rr1.googlevideo.com/videoplayback?expire=%d&ei=x", exp)); e.Unix() != exp-20*60 {
		t.Fatalf("expiry %v", e)
	}
	ok200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(206) }))
	defer ok200.Close()
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer gone.Close()
	if !streamAlive(t.Context(), ok200.URL) || streamAlive(t.Context(), gone.URL) {
		t.Fatal("streamAlive")
	}
}

func TestFlights(t *testing.T) {
	f := &flights{m: map[string]*flight{}}
	calls := 0
	start := make(chan struct{})
	res := make(chan string, 3)
	for i := 0; i < 3; i++ {
		go func() {
			v, _ := f.do("k", func() (string, error) { calls++; <-start; return "v", nil })
			res <- v
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(start)
	for i := 0; i < 3; i++ {
		if v := <-res; v != "v" {
			t.Fatal(v)
		}
	}
	if calls != 1 {
		t.Fatalf("%d calls", calls)
	}
}
