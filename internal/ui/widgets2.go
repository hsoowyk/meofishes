package ui

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/core"
	"meofishes/internal/theme"
	"meofishes/internal/viz"
)

// ---------------------------------------------------------------- LyricLine

// LyricLine shows the line being sung and swipes to the next one.
// Props (#CurrentLyric): -meo-swipe (up | down | left | right | fade),
// -meo-swipe-duration, -meo-swipe-easing, -meo-swipe-distance, -meo-font-size,
// -meo-font-weight, -meo-font-family, -meo-active-color, -meo-align.
type LyricLine struct {
	*qt.QWidget
	u     *UI
	idx   int
	cur   string
	prev  string
	t     float64 // swipe progress 0..1
	timer *qt.QTimer
	empty string
}

func (u *UI) lyricLine(name string) *LyricLine {
	l := &LyricLine{QWidget: qt.NewQWidget2(), u: u, idx: -2, t: 1}
	l.SetObjectName(name)
	l.SetMinimumHeight(int(u.style.ForName(name, "LyricLine").Len("height", 56)))
	l.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { l.paint() })
	l.timer = qt.NewQTimer2(l.QObject)
	l.timer.SetInterval(60)
	l.timer.OnTimeout(l.tick)
	l.timer.Start2()
	l.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { u.toggleFull() })
	handCursor(l.QWidget)
	l.SetToolTip(tr("Open the lyrics (Ctrl+L)"))
	u.style.OnChange(func() {
		l.SetMinimumHeight(int(u.style.ForName(name, "LyricLine").Len("height", 56)))
		l.Update()
	})
	return l
}

func (l *LyricLine) tick() {
	if !l.IsVisible() {
		return
	}
	ly := l.u.lyrics
	idx, text := -1, ""
	if ly != nil && ly.Sync != core.SyncNone {
		pos := l.u.lyricsPos()
		for i, ln := range ly.Lines {
			if ln.Start <= pos {
				idx = i
			} else {
				break
			}
		}
		if idx >= 0 {
			ln := ly.Lines[idx]
			if pos > ln.End+1500 && (idx+1 >= len(ly.Lines) || ly.Lines[idx+1].Start-ln.End > 4000) {
				text = "♪" // instrumental gap
			} else {
				text = ln.Text
			}
		}
	}
	if idx == l.idx && text == l.cur {
		return
	}
	l.idx = idx
	l.swipeTo(text)
}

func (l *LyricLine) swipeTo(text string) {
	l.prev, l.cur = l.cur, text
	p := l.u.style.For(l.QWidget, "LyricLine")
	dur := time.Duration(p.Ms("swipe-duration", 420)) * time.Millisecond
	l.u.anim.Tween(l, dur, 0, p.Str("swipe-easing", "out-cubic"), func(t float64) {
		l.t = t
		l.Update()
	}, nil)
}

