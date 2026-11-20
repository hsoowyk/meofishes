package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/player"
	"meofishes/internal/plugin"
)

func (u *UI) buildPages() *qt.QWidget {
	u.pages = qt.NewQStackedWidget2()
	u.pages.SetObjectName("Pages")
	add := func(id string, w *qt.QWidget) {
		w.SetObjectName(strings.ToUpper(id[:1]) + id[1:] + "Page")
		u.pageIndex[id] = u.pages.AddWidget(w)
	}
	u.home = u.buildHome()
	add("home", u.home.w)
	u.search = u.buildSearch()
	add("search", u.search.w)
	add("queue", u.buildQueuePage())
	add("lyrics", u.buildLyricsPage())
	u.plugPg = u.buildPluginsPage()
	add("plugins", u.plugPg.w.QWidget)
	u.themePg = u.buildThemesPage()
	add("themes", u.themePg.w.QWidget)
	u.setPg = u.buildSettingsPage()
	add("settings", u.setPg.w.QWidget)
	return u.pages.QWidget
}

func pageShell(title, subtitle string) (*qt.QWidget, *qt.QVBoxLayout) {
	w := qt.NewQWidget2()
	b := vbox(w, []int{34, 0, 26, 0}, 4)
	b.AddWidget(label(title, "", "title").QWidget)
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

func (u *UI) wireList(l *TrackList) {
	l.OnPlay = u.PlayList
	l.OnQueue = u.Enqueue
	l.OnRemove = func(i int) {
		a := u.app
		if i < 0 || i >= len(a.queue) {
			return
		}
		a.queue = append(a.queue[:i], a.queue[i+1:]...)
		if i < a.qIndex {
			a.qIndex--
		} else if i == a.qIndex {
			if a.mpv != nil {
				_ = a.mpv.Stop()
			}
			if a.qIndex >= len(a.queue) {
				a.qIndex = len(a.queue) - 1
			}
			if a.qIndex >= 0 {
				u.playCurrent()
			}
		}
		u.refreshQueue()
	}
	if l.QueueMode {
		l.OnPlay = func(_ []core.Track, i int) {
			u.emitEnd()
			u.app.qIndex = i
			u.playCurrent()
			u.refreshQueue()
		}
	}
	u.allLists = append(u.allLists, l)
}

// sourceCombo lists the enabled sources ("*" = all).
func (u *UI) fillSources(cb *qt.QComboBox, withAll bool) {
	prev := ""
	if cb.Count() > 0 {
		v := cb.CurrentData()
		prev = v.ToString()
	}
	if prev == "" {
		prev = u.app.cfg.Source
	}
	cb.Clear()
	if withAll {
		v := qt.NewQVariant14("*")
		cb.AddItem3("Todas as fontes", v)
		v.Delete()
	}
	sel := 0
	for _, p := range u.app.plugins.Sources() {
		v := qt.NewQVariant14(p.Manifest.ID)
		cb.AddItem3(p.Manifest.Name, v)
		v.Delete()
		if p.Manifest.ID == prev {
			sel = cb.Count() - 1
		}
	}
	cb.SetCurrentIndex(sel)
}

func comboValue(cb *qt.QComboBox) string {
	return cb.CurrentData().ToString()
}

// ---------------------------------------------------------------- home

type homePage struct {
	w       *qt.QWidget
	greet   *qt.QLabel
	sources *qt.QComboBox
	list    *TrackList
	u       *UI
}

func greeting() string {
	h := time.Now().Hour()
	switch {
	case h < 5:
		return "Boa madrugada"
	case h < 12:
		return "Bom dia"
	case h < 18:
		return "Boa tarde"
	}
	return "Boa noite"
}

func (u *UI) buildHome() *homePage {
	hp := &homePage{u: u}
	hp.w = qt.NewQWidget2()
	b := vbox(hp.w, []int{34, 0, 26, 0}, 6)
	hp.greet = label(greeting(), "Greeting", "")
	b.AddWidget(hp.greet.QWidget)
	row := qt.NewQHBoxLayout2()
	sub := label("Destaques da fonte", "", "subtitle")
	row.AddWidget2(sub.QWidget, 1)
	hp.sources = qt.NewQComboBox2()
	hp.sources.SetObjectName("HomeSource")
	hp.sources.SetMinimumWidth(220)
	row.AddWidget(hp.sources.QWidget)
	b.AddLayout(row.QLayout)
	hp.list = u.trackList("HomeList")
	u.wireList(hp.list)
	b.AddWidget2(hp.list.QWidget, 1)
	u.fillSources(hp.sources, false)
	hp.sources.OnCurrentIndexChanged(func(int) {
		if hp.sources.Count() > 0 {
			u.app.cfg.Source = comboValue(hp.sources)
			hp.refresh()
		}
	})
	u.app.plugins.OnChange(func() {
		// may run on a worker goroutine (installs): hop to the UI thread
		mainthread.Start(func() {
			u.fillSources(hp.sources, false)
			if u.search != nil {
				u.fillSources(u.search.sources, true)
			}
		})
	})
	return hp
}

func (hp *homePage) refresh() {
	u := hp.u
	hp.greet.SetText(greeting())
	id := comboValue(hp.sources)
	p := u.app.plugins.Get(id)
	if p == nil {
		hp.list.SetTracks(nil, "Nenhuma fonte de música ativa. Instale ou ative um plugin em Plugins.")
		return
	}
	br := p.Browser()
	if br == nil {
		hp.list.SetTracks(nil, "Esta fonte não tem página inicial. Use a busca.")
		return
	}
	hp.list.SetTracks(nil, "Carregando…")
	var res *core.SearchResult
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		var err error
		res, err = br.Home(ctx)
		return err
	}, func(err error) {
		if comboValue(hp.sources) != id {
			return
		}
		switch {
		case err != nil:
			hp.list.SetTracks(nil, "Erro: "+err.Error())
		case res == nil || len(res.Tracks) == 0:
			hp.list.SetTracks(nil, "Nada por aqui ainda. Tente a busca (Ctrl+F).")
		default:
			hp.list.SetTracks(res.Tracks, "")
		}
	})
}

