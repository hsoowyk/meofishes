package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Lavalink is a small client for the Lavalink v4 REST API
// (https://lavalink.dev/api/rest). Lavalink resolves tracks from many
// providers (YouTube, SoundCloud, Bandcamp, Spotify/Deezer/Apple through
// LavaSrc…). It streams audio to Discord voice only, so for local playback
// the track's "uri" is handed to the player (mpv + yt-dlp) instead.
type Lavalink struct {
	Base     string
	Password string
	Client   *http.Client
}

// NewLavalink builds a client from {host, port, password, secure}.
func NewLavalink(opts map[string]any, client *http.Client) (*Lavalink, error) {
	host := toString(opts["host"])
	if host == "" {
		host = "localhost"
	}
	port := toString(opts["port"])
	if port == "" {
		port = "2333"
	}
	scheme := "http"
	if s, _ := opts["secure"].(bool); s || toString(opts["secure"]) == "true" {
		scheme = "https"
	}
	pw := toString(opts["password"])
	if pw == "" {
		pw = "youshallnotpass"
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Lavalink{Base: fmt.Sprintf("%s://%s:%s", scheme, host, port), Password: pw, Client: client}, nil
}

func (l *Lavalink) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", l.Base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", l.Password)
	resp, err := l.Client.Do(req)
	if err != nil {
		return fmt.Errorf("lavalink: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("lavalink: wrong password")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("lavalink: http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, v)
}

// LavaTrack mirrors Lavalink's track object.
type LavaTrack struct {
	Encoded string `json:"encoded"`
	Info    struct {
		Identifier string `json:"identifier"`
		IsSeekable bool   `json:"isSeekable"`
		Author     string `json:"author"`
		Length     int64  `json:"length"`
		IsStream   bool   `json:"isStream"`
		Title      string `json:"title"`
		URI        string `json:"uri"`
		ArtworkURL string `json:"artworkUrl"`
		ISRC       string `json:"isrc"`
		SourceName string `json:"sourceName"`
	} `json:"info"`
}

func (t LavaTrack) asMap() map[string]any {
	m := map[string]any{
		"id": t.Info.Identifier, "title": t.Info.Title, "artist": strings.TrimSuffix(t.Info.Author, " - Topic"),
		"durationMs": t.Info.Length, "cover": t.Info.ArtworkURL, "streamUrl": t.Info.URI, "isrc": t.Info.ISRC,
		"extra": map[string]any{"encoded": t.Encoded, "sourceName": t.Info.SourceName},
	}
	if t.Info.SourceName == "youtube" {
		m["videoId"] = t.Info.Identifier
	}
	if t.Info.IsStream {
		m["durationMs"] = 0
	}
	return m
}

// LoadTracks calls /v4/loadtracks. Identifier may be a URL or "ytsearch:…",
// "ytmsearch:…", "scsearch:…", "spsearch:…" (LavaSrc), "dzsearch:…"…
func (l *Lavalink) LoadTracks(ctx context.Context, identifier string) (map[string]any, error) {
	var raw struct {
		LoadType string          `json:"loadType"`
		Data     json.RawMessage `json:"data"`
	}
	if err := l.get(ctx, "/v4/loadtracks?identifier="+url.QueryEscape(identifier), &raw); err != nil {
		return nil, err
	}
	out := map[string]any{"loadType": raw.LoadType, "tracks": []map[string]any{}}
	var tracks []LavaTrack
	switch raw.LoadType {
	case "track":
		var t LavaTrack
		_ = json.Unmarshal(raw.Data, &t)
		tracks = []LavaTrack{t}
	case "search":
		_ = json.Unmarshal(raw.Data, &tracks)
	case "playlist":
		var pl struct {
			Info struct {
				Name string `json:"name"`
			} `json:"info"`
			Tracks []LavaTrack `json:"tracks"`
		}
		_ = json.Unmarshal(raw.Data, &pl)
		tracks = pl.Tracks
		out["playlist"] = pl.Info.Name
	case "error":
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw.Data, &e)
		return nil, fmt.Errorf("lavalink: %s", e.Message)
	}
	list := make([]map[string]any, 0, len(tracks))
	for _, t := range tracks {
		list = append(list, t.asMap())
	}
	out["tracks"] = list
	return out, nil
}

func lavalinkModule(e *moduleEnv) map[string]any {
	ctx := context.Background()
	return map[string]any{
		"connect": func(opts map[string]any) (map[string]any, error) {
			c, err := NewLavalink(opts, e.host.Client)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"loadTracks": func(id string) (map[string]any, error) { return c.LoadTracks(ctx, id) },
				"search": func(query, prefix string) ([]map[string]any, error) {
					if prefix == "" {
						prefix = "ytsearch"
					}
					r, err := c.LoadTracks(ctx, prefix+":"+query)
					if err != nil {
						return nil, err
					}
					return r["tracks"].([]map[string]any), nil
				},
				"decode": func(encoded string) (map[string]any, error) {
					var t LavaTrack
					if err := c.get(ctx, "/v4/decodetrack?encodedTrack="+url.QueryEscape(encoded), &t); err != nil {
						return nil, err
					}
					t.Encoded = encoded
					return t.asMap(), nil
				},
				"info": func() (map[string]any, error) {
					var m map[string]any
					err := c.get(ctx, "/v4/info", &m)
					return m, err
				},
			}, nil
		},
	}
}
