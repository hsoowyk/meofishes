// Package ui is the Qt5 interface of meofishes (built with miqt).
package ui

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/app"
	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/ctl"
	"meofishes/internal/i18n"
	"meofishes/internal/library"
	"meofishes/internal/mpris"
	"meofishes/internal/netlog"
	"meofishes/internal/offline"
	"meofishes/internal/player"
	"meofishes/internal/plugin"
	"meofishes/internal/sysproc"
	"meofishes/internal/theme"
	"meofishes/internal/update"
)

// tr translates a UI string (the English text is the key).
func tr(s string, kv ...string) string { return i18n.T(s, kv...) }

// UI is the whole interface plus the playback state.
type UI struct {
	core *app.Core
	cfg  *config.Config

	qapp        *qt.QApplication
	win         *qt.QMainWindow
	style       *Styler
	anim        *Animator
	coverLoader *CoverLoader
	art         *ArtCache
	icons       *IconPack
	root        *Backdrop
	toastW      *Toast

	// playback
	player          player.Backend
	mpv             *player.MPV // nil when a player plugin is used
	queue           []core.Track
	qIndex          int
	gen             int // playback generation, drops stale async results
	lyrics          *core.Lyrics
	lyricsInfo      string
	lyricsSearching bool
	stream          *plugin.Stream // what is playing (developer mode)
	streamFrom      string         // original URL before the proxy
	streamTrack     core.Track
	videoOn         bool
	lastVol         float64
	tray            *qt.QSystemTrayIcon
	trayNow         *qt.QAction
	quitting        bool
	mpris           *mpris.Server
	sleepTimer      *qt.QTimer
	sleepAt         time.Time
	sleepVol        float64
	mprisTick       int
	eqTimer         *qt.QTimer
	listenMs        int64 // time the current song really played (statistics)
	prefetched      map[string]prefetch
	autoplayOn      bool // a recommendation request is running
	restorePos      int64

	// widgets updated by playback
	nav         map[string]*NavButton
	pages       *qt.QStackedWidget
	pageIndex   map[string]int
	curPage     string
	history     []string // page history for "back"
	covers      []*CoverView
	lyricViews  []*LyricsView
	lyricStatus []*qt.QLabel
	titleLbls   []*qt.QLabel
	artistLbls  []*qt.QLabel
	albumLbls   []*qt.QLabel
	playBtns    []*IconButton
	shuffleBtns []*IconButton
	repeatBtns  []*IconButton
	likeBtns    []*IconButton
	volBtns     []*IconButton
	volSliders  []*Slider
	seeks       []*Slider
	tElapsed    []*qt.QLabel
	tTotal      []*qt.QLabel
	queueLists  []*TrackList
	allLists    []*TrackList
	full        *FullPlayer
	fps         *fpsMeter
	curLyric    *LyricLine

	// chrome
	menu       *qt.QWidget
	menuOpen   bool
	pageTitle  *qt.QLabel
	pageTitles map[string]string
	maxBtn     *IconButton
	grips      []*qt.QWidget
	gripEdges  []qt.Edge
	leftPanel  *qt.QWidget
	libNavBox  *qt.QVBoxLayout
	libRows    []*libRow
	logoW      *qt.QWidget
	libNav     *qt.QWidget
	nowCard    *qt.QWidget
	tour       *Tour
	pageFx     *Transition
	styleTimer *qt.QTimer

	// pages
	home    *homePage
	search  *searchPage
	lib     *libraryPage
	plugPg  *pluginsPage
	themePg *themesPage
	setPg   *settingsPage
	devPg   *devPage

	themeFiles map[string]time.Time
}

type prefetch struct {
	st   *plugin.Stream
	from string
	at   time.Time
}

