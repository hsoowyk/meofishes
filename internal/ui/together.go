package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/together"
)

// listenTogether is the "Listen together" session (Metrolist-compatible).
type listenTogether struct {
	c        *together.Client
	host     bool
	room     string
	users    map[string]together.User
	pending  map[string]string // user id → name (join requests)
	suggests []together.Event
	applying bool   // we are applying a remote action (do not echo it)
	trackID  string // video id of the room's current track
	revision uint64
	lastTok  string
}

// ytID finds the YouTube video id of a track (what Metrolist uses).
func ytID(t core.Track, pageURL string) string {
	if t.VideoID != "" {
		return t.VideoID
	}
	for _, s := range []string{pageURL, t.StreamURL} {
		if u, err := url.Parse(s); err == nil {
			if v := u.Query().Get("v"); v != "" {
				return v
			}
			if strings.HasSuffix(u.Host, "youtu.be") {
				return strings.Trim(u.Path, "/")
			}
		}
	}
	return ""
}

func trackFromRoom(t together.Track) core.Track {
	return core.Track{Source: "ytdlp-search", ID: t.ID, VideoID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
		DurationMs: t.DurationMs, Cover: t.Thumbnail, StreamURL: "https://music.youtube.com/watch?v=" + t.ID}
}

func roomTrack(t core.Track, id string) together.Track {
	return together.Track{ID: id, Title: t.Title, Artist: t.Artist, Album: t.Album, DurationMs: t.DurationMs, Thumbnail: t.Cover}
}

// ---------------------------------------------------------------- connection

func (u *UI) ltServer() string {
	if s := strings.TrimSpace(u.cfg.Prefs.TogetherServer); s != "" {
		return s
	}
	return together.DefaultServer
}

func (u *UI) ltName() string {
	if n := strings.TrimSpace(u.cfg.Prefs.UserName); n != "" {
		return n
	}
	return "meofishes"
}

// ltConnect dials the server, then runs then(client) on the UI thread.
func (u *UI) ltConnect(then func()) {
	if u.lt.c != nil {
		then()
		return
	}
	u.toast(tr("Connecting…"))
	srv := u.ltServer()
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		c, err := together.Dial(ctx, srv, config.UserAgent, "meofishes "+config.Version, func(e together.Event) {
			mainthread.Start(func() { u.ltEvent(e) })
		})
		if err != nil {
			return err
		}
		mainthread.Start(func() { u.lt.c = c })
		return nil
	}, func(err error) {
		if err != nil {
			u.toast(tr("Could not connect: {error}", "error", err.Error()))
			return
		}
		then()
	})
}

func (u *UI) ltCreate() {
	u.ltConnect(func() { _ = u.lt.c.Create(u.ltName()) })
}

func (u *UI) ltJoin(code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if i := strings.LastIndex(code, "CODE="); i >= 0 { // a metrolist.cc invite link
		code = code[i+5:]
	}
	if code == "" {
		return
	}
	u.ltConnect(func() {
		_ = u.lt.c.Join(code, u.ltName())
		u.toast(tr("Asking the host to let you in…"))
	})
}

func (u *UI) ltLeave() {
	if u.lt.c != nil {
		_ = u.lt.c.Leave()
		u.lt.c.Close()
	}
	u.lt = listenTogether{}
	u.togetherPg.refresh()
	u.toast(tr("You left the room"))
}

// ---------------------------------------------------------------- events

