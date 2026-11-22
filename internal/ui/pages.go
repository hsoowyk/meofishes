package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/lyrics"
	"meofishes/internal/plugin"
)

func (u *UI) buildPages() *qt.QWidget {
	u.pages = qt.NewQStackedWidget2()
	u.pages.SetObjectName("Pages")
	add := func(id, title string, w *qt.QWidget) {
		w.SetObjectName(strings.ToUpper(id[:1]) + id[1:] + "Page")
		u.pageIndex[id] = u.pages.AddWidget(w)
		u.pageTitles[id] = title
	}
	u.home = u.buildHome()
	add("home", "", u.home.w.QWidget)
	u.search = u.buildSearch()
	add("search", tr("Search"), u.search.w)
	u.lib = u.buildLibrary()
	add("library", tr("Library"), u.lib.w.QWidget)
	add("playlist", tr("Playlist"), u.lib.detail)
	add("queue", tr("Queue"), u.buildQueuePage())
	u.plugPg = u.buildPluginsPage() // shown inside Settings
	u.themePg = u.buildThemesPage() // shown inside Settings
	u.setPg = u.buildSettingsPage()
	add("settings", tr("Settings"), u.setPg.w)
	u.togetherPg = u.buildTogetherPage()
	add("together", tr("Listen together"), u.togetherPg.w.QWidget)
	u.devPg = u.buildDevPage()
	add("dev", tr("Developer mode"), u.devPg.w)
	return u.pages.QWidget
}

func pageShell(title, subtitle string) (*qt.QWidget, *qt.QVBoxLayout) {
	w := qt.NewQWidget2()
	b := vbox(w, []int{28, 18, 22, 0}, 4)
	if title != "" {
		b.AddWidget(label(title, "", "title").QWidget)
	}
	if subtitle != "" {
		s := label(subtitle, "", "subtitle")
		s.SetWordWrap(true)
		b.AddWidget(s.QWidget)
	}
	return w, b
}

// scrollPage is a page whose content scrolls.
func scrollPage(title, subtitle string) (*qt.QScrollArea, *qt.QVBoxLayout) {
	sa := qt.NewQScrollArea2()
	sa.SetWidgetResizable(true)
	sa.SetFrameShape(qt.QFrame__NoFrame)
	sa.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	inner, b := pageShell(title, subtitle)
	sa.SetWidget(inner)
	return sa, b
}

// fillSources lists the plugins that can search ("*" = all).
func (u *UI) fillSources(cb *qt.QComboBox, withAll bool) {
	prev := ""
	if cb.Count() > 0 {
		prev = cb.CurrentData().ToString()
	}
	if prev == "" {
		prev = u.cfg.Source
	}
	cb.BlockSignals(true)
	cb.Clear()
	if withAll {
		v := qt.NewQVariant14("*")
		cb.AddItem3(tr("All sources"), v)
		v.Delete()
	}
	sel := 0
	if withAll {
		v := qt.NewQVariant14("@lyrics")
		cb.AddItem3(tr("By a line of the lyrics"), v)
		v.Delete()
		if prev == "@lyrics" {
			sel = 1
		}
	}
	for _, p := range u.core.Plugins.Sources() {
		v := qt.NewQVariant14(p.Manifest.ID)
		cb.AddItem3(p.Manifest.Name, v)
		v.Delete()
		if p.Manifest.ID == prev {
			sel = cb.Count() - 1
		}
	}
	cb.SetCurrentIndex(sel)
	cb.BlockSignals(false)
}

func comboValue(cb *qt.QComboBox) string {
	if cb.Count() == 0 {
		return ""
	}
	return cb.CurrentData().ToString()
}

// ---------------------------------------------------------------- cards

// CardRow is a horizontal, scrollable row of big square covers.
type CardRow struct {
	*qt.QScrollArea
	u      *UI
	box    *qt.QHBoxLayout
	cards  []*trackCard
	tracks []core.Track
	empty  *qt.QLabel
	OnPlay func(ts []core.Track, i int)
}

type trackCard struct {
	*qt.QWidget
	cover *CoverView
	d     *deco
}