// Run starts the application and blocks until it quits.
func Run(assets fs.FS) int {
	// already open? bring that window back instead of starting a second app
	if _, err := ctl.Send("show"); err == nil {
		return 0
	}
	c := app.New(assets)
	u := &UI{core: c, cfg: c.Cfg, qIndex: -1, style: &Styler{}, nav: map[string]*NavButton{},
		pageIndex: map[string]int{}, prefetched: map[string]prefetch{}, pageTitles: map[string]string{}}
	qt.QCoreApplication_SetApplicationName("meofishes")
	qt.QCoreApplication_SetApplicationVersion(config.Version)
	qt.QGuiApplication_SetDesktopFileName("meofishes")
	// mpv can only draw video inside our window on X11 (XWayland on Wayland)
	if c.Cfg.Prefs.VideoX11 && os.Getenv("QT_QPA_PLATFORM") == "" && os.Getenv("WAYLAND_DISPLAY") != "" {
		os.Setenv("QT_QPA_PLATFORM", "xcb")
	}
	u.qapp = qt.NewQApplication(os.Args)
	if i18n.Current().RTL {
		qt.QGuiApplication_SetLayoutDirection(qt.RightToLeft)
	}
	u.anim = NewAnimator()
	u.style.speed = func() float64 { return u.anim.Speed() }
	u.anim.Speed = func() float64 {
		if u.cfg.Prefs.ReduceMotion {
			return 0
		}
		return u.cfg.Prefs.AnimSpeed
	}
	u.fps = &fpsMeter{}
	u.anim.OnFrame = u.fps.frame
	u.coverLoader = newCoverLoader(c.Paths.Cache+"/covers", func() string { return config.UserAgent })
	u.art = newArtCache(u, c.Paths.Cache+"/artwork.json")
	u.icons = &IconPack{}
	u.icons.Load(c.Plugins.IconPack())
	c.StartProxy()

	u.applyTheme(false) // before building, so widgets read the right props
	u.build()
	u.startPlayer()
	u.applyTheme(true)
	if u.cfg.Maximized {
		u.win.ShowMaximized()
	} else {
		u.win.Show()
	}
	u.anim.FollowScreen(u.win.QWidget) // animations run at the screen's refresh rate
	u.buildTray()
	u.startMPRIS()
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false) // hiding to the tray must not quit
	u.enterAll()
	u.startTimers()
	u.restoreQueue()
	if u.cfg.Prefs.AutoUpdate {
		singleShot(8000, func() { u.checkUpdate(false) })
	}
	if !u.cfg.Prefs.TutorialAsked {
		singleShot(700, func() { u.startTour(true) })
	}
	c.Plugins.OnChange(func() { mainthread.Start(u.pluginsChanged) })
	// `meofishes next`, `meofishes lyrics`… talk to us through a socket
	if stop, err := ctl.Serve(func(cmd string, args []string) (any, error) {
		type res struct {
			v   any
			err error
		}
		r := mainthread.Wait2(func() res { v, err := u.control(cmd, args); return res{v, err} })
		return r.v, r.err
	}); err == nil {
		defer stop()
	} else {
		c.Log("ctl", "%v", err)
	}
	c.Library.OnChange(func() { mainthread.Start(u.libraryChanged) })

	code := qt.QApplication_Exec()
	u.stopMPRIS()
	if u.streamTrack.Title != "" {
		u.core.Library.RecordListen(u.streamTrack, u.listenMs)
	}
	u.saveQueue()
	if u.player != nil {
		u.player.Close()
	}
	c.Close()
	return code
}

// restart re-executes the app (used after changing the language).
func (u *UI) restart() {
	u.saveQueue()
	if u.player != nil {
		u.player.Close()
	}
	u.core.Close()
	exe, err := os.Executable()
	if err == nil {
		_ = sysproc.Restart(exe, os.Args) // Unix: replaces us; Windows: starts a new copy
	}
	qt.QCoreApplication_Quit()
}

func (u *UI) pluginsChanged() {
	u.icons.Load(u.core.Plugins.IconPack())
	u.core.ApplyEasings()
	if u.home != nil {
		u.fillSources(u.home.sources, false)
	}
	if u.search != nil {
		u.fillSources(u.search.sources, true)
	}
	u.win.Update()
}

func (u *UI) libraryChanged() {
	if u.lib != nil {
		u.lib.refresh()
	}
	u.refreshLibraryNav()
	u.syncLike()
	for _, l := range u.allLists {
		l.refreshLikes()
	}
}

// ---------------------------------------------------------------- theme

func (u *UI) currentTheme() *theme.Theme { return u.core.CurrentTheme() }

// applyTheme compiles the theme (+ ani plugins + Settings style + user CSS)
// and installs the stylesheet.
func (u *UI) applyTheme(notify bool) {
	c, files, err := u.core.CompileTheme()
	if err != nil {
		u.core.Log("theme", "%v", err)
		if u.toastW != nil {
			u.toast(tr("Theme error: {error}", "error", err.Error()))
		}
		if c == nil {
			return
		}
	}
	if t := u.currentTheme(); t != nil {
		for _, f := range u.core.Themes.Fonts(t) {
			qt.QFontDatabase_AddApplicationFont(f)
		}
	}
	u.themeFiles = map[string]time.Time{}
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			u.themeFiles[f] = st.ModTime()
		}
	}
	u.qapp.SetStyleSheet(c.QSS)
	u.style.Set(c)
	if notify {
		if t := u.currentTheme(); t != nil {
			u.core.Plugins.Emit(plugin.EventThemeChange, map[string]any{"theme": t.ID})
		}
	}
}

func (u *UI) setTheme(id string) {
	u.cfg.Theme = id
	_ = u.cfg.Save()
	u.applyTheme(true)
	if u.themePg != nil {
		u.themePg.refresh()
	}
	if t := u.currentTheme(); t != nil {
		u.toast(tr("Theme applied: {name}", "name", t.Name))
	}
}

// setStyle changes one "selector|property" of the Settings style layer.
// The theme is recompiled once the changes stop for a moment (spin boxes and
// sliders send a change per step).
func (u *UI) setStyle(key, value string) {
	if strings.TrimSpace(value) == "" {
		delete(u.cfg.Style, key)
	} else {
		u.cfg.Style[key] = value
	}
	if u.styleTimer == nil {
		u.styleTimer = qt.NewQTimer()
		u.styleTimer.SetSingleShot(true)
		u.styleTimer.OnTimeout(func() {
			_ = u.cfg.Save()
			u.applyTheme(false)
		})
	}
	u.styleTimer.Start(220)
}

func (u *UI) checkThemeFiles() {
	if !u.cfg.LiveReload {
		return
	}
	for f, mt := range u.themeFiles {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(mt) {
			u.applyTheme(false)
			u.toast(tr("Theme reloaded"))
			return
		}
	}
}

func (u *UI) toast(msg string) {
	if u.toastW != nil {
		u.toastW.Show(msg)
	}
}

