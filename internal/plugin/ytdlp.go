package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"meofishes/internal/core"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// YtdlpPath returns the yt-dlp binary or "" if it is not installed.
func YtdlpPath() string {
	for _, name := range []string{"yt-dlp", "yt-dlp_linux", "youtube-dl"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// YtdlpArgs are extra arguments for every yt-dlp call (Settings → Playback
// → YouTube account: --cookies-from-browser / --cookies).
var YtdlpArgs []string

// CookieArgs turns the setting into yt-dlp arguments: a browser name
// (firefox, chrome, chromium, brave, edge, opera, vivaldi, librewolf…) or
// "file:/path/cookies.txt". The second value is for mpv's --ytdl-raw-options.
func CookieArgs(setting string) (ytdlp []string, mpvRaw string) {
	setting = strings.TrimSpace(setting)
	switch {
	case setting == "":
		return nil, ""
	case strings.HasPrefix(setting, "file:"):
		p := strings.TrimPrefix(setting, "file:")
		return []string{"--cookies", p}, "cookies=" + p
	}
	return []string{"--cookies-from-browser", setting}, "cookies-from-browser=" + setting
}

// IsBotCheck reports whether YouTube refused yt-dlp until the user signs in
// ("Sign in to confirm you're not a bot").
func IsBotCheck(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "confirm you") || strings.Contains(err.Error(), "not a bot"))
}

func runYtdlp(ctx context.Context, args ...string) ([]byte, error) {
	bin := YtdlpPath()
	if bin == "" {
		return nil, fmt.Errorf("yt-dlp: %w (install it with your package manager)", errNoBinary)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	all := append(append([]string{"--no-warnings", "--ignore-config"}, YtdlpArgs...), args...)
	cmd := exec.CommandContext(ctx, bin, all...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return nil, fmt.Errorf("yt-dlp: %v: %s", err, msg)
	}
	return out, nil
}

// ytdlpEntry converts a yt-dlp JSON entry into a track-like map.
func ytdlpEntry(e map[string]any) map[string]any {
	id := toString(e["id"])
	u := toString(Lookup(e, "webpage_url || url || original_url"))
	if u == "" && id != "" && strings.Contains(toString(e["ie_key"]), "Youtube") {
		u = "https://www.youtube.com/watch?v=" + id
	}
	artist := toString(Lookup(e, "artist || creator || uploader || channel"))
	artist = strings.TrimSuffix(artist, " - Topic")
	title := toString(Lookup(e, "track || title"))
	cover := toString(e["thumbnail"])
	if cover == "" {
		if th, ok := e["thumbnails"].([]any); ok && len(th) > 0 {
			cover = toString(Lookup(th[len(th)-1], "url"))
		}
	}
	if cover == "" && strings.Contains(toString(e["ie_key"]), "Youtube") && id != "" {
		cover = "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
	}
	// "Artist - Title (Official Video)" → artist + title
	if toString(e["track"]) == "" {
		if a, t, ok := strings.Cut(title, " - "); ok && len(a) < 60 {
			if artist == "" || strings.Contains(strings.ToLower(artist), "vevo") || strings.Contains(strings.ToLower(a), strings.ToLower(strings.Fields(artist + " x")[0])) {
				artist, title = strings.TrimSpace(a), strings.TrimSpace(t)
			}
		}
	}
	dur := int64(0)
	if d, ok := e["duration"].(float64); ok {
		dur = int64(d * 1000)
	}
	t := map[string]any{
		"id": id, "title": title, "artist": artist, "album": toString(e["album"]),
		"durationMs": dur, "cover": cover, "streamUrl": u,
	}
	if strings.Contains(u, "youtube.com") || strings.Contains(u, "youtu.be") {
		t["videoId"] = id
	}
	return t
}

// YtdlpSearch searches a site. site: youtube (default), ytmusic, soundcloud.
func YtdlpSearch(ctx context.Context, query string, limit int, site string) ([]map[string]any, error) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	var target string
	switch site {
	case "soundcloud", "sc":
		target = fmt.Sprintf("scsearch%d:%s", limit, query)
	case "ytmusic", "youtube-music":
		target = "https://music.youtube.com/search?q=" + strings.ReplaceAll(query, " ", "+") + "#songs"
	default:
		target = fmt.Sprintf("ytsearch%d:%s", limit, query)
	}
	out, err := runYtdlp(ctx, "--flat-playlist", "-J", "--playlist-end", strconv.Itoa(limit), target)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var res []map[string]any
	for _, e := range doc.Entries {
		if t := ytdlpEntry(e); t["id"] != "" {
			res = append(res, t)
		}
	}
	return res, nil
}

