package ui

import (
	"context"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
)

// FullPlayer is the fullscreen player. Minimal: the tilting cover in the
// middle (hover shows the title and artist over a blurred cover), the play
// button and a bar with everything else under it, and a side panel with the
// lyrics (or the visualizer, or the video) that can be shown or hidden.
type FullPlayer struct {
	*qt.QWidget
	u      *UI
	bg     *Backdrop
	cover  *TiltCover
	side   *qt.QWidget // lyrics panel
	right  *qt.QStackedWidget
	lyrics *LyricsView
	viz    *Visualizer
	video  *qt.QWidget // native window mpv draws video into
	vidBtn *IconButton
	lyrBtn *IconButton
	status *qt.QLabel
	vizFor string // stream URL the visualizer analyzes
	open   bool
}

func (u *UI) buildFullPlayer(parent *qt.QWidget) *FullPlayer {
	f := &FullPlayer{u: u}
	f.bg = u.backdrop("FullPlayer")
	f.QWidget = f.bg.QWidget
	f.SetParent(parent)
	f.Hide()
	root := vbox(f.QWidget, []int{24, 16, 24, 24}, 10)

	// top row: close … video, fullscreen window
	top := qt.NewQHBoxLayout2()
	down := u.iconButton("FullClose", "chevron-down", tr("Close (Esc)"))
	down.OnClicked(u.closeFull)
	top.AddWidget(down.QWidget)
	top.AddStretch()
	f.vidBtn = u.iconButton("FullVideo", "video", tr("Show the video (when the song has one)"))
	f.vidBtn.OnClicked(func() { u.setVideo(!u.videoOn) })
	fs := u.iconButton("FullWindow", "fullscreen", tr("Fullscreen window (F11)"))
	fs.OnClicked(func() {
		if u.win.IsFullScreen() {
			u.win.ShowNormal()
		} else {
			u.win.ShowFullScreen()
		}
	})
	top.AddWidget(f.vidBtn.QWidget)
	top.AddWidget(fs.QWidget)
	root.AddLayout(top.QLayout)

	body := qt.NewQHBoxLayout2()
	body.SetSpacing(56)
	body.AddStretch()

	// center column: cover, then play + the bar
	center := qt.NewQWidget2()
	center.SetObjectName("FullCenter")
	cb := vbox(center, nil, 14)
	cb.AddStretch()
	f.cover = u.tiltCover("FullCover")
	f.cover.ShowInfo = true
	f.cover.SetMinimumSize2(240, 240)
	f.cover.SetMaximumSize2(560, 560)
	u.covers = append(u.covers, f.cover.CoverView)
	cb.AddWidget3(f.cover.QWidget, 0, qt.AlignHCenter)

	controls := qt.NewQHBoxLayout2()
	controls.SetSpacing(10)
	play := u.iconButton("FullPlay", "play", tr("Play / pause (Ctrl+Space)"))
	play.OnClicked(u.TogglePlay)
	u.playBtns = append(u.playBtns, play)
	controls.AddWidget(play.QWidget)

	bar := frame("FullBar")
	bb := hbox(bar.QWidget, []int{8, 4, 8, 4}, 2)
	add := func(name, icon, tip string, fn func()) *IconButton {
		b := u.iconButton(name, icon, tip)
		b.OnClicked(fn)
		bb.AddWidget(b.QWidget)
		return b
	}
	add("FullPrev", "prev", tr("Previous (Ctrl+←)"), u.Prev)
	add("FullNext", "next", tr("Next (Ctrl+→)"), func() { u.Next(true) })
	sh := add("FullShuffle", "shuffle", tr("Shuffle"), u.toggleShuffle)
	rep := add("FullRepeat", "repeat", tr("Repeat"), u.cycleRepeat)
	u.shuffleBtns = append(u.shuffleBtns, sh)
	u.repeatBtns = append(u.repeatBtns, rep)
	sh.SetActive(u.cfg.Shuffle)
	add("FullQueue", "queue", tr("Queue"), func() { u.closeFull(); u.goPage("queue") })
	f.lyrBtn = add("FullLyricsToggle", "lyrics", tr("Show or hide the lyrics"), func() {
		u.cfg.Prefs.FullLyrics = !u.cfg.Prefs.FullLyrics
		_ = u.cfg.Save()
		f.syncSide()
	})
	add("FullArtist", "search", tr("Artist"), func() {
		if cur := u.current(); cur != nil && cur.Artist != "" {
			u.closeFull()
			u.search.box.SetText(cur.Artist)
			u.goPage("search")
			u.doSearch(u.search)
		}
	})
	bb.AddWidget(u.likeButton("FullLike").QWidget)
	vol := add("FullVolumeIcon", "volume", tr("Volume: click to mute, scroll to change"), func() {
		if u.cfg.Volume > 0 {
			u.lastVol = u.cfg.Volume
			u.setVolume(0)
		} else {
			u.setVolume(max(u.lastVol, 10))
		}
	})
	vol.OnWheelEvent(func(super func(*qt.QWheelEvent), e *qt.QWheelEvent) {
		d := e.AngleDelta()
		u.setVolume(u.cfg.Volume + float64(d.Y())/120*5)
		_ = u.cfg.Save()
	})
	u.volBtns = append(u.volBtns, vol)
	add("FullMore", "more", tr("More"), func() {
		if cur := u.current(); cur != nil {
			u.trackMenuFor([]core.Track{*cur}, 0, u.PlayList, nil)
		}
	})
	controls.AddWidget2(bar.QWidget, 1)
	cb.AddLayout(controls.QLayout)
	cb.AddLayout(u.seekRow("Full").QLayout)
	cb.AddStretch()
	center.SetMaximumWidth(600)
	body.AddWidget2(center, 5)

	// side panel: lyrics, visualizer or video
	f.side = qt.NewQWidget2()
	f.side.SetObjectName("FullSide")
	rb := vbox(f.side, nil, 6)
	f.right = qt.NewQStackedWidget2()
	f.right.SetObjectName("FullRight")
	f.lyrics = u.lyricsView("FullLyrics")
	u.addLyricsView(f.lyrics)
	f.viz = u.visualizer("FullVisualizer")
	f.right.AddWidget(f.lyrics.QWidget)
	f.right.AddWidget(f.viz.QWidget)
	// video: a native child window (only this widget, not its parents)
	f.video = qt.NewQWidget2()
	f.video.SetObjectName("FullVideo")
	f.video.SetAttribute(qt.WA_DontCreateNativeAncestors)
	f.video.SetAttribute(qt.WA_NativeWindow)
	f.video.SetAttribute(qt.WA_StyledBackground)
	f.video.SetMinimumSize2(320, 180)
	f.right.AddWidget(f.video)
	_ = f.video.WinId() // create the native window now: mpv gets its id at start
	rb.AddWidget2(f.right.QWidget, 1)
	row := qt.NewQHBoxLayout2()
	f.status = label("", "FullLyricsStatus", "hint")
	u.lyricStatus = append(u.lyricStatus, f.status)
	row.AddWidget2(f.status.QWidget, 1)
	retry := u.iconButton("FullLyricsRetry", "refresh", tr("Search the lyrics again (skips the cache)"))
	retry.OnClicked(u.retryLyrics)
	row.AddWidget(retry.QWidget)
	rb.AddLayout(row.QLayout)
	body.AddWidget2(f.side, 6)
	body.AddStretch()
	root.AddLayout2(body.QLayout, 1)

	f.OnKeyPressEvent(func(super func(*qt.QKeyEvent), e *qt.QKeyEvent) {
		if e.Key() == int(qt.Key_Escape) {
			u.closeFull()
			return
		}
		super(e)
	})
	f.syncSide()
	return f
}

