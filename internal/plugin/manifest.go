// Package plugin implements meofishes' plugin system.
//
// A plugin is a folder with a plugin.json manifest. It declares one or more
// types (what it is for) and one runtime (how it runs):
//
//	types:   fnt | src | ani | dep | xtm | evt
//	runtime: declarative | script | process | builtin
//
// See docs/plugins/ for the full guide.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Plugin types (the short codes shown in the app).
const (
	TypeFnt = "fnt" // sources of music/video/lyrics/artwork: resolve streams, lyrics, covers
	TypeSrc = "src" // search, home page, recommendations, playlists
	TypeAni = "ani" // animation changes: CSS animation layer + easing curves
	TypeDep = "dep" // tools for other plugins: JS libraries, executables
	TypeXtm = "xtm" // extreme changes: replace the player (mpv) or the icon pack
	TypeEvt = "evt" // reacts to player events (scrobblers, rich presence…)
)

// Permissions maps each type to the capabilities it allows. Types are
// informative (shown in the app) and permissive: a plugin can only use the
// capabilities of the types it declares.
var Permissions = map[string][]string{
	TypeFnt: {CapStream, CapMatch, CapLyrics, CapArtwork},
	TypeSrc: {CapSearch, CapHome, CapRecommend, CapPlaylists, CapOpen},
	TypeAni: {CapAnimation},
	TypeDep: {CapLibrary},
	TypeXtm: {CapPlayer, CapIcons},
	TypeEvt: {CapEvents},
}

// Allows reports whether the declared types permit a capability.
func (m *Manifest) Allows(c string) bool {
	for _, t := range m.Types {
		for _, x := range Permissions[t] {
			if x == c {
				return true
			}
		}
	}
	return false
}

// Types lists every type code in display order.
var Types = []string{TypeFnt, TypeSrc, TypeAni, TypeDep, TypeXtm, TypeEvt}

// legacyTypes maps the names used before 0.2 to the new codes.
var legacyTypes = map[string][]string{
	"source": {TypeFnt, TypeSrc},
	"lyrics": {TypeFnt},
	"hook":   {TypeEvt},
}

// Capabilities a plugin may provide. They are discovered from the runtime
// (exported JS functions, declarative blocks, process "capabilities").
const (
	CapSearch    = "search"    // src
	CapHome      = "home"      // src
	CapRecommend = "recommend" // src
	CapPlaylists = "playlists" // src
	CapOpen      = "open"      // src: list the tracks of a collection
	CapStream    = "stream"    // fnt: resolve its own tracks to a URL
	CapMatch     = "match"     // fnt: resolve ANY track by title/artist
	CapLyrics    = "lyrics"    // fnt
	CapArtwork   = "artwork"   // fnt: find a cover for a track
	CapEvents    = "events"    // evt
	CapPlayer    = "player"    // xtm: replaces mpv
	CapIcons     = "icons"     // xtm: icon pack
	CapAnimation = "animation" // ani
	CapLibrary   = "library"   // dep: require("dep:<id>")
)

// Runtimes.
const (
	RuntimeDeclarative = "declarative" // JSON only: endpoints + field mapping
	RuntimeScript      = "script"      // JavaScript (embedded engine) using host Go modules
	RuntimeProcess     = "process"     // any executable speaking JSON over stdio (Go SDK in /sdk)
	RuntimeBuiltin     = "builtin"     // compiled into meofishes
)

