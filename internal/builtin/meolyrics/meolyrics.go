// Package meolyrics chains the built-in lyrics providers in priority order:
// BetterLyrics, lrc.red, Unison, Paxsenix, Kugou, LRCLIB. It is part of the
// "Built-in" plugin (internal/builtin/integrated).
package meolyrics

import (
	"context"
	"net/http"
	"strings"
	"time"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
	"meofishes/internal/plugin"
)

const ID = "meo-lyrics"

// Manifest describes the plugin in the Plugins page.
var Manifest = plugin.Manifest{
	ID:          ID,
	Name:        "Meo Lyrics",
	Version:     "1.0.0",
	Author:      "meofishes",
	Description: "Word-synced lyrics. Order: BetterLyrics → lrc.red → Unison → Paxsenix → Kugou → LRCLIB. The first provider with word sync wins.",
	Types:       []string{plugin.TypeFnt},
	Runtime:     plugin.RuntimeBuiltin,
	Settings: []plugin.Setting{
		{Key: "order", Label: "Provider order", Type: "list", Default: strings.Join(lyrics.DefaultOrder, ","),
			Description: "Comma-separated IDs. Remove an ID to disable that provider."},
		{Key: "betterlyrics.apiKey", Label: "BetterLyrics: X-API-Key (optional)", Type: "password",
			Description: "Not needed today; only if the API starts requiring a key for uncached lookups."},
		{Key: "paxsenix.apiKey", Label: "Paxsenix: key (optional)", Type: "password"},
		{Key: "paxsenix.baseUrl", Label: "Paxsenix: base URL", Default: "https://lyrics.paxsenix.org"},
		{Key: "paxsenix.country", Label: "Paxsenix: Apple Music search country", Default: "us"},
	},
}

type impl struct {
	host     *plugin.Host
	settings func() map[string]string
	client   *http.Client
}

// New is the plugin factory.
func New(host *plugin.Host, settings func() map[string]string) (any, error) {
	return &impl{host: host, settings: settings, client: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (p *impl) Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	s := p.settings()
	per := map[string]map[string]string{}
	for k, v := range s {
		if i := strings.IndexByte(k, '.'); i > 0 {
			id, key := k[:i], k[i+1:]
			if per[id] == nil {
				per[id] = map[string]string{}
			}
			per[id][key] = v
		}
	}
	env := &lyrics.Env{Client: p.client, UserAgent: p.host.UserAgent, Settings: per}
	all := lyrics.Builtins(env)
	var provs []lyrics.Provider
	order := s["order"]
	if strings.TrimSpace(order) == "" {
		order = strings.Join(lyrics.DefaultOrder, ",")
	}
	seen := map[string]bool{}
	for _, id := range strings.Split(order, ",") {
		id = strings.ToLower(strings.TrimSpace(id))
		if pr, ok := all[id]; ok && !seen[id] {
			seen[id] = true
			provs = append(provs, pr)
		}
	}
	chain := &lyrics.Chain{Providers: provs, Timeout: 35 * time.Second}
	l, att, err := chain.Find(ctx, q)
	p.host.Log(ID, "%s — %s: %s", q.Artist, q.Title, lyrics.Describe(att))
	if err != nil {
		return nil, err
	}
	l.Provider = l.Provider + " · Built-in"
	return l, nil
}