func (l *LyricLine) paint() {
	p := qt.NewQPainter2(l.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.SetRenderHint(qt.QPainter__TextAntialiasing)
	pr := l.u.style.For(l.QWidget, "LyricLine")
	w, h := float64(l.Width()), float64(l.Height())
	f := font(pr.Str("font-family", ""), pr.Len("font-size", 17), int(pr.Num("font-weight", 700)))
	defer f.Delete()
	p.SetFont(f)
	col := pr.Color("active-color", paletteText(l.QWidget, qt.QPalette__WindowText))
	mode := pr.Str("swipe", "up")
	dist := pr.Len("swipe-distance", h*0.7)
	align := int(qt.AlignVCenter) | int(qt.TextWordWrap)
	switch pr.Str("align", "left") {
	case "center":
		align |= int(qt.AlignHCenter)
	case "right":
		align |= int(qt.AlignRight)
	default:
		align |= int(qt.AlignLeft)
	}
	draw := func(text string, off, alpha float64) {
		if text == "" || alpha <= 0.01 {
			return
		}
		dx, dy := 0.0, 0.0
		switch mode {
		case "up":
			dy = off
		case "down":
			dy = -off
		case "left":
			dx = off
		case "right":
			dx = -off
		}
		setPenColor(p, withAlpha(col, alpha))
		r := rectF(dx, dy, w, h)
		p.DrawText5(r, align, text)
		r.Delete()
	}
	t := l.t
	if mode == "fade" {
		draw(l.prev, 0, 1-t)
		draw(l.cur, 0, t)
		return
	}
	draw(l.prev, -dist*t, 1-t)
	draw(l.cur, dist*(1-t), t)
}

// ---------------------------------------------------------------- TiltCover

// TiltCover is the big cover of the fullscreen player. Under the mouse it
// tilts toward the pointer (gradually) and casts a shadow.
// Props: -meo-tilt-max (deg), -meo-tilt-smoothing (0..1 per frame),
// -meo-shadow-color, -meo-shadow-size, -meo-hover-scale, -meo-radius.
type TiltCover struct {
	*CoverView
	rx, ry  float64 // current rotation (deg)
	tx, ty  float64 // target rotation
	hover   float64 // 0..1
	hoverT  float64
	running bool
}

func (u *UI) tiltCover(name string) *TiltCover {
	c := &TiltCover{CoverView: u.coverView(name, false)}
	c.SetMouseTracking(true)
	c.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { c.paintTilted() })
	c.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if !u.cfg.Prefs.CoverTilt {
			return
		}
		mx := u.style.For(c.QWidget, "TiltCover").Num("tilt-max", u.cfg.Prefs.TiltMax)
		w, h := float64(c.Width()), float64(c.Height())
		nx := clamp((float64(e.X())-w/2)/(w/2), -1, 1)
		ny := clamp((float64(e.Y())-h/2)/(h/2), -1, 1)
		c.ty, c.tx = nx*mx, -ny*mx
		c.run()
	})
	c.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { c.hoverT = 1; c.run() })
	c.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { c.hoverT, c.tx, c.ty = 0, 0, 0; c.run() })
	return c
}

func (c *TiltCover) run() {
	if c.running {
		return
	}
	c.running = true
	c.u.anim.Ticker(c, func(time.Time) bool {
		k := clamp(c.u.style.For(c.QWidget, "TiltCover").Num("tilt-smoothing", 0.12), 0.01, 1)
		c.rx += (c.tx - c.rx) * k
		c.ry += (c.ty - c.ry) * k
		c.hover += (c.hoverT - c.hover) * k
		c.Update()
		done := math.Abs(c.tx-c.rx) < 0.02 && math.Abs(c.ty-c.ry) < 0.02 && math.Abs(c.hoverT-c.hover) < 0.01
		if done {
			c.rx, c.ry, c.hover = c.tx, c.ty, c.hoverT
			c.running = false
		}
		return !done
	})
}

