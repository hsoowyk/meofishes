// Package library stores the user's playlists, liked songs and listening
// history in ~/.local/share/meofishes/library.json, and reads/writes the
// .mepw playlist format (see docs/playlists.md).
package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meofishes/internal/core"
)

// LikedID is the id of the "Liked songs" playlist.
const LikedID = "liked"

// Playlist kinds.
const (
	KindLocal  = "local"  // made in meofishes
	KindLiked  = "liked"  // liked songs
	KindRemote = "remote" // copied from a src plugin (can be refreshed)
)

// Playlist is an ordered list of tracks.
type Playlist struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Cover       string       `json:"cover,omitempty"`
	Kind        string       `json:"kind"`
	Source      string       `json:"source,omitempty"`   // remote: plugin id
	RemoteID    string       `json:"remoteId,omitempty"` // remote: collection id in the plugin
	Pinned      bool         `json:"pinned,omitempty"`
	Tracks      []core.Track `json:"tracks"`
	Created     time.Time    `json:"created"`
	Updated     time.Time    `json:"updated"`
}

// CoverURL is the playlist cover or the cover of its first track.
func (p *Playlist) CoverURL() string {
	if p.Cover != "" {
		return p.Cover
	}
	for _, t := range p.Tracks {
		if t.Cover != "" {
			return t.Cover
		}
	}
	return ""
}

// DurationMs is the sum of the known track durations.
func (p *Playlist) DurationMs() int64 {
	var d int64
	for _, t := range p.Tracks {
		d += t.DurationMs
	}
	return d
}

// HistoryItem is one played track.
type HistoryItem struct {
	Track core.Track `json:"track"`
	At    time.Time  `json:"at"`
}

// Library is the persisted collection. Methods are safe for concurrent use.
type Library struct {
	path      string
	mu        sync.Mutex
	Playlists []*Playlist          `json:"playlists"`
	History   []HistoryItem        `json:"history,omitempty"`
	Stats     map[string]*PlayStat `json:"stats,omitempty"`
	Searches  []string             `json:"searches,omitempty"`
	// KeepHistory=false pauses the history (privacy setting).
	KeepHistory bool `json:"-"`
	listeners   []func()
}

// Open loads (or creates) the library at path.
func Open(path string) *Library {
	l := &Library{path: path, KeepHistory: true}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, l)
	}
	l.ensureLiked()
	return l
}

func (l *Library) ensureLiked() {
	for _, p := range l.Playlists {
		if p.ID == LikedID {
			p.Kind = KindLiked
			return
		}
	}
	now := time.Now()
	l.Playlists = append([]*Playlist{{ID: LikedID, Name: "Liked songs", Kind: KindLiked, Pinned: true, Created: now, Updated: now}}, l.Playlists...)
}

// OnChange registers a callback run after every change (any goroutine).
func (l *Library) OnChange(f func()) { l.listeners = append(l.listeners, f) }

func (l *Library) changed() {
	_ = l.save()
	for _, f := range l.listeners {
		f()
	}
}

func (l *Library) save() error {
	l.mu.Lock()
	data, err := json.MarshalIndent(l, "", " ")
	l.mu.Unlock()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(l.path), 0o755)
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// All returns the playlists (liked first, then pinned, then by update).
func (l *Library) All() []*Playlist {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*Playlist(nil), l.Playlists...)
}

// Get returns a playlist by id.
func (l *Library) Get(id string) *Playlist {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.Playlists {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Liked is the "Liked songs" playlist.
func (l *Library) Liked() *Playlist { return l.Get(LikedID) }

// IsLiked reports whether a track is in the liked songs.
func (l *Library) IsLiked(t core.Track) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.Playlists {
		if p.ID == LikedID {
			for _, x := range p.Tracks {
				if x.Key() == t.Key() {
					return true
				}
			}
		}
	}
	return false
}

// SetLiked adds or removes a track from the liked songs.
func (l *Library) SetLiked(t core.Track, on bool) {
	l.mu.Lock()
	var liked *Playlist
	for _, p := range l.Playlists {
		if p.ID == LikedID {
			liked = p
		}
	}
	idx := -1
	for i, x := range liked.Tracks {
		if x.Key() == t.Key() {
			idx = i
		}
	}
	switch {
	case on && idx < 0:
		liked.Tracks = append([]core.Track{t}, liked.Tracks...) // newest first
	case !on && idx >= 0:
		liked.Tracks = append(liked.Tracks[:idx], liked.Tracks[idx+1:]...)
	default:
		l.mu.Unlock()
		return
	}
	liked.Updated = time.Now()
	l.mu.Unlock()
	l.changed()
}

// Create makes a new local playlist.
func (l *Library) Create(name string, tracks ...core.Track) *Playlist {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Playlist"
	}
	now := time.Now()
	p := &Playlist{ID: newID(), Name: name, Kind: KindLocal, Tracks: append([]core.Track(nil), tracks...), Created: now, Updated: now}
	l.mu.Lock()
	l.Playlists = append(l.Playlists, p)
	l.mu.Unlock()
	l.changed()
	return p
}

