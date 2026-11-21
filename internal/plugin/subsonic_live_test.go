//go:build live

package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meofishes/internal/pkgfs"
)

// TestLiveSubsonic runs the bundled Subsonic plugin against Navidrome's demo.
func TestLiveSubsonic(t *testing.T) {
	dir := t.TempDir()
	if err := pkgfs.CopyFS(os.DirFS("../../assets/plugins/subsonic"), ".", filepath.Join(dir, "subsonic")); err != nil {
		t.Fatal(err)
	}
	host := &Host{Version: "test", UserAgent: "meofishes-test/0.1", DataDir: t.TempDir(), Client: &http.Client{Timeout: 30 * time.Second},
		Logf: func(id, f string, a ...any) { t.Logf("["+id+"] "+f, a...) }}
	m := NewManager(host, dir, &State{})
	m.Load()
	defer m.Close()
	if err := m.SetSettings("subsonic", map[string]string{"server": "https://demo.navidrome.org", "user": "demo", "password": "demo", "scrobble": "false"}); err != nil {
		t.Fatal(err)
	}
	p := m.Get("subsonic")
	if p.Err != nil {
		t.Fatal(p.Err)
	}
	ctx := context.Background()
	home, err := m.Home(ctx, p)
	if err != nil || len(home.Tracks) == 0 {
		t.Fatalf("home: %v", err)
	}
	tr := home.Tracks[0]
	t.Logf("home %d tracks, first %q by %q (%d ms)", len(home.Tracks), tr.Title, tr.Artist, tr.DurationMs)
	r, err := p.Source().Search(ctx, tr.Artist)
	t.Logf("search %q: %d err=%v", tr.Artist, len(r.Tracks), err)
	if err != nil || len(r.Tracks) == 0 {
		t.Error("search failed")
	}
	st, err := m.Resolve(ctx, tr)
	if err != nil {
		t.Fatal("resolve:", err)
	}
	resp, err := http.Get(st.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	t.Logf("stream via %s: HTTP %d %s", st.Via, resp.StatusCode, resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 {
		t.Error("stream not 200")
	}
	pls, errs := m.RemotePlaylists(ctx)
	t.Logf("collections: %d errs=%v", len(pls), errs)
}
