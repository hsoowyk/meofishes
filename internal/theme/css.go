package theme

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Compiled is the result of preprocessing a theme's CSS.
type Compiled struct {
	QSS   string                       // plain Qt stylesheet
	Vars  map[string]string            // --variables (resolved)
	Props map[string]map[string]string // selector -> -meo-* property (without prefix) -> value
}

type rule struct {
	selector string
	decls    [][2]string
	at       string // raw at-rule text kept as is
}

var (
	commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	importRe  = regexp.MustCompile(`@import\s+(?:url\()?["']?([^"');]+)["']?\)?\s*;`)
	varRe     = regexp.MustCompile(`var\(\s*(--[\w-]+)\s*(?:,\s*([^()]*(?:\([^()]*\))?[^()]*))?\)`)
)

// Preprocess expands @import relative to dir, strips comments.
func expandImports(css, dir string, depth int) (string, error) {
	if depth > 8 {
		return "", fmt.Errorf("@import nested too deep")
	}
	css = commentRe.ReplaceAllString(css, "")
	var firstErr error
	out := importRe.ReplaceAllStringFunc(css, func(m string) string {
		file := importRe.FindStringSubmatch(m)[1]
		p := filepath.Join(dir, file)
		data, err := os.ReadFile(p)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("@import %s: %w", file, err)
			}
			return ""
		}
		sub, err := expandImports(string(data), filepath.Dir(p), depth+1)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return sub
	})
	return out, firstErr
}

