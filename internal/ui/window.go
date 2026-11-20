package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/plugin"
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
	return f
}

func (u *UI) build() {
	u.win = qt.NewQMainWindow2()
	u.win.SetObjectName("MainWindow")
	u.win.SetWindowTitle("meofishes")
	w, h := u.app.cfg.WindowW, u.app.cfg.WindowH
	if w < 800 || h < 500 {
		w, h = 1360, 860
	}
	u.win.Resize(w, h)
	u.win.SetMinimumSize2(900, 600)

	u.root = u.backdrop()
	u.win.SetCentralWidget(u.root.QWidget)
	vbox(u.root.QWidget, nil, 0)
	u.holder = qt.NewQWidget2()
	u.holder.Hide()

	u.regions["sidebar"] = u.buildSidebar()
	u.regions["pages"] = u.buildPages()
	u.regions["nowplaying"] = u.buildNowPlaying()
	u.regions["playerbar"] = u.buildPlayerBar()
	u.regions["lyrics"] = u.buildLyricsPanel()
	u.regions["queue"] = u.buildQueuePanel()
	u.regions["cover"] = u.buildCoverRegion()
	for _, r := range u.regions {
		r.SetParent(u.holder)
	}
	u.applyLayout()
	u.toastW = u.newToast(u.root.QWidget)

	u.win.OnCloseEvent(func(super func(*qt.QCloseEvent), e *qt.QCloseEvent) {
		u.app.cfg.WindowW, u.app.cfg.WindowH = u.win.Width(), u.win.Height()
		_ = u.app.cfg.Save()
		super(e)
	})
	u.shortcuts()
	u.goPage("home")
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
	add("Ctrl+Right", func() { u.Next(true) })
	add("Ctrl+Left", u.Prev)
	add("Ctrl+F", func() { u.goPage("search"); u.search.box.SetFocus() })
	add("Ctrl+L", func() { u.goPage("lyrics") })
	add("Ctrl+Q", func() { u.win.Close() })
	add("F5", func() { u.applyTheme(false); u.toast("Tema recarregado") })
}

// applyLayout (re)builds the window from the active layout tree.
func (u *UI) applyLayout() {
	tree, id := u.app.plugins.ActiveLayout()
	for _, r := range u.regions {
		r.SetParent(u.holder)
	}
	if u.layoutW != nil {
		u.layoutW.Hide()
		u.layoutW.DeleteLater()
	}
	used := map[string]bool{}
	u.layoutW = u.buildNode(tree, used)
	if !used["pages"] { // pages hold plugins/themes/settings: never lose them
		u.layoutW = u.wrapWithPages(u.layoutW, used)
	}
	u.root.Layout().AddWidget(u.layoutW)
	u.layoutW.Show()
	if id != "" {
		u.app.log("layout", "using %s", id)
	}
}

func (u *UI) wrapWithPages(w *qt.QWidget, used map[string]bool) *qt.QWidget {
	sp := qt.NewQSplitter3(qt.Horizontal)
	sp.AddWidget(w)
	sp.AddWidget(u.regions["pages"])
	u.regions["pages"].Show()
	used["pages"] = true
	return sp.QWidget
}

func (u *UI) buildNode(n *plugin.LayoutNode, used map[string]bool) *qt.QWidget {
	var w *qt.QWidget
	switch {
	case n.Region != "":
		if n.Region == "spacer" {
			w = qt.NewQWidget2()
			break
		}
		r, ok := u.regions[n.Region]
		if !ok || used[n.Region] {
			w = qt.NewQWidget2() // unknown or duplicated region: empty space
			break
		}
		used[n.Region] = true
		w = r
		w.Show()
	case n.Split != "":
		orient := qt.Horizontal
		if n.Split == "v" {
			orient = qt.Vertical
		}
		sp := qt.NewQSplitter3(orient)
		sp.SetChildrenCollapsible(false)
		sp.SetHandleWidth(1)
		if n.Name != "" {
			sp.SetObjectName(n.Name)
		}
		for i, c := range n.Children {
			sp.AddWidget(u.buildNode(c, used))
			sp.SetStretchFactor(i, c.Stretch)
		}
		if len(n.Sizes) > 0 {
			sp.SetSizes(n.Sizes)
		}
		w = sp.QWidget
	default:
		w = qt.NewQWidget2()
		if n.Name != "" {
			w.SetObjectName(n.Name)
			w.SetAttribute(qt.WA_StyledBackground)
		}
		sp := 0
		if n.Spacing != nil {
			sp = *n.Spacing
		}
		var box *qt.QBoxLayout
		if n.Box == "h" {
			box = hbox(w, n.Margins, sp).QBoxLayout
		} else {
			box = vbox(w, n.Margins, sp).QBoxLayout
		}
		for _, c := range n.Children {
			box.AddWidget2(u.buildNode(c, used), c.Stretch)
		}
	}
	if n.MinW > 0 {
		w.SetMinimumWidth(n.MinW)
	}
	if n.MaxW > 0 {
		w.SetMaximumWidth(n.MaxW)
	}
	if n.MinH > 0 {
		w.SetMinimumHeight(n.MinH)
	}
	if n.MaxH > 0 {
		w.SetMaximumHeight(n.MaxH)
	}
	return w
}

