package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// Tour is the optional tutorial: it dims the window, highlights one part of
// the interface at a time and explains it in a bubble.
// Props: #Tour { -meo-dim-color, -meo-spot-color, -meo-spot-radius }.
type Tour struct {
	*qt.QWidget
	u      *UI
	steps  []tourStep
	i      int
	bubble *qt.QFrame
	title  *qt.QLabel
	text   *qt.QLabel
	count  *qt.QLabel
	back   *qt.QPushButton
	next   *qt.QPushButton
	skip   *qt.QPushButton
	spot   [4]float64 // animated highlight rect (root coordinates)
	ask    bool       // first step asks whether to take the tour
}

type tourStep struct {
	target func() *qt.QWidget // nil = centered bubble, no highlight
	title  string
	text   string
	before func()
}

func (u *UI) tourSteps() []tourStep {
	return []tourStep{
		{nil, tr("Welcome to meofishes!"), tr("Everything you hear comes from plugins: search, playback, lyrics and covers. Want a quick tour? It takes less than a minute."), nil},
		{func() *qt.QWidget { return u.logoW }, tr("The menu"), tr("Click the logo to open every tab of the app: Home, Search, Library, Queue and Settings. Double-click it to go home. The arrow hides the menu again."), func() { u.setMenuOpen(true) }},
		{func() *qt.QWidget { return u.home.box.QWidget }, tr("Search"), tr("Type here and press Enter to search every music source. Right-click any song for more: play next, add to a playlist, start a radio…"), func() { u.goPage("home") }},
		{func() *qt.QWidget { return u.libNav }, tr("Your library"), tr("Liked songs and your playlists live here. Use + to create one, or import a .mepw / .m3u file. Playlists can be exported as .mepw to share."), nil},
		{func() *qt.QWidget { return u.nowCard }, tr("The player"), tr("The song playing now. Click the cover (or press Ctrl+L) to open the fullscreen player with lyrics. Scroll on the speaker icon to change the volume."), nil},
		{func() *qt.QWidget { return u.curLyric.QWidget }, tr("The current lyric"), tr("The line being sung shows up here and swipes to the next one. Songs without lyrics get a frequency visualizer in the fullscreen player."), nil},
		{func() *qt.QWidget { return u.nav["settings"].QWidget }, tr("Settings"), tr("Themes, plugins, languages, lyrics animations, audio and much more. Almost everything in meofishes can be changed there."), func() { u.setMenuOpen(true) }},
		{nil, tr("That's it!"), tr("Handy shortcuts: Ctrl+Space play/pause · Ctrl+L lyrics · Ctrl+F search · Ctrl+D like. You can replay this tour in Settings → About."), nil},
	}
}

// startTour shows the tutorial. ask=true starts with "show / skip".
func (u *UI) startTour(ask bool) {
	u.cfg.Prefs.TutorialAsked = true
	_ = u.cfg.Save()
	u.closeFull()
	if u.tour == nil {
		u.tour = u.newTour()
	}
	t := u.tour
	t.ask = ask
	t.steps = u.tourSteps()
	t.i = 0
	t.SetGeometry(0, 0, u.root.Width(), u.root.Height())
	t.Show()
	t.Raise()
	t.show(0)
}

func (u *UI) newTour() *Tour {
	t := &Tour{QWidget: qt.NewQWidget(u.root.QWidget), u: u}
	t.SetObjectName("Tour")
	t.Hide()
	t.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) { t.paint() })
	t.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {}) // block clicks
	u.root.onResize = append(u.root.onResize, func() {
		if t.IsVisible() {
			t.SetGeometry(0, 0, u.root.Width(), u.root.Height())
			t.show(t.i)
		}
	})

	t.bubble = qt.NewQFrame(t.QWidget)
	t.bubble.SetObjectName("TourBubble")
	t.bubble.SetAttribute(qt.WA_StyledBackground)
	t.bubble.SetFixedWidth(360)
	b := vbox(t.bubble.QWidget, []int{20, 18, 20, 16}, 8)
	t.title = label("", "TourTitle", "")
	t.text = label("", "TourText", "")
	t.text.SetWordWrap(true)
	b.AddWidget(t.title.QWidget)
	b.AddWidget(t.text.QWidget)
	row := qt.NewQHBoxLayout2()
	t.count = label("", "TourCount", "hint")
	row.AddWidget2(t.count.QWidget, 1)
	t.skip = qt.NewQPushButton3(tr("Skip"))
	t.back = qt.NewQPushButton3(tr("Back"))
	t.next = qt.NewQPushButton3(tr("Next"))
	role(t.next.QWidget, "primary")
	t.skip.OnClicked(t.finish)
	t.back.OnClicked(func() { t.show(t.i - 1) })
	t.next.OnClicked(func() {
		if t.i+1 >= len(t.steps) {
			t.finish()
			return
		}
		t.show(t.i + 1)
	})
	row.AddWidget(t.skip.QWidget)
	row.AddWidget(t.back.QWidget)
	row.AddWidget(t.next.QWidget)
	b.AddLayout(row.QLayout)
	return t
}

