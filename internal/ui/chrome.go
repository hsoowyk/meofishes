package ui

import (
	"math"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
	"meofishes/internal/library"
	"meofishes/internal/theme"
)

// ---------------------------------------------------------------- logo

// logoPoints is the meofishes logo (assets/icons/meofishes.svg), on its own
// 135×135 viewBox starting at (307.5, 127.5).
var logoPoints = [][2]float64{{315, 135}, {315, 240}, {360, 225}, {390, 255}, {435, 240}, {435, 135},
	{405, 165}, {375, 150}, {345, 165}, {315, 135}}

// drawLogo paints the logo centered at (cx, cy) inside a size×size box.
func drawLogo(p *qt.QPainter, cx, cy, size float64, c theme.Color) {
	k := size / 135
	X := func(v float64) float64 { return cx + (v-307.5-67.5)*k }
	Y := func(v float64) float64 { return cy + (v-127.5-67.5)*k }
	path := qt.NewQPainterPath()
	path.MoveTo2(X(logoPoints[0][0]), Y(logoPoints[0][1]))
	for _, pt := range logoPoints[1:] {
		path.LineTo2(X(pt[0]), Y(pt[1]))
	}
	path.CloseSubpath()
	col := qc(c)
	br := qt.NewQBrush3(col)
	pen := qt.NewQPen4(br, math.Max(1, 15*k))
	pen.SetCapStyle(qt.RoundCap)
	pen.SetJoinStyle(qt.RoundJoin)
	p.SetPenWithPen(pen)
	p.SetBrush(br)
	p.DrawPath(path)
	pen.Delete()
	br.Delete()
	col.Delete()
	path.Delete()
}

// logoColor follows the theme: the text color (dark on light themes, light
// on dark ones) unless the theme sets #Logo { -meo-logo-color: … }.
func (u *UI) logoColor(w *qt.QWidget) theme.Color {
	return u.style.For(w, "QWidget").Color("logo-color", paletteText(w, qt.QPalette__WindowText))
}

// ---------------------------------------------------------------- top bar

// buildTopBar is the title bar: the logo (click opens the menu with every
// tab of the app, double click goes home, the arrow hides the menu again), a
// drag area, and the window buttons on the far right.
func (u *UI) buildTopBar() *qt.QWidget {
	f := frame("TopBar")
	f.SetFixedHeight(int(u.style.ForName("TopBar", "QFrame").Len("height", 52)))
	b := hbox(f.QWidget, []int{14, 6, 8, 6}, 4)

	logo := qt.NewQWidget2()
	u.logoW = logo
	logo.SetObjectName("Logo")
	logo.SetFixedSize2(40, 40)
	logo.SetToolTip(tr("Menu (double-click: Home)"))
	handCursor(logo)
	d := u.newDeco(logo, "QWidget")
	logo.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		p := qt.NewQPainter2(logo.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		d.paintBg(p, 10)
		s := u.style.For(logo, "QWidget").Len("logo-size", 26) * d.scale
		drawLogo(p, 20, 20, s, u.logoColor(logo))
		p.End()
		p.Delete()
	})
	logo.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { d.setHover(true) })
	logo.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { d.setHover(false) })
	logo.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		d.setPressed(true)
		u.setMenuOpen(true)
	})
	logo.OnMouseReleaseEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { d.setPressed(false) })
	logo.OnMouseDoubleClickEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		u.closeFull()
		u.goPage("home")
	})
	b.AddWidget(logo)

	// the menu that slides out to the right of the logo
	u.menu = qt.NewQWidget2()
	u.menu.SetObjectName("LogoMenu")
	mb := hbox(u.menu, []int{6, 0, 0, 0}, 4)
	sep := qt.NewQFrame2()
	sep.SetObjectName("LogoMenuSeparator")
	sep.SetFixedSize2(1, 24)
	sep.SetAttribute(qt.WA_StyledBackground)
	mb.AddWidget(sep.QWidget)
	add := func(id, icon, tip string) {
		nb := u.navButton("Nav"+id, icon, tip)
		nb.OnClicked(func() { u.closeFull(); u.goPage(id) })
		u.nav[id] = nb
		mb.AddWidget(nb.QWidget)
	}
	add("home", "home", tr("Home (Ctrl+H)"))
	add("search", "search", tr("Search (Ctrl+F)"))
	add("library", "library", tr("Library (Ctrl+3)"))
	add("queue", "queue", tr("Queue (Ctrl+4)"))
	add("together", "people", tr("Listen together"))
	add("settings", "settings", tr("Settings (Ctrl+,)"))
	add("dev", "code", tr("Developer mode"))
	hide := u.iconButton("LogoMenuHide", "chevron-left", tr("Hide the menu"))
	hide.OnClicked(func() { u.setMenuOpen(false) })
	mb.AddWidget(hide.QWidget)
	u.menu.SetMaximumWidth(0)
	b.AddWidget(u.menu)

	b.AddStretch()
	u.pageTitle = label("", "TopTitle", "")
	b.AddWidget(u.pageTitle.QWidget)
	b.AddStretch()

	min := u.iconButton("WinMinimize", "minimize", tr("Minimize"))
	min.OnClicked(func() { u.win.ShowMinimized() })
	maxb := u.iconButton("WinMaximize", "maximize", tr("Maximize"))
	maxb.OnClicked(u.toggleMaximized)
	cls := u.iconButton("WinClose", "close", tr("Close"))
	cls.OnClicked(func() { u.win.Close() })
	for _, x := range []*IconButton{min, maxb, cls} {
		role(x.QWidget, "window")
		b.AddWidget(x.QWidget)
	}
	u.maxBtn = maxb

	// drag the window by the bar; double click maximizes
	f.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if e.Button() == qt.LeftButton {
			if wh := u.win.WindowHandle(); wh != nil {
				wh.StartSystemMove()
			}
		}
	})
	f.OnMouseDoubleClickEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { u.toggleMaximized() })
	return f.QWidget
}

