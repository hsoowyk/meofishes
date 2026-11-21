package ui

import (
	"math"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

func handCursor(w *qt.QWidget) {
	c := qt.NewQCursor2(qt.PointingHandCursor)
	w.SetCursor(c)
	c.Delete()
}

// applyShadow applies -meo-shadow as a QGraphicsDropShadowEffect.
func (u *UI) applyShadow(w *qt.QWidget, kind string) {
	dx, dy, blur, c, ok := u.style.For(w, kind).Shadow("shadow")
	if !ok {
		w.SetGraphicsEffect(nil)
		return
	}
	eff := qt.NewQGraphicsDropShadowEffect2(w.QObject)
	eff.SetOffset2(dx, dy)
	eff.SetBlurRadius(blur)
	col := qc(c)
	eff.SetColor(col)
	col.Delete()
	w.SetGraphicsEffect(eff.QGraphicsEffect)
}

// ---------------------------------------------------------------- CoverView

// CoverView paints album art with rounded corners, cross-fades between
// covers and zooms new covers in. Props: -meo-radius, -meo-crossfade,
// -meo-placeholder, -meo-placeholder-icon, -meo-shadow, -meo-zoom-on-change,
// -meo-zoom-easing.
type CoverView struct {
	*qt.QWidget
	u       *UI
	cur     *qt.QPixmap
	prev    *qt.QPixmap
	fade    float64 // 0 → only prev, 1 → only cur
	zoom    float64
	square  bool
	OnClick func()
	Icon    string // placeholder icon (default "note")
}

func (u *UI) coverView(name string, square bool) *CoverView {
	c := &CoverView{QWidget: qt.NewQWidget2(), u: u, fade: 1, zoom: 1, square: square}
	c.SetObjectName(name)
	c.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { c.paint() })
	c.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if c.OnClick != nil {
			c.OnClick()
		}
	})
	if square {
		c.OnResizeEvent(func(super func(*qt.QResizeEvent), e *qt.QResizeEvent) {
			super(e)
			if c.Height() != c.Width() {
				c.SetFixedHeight(c.Width())
			}
		})
	}
	u.style.Watch(c, func() { u.applyShadow(c.QWidget, "CoverView"); c.Update() })
	return c
}

// dispose forgets the cover (call before deleting it).
func (c *CoverView) dispose() {
	c.u.style.Unwatch(c)
	c.u.anim.Cancel(c)
	c.u.coverLoader.Forget(c)
	if c.cur != nil {
		c.cur.Delete()
		c.cur = nil
	}
	if c.prev != nil {
		c.prev.Delete()
		c.prev = nil
	}
}

// SetPixmap shows a new cover (nil = placeholder) with the transition.
func (c *CoverView) SetPixmap(px *qt.QPixmap) {
	if c.prev != nil {
		c.prev.Delete()
	}
	c.prev = c.cur
	if px != nil {
		c.cur = qt.NewQPixmap5(px)
	} else {
		c.cur = nil
	}
	p := c.u.style.For(c.QWidget, "CoverView")
	dur := time.Duration(p.Ms("crossfade", 400)) * time.Millisecond
	z0 := p.Num("zoom-on-change", 1)
	ease := p.Str("zoom-easing", "out-cubic")
	c.u.anim.Tween(c, dur, 0, "linear", func(t float64) {
		c.fade = t
		c.zoom = lerp(z0, 1, theme.Ease(ease, t))
		c.Update()
	}, func() {
		if c.prev != nil {
			c.prev.Delete()
			c.prev = nil
		}
	})
}

