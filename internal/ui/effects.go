package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
	"meofishes/internal/player"
)

// effects turns the preferences into the player's effects.
func (u *UI) effects() player.Effects {
	pr := u.cfg.Prefs
	return player.Effects{Karaoke: pr.Karaoke, Pitch: float64(pr.Pitch), Reverb: float64(pr.Reverb) / 100,
		Balance: float64(pr.Balance) / 100, Mono: pr.Mono, Spatial: pr.Spatial, Crossfeed: pr.Crossfeed,
		Noise: pr.Noise, NoiseVol: float64(pr.NoiseVol) / 100}
}

// soundMode is a named mix of speed, pitch and effects.
type soundMode struct {
	id, name string
	speed    float64
	vinyl    bool
	reverb   int
	spatial  bool
	karaoke  bool
	pitch    int
}

func soundModes() []soundMode {
	return []soundMode{
		{id: "normal", name: tr("Normal"), speed: 1},
		{id: "nightcore", name: tr("Nightcore (faster and higher)"), speed: 1.25, vinyl: true},
		{id: "slowed", name: tr("Slowed + reverb"), speed: 0.82, vinyl: true, reverb: 45},
		{id: "8d", name: tr("8D (turns around your head)"), speed: 1, spatial: true, reverb: 15},
		{id: "karaoke", name: tr("Karaoke (removes the voice)"), speed: 1, karaoke: true},
	}
}

// currentMode is the id of the mode the settings match ("" = custom).
func (u *UI) currentMode() string {
	pr := u.cfg.Prefs
	for _, m := range soundModes() {
		if pr.Speed == m.speed && pr.VinylPitch == m.vinyl && pr.Reverb == m.reverb && pr.Spatial == m.spatial &&
			pr.Karaoke == m.karaoke && pr.Pitch == m.pitch {
			return m.id
		}
	}
	return ""
}

func (u *UI) setMode(m soundMode) {
	pr := &u.cfg.Prefs
	pr.VinylPitch, pr.Reverb, pr.Spatial, pr.Karaoke, pr.Pitch = m.vinyl, m.reverb, m.spatial, m.karaoke, m.pitch
	u.applyFilters()
	u.setSpeed(m.speed)
	u.toast(tr("Sound: {mode}", "mode", m.name))
}

// effectsMenu fills a menu with the sound modes, ambient sounds and the A-B loop.
func (u *UI) effectsMenu(m *qt.QMenu) {
	cur := u.currentMode()
	for _, md := range soundModes() {
		md := md
		a := m.AddAction(md.name)
		a.SetCheckable(true)
		a.SetChecked(cur == md.id)
		a.OnTriggered(func() { u.setMode(md) })
	}
	m.AddSeparator()
	amb := m.AddMenuWithTitle(tr("Ambient sound"))
	for _, n := range [][2]string{{"", tr("None")}, {"brown", tr("Rain (brown noise)")}, {"pink", tr("Wind (pink noise)")}, {"white", tr("White noise")}} {
		n := n
		a := amb.AddAction(n[1])
		a.SetCheckable(true)
		a.SetChecked(u.cfg.Prefs.Noise == n[0])
		a.OnTriggered(func() { u.cfg.Prefs.Noise = n[0]; u.applyFilters() })
	}
	m.AddSeparator()
	pos := u.state().PositionMs
	if u.abA < 0 {
		menuAct(m, tr("Loop from here (A)"), func() {
			u.abA = pos
			u.toast(tr("Loop start set at {time} — now choose the end (B)", "time", core.FormatMs(pos)))
		})
	} else if u.abB < 0 {
		menuAct(m, tr("Loop until here (B)"), func() {
			if pos <= u.abA+300 {
				u.toast(tr("The end must be after the start"))
				return
			}
			u.abB = pos
			if u.mpv != nil {
				_ = u.mpv.SetABLoop(float64(u.abA)/1000, float64(u.abB)/1000)
			}
			u.toast(tr("Looping {a} – {b}", "a", core.FormatMs(u.abA), "b", core.FormatMs(u.abB)))
		})
	}
	if u.abA >= 0 {
		menuAct(m, tr("Stop the loop"), u.clearABLoop)
	}
	m.AddSeparator()
	menuAct(m, tr("All effects…"), func() { u.goPage("settings"); u.setPg.open("playback") })
}

func (u *UI) clearABLoop() {
	if u.abA < 0 {
		return
	}
	u.abA, u.abB = -1, -1
	if u.mpv != nil {
		_ = u.mpv.SetABLoop(-1, -1)
	}
}

// ---------------------------------------------------------------- fade

// fadeVolume moves the player volume from a to b in d, then calls done.
func (u *UI) fadeVolume(a, b float64, d time.Duration, done func()) {
	if u.mpv == nil {
		done()
		return
	}
	if u.fadeT != nil {
		u.fadeT.Stop()
		u.fadeT.DeleteLater()
	}
	t := qt.NewQTimer()
	u.fadeT = t
	t0 := time.Now()
	t.SetInterval(15)
	t.OnTimeout(func() {
		k := float64(time.Since(t0)) / float64(d)
		if k >= 1 {
			t.Stop()
			if u.fadeT == t {
				u.fadeT = nil
			}
			t.DeleteLater()
			_ = u.mpv.SetVolume(b)
			done()
			return
		}
		_ = u.mpv.SetVolume(a + (b-a)*k)
	})
	t.Start2()
}

// togglePauseFaded pauses or resumes with a short fade.
func (u *UI) togglePauseFaded(wasPaused bool) {
	vol := u.cfg.Volume
	if !u.cfg.Prefs.FadePause || u.mpv == nil || vol <= 0 {
		_ = u.player.TogglePause()
		return
	}
	if wasPaused {
		_ = u.mpv.SetVolume(0)
		_ = u.player.SetPaused(false)
		u.fadeVolume(0, vol, 220*time.Millisecond, func() {})
		return
	}
	u.fadeVolume(vol, 0, 180*time.Millisecond, func() {
		_ = u.player.SetPaused(true)
		_ = u.mpv.SetVolume(u.cfg.Volume)
	})
}

func menuAct(m *qt.QMenu, text string, f func()) *qt.QAction {
	a := m.AddAction(text)
	a.OnTriggered(f)
	return a
}

// applyFiltersSoon applies the filters once the value stops changing (each
// change rebuilds mpv's filter graph).
func (u *UI) applyFiltersSoon() {
	if u.fxTimer == nil {
		u.fxTimer = qt.NewQTimer()
		u.fxTimer.SetSingleShot(true)
		u.fxTimer.OnTimeout(u.applyFilters)
	}
	u.fxTimer.Start(300)
}
