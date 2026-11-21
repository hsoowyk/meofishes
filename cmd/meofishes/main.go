// meofishes — a music player whose music comes from plugins.
//
//	meofishes                          open the app
//	meofishes lyrics "Artist" "Title"  test the lyrics chain from the terminal
//	meofishes search <plugin> <query>  test a source plugin
//	meofishes check <plugin folder>    validate a plugin.json
//	meofishes css <theme folder>       show the compiled Qt stylesheet of a theme
//	meofishes setup                    install the default themes and plugins
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"meofishes/assets"
	"meofishes/internal/app"
	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/ctl"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
	"meofishes/internal/ui"
	"meofishes/internal/update"
)

// Qt must run on the process' main OS thread. Lock it before anything else
// runs, so the Go scheduler never moves the main goroutine elsewhere.
func init() { runtime.LockOSThread() }

func main() {
	if len(os.Args) > 1 {
		if code, ok := cli(os.Args[1:]); ok {
			os.Exit(code)
		}
	}
	crashLog()
	os.Exit(ui.Run(assets.FS))
}

// crashLog keeps a log and the trace of any crash in
// ~/.cache/meofishes/meofishes.log and crash.log, so problems can be reported
// even when the app was opened from the applications menu.
func crashLog() {
	dir := config.DefaultPaths().Cache
	if f, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		fmt.Fprintf(f, "\n=== meofishes %s started %s ===\n", config.Version, time.Now().Format(time.RFC3339))
		_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	}
	logPath := filepath.Join(dir, "meofishes.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 4<<20 {
		_ = os.Rename(logPath, logPath+".old")
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
}

func manager() (*plugin.Manager, *config.Config) {
	c := app.New(assets.FS)
	c.Host.Logf = func(id, f string, a ...any) { fmt.Fprintf(os.Stderr, "[%s] %s\n", id, fmt.Sprintf(f, a...)) }
	return c.Plugins, c.Cfg
}

