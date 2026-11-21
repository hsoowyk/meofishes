// Package config stores the user's settings in ~/.config/meofishes/config.json.
package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"meofishes/internal/core"
	"meofishes/internal/plugin"
)

// Version of the app.
const Version = "0.2.0"

// Contact identifies the app to the services it talks to. lrc.red requires
// the app name, a link and an e-mail in the User-Agent; Unison asks for the
// name and a link. It is fixed: users never type their own contact.
const (
	ContactURL   = "https://git.5418.org/kywoosh/meofishes"
	ContactEmail = "mail@kywoosh.xyz"
)

// UserAgent is sent with every request.
var UserAgent = "meofishes/" + Version + " (+" + ContactURL + "; " + ContactEmail + ")"

// Paths are the folders the app uses (XDG).
type Paths struct {
	Config  string // ~/.config/meofishes
	Data    string // ~/.local/share/meofishes
	Cache   string // ~/.cache/meofishes
	Plugins string // Data/plugins
	Themes  string // Data/themes
	PData   string // Data/plugin-data
	Runtime string // $XDG_RUNTIME_DIR or temp
}

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

// DefaultPaths resolves the XDG folders and creates them.
func DefaultPaths() Paths {
	p := Paths{
		Config: filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "meofishes"),
		Data:   filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "meofishes"),
		Cache:  filepath.Join(xdg("XDG_CACHE_HOME", ".cache"), "meofishes"),
	}
	p.Plugins = filepath.Join(p.Data, "plugins")
	p.Themes = filepath.Join(p.Data, "themes")
	p.PData = filepath.Join(p.Data, "plugin-data")
	p.Runtime = os.Getenv("XDG_RUNTIME_DIR")
	if p.Runtime == "" {
		p.Runtime = os.TempDir()
	}
	for _, d := range []string{p.Config, p.Data, p.Cache, p.Plugins, p.Themes, p.PData} {
		_ = os.MkdirAll(d, 0o755)
	}
	return p
}

// Config is the persisted state.
type Config struct {
	Theme      string `json:"theme"`
	UserCSS    string `json:"userCss,omitempty"`    // extra CSS applied over the theme
	LiveReload bool   `json:"liveReload,omitempty"` // reload the theme when its files change
	// Style holds the changes made in Settings, as "selector|property" →
	// value (":root|--red" → "#e06070"). It is compiled into a CSS layer
	// between the theme and the user CSS. Keys of the form "theme:<id>|…"
	// only apply to one theme.
	Style map[string]string `json:"style,omitempty"`

	Volume    float64 `json:"volume"`
	Source    string  `json:"source,omitempty"` // last selected source
	Shuffle   bool    `json:"shuffle,omitempty"`
	Repeat    string  `json:"repeat,omitempty"` // off | all | one
	WindowW   int     `json:"windowW,omitempty"`
	WindowH   int     `json:"windowH,omitempty"`
	Maximized bool    `json:"maximized,omitempty"`
	Language  string  `json:"language,omitempty"` // "" = English

	Catalogs []string `json:"catalogs,omitempty"` // extra catalog index URLs

	Plugins plugin.State `json:"plugins"`
	Prefs   Prefs        `json:"prefs"`

	// persistent queue
	Queue      []core.Track `json:"queue,omitempty"`
	QueueIndex int          `json:"queueIndex,omitempty"`
	QueuePosMs int64        `json:"queuePosMs,omitempty"`

	FirstRunDone bool `json:"firstRunDone,omitempty"`

	path string
	mu   sync.Mutex
}

