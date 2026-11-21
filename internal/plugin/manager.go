package plugin

import (
	"context"
	"errors"
	"fmt"
	"meofishes/internal/sysproc"
	"os"
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
	Disabled map[string]bool              `json:"disabled,omitempty"`
	Settings map[string]map[string]string `json:"settings,omitempty"`
	// Order is the priority of the plugins for one capability
	// ("lyrics", "artwork", "search", "match", "recommend").
	Order map[string][]string `json:"order,omitempty"`
	// IconPack / Player: the active xtm plugin ("" = built-in).
	IconPack string `json:"iconPack,omitempty"`
	Player   string `json:"player,omitempty"`

	LyricsOrder []string `json:"lyricsOrder,omitempty"` // legacy (0.1), moved to Order["lyrics"]
}

// BuiltinFactory creates the implementation of a builtin plugin. The returned
// value may implement any of the capability interfaces (Searcher, Resolver,
// Browser, LyricsSource, ArtworkSource, Hook…).
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
	Stats    *Stats // call counters for the developer mode
}

// Stats counts calls per capability (developer mode).
type Stats struct {
	mu    sync.Mutex
	Calls map[string]*CallStat
}

// CallStat is the record of one capability.
type CallStat struct {
	Count, Errors int
	Total, Last   time.Duration
	LastErr       string
	LastAt        time.Time
}

func (s *Stats) record(cap string, d time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Calls == nil {
		s.Calls = map[string]*CallStat{}
	}
	c := s.Calls[cap]
	if c == nil {
		c = &CallStat{}
		s.Calls[cap] = c
	}
	c.Count++
	c.Total += d
	c.Last = d
	c.LastAt = time.Now()
	if err != nil && !errors.Is(err, lyrics.ErrNotFound) {
		c.Errors++
		c.LastErr = err.Error()
	}
}

// Snapshot copies the counters.
func (s *Stats) Snapshot() map[string]CallStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]CallStat{}
	for k, v := range s.Calls {
		out[k] = *v
	}
	return out
}

// Impl exposes the runtime (developer mode / tests).
func (p *Plugin) Impl() any { return p.impl }

// Can reports whether the running plugin provides a capability and its
// declared types permit it.
func (p *Plugin) Can(c string) bool {
	if p == nil || !p.Manifest.Allows(c) {
		return false
	}
	return p.provides(c)
}

// Denied lists capabilities the plugin implements but its types do not
// permit (shown in the plugin card and the developer mode).
func (p *Plugin) Denied() []string {
	var out []string
	for _, c := range AllCaps {
		if p.provides(c) && !p.Manifest.Allows(c) {
			out = append(out, c)
		}
	}
	return out
}

func (p *Plugin) provides(c string) bool {
	if p == nil || p.impl == nil {
		switch c { // asset-only plugins
		case CapIcons:
			return p != nil && p.Manifest.Icons != ""
		case CapAnimation:
			return p != nil && (p.Manifest.Animations != "" || len(p.Manifest.Easings) > 0)
		}
		return false
	}
	if cp, ok := p.impl.(capable); ok {
		caps := cp.Caps()
		if c == CapMatch {
			return p.Manifest.Match && (caps[CapStream] || caps[CapMatch])
		}
		if c == CapIcons {
			return p.Manifest.Icons != ""
		}
		if c == CapAnimation {
			return p.Manifest.Animations != "" || len(p.Manifest.Easings) > 0
		}
		return caps[c]
	}
	ok := false
	switch c {
	case CapSearch:
		_, ok = p.impl.(Searcher)
	case CapStream:
		_, ok = p.impl.(Resolver)
	case CapMatch:
		_, ok = p.impl.(Resolver)
		ok = ok && p.Manifest.Match
	case CapHome, CapOpen:
		_, ok = p.impl.(Browser)
	case CapRecommend:
		_, ok = p.impl.(Recommender)
	case CapPlaylists:
		_, ok = p.impl.(PlaylistProvider)
	case CapLyrics:
		_, ok = p.impl.(LyricsSource)
	case CapArtwork:
		_, ok = p.impl.(ArtworkSource)
	case CapEvents:
		_, ok = p.impl.(Hook)
	case CapPlayer:
		ok = p.Manifest.Player
	case CapIcons:
		ok = p.Manifest.Icons != ""
	case CapAnimation:
		ok = p.Manifest.Animations != "" || len(p.Manifest.Easings) > 0
	}
	return ok
}

