package lyrics

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meofishes/internal/core"
)

// Query describes the song being looked up.
type Query struct {
	Title      string
	Artist     string
	Album      string
	DurationMs int64
	ISRC       string
	VideoID    string
}

// ErrNotFound means the provider answered but has no lyric for the song.
var ErrNotFound = errors.New("lyrics not found")

// ErrNotConfigured means the provider needs settings (key, contact…) first.
var ErrNotConfigured = errors.New("provider not configured")

// Provider fetches lyrics from one service.
type Provider interface {
	ID() string
	Name() string
	Fetch(ctx context.Context, q Query) (*core.Lyrics, error)
}

// Env carries what providers need from the host: an HTTP client, the
// User-Agent and per-provider settings.
type Env struct {
	Client    *http.Client
	UserAgent string
	Settings  map[string]map[string]string // provider id -> key -> value
}

func (e *Env) setting(id, key string) string {
	if e.Settings == nil || e.Settings[id] == nil {
		return ""
	}
	return strings.TrimSpace(e.Settings[id][key])
}

func (e *Env) do(ctx context.Context, method, u string, body []byte, hdr map[string]string) (int, http.Header, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", e.UserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, resp.Header, data, err
}

func (e *Env) getJSON(ctx context.Context, u string, hdr map[string]string, v any) (int, error) {
	code, _, data, err := e.do(ctx, "GET", u, nil, hdr)
	if err != nil {
		return code, err
	}
	if code == 404 {
		return code, ErrNotFound
	}
	if code >= 400 {
		return code, fmt.Errorf("http %d", code)
	}
	return code, json.Unmarshal(data, v)
}

// ---------------------------------------------------------------- matching helpers

var nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var bracketed = regexp.MustCompile(`\s*[\(\[【].*?[\)\]】]`)
var featPart = regexp.MustCompile(`(?i)\s+(feat\.?|ft\.?|featuring)\s.*$`)

func norm(s string) string {
	s = strings.ToLower(s)
	s = featPart.ReplaceAllString(s, "")
	return strings.TrimSpace(nonAlnum.ReplaceAllString(s, " "))
}

