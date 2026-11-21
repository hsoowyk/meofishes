package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"

	"meofishes/internal/core"
	"meofishes/internal/lyrics"
)

// script runs JavaScript plugins on the embedded goja engine (ES2015+, sync).
//
//	const http = require("http")          // host Go module (must be in "modules")
//	const util = require("./util.js")     // file inside the plugin folder
//	exports.search  = (query) => ({ tracks: [...] })
//	exports.resolve = (track) => "https://…"  or  { url, headers, page }
//	exports.home    = () => ({ tracks: [...] })
//	exports.lyrics  = (q) => ({ format: "ttml", content: "…" })
//	exports.artwork = (q) => "https://…/cover.jpg"
//	exports.recommend = (track) => ({ tracks: [...] })
//	exports.playlists = () => [{ id, title, cover }]
//	exports.open    = (collection) => ({ ...collection, tracks: [...] })
//	exports.onEvent = (name, data) => {}
//
// dep plugins: require("dep:<id>") loads the entry file of a plugin listed in
// "requires" (its exports become the module).
type script struct {
	m        *Manifest
	dir      string
	host     *Host
	env      *moduleEnv
	settings func() map[string]string
	deps     func(id string) (string, *Manifest, error)

	mu      sync.Mutex
	rt      *goja.Runtime
	exports *goja.Object
}

