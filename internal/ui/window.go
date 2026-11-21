package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
)

func label(text, name, r string) *qt.QLabel {
	l := qt.NewQLabel3(text)
	if name != "" {
		l.SetObjectName(name)
	}
	if r != "" {
		role(l.QWidget, r)
	}
	return l
}

func vbox(w *qt.QWidget, margins []int, spacing int) *qt.QVBoxLayout {
	b := qt.NewQVBoxLayout(w)
	if len(margins) == 4 {
		b.SetContentsMargins(margins[0], margins[1], margins[2], margins[3])
	} else {
		b.SetContentsMargins(0, 0, 0, 0)
	}
	b.SetSpacing(spacing)
	return b
}

func hbox(w *qt.QWidget, margins []int, spacing int) *qt.QHBoxLayout {
	b := qt.NewQHBoxLayout(w)
	if len(margins) == 4 {
		b.SetContentsMargins(margins[0], margins[1], margins[2], margins[3])
	} else {
		b.SetContentsMargins(0, 0, 0, 0)
	}
	b.SetSpacing(spacing)
	return b
}

func frame(name string) *qt.QFrame {
	f := qt.NewQFrame2()
	f.SetObjectName(name)
	f.SetAttribute(qt.WA_StyledBackground)
	return f
}

// build creates the window (frameless, our own title bar):
//
//	Root (Backdrop)
//	├── TopBar: logo + menu (home, settings) … minimize, maximize, close
//	├── Body
//	│   ├── LeftPanel: library list, current lyric line, now-playing card
//	│   └── Pages
//	└── FullPlayer (overlay, hidden until opened)
func (u *UI) build() {
	u.win = qt.NewQMainWindow2()
	u.win.SetObjectName("MainWindow")
	u.win.SetWindowTitle("meofishes")
	u.win.SetWindowFlags(qt.Window | qt.FramelessWindowHint)
	w, h := u.cfg.WindowW, u.cfg.WindowH
	if w < 800 || h < 500 {
		w, h = 1280, 820
	}
	u.win.Resize(w, h)
	u.win.SetMinimumSize2(900, 600)

	u.root = u.backdrop("Root")
	u.win.SetCentralWidget(u.root.QWidget)
	rb := vbox(u.root.QWidget, nil, 0)
	rb.AddWidget(u.buildTopBar())

	body := qt.NewQWidget2()
	body.SetObjectName("Body")
	bb := hbox(body, []int{10, 0, 10, 10}, 10)
	bb.AddWidget(u.buildLeftPanel())
	bb.AddWidget2(u.buildPages(), 1)
	rb.AddWidget2(body, 1)

	u.full = u.buildFullPlayer(u.root.QWidget)
	u.toastW = u.newToast(u.root.QWidget)
	u.buildResizeGrips()
	u.root.onResize = append(u.root.onResize, func() {
		if u.full.IsVisible() {
			u.full.SetGeometry(0, 0, u.root.Width(), u.root.Height())
		}
	})
	u.buildFPS(u.root.QWidget)

	u.win.OnCloseEvent(func(super func(*qt.QCloseEvent), e *qt.QCloseEvent) {
		u.saveQueue()
		// keep playing in the background: hide to the tray instead of quitting
		if !u.quitting && u.cfg.Prefs.Background && u.tray != nil && u.tray.IsVisible() {
			e.Ignore()
			u.win.Hide()
			if !u.cfg.Prefs.BackgroundHinted {
				u.cfg.Prefs.BackgroundHinted = true
				u.tray.ShowMessage("meofishes", tr("Still playing here. Right-click the icon to quit."), u.logoIcon())
			}
			return
		}
		super(e)
		qt.QCoreApplication_Quit()
	})
	u.shortcuts()
	start := u.cfg.Prefs.StartPage
	if _, ok := u.pageIndex[start]; !ok {
		start = "home"
	}
	u.goPage(start)
}