func (c *CoverView) paint() {
	p := qt.NewQPainter2(c.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
	pr := c.u.style.For(c.QWidget, "CoverView")
	w, h := float64(c.Width()), float64(c.Height())
	side := math.Min(w, h)
	x0, y0 := (w-side)/2, (h-side)/2
	r := math.Min(pr.Len("radius", 12), side/2) // "999px" makes a circle

	draw := func(px *qt.QPixmap, alpha, zoom float64) {
		s := side * zoom
		x, y := x0+(side-s)/2, y0+(side-s)/2
		p.Save()
		p.SetOpacity(alpha)
		path := qt.NewQPainterPath()
		path.AddRoundedRect2(x, y, s, s, r*zoom, r*zoom)
		if px == nil || px.IsNull() {
			col := qc(pr.Color("placeholder", theme.Color{R: 60, G: 30, B: 36, A: 1}))
			br := qt.NewQBrush3(col)
			p.FillPath(path, br)
			br.Delete()
			col.Delete()
			icon := c.Icon
			if icon == "" {
				icon = "note"
			}
			drawIcon(p, icon, x+s/2, y+s/2, s*0.36, pr.Color("placeholder-icon", theme.Color{R: 255, G: 255, B: 255, A: .3}))
		} else {
			p.SetClipPath(path)
			// cover-fit the pixmap into the square
			pw, ph := float64(px.Width()), float64(px.Height())
			k := math.Max(s/pw, s/ph)
			dw, dh := pw*k, ph*k
			tr := rectF(x+(s-dw)/2, y+(s-dh)/2, dw, dh)
			sr := rectF(0, 0, pw, ph)
			p.DrawPixmap(tr, px, sr)
			tr.Delete()
			sr.Delete()
		}
		path.Delete()
		p.Restore()
	}
	if c.prev != nil && c.fade < 1 {
		draw(c.prev, 1, 1)
	}
	draw(c.cur, c.fade, c.zoom)
}

// ---------------------------------------------------------------- Slider

// Slider is the seek/volume bar. Props: -meo-track-color, -meo-fill-color,
// -meo-fill-color-2, -meo-buffer-color, -meo-handle-color, -meo-handle-size,
// -meo-height, -meo-hover-height, -meo-radius, -meo-handle-on-hover, -meo-smooth.
type Slider struct {
	*qt.QWidget
	u        *UI
	value    float64 // shown value (eased)
	target   float64
	dragging bool
	hover    float64                // 0..1 hover animation
	OnSeek   func(v float64)        // released
	OnDrag   func(v float64)        // while dragging
	Tip      func(v float64) string // tooltip text for the hovered position
}

func (u *UI) slider(name string) *Slider {
	s := &Slider{QWidget: qt.NewQWidget2(), u: u}
	s.SetObjectName(name)
	s.SetMouseTracking(true)
	s.SetMinimumHeight(18)
	handCursor(s.QWidget)
	s.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { s.paint() })
	val := func(e *qt.QMouseEvent) float64 { return clamp(float64(e.X())/math.Max(1, float64(s.Width())), 0, 1) }
	s.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		s.dragging = true
		s.value, s.target = val(e), val(e)
		if s.OnDrag != nil {
			s.OnDrag(s.value)
		}
		s.Update()
	})
	s.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if s.Tip != nil {
			if t := s.Tip(val(e)); t != "" {
				qt.QToolTip_ShowText(e.GlobalPos(), t)
			}
		}
		if s.dragging {
			s.value, s.target = val(e), val(e)
			if s.OnDrag != nil {
				s.OnDrag(s.value)
			}
			s.Update()
		}
	})
	s.OnMouseReleaseEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if s.dragging {
			s.dragging = false
			s.value, s.target = val(e), val(e)
			if s.OnSeek != nil {
				s.OnSeek(s.value)
			}
		}
	})
	s.OnWheelEvent(func(super func(*qt.QWheelEvent), e *qt.QWheelEvent) {
		d := e.AngleDelta()
		step := float64(d.Y()) / 120 * 0.05
		v := clamp(s.target+step, 0, 1)
		s.SetValue(v)
		if s.OnSeek != nil {
			s.OnSeek(v)
		}
	})
	hoverTo := func(to float64) {
		from := s.hover
		u.anim.Tween(s, 160*time.Millisecond, 0, "out-quad", func(t float64) { s.hover = lerp(from, to, t); s.Update() }, nil)
	}
	s.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { hoverTo(1) })
	s.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { hoverTo(0) })
	u.style.OnChange(s.Update)
	return s
}

// SetValue moves the bar (ignored while the user drags it).
func (s *Slider) SetValue(v float64) {
	if s.dragging {
		return
	}
	v = clamp(v, 0, 1)
	if math.Abs(v-s.target) < 1e-4 {
		return
	}
	smooth := s.u.style.For(s.QWidget, "Slider").Ms("smooth", 0)
	from := s.value
	s.target = v
	if smooth <= 0 || math.Abs(v-from) > 0.2 {
		s.value = v
		s.Update()
		return
	}
	s.u.anim.Tween([2]any{s, "v"}, time.Duration(smooth)*time.Millisecond, 0, "linear", func(t float64) {
		s.value = lerp(from, v, t)
		s.Update()
	}, nil)
}

