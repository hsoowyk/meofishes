// Package sponsorblock asks the SponsorBlock community database
// (sponsor.ajay.app) which parts of a YouTube video are not music (intro,
// outro, talking, sponsor…) so the player can skip them.
package sponsorblock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

// Categories meofishes can skip.
var Categories = []string{"music_offtopic", "intro", "outro", "sponsor", "selfpromo", "interaction", "preview"}

// Segment is a part to skip (ms).
type Segment struct {
	StartMs, EndMs int64
	Category       string
}

// Client fetches and caches segments.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	API       string // default https://sponsor.ajay.app
	mu        sync.Mutex
	cache     map[string][]Segment
}

// New returns a client.
func New(ua string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, UserAgent: ua, API: "https://sponsor.ajay.app", cache: map[string][]Segment{}}
}

// Segments returns the parts of videoID to skip, for the given categories.
func (c *Client) Segments(ctx context.Context, videoID string, cats []string) ([]Segment, error) {
	if videoID == "" || len(cats) == 0 {
		return nil, nil
	}
	key := videoID + fmt.Sprint(cats)
	c.mu.Lock()
	if s, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return s, nil
	}
	c.mu.Unlock()
	cj, _ := json.Marshal(cats)
	u := c.API + "/api/skipSegments?videoID=" + url.QueryEscape(videoID) + "&categories=" + url.QueryEscape(string(cj))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []Segment
	switch resp.StatusCode {
	case 404: // no segments for this video
	case 200:
		var raw []struct {
			Segment  [2]float64 `json:"segment"`
			Category string     `json:"category"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, err
		}
		for _, r := range raw {
			if r.Segment[1] > r.Segment[0] {
				out = append(out, Segment{StartMs: int64(r.Segment[0] * 1000), EndMs: int64(r.Segment[1] * 1000), Category: r.Category})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].StartMs < out[j].StartMs })
	default:
		return nil, fmt.Errorf("sponsorblock: http %d", resp.StatusCode)
	}
	c.mu.Lock()
	c.cache[key] = out
	c.mu.Unlock()
	return out, nil
}

// SkipTo returns where to jump when pos is inside a segment (0 = stay).
// Segments that start in the first 1.5 s are skipped right away.
func SkipTo(segs []Segment, posMs int64) (int64, string) {
	for _, s := range segs {
		if posMs >= s.StartMs && posMs < s.EndMs-500 {
			return s.EndMs, s.Category
		}
	}
	return 0, ""
}
