package ui

import (
	"math"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// deco animates the background (and icon color) of a widget between its
// normal, hover and active states, using -meo-bg / -meo-hover-bg /
// -meo-active-bg / -meo-icon-color / -meo-hover-icon-color /
// -meo-active-icon-color / -meo-hover-duration / -meo-radius /
// -meo-press-scale.
type deco struct {
	u        *UI
	w        *qt.QWidget
	kind     string
	hovered  bool
	active   bool
	pressed  bool
	bg, icon theme.Color
	scale    float64
	enabled  bool // the theme defines -meo-bg or -meo-hover-bg
}

func (u *UI) newDeco(w *qt.QWidget, kind string) *deco {
	d := &deco{u: u, w: w, kind: kind, scale: 1}
	d.refresh(true)
	u.style.Watch(d, func() { d.refresh(true) })
	return d
}

// dispose forgets the deco (call before deleting its widget).
func (d *deco) dispose() {
	d.u.style.Unwatch(d)
	d.u.anim.Cancel(d, [2]any{d, "press"}, d.w.UnsafePointer())
}

func (d *deco) props() P { return d.u.style.For(d.w, d.kind) }

func (d *deco) target() (bg, icon theme.Color) {
	p := d.props()
	text := paletteText(d.w, qt.QPalette__ButtonText)
	bg = p.Color("bg", theme.Color{})
	icon = p.Color("icon-color", text)
	if d.hovered {
		bg = p.Color("hover-bg", bg)
		icon = p.Color("hover-icon-color", icon)
	}
	if d.active {
		bg = p.Color("active-bg", bg)
		icon = p.Color("active-icon-color", icon)
	}
	return
}

func (d *deco) refresh(immediate bool) {
	p := d.props()
	d.enabled = p.Has("bg") || p.Has("hover-bg") || p.Has("active-bg")
	bg, icon := d.target()
	if immediate {
		d.bg, d.icon = bg, icon
		d.w.Update()
		return
	}
	from, fromIcon := d.bg, d.icon
	dur := time.Duration(p.Ms("hover-duration", 160)) * time.Millisecond
	d.u.anim.Tween(d, dur, 0, p.Str("hover-easing", "out-quad"), func(t float64) {
		d.bg = theme.Mix(from, bg, t)
		d.icon = theme.Mix(fromIcon, icon, t)
		d.w.Update()
	}, nil)
}

func (d *deco) setHover(h bool)  { d.hovered = h; d.refresh(false) }
func (d *deco) setActive(a bool) { d.active = a; d.refresh(false) }

func (d *deco) setPressed(on bool) {
	d.pressed = on
	target := 1.0
	if on {
		target = d.props().Num("press-scale", 1)
	}
	from := d.scale
	d.u.anim.Tween([2]any{d, "press"}, 140*time.Millisecond, 0, "out-back", func(t float64) {
		d.scale = lerp(from, target, t)
		d.w.Update()
	}, nil)
}

// paintBg paints the animated background.
func (d *deco) paintBg(p *qt.QPainter, defRadius float64) {
	if !d.enabled {
		return
	}
	w, h := float64(d.w.Width()), float64(d.w.Height())
	r := d.props().Len("radius", defRadius)
	s := d.scale
	fillRounded(p, w*(1-s)/2, h*(1-s)/2, w*s, h*s, r*s, d.bg)
}

// ---------------------------------------------------------------- icon buttons

// IconButton is a QPushButton whose icon is drawn as vector art.
type IconButton struct {
	*qt.QPushButton
	d    *deco
	Icon string
}

func (u *UI) iconButton(name, icon, tip string) *IconButton {
	b := &IconButton{QPushButton: qt.NewQPushButton2(), Icon: icon}
	b.SetObjectName(name)
	role(b.QWidget, "control")
	b.SetToolTip(tip)
	b.SetCursor(qt.NewQCursor2(qt.PointingHandCursor))
	b.d = u.newDeco(b.QWidget, "QPushButton")
	b.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { super(e); b.d.setHover(true) })
	b.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { super(e); b.d.setHover(false) })
	b.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { super(e); b.d.setPressed(true) })
	b.OnMouseReleaseEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { super(e); b.d.setPressed(false) })
	b.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		super(e) // QSS background/border
		p := qt.NewQPainter2(b.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		w, h := float64(b.Width()), float64(b.Height())
		b.d.paintBg(p, math.Min(w, h)/2)
		size := b.d.props().Len("icon-size", 18) * b.d.scale
		drawIcon(p, b.Icon, w/2, h/2, size, b.d.icon)
		p.End()
		p.Delete()
	})
	return b
}