func (u *UI) ltEvent(e together.Event) {
	lt := &u.lt
	switch e.Type {
	case "room_created":
		lt.host, lt.room, lt.lastTok = true, e.RoomCode, e.SessionToken
		lt.users = map[string]together.User{e.UserID: {ID: e.UserID, Name: u.ltName(), Host: true, Connected: true}}
		lt.pending = map[string]string{}
		u.toast(tr("Room created: {code}", "code", e.RoomCode))
		u.ltSendCurrent() // share what is playing
	case "join_approved", "reconnected":
		lt.host, lt.room = e.IsHost, e.RoomCode
		if e.SessionToken != "" {
			lt.lastTok = e.SessionToken
		}
		lt.users = map[string]together.User{}
		lt.pending = map[string]string{}
		if e.State != nil {
			for _, us := range e.State.Users {
				lt.users[us.ID] = us
			}
			u.ltApplyState(*e.State)
		}
		u.toast(tr("You joined room {code}", "code", e.RoomCode))
	case "join_rejected":
		u.toast(tr("The host did not let you in: {reason}", "reason", e.Reason))
		u.ltLeave()
		return
	case "join_request":
		lt.pending[e.UserID] = e.Name
		u.toast(tr("{name} wants to join — Listen together", "name", e.Name))
		if u.tray != nil && !u.win.IsActiveWindow() {
			u.tray.ShowMessage("meofishes", tr("{name} wants to join your room", "name", e.Name), u.logoIcon())
		}
	case "user_joined", "user_reconnected":
		lt.users[e.UserID] = together.User{ID: e.UserID, Name: e.Name, Connected: true}
		delete(lt.pending, e.UserID)
		if e.Type == "user_joined" {
			u.toast(tr("{name} joined", "name", e.Name))
			if lt.host {
				u.ltSendCurrent()
			}
		}
	case "user_left", "user_disconnected":
		if e.Type == "user_left" {
			delete(lt.users, e.UserID)
			u.toast(tr("{name} left", "name", e.Name))
		} else if us, ok := lt.users[e.UserID]; ok {
			us.Connected = false
			lt.users[e.UserID] = us
		}
	case "host_changed":
		lt.host = e.IsHost
		for id, us := range lt.users {
			us.Host = id == e.UserID
			lt.users[id] = us
		}
		if lt.host {
			u.toast(tr("You control the room now"))
		} else {
			u.toast(tr("{name} controls the room now", "name", e.Name))
		}
	case "kicked":
		u.toast(tr("You were removed from the room: {reason}", "reason", e.Reason))
		u.ltLeave()
		return
	case "sync_playback":
		if e.Action != nil && !lt.host {
			u.ltApplyAction(*e.Action)
		}
	case "sync_state":
		if e.State != nil && !lt.host {
			u.ltApplyState(*e.State)
		}
	case "suggestion_received":
		lt.suggests = append(lt.suggests, e)
		u.toast(tr("{name} suggested {track}", "name", e.Name, "track", e.Track.Title))
	case "suggestion_approved":
		u.toast(tr("Your suggestion was added: {track}", "track", e.Track.Title))
	case "suggestion_rejected":
		u.toast(tr("Your suggestion was not accepted"))
	case "error":
		u.core.Log("together", "%s: %s", e.Code, e.Reason)
		if e.Code != "stale_action" && e.Code != "stale_track" {
			u.toast(tr("Listen together: {error}", "error", firstNonEmpty(e.Reason, e.Code)))
		}
	case "disconnected", "server_restarting":
		tok, room := lt.lastTok, lt.room
		lt.c = nil
		if room != "" && tok != "" {
			u.toast(tr("Connection lost — reconnecting…"))
			singleShot(3000, func() {
				u.ltConnect(func() { _ = u.lt.c.Reconnect(tok) })
			})
		}
	}
	if u.togetherPg != nil {
		u.togetherPg.refresh()
	}
}

// projected is where the room is now (positions are stamped with server time).
func (u *UI) ltProjected(pos, at int64, playing bool) int64 {
	if playing && at > 0 && u.lt.c != nil {
		if d := u.lt.c.ServerTime() - at; d > 0 && d < 60000 {
			pos += d
		}
	}
	return pos
}

func (u *UI) ltApplyState(st together.State) {
	if st.Track == nil {
		return
	}
	u.lt.revision = st.Revision
	pos := u.ltProjected(st.PositionMs, st.LastUpdate, st.Playing)
	u.ltPlayRemote(*st.Track, pos, st.Playing)
}

func (u *UI) ltApplyAction(a together.Action) {
	u.lt.revision = a.Revision
	u.lt.applying = true
	defer func() { u.lt.applying = false }()
	switch a.Action {
	case "change_track":
		if a.Track != nil {
			u.ltPlayRemote(*a.Track, a.PositionMs, true)
		}
	case "play":
		pos := u.ltProjected(a.PositionMs, a.ServerTime, true)
		if a.TrackID != "" && a.TrackID != u.lt.trackID && a.Track != nil {
			u.ltPlayRemote(*a.Track, pos, true)
			return
		}
		u.SeekTo(pos)
		if u.player != nil {
			_ = u.player.SetPaused(false)
		}
	case "pause":
		if u.player != nil {
			_ = u.player.SetPaused(true)
		}
		u.SeekTo(a.PositionMs)
	case "seek":
		u.SeekTo(u.ltProjected(a.PositionMs, a.ServerTime, !u.state().Paused))
	}
}

