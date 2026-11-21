package ui

import (
	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/theme"
)

// logoPixmap draws the logo (no background) in a color, for the window and
// tray icons.
func logoPixmap(size int, c theme.Color) *qt.QPixmap {
	px := qt.NewQPixmap2(size, size)
	clear := qt.NewQColor11(0, 0, 0, 0)
	px.FillWithFillColor(clear)
	clear.Delete()
	p := qt.NewQPainter2(px.QPaintDevice)
	p.SetRenderHint(qt.QPainter__Antialiasing)
	drawLogo(p, float64(size)/2, float64(size)/2, float64(size)*0.82, c)
	p.End()
	p.Delete()
	return px
}

// logoIcon is the app icon (the logo in the theme's accent color).
func (u *UI) logoIcon() *qt.QIcon {
	c := theme.Color{R: 217, G: 115, B: 122, A: 1}
	if v := u.style.c.Var("--red"); v != "" {
		if cc, ok := theme.ParseColor(v); ok {
			c = cc
		}
	}
	icon := qt.NewQIcon()
	for _, s := range []int{16, 22, 24, 32, 48, 64, 128, 256} {
		px := logoPixmap(s, c)
		icon.AddPixmap(px)
		px.Delete()
	}
	return icon
}

// buildTray adds the icon in the system tray (next to Flameshot & co.):
// click shows/hides the window; the menu controls playback.
func (u *UI) buildTray() {
	icon := u.logoIcon()
	qt.QGuiApplication_SetWindowIcon(icon)
	if !qt.QSystemTrayIcon_IsSystemTrayAvailable() {
		icon.Delete()
		return
	}
	u.tray = qt.NewQSystemTrayIcon4(icon, u.win.QObject)
	icon.Delete()
	u.tray.SetToolTip("meofishes")
	m := qt.NewQMenu(u.win.QWidget)
	m.SetObjectName("TrayMenu")
	add := func(text string, f func()) *qt.QAction {
		a := m.AddAction(text)
		a.OnTriggered(f)
		return a
	}
	u.trayNow = m.AddAction("")
	u.trayNow.SetEnabled(false)
	m.AddSeparator()
	add(tr("Play / pause"), u.TogglePlay)
	add(tr("Next"), func() { u.Next(true) })
	add(tr("Previous"), u.Prev)
	sm := m.AddMenuWithTitle(tr("Sleep timer"))
	sm.OnAboutToShow(func() {
		sm.Clear()
		u.sleepMenu(sm)
	})
	m.AddSeparator()
	add(tr("Show / hide meofishes"), u.toggleWindow)
	add(tr("Quit"), u.quit)
	u.tray.SetContextMenu(m)
	u.tray.OnActivated(func(r qt.QSystemTrayIcon__ActivationReason) {
		if r == qt.QSystemTrayIcon__Trigger || r == qt.QSystemTrayIcon__DoubleClick {
			u.toggleWindow()
		}
	})
	u.tray.SetVisible(u.cfg.Prefs.TrayIcon)
	u.syncTray()
}

func (u *UI) syncTray() {
	if u.tray == nil {
		return
	}
	text := tr("Nothing playing")
	if cur := u.current(); cur != nil {
		text = cur.String()
	}
	u.trayNow.SetText(ellipsis(text, 48))
	u.tray.SetToolTip("meofishes — " + text)
}

func (u *UI) toggleWindow() {
	if u.win.IsVisible() && !u.win.IsMinimized() {
		u.win.Hide()
		return
	}
	u.win.ShowNormal()
	if u.cfg.Maximized {
		u.win.ShowMaximized()
	}
	u.win.Raise()
	u.win.ActivateWindow()
}

// quit really quits (the close button may only hide the window).
func (u *UI) quit() {
	u.quitting = true
	u.win.Close()
	qt.QCoreApplication_Quit()
}
