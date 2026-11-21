package ui

import (
	"context"
	"fmt"
	"math/rand"
	"meofishes/internal/plugin"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/library"
	"meofishes/internal/offline"
)

type libraryPage struct {
	u      *UI
	w      *qt.QScrollArea
	grid   *qt.QGridLayout
	remote *qt.QVBoxLayout
	// detail page
	detail   *qt.QWidget
	openID   string
	dCover   *CoverView
	dTitle   *qt.QLabel
	dSub     *qt.QLabel
	dList    *TrackList
	dActions *qt.QHBoxLayout
	cards    []*CoverView
	stats    *qt.QVBoxLayout
}

func (u *UI) buildLibrary() *libraryPage {
	lp := &libraryPage{u: u}
	sa, b := scrollPage(tr("Library"), tr("Your playlists, liked songs and the playlists of your plugins."))
	lp.w = sa
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(8)
	newp := qt.NewQPushButton3(tr("New playlist"))
	role(newp.QWidget, "primary")
	newp.OnClicked(func() { u.newPlaylistDialog(nil) })
	imp := qt.NewQPushButton3(tr("Import…"))
	imp.OnClicked(u.importPlaylists)
	exp := qt.NewQPushButton3(tr("Export everything…"))
	exp.OnClicked(func() { u.exportPlaylists(u.core.Library.All()...) })
	link := qt.NewQPushButton3(tr("Import from link…"))
	link.OnClicked(u.importFromLink)
	dls := qt.NewQPushButton3(tr("Downloaded songs"))
	dls.OnClicked(func() { u.openPlaylist(downloadsID) })
	for _, x := range []*qt.QPushButton{newp, imp, link, exp, dls} {
		row.AddWidget(x.QWidget)
	}
	row.AddStretch()
	b.AddLayout(row.QLayout)
	b.AddSpacing(8)
	gw := qt.NewQWidget2()
	lp.grid = qt.NewQGridLayout(gw)
	lp.grid.SetSpacing(14)
	lp.grid.SetContentsMargins(0, 0, 0, 0)
	b.AddWidget(gw)

	b.AddSpacing(10)
	b.AddWidget(label(tr("Your stats"), "", "heading").QWidget)
	sw := qt.NewQWidget2()
	lp.stats = vbox(sw, nil, 4)
	b.AddWidget(sw)

	b.AddSpacing(10)
	b.AddWidget(label(tr("From your plugins"), "", "heading").QWidget)
	hint := label(tr("Playlists of src plugins that support them (for example a plugin for your account on a music service). Opening one saves a copy in your library."), "", "hint")
	hint.SetWordWrap(true)
	b.AddWidget(hint.QWidget)
	load := qt.NewQPushButton3(tr("Load plugin playlists"))
	load.OnClicked(lp.loadRemote)
	b.AddWidget3(load.QWidget, 0, qt.AlignLeft)
	rw := qt.NewQWidget2()
	lp.remote = vbox(rw, nil, 6)
	b.AddWidget(rw)
	b.AddStretch()

	lp.buildDetail()
	return lp
}

func (lp *libraryPage) refresh() {
	u := lp.u
	for _, c := range lp.cards {
		c.dispose()
	}
	lp.cards = nil
	clearLayout(lp.grid.QLayout)
	cols := 4
	if u.cfg.Prefs.GridSize == "small" {
		cols = 5
	} else if u.cfg.Prefs.GridSize == "large" {
		cols = 3
	}
	for i, p := range u.core.Library.All() {
		lp.grid.AddWidget2(lp.playlistCard(p), i/cols, i%cols)
	}
	if lp.openID != "" {
		lp.fillDetail()
	}
	lp.fillStats()
}