// ---------------------------------------------------------------- search

type searchPage struct {
	w       *qt.QWidget
	box     *qt.QLineEdit
	sources *qt.QComboBox
	list    *TrackList
	info    *qt.QLabel
	seq     int
}

func (u *UI) buildSearch() *searchPage {
	sp := &searchPage{}
	w, b := pageShell("Buscar", "")
	sp.w = w
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(10)
	sp.box = qt.NewQLineEdit2()
	sp.box.SetObjectName("SearchBox")
	sp.box.SetPlaceholderText("Músicas, artistas, rádios…")
	row.AddWidget2(sp.box.QWidget, 1)
	sp.sources = qt.NewQComboBox2()
	sp.sources.SetObjectName("SourcePicker")
	sp.sources.SetMinimumWidth(220)
	row.AddWidget(sp.sources.QWidget)
	b.AddLayout(row.QLayout)
	sp.info = label("", "SearchInfo", "hint")
	b.AddWidget(sp.info.QWidget)
	sp.list = u.trackList("SearchList")
	u.wireList(sp.list)
	b.AddWidget2(sp.list.QWidget, 1)
	u.fillSources(sp.sources, true)
	sp.list.SetTracks(nil, "Digite algo e aperte Enter.")
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
	var targets []*plugin.Plugin
	for _, p := range u.app.plugins.Sources() {
		if src == "*" || p.Manifest.ID == src {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		sp.list.SetTracks(nil, "Nenhuma fonte ativa.")
		return
	}
	sp.list.SetTracks(nil, "Buscando…")
	sp.info.SetText("")
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
				r, err := p.Source().Search(ctx, q)
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
		sp.list.SetTracks(all, "Nada encontrado para “"+q+"”.")
	})
}

// ---------------------------------------------------------------- queue & lyrics pages

func (u *UI) buildQueuePage() *qt.QWidget {
	w, b := pageShell("Fila", "O que vem a seguir. Duplo clique para pular para uma música.")
	row := qt.NewQHBoxLayout2()
	clear := qt.NewQPushButton3("Limpar fila")
	clear.OnClicked(func() {
		a := u.app
		if cur := a.current(); cur != nil {
			a.queue = []core.Track{*cur}
			a.qIndex = 0
		} else {
			a.queue, a.qIndex = nil, -1
		}
		u.refreshQueue()
	})
	row.AddWidget(clear.QWidget)
	row.AddStretch()
	b.AddLayout(row.QLayout)
	q := u.trackList("QueueList")
	q.QueueMode = true
	u.wireList(q)
	u.queueLists = append(u.queueLists, q)
	b.AddWidget2(q.QWidget, 1)
	q.SetTracks(nil, "A fila está vazia.")
	return w
}

