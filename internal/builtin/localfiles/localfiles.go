// Package localfiles is a builtin source plugin for music on disk.
package localfiles

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dhowden/tag"

	"meofishes/internal/core"
	"meofishes/internal/plugin"
)

const ID = "local-files"

var Manifest = plugin.Manifest{
	ID:          ID,
	Name:        "Arquivos locais",
	Version:     "1.0.0",
	Author:      "meofishes",
	Description: "Toca as músicas das suas pastas (mp3, flac, ogg, opus, m4a, wav…), lendo as tags e capas embutidas.",
	Types:       []string{plugin.TypeSource},
	Runtime:     plugin.RuntimeBuiltin,
	Settings: []plugin.Setting{
		{Key: "folders", Label: "Pastas de música", Type: "list", Default: defaultFolders(),
			Description: "Separe várias pastas com ';'."},
	},
}

func defaultFolders() string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, n := range []string{"Music", "Músicas", "Musicas"} {
		p := filepath.Join(home, n)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, filepath.Join(home, "Music"))
	}
	return strings.Join(out, ";")
}

var audioExt = map[string]bool{".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".m4a": true,
	".aac": true, ".wav": true, ".wma": true, ".alac": true, ".aiff": true, ".ape": true, ".mka": true, ".webm": true}

type impl struct {
	host     *plugin.Host
	settings func() map[string]string
	coverDir string

	mu      sync.Mutex
	scanned string // folders the index was built for
	tracks  []core.Track
}

func New(host *plugin.Host, settings func() map[string]string) (any, error) {
	return &impl{host: host, settings: settings, coverDir: filepath.Join(host.DataDir, ID, "covers")}, nil
}

func (p *impl) index() []core.Track {
	folders := p.settings()["folders"]
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scanned == folders && p.tracks != nil {
		return p.tracks
	}
	var list []core.Track
	for _, root := range strings.Split(folders, ";") {
		root = strings.TrimSpace(root)
		if strings.HasPrefix(root, "~/") {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, root[2:])
		}
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !audioExt[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			list = append(list, p.read(path))
			return nil
		})
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if !strings.EqualFold(a.Artist, b.Artist) {
			return strings.ToLower(a.Artist) < strings.ToLower(b.Artist)
		}
		if a.Album != b.Album {
			return a.Album < b.Album
		}
		return a.Extra["trackNo"] < b.Extra["trackNo"]
	})
	p.tracks, p.scanned = list, folders
	p.host.Log(ID, "indexed %d files", len(list))
	return list
}

func (p *impl) read(path string) core.Track {
	t := core.Track{ID: path, Source: ID, StreamURL: path, Extra: map[string]string{}}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	t.Title = base
	if i := strings.Index(base, " - "); i > 0 {
		t.Artist, t.Title = strings.TrimSpace(base[:i]), strings.TrimSpace(base[i+3:])
	}
	f, err := os.Open(path)
	if err != nil {
		return t
	}
	defer f.Close()
	md, err := tag.ReadFrom(f)
	if err != nil {
		return t
	}
	if s := strings.TrimSpace(md.Title()); s != "" {
		t.Title = s
	}
	if s := strings.TrimSpace(md.Artist()); s != "" {
		t.Artist = s
	} else if s := strings.TrimSpace(md.AlbumArtist()); s != "" {
		t.Artist = s
	}
	t.Album = md.Album()
	n, _ := md.Track()
	t.Extra["trackNo"] = strings.Repeat("0", 4-len(itoa(n))) + itoa(n)
	if pic := md.Picture(); pic != nil && len(pic.Data) > 0 {
		h := sha1.Sum(pic.Data)
		name := hex.EncodeToString(h[:8]) + "." + strings.TrimPrefix(strings.ToLower(pic.Ext), ".")
		if pic.Ext == "" {
			name += "jpg"
		}
		cp := filepath.Join(p.coverDir, name)
		if _, err := os.Stat(cp); err != nil {
			_ = os.MkdirAll(p.coverDir, 0o755)
			_ = os.WriteFile(cp, pic.Data, 0o644)
		}
		t.Cover = cp
	} else {
		for _, n := range []string{"cover.jpg", "cover.png", "folder.jpg", "front.jpg", "Cover.jpg", "Folder.jpg"} {
			if c := filepath.Join(filepath.Dir(path), n); fileExists(c) {
				t.Cover = c
				break
			}
		}
	}
	return t
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (p *impl) Search(ctx context.Context, q string) (*core.SearchResult, error) {
	words := strings.Fields(strings.ToLower(q))
	res := &core.SearchResult{}
	for _, t := range p.index() {
		hay := strings.ToLower(t.Title + " " + t.Artist + " " + t.Album + " " + filepath.Base(t.ID))
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			res.Tracks = append(res.Tracks, t)
		}
		if len(res.Tracks) >= 300 {
			break
		}
	}
	return res, nil
}

func (p *impl) Home(ctx context.Context) (*core.SearchResult, error) {
	all := p.index()
	if len(all) > 300 {
		all = all[:300]
	}
	return &core.SearchResult{Tracks: all}, nil
}

func (p *impl) Open(ctx context.Context, c core.Collection) (*core.Collection, error) { return &c, nil }

func (p *impl) Resolve(ctx context.Context, t core.Track) (*plugin.Stream, error) {
	return &plugin.Stream{URL: t.StreamURL}, nil
}

// SettingsChanged forces a rescan.
func (p *impl) SettingsChanged() {
	p.mu.Lock()
	p.tracks = nil
	p.mu.Unlock()
}