func (u *UI) cardRow(name string) *CardRow {
	r := &CardRow{QScrollArea: qt.NewQScrollArea2(), u: u, OnPlay: u.PlayList}
	r.SetObjectName(name)
	r.SetWidgetResizable(true)
	r.SetFrameShape(qt.QFrame__NoFrame)
	r.SetVerticalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	r.SetHorizontalScrollBarPolicy(qt.ScrollBarAsNeeded)
	size := u.cardSize()
	r.SetFixedHeight(size + 86)
	inner := qt.NewQWidget2()
	inner.SetObjectName("CardRowInner")
	r.box = hbox(inner, []int{0, 0, 0, 10}, 14)
	r.empty = label("", "", "hint")
	r.box.AddWidget(r.empty.QWidget)
	r.box.AddStretch()
	r.SetWidget(inner)
	// vertical wheel scrolls the row sideways
	r.OnWheelEvent(func(super func(*qt.QWheelEvent), e *qt.QWheelEvent) {
		d := e.AngleDelta()
		sb := r.HorizontalScrollBar()
		if d.X() == 0 && sb.Maximum() > 0 {
			sb.SetValue(sb.Value() - d.Y())
			return
		}
		super(e)
	})
	return r
}

func (u *UI) cardSize() int {
	switch u.cfg.Prefs.GridSize {
	case "small":
		return 120
	case "large":
		return 190
	}
	return 154
}

// SetTracks fills the row.
func (r *CardRow) SetTracks(ts []core.Track, emptyMsg string) {
	for _, c := range r.cards {
		c.cover.dispose()
		c.d.dispose()
		c.Hide()
		c.DeleteLater()
	}
	r.cards = nil
	r.tracks = ts
	r.empty.SetText(emptyMsg)
	r.empty.SetVisible(len(ts) == 0)
	size := r.u.cardSize()
	for i, t := range ts {
		c := r.u.newCard(t, size)
		i := i
		c.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
			if e.Button() == qt.RightButton {
				r.u.trackMenuFor(r.tracks, i, r.OnPlay, nil)
				return
			}
			r.OnPlay(r.tracks, i)
		})
		r.cards = append(r.cards, c)
		r.box.InsertWidget(r.box.Count()-1, c.QWidget)
		key := t.Key()
		r.u.art.Get(t, func(url string) {
			if i < len(r.cards) && i < len(r.tracks) && r.tracks[i].Key() == key {
				r.tracks[i].Cover = url
				r.u.coverLoader.Into(url, r.cards[i].cover)
			}
		})
	}
	singleShot(0, func() {
		for i, c := range r.cards {
			if i > 12 {
				break
			}
			r.u.Enter(c.QWidget, "Card", i)
		}
	})
}

func (u *UI) newCard(t core.Track, size int) *trackCard {
	c := &trackCard{QWidget: qt.NewQWidget2()}
	c.SetObjectName("Card")
	c.SetAttribute(qt.WA_StyledBackground)
	c.SetFixedWidth(size + 16)
	c.d = u.newDeco(c.QWidget, "Card")
	b := vbox(c.QWidget, []int{8, 8, 8, 8}, 4)
	c.cover = u.coverView("CardCover", false)
	c.cover.SetFixedSize2(size, size)
	u.coverLoader.Into(t.Cover, c.cover)
	b.AddWidget(c.cover.QWidget)
	title := label(ellipsis(t.Title, 28), "", "card-title")
	sub := label(ellipsis(t.Artist, 30), "", "card-sub")
	title.SetToolTip(t.String())
	b.AddWidget(title.QWidget)
	b.AddWidget(sub.QWidget)
	handCursor(c.QWidget)
	c.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { c.d.setHover(true); u.hoverWarm(t) })
	c.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { c.d.setHover(false); u.hoverLeave(t) })
	c.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		super(e)
		p := qt.NewQPainter2(c.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		c.d.paintBg(p, 14)
		p.End()
		p.Delete()
	})
	return c
}

// ---------------------------------------------------------------- home

type homePage struct {
	w        *qt.QScrollArea
	greet    *qt.QLabel
	box      *qt.QLineEdit
	recent   *CardRow
	top      *CardRow
	topTitle *qt.QLabel
	recs     *CardRow
	recTitle *qt.QLabel
	popular  *CardRow
	sources  *qt.QComboBox
	popTitle *qt.QLabel
	u        *UI
	seq      int
	lastSeed string
	popFor   string
}

func greeting() string {
	h := time.Now().Hour()
	switch {
	case h < 5:
		return tr("Good night")
	case h < 12:
		return tr("Good morning")
	case h < 18:
		return tr("Good afternoon")
	}
	return tr("Good evening")
}

