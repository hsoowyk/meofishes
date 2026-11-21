package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
)

// declarative runs plugins that are pure JSON: endpoints + field mapping.
type declarative struct {
	m        *Manifest
	host     *Host
	env      *moduleEnv
	settings func() map[string]string
}

// Caps lists what the JSON blocks provide.
func (d *declarative) Caps() map[string]bool {
	c := map[string]bool{}
	if src := d.m.Source; src != nil {
		c[CapSearch] = src.Search != nil
		c[CapHome] = src.Home != nil
		c[CapRecommend] = src.Recommend != nil
		c[CapStream] = src.Resolver != "match"
		c[CapMatch] = src.Resolver != "match" && src.Search != nil
	}
	c[CapLyrics] = d.m.Lyrics != nil
	c[CapArtwork] = d.m.Artwork != nil
	return c
}

func (d *declarative) vars(extra map[string]any) map[string]any {
	s := map[string]any{}
	for k, v := range d.settings() {
		s[k] = v
	}
	v := map[string]any{
		"settings": s,
		"app":      map[string]any{"version": d.host.Version, "userAgent": d.host.UserAgent},
	}
	for k, x := range extra {
		v[k] = x
	}
	return v
}

func (d *declarative) call(ctx context.Context, base string, common map[string]string, r Request, vars map[string]any) (any, error) {
	u := Render(r.URL, vars, true)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = strings.TrimRight(Render(base, vars, false), "/") + "/" + strings.TrimLeft(u, "/")
	}
	if err := d.env.hostAllowed(u); err != nil {
		return nil, err
	}
	method := strings.ToUpper(r.Method)
	if method == "" {
		method = "GET"
	}
	var body io.Reader
	if r.Body != "" {
		body = strings.NewReader(Render(r.Body, vars, false))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", d.host.UserAgent)
	if r.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, hs := range []map[string]string{common, r.Headers} {
		for k, v := range hs {
			req.Header.Set(k, Render(v, vars, false))
		}
	}
	resp, err := d.host.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, lyrics.ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: http %d", method, u, resp.StatusCode)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return string(data), nil // not JSON: hand back the text
	}
	return out, nil
}

func (d *declarative) listTracks(ctx context.Context, ep *Endpoint, extra map[string]any) (*core.SearchResult, error) {
	src := d.m.Source
	vars := d.vars(extra)
	doc, err := d.call(ctx, src.BaseURL, src.Headers, ep.Request, vars)
	if err != nil {
		return nil, err
	}
	items, _ := Lookup(doc, ep.Items).([]any)
	res := &core.SearchResult{}
	for _, it := range items {
		t := core.Track{Source: d.m.ID, Extra: map[string]string{}}
		for field, expr := range ep.Map {
			v := mapValue(expr, it, vars)
			s := toString(v)
			switch field {
			case "id":
				t.ID = s
			case "title":
				t.Title = s
			case "artist":
				t.Artist = s
			case "album":
				t.Album = s
			case "cover":
				t.Cover = s
			case "streamUrl":
				t.StreamURL = s
			case "isrc":
				t.ISRC = s
			case "videoId":
				t.VideoID = s
			case "durationMs":
				f, _ := strconv.ParseFloat(s, 64)
				t.DurationMs = int64(f)
			case "durationSec":
				f, _ := strconv.ParseFloat(s, 64)
				t.DurationMs = int64(f * 1000)
			default:
				t.Extra[field] = s
			}
		}
		if t.ID == "" {
			t.ID = t.StreamURL
		}
		if t.Title != "" {
			res.Tracks = append(res.Tracks, t)
		}
	}
	return res, nil
}

func (d *declarative) Search(ctx context.Context, q string) (*core.SearchResult, error) {
	if d.m.Source == nil || d.m.Source.Search == nil {
		return nil, fmt.Errorf("plugin has no search endpoint")
	}
	return d.listTracks(ctx, d.m.Source.Search, map[string]any{"query": q})
}

func (d *declarative) Home(ctx context.Context) (*core.SearchResult, error) {
	if d.m.Source == nil || d.m.Source.Home == nil {
		return &core.SearchResult{}, nil
	}
	return d.listTracks(ctx, d.m.Source.Home, nil)
}

