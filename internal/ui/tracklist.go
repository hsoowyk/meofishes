package ui

import (
	"encoding/json"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
	"meofishes/internal/library"
	"meofishes/internal/offline"
)

// TrackList is a scrollable list of track rows. Lists with Reorderable set
// (the queue, local playlists) can be reordered by dragging.
type TrackList struct {
	*qt.QScrollArea
	u      *UI
	inner  *qt.QWidget
	box    *qt.QVBoxLayout
	rows   []*trackRow
	tracks []core.Track
	empty  *qt.QLabel

	OnPlay func(tracks []core.Track, i int)
	// QueueMode marks the playing row by index (the queue).
	QueueMode   bool
	Reorderable bool
	OnMove      func(from, to int)
	OnRemove    func(i int)
	RemoveLabel string

	drag struct {
		active bool
		from   int
		startY int
		target int
	}
}

type trackRow struct {
	*qt.QWidget
	d      *deco
	cover  *CoverView
	title  *qt.QLabel
	sub    *qt.QLabel
	meta   *qt.QLabel
	like   *IconButton
	more   *IconButton
	handle *IconButton
	track  core.Track
	index  int
	drop   int // -1 above, +1 below, 0 none (drag indicator)
}

func (u *UI) trackList(name string) *TrackList {
	l := &TrackList{QScrollArea: qt.NewQScrollArea2(), u: u}
	l.SetObjectName(name)
	l.SetWidgetResizable(true)
	l.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
	l.SetFrameShape(qt.QFrame__NoFrame)
	l.inner = qt.NewQWidget2()
	l.inner.SetObjectName(name + "Inner")
	l.box = qt.NewQVBoxLayout(l.inner)
	l.box.SetContentsMargins(0, 0, 8, 12)
	l.box.SetSpacing(2)
	l.empty = qt.NewQLabel3("")
	role(l.empty.QWidget, "hint")
	l.empty.SetAlignment(qt.AlignCenter)
	l.empty.SetWordWrap(true)
	l.box.AddWidget(l.empty.QWidget)
	l.box.AddStretch()
	l.SetWidget(l.inner)
	l.OnPlay = u.PlayList
	u.allLists = append(u.allLists, l)
	return l
}

// SetTracks replaces the rows. emptyMsg is shown when there are none.
func (l *TrackList) SetTracks(ts []core.Track, emptyMsg string) {
	for _, r := range l.rows {
		r.d.dispose()
		r.like.d.dispose()
		r.more.d.dispose()
		if r.handle != nil {
			r.handle.d.dispose()
		}
		r.cover.dispose()
		r.Hide()
		r.DeleteLater()
	}
	l.rows = nil
	l.tracks = ts
	l.empty.SetText(emptyMsg)
	l.empty.SetVisible(len(ts) == 0 && emptyMsg != "")
	for i, t := range ts {
		r := l.u.newRow(l, t, i)
		l.rows = append(l.rows, r)
		l.box.InsertWidget(l.box.Count()-1, r.QWidget)
		i := i
		key := t.Key()
		l.u.art.Get(t, func(url string) {
			if i < len(l.rows) && l.rows[i].track.Key() == key {
				l.rows[i].track.Cover = url
				if i < len(l.tracks) {
					l.tracks[i].Cover = url
				}
				l.u.coverLoader.Into(url, l.rows[i].cover)
			}
		})
	}
	// cascade in
	singleShot(0, func() {
		for i, r := range l.rows {
			if i > 12 { // only what is on screen
				break
			}
			l.u.Enter(r.QWidget, "TrackRow", i)
		}
	})
	l.MarkPlaying(l.u.current())
}

// MarkPlaying highlights the row of the playing track.
func (l *TrackList) MarkPlaying(cur *core.Track) {
	for i, r := range l.rows {
		on := cur != nil && cur.Title != "" && r.track.Key() == cur.Key()
		if l.QueueMode {
			on = i == l.u.qIndex
		}
		if r.d.active != on {
			setProp(r.QWidget, "playing", boolStr(on))
			repolish(r.title.QWidget)
			r.d.setActive(on)
		}
	}
}

// refreshLikes updates the hearts after the liked songs changed.
func (l *TrackList) refreshLikes() {
	for _, r := range l.rows {
		r.syncLike(l.u)
	}
}

func (r *trackRow) syncLike(u *UI) {
	liked := u.core.Library.IsLiked(r.track)
	if liked {
		r.like.SetIcon("heart")
	} else {
		r.like.SetIcon("heart-outline")
	}
	r.like.SetActive(liked)
	r.like.SetVisible(liked || r.d.hovered)
}

