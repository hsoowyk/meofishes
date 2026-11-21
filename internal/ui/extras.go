package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/mpris"
	"meofishes/internal/player"
)

// ---------------------------------------------------------------- equalizer & speed

// eqGains returns the equalizer gains, or nil when it is off.
func (u *UI) eqGains() []float64 {
	pr := u.cfg.Prefs
	if !pr.EQOn {
		return nil
	}
	if len(pr.EQ) == len(player.EQBands) {
		return pr.EQ
	}
	if g, ok := player.EQPresets[pr.EQPreset]; ok {
		return g
	}
	return nil
}

// applyFilters updates normalization and equalizer while playing.
func (u *UI) applyFilters() {
	if u.mpv == nil {
		return
	}
	if err := u.mpv.SetFilters(player.Filters(u.cfg.Prefs.Normalize, u.eqGains())); err != nil {
		u.toast(tr("The equalizer could not be applied: {error}", "error", err.Error()))
	}
}

// setSpeed changes the playback speed (0.25–4, pitch kept).
func (u *UI) setSpeed(s float64) {
	s = clamp(s, 0.25, 4)
	u.cfg.Prefs.Speed = s
	_ = u.cfg.Save()
	if u.mpv != nil {
		_ = u.mpv.SetSpeed(s)
	}
	u.syncMPRIS()
}

// ---------------------------------------------------------------- sleep timer

// setSleep stops playback after minutes (0 = off, -1 = at the end of the song).
// The volume fades out during the last 15 seconds.
func (u *UI) setSleep(minutes int) {
	if u.sleepTimer == nil {
		u.sleepTimer = qt.NewQTimer()
		u.sleepTimer.SetInterval(1000)
		u.sleepTimer.OnTimeout(u.sleepTick)
	}
	u.sleepAt = time.Time{}
	u.cfg.Prefs.StopAfterTrack = false
	switch {
	case minutes == 0:
		u.sleepTimer.Stop()
		u.restoreSleepVolume()
		u.toast(tr("Sleep timer off"))
		return
	case minutes < 0:
		u.cfg.Prefs.StopAfterTrack = true
		u.toast(tr("Stopping at the end of this song"))
		return
	}
	u.sleepAt = time.Now().Add(time.Duration(minutes) * time.Minute)
	u.sleepVol = u.cfg.Volume
	u.sleepTimer.Start2()
	u.toast(tr("Stopping in {n} min", "n", itoa(minutes)))
}

func (u *UI) sleepTick() {
	if u.sleepAt.IsZero() {
		u.sleepTimer.Stop()
		return
	}
	left := time.Until(u.sleepAt)
	if left <= 0 {
		u.sleepTimer.Stop()
		u.sleepAt = time.Time{}
		if u.player != nil {
			_ = u.player.SetPaused(true)
		}
		u.restoreSleepVolume()
		u.toast(tr("Sleep timer: paused"))
		return
	}
	if left < 15*time.Second && u.player != nil { // fade out
		_ = u.player.SetVolume(u.sleepVol * left.Seconds() / 15)
	}
}

func (u *UI) restoreSleepVolume() {
	if u.player != nil && u.sleepVol > 0 {
		_ = u.player.SetVolume(u.sleepVol)
	}
	u.sleepVol = 0
}

// sleepLeft describes the timer ("" = off).
func (u *UI) sleepLeft() string {
	if u.cfg.Prefs.StopAfterTrack {
		return tr("end of the song")
	}
	if u.sleepAt.IsZero() {
		return ""
	}
	return core_FormatDur(time.Until(u.sleepAt))
}

