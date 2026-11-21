//go:build live

package lyrics

// Run with: go test -tags live -run Live -v ./internal/lyrics
// Hits the real services.

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestLiveProviders(t *testing.T) {
	ua := os.Getenv("MEO_UA")
	if ua == "" {
		ua = "meofishes/0.1 (https://github.com/meofishes/meofishes)"
	}
	env := &Env{Client: &http.Client{Timeout: 20 * time.Second}, UserAgent: ua}
	q := Query{Title: "Blinding Lights", Artist: "The Weeknd", Album: "After Hours", DurationMs: 200046}
	for _, id := range DefaultOrder {
		p := Builtins(env)[id]
		start := time.Now()
		l, err := p.Fetch(context.Background(), q)
		if err != nil {
			t.Logf("%-13s ERR %v (%s)", id, err, time.Since(start).Round(time.Millisecond))
			continue
		}
		first := ""
		if len(l.Lines) > 0 {
			first = l.Lines[0].Text
		}
		t.Logf("%-13s %-5s %3d lines  first=%q (%s)", id, l.Sync, len(l.Lines), first, time.Since(start).Round(time.Millisecond))
	}
}

func TestLiveSearchByLyrics(t *testing.T) {
	e := &Env{Client: http.DefaultClient, UserAgent: "meofishes-test"}
	hits, err := SearchByLyrics(context.Background(), e, "we're no strangers to love")
	if err != nil || len(hits) == 0 {
		t.Fatalf("%v %v", hits, err)
	}
	t.Logf("%d hits, first %q by %q: %q", len(hits), hits[0].Title, hits[0].Artist, hits[0].Snippet)
}
