package ui

import (
	"math"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
	"meofishes/internal/theme"
)

// LyricsView paints word-synced lyrics. Everything about its look is read
// from -meo-* properties (see docs/THEMES.md, "Letras").
type LyricsView struct {
	*qt.QWidget
	u      *UI
	lyr    *core.Lyrics
	msg    string
	Pos    func() int64 // current position (ms)
	OnSeek func(ms int64)

	// layout cache
	lay      []layLine
	layW     int
	layKey   string
	totalH   float64
	fonts    *lyrFonts
	st       lyrStyle
	// scrolling
	scroll     float64
	scrollFrom float64
	scrollTo   float64
	scrollT0   time.Time
	active     int
	manualTill time.Time
	hoverLine  int
	// per-line scale animation
	lineScale []float64
	scaleFrom []float64
	scaleT0   time.Time
	running   bool
}

type layWord struct {
	w        core.Word
	x, y, wd float64 // relative to the line origin, y = baseline row index * rowH
	row      int
}

type layLine struct {
	idx      int
	y, h     float64 // top inside the document, height
	words    []layWord
	rows     int
	rowW     []float64
	bg       []layWord
	bgRows   int
	bgRowW   []float64
	align    string
	timed    bool // has word timing
	interlude bool // a gap marker before this line
}

type lyrFonts struct {
	main, bg       *qt.QFont
	mainFM, bgFM   *qt.QFontMetricsF
	rowH, bgRowH   float64
	ascent, bgAsc  float64
}

func (f *lyrFonts) delete() {
	if f == nil {
		return
	}
	f.main.Delete()
	f.bg.Delete()
	f.mainFM.Delete()
	f.bgFM.Delete()
}

type lyrStyle struct {
	color, past, active, upcoming, glow, bgVocal, dot, credit, hoverC, empty theme.Color
	glowR, lift, soft, activeScale, inactiveScale, focus, fade, pad, spacing, bgScale float64
	align, oppAlign, fill, scrollEase                                               string
	scrollDur, scaleDur                                                             int
	dots, lineFill                                                                  bool
	family                                                                          string
	size                                                                            float64
	weight                                                                          int
}

func (u *UI) lyricsView(name string) *LyricsView {
	v := &LyricsView{QWidget: qt.NewQWidget2(), u: u, active: -1, hoverLine: -1, Pos: func() int64 { return 0 }}
	v.SetObjectName(name)
	v.SetMouseTracking(true)
	v.SetMinimumHeight(80)
	v.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { v.paint() })
	v.OnResizeEvent(func(super func(*qt.QResizeEvent), e *qt.QResizeEvent) { v.layKey = ""; v.Update() })
	v.OnWheelEvent(func(super func(*qt.QWheelEvent), e *qt.QWheelEvent) {
		d := e.AngleDelta()
		v.scroll -= float64(d.Y()) * 0.6
		d.Delete()
		v.scroll = clamp(v.scroll, -float64(v.Height())/2, math.Max(0, v.totalH-float64(v.Height())/2))
		v.scrollTo = v.scroll
		v.manualTill = time.Now().Add(3 * time.Second)
		v.Update()
	})
	v.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		h := v.lineAt(float64(e.Y()))
		if h != v.hoverLine {
			v.hoverLine = h
			if h >= 0 && v.lyr != nil && v.lyr.Sync != core.SyncNone {
				handCursor(v.QWidget)
			} else {
				v.UnsetCursor()
			}
			v.Update()
		}
	})
	v.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { v.hoverLine = -1; v.Update() })
	v.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if i := v.lineAt(float64(e.Y())); i >= 0 && v.lyr != nil && v.lyr.Sync != core.SyncNone && v.OnSeek != nil {
			v.manualTill = time.Time{}
			v.OnSeek(v.lyr.Lines[v.lay[i].idx].Start)
		}
	})
	u.style.OnChange(func() { v.layKey = ""; v.Update() })
	return v
}