// ltPlayRemote plays the room's track (as a listener).
func (u *UI) ltPlayRemote(t together.Track, pos int64, playing bool) {
	u.lt.applying = true
	defer func() { u.lt.applying = false }()
	if t.ID != u.lt.trackID || u.current() == nil {
		u.lt.trackID = t.ID
		u.restorePos = pos
		u.PlayList([]core.Track{trackFromRoom(t)}, 0)
		rev := u.lt.revision
		id := t.ID
		singleShot(2500, func() { // tell the room we are ready
			if u.lt.c != nil {
				_ = u.lt.c.BufferReady(id, rev)
			}
		})
		if !playing {
			singleShot(2000, func() {
				if u.player != nil {
					_ = u.player.SetPaused(true)
				}
			})
		}
		return
	}
	u.SeekTo(pos)
	if u.player != nil {
		_ = u.player.SetPaused(!playing)
	}
}

// ---------------------------------------------------------------- host hooks

// ltSendCurrent shares the playing track with the room (host).
func (u *UI) ltSendCurrent() {
	lt := &u.lt
	if lt.c == nil || !lt.host || lt.applying {
		return
	}
	cur := u.current()
	if cur == nil {
		return
	}
	page := ""
	if u.stream != nil {
		page = u.stream.PageURL
	}
	id := ytID(*cur, page)
	if id == "" {
		u.toast(tr("This song can't be shared with the room (it is not on YouTube)"))
		return
	}
	lt.trackID = id
	rt := roomTrack(*cur, id)
	_ = lt.c.Playback(together.Action{Action: "change_track", TrackID: id, Track: &rt})
	st := u.state()
	act := "play"
	if st.Paused {
		act = "pause"
	}
	_ = lt.c.Playback(together.Action{Action: act, TrackID: id, PositionMs: st.PositionMs, Track: &rt})
}

// ltPlayState sends play/pause (host).
func (u *UI) ltPlayState() {
	lt := &u.lt
	if lt.c == nil || !lt.host || lt.applying || lt.trackID == "" {
		return
	}
	act := "play"
	if u.state().Paused {
		act = "pause"
	}
	_ = lt.c.Playback(together.Action{Action: act, TrackID: lt.trackID, PositionMs: u.state().PositionMs})
}

// ltSeek sends a seek (host).
func (u *UI) ltSeek(ms int64) {
	lt := &u.lt
	if lt.c == nil || !lt.host || lt.applying || lt.trackID == "" {
		return
	}
	_ = lt.c.Playback(together.Action{Action: "seek", TrackID: lt.trackID, PositionMs: ms})
}

// ltGuestBlocked stops listeners from changing the shared playback.
func (u *UI) ltGuestBlocked() bool {
	if u.lt.c != nil && u.lt.room != "" && !u.lt.host && !u.lt.applying {
		u.toast(tr("The host controls the music — suggest songs from their menu"))
		return true
	}
	return false
}

// ltSuggest proposes a track to the host (listener).
func (u *UI) ltSuggest(t core.Track) {
	id := ytID(t, "")
	if id == "" {
		u.toast(tr("Only songs from YouTube can be suggested"))
		return
	}
	_ = u.lt.c.Suggest(roomTrack(t, id))
	u.toast(tr("Suggested: {track}", "track", t.Title))
}

// ---------------------------------------------------------------- page

type togetherPage struct {
	u    *UI
	w    *qt.QScrollArea
	body *qt.QVBoxLayout
}

func (u *UI) buildTogetherPage() *togetherPage {
	tp := &togetherPage{u: u}
	sa, b := scrollPage(tr("Listen together"), tr("Listen in sync with friends — also with people on Metrolist and SimpMusic. The host controls the music; the others follow and can suggest songs."))
	tp.w = sa
	bw := qt.NewQWidget2()
	tp.body = vbox(bw, nil, 8)
	b.AddWidget(bw)
	b.AddStretch()
	return tp
}

