// Package core holds the data types shared by the player, the plugins and the UI.
package core

import (
	"fmt"
	"strings"
)

// Track is a playable item returned by a source plugin.
type Track struct {
	ID         string            `json:"id"`     // unique inside its source
	Source     string            `json:"source"` // plugin id that produced it
	Title      string            `json:"title"`
	Artist     string            `json:"artist"`
	Album      string            `json:"album,omitempty"`
	DurationMs int64             `json:"durationMs,omitempty"`
	Cover      string            `json:"cover,omitempty"`     // URL or local path
	StreamURL  string            `json:"streamUrl,omitempty"` // may be empty: resolved on play
	ISRC       string            `json:"isrc,omitempty"`
	VideoID    string            `json:"videoId,omitempty"` // YouTube id, helps lyrics lookup
	Extra      map[string]string `json:"extra,omitempty"`   // free-form data for the plugin
}

// Key identifies a track across sources.
func (t Track) Key() string { return t.Source + ":" + t.ID }

func (t Track) String() string {
	if t.Artist == "" {
		return t.Title
	}
	return t.Artist + " — " + t.Title
}

// Album / Artist / Playlist are browsable collections.
type Collection struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Kind   string  `json:"kind"` // album | artist | playlist
	Title  string  `json:"title"`
	Sub    string  `json:"subtitle,omitempty"`
	Cover  string  `json:"cover,omitempty"`
	Tracks []Track `json:"tracks,omitempty"`
}

// SearchResult is what a source returns for a query.
type SearchResult struct {
	Tracks      []Track      `json:"tracks"`
	Collections []Collection `json:"collections,omitempty"`
}

// SyncType tells how precise a lyric is.
type SyncType int

const (
	SyncNone SyncType = iota // plain text
	SyncLine                 // one timestamp per line
	SyncWord                 // per word / syllable
)

func (s SyncType) String() string {
	switch s {
	case SyncWord:
		return "word"
	case SyncLine:
		return "line"
	}
	return "plain"
}

// Word is one timed word or syllable. Part=true means it glues to the previous
// word without a space (syllables of the same word).
type Word struct {
	Start int64  `json:"start"` // ms
	End   int64  `json:"end"`   // ms
	Text  string `json:"text"`
	Part  bool   `json:"part,omitempty"`
}

// Line is one lyric line.
type Line struct {
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Text       string `json:"text"`
	Words      []Word `json:"words,omitempty"`
	Background []Word `json:"background,omitempty"` // backing vocals
	Agent      string `json:"agent,omitempty"`      // singer id (v1, v2…)
	Opposite   bool   `json:"opposite,omitempty"`   // second singer, drawn on the other side
}

// Lyrics is a full parsed lyric document.
type Lyrics struct {
	Provider string   `json:"provider"`
	Sync     SyncType `json:"sync"`
	Lines    []Line   `json:"lines"`
	Credit   string   `json:"credit,omitempty"` // attribution shown under the lyrics
}

// Normalize fills missing end times and line text, and computes the sync type.
func (l *Lyrics) Normalize() {
	hasWords, hasTimes := false, false
	for i := range l.Lines {
		ln := &l.Lines[i]
		if len(ln.Words) > 0 {
			hasWords = true
			if ln.Start == 0 || ln.Start > ln.Words[0].Start {
				ln.Start = ln.Words[0].Start
			}
			last := ln.Words[len(ln.Words)-1]
			if ln.End < last.End {
				ln.End = last.End
			}
			if strings.TrimSpace(ln.Text) == "" {
				ln.Text = JoinWords(ln.Words)
			}
			for j := range ln.Words {
				w := &ln.Words[j]
				if w.End <= w.Start {
					if j+1 < len(ln.Words) {
						w.End = ln.Words[j+1].Start
					} else {
						w.End = ln.End
					}
				}
			}
		}
		if ln.Start > 0 {
			hasTimes = true
		}
	}
	for i := range l.Lines {
		if l.Lines[i].End <= l.Lines[i].Start {
			if i+1 < len(l.Lines) {
				l.Lines[i].End = l.Lines[i+1].Start
			} else {
				l.Lines[i].End = l.Lines[i].Start + 5000
			}
		}
	}
	switch {
	case hasWords:
		l.Sync = SyncWord
	case hasTimes:
		l.Sync = SyncLine
	default:
		l.Sync = SyncNone
	}
}

// JoinWords rebuilds the text of a line from its words.
func JoinWords(ws []Word) string {
	var b strings.Builder
	for i, w := range ws {
		if i > 0 && !w.Part && !strings.HasPrefix(w.Text, " ") && !strings.HasSuffix(ws[i-1].Text, " ") {
			b.WriteByte(' ')
		}
		b.WriteString(w.Text)
	}
	return strings.TrimSpace(b.String())
}

// FormatMs renders 83456 as "1:23".
func FormatMs(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, (s/60)%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
