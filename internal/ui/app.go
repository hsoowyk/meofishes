// Package ui is the Qt5 interface of meofishes (built with miqt).
package ui

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/builtin/localfiles"
	"meofishes/internal/builtin/meolyrics"
	"meofishes/internal/catalog"
	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/player"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
)

// App is the non-visual state.
type App struct {
	paths   config.Paths
	cfg     *config.Config
	host    *plugin.Host
	plugins *plugin.Manager
	themes  theme.Store
	catalog *catalog.Catalog
	mpv     *player.MPV

	queue  []core.Track
	qIndex int
	gen    int // playback generation, to drop stale async results

	lyrics     *core.Lyrics
	lyricsInfo string

	logMu sync.Mutex
	logs  []string
}

func (a *App) current() *core.Track {
	if a.qIndex >= 0 && a.qIndex < len(a.queue) {
		return &a.queue[a.qIndex]
	}
	return nil
}

func (a *App) log(id, format string, args ...any) {
	line := time.Now().Format("15:04:05") + " [" + id + "] " + fmt.Sprintf(format, args...)
	log.Println(line)
	a.logMu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
	a.logMu.Unlock()
}

// UI is the visual state.
type UI struct {
	app         *App
	qapp        *qt.QApplication
	win         *qt.QMainWindow
	style       *Styler
	anim        *Animator
	coverLoader *CoverLoader
	root        *Backdrop
	toastW      *Toast

	regions map[string]*qt.QWidget
	holder  *qt.QWidget // parent of unused regions
	layoutW *qt.QWidget // current layout container

	// widgets that are updated from playback
	nav         map[string]*NavButton
	pages       *qt.QStackedWidget
	pageIndex   map[string]int
	curPage     string
	covers      []*CoverView
	lyricViews  []*LyricsView
	lyricStatus []*qt.QLabel
	titleLbls   []*qt.QLabel
	artistLbls  []*qt.QLabel
	albumLbls   []*qt.QLabel
	playBtn     *IconButton
	shuffleBtn  *IconButton
	repeatBtn   *IconButton
	volBtn      *IconButton
	seek        *Slider
	vol         *Slider
	tElapsed    *qt.QLabel
	tTotal      *qt.QLabel
	queueLists  []*TrackList
	allLists    []*TrackList

	// pages
	home    *homePage
	search  *searchPage
	plugPg  *pluginsPage
	themePg *themesPage
	setPg   *settingsPage

	themeFiles map[string]time.Time
}

// Run starts the application and blocks until it quits.
func Run(assets fs.FS) int {
	paths := config.DefaultPaths()
	cfg := config.Load(paths)
	a := &App{paths: paths, cfg: cfg, qIndex: -1, themes: theme.Store{Dir: paths.Themes},
		catalog: &catalog.Catalog{Bundled: assets}}
	a.host = &plugin.Host{Version: config.Version, UserAgent: cfg.UserAgent(), DataDir: paths.PData,
		Client: &http.Client{Timeout: 25 * time.Second}, Logf: a.log}
	a.plugins = plugin.NewManager(a.host, paths.Plugins, &cfg.Plugins)
	a.plugins.CacheDir = filepath.Join(paths.Cache, "lyrics")
	a.plugins.Save = func() { _ = cfg.Save() }
	a.plugins.Register(plugin.Builtin{Manifest: meolyrics.Manifest, New: meolyrics.New})
	a.plugins.Register(plugin.Builtin{Manifest: localfiles.Manifest, New: localfiles.New})
	if len(cfg.Plugins.LyricsOrder) == 0 {
		cfg.Plugins.LyricsOrder = []string{meolyrics.ID}
	}
	a.firstRun()
	a.plugins.Load()

	qt.QCoreApplication_SetApplicationName("meofishes")
	u := &UI{app: a, style: &Styler{}, regions: map[string]*qt.QWidget{}, nav: map[string]*NavButton{}, pageIndex: map[string]int{}}
	u.qapp = qt.NewQApplication(os.Args)
	u.anim = NewAnimator()
	u.coverLoader = newCoverLoader(filepath.Join(paths.Cache, "covers"), func() string { return a.host.UserAgent })

	u.applyTheme(false) // before building, so widgets read the right props
	u.build()
	u.startPlayer()
	u.applyTheme(true)
	u.win.Show()
	u.enterAll()
	u.home.refresh()
	u.startTimers()

	code := qt.QApplication_Exec()
	a.plugins.Close()
	if a.mpv != nil {
		a.mpv.Close()
	}
	_ = cfg.Save()
	return code
}