func (u *UI) shortcuts() {
	add := func(key string, f func()) {
		ks := qt.NewQKeySequence2(key)
		sc := qt.NewQShortcut2(ks, u.win.QWidget)
		ks.Delete()
		sc.OnActivated(f)
	}
	add("Ctrl+Space", u.TogglePlay)
	add("Media Play", u.TogglePlay)
	add("Media Next", func() { u.Next(true) })
	add("Media Previous", u.Prev)
	add("Ctrl+Right", func() { u.Next(true) })
	add("Ctrl+Left", u.Prev)
	add("Shift+Right", func() { u.seekBy(u.cfg.Prefs.SeekStepSecs) })
	add("Shift+Left", func() { u.seekBy(-u.cfg.Prefs.SeekStepSecs) })
	add("Ctrl+Up", func() { u.setVolume(u.cfg.Volume + 5) })
	add("Ctrl+Down", func() { u.setVolume(u.cfg.Volume - 5) })
	add("Ctrl+F", func() { u.closeFull(); u.goPage("search"); u.search.box.SetFocus() })
	add("Ctrl+H", func() { u.closeFull(); u.goPage("home") })
	add("Ctrl+L", u.toggleFull)
	add("Ctrl+D", func() {
		if cur := u.current(); cur != nil {
			u.toggleLike(*cur)
		}
	})
	add("Ctrl+1", func() { u.goPage("home") })
	add("Ctrl+2", func() { u.goPage("search") })
	add("Ctrl+3", func() { u.goPage("library") })
	add("Ctrl+4", func() { u.goPage("queue") })
	add("Ctrl+,", func() { u.goPage("settings") })
	add("Ctrl+Q", u.quit)
	add("Escape", u.closeFull)
	add("F11", func() {
		if u.win.IsFullScreen() {
			u.win.ShowNormal()
		} else {
			u.win.ShowFullScreen()
		}
	})
	add("F5", func() { u.applyTheme(false); u.toast(tr("Theme reloaded")) })
	add("F12", func() {
		if u.cfg.Prefs.DevMode {
			u.inspectUnderMouse()
		}
	})
	add("Ctrl+Shift+D", func() {
		u.cfg.Prefs.DevMode = !u.cfg.Prefs.DevMode
		_ = u.cfg.Save()
		u.devModeChanged()
	})
}

// Shortcuts lists the keyboard shortcuts (Settings → Shortcuts).
var Shortcuts = [][2]string{
	{"Ctrl+Space", "Play / pause"}, {"Ctrl+→ / Ctrl+←", "Next / previous"},
	{"Shift+→ / Shift+←", "Seek forward / back"}, {"Ctrl+↑ / Ctrl+↓", "Volume up / down"},
	{"Ctrl+L", "Fullscreen player and lyrics"}, {"Ctrl+D", "Like the current song"},
	{"Ctrl+F", "Search"}, {"Ctrl+1…4", "Home, Search, Library, Queue"}, {"Ctrl+,", "Settings"},
	{"Esc", "Close the fullscreen player"}, {"F11", "Fullscreen window"}, {"F5", "Reload the theme"},
	{"Ctrl+Shift+D", "Developer mode"}, {"F12", "Inspect the widget under the mouse (developer mode)"},
	{"Ctrl+Q", "Quit"},
}

func (u *UI) enterAll() {
	singleShot(30, func() {
		for _, w := range []*qt.QWidget{u.leftPanel, u.pages.QWidget} {
			if w != nil && w.IsVisible() {
				u.Enter(w, "QFrame", 0)
			}
		}
	})
}

// goPage switches the stacked pages with the -meo-transition animation.
func (u *UI) goPage(id string) {
	if id == "plugins" || id == "themes" { // they live inside Settings
		u.goPage("settings")
		u.setPg.open(id)
		return
	}
	idx, ok := u.pageIndex[id]
	if !ok {
		return
	}
	for k, b := range u.nav {
		b.SetActive(k == id)
	}
	u.syncLibraryNav(id)
	if u.curPage == id {
		return
	}
	if u.curPage != "" {
		u.history = append(u.history, u.curPage)
		if len(u.history) > 30 {
			u.history = u.history[1:]
		}
	}
	u.curPage = id
	old := u.pages.CurrentWidget()
	p := u.style.For(u.pages.QWidget, "QStackedWidget")
	mode := p.Str("transition", "fade")
	if mode != "none" && old != nil && old.IsVisible() {
		if u.pageFx == nil {
			u.pageFx = u.newTransition(u.pages.ParentWidget())
		}
		dur := time.Duration(p.Ms("transition-duration", 300)) * time.Millisecond
		pos := u.pages.Pos()
		u.pageFx.Run(old, mode, dur, p.Str("transition-easing", "out-cubic"), p.Len("transition-distance", 24))
		u.pageFx.Move(pos.X(), pos.Y())
	}
	u.pages.SetCurrentIndex(idx)
	switch id {
	case "home":
		u.home.refresh()
	case "settings":
		u.setPg.refresh()
	case "library":
		u.lib.refresh()
	case "dev":
		u.devPg.refresh()
	}
}

func (u *UI) goBack() {
	if n := len(u.history); n > 0 {
		id := u.history[n-1]
		u.history = u.history[:n-1]
		cur := u.curPage
		u.goPage(id)
		u.history = u.history[:len(u.history)-1] // goPage pushed cur again
		_ = cur
	}
}

