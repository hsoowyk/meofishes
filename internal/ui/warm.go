package ui

import (
	"context"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/core"
	"meofishes/internal/plugin"
)

// Warming up: YouTube songs need yt-dlp to find their stream (10–20 s on a
// slow computer, most of it solving YouTube's JavaScript challenge). The
// answers are cached (plugin.Cache), so asking early — when the mouse rests
// on a song, for the first search result — makes the click start at once.

// hoverWarm warms a song the mouse has rested on for a moment.
func (u *UI) hoverWarm(t core.Track) {
	if u.warmTimer == nil {
		u.warmTimer = qt.NewQTimer()
		u.warmTimer.SetSingleShot(true)
		u.warmTimer.OnTimeout(func() { u.warm(u.warmCand) })
	}
	u.warmCand = t
	u.warmTimer.Start(350)
}

// hoverLeave cancels a warm-up that has not started yet.
func (u *UI) hoverLeave(t core.Track) {
	if u.warmTimer != nil && u.warmCand.Key() == t.Key() {
		u.warmTimer.Stop()
	}
}

// warm resolves a song in the background (one at a time, newest first).
func (u *UI) warm(t core.Track) {
	if t.Title == "" || !u.cfg.Prefs.Prefetch {
		return
	}
	if _, ok := u.core.Offline.Lookup(t); ok {
		return
	}
	if at, ok := u.warmed[t.Key()]; ok && time.Since(at) < time.Hour {
		return
	}
	u.warmNext = &t
	if u.warmBusy {
		return
	}
	u.warmBusy = true
	go u.warmWorker()
}

func (u *UI) warmWorker() {
	for {
		next := mainthread.Wait2(func() *core.Track {
			if u.warmNext == nil {
				u.warmBusy = false
				return nil
			}
			t := *u.warmNext
			u.warmNext = nil
			u.warmed[t.Key()] = time.Now()
			return &t
		})
		if next == nil {
			return
		}
		t := *next
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t0 := time.Now()
		st, err := u.core.Plugins.Resolve(ctx, t)
		if err == nil && st.Page && plugin.YtdlpPath() != "" && u.cfg.Prefs.DirectYtdlp {
			_, err = plugin.YtdlpStreamURL(ctx, st.URL)
		}
		cancel()
		if err != nil {
			u.core.Log("warm", "%s: %v", t.String(), err)
			mainthread.Start(func() { delete(u.warmed, t.Key()) })
		} else {
			u.core.Log("warm", "%s ready in %v", t.String(), time.Since(t0).Round(time.Millisecond))
		}
	}
}