func (c *TiltCover) paintTilted() {
	p := qt.NewQPainter2(c.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
	pr := c.u.style.For(c.QWidget, "TiltCover")
	w, h := float64(c.Width()), float64(c.Height())
	margin := pr.Len("shadow-size", 28)
	side := math.Min(w, h) - 2*margin
	if side < 10 {
		return
	}
	scale := 1 + (pr.Num("hover-scale", 1.03)-1)*c.hover
	x0, y0 := (w-side)/2, (h-side)/2
	r := math.Min(pr.Len("radius", 16), side/2)

	// soft shadow, offset away from the tilt, stronger on hover
	sc := pr.Color("shadow-color", theme.Color{A: 0.55})
	sa := sc.A * (0.35 + 0.65*c.hover)
	ox, oy := -c.ry*1.2, c.rx*1.2+margin*0.35*c.hover
	for i := 6; i >= 1; i-- {
		g := float64(i) / 6 * margin * (0.5 + 0.5*c.hover)
		fillRounded(p, x0+ox-g, y0+oy-g, side+2*g, side+2*g, r+g, withAlpha(sc, sa/7))
	}

	tr := qt.NewQTransform2()
	tr.Translate(w/2, h/2)
	tr.Rotate2(c.rx, qt.XAxis)
	tr.Rotate2(c.ry, qt.YAxis)
	tr.Scale(scale, scale)
	tr.Translate(-w/2, -h/2)
	p.SetTransform(tr)
	tr.Delete()

	path := qt.NewQPainterPath()
	path.AddRoundedRect2(x0, y0, side, side, r, r)
	if c.cur == nil || c.cur.IsNull() {
		fillRounded(p, x0, y0, side, side, r, pr.Color("placeholder", theme.Color{R: 60, G: 30, B: 36, A: 1}))
		drawIcon(p, "note", w/2, h/2, side*0.3, pr.Color("placeholder-icon", theme.Color{R: 255, G: 255, B: 255, A: .3}))
	} else {
		p.SetClipPath(path)
		draw := func(px *qt.QPixmap, a float64) {
			pw, ph := float64(px.Width()), float64(px.Height())
			k := math.Max(side/pw, side/ph)
			dst := rectF(x0+(side-pw*k)/2, y0+(side-ph*k)/2, pw*k, ph*k)
			src := rectF(0, 0, pw, ph)
			p.SetOpacity(a)
			p.DrawPixmap(dst, px, src)
			dst.Delete()
			src.Delete()
		}
		if c.prev != nil && c.fade < 1 {
			draw(c.prev, 1)
		}
		draw(c.cur, c.fade)
		p.SetOpacity(1)
		// glossy light that follows the tilt
		if c.hover > 0.01 {
			gx := x0 + side*(0.5+c.ry/40)
			gy := y0 + side*(0.5-c.rx/40)
			rg := qt.NewQRadialGradient3(gx, gy, side*0.8, gx, gy)
			c0, c1 := qc(theme.Color{R: 255, G: 255, B: 255, A: 0.16 * c.hover}), qc(theme.Color{R: 255, G: 255, B: 255, A: 0})
			rg.SetColorAt(0, c0)
			rg.SetColorAt(1, c1)
			br := qt.NewQBrush10(rg.QGradient)
			p.FillPath(path, br)
			br.Delete()
			c0.Delete()
			c1.Delete()
			rg.Delete()
		}
	}
	path.Delete()
}

// ---------------------------------------------------------------- Visualizer

// Visualizer draws the frequencies of the song as a smooth line when the
// song has no lyrics. The bands change every 0.1 s (internal/viz).
// Props: -meo-line-color, -meo-fill-color, -meo-line-width, -meo-bands,
// -meo-amplitude (0..1 of the height), -meo-mirror (true/false).
type Visualizer struct {
	*qt.QWidget
	u        *UI
	spec     *viz.Spectrum
	cancel   context.CancelFunc
	from, to []float32
	t0       time.Time
	frame    int64
	running  bool
}

func (u *UI) visualizer(name string) *Visualizer {
	v := &Visualizer{QWidget: qt.NewQWidget2(), u: u, frame: -1}
	v.SetObjectName(name)
	v.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { v.paint() })
	return v
}

func (v *Visualizer) bands() int {
	return int(clamp(v.u.style.For(v.QWidget, "Visualizer").Num("bands", 48), 8, 128))
}

// Start analyzes url (a local proxy URL, a file or any mpv-playable URL).
func (v *Visualizer) Start(url string, headers map[string]string) {
	v.Stop()
	if url == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.spec = viz.Analyze(ctx, url, headers, v.bands())
	v.from, v.to, v.frame = nil, nil, -1
}

// Stop cancels the analysis.
func (v *Visualizer) Stop() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.spec = nil
	v.from, v.to = nil, nil
	v.Update()
}

// SetRunning starts/stops the animation.
func (v *Visualizer) SetRunning(on bool) {
	v.running = on
	if !on {
		return
	}
	v.u.anim.Ticker(v, func(now time.Time) bool {
		if v.IsVisible() && v.spec != nil {
			pos := v.u.state().PositionMs
			fr := pos / viz.FrameMs
			if fr != v.frame {
				v.frame = fr
				v.from = v.current(now)
				v.to = v.spec.Frame(pos)
				v.t0 = now
			}
			v.Update()
		}
		return v.running
	})
}

// current returns the interpolated bands at now.
func (v *Visualizer) current(now time.Time) []float32 {
	if v.to == nil {
		return v.from
	}
	t := clamp(float64(now.Sub(v.t0))/float64(viz.FrameMs*time.Millisecond), 0, 1)
	t = theme.Ease("out-quad", t)
	out := make([]float32, len(v.to))
	for i := range out {
		var a float32
		if i < len(v.from) {
			a = v.from[i]
		}
		out[i] = a + (v.to[i]-a)*float32(t)
	}
	return out
}

