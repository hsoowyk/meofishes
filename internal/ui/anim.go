package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// Animator drives every animation from one ~60 fps timer that only runs
// while something is moving.
type Animator struct {
	timer   *qt.QTimer
	tweens  map[any]*tween
	tickers map[any]func(now time.Time) bool
	// Speed multiplies tween durations (Settings → Animations); 0 = instant.
	Speed func() float64
	// OnFrame is called once per frame (developer mode FPS meter).
	OnFrame func(now time.Time)
	// Hz is the frame rate (the screen's refresh rate).
	Hz float64
}

// Active is the number of running tweens and tickers (developer mode).
func (a *Animator) Active() (tweens, tickers int) { return len(a.tweens), len(a.tickers) }

type tween struct {
	start    time.Time
	delay    time.Duration
	dur      time.Duration
	ease     string
	step     func(t float64)
	done     func()
	finished bool
}

func NewAnimator() *Animator {
	a := &Animator{tweens: map[any]*tween{}, tickers: map[any]func(time.Time) bool{}}
	a.timer = qt.NewQTimer()
	a.timer.SetTimerType(qt.PreciseTimer)
	a.SetRefreshRate(60)
	a.timer.OnTimeout(a.tick)
	return a
}

// SetRefreshRate makes frames match the screen (60, 75, 144 Hz…).
func (a *Animator) SetRefreshRate(hz float64) {
	if hz < 20 || hz > 500 {
		hz = 60
	}
	a.Hz = hz
	a.timer.SetInterval(max(1, int(1000/hz+0.5)))
}

// FollowScreen tracks the refresh rate of the screen the window is on.
func (a *Animator) FollowScreen(w *qt.QWidget) {
	if s := qt.QGuiApplication_PrimaryScreen(); s != nil {
		a.SetRefreshRate(s.RefreshRate())
	}
	if wh := w.WindowHandle(); wh != nil {
		if s := wh.Screen(); s != nil {
			a.SetRefreshRate(s.RefreshRate())
		}
		wh.OnScreenChanged(func(s *qt.QScreen) {
			if s != nil {
				a.SetRefreshRate(s.RefreshRate())
			}
		})
	}
}

func (a *Animator) wake() {
	if !a.timer.IsActive() {
		a.timer.Start2()
	}
}

// Tween runs step(eased t) for dur. key replaces any running tween with the
// same key (step is not called again for the old one).
func (a *Animator) Tween(key any, dur, delay time.Duration, ease string, step func(t float64), done func()) {
	if a.Speed != nil {
		k := a.Speed()
		if k <= 0.01 {
			dur, delay = 0, 0
		} else {
			dur = time.Duration(float64(dur) / k)
			delay = time.Duration(float64(delay) / k)
		}
	}
	if dur <= 0 {
		step(1)
		if done != nil {
			done()
		}
		delete(a.tweens, key)
		return
	}
	a.tweens[key] = &tween{start: time.Now(), delay: delay, dur: dur, ease: ease, step: step, done: done}
	a.wake()
}

// Ticker calls f every frame until it returns false.
func (a *Animator) Ticker(key any, f func(now time.Time) bool) {
	a.tickers[key] = f
	a.wake()
}

func (a *Animator) StopTicker(key any) { delete(a.tickers, key) }

// Cancel drops tweens and tickers by key without running them again.
func (a *Animator) Cancel(keys ...any) {
	for _, k := range keys {
		delete(a.tweens, k)
		delete(a.tickers, k)
	}
}

func (a *Animator) tick() {
	now := time.Now()
	if a.OnFrame != nil {
		a.OnFrame(now)
	}
	for k, tw := range a.tweens {
		el := now.Sub(tw.start) - tw.delay
		if el < 0 {
			tw.step(0)
			continue
		}
		t := float64(el) / float64(tw.dur)
		if t >= 1 {
			t = 1
		}
		tw.step(theme.Ease(tw.ease, t))
		if t >= 1 {
			delete(a.tweens, k)
			if tw.done != nil {
				tw.done()
			}
		}
	}
	for k, f := range a.tickers {
		if !f(now) {
			delete(a.tickers, k)
		}
	}
	if len(a.tweens) == 0 && len(a.tickers) == 0 {
		a.timer.Stop()
	}
}

// singleShot runs f once after ms on the UI thread.
func singleShot(ms int, f func()) {
	t := qt.NewQTimer()
	t.SetSingleShot(true)
	t.OnTimeout(func() {
		f()
		t.DeleteLater()
	})
	t.Start(ms)
}

// ---------------------------------------------------------------- enter animations