// ---------------------------------------------------------------- player

func (u *UI) mpvOptions() player.Options {
	pr := u.cfg.Prefs
	o := player.Options{CacheMB: pr.CacheMB, ReadaheadSecs: pr.ReadaheadSecs, ReplayGain: pr.ReplayGain,
		Normalize: pr.Normalize, EQ: u.eqGains(), Speed: pr.Speed, Device: pr.AudioDevice, Exclusive: pr.Exclusive, Volume: u.cfg.Volume,
		Extra: strings.Fields(pr.ExtraMpvArgs), VideoFmt: u.videoFormat(), AudioFmt: plugin.QualityFormat(pr.AudioQuality)}
	if u.full != nil && u.full.video != nil {
		o.Wid = u.full.video.WinId()
		o.X11Video = qt.QGuiApplication_PlatformName() == "xcb"
	}
	return o
}

// startPlayer starts mpv, or the xtm player plugin chosen in Settings.
func (u *UI) startPlayer() {
	ev := player.Events{
		OnEnd: func(reason string) {
			mainthread.Start(func() {
				if reason == "eof" {
					u.onTrackEnded()
				}
			})
		},
		OnError: func(err error) { mainthread.Start(func() { u.toast(err.Error()) }) },
	}
	if p := u.core.Plugins.PlayerPlugin(); p != nil {
		pp, err := plugin.NewPluginPlayer(p, ev)
		if err == nil {
			u.player, u.mpv = pp, nil
			_ = pp.SetVolume(u.cfg.Volume)
			u.core.Log("player", "using %s", p.Manifest.Name)
			return
		}
		u.core.Log("player", "%s: %v — falling back to mpv", p.Manifest.ID, err)
	}
	m, err := player.Start(u.core.Paths.Runtime, u.mpvOptions(), ev)
	if err != nil {
		u.core.Log("player", "%v", err)
		singleShot(800, func() { u.toast(tr("mpv not found — install mpv to play music")) })
		return
	}
	u.player, u.mpv = m, m
	_ = m.SetVolume(u.cfg.Volume)
}

// restartPlayer applies new audio settings (keeps the position).
func (u *UI) restartPlayer() {
	pos := u.state().PositionMs
	playing := !u.state().Paused && !u.state().Idle
	if u.player != nil {
		u.player.Close()
	}
	u.player, u.mpv = nil, nil
	u.startPlayer()
	if u.current() != nil && pos > 0 {
		u.restorePos = pos
		u.playCurrent()
		if !playing {
			singleShot(1500, func() {
				if u.player != nil {
					_ = u.player.SetPaused(true)
				}
			})
		}
	}
}

func (u *UI) state() player.State {
	if u.player == nil {
		return player.State{Idle: true, Paused: true}
	}
	return u.player.State()
}

func (u *UI) current() *core.Track {
	if u.qIndex >= 0 && u.qIndex < len(u.queue) {
		return &u.queue[u.qIndex]
	}
	return nil
}

func (u *UI) duration() int64 {
	d := u.state().DurationMs
	if cur := u.current(); cur != nil && d == 0 {
		d = cur.DurationMs
	}
	return d
}

func (u *UI) startTimers() {
	t := qt.NewQTimer()
	t.SetInterval(200)
	lastPlaying := false
	first := true
	t.OnTimeout(func() {
		st := u.state()
		dur := u.duration()
		v := 0.0
		if dur > 0 {
			v = float64(st.PositionMs) / float64(dur)
		}
		for _, s := range u.seeks {
			if s.IsVisible() {
				s.SetValue(v)
			}
		}
		total := "—"
		if dur > 0 {
			total = core.FormatMs(dur)
		}
		for i := range u.tElapsed {
			u.tElapsed[i].SetText(core.FormatMs(st.PositionMs))
			u.tTotal[i].SetText(total)
		}
		playing := !st.Paused && !st.Idle
		if playing {
			u.listenMs += 200
		}
		if playing != lastPlaying || first {
			first = false
			lastPlaying = playing
			icon := "play"
			if playing {
				icon = "pause"
			}
			for _, b := range u.playBtns {
				b.SetIcon(icon)
			}
			for _, lv := range u.lyricViews {
				lv.SetRunning(playing)
			}
			if u.full.open {
				u.full.syncRight()
			}
			u.syncMPRIS()
		}
		if u.mpris != nil {
			if u.mprisTick++; u.mprisTick%5 == 0 {
				u.mpris.SetPosition(st.PositionMs)
			}
		}
		if !playing {
			for _, lv := range u.lyricViews {
				if lv.IsVisible() {
					lv.Update()
				}
			}
		}
	})
	t.Start2()

	slow := qt.NewQTimer()
	slow.SetInterval(900)
	slow.OnTimeout(func() {
		u.checkThemeFiles()
		u.expirePrefetch()
	})
	slow.Start2()
}

// ---------------------------------------------------------------- queue

// PlayList replaces the queue and starts track i.
func (u *UI) PlayList(ts []core.Track, i int) {
	u.queue = append([]core.Track(nil), ts...)
	u.qIndex = i
	if u.cfg.Shuffle {
		u.shuffleRest()
	}
	u.playCurrent()
	u.refreshQueue()
}

