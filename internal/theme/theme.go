// Package theme loads, installs and compiles meofishes themes.
//
// A theme is a folder with theme.json and a CSS file. Themes may "extend"
// another theme, so a small theme can only change colors.
package theme

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"meofishes/internal/pkgfs"
)

// Manifest is theme.json.
type Manifest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	Author      string            `json:"author,omitempty"`
	Description string            `json:"description,omitempty"`
	Style       string            `json:"style,omitempty"`   // default "style.css"
	Extends     string            `json:"extends,omitempty"` // parent theme id
	Fonts       []string          `json:"fonts,omitempty"`   // .ttf/.otf files to load
	Preview     map[string]string `json:"preview,omitempty"` // background, surface, accent, text: swatches for the picker
	Dark        *bool             `json:"dark,omitempty"`
}

// Theme is an installed theme.
type Theme struct {
	Manifest
	Dir string
}

// Load reads dir/theme.json.
func Load(dir string) (*Theme, error) {
	data, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("theme.json: %w", err)
	}
	if m.ID == "" || m.Name == "" {
		return nil, fmt.Errorf("theme.json needs id and name")
	}
	if m.Style == "" {
		m.Style = "style.css"
	}
	return &Theme{Manifest: m, Dir: dir}, nil
}

// Store is the folder holding installed themes.
type Store struct{ Dir string }

