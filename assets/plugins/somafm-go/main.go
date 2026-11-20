//go:build meoplugin

// SomaFM source plugin for meofishes, written in Go.
//
// Process plugins are ordinary Go programs: import whatever library you need
// (here only net/http), then hand your handlers to meosdk.Serve.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"somafm-go/meosdk"
)

type channel struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DJ          string `json:"dj"`
	Genre       string `json:"genre"`
	Image       string `json:"largeimage"`
	Playlists   []struct {
		URL     string `json:"url"`
		Format  string `json:"format"`
		Quality string `json:"quality"`
	} `json:"playlists"`
}

var (
	mu       sync.Mutex
	cached   []channel
	cachedAt time.Time
)

func channels(ctx *meosdk.Context) ([]channel, error) {
	mu.Lock()
	defer mu.Unlock()
	if cached != nil && time.Since(cachedAt) < 10*time.Minute {
		return cached, nil
	}
	req, _ := http.NewRequest("GET", "https://somafm.com/channels.json", nil)
	req.Header.Set("User-Agent", ctx.UserAgent)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var doc struct {
		Channels []channel `json:"channels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	cached, cachedAt = doc.Channels, time.Now()
	ctx.Log("loaded %d channels", len(cached))
	return cached, nil
}

func toTrack(ctx *meosdk.Context, c channel) meosdk.Track {
	want := ctx.Settings["quality"]
	url := ""
	for _, p := range c.Playlists {
		if p.Format == "mp3" && (p.Quality == want || url == "") {
			url = p.URL
		}
	}
	return meosdk.Track{ID: c.ID, Title: c.Title, Artist: "SomaFM · " + c.Genre, Album: c.Description, Cover: c.Image, StreamURL: url}
}

func main() {
	meosdk.Serve(meosdk.Plugin{
		Home: func(ctx *meosdk.Context) (*meosdk.SearchResult, error) {
			chs, err := channels(ctx)
			if err != nil {
				return nil, err
			}
			res := &meosdk.SearchResult{}
			for _, c := range chs {
				res.Tracks = append(res.Tracks, toTrack(ctx, c))
			}
			return res, nil
		},
		Search: func(ctx *meosdk.Context, q string) (*meosdk.SearchResult, error) {
			chs, err := channels(ctx)
			if err != nil {
				return nil, err
			}
			q = strings.ToLower(q)
			res := &meosdk.SearchResult{}
			for _, c := range chs {
				if strings.Contains(strings.ToLower(c.Title+" "+c.Genre+" "+c.Description+" "+c.DJ), q) {
					res.Tracks = append(res.Tracks, toTrack(ctx, c))
				}
			}
			return res, nil
		},
		Resolve: func(ctx *meosdk.Context, t meosdk.Track) (*meosdk.Stream, error) {
			if t.StreamURL == "" {
				return nil, fmt.Errorf("channel %s has no mp3 stream", t.ID)
			}
			return &meosdk.Stream{URL: t.StreamURL}, nil // mpv plays .pls playlists directly
		},
	})
}
