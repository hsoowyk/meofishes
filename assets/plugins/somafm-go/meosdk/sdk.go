//go:build meoplugin

// Package sdk is the Go SDK for meofishes "process" plugins.
//
// A process plugin is a normal Go program. It can import any Go library it
// needs (a Lavalink client, a Spotify API wrapper, a scraper…) and talks to
// meofishes over stdin/stdout. This file only uses the standard library, so
// you can import it or copy it into your plugin.
//
//	func main() {
//		sdk.Serve(sdk.Plugin{
//			Search: func(ctx *sdk.Context, query string) (*sdk.SearchResult, error) {
//				return &sdk.SearchResult{Tracks: []sdk.Track{{ID: "1", Title: "Song", StreamURL: "https://…/song.mp3"}}}, nil
//			},
//		})
//	}
package meosdk

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// Track is a playable item.
type Track struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Artist     string            `json:"artist"`
	Album      string            `json:"album,omitempty"`
	DurationMs int64             `json:"durationMs,omitempty"`
	Cover      string            `json:"cover,omitempty"`
	StreamURL  string            `json:"streamUrl,omitempty"`
	ISRC       string            `json:"isrc,omitempty"`
	VideoID    string            `json:"videoId,omitempty"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// Collection is an album, artist or playlist.
type Collection struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Title  string  `json:"title"`
	Sub    string  `json:"subtitle,omitempty"`
	Cover  string  `json:"cover,omitempty"`
	Tracks []Track `json:"tracks,omitempty"`
}

// SearchResult is returned by Search and Home.
type SearchResult struct {
	Tracks      []Track      `json:"tracks"`
	Collections []Collection `json:"collections,omitempty"`
}

// Stream is a resolved playable URL. Page=true means URL is a web page that
// meofishes should open through yt-dlp.
type Stream struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Page    bool              `json:"page,omitempty"`
}

// LyricsQuery describes the song whose lyrics are wanted.
type LyricsQuery struct {
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	DurationMs int64  `json:"durationMs"`
	ISRC       string `json:"isrc"`
	VideoID    string `json:"videoId"`
}

// Lyrics can be returned raw (Format + Content, parsed by meofishes: ttml,
// lrc, krc, plain) or structured (Lines).
type Lyrics struct {
	Format  string `json:"format,omitempty"`
	Content string `json:"content,omitempty"`
	Credit  string `json:"credit,omitempty"`
	Lines   []Line `json:"lines,omitempty"`
}

// Line / Word are structured lyrics (times in ms).
type Line struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
	Words []Word `json:"words,omitempty"`
}

type Word struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
	Part  bool   `json:"part,omitempty"`
}

// ErrNotFound tells meofishes there is no result (not an error to show).
var ErrNotFound = errors.New("not found")

// Context is passed to every handler.
type Context struct {
	Plugin    string            // plugin id
	Version   string            // meofishes version
	UserAgent string            // User-Agent meofishes uses
	DataDir   string            // a folder the plugin may write to
	Settings  map[string]string // current settings (see plugin.json "settings")
}

// Log writes to the meofishes plugin log.
func (c *Context) Log(format string, args ...any) {
	write(map[string]any{"method": "log", "params": map[string]any{"message": fmt.Sprintf(format, args...)}})
}

// Plugin lists the handlers. Leave unsupported ones nil.
type Plugin struct {
	Search  func(ctx *Context, query string) (*SearchResult, error)
	Home    func(ctx *Context) (*SearchResult, error)
	Open    func(ctx *Context, c Collection) (*Collection, error)
	Resolve func(ctx *Context, t Track) (*Stream, error)
	Lyrics  func(ctx *Context, q LyricsQuery) (*Lyrics, error)
	Event   func(ctx *Context, name string, payload json.RawMessage)
	// Init runs once after meofishes sent the settings.
	Init func(ctx *Context) error
}

var outMu sync.Mutex

func write(v any) {
	data, _ := json.Marshal(v)
	outMu.Lock()
	defer outMu.Unlock()
	os.Stdout.Write(append(data, '\n'))
}

type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Serve runs the plugin until meofishes closes stdin.
func Serve(p Plugin) {
	ctx := &Context{Settings: map[string]string{}}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}
		// requests run concurrently; notifications in order
		if req.ID == 0 {
			handleNotification(p, ctx, req)
			continue
		}
		go handle(p, ctx, req)
	}
}

func handleNotification(p Plugin, ctx *Context, req request) {
	switch req.Method {
	case "event":
		var ev struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(req.Params, &ev) == nil && p.Event != nil {
			p.Event(ctx, ev.Name, ev.Payload)
		}
	case "settings":
		var s map[string]string
		if json.Unmarshal(req.Params, &s) == nil {
			ctx.Settings = s
		}
	}
}

func handle(p Plugin, ctx *Context, req request) {
	var result any
	var err error
	switch req.Method {
	case "initialize":
		var in struct {
			Plugin    string            `json:"plugin"`
			Version   string            `json:"version"`
			UserAgent string            `json:"userAgent"`
			Settings  map[string]string `json:"settings"`
			DataDir   string            `json:"dataDir"`
		}
		_ = json.Unmarshal(req.Params, &in)
		ctx.Plugin, ctx.Version, ctx.UserAgent, ctx.DataDir = in.Plugin, in.Version, in.UserAgent, in.DataDir
		if in.Settings != nil {
			ctx.Settings = in.Settings
		}
		if p.Init != nil {
			err = p.Init(ctx)
		}
		var caps []string
		for name, ok := range map[string]bool{"search": p.Search != nil, "home": p.Home != nil, "open": p.Open != nil,
			"resolve": p.Resolve != nil, "lyrics": p.Lyrics != nil, "event": p.Event != nil} {
			if ok {
				caps = append(caps, name)
			}
		}
		result = map[string]any{"capabilities": caps}
	case "search":
		var in struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(req.Params, &in)
		if p.Search == nil {
			err = errors.New("search not supported")
		} else {
			result, err = p.Search(ctx, in.Query)
		}
	case "home":
		if p.Home != nil {
			result, err = p.Home(ctx)
		}
	case "open":
		var in struct {
			Collection Collection `json:"collection"`
		}
		_ = json.Unmarshal(req.Params, &in)
		if p.Open != nil {
			result, err = p.Open(ctx, in.Collection)
		}
	case "resolve":
		var in struct {
			Track Track `json:"track"`
		}
		_ = json.Unmarshal(req.Params, &in)
		if p.Resolve == nil {
			result = Stream{URL: in.Track.StreamURL}
		} else {
			result, err = p.Resolve(ctx, in.Track)
		}
	case "lyrics":
		var in struct {
			Query LyricsQuery `json:"query"`
		}
		_ = json.Unmarshal(req.Params, &in)
		if p.Lyrics == nil {
			err = ErrNotFound
		} else {
			result, err = p.Lyrics(ctx, in.Query)
		}
	default:
		err = fmt.Errorf("unknown method %q", req.Method)
	}
	resp := map[string]any{"id": req.ID}
	if err != nil {
		resp["error"] = err.Error()
	} else {
		resp["result"] = result
	}
	write(resp)
}
