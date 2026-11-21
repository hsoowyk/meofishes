//go:build live

package artwork

import (
	"context"
	"testing"

	"meofishes/internal/lyrics"
	"meofishes/internal/plugin"
)

func TestLive(t *testing.T) {
	host := &plugin.Host{UserAgent: "meofishes-test"}
	for _, svc := range []string{"itunes", "deezer", "piped"} {
		p, _ := New(host, func() map[string]string {
			return map[string]string{"order": svc, "size": "600", "minScore": "0.55", "piped": "https://pipedapi.kavin.rocks,https://api.piped.private.coffee,https://pipedapi.adminforge.de"}
		})
		u, err := p.(*impl).Artwork(context.Background(), lyrics.Query{Title: "Blinding Lights (Official Video)", Artist: "The Weeknd", DurationMs: 200000})
		if err != nil || u == "" {
			t.Fatalf("%s: %v", svc, err)
		}
		t.Log(svc, u)
	}
}
