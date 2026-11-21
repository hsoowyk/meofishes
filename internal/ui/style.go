package ui

import (
	"math"
	"strconv"
	"strings"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// Styler gives widgets access to the compiled theme's -meo-* properties.
type Styler struct {
	speed     func() float64
	c         *theme.Compiled
	listeners []func()
	keyed     map[any]func()
}

// Set installs a new compiled theme and notifies widgets.
func (s *Styler) Set(c *theme.Compiled) {
	s.c = c
	for _, f := range s.listeners {
		f()
	}
	for _, f := range s.keyed {
		f()
	}
}

// OnChange registers a permanent callback run after every theme change.
func (s *Styler) OnChange(f func()) { s.listeners = append(s.listeners, f) }

// Watch registers a callback that can be removed with Unwatch (for widgets
// that get deleted, like list rows).
func (s *Styler) Watch(key any, f func()) {
	if s.keyed == nil {
		s.keyed = map[any]func(){}
	}
	s.keyed[key] = f
}

func (s *Styler) Unwatch(key any) { delete(s.keyed, key) }

// Selectors lists the CSS selectors that may style w, most specific first.
func Selectors(w *qt.QWidget, kind string) []string {
	var out []string
	name := w.ObjectName()
	role := ""
	if v := w.Property("role"); v != nil {
		role = v.ToString()
	}
	if name != "" {
		out = append(out, kind+"#"+name, "#"+name)
	}
	if role != "" {
		out = append(out, kind+`[role="`+role+`"]`, `[role="`+role+`"]`)
	}
	return append(out, kind)
}

// P reads properties for one widget.
type P struct {
	s    *Styler
	sels []string
}

func (s *Styler) For(w *qt.QWidget, kind string) P { return P{s, Selectors(w, kind)} }

// ForName reads properties by object name, before the widget exists.
func (s *Styler) ForName(name, kind string) P {
	return P{s, []string{kind + "#" + name, "#" + name, kind}}
}

func (p P) Str(name, def string) string {
	if v, ok := p.s.c.Prop(p.sels, name); ok {
		return strings.Trim(v, `"' `)
	}
	return def
}

func (p P) Has(name string) bool { _, ok := p.s.c.Prop(p.sels, name); return ok }

func (p P) Color(name string, def theme.Color) theme.Color {
	if v, ok := p.s.c.Prop(p.sels, name); ok {
		if c, ok := theme.ParseColor(v); ok {
			return c
		}
	}
	return def
}

func (p P) Len(name string, def float64) float64 {
	v, _ := p.s.c.Prop(p.sels, name)
	return theme.Length(v, 16, def)
}

func (p P) Num(name string, def float64) float64 {
	v, _ := p.s.c.Prop(p.sels, name)
	return theme.Number(v, def)
}

func (p P) Ms(name string, def int) int {
	v, _ := p.s.c.Prop(p.sels, name)
	return theme.Duration(v, def)
}

// MsScaled is Ms divided by the animation speed setting (0 when animations
// are reduced).
func (p P) MsScaled(name string, def int) int {
	ms := p.Ms(name, def)
	if p.s.speed != nil {
		k := p.s.speed()
		if k <= 0.01 {
			return 0
		}
		return int(float64(ms) / k)
	}
	return ms
}

func (p P) Bool(name string, def bool) bool {
	v, _ := p.s.c.Prop(p.sels, name)
	return theme.Bool(v, def)
}

// Shadow parses "x y blur color".
func (p P) Shadow(name string) (dx, dy, blur float64, c theme.Color, ok bool) {
	v, has := p.s.c.Prop(p.sels, name)
	if !has || strings.TrimSpace(v) == "none" {
		return 0, 0, 0, theme.Color{}, false
	}
	parts := splitTop(v)
	var nums []float64
	for _, x := range parts {
		if col, isCol := theme.ParseColor(x); isCol {
			c = col
			ok = true
			continue
		}
		nums = append(nums, theme.Length(x, 16, 0))
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}
	if !ok {
		c, ok = theme.Color{A: .4}, true
	}
	return nums[0], nums[1], nums[2], c, ok
}

// splitTop splits on spaces outside parentheses.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ' ':
			if depth == 0 {
				if t := strings.TrimSpace(s[start:i]); t != "" {
					out = append(out, t)
				}
				start = i + 1
			}
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

// ---------------------------------------------------------------- Qt helpers

// qc converts a theme color into a new QColor (caller deletes).
func qc(c theme.Color) *qt.QColor {
	return qt.NewQColor11(clampByte(c.R), clampByte(c.G), clampByte(c.B), clampByte(c.A*255))
}

func clampByte(f float64) int { return int(math.Max(0, math.Min(255, math.Round(f)))) }

// fromQ converts a QColor (not deleted).
func fromQ(c *qt.QColor) theme.Color {
	return theme.Color{R: float64(c.Red()), G: float64(c.Green()), B: float64(c.Blue()), A: float64(c.Alpha()) / 255}
}

// paletteText returns the text color QSS gave the widget.
func paletteText(w *qt.QWidget, role qt.QPalette__ColorRole) theme.Color {
	pal := w.Palette()
	c := pal.ColorWithCr(role)
	return fromQ(c)
}

func withAlpha(c theme.Color, a float64) theme.Color { c.A *= a; return c }

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// setProp sets a dynamic property and re-polishes so QSS selectors update.
func setProp(w *qt.QWidget, name, value string) {
	v := qt.NewQVariant14(value)
	w.SetProperty(name, v)
	v.Delete()
	repolish(w)
}

func repolish(w *qt.QWidget) {
	st := w.Style()
	st.Unpolish(w)
	st.Polish(w)
	w.Update()
}

func role(w *qt.QWidget, r string) { setProp(w, "role", r) }

// font builds a QFont from CSS-like values (caller deletes).
func font(families string, px float64, weight int) *qt.QFont {
	f := qt.NewQFont()
	var fams []string
	for _, x := range strings.Split(families, ",") {
		x = strings.Trim(strings.TrimSpace(x), `"'`)
		if x != "" && x != "sans-serif" && x != "serif" {
			fams = append(fams, x)
		}
	}
	if len(fams) > 0 {
		f.SetFamilies(fams)
	}
	f.SetPixelSize(int(math.Max(6, px)))
	f.SetWeight(cssWeight(weight))
	return f
}

// cssWeight maps CSS 100..900 to Qt5 0..99.
func cssWeight(w int) int {
	switch {
	case w <= 150:
		return 0
	case w <= 250:
		return 12
	case w <= 350:
		return 25
	case w <= 450:
		return 50
	case w <= 550:
		return 57
	case w <= 650:
		return 63
	case w <= 750:
		return 75
	case w <= 850:
		return 81
	}
	return 87
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}

func rectF(x, y, w, h float64) *qt.QRectF { return qt.NewQRectF4(x, y, w, h) }

// fillRounded fills a rounded rect with a color.
func fillRounded(p *qt.QPainter, x, y, w, h, r float64, c theme.Color) {
	if c.A <= 0.001 || w <= 0 || h <= 0 {
		return
	}
	path := qt.NewQPainterPath()
	path.AddRoundedRect2(x, y, w, h, r, r)
	col := qc(c)
	br := qt.NewQBrush3(col)
	p.FillPath(path, br)
	br.Delete()
	col.Delete()
	path.Delete()
}

func setPenColor(p *qt.QPainter, c theme.Color) {
	col := qc(c)
	p.SetPen(col)
	col.Delete()
}