func (u *UI) enterAll() {
	singleShot(30, func() {
		for _, name := range []string{"sidebar", "nowplaying", "playerbar", "lyrics", "queue", "cover"} {
			if r := u.regions[name]; r.IsVisible() {
				u.Enter(r, "QFrame", 0)
			}
		}
	})
}

// ---------------------------------------------------------------- sidebar

func (u *UI) buildSidebar() *qt.QWidget {
	f := frame("Sidebar")
	b := vbox(f.QWidget, nil, 0)
	b.AddWidget(label("meofishes", "Logo", "").QWidget)
	b.AddWidget(label("música por plugins", "LogoSub", "").QWidget)
	b.AddWidget(label("NAVEGAR", "", "section").QWidget)
	items := []struct{ id, icon, text string }{
		{"home", "home", "Início"}, {"search", "search", "Buscar"}, {"queue", "queue", "Fila"}, {"lyrics", "lyrics", "Letras"},
	}
	for _, it := range items {
		it := it
		nb := u.navButton("Nav"+it.id, it.icon, it.text)
		nb.OnClicked(func() { u.goPage(it.id) })
		u.nav[it.id] = nb
		b.AddWidget(nb.QWidget)
	}
	b.AddWidget(label("PERSONALIZAR", "", "section").QWidget)
	for _, it := range []struct{ id, icon, text string }{
		{"plugins", "plugin", "Plugins"}, {"themes", "theme", "Temas"}, {"settings", "settings", "Ajustes"},
	} {
		it := it
		nb := u.navButton("Nav"+it.id, it.icon, it.text)
		nb.OnClicked(func() { u.goPage(it.id) })
		u.nav[it.id] = nb
		b.AddWidget(nb.QWidget)
	}
	b.AddStretch()
	footer := label("v0.1", "SidebarFooter", "")
	b.AddWidget(footer.QWidget)
	u.style.OnChange(func() {
		if t := u.currentTheme(); t != nil {
			footer.SetText("v0.1 · tema " + t.Name)
		}
	})
	return f.QWidget
}

// goPage switches the stacked pages with the -meo-transition animation.
func (u *UI) goPage(id string) {
	idx, ok := u.pageIndex[id]
	if !ok {
		return
	}
	for k, b := range u.nav {
		b.SetActive(k == id)
	}
	if u.curPage == id {
		return
	}
	u.curPage = id
	u.pages.SetCurrentIndex(idx)
	page := u.pages.Widget(idx)
	p := u.style.For(u.pages.QWidget, "QStackedWidget")
	mode := p.Str("transition", "fade")
	if mode != "none" {
		dur := time.Duration(p.Ms("transition-duration", 300)) * time.Millisecond
		u.animateIn(page, mode, dur, 0, p.Str("transition-easing", "out-cubic"), p.Len("transition-distance", 24), nil)
	}
	switch id {
	case "plugins":
		u.plugPg.refresh()
	case "themes":
		u.themePg.refresh()
	case "settings":
		u.setPg.refresh()
	}
}

// ---------------------------------------------------------------- now playing