// Caps lists the capabilities the plugin provides.
func (p *Plugin) Caps() []string {
	var out []string
	for _, c := range AllCaps {
		if p.Can(c) {
			out = append(out, c)
		}
	}
	return out
}

// AllCaps lists every capability.
var AllCaps = []string{CapSearch, CapHome, CapRecommend, CapPlaylists, CapOpen, CapStream, CapMatch,
	CapLyrics, CapArtwork, CapEvents, CapPlayer, CapIcons, CapAnimation, CapLibrary}

func (p *Plugin) Searcher() Searcher           { s, _ := p.impl.(Searcher); return s }
func (p *Plugin) Resolver() Resolver           { r, _ := p.impl.(Resolver); return r }
func (p *Plugin) Browser() Browser             { b, _ := p.impl.(Browser); return b }
func (p *Plugin) Recommender() Recommender     { r, _ := p.impl.(Recommender); return r }
func (p *Plugin) Playlists() PlaylistProvider  { r, _ := p.impl.(PlaylistProvider); return r }
func (p *Plugin) LyricsSource() LyricsSource   { l, _ := p.impl.(LyricsSource); return l }
func (p *Plugin) ArtworkSource() ArtworkSource { a, _ := p.impl.(ArtworkSource); return a }
func (p *Plugin) Hook() Hook                   { h, _ := p.impl.(Hook); return h }

// Source is kept for older callers: a plugin that searches.
func (p *Plugin) Source() Searcher { return p.Searcher() }

// timed runs f and records its duration under cap.
func (p *Plugin) timed(cap string, f func() error) error {
	t0 := time.Now()
	err := f()
	if p.Stats != nil {
		p.Stats.record(cap, time.Since(t0), err)
	}
	return err
}

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
	lastMu    sync.Mutex
	last      *LastLyrics
	// stMu guards the maps of State: background searches read them while
	// the interface changes them (a concurrent map access kills the app).
	stMu  sync.RWMutex
	alias map[string]string // old id → id
}

// NewManager creates a manager. Call Register for builtins, then Load.
func NewManager(host *Host, dir string, st *State) *Manager {
	if st.Disabled == nil {
		st.Disabled = map[string]bool{}
	}
	if st.Settings == nil {
		st.Settings = map[string]map[string]string{}
	}
	if st.Order == nil {
		st.Order = map[string][]string{}
	}
	if len(st.LyricsOrder) > 0 && len(st.Order[CapLyrics]) == 0 {
		st.Order[CapLyrics] = st.LyricsOrder
	}
	st.LyricsOrder = nil
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
	mg.stMu.RLock()
	for k, v := range mg.State.Settings[id] {
		out[k] = v
	}
	mg.stMu.RUnlock()
	return out
}

// SetSettings stores new settings and restarts the plugin.
func (mg *Manager) SetSettings(id string, values map[string]string) error {
	mg.stMu.Lock()
	mg.State.Settings[id] = values
	mg.stMu.Unlock()
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
func (mg *Manager) Enabled(id string) bool {
	mg.stMu.RLock()
	defer mg.stMu.RUnlock()
	return !mg.State.Disabled[id]
}

// SetEnabled toggles a plugin.
func (mg *Manager) SetEnabled(id string, on bool) error {
	mg.stMu.Lock()
	if on {
		delete(mg.State.Disabled, id)
	} else {
		mg.State.Disabled[id] = true
	}
	mg.stMu.Unlock()
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
		mg.add(&Plugin{Manifest: b.Manifest, Builtin: true, Stats: &Stats{}})
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
		mg.add(&Plugin{Manifest: *m, Dir: dir, Err: err, Stats: &Stats{}})
	}
	// dep plugins first, so the others can use them
	all := mg.All()
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Manifest.Has(TypeDep) && !all[j].Manifest.Has(TypeDep)
	})
	for _, p := range all {
		if p.Err == nil && mg.Enabled(p.Manifest.ID) {
			mg.startPlugin(p)
		}
	}
	mg.changed()
}