// CleanTitle removes "(Official Video)", "[Remastered]", "feat. X"…
func CleanTitle(s string) string {
	s = bracketed.ReplaceAllString(s, "")
	s = featPart.ReplaceAllString(s, "")
	if i := strings.Index(s, " | "); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// PrimaryArtist returns the first credited artist.
func PrimaryArtist(s string) string {
	for _, sep := range []string{",", " & ", " x ", " feat", " ft.", ";", "、", "/"} {
		if i := strings.Index(strings.ToLower(s), sep); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

// MatchScore rates how well a candidate fits the query (0..1).
func MatchScore(q Query, title, artist string, durationMs int64) float64 {
	qt, ct := norm(CleanTitle(q.Title)), norm(CleanTitle(title))
	qa, ca := norm(q.Artist), norm(artist)
	score := 0.0
	switch {
	case qt == ct:
		score += 0.5
	case strings.Contains(ct, qt) || strings.Contains(qt, ct):
		score += 0.35
	default:
		score += 0.5 * tokenOverlap(qt, ct)
	}
	pa := norm(PrimaryArtist(q.Artist))
	switch {
	case qa == ca || (pa != "" && strings.Contains(ca, pa)):
		score += 0.35
	default:
		score += 0.35 * tokenOverlap(qa, ca)
	}
	if q.DurationMs > 0 && durationMs > 0 {
		d := math.Abs(float64(q.DurationMs-durationMs)) / 1000
		switch {
		case d <= 2:
			score += 0.15
		case d <= 5:
			score += 0.08
		case d > 20:
			score -= 0.3
		}
	} else {
		score += 0.05
	}
	return score
}

func tokenOverlap(a, b string) float64 {
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, t := range tb {
		set[t] = true
	}
	hit := 0
	for _, t := range ta {
		if set[t] {
			hit++
		}
	}
	return float64(hit) / math.Max(float64(len(ta)), float64(len(tb)))
}

const minScore = 0.6

// ================================================================ BetterLyrics

type BetterLyrics struct{ Env *Env }

func (BetterLyrics) ID() string   { return "betterlyrics" }
func (BetterLyrics) Name() string { return "BetterLyrics" }

func (p BetterLyrics) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	v := url.Values{}
	v.Set("s", CleanTitle(q.Title))
	v.Set("a", PrimaryArtist(q.Artist))
	if q.Album != "" {
		v.Set("al", q.Album)
	}
	if q.DurationMs > 0 {
		v.Set("d", strconv.FormatInt(q.DurationMs/1000, 10))
	}
	hdr := map[string]string{}
	if k := p.Env.setting(p.ID(), "apiKey"); k != "" {
		hdr["X-API-Key"] = k
	}
	var resp map[string]any
	if _, err := p.Env.getJSON(ctx, "https://api.betterlyrics.org/getLyrics?"+v.Encode(), hdr, &resp); err != nil {
		return nil, err
	}
	for _, key := range []string{"ttml", "lyrics"} {
		if s, _ := resp[key].(string); strings.TrimSpace(s) != "" {
			l, err := Parse("", s)
			if err != nil {
				return nil, err
			}
			l.Credit = "Lyrics via BetterLyrics"
			return l, nil
		}
	}
	return nil, ErrNotFound
}

// ================================================================ lrc.red

type LrcRed struct{ Env *Env }

func (LrcRed) ID() string   { return "lrcred" }
func (LrcRed) Name() string { return "lrc.red" }

func (p LrcRed) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	// lrc.red's terms require a User-Agent naming the app with a link and an
	// e-mail to contact. Without a contact e-mail we do not call it.
	if !strings.Contains(p.Env.UserAgent, "@") {
		return nil, ErrNotConfigured
	}
	isrc := q.ISRC
	if isrc == "" {
		var res struct {
			Hits []struct {
				ISRC     string  `json:"isrc"`
				Title    string  `json:"title"`
				Artist   string  `json:"artist"`
				Duration float64 `json:"duration"`
			} `json:"hits"`
		}
		u := "https://lrc.red/search.json?q=" + url.QueryEscape(CleanTitle(q.Title)+" "+PrimaryArtist(q.Artist))
		if _, err := p.Env.getJSON(ctx, u, nil, &res); err != nil {
			return nil, err
		}
		best := 0.0
		for _, h := range res.Hits {
			if s := MatchScore(q, h.Title, h.Artist, int64(h.Duration*1000)); s > best {
				best, isrc = s, h.ISRC
			}
		}
		if best < minScore {
			return nil, ErrNotFound
		}
	}
	code, _, data, err := p.Env.do(ctx, "GET", "https://lrc.red/s/"+url.PathEscape(isrc)+".ttml", nil, nil)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, ErrNotFound
	}
	l, err := ParseTTML(string(data))
	if err != nil {
		return nil, err
	}
	l.Credit = "Lyrics from lrc.red"
	return l, nil
}

// ================================================================ Unison

type Unison struct{ Env *Env }

func (Unison) ID() string   { return "unison" }
func (Unison) Name() string { return "Unison (Better Lyrics community)" }

func (p Unison) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	v := url.Values{}
	switch {
	case q.VideoID != "":
		v.Set("v", q.VideoID)
	case q.ISRC != "":
		v.Set("isrc", q.ISRC)
	default:
		v.Set("song", CleanTitle(q.Title))
		v.Set("artist", PrimaryArtist(q.Artist))
		if q.Album != "" {
			v.Set("album", q.Album)
		}
		if q.DurationMs > 0 {
			v.Set("duration", strconv.FormatInt(q.DurationMs/1000, 10))
		}
	}
	var res struct {
		Success bool `json:"success"`
		Data    struct {
			ID      int64  `json:"id"`
			Lyrics  string `json:"lyrics"`
			Format  string `json:"format"`
			Song    string `json:"song"`
			Artist  string `json:"artist"`
			Editors []struct {
				DisplayName string `json:"displayName"`
			} `json:"editors"`
		} `json:"data"`
	}
	if _, err := p.Env.getJSON(ctx, "https://unison.betterlyrics.org/lyrics?"+v.Encode(), nil, &res); err != nil {
		return nil, err
	}
	if !res.Success || res.Data.Lyrics == "" {
		return nil, ErrNotFound
	}
	l, err := Parse(res.Data.Format, res.Data.Lyrics)
	if err != nil {
		return nil, err
	}
	l.Credit = "Lyrics from Unison, by the Better Lyrics community"
	if len(res.Data.Editors) > 0 && res.Data.Editors[0].DisplayName != "" {
		l.Credit += " · edited by " + res.Data.Editors[0].DisplayName
	}
	return l, nil
}

