package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
	"meofishes/internal/pkgfs"
)

// State is the persisted plugin configuration (stored in config.json).
type State struct {
	Disabled    map[string]bool              `json:"disabled,omitempty"`
	Settings    map[string]map[string]string `json:"settings,omitempty"`
	LyricsOrder []string                     `json:"lyricsOrder,omitempty"` // lyrics plugins priority
	Layout      string                       `json:"layout,omitempty"`      // active layout plugin id ("" = default)
}

// BuiltinFactory creates the implementation of a builtin plugin. The returned
// value may implement Source, Browser, LyricsSource and Hook.
type BuiltinFactory func(host *Host, settings func() map[string]string) (any, error)

// Builtin is a plugin compiled into the app.
type Builtin struct {
	Manifest Manifest
	New      BuiltinFactory
}

// Plugin is a loaded plugin.
type Plugin struct {
	Manifest Manifest
	Dir      string // "" for builtins
	Builtin  bool
	Err      error // load error, shown in the UI
	impl     any
}

func (p *Plugin) Source() Source             { s, _ := p.impl.(Source); return s }
func (p *Plugin) Browser() Browser           { b, _ := p.impl.(Browser); return b }
func (p *Plugin) LyricsSource() LyricsSource { l, _ := p.impl.(LyricsSource); return l }
func (p *Plugin) Hook() Hook                 { h, _ := p.impl.(Hook); return h }

// Manager discovers, loads and runs plugins.
type Manager struct {
	Host      *Host
	Dir       string // user plugins: ~/.local/share/meofishes/plugins
	CacheDir  string // lyrics cache
	Save      func() // persists State
	State     *State
	builtins  []Builtin
	mu        sync.RWMutex
	plugins   map[string]*Plugin
	order     []string
	listeners []func()
}

// NewManager creates a manager. Call Register for builtins, then Load.
func NewManager(host *Host, dir string, st *State) *Manager {
	if st.Disabled == nil {
		st.Disabled = map[string]bool{}
	}
	if st.Settings == nil {
		st.Settings = map[string]map[string]string{}
	}
	return &Manager{Host: host, Dir: dir, State: st, plugins: map[string]*Plugin{}}
}

// Register adds a builtin plugin.
func (mg *Manager) Register(b Builtin) { mg.builtins = append(mg.builtins, b) }

// OnChange registers a callback run after plugins are (re)loaded or toggled.
func (mg *Manager) OnChange(f func()) { mg.listeners = append(mg.listeners, f) }

func (mg *Manager) changed() {
	for _, f := range mg.listeners {
		f()
	}
}

// Settings returns the effective settings (defaults + user values) of a plugin.
func (mg *Manager) Settings(id string) map[string]string {
	mg.mu.RLock()
	p := mg.plugins[id]
	mg.mu.RUnlock()
	out := map[string]string{}
	if p != nil {
		for _, s := range p.Manifest.Settings {
			out[s.Key] = s.Default
		}
	}
	for k, v := range mg.State.Settings[id] {
		out[k] = v
	}
	return out
}

// SetSettings stores new settings and restarts the plugin.
func (mg *Manager) SetSettings(id string, values map[string]string) error {
	mg.State.Settings[id] = values
	if mg.Save != nil {
		mg.Save()
	}
	if p := mg.Get(id); p != nil {
		if sc, ok := p.impl.(interface{ SettingsChanged() }); ok {
			sc.SettingsChanged()
		} else if !p.Builtin && p.Manifest.Runtime == RuntimeScript {
			return mg.reload(id)
		}
	}
	mg.changed()
	return nil
}

// Enabled reports whether a plugin is enabled.
func (mg *Manager) Enabled(id string) bool { return !mg.State.Disabled[id] }

// SetEnabled toggles a plugin.
func (mg *Manager) SetEnabled(id string, on bool) error {
	if on {
		delete(mg.State.Disabled, id)
	} else {
		mg.State.Disabled[id] = true
	}
	if mg.Save != nil {
		mg.Save()
	}
	return mg.reload(id)
}