// parse splits CSS into rules (no nesting except at-rules kept verbatim).
func parse(css string) []rule {
	var rules []rule
	i := 0
	for i < len(css) {
		open := strings.IndexByte(css[i:], '{')
		if open < 0 {
			break
		}
		sel := strings.TrimSpace(css[i : i+open])
		// find the matching brace
		depth, j := 0, i+open
		for ; j < len(css); j++ {
			if css[j] == '{' {
				depth++
			} else if css[j] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		body := css[i+open+1 : min(j, len(css))]
		i = j + 1
		if strings.HasPrefix(sel, "@") {
			rules = append(rules, rule{at: sel + " {" + body + "}"})
			continue
		}
		r := rule{selector: sel}
		for _, d := range splitDecls(body) {
			k, v, ok := strings.Cut(d, ":")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if k != "" {
				r.decls = append(r.decls, [2]string{k, v})
			}
		}
		rules = append(rules, r)
	}
	return rules
}

// splitDecls splits on ';' outside parentheses and quotes.
func splitDecls(body string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ';' && depth == 0:
			out = append(out, body[start:i])
			start = i + 1
		}
	}
	if s := strings.TrimSpace(body[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// Compile turns meofishes CSS into a Qt stylesheet plus custom properties.
// sources are concatenated in order (parent themes first).
func Compile(sources []Source) (*Compiled, error) {
	var all strings.Builder
	var firstErr error
	for _, s := range sources {
		css, err := expandImports(s.CSS, s.Dir, 0)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		all.WriteString(css)
		all.WriteByte('\n')
	}
	rules := parse(all.String())

	c := &Compiled{Vars: map[string]string{}, Props: map[string]map[string]string{}}
	// 1) variables from :root (later wins)
	for _, r := range rules {
		if r.selector == ":root" || r.selector == "*:root" {
			for _, d := range r.decls {
				if strings.HasPrefix(d[0], "--") {
					c.Vars[d[0]] = d[1]
				}
			}
		}
	}
	for k, v := range c.Vars {
		c.Vars[k] = c.resolve(v, 0)
	}
	// 2) rules
	var q strings.Builder
	for _, r := range rules {
		if r.at != "" {
			continue // Qt does not understand at-rules
		}
		if r.selector == ":root" || r.selector == "*:root" {
			continue
		}
		var qd []string
		for _, d := range r.decls {
			val := EvalFuncs(c.resolve(d[1], 0))
			if strings.HasPrefix(d[0], "-meo-") {
				name := strings.TrimPrefix(d[0], "-meo-")
				for _, sel := range strings.Split(r.selector, ",") {
					sel = NormSelector(sel)
					if c.Props[sel] == nil {
						c.Props[sel] = map[string]string{}
					}
					c.Props[sel][name] = val
				}
				continue
			}
			if strings.HasPrefix(d[0], "--") {
				continue
			}
			qd = append(qd, "  "+d[0]+": "+val+";")
		}
		if len(qd) > 0 {
			q.WriteString(r.selector + " {\n" + strings.Join(qd, "\n") + "\n}\n")
		}
	}
	c.QSS = q.String()
	return c, firstErr
}

// Source is one CSS file and the folder its imports are relative to.
type Source struct {
	CSS string
	Dir string
}

func (c *Compiled) resolve(v string, depth int) string {
	if depth > 10 || !strings.Contains(v, "var(") {
		return v
	}
	out := varRe.ReplaceAllStringFunc(v, func(m string) string {
		sub := varRe.FindStringSubmatch(m)
		if val, ok := c.Vars[sub[1]]; ok {
			return val
		}
		return strings.TrimSpace(sub[2])
	})
	return c.resolve(out, depth+1)
}

var spaceRe = regexp.MustCompile(`\s+`)

// NormSelector canonicalizes a selector for -meo-* lookups.
func NormSelector(s string) string {
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	s = strings.ReplaceAll(s, "'", `"`)
	return s
}

// Prop looks a custom property up in several selectors, first match wins.
func (c *Compiled) Prop(selectors []string, name string) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, s := range selectors {
		if m := c.Props[s]; m != nil {
			if v, ok := m[name]; ok {
				return v, true
			}
		}
	}
	return "", false
}

// Var returns a resolved --variable.
func (c *Compiled) Var(name string) string {
	if c == nil {
		return ""
	}
	return c.Vars[name]
}

// ---------------------------------------------------------------- color functions

var funcRe = regexp.MustCompile(`\b(alpha|mix|lighten|darken|saturate|desaturate)\(([^()]*(?:\([^()]*\))?[^()]*(?:\([^()]*\))?[^()]*)\)`)

// EvalFuncs evaluates alpha(), mix(), lighten(), darken(), saturate(),
// desaturate() (innermost first).
func EvalFuncs(v string) string {
	for i := 0; i < 10 && funcRe.MatchString(v); i++ {
		v = funcRe.ReplaceAllStringFunc(v, func(m string) string {
			sub := funcRe.FindStringSubmatch(m)
			args := splitArgs(sub[2])
			switch sub[1] {
			case "alpha":
				if len(args) == 2 {
					if col, ok := ParseColor(args[0]); ok {
						col.A = clamp01(parseNum(args[1]))
						return col.CSS()
					}
				}
			case "mix":
				if len(args) >= 2 {
					a, ok1 := ParseColor(args[0])
					b, ok2 := ParseColor(args[1])
					t := 0.5
					if len(args) == 3 {
						t = clamp01(parseNum(args[2]))
					}
					if ok1 && ok2 {
						return Mix(a, b, t).CSS()
					}
				}
			case "lighten", "darken", "saturate", "desaturate":
				if len(args) == 2 {
					if col, ok := ParseColor(args[0]); ok {
						amt := clamp01(parseNum(args[1]))
						h, s, l := col.HSL()
						switch sub[1] {
						case "lighten":
							l = clamp01(l + amt)
						case "darken":
							l = clamp01(l - amt)
						case "saturate":
							s = clamp01(s + amt)
						case "desaturate":
							s = clamp01(s - amt)
						}
						n := FromHSL(h, s, l)
						n.A = col.A
						return n.CSS()
					}
				}
			}
			return "transparent"
		})
	}
	return v
}

func splitArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func parseNum(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return f / 100
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// Color is an RGBA color with components 0..255 and alpha 0..1.
type Color struct {
	R, G, B float64
	A       float64
}

var rgbRe = regexp.MustCompile(`^rgba?\(\s*([\d.]+%?)\s*,?\s*([\d.]+%?)\s*,?\s*([\d.]+%?)\s*(?:[,/]\s*([\d.]+%?))?\s*\)$`)

var named = map[string]string{"white": "#ffffff", "black": "#000000", "transparent": "#00000000", "red": "#ff0000",
	"pink": "#ffc0cb", "gray": "#808080", "grey": "#808080"}

// ParseColor understands #rgb, #rgba, #rrggbb, #rrggbbaa, rgb(), rgba() and a few names.
func ParseColor(s string) (Color, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if n, ok := named[s]; ok {
		s = n
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		if len(h) == 3 || len(h) == 4 {
			var b strings.Builder
			for _, c := range h {
				b.WriteRune(c)
				b.WriteRune(c)
			}
			h = b.String()
		}
		if len(h) != 6 && len(h) != 8 {
			return Color{}, false
		}
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return Color{}, false
		}
		if len(h) == 6 {
			return Color{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255), 1}, true
		}
		return Color{float64(v >> 24 & 255), float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v&255) / 255}, true
	}
	if m := rgbRe.FindStringSubmatch(s); m != nil {
		ch := func(x string) float64 {
			if strings.HasSuffix(x, "%") {
				return parseNum(x) * 255
			}
			f, _ := strconv.ParseFloat(x, 64)
			return f
		}
		a := 1.0
		if m[4] != "" {
			a = parseNum(m[4])
			if a > 1 { // rgba(…, 128) Qt style
				a /= 255
			}
		}
		return Color{ch(m[1]), ch(m[2]), ch(m[3]), clamp01(a)}, true
	}
	return Color{}, false
}