// ================================================================ Paxsenix

type Paxsenix struct{ Env *Env }

func (Paxsenix) ID() string   { return "paxsenix" }
func (Paxsenix) Name() string { return "Paxsenix (Apple Music)" }

func (p Paxsenix) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	// 1) find the Apple Music track id through the public iTunes Search API
	country := p.Env.setting(p.ID(), "country")
	if country == "" {
		country = "us"
	}
	var search struct {
		Results []struct {
			TrackID int64  `json:"trackId"`
			Track   string `json:"trackName"`
			Artist  string `json:"artistName"`
			TimeMs  int64  `json:"trackTimeMillis"`
		} `json:"results"`
	}
	term := CleanTitle(q.Title) + " " + PrimaryArtist(q.Artist)
	su := "https://itunes.apple.com/search?entity=song&limit=15&country=" + url.QueryEscape(country) + "&term=" + url.QueryEscape(term)
	if _, err := p.Env.getJSON(ctx, su, nil, &search); err != nil {
		return nil, err
	}
	var id int64
	best := 0.0
	for _, r := range search.Results {
		if s := MatchScore(q, r.Track, r.Artist, r.TimeMs); s > best {
			best, id = s, r.TrackID
		}
	}
	if best < minScore || id == 0 {
		return nil, ErrNotFound
	}
	// 2) lyrics
	base := p.Env.setting(p.ID(), "baseUrl")
	if base == "" {
		base = "https://lyrics.paxsenix.org"
	}
	hdr := map[string]string{}
	if k := p.Env.setting(p.ID(), "apiKey"); k != "" {
		hdr["Authorization"] = "Bearer " + k
	}
	var res paxResponse
	if _, err := p.Env.getJSON(ctx, strings.TrimRight(base, "/")+"/apple-music/lyrics?id="+strconv.FormatInt(id, 10), hdr, &res); err != nil {
		return nil, err
	}
	l, err := res.toLyrics()
	if err != nil {
		return nil, err
	}
	l.Credit = "Lyrics via Paxsenix (Apple Music)"
	return l, nil
}

type paxWord struct {
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Endtime   int64  `json:"endtime"`
	Part      bool   `json:"part"`
}

type paxResponse struct {
	Type    string `json:"type"`
	TTML    string `json:"ttmlContent"`
	ELRCMP  string `json:"elrcMultiPerson"`
	ELRC    string `json:"elrc"`
	Plain   string `json:"plain"`
	Content []struct {
		Timestamp      int64     `json:"timestamp"`
		Endtime        int64     `json:"endtime"`
		Text           []paxWord `json:"text"`
		BackgroundText []paxWord `json:"backgroundText"`
		OppositeTurn   bool      `json:"oppositeTurn"`
	} `json:"content"`
}

func paxWords(in []paxWord) []core.Word {
	var out []core.Word
	glue := false
	for _, w := range in {
		t := strings.TrimSpace(w.Text)
		if t == "" {
			continue
		}
		out = append(out, core.Word{Start: w.Timestamp, End: w.Endtime, Text: t, Part: glue && len(out) > 0})
		glue = w.Part // "part" = this syllable continues into the next one
	}
	return out
}

