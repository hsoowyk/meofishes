package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/player"
	"meofishes/internal/theme"
)

// ---------------------------------------------------------------- mini player

// miniPlayer is a small window that stays on top: cover, title, controls.
type miniPlayer struct {
	w     *qt.QWidget
	drag  *qt.QPoint
	cover *CoverView
}

func (u *UI) toggleMini() {
	if u.mini != nil && u.mini.w.IsVisible() {
		u.mini.w.Hide()
		return
	}
	if u.mini == nil {
		u.mini = u.buildMini()
	}
	u.mini.w.Show()
	u.mini.w.Raise()
	if cur := u.current(); cur != nil {
		u.coverLoader.Into(cur.Cover, u.mini.cover)
	}
}

func (u *UI) buildMini() *miniPlayer {
	mp := &miniPlayer{}
	f := frame("MiniPlayer")
	w := f.QWidget
	mp.w = w
	w.SetWindowFlags(qt.Tool | qt.FramelessWindowHint | qt.WindowStaysOnTopHint)
	w.SetWindowTitle("meofishes")
	w.SetAttribute(qt.WA_StyledBackground)
	w.SetFixedSize2(340, 96)
	b := hbox(w, []int{10, 10, 10, 10}, 10)
	mp.cover = u.coverView("MiniCover", false)
	mp.cover.SetFixedSize2(76, 76)
	mp.cover.OnClick = func() { u.toggleWindowShown(); u.win.Raise(); u.win.ActivateWindow() }
	mp.cover.SetToolTip(tr("Open meofishes"))
	u.covers = append(u.covers, mp.cover)
	b.AddWidget(mp.cover.QWidget)
	col := qt.NewQVBoxLayout2()
	col.SetSpacing(2)
	t := label(tr("Nothing playing"), "MiniTitle", "")
	a := label("", "MiniArtist", "hint")
	u.titleLbls = append(u.titleLbls, t)
	u.artistLbls = append(u.artistLbls, a)
	col.AddWidget(t.QWidget)
	col.AddWidget(a.QWidget)
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(2)
	prev := u.iconButton("MiniPrev", "prev", tr("Previous"))
	prev.OnClicked(u.Prev)
	play := u.iconButton("MiniPlay", "play", tr("Play / pause"))
	play.OnClicked(u.TogglePlay)
	u.playBtns = append(u.playBtns, play)
	next := u.iconButton("MiniNext", "next", tr("Next"))
	next.OnClicked(func() { u.Next(true) })
	row.AddWidget(prev.QWidget)
	row.AddWidget(play.QWidget)
	row.AddWidget(next.QWidget)
	row.AddWidget(u.likeButton("MiniLike").QWidget)
	row.AddStretch()
	closeB := u.iconButton("MiniClose", "close", tr("Close the mini player"))
	closeB.OnClicked(func() { w.Hide() })
	row.AddWidget(closeB.QWidget)
	col.AddLayout(row.QLayout)
	s := u.slider("MiniSeek")
	s.OnSeek = func(v float64) {
		if d := u.duration(); d > 0 {
			u.SeekTo(int64(v * float64(d)))
		}
	}
	u.seeks = append(u.seeks, s)
	col.AddWidget(s.QWidget)
	b.AddLayout2(col.QLayout, 1)

	// drag the window by its background
	f.OnMousePressEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if mp.drag != nil {
			mp.drag.Delete()
		}
		gp := e.GlobalPos()
		pos := w.Pos()
		mp.drag = qt.NewQPoint2(gp.X()-pos.X(), gp.Y()-pos.Y())
	})
	f.OnMouseMoveEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if mp.drag != nil {
			gp := e.GlobalPos()
			w.Move(gp.X()-mp.drag.X(), gp.Y()-mp.drag.Y())
		}
	})
	f.OnMouseReleaseEvent(func(super func(*qt.QMouseEvent), e *qt.QMouseEvent) {
		if mp.drag != nil {
			mp.drag.Delete()
			mp.drag = nil
		}
		u.cfg.MiniX, u.cfg.MiniY = w.X(), w.Y()
		_ = u.cfg.Save()
	})
	if u.cfg.MiniX != 0 || u.cfg.MiniY != 0 {
		w.Move(u.cfg.MiniX, u.cfg.MiniY)
	} else if scr := qt.QGuiApplication_PrimaryScreen(); scr != nil {
		g := scr.AvailableGeometry()
		w.Move(g.X()+g.Width()-360, g.Y()+g.Height()-120)
	}
	return mp
}