// Load (re)loads every plugin.
func (mg *Manager) Load() {
	mg.mu.Lock()
	for _, p := range mg.plugins {
		closeImpl(p)
	}
	mg.plugins = map[string]*Plugin{}
	mg.order = nil
	mg.mu.Unlock()

	for _, b := range mg.builtins {
		b := b
		p := &Plugin{Manifest: b.Manifest, Builtin: true}
		mg.add(p)
		if mg.Enabled(p.Manifest.ID) {
			p.impl, p.Err = b.New(mg.Host, func() map[string]string { return mg.Settings(b.Manifest.ID) })
		}
	}
	entries, _ := os.ReadDir(mg.Dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(mg.Dir, e.Name())
		m, err := LoadManifest(dir)
		if m == nil {
			mg.Host.logf(e.Name(), "skipped: %v", err)
			continue
		}
		p := &Plugin{Manifest: *m, Dir: dir, Err: err}
		mg.add(p)
		if err == nil && mg.Enabled(m.ID) {
			p.impl, p.Err = mg.start(p)
		}
	}
	mg.changed()
}

func (mg *Manager) add(p *Plugin) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	if old, ok := mg.plugins[p.Manifest.ID]; ok {
		mg.Host.logf(p.Manifest.ID, "duplicate id; %s overrides %s", p.Dir, old.Dir)
	} else {
		mg.order = append(mg.order, p.Manifest.ID)
	}
	mg.plugins[p.Manifest.ID] = p
}

func (mg *Manager) start(p *Plugin) (any, error) {
	m := &p.Manifest
	settings := func() map[string]string { return mg.Settings(m.ID) }
	switch m.Runtime {
	case RuntimeDeclarative:
		return &declarative{m: m, host: mg.Host, env: &moduleEnv{m: m, dir: p.Dir, host: mg.Host}, settings: settings}, nil
	case RuntimeScript:
		return newScript(m, p.Dir, mg.Host, settings)
	case RuntimeProcess:
		return newProcess(m, p.Dir, mg.Host, settings)
	case "", RuntimeBuiltin:
		return nil, nil // layout-only plugin
	}
	return nil, fmt.Errorf("unknown runtime %q", m.Runtime)
}

func closeImpl(p *Plugin) {
	if c, ok := p.impl.(Closer); ok {
		_ = c.Close()
	}
	p.impl = nil
}

func (mg *Manager) reload(id string) error {
	p := mg.Get(id)
	if p == nil {
		return fmt.Errorf("no plugin %q", id)
	}
	closeImpl(p)
	p.Err = nil
	if mg.Enabled(id) {
		if p.Builtin {
			for _, b := range mg.builtins {
				if b.Manifest.ID == id {
					p.impl, p.Err = b.New(mg.Host, func() map[string]string { return mg.Settings(id) })
				}
			}
		} else {
			m, err := LoadManifest(p.Dir)
			if m != nil {
				p.Manifest = *m
			}
			if err == nil {
				p.impl, p.Err = mg.start(p)
			} else {
				p.Err = err
			}
		}
	}
	mg.changed()
	return p.Err
}

// Close stops every plugin.
func (mg *Manager) Close() {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	for _, p := range mg.plugins {
		closeImpl(p)
	}
}

// Get returns a plugin by id.
func (mg *Manager) Get(id string) *Plugin {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return mg.plugins[id]
}

// All returns every plugin in load order.
func (mg *Manager) All() []*Plugin {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	out := make([]*Plugin, 0, len(mg.order))
	for _, id := range mg.order {
		out = append(out, mg.plugins[id])
	}
	return out
}

// OfType returns enabled, working plugins providing t.
func (mg *Manager) OfType(t string) []*Plugin {
	var out []*Plugin
	for _, p := range mg.All() {
		if p.Manifest.Has(t) && mg.Enabled(p.Manifest.ID) && p.Err == nil {
			out = append(out, p)
		}
	}
	return out
}

// Sources returns enabled source plugins.
func (mg *Manager) Sources() []*Plugin {
	var out []*Plugin
	for _, p := range mg.OfType(TypeSource) {
		if p.Source() != nil {
			out = append(out, p)
		}
	}
	return out
}

// Resolve finds the stream of a track through its source plugin.
func (mg *Manager) Resolve(ctx context.Context, t core.Track) (*Stream, error) {
	p := mg.Get(t.Source)
	if p == nil || p.Source() == nil {
		if t.StreamURL != "" {
			return &Stream{URL: t.StreamURL, Page: !looksLikeMedia(t.StreamURL)}, nil
		}
		return nil, fmt.Errorf("source %q is not available", t.Source)
	}
	return p.Source().Resolve(ctx, t)
}