func (v *Visualizer) paint() {
	p := qt.NewQPainter2(v.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	pr := v.u.style.For(v.QWidget, "Visualizer")
	w, h := float64(v.Width()), float64(v.Height())
	vals := v.current(time.Now())
	n := v.bands()
	if len(vals) == 0 {
		vals = make([]float32, n)
	}
	amp := pr.Num("amplitude", 0.8)
	mirror := pr.Bool("mirror", false)
	base := h * 0.85
	if mirror {
		base = h / 2
	}
	pts := make([][2]float64, len(vals)+2)
	pts[0] = [2]float64{0, base}
	for i, x := range vals {
		px := w * (float64(i) + 0.5) / float64(len(vals))
		hh := float64(x) * h * amp
		if mirror {
			hh /= 2
		}
		pts[i+1] = [2]float64{px, base - hh}
	}
	pts[len(pts)-1] = [2]float64{w, base}
	path := qt.NewQPainterPath()
	path.MoveTo2(pts[0][0], pts[0][1])
	// Catmull–Rom → Bézier for a smooth line
	for i := 0; i < len(pts)-1; i++ {
		p0 := pts[max(0, i-1)]
		p1, p2 := pts[i], pts[i+1]
		p3 := pts[min(len(pts)-1, i+2)]
		path.CubicTo2(p1[0]+(p2[0]-p0[0])/6, p1[1]+(p2[1]-p0[1])/6,
			p2[0]-(p3[0]-p1[0])/6, p2[1]-(p3[1]-p1[1])/6, p2[0], p2[1])
	}
	line := pr.Color("line-color", paletteText(v.QWidget, qt.QPalette__WindowText))
	fillC := pr.Color("fill-color", withAlpha(line, 0.18))
	// fill under the curve
	fp := qt.NewQPainterPath3(path)
	fp.LineTo2(w, h)
	fp.LineTo2(0, h)
	fp.CloseSubpath()
	g := qt.NewQLinearGradient3(0, base-h*amp, 0, h)
	c0, c1 := qc(fillC), qc(withAlpha(fillC, 0))
	g.SetColorAt(0, c0)
	g.SetColorAt(1, c1)
	br := qt.NewQBrush10(g.QGradient)
	p.FillPath(fp, br)
	br.Delete()
	c0.Delete()
	c1.Delete()
	g.Delete()
	fp.Delete()
	col := qc(line)
	lb := qt.NewQBrush3(col)
	pen := qt.NewQPen4(lb, pr.Len("line-width", 3))
	pen.SetCapStyle(qt.RoundCap)
	pen.SetJoinStyle(qt.RoundJoin)
	p.SetPenWithPen(pen)
	p.SetBrushWithStyle(qt.NoBrush)
	p.DrawPath(path)
	pen.Delete()
	lb.Delete()
	col.Delete()
	path.Delete()
}

// ---------------------------------------------------------------- Switch

// Switch is an animated on/off toggle (a QCheckBox painted as a pill).
// Props: -meo-track-color, -meo-on-color, -meo-knob-color.
type Switch struct {
	*qt.QCheckBox
	u   *UI
	pos float64
}

func (u *UI) toggle(on bool, onChange func(bool)) *Switch {
	s := &Switch{QCheckBox: qt.NewQCheckBox2(), u: u}
	s.SetObjectName("Switch")
	s.SetChecked(on)
	if on {
		s.pos = 1
	}
	s.SetFixedSize2(44, 24)
	handCursor(s.QWidget)
	s.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		p := qt.NewQPainter2(s.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		pr := u.style.For(s.QWidget, "Switch")
		w, h := float64(s.Width()), float64(s.Height())
		off := pr.Color("track-color", theme.Color{R: 255, G: 255, B: 255, A: 0.16})
		onC := pr.Color("on-color", theme.Color{R: 217, G: 115, B: 122, A: 1})
		fillRounded(p, 0, 0, w, h, h/2, theme.Mix(off, onC, s.pos))
		k := h - 6
		fillRounded(p, 3+(w-k-6)*s.pos, 3, k, k, k/2, pr.Color("knob-color", theme.Color{R: 255, G: 255, B: 255, A: 1}))
		p.End()
		p.Delete()
	})
	s.OnToggled(func(c bool) {
		from, to := s.pos, 0.0
		if c {
			to = 1
		}
		u.anim.Tween(s, 180*time.Millisecond, 0, "out-cubic", func(t float64) { s.pos = lerp(from, to, t); s.Update() }, nil)
		if onChange != nil {
			onChange(c)
		}
	})
	return s
}

// ---------------------------------------------------------------- FPS meter