func (s *Slider) paint() {
	p := qt.NewQPainter2(s.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	pr := s.u.style.For(s.QWidget, "Slider")
	w, h := float64(s.Width()), float64(s.Height())
	bh := lerp(pr.Len("height", 4), pr.Len("hover-height", pr.Len("height", 4)), s.hover)
	if s.dragging {
		bh = pr.Len("hover-height", bh)
	}
	r := math.Min(pr.Len("radius", bh/2), bh/2)
	y := (h - bh) / 2
	fillRounded(p, 0, y, w, bh, r, pr.Color("track-color", theme.Color{R: 255, G: 255, B: 255, A: .15}))
	fw := w * s.value
	c1 := pr.Color("fill-color", theme.Color{R: 220, G: 110, B: 120, A: 1})
	if fw > 0.5 {
		path := qt.NewQPainterPath()
		path.AddRoundedRect2(0, y, fw, bh, r, r)
		if pr.Has("fill-color-2") {
			g := qt.NewQLinearGradient3(0, 0, math.Max(fw, 1), 0)
			a, b := qc(c1), qc(pr.Color("fill-color-2", c1))
			g.SetColorAt(0, a)
			g.SetColorAt(1, b)
			br := qt.NewQBrush10(g.QGradient)
			p.FillPath(path, br)
			br.Delete()
			a.Delete()
			b.Delete()
			g.Delete()
		} else {
			col := qc(c1)
			br := qt.NewQBrush3(col)
			p.FillPath(path, br)
			br.Delete()
			col.Delete()
		}
		path.Delete()
	}
	show := 1.0
	if pr.Bool("handle-on-hover", true) {
		show = s.hover
		if s.dragging {
			show = 1
		}
	}
	if hs := pr.Len("handle-size", 12) * show; hs > 0.5 {
		x := clamp(fw, hs/2, w-hs/2)
		fillRounded(p, x-hs/2, h/2-hs/2, hs, hs, hs/2, pr.Color("handle-color", theme.Color{R: 255, G: 255, B: 255, A: 1}))
	}
}

// ---------------------------------------------------------------- Backdrop

// Backdrop is the window background: a gradient tinted by the cover color,
// with a soft breathing light, and optionally the blurred cover (fullscreen
// player). It is painted into a cached pixmap and only repainted when
// something changes, so the window stays light. Props: -meo-backdrop-*.
type Backdrop struct {
	*qt.QWidget
	u        *UI
	tint     theme.Color // current dynamic color
	hasTint  bool
	pulseT0  time.Time
	pulseOn  bool
	img      *qt.QPixmap // blurred cover (nil = none)
	imgPrev  *qt.QPixmap
	imgFade  float64
	cache    *qt.QPixmap
	dirty    bool
	onResize []func()
}

func (u *UI) backdrop(name string) *Backdrop {
	b := &Backdrop{QWidget: qt.NewQWidget2(), u: u, pulseT0: time.Now(), dirty: true, imgFade: 1}
	b.SetObjectName(name)
	b.SetAttribute(qt.WA_StyledBackground)
	b.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { b.paint() })
	b.OnResizeEvent(func(super func(*qt.QResizeEvent), e *qt.QResizeEvent) {
		b.dirty = true
		for _, f := range b.onResize {
			f()
		}
	})
	u.style.OnChange(func() { b.restartPulse(); b.invalidate() })
	return b
}

func (b *Backdrop) invalidate() { b.dirty = true; b.Update() }

func (b *Backdrop) props() P { return b.u.style.For(b.QWidget, "Backdrop") }

func (b *Backdrop) restartPulse() {
	period := b.props().Ms("backdrop-glow-pulse", 0)
	if period <= 0 || b.u.cfg.Prefs.ReduceMotion {
		b.u.anim.StopTicker(b)
		b.pulseOn = false
		return
	}
	b.pulseOn = true
	last := time.Time{}
	b.u.anim.Ticker(b, func(now time.Time) bool {
		if now.Sub(last) > 100*time.Millisecond && b.IsVisible() { // 10 fps is plenty for a slow glow
			last = now
			b.invalidate()
		}
		return b.pulseOn
	})
}

