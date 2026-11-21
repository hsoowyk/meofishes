package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/core"
)

// Status is what `meofishes now` prints (JSON with --json).
type Status struct {
	Playing    bool   `json:"playing"`
	Title      string `json:"title,omitempty"`
	Artist     string `json:"artist,omitempty"`
	Album      string `json:"album,omitempty"`
	Source     string `json:"source,omitempty"`
	Cover      string `json:"cover,omitempty"`
	PositionMs int64  `json:"positionMs"`
	DurationMs int64  `json:"durationMs"`
	Volume     int    `json:"volume"`
	Shuffle    bool   `json:"shuffle"`
	Repeat     string `json:"repeat"`
	Liked      bool   `json:"liked"`
	Line       string `json:"line,omitempty"` // the lyric line being sung
	Queue      int    `json:"queue"`
	Index      int    `json:"index"`
}

func (u *UI) status() Status {
	st := u.state()
	s := Status{Playing: !st.Paused && !st.Idle, PositionMs: st.PositionMs, DurationMs: u.duration(),
		Volume: int(u.cfg.Volume + 0.5), Shuffle: u.cfg.Shuffle, Repeat: u.cfg.Repeat, Queue: len(u.queue), Index: u.qIndex + 1}
	if s.Repeat == "" {
		s.Repeat = "off"
	}
	if cur := u.current(); cur != nil {
		s.Title, s.Artist, s.Album, s.Source, s.Cover = cur.Title, cur.Artist, cur.Album, cur.Source, cur.Cover
		s.Liked = u.core.Library.IsLiked(*cur)
	}
	if l := u.lyrics; l != nil {
		pos := u.lyricsPos()
		for _, ln := range l.Lines {
			if ln.Start <= pos {
				s.Line = ln.Text
			}
		}
	}
	return s
}

// parseTime reads "90", "1:30", "+10", "-5".
func parseTime(s string, cur int64) (int64, error) {
	rel := strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	var secs float64
	for _, part := range strings.Split(s, ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, fmt.Errorf("bad time %q", s)
		}
		secs = secs*60 + v
	}
	ms := int64(secs * 1000)
	if rel {
		if neg {
			ms = -ms
		}
		return cur + ms, nil
	}
	return ms, nil
}

// control runs a command sent by `meofishes <cmd>` (on the UI thread).
func (u *UI) control(cmd string, args []string) (any, error) {
	switch cmd {
	case "show":
		u.win.ShowNormal()
		u.win.Raise()
		u.win.ActivateWindow()
		return nil, nil
	case "now", "status":
		return u.status(), nil
	case "play", "resume":
		if len(args) > 0 {
			return nil, u.playQuery(strings.Join(args, " "))
		}
		if u.state().Idle {
			u.TogglePlay()
		} else if u.player != nil {
			_ = u.player.SetPaused(false)
		}
	case "pause":
		if u.player != nil {
			_ = u.player.SetPaused(true)
		}
	case "toggle":
		u.TogglePlay()
	case "stop":
		if u.player != nil {
			_ = u.player.Stop()
		}
	case "next":
		u.Next(true)
	case "prev", "previous", "back":
		u.Prev()
	case "seek":
		if len(args) == 0 {
			return nil, errors.New("seek needs a time: 90, 1:30, +10, -5")
		}
		ms, err := parseTime(args[0], u.state().PositionMs)
		if err != nil {
			return nil, err
		}
		u.SeekTo(ms)
	case "volume", "vol":
		if len(args) == 0 {
			return int(u.cfg.Volume + 0.5), nil
		}
		v, err := strconv.ParseFloat(strings.TrimLeft(args[0], "+"), 64)
		if err != nil {
			return nil, fmt.Errorf("bad volume %q", args[0])
		}
		if strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
			v += u.cfg.Volume
		}
		u.setVolume(v)
		_ = u.cfg.Save()
		return int(u.cfg.Volume + 0.5), nil
	case "mute":
		if u.cfg.Volume > 0 {
			u.lastVol = u.cfg.Volume
			u.setVolume(0)
		} else {
			u.setVolume(max(u.lastVol, 10))
		}
	case "like", "unlike":
		cur := u.current()
		if cur == nil {
			return nil, errors.New("nothing playing")
		}
		if (cmd == "like") != u.core.Library.IsLiked(*cur) {
			u.toggleLike(*cur)
		}
		return u.core.Library.IsLiked(*cur), nil
	case "shuffle":
		u.toggleShuffle()
		return u.cfg.Shuffle, nil
	case "repeat":
		if len(args) > 0 && (args[0] == "off" || args[0] == "all" || args[0] == "one") {
			u.cfg.Repeat = args[0]
			u.syncRepeat()
		} else {
			u.cycleRepeat()
		}
		return u.cfg.Repeat, nil
	case "lyrics":
		if u.lyrics == nil {
			return nil, errors.New("no lyrics for this song")
		}
		var lines []string
		for _, ln := range u.lyrics.Lines {
			lines = append(lines, ln.Text)
		}
		return map[string]any{"provider": u.lyrics.Provider, "sync": u.lyrics.Sync.String(), "lines": lines, "line": u.status().Line}, nil
	case "queue":
		var out []string
		for i, t := range u.queue {
			mark := "  "
			if i == u.qIndex {
				mark = "▶ "
			}
			out = append(out, fmt.Sprintf("%s%d. %s", mark, i+1, t.String()))
		}
		return out, nil
	case "sleep":
		if len(args) == 0 {
			if l := u.sleepLeft(); l != "" {
				return l, nil
			}
			return "off", nil
		}
		switch args[0] {
		case "off", "0":
			u.setSleep(0)
		case "end":
			u.setSleep(-1)
		default:
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return nil, errors.New("sleep needs minutes, end or off")
			}
			u.setSleep(n)
		}
		return u.sleepLeft(), nil
	case "speed":
		if len(args) > 0 {
			v, err := strconv.ParseFloat(args[0], 64)
			if err != nil {
				return nil, fmt.Errorf("bad speed %q", args[0])
			}
			u.setSpeed(v)
		}
		return u.cfg.Prefs.Speed, nil
	case "fullscreen", "lyrics-view":
		u.toggleWindowShown()
		u.openFull()
	case "quit":
		singleShot(50, u.quit)
	default:
		return nil, fmt.Errorf("unknown command %q", cmd)
	}
	return nil, nil
}

func (u *UI) toggleWindowShown() {
	if !u.win.IsVisible() {
		u.toggleWindow()
	}
}

// playQuery searches every source and plays the first result.
func (u *UI) playQuery(q string) error {
	ps := u.core.Plugins.Sources()
	if len(ps) == 0 {
		return errors.New("no music source enabled")
	}
	// search off the UI thread, then play on it
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		for _, p := range ps {
			r, err := u.core.Plugins.Search(ctx, p, q)
			if err == nil && r != nil && len(r.Tracks) > 0 {
				ts := r.Tracks
				mainthread.Start(func() { u.PlayList([]core.Track{ts[0]}, 0) })
				return
			}
		}
		mainthread.Start(func() { u.toast(tr("Nothing found for “{query}”.", "query", q)) })
	}()
	return nil
}