type fpsMeter struct {
	label  *qt.QLabel
	frames int
	since  time.Time
	fps    int
}

func (f *fpsMeter) frame(now time.Time) {
	f.frames++
	if f.since.IsZero() {
		f.since = now
	}
	if d := now.Sub(f.since); d >= time.Second {
		f.fps = int(float64(f.frames) / d.Seconds())
		f.frames, f.since = 0, now
	}
}

func (u *UI) buildFPS(parent *qt.QWidget) {
	l := qt.NewQLabel(parent)
	l.SetObjectName("FpsMeter")
	l.SetAttribute(qt.WA_TransparentForMouseEvents)
	u.fps.label = l
	t := qt.NewQTimer2(l.QObject)
	t.SetInterval(500)
	t.OnTimeout(func() {
		if !l.IsVisible() {
			return
		}
		tw, tk := u.anim.Active()
		fps := u.fps.fps
		if tw+tk == 0 {
			fps = 0
		}
		l.SetText(tr("{fps} fps · {tweens} tweens · {tickers} tickers", "fps", itoa(fps), "tweens", itoa(tw), "tickers", itoa(tk)))
		l.AdjustSize()
		l.Move(parent.Width()-l.Width()-16, 58)
		l.Raise()
	})
	t.Start2()
	l.SetVisible(u.cfg.Prefs.DevMode && u.cfg.Prefs.ShowFPS)
}

// ---------------------------------------------------------------- artwork cache

// ArtCache finds album covers through the fnt "artwork" plugins, once per
// track, and remembers them on disk.
type ArtCache struct {
	u       *UI
	path    string
	mu      sync.Mutex
	known   map[string]string // key → url ("" = none found)
	waiting map[string][]func(string)
	sem     chan struct{}
	dirty   bool
}

func newArtCache(u *UI, path string) *ArtCache {
	a := &ArtCache{u: u, path: path, known: map[string]string{}, waiting: map[string][]func(string){}, sem: make(chan struct{}, 3)}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &a.known)
	}
	return a
}

// needs reports whether a track should get album art.
func (a *ArtCache) needs(t core.Track) bool {
	pr := a.u.cfg.Prefs
	if !pr.FetchArtwork || t.Title == "" || t.DurationMs == 0 && t.Cover != "" {
		return false
	}
	if t.Cover == "" {
		return true
	}
	return pr.PreferArtwork && isThumbnail(t.Cover)
}

func isThumbnail(u string) bool {
	return strings.Contains(u, "ytimg.com") || strings.Contains(u, "googleusercontent.com/vi")
}

// Get calls f on the UI thread with the art URL (only when one is found).
func (a *ArtCache) Get(t core.Track, f func(url string)) {
	if !a.needs(t) {
		return
	}
	key := strings.ToLower(t.Artist + "\x00" + t.Title)
	a.mu.Lock()
	if url, ok := a.known[key]; ok {
		a.mu.Unlock()
		if url != "" {
			f(url)
		}
		return
	}
	busy := len(a.waiting[key]) > 0
	a.waiting[key] = append(a.waiting[key], f)
	a.mu.Unlock()
	if busy {
		return
	}
	go func() {
		a.sem <- struct{}{}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		url, err := a.u.core.Plugins.Artwork(ctx, t)
		cancel()
		<-a.sem
		a.mu.Lock()
		if err == nil || strings.Contains(err.Error(), "not found") {
			a.known[key] = url
			a.dirty = true
		}
		cbs := a.waiting[key]
		delete(a.waiting, key)
		a.mu.Unlock()
		a.save()
		if url == "" {
			return
		}
		mainthread.Start(func() {
			for _, cb := range cbs {
				cb(url)
			}
		})
	}()
}

func (a *ArtCache) save() {
	a.mu.Lock()
	if !a.dirty {
		a.mu.Unlock()
		return
	}
	a.dirty = false
	data, _ := json.Marshal(a.known)
	a.mu.Unlock()
	_ = os.MkdirAll(filepath.Dir(a.path), 0o755)
	_ = os.WriteFile(a.path, data, 0o644)
}

// Clear forgets everything (Settings → Storage).
func (a *ArtCache) Clear() {
	a.mu.Lock()
	a.known = map[string]string{}
	a.dirty = true
	a.mu.Unlock()
	a.save()
}

// Size is the number of remembered tracks.
func (a *ArtCache) Size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.known)
}