// SetLyrics shows a lyric (nil + msg shows a message instead).
func (v *LyricsView) SetLyrics(l *core.Lyrics, msg string) {
	v.lyr, v.msg = l, msg
	v.layKey = ""
	v.active = -1
	v.scroll, v.scrollTo = -float64(v.Height()) * 0.2, 0
	v.manualTill = time.Time{}
	v.Update()
}

// SetRunning starts/stops the 60 fps animation (only while playing).
func (v *LyricsView) SetRunning(on bool) {
	v.running = on
	if on {
		v.u.anim.Ticker(v, func(time.Time) bool {
			if v.IsVisible() {
				v.Update()
			}
			return v.running
		})
	}
	v.Update()
}

func (v *LyricsView) readStyle() {
	p := v.u.style.For(v.QWidget, "LyricsView")
	white := theme.Color{R: 255, G: 255, B: 255, A: 1}
	s := lyrStyle{
		color:         p.Color("color", withAlpha(white, .3)),
		active:        p.Color("active-color", white),
		glow:          p.Color("glow-color", withAlpha(white, .5)),
		dot:           p.Color("dot-color", white),
		glowR:         p.Num("glow-radius", 0),
		lift:          p.Len("word-lift", 0),
		soft:          p.Len("fill-softness", 10),
		activeScale:   p.Num("active-scale", 1),
		inactiveScale: p.Num("inactive-scale", 1),
		focus:         p.Num("focus-position", .35),
		fade:          p.Num("distance-fade", .15),
		pad:           p.Len("padding", 24),
		spacing:       p.Len("line-spacing", 16),
		bgScale:       p.Num("bg-vocal-scale", .65),
		align:         p.Str("align", "left"),
		oppAlign:      p.Str("opposite-align", "right"),
		fill:          p.Str("word-fill", "gradient"),
		scrollEase:    p.Str("scroll-easing", "out-cubic"),
		scrollDur:     p.Ms("scroll-duration", 550),
		scaleDur:      p.Ms("scale-duration", 400),
		dots:          p.Bool("interlude-dots", true),
		lineFill:      p.Bool("line-sync-fill", true),
		family:        p.Str("font-family", ""),
		size:          p.Len("font-size", 28),
		weight:        int(p.Num("font-weight", 700)),
	}
	s.past = p.Color("past-color", s.color)
	s.upcoming = p.Color("upcoming-color", s.color)
	s.bgVocal = p.Color("bg-vocal-color", s.color)
	s.credit = p.Color("credit-color", s.color)
	s.hoverC = p.Color("hover-color", s.active)
	s.empty = p.Color("empty-color", s.color)
	v.st = s
}

func (v *LyricsView) relayout() {
	key := ""
	if v.lyr != nil {
		key = v.lyr.Provider
	}
	key += "|" + itoa(v.Width())
	if key == v.layKey && v.fonts != nil {
		return
	}
	v.layKey = key
	v.readStyle()
	v.fonts.delete()
	s := v.st
	f := &lyrFonts{main: font(s.family, s.size, s.weight), bg: font(s.family, s.size*s.bgScale, s.weight)}
	f.mainFM = qt.NewQFontMetricsF(f.main)
	f.bgFM = qt.NewQFontMetricsF(f.bg)
	f.rowH, f.ascent = f.mainFM.Height()*1.08, f.mainFM.Ascent()
	f.bgRowH, f.bgAsc = f.bgFM.Height()*1.05, f.bgFM.Ascent()
	v.fonts = f

	v.lay = nil
	if v.lyr == nil {
		return
	}
	maxW := float64(v.Width()) - 2*s.pad
	if maxW < 60 {
		maxW = 60
	}
	space := f.mainFM.HorizontalAdvance(" ")
	bgSpace := f.bgFM.HorizontalAdvance(" ")
	y := 0.0
	lines := v.lyr.Lines
	for i, ln := range lines {
		ll := layLine{idx: i, align: s.align, timed: len(ln.Words) > 0}
		if ln.Opposite {
			ll.align = s.oppAlign
		}
		// interlude before this line?
		var prevEnd int64
		if i > 0 {
			prevEnd = lines[i-1].End
		}
		if s.dots && v.lyr.Sync != core.SyncNone && ln.Start-prevEnd > 5000 {
			ll.interlude = true
		}
		words := ln.Words
		if len(words) == 0 { // line or plain: split text so it wraps
			for j, t := range strings.Fields(ln.Text) {
				words = append(words, core.Word{Text: t, Start: ln.Start, End: ln.End, Part: false})
				_ = j
			}
		}
		ll.words, ll.rows, ll.rowW = wrapWords(words, f.mainFM, space, maxW)
		if len(ln.Background) > 0 {
			ll.bg, ll.bgRows, ll.bgRowW = wrapWords(ln.Background, f.bgFM, bgSpace, maxW)
		}
		ll.h = float64(ll.rows)*f.rowH + float64(ll.bgRows)*f.bgRowH
		if ll.interlude {
			y += f.rowH * 0.9
		}
		ll.y = y
		y += ll.h + s.spacing
		v.lay = append(v.lay, ll)
	}
	v.totalH = y
	v.lineScale = make([]float64, len(v.lay))
	v.scaleFrom = make([]float64, len(v.lay))
	for i := range v.lineScale {
		v.lineScale[i] = s.inactiveScale
	}
}

