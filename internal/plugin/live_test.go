//go:build live

package plugin

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"meofishes/internal/lyrics"
	"meofishes/internal/pkgfs"
)

func TestLiveRuntimes(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"radio-browser", "ytdlp-search", "somafm-go", "lrclib", "deezer"} {
		if err := pkgfs.CopyFS(os.DirFS("../../assets/plugins/"+id), ".", filepath.Join(dir, id)); err != nil {
			t.Fatal(err)
		}
	}
	b := exec.Command("sh", "-c", "test -f go.mod || cp go.mod.in go.mod; go build -tags meoplugin -o somafm-go .")
	b.Dir = filepath.Join(dir, "somafm-go")
	if out, err := b.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	host := &Host{Version: "test", UserAgent: "meofishes-test/0.1", DataDir: t.TempDir(), Client: &http.Client{Timeout: 30 * time.Second},
		Logf: func(id, f string, a ...any) { t.Logf("["+id+"] "+f, a...) }}
	m := NewManager(host, dir, &State{})
	m.Load()
	defer m.Close()
	for _, p := range m.All() {
		if p.Err != nil {
			t.Errorf("%s: %v", p.Manifest.ID, p.Err)
		}
	}
	ctx := context.Background()
	for id, q := range map[string]string{"radio-browser": "jazz", "ytdlp-search": "the weeknd blinding lights", "somafm-go": "ambient"} {
		p := m.Get(id)
		r, err := p.Source().Search(ctx, q)
		if err != nil {
			t.Errorf("%s search: %v", id, err)
			continue
		}
		if len(r.Tracks) == 0 {
			t.Errorf("%s: no tracks", id)
			continue
		}
		st, err := m.Resolve(ctx, r.Tracks[0])
		t.Logf("%s: %d tracks, first=%q artist=%q stream=%v err=%v", id, len(r.Tracks), r.Tracks[0].Title, r.Tracks[0].Artist, st, err)
	}
	l, err := m.Get("lrclib").LyricsSource().Lyrics(ctx, lyricsQ())
	if err != nil {
		t.Errorf("lrclib: %v", err)
	} else {
		t.Logf("lrclib: %s, %d lines", l.Sync, len(l.Lines))
	}
	for _, p := range m.All() {
		t.Logf("caps %s: %v", p.Manifest.ID, p.Caps())
	}
	// src-only plugin (Deezer) played through the fnt "match" plugin (yt-dlp)
	dz := m.Get("deezer")
	r, err := m.Search(ctx, dz, "daft punk one more time")
	if err != nil || len(r.Tracks) == 0 {
		t.Fatalf("deezer: %v", err)
	}
	t.Logf("deezer: %q by %q cover=%s", r.Tracks[0].Title, r.Tracks[0].Artist, r.Tracks[0].Cover)
	st, err := m.Resolve(ctx, r.Tracks[0])
	if err != nil {
		t.Errorf("match resolve: %v", err)
	} else {
		t.Logf("matched via %s: %s", st.Via, st.URL)
	}
	recs, via, err := m.Recommend(ctx, r.Tracks[0])
	t.Logf("recommend via %s: %d tracks, err=%v", via, len(recs), err)
	if err != nil || len(recs) == 0 {
		t.Errorf("recommend: %v", err)
	}
}

func lyricsQ() lyricsQuery {
	return lyricsQuery{Title: "Blinding Lights", Artist: "The Weeknd", DurationMs: 200000}
}

type lyricsQuery = lyrics.Query