func (u *UI) buildLyricsPage() *qt.QWidget {
	w := qt.NewQWidget2()
	b := vbox(w, []int{0, 0, 0, 12}, 4)
	lv := u.lyricsView("LyricsView")
	u.addLyricsView(lv)
	b.AddWidget2(lv.QWidget, 1)
	row := qt.NewQHBoxLayout2()
	row.SetContentsMargins(48, 0, 26, 0)
	st := label("", "LyricsStatus", "")
	st.SetWordWrap(true)
	u.lyricStatus = append(u.lyricStatus, st)
	row.AddWidget2(st.QWidget, 1)
	retry := qt.NewQPushButton3("Buscar de novo")
	retry.SetToolTip("Ignora o cache e consulta os provedores outra vez")
	retry.OnClicked(func() {
		if cur := u.app.current(); cur != nil {
			gen := u.app.gen
			track := *cur
			u.setLyrics(nil, "Procurando letra…")
			var l *core.Lyrics
			var info string
			u.runAsync(func() error {
				var err error
				l, info, err = u.app.plugins.FindLyricsFresh(context.Background(), track)
				return err
			}, func(err error) {
				if gen != u.app.gen {
					return
				}
				u.app.lyricsInfo = info
				if err != nil {
					u.setLyrics(nil, "Sem letra para esta música")
				} else {
					u.setLyrics(l, "")
				}
				u.toast(info)
			})
		}
	})
	row.AddWidget(retry.QWidget)
	b.AddLayout(row.QLayout)
	return w
}

// ---------------------------------------------------------------- settings

type settingsPage struct {
	w        *qt.QScrollArea
	email    *qt.QLineEdit
	url      *qt.QLineEdit
	catalogs *qt.QPlainTextEdit
	order    *qt.QVBoxLayout
	orderBox *qt.QWidget
	info     *qt.QLabel
	logs     *qt.QPlainTextEdit
	u        *UI
}

func (u *UI) buildSettingsPage() *settingsPage {
	sp := &settingsPage{u: u}
	sa, b := scrollPage("Ajustes", "")
	sp.w = sa

	b.AddWidget(label("Contato (User-Agent)", "", "heading").QWidget)
	hint := label("Alguns provedores pedem que o app se identifique. O lrc.red exige nome do app, um link e um e-mail de contato; "+
		"o Unison pede nome e link. Sem e-mail, o lrc.red fica desligado.", "", "hint")
	hint.SetWordWrap(true)
	b.AddWidget(hint.QWidget)
	sp.email = qt.NewQLineEdit2()
	sp.email.SetPlaceholderText("seu e-mail de contato")
	sp.url = qt.NewQLineEdit2()
	sp.url.SetPlaceholderText("link do app/projeto (ex.: https://github.com/voce/meofishes)")
	b.AddWidget(sp.email.QWidget)
	b.AddWidget(sp.url.QWidget)
	ua := label("", "UserAgentPreview", "hint")
	b.AddWidget(ua.QWidget)
	saveContact := qt.NewQPushButton3("Salvar contato")
	role(saveContact.QWidget, "primary")
	saveContact.OnClicked(func() {
		u.app.cfg.ContactEmail = strings.TrimSpace(sp.email.Text())
		u.app.cfg.ContactURL = strings.TrimSpace(sp.url.Text())
		u.app.host.UserAgent = u.app.cfg.UserAgent()
		_ = u.app.cfg.Save()
		ua.SetText("User-Agent: " + u.app.host.UserAgent)
		u.toast("Contato salvo")
	})
	b.AddWidget3(saveContact.QWidget, 0, qt.AlignLeft)

	b.AddWidget(label("Prioridade das letras", "", "heading").QWidget)
	h2 := label("Ordem em que os plugins de letras são consultados. Vence o primeiro com sincronia por palavra. "+
		"A ordem dos provedores dentro do Meo Lyrics é configurada no próprio plugin.", "", "hint")
	h2.SetWordWrap(true)
	b.AddWidget(h2.QWidget)
	sp.orderBox = qt.NewQWidget2()
	sp.order = vbox(sp.orderBox, nil, 6)
	b.AddWidget(sp.orderBox)

	b.AddWidget(label("Catálogos de temas e plugins", "", "heading").QWidget)
	h3 := label("URLs de index.json extras (uma por linha). O catálogo oficial já vem embutido.", "", "hint")
	b.AddWidget(h3.QWidget)
	sp.catalogs = qt.NewQPlainTextEdit2()
	sp.catalogs.SetMaximumHeight(110)
	sp.catalogs.SetPlaceholderText("https://exemplo.com/meofishes/index.json")
	b.AddWidget(sp.catalogs.QWidget)
	saveCat := qt.NewQPushButton3("Salvar catálogos")
	saveCat.OnClicked(func() {
		var list []string
		for _, l := range strings.Split(sp.catalogs.ToPlainText(), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				list = append(list, l)
			}
		}
		u.app.cfg.Catalogs = list
		u.app.catalog.Refresh()
		_ = u.app.cfg.Save()
		u.toast("Catálogos salvos")
	})
	b.AddWidget3(saveCat.QWidget, 0, qt.AlignLeft)

	b.AddWidget(label("Sobre", "", "heading").QWidget)
	sp.info = label("", "AboutInfo", "hint")
	sp.info.SetWordWrap(true)
	sp.info.SetTextInteractionFlags(qt.TextSelectableByMouse)
	b.AddWidget(sp.info.QWidget)

	b.AddWidget(label("Registro", "", "heading").QWidget)
	sp.logs = qt.NewQPlainTextEdit2()
	sp.logs.SetReadOnly(true)
	sp.logs.SetMinimumHeight(200)
	b.AddWidget(sp.logs.QWidget)
	b.AddStretch()
	return sp
}