func (u *UI) newRow(l *TrackList, t core.Track, i int) *trackRow {
	r := &trackRow{QWidget: qt.NewQWidget2(), track: t, index: i}
	r.SetObjectName("TrackRow")
	r.SetAttribute(qt.WA_StyledBackground)
	setProp(r.QWidget, "playing", "false")
	r.SetMinimumHeight(56)
	r.d = u.newDeco(r.QWidget, "TrackRow")
	h := qt.NewQHBoxLayout(r.QWidget)
	h.SetContentsMargins(8, 5, 8, 5)
	h.SetSpacing(12)
	if l.Reorderable {
		r.handle = u.iconButton("RowDrag", "drag", tr("Drag to reorder"))
		c := qt.NewQCursor2(qt.OpenHandCursor)
		r.handle.SetCursor(c)
		c.Delete()
		r.handle.SetAttribute(qt.WA_TransparentForMouseEvents) // the row handles the drag
		h.AddWidget(r.handle.QWidget)
	}
	r.cover = u.coverView("RowCover", false)
	r.cover.SetFixedSize2(44, 44)
	r.cover.SetVisible(u.cfg.Prefs.ListCovers)
	u.coverLoader.Into(t.Cover, r.cover)
	h.AddWidget(r.cover.QWidget)

	text := qt.NewQVBoxLayout2()
	text.SetSpacing(1)
	r.title = qt.NewQLabel3(t.Title)
	role(r.title.QWidget, "row-title")
	r.sub = qt.NewQLabel3(joinNonEmpty(" · ", t.Artist, t.Album))
	role(r.sub.QWidget, "row-sub")
	text.AddWidget(r.title.QWidget)
	text.AddWidget(r.sub.QWidget)
	h.AddLayout2(text.QLayout, 1)

	r.like = u.iconButton("RowLike", "heart-outline", tr("Like"))
	r.like.OnClicked(func() { u.toggleLike(r.track) })
	h.AddWidget(r.like.QWidget)

	meta := ""
	if t.DurationMs > 0 {
		meta = core.FormatMs(t.DurationMs)
	}
	src := t.Source
	if p := u.core.Plugins.Get(t.Source); p != nil {
		src = p.Manifest.Name
	}
	r.meta = qt.NewQLabel3(meta)
	role(r.meta.QWidget, "row-meta")
	r.meta.SetToolTip(src)
	h.AddWidget(r.meta.QWidget)

	r.more = u.iconButton("RowMore", "more", tr("More"))
	r.more.OnClicked(func() { u.trackMenu(l, r.index) })
	r.more.SetVisible(false)
	h.AddWidget(r.more.QWidget)
	r.syncLike(u)

	r.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) {
		r.d.setHover(true)
		r.more.SetVisible(true)
		r.syncLike(u)
	})
	r.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) {
		r.d.setHover(false)
		r.more.SetVisible(false)
		r.syncLike(u)
	})
	r.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		super(e)
		p := qt.NewQPainter2(r.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		r.d.paintBg(p, 12)
		if r.drop != 0 {
			c := u.style.For(r.QWidget, "TrackRow").Color("drop-color", r.d.icon)
			y := 0.0
			if r.drop > 0 {
				y = float64(r.Height()) - 3
			}
			fillRounded(p, 6, y, float64(r.Width())-12, 3, 1.5, c)
		}
		p.End()
		p.Delete()
	})
	r.OnMouseDoubleClickEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if l.OnPlay != nil {
			l.OnPlay(l.tracks, r.index)
		}
	})
	r.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		switch e.Button() {
		case qt.RightButton:
			u.trackMenu(l, r.index)
		case qt.LeftButton:
			l.drag.from, l.drag.target, l.drag.startY, l.drag.active = r.index, r.index, e.GlobalPos().Y(), false
		}
	})
	r.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if !l.Reorderable || e.Buttons()&qt.LeftButton == 0 {
			return
		}
		gy := e.GlobalPos().Y()
		if !l.drag.active && abs(float64(gy-l.drag.startY)) > 6 {
			l.drag.active = true
			c := qt.NewQCursor2(qt.ClosedHandCursor)
			r.SetCursor(c)
			c.Delete()
		}
		if l.drag.active {
			l.dragOver(e.GlobalPos())
		}
	})
	r.OnMouseReleaseEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if !l.drag.active {
			return
		}
		l.drag.active = false
		r.UnsetCursor()
		for _, x := range l.rows {
			if x.drop != 0 {
				x.drop = 0
				x.Update()
			}
		}
		if l.OnMove != nil && l.drag.target >= 0 && l.drag.target != l.drag.from {
			l.OnMove(l.drag.from, l.drag.target)
		}
	})
	r.SetToolTip(tr("Double-click to play · right-click for more"))
	return r
}