// CSS renders the color in a form Qt accepts.
func (c Color) CSS() string {
	r, g, b := int(math.Round(c.R)), int(math.Round(c.G)), int(math.Round(c.B))
	if c.A >= 0.999 {
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %d)", r, g, b, int(math.Round(c.A*255)))
}

// Mix blends a toward b by t.
func Mix(a, b Color, t float64) Color {
	return Color{a.R + (b.R-a.R)*t, a.G + (b.G-a.G)*t, a.B + (b.B-a.B)*t, a.A + (b.A-a.A)*t}
}

// HSL converts to hue (0..1), saturation, lightness.
func (c Color) HSL() (h, s, l float64) {
	r, g, b := c.R/255, c.G/255, c.B/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

// FromHSL builds an opaque color.
func FromHSL(h, s, l float64) Color {
	if s == 0 {
		return Color{l * 255, l * 255, l * 255, 1}
	}
	hue := func(p, q, t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return Color{hue(p, q, h+1.0/3) * 255, hue(p, q, h) * 255, hue(p, q, h-1.0/3) * 255, 1}
}

// ---------------------------------------------------------------- value helpers

// Duration parses "300ms" / "0.3s" into milliseconds.
func Duration(s string, def int) int {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return def
	case strings.HasSuffix(s, "ms"):
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64)
		if err == nil {
			return int(f)
		}
	case strings.HasSuffix(s, "s"):
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64)
		if err == nil {
			return int(f * 1000)
		}
	default:
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return int(f)
		}
	}
	return def
}

// Length parses "12px", "12", "1.5em" (relative to base) into pixels.
func Length(s string, base, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	mul := 1.0
	switch {
	case strings.HasSuffix(s, "px"):
		s = strings.TrimSuffix(s, "px")
	case strings.HasSuffix(s, "pt"):
		s, mul = strings.TrimSuffix(s, "pt"), 4.0/3
	case strings.HasSuffix(s, "em"):
		s, mul = strings.TrimSuffix(s, "em"), base
	case strings.HasSuffix(s, "%"):
		s, mul = strings.TrimSuffix(s, "%"), base/100
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f * mul
}

// Number parses a float with default.
func Number(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if strings.HasSuffix(s, "%") {
		return parseNum(s)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}

// Bool parses true/false/yes/no/on/off/1/0.
func Bool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0", "none":
		return false
	}
	return def
}