// Setting is a user-editable option shown in the plugin's settings dialog.
type Setting struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type,omitempty"` // string | password | bool | number | choice | list
	Default     string   `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Manifest is the content of plugin.json.
type Manifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Types       []string `json:"types"`
	Runtime     string   `json:"runtime"`

	// script: JS file. process: command line (relative to the plugin dir).
	Entry string `json:"entry,omitempty"`
	// process: optional build command run once at install time, e.g. "go build -o plugin ."
	Build string `json:"build,omitempty"`

	// Host Go libraries the plugin wants (http, ytdlp, lavalink, lyrics, crypto, storage, html).
	Modules []string `json:"modules,omitempty"`
	// Hosts the http module may reach. Empty means any host.
	Hosts []string `json:"hosts,omitempty"`

	Settings []Setting `json:"settings,omitempty"`

	// dep: other plugins this one needs (ids). Missing ones are reported.
	Requires []string `json:"requires,omitempty"`
	// dep: a folder (relative) with executables added to PATH of process
	// plugins that require this one.
	Bin string `json:"bin,omitempty"`

	Source  *DeclSource  `json:"source,omitempty"`  // declarative src/fnt definition
	Lyrics  *DeclLyrics  `json:"lyrics,omitempty"`  // declarative lyrics definition
	Artwork *DeclArtwork `json:"artwork,omitempty"` // declarative artwork definition
	Events  []string     `json:"events,omitempty"`  // evt: events to receive ("*" = all)

	// fnt: this plugin can play any track by searching its title/artist.
	Match bool `json:"match,omitempty"`

	// ani: a CSS file applied over the theme (only animation -meo-* props
	// are meant to go there) and named cubic-bezier easing curves.
	Animations string                `json:"animations,omitempty"`
	Easings    map[string][4]float64 `json:"easings,omitempty"`

	// xtm: an icon pack folder (one SVG/PNG per icon name).
	Icons string `json:"icons,omitempty"`
	// xtm: icons are single-color and should be tinted by the theme.
	IconsTint *bool `json:"iconsTint,omitempty"`
	// xtm: this process plugin implements the player protocol (replaces mpv).
	Player bool `json:"player,omitempty"`
}

// Has reports whether the plugin provides type t.
func (m *Manifest) Has(t string) bool {
	for _, x := range m.Types {
		if x == t {
			return true
		}
	}
	return false
}

// Wants reports whether the plugin requested module name.
func (m *Manifest) Wants(name string) bool {
	for _, x := range m.Modules {
		if x == name {
			return true
		}
	}
	return false
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// Validate checks the manifest for mistakes and returns a friendly error.
func (m *Manifest) Validate() error {
	var errs []string
	if !idRe.MatchString(m.ID) {
		errs = append(errs, "id must be lowercase letters, digits, '.', '_' or '-' (2-64 chars)")
	}
	if m.Name == "" {
		errs = append(errs, "name is required")
	}
	if len(m.Types) == 0 {
		errs = append(errs, "types must list at least one of "+strings.Join(Types, ", "))
	}
	for _, t := range m.Types {
		switch t {
		case TypeFnt, TypeSrc, TypeAni, TypeDep, TypeXtm, TypeEvt:
		case "layout":
			errs = append(errs, "layout plugins were removed in 0.2: layouts are part of the app now")
		default:
			errs = append(errs, fmt.Sprintf("unknown type %q (use %s)", t, strings.Join(Types, ", ")))
		}
	}
	switch m.Runtime {
	case RuntimeDeclarative:
		if m.Source == nil && m.Lyrics == nil && m.Artwork == nil {
			errs = append(errs, `declarative plugins need a "source", "lyrics" or "artwork" block`)
		}
	case RuntimeScript, RuntimeProcess:
		if m.Entry == "" {
			errs = append(errs, `"entry" is required for the `+m.Runtime+" runtime")
		}
	case RuntimeBuiltin:
	case "":
		// asset-only plugins: ani (CSS), xtm icon packs
		if m.Animations == "" && m.Icons == "" {
			errs = append(errs, "runtime is required")
		}
	default:
		errs = append(errs, fmt.Sprintf("unknown runtime %q", m.Runtime))
	}
	if m.Player && m.Runtime != RuntimeProcess {
		errs = append(errs, `"player": true needs the process runtime`)
	}
	for _, f := range []string{m.Animations, m.Icons, m.Bin} {
		if f != "" && (filepath.IsAbs(f) || strings.Contains(filepath.ToSlash(f), "../")) {
			errs = append(errs, fmt.Sprintf("%q must be a path inside the plugin folder", f))
		}
	}
	for _, mod := range m.Modules {
		if _, ok := moduleDocs[mod]; !ok {
			errs = append(errs, fmt.Sprintf("unknown module %q (available: %s)", mod, strings.Join(ModuleNames(), ", ")))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// normalizeTypes converts legacy type names and removes duplicates.
func (m *Manifest) normalizeTypes() {
	seen := map[string]bool{}
	var out []string
	for _, t := range m.Types {
		ts := []string{t}
		if l, ok := legacyTypes[t]; ok {
			ts = l
		}
		for _, x := range ts {
			if !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	m.Types = out
}

// LoadManifest reads dir/plugin.json.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin.json: %w", err)
	}
	m.normalizeTypes()
	if err := m.Validate(); err != nil {
		return &m, fmt.Errorf("plugin.json: %w", err)
	}
	return &m, nil
}

// ---------------------------------------------------------------- declarative blocks

// Request describes one HTTP call. Strings may contain {{templates}}.
// In URLs, template values are URL-escaped automatically.
type Request struct {
	Method  string            `json:"method,omitempty"` // default GET
	URL     string            `json:"url"`              // absolute or relative to baseUrl
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// Mapping turns one JSON item into a Track. Values are paths into the item
// ("snippet.title", "images[0].url") or templates ("https://x/{{item.id}}.jpg").
type Mapping map[string]string

// Endpoint is a request plus where to find the list of items and how to map them.
type Endpoint struct {
	Request
	Items string  `json:"items,omitempty"` // path to the array ("$" = the root)
	Map   Mapping `json:"map,omitempty"`
}

// DeclSource is the "source" block of a declarative plugin.
type DeclSource struct {
	BaseURL string            `json:"baseUrl,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// Search is called with {{query}}.
	Search *Endpoint `json:"search"`
	// Home (optional) fills the start page.
	Home *Endpoint `json:"home,omitempty"`
	// Recommend (optional) lists tracks similar to {{track.*}} (radio/autoplay).
	Recommend *Endpoint `json:"recommend,omitempty"`
	// Resolve (optional) turns a track into a stream URL: called with {{track.*}},
	// the URL is read from "path" in the response.
	Resolve *struct {
		Request
		Path string `json:"path"`
	} `json:"resolve,omitempty"`
	// Resolver says how the final URL is played:
	//   "direct" (default) – the URL is an audio stream
	//   "ytdlp"           – the URL is a web page; the ytdlp module extracts the audio
	//   "match"           – the plugin only has metadata: meofishes plays the
	//                       track through an fnt plugin with "match": true
	Resolver string `json:"resolver,omitempty"`
}

// DeclLyrics is the "lyrics" block of a declarative plugin. Called with
// {{title}}, {{artist}}, {{album}}, {{durationSec}}, {{isrc}}, {{videoId}}.
type DeclLyrics struct {
	BaseURL    string            `json:"baseUrl,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Request    Request           `json:"request"`
	Path       string            `json:"path"`                 // where the lyric text is
	Format     string            `json:"format,omitempty"`     // ttml | lrc | krc | plain | auto
	FormatPath string            `json:"formatPath,omitempty"` // or read the format from the response
	Credit     string            `json:"credit,omitempty"`
}

// DeclArtwork is the "artwork" block of a declarative fnt plugin. Called
// with the same variables as lyrics; "path" points at the image URL.
type DeclArtwork struct {
	BaseURL string            `json:"baseUrl,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Request Request           `json:"request"`
	Path    string            `json:"path"`
	// Optional: replace part of the URL to ask for a bigger image, e.g.
	// ["100x100bb", "600x600bb"] for iTunes.
	Replace []string `json:"replace,omitempty"`
}
