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
}

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
	a.timer.SetInterval(15)
	a.timer.OnTimeout(a.tick)
	return a
}

func (a *Animator) wake() {
	if !a.timer.IsActive() {
		a.timer.Start2()
	}
}

// Tween runs step(eased t) for dur. key replaces any running tween with the
// same key (step is not called again for the old one).
func (a *Animator) Tween(key any, dur, delay time.Duration, ease string, step func(t float64), done func()) {
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

// Enter plays the -meo-enter animation of a widget (fade, slide-up,
// slide-down, slide-left, slide-right, zoom, none).
func (u *UI) Enter(w *qt.QWidget, kind string, index int) {
	p := u.style.For(w, kind)
	mode := p.Str("enter", "none")
	if mode == "none" || !w.IsVisible() {
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

// animateIn fades/slides w in. Sliding moves the widget from an offset back
// to the position its layout gave it.
func (u *UI) animateIn(w *qt.QWidget, mode string, dur, delay time.Duration, ease string, dist float64, done func()) {
	eff := qt.NewQGraphicsOpacityEffect2(w.QObject)
	eff.SetOpacity(0)
	w.SetGraphicsEffect(eff.QGraphicsEffect)
	pos := w.Pos()
	x0, y0 := pos.X(), pos.Y()
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
	key := w.UnsafePointer()
	u.anim.Tween(key, dur, delay, ease, func(t float64) {
		eff.SetOpacity(clamp(t, 0, 1))
		if dx != 0 || dy != 0 {
			w.Move(x0+int(dx*(1-t)), y0+int(dy*(1-t)))
		}
	}, func() {
		w.SetGraphicsEffect(nil)
		if dx != 0 || dy != 0 {
			w.Move(x0, y0)
		}
		if done != nil {
			done()
		}
	})
}