// YtdlpStreamURL extracts the direct audio URL of a page.
// AudioFormat is the yt-dlp format used for audio (set from the quality
// setting: see QualityFormat).
var AudioFormat = "bestaudio/best"

// QualityFormat maps a quality level to a yt-dlp format.
//
//	max    the best the source has (lossless when there is one)
//	high   up to ~192 kbps
//	normal up to ~128 kbps
//	low    up to ~64 kbps (saves data)
func QualityFormat(q string) string {
	switch q {
	case "high":
		return "bestaudio[abr<=192]/bestaudio/best"
	case "normal":
		return "bestaudio[abr<=130]/bestaudio/best"
	case "low":
		return "bestaudio[abr<=70]/worstaudio/best"
	}
	return "bestaudio/best"
}

func YtdlpStreamURL(ctx context.Context, page string) (string, error) {
	key := "stream|" + AudioFormat + "|" + page
	if u, ok := Cache.Get(key); ok {
		if streamAlive(ctx, u) {
			return u, nil
		}
		Cache.Drop(key) // signed for another IP, or revoked
	}
	// the same page being resolved already (warm-up, prefetch): wait for it
	return inflight.do(key, func() (string, error) { return ytdlpStreamURL(ctx, key, page) })
}

func ytdlpStreamURL(ctx context.Context, key, page string) (string, error) {
	out, err := runYtdlp(ctx, "-f", AudioFormat, "-g", "--no-playlist", page)
	if err != nil {
		return "", err
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return "", fmt.Errorf("yt-dlp returned no url")
	}
	if until := urlExpiry(lines[0]); !until.IsZero() {
		Cache.Put(key, lines[0], until)
	}
	return lines[0], nil
}

// ForgetStream drops the cached stream URL of a page (it stopped working).
func ForgetStream(page string) { Cache.Drop("stream|" + AudioFormat + "|" + page) }

// YtdlpVideoURL returns the URL of the video stream only (no audio) of a
// page, at most height pixels high: the audio keeps coming from the stream
// that is already playing.
func YtdlpVideoURL(ctx context.Context, page string, height int) (string, error) {
	if height <= 0 {
		height = 720
	}
	f := fmt.Sprintf("bestvideo[height<=?%d][vcodec!=none]/best[height<=?%d]", height, height)
	out, err := runYtdlp(ctx, "-f", f, "-g", "--no-playlist", page)
	if err != nil {
		return "", err
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return "", fmt.Errorf("yt-dlp returned no video url")
	}
	return lines[0], nil
}

// YtdlpPlaylist lists a playlist/album/channel page (YouTube, SoundCloud,
// Bandcamp…): its title and its entries as track-like maps.
func YtdlpPlaylist(ctx context.Context, page string, limit int) (string, []map[string]any, error) {
	args := []string{"--flat-playlist", "-J"}
	if limit > 0 {
		args = append(args, "--playlist-end", strconv.Itoa(limit))
	}
	out, err := runYtdlp(ctx, append(args, page)...)
	if err != nil {
		return "", nil, err
	}
	var doc struct {
		Title   string           `json:"title"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", nil, err
	}
	var res []map[string]any
	for _, x := range doc.Entries {
		res = append(res, ytdlpEntry(x))
	}
	return doc.Title, res, nil
}

// TrackFromMap converts a track-like map (ytdlp module) into a Track that
// the plugin "source" plays.
func TrackFromMap(m map[string]any, source string) core.Track {
	t := core.Track{Source: source, ID: toString(m["id"]), Title: toString(m["title"]), Artist: toString(m["artist"]),
		Album: toString(m["album"]), Cover: toString(m["cover"]), StreamURL: toString(m["streamUrl"]), VideoID: toString(m["videoId"])}
	if d, ok := m["durationMs"].(int64); ok {
		t.DurationMs = d
	}
	if t.ID == "" {
		t.ID = t.StreamURL
	}
	return t
}

func ytdlpModule(e *moduleEnv) map[string]any {
	ctx := context.Background()
	return map[string]any{
		"available": func() bool { return YtdlpPath() != "" },
		"search": func(query string, limit int, site string) ([]map[string]any, error) {
			return YtdlpSearch(ctx, query, limit, site)
		},
		"info": func(page string) (map[string]any, error) {
			out, err := runYtdlp(ctx, "-J", "--no-playlist", page)
			if err != nil {
				return nil, err
			}
			var m map[string]any
			if err := json.Unmarshal(out, &m); err != nil {
				return nil, err
			}
			return ytdlpEntry(m), nil
		},
		"streamUrl": func(page string) (string, error) { return YtdlpStreamURL(ctx, page) },
		"playlist": func(page string, limit int) ([]map[string]any, error) {
			_, res, err := YtdlpPlaylist(ctx, page, limit)
			return res, err
		},
	}
}