func (u *UI) toggleMaximized() {
	if u.win.IsMaximized() {
		u.win.ShowNormal()
	} else {
		u.win.ShowMaximized()
	}
	singleShot(50, u.syncMaximized)
}

func (u *UI) syncMaximized() {
	if u.win.IsMaximized() {
		u.maxBtn.SetIcon("restore")
		u.maxBtn.SetToolTip(tr("Restore"))
	} else {
		u.maxBtn.SetIcon("maximize")
		u.maxBtn.SetToolTip(tr("Maximize"))
	}
	for _, g := range u.grips {
		g.SetVisible(!u.win.IsMaximized() && !u.win.IsFullScreen())
	}
}

// setMenuOpen slides the logo menu in or out.
func (u *UI) setMenuOpen(open bool) {
	if u.menuOpen == open {
		return
	}
	u.menuOpen = open
	u.nav["dev"].SetVisible(u.cfg.Prefs.DevMode)
	from := float64(u.menu.Width())
	if !open {
		from = float64(u.menu.MaximumWidth())
		if from > 2000 {
			from = float64(u.menu.Width())
		}
	}
	to := 0.0
	if open {
		to = float64(u.menu.SizeHint().Width())
	}
	p := u.style.For(u.menu, "QWidget")
	dur := time.Duration(p.Ms("open-duration", 260)) * time.Millisecond
	u.anim.Tween(u.menu, dur, 0, p.Str("open-easing", "out-cubic"), func(t float64) {
		u.menu.SetMaximumWidth(int(lerp(from, to, t)))
	}, func() {
		if open {
			u.menu.SetMaximumWidth(16777215)
		}
	})
}

// buildResizeGrips adds invisible borders to resize the frameless window.
func (u *UI) buildResizeGrips() {
	type grip struct {
		edges  qt.Edge
		cursor qt.CursorShape
	}
	defs := []grip{
		{qt.LeftEdge, qt.SizeHorCursor}, {qt.RightEdge, qt.SizeHorCursor},
		{qt.TopEdge, qt.SizeVerCursor}, {qt.BottomEdge, qt.SizeVerCursor},
		{qt.TopEdge | qt.LeftEdge, qt.SizeFDiagCursor}, {qt.BottomEdge | qt.RightEdge, qt.SizeFDiagCursor},
		{qt.TopEdge | qt.RightEdge, qt.SizeBDiagCursor}, {qt.BottomEdge | qt.LeftEdge, qt.SizeBDiagCursor},
	}
	for _, d := range defs {
		d := d
		g := qt.NewQWidget(u.root.QWidget)
		g.SetObjectName("ResizeGrip")
		c := qt.NewQCursor2(d.cursor)
		g.SetCursor(c)
		c.Delete()
		g.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
			if wh := u.win.WindowHandle(); wh != nil {
				wh.StartSystemResize(d.edges)
			}
		})
		u.grips = append(u.grips, g)
		u.gripEdges = append(u.gripEdges, d.edges)
	}
	place := func() {
		w, h, t := u.root.Width(), u.root.Height(), 5
		for i, g := range u.grips {
			e := u.gripEdges[i]
			x, y, gw, gh := t, 0, w-2*t, t
			switch e {
			case qt.LeftEdge:
				x, y, gw, gh = 0, t, t, h-2*t
			case qt.RightEdge:
				x, y, gw, gh = w-t, t, t, h-2*t
			case qt.BottomEdge:
				x, y, gw, gh = t, h-t, w-2*t, t
			case qt.TopEdge | qt.LeftEdge:
				x, y, gw, gh = 0, 0, t*2, t*2
			case qt.BottomEdge | qt.RightEdge:
				x, y, gw, gh = w-2*t, h-2*t, t*2, t*2
			case qt.TopEdge | qt.RightEdge:
				x, y, gw, gh = w-2*t, 0, t*2, t*2
			case qt.BottomEdge | qt.LeftEdge:
				x, y, gw, gh = 0, h-2*t, t*2, t*2
			}
			g.SetGeometry(x, y, gw, gh)
			g.Raise()
		}
		u.syncMaximized()
	}
	u.root.onResize = append(u.root.onResize, place)
}