func (b *IconButton) SetIcon(name string) { b.Icon = name; b.Update() }

func (b *IconButton) SetActive(on bool) {
	setProp(b.QWidget, "active", boolStr(on))
	b.d.setActive(on)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// NavButton is a sidebar button: icon + text, fully painted so the icon and
// the background animate together.
type NavButton struct {
	*qt.QPushButton
	d    *deco
	Icon string
}

func (u *UI) navButton(name, icon, text string) *NavButton {
	b := &NavButton{QPushButton: qt.NewQPushButton2(), Icon: icon}
	b.SetObjectName(name)
	b.SetText(text)
	role(b.QWidget, "nav")
	setProp(b.QWidget, "active", "false")
	b.SetCursor(qt.NewQCursor2(qt.PointingHandCursor))
	b.SetMinimumHeight(40)
	b.d = u.newDeco(b.QWidget, "QPushButton")
	b.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { super(e); b.d.setHover(true) })
	b.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { super(e); b.d.setHover(false) })
	b.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		if !b.d.enabled {
			super(e)
		}
		p := qt.NewQPainter2(b.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		p.SetRenderHint(qt.QPainter__TextAntialiasing)
		w, h := float64(b.Width()), float64(b.Height())
		b.d.paintBg(p, 10)
		pad := b.d.props().Len("padding-x", 14)
		size := b.d.props().Len("icon-size", 17)
		drawIcon(p, b.Icon, pad+size/2, h/2, size, b.d.icon)
		if b.d.enabled {
			p.SetFont(b.Font())
			setPenColor(p, paletteText(b.QWidget, qt.QPalette__ButtonText))
			r := rectF(pad+size+12, 0, w-pad-size-12, h)
			p.DrawText5(r, int(qt.AlignLeft|qt.AlignVCenter), b.Text())
			r.Delete()
		}
		p.End()
		p.Delete()
	})
	return b
}

func (b *NavButton) SetActive(on bool) {
	setProp(b.QWidget, "active", boolStr(on))
	b.d.setActive(on)
}

// ---------------------------------------------------------------- vector icons