func newScript(m *Manifest, dir string, host *Host, settings func() map[string]string,
	deps func(id string) (string, *Manifest, error)) (*script, error) {
	s := &script{m: m, dir: dir, host: host, settings: settings, deps: deps, env: &moduleEnv{m: m, dir: dir, host: host}}
	if err := s.boot(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *script) boot() error {
	rt := goja.New()
	rt.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	cache := map[string]*goja.Object{}

	logf := func(level string) func(args ...any) {
		return func(args ...any) {
			parts := make([]string, len(args))
			for i, a := range args {
				if b, err := json.Marshal(a); err == nil && !isString(a) {
					parts[i] = string(b)
				} else {
					parts[i] = fmt.Sprint(a)
				}
			}
			s.host.logf(s.m.ID, "%s %s", level, strings.Join(parts, " "))
		}
	}
	console := rt.NewObject()
	_ = console.Set("log", logf("log"))
	_ = console.Set("warn", logf("warn"))
	_ = console.Set("error", logf("error"))
	_ = rt.Set("console", console)
	app := rt.NewObject()
	_ = app.Set("version", s.host.Version)
	_ = app.Set("userAgent", s.host.UserAgent)
	_ = app.Set("plugin", s.m.ID)
	_ = app.DefineAccessorProperty("quality", rt.ToValue(func() string { return s.host.Quality }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = rt.Set("app", app)
	_ = rt.Set("settings", rt.NewDynamicObject(settingsObj{s.settings, rt}))

	var require func(name string) (goja.Value, error)
	var load func(base, file string) (*goja.Object, error)
	load = func(base, file string) (*goja.Object, error) {
		abs, err := filepath.Abs(filepath.Join(base, file))
		if err != nil {
			return nil, err
		}
		root, _ := filepath.Abs(base)
		if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			return nil, fmt.Errorf("require(%q): outside the plugin folder", file)
		}
		if mod, ok := cache[abs]; ok {
			return mod.Get("exports").ToObject(rt), nil
		}
		src, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		module := rt.NewObject()
		exports := rt.NewObject()
		_ = module.Set("exports", exports)
		cache[abs] = module
		wrapped := "(function(module, exports, require){" + string(src) + "\n})"
		fnVal, err := rt.RunScript(abs, wrapped)
		if err != nil {
			return nil, err
		}
		fn, _ := goja.AssertFunction(fnVal)
		// files of a dep plugin require relative to their own folder
		var req goja.Value = rt.Get("require")
		if base != s.dir {
			req = rt.ToValue(func(name string) (goja.Value, error) {
				if strings.HasPrefix(name, "./") {
					if !strings.HasSuffix(name, ".js") {
						name += ".js"
					}
					return load(base, name)
				}
				return require(name)
			})
		}
		if _, err := fn(goja.Undefined(), module, exports, req); err != nil {
			return nil, err
		}
		return module.Get("exports").ToObject(rt), nil
	}
	require = func(name string) (goja.Value, error) {
		if strings.HasPrefix(name, "./") || strings.HasPrefix(name, "../") {
			if !strings.HasSuffix(name, ".js") {
				name += ".js"
			}
			return load(s.dir, name)
		}
		if id, ok := strings.CutPrefix(name, "dep:"); ok {
			if s.deps == nil {
				return nil, fmt.Errorf("require(%q): dependencies are not available", name)
			}
			dir, dm, err := s.deps(id)
			if err != nil {
				return nil, err
			}
			if dm.Entry == "" {
				return nil, fmt.Errorf("dep %s has no entry file", id)
			}
			return load(dir, dm.Entry)
		}
		mod, err := s.env.newModule(name)
		if err != nil {
			return nil, err
		}
		return rt.ToValue(mod), nil
	}
	_ = rt.Set("require", require)
	exports, err := load(s.dir, s.m.Entry)
	if err != nil {
		return fmt.Errorf("%s: %w", s.m.Entry, err)
	}
	s.rt, s.exports = rt, exports
	return nil
}

// Caps lists the functions the script exports.
func (s *script) Caps() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	has := func(n string) bool { _, ok := goja.AssertFunction(s.exports.Get(n)); return ok }
	c := map[string]bool{
		CapSearch: has("search"), CapHome: has("home"), CapRecommend: has("recommend"),
		CapPlaylists: has("playlists"), CapOpen: has("open"), CapLyrics: has("lyrics"),
		CapArtwork: has("artwork"), CapEvents: has("onEvent"),
		CapLibrary: true,
	}
	// a script that searches can play its own tracks (resolve() is optional)
	c[CapStream] = has("resolve") || c[CapSearch]
	c[CapMatch] = has("resolve")
	return c
}

func isString(v any) bool { _, ok := v.(string); return ok }

// settingsObj exposes live settings to JS as a read-only object.
type settingsObj struct {
	get func() map[string]string
	rt  *goja.Runtime
}

func (o settingsObj) Get(key string) goja.Value {
	if v, ok := o.get()[key]; ok {
		return o.rt.ToValue(v)
	}
	return goja.Undefined()
}
func (o settingsObj) Set(string, goja.Value) bool { return false }
func (o settingsObj) Has(key string) bool         { _, ok := o.get()[key]; return ok }
func (o settingsObj) Delete(string) bool          { return false }
func (o settingsObj) Keys() []string {
	var ks []string
	for k := range o.get() {
		ks = append(ks, k)
	}
	return ks
}

// call invokes exports[name](args...) and JSON-decodes the result into out.
func (s *script) call(ctx context.Context, name string, out any, args ...any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn, ok := goja.AssertFunction(s.exports.Get(name))
	if !ok {
		return false, nil
	}
	stop := context.AfterFunc(ctx, func() { s.rt.Interrupt(ctx.Err()) })
	defer stop()
	vals := make([]goja.Value, len(args))
	for i, a := range args {
		vals[i] = s.rt.ToValue(a)
	}
	res, err := fn(goja.Undefined(), vals...)
	s.rt.ClearInterrupt()
	if err != nil {
		if ex, ok := err.(*goja.Exception); ok {
			return true, fmt.Errorf("%s: %s", name, ex.Value().String())
		}
		return true, err
	}
	if out == nil || res == nil || goja.IsUndefined(res) || goja.IsNull(res) {
		return true, nil
	}
	data, err := json.Marshal(res.Export())
	if err != nil {
		return true, err
	}
	return true, json.Unmarshal(data, out)
}

func trackArg(t core.Track) map[string]any {
	var m map[string]any
	b, _ := json.Marshal(t)
	_ = json.Unmarshal(b, &m)
	return m
}

func (s *script) fixTracks(r *core.SearchResult) *core.SearchResult {
	if r == nil {
		return &core.SearchResult{}
	}
	for i := range r.Tracks {
		r.Tracks[i].Source = s.m.ID
		if r.Tracks[i].ID == "" {
			r.Tracks[i].ID = r.Tracks[i].StreamURL
		}
	}
	for i := range r.Collections {
		r.Collections[i].Source = s.m.ID
	}
	return r
}

func (s *script) Search(ctx context.Context, q string) (*core.SearchResult, error) {
	var r core.SearchResult
	ok, err := s.call(ctx, "search", &r, q)
	if !ok {
		return nil, fmt.Errorf("plugin does not export search()")
	}
	return s.fixTracks(&r), err
}

func (s *script) Home(ctx context.Context) (*core.SearchResult, error) {
	var r core.SearchResult
	if _, err := s.call(ctx, "home", &r); err != nil {
		return nil, err
	}
	return s.fixTracks(&r), nil
}

func (s *script) Recommend(ctx context.Context, seed core.Track) (*core.SearchResult, error) {
	var r core.SearchResult
	if _, err := s.call(ctx, "recommend", &r, trackArg(seed)); err != nil {
		return nil, err
	}
	return s.fixTracks(&r), nil
}

func (s *script) Playlists(ctx context.Context) ([]core.Collection, error) {
	var out []core.Collection
	_, err := s.call(ctx, "playlists", &out)
	for i := range out {
		out[i].Source = s.m.ID
	}
	return out, err
}

func (s *script) Artwork(ctx context.Context, q lyrics.Query) (string, error) {
	var u string
	arg := map[string]any{"title": q.Title, "artist": q.Artist, "album": q.Album, "durationMs": q.DurationMs, "isrc": q.ISRC, "videoId": q.VideoID}
	ok, err := s.call(ctx, "artwork", &u, arg)
	if !ok || (err == nil && u == "") {
		return "", lyrics.ErrNotFound
	}
	return u, err
}

func (s *script) Open(ctx context.Context, c core.Collection) (*core.Collection, error) {
	var out core.Collection
	ok, err := s.call(ctx, "open", &out, c)
	if !ok {
		return &c, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range out.Tracks {
		out.Tracks[i].Source = s.m.ID
	}
	return &out, nil
}

func (s *script) Resolve(ctx context.Context, t core.Track) (*Stream, error) {
	var raw json.RawMessage
	ok, err := s.call(ctx, "resolve", &raw, trackArg(t))
	if err != nil {
		return nil, err
	}
	if !ok || len(raw) == 0 {
		if t.StreamURL == "" {
			return nil, fmt.Errorf("no stream url")
		}
		return &Stream{URL: t.StreamURL, Page: !looksLikeMedia(t.StreamURL)}, nil
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return &Stream{URL: str, Page: !looksLikeMedia(str)}, nil
	}
	var st Stream
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// looksLikeMedia guesses whether a URL is a direct stream or a web page.
func looksLikeMedia(u string) bool {
	l := strings.ToLower(u)
	if strings.HasPrefix(l, "file://") || strings.HasPrefix(l, "/") {
		return true
	}
	for _, p := range []string{"youtube.com/", "youtu.be/", "soundcloud.com/", "bandcamp.com/", "music.youtube.com/"} {
		if strings.Contains(l, p) {
			return false
		}
	}
	return true
}

func (s *script) Lyrics(ctx context.Context, q lyrics.Query) (*core.Lyrics, error) {
	var raw map[string]any
	arg := map[string]any{"title": q.Title, "artist": q.Artist, "album": q.Album, "durationMs": q.DurationMs, "isrc": q.ISRC, "videoId": q.VideoID}
	ok, err := s.call(ctx, "lyrics", &raw, arg)
	if !ok {
		return nil, fmt.Errorf("plugin does not export lyrics()")
	}
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, lyrics.ErrNotFound
	}
	if content, ok := raw["content"].(string); ok { // {format, content, credit}
		l, err := lyrics.Parse(toString(raw["format"]), content)
		if err != nil {
			return nil, err
		}
		l.Credit = toString(raw["credit"])
		return l, nil
	}
	var l core.Lyrics // already structured {lines: [...]}
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	l.Normalize()
	return &l, nil
}

func (s *script) Event(name string, payload any) {
	go func() {
		if _, err := s.call(context.Background(), "onEvent", nil, name, payload); err != nil {
			s.host.logf(s.m.ID, "onEvent(%s): %v", name, err)
		}
	}()
}