// Enter plays the -meo-enter animation of a widget (slide-up, slide-down,
// slide-left, slide-right, none). Widgets are only moved, never faded with a
// QGraphicsEffect: effects render the whole subtree off-screen every frame
// (very slow) and nesting them crashes Qt. "fade" behaves like "none".
func (u *UI) Enter(w *qt.QWidget, kind string, index int) {
	p := u.style.For(w, kind)
	mode := p.Str("enter", "none")
	if mode == "none" || mode == "fade" || !w.IsVisible() {
		return
	}
	dur := time.Duration(p.Ms("enter-duration", 400)) * time.Millisecond
	delay := time.Duration(p.Ms("enter-delay", 0)+index*p.Ms("enter-stagger", 0)) * time.Millisecond
	if delay > 600*time.Millisecond {
		delay = 600 * time.Millisecond
	}
	ease := p.Str("enter-easing", "out-cubic")
	dist := p.Len("enter-distance", 24)
	u.animateIn(w, mode, dur, delay, ease, dist, nil)
}

// animateIn slides w from an offset back to the position its layout gave it.
// The tween is keyed by the widget pointer: whoever deletes w must cancel it
// (deco.dispose does) — use slideKey(w).
func (u *UI) animateIn(w *qt.QWidget, mode string, dur, delay time.Duration, ease string, dist float64, done func()) {
	dx, dy := 0.0, 0.0
	switch mode {
	case "slide-up":
		dy = dist
	case "slide-down":
		dy = -dist
	case "slide-left":
		dx = dist
	case "slide-right":
		dx = -dist
	}
	if dx == 0 && dy == 0 {
		if done != nil {
			done()
		}
		return
	}
	pos := w.Pos()
	x0, y0 := pos.X(), pos.Y()
	w.Move(x0+int(dx), y0+int(dy))
	u.anim.Tween(slideKey(w), dur, delay, ease, func(t float64) {
		w.Move(x0+int(dx*(1-t)), y0+int(dy*(1-t)))
	}, func() {
		w.Move(x0, y0)
		if done != nil {
			done()
		}
	})
}

func slideKey(w *qt.QWidget) any { return w.UnsafePointer() }

// ---------------------------------------------------------------- page transitions

// Transition animates a page change cheaply: a snapshot of the old page is
// painted on top of the new one and fades/slides away (one pixmap per frame,
// instead of re-rendering the page through an effect).
type Transition struct {
	*qt.QWidget
	u    *UI
	px   *qt.QPixmap
	t    float64
	mode string
	dist float64
}

func (u *UI) newTransition(parent *qt.QWidget) *Transition {
	tr := &Transition{QWidget: qt.NewQWidget(parent), u: u}
	tr.SetObjectName("PageTransition")
	tr.SetAttribute(qt.WA_TransparentForMouseEvents)
	tr.Hide()
	tr.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		if tr.px == nil {
			return
		}
		p := qt.NewQPainter2(tr.QPaintDevice)
		k := 1 - tr.t
		dx, dy := 0, 0
		switch tr.mode {
		case "slide-up":
			dy = -int(tr.dist * tr.t)
		case "slide-left":
			dx = -int(tr.dist * tr.t)
		case "slide-right":
			dx = int(tr.dist * tr.t)
		case "zoom":
			k = 1 - tr.t
		}
		p.SetOpacity(k)
		if tr.mode == "zoom" {
			s := 1 + 0.06*tr.t
			w, h := float64(tr.Width()), float64(tr.Height())
			r := rectF(w*(1-s)/2, h*(1-s)/2, w*s, h*s)
			sr := rectF(0, 0, float64(tr.px.Width()), float64(tr.px.Height()))
			p.DrawPixmap(r, tr.px, sr)
			r.Delete()
			sr.Delete()
		} else {
			p.DrawPixmap9(dx, dy, tr.px)
		}
		p.End()
		p.Delete()
	})
	return tr
}

// Run snapshots old, then animates the snapshot away over the new page.
func (tr *Transition) Run(old *qt.QWidget, mode string, dur time.Duration, ease string, dist float64) {
	if old == nil || mode == "none" || dur <= 0 {
		return
	}
	if tr.px != nil {
		tr.px.Delete()
		tr.px = nil
	}
	snap := old.Grab()
	if snap == nil || snap.IsNull() {
		return
	}
	tr.px = qt.NewQPixmap5(snap)
	tr.mode, tr.dist, tr.t = mode, dist, 0
	tr.SetGeometry(old.X(), old.Y(), old.Width(), old.Height())
	tr.Show()
	tr.Raise()
	tr.u.anim.Tween(tr, dur, 0, ease, func(t float64) {
		tr.t = t
		tr.Update()
	}, func() {
		tr.Hide()
		if tr.px != nil {
			tr.px.Delete()
			tr.px = nil
		}
	})
}