// firstRun installs the bundled defaults (Fishalike, example plugins) and
// makes sure the default theme is always present.
func (a *App) firstRun() {
	if a.themes.Get("fishalike") == nil || !a.cfg.FirstRunDone {
		for _, e := range a.catalog.Defaults() {
			if e.Kind == "plugin" && a.cfg.FirstRunDone {
				continue
			}
			if err := a.installEntry(e); err != nil {
				a.log("setup", "install %s: %v", e.ID, err)
			}
		}
		a.cfg.FirstRunDone = true
		_ = a.cfg.Save()
	}
}

func (a *App) installEntry(e catalog.Entry) error {
	src, cleanup, err := a.catalog.Materialize(e)
	if err != nil {
		return err
	}
	defer cleanup()
	switch e.Kind {
	case "theme":
		_, err = a.themes.Install(src)
	case "plugin":
		_, err = a.plugins.InstallFrom(src)
	}
	return err
}

// ---------------------------------------------------------------- theme

func (u *UI) currentTheme() *theme.Theme {
	t := u.app.themes.Get(u.app.cfg.Theme)
	if t == nil {
		t = u.app.themes.Get("fishalike")
	}
	return t
}

// applyTheme compiles the theme and installs the stylesheet.
func (u *UI) applyTheme(notify bool) {
	t := u.currentTheme()
	if t == nil {
		u.app.log("theme", "no theme installed")
		return
	}
	for _, f := range u.app.themes.Fonts(t) {
		qt.QFontDatabase_AddApplicationFont(f)
	}
	c, files, err := u.app.themes.Compile(t, u.app.cfg.UserCSS)
	if err != nil {
		u.app.log("theme", "%s: %v", t.ID, err)
		if u.toastW != nil {
			u.toast("Erro no tema: " + err.Error())
		}
		if c == nil {
			return
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
		u.app.plugins.Emit(plugin.EventThemeChange, map[string]any{"theme": t.ID})
	}
}

func (u *UI) setTheme(id string) {
	u.app.cfg.Theme = id
	_ = u.app.cfg.Save()
	u.applyTheme(true)
	if u.themePg != nil {
		u.themePg.refresh()
	}
	if t := u.currentTheme(); t != nil {
		u.toast("Tema aplicado: " + t.Name)
	}
}

func (u *UI) checkThemeFiles() {
	if !u.app.cfg.LiveReload {
		return
	}
	for f, mt := range u.themeFiles {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(mt) {
			u.applyTheme(false)
			u.toast("Tema recarregado")
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

func (u *UI) startPlayer() {
	a := u.app
	m, err := player.Start(a.paths.Runtime, player.Events{
		OnEnd: func(reason string) {
			mainthread.Start(func() {
				if reason == "eof" {
					u.onTrackEnded()
				}
			})
		},
		OnError: func(err error) { mainthread.Start(func() { u.toast(err.Error()) }) },
	})
	if err != nil {
		a.log("player", "%v", err)
		singleShot(800, func() { u.toast("mpv não encontrado — instale o mpv para tocar músicas") })
		return
	}
	a.mpv = m
	_ = m.SetVolume(a.cfg.Volume)
}

func (u *UI) state() player.State {
	if u.app.mpv == nil {
		return player.State{Idle: true, Paused: true}
	}
	return u.app.mpv.State()
}

func (u *UI) startTimers() {
	t := qt.NewQTimer()
	t.SetInterval(200)
	lastPaused := true
	t.OnTimeout(func() {
		st := u.state()
		dur := st.DurationMs
		if cur := u.app.current(); cur != nil && dur == 0 {
			dur = cur.DurationMs
		}
		if dur > 0 {
			u.seek.SetValue(float64(st.PositionMs) / float64(dur))
		} else {
			u.seek.SetValue(0)
		}
		u.tElapsed.SetText(core.FormatMs(st.PositionMs))
		if dur > 0 {
			u.tTotal.SetText(core.FormatMs(dur))
		} else {
			u.tTotal.SetText("—")
		}
		playing := !st.Paused && !st.Idle
		if playing == lastPaused { // changed
			lastPaused = !playing
			if playing {
				u.playBtn.SetIcon("pause")
			} else {
				u.playBtn.SetIcon("play")
			}
			for _, v := range u.lyricViews {
				v.SetRunning(playing)
			}
		}
		if !playing {
			for _, v := range u.lyricViews {
				v.Update()
			}
		}
	})
	t.Start2()

	themeTimer := qt.NewQTimer()
	themeTimer.SetInterval(900)
	themeTimer.OnTimeout(u.checkThemeFiles)
	themeTimer.Start2()
}

// PlayList replaces the queue and starts track i.
func (u *UI) PlayList(ts []core.Track, i int) {
	a := u.app
	a.queue = append([]core.Track(nil), ts...)
	a.qIndex = i
	if a.cfg.Shuffle {
		u.shuffleRest()
	}
	u.playCurrent()
	u.refreshQueue()
}

func (u *UI) Enqueue(t core.Track) {
	a := u.app
	a.queue = append(a.queue, t)
	u.refreshQueue()
	u.toast("Na fila: " + t.String())
	if a.qIndex < 0 {
		a.qIndex = len(a.queue) - 1
		u.playCurrent()
	}
}

func (u *UI) shuffleRest() {
	a := u.app
	rest := a.queue[a.qIndex+1:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
}

func (u *UI) emitEnd() {
	if cur := u.app.current(); cur != nil {
		u.app.plugins.Emit(plugin.EventTrackEnd, map[string]any{"track": *cur, "positionMs": u.state().PositionMs})
	}
}

func (u *UI) playCurrent() {
	a := u.app
	t := a.current()
	if t == nil {
		return
	}
	a.gen++
	gen := a.gen
	track := *t
	u.showNowPlaying(track)
	u.setLyrics(nil, "Procurando letra…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		st, err := a.plugins.Resolve(ctx, track)
		mainthread.Start(func() {
			if gen != a.gen {
				return
			}
			if err != nil {
				u.toast("Não consegui tocar: " + err.Error())
				return
			}
			if a.mpv == nil {
				u.toast("mpv não está disponível")
				return
			}
			if err := a.mpv.Load(st.URL, st.Headers); err != nil {
				u.toast(err.Error())
				return
			}
			a.plugins.Emit(plugin.EventTrackStart, map[string]any{"track": track})
		})
		// lyrics: wait for the duration if the source did not know it
		if track.DurationMs == 0 && a.mpv != nil {
			for i := 0; i < 30 && gen == a.gen; i++ {
				if d := a.mpv.State().DurationMs; d > 0 {
					track.DurationMs = d
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
		}
		if gen != a.gen {
			return
		}
		l, info, err := a.plugins.FindLyrics(ctx, track)
		mainthread.Start(func() {
			if gen != a.gen {
				return
			}
			a.lyricsInfo = info
			if err != nil {
				u.setLyrics(nil, "Sem letra para esta música")
				return
			}
			u.setLyrics(l, "")
			a.plugins.Emit(plugin.EventLyricsFound, map[string]any{"track": track, "provider": l.Provider, "sync": l.Sync.String()})
		})
	}()
}

func (u *UI) onTrackEnded() {
	a := u.app
	u.emitEnd()
	if a.cfg.Repeat == "one" {
		u.playCurrent()
		return
	}
	u.Next(false)
}

func (u *UI) Next(manual bool) {
	a := u.app
	if len(a.queue) == 0 {
		return
	}
	if manual {
		u.emitEnd()
	}
	if a.qIndex+1 >= len(a.queue) {
		if a.cfg.Repeat != "all" {
			if !manual {
				a.qIndex = len(a.queue) // finished
				u.refreshQueue()
			}
			return
		}
		a.qIndex = 0
	} else {
		a.qIndex++
	}
	u.playCurrent()
	u.refreshQueue()
}

func (u *UI) Prev() {
	a := u.app
	if u.state().PositionMs > 4000 || a.qIndex <= 0 {
		u.SeekTo(0)
		return
	}
	u.emitEnd()
	a.qIndex--
	u.playCurrent()
	u.refreshQueue()
}

func (u *UI) TogglePlay() {
	a := u.app
	if a.mpv == nil {
		return
	}
	if u.state().Idle {
		if a.qIndex >= len(a.queue) {
			a.qIndex = 0
		}
		u.playCurrent()
		return
	}
	wasPaused := u.state().Paused
	_ = a.mpv.TogglePause()
	if wasPaused {
		a.plugins.Emit(plugin.EventResume, nil)
	} else {
		a.plugins.Emit(plugin.EventPause, nil)
	}
}

func (u *UI) SeekTo(ms int64) {
	if u.app.mpv != nil {
		_ = u.app.mpv.SeekTo(ms)
		u.app.plugins.Emit(plugin.EventSeek, map[string]any{"positionMs": ms})
	}
}

func (u *UI) setVolume(v float64) {
	u.app.cfg.Volume = v
	if u.app.mpv != nil {
		_ = u.app.mpv.SetVolume(v)
	}
	switch {
	case v == 0:
		u.volBtn.SetIcon("mute")
	case v < 45:
		u.volBtn.SetIcon("volume-low")
	default:
		u.volBtn.SetIcon("volume")
	}
}

// ---------------------------------------------------------------- now playing

func (u *UI) showNowPlaying(t core.Track) {
	for _, l := range u.titleLbls {
		l.SetText(t.Title)
	}
	for _, l := range u.artistLbls {
		l.SetText(t.Artist)
	}
	for _, l := range u.albumLbls {
		l.SetText(t.Album)
	}
	u.win.SetWindowTitle(t.String() + " — meofishes")
	for _, c := range u.covers {
		u.coverLoader.Into(t.Cover, c)
	}
	if t.Cover == "" {
		u.root.SetTint(theme.Color{}, false)
	} else {
		u.coverLoader.Get(t.Cover, func(px *qt.QPixmap) {
			if cur := u.app.current(); cur != nil && cur.Cover == t.Cover {
				col, ok := dominantColor(px)
				u.root.SetTint(col, ok)
			}
		})
	}
	for _, l := range u.allLists {
		l.MarkPlaying(&t)
	}
}

func (u *UI) setLyrics(l *core.Lyrics, msg string) {
	u.app.lyrics = l
	for _, v := range u.lyricViews {
		v.SetLyrics(l, msg)
	}
	status := msg
	if l != nil {
		status = fmt.Sprintf("%s · sincronia por %s", l.Provider, map[core.SyncType]string{core.SyncWord: "palavra", core.SyncLine: "linha", core.SyncNone: "nenhuma"}[l.Sync])
	}
	for _, s := range u.lyricStatus {
		s.SetText(status)
		s.SetToolTip(u.app.lyricsInfo)
	}
}

func (u *UI) refreshQueue() {
	for _, q := range u.queueLists {
		rest := u.app.queue
		q.SetTracks(rest, "A fila está vazia. Dê um duplo clique numa música ou use o + para adicionar.")
	}
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