// List returns installed themes sorted by name.
func (s Store) List() []*Theme {
	entries, _ := os.ReadDir(s.Dir)
	var out []*Theme
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if t, err := Load(filepath.Join(s.Dir, e.Name())); err == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Get returns an installed theme by id.
func (s Store) Get(id string) *Theme {
	for _, t := range s.List() {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Chain returns the theme and its parents, root first.
func (s Store) Chain(t *Theme) []*Theme {
	chain := []*Theme{t}
	seen := map[string]bool{t.ID: true}
	for cur := t; cur.Extends != ""; {
		p := s.Get(cur.Extends)
		if p == nil || seen[p.ID] {
			break
		}
		seen[p.ID] = true
		chain = append([]*Theme{p}, chain...)
		cur = p
	}
	return chain
}

// Compile compiles a theme with its parents. extra CSS (user overrides) is
// appended last.
func (s Store) Compile(t *Theme, extra string) (*Compiled, []string, error) {
	var srcs []Source
	var files []string
	for _, th := range s.Chain(t) {
		p := filepath.Join(th.Dir, th.Style)
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		srcs = append(srcs, Source{CSS: string(data), Dir: th.Dir})
		files = append(files, p)
	}
	if strings.TrimSpace(extra) != "" {
		srcs = append(srcs, Source{CSS: extra, Dir: t.Dir})
	}
	c, err := Compile(srcs)
	return c, files, err
}

// Fonts returns absolute font paths of a theme chain.
func (s Store) Fonts(t *Theme) []string {
	var out []string
	for _, th := range s.Chain(t) {
		for _, f := range th.Fonts {
			out = append(out, filepath.Join(th.Dir, f))
		}
	}
	return out
}

// Install installs a theme from a folder, .zip or URL.
func (s Store) Install(src string) (*Theme, error) {
	dir, cleanup, err := pkgfs.Fetch(src)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	root, err := pkgfs.FindRoot(dir, "theme.json")
	if err != nil {
		return nil, err
	}
	t, err := Load(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, t.Style)); err != nil {
		return nil, fmt.Errorf("theme is missing %s", t.Style)
	}
	dst := filepath.Join(s.Dir, t.ID)
	if err := pkgfs.ReplaceDir(root, dst); err != nil {
		return nil, err
	}
	return Load(dst)
}

// Remove deletes an installed theme.
func (s Store) Remove(id string) error {
	t := s.Get(id)
	if t == nil {
		return fmt.Errorf("no theme %q", id)
	}
	return os.RemoveAll(t.Dir)
}

// ---------------------------------------------------------------- easing

// Ease maps t (0..1) through a named easing curve. Names follow CSS and Qt:
// linear, ease, ease-in, ease-out, ease-in-out, in-quad, out-quad, in-out-quad,
// in-cubic, out-cubic, in-out-cubic, out-quart, out-quint, out-expo,
// in-out-expo, out-back, out-elastic, out-bounce, out-sine, in-out-sine.
func Ease(name string, t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	n := strings.ToLower(strings.NewReplacer("_", "-", " ", "").Replace(name))
	if strings.HasPrefix(n, "cubic-bezier(") {
		if b, ok := parseBezier(n); ok {
			return b.at(t)
		}
	}
	customMu.RLock()
	b, ok := custom[n]
	customMu.RUnlock()
	if ok {
		return b.at(t)
	}
	n = strings.TrimPrefix(n, "ease-")
	switch n {
	case "linear":
		return t
	case "in", "in-quad":
		return t * t
	case "out-quad":
		return 1 - (1-t)*(1-t)
	case "in-out", "in-out-quad", "ease":
		if t < .5 {
			return 2 * t * t
		}
		return 1 - math.Pow(-2*t+2, 2)/2
	case "in-cubic":
		return t * t * t
	case "in-out-cubic":
		if t < .5 {
			return 4 * t * t * t
		}
		return 1 - math.Pow(-2*t+2, 3)/2
	case "out-quart":
		return 1 - math.Pow(1-t, 4)
	case "out-quint":
		return 1 - math.Pow(1-t, 5)
	case "out-expo":
		return 1 - math.Pow(2, -10*t)
	case "in-out-expo":
		if t < .5 {
			return math.Pow(2, 20*t-10) / 2
		}
		return (2 - math.Pow(2, -20*t+10)) / 2
	case "out-back":
		c1 := 1.70158
		c3 := c1 + 1
		return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
	case "out-elastic":
		return math.Pow(2, -10*t)*math.Sin((t*10-0.75)*(2*math.Pi)/3) + 1
	case "out-bounce":
		return bounce(t)
	case "out-sine":
		return math.Sin(t * math.Pi / 2)
	case "in-out-sine":
		return -(math.Cos(math.Pi*t) - 1) / 2
	}
	return 1 - math.Pow(1-t, 3) // out-cubic (default, also "out")
}

func bounce(t float64) float64 {
	n1, d1 := 7.5625, 2.75
	switch {
	case t < 1/d1:
		return n1 * t * t
	case t < 2/d1:
		t -= 1.5 / d1
		return n1*t*t + .75
	case t < 2.5/d1:
		t -= 2.25 / d1
		return n1*t*t + .9375
	}
	t -= 2.625 / d1
	return n1*t*t + .984375
}

// ---------------------------------------------------------------- cubic-bezier

type bezier struct{ x1, y1, x2, y2 float64 }

var (
	customMu sync.RWMutex
	custom   = map[string]bezier{}
)

// RegisterEasing adds a named cubic-bezier curve (used by ani plugins).
func RegisterEasing(name string, x1, y1, x2, y2 float64) {
	customMu.Lock()
	custom[strings.ToLower(name)] = bezier{x1, y1, x2, y2}
	customMu.Unlock()
}

// ResetCustomEasings forgets the curves of ani plugins.
func ResetCustomEasings() {
	customMu.Lock()
	custom = map[string]bezier{}
	customMu.Unlock()
}

// CustomEasings lists the registered curve names.
func CustomEasings() []string {
	customMu.RLock()
	defer customMu.RUnlock()
	out := make([]string, 0, len(custom))
	for k := range custom {
		out = append(out, k)
	}
	return out
}

func parseBezier(s string) (bezier, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "cubic-bezier("), ")")
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return bezier{}, false
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return bezier{}, false
		}
		v[i] = f
	}
	return bezier{v[0], v[1], v[2], v[3]}, true
}

// at solves x(u) = t with Newton steps, then returns y(u) (like CSS).
func (b bezier) at(t float64) float64 {
	cx := 3 * b.x1
	bx := 3*(b.x2-b.x1) - cx
	ax := 1 - cx - bx
	cy := 3 * b.y1
	by := 3*(b.y2-b.y1) - cy
	ay := 1 - cy - by
	x := func(u float64) float64 { return ((ax*u+bx)*u + cx) * u }
	dx := func(u float64) float64 { return (3*ax*u+2*bx)*u + cx }
	u := t
	for i := 0; i < 8; i++ {
		d := dx(u)
		if math.Abs(d) < 1e-6 {
			break
		}
		u -= (x(u) - t) / d
	}
	if u < 0 || u > 1 || math.Abs(x(u)-t) > 1e-3 { // fall back to bisection
		lo, hi := 0.0, 1.0
		u = t
		for i := 0; i < 30; i++ {
			if x(u) < t {
				lo = u
			} else {
				hi = u
			}
			u = (lo + hi) / 2
		}
	}
	return ((ay*u+by)*u + cy) * u
}