func (u *UI) buildHome() *homePage {
	hp := &homePage{u: u}
	sa, b := scrollPage("", "")
	hp.w = sa
	b.SetSpacing(8)

	top := qt.NewQHBoxLayout2()
	hp.greet = label("", "Greeting", "")
	top.AddWidget2(hp.greet.QWidget, 1)
	hp.box = qt.NewQLineEdit2()
	hp.box.SetObjectName("HomeSearch")
	hp.box.SetPlaceholderText(tr("Search songs, artists, radios…"))
	hp.box.SetMinimumWidth(260)
	hp.box.SetMaximumWidth(380)
	hp.box.OnReturnPressed(func() {
		q := strings.TrimSpace(hp.box.Text())
		if q == "" {
			return
		}
		u.search.box.SetText(q)
		hp.box.Clear()
		u.goPage("search")
		u.doSearch(u.search)
	})
	top.AddWidget3(hp.box.QWidget, 0, qt.AlignTop)
	b.AddLayout(top.QLayout)
	b.AddSpacing(10)

	b.AddWidget(label(tr("Recently played"), "", "heading").QWidget)
	hp.recent = u.cardRow("RecentRow")
	b.AddWidget(hp.recent.QWidget)

	hp.topTitle = label(tr("Your most played"), "", "heading")
	b.AddWidget(hp.topTitle.QWidget)
	hp.top = u.cardRow("TopRow")
	b.AddWidget(hp.top.QWidget)

	hp.recTitle = label(tr("Recommended for you"), "", "heading")
	b.AddWidget(hp.recTitle.QWidget)
	hp.recs = u.cardRow("RecommendedRow")
	b.AddWidget(hp.recs.QWidget)

	pr := qt.NewQHBoxLayout2()
	hp.popTitle = label(tr("Popular near you"), "", "heading")
	pr.AddWidget2(hp.popTitle.QWidget, 1)
	hp.sources = qt.NewQComboBox2()
	hp.sources.SetObjectName("HomeSource")
	hp.sources.SetMinimumWidth(200)
	pr.AddWidget(hp.sources.QWidget)
	b.AddLayout(pr.QLayout)
	hp.popular = u.cardRow("PopularRow")
	b.AddWidget(hp.popular.QWidget)
	b.AddStretch()

	u.fillSources(hp.sources, false)
	hp.sources.OnCurrentIndexChanged(func(int) {
		if hp.sources.Count() > 0 {
			u.cfg.Source = comboValue(hp.sources)
			hp.loadPopular()
		}
	})
	return hp
}

func (hp *homePage) refresh() {
	u := hp.u
	name := strings.TrimSpace(u.cfg.Prefs.UserName)
	if name != "" {
		hp.greet.SetText(greeting() + ", " + name)
	} else {
		hp.greet.SetText(greeting())
	}
	recent := u.core.Library.Recent(20)
	hp.recent.SetTracks(recent, tr("Nothing yet. Play something!"))
	songs, _, _ := u.core.Library.Top(time.Now().AddDate(0, 0, -30), 20)
	var top []core.Track
	for _, r := range songs {
		if r.Plays > 0 {
			top = append(top, *r.Track)
		}
	}
	hp.top.SetTracks(top, "")
	hp.top.SetVisible(len(top) > 0)
	hp.topTitle.SetVisible(len(top) > 0)
	seed := ""
	if len(recent) > 0 {
		seed = recent[0].Key()
	}
	if seed != hp.lastSeed || len(hp.recs.tracks) == 0 {
		hp.lastSeed = seed
		hp.loadRecommendations(recent)
	}
	if hp.popFor != comboValue(hp.sources) {
		hp.loadPopular()
	}
}

// loadRecommendations asks src plugins for songs like the last played one.
func (hp *homePage) loadRecommendations(recent []core.Track) {
	u := hp.u
	if len(recent) == 0 {
		hp.recs.SetTracks(nil, tr("Play a song to get recommendations."))
		return
	}
	hp.seq++
	seq := hp.seq
	hp.recs.SetTracks(nil, tr("Loading…"))
	var ts []core.Track
	var via string
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		var err error
		ts, via, err = u.core.Plugins.Recommend(ctx, recent[0])
		return err
	}, func(err error) {
		if seq != hp.seq {
			return
		}
		if err != nil {
			hp.recs.SetTracks(nil, tr("No recommendations yet (install a src plugin with recommendations, e.g. Deezer)."))
			return
		}
		name := via
		if p := u.core.Plugins.Get(via); p != nil {
			name = p.Manifest.Name
		}
		hp.recTitle.SetText(tr("Recommended for you") + "  ·  " + name)
		hp.recs.SetTracks(ts, "")
	})
}