func cli(args []string) (int, bool) {
	if code, ok := control(args); ok {
		return code, true
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Println(strings.TrimSpace(`
meofishes ` + config.Version + ` — music from plugins

  meofishes                             open the app (or bring it back if it is open)

Control the open app:
  meofishes now [--json]                what is playing (and the lyric line being sung)
  meofishes resume | pause | toggle | stop
  meofishes next | prev
  meofishes play "query"                search every source and play the first result
  meofishes seek 1:30 | 90 | +10 | -10
  meofishes volume [50 | +5 | -5]       print or change the volume
  meofishes mute | like | unlike | shuffle | repeat [off|all|one]
  meofishes lyrics                      the lyrics of the song playing
  meofishes queue                       the queue
  meofishes sleep [minutes|end|off]     sleep timer (fades out, then pauses)
  meofishes speed [0.5 … 2]             playback speed
  meofishes show | fullscreen | quit

Tools:
  meofishes lyrics "Artist" "Title" [s] test the lyrics chain in the terminal
  meofishes search <plugin> <query>     test a plugin's search (and play the 1st result)
  meofishes check <plugin folder>       validate a plugin.json
  meofishes css <theme folder>          print the compiled Qt stylesheet of a theme
  meofishes modules                     list the Go libraries plugins can use
  meofishes types                       list the plugin types and what they allow
  meofishes setup                       install the default themes and plugins
  meofishes update [--check|--force]    install the latest build
  meofishes uninstall [--purge]         remove meofishes (--purge: also settings, library, caches)
  meofishes version                     print the version`))
		return 0, true
	case "lyrics":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, `usage: meofishes lyrics "Artist" "Title" [duration in seconds]`)
			return 2, true
		}
		m, _ := manager()
		defer m.Close()
		t := core.Track{Artist: args[1], Title: args[2]}
		if len(args) > 3 {
			var s int64
			fmt.Sscan(args[3], &s)
			t.DurationMs = s * 1000
		}
		l, info, err := m.FindLyricsFresh(context.Background(), t)
		fmt.Fprintln(os.Stderr, info)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1, true
		}
		fmt.Printf("# %s — sync: %s — %d lines\n", l.Provider, l.Sync, len(l.Lines))
		for _, ln := range l.Lines {
			fmt.Printf("[%s] %s\n", core.FormatMs(ln.Start), ln.Text)
			if len(ln.Words) > 0 && os.Getenv("MEO_WORDS") != "" {
				for _, w := range ln.Words {
					fmt.Printf("    %6d-%6d %q part=%v\n", w.Start, w.End, w.Text, w.Part)
				}
			}
		}
		return 0, true
	case "search":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: meofishes search <plugin> <query>")
			return 2, true
		}
		m, _ := manager()
		defer m.Close()
		p := m.Get(args[1])
		if p == nil || !p.Can(plugin.CapSearch) {
			fmt.Fprintf(os.Stderr, "plugin %q is not installed/enabled or cannot search\n", args[1])
			if p != nil && p.Err != nil {
				fmt.Fprintln(os.Stderr, p.Err)
			}
			return 1, true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := m.Search(ctx, p, strings.Join(args[2:], " "))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1, true
		}
		for i, t := range r.Tracks {
			fmt.Printf("%2d. %s — %s  [%s] %s\n", i+1, t.Artist, t.Title, core.FormatMs(t.DurationMs), t.StreamURL)
		}
		if len(r.Tracks) > 0 {
			st, err := m.Resolve(ctx, r.Tracks[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve:", err)
			} else {
				fmt.Printf("\nstream of the 1st: %s (web page=%v, via %s)\n", st.URL, st.Page, st.Via)
			}
		}
		return 0, true
	case "check":
		if len(args) < 2 {
			return 2, true
		}
		mf, err := plugin.LoadManifest(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "✗", err)
			return 1, true
		}
		fmt.Printf("✓ %s (%s) %s — types: %s, runtime: %s\n", mf.Name, mf.ID, mf.Version, strings.Join(mf.Types, ","), mf.Runtime)
		return 0, true
	case "css":
		if len(args) < 2 {
			return 2, true
		}
		dir, _ := filepath.Abs(args[1])
		t, err := theme.Load(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1, true
		}
		store := theme.Store{Dir: config.DefaultPaths().Themes}
		c, _, err := store.Compile(t, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		if c != nil {
			fmt.Println(c.QSS)
			fmt.Fprintf(os.Stderr, "%d variables, %d selectors with -meo-*\n", len(c.Vars), len(c.Props))
		}
		return 0, true
	case "version", "--version", "-v":
		fmt.Println("meofishes", config.Version, "("+config.Commit+")")
		return 0, true
	case "update":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		r, err := update.Latest(ctx, config.UpdateRepo, config.UserAgent)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			return 1, true
		}
		fmt.Printf("current: %s · latest: %s (%s)\n", config.Commit, r.Short(), r.Published.Local().Format("2006-01-02 15:04"))
		if !r.Newer(config.Commit) && !(len(args) > 1 && args[1] == "--force") {
			fmt.Println("already up to date (use --force to reinstall)")
			return 0, true
		}
		if len(args) > 1 && args[1] == "--check" {
			fmt.Println("an update is available: meofishes update")
			return 0, true
		}
		last := 0
		err = update.Apply(ctx, r, config.UserAgent, func(done, total int64) {
			if total > 0 && int(done*100/total) >= last+10 {
				last = int(done * 100 / total)
				fmt.Fprintf(os.Stderr, "\r%d%%", last)
			}
		})
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			return 1, true
		}
		fmt.Println("updated to", r.Short(), "— restart meofishes to use it")
		return 0, true
	case "uninstall":
		purge := len(args) > 1 && args[1] == "--purge"
		p := config.DefaultPaths()
		_, _ = ctl.Send("quit")
		removed, err := update.Uninstall(p.Config, p.Data, p.Cache, purge)
		for _, r := range removed {
			fmt.Println("removed", r)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "uninstall:", err)
			return 1, true
		}
		if !purge {
			fmt.Println("settings, library and plugins kept (meofishes uninstall --purge removes them too)")
		}
		return 0, true
	case "types":
		for _, t := range plugin.Types {
			fmt.Printf("[%s] allows: %s\n", t, strings.Join(plugin.Permissions[t], ", "))
		}
		return 0, true
	case "setup":
		c := app.New(assets.FS) // installs the defaults on the first run
		c.Close()
		fmt.Println("themes:", c.Paths.Themes)
		fmt.Println("plugins:", c.Paths.Plugins)
		return 0, true
	case "modules":
		for _, n := range plugin.ModuleNames() {
			fmt.Printf("%-9s %s\n", n, plugin.ModuleDoc(n))
		}
		return 0, true
	}
	return 0, false
}

