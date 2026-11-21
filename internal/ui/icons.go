package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// activeIcons is the icon pack drawIcon uses (nil = built-in vector icons).
var activeIcons *IconPack

// IconNames lists every icon the app draws (an icon pack may replace any of
// them; missing ones fall back to the built-in drawing).
var IconNames = []string{"play", "pause", "next", "prev", "shuffle", "repeat", "repeat-one", "volume", "volume-low", "mute",
	"lyrics", "queue", "home", "search", "library", "plugin", "theme", "settings", "code", "plus", "close", "note", "heart",
	"heart-outline", "folder", "refresh", "fish", "chevron-down", "chevron-up", "chevron-left", "chevron-right", "back",
	"more", "drag", "playlist", "export", "import", "radio", "trash", "edit", "check", "info", "fullscreen", "play-next",
	"minimize", "maximize", "restore", "video", "download", "timer", "eq", "people"}

// IconPack draws icons from an xtm plugin folder: <name>.svg or <name>.png.
// When tint is true the icon is recolored with the theme color.
type IconPack struct {
	dir   string
	tint  bool
	files map[string]string
	cache map[string]*qt.QPixmap
}

// Load switches to the pack in dir ("" = built-in icons).
func (ip *IconPack) Load(dir string, tint bool) {
	for _, px := range ip.cache {
		px.Delete()
	}
	ip.cache = map[string]*qt.QPixmap{}
	ip.dir, ip.tint, ip.files = dir, tint, map[string]string{}
	if dir == "" {
		activeIcons = nil
		return
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".svg" || ext == ".png" {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if old, ok := ip.files[name]; !ok || strings.HasSuffix(old, ".png") {
				ip.files[name] = filepath.Join(dir, e.Name())
			}
		}
	}
	activeIcons = ip
}

// Has reports whether the pack provides an icon.
func (ip *IconPack) Has(name string) bool { _, ok := ip.files[name]; return ok }

func (ip *IconPack) draw(p *qt.QPainter, name string, cx, cy, size float64, c theme.Color) bool {
	f, ok := ip.files[name]
	if !ok {
		return false
	}
	px := int(math.Ceil(size * 2)) // render at 2x for sharp edges
	key := fmt.Sprintf("%s|%d|%.0f,%.0f,%.0f,%.2f", name, px, c.R, c.G, c.B, c.A)
	pm, ok := ip.cache[key]
	if !ok {
		if len(ip.cache) > 400 {
			for k, v := range ip.cache {
				v.Delete()
				delete(ip.cache, k)
			}
		}
		pm = ip.render(f, px, c)
		ip.cache[key] = pm
	}
	if pm == nil {
		return false
	}
	r := rectF(cx-size/2, cy-size/2, size, size)
	sr := rectF(0, 0, float64(pm.Width()), float64(pm.Height()))
	p.DrawPixmap(r, pm, sr)
	r.Delete()
	sr.Delete()
	return true
}

func (ip *IconPack) render(file string, px int, c theme.Color) *qt.QPixmap {
	icon := qt.NewQIcon4(file)
	defer icon.Delete()
	sz := qt.NewQSize2(px, px)
	defer sz.Delete()
	src := icon.Pixmap(sz)
	if src == nil || src.IsNull() {
		return nil
	}
	out := qt.NewQPixmap2(px, px)
	transparent := qt.NewQColor11(0, 0, 0, 0)
	out.FillWithFillColor(transparent)
	transparent.Delete()
	p := qt.NewQPainter2(out.QPaintDevice)
	p.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
	r := rectF(0, 0, float64(px), float64(px))
	sr := rectF(0, 0, float64(src.Width()), float64(src.Height()))
	p.DrawPixmap(r, src, sr)
	if ip.tint {
		p.SetCompositionMode(qt.QPainter__CompositionMode_SourceIn)
		col := qc(c)
		p.FillRect4(r, col)
		col.Delete()
	}
	r.Delete()
	sr.Delete()
	p.End()
	p.Delete()
	return out
}
