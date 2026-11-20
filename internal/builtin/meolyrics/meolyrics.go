// Package meolyrics is the default lyrics plugin. It chains the built-in
// providers in priority order: Lyprio, BetterLyrics, lrc.red, Unison,
// Paxsenix, Kugou.
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
	Description: "Letras sincronizadas por palavra. Ordem: Lyprio → BetterLyrics → lrc.red → Unison → Paxsenix → Kugou. Vence o primeiro provedor com sincronia por palavra.",
	Types:       []string{plugin.TypeLyrics},
	Runtime:     plugin.RuntimeBuiltin,
	Settings: []plugin.Setting{
		{Key: "order", Label: "Ordem dos provedores", Type: "list", Default: strings.Join(lyrics.DefaultOrder, ","),
			Description: "IDs separados por vírgula. Remova um ID para desativar o provedor."},
		{Key: "lyprio.baseUrl", Label: "Lyprio: URL base", Description: "Lyprio é uma API assinada. Peça a URL e a chave aos mantenedores do Lyprio."},
		{Key: "lyprio.signingKey", Label: "Lyprio: chave de assinatura (base64, 32 bytes)", Type: "password"},
		{Key: "betterlyrics.apiKey", Label: "BetterLyrics: X-API-Key (opcional)", Type: "password",
			Description: "Não é necessária hoje; só se a API passar a exigir chave para buscas fora do cache."},
		{Key: "paxsenix.apiKey", Label: "Paxsenix: chave (opcional)", Type: "password"},
		{Key: "paxsenix.baseUrl", Label: "Paxsenix: URL base", Default: "https://lyrics.paxsenix.org"},
		{Key: "paxsenix.country", Label: "Paxsenix: país da busca Apple Music", Default: "us"},
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
	l.Provider = l.Provider + " (Meo Lyrics)"
	return l, nil
}
