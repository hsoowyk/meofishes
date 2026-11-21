package library

import (
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"

	"meofishes/internal/core"
)

// ---------------------------------------------------------------- hidden

// SetHidden hides a song from autoplay, radio and mixes (or shows it again).
func (l *Library) SetHidden(t core.Track, on bool) {
	l.mu.Lock()
	if l.Hidden == nil {
		l.Hidden = map[string]core.Track{}
	}
	if on {
		l.Hidden[t.Key()] = t
	} else {
		delete(l.Hidden, t.Key())
		delete(l.Hidden, SongKey(t))
	}
	if on {
		l.Hidden[SongKey(t)] = t // the same song from another source too
	}
	l.mu.Unlock()
	l.changed()
}

// IsHidden reports whether a song is hidden.
func (l *Library) IsHidden(t core.Track) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Hidden == nil {
		return false
	}
	_, a := l.Hidden[t.Key()]
	_, b := l.Hidden[SongKey(t)]
	return a || b
}

// HiddenTracks lists the hidden songs.
func (l *Library) HiddenTracks() []core.Track {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []core.Track
	for _, t := range l.Hidden {
		if !seen[t.Key()] {
			seen[t.Key()] = true
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// ---------------------------------------------------------------- ratings

// SetRating gives a song 1–5 stars (0 removes the rating).
func (l *Library) SetRating(t core.Track, stars int) {
	l.mu.Lock()
	if l.Ratings == nil {
		l.Ratings = map[string]int{}
	}
	if stars <= 0 {
		delete(l.Ratings, SongKey(t))
	} else {
		l.Ratings[SongKey(t)] = min(stars, 5)
	}
	if stars > 0 { // keep the track so "Top rated" can play it
		if l.Stats == nil {
			l.Stats = map[string]*PlayStat{}
		}
		if l.Stats[t.Key()] == nil {
			l.Stats[t.Key()] = &PlayStat{Track: t}
		}
	}
	l.mu.Unlock()
	l.changed()
}

// Rating returns a song's stars (0 = none).
func (l *Library) Rating(t core.Track) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Ratings[SongKey(t)]
}

// ---------------------------------------------------------------- skips

// RecordSkip counts a song skipped in its first seconds.
func (l *Library) RecordSkip(t core.Track) {
	if !l.KeepHistory || t.Title == "" {
		return
	}
	l.mu.Lock()
	if l.Stats == nil {
		l.Stats = map[string]*PlayStat{}
	}
	st := l.Stats[t.Key()]
	if st == nil {
		st = &PlayStat{Track: t}
		l.Stats[t.Key()] = st
	}
	st.Skips++
	l.mu.Unlock()
	_ = l.save()
}

// Unwanted reports songs autoplay should not pick: hidden, or skipped at
// least 3 times and skipped more often than played.
func (l *Library) Unwanted(t core.Track) bool {
	if l.IsHidden(t) {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if st := l.Stats[t.Key()]; st != nil && st.Skips >= 3 && st.Skips > st.Plays*2 {
		return true
	}
	return false
}

// ---------------------------------------------------------------- matching

var keyJunk = regexp.MustCompile(`(?i)\s*[\(\[](official|video|audio|lyrics?|letra|hd|4k|remaster(ed)?|visuali[sz]er|mv|clip)[^\)\]]*[\)\]]`)
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// SongKey identifies a song across sources: normalized title + first artist.
func SongKey(t core.Track) string {
	title := keyJunk.ReplaceAllString(t.Title, "")
	artist := t.Artist
	for _, sep := range []string{",", "&", " feat", " ft.", " x ", " e "} {
		if i := strings.Index(strings.ToLower(artist), sep); i > 0 {
			artist = artist[:i]
		}
	}
	artist = strings.TrimSuffix(strings.TrimSpace(artist), " - Topic")
	n := func(s string) string { return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(s), " ")) }
	return n(title) + "|" + n(artist)
}

// Duplicates returns the indexes of songs that already appeared earlier.
func Duplicates(ts []core.Track) []int {
	seen := map[string]bool{}
	var out []int
	for i, t := range ts {
		k := SongKey(t)
		if seen[k] {
			out = append(out, i)
		}
		seen[k] = true
	}
	return out
}

// RemoveDuplicates removes repeated songs from a playlist; returns how many.
func (l *Library) RemoveDuplicates(id string) int {
	n := 0
	_ = l.Update(id, func(p *Playlist) {
		dup := map[int]bool{}
		for _, i := range Duplicates(p.Tracks) {
			dup[i] = true
		}
		var keep []core.Track
		for i, t := range p.Tracks {
			if !dup[i] {
				keep = append(keep, t)
			}
		}
		n = len(p.Tracks) - len(keep)
		p.Tracks = keep
	})
	return n
}

// Merge creates a playlist with the songs of several (without repeats).
func (l *Library) Merge(name string, ids ...string) *Playlist {
	var all []core.Track
	seen := map[string]bool{}
	for _, id := range ids {
		if p := l.Get(id); p != nil {
			for _, t := range p.Tracks {
				if k := SongKey(t); !seen[k] {
					seen[k] = true
					all = append(all, t)
				}
			}
		}
	}
	return l.Create(name, all...)
}

// ---------------------------------------------------------------- shuffle and mixes

// SmartShuffle shuffles so the same artist does not play twice in a row
// when it can be avoided. Each step picks a random artist (weighted by how
// many songs it has left) other than the last one, but must take the
// biggest artist when it would otherwise be left with neighbours.
func SmartShuffle(ts []core.Track) {
	groups := map[string][]core.Track{}
	var names []string
	for _, t := range ts {
		a := strings.SplitN(SongKey(t), "|", 2)[1]
		if _, ok := groups[a]; !ok {
			names = append(names, a)
		}
		groups[a] = append(groups[a], t)
	}
	for _, g := range groups {
		rand.Shuffle(len(g), func(i, j int) { g[i], g[j] = g[j], g[i] })
	}
	last := "\x00"
	for i := range ts {
		left := len(ts) - i
		pick, biggest := "", ""
		for _, n := range names {
			if len(groups[n]) > 0 && (biggest == "" || len(groups[n]) > len(groups[biggest])) {
				biggest = n
			}
		}
		if biggest != last && len(groups[biggest])*2 > left {
			pick = biggest
		} else {
			total := 0
			for _, n := range names {
				if n != last {
					total += len(groups[n])
				}
			}
			if total == 0 {
				pick = last // only this artist is left
			} else {
				r := rand.Intn(total)
				for _, n := range names {
					if n == last {
						continue
					}
					if r -= len(groups[n]); r < 0 {
						pick = n
						break
					}
				}
			}
		}
		ts[i] = groups[pick][0]
		groups[pick] = groups[pick][1:]
		last = pick
	}
}

// Smart lists. The ids are used by the UI.
const (
	SmartTopRated  = "smart:top-rated"
	SmartMostPlay  = "smart:most-played"
	SmartForgotten = "smart:forgotten" // liked or loved, not played for 30 days
	SmartSkipped   = "smart:skipped"
	SmartHidden    = "smart:hidden"
)

// SmartTracks builds a smart list.
func (l *Library) SmartTracks(id string) []core.Track {
	l.mu.Lock()
	stats := make([]*PlayStat, 0, len(l.Stats))
	for _, st := range l.Stats {
		stats = append(stats, st)
	}
	ratings := l.Ratings
	l.mu.Unlock()
	var out []core.Track
	switch id {
	case SmartTopRated:
		sort.Slice(stats, func(i, j int) bool {
			ri, rj := ratings[SongKey(stats[i].Track)], ratings[SongKey(stats[j].Track)]
			if ri != rj {
				return ri > rj
			}
			return stats[i].Plays > stats[j].Plays
		})
		for _, st := range stats {
			if ratings[SongKey(st.Track)] >= 4 {
				out = append(out, st.Track)
			}
		}
	case SmartMostPlay:
		sort.Slice(stats, func(i, j int) bool { return stats[i].Plays > stats[j].Plays })
		for _, st := range stats {
			if st.Plays >= 2 && len(out) < 100 {
				out = append(out, st.Track)
			}
		}
	case SmartSkipped:
		sort.Slice(stats, func(i, j int) bool { return stats[i].Skips > stats[j].Skips })
		for _, st := range stats {
			if st.Skips >= 2 {
				out = append(out, st.Track)
			}
		}
	case SmartForgotten:
		cut := time.Now().AddDate(0, 0, -30)
		last := map[string]time.Time{}
		for _, st := range stats {
			last[st.Track.Key()] = st.Last
			if st.Plays >= 3 && st.Last.Before(cut) {
				out = append(out, st.Track)
			}
		}
		if lk := l.Liked(); lk != nil {
			for _, t := range lk.Tracks {
				if at, ok := last[t.Key()]; !ok || at.Before(cut) {
					out = append(out, t)
				}
			}
		}
		out = dedupe(out)
	case SmartHidden:
		return l.HiddenTracks()
	}
	return out
}

func dedupe(ts []core.Track) []core.Track {
	seen := map[string]bool{}
	out := ts[:0]
	for _, t := range ts {
		if k := SongKey(t); !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	return out
}

// Surprise builds a mix: liked songs, top songs and good ratings, recently
// played ones less likely, without hidden or often-skipped songs.
func (l *Library) Surprise(n int) []core.Track {
	var pool []core.Track
	if lk := l.Liked(); lk != nil {
		pool = append(pool, lk.Tracks...)
	}
	pool = append(pool, l.SmartTracks(SmartTopRated)...)
	pool = append(pool, l.SmartTracks(SmartMostPlay)...)
	pool = append(pool, l.SmartTracks(SmartForgotten)...) // twice the chance for forgotten ones
	recent := map[string]bool{}
	for _, t := range l.Recent(15) {
		recent[SongKey(t)] = true
	}
	var out []core.Track
	for _, t := range dedupe(pool) {
		if !l.Unwanted(t) && !recent[SongKey(t)] {
			out = append(out, t)
		}
	}
	SmartShuffle(out)
	if len(out) > n {
		out = out[:n]
	}
	return out
}
