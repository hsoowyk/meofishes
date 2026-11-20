// Package plugin implements meofishes' plugin system.
//
// A plugin is a folder with a plugin.json manifest. It declares one or more
// types (what it provides) and one runtime (how it runs):
//
//	types:   source | lyrics | layout | hook
//	runtime: declarative | script | process | builtin
//
// See docs/PLUGINS.md for the full guide.
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

// Plugin types.
const (
	TypeSource = "source" // provides music: search, browse, stream
	TypeLyrics = "lyrics" // provides lyrics
	TypeLayout = "layout" // rearranges the UI regions
	TypeHook   = "hook"   // reacts to player events (scrobblers, rich presence…)
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

	Source *DeclSource `json:"source,omitempty"` // declarative source definition
	Lyrics *DeclLyrics `json:"lyrics,omitempty"` // declarative lyrics definition
	Layout *LayoutNode `json:"layout,omitempty"` // layout plugins
	Events []string    `json:"events,omitempty"` // hook plugins: events to receive ("*" = all)
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
		errs = append(errs, "types must list at least one of source, lyrics, layout, hook")
	}
	for _, t := range m.Types {
		switch t {
		case TypeSource, TypeLyrics, TypeLayout, TypeHook:
		default:
			errs = append(errs, fmt.Sprintf("unknown type %q", t))
		}
	}
	switch m.Runtime {
	case RuntimeDeclarative:
		if m.Has(TypeSource) && m.Source == nil {
			errs = append(errs, `declarative source plugins need a "source" block`)
		}
		if m.Has(TypeLyrics) && m.Lyrics == nil {
			errs = append(errs, `declarative lyrics plugins need a "lyrics" block`)
		}
		if m.Has(TypeHook) {
			errs = append(errs, "hook plugins need the script or process runtime")
		}
	case RuntimeScript, RuntimeProcess:
		if m.Entry == "" {
			errs = append(errs, `"entry" is required for the `+m.Runtime+" runtime")
		}
	case RuntimeBuiltin:
	case "":
		if !(len(m.Types) == 1 && m.Types[0] == TypeLayout) {
			errs = append(errs, "runtime is required")
		}
	default:
		errs = append(errs, fmt.Sprintf("unknown runtime %q", m.Runtime))
	}
	if m.Has(TypeLayout) && m.Layout == nil {
		errs = append(errs, `layout plugins need a "layout" block`)
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
	// Resolve (optional) turns a track into a stream URL: called with {{track.*}},
	// the URL is read from "path" in the response.
	Resolve *struct {
		Request
		Path string `json:"path"`
	} `json:"resolve,omitempty"`
	// Resolver says how the final URL is played:
	//   "direct" (default) – the URL is an audio stream
	//   "ytdlp"           – the URL is a web page; the ytdlp module extracts the audio
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

// LayoutNode is one node of a layout tree.
//
//	{"box": "v", "children": [...]}          vertical box
//	{"split": "h", "sizes": [240, 800]}      resizable splitter
//	{"region": "lyrics", "stretch": 2}       a UI region
//
// Regions: sidebar, pages, nowplaying, lyrics, queue, playerbar, cover, spacer.
type LayoutNode struct {
	Box      string        `json:"box,omitempty"`   // h | v
	Split    string        `json:"split,omitempty"` // h | v
	Region   string        `json:"region,omitempty"`
	Name     string        `json:"name,omitempty"` // objectName, so CSS can target it (#name)
	Stretch  int           `json:"stretch,omitempty"`
	Sizes    []int         `json:"sizes,omitempty"`
	Spacing  *int          `json:"spacing,omitempty"`
	Margins  []int         `json:"margins,omitempty"` // [l, t, r, b]
	MinW     int           `json:"minWidth,omitempty"`
	MaxW     int           `json:"maxWidth,omitempty"`
	MinH     int           `json:"minHeight,omitempty"`
	MaxH     int           `json:"maxHeight,omitempty"`
	Children []*LayoutNode `json:"children,omitempty"`
}

// Regions lists every region name the UI knows.
var Regions = []string{"sidebar", "pages", "nowplaying", "lyrics", "queue", "playerbar", "cover", "spacer"}

// DefaultLayout is used when no layout plugin is active.
func DefaultLayout() *LayoutNode {
	zero := 0
	return &LayoutNode{Box: "v", Spacing: &zero, Children: []*LayoutNode{
		{Split: "h", Name: "MainSplit", Stretch: 1, Sizes: []int{230, 760, 380}, Children: []*LayoutNode{
			{Region: "sidebar", MinW: 190, MaxW: 320},
			{Region: "pages", Stretch: 1},
			{Region: "nowplaying", MinW: 300},
		}},
		{Region: "playerbar"},
	}}
}