func (hp *homePage) loadPopular() {
	u := hp.u
	id := comboValue(hp.sources)
	hp.popFor = id
	p := u.core.Plugins.Get(id)
	if p == nil {
		hp.popular.SetTracks(nil, tr("No music source enabled. Install or enable a plugin in Plugins."))
		return
	}
	if !p.Can(plugin.CapHome) {
		hp.popular.SetTracks(nil, tr("This source has no home page. Use the search."))
		return
	}
	hp.popular.SetTracks(nil, tr("Loading…"))
	var res *core.SearchResult
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		var err error
		res, err = u.core.Plugins.Home(ctx, p)
		return err
	}, func(err error) {
		if comboValue(hp.sources) != id {
			return
		}
		switch {
		case err != nil:
			hp.popular.SetTracks(nil, tr("Error: {error}", "error", err.Error()))
		case res == nil || len(res.Tracks) == 0:
			hp.popular.SetTracks(nil, tr("Nothing here yet."))
		default:
			hp.popular.SetTracks(res.Tracks, "")
		}
	})
}

// ---------------------------------------------------------------- search

type searchPage struct {
	w        *qt.QWidget
	box      *qt.QLineEdit
	sources  *qt.QComboBox
	list     *TrackList
	info     *qt.QLabel
	seq      int
	history  *qt.QHBoxLayout
	historyW *qt.QWidget
	u        *UI
}

// refreshHistory shows the recent searches as chips.
func (sp *searchPage) refreshHistory() {
	u := sp.u
	clearLayout(sp.history.QLayout)
	qs := u.core.Library.RecentSearches()
	sp.historyW.SetVisible(len(qs) > 0)
	if len(qs) > 8 {
		qs = qs[:8]
	}
	sp.history.AddWidget(label(tr("Recent:"), "", "hint").QWidget)
	for _, q := range qs {
		q := q
		b := qt.NewQPushButton3(ellipsis(q, 28))
		role(b.QWidget, "chip-button")
		handCursor(b.QWidget)
		b.OnClicked(func() {
			sp.box.SetText(q)
			u.doSearch(sp)
		})
		sp.history.AddWidget(b.QWidget)
	}
	clear := u.iconButton("SearchHistoryClear", "close", tr("Clear the recent searches"))
	clear.OnClicked(func() { u.core.Library.ClearSearches(); sp.refreshHistory() })
	sp.history.AddWidget(clear.QWidget)
	sp.history.AddStretch()
}

func (u *UI) buildSearch() *searchPage {
	sp := &searchPage{}
	w, b := pageShell(tr("Search"), "")
	sp.w = w
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(10)
	sp.box = qt.NewQLineEdit2()
	sp.box.SetObjectName("SearchBox")
	sp.box.SetPlaceholderText(tr("Songs, artists, radios…"))
	sp.box.SetClearButtonEnabled(true)
	row.AddWidget2(sp.box.QWidget, 1)
	sp.sources = qt.NewQComboBox2()
	sp.sources.SetObjectName("SourcePicker")
	sp.sources.SetMinimumWidth(220)
	row.AddWidget(sp.sources.QWidget)
	b.AddLayout(row.QLayout)
	sp.info = label("", "SearchInfo", "hint")
	b.AddWidget(sp.info.QWidget)
	hw := qt.NewQWidget2()
	hw.SetObjectName("SearchHistory")
	sp.history = hbox(hw, []int{0, 0, 0, 6}, 6)
	sp.historyW = hw
	b.AddWidget(hw)
	sp.u = u
	sp.refreshHistory()
	sp.list = u.trackList("SearchList")
	b.AddWidget2(sp.list.QWidget, 1)
	u.fillSources(sp.sources, true)
	sp.list.SetTracks(nil, tr("Type something and press Enter."))
	run := func() { u.doSearch(sp) }
	sp.box.OnReturnPressed(run)
	sp.sources.OnCurrentIndexChanged(func(int) {
		if strings.TrimSpace(sp.box.Text()) != "" {
			run()
		}
	})
	return sp
}