func (u *UI) buildNowPlaying() *qt.QWidget {
	f := frame("NowPlaying")
	b := vbox(f.QWidget, []int{26, 28, 26, 18}, 10)
	cover := u.coverView("Cover", true)
	cover.SetMinimumSize2(200, 200)
	cover.OnClick = func() { u.goPage("lyrics") }
	u.covers = append(u.covers, cover)
	b.AddWidget(cover.QWidget)
	b.AddSpacing(8)
	t := label("Nada tocando", "NowTitle", "")
	t.SetWordWrap(true)
	ar := label("Escolha uma música para começar", "NowArtist", "")
	ar.SetWordWrap(true)
	al := label("", "NowAlbum", "")
	u.titleLbls = append(u.titleLbls, t)
	u.artistLbls = append(u.artistLbls, ar)
	u.albumLbls = append(u.albumLbls, al)
	b.AddWidget(t.QWidget)
	b.AddWidget(ar.QWidget)
	b.AddWidget(al.QWidget)
	b.AddSpacing(6)
	lv := u.lyricsView("MiniLyrics")
	u.addLyricsView(lv)
	b.AddWidget2(lv.QWidget, 1)
	st := label("", "MiniLyricsStatus", "")
	u.lyricStatus = append(u.lyricStatus, st)
	b.AddWidget(st.QWidget)
	return f.QWidget
}

func (u *UI) addLyricsView(lv *LyricsView) {
	lv.Pos = func() int64 { return u.state().PositionMs }
	lv.OnSeek = u.SeekTo
	u.lyricViews = append(u.lyricViews, lv)
}

func (u *UI) buildLyricsPanel() *qt.QWidget {
	f := frame("LyricsPanel")
	b := vbox(f.QWidget, []int{20, 20, 20, 12}, 6)
	lv := u.lyricsView("PanelLyrics")
	u.addLyricsView(lv)
	b.AddWidget2(lv.QWidget, 1)
	st := label("", "LyricsStatus", "")
	u.lyricStatus = append(u.lyricStatus, st)
	b.AddWidget(st.QWidget)
	return f.QWidget
}

func (u *UI) buildQueuePanel() *qt.QWidget {
	f := frame("QueuePanel")
	b := vbox(f.QWidget, []int{14, 16, 8, 8}, 6)
	b.AddWidget(label("Fila", "", "heading").QWidget)
	q := u.trackList("QueuePanelList")
	q.QueueMode = true
	u.wireList(q)
	u.queueLists = append(u.queueLists, q)
	b.AddWidget2(q.QWidget, 1)
	return f.QWidget
}

func (u *UI) buildCoverRegion() *qt.QWidget {
	f := frame("BigCoverRegion")
	b := vbox(f.QWidget, nil, 10)
	c := u.coverView("BigCover", true)
	c.SetMinimumSize2(180, 180)
	u.covers = append(u.covers, c)
	b.AddWidget(c.QWidget)
	t := label("", "NowTitle", "")
	t.SetWordWrap(true)
	ar := label("", "NowArtist", "")
	u.titleLbls = append(u.titleLbls, t)
	u.artistLbls = append(u.artistLbls, ar)
	b.AddWidget(t.QWidget)
	b.AddWidget(ar.QWidget)
	return f.QWidget
}

// ---------------------------------------------------------------- player bar

