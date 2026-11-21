package library

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"meofishes/internal/core"
)

// MepwVersion is the version of the .mepw format written by this build.
const MepwVersion = 1

// Mepw is the content of a .mepw file: UTF-8 JSON, optionally gzip-compressed.
//
//	{
//	  "mepw": 1,
//	  "generator": "meofishes 0.2.0",
//	  "exported": "2026-11-21T18:00:00Z",
//	  "playlists": [ { "name": "...", "tracks": [ {...}, ... ] } ]
//	}
type Mepw struct {
	Mepw      int            `json:"mepw"`
	Generator string         `json:"generator,omitempty"`
	Exported  time.Time      `json:"exported"`
	Playlists []MepwPlaylist `json:"playlists"`
}

// MepwPlaylist is one playlist inside a .mepw file.
type MepwPlaylist struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Cover       string       `json:"cover,omitempty"`
	Kind        string       `json:"kind,omitempty"` // local | liked | remote
	Source      string       `json:"source,omitempty"`
	RemoteID    string       `json:"remoteId,omitempty"`
	Created     time.Time    `json:"created,omitzero"`
	Tracks      []core.Track `json:"tracks"`
}

// WriteMepw writes playlists as .mepw (gzip when compress is true).
func WriteMepw(w io.Writer, generator string, compress bool, ps ...*Playlist) error {
	doc := Mepw{Mepw: MepwVersion, Generator: generator, Exported: time.Now().UTC()}
	for _, p := range ps {
		doc.Playlists = append(doc.Playlists, MepwPlaylist{Name: p.Name, Description: p.Description, Cover: p.Cover,
			Kind: p.Kind, Source: p.Source, RemoteID: p.RemoteID, Created: p.Created, Tracks: p.Tracks})
	}
	if compress {
		gz := gzip.NewWriter(w)
		gz.Name = "playlist.json"
		if err := json.NewEncoder(gz).Encode(doc); err != nil {
			return err
		}
		return gz.Close()
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// ReadMepw parses a .mepw file (plain or gzip). It also accepts a bare
// playlist object and .m3u/.m3u8 lists of URLs/paths.
func ReadMepw(r io.Reader) (*Mepw, error) {
	br := bufio.NewReader(r)
	if magic, _ := br.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		br = bufio.NewReader(gz)
	}
	data, err := io.ReadAll(io.LimitReader(br, 64<<20))
	if err != nil {
		return nil, err
	}
	trim := bytes.TrimSpace(data)
	if len(trim) > 0 && trim[0] != '{' {
		return readM3U(string(data))
	}
	var doc Mepw
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("not a .mepw file: %w", err)
	}
	if doc.Mepw == 0 { // a single playlist object
		var p MepwPlaylist
		if json.Unmarshal(data, &p) == nil && len(p.Tracks) > 0 {
			doc = Mepw{Mepw: MepwVersion, Playlists: []MepwPlaylist{p}}
		}
	}
	if doc.Mepw > MepwVersion {
		return nil, fmt.Errorf(".mepw version %d is newer than this meofishes (%d)", doc.Mepw, MepwVersion)
	}
	if len(doc.Playlists) == 0 {
		return nil, errors.New("the file has no playlists")
	}
	return &doc, nil
}

func readM3U(s string) (*Mepw, error) {
	p := MepwPlaylist{Name: "Imported", Kind: KindLocal}
	title := ""
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || line == "#EXTM3U":
		case strings.HasPrefix(line, "#PLAYLIST:"):
			p.Name = strings.TrimSpace(strings.TrimPrefix(line, "#PLAYLIST:"))
		case strings.HasPrefix(line, "#EXTINF:"):
			if _, t, ok := strings.Cut(line, ","); ok {
				title = strings.TrimSpace(t)
			}
		case strings.HasPrefix(line, "#"):
		default:
			t := core.Track{ID: line, Source: "local-files", StreamURL: line, Title: title}
			if strings.HasPrefix(line, "http") {
				t.Source = "url"
			}
			if a, b, ok := strings.Cut(title, " - "); ok {
				t.Artist, t.Title = a, b
			}
			if t.Title == "" {
				t.Title = line[strings.LastIndexAny(line, "/\\")+1:]
			}
			p.Tracks = append(p.Tracks, t)
			title = ""
		}
	}
	if len(p.Tracks) == 0 {
		return nil, errors.New("empty playlist")
	}
	return &Mepw{Mepw: MepwVersion, Playlists: []MepwPlaylist{p}}, nil
}

// Import adds the playlists of a .mepw document. A "liked" playlist is merged
// into the liked songs; the others become new local playlists.
func (l *Library) Import(doc *Mepw) []*Playlist {
	var out []*Playlist
	for _, mp := range doc.Playlists {
		if mp.Kind == KindLiked {
			for i := len(mp.Tracks) - 1; i >= 0; i-- {
				l.SetLiked(mp.Tracks[i], true)
			}
			out = append(out, l.Liked())
			continue
		}
		p := l.Create(mp.Name, mp.Tracks...)
		_ = l.Update(p.ID, func(x *Playlist) {
			x.Description, x.Cover = mp.Description, mp.Cover
			if mp.Kind == KindRemote {
				x.Kind, x.Source, x.RemoteID = KindRemote, mp.Source, mp.RemoteID
			}
		})
		out = append(out, p)
	}
	return out
}

// WriteM3U exports a playlist as .m3u8 (stream URLs or paths when known).
func WriteM3U(w io.Writer, p *Playlist) error {
	b := &strings.Builder{}
	b.WriteString("#EXTM3U\n#PLAYLIST:" + p.Name + "\n")
	for _, t := range p.Tracks {
		fmt.Fprintf(b, "#EXTINF:%d,%s\n", t.DurationMs/1000, t.String())
		u := t.StreamURL
		if u == "" {
			u = "meofishes://" + t.Source + "/" + t.ID
		}
		b.WriteString(u + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