// SetTint animates the dynamic color toward c (from the album cover).
func (b *Backdrop) SetTint(c theme.Color, ok bool) {
	p := b.props()
	if !p.Bool("backdrop-dynamic", false) {
		return
	}
	from := b.tint
	if !b.hasTint {
		from = withAlpha(c, 0)
	}
	if !ok {
		c = withAlpha(from, 0)
	}
	b.hasTint = true
	dur := time.Duration(p.Ms("backdrop-dynamic-duration", 1200)) * time.Millisecond
	// repainting the background repaints the whole window: a slow color
	// fade at 30 fps looks the same and costs a fraction
	last := time.Time{}
	b.u.anim.Tween(b, dur, 0, "in-out-sine", func(t float64) {
		b.tint = theme.Mix(from, c, t)
		if now := time.Now(); now.Sub(last) >= 33*time.Millisecond || t >= 1 {
			last = now
			b.invalidate()
		}
	}, nil)
}

// SetImage shows a blurred copy of px behind everything (nil removes it).
// The blur is a cheap downscale/upscale. Props: -meo-backdrop-image-opacity,
// -meo-backdrop-image-blur (the downscale size, smaller = blurrier).
func (b *Backdrop) SetImage(px *qt.QPixmap) {
	if b.imgPrev != nil {
		b.imgPrev.Delete()
	}
	b.imgPrev = b.img
	b.img = nil
	if px != nil && !px.IsNull() {
		n := int(b.props().Num("backdrop-image-blur", 18))
		small := px.Scaled3(max(4, n), max(4, n), qt.KeepAspectRatioByExpanding, qt.SmoothTransformation)
		b.img = qt.NewQPixmap5(small.Scaled3(256, 256, qt.KeepAspectRatioByExpanding, qt.SmoothTransformation))
	}
	b.u.anim.Tween([2]any{b, "img"}, 900*time.Millisecond, 0, "in-out-sine", func(t float64) {
		b.imgFade = t
		b.invalidate()
	}, func() {
		if b.imgPrev != nil {
			b.imgPrev.Delete()
			b.imgPrev = nil
		}
	})
}

func (b *Backdrop) paint() {
	w, h := b.Width(), b.Height()
	if w <= 0 || h <= 0 {
		return
	}
	if b.dirty || b.cache == nil || b.cache.Width() != w || b.cache.Height() != h {
		if b.cache == nil || b.cache.Width() != w || b.cache.Height() != h {
			if b.cache != nil {
				b.cache.Delete()
			}
			b.cache = qt.NewQPixmap2(w, h)
		}
		cp := qt.NewQPainter2(b.cache.QPaintDevice)
		cp.SetRenderHint(qt.QPainter__Antialiasing)
		cp.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
		b.render(cp, float64(w), float64(h))
		cp.End()
		cp.Delete()
		b.dirty = false
	}
	p := qt.NewQPainter2(b.QPaintDevice)
	p.DrawPixmap9(0, 0, b.cache)
	p.End()
	p.Delete()
}

