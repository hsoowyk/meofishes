package ui

import (
	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/core"
)

// TrackList is a scrollable list of track rows.
type TrackList struct {
	*qt.QScrollArea
	u       *UI
	inner   *qt.QWidget
	box     *qt.QVBoxLayout
	rows    []*trackRow
	tracks  []core.Track
	empty   *qt.QLabel
	OnPlay  func(tracks []core.Track, i int)
	OnQueue func(t core.Track)
	OnMenu  func(t core.Track, i int)
	// QueueMode shows a remove button instead of "add to queue"
	QueueMode bool
	OnRemove  func(i int)
}

type trackRow struct {
	*qt.QWidget
	d     *deco
	cover *CoverView
	title *qt.QLabel
	sub   *qt.QLabel
	meta  *qt.QLabel
	btn   *IconButton
	track core.Track
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
	return l
}

// SetTracks replaces the rows. emptyMsg is shown when there are none.
func (l *TrackList) SetTracks(ts []core.Track, emptyMsg string) {
	for _, r := range l.rows {
		r.d.dispose()
		r.btn.d.dispose()
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
	}
	l.u.coverLoader.Prefetch(ts)
	// cascade in
	singleShot(0, func() {
		for i, r := range l.rows {
			if i > 40 {
				break
			}
			l.u.Enter(r.QWidget, "TrackRow", i)
		}
	})
	l.MarkPlaying(l.u.app.current())
}

// MarkPlaying highlights the row of the playing track.
func (l *TrackList) MarkPlaying(cur *core.Track) {
	for i, r := range l.rows {
		on := cur != nil && r.track.Key() == cur.Key()
		if l.QueueMode {
			on = i == l.u.app.qIndex
		}
		if r.d.active != on {
			setProp(r.QWidget, "playing", boolStr(on))
			repolish(r.title.QWidget)
			r.d.setActive(on)
		}
	}
}

func (u *UI) newRow(l *TrackList, t core.Track, i int) *trackRow {
	r := &trackRow{QWidget: qt.NewQWidget2(), track: t}
	r.SetObjectName("TrackRow")
	r.SetAttribute(qt.WA_StyledBackground)
	setProp(r.QWidget, "playing", "false")
	r.SetMinimumHeight(58)
	r.d = u.newDeco(r.QWidget, "TrackRow")
	h := qt.NewQHBoxLayout(r.QWidget)
	h.SetContentsMargins(10, 6, 10, 6)
	h.SetSpacing(12)
	r.cover = u.coverView("RowCover", false)
	r.cover.SetFixedSize2(46, 46)
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

	meta := ""
	if t.DurationMs > 0 {
		meta = core.FormatMs(t.DurationMs)
	}
	src := t.Source
	if p := u.app.plugins.Get(t.Source); p != nil {
		src = p.Manifest.Name
	}
	r.meta = qt.NewQLabel3(meta)
	role(r.meta.QWidget, "row-meta")
	r.meta.SetToolTip(src)
	h.AddWidget(r.meta.QWidget)

	if l.QueueMode {
		r.btn = u.iconButton("RowRemove", "close", "Remover da fila")
		r.btn.OnClicked(func() {
			if l.OnRemove != nil {
				l.OnRemove(i)
			}
		})
	} else {
		r.btn = u.iconButton("RowQueue", "plus", "Adicionar à fila")
		r.btn.OnClicked(func() {
			if l.OnQueue != nil {
				l.OnQueue(t)
			}
		})
	}
	r.btn.SetVisible(false)
	h.AddWidget(r.btn.QWidget)

	r.OnEnterEvent(func(super func(*qt.QEvent), e *qt.QEvent) { r.d.setHover(true); r.btn.SetVisible(true) })
	r.OnLeaveEvent(func(super func(*qt.QEvent), e *qt.QEvent) { r.d.setHover(false); r.btn.SetVisible(false) })
	r.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		super(e)
		p := qt.NewQPainter2(r.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		r.d.paintBg(p, 12)
		p.End()
		p.Delete()
	})
	r.OnMouseDoubleClickEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if l.OnPlay != nil {
			l.OnPlay(l.tracks, i)
		}
	})
	r.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if e.Button() == qt.RightButton && l.OnMenu != nil {
			l.OnMenu(t, i)
		}
	})
	r.SetToolTip("Duplo clique para tocar")
	return r
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