// Enqueue adds a track at the end of the queue.
func (u *UI) Enqueue(t core.Track) {
	u.queue = append(u.queue, t)
	u.refreshQueue()
	u.toast(tr("Added to queue: {track}", "track", t.String()))
	if u.qIndex < 0 || u.qIndex >= len(u.queue)-1 && u.state().Idle {
		u.qIndex = len(u.queue) - 1
		u.playCurrent()
	}
}

// PlayNext inserts a track right after the current one.
func (u *UI) PlayNext(t core.Track) {
	at := u.qIndex + 1
	if at < 0 || at > len(u.queue) {
		at = len(u.queue)
	}
	u.queue = append(u.queue[:at], append([]core.Track{t}, u.queue[at:]...)...)
	u.refreshQueue()
	u.toast(tr("Playing next: {track}", "track", t.String()))
	if u.qIndex < 0 {
		u.qIndex = 0
		u.playCurrent()
	}
}

// MoveInQueue moves a queued track (drag and drop / menu).
func (u *UI) MoveInQueue(from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(u.queue) || to >= len(u.queue) {
		return
	}
	cur := u.current()
	var key string
	if cur != nil {
		key = cur.Key()
	}
	u.queue = library.MoveItem(u.queue, from, to)
	switch {
	case from == u.qIndex:
		u.qIndex = to
	case from < u.qIndex && to >= u.qIndex:
		u.qIndex--
	case from > u.qIndex && to <= u.qIndex:
		u.qIndex++
	}
	_ = key
	u.refreshQueue()
}

// RemoveFromQueue removes track i.
func (u *UI) RemoveFromQueue(i int) {
	if i < 0 || i >= len(u.queue) {
		return
	}
	u.queue = append(u.queue[:i], u.queue[i+1:]...)
	switch {
	case i < u.qIndex:
		u.qIndex--
	case i == u.qIndex:
		if u.player != nil {
			_ = u.player.Stop()
		}
		if u.qIndex >= len(u.queue) {
			u.qIndex = len(u.queue) - 1
		}
		if u.qIndex >= 0 {
			u.playCurrent()
		} else {
			u.showNowPlaying(core.Track{})
		}
	}
	u.refreshQueue()
}

func (u *UI) clearQueue() {
	if cur := u.current(); cur != nil {
		u.queue = []core.Track{*cur}
		u.qIndex = 0
	} else {
		u.queue, u.qIndex = nil, -1
	}
	u.refreshQueue()
}

func (u *UI) shuffleRest() {
	if u.qIndex+1 >= len(u.queue) {
		return
	}
	rest := u.queue[u.qIndex+1:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
}

func (u *UI) saveQueue() {
	if !u.cfg.Prefs.PersistQueue {
		u.cfg.Queue, u.cfg.QueueIndex, u.cfg.QueuePosMs = nil, 0, 0
	} else {
		u.cfg.Queue = u.queue
		if len(u.cfg.Queue) > 500 {
			u.cfg.Queue = u.cfg.Queue[:500]
		}
		u.cfg.QueueIndex = u.qIndex
		u.cfg.QueuePosMs = u.state().PositionMs
	}
	if u.win != nil {
		u.cfg.Maximized = u.win.IsMaximized()
		if !u.cfg.Maximized {
			u.cfg.WindowW, u.cfg.WindowH = u.win.Width(), u.win.Height()
		}
	}
	_ = u.cfg.Save()
}

func (u *UI) restoreQueue() {
	if !u.cfg.Prefs.PersistQueue || len(u.cfg.Queue) == 0 {
		return
	}
	u.queue = u.cfg.Queue
	u.qIndex = u.cfg.QueueIndex
	if u.qIndex < 0 || u.qIndex >= len(u.queue) {
		u.qIndex = 0
	}
	u.refreshQueue()
	if cur := u.current(); cur != nil {
		u.showNowPlaying(*cur)
		if u.cfg.Prefs.ResumeOnStart {
			u.restorePos = u.cfg.QueuePosMs
			u.playCurrent()
		}
	}
}

func (u *UI) emitEnd() {
	if cur := u.current(); cur != nil {
		u.core.Plugins.Emit(plugin.EventTrackEnd, map[string]any{"track": *cur, "positionMs": u.state().PositionMs})
	}
}

// ---------------------------------------------------------------- streams

// localize turns a resolved stream into what the player opens: web pages are
// resolved with yt-dlp (so the audio URL can be chunked), and HTTP files go
// through the chunked download proxy.
func (u *UI) localize(ctx context.Context, t core.Track, st *plugin.Stream) (*plugin.Stream, string) {
	pr := u.cfg.Prefs
	out := *st
	from := st.URL
	if out.Page {
		out.PageURL = out.URL
	}
	if out.Page && pr.DirectYtdlp && plugin.YtdlpPath() != "" {
		if direct, err := plugin.YtdlpStreamURL(ctx, out.URL); err == nil {
			out.PageURL = out.URL
			out.URL, out.Page = direct, false
			from = direct
		} else {
			u.core.Log("stream", "yt-dlp: %v (mpv will open the page)", err)
		}
	}
	l := strings.ToLower(out.URL)
	isList := false
	for _, ext := range []string{".pls", ".m3u", ".m3u8", ".asx", ".xspf"} {
		if strings.Contains(l, ext) {
			isList = true
		}
	}
	if pr.Chunked && u.core.Proxy != nil && !out.Page && !isList && t.DurationMs > 0 &&
		(strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")) {
		out.URL = u.core.Proxy.Register(out.URL, out.Headers)
		out.Headers = nil
	}
	return &out, from
}

// prepare resolves a track (using the prefetched stream when there is one).
func (u *UI) prepare(ctx context.Context, t core.Track, video bool) (*plugin.Stream, string, error) {
	if !video {
		if path, ok := u.core.Offline.Lookup(t); ok {
			return &plugin.Stream{URL: path, Via: "offline"}, path, nil
		}
		if p, ok := u.takePrefetch(t.Key()); ok {
			return p.st, p.from, nil
		}
	}
	st, err := u.core.Plugins.Resolve(ctx, t)
	if err != nil {
		return nil, "", err
	}
	if video && st.Page { // mpv opens the page itself, with video
		out := *st
		out.PageURL = st.URL
		return &out, st.URL, nil
	}
	l, from := u.localize(ctx, t, st)
	return l, from, nil
}

// takePrefetch is called from worker goroutines too: hop through the UI thread.
func (u *UI) takePrefetch(key string) (prefetch, bool) {
	p := mainthread.Wait2(func() prefetch {
		p, ok := u.prefetched[key]
		if ok && time.Since(p.at) < 20*time.Minute {
			delete(u.prefetched, key)
			return p
		}
		return prefetch{}
	})
	return p, p.st != nil
}

func (u *UI) expirePrefetch() {
	for k, p := range u.prefetched {
		if time.Since(p.at) > 20*time.Minute {
			delete(u.prefetched, k)
		}
	}
}

// prefetchNext resolves the next track and downloads its first chunks.
func (u *UI) prefetchNext(gen int) {
	if !u.cfg.Prefs.Prefetch || gen != u.gen || u.qIndex+1 >= len(u.queue) {
		return
	}
	next := u.queue[u.qIndex+1]
	if _, ok := u.prefetched[next.Key()]; ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		st, err := u.core.Plugins.Resolve(ctx, next)
		if err != nil {
			u.core.Log("prefetch", "%s: %v", next.String(), err)
			return
		}
		l, from := u.localize(ctx, next, st)
		if u.core.Proxy != nil && strings.Contains(l.URL, "127.0.0.1") {
			u.core.Proxy.Prefetch(l.URL)
		}
		mainthread.Start(func() {
			u.prefetched[next.Key()] = prefetch{st: l, from: from, at: time.Now()}
			u.core.Log("prefetch", "ready: %s (via %s)", next.String(), st.Via)
		})
	}()
}