// Prefs are the behaviour settings (the look lives in Style).
type Prefs struct {
	// appearance
	UserName     string  `json:"userName"`     // shown in the greeting
	AnimSpeed    float64 `json:"animSpeed"`    // multiplies every animation duration (1 = theme speed)
	ReduceMotion bool    `json:"reduceMotion"` // no enter/transition animations
	StartPage    string  `json:"startPage"`    // home | search | library
	GridSize     string  `json:"gridSize"`     // small | medium | large (playlist cards)
	ListCovers   bool    `json:"listCovers"`   // covers in track lists
	CoverTilt    bool    `json:"coverTilt"`    // tilt the fullscreen cover under the mouse
	TiltMax      float64 `json:"tiltMax"`      // degrees

	// lyrics
	LyricsOffsetMs int  `json:"lyricsOffsetMs"` // + shows lyrics later, − earlier
	BgVocals       bool `json:"bgVocals"`       // show background vocals
	LyricsCredits  bool `json:"lyricsCredits"`
	LyricsAutoOpen bool `json:"lyricsAutoOpen"` // open the fullscreen player with lyrics on play

	// playback & audio
	ReplayGain     string `json:"replayGain"` // no | track | album
	Normalize      bool   `json:"normalize"`
	AudioDevice    string `json:"audioDevice,omitempty"`
	Exclusive      bool   `json:"exclusive"`
	CacheMB        int    `json:"cacheMb"`
	ReadaheadSecs  int    `json:"readaheadSecs"`
	SeekStepSecs   int    `json:"seekStepSecs"`
	Autoplay       bool   `json:"autoplay"`      // play recommendations when the queue ends
	PersistQueue   bool   `json:"persistQueue"`  // restore the queue on start
	ResumeOnStart  bool   `json:"resumeOnStart"` // start playing the restored queue
	ExtraMpvArgs   string `json:"extraMpvArgs,omitempty"`
	PauseOnUnplug  bool   `json:"pauseOnUnplug"`  // reserved
	StopAfterTrack bool   `json:"stopAfterTrack"` // stop when the current track ends

	// streaming
	Chunked      bool `json:"chunked"`      // download in parallel chunks through the local proxy
	ChunkKB      int  `json:"chunkKb"`      // chunk size
	Workers      int  `json:"workers"`      // parallel requests
	FullDownload bool `json:"fullDownload"` // keep downloading the whole track in the background
	Prefetch     bool `json:"prefetch"`     // resolve and start downloading the next track early
	DirectYtdlp  bool `json:"directYtdlp"`  // resolve web pages with yt-dlp ourselves (needed for chunks)

	// artwork
	FetchArtwork  bool `json:"fetchArtwork"`  // find covers for tracks without one
	PreferArtwork bool `json:"preferArtwork"` // replace video thumbnails by album art

	// library & privacy
	KeepHistory bool `json:"keepHistory"`

	// tutorial: asked once on the first start, can be replayed in Settings
	TutorialAsked bool `json:"tutorialAsked"`

	// developer
	DevMode bool `json:"devMode"`
	ShowFPS bool `json:"showFps"`
	LogHTTP bool `json:"logHttp"`
}

// DefaultPrefs are used for missing values.
func DefaultPrefs() Prefs {
	name := ""
	if u, err := user.Current(); err == nil {
		name = strings.TrimSpace(strings.Split(u.Name, ",")[0])
		if name == "" {
			name = u.Username
		}
		if f := strings.Fields(name); len(f) > 0 {
			name = f[0]
		}
	}
	return Prefs{UserName: name,
		AnimSpeed: 1, StartPage: "home", GridSize: "medium", ListCovers: true, CoverTilt: true, TiltMax: 12,
		BgVocals: true, LyricsCredits: true,
		ReplayGain: "track", CacheMB: 64, ReadaheadSecs: 30, SeekStepSecs: 10, Autoplay: true, PersistQueue: true,
		Chunked: true, ChunkKB: 1024, Workers: 4, FullDownload: true, Prefetch: true, DirectYtdlp: true,
		FetchArtwork: true, PreferArtwork: true, KeepHistory: true, LogHTTP: true,
	}
}

// Load reads the config (or returns defaults).
func Load(p Paths) *Config {
	c := &Config{Theme: "fishalike", Volume: 80, Repeat: "off", Prefs: DefaultPrefs(), path: filepath.Join(p.Config, "config.json")}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, c)
	}
	if c.Theme == "" {
		c.Theme = "fishalike"
	}
	if c.Style == nil {
		c.Style = map[string]string{}
	}
	if c.Prefs.AnimSpeed <= 0 {
		c.Prefs.AnimSpeed = 1
	}
	return c
}

// StyleCSS compiles the Style map into CSS for a theme.
func (c *Config) StyleCSS(themeID string) string {
	rules := map[string][]string{}
	var order []string
	keys := make([]string, 0, len(c.Style))
	for k := range c.Style {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sel, prop, ok := strings.Cut(k, "|")
		if !ok || strings.TrimSpace(c.Style[k]) == "" {
			continue
		}
		if t, rest, ok := strings.Cut(sel, ":"); ok && t == "theme" {
			id, s2, _ := strings.Cut(rest, " ")
			if id != themeID {
				continue
			}
			sel = strings.TrimSpace(s2)
			if sel == "" {
				sel = ":root"
			}
		}
		if _, ok := rules[sel]; !ok {
			order = append(order, sel)
		}
		rules[sel] = append(rules[sel], "  "+prop+": "+c.Style[k]+";")
	}
	var b strings.Builder
	for _, sel := range order {
		b.WriteString(sel + " {\n" + strings.Join(rules[sel], "\n") + "\n}\n")
	}
	return b.String()
}

// Save writes the config atomically.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