// Update runs f on a playlist and saves it.
func (l *Library) Update(id string, f func(p *Playlist)) error {
	l.mu.Lock()
	var p *Playlist
	for _, x := range l.Playlists {
		if x.ID == id {
			p = x
		}
	}
	if p == nil {
		l.mu.Unlock()
		return errors.New("playlist not found")
	}
	f(p)
	p.Updated = time.Now()
	l.mu.Unlock()
	l.changed()
	return nil
}

// Add appends tracks (skipping ones already there).
func (l *Library) Add(id string, ts ...core.Track) error {
	return l.Update(id, func(p *Playlist) {
		have := map[string]bool{}
		for _, t := range p.Tracks {
			have[t.Key()] = true
		}
		for _, t := range ts {
			if !have[t.Key()] {
				p.Tracks = append(p.Tracks, t)
				have[t.Key()] = true
			}
		}
	})
}

// RemoveAt removes track i.
func (l *Library) RemoveAt(id string, i int) error {
	return l.Update(id, func(p *Playlist) {
		if i >= 0 && i < len(p.Tracks) {
			p.Tracks = append(p.Tracks[:i], p.Tracks[i+1:]...)
		}
	})
}

// Move moves track from → to.
func (l *Library) Move(id string, from, to int) error {
	return l.Update(id, func(p *Playlist) { p.Tracks = MoveItem(p.Tracks, from, to) })
}

// MoveItem moves s[from] to index to (used by the queue too).
func MoveItem[T any](s []T, from, to int) []T {
	if from < 0 || from >= len(s) || to < 0 || to >= len(s) || from == to {
		return s
	}
	x := s[from]
	s = append(s[:from], s[from+1:]...)
	s = append(s[:to], append([]T{x}, s[to:]...)...)
	return s
}

// Delete removes a playlist (the liked songs cannot be deleted).
func (l *Library) Delete(id string) error {
	if id == LikedID {
		return errors.New("the liked songs cannot be deleted")
	}
	l.mu.Lock()
	for i, p := range l.Playlists {
		if p.ID == id {
			l.Playlists = append(l.Playlists[:i], l.Playlists[i+1:]...)
			l.mu.Unlock()
			l.changed()
			return nil
		}
	}
	l.mu.Unlock()
	return errors.New("playlist not found")
}

// SaveRemote stores (or refreshes) a playlist copied from a src plugin.
func (l *Library) SaveRemote(c core.Collection) *Playlist {
	l.mu.Lock()
	var p *Playlist
	for _, x := range l.Playlists {
		if x.Kind == KindRemote && x.Source == c.Source && x.RemoteID == c.ID {
			p = x
		}
	}
	now := time.Now()
	if p == nil {
		p = &Playlist{ID: newID(), Kind: KindRemote, Source: c.Source, RemoteID: c.ID, Created: now}
		l.Playlists = append(l.Playlists, p)
	}
	p.Name, p.Description, p.Cover, p.Tracks, p.Updated = c.Title, c.Sub, c.Cover, c.Tracks, now
	l.mu.Unlock()
	l.changed()
	return p
}

// AddHistory records a played track (keeps the last 300).
func (l *Library) AddHistory(t core.Track) {
	if !l.KeepHistory {
		return
	}
	l.mu.Lock()
	if n := len(l.History); n > 0 && l.History[n-1].Track.Key() == t.Key() {
		l.mu.Unlock()
		return
	}
	l.History = append(l.History, HistoryItem{Track: t, At: time.Now()})
	if len(l.History) > 300 {
		l.History = l.History[len(l.History)-300:]
	}
	l.mu.Unlock()
	_ = l.save()
}