func (r paxResponse) toLyrics() (*core.Lyrics, error) {
	if strings.EqualFold(r.Type, "None") && r.Plain != "" {
		return ParsePlain(r.Plain), nil
	}
	if r.TTML != "" {
		if l, err := ParseTTML(r.TTML); err == nil {
			return l, nil
		}
	}
	for _, s := range []string{r.ELRCMP, r.ELRC} {
		if s != "" {
			if l := ParseLRC(s); len(l.Lines) > 0 {
				return l, nil
			}
		}
	}
	if len(r.Content) > 0 {
		l := &core.Lyrics{}
		for _, c := range r.Content {
			ln := core.Line{Start: c.Timestamp, End: c.Endtime, Opposite: c.OppositeTurn}
			ln.Words = paxWords(c.Text)
			ln.Background = paxWords(c.BackgroundText)
			if strings.EqualFold(r.Type, "Line") && len(ln.Words) > 0 {
				ln.Text = core.JoinWords(ln.Words)
				ln.Words = nil
			}
			l.Lines = append(l.Lines, ln)
		}
		l.Normalize()
		return l, nil
	}
	if r.Plain != "" {
		return ParsePlain(r.Plain), nil
	}
	return nil, ErrNotFound
}

// ================================================================ Kugou

type Kugou struct{ Env *Env }

func (Kugou) ID() string   { return "kugou" }
func (Kugou) Name() string { return "Kugou" }

func (p Kugou) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	var search struct {
		Data struct {
			Info []struct {
				Hash     string `json:"hash"`
				Song     string `json:"songname"`
				Singer   string `json:"singername"`
				Duration int64  `json:"duration"` // seconds
			} `json:"info"`
		} `json:"data"`
	}
	kw := PrimaryArtist(q.Artist) + " " + CleanTitle(q.Title)
	su := "http://mobilecdn.kugou.com/api/v3/search/song?format=json&page=1&pagesize=10&showtype=1&keyword=" + strings.ReplaceAll(url.QueryEscape(kw), "+", "%20")
	if _, err := p.Env.getJSON(ctx, su, nil, &search); err != nil {
		return nil, err
	}
	hash, best := "", 0.0
	for _, s := range search.Data.Info {
		if sc := MatchScore(q, s.Song, s.Singer, s.Duration*1000); sc > best {
			best, hash = sc, s.Hash
		}
	}
	if best < minScore {
		return nil, ErrNotFound
	}
	var cand struct {
		Candidates []struct {
			ID        string `json:"id"`
			AccessKey string `json:"accesskey"`
		} `json:"candidates"`
	}
	cu := "https://krcs.kugou.com/search?ver=1&man=yes&client=mobi&keyword=&duration=&album_audio_id=&hash=" + url.QueryEscape(hash)
	if _, err := p.Env.getJSON(ctx, cu, nil, &cand); err != nil {
		return nil, err
	}
	if len(cand.Candidates) == 0 {
		return nil, ErrNotFound
	}
	c := cand.Candidates[0]
	var dl struct {
		Content string `json:"content"`
		Fmt     string `json:"fmt"`
	}
	du := fmt.Sprintf("https://lyrics.kugou.com/download?ver=1&client=pc&fmt=krc&charset=utf8&id=%s&accesskey=%s", url.QueryEscape(c.ID), url.QueryEscape(c.AccessKey))
	if _, err := p.Env.getJSON(ctx, du, nil, &dl); err != nil {
		return nil, err
	}
	if dl.Content == "" {
		return nil, ErrNotFound
	}
	text, err := DecodeKRC(dl.Content)
	var l *core.Lyrics
	if err == nil {
		l, err = ParseKRC(text)
	} else if raw, e2 := base64.StdEncoding.DecodeString(dl.Content); e2 == nil {
		l, err = ParseLRC(string(raw)), nil
	}
	if err != nil {
		return nil, err
	}
	l.Lines = dropKugouCredits(l.Lines)
	l.Credit = "Lyrics from Kugou"
	return l, nil
}

// Kugou prefixes lyrics with "Title - Artist" and credit lines ("作词：", "Produced by：").
var kugouCredit = regexp.MustCompile(`(?i)^[^:：]{1,30}(by|词|曲|编|制作|监制|混音|和声|录音|吉他|贝斯|鼓)[^:：]{0,10}[:：]`)