// missingDeps lists the required plugins that are not installed/enabled.
func (mg *Manager) missingDeps(m *Manifest) []string {
	var out []string
	for _, id := range m.Requires {
		d := mg.Get(id)
		if d == nil || !mg.Enabled(id) || d.Err != nil {
			out = append(out, id)
		}
	}
	return out
}

func (mg *Manager) startPlugin(p *Plugin) {
	if miss := mg.missingDeps(&p.Manifest); len(miss) > 0 {
		p.Err = fmt.Errorf("missing dependency: %s (install or enable it)", strings.Join(miss, ", "))
		return
	}
	if p.Builtin {
		for _, b := range mg.builtins {
			if b.Manifest.ID == p.Manifest.ID {
				id := b.Manifest.ID
				p.impl, p.Err = b.New(mg.Host, func() map[string]string { return mg.Settings(id) })
			}
		}
		return
	}
	p.impl, p.Err = mg.start(p)
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
	if mg.alias == nil {
		mg.alias = map[string]string{}
	}
	for _, a := range p.Manifest.Aliases {
		mg.alias[a] = p.Manifest.ID
	}
}

// depDir returns the folder of an enabled dep plugin required by m.
func (mg *Manager) depDir(m *Manifest, id string) (string, *Manifest, error) {
	ok := false
	for _, r := range m.Requires {
		ok = ok || r == id
	}
	if !ok {
		return "", nil, fmt.Errorf("dep:%s is not listed in \"requires\"", id)
	}
	d := mg.Get(id)
	if d == nil || d.Dir == "" || !mg.Enabled(id) {
		return "", nil, fmt.Errorf("dependency %s is not installed", id)
	}
	if !d.Manifest.Has(TypeDep) {
		return "", nil, fmt.Errorf("%s is not a dep plugin (its types do not allow being used as a library)", id)
	}
	return d.Dir, &d.Manifest, nil
}

