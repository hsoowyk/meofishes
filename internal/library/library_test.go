package library

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"meofishes/internal/core"
)

func TestLibraryAndMepw(t *testing.T) {
	dir := t.TempDir()
	l := Open(filepath.Join(dir, "library.json"))
	a := core.Track{ID: "1", Source: "x", Title: "A", Artist: "Z"}
	b := core.Track{ID: "2", Source: "x", Title: "B"}
	c := core.Track{ID: "3", Source: "y", Title: "C"}
	l.SetLiked(a, true)
	if !l.IsLiked(a) || l.IsLiked(b) {
		t.Fatal("liked")
	}
	p := l.Create("Mine", a, b, c)
	_ = l.Move(p.ID, 0, 2)
	if got := l.Get(p.ID).Tracks; got[0].ID != "2" || got[2].ID != "1" {
		t.Fatalf("move: %+v", got)
	}
	_ = l.Add(p.ID, a) // duplicate ignored
	if len(l.Get(p.ID).Tracks) != 3 {
		t.Fatal("duplicate added")
	}
	for _, gz := range []bool{false, true} {
		var buf bytes.Buffer
		if err := WriteMepw(&buf, "test", gz, l.Get(p.ID), l.Liked()); err != nil {
			t.Fatal(err)
		}
		doc, err := ReadMepw(&buf)
		if err != nil {
			t.Fatal(err)
		}
		l2 := Open(filepath.Join(dir, "other.json"))
		ps := l2.Import(doc)
		if len(ps) != 2 || len(ps[0].Tracks) != 3 || !l2.IsLiked(a) {
			t.Fatalf("import gz=%v: %d playlists", gz, len(ps))
		}
	}
	// reload from disk
	l3 := Open(filepath.Join(dir, "library.json"))
	if len(l3.All()) != 2 || !l3.IsLiked(a) {
		t.Fatal("reload")
	}
	doc, err := ReadMepw(bytes.NewBufferString("#EXTM3U\n#EXTINF:10,Art - Song\nhttp://x/a.mp3\n"))
	if err != nil || doc.Playlists[0].Tracks[0].Artist != "Art" {
		t.Fatalf("m3u: %v", err)
	}
	if err := l.Delete(LikedID); err == nil {
		t.Fatal("deleted liked")
	}
}

func TestStatsAndSearches(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "l.json"))
	a := core.Track{ID: "1", Source: "x", Title: "A", Artist: "Ana", DurationMs: 200000}
	b := core.Track{ID: "2", Source: "x", Title: "B", Artist: "Bia", DurationMs: 200000}
	l.RecordListen(a, 120000)
	l.RecordListen(a, 60000)
	l.RecordListen(b, 10000) // too short to count as a play
	songs, artists, total := l.Top(time.Time{}, 10)
	if len(songs) != 2 || songs[0].Track.ID != "1" || songs[0].Plays != 2 || songs[1].Plays != 0 {
		t.Fatalf("songs %+v", songs)
	}
	if artists[0].Name != "Ana" || total != 190*time.Second {
		t.Fatalf("artists %+v total %v", artists, total)
	}
	l.AddSearch("daft punk")
	l.AddSearch("Weeknd")
	l.AddSearch("DAFT PUNK")
	if s := l.RecentSearches(); len(s) != 2 || s[0] != "DAFT PUNK" {
		t.Fatalf("searches %v", s)
	}
}