// ---------------------------------------------------------------- left panel

// buildLeftPanel: the library (liked songs + playlists), the
// current lyric line and the now-playing card.
func (u *UI) buildLeftPanel() *qt.QWidget {
	f := frame("LeftPanel")
	f.SetFixedWidth(int(u.style.ForName("LeftPanel", "QFrame").Len("width", 290)))
	u.style.OnChange(func() { f.SetFixedWidth(int(u.style.ForName("LeftPanel", "QFrame").Len("width", 290))) })
	u.leftPanel = f.QWidget
	b := vbox(f.QWidget, []int{12, 12, 12, 12}, 8)

	// library header
	hr := qt.NewQHBoxLayout2()
	hr.AddWidget2(label(tr("Your library"), "LibraryHeading", "heading").QWidget, 1)
	np := u.iconButton("NewPlaylist", "plus", tr("New playlist"))
	np.OnClicked(func() { u.newPlaylistDialog(nil) })
	imp := u.iconButton("ImportPlaylist", "import", tr("Import playlist (.mepw, .m3u)"))
	imp.OnClicked(u.importPlaylists)
	hr.AddWidget(np.QWidget)
	hr.AddWidget(imp.QWidget)
	b.AddLayout(hr.QLayout)

	sa := qt.NewQScrollArea2()
	sa.SetObjectName("LibraryNav")
	sa.SetWidgetResizable(true)
	sa.SetFrameShape(qt.QFrame__NoFrame)
	sa.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	inner := qt.NewQWidget2()
	inner.SetObjectName("LibraryNavInner")
	u.libNavBox = vbox(inner, nil, 2)
	u.libNavBox.AddStretch()
	sa.SetWidget(inner)
	u.libNav = sa.QWidget
	b.AddWidget2(sa.QWidget, 1)
	u.refreshLibraryNav()

	u.curLyric = u.lyricLine("CurrentLyric")
	b.AddWidget(u.curLyric.QWidget)
	b.AddWidget(u.buildNowCard())
	return f.QWidget
}

// refreshLibraryNav lists the playlists in the left panel.
func (u *UI) refreshLibraryNav() {
	for _, r := range u.libRows {
		r.d.dispose()
		r.cover.dispose()
		r.Hide()
		r.DeleteLater()
	}
	u.libRows = nil
	for _, p := range u.core.Library.All() {
		r := u.libNavRow(p)
		u.libRows = append(u.libRows, r)
		u.libNavBox.InsertWidget(u.libNavBox.Count()-1, r.QWidget)
	}
	u.syncLibraryNav(u.curPage)
}

type libRow struct {
	*qt.QWidget
	id    string
	d     *deco
	cover *CoverView
}

func (u *UI) libNavRow(p *library.Playlist) *libRow {
	r := &libRow{QWidget: qt.NewQWidget2(), id: p.ID}
	r.SetObjectName("LibraryItem")
	r.SetAttribute(qt.WA_StyledBackground)
	setProp(r.QWidget, "active", "false")
	r.d = u.newDeco(r.QWidget, "QWidget")
	h := hbox(r.QWidget, []int{6, 5, 6, 5}, 10)
	r.cover = u.coverView("LibraryItemCover", false)
	r.cover.SetFixedSize2(40, 40)
	if p.ID == library.LikedID {
		r.cover.Icon = "heart"
		r.cover.SetPixmap(nil)
	} else {
		u.coverLoader.Into(p.CoverURL(), r.cover)
	}
	h.AddWidget(r.cover.QWidget)
	t := qt.NewQVBoxLayout2()
	t.SetSpacing(0)
	name := p.Name
	if p.ID == library.LikedID {
		name = tr("Liked songs")
	}
	tl := label(name, "", "row-title")
	sub := plural(len(p.Tracks), "{n} song", "{n} songs")
	if p.Kind == library.KindRemote {
		if pl := u.core.Plugins.Get(p.Source); pl != nil {
			sub += " · " + pl.Manifest.Name
		}
	}
	t.AddWidget(tl.QWidget)
	t.AddWidget(label(sub, "", "row-sub").QWidget)
	h.AddLayout2(t.QLayout, 1)
	handCursor(r.QWidget)
	r.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { r.d.setHover(true) })
	r.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { r.d.setHover(false) })
	r.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		super(e)
		pa := qt.NewQPainter2(r.QPaintDevice)
		pa.SetRenderHint(qt.QPainter__Antialiasing)
		r.d.paintBg(pa, 10)
		pa.End()
		pa.Delete()
	})
	id := p.ID
	r.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if e.Button() == qt.RightButton {
			u.playlistMenu(id)
			return
		}
		u.closeFull()
		u.openPlaylist(id)
	})
	r.OnMouseDoubleClickEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if pl := u.core.Library.Get(id); pl != nil && len(pl.Tracks) > 0 {
			u.PlayList(pl.Tracks, 0)
		}
	})
	return r
}