// fillStats shows listening time and the top songs and artists.
func (lp *libraryPage) fillStats() {
	u := lp.u
	clearLayout(lp.stats.QLayout)
	_, _, all := u.core.Library.Top(time.Time{}, 0)
	songs, artists, month := u.core.Library.Top(time.Now().AddDate(0, 0, -30), 10)
	if all == 0 {
		lp.stats.AddWidget(label(tr("Play some music and your stats show up here."), "", "hint").QWidget)
		return
	}
	hours := func(d time.Duration) string {
		if d < time.Hour {
			return tr("{n} min", "n", itoa(int(d.Minutes())))
		}
		return tr("{n} h", "n", strconv.FormatFloat(d.Hours(), 'f', 1, 64))
	}
	lp.stats.AddWidget(label(tr("Listened: {month} in the last 30 days · {all} in total", "month", hours(month), "all", hours(all)), "", "card-sub").QWidget)
	cols := qt.NewQHBoxLayout2()
	cols.SetSpacing(24)
	col := func(title string, rows []library.Ranked, play bool) {
		w := qt.NewQWidget2()
		v := vbox(w, nil, 2)
		v.AddWidget(label(title, "", "card-title").QWidget)
		for i, r := range rows {
			r := r
			text := fmt.Sprintf("%d. %s", i+1, ellipsis(r.Name, 48))
			l := label(text, "", "row-title")
			l.SetToolTip(plural(r.Plays, "{n} play", "{n} plays") + " · " + hours(time.Duration(r.Ms)*time.Millisecond))
			if play && r.Track != nil {
				handCursor(l.QWidget)
				t := *r.Track
				l.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) { u.PlayList([]core.Track{t}, 0) })
			}
			v.AddWidget(l.QWidget)
		}
		v.AddStretch()
		cols.AddWidget2(w, 1)
	}
	col(tr("Top songs (30 days)"), songs, true)
	col(tr("Top artists (30 days)"), artists, false)
	lp.stats.AddLayout(cols.QLayout)
}

// importFromLink makes a playlist from a YouTube/SoundCloud/Bandcamp link.
func (u *UI) importFromLink() {
	ok := false
	link := strings.TrimSpace(qt.QInputDialog_GetText4(u.win.QWidget, tr("Import from link"),
		tr("Link of a playlist or album (YouTube, YouTube Music, SoundCloud, Bandcamp…):"), qt.QLineEdit__Normal, "", &ok))
	if !ok || link == "" {
		return
	}
	if plugin.YtdlpPath() == "" {
		u.toast(tr("yt-dlp is not installed"))
		return
	}
	u.toast(tr("Reading the playlist…"))
	var title string
	var ts []core.Track
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		t, items, err := plugin.YtdlpPlaylist(ctx, link, 1000)
		if err != nil {
			return err
		}
		title = t
		for _, m := range items {
			if tr := plugin.TrackFromMap(m, "ytdlp-search"); tr.Title != "" {
				ts = append(ts, tr)
			}
		}
		return nil
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, tr("Could not import"), err.Error())
			return
		}
		if len(ts) == 0 {
			u.toast(tr("The link has no songs"))
			return
		}
		if title == "" {
			title = tr("Imported")
		}
		p := u.core.Library.Create(title, ts...)
		u.toast(plural(len(ts), "{n} song imported", "{n} songs imported"))
		u.openPlaylist(p.ID)
	})
}

func (lp *libraryPage) playlistCard(p *library.Playlist) *qt.QWidget {
	u := lp.u
	f, b := card(false)
	f.SetObjectName("PlaylistCard")
	c := u.coverView("PlaylistCover", true)
	c.SetMinimumSize2(120, 120)
	if p.ID == library.LikedID {
		c.Icon = "heart"
		c.SetPixmap(nil)
	} else {
		u.coverLoader.Into(p.CoverURL(), c)
	}
	lp.cards = append(lp.cards, c)
	b.AddWidget(c.QWidget)
	name := p.Name
	if p.ID == library.LikedID {
		name = tr("Liked songs")
	}
	b.AddWidget(label(ellipsis(name, 30), "", "card-title").QWidget)
	b.AddWidget(label(plural(len(p.Tracks), "{n} song", "{n} songs"), "", "card-sub").QWidget)
	handCursor(f.QWidget)
	id := p.ID
	f.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if e.Button() == qt.RightButton {
			u.playlistMenu(id)
			return
		}
		u.openPlaylist(id)
	})
	c.OnClick = func() { u.openPlaylist(id) }
	return f.QWidget
}

