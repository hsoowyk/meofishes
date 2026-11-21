package ui

import (
	"context"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
)

// FullPlayer is the fullscreen player that covers the window: blurred cover
// background, the tilting cover, controls, and the lyrics (or the frequency
// visualizer when the song has no lyrics).
type FullPlayer struct {
	*qt.QWidget
	u      *UI
	bg     *Backdrop
	cover  *TiltCover
	right  *qt.QStackedWidget
	lyrics *LyricsView
	viz    *Visualizer
	video  *qt.QWidget // native window mpv draws video into
	vidBtn *IconButton
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
	root := vbox(f.QWidget, []int{28, 18, 28, 24}, 10)

	// top row
	top := qt.NewQHBoxLayout2()
	down := u.iconButton("FullClose", "chevron-down", tr("Close (Esc)"))
	down.OnClicked(u.closeFull)
	top.AddWidget(down.QWidget)
	top.AddStretch()
	f.vidBtn = u.iconButton("FullVideo", "video", tr("Show the video (when the song has one)"))
	f.vidBtn.OnClicked(func() { u.setVideo(!u.videoOn) })
	top.AddWidget(f.vidBtn.QWidget)
	fs := u.iconButton("FullWindow", "fullscreen", tr("Fullscreen window (F11)"))
	fs.OnClicked(func() {
		if u.win.IsFullScreen() {
			u.win.ShowNormal()
		} else {
			u.win.ShowFullScreen()
		}
	})
	top.AddWidget(fs.QWidget)
	root.AddLayout(top.QLayout)

	body := qt.NewQHBoxLayout2()
	body.SetSpacing(48)

	// left: cover + info + controls
	left := qt.NewQWidget2()
	left.SetObjectName("FullLeft")
	left.SetMaximumWidth(560)
	lb := vbox(left, nil, 8)
	f.cover = u.tiltCover("FullCover")
	f.cover.SetMinimumSize2(260, 260)
	u.covers = append(u.covers, f.cover.CoverView)
	lb.AddWidget2(f.cover.QWidget, 1)
	info := qt.NewQHBoxLayout2()
	tv := qt.NewQVBoxLayout2()
	tv.SetSpacing(2)
	t := label("", "FullTitle", "")
	t.SetWordWrap(true)
	a := label("", "FullArtist", "")
	al := label("", "FullAlbum", "")
	u.titleLbls = append(u.titleLbls, t)
	u.artistLbls = append(u.artistLbls, a)
	u.albumLbls = append(u.albumLbls, al)
	tv.AddWidget(t.QWidget)
	tv.AddWidget(a.QWidget)
	tv.AddWidget(al.QWidget)
	info.AddLayout2(tv.QLayout, 1)
	info.AddWidget(u.likeButton("FullLike").QWidget)
	lb.AddLayout(info.QLayout)
	lb.AddLayout(u.seekRow("Full").QLayout)
	ctr := u.transport("Full")
	ctr.InsertStretch(0)
	ctr.AddStretch()
	lb.AddLayout(ctr.QLayout)
	vr := u.volumeRow("Full")
	vr.InsertStretch(0)
	vr.AddStretch()
	lb.AddLayout(vr.QLayout)
	body.AddWidget2(left, 4)

	// right: lyrics or visualizer
	rw := qt.NewQWidget2()
	rb := vbox(rw, nil, 6)
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
	body.AddWidget2(rw, 6)
	root.AddLayout2(body.QLayout, 1)

	f.OnKeyPressEvent(func(super func(*qt.QKeyEvent), e *qt.QKeyEvent) {
		if e.Key() == int(qt.Key_Escape) {
			u.closeFull()
			return
		}
		super(e)
	})
	return f
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
	f.syncRight()
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