func (d *declarative) Recommend(ctx context.Context, seed core.Track) (*core.SearchResult, error) {
	if d.m.Source == nil || d.m.Source.Recommend == nil {
		return &core.SearchResult{}, nil
	}
	return d.listTracks(ctx, d.m.Source.Recommend, trackVars(seed))
}

func lyricsVars(q lyrics.Query) map[string]any {
	return map[string]any{
		"title": q.Title, "artist": q.Artist, "album": q.Album, "isrc": q.ISRC, "videoId": q.VideoID,
		"durationMs": q.DurationMs, "durationSec": q.DurationMs / 1000,
		"cleanTitle": lyrics.CleanTitle(q.Title), "primaryArtist": lyrics.PrimaryArtist(q.Artist),
	}
}

func (d *declarative) Artwork(ctx context.Context, q lyrics.Query) (string, error) {
	aw := d.m.Artwork
	if aw == nil {
		return "", lyrics.ErrNotFound
	}
	doc, err := d.call(ctx, aw.BaseURL, aw.Headers, aw.Request, d.vars(lyricsVars(q)))
	if err != nil {
		return "", err
	}
	u := toString(Lookup(doc, aw.Path))
	if u == "" {
		return "", lyrics.ErrNotFound
	}
	if len(aw.Replace) == 2 {
		u = strings.Replace(u, aw.Replace[0], aw.Replace[1], 1)
	}
	return u, nil
}

func (d *declarative) Open(ctx context.Context, c core.Collection) (*core.Collection, error) {
	return &c, nil
}

func trackVars(t core.Track) map[string]any {
	extra := map[string]any{}
	for k, v := range t.Extra {
		extra[k] = v
	}
	return map[string]any{"track": map[string]any{
		"id": t.ID, "title": t.Title, "artist": t.Artist, "album": t.Album,
		"streamUrl": t.StreamURL, "isrc": t.ISRC, "videoId": t.VideoID, "extra": extra,
	}}
}

func (d *declarative) Resolve(ctx context.Context, t core.Track) (*Stream, error) {
	src := d.m.Source
	if src == nil || src.Resolver == "match" {
		return nil, fmt.Errorf("%s only has metadata", d.m.ID)
	}
	u := t.StreamURL
	if t.Source != d.m.ID { // "match": play another plugin's track by searching it
		if src.Search == nil {
			return nil, fmt.Errorf("%s cannot search", d.m.ID)
		}
		r, err := d.Search(ctx, strings.TrimSpace(t.Artist+" "+t.Title))
		if err != nil {
			return nil, err
		}
		if r == nil || len(r.Tracks) == 0 {
			return nil, lyrics.ErrNotFound
		}
		best := r.Tracks[0]
		u = best.StreamURL
		t = best
	}
	if src.Resolve != nil {
		doc, err := d.call(ctx, src.BaseURL, src.Headers, src.Resolve.Request, d.vars(trackVars(t)))
		if err != nil {
			return nil, err
		}
		u = toString(Lookup(doc, src.Resolve.Path))
	}
	if u == "" {
		return nil, fmt.Errorf("no stream url for %s", t.Title)
	}
	if src.Resolver == "ytdlp" {
		return &Stream{URL: u, Page: true}, nil
	}
	return &Stream{URL: u}, nil
}

func (d *declarative) Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	ly := d.m.Lyrics
	if ly == nil {
		return nil, lyrics.ErrNotFound
	}
	vars := d.vars(lyricsVars(q))
	doc, err := d.call(ctx, ly.BaseURL, ly.Headers, ly.Request, vars)
	if err != nil {
		return nil, err
	}
	content := toString(Lookup(doc, ly.Path))
	if content == "" {
		return nil, lyrics.ErrNotFound
	}
	format := ly.Format
	if ly.FormatPath != "" {
		format = toString(Lookup(doc, ly.FormatPath))
	}
	if format == "auto" {
		format = ""
	}
	l, err := lyrics.Parse(format, content)
	if err != nil {
		return nil, err
	}
	l.Credit = ly.Credit
	return l, nil
}