// loadRemote lists the playlists of src plugins.
func (lp *libraryPage) loadRemote() {
	u := lp.u
	clearLayout(lp.remote.QLayout)
	lp.remote.AddWidget(label(tr("Loading…"), "", "hint").QWidget)
	var cs []core.Collection
	var errs []error
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		cs, errs = u.core.Plugins.RemotePlaylists(ctx)
		return nil
	}, func(error) {
		clearLayout(lp.remote.QLayout)
		if len(cs) == 0 && len(errs) == 0 {
			lp.remote.AddWidget(label(tr("No installed plugin offers playlists."), "", "hint").QWidget)
		}
		for _, c := range cs {
			c := c
			row := qt.NewQHBoxLayout2()
			name := c.Title
			if p := u.core.Plugins.Get(c.Source); p != nil {
				name += "  ·  " + p.Manifest.Name
			}
			row.AddWidget2(label(name, "", "card-title").QWidget, 1)
			open := qt.NewQPushButton3(tr("Open"))
			open.OnClicked(func() { u.importRemote(c) })
			row.AddWidget(open.QWidget)
			lp.remote.AddLayout(row.QLayout)
		}
		for _, e := range errs {
			lp.remote.AddWidget(label(e.Error(), "", "card-error").QWidget)
		}
	})
}

// importRemote loads the tracks of a plugin playlist and saves a copy.
func (u *UI) importRemote(c core.Collection) {
	u.toast(tr("Loading {name}…", "name", c.Title))
	var full *core.Collection
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var err error
		full, err = u.core.Plugins.Open(ctx, c)
		return err
	}, func(err error) {
		if err != nil {
			u.toast(err.Error())
			return
		}
		p := u.core.Library.SaveRemote(*full)
		u.openPlaylist(p.ID)
	})
}

// ---------------------------------------------------------------- detail

func (lp *libraryPage) buildDetail() {
	u := lp.u
	w := qt.NewQWidget2()
	lp.detail = w
	b := vbox(w, []int{28, 18, 22, 0}, 8)
	head := qt.NewQHBoxLayout2()
	head.SetSpacing(20)
	lp.dCover = u.coverView("PlaylistHeaderCover", false)
	lp.dCover.SetFixedSize2(150, 150)
	head.AddWidget(lp.dCover.QWidget)
	tv := qt.NewQVBoxLayout2()
	tv.AddStretch()
	lp.dTitle = label("", "PlaylistTitle", "title")
	lp.dTitle.SetWordWrap(true)
	lp.dSub = label("", "PlaylistSub", "subtitle")
	tv.AddWidget(lp.dTitle.QWidget)
	tv.AddWidget(lp.dSub.QWidget)
	lp.dActions = qt.NewQHBoxLayout2()
	lp.dActions.SetSpacing(8)
	tv.AddLayout(lp.dActions.QLayout)
	tv.AddStretch()
	head.AddLayout2(tv.QLayout, 1)
	b.AddLayout(head.QLayout)
	lp.dList = u.trackList("PlaylistTracks")
	b.AddWidget2(lp.dList.QWidget, 1)
}

// openPlaylist shows a playlist.
func (u *UI) openPlaylist(id string) {
	u.lib.openID = id
	u.lib.fillDetail()
	u.goPage("playlist")
	u.syncLibraryNav("playlist")
}

// downloadsID opens the virtual playlist of downloaded songs.
const downloadsID = "downloads"

func (lp *libraryPage) downloadsPlaylist() *library.Playlist {
	p := &library.Playlist{ID: downloadsID, Name: tr("Downloaded songs"), Kind: "downloads"}
	for _, e := range lp.u.core.Offline.List(offline.Download) {
		p.Tracks = append(p.Tracks, e.Track)
	}
	n, size := lp.u.core.Offline.Usage(offline.Download)
	p.Description = humanBytes(size) + " · " + lp.u.core.DownloadDir()
	_ = n
	return p
}