func (u *UI) syncLibraryNav(page string) {
	open := ""
	if page == "playlist" && u.lib != nil {
		open = u.lib.openID
	}
	for _, r := range u.libRows {
		on := r.id == open
		if r.d.active != on {
			setProp(r.QWidget, "active", boolStr(on))
			r.d.setActive(on)
		}
	}
	if u.pageTitle != nil {
		u.pageTitle.SetText(u.pageTitles[page])
	}
}

// ---------------------------------------------------------------- now-playing card

// buildNowCard is the player in the left panel: round cover, title by
// artist, a thin seek bar and the buttons (previous, play, next, like,
// lyrics, queue, volume).
func (u *UI) buildNowCard() *qt.QWidget {
	f := frame("NowCard")
	u.nowCard = f.QWidget
	b := vbox(f.QWidget, []int{12, 12, 12, 10}, 8)
	top := qt.NewQHBoxLayout2()
	top.SetSpacing(12)
	cover := u.coverView("NowCardCover", false)
	cover.SetFixedSize2(58, 58)
	cover.OnClick = u.toggleFull
	cover.SetToolTip(tr("Open the player (Ctrl+L)"))
	handCursor(cover.QWidget)
	u.covers = append(u.covers, cover)
	top.AddWidget(cover.QWidget)
	tv := qt.NewQVBoxLayout2()
	tv.SetSpacing(1)
	t := label(tr("Nothing playing"), "NowCardTitle", "")
	a := label(tr("Pick a song to start"), "NowCardArtist", "")
	u.titleLbls = append(u.titleLbls, t)
	u.artistLbls = append(u.artistLbls, a)
	tv.AddStretch()
	tv.AddWidget(t.QWidget)
	tv.AddWidget(a.QWidget)
	tv.AddStretch()
	top.AddLayout2(tv.QLayout, 1)
	top.AddWidget(u.likeButton("NowCardLike").QWidget)
	b.AddLayout(top.QLayout)

	s := u.slider("NowCardSeek")
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
	u.seeks = append(u.seeks, s)
	b.AddWidget(s.QWidget)

	row := qt.NewQHBoxLayout2()
	row.SetSpacing(2)
	prev := u.iconButton("NowCardPrev", "prev", tr("Previous (Ctrl+←)"))
	prev.OnClicked(u.Prev)
	play := u.iconButton("NowCardPlay", "play", tr("Play / pause (Ctrl+Space)"))
	play.OnClicked(u.TogglePlay)
	next := u.iconButton("NowCardNext", "next", tr("Next (Ctrl+→)"))
	next.OnClicked(func() { u.Next(true) })
	u.playBtns = append(u.playBtns, play)
	lyr := u.iconButton("NowCardLyrics", "lyrics", tr("Lyrics and fullscreen player (Ctrl+L)"))
	lyr.OnClicked(u.toggleFull)
	que := u.iconButton("NowCardQueue", "queue", tr("Queue"))
	que.OnClicked(func() { u.closeFull(); u.goPage("queue") })
	vol := u.iconButton("NowCardVolume", "volume", tr("Volume: click to mute, scroll to change"))
	last := u.cfg.Volume
	vol.OnClicked(func() {
		if u.cfg.Volume > 0 {
			last = u.cfg.Volume
			u.setVolume(0)
		} else {
			u.setVolume(max(last, 10))
		}
	})
	vol.OnWheelEvent(func(super func(*qt.QWheelEvent), e *qt.QWheelEvent) {
		d := e.AngleDelta()
		u.setVolume(u.cfg.Volume + float64(d.Y())/120*5)
		vol.SetToolTip(tr("Volume: {n}%", "n", itoa(int(u.cfg.Volume))))
		_ = u.cfg.Save()
	})
	u.volBtns = append(u.volBtns, vol)
	for _, x := range []*IconButton{prev, play, next} {
		row.AddWidget(x.QWidget)
	}
	row.AddStretch()
	for _, x := range []*IconButton{lyr, que, vol} {
		row.AddWidget(x.QWidget)
	}
	b.AddLayout(row.QLayout)
	u.setVolume(u.cfg.Volume)
	return f.QWidget
}
