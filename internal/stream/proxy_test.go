package stream

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyChunks(t *testing.T) {
	data := make([]byte, 5<<20+12345)
	rand.New(rand.NewSource(1)).Read(data)
	var ranged atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			ranged.Add(1)
		}
		http.ServeContent(w, r, "a.mp3", time.Now(), bytes.NewReader(data))
	}))
	defer up.Close()
	p, err := New(Options{ChunkSize: 256 << 10, Workers: 4, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	local := p.Register(up.URL+"/a.mp3", nil)
	p.Prefetch(local)

	resp, err := http.Get(local)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("body differs: %d vs %d bytes", len(got), len(data))
	}
	if ranged.Load() < 10 {
		t.Fatalf("expected many range requests, got %d", ranged.Load())
	}
	// seek
	req, _ := http.NewRequest("GET", local, nil)
	req.Header.Set("Range", "bytes=3000000-")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || !bytes.Equal(got, data[3000000:]) {
		t.Fatalf("seek: status %d, %d bytes", resp.StatusCode, len(got))
	}
	st := p.Stats()
	if len(st) != 1 || st[0].Size != int64(len(data)) {
		t.Fatalf("stats: %+v", st)
	}
}

func TestProxyPassthrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, strings.Repeat("radio", 1000)) // ignores Range, like a live radio
	}))
	defer up.Close()
	p, _ := New(Options{})
	defer p.Close()
	resp, err := http.Get(p.Register(up.URL, nil))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != strings.Repeat("radio", 1000) {
		t.Fatalf("passthrough body: %d bytes", len(got))
	}
}
