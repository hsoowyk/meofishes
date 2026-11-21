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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"meofishes/assets"
	"meofishes/internal/app"
	"meofishes/internal/config"
	"meofishes/internal/core"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
	"meofishes/internal/ui"
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
	os.Exit(ui.Run(assets.FS))
}

func manager() (*plugin.Manager, *config.Config) {
	c := app.New(assets.FS)
	c.Host.Logf = func(id, f string, a ...any) { fmt.Fprintf(os.Stderr, "[%s] %s\n", id, fmt.Sprintf(f, a...)) }
	return c.Plugins, c.Cfg
}

func cli(args []string) (int, bool) {
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Println(strings.TrimSpace(`
meofishes ` + config.Version + ` — music from plugins

  meofishes                             open the app
  meofishes lyrics "Artist" "Title" [s] test the lyrics chain in the terminal
  meofishes search <plugin> <query>     test a plugin's search (and play the 1st result)
  meofishes check <plugin folder>       validate a plugin.json
  meofishes css <theme folder>          print the compiled Qt stylesheet of a theme
  meofishes modules                     list the Go libraries plugins can use
  meofishes types                       list the plugin types and what they allow
  meofishes setup                       install the default themes and plugins
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
		fmt.Println("meofishes", config.Version)
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
