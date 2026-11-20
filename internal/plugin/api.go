package plugin

import (
	"context"
	"log"
	"net/http"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
)

// Host is what the app gives to plugins.
type Host struct {
	Version   string
	UserAgent string
	DataDir   string // plugin storage goes to DataDir/<plugin id>
	Client    *http.Client
	Logf      func(plugin, format string, args ...any)
}

func (h *Host) logf(id, format string, args ...any) {
	if h.Logf != nil {
		h.Logf(id, format, args...)
		return
	}
	log.Printf("[plugin %s] "+format, append([]any{id}, args...)...)
}

// Stream is a resolved, playable URL.
type Stream struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	// Page=true means URL is a web page that the player should open through
	// yt-dlp (mpv does that natively).
	Page bool `json:"page,omitempty"`
}

// Source is implemented by source plugins.
type Source interface {
	Search(ctx context.Context, query string) (*core.SearchResult, error)
	Resolve(ctx context.Context, t core.Track) (*Stream, error)
}

// Browser is optional: sources that can list a home page and open collections.
type Browser interface {
	Home(ctx context.Context) (*core.SearchResult, error)
	Open(ctx context.Context, c core.Collection) (*core.Collection, error)
}

// LyricsSource is implemented by lyrics plugins.
type LyricsSource interface {
	Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error)
}

// Hook is implemented by hook plugins.
type Hook interface {
	Event(name string, payload any)
}

// Closer is implemented by runtimes that hold resources (processes…).
type Closer interface{ Close() error }

// Event names sent to hook plugins.
const (
	EventTrackStart  = "track.start" // {track}
	EventTrackEnd    = "track.end"   // {track, positionMs}
	EventPause       = "playback.pause"
	EventResume      = "playback.resume"
	EventSeek        = "playback.seek" // {positionMs}
	EventLyricsFound = "lyrics.found"  // {track, provider, sync}
	EventThemeChange = "theme.change"  // {theme}
)

// lyricsAdapter exposes a LyricsSource as a lyrics.Provider so plugin lyrics
// can join the default chain.
type lyricsAdapter struct {
	id, name string
	src      LyricsSource
}

func (a lyricsAdapter) ID() string   { return a.id }
func (a lyricsAdapter) Name() string { return a.name }
func (a lyricsAdapter) Fetch(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	return a.src.Lyrics(ctx, q)
}

// Log writes a line to the plugin log.
func (h *Host) Log(id, format string, args ...any) { h.logf(id, format, args...) }