func core_FormatDur(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// sleepMenu fills a menu with the timer choices.
func (u *UI) sleepMenu(m *qt.QMenu) {
	if left := u.sleepLeft(); left != "" {
		a := m.AddAction(tr("Sleep timer: {left}", "left", left))
		a.SetEnabled(false)
		off := m.AddAction(tr("Turn off"))
		off.OnTriggered(func() { u.setSleep(0) })
		m.AddSeparator()
	}
	for _, n := range []int{5, 15, 30, 45, 60, 90} {
		n := n
		a := m.AddAction(tr("{n} minutes", "n", itoa(n)))
		a.OnTriggered(func() { u.setSleep(n) })
	}
	end := m.AddAction(tr("End of this song"))
	end.OnTriggered(func() { u.setSleep(-1) })
}

// ---------------------------------------------------------------- MPRIS

// mprisPlayer adapts the UI to MPRIS (calls come from D-Bus goroutines).
type mprisPlayer struct{ u *UI }

func (m mprisPlayer) do(f func()) { mainthread.Start(f) }
func (m mprisPlayer) PlayPause()  { m.do(m.u.TogglePlay) }
func (m mprisPlayer) Play() {
	m.do(func() {
		if m.u.state().Paused || m.u.state().Idle {
			m.u.TogglePlay()
		}
	})
}
func (m mprisPlayer) Pause() {
	m.do(func() {
		if m.u.player != nil {
			_ = m.u.player.SetPaused(true)
		}
	})
}
func (m mprisPlayer) Stop() {
	m.do(func() {
		if m.u.player != nil {
			_ = m.u.player.Stop()
		}
	})
}
func (m mprisPlayer) Next()     { m.do(func() { m.u.Next(true) }) }
func (m mprisPlayer) Previous() { m.do(m.u.Prev) }
func (m mprisPlayer) SeekBy(ms int64) {
	m.do(func() { m.u.SeekTo(m.u.state().PositionMs + ms) })
}
func (m mprisPlayer) SeekTo(ms int64)     { m.do(func() { m.u.SeekTo(ms) }) }
func (m mprisPlayer) SetVolume(v float64) { m.do(func() { m.u.setVolume(v * 100) }) }
func (m mprisPlayer) SetShuffle(on bool) {
	m.do(func() {
		if on != m.u.cfg.Shuffle {
			m.u.toggleShuffle()
		}
	})
}
func (m mprisPlayer) SetLoop(mode string) {
	m.do(func() {
		m.u.cfg.Repeat = map[string]string{"None": "off", "Track": "one", "Playlist": "all"}[mode]
		m.u.syncRepeat()
	})
}
func (m mprisPlayer) SetRate(r float64) { m.do(func() { m.u.setSpeed(r) }) }
func (m mprisPlayer) Raise() {
	m.do(func() { m.u.toggleWindowShown(); m.u.win.Raise(); m.u.win.ActivateWindow() })
}
func (m mprisPlayer) Quit() { m.do(m.u.quit) }

func (u *UI) startMPRIS() {
	if !u.cfg.Prefs.MPRIS || u.mpris != nil {
		return
	}
	s, err := mpris.Start(mprisPlayer{u})
	if err != nil {
		u.core.Log("mpris", "%v", err)
		return
	}
	u.mpris = s
	u.syncMPRIS()
}

func (u *UI) stopMPRIS() {
	if u.mpris != nil {
		u.mpris.Close()
		u.mpris = nil
	}
}

// syncMPRIS publishes the current state.
func (u *UI) syncMPRIS() {
	if u.mpris == nil {
		return
	}
	st := u.state()
	ms := mpris.State{Status: "Stopped", Volume: u.cfg.Volume / 100, Shuffle: u.cfg.Shuffle, Rate: u.cfg.Prefs.Speed,
		Loop: map[string]string{"one": "Track", "all": "Playlist"}[u.cfg.Repeat]}
	if ms.Loop == "" {
		ms.Loop = "None"
	}
	if ms.Rate <= 0 {
		ms.Rate = 1
	}
	if cur := u.current(); cur != nil {
		ms.Track = mpris.Track{Key: cur.Key(), Title: cur.Title, Artist: cur.Artist, Album: cur.Album,
			LengthMs: u.duration(), ArtURL: u.artURL(cur.Cover)}
		switch {
		case st.Idle:
			ms.Status = "Stopped"
		case st.Paused:
			ms.Status = "Paused"
		default:
			ms.Status = "Playing"
		}
	}
	u.mpris.Update(ms)
}

// artURL prefers the downloaded copy of a cover (file://) so every desktop
// widget can show it.
func (u *UI) artURL(cover string) string {
	if cover == "" {
		return ""
	}
	if strings.HasPrefix(cover, "/") {
		return "file://" + cover
	}
	if p := u.coverLoader.path(cover); fileExists(p) {
		return "file://" + p
	}
	return cover
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

// notifyTrack shows a desktop notification for the new song.
func (u *UI) notifyTrack() {
	cur := u.current()
	if !u.cfg.Prefs.Notify || cur == nil || u.mpris == nil || u.win.IsActiveWindow() {
		return
	}
	icon := "meofishes"
	if a := u.artURL(cur.Cover); strings.HasPrefix(a, "file://") {
		icon = strings.TrimPrefix(a, "file://")
	}
	body := cur.Artist
	if cur.Album != "" {
		body += " · " + cur.Album
	}
	go u.mpris.Notify(cur.Title, body, icon)
}