func (u *UI) buildPlayerBar() *qt.QWidget {
	f := frame("PlayerBar")
	b := hbox(f.QWidget, []int{18, 10, 22, 10}, 14)

	// left: cover + title
	left := qt.NewQWidget2()
	left.SetMinimumWidth(220)
	left.SetMaximumWidth(340)
	lb := hbox(left, nil, 12)
	pc := u.coverView("PlayerCover", false)
	pc.SetFixedSize2(56, 56)
	pc.OnClick = func() { u.goPage("lyrics") }
	u.covers = append(u.covers, pc)
	lb.AddWidget(pc.QWidget)
	tv := qt.NewQVBoxLayout2()
	tv.SetSpacing(2)
	bt := label("meofishes", "BarTitle", "")
	ba := label("", "BarArtist", "")
	u.titleLbls = append(u.titleLbls, bt)
	u.artistLbls = append(u.artistLbls, ba)
	tv.AddStretch()
	tv.AddWidget(bt.QWidget)
	tv.AddWidget(ba.QWidget)
	tv.AddStretch()
	lb.AddLayout2(tv.QLayout, 1)
	b.AddWidget(left)

	// center: controls + seek
	center := qt.NewQWidget2()
	cv := vbox(center, nil, 2)
	ctr := qt.NewQHBoxLayout2()
	ctr.SetSpacing(10)
	ctr.AddStretch()
	u.shuffleBtn = u.iconButton("Shuffle", "shuffle", "Aleatório")
	u.shuffleBtn.OnClicked(func() {
		u.app.cfg.Shuffle = !u.app.cfg.Shuffle
		u.shuffleBtn.SetActive(u.app.cfg.Shuffle)
		if u.app.cfg.Shuffle && u.app.qIndex >= 0 {
			u.shuffleRest()
			u.refreshQueue()
		}
	})
	prev := u.iconButton("Prev", "prev", "Anterior (Ctrl+←)")
	prev.OnClicked(u.Prev)
	u.playBtn = u.iconButton("Play", "play", "Tocar/pausar (Ctrl+Espaço)")
	u.playBtn.OnClicked(u.TogglePlay)
	next := u.iconButton("Next", "next", "Próxima (Ctrl+→)")
	next.OnClicked(func() { u.Next(true) })
	u.repeatBtn = u.iconButton("Repeat", "repeat", "Repetir")
	u.repeatBtn.OnClicked(func() {
		switch u.app.cfg.Repeat {
		case "off", "":
			u.app.cfg.Repeat = "all"
		case "all":
			u.app.cfg.Repeat = "one"
		default:
			u.app.cfg.Repeat = "off"
		}
		u.syncRepeat()
	})
	for _, x := range []*IconButton{u.shuffleBtn, prev, u.playBtn, next, u.repeatBtn} {
		ctr.AddWidget(x.QWidget)
	}
	ctr.AddStretch()
	cv.AddLayout(ctr.QLayout)
	sk := qt.NewQHBoxLayout2()
	sk.SetSpacing(10)
	u.tElapsed = label("0:00", "TimeElapsed", "")
	u.tElapsed.SetAlignment(qt.AlignRight | qt.AlignVCenter)
	u.tTotal = label("—", "TimeTotal", "")
	u.seek = u.slider("SeekBar")
	u.seek.OnSeek = func(v float64) {
		dur := u.state().DurationMs
		if cur := u.app.current(); dur == 0 && cur != nil {
			dur = cur.DurationMs
		}
		if dur > 0 {
			u.SeekTo(int64(v * float64(dur)))
		}
	}
	sk.AddWidget(u.tElapsed.QWidget)
	sk.AddWidget2(u.seek.QWidget, 1)
	sk.AddWidget(u.tTotal.QWidget)
	cv.AddLayout(sk.QLayout)
	b.AddWidget2(center, 1)

	// right: lyrics, queue, volume
	right := qt.NewQWidget2()
	right.SetMinimumWidth(200)
	rb := hbox(right, nil, 6)
	rb.AddStretch()
	lyr := u.iconButton("LyricsToggle", "lyrics", "Letras (Ctrl+L)")
	lyr.OnClicked(func() { u.goPage("lyrics") })
	que := u.iconButton("QueueToggle", "queue", "Fila")
	que.OnClicked(func() { u.goPage("queue") })
	u.volBtn = u.iconButton("VolumeIcon", "volume", "Volume")
	lastVol := u.app.cfg.Volume
	u.volBtn.OnClicked(func() {
		if u.app.cfg.Volume > 0 {
			lastVol = u.app.cfg.Volume
			u.setVolume(0)
			u.vol.SetValue(0)
		} else {
			u.setVolume(lastVol)
			u.vol.SetValue(lastVol / 100)
		}
	})
	u.vol = u.slider("VolumeBar")
	u.vol.SetValue(u.app.cfg.Volume / 100)
	u.vol.OnDrag = func(v float64) { u.setVolume(v * 100) }
	u.vol.OnSeek = func(v float64) { u.setVolume(v * 100); _ = u.app.cfg.Save() }
	for _, x := range []*qt.QWidget{lyr.QWidget, que.QWidget, u.volBtn.QWidget, u.vol.QWidget} {
		rb.AddWidget(x)
	}
	b.AddWidget(right)
	u.shuffleBtn.SetActive(u.app.cfg.Shuffle)
	u.syncRepeat()
	return f.QWidget
}

func (u *UI) syncRepeat() {
	switch u.app.cfg.Repeat {
	case "one":
		u.repeatBtn.SetIcon("repeat-one")
		u.repeatBtn.SetActive(true)
	case "all":
		u.repeatBtn.SetIcon("repeat")
		u.repeatBtn.SetActive(true)
	default:
		u.repeatBtn.SetIcon("repeat")
		u.repeatBtn.SetActive(false)
	}
}