// syncSide shows or hides the lyrics panel (always shown for video).
func (f *FullPlayer) syncSide() {
	show := f.u.cfg.Prefs.FullLyrics || f.u.videoOn
	f.side.SetVisible(show)
	f.lyrBtn.SetActive(f.u.cfg.Prefs.FullLyrics)
	if show {
		f.syncRight()
	}
}

func (u *UI) toggleFull() {
	if u.full.open {
		u.closeFull()
	} else {
		u.openFull()
	}
}

func (u *UI) openFull() {
	f := u.full
	if f.open {
		return
	}
	f.open = true
	w, h := u.root.Width(), u.root.Height()
	f.SetGeometry(0, h, w, h)
	f.Show()
	f.Raise()
	u.toastW.Raise()
	f.SetFocus()
	f.syncRight()
	singleShot(30, f.bg.startPotion)
	playing := !u.state().Paused && !u.state().Idle
	singleShot(0, func() {
		for _, lv := range u.lyricViews {
			lv.SetRunning(playing)
		}
	})
	p := u.style.For(f.QWidget, "FullPlayer")
	dur := time.Duration(p.Ms("open-duration", 420)) * time.Millisecond
	u.anim.Tween(f, dur, 0, p.Str("open-easing", "out-cubic"), func(t float64) {
		f.Move(0, int(float64(u.root.Height())*(1-t)))
	}, func() { f.SetGeometry(0, 0, u.root.Width(), u.root.Height()) })
}

