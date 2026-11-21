package theme

import (
	"strings"
	"testing"
)

func TestCompile(t *testing.T) {
	css := `/* comment */
:root { --accent: #c25a62; --soft: alpha(var(--accent), .5); --text: #fff; }
QPushButton, #Play { color: var(--text); background: var(--soft); -meo-hover-bg: lighten(var(--accent), 10%); }
#LyricsView { -meo-active-color: mix(var(--accent), #ffffff, 0.5); font-size: 20px; }
QWidget[role='nav'] { -meo-enter: fade; }
@keyframes x { from { a: b } }
`
	c, err := Compile([]Source{{CSS: css}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.QSS, "background: rgba(194, 90, 98, 128);") {
		t.Errorf("qss:\n%s", c.QSS)
	}
	if strings.Contains(c.QSS, "-meo-") || strings.Contains(c.QSS, ":root") || strings.Contains(c.QSS, "keyframes") {
		t.Errorf("custom stuff leaked:\n%s", c.QSS)
	}
	if v, _ := c.Prop([]string{"#Play"}, "hover-bg"); !strings.HasPrefix(v, "#") {
		t.Errorf("hover-bg = %q", v)
	}
	if v, _ := c.Prop([]string{"#LyricsView"}, "active-color"); v != "#e1adb1" {
		t.Errorf("active-color = %q", v)
	}
	if _, ok := c.Prop([]string{`QWidget[role="nav"]`}, "enter"); !ok {
		t.Errorf("attribute selector not normalized: %v", c.Props)
	}
}

func TestEase(t *testing.T) {
	for _, n := range []string{"linear", "out-cubic", "out-back", "in-out-expo", "out-bounce", "ease-out"} {
		if Ease(n, 0) != 0 || Ease(n, 1) != 1 {
			t.Errorf("%s endpoints", n)
		}
	}
}

func TestBezier(t *testing.T) {
	if v := Ease("cubic-bezier(0,0,1,1)", 0.3); v < 0.29 || v > 0.31 {
		t.Fatalf("linear bezier: %v", v)
	}
	RegisterEasing("snappy", 0.2, 0.9, 0.1, 1)
	if v := Ease("snappy", 0.5); v < 0.85 {
		t.Fatalf("snappy: %v", v)
	}
	ResetCustomEasings()
}