func (u *UI) devModeChanged() {
	u.nav["dev"].SetVisible(u.cfg.Prefs.DevMode && u.menuOpen)
	u.netlogSync()
	u.fps.label.SetVisible(u.cfg.Prefs.DevMode && u.cfg.Prefs.ShowFPS)
	if u.cfg.Prefs.DevMode {
		u.toast(tr("Developer mode on"))
	} else {
		u.toast(tr("Developer mode off"))
		if u.curPage == "dev" {
			u.goPage("settings")
		}
	}
}

// ---------------------------------------------------------------- player bar

// transport builds shuffle/prev/play/next/repeat buttons (registered so that
// playback updates every copy: bar and fullscreen player).
func (u *UI) transport(prefix string) *qt.QHBoxLayout {
	ctr := qt.NewQHBoxLayout2()
	ctr.SetSpacing(10)
	sh := u.iconButton(prefix+"Shuffle", "shuffle", tr("Shuffle"))
	sh.OnClicked(u.toggleShuffle)
	prev := u.iconButton(prefix+"Prev", "prev", tr("Previous (Ctrl+←)"))
	prev.OnClicked(u.Prev)
	play := u.iconButton(prefix+"Play", "play", tr("Play / pause (Ctrl+Space)"))
	play.OnClicked(u.TogglePlay)
	next := u.iconButton(prefix+"Next", "next", tr("Next (Ctrl+→)"))
	next.OnClicked(func() { u.Next(true) })
	rep := u.iconButton(prefix+"Repeat", "repeat", tr("Repeat"))
	rep.OnClicked(u.cycleRepeat)
	for _, x := range []*IconButton{sh, prev, play, next, rep} {
		ctr.AddWidget(x.QWidget)
	}
	u.playBtns = append(u.playBtns, play)
	u.shuffleBtns = append(u.shuffleBtns, sh)
	u.repeatBtns = append(u.repeatBtns, rep)
	sh.SetActive(u.cfg.Shuffle)
	return ctr
}

// seekRow builds elapsed time + seek bar + total time.
func (u *UI) seekRow(prefix string) *qt.QHBoxLayout {
	sk := qt.NewQHBoxLayout2()
	sk.SetSpacing(10)
	el := label("0:00", prefix+"TimeElapsed", "time")
	el.SetAlignment(qt.AlignRight | qt.AlignVCenter)
	el.SetMinimumWidth(44)
	tot := label("—", prefix+"TimeTotal", "time")
	tot.SetMinimumWidth(44)
	s := u.slider(prefix + "SeekBar")
	s.OnSeek = func(v float64) {
		if d := u.duration(); d > 0 {
			u.SeekTo(int64(v * float64(d)))
		}
	}
	s.Tip = func(v float64) string {
		if d := u.duration(); d > 0 {
			return core.FormatMs(int64(v * float64(d)))
		}
		return ""
	}
	sk.AddWidget(el.QWidget)
	sk.AddWidget2(s.QWidget, 1)
	sk.AddWidget(tot.QWidget)
	u.seeks = append(u.seeks, s)
	u.tElapsed = append(u.tElapsed, el)
	u.tTotal = append(u.tTotal, tot)
	return sk
}

// volumeRow builds the mute button + volume slider.
func (u *UI) volumeRow(prefix string) *qt.QHBoxLayout {
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(6)
	vb := u.iconButton(prefix+"VolumeIcon", "volume", tr("Volume (Ctrl+↑/↓)"))
	last := u.cfg.Volume
	vb.OnClicked(func() {
		if u.cfg.Volume > 0 {
			last = u.cfg.Volume
			u.setVolume(0)
		} else {
			u.setVolume(max(last, 10))
		}
	})
	vs := u.slider(prefix + "VolumeBar")
	vs.SetMinimumWidth(90)
	vs.SetMaximumWidth(140)
	vs.SetValue(u.cfg.Volume / 100)
	vs.OnDrag = func(v float64) { u.setVolume(v * 100) }
	vs.OnSeek = func(v float64) { u.setVolume(v * 100); _ = u.cfg.Save() }
	vs.Tip = func(v float64) string { return itoa(int(v*100+0.5)) + "%" }
	row.AddWidget(vb.QWidget)
	row.AddWidget(vs.QWidget)
	u.volBtns = append(u.volBtns, vb)
	u.volSliders = append(u.volSliders, vs)
	return row
}

func (u *UI) likeButton(name string) *IconButton {
	b := u.iconButton(name, "heart-outline", tr("Like (Ctrl+D)"))
	b.OnClicked(func() {
		if cur := u.current(); cur != nil {
			u.toggleLike(*cur)
		}
	})
	u.likeBtns = append(u.likeBtns, b)
	return b
}

func (u *UI) addLyricsView(lv *LyricsView) {
	lv.Pos = u.lyricsPos
	lv.OnSeek = func(ms int64) { u.SeekTo(ms + int64(u.cfg.Prefs.LyricsOffsetMs)) }
	u.lyricViews = append(u.lyricViews, lv)
}