func (sp *settingsPage) refresh() {
	u := sp.u
	a := u.app
	sp.email.SetText(a.cfg.ContactEmail)
	sp.url.SetText(a.cfg.ContactURL)
	sp.catalogs.SetPlainText(strings.Join(a.cfg.Catalogs, "\n"))
	// lyrics order
	clearLayout(sp.order.QLayout)
	lps := a.plugins.LyricsPlugins()
	for i, p := range lps {
		i, p := i, p
		row := qt.NewQWidget2()
		h := hbox(row, nil, 8)
		h.AddWidget2(label(fmt.Sprintf("%d. %s", i+1, p.Manifest.Name), "", "card-title").QWidget, 1)
		up := qt.NewQPushButton3("↑")
		down := qt.NewQPushButton3("↓")
		up.SetEnabled(i > 0)
		down.SetEnabled(i < len(lps)-1)
		move := func(d int) {
			ids := make([]string, len(lps))
			for k, x := range lps {
				ids[k] = x.Manifest.ID
			}
			ids[i], ids[i+d] = ids[i+d], ids[i]
			a.cfg.Plugins.LyricsOrder = ids
			_ = a.cfg.Save()
			sp.refresh()
		}
		up.OnClicked(func() { move(-1) })
		down.OnClicked(func() { move(1) })
		h.AddWidget(up.QWidget)
		h.AddWidget(down.QWidget)
		sp.order.AddWidget(row)
	}
	if len(lps) == 0 {
		sp.order.AddWidget(label("Nenhum plugin de letras ativo.", "", "hint").QWidget)
	}
	yt := "não encontrado"
	if p := plugin.YtdlpPath(); p != "" {
		yt = p
	}
	mpv := "não encontrado"
	if player.Available() {
		mpv, _ = exec.LookPath("mpv")
	}
	sp.info.SetText(fmt.Sprintf("meofishes %s\nConfiguração: %s\nPlugins: %s\nTemas: %s\nCache: %s\nmpv: %s\nyt-dlp: %s\nMódulos Go para plugins: %s",
		config.Version, a.paths.Config, a.paths.Plugins, a.paths.Themes, a.paths.Cache, mpv, yt, strings.Join(plugin.ModuleNames(), ", ")))
	a.logMu.Lock()
	logs := append([]string(nil), a.logs...)
	a.logMu.Unlock()
	sp.logs.SetPlainText(strings.Join(logs, "\n"))
}

// clearLayout deletes every widget of a layout.
func clearLayout(l *qt.QLayout) {
	for l.Count() > 0 {
		it := l.TakeAt(0)
		if w := it.Widget(); w != nil {
			w.Hide()
			w.DeleteLater()
		}
		it.Delete()
	}
}
