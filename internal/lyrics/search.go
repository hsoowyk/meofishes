package lyrics

import (
	"context"
	"net/url"
	"strings"
)

// Hit is a song found by a line of its lyrics.
type Hit struct {
	Title, Artist, Album string
	DurationMs           int64
	Cover                string
	Snippet              string // the matching line
}

// SearchByLyrics finds songs whose lyrics contain text: Genius' lyric
// search first (it searches inside the lyrics), then LRCLIB (titles).
func SearchByLyrics(ctx context.Context, e *Env, text string) ([]Hit, error) {
	hits, err := geniusLyricSearch(ctx, e, text)
	if len(hits) > 0 {
		return hits, nil
	}
	more, err2 := lrclibSearch(ctx, e, text)
	if len(more) == 0 && err != nil {
		return nil, err
	}
	return more, err2
}

func geniusLyricSearch(ctx context.Context, e *Env, text string) ([]Hit, error) {
	var res struct {
		Response struct {
			Sections []struct {
				Hits []struct {
					Highlights []struct {
						Value string `json:"value"`
					} `json:"highlights"`
					Result struct {
						Title       string `json:"title"`
						ArtistNames string `json:"artist_names"`
						Art         string `json:"song_art_image_url"`
					} `json:"result"`
				} `json:"hits"`
			} `json:"sections"`
		} `json:"response"`
	}
	if _, err := e.getJSON(ctx, "https://genius.com/api/search/lyric?per_page=20&q="+url.QueryEscape(text), nil, &res); err != nil {
		return nil, err
	}
	norm := func(s string) string { return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(s), " ")) }
	want := norm(text)
	var out []Hit
	seen := map[string]bool{}
	for _, sec := range res.Response.Sections {
		for _, h := range sec.Hits {
			r := h.Result
			key := norm(CleanTitle(r.Title)) + "|" + norm(PrimaryArtist(r.ArtistNames))
			if r.Title == "" || seen[key] {
				continue
			}
			seen[key] = true
			hit := Hit{Title: r.Title, Artist: r.ArtistNames, Cover: r.Art}
			for _, hl := range h.Highlights {
				for _, ln := range strings.Split(hl.Value, "\n") {
					if strings.Contains(norm(ln), want) || (hit.Snippet == "" && want != "" && strings.Contains(want, norm(ln)) && len(norm(ln)) > 8) {
						hit.Snippet = strings.TrimSpace(ln)
						break
					}
				}
			}
			out = append(out, hit)
		}
	}
	return out, nil
}

func lrclibSearch(ctx context.Context, e *Env, text string) ([]Hit, error) {
	var res []struct {
		TrackName  string  `json:"trackName"`
		ArtistName string  `json:"artistName"`
		AlbumName  string  `json:"albumName"`
		Duration   float64 `json:"duration"`
	}
	if _, err := e.getJSON(ctx, "https://lrclib.net/api/search?q="+url.QueryEscape(text), nil, &res); err != nil {
		return nil, err
	}
	var out []Hit
	for _, r := range res {
		if r.TrackName != "" {
			out = append(out, Hit{Title: r.TrackName, Artist: r.ArtistName, Album: r.AlbumName, DurationMs: int64(r.Duration * 1000)})
		}
	}
	return out, nil
}