// control sends commands to the running app (see internal/ctl).
func control(args []string) (int, bool) {
	cmd := args[0]
	switch cmd {
	case "now", "status", "resume", "pause", "toggle", "stop", "next", "prev", "previous", "back",
		"seek", "volume", "vol", "mute", "like", "unlike", "shuffle", "repeat", "queue", "show",
		"fullscreen", "quit", "sleep", "speed":
	case "play":
		if len(args) > 1 && args[1] == "--help" {
			return 0, false
		}
	case "lyrics":
		if len(args) > 1 && args[1] != "--json" { // "Artist" "Title": the provider test below
			return 0, false
		}
	default:
		return 0, false
	}
	jsonOut := false
	var rest []string
	for _, a := range args[1:] {
		if a == "--json" {
			jsonOut = true
		} else {
			rest = append(rest, a)
		}
	}
	raw, err := ctl.Send(cmd, rest...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "meofishes:", err)
		if err == ctl.ErrNotRunning {
			return 3, true
		}
		return 1, true
	}
	if jsonOut || len(raw) == 0 {
		if len(raw) > 0 {
			fmt.Println(string(raw))
		}
		return 0, true
	}
	switch cmd {
	case "now", "status":
		var s struct {
			Playing                bool
			Title, Artist, Album   string
			Line                   string
			PositionMs, DurationMs int64
			Volume                 int
			Liked, Shuffle         bool
			Repeat                 string
		}
		_ = json.Unmarshal(raw, &s)
		if s.Title == "" {
			fmt.Println("nothing playing")
			return 0, true
		}
		state := "▶"
		if !s.Playing {
			state = "⏸"
		}
		heart := ""
		if s.Liked {
			heart = " ♥"
		}
		fmt.Printf("%s %s — %s%s\n", state, s.Artist, s.Title, heart)
		if s.Album != "" {
			fmt.Printf("  %s\n", s.Album)
		}
		fmt.Printf("  %s / %s · volume %d%% · shuffle %v · repeat %s\n", core.FormatMs(s.PositionMs), core.FormatMs(s.DurationMs), s.Volume, s.Shuffle, s.Repeat)
		if s.Line != "" {
			fmt.Printf("  ♪ %s\n", s.Line)
		}
	case "lyrics":
		var l struct {
			Provider, Sync string
			Lines          []string
		}
		_ = json.Unmarshal(raw, &l)
		for _, ln := range l.Lines {
			fmt.Println(ln)
		}
		fmt.Fprintf(os.Stderr, "\n(%s, %s sync)\n", l.Provider, l.Sync)
	case "queue":
		var lines []string
		_ = json.Unmarshal(raw, &lines)
		for _, ln := range lines {
			fmt.Println(ln)
		}
	default:
		var v any
		_ = json.Unmarshal(raw, &v)
		fmt.Println(v)
	}
	return 0, true
}
