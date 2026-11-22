//go:build live

package plugin

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveStreamCache(t *testing.T) {
	if YtdlpPath() == "" {
		t.Skip("no yt-dlp")
	}
	Cache = OpenResolveCache(filepath.Join(t.TempDir(), "resolve.json"))
	defer func() { Cache = nil }()
	page := "https://www.youtube.com/watch?v=b9Y4TACmvE8"
	ctx := context.Background()
	t0 := time.Now()
	u1, err := YtdlpStreamURL(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	first := time.Since(t0)
	t0 = time.Now()
	u2, err := YtdlpStreamURL(ctx, page)
	second := time.Since(t0)
	t.Logf("first %v, cached %v", first.Round(time.Millisecond), second.Round(time.Millisecond))
	if err != nil || u1 != u2 || second > 3*time.Second {
		t.Fatalf("cache not used: %v %v", second, err)
	}
	t0 = time.Now()
	v, err := YtdlpVideoURL(ctx, page, 480)
	t.Logf("video-only url in %v: %.60s… err=%v", time.Since(t0).Round(time.Millisecond), v, err)
	if err != nil {
		t.Fatal(err)
	}
}