// ---------------------------------------------------------------- playback

func (u *UI) playCurrent() {
	t := u.current()
	if t == nil {
		return
	}
	u.cacheLeaving()
	u.gen++
	gen := u.gen
	if u.player != nil && !u.state().Idle {
		_ = u.player.Stop() // the previous song stops now, not when the next one is ready
	}
	track := *t
	if u.cfg.Prefs.VideoAuto && track.VideoID != "" && u.mpv != nil && !u.videoOn {
		u.videoOn = true
		_ = u.mpv.SetVideo(true, u.videoFormat())
		u.full.syncVideo()
	}
	video := u.videoOn
	u.showNowPlaying(track)
	u.stream = nil
	u.lyricsSearching = true
	u.setLyrics(nil, tr("Looking for lyrics…"))
	u.core.Library.AddHistory(track)
	u.enrichCurrent(track)
	restore := u.restorePos
	u.restorePos = 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		t0 := time.Now()
		st, from, err := u.prepare(ctx, track, video)
		mainthread.Start(func() {
			if gen != u.gen {
				return
			}
			if err != nil {
				u.toast(tr("Could not play: {error}", "error", err.Error()))
				u.core.Log("player", "resolve %s: %v", track.String(), err)
				return
			}
			if u.player == nil {
				u.toast(tr("The player is not available"))
				return
			}
			if err := u.player.Load(st.URL, st.Headers); err != nil {
				u.toast(err.Error())
				return
			}
			u.stream, u.streamFrom, u.streamTrack = st, from, track
			if u.full.open {
				u.full.syncRight()
			}
			u.core.Log("player", "%s ← %s (%s, %v)", track.String(), st.Via, ellipsis(from, 80), time.Since(t0).Round(time.Millisecond))
			if restore > 0 {
				singleShot(900, func() {
					if gen == u.gen {
						u.SeekTo(restore)
					}
				})
			}
			u.core.Plugins.Emit(plugin.EventTrackStart, map[string]any{"track": track})
			u.notifyTrack()
			singleShot(1500, u.syncMPRIS) // the length is known now
			singleShot(4000, func() { u.prefetchNext(gen) })
		})
		// lyrics: wait for the duration if the source did not know it
		if track.DurationMs == 0 && u.player != nil {
			for i := 0; i < 30 && gen == u.gen; i++ {
				if d := u.player.State().DurationMs; d > 0 {
					track.DurationMs = d
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
		}
		if gen != u.gen {
			return
		}
		l, info, err := u.core.Plugins.FindLyrics(ctx, track)
		mainthread.Start(func() {
			if gen != u.gen {
				return
			}
			u.lyricsSearching = false
			u.lyricsInfo = info
			if err != nil {
				u.setLyrics(nil, tr("No lyrics for this song"))
				return
			}
			u.setLyrics(l, "")
			u.core.Plugins.Emit(plugin.EventLyricsFound, map[string]any{"track": track, "provider": l.Provider, "sync": l.Sync.String()})
		})
	}()
}

func (u *UI) onTrackEnded() {
	u.emitEnd()
	if u.cfg.Prefs.StopAfterTrack {
		u.cfg.Prefs.StopAfterTrack = false
		return
	}
	if u.cfg.Repeat == "one" {
		u.playCurrent()
		return
	}
	u.Next(false)
}

// Next plays the next track (autoplay asks src plugins for more at the end).
func (u *UI) Next(manual bool) {
	if len(u.queue) == 0 {
		return
	}
	if manual {
		u.emitEnd()
	}
	if u.qIndex+1 >= len(u.queue) {
		if u.cfg.Repeat == "all" {
			u.qIndex = 0
			u.playCurrent()
			u.refreshQueue()
			return
		}
		if u.cfg.Prefs.Autoplay {
			u.autoplay()
		}
		return
	}
	u.qIndex++
	u.playCurrent()
	u.refreshQueue()
}

// autoplay appends recommendations for the last track and plays them.
func (u *UI) autoplay() {
	cur := u.current()
	if cur == nil || u.autoplayOn {
		return
	}
	u.autoplayOn = true
	seed := *cur
	u.toast(tr("Finding similar songs…"))
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		ts, via, err := u.core.Plugins.Recommend(ctx, seed)
		if err != nil {
			return err
		}
		mainthread.Start(func() { u.addRecommendations(ts, via, true) })
		return nil
	}, func(err error) {
		u.autoplayOn = false
		if err != nil {
			u.core.Log("autoplay", "%v", err)
		}
	})
}