func (b *Backdrop) render(p *qt.QPainter, w, h float64) {
	pr := b.props()
	from := pr.Color("backdrop-from", theme.Color{R: 36, G: 16, B: 21, A: 1})
	to := pr.Color("backdrop-to", theme.Color{R: 27, G: 11, B: 15, A: 1})
	if b.hasTint && b.tint.A > 0 {
		k := pr.Num("backdrop-dynamic-strength", 0.35) * b.tint.A
		t := b.tint
		t.A = 1
		from = theme.Mix(from, darkenFor(t, from), k)
		to = theme.Mix(to, darkenFor(t, to), k*0.6)
	}
	ang := pr.Num("backdrop-angle", 160) * math.Pi / 180
	cx, cy := w/2, h/2
	dx, dy := math.Sin(ang)*h/2, -math.Cos(ang)*h/2
	g := qt.NewQLinearGradient3(cx-dx, cy-dy, cx+dx, cy+dy)
	a, c := qc(from), qc(to)
	g.SetColorAt(0, a)
	g.SetColorAt(1, c)
	br := qt.NewQBrush10(g.QGradient)
	r := rectF(0, 0, w, h)
	p.FillRect(r, br)
	br.Delete()
	a.Delete()
	c.Delete()
	g.Delete()

	// blurred cover
	op := pr.Num("backdrop-image-opacity", 0.55)
	drawImg := func(px *qt.QPixmap, alpha float64) {
		if px == nil || alpha <= 0 {
			return
		}
		pw, ph := float64(px.Width()), float64(px.Height())
		k := math.Max(w/pw, h/ph)
		tr := rectF((w-pw*k)/2, (h-ph*k)/2, pw*k, ph*k)
		sr := rectF(0, 0, pw, ph)
		p.SetOpacity(alpha)
		p.DrawPixmap(tr, px, sr)
		p.SetOpacity(1)
		tr.Delete()
		sr.Delete()
	}
	drawImg(b.imgPrev, op*(1-b.imgFade))
	drawImg(b.img, op*b.imgFade)
	if b.img != nil || b.imgPrev != nil {
		shade := pr.Color("backdrop-image-shade", theme.Color{A: 0.35})
		col := qc(shade)
		p.FillRect4(r, col)
		col.Delete()
	}

	if pr.Has("backdrop-glow") {
		glow := pr.Color("backdrop-glow", theme.Color{})
		if b.hasTint && b.tint.A > 0 {
			t := b.tint
			t.A = glow.A
			glow = theme.Mix(glow, t, 0.5*b.tint.A)
		}
		if period := pr.Ms("backdrop-glow-pulse", 0); period > 0 && b.pulseOn {
			ph := float64(time.Since(b.pulseT0).Milliseconds()%int64(period)) / float64(period)
			glow.A *= 0.75 + 0.25*math.Sin(ph*2*math.Pi)
		}
		gx, gy := pr.Num("backdrop-glow-x", .8)*w, pr.Num("backdrop-glow-y", .1)*h
		rad := pr.Num("backdrop-glow-radius", .8) * math.Max(w, h)
		rg := qt.NewQRadialGradient3(gx, gy, rad, gx, gy)
		c0, c1 := qc(glow), qc(withAlpha(glow, 0))
		rg.SetColorAt(0, c0)
		rg.SetColorAt(1, c1)
		br2 := qt.NewQBrush10(rg.QGradient)
		p.FillRect(r, br2)
		br2.Delete()
		c0.Delete()
		c1.Delete()
		rg.Delete()
	}
	r.Delete()
}

// darkenFor keeps the tint at the lightness of the base color, so a bright
// cover never makes the background unreadable.
func darkenFor(tint, base theme.Color) theme.Color {
	h, s, _ := tint.HSL()
	_, _, l := base.HSL()
	return theme.FromHSL(h, math.Min(s, 0.75), l)
}

// ---------------------------------------------------------------- Toast

// Toast shows short messages at the bottom of the window.
type Toast struct {
	*qt.QLabel
	u     *UI
	eff   *qt.QGraphicsOpacityEffect
	timer *qt.QTimer
}

func (u *UI) newToast(parent *qt.QWidget) *Toast {
	t := &Toast{QLabel: qt.NewQLabel(parent), u: u}
	t.SetObjectName("Toast")
	t.SetAttribute(qt.WA_TransparentForMouseEvents)
	t.SetWordWrap(true)
	t.SetAlignment(qt.AlignCenter)
	t.eff = qt.NewQGraphicsOpacityEffect2(t.QObject)
	t.eff.SetOpacity(0)
	t.SetGraphicsEffect(t.eff.QGraphicsEffect)
	t.Hide()
	t.timer = qt.NewQTimer2(t.QObject)
	t.timer.SetSingleShot(true)
	t.timer.OnTimeout(t.hide)
	return t
}

func (t *Toast) Show(msg string) {
	t.SetText(msg)
	t.SetMaximumWidth(520)
	t.AdjustSize()
	par := t.ParentWidget()
	w := t.Width()
	if w > 520 {
		w = 520
	}
	t.Move((par.Width()-w)/2, par.Height()-t.Height()-110)
	t.QWidget.Show()
	t.Raise()
	p := t.u.style.For(t.QWidget, "Toast")
	from := t.eff.Opacity()
	t.u.anim.Tween(t, time.Duration(p.Ms("show-duration", 240))*time.Millisecond, 0, "out-quad",
		func(x float64) { t.eff.SetOpacity(lerp(from, 1, x)) }, nil)
	t.timer.Start(p.Ms("hide-after", 3000))
}

func (t *Toast) hide() {
	from := t.eff.Opacity()
	t.u.anim.Tween(t, 300*time.Millisecond, 0, "in-quad", func(x float64) { t.eff.SetOpacity(lerp(from, 0, x)) },
		func() { t.QWidget.Hide() })
}