// dragOver finds the drop position under the pointer and shows a line.
func (l *TrackList) dragOver(global *qt.QPoint) {
	pt := l.inner.MapFromGlobal(global)
	y := pt.Y()
	target := len(l.rows) - 1
	for i, r := range l.rows {
		if y < r.Y()+r.Height()/2 {
			target = i
			break
		}
	}
	// moving down: the item lands after the hovered one
	if target > l.drag.from && y < l.rows[target].Y()+l.rows[target].Height()/2 {
		target--
	}
	l.drag.target = target
	for i, r := range l.rows {
		d := 0
		if i == target && target != l.drag.from {
			if target > l.drag.from {
				d = 1
			} else {
				d = -1
			}
		}
		if r.drop != d {
			r.drop = d
			r.Update()
		}
	}
	// auto-scroll near the edges
	vp := l.Viewport().MapFromGlobal(global).Y()
	sb := l.VerticalScrollBar()
	if vp < 30 {
		sb.SetValue(sb.Value() - 12)
	} else if vp > l.Viewport().Height()-30 {
		sb.SetValue(sb.Value() + 12)
	}
}

// trackMenu is the context menu of a track in a list.
func (u *UI) trackMenu(l *TrackList, i int) {
	u.trackMenuFor(l.tracks, i, l.OnPlay, l)
}

// trackMenuFor shows the menu of tracks[i]; l (optional) adds the list
// actions (move, remove).
func (u *UI) trackMenuFor(tracks []core.Track, i int, play func([]core.Track, int), l *TrackList) {
	if i < 0 || i >= len(tracks) {
		return
	}
	t := tracks[i]
	m := qt.NewQMenu(u.win.QWidget)
	m.SetObjectName("TrackMenu")
	act := func(menu *qt.QMenu, text string, f func()) *qt.QAction {
		a := menu.AddAction(text)
		a.OnTriggered(f)
		return a
	}
	act(m, tr("Play"), func() { play(tracks, i) })
	act(m, tr("Play next"), func() { u.PlayNext(t) })
	act(m, tr("Add to queue"), func() { u.Enqueue(t) })
	act(m, tr("Start radio"), func() { u.startRadio(t) })
	m.AddSeparator()
	if u.core.Library.IsLiked(t) {
		act(m, tr("Remove from Liked songs"), func() { u.toggleLike(t) })
	} else {
		act(m, tr("Add to Liked songs"), func() { u.toggleLike(t) })
	}
	switch {
	case u.core.Offline.Busy(t):
		a := m.AddAction(tr("Downloading…"))
		a.SetEnabled(false)
	case u.core.Offline.Kind(t) == offline.Download:
		act(m, tr("Remove download"), func() { u.core.Offline.Remove(t); u.toast(tr("Download removed")) })
	default:
		act(m, tr("Download"), func() { u.downloadTracks([]core.Track{t}) })
	}
	sub := m.AddMenuWithTitle(tr("Add to playlist"))
	act(sub, tr("New playlist…"), func() { u.newPlaylistDialog([]core.Track{t}) })
	sub.AddSeparator()
	for _, p := range u.core.Library.All() {
		if p.Kind != library.KindLocal {
			continue
		}
		id, name := p.ID, p.Name
		act(sub, name, func() {
			_ = u.core.Library.Add(id, t)
			u.toast(tr("Added to {playlist}", "playlist", name))
		})
	}
	if l != nil && l.Reorderable && l.OnMove != nil {
		m.AddSeparator()
		up := act(m, tr("Move up"), func() { l.OnMove(i, i-1) })
		up.SetEnabled(i > 0)
		down := act(m, tr("Move down"), func() { l.OnMove(i, i+1) })
		down.SetEnabled(i < len(tracks)-1)
		top := act(m, tr("Move to top"), func() { l.OnMove(i, 0) })
		top.SetEnabled(i > 0)
	}
	if l != nil && l.OnRemove != nil {
		label := l.RemoveLabel
		if label == "" {
			label = tr("Remove")
		}
		act(m, label, func() { l.OnRemove(i) })
	}
	m.AddSeparator()
	act(m, tr("Copy title"), func() {
		qt.QGuiApplication_Clipboard().SetText(t.String())
	})
	if u.cfg.Prefs.DevMode {
		act(m, tr("Inspect (JSON)"), func() {
			data, _ := json.MarshalIndent(t, "", "  ")
			u.showText(tr("Track"), string(data))
		})
	}
	m.ExecWithPos(qt.QCursor_Pos())
	m.DeleteLater()
}

func joinNonEmpty(sep string, parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}
