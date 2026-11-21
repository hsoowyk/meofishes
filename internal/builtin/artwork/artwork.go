// Package artwork is the default fnt "artwork" plugin: it finds album covers
// for tracks that have none (or only a video thumbnail) using the iTunes
// Search API, the Deezer API and Piped (YouTube Music art through a Piped
// instance). None needs an account or a key.
package artwork

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"meofishes/internal/lyrics"
	"meofishes/internal/plugin"
)

const ID = "meo-artwork"

// Manifest describes the plugin in the Plugins page.
var Manifest = plugin.Manifest{
	ID:          ID,
	Name:        "Meo Artwork",
	Version:     "1.0.0",
	Author:      "meofishes",
	Description: "Finds album covers for tracks without one (or with only a video thumbnail) on iTunes and Deezer. No account needed.",
	Types:       []string{plugin.TypeFnt},
	Runtime:     plugin.RuntimeBuiltin,
	Settings: []plugin.Setting{
		{Key: "order", Label: "Services", Type: "list", Default: "itunes,deezer,piped",
			Description: "Comma-separated: itunes, deezer, piped. Remove one to disable it."},
		{Key: "piped", Label: "Piped instances", Type: "list",
			Default:     "https://pipedapi.kavin.rocks,https://api.piped.private.coffee,https://pipedapi.adminforge.de",
			Description: "Piped API instances tried in order (comma-separated)."},
		{Key: "size", Label: "Image size (px)", Type: "number", Default: "600"},
		{Key: "country", Label: "iTunes store country", Default: "us"},
		{Key: "minScore", Label: "Minimum match score (0–1)", Type: "number", Default: "0.55",
			Description: "How closely title and artist must match before a cover is used."},
	},
}

type impl struct {
	host     *plugin.Host
	settings func() map[string]string
	client   *http.Client
}

// New is the plugin factory.
func New(host *plugin.Host, settings func() map[string]string) (any, error) {
	return &impl{host: host, settings: settings, client: &http.Client{Timeout: 12 * time.Second}}, nil
}

func (p *impl) get(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", p.host.UserAgent)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func atof(s string, def float64) float64 {
	var f float64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &f); err != nil {
		return def
	}
	return f
}

// Artwork implements plugin.ArtworkSource.
func (p *impl) Artwork(ctx context.Context, q lyrics.Query) (string, error) {
	s := p.settings()
	q.Title = lyrics.CleanTitle(q.Title)
	if q.Title == "" {
		return "", lyrics.ErrNotFound
	}
	min := atof(s["minScore"], 0.55)
	size := int(atof(s["size"], 600))
	if size < 100 {
		size = 600
	}
	for _, svc := range strings.Split(s["order"], ",") {
		var u string
		var err error
		switch strings.TrimSpace(strings.ToLower(svc)) {
		case "itunes":
			u, err = p.itunes(ctx, q, min, size, s["country"])
		case "deezer":
			u, err = p.deezer(ctx, q, min, size)
		case "piped":
			u, err = p.piped(ctx, q, min, size, s["piped"])
		default:
			continue
		}
		if err == nil && u != "" {
			return u, nil
		}
	}
	return "", lyrics.ErrNotFound
}

func (p *impl) itunes(ctx context.Context, q lyrics.Query, min float64, size int, country string) (string, error) {
	if country == "" {
		country = "us"
	}
	term := strings.TrimSpace(lyrics.PrimaryArtist(q.Artist) + " " + q.Title)
	u := "https://itunes.apple.com/search?media=music&entity=song&limit=8&country=" + url.QueryEscape(country) + "&term=" + url.QueryEscape(term)
	var r struct {
		Results []struct {
			TrackName      string `json:"trackName"`
			ArtistName     string `json:"artistName"`
			TrackTimeMs    int64  `json:"trackTimeMillis"`
			ArtworkURL100  string `json:"artworkUrl100"`
			CollectionName string `json:"collectionName"`
		} `json:"results"`
	}
	if err := p.get(ctx, u, &r); err != nil {
		return "", err
	}
	best, bestScore := "", 0.0
	for _, it := range r.Results {
		sc := lyrics.MatchScore(q, it.TrackName, it.ArtistName, it.TrackTimeMs)
		if sc > bestScore && it.ArtworkURL100 != "" {
			best, bestScore = it.ArtworkURL100, sc
		}
	}
	if bestScore < min {
		return "", lyrics.ErrNotFound
	}
	px := fmt.Sprintf("%dx%dbb", size, size)
	return strings.Replace(best, "100x100bb", px, 1), nil
}

func (p *impl) deezer(ctx context.Context, q lyrics.Query, min float64, size int) (string, error) {
	query := strings.TrimSpace(lyrics.PrimaryArtist(q.Artist) + " " + q.Title)
	u := "https://api.deezer.com/search?limit=8&q=" + url.QueryEscape(query)
	var r struct {
		Data []struct {
			Title    string `json:"title"`
			Duration int64  `json:"duration"`
			Artist   struct {
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				CoverBig string `json:"cover_big"`
				CoverXL  string `json:"cover_xl"`
			} `json:"album"`
		} `json:"data"`
	}
	if err := p.get(ctx, u, &r); err != nil {
		return "", err
	}
	best, bestScore := "", 0.0
	for _, it := range r.Data {
		sc := lyrics.MatchScore(q, it.Title, it.Artist.Name, it.Duration*1000)
		c := it.Album.CoverXL
		if size <= 500 || c == "" {
			c = it.Album.CoverBig
		}
		if sc > bestScore && c != "" {
			best, bestScore = c, sc
		}
	}
	if bestScore < min {
		return "", lyrics.ErrNotFound
	}
	return best, nil
}

// piped searches YouTube Music songs through a Piped instance; their
// thumbnails are square album art (lh3.googleusercontent.com).
func (p *impl) piped(ctx context.Context, q lyrics.Query, min float64, size int, instances string) (string, error) {
	query := strings.TrimSpace(lyrics.PrimaryArtist(q.Artist) + " " + q.Title)
	for _, inst := range strings.Split(instances, ",") {
		inst = strings.TrimRight(strings.TrimSpace(inst), "/")
		if inst == "" {
			continue
		}
		var r struct {
			Items []struct {
				Title        string `json:"title"`
				UploaderName string `json:"uploaderName"`
				Duration     int64  `json:"duration"`
				Thumbnail    string `json:"thumbnail"`
			} `json:"items"`
		}
		ictx, cancel := context.WithTimeout(ctx, 6*time.Second)
		err := p.get(ictx, inst+"/search?filter=music_songs&q="+url.QueryEscape(query), &r)
		cancel()
		if err != nil {
			continue // next instance
		}
		best, bestScore := "", 0.0
		for _, it := range r.Items {
			artist := strings.TrimSuffix(it.UploaderName, " - Topic")
			sc := lyrics.MatchScore(q, it.Title, artist, it.Duration*1000)
			if sc > bestScore && it.Thumbnail != "" {
				best, bestScore = it.Thumbnail, sc
			}
		}
		if bestScore < min {
			return "", lyrics.ErrNotFound
		}
		return PipedArt(best, size), nil
	}
	return "", lyrics.ErrNotFound
}

// PipedArt asks googleusercontent for a bigger square image when possible
// ("…=w120-h120-l90-rj" → "…=w600-h600-l90-rj").
func PipedArt(u string, size int) string {
	if i := strings.LastIndex(u, "=w"); i > 0 && strings.Contains(u, "googleusercontent") {
		return u[:i] + fmt.Sprintf("=w%d-h%d-l90-rj", size, size)
	}
	return u
}