// ---------------------------------------------------------------- quality badge

// refreshTech asks mpv about the stream (off the UI thread) and shows it.
func (u *UI) refreshTech() {
	if u.mpv == nil || u.techBusy || u.full == nil || !u.full.open {
		return
	}
	u.techBusy = true
	m := u.mpv
	go func() {
		t := m.Tech()
		mainthread.Start(func() {
			u.techBusy = false
			u.showTech(t)
		})
	}()
}

func (u *UI) showTech(t player.Tech) {
	if u.full == nil || u.full.tech == nil {
		return
	}
	text := t.Badge()
	if text == "" && t.Codec != "" {
		text = strings.ToUpper(t.Codec)
	}
	u.full.tech.SetText(text)
	u.full.tech.SetToolTip(t.String())
	u.full.tech.SetVisible(text != "")
}

// ---------------------------------------------------------------- lyric card

// shareLyric draws the current lyric line (and the next one) on a picture
// with the cover, saves it and copies it to the clipboard.
func (u *UI) shareLyric() {
	cur := u.current()
	if cur == nil {
		return
	}
	var lines []string
	if u.lyrics != nil {
		pos := u.lyricsPos()
		idx := -1
		for i, ln := range u.lyrics.Lines {
			if ln.Start <= pos {
				idx = i
			}
		}
		for i := max(idx, 0); i < len(u.lyrics.Lines) && len(lines) < 2; i++ {
			if t := strings.TrimSpace(u.lyrics.Lines[i].Text); t != "" {
				lines = append(lines, t)
			}
		}
	}
	if len(lines) == 0 {
		u.toast(tr("There is no lyric line to share right now"))
		return
	}
	var cover *qt.QPixmap
	for _, c := range u.covers {
		if c.cur != nil && !c.cur.IsNull() {
			cover = c.cur
			break
		}
	}
	const W, H = 1080, 1350
	img := qt.NewQImage3(W, H, qt.QImage__Format_ARGB32_Premultiplied)
	defer img.Delete()
	p := qt.NewQPainter2(img.QPaintDevice)
	p.SetRenderHint(qt.QPainter__Antialiasing)
	p.SetRenderHint(qt.QPainter__SmoothPixmapTransform)
	cols := paletteColors(cover, 2)
	c1, c2 := theme.Color{R: 30, G: 34, B: 60, A: 1}, theme.Color{R: 10, G: 10, B: 18, A: 1}
	if len(cols) > 0 {
		c1 = cols[0]
		c1.R, c1.G, c1.B = c1.R*0.55, c1.G*0.55, c1.B*0.55
	}
	if len(cols) > 1 {
		c2 = cols[1]
		c2.R, c2.G, c2.B = c2.R*0.25, c2.G*0.25, c2.B*0.25
	}
	grad := qt.NewQLinearGradient3(0, 0, W, H)
	q1, q2 := qc(c1), qc(c2)
	grad.SetColorAt(0, q1)
	grad.SetColorAt(1, q2)
	br := qt.NewQBrush10(grad.QGradient)
	full := rectF(0, 0, W, H)
	p.FillRect(full, br)
	br.Delete()
	grad.Delete()
	q1.Delete()
	q2.Delete()
	full.Delete()

	y := 110.0
	if cover != nil {
		r := rectF(110, y, 220, 220)
		sr := rectF(0, 0, float64(cover.Width()), float64(cover.Height()))
		path := qt.NewQPainterPath()
		path.AddRoundedRect(r, 24, 24)
		p.Save()
		p.SetClipPath(path)
		p.DrawPixmap(r, cover, sr)
		p.Restore()
		path.Delete()
		r.Delete()
		sr.Delete()
	}
	white := theme.Color{R: 255, G: 255, B: 255, A: 1}
	ft := font("Inter, Noto Sans, sans-serif", 46, 800)
	p.SetFont(ft)
	setPenColor(p, white)
	tr1 := rectF(370, y+40, W-480, 140)
	p.DrawText5(tr1, int(qt.AlignLeft|qt.AlignTop)|int(qt.TextWordWrap), cur.Title)
	tr1.Delete()
	ft.Delete()
	fa := font("Inter, Noto Sans, sans-serif", 34, 500)
	p.SetFont(fa)
	setPenColor(p, withAlpha(white, 0.7))
	ar := rectF(370, y+150, W-480, 60)
	p.DrawText5(ar, int(qt.AlignLeft|qt.AlignTop), cur.Artist)
	ar.Delete()
	fa.Delete()

	fl := font("Inter, Noto Sans, sans-serif", 72, 800)
	p.SetFont(fl)
	for i, ln := range lines {
		a := 1.0
		if i > 0 {
			a = 0.45
		}
		setPenColor(p, withAlpha(white, a))
		lr := rectF(110, 470+float64(i)*330, W-220, 320)
		p.DrawText5(lr, int(qt.AlignLeft|qt.AlignTop)|int(qt.TextWordWrap), ln)
		lr.Delete()
	}
	fl.Delete()
	fs := font("Inter, Noto Sans, sans-serif", 28, 600)
	p.SetFont(fs)
	setPenColor(p, withAlpha(white, 0.5))
	fr := rectF(110, H-110, W-220, 50)
	p.DrawText5(fr, int(qt.AlignLeft|qt.AlignVCenter), "♪ meofishes")
	fr.Delete()
	fs.Delete()
	p.End()
	p.Delete()

	dir := filepath.Join(homeDir(), "Pictures")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = homeDir()
	}
	file := filepath.Join(dir, fmt.Sprintf("meofishes %s - %s.png", safeName(cur.Artist), safeName(cur.Title)))
	if !img.Save(file) {
		u.toast(tr("Could not save the picture"))
		return
	}
	qt.QGuiApplication_Clipboard().SetImage(img)
	u.toast(tr("Lyric card saved in {path} and copied", "path", file))
}