// LyricsPlugins returns enabled lyrics plugins in priority order.
func (mg *Manager) LyricsPlugins() []*Plugin {
	ps := mg.OfType(TypeLyrics)
	rank := map[string]int{}
	for i, id := range mg.State.LyricsOrder {
		rank[id] = i + 1
	}
	sort.SliceStable(ps, func(i, j int) bool {
		ri, rj := rank[ps[i].Manifest.ID], rank[ps[j].Manifest.ID]
		if ri == 0 {
			ri = 1 << 20
		}
		if rj == 0 {
			rj = 1 << 20
		}
		return ri < rj
	})
	return ps
}

// FindLyrics asks the lyrics plugins in priority order (best sync wins, ties
// go to the higher priority).
func (mg *Manager) FindLyrics(ctx context.Context, t core.Track) (*core.Lyrics, string, error) {
	return mg.findLyrics(ctx, t, false)
}

// FindLyricsFresh ignores the cache.
func (mg *Manager) FindLyricsFresh(ctx context.Context, t core.Track) (*core.Lyrics, string, error) {
	return mg.findLyrics(ctx, t, true)
}

func (mg *Manager) findLyrics(ctx context.Context, t core.Track, fresh bool) (*core.Lyrics, string, error) {
	var provs []lyrics.Provider
	for _, p := range mg.LyricsPlugins() {
		if ls := p.LyricsSource(); ls != nil {
			provs = append(provs, lyricsAdapter{p.Manifest.ID, p.Manifest.Name, ls})
		}
	}
	if len(provs) == 0 {
		return nil, "no lyrics plugin enabled", lyrics.ErrNotFound
	}
	ch := &lyrics.Chain{Providers: provs, CacheDir: mg.CacheDir, Timeout: 40 * time.Second, SkipRead: fresh}
	q := lyrics.Query{Title: t.Title, Artist: t.Artist, Album: t.Album, DurationMs: t.DurationMs, ISRC: t.ISRC, VideoID: t.VideoID}
	l, att, err := ch.Find(ctx, q)
	return l, lyrics.Describe(att), err
}

// Emit sends an event to hook plugins.
func (mg *Manager) Emit(name string, payload any) {
	for _, p := range mg.OfType(TypeHook) {
		h := p.Hook()
		if h == nil {
			continue
		}
		evs := p.Manifest.Events
		ok := len(evs) == 0
		for _, e := range evs {
			if e == "*" || e == name || (strings.HasSuffix(e, ".*") && strings.HasPrefix(name, strings.TrimSuffix(e, "*"))) {
				ok = true
			}
		}
		if ok {
			h.Event(name, payload)
		}
	}
}

// Layouts returns the layout plugins.
func (mg *Manager) Layouts() []*Plugin { return mg.OfType(TypeLayout) }

// ActiveLayout returns the chosen layout tree (or the default one).
func (mg *Manager) ActiveLayout() (*LayoutNode, string) {
	if id := mg.State.Layout; id != "" {
		if p := mg.Get(id); p != nil && p.Manifest.Layout != nil && mg.Enabled(id) {
			return p.Manifest.Layout, id
		}
	}
	return DefaultLayout(), ""
}

// ---------------------------------------------------------------- install

// Build runs the manifest's build command (process plugins). The UI asks the
// user before calling it, because it executes code from the plugin.
func (mg *Manager) Build(id string) (string, error) {
	p := mg.Get(id)
	if p == nil || p.Dir == "" || p.Manifest.Build == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", p.Manifest.Build)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("build failed: %w", err)
	}
	return string(out), mg.reload(id)
}

// InstallFrom installs a plugin from a folder, a .zip file or an http(s) URL
// to a .zip. It returns the installed manifest.
func (mg *Manager) InstallFrom(src string) (*Manifest, error) {
	dir, cleanup, err := pkgfs.Fetch(src)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	root, err := pkgfs.FindRoot(dir, "plugin.json")
	if err != nil {
		return nil, err
	}
	m, err := LoadManifest(root)
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(mg.Dir, m.ID)
	if err := pkgfs.ReplaceDir(root, dst); err != nil {
		return nil, err
	}
	mg.Load()
	return m, nil
}

// Uninstall removes a user plugin.
func (mg *Manager) Uninstall(id string) error {
	p := mg.Get(id)
	if p == nil {
		return fmt.Errorf("no plugin %q", id)
	}
	if p.Builtin {
		return errors.New("builtin plugins can be disabled but not removed")
	}
	closeImpl(p)
	if err := os.RemoveAll(p.Dir); err != nil {
		return err
	}
	delete(mg.State.Settings, id)
	delete(mg.State.Disabled, id)
	if mg.Save != nil {
		mg.Save()
	}
	mg.Load()
	return nil
}