func (u *UI) closeFull() {
	f := u.full
	if f == nil || !f.open {
		return
	}
	f.open = false
	p := u.style.For(f.QWidget, "FullPlayer")
	dur := time.Duration(p.Ms("close-duration", 320)) * time.Millisecond
	y0 := f.Y()
	u.anim.Tween(f, dur, 0, p.Str("close-easing", "in-cubic"), func(t float64) {
		f.Move(0, int(lerp(float64(y0), float64(u.root.Height()), t)))
	}, func() {
		f.Hide()
		f.viz.SetRunning(false)
	})
}

// trackChanged is called for every new track.
func (f *FullPlayer) trackChanged(t core.Track) {
	f.cover.SetInfo(t.Title, t.Artist)
	f.viz.Stop()
	f.vizFor = ""
	if t.Cover == "" {
		f.bg.SetImage(nil)
	}
}

// lyricsChanged switches between lyrics and the visualizer.
func (f *FullPlayer) lyricsChanged(l *core.Lyrics) {
	f.syncRight()
}

// syncVideo shows the video area while video is on.
func (f *FullPlayer) syncVideo() {
	f.vidBtn.SetActive(f.u.videoOn)
	f.syncSide()
}

func (f *FullPlayer) syncRight() {
	u := f.u
	if u.videoOn {
		f.right.SetCurrentWidget(f.video)
		f.viz.SetRunning(false)
		return
	}
	noLyrics := u.lyrics == nil && !u.lyricsSearching
	if !noLyrics {
		f.right.SetCurrentIndex(0)
		f.viz.SetRunning(false)
		return
	}
	f.right.SetCurrentIndex(1)
	if !f.open {
		return
	}
	if u.stream != nil && f.vizFor != u.stream.URL {
		f.vizFor = u.stream.URL
		f.viz.Start(u.stream.URL, u.stream.Headers)
	}
	playing := !u.state().Paused && !u.state().Idle
	f.viz.SetRunning(playing)
}

// retryLyrics searches again, skipping the cache.
func (u *UI) retryLyrics() {
	cur := u.current()
	if cur == nil {
		return
	}
	gen := u.gen
	track := *cur
	u.lyricsSearching = true
	u.setLyrics(nil, tr("Looking for lyrics…"))
	var l *core.Lyrics
	var info string
	u.runAsync(func() error {
		var err error
		l, info, err = u.core.Plugins.FindLyricsFresh(context.Background(), track)
		return err
	}, func(err error) {
		if gen != u.gen {
			return
		}
		u.lyricsSearching = false
		u.lyricsInfo = info
		if err != nil {
			u.setLyrics(nil, tr("No lyrics for this song"))
		} else {
			u.setLyrics(l, "")
		}
		u.toast(info)
	})
}