// startRadio queues tracks similar to t.
func (u *UI) startRadio(t core.Track) {
	u.toast(tr("Starting radio from {track}…", "track", t.String()))
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		ts, via, err := u.core.Plugins.Recommend(ctx, t)
		if err != nil {
			return err
		}
		mainthread.Start(func() {
			u.PlayList(append([]core.Track{t}, ts...), 0)
			u.core.Log("radio", "%d tracks from %s", len(ts), via)
		})
		return nil
	}, func(err error) {
		if err != nil {
			u.toast(tr("No recommendations: {error}", "error", err.Error()))
		}
	})
}

func (u *UI) addRecommendations(ts []core.Track, via string, play bool) {
	have := map[string]bool{}
	for _, t := range u.queue {
		have[t.Key()] = true
	}
	n := 0
	for _, t := range ts {
		if !have[t.Key()] {
			u.queue = append(u.queue, t)
			have[t.Key()] = true
			n++
		}
	}
	u.core.Log("autoplay", "%d tracks from %s", n, via)
	if n > 0 && play && u.qIndex+1 < len(u.queue) {
		u.qIndex++
		u.playCurrent()
	}
	u.refreshQueue()
}

// Prev restarts the track, or goes back when near its start.
func (u *UI) Prev() {
	if u.state().PositionMs > 4000 || u.qIndex <= 0 {
		u.SeekTo(0)
		return
	}
	u.emitEnd()
	u.qIndex--
	u.playCurrent()
	u.refreshQueue()
}

// TogglePlay pauses/resumes (or starts the queue).
func (u *UI) TogglePlay() {
	if u.player == nil {
		return
	}
	if u.state().Idle {
		if u.qIndex >= len(u.queue) || u.qIndex < 0 {
			u.qIndex = 0
		}
		u.playCurrent()
		return
	}
	wasPaused := u.state().Paused
	_ = u.player.TogglePause()
	if wasPaused {
		u.core.Plugins.Emit(plugin.EventResume, nil)
	} else {
		u.core.Plugins.Emit(plugin.EventPause, nil)
	}
}

// SeekTo jumps to a position.
func (u *UI) SeekTo(ms int64) {
	if u.player != nil {
		_ = u.player.SeekTo(max(0, ms))
		if u.mpris != nil {
			u.mpris.Seeked(max(0, ms))
		}
		u.core.Plugins.Emit(plugin.EventSeek, map[string]any{"positionMs": ms})
	}
}

func (u *UI) seekBy(secs int) {
	u.SeekTo(u.state().PositionMs + int64(secs)*1000)
}

func (u *UI) setVolume(v float64) {
	v = clamp(v, 0, 100)
	u.cfg.Volume = v
	if u.player != nil {
		_ = u.player.SetVolume(v)
	}
	icon := "volume"
	switch {
	case v == 0:
		icon = "mute"
	case v < 45:
		icon = "volume-low"
	}
	for _, b := range u.volBtns {
		b.SetIcon(icon)
	}
	for _, s := range u.volSliders {
		s.SetValue(v / 100)
	}
	u.syncMPRIS()
}

func (u *UI) toggleShuffle() {
	u.cfg.Shuffle = !u.cfg.Shuffle
	for _, b := range u.shuffleBtns {
		b.SetActive(u.cfg.Shuffle)
	}
	if u.cfg.Shuffle && u.qIndex >= 0 {
		u.shuffleRest()
		u.refreshQueue()
	}
	u.syncMPRIS()
}