func (u *UI) doSearch(sp *searchPage) {
	q := strings.TrimSpace(sp.box.Text())
	if q == "" {
		return
	}
	sp.seq++
	seq := sp.seq
	src := comboValue(sp.sources)
	if src == "@lyrics" {
		u.searchLyrics(sp, q, seq)
		return
	}
	var targets []*plugin.Plugin
	for _, p := range u.core.Plugins.Sources() {
		if src == "*" || p.Manifest.ID == src {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		sp.list.SetTracks(nil, tr("No source enabled."))
		return
	}
	sp.list.SetTracks(nil, tr("Searching…"))
	sp.info.SetText("")
	u.core.Library.AddSearch(q)
	sp.refreshHistory()
	type result struct {
		name   string
		tracks []core.Track
		err    error
	}
	results := make([]result, len(targets))
	u.runAsync(func() error {
		var wg sync.WaitGroup
		for i, p := range targets {
			wg.Add(1)
			go func(i int, p *plugin.Plugin) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				r, err := u.core.Plugins.Search(ctx, p, q)
				results[i] = result{name: p.Manifest.Name, err: err}
				if r != nil {
					results[i].tracks = r.Tracks
				}
			}(i, p)
		}
		wg.Wait()
		return nil
	}, func(error) {
		if seq != sp.seq {
			return
		}
		var all []core.Track
		var notes []string
		for _, r := range results {
			if r.err != nil {
				notes = append(notes, r.name+": "+ellipsis(r.err.Error(), 90))
				continue
			}
			all = append(all, r.tracks...)
			if len(targets) > 1 {
				notes = append(notes, fmt.Sprintf("%s: %d", r.name, len(r.tracks)))
			}
		}
		sp.info.SetText(strings.Join(notes, " · "))
		sp.list.SetTracks(all, tr("Nothing found for “{query}”.", "query", q))
		if len(all) > 0 {
			u.warm(all[0]) // the first result is the one most often played
		}
	})
}

// searchLyrics finds songs by a line of their lyrics; they play through
// the "match" plugin (yt-dlp).
func (u *UI) searchLyrics(sp *searchPage, q string, seq int) {
	sp.list.SetTracks(nil, tr("Searching…"))
	sp.info.SetText("")
	u.core.Library.AddSearch(q)
	sp.refreshHistory()
	var hits []lyrics.Hit
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var err error
		hits, err = lyrics.SearchByLyrics(ctx, &lyrics.Env{Client: u.core.Host.Client, UserAgent: config.UserAgent}, q)
		return err
	}, func(err error) {
		if seq != sp.seq {
			return
		}
		if err != nil {
			sp.info.SetText(ellipsis(err.Error(), 120))
		}
		var ts []core.Track
		for _, h := range hits {
			t := core.Track{Source: "lyrics-search", ID: h.Artist + " - " + h.Title, Title: h.Title, Artist: h.Artist, Album: h.Album,
				DurationMs: h.DurationMs, Cover: h.Cover}
			if h.Snippet != "" {
				t.Album = "“" + h.Snippet + "”"
			}
			ts = append(ts, t)
		}
		if len(ts) > 0 {
			sp.info.SetText(tr("Songs with these words (Genius) — they play from YouTube."))
		}
		sp.list.SetTracks(ts, tr("No song has these words."))
	})
}

// ---------------------------------------------------------------- queue

func (u *UI) buildQueuePage() *qt.QWidget {
	w, b := pageShell(tr("Queue"), tr("What comes next. Drag to reorder, double-click to jump, right-click for more."))
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(8)
	clear := qt.NewQPushButton3(tr("Clear queue"))
	clear.OnClicked(u.clearQueue)
	save := qt.NewQPushButton3(tr("Save as playlist"))
	save.OnClicked(func() {
		if len(u.queue) > 0 {
			u.newPlaylistDialog(u.queue)
		}
	})
	shuffle := qt.NewQPushButton3(tr("Shuffle upcoming"))
	shuffle.OnClicked(func() { u.shuffleRest(); u.refreshQueue() })
	for _, x := range []*qt.QPushButton{clear, save, shuffle} {
		row.AddWidget(x.QWidget)
	}
	row.AddStretch()
	b.AddLayout(row.QLayout)
	q := u.trackList("QueueList")
	q.QueueMode = true
	q.Reorderable = true
	q.OnMove = u.MoveInQueue
	q.OnRemove = u.RemoveFromQueue
	q.RemoveLabel = tr("Remove from queue")
	q.OnPlay = func(_ []core.Track, i int) {
		u.emitEnd()
		u.qIndex = i
		u.playCurrent()
		u.refreshQueue()
	}
	u.queueLists = append(u.queueLists, q)
	b.AddWidget2(q.QWidget, 1)
	q.SetTracks(nil, tr("The queue is empty."))
	return w
}

// clearLayout deletes every widget of a layout.
func clearLayout(l *qt.QLayout) {
	for l.Count() > 0 {
		it := l.TakeAt(0)
		if w := it.Widget(); w != nil {
			w.Hide()
			w.DeleteLater()
		} else if sub := it.Layout(); sub != nil {
			clearLayout(sub)
		}
		it.Delete()
	}
}