func wrapWords(ws []core.Word, fm *qt.QFontMetricsF, space, maxW float64) ([]layWord, int, []float64) {
	var out []layWord
	var rowW []float64
	x, row := 0.0, 0
	for i, w := range ws {
		wd := fm.HorizontalAdvance(w.Text)
		gap := 0.0
		if i > 0 && !w.Part {
			gap = space
		}
		if x > 0 && x+gap+wd > maxW && !w.Part {
			rowW = append(rowW, x)
			row++
			x, gap = 0, 0
		}
		x += gap
		out = append(out, layWord{w: w, x: x, wd: wd, row: row})
		x += wd
	}
	rowW = append(rowW, x)
	return out, row + 1, rowW
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

// activeIndex is the last line that started (−1 before the first).
func (v *LyricsView) activeIndex(pos int64) int {
	if v.lyr == nil || v.lyr.Sync == core.SyncNone {
		return -1
	}
	idx := -1
	for i, l := range v.lay {
		if v.lyr.Lines[l.idx].Start <= pos {
			idx = i
		} else {
			break
		}
	}
	return idx
}

func (v *LyricsView) lineAt(y float64) int {
	for i, l := range v.lay {
		top := l.y - v.scroll
		if y >= top && y <= top+l.h {
			return i
		}
	}
	return -1
}

func (v *LyricsView) updateScroll(pos int64, now time.Time) {
	idx := v.activeIndex(pos)
	if idx != v.active {
		// scale animation
		for i := range v.lineScale {
			v.scaleFrom[i] = v.lineScale[i]
		}
		v.scaleT0 = now
		v.active = idx
		// scroll animation
		target := -float64(v.Height()) * 0.15
		if idx >= 0 {
			target = v.lay[idx].y - v.st.focus*float64(v.Height()) + v.lay[idx].h/2
		}
		if now.After(v.manualTill) {
			v.scrollFrom, v.scrollTo, v.scrollT0 = v.scroll, target, now
		}
	}
	if v.lyr != nil && v.lyr.Sync == core.SyncNone {
		return
	}
	if now.After(v.manualTill) {
		t := clamp(float64(now.Sub(v.scrollT0))/float64(time.Duration(v.st.scrollDur)*time.Millisecond), 0, 1)
		v.scroll = lerp(v.scrollFrom, v.scrollTo, theme.Ease(v.st.scrollEase, t))
	}
	st := clamp(float64(now.Sub(v.scaleT0))/float64(time.Duration(math.Max(1, float64(v.st.scaleDur)))*time.Millisecond), 0, 1)
	e := theme.Ease("out-cubic", st)
	for i := range v.lineScale {
		target := v.st.inactiveScale
		if i == v.active {
			target = v.st.activeScale
		}
		v.lineScale[i] = lerp(v.scaleFrom[i], target, e)
	}
}

func (v *LyricsView) paint() {
	p := qt.NewQPainter2(v.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.SetRenderHint(qt.QPainter__TextAntialiasing)
	v.relayout()
	s := v.st
	W, H := float64(v.Width()), float64(v.Height())

	if v.lyr == nil || len(v.lay) == 0 {
		msg := v.msg
		if msg == "" {
			msg = "♪"
		}
		f := font(s.family, s.size*0.6, 600)
		p.SetFont(f)
		setPenColor(p, s.empty)
		r := rectF(s.pad, 0, W-2*s.pad, H)
		p.DrawText5(r, int(qt.AlignCenter)|int(qt.TextWordWrap), msg)
		r.Delete()
		f.Delete()
		return
	}
	now := time.Now()
	pos := v.Pos()
	v.updateScroll(pos, now)

	for i, l := range v.lay {
		top := l.y - v.scroll
		if top+l.h < -60 || top > H+60 {
			continue
		}
		line := v.lyr.Lines[l.idx]
		dist := math.Abs(float64(i - v.active))
		if v.active < 0 {
			dist = float64(i + 1)
		}
		opacity := clamp(1-s.fade*math.Max(0, dist-1), 0.12, 1)
		if v.lyr.Sync == core.SyncNone {
			opacity = 1
		}
		isActive := i == v.active && pos < line.End+400
		if l.interlude {
			v.paintDots(p, l, top, pos, i)
		}
		scale := v.lineScale[i]
		if v.lyr.Sync == core.SyncNone {
			scale = 1
		}
		p.Save()
		// scale around the line's anchor
		ax := s.pad
		switch l.align {
		case "center":
			ax = W / 2
		case "right":
			ax = W - s.pad
		}
		p.Translate2(ax, top)
		p.Scale(scale, scale)
		p.Translate2(-ax, 0)
		p.SetOpacity(opacity)
		base := s.color
		switch {
		case v.lyr.Sync == core.SyncNone:
			base = s.active
		case i < v.active || (i == v.active && !isActive):
			base = s.past
		}
		if i == v.hoverLine && v.lyr.Sync != core.SyncNone && !isActive {
			base = s.hoverC
		}
		v.paintWords(p, l.words, l.rowW, l.align, 0, v.fonts.main, v.fonts.rowH, v.fonts.ascent, W,
			line, isActive, base, l.timed)
		if len(l.bg) > 0 {
			bgBase := withAlpha(s.bgVocal, 0.7)
			v.paintWords(p, l.bg, l.bgRowW, l.align, float64(l.rows)*v.fonts.rowH, v.fonts.bg, v.fonts.bgRowH, v.fonts.bgAsc, W,
				line, isActive, bgBase, true)
		}
		p.Restore()
	}
	if v.lyr.Credit != "" {
		f := font(s.family, math.Max(11, s.size*0.36), 500)
		p.SetFont(f)
		setPenColor(p, s.credit)
		y := v.totalH - v.scroll + 12
		r := rectF(s.pad, y, W-2*s.pad, 60)
		flag := int(qt.AlignLeft)
		if s.align == "center" {
			flag = int(qt.AlignHCenter)
		}
		p.DrawText5(r, flag|int(qt.TextWordWrap), v.lyr.Credit+"\n"+v.lyr.Provider)
		r.Delete()
		f.Delete()
	}
}

// paintWords draws one block of words (main or background vocals).
func (v *LyricsView) paintWords(p *qt.QPainter, ws []layWord, rowW []float64, align string, yOff float64,
	f *qt.QFont, rowH, ascent, W float64, line core.Line, active bool, base theme.Color, timed bool) {
	s := v.st
	p.SetFont(f)
	pos := v.Pos()
	for _, w := range ws {
		x := s.pad + w.x
		switch align {
		case "center":
			x = (W-rowW[w.row])/2 + w.x
		case "right":
			x = W - s.pad - rowW[w.row] + w.x
		}
		y := yOff + float64(w.row)*rowH + ascent
		if !active || v.lyr.Sync == core.SyncNone {
			setPenColor(p, base)
			pt := qt.NewQPointF3(x, y)
			p.DrawText(pt, w.w.Text)
			pt.Delete()
			continue
		}
		// progress of this word (or of the whole line for line sync)
		var frac float64
		if timed {
			frac = progress(pos, w.w.Start, w.w.End)
		} else if s.lineFill {
			// line-synced: sweep across the line
			total := 0.0
			for _, rw := range rowW {
				total += rw
			}
			lp := progress(pos, line.Start, line.End)
			before := 0.0
			for r := 0; r < w.row; r++ {
				before += rowW[r]
			}
			frac = clamp((lp*total-before-w.x)/math.Max(1, w.wd), 0, 1)
		} else {
			frac = 1
		}
		lift := 0.0
		if s.lift > 0 && frac > 0 {
			lift = s.lift * theme.Ease("out-cubic", math.Min(1, frac*1.6))
		}
		y -= lift
		// glow behind sung words
		if s.glowR > 0 && frac > 0 && s.glow.A > 0 {
			gc := withAlpha(s.glow, 0.22*math.Min(1, frac*2))
			setPenColor(p, gc)
			for k := 0; k < 8; k++ {
				a := float64(k) * math.Pi / 4
				pt := qt.NewQPointF3(x+math.Cos(a)*s.glowR, y+math.Sin(a)*s.glowR)
				p.DrawText(pt, w.w.Text)
				pt.Delete()
			}
		}
		pt := qt.NewQPointF3(x, y)
		switch {
		case frac <= 0:
			setPenColor(p, s.upcoming)
		case frac >= 1:
			setPenColor(p, s.active)
		case s.fill == "solid":
			setPenColor(p, s.active)
		default:
			edge := x + frac*w.wd
			soft := math.Max(1, s.soft)
			g := qt.NewQLinearGradient3(edge-soft/2, 0, edge+soft/2, 0)
			c1, c2 := qc(s.active), qc(s.upcoming)
			g.SetColorAt(0, c1)
			g.SetColorAt(1, c2)
			br := qt.NewQBrush10(g.QGradient)
			pen := qt.NewQPen4(br, 1)
			p.SetPenWithPen(pen)
			pen.Delete()
			br.Delete()
			c1.Delete()
			c2.Delete()
			g.Delete()
		}
		p.DrawText(pt, w.w.Text)
		pt.Delete()
	}
}

func progress(pos, start, end int64) float64 {
	if end <= start {
		if pos >= start {
			return 1
		}
		return 0
	}
	return clamp(float64(pos-start)/float64(end-start), 0, 1)
}

// paintDots draws the three "breathing" dots during an instrumental gap.
func (v *LyricsView) paintDots(p *qt.QPainter, l layLine, top float64, pos int64, i int) {
	line := v.lyr.Lines[l.idx]
	var gapStart int64
	if l.idx > 0 {
		gapStart = v.lyr.Lines[l.idx-1].End
	}
	if pos < gapStart || pos >= line.Start {
		return
	}
	s := v.st
	frac := progress(pos, gapStart, line.Start)
	r := v.fonts.rowH * 0.12
	y := top - v.fonts.rowH*0.55
	x0 := s.pad + r
	switch l.align {
	case "center":
		x0 = float64(v.Width())/2 - 3*r
	case "right":
		x0 = float64(v.Width()) - s.pad - 6*r
	}
	breathe := 1 + 0.12*math.Sin(float64(time.Now().UnixMilli())/260)
	for k := 0; k < 3; k++ {
		f := clamp(frac*3-float64(k), 0, 1)
		c := withAlpha(s.dot, 0.25+0.75*f)
		rr := r * breathe * (0.8 + 0.2*f)
		fillRounded(p, x0+float64(k)*r*3-rr, y-rr, 2*rr, 2*rr, rr, c)
	}
}