func (u *UI) cycleRepeat() {
	switch u.cfg.Repeat {
	case "off", "":
		u.cfg.Repeat = "all"
	case "all":
		u.cfg.Repeat = "one"
	default:
		u.cfg.Repeat = "off"
	}
	u.syncRepeat()
}

func (u *UI) syncRepeat() {
	defer u.syncMPRIS()
	for _, b := range u.repeatBtns {
		switch u.cfg.Repeat {
		case "one":
			b.SetIcon("repeat-one")
			b.SetActive(true)
		case "all":
			b.SetIcon("repeat")
			b.SetActive(true)
		default:
			b.SetIcon("repeat")
			b.SetActive(false)
		}
	}
}

// ---------------------------------------------------------------- likes

func (u *UI) toggleLike(t core.Track) {
	on := !u.core.Library.IsLiked(t)
	u.core.Library.SetLiked(t, on)
	u.core.Plugins.Emit(plugin.EventLike, map[string]any{"track": t, "liked": on})
	if on {
		u.toast(tr("Added to Liked songs"))
	} else {
		u.toast(tr("Removed from Liked songs"))
	}
}

func (u *UI) syncLike() {
	cur := u.current()
	liked := cur != nil && u.core.Library.IsLiked(*cur)
	icon := "heart-outline"
	if liked {
		icon = "heart"
	}
	for _, b := range u.likeBtns {
		b.SetIcon(icon)
		b.SetActive(liked)
		b.SetEnabled(cur != nil)
	}
}

// ---------------------------------------------------------------- now playing

func (u *UI) showNowPlaying(t core.Track) {
	title, artist := t.Title, t.Artist
	if title == "" {
		title, artist = tr("Nothing playing"), tr("Pick a song to start")
	}
	for _, l := range u.titleLbls {
		l.SetText(title)
		l.SetToolTip(title)
	}
	for _, l := range u.artistLbls {
		l.SetText(artist)
	}
	for _, l := range u.albumLbls {
		l.SetText(t.Album)
	}
	if t.Title != "" {
		u.win.SetWindowTitle(t.String() + " — meofishes")
	} else {
		u.win.SetWindowTitle("meofishes")
	}
	u.setCover(t.Cover)
	for _, l := range u.allLists {
		l.MarkPlaying(&t)
	}
	u.syncLike()
	u.syncTray()
	u.syncMPRIS()
	if u.full != nil {
		u.full.trackChanged(t)
	}
}

func (u *UI) setCover(url string) {
	for _, c := range u.covers {
		u.coverLoader.Into(url, c)
	}
	if url == "" {
		u.root.SetTint(theme.Color{}, false)
		if u.full != nil {
			u.full.bg.SetTint(theme.Color{}, false)
		}
		return
	}
	u.coverLoader.Get(url, func(px *qt.QPixmap) {
		if cur := u.current(); cur != nil && cur.Cover == url {
			col, ok := dominantColor(px)
			u.root.SetTint(col, ok)
			if u.full != nil {
				u.full.bg.SetTint(col, ok)
				u.full.bg.SetImage(px)
				u.full.bg.SetPalette(paletteColors(px, 3))
			}
		}
	})
}

// enrichCurrent finds album art for the playing track when it has none.
func (u *UI) enrichCurrent(t core.Track) {
	key := t.Key()
	u.art.Get(t, func(url string) {
		if cur := u.current(); cur != nil && cur.Key() == key && url != "" && url != cur.Cover {
			cur.Cover = url
			u.setCover(url)
		}
	})
}

func (u *UI) setLyrics(l *core.Lyrics, msg string) {
	u.lyrics = l
	for _, v := range u.lyricViews {
		v.SetLyrics(l, msg)
	}
	status := msg
	if l != nil {
		sync := map[core.SyncType]string{core.SyncWord: tr("word sync"), core.SyncLine: tr("line sync"), core.SyncNone: tr("not synced")}[l.Sync]
		status = strings.Replace(l.Provider, "Built-in", tr("Built-in"), 1) + " · " + sync
	}
	for _, s := range u.lyricStatus {
		s.SetText(status)
		s.SetToolTip(u.lyricsInfo)
	}
	if u.full != nil {
		u.full.lyricsChanged(l)
	}
}

func (u *UI) refreshQueue() {
	for _, q := range u.queueLists {
		q.SetTracks(u.queue, tr("The queue is empty. Double-click a song or use + to add one."))
	}
	u.core.Plugins.Emit(plugin.EventQueue, map[string]any{"size": len(u.queue), "index": u.qIndex})
}

// lyricsPos is the position the lyrics follow (with the sync offset).
func (u *UI) lyricsPos() int64 {
	return u.state().PositionMs - int64(u.cfg.Prefs.LyricsOffsetMs)
}

// ---------------------------------------------------------------- helpers

func (u *UI) runAsync(work func() error, done func(err error)) {
	go func() {
		err := work()
		mainthread.Start(func() { done(err) })
	}()
}

func ellipsis(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func (u *UI) netlogSync() {
	netlog.Enable(u.cfg.Prefs.DevMode && u.cfg.Prefs.LogHTTP)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return tr(one, "n", "1")
	}
	return tr(many, "n", fmt.Sprint(n))
}

// ---------------------------------------------------------------- offline

