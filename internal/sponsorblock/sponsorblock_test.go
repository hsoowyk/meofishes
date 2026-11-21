package sponsorblock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSegments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("videoID") == "none" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[{"segment":[200.5,210],"category":"outro"},{"segment":[0,12.25],"category":"music_offtopic"}]`))
	}))
	defer srv.Close()
	c := New("test")
	c.API = srv.URL
	s, err := c.Segments(context.Background(), "abc", Categories)
	if err != nil || len(s) != 2 || s[0].StartMs != 0 || s[0].EndMs != 12250 {
		t.Fatalf("%v %v", s, err)
	}
	if to, cat := SkipTo(s, 3000); to != 12250 || cat != "music_offtopic" {
		t.Fatalf("skip %d %s", to, cat)
	}
	if to, _ := SkipTo(s, 100000); to != 0 {
		t.Fatal("skipped outside")
	}
	if s, err := c.Segments(context.Background(), "none", Categories); err != nil || len(s) != 0 {
		t.Fatalf("404: %v %v", s, err)
	}
}