func (t *Tour) finish() {
	t.Hide()
	t.u.setMenuOpen(false)
}

// show moves to step i: runs its action, then animates the highlight.
func (t *Tour) show(i int) {
	if i < 0 || i >= len(t.steps) {
		return
	}
	t.i = i
	st := t.steps[i]
	if st.before != nil {
		st.before()
	}
	t.title.SetText(st.title)
	t.text.SetText(st.text)
	t.count.SetText(itoa(i+1) + " / " + itoa(len(t.steps)))
	t.back.SetVisible(i > 0)
	switch {
	case i == 0 && t.ask:
		t.next.SetText(tr("Show me"))
		t.skip.SetText(tr("No, thanks"))
	case i == len(t.steps)-1:
		t.next.SetText(tr("Done"))
		t.skip.SetVisible(false)
	default:
		t.next.SetText(tr("Next"))
		t.skip.SetText(tr("Skip"))
		t.skip.SetVisible(true)
	}
	// wait for menus/pages to settle before measuring the target
	singleShot(280, func() { t.place(st) })
}

func (t *Tour) place(st tourStep) {
	W, H := float64(t.Width()), float64(t.Height())
	to := [4]float64{W / 2, H / 2, 0, 0}
	if st.target != nil {
		if w := st.target(); w != nil && w.IsVisible() {
			origin := qt.NewQPoint2(0, 0)
			p := w.MapTo(t.u.root.QWidget, origin)
			origin.Delete()
			pad := 8.0
			to = [4]float64{float64(p.X()) - pad, float64(p.Y()) - pad, float64(w.Width()) + 2*pad, float64(w.Height()) + 2*pad}
		}
	}
	from := t.spot
	if from[2] == 0 && from[3] == 0 && to[2] > 0 {
		from = [4]float64{to[0] + to[2]/2, to[1] + to[3]/2, 0, 0}
	}
	t.u.anim.Tween(t, 420*time.Millisecond, 0, "out-cubic", func(k float64) {
		for j := range t.spot {
			t.spot[j] = lerp(from[j], to[j], k)
		}
		t.Update()
	}, nil)
	// bubble: next to the highlight, or centered
	t.bubble.AdjustSize()
	bw, bh := float64(t.bubble.Width()), float64(t.bubble.Height())
	x, y := (W-bw)/2, (H-bh)/2
	if to[2] > 0 {
		x = to[0] + to[2] + 18
		y = to[1]
		if x+bw > W-16 {
			x = to[0] - bw - 18
		}
		if x < 16 {
			x = clamp(to[0], 16, W-bw-16)
			y = to[1] + to[3] + 14
			if y+bh > H-16 {
				y = to[1] - bh - 14
			}
		}
		y = clamp(y, 16, H-bh-16)
	}
	t.bubble.Move(int(x), int(y))
	t.bubble.Raise()
	t.u.Enter(t.bubble.QWidget, "QFrame", 0)
}

func (t *Tour) paint() {
	p := qt.NewQPainter2(t.QPaintDevice)
	defer p.Delete()
	defer p.End()
	p.SetRenderHint(qt.QPainter__Antialiasing)
	pr := t.u.style.For(t.QWidget, "Tour")
	W, H := float64(t.Width()), float64(t.Height())
	all := qt.NewQPainterPath()
	all.AddRect2(0, 0, W, H)
	if t.spot[2] > 1 {
		r := pr.Len("spot-radius", 14)
		hole := qt.NewQPainterPath()
		hole.AddRoundedRect2(t.spot[0], t.spot[1], t.spot[2], t.spot[3], r, r)
		cut := all.Subtracted(hole)
		col := qc(pr.Color("dim-color", theme.Color{A: 0.62}))
		br := qt.NewQBrush3(col)
		p.FillPath(cut, br)
		br.Delete()
		col.Delete()
		ring := qc(pr.Color("spot-color", theme.Color{R: 244, G: 180, B: 198, A: 0.9}))
		rb := qt.NewQBrush3(ring)
		pen := qt.NewQPen4(rb, 2)
		p.SetPenWithPen(pen)
		p.SetBrushWithStyle(qt.NoBrush)
		p.DrawPath(hole)
		pen.Delete()
		rb.Delete()
		ring.Delete()
		hole.Delete()
	} else {
		col := qc(pr.Color("dim-color", theme.Color{A: 0.62}))
		br := qt.NewQBrush3(col)
		p.FillPath(all, br)
		br.Delete()
		col.Delete()
	}
	all.Delete()
}