func (lp *libraryPage) fillDetail() {
	u := lp.u
	p := u.core.Library.Get(lp.openID)
	if lp.openID == downloadsID {
		p = lp.downloadsPlaylist()
	}
	if p == nil {
		lp.openID = ""
		if u.curPage == "playlist" {
			u.goPage("library")
		}
		return
	}
	name := p.Name
	if p.ID == library.LikedID {
		name = tr("Liked songs")
		lp.dCover.Icon = "heart"
		lp.dCover.SetPixmap(nil)
	} else {
		lp.dCover.Icon = ""
		u.coverLoader.Into(p.CoverURL(), lp.dCover)
	}
	lp.dTitle.SetText(name)
	sub := plural(len(p.Tracks), "{n} song", "{n} songs")
	if d := p.DurationMs(); d > 0 {
		sub += " · " + core.FormatMs(d)
	}
	if p.Description != "" {
		sub += " · " + p.Description
	}
	lp.dSub.SetText(sub)

	clearLayout(lp.dActions.QLayout)
	btn := func(text string, primary bool, f func()) {
		x := qt.NewQPushButton3(text)
		if primary {
			role(x.QWidget, "primary")
		}
		x.OnClicked(f)
		lp.dActions.AddWidget(x.QWidget)
	}
	id := p.ID
	btn(tr("Play"), true, func() {
		if len(p.Tracks) > 0 {
			u.PlayList(p.Tracks, 0)
		}
	})
	btn(tr("Shuffle"), false, func() {
		if len(p.Tracks) > 0 {
			ts := append([]core.Track(nil), p.Tracks...)
			rand.Shuffle(len(ts), func(i, j int) { ts[i], ts[j] = ts[j], ts[i] })
			u.PlayList(ts, 0)
		}
	})
	tracks := p.Tracks
	btn(tr("Download all"), false, func() { u.downloadTracks(tracks) })
	if id == downloadsID {
		btn(tr("Open folder"), false, func() { u.openFolder(u.core.DownloadDir()) })
	} else {
		btn(tr("Export…"), false, func() {
			if pl := u.core.Library.Get(id); pl != nil {
				u.exportPlaylists(pl)
			}
		})
	}
	if p.Kind == library.KindRemote {
		btn(tr("Refresh"), false, func() {
			if pl := u.core.Library.Get(id); pl != nil {
				u.importRemote(core.Collection{ID: pl.RemoteID, Source: pl.Source, Kind: "playlist", Title: pl.Name})
			}
		})
	}
	if id != downloadsID {
		more := u.iconButton("PlaylistMore", "more", tr("More"))
		more.OnClicked(func() { u.playlistMenu(id) })
		lp.dActions.AddWidget(more.QWidget)
	}
	lp.dActions.AddStretch()

	editable := p.Kind != library.KindRemote && id != downloadsID
	lp.dList.Reorderable = editable
	lp.dList.OnMove = nil
	lp.dList.OnRemove = nil
	if editable {
		lp.dList.OnMove = func(from, to int) { _ = u.core.Library.Move(id, from, to) }
		lp.dList.OnRemove = func(i int) { _ = u.core.Library.RemoveAt(id, i) }
		lp.dList.RemoveLabel = tr("Remove from this playlist")
	}
	lp.dList.SetTracks(p.Tracks, tr("This playlist is empty. Right-click a song → Add to playlist."))
}