func dropKugouCredits(ls []core.Line) []core.Line {
	out := ls[:0]
	for i, l := range ls {
		if i < 12 && (kugouCredit.MatchString(l.Text) || (i == 0 && strings.Contains(l.Text, " - "))) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// ================================================================ Lyprio

// Lyprio is a signed API. It needs a base URL and a 32-byte HMAC signing key
// (base64) given by the Lyprio maintainers.
type Lyprio struct{ Env *Env }

func (Lyprio) ID() string   { return "lyprio" }
func (Lyprio) Name() string { return "Lyprio" }

type lyprioResult struct {
	Subprovider string `json:"subprovider"`
	Format      string `json:"format"`
	SyncType    string `json:"syncType"`
	Lyrics      string `json:"lyrics"`
}

func (p Lyprio) Fetch(ctx context.Context, q Query) (*core.Lyrics, error) {
	base := strings.TrimRight(p.Env.setting(p.ID(), "baseUrl"), "/")
	key, err := base64.StdEncoding.DecodeString(p.Env.setting(p.ID(), "signingKey"))
	if base == "" || err != nil || len(key) != 32 {
		return nil, ErrNotConfigured
	}
	if !strings.HasPrefix(base, "https://") {
		return nil, errors.New("lyprio base url must be https")
	}
	reqBody := map[string]any{"title": q.Title, "artist": q.Artist, "allSources": false}
	if q.Album != "" {
		reqBody["album"] = q.Album
	}
	if q.DurationMs > 0 {
		reqBody["duration"] = q.DurationMs / 1000
	}
	if q.VideoID != "" {
		reqBody["videoId"] = q.VideoID
	}
	body, _ := json.Marshal(reqBody)
	send := func(path string) (int, http.Header, []byte, error) {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(ts + "\n" + string(body)))
		return p.Env.do(ctx, "POST", base+path, body, map[string]string{
			"Content-Type":       "application/json",
			"X-Lyprio-Timestamp": ts,
			"X-Lyprio-Signature": hex.EncodeToString(mac.Sum(nil)),
		})
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		code, hdr, data, err := send("/v2/lyrics/search")
		if err != nil {
			return nil, err
		}
		if (code == 404 && hdr.Get("X-Lyprio-Protocol") == "") || code == 405 {
			code, _, data, err = send("/v1/lyrics/search")
			if err != nil {
				return nil, err
			}
		}
		var res struct {
			Results      []lyprioResult `json:"results"`
			Status       string         `json:"status"`
			Complete     bool           `json:"complete"`
			RetryAfterMs int64          `json:"retryAfterMs"`
		}
		_ = json.Unmarshal(data, &res)
		switch {
		case code == 200 && len(res.Results) > 0:
			if l := bestLyprio(res.Results); l != nil {
				return l, nil
			}
			if res.Complete {
				return nil, ErrNotFound
			}
		case code == 404:
			return nil, ErrNotFound
		case code == 202 || code == 429 || code == 503:
			if res.Complete {
				return nil, ErrNotFound
			}
		default:
			return nil, fmt.Errorf("lyprio http %d", code)
		}
		wait := time.Duration(res.RetryAfterMs) * time.Millisecond
		if wait <= 0 {
			wait = 2 * time.Second
		}
		if time.Now().Add(wait).After(deadline) {
			return nil, ErrNotFound
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func bestLyprio(rs []lyprioResult) *core.Lyrics {
	var best *core.Lyrics
	for _, r := range rs {
		if strings.EqualFold(r.Format, "mlx") { // Metrolist's internal format, not supported here
			continue
		}
		l, err := Parse(r.Format, r.Lyrics)
		if err != nil || len(l.Lines) == 0 {
			continue
		}
		if best == nil || l.Sync > best.Sync {
			best = l
			best.Credit = "Lyrics via Lyprio"
			if r.Subprovider != "" {
				best.Credit += " (" + r.Subprovider + ")"
			}
		}
	}
	return best
}