// Recent returns up to n distinct recently played tracks, newest first.
func (l *Library) Recent(n int) []core.Track {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []core.Track
	for i := len(l.History) - 1; i >= 0 && len(out) < n; i-- {
		t := l.History[i].Track
		if !seen[t.Key()] {
			seen[t.Key()] = true
			out = append(out, t)
		}
	}
	return out
}

// ClearHistory forgets the listening history.
func (l *Library) ClearHistory() {
	l.mu.Lock()
	l.History = nil
	l.Stats = nil
	l.Searches = nil
	l.mu.Unlock()
	l.changed()
}

// ---------------------------------------------------------------- statistics

// PlayStat counts how much a song was listened to.
type PlayStat struct {
	Track core.Track `json:"track"`
	Plays int        `json:"plays"`
	Ms    int64      `json:"ms"`
	Last  time.Time  `json:"last"`
}

// RecordListen adds listening time to a song; it counts as a play after
// 30 seconds or half of the song.
func (l *Library) RecordListen(t core.Track, listenedMs int64) {
	if !l.KeepHistory || listenedMs < 5000 || t.Title == "" {
		return
	}
	l.mu.Lock()
	if l.Stats == nil {
		l.Stats = map[string]*PlayStat{}
	}
	st := l.Stats[t.Key()]
	if st == nil {
		st = &PlayStat{}
		l.Stats[t.Key()] = st
	}
	st.Track = t
	st.Ms += listenedMs
	if listenedMs >= 30000 || (t.DurationMs > 0 && listenedMs*2 >= t.DurationMs) {
		st.Plays++
	}
	st.Last = time.Now()
	l.mu.Unlock()
	_ = l.save()
}

// Ranked is one row of a top list.
type Ranked struct {
	Name  string
	Track *core.Track // nil for artists
	Plays int
	Ms    int64
}

// Top returns the most played songs and artists since a time (zero = ever),
// and the total listening time.
func (l *Library) Top(since time.Time, n int) (songs, artists []Ranked, total time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	byArtist := map[string]*Ranked{}
	for _, st := range l.Stats {
		if !since.IsZero() && st.Last.Before(since) {
			continue
		}
		t := st.Track
		total += time.Duration(st.Ms) * time.Millisecond
		songs = append(songs, Ranked{Name: t.String(), Track: &t, Plays: st.Plays, Ms: st.Ms})
		a := strings.TrimSpace(t.Artist)
		if a == "" {
			continue
		}
		r := byArtist[strings.ToLower(a)]
		if r == nil {
			r = &Ranked{Name: a}
			byArtist[strings.ToLower(a)] = r
		}
		r.Plays += st.Plays
		r.Ms += st.Ms
	}
	for _, r := range byArtist {
		artists = append(artists, *r)
	}
	less := func(s []Ranked) func(i, j int) bool {
		return func(i, j int) bool {
			if s[i].Plays != s[j].Plays {
				return s[i].Plays > s[j].Plays
			}
			return s[i].Ms > s[j].Ms
		}
	}
	sort.Slice(songs, less(songs))
	sort.Slice(artists, less(artists))
	if len(songs) > n {
		songs = songs[:n]
	}
	if len(artists) > n {
		artists = artists[:n]
	}
	return
}

// ---------------------------------------------------------------- searches

// AddSearch remembers a query (the last 20, newest first).
func (l *Library) AddSearch(q string) {
	q = strings.TrimSpace(q)
	if q == "" || !l.KeepHistory {
		return
	}
	l.mu.Lock()
	out := []string{q}
	for _, s := range l.Searches {
		if !strings.EqualFold(s, q) && len(out) < 20 {
			out = append(out, s)
		}
	}
	l.Searches = out
	l.mu.Unlock()
	_ = l.save()
}

// RecentSearches returns the remembered queries.
func (l *Library) RecentSearches() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.Searches...)
}

// ClearSearches forgets the queries.
func (l *Library) ClearSearches() {
	l.mu.Lock()
	l.Searches = nil
	l.mu.Unlock()
	_ = l.save()
}
