// Package integrated is the "Built-in" plugin: everything meofishes does by
// itself in one plugin — word-synced lyrics (BetterLyrics, lrc.red, Unison,
// Paxsenix, Kugou, LRCLIB), album covers (iTunes, Deezer, Piped) and your
// music folders. Its settings are prefixed: lyrics.*, artwork.*, local.*.
package integrated

import (
	"context"
	"strings"

	"meofishes/internal/builtin/artwork"
	"meofishes/internal/builtin/localfiles"
	"meofishes/internal/builtin/meolyrics"
	"meofishes/internal/core"
	"meofishes/internal/lyrics"
	"meofishes/internal/plugin"
)

// ID of the plugin. The ids of the plugins it replaced are aliases.
const ID = "builtin"

// Aliases are the old ids (0.2.0): tracks, settings and priorities that use
// them keep working.
var Aliases = []string{meolyrics.ID, artwork.ID, "local-files"}

func prefixed(prefix string, ss []plugin.Setting) []plugin.Setting {
	out := make([]plugin.Setting, len(ss))
	for i, s := range ss {
		s.Key = prefix + s.Key
		out[i] = s
	}
	return out
}

// Manifest describes the plugin.
var Manifest = plugin.Manifest{
	ID:          ID,
	Name:        "Built-in",
	Version:     "1.0.0",
	Author:      "meofishes",
	Description: "Everything meofishes does by itself: word-synced lyrics (BetterLyrics, lrc.red, Unison, Paxsenix, Kugou, LRCLIB), album covers (iTunes, Deezer, Piped) and the music in your folders.",
	Types:       []string{plugin.TypeFnt, plugin.TypeSrc},
	Runtime:     plugin.RuntimeBuiltin,
	Aliases:     Aliases,
	Settings: append(append(prefixed("lyrics.", meolyrics.Manifest.Settings),
		prefixed("artwork.", artwork.Manifest.Settings)...), prefixed("local.", localfiles.Manifest.Settings)...),
}

type impl struct {
	lyr   plugin.LyricsSource
	art   plugin.ArtworkSource
	local any
}

// sub returns the settings of one part, without its prefix.
func sub(settings func() map[string]string, prefix string) func() map[string]string {
	return func() map[string]string {
		out := map[string]string{}
		for k, v := range settings() {
			if rest, ok := strings.CutPrefix(k, prefix); ok {
				out[rest] = v
			}
		}
		return out
	}
}

// New is the plugin factory.
func New(host *plugin.Host, settings func() map[string]string) (any, error) {
	l, err := meolyrics.New(host, sub(settings, "lyrics."))
	if err != nil {
		return nil, err
	}
	a, err := artwork.New(host, sub(settings, "artwork."))
	if err != nil {
		return nil, err
	}
	lf, err := localfiles.New(host, sub(settings, "local."))
	if err != nil {
		return nil, err
	}
	return &impl{lyr: l.(plugin.LyricsSource), art: a.(plugin.ArtworkSource), local: lf}, nil
}

func (p *impl) Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	return p.lyr.Lyrics(ctx, q)
}

func (p *impl) Artwork(ctx context.Context, q lyrics.Query) (string, error) {
	return p.art.Artwork(ctx, q)
}

func (p *impl) Search(ctx context.Context, q string) (*core.SearchResult, error) {
	return p.local.(plugin.Searcher).Search(ctx, q)
}

func (p *impl) Resolve(ctx context.Context, t core.Track) (*plugin.Stream, error) {
	return p.local.(plugin.Resolver).Resolve(ctx, t)
}

func (p *impl) Home(ctx context.Context) (*core.SearchResult, error) {
	if b, ok := p.local.(plugin.Browser); ok {
		return b.Home(ctx)
	}
	return &core.SearchResult{}, nil
}

func (p *impl) Open(ctx context.Context, c core.Collection) (*core.Collection, error) {
	if b, ok := p.local.(plugin.Browser); ok {
		return b.Open(ctx, c)
	}
	return &c, nil
}