// cacheLeaving keeps the song being left in the offline cache when it was
// downloaded in full by the chunk proxy (no extra download).
func (u *UI) cacheLeaving() {
	st, t := u.stream, u.streamTrack
	if t.Title != "" {
		u.core.Library.RecordListen(t, u.listenMs)
	}
	u.listenMs = 0
	if st == nil || !u.cfg.Prefs.AutoCache || u.core.Proxy == nil || t.Title == "" || u.core.Offline.Kind(t) != "" {
		return
	}
	data, ctype, src, ok := u.core.Proxy.Complete(st.URL)
	if !ok {
		return
	}
	go func() {
		if _, err := u.core.Offline.Save(t, data, offline.ExtFor(ctype, src), offline.Cache); err != nil {
			u.core.Log("offline", "cache %s: %v", t.String(), err)
		}
	}()
}

// downloadTracks saves songs on disk to play them without internet.
func (u *UI) downloadTracks(ts []core.Track) {
	var todo []core.Track
	for _, t := range ts {
		if u.core.Offline.Kind(t) != offline.Download && !u.core.Offline.Busy(t) {
			todo = append(todo, t)
		}
	}
	if len(todo) == 0 {
		u.toast(tr("Already downloaded"))
		return
	}
	u.toast(plural(len(todo), "Downloading {n} song…", "Downloading {n} songs…"))
	go func() {
		ok, failed := 0, 0
		for _, t := range todo {
			if err := u.downloadOne(t); err != nil {
				failed++
				u.core.Log("offline", "download %s: %v", t.String(), err)
			} else {
				ok++
			}
		}
		mainthread.Start(func() {
			if failed > 0 {
				u.toast(tr("Downloaded {ok}, failed {failed}", "ok", itoa(ok), "failed", itoa(failed)))
			} else {
				u.toast(plural(ok, "{n} song downloaded", "{n} songs downloaded"))
			}
			if u.lib != nil && u.curPage == "playlist" {
				u.lib.fillDetail()
			}
		})
	}()
}

func (u *UI) downloadOne(t core.Track) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	// the song playing now may already be complete in the proxy
	st := mainthread.Wait2(func() *plugin.Stream {
		if u.stream != nil && u.streamTrack.Key() == t.Key() {
			return u.stream
		}
		return nil
	})
	if st != nil && u.core.Proxy != nil {
		if data, ctype, src, ok := u.core.Proxy.Complete(st.URL); ok {
			_, err := u.core.Offline.Save(t, data, offline.ExtFor(ctype, src), offline.Download)
			return err
		}
	}
	rs, err := u.core.Plugins.Resolve(ctx, t)
	if err != nil {
		return err
	}
	url := rs.URL
	if rs.Page {
		if url, err = plugin.YtdlpStreamURL(ctx, rs.URL); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("not a downloadable URL: %s", url)
	}
	_, err = u.core.Offline.Fetch(ctx, t, url, rs.Headers, offline.Download, nil)
	return err
}

// ---------------------------------------------------------------- video

func (u *UI) videoFormat() string {
	h := u.cfg.Prefs.VideoQuality
	if h <= 0 {
		h = 720
	}
	return fmt.Sprintf("bestvideo[height<=?%d]+bestaudio/best[height<=?%d]/best", h, h)
}

// setVideo turns the video of the current song on or off. Turning it on
// reloads the song as video at the same position.
func (u *UI) setVideo(on bool) {
	if u.mpv == nil {
		u.toast(tr("Video needs mpv as the player"))
		return
	}
	if on && (u.full.video.WinId() == 0 || u.mpv.NoVideo) {
		u.toast(tr("Video is not available on this system"))
		return
	}
	u.videoOn = on
	_ = u.mpv.SetVideo(on, u.videoFormat())
	u.full.syncVideo()
	if cur := u.current(); cur != nil && !u.state().Idle {
		u.restorePos = u.state().PositionMs
		u.playCurrent()
	}
}

// ---------------------------------------------------------------- updates

// checkUpdate looks for a new build. manual=true reports "up to date" too
// and offers to install.
func (u *UI) checkUpdate(manual bool) {
	if manual {
		u.toast(tr("Checking for updates…"))
	}
	var rel *update.Release
	u.runAsync(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var err error
		rel, err = update.Latest(ctx, config.UpdateRepo, config.UserAgent)
		return err
	}, func(err error) {
		if err != nil {
			if manual {
				u.toast(tr("Could not check: {error}", "error", err.Error()))
			}
			return
		}
		if !rel.Newer(config.Commit) {
			if manual {
				if config.Commit == "dev" {
					u.toast(tr("This is a development build: updates are only for builds from the CI."))
				} else {
					u.toast(tr("meofishes is up to date"))
				}
			}
			return
		}
		if !manual {
			u.toast(tr("An update is available — Settings → About"))
			return
		}
		msg := tr("A new build is available ({commit}, {date}). Install it now?", "commit", rel.Short(), "date", rel.Published.Local().Format("2006-01-02 15:04"))
		if !u.confirm(tr("Update"), msg) {
			return
		}
		u.toast(tr("Downloading the update…"))
		u.runAsync(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			return update.Apply(ctx, rel, config.UserAgent, nil)
		}, func(err error) {
			if err != nil {
				qt.QMessageBox_Warning(u.win.QWidget, tr("Update"), err.Error())
				return
			}
			if u.confirm(tr("Update"), tr("Updated. Restart meofishes now?")) {
				u.restart()
			}
		})
	})
}
