// Package app wires the non-visual parts of meofishes together: config,
// plugins, themes, catalog, library, the chunked stream proxy and the
// translations. The Qt interface (internal/ui) and the CLI both use it.
package app

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"meofishes/internal/builtin/artwork"
	"meofishes/internal/builtin/localfiles"
	"meofishes/internal/builtin/meolyrics"
	"meofishes/internal/catalog"
	"meofishes/internal/config"
	"meofishes/internal/i18n"
	"meofishes/internal/library"
	"meofishes/internal/netlog"
	"meofishes/internal/plugin"
	"meofishes/internal/stream"
	"meofishes/internal/theme"
)

// Core is the app without its interface.
type Core struct {
	Paths   config.Paths
	Cfg     *config.Config
	Host    *plugin.Host
	Plugins *plugin.Manager
	Themes  theme.Store
	Catalog *catalog.Catalog
	Library *library.Library
	Proxy   *stream.Proxy
	Assets  fs.FS

	logMu sync.Mutex
	logs  []LogLine
	OnLog func(LogLine) // called for every line (any goroutine)
}

// LogLine is one line of the app log.
type LogLine struct {
	At     time.Time
	Source string
	Text   string
}

func (l LogLine) String() string {
	return l.At.Format("15:04:05.000") + " [" + l.Source + "] " + l.Text
}

// New loads everything. quiet=true sends the log only to the in-memory
// buffer (the CLI prints to stderr itself).
func New(assets fs.FS) *Core {
	paths := config.DefaultPaths()
	cfg := config.Load(paths)
	c := &Core{Paths: paths, Cfg: cfg, Themes: theme.Store{Dir: paths.Themes}, Assets: assets,
		Catalog: &catalog.Catalog{Bundled: assets}}
	netlog.Install()
	netlog.Enable(cfg.Prefs.DevMode && cfg.Prefs.LogHTTP)
	i18n.Load(assets, "locales", filepath.Join(paths.Data, "locales"))
	i18n.Set(cfg.Language)

	c.Host = &plugin.Host{Version: config.Version, UserAgent: config.UserAgent, DataDir: paths.PData,
		Client: &http.Client{Timeout: 25 * time.Second}, Logf: c.Log}
	c.Plugins = plugin.NewManager(c.Host, paths.Plugins, &cfg.Plugins)
	c.Plugins.CacheDir = filepath.Join(paths.Cache, "lyrics")
	c.Plugins.Save = func() { _ = cfg.Save() }
	c.Plugins.Register(plugin.Builtin{Manifest: meolyrics.Manifest, New: meolyrics.New})
	c.Plugins.Register(plugin.Builtin{Manifest: artwork.Manifest, New: artwork.New})
	c.Plugins.Register(plugin.Builtin{Manifest: localfiles.Manifest, New: localfiles.New})
	if len(cfg.Plugins.Order[plugin.CapLyrics]) == 0 {
		cfg.Plugins.Order[plugin.CapLyrics] = []string{meolyrics.ID}
	}
	if len(cfg.Plugins.Order[plugin.CapArtwork]) == 0 {
		cfg.Plugins.Order[plugin.CapArtwork] = []string{artwork.ID}
	}
	c.Library = library.Open(filepath.Join(paths.Data, "library.json"))
	c.Library.KeepHistory = cfg.Prefs.KeepHistory
	c.FirstRun()
	c.Plugins.Load()
	c.ApplyEasings()
	return c
}

// Log records a line in the app log.
func (c *Core) Log(source, format string, args ...any) {
	l := LogLine{At: time.Now(), Source: source, Text: fmt.Sprintf(format, args...)}
	log.Println(l.String())
	c.logMu.Lock()
	c.logs = append(c.logs, l)
	if len(c.logs) > 2000 {
		c.logs = c.logs[len(c.logs)-2000:]
	}
	c.logMu.Unlock()
	if c.OnLog != nil {
		c.OnLog(l)
	}
}

// Logs returns a copy of the log buffer.
func (c *Core) Logs() []LogLine {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	return append([]LogLine(nil), c.logs...)
}

