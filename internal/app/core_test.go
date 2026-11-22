package app

import (
	"strings"
	"testing"

	"meofishes/assets"
	"meofishes/internal/plugin"
)

func TestCoreFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("XDG_DATA_HOME", dir+"/data")
	t.Setenv("XDG_CACHE_HOME", dir+"/cache")
	c := New(assets.FS)
	defer c.Close()
	for _, id := range []string{"fishalike", "abyss", "sakura-paper", "ink", "moss", "daylight"} {
		if id != "fishalike" {
			if err := c.InstallEntry(entry(c, id)); err != nil {
				t.Fatal(err)
			}
		}
		c.Cfg.Theme = id
		comp, _, err := c.CompileTheme()
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !strings.Contains(comp.QSS, "#MiniPlayer") || comp.Props["#VideoLyric"]["font-size"] == "" || !strings.Contains(comp.QSS, `role="badge"`) || strings.Contains(comp.QSS, "alpha(") {
			t.Fatalf("%s: mini player / badge rules missing or not resolved", id)
		}
		if !strings.Contains(comp.QSS, "#LeftPanel") || comp.Props["#FullLyrics"]["font-size"] == "" {
			t.Fatalf("%s: missing rules; props=%v", id, comp.Props["#FullLyrics"])
		}
	}
	c.Cfg.Theme = "fishalike"
	c.Cfg.Style["#FullLyrics|-meo-font-size"] = "50px"
	c.Cfg.Style[":root|--red"] = "#00ff00"
	comp, _, err := c.CompileTheme()
	if err != nil {
		t.Fatal(err)
	}
	if comp.Props["#FullLyrics"]["font-size"] != "50px" || !strings.Contains(strings.ToLower(comp.Vars["--accent"]), "00ff00") {
		t.Fatalf("style layer not applied: %q %q", comp.Props["#FullLyrics"]["font-size"], comp.Vars["--accent"])
	}
	var ids []string
	for _, p := range c.Plugins.All() {
		ids = append(ids, p.Manifest.ID)
		if p.Err != nil {
			t.Errorf("%s: %v", p.Manifest.ID, p.Err)
		}
	}
	t.Log("plugins:", ids)
	if len(c.Plugins.WithCap(plugin.CapArtwork)) == 0 || len(c.Plugins.WithCap(plugin.CapMatch)) == 0 {
		t.Fatal("artwork/match plugins missing")
	}
}

func entry(c *Core, id string) (e catalogEntry) {
	for _, x := range c.Catalog.BuiltIn() {
		if x.ID == id {
			return x
		}
	}
	return
}