// playlistMenu: rename, cover, export, delete…
func (u *UI) playlistMenu(id string) {
	p := u.core.Library.Get(id)
	if p == nil {
		return
	}
	m := qt.NewQMenu(u.win.QWidget)
	add := func(text string, f func()) { a := m.AddAction(text); a.OnTriggered(f) }
	add(tr("Play"), func() {
		if len(p.Tracks) > 0 {
			u.PlayList(p.Tracks, 0)
		}
	})
	add(tr("Add to queue"), func() {
		for _, t := range p.Tracks {
			u.queue = append(u.queue, t)
		}
		u.refreshQueue()
		u.toast(plural(len(p.Tracks), "{n} song added to the queue", "{n} songs added to the queue"))
	})
	add(tr("Export…"), func() { u.exportPlaylists(p) })
	if id != library.LikedID {
		m.AddSeparator()
		add(tr("Rename…"), func() {
			ok := false
			name := qt.QInputDialog_GetText4(u.win.QWidget, tr("Rename playlist"), tr("Name:"), qt.QLineEdit__Normal, p.Name, &ok)
			if ok && strings.TrimSpace(name) != "" {
				_ = u.core.Library.Update(id, func(x *library.Playlist) { x.Name = strings.TrimSpace(name) })
			}
		})
		add(tr("Change cover…"), func() {
			f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, tr("Cover image"), homeDir(), tr("Images (*.png *.jpg *.jpeg *.webp)"))
			if f != "" {
				_ = u.core.Library.Update(id, func(x *library.Playlist) { x.Cover = f })
			}
		})
		add(tr("Delete"), func() {
			if u.confirm(tr("Delete playlist"), tr("Delete “{name}”?", "name", p.Name)) {
				_ = u.core.Library.Delete(id)
			}
		})
	}
	m.ExecWithPos(qt.QCursor_Pos())
	m.DeleteLater()
}

// newPlaylistDialog creates a playlist (optionally with tracks).
func (u *UI) newPlaylistDialog(ts []core.Track) {
	ok := false
	name := qt.QInputDialog_GetText4(u.win.QWidget, tr("New playlist"), tr("Name:"), qt.QLineEdit__Normal, "", &ok)
	if !ok {
		return
	}
	p := u.core.Library.Create(name, ts...)
	u.toast(tr("Playlist created: {name}", "name", p.Name))
}

// importPlaylists reads .mepw / .m3u files.
func (u *UI) importPlaylists() {
	f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, tr("Import playlist"), homeDir(), tr("Playlists (*.mepw *.m3u *.m3u8 *.json)"))
	if f == "" {
		return
	}
	fh, err := os.Open(f)
	if err != nil {
		u.toast(err.Error())
		return
	}
	defer fh.Close()
	doc, err := library.ReadMepw(fh)
	if err != nil {
		qt.QMessageBox_Warning(u.win.QWidget, tr("Could not import"), err.Error())
		return
	}
	if len(doc.Playlists) == 1 && doc.Playlists[0].Name == "Imported" {
		doc.Playlists[0].Name = strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
	}
	ps := u.core.Library.Import(doc)
	u.toast(plural(len(ps), "{n} playlist imported", "{n} playlists imported"))
	if len(ps) == 1 {
		u.openPlaylist(ps[0].ID)
	}
}

// exportPlaylists writes .mepw (or .m3u8 for a single playlist).
func (u *UI) exportPlaylists(ps ...*library.Playlist) {
	if len(ps) == 0 {
		return
	}
	name := "meofishes-library"
	if len(ps) == 1 {
		name = ps[0].Name
		if ps[0].ID == library.LikedID {
			name = tr("Liked songs")
		}
	}
	filter := tr("meofishes playlist (*.mepw)")
	if len(ps) == 1 {
		filter += ";;" + tr("M3U playlist (*.m3u8)")
	}
	f := qt.QFileDialog_GetSaveFileName4(u.win.QWidget, tr("Export playlist"), filepath.Join(homeDir(), safeName(name)+".mepw"), filter)
	if f == "" {
		return
	}
	fh, err := os.Create(f)
	if err != nil {
		u.toast(err.Error())
		return
	}
	defer fh.Close()
	if strings.HasSuffix(strings.ToLower(f), ".m3u8") || strings.HasSuffix(strings.ToLower(f), ".m3u") {
		err = library.WriteM3U(fh, ps[0])
	} else {
		err = library.WriteMepw(fh, "meofishes "+config.Version, false, ps...)
	}
	if err != nil {
		u.toast(err.Error())
		return
	}
	u.toast(tr("Exported to {file}", "file", f))
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
}