// drawIcon draws a named icon centered at (cx, cy) inside a size×size box.
func drawIcon(p *qt.QPainter, name string, cx, cy, size float64, c theme.Color) {
	if name == "" {
		return
	}
	s := size / 24 // icons are designed on a 24 px grid
	X := func(v float64) float64 { return cx + (v-12)*s }
	Y := func(v float64) float64 { return cy + (v-12)*s }
	col := qc(c)
	defer col.Delete()
	brush := qt.NewQBrush3(col)
	defer brush.Delete()
	pen := qt.NewQPen4(brush, math.Max(1.2, 2*s))
	pen.SetCapStyle(qt.RoundCap)
	pen.SetJoinStyle(qt.RoundJoin)
	defer pen.Delete()

	fill := func(pts ...float64) {
		path := qt.NewQPainterPath()
		path.MoveTo2(X(pts[0]), Y(pts[1]))
		for i := 2; i+1 < len(pts); i += 2 {
			path.LineTo2(X(pts[i]), Y(pts[i+1]))
		}
		path.CloseSubpath()
		p.FillPath(path, brush)
		path.Delete()
	}
	rect := func(x, y, w, h, r float64) {
		path := qt.NewQPainterPath()
		path.AddRoundedRect2(X(x), Y(y), w*s, h*s, r*s, r*s)
		p.FillPath(path, brush)
		path.Delete()
	}
	stroke := func(closed bool, pts ...float64) {
		path := qt.NewQPainterPath()
		path.MoveTo2(X(pts[0]), Y(pts[1]))
		for i := 2; i+1 < len(pts); i += 2 {
			path.LineTo2(X(pts[i]), Y(pts[i+1]))
		}
		if closed {
			path.CloseSubpath()
		}
		p.SetPenWithPen(pen)
		p.SetBrushWithStyle(qt.NoBrush)
		p.DrawPath(path)
		path.Delete()
	}
	circle := func(x, y, r float64, filled bool) {
		path := qt.NewQPainterPath()
		path.AddEllipse2(X(x-r), Y(y-r), 2*r*s, 2*r*s)
		if filled {
			p.FillPath(path, brush)
		} else {
			p.SetPenWithPen(pen)
			p.SetBrushWithStyle(qt.NoBrush)
			p.DrawPath(path)
		}
		path.Delete()
	}

	switch name {
	case "play":
		fill(8, 5, 19, 12, 8, 19)
	case "pause":
		rect(6.5, 5, 4, 14, 1.3)
		rect(13.5, 5, 4, 14, 1.3)
	case "next":
		fill(5, 5.5, 15, 12, 5, 18.5)
		rect(16, 5.5, 3, 13, 1)
	case "prev":
		fill(19, 5.5, 9, 12, 19, 18.5)
		rect(5, 5.5, 3, 13, 1)
	case "shuffle":
		stroke(false, 3, 7, 7, 7, 15, 17, 20, 17)
		stroke(false, 3, 17, 7, 17, 15, 7, 20, 7)
		stroke(false, 17.5, 4.5, 20, 7, 17.5, 9.5)
		stroke(false, 17.5, 14.5, 20, 17, 17.5, 19.5)
	case "repeat", "repeat-one":
		stroke(false, 4, 12, 4, 9, 6, 7, 18, 7)
		stroke(false, 15.5, 4.5, 18, 7, 15.5, 9.5)
		stroke(false, 20, 12, 20, 15, 18, 17, 6, 17)
		stroke(false, 8.5, 14.5, 6, 17, 8.5, 19.5)
		if name == "repeat-one" {
			rect(11, 9.8, 2, 4.6, .6)
		}
	case "volume", "volume-low", "mute":
		fill(3, 9, 7, 9, 12, 4.5, 12, 19.5, 7, 15, 3, 15)
		switch name {
		case "volume":
			stroke(false, 15.5, 8.5, 17, 10.5, 17, 13.5, 15.5, 15.5)
			stroke(false, 18.5, 5.5, 21, 9, 21, 15, 18.5, 18.5)
		case "volume-low":
			stroke(false, 15.5, 8.5, 17, 10.5, 17, 13.5, 15.5, 15.5)
		case "mute":
			stroke(false, 15, 9, 21, 15)
			stroke(false, 21, 9, 15, 15)
		}
	case "lyrics":
		stroke(true, 4, 5, 20, 5, 20, 16, 11, 16, 7, 20, 7, 16, 4, 16)
		stroke(false, 8, 9, 16, 9)
		stroke(false, 8, 12.5, 13, 12.5)
	case "queue":
		stroke(false, 4, 6, 15, 6)
		stroke(false, 4, 11, 15, 11)
		stroke(false, 4, 16, 11, 16)
		fill(15, 13.5, 21, 17, 15, 20.5)
	case "home":
		stroke(true, 4, 11, 12, 4, 20, 11, 20, 20, 4, 20)
		rect(10, 14, 4, 6, 1)
	case "search":
		circle(10.5, 10.5, 6, false)
		stroke(false, 15, 15, 20, 20)
	case "plugin":
		stroke(true, 5, 8, 9, 8, 9, 5.5, 11, 4, 13, 5.5, 13, 8, 18, 8, 18, 12, 20, 13.5, 18, 15, 18, 19, 5, 19)
	case "theme":
		stroke(true, 12, 3.5, 17, 5, 20, 9, 20, 13, 17, 14, 15.5, 16, 16, 19, 13, 20.5, 8, 19, 4.5, 15, 4, 10, 7, 5.5)
		circle(8.5, 11, 1.4, true)
		circle(12, 7.8, 1.4, true)
		circle(16, 9.5, 1.4, true)
	case "settings":
		circle(12, 12, 3, false)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			x1, y1 := 12+math.Cos(a)*6.5, 12+math.Sin(a)*6.5
			x2, y2 := 12+math.Cos(a)*8.8, 12+math.Sin(a)*8.8
			stroke(false, x1, y1, x2, y2)
		}
		circle(12, 12, 6.5, false)
	case "plus":
		stroke(false, 12, 5, 12, 19)
		stroke(false, 5, 12, 19, 12)
	case "close":
		stroke(false, 6, 6, 18, 18)
		stroke(false, 18, 6, 6, 18)
	case "note":
		stroke(false, 9, 17, 9, 5, 19, 3.5, 19, 15)
		circle(7, 17.5, 2.6, true)
		circle(17, 15.5, 2.6, true)
	case "heart":
		fill(12, 20, 4, 12, 3.5, 8, 6, 5, 9.5, 5, 12, 7.5, 14.5, 5, 18, 5, 20.5, 8, 20, 12)
	case "folder":
		stroke(true, 3.5, 6, 9.5, 6, 11.5, 8, 20.5, 8, 20.5, 18.5, 3.5, 18.5)
	case "refresh":
		stroke(false, 19, 8, 17, 5.5, 13.5, 4.2, 10, 4.5, 6.5, 6.5, 4.5, 10, 4.5, 14, 6.5, 17.5, 10, 19.5, 14, 19.5, 17.5, 17.5, 19.3, 14.5)
		fill(20, 3.5, 20, 9, 14.5, 8.5)
	}
}
