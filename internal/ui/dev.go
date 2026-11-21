package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/i18n"
	"meofishes/internal/netlog"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
)

// devPage is the developer mode: everything the app knows, live.
type devPage struct {
	u       *UI
	w       *qt.QWidget
	tabs    *qt.QTabWidget
	views   map[string]*qt.QPlainTextEdit
	filter  *qt.QLineEdit
	timer   *qt.QTimer
	paused  bool
	logSeen int
}

var devTabs = []struct{ id, title string }{
	{"overview", "Overview"}, {"log", "Log"}, {"network", "Network"}, {"plugins", "Plugins"},
	{"lyrics", "Lyrics"}, {"stream", "Stream"}, {"player", "Player"}, {"theme", "Theme"},
	{"queue", "Queue"}, {"config", "Config"}, {"i18n", "Translations"},
}

func (u *UI) buildDevPage() *devPage {
	d := &devPage{u: u, views: map[string]*qt.QPlainTextEdit{}}
	w, b := pageShell(tr("Developer mode"), tr("Live view of what meofishes is doing. F12 inspects the widget under the mouse; Ctrl+Shift+D toggles this mode."))
	d.w = w
	row := qt.NewQHBoxLayout2()
	row.SetSpacing(8)
	d.filter = qt.NewQLineEdit2()
	d.filter.SetPlaceholderText(tr("Filter (log and network)"))
	d.filter.SetClearButtonEnabled(true)
	d.filter.OnTextChanged(func(string) { d.logSeen = -1; d.update() })
	row.AddWidget2(d.filter.QWidget, 1)
	pause := qt.NewQPushButton3(tr("Pause"))
	pause.SetCheckable(true)
	pause.OnToggled(func(on bool) { d.paused = on })
	cp := qt.NewQPushButton3(tr("Copy tab"))
	cp.OnClicked(func() {
		if v := d.current(); v != nil {
			qt.QGuiApplication_Clipboard().SetText(v.ToPlainText())
			u.toast(tr("Copied"))
		}
	})
	clr := qt.NewQPushButton3(tr("Clear network"))
	clr.OnClicked(func() { netlog.Clear(); d.update() })
	reload := qt.NewQPushButton3(tr("Reload plugins"))
	reload.OnClicked(func() { u.core.Plugins.Load(); u.toast(tr("Plugins reloaded")) })
	for _, x := range []*qt.QPushButton{pause, cp, clr, reload} {
		row.AddWidget(x.QWidget)
	}
	b.AddLayout(row.QLayout)
	d.tabs = qt.NewQTabWidget2()
	d.tabs.SetObjectName("DevTabs")
	for _, t := range devTabs {
		v := qt.NewQPlainTextEdit2()
		v.SetObjectName("DevText")
		v.SetReadOnly(true)
		v.SetLineWrapMode(qt.QPlainTextEdit__NoWrap)
		d.views[t.id] = v
		d.tabs.AddTab(v.QWidget, tr(t.title))
	}
	d.tabs.OnCurrentChanged(func(int) { d.logSeen = -1; d.update() })
	b.AddWidget2(d.tabs.QWidget, 1)
	d.timer = qt.NewQTimer2(w.QObject)
	d.timer.SetInterval(1000)
	d.timer.OnTimeout(func() {
		if w.IsVisible() && !d.paused {
			d.update()
		}
	})
	d.timer.Start2()
	return d
}

func (d *devPage) refresh() { d.logSeen = -1; d.update() }

func (d *devPage) current() *qt.QPlainTextEdit {
	i := d.tabs.CurrentIndex()
	if i < 0 || i >= len(devTabs) {
		return nil
	}
	return d.views[devTabs[i].id]
}

func (d *devPage) match(s string) bool {
	f := strings.ToLower(strings.TrimSpace(d.filter.Text()))
	return f == "" || strings.Contains(strings.ToLower(s), f)
}

// set replaces a view's text keeping the scroll position.
func set(v *qt.QPlainTextEdit, text string) {
	if v.ToPlainText() == text {
		return
	}
	sb := v.VerticalScrollBar()
	pos := sb.Value()
	atEnd := pos >= sb.Maximum()-4
	v.SetPlainText(text)
	if atEnd {
		sb.SetValue(sb.Maximum())
	} else {
		sb.SetValue(pos)
	}
}