// depEnv is the extra environment for process plugins: PATH gets the bin
// folders of their dependencies, and MEOFISHES_DEP_<ID> points at each dep.
func (mg *Manager) depEnv(m *Manifest) []string {
	var env, path []string
	for _, id := range m.Requires {
		dir, dm, err := mg.depDir(m, id)
		if err != nil {
			continue
		}
		key := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(id))
		env = append(env, "MEOFISHES_DEP_"+key+"="+dir)
		if dm.Bin != "" {
			path = append(path, filepath.Join(dir, dm.Bin))
		}
	}
	if len(path) > 0 {
		env = append(env, "PATH="+strings.Join(path, string(os.PathListSeparator))+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env
}

func (mg *Manager) start(p *Plugin) (any, error) {
	m := &p.Manifest
	settings := func() map[string]string { return mg.Settings(m.ID) }
	env := &moduleEnv{m: m, dir: p.Dir, host: mg.Host}
	switch m.Runtime {
	case RuntimeDeclarative:
		return &declarative{m: m, host: mg.Host, env: env, settings: settings}, nil
	case RuntimeScript:
		return newScript(m, p.Dir, mg.Host, settings, func(id string) (string, *Manifest, error) { return mg.depDir(m, id) })
	case RuntimeProcess:
		return newProcess(m, p.Dir, mg.Host, settings, mg.depEnv(m))
	case "", RuntimeBuiltin:
		return nil, nil // asset-only plugin (ani CSS, icon pack)
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
	if !p.Builtin {
		m, err := LoadManifest(p.Dir)
		if m != nil {
			p.Manifest = *m
		}
		p.Err = err
	}
	if p.Err == nil && mg.Enabled(id) {
		mg.startPlugin(p)
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

// Get returns a plugin by id (or by one of its old ids).
func (mg *Manager) Get(id string) *Plugin {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	if p := mg.plugins[id]; p != nil {
		return p
	}
	if real, ok := mg.alias[id]; ok {
		return mg.plugins[real]
	}
	return nil
}

// Migrate moves settings, priorities and the enabled state stored under
// old ids to the plugin that replaced them. prefix maps an old id to the
// prefix its settings get in the new plugin.
func (mg *Manager) Migrate(oldID, newID, prefix string) {
	mg.stMu.Lock()
	defer mg.stMu.Unlock()
	st := mg.State
	if old, ok := st.Settings[oldID]; ok {
		if st.Settings[newID] == nil {
			st.Settings[newID] = map[string]string{}
		}
		for k, v := range old {
			if _, exists := st.Settings[newID][prefix+k]; !exists {
				st.Settings[newID][prefix+k] = v
			}
		}
		delete(st.Settings, oldID)
	}
	for c, ids := range st.Order {
		var out []string
		seen := map[string]bool{}
		for _, id := range ids {
			if id == oldID {
				id = newID
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		st.Order[c] = out
	}
	delete(st.Disabled, oldID)
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

// usable reports whether a plugin is enabled and loaded without errors.
func (mg *Manager) usable(p *Plugin) bool {
	return p != nil && mg.Enabled(p.Manifest.ID) && p.Err == nil
}

// OfType returns enabled, working plugins of type t.
func (mg *Manager) OfType(t string) []*Plugin {
	var out []*Plugin
	for _, p := range mg.All() {
		if p.Manifest.Has(t) && mg.usable(p) {
			out = append(out, p)
		}
	}
	return out
}

// WithCap returns enabled, working plugins providing a capability, in the
// user's priority order for it (State.Order[cap]).
func (mg *Manager) WithCap(c string) []*Plugin {
	var out []*Plugin
	for _, p := range mg.All() {
		if mg.usable(p) && p.Can(c) {
			out = append(out, p)
		}
	}
	rank := map[string]int{}
	mg.stMu.RLock()
	for i, id := range mg.State.Order[c] {
		rank[id] = i + 1
	}
	mg.stMu.RUnlock()
	r := func(id string) int {
		if v := rank[id]; v > 0 {
			return v
		}
		return 1 << 20
	}
	sort.SliceStable(out, func(i, j int) bool { return r(out[i].Manifest.ID) < r(out[j].Manifest.ID) })
	return out
}

// SetOrder stores the priority of plugins for a capability.
func (mg *Manager) SetOrder(c string, ids []string) {
	mg.stMu.Lock()
	mg.State.Order[c] = ids
	mg.stMu.Unlock()
	if mg.Save != nil {
		mg.Save()
	}
	mg.changed()
}

// Sources returns the plugins that can search (src "search").
func (mg *Manager) Sources() []*Plugin { return mg.WithCap(CapSearch) }

// Search runs a query on one plugin.
func (mg *Manager) Search(ctx context.Context, p *Plugin, q string) (*core.SearchResult, error) {
	var r *core.SearchResult
	err := p.timed(CapSearch, func() (err error) {
		r, err = p.Searcher().Search(ctx, q)
		return
	})
	return r, err
}

// Home loads the start page of a src plugin.
func (mg *Manager) Home(ctx context.Context, p *Plugin) (*core.SearchResult, error) {
	var r *core.SearchResult
	err := p.timed(CapHome, func() (err error) {
		r, err = p.Browser().Home(ctx)
		return
	})
	return r, err
}

// Open loads the tracks of a collection from the plugin that listed it.
func (mg *Manager) Open(ctx context.Context, c core.Collection) (*core.Collection, error) {
	p := mg.Get(c.Source)
	if !mg.usable(p) || !p.Can(CapOpen) {
		return &c, nil
	}
	var out *core.Collection
	err := p.timed(CapOpen, func() (err error) {
		out, err = p.Browser().Open(ctx, c)
		return
	})
	return out, err
}

// Resolve finds the stream of a track: first its own plugin, then a direct
// URL, then the fnt plugins that can "match" any track by title/artist.
func (mg *Manager) Resolve(ctx context.Context, t core.Track) (*Stream, error) {
	var firstErr error
	if p := mg.Get(t.Source); mg.usable(p) && p.Can(CapStream) {
		var st *Stream
		err := p.timed(CapStream, func() (err error) {
			st, err = p.Resolver().Resolve(ctx, t)
			return
		})
		if err == nil && st != nil && st.URL != "" {
			st.Via = p.Manifest.ID
			return st, nil
		}
		firstErr = err
	}
	if t.StreamURL != "" {
		return &Stream{URL: t.StreamURL, Page: !looksLikeMedia(t.StreamURL), Via: "direct"}, nil
	}
	for _, p := range mg.WithCap(CapMatch) {
		if p.Manifest.ID == t.Source {
			continue
		}
		var st *Stream
		err := p.timed(CapMatch, func() (err error) {
			st, err = p.Resolver().Resolve(ctx, t)
			return
		})
		if err == nil && st != nil && st.URL != "" {
			st.Via = p.Manifest.ID
			return st, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no plugin can play %q (install an fnt plugin with \"match\", e.g. yt-dlp)", t.String())
	}
	return nil, firstErr
}

// Recommend asks src plugins for tracks similar to seed (its own source first).
func (mg *Manager) Recommend(ctx context.Context, seed core.Track) ([]core.Track, string, error) {
	ps := mg.WithCap(CapRecommend)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Manifest.ID == seed.Source && ps[j].Manifest.ID != seed.Source })
	var lastErr error
	for _, p := range ps {
		var r *core.SearchResult
		err := p.timed(CapRecommend, func() (err error) {
			r, err = p.Recommender().Recommend(ctx, seed)
			return
		})
		if err == nil && r != nil && len(r.Tracks) > 0 {
			return r.Tracks, p.Manifest.ID, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no recommendations")
	}
	return nil, "", lastErr
}

// RemotePlaylists lists the playlists of every src plugin that has them.
func (mg *Manager) RemotePlaylists(ctx context.Context) ([]core.Collection, []error) {
	var out []core.Collection
	var errs []error
	for _, p := range mg.WithCap(CapPlaylists) {
		var cs []core.Collection
		err := p.timed(CapPlaylists, func() (err error) {
			cs, err = p.Playlists().Playlists(ctx)
			return
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Manifest.Name, err))
			continue
		}
		for i := range cs {
			cs[i].Source = p.Manifest.ID
			if cs[i].Kind == "" {
				cs[i].Kind = "playlist"
			}
		}
		out = append(out, cs...)
	}
	return out, errs
}

// Artwork finds a cover for a track through the fnt "artwork" plugins.
func (mg *Manager) Artwork(ctx context.Context, t core.Track) (string, error) {
	q := lyrics.Query{Title: t.Title, Artist: t.Artist, Album: t.Album, DurationMs: t.DurationMs, ISRC: t.ISRC, VideoID: t.VideoID}
	var lastErr error = lyrics.ErrNotFound
	for _, p := range mg.WithCap(CapArtwork) {
		var u string
		err := p.timed(CapArtwork, func() (err error) {
			u, err = p.ArtworkSource().Artwork(ctx, q)
			return
		})
		if err == nil && u != "" {
			return u, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	return "", lastErr
}

// LyricsPlugins returns enabled lyrics plugins in priority order.
func (mg *Manager) LyricsPlugins() []*Plugin { return mg.WithCap(CapLyrics) }

// FindLyrics asks the lyrics plugins in priority order (best sync wins, ties
// go to the higher priority).
func (mg *Manager) FindLyrics(ctx context.Context, t core.Track) (*core.Lyrics, string, error) {
	return mg.findLyrics(ctx, t, false)
}

// FindLyricsFresh ignores the cache.
func (mg *Manager) FindLyricsFresh(ctx context.Context, t core.Track) (*core.Lyrics, string, error) {
	return mg.findLyrics(ctx, t, true)
}

// LastLyrics is the attempt log of the last lyrics search (developer mode).
type LastLyrics struct {
	Track    core.Track
	Attempts []lyrics.Attempt
	At       time.Time
}

func (mg *Manager) findLyrics(ctx context.Context, t core.Track, fresh bool) (*core.Lyrics, string, error) {
	var provs []lyrics.Provider
	for _, p := range mg.LyricsPlugins() {
		if ls := p.LyricsSource(); ls != nil {
			provs = append(provs, lyricsAdapter{p, ls})
		}
	}
	if len(provs) == 0 {
		return nil, "no lyrics plugin enabled", lyrics.ErrNotFound
	}
	ch := &lyrics.Chain{Providers: provs, CacheDir: mg.CacheDir, Timeout: 40 * time.Second, SkipRead: fresh}
	q := lyrics.Query{Title: t.Title, Artist: t.Artist, Album: t.Album, DurationMs: t.DurationMs, ISRC: t.ISRC, VideoID: t.VideoID}
	l, att, err := ch.Find(ctx, q)
	mg.lastMu.Lock()
	mg.last = &LastLyrics{Track: t, Attempts: att, At: time.Now()}
	mg.lastMu.Unlock()
	return l, lyrics.Describe(att), err
}

// LastLyricsSearch returns the attempts of the latest lyrics search.
func (mg *Manager) LastLyricsSearch() *LastLyrics {
	mg.lastMu.Lock()
	defer mg.lastMu.Unlock()
	return mg.last
}

// Emit sends an event to evt plugins (and any plugin that lists "events").
func (mg *Manager) Emit(name string, payload any) {
	for _, p := range mg.WithCap(CapEvents) {
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

// AnimationCSS returns the CSS layers of the enabled ani plugins, in their
// priority order, and the files they come from (for live reload).
func (mg *Manager) AnimationCSS() (css string, files []string) {
	var b strings.Builder
	for _, p := range mg.WithCap(CapAnimation) {
		if p.Manifest.Animations == "" || p.Dir == "" {
			continue
		}
		f := filepath.Join(p.Dir, p.Manifest.Animations)
		data, err := os.ReadFile(f)
		if err != nil {
			mg.Host.logf(p.Manifest.ID, "animations: %v", err)
			continue
		}
		b.WriteString("\n/* ani: " + p.Manifest.ID + " */\n")
		b.Write(data)
		files = append(files, f)
	}
	return b.String(), files
}

// Easings returns the cubic-bezier curves of the enabled ani plugins.
func (mg *Manager) Easings() map[string][4]float64 {
	out := map[string][4]float64{}
	for _, p := range mg.WithCap(CapAnimation) {
		for k, v := range p.Manifest.Easings {
			out[k] = v
		}
	}
	return out
}

// IconPack returns the folder of the active icon pack ("" = built-in icons).
func (mg *Manager) IconPack() (dir string, tint bool) {
	p := mg.Get(mg.State.IconPack)
	if !mg.usable(p) || p.Manifest.Icons == "" || p.Dir == "" || !p.Manifest.Allows(CapIcons) {
		return "", false
	}
	tint = true
	if p.Manifest.IconsTint != nil {
		tint = *p.Manifest.IconsTint
	}
	return filepath.Join(p.Dir, p.Manifest.Icons), tint
}

// PlayerPlugin returns the active player plugin (nil = mpv).
func (mg *Manager) PlayerPlugin() *Plugin {
	p := mg.Get(mg.State.Player)
	if !mg.usable(p) || !p.Manifest.Player || !p.Manifest.Allows(CapPlayer) {
		return nil
	}
	return p
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
	cmd := sysproc.Shell(ctx, p.Manifest.Build)
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
	for _, o := range mg.All() {
		for _, r := range o.Manifest.Requires {
			if r == id && o.Manifest.ID != id {
				mg.Host.logf(id, "warning: %s requires this plugin", o.Manifest.ID)
			}
		}
	}
	closeImpl(p)
	if err := os.RemoveAll(p.Dir); err != nil {
		return err
	}
	mg.stMu.Lock()
	delete(mg.State.Settings, id)
	delete(mg.State.Disabled, id)
	mg.stMu.Unlock()
	if mg.Save != nil {
		mg.Save()
	}
	mg.Load()
	return nil
}