// FirstRun installs the bundled defaults (Fishalike, base plugins) the first
// time, and makes sure the default theme is always present.
func (c *Core) FirstRun() {
	if c.Themes.Get("fishalike") != nil && c.Cfg.FirstRunDone {
		c.migrate()
		return
	}
	for _, e := range c.Catalog.Defaults() {
		if e.Kind == "plugin" && c.Cfg.FirstRunDone {
			continue
		}
		if err := c.InstallEntry(e); err != nil {
			c.Log("setup", "install %s: %v", e.ID, err)
		}
	}
	c.Cfg.FirstRunDone = true
	_ = c.Cfg.Save()
}

// migrate updates bundled plugins installed by an older version (0.1 → 0.2:
// new plugin types, Deezer covers) and removes the old layout plugins.
func (c *Core) migrate() {
	changed := false
	for _, e := range c.Catalog.BuiltIn() {
		if e.Kind == "theme" { // bundled themes follow the app version
			if t := c.Themes.Get(e.ID); t != nil && t.Version != e.Version && e.Bundled != "" {
				if err := c.InstallEntry(e); err == nil {
					c.Log("setup", "updated theme %s to %s", e.ID, e.Version)
				}
			}
			continue
		}
		if !e.Default {
			continue
		}
		m, err := plugin.LoadManifest(filepath.Join(c.Paths.Plugins, e.ID))
		if (m == nil && err != nil) || (m != nil && m.Version != e.Version && e.Bundled != "") {
			if m == nil && c.Cfg.Plugins.Disabled[e.ID] {
				continue
			}
			if err := c.InstallEntry(e); err == nil {
				c.Log("setup", "updated %s to %s", e.ID, e.Version)
				changed = true
			}
		}
	}
	for _, id := range []string{"layout-focus", "layout-cinema"} { // layouts are not plugins anymore
		_ = removeAllIfExists(filepath.Join(c.Paths.Plugins, id))
	}
	if changed {
		_ = c.Cfg.Save()
	}
}

// InstallEntry installs a catalog entry (theme or plugin).
func (c *Core) InstallEntry(e catalog.Entry) error {
	src, cleanup, err := c.Catalog.Materialize(e)
	if err != nil {
		return err
	}
	defer cleanup()
	switch e.Kind {
	case "theme":
		_, err = c.Themes.Install(src)
	case "plugin":
		_, err = c.Plugins.InstallFrom(src)
	}
	return err
}

// CurrentTheme is the selected theme (falls back to Fishalike).
func (c *Core) CurrentTheme() *theme.Theme {
	t := c.Themes.Get(c.Cfg.Theme)
	if t == nil {
		t = c.Themes.Get("fishalike")
	}
	return t
}

// CompileTheme compiles theme + ani plugins + Settings style + user CSS.
// files are the files to watch for live reload.
func (c *Core) CompileTheme() (*theme.Compiled, []string, error) {
	t := c.CurrentTheme()
	if t == nil {
		return nil, nil, fmt.Errorf("no theme installed")
	}
	ani, aniFiles := c.Plugins.AnimationCSS()
	extra := strings.Join([]string{ani, c.Cfg.StyleCSS(t.ID), c.Cfg.UserCSS}, "\n")
	comp, files, err := c.Themes.Compile(t, extra)
	return comp, append(files, aniFiles...), err
}

// ApplyEasings registers the easing curves of the ani plugins.
func (c *Core) ApplyEasings() {
	theme.ResetCustomEasings()
	for name, b := range c.Plugins.Easings() {
		theme.RegisterEasing(name, b[0], b[1], b[2], b[3])
	}
}

// StartProxy starts the chunked download proxy with the current prefs.
func (c *Core) StartProxy() {
	if c.Proxy != nil {
		c.Proxy.SetOptions(c.proxyOptions())
		return
	}
	p, err := stream.New(c.proxyOptions())
	if err != nil {
		c.Log("stream", "proxy: %v", err)
		return
	}
	p.Logf = func(f string, a ...any) { c.Log("stream", f, a...) }
	c.Proxy = p
}

func (c *Core) proxyOptions() stream.Options {
	pr := c.Cfg.Prefs
	return stream.Options{ChunkSize: int64(max(64, pr.ChunkKB)) << 10, Workers: pr.Workers, Full: pr.FullDownload,
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"}
}

// Close stops plugins and the proxy and saves the config.
func (c *Core) Close() {
	c.Plugins.Close()
	if c.Proxy != nil {
		c.Proxy.Close()
	}
	_ = c.Cfg.Save()
}

func removeAllIfExists(dir string) error {
	if st, err := osStat(dir); err == nil && st.IsDir() {
		return osRemoveAll(dir)
	}
	return nil
}