func jsonText(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (d *devPage) update() {
	i := d.tabs.CurrentIndex()
	if i < 0 {
		return
	}
	id := devTabs[i].id
	v := d.views[id]
	switch id {
	case "overview":
		set(v, d.overview())
	case "log":
		lines := d.u.core.Logs()
		if d.logSeen == len(lines) {
			return
		}
		d.logSeen = len(lines)
		var b strings.Builder
		for _, l := range lines {
			if s := l.String(); d.match(s) {
				b.WriteString(s + "\n")
			}
		}
		set(v, b.String())
	case "network":
		set(v, d.network())
	case "plugins":
		set(v, d.plugins())
	case "lyrics":
		set(v, d.lyrics())
	case "stream":
		set(v, d.stream())
	case "player":
		set(v, d.player())
	case "theme":
		set(v, d.theme())
	case "queue":
		set(v, d.queue())
	case "config":
		set(v, jsonText(d.u.cfg))
	case "i18n":
		set(v, d.i18n())
	}
}

func (d *devPage) overview() string {
	u := d.u
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	tw, tk := u.anim.Active()
	st := u.state()
	cur := "—"
	if c := u.current(); c != nil {
		cur = c.String() + " [" + c.Source + ":" + c.ID + "]"
	}
	pname := "—"
	if u.player != nil {
		pname = u.player.Name()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "meofishes %s · Go %s · %s/%s · Qt %s\n", config.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, qt.QLibraryInfo_Version().ToString())
	fmt.Fprintf(&b, "User-Agent: %s\n\n", config.UserAgent)
	fmt.Fprintf(&b, "Now:        %s\n", cur)
	fmt.Fprintf(&b, "Position:   %s / %s  paused=%v idle=%v buffering=%v volume=%.0f\n", core.FormatMs(st.PositionMs), core.FormatMs(st.DurationMs), st.Paused, st.Idle, st.Buffering, st.Volume)
	fmt.Fprintf(&b, "Player:     %s\n", pname)
	fmt.Fprintf(&b, "Queue:      %d tracks, index %d · prefetched %d\n", len(u.queue), u.qIndex, len(u.prefetched))
	fmt.Fprintf(&b, "Page:       %s · theme %s · language %s\n\n", u.curPage, u.cfg.Theme, i18n.Current().Code)
	fmt.Fprintf(&b, "Frames:     %d fps at %.0f Hz (0 = idle) · %d tweens · %d tickers · speed ×%.2f\n", u.fps.fps, u.anim.Hz, tw, tk, u.anim.Speed())
	fmt.Fprintf(&b, "Memory:     heap %s · sys %s · GC %d · goroutines %d\n", humanBytes(int64(m.HeapAlloc)), humanBytes(int64(m.Sys)), m.NumGC, runtime.NumGoroutine())
	fmt.Fprintf(&b, "Network:    %d requests since start (recording: %v)\n", netlog.Total(), netlog.Enabled())
	fmt.Fprintf(&b, "Covers:     %d in memory · artwork known for %d songs\n\n", len(u.coverLoader.pix), u.art.Size())
	fmt.Fprintf(&b, "Config:     %s\nData:       %s\nCache:      %s\nRuntime:    %s\n", u.core.Paths.Config, u.core.Paths.Data, u.core.Paths.Cache, u.core.Paths.Runtime)
	fmt.Fprintf(&b, "Executable: %s\n", func() string { e, _ := os.Executable(); return e }())
	return b.String()
}

func (d *devPage) network() string {
	var b strings.Builder
	es := netlog.Entries()
	if !netlog.Enabled() {
		b.WriteString(tr("Recording is off (Settings → Developer → Record network requests).") + "\n\n")
	}
	for i := len(es) - 1; i >= 0; i-- {
		e := es[i]
		line := fmt.Sprintf("%s %-6s %3d %6dms %8s  %s", e.At.Format("15:04:05.000"), e.Method, e.Status, e.Duration.Milliseconds(), humanBytes(max(0, e.Bytes)), e.URL)
		if e.Err != "" {
			line += "  ✗ " + e.Err
		}
		if d.match(line) {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func (d *devPage) plugins() string {
	u := d.u
	var b strings.Builder
	for _, p := range u.core.Plugins.All() {
		m := p.Manifest
		state := "enabled"
		if !u.core.Plugins.Enabled(m.ID) {
			state = "disabled"
		}
		fmt.Fprintf(&b, "■ %s (%s) v%s · %s · runtime %s · types [%s]\n", m.Name, m.ID, m.Version, state, firstNonEmpty(m.Runtime, "declarative"), strings.Join(m.Types, "] ["))
		fmt.Fprintf(&b, "  provides: %s\n", strings.Join(p.Caps(), ", "))
		if den := p.Denied(); len(den) > 0 {
			fmt.Fprintf(&b, "  denied by its types: %s\n", strings.Join(den, ", "))
		}
		if p.Dir != "" {
			fmt.Fprintf(&b, "  folder: %s\n", p.Dir)
		}
		if len(m.Modules) > 0 {
			fmt.Fprintf(&b, "  Go modules: %s · hosts: %s\n", strings.Join(m.Modules, ", "), strings.Join(m.Hosts, ", "))
		}
		if len(m.Requires) > 0 {
			fmt.Fprintf(&b, "  requires: %s\n", strings.Join(m.Requires, ", "))
		}
		if p.Err != nil {
			fmt.Fprintf(&b, "  ERROR: %v\n", p.Err)
		}
		if p.Stats != nil {
			stats := p.Stats.Snapshot()
			var caps []string
			for c := range stats {
				caps = append(caps, c)
			}
			sort.Strings(caps)
			for _, c := range caps {
				s := stats[c]
				avg := time.Duration(0)
				if s.Count > 0 {
					avg = s.Total / time.Duration(s.Count)
				}
				fmt.Fprintf(&b, "  %-10s %4d calls · %d errors · avg %v · last %v at %s", c, s.Count, s.Errors, avg.Round(time.Millisecond), s.Last.Round(time.Millisecond), s.LastAt.Format("15:04:05"))
				if s.LastErr != "" {
					fmt.Fprintf(&b, " · last error: %s", s.LastErr)
				}
				b.WriteString("\n")
			}
		}
		set := u.core.Plugins.Settings(m.ID)
		if len(set) > 0 {
			var ks []string
			for k := range set {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			for _, k := range ks {
				val := set[k]
				for _, st := range m.Settings {
					if st.Key == k && st.Type == "password" && val != "" {
						val = "••••"
					}
				}
				fmt.Fprintf(&b, "  setting %s = %q\n", k, val)
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Priority: %s\n", jsonText(u.cfg.Plugins.Order))
	fmt.Fprintf(&b, "Events: %s\n", strings.Join(plugin.EventNames, ", "))
	return b.String()
}

func (d *devPage) lyrics() string {
	u := d.u
	var b strings.Builder
	if last := u.core.Plugins.LastLyricsSearch(); last != nil {
		fmt.Fprintf(&b, "Last search: %s (%s) at %s\n", last.Track.String(), core.FormatMs(last.Track.DurationMs), last.At.Format("15:04:05"))
		for _, a := range last.Attempts {
			b.WriteString("  " + jsonLine(a) + "\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Summary: %s\nOffset: %d ms\n\n", u.lyricsInfo, u.cfg.Prefs.LyricsOffsetMs)
	l := u.lyrics
	if l == nil {
		b.WriteString(tr("No lyrics loaded.") + "\n")
		return b.String()
	}
	pos := u.lyricsPos()
	fmt.Fprintf(&b, "Provider: %s · sync: %s · %d lines · credit: %s\n\n", l.Provider, l.Sync, len(l.Lines), l.Credit)
	for _, ln := range l.Lines {
		mark := "  "
		if pos >= ln.Start && pos < ln.End {
			mark = "▶ "
		}
		agent := ""
		if ln.Agent != "" {
			agent = " (" + ln.Agent + ")"
		}
		fmt.Fprintf(&b, "%s[%s–%s]%s %s\n", mark, core.FormatMs(ln.Start), core.FormatMs(ln.End), agent, ln.Text)
		if mark != "  " {
			for _, w := range ln.Words {
				fmt.Fprintf(&b, "      %6d–%6d %q\n", w.Start, w.End, w.Text)
			}
		}
	}
	return b.String()
}

func jsonLine(v any) string { b, _ := json.Marshal(v); return string(b) }

func (d *devPage) stream() string {
	u := d.u
	var b strings.Builder
	if u.stream != nil {
		fmt.Fprintf(&b, "Playing via: %s\nPlayer URL:  %s\nSource URL:  %s\nWeb page:    %v\nHeaders:     %s\n\n", u.stream.Via, u.stream.URL, u.streamFrom, u.stream.Page, jsonLine(u.stream.Headers))
	}
	if u.core.Proxy == nil {
		b.WriteString(tr("The chunked download proxy is not running.") + "\n")
		return b.String()
	}
	b.WriteString("Chunked download proxy:\n")
	for _, s := range u.core.Proxy.Stats() {
		mode := fmt.Sprintf("%d/%d parts", s.Done, s.Chunks)
		if s.Passthrough {
			mode = "passthrough (no ranges / live)"
		}
		fmt.Fprintf(&b, "  %s · %s · %s downloaded of %s · %d in flight · %s/s\n    %s\n", s.ID, mode, humanBytes(s.Bytes), humanBytes(max(0, s.Size)), s.Inflight, humanBytes(int64(s.Speed)), ellipsis(s.URL, 140))
		if s.Err != "" {
			fmt.Fprintf(&b, "    error: %s\n", s.Err)
		}
	}
	b.WriteString("\nPrefetched:\n")
	for k, p := range u.prefetched {
		fmt.Fprintf(&b, "  %s · via %s · %s ago\n", k, p.st.Via, time.Since(p.at).Round(time.Second))
	}
	if u.full.viz.spec != nil {
		ms, done, err := u.full.viz.spec.Progress()
		fmt.Fprintf(&b, "\nVisualizer: %s analyzed · done=%v · err=%v\n", core.FormatMs(ms), done, err)
	}
	return b.String()
}

func (d *devPage) player() string {
	u := d.u
	if u.mpv == nil {
		if u.player != nil {
			return jsonText(u.player.State())
		}
		return tr("No player.")
	}
	var b strings.Builder
	for _, p := range []string{"media-title", "path", "file-format", "audio-codec-name", "audio-params", "audio-bitrate",
		"demuxer-cache-duration", "demuxer-cache-state", "cache-speed", "audio-device", "volume", "replaygain", "af", "mpv-version", "ffmpeg-version"} {
		v, err := u.mpv.Get(p)
		if err != nil {
			fmt.Fprintf(&b, "%-24s (%v)\n", p, err)
			continue
		}
		s := jsonLine(v)
		if len(s) > 400 {
			s = s[:400] + "…"
		}
		fmt.Fprintf(&b, "%-24s %s\n", p, s)
	}
	return b.String()
}

func (d *devPage) theme() string {
	u := d.u
	c := u.style.c
	if c == nil {
		return ""
	}
	var b strings.Builder
	t := u.currentTheme()
	if t != nil {
		chain := []string{}
		for _, x := range u.core.Themes.Chain(t) {
			chain = append(chain, x.ID)
		}
		fmt.Fprintf(&b, "Theme: %s (%s) · chain: %s · dir: %s\n", t.Name, t.ID, strings.Join(chain, " → "), t.Dir)
	}
	fmt.Fprintf(&b, "Settings overrides: %d · user CSS: %d bytes · live reload: %v\n\n", len(u.cfg.Style), len(u.cfg.UserCSS), u.cfg.LiveReload)
	b.WriteString("Variables:\n")
	var ks []string
	for k := range c.Vars {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Fprintf(&b, "  %-22s %s\n", k, c.Vars[k])
	}
	b.WriteString("\n-meo-* properties:\n")
	var sels []string
	for s := range c.Props {
		sels = append(sels, s)
	}
	sort.Strings(sels)
	for _, s := range sels {
		fmt.Fprintf(&b, "  %s\n", s)
		var ps []string
		for p := range c.Props[s] {
			ps = append(ps, p)
		}
		sort.Strings(ps)
		for _, p := range ps {
			fmt.Fprintf(&b, "      -meo-%s: %s\n", p, c.Props[s][p])
		}
	}
	if len(u.cfg.Style) > 0 {
		b.WriteString("\nSettings layer (CSS):\n" + u.cfg.StyleCSS(u.cfg.Theme))
	}
	b.WriteString("\nCustom easings: " + strings.Join(theme.CustomEasings(), ", ") + "\n")
	b.WriteString("\nCompiled QSS:\n" + c.QSS)
	return b.String()
}

func (d *devPage) queue() string {
	u := d.u
	var b strings.Builder
	for i, t := range u.queue {
		mark := "  "
		if i == u.qIndex {
			mark = "▶ "
		}
		fmt.Fprintf(&b, "%s%3d. %s  [%s:%s] %s cover=%s\n", mark, i+1, t.String(), t.Source, t.ID, core.FormatMs(t.DurationMs), ellipsis(t.Cover, 60))
	}
	return b.String()
}

func (d *devPage) i18n() string {
	var b strings.Builder
	keys := uiStrings()
	for _, l := range i18n.Languages() {
		fmt.Fprintf(&b, "%-6s %-22s %3.0f%%\n", l.Code, l.Name, i18n.Coverage(l, keys)*100)
	}
	if m := i18n.Missing(); len(m) > 0 {
		fmt.Fprintf(&b, "\nMissing in %s (used so far):\n", i18n.Current().Code)
		for _, s := range m {
			b.WriteString("  " + s + "\n")
		}
	}
	return b.String()
}

// showText opens a read-only text dialog.
func (u *UI) showText(title, text string) {
	dlg := qt.NewQDialog(u.win.QWidget)
	dlg.SetWindowTitle(title)
	dlg.Resize(640, 520)
	b := vbox(dlg.QWidget, []int{14, 14, 14, 14}, 8)
	te := qt.NewQPlainTextEdit2()
	te.SetObjectName("DevText")
	te.SetReadOnly(true)
	te.SetPlainText(text)
	b.AddWidget2(te.QWidget, 1)
	ok := qt.NewQPushButton3(tr("Close"))
	ok.OnClicked(dlg.Accept)
	b.AddWidget3(ok.QWidget, 0, qt.AlignRight)
	dlg.Exec()
	dlg.DeleteLater()
}

// inspectUnderMouse (F12) shows the widget under the pointer: its object
// name, role and the CSS selectors and -meo-* properties that apply to it.
func (u *UI) inspectUnderMouse() {
	w := qt.QApplication_WidgetAt(qt.QCursor_Pos())
	if w == nil {
		return
	}
	var b strings.Builder
	depth := 0
	for x := w; x != nil && depth < 8; x = x.ParentWidget() {
		cls := x.MetaObject().ClassName()
		role := ""
		if v := x.Property("role"); v != nil {
			role = v.ToString()
		}
		fmt.Fprintf(&b, "%s%s", strings.Repeat("  ", depth), cls)
		if n := x.ObjectName(); n != "" {
			fmt.Fprintf(&b, "#%s", n)
		}
		if role != "" {
			fmt.Fprintf(&b, `[role="%s"]`, role)
		}
		fmt.Fprintf(&b, "  %dx%d at %d,%d\n", x.Width(), x.Height(), x.X(), x.Y())
		if depth == 0 {
			for _, kind := range []string{cls, "QWidget"} {
				for _, sel := range Selectors(x, kind) {
					if props := u.style.c.Props[sel]; len(props) > 0 {
						fmt.Fprintf(&b, "    %s:\n", sel)
						for k, v := range props {
							fmt.Fprintf(&b, "        -meo-%s: %s\n", k, v)
						}
					}
				}
			}
		}
		depth++
	}
	u.showText(tr("Inspector"), b.String())
}
