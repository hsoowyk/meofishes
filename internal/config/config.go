// Package config stores the user's settings in ~/.config/meofishes/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"meofishes/internal/plugin"
)

// Version of the app.
const Version = "0.1.0"

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

	Volume   float64 `json:"volume"`
	Source   string  `json:"source,omitempty"` // last selected source
	Shuffle  bool    `json:"shuffle,omitempty"`
	Repeat   string  `json:"repeat,omitempty"` // off | all | one
	WindowW  int     `json:"windowW,omitempty"`
	WindowH  int     `json:"windowH,omitempty"`
	Language string  `json:"language,omitempty"`

	// Contact is sent in the User-Agent. lrc.red requires an app name, a link
	// and an e-mail; Unison asks for the app name and a link.
	ContactEmail string `json:"contactEmail,omitempty"`
	ContactURL   string `json:"contactUrl,omitempty"`

	Catalogs []string `json:"catalogs,omitempty"` // extra catalog index URLs

	Plugins plugin.State `json:"plugins"`

	FirstRunDone bool `json:"firstRunDone,omitempty"`

	path string
	mu   sync.Mutex
}

// Load reads the config (or returns defaults).
func Load(p Paths) *Config {
	c := &Config{Theme: "fishalike", Volume: 80, Repeat: "off", path: filepath.Join(p.Config, "config.json")}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, c)
	}
	if c.Theme == "" {
		c.Theme = "fishalike"
	}
	return c
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

// UserAgent builds the User-Agent sent to services.
func (c *Config) UserAgent() string {
	var extra []string
	if u := strings.TrimSpace(c.ContactURL); u != "" {
		extra = append(extra, u)
	}
	if e := strings.TrimSpace(c.ContactEmail); e != "" {
		extra = append(extra, e)
	}
	ua := "meofishes/" + Version
	if len(extra) > 0 {
		ua += " (" + strings.Join(extra, "; ") + ")"
	}
	return ua
}