func (tp *togetherPage) refresh() {
	u := tp.u
	lt := &u.lt
	clearLayout(tp.body.QLayout)
	add := func(w *qt.QWidget) { tp.body.AddWidget(w) }
	btn := func(text string, primary bool, f func()) *qt.QPushButton {
		x := qt.NewQPushButton3(text)
		if primary {
			role(x.QWidget, "primary")
		}
		x.OnClicked(f)
		return x
	}
	if lt.room == "" {
		row := qt.NewQHBoxLayout2()
		row.SetSpacing(8)
		row.AddWidget(btn(tr("Create a room"), true, u.ltCreate).QWidget)
		code := qt.NewQLineEdit2()
		code.SetPlaceholderText(tr("Room code or invite link"))
		code.SetMaximumWidth(260)
		code.OnReturnPressed(func() { u.ltJoin(code.Text()) })
		row.AddWidget(code.QWidget)
		row.AddWidget(btn(tr("Join"), false, func() { u.ltJoin(code.Text()) }).QWidget)
		row.AddStretch()
		tp.body.AddLayout(row.QLayout)
		add(label(tr("Your name in rooms: {name} (Settings → Appearance → Your name)", "name", u.ltName()), "", "hint").QWidget)
		srv := qt.NewQLineEdit2()
		srv.SetText(u.cfg.Prefs.TogetherServer)
		srv.SetPlaceholderText(together.DefaultServer)
		srv.SetMaximumWidth(420)
		srv.OnEditingFinished(func() { u.cfg.Prefs.TogetherServer = strings.TrimSpace(srv.Text()); _ = u.cfg.Save() })
		add(label(tr("Server (the Metrolist community server by default; metroserver is open source if you want your own)"), "", "hint").QWidget)
		add(srv.QWidget)
		return
	}
	title := label(lt.room, "TogetherCode", "title")
	title.SetTextInteractionFlags(qt.TextSelectableByMouse)
	add(title.QWidget)
	role2 := tr("You are listening")
	if lt.host {
		role2 = tr("You control the music")
	}
	add(label(role2, "", "subtitle").QWidget)
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(8)
	row.AddWidget(btn(tr("Copy code"), false, func() {
		qt.QGuiApplication_Clipboard().SetText(lt.room)
		u.toast(tr("Copied"))
	}).QWidget)
	row.AddWidget(btn(tr("Copy invite link"), false, func() {
		qt.QGuiApplication_Clipboard().SetText("https://metrolist.cc/listen?code=" + lt.room)
		u.toast(tr("Copied"))
	}).QWidget)
	if !lt.host {
		row.AddWidget(btn(tr("Sync now"), false, func() {
			if lt.c != nil {
				_ = lt.c.RequestSync()
			}
		}).QWidget)
	}
	row.AddWidget(btn(tr("Leave"), false, u.ltLeave).QWidget)
	row.AddStretch()
	tp.body.AddLayout(row.QLayout)
	if lt.c != nil {
		add(label(tr("Latency: {n} ms", "n", fmt.Sprint(lt.c.RTT())), "", "hint").QWidget)
	}

	if lt.host && len(lt.pending) > 0 {
		add(label(tr("Want to join"), "", "heading").QWidget)
		for id, name := range lt.pending {
			id, name := id, name
			r := qt.NewQHBoxLayout2()
			r.AddWidget2(label(name, "", "card-title").QWidget, 1)
			r.AddWidget(btn(tr("Let in"), true, func() { _ = lt.c.Approve(id); delete(lt.pending, id); tp.refresh() }).QWidget)
			r.AddWidget(btn(tr("Refuse"), false, func() { _ = lt.c.Reject(id, ""); delete(lt.pending, id); tp.refresh() }).QWidget)
			tp.body.AddLayout(r.QLayout)
		}
	}
	add(label(tr("People"), "", "heading").QWidget)
	for _, us := range lt.users {
		us := us
		r := qt.NewQHBoxLayout2()
		name := us.Name
		if us.Host {
			name += "  ·  " + tr("host")
		}
		if !us.Connected {
			name += "  ·  " + tr("offline")
		}
		r.AddWidget2(label(name, "", "card-title").QWidget, 1)
		if lt.host && !us.Host {
			r.AddWidget(btn(tr("Give control"), false, func() { _ = lt.c.TransferHost(us.ID) }).QWidget)
			r.AddWidget(btn(tr("Remove"), false, func() { _ = lt.c.Kick(us.ID, "") }).QWidget)
		}
		tp.body.AddLayout(r.QLayout)
	}
	if lt.host && len(lt.suggests) > 0 {
		add(label(tr("Suggestions"), "", "heading").QWidget)
		for i, sg := range lt.suggests {
			i, sg := i, sg
			r := qt.NewQHBoxLayout2()
			r.AddWidget2(label(sg.Track.Title+" — "+sg.Track.Artist+"  ·  "+sg.Name, "", "card-title").QWidget, 1)
			r.AddWidget(btn(tr("Play next"), true, func() {
				_ = lt.c.ApproveSuggestion(sg.SuggestionID)
				u.PlayNext(trackFromRoom(*sg.Track))
				lt.suggests = append(lt.suggests[:i], lt.suggests[i+1:]...)
				tp.refresh()
			}).QWidget)
			r.AddWidget(btn(tr("Refuse"), false, func() {
				_ = lt.c.RejectSuggestion(sg.SuggestionID, "")
				lt.suggests = append(lt.suggests[:i], lt.suggests[i+1:]...)
				tp.refresh()
			}).QWidget)
			tp.body.AddLayout(r.QLayout)
		}
	}
}