// ---------------------------------------------------------------- alarm

// setAlarm starts playing at hh:mm (empty = off), the volume rising slowly.
func (u *UI) setAlarm(at string) error {
	at = strings.TrimSpace(at)
	if at == "" || at == "off" {
		u.alarmAt = time.Time{}
		return nil
	}
	t, err := time.ParseInLocation("15:04", at, time.Local)
	if err != nil {
		return fmt.Errorf("the alarm needs a time like 07:30")
	}
	now := time.Now()
	when := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if !when.After(now) {
		when = when.AddDate(0, 0, 1)
	}
	u.alarmAt = when
	u.toast(tr("Alarm at {time} — keep meofishes open", "time", when.Format("15:04")))
	return nil
}

func (u *UI) alarmLeft() string {
	if u.alarmAt.IsZero() {
		return ""
	}
	return u.alarmAt.Format("15:04")
}

// alarmTick runs every second (slow timer).
func (u *UI) alarmTick() {
	if u.alarmAt.IsZero() || time.Now().Before(u.alarmAt) {
		return
	}
	u.alarmAt = time.Time{}
	target := u.cfg.Volume
	if target < 20 {
		target = 50
	}
	if u.player == nil {
		return
	}
	if u.state().Idle || len(u.queue) == 0 {
		if ts := u.core.Library.Surprise(40); len(ts) > 0 {
			u.PlayList(ts, 0)
		} else if len(u.queue) > 0 {
			u.playCurrent()
		}
	} else {
		_ = u.player.SetPaused(false)
	}
	if u.mpv != nil {
		_ = u.mpv.SetVolume(0)
		u.cfg.Volume = target
		u.fadeVolume(0, target, 60*time.Second, func() {})
	}
	u.toast(tr("Good morning!"))
}

func (u *UI) alarmDialog() {
	ok := false
	def := u.alarmLeft()
	if def == "" {
		def = "07:00"
	}
	v := qt.QInputDialog_GetText4(u.win.QWidget, tr("Alarm"), tr("Wake up at (HH:MM, empty = off):"), qt.QLineEdit__Normal, def, &ok)
	if !ok {
		return
	}
	if err := u.setAlarm(v); err != nil {
		u.toast(err.Error())
	} else if strings.TrimSpace(v) == "" {
		u.toast(tr("Alarm off"))
	}
}
