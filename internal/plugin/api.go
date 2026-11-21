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
	// Via is the plugin that resolved it (filled by the manager).
	Via string `json:"-"`
	// PageURL keeps the web page a direct URL was extracted from (video).
	PageURL string `json:"-"`
}

// Searcher is a src capability.
type Searcher interface {
	Search(ctx context.Context, query string) (*core.SearchResult, error)
}

// Resolver turns a track into a playable stream (fnt "stream"/"match").
type Resolver interface {
	Resolve(ctx context.Context, t core.Track) (*Stream, error)
}

// Source is the old name of a plugin that searches and resolves.
type Source interface {
	Searcher
	Resolver
}

// Browser lists a home page and opens collections (src "home"/"open").
type Browser interface {
	Home(ctx context.Context) (*core.SearchResult, error)
	Open(ctx context.Context, c core.Collection) (*core.Collection, error)
}

// Recommender lists tracks similar to a seed track (src "recommend").
type Recommender interface {
	Recommend(ctx context.Context, seed core.Track) (*core.SearchResult, error)
}

// PlaylistProvider lists the user's playlists on a service (src "playlists").
// The tracks are loaded with Browser.Open.
type PlaylistProvider interface {
	Playlists(ctx context.Context) ([]core.Collection, error)
}

// LyricsSource is implemented by lyrics plugins (fnt "lyrics").
type LyricsSource interface {
	Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error)
}

// ArtworkSource finds a cover image URL (fnt "artwork").
type ArtworkSource interface {
	Artwork(ctx context.Context, q lyrics.Query) (string, error)
}

// Hook receives player events (evt).
type Hook interface {
	Event(name string, payload any)
}

// capable is implemented by runtimes that know which capabilities the plugin
// really provides (a script that exports search() but not home()…).
type capable interface {
	Caps() map[string]bool
}

// Closer is implemented by runtimes that hold resources (processes…).
type Closer interface{ Close() error }

// Event names sent to hook plugins.
const (
	EventTrackStart  = "track.start" // {track}
	EventTrackEnd    = "track.end"   // {track, positionMs}
	EventPause       = "playback.pause"
	EventResume      = "playback.resume"
	EventSeek        = "playback.seek"   // {positionMs}
	EventLyricsFound = "lyrics.found"    // {track, provider, sync}
	EventThemeChange = "theme.change"    // {theme}
	EventQueue       = "queue.change"    // {size, index}
	EventLike        = "track.like"      // {track, liked}
	EventPlaylist    = "playlist.change" // {id, name, size}
)

// EventNames lists every event, for the docs and the developer mode.
var EventNames = []string{EventTrackStart, EventTrackEnd, EventPause, EventResume, EventSeek,
	EventLyricsFound, EventThemeChange, EventQueue, EventLike, EventPlaylist}

// lyricsAdapter exposes a LyricsSource as a lyrics.Provider so plugin lyrics
// can join the default chain.
type lyricsAdapter struct {
	p   *Plugin
	src LyricsSource
}

func (a lyricsAdapter) ID() string   { return a.p.Manifest.ID }
func (a lyricsAdapter) Name() string { return a.p.Manifest.Name }
func (a lyricsAdapter) Fetch(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	var l *core.Lyrics
	err := a.p.timed(CapLyrics, func() (err error) {
		l, err = a.src.Lyrics(ctx, q)
		return
	})
	return l, err
}

// Log writes a line to the plugin log.
func (h *Host) Log(id, format string, args ...any) { h.logf(id, format, args...) }
