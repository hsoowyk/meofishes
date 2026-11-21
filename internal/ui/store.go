package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/catalog"
	"meofishes/internal/config"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
)

func card(active bool) (*qt.QFrame, *qt.QVBoxLayout) {
	f := qt.NewQFrame2()
	role(f.QWidget, "card")
	setProp(f.QWidget, "active", boolStr(active))
	f.SetAttribute(qt.WA_StyledBackground)
	b := vbox(f.QWidget, []int{16, 14, 16, 14}, 6)
	return f, b
}

func chip(text string) *qt.QLabel { return label(text, "", "chip") }

// typeHelp explains each plugin type code (tooltips of the chips).
func typeHelp(t string) string {
	switch t {
	case plugin.TypeFnt:
		return tr("fnt: sources of music, video, lyrics and covers")
	case plugin.TypeSrc:
		return tr("src: search, home page, recommendations and playlists")
	case plugin.TypeAni:
		return tr("ani: changes the app's animations")
	case plugin.TypeDep:
		return tr("dep: tools and libraries for other plugins")
	case plugin.TypeXtm:
		return tr("xtm: extreme changes (another player, another icon pack)")
	case plugin.TypeEvt:
		return tr("evt: reacts to what you play (scrobbling…)")
	}
	return t
}

func typeChip(t string) *qt.QLabel {
	c := chip("[" + t + "]")
	c.SetToolTip(typeHelp(t))
	return c
}

func runtimeName(r string) string {
	switch r {
	case "declarative", "":
		return tr("declarative")
	case "script":
		return tr("JS script")
	case "process":
		return tr("process")
	case "builtin":
		return tr("built-in")
	}
	return r
}

func (u *UI) openFolder(dir string) {
	url := qt.QUrl_FromLocalFile(dir)
	qt.QDesktopServices_OpenUrl(url)
}

func (u *UI) confirm(title, text string) bool {
	return qt.QMessageBox_Question(u.win.QWidget, title, text) == qt.QMessageBox__Yes
}

// ---------------------------------------------------------------- plugins

type pluginsPage struct {
	w       *qt.QScrollArea
	list    *qt.QVBoxLayout
	catalog *qt.QVBoxLayout
	u       *UI
	busy    bool
}

func (u *UI) buildPluginsPage() *pluginsPage {
	pp := &pluginsPage{u: u}
	sa, b := scrollPage(tr("Plugins"), tr("Everything meofishes plays comes from a plugin. The [codes] say what each one is allowed to do."))
	pp.w = sa

	actions := qt.NewQHBoxLayout2()
	actions.SetSpacing(8)
	for _, a := range []struct {
		text string
		fn   func()
	}{
		{tr("Install .zip…"), func() {
			f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, tr("Install plugin"), homeDir(), tr("Plugin (*.zip)"))
			if f != "" {
				pp.install(f, "")
			}
		}},
		{tr("Install folder…"), func() {
			d := qt.QFileDialog_GetExistingDirectory2(u.win.QWidget, tr("Folder with plugin.json"))
			if d != "" {
				pp.install(d, "")
			}
		}},
		{tr("Install from URL…"), func() {
			url := qt.QInputDialog_GetText(u.win.QWidget, tr("Install from URL"), tr("URL of a .zip with plugin.json:"))
			if strings.TrimSpace(url) != "" {
				pp.install(strings.TrimSpace(url), "")
			}
		}},
		{tr("Reload"), func() { u.core.Plugins.Load(); pp.refresh(); u.toast(tr("Plugins reloaded")) }},
		{tr("Open folder"), func() { u.openFolder(u.core.Paths.Plugins) }},
	} {
		btn := qt.NewQPushButton3(a.text)
		btn.OnClicked(a.fn)
		actions.AddWidget(btn.QWidget)
	}
	actions.AddStretch()
	b.AddLayout(actions.QLayout)

	b.AddWidget(label(tr("Installed"), "", "heading").QWidget)
	lw := qt.NewQWidget2()
	pp.list = vbox(lw, nil, 10)
	b.AddWidget(lw)
	b.AddWidget(label(tr("Catalog"), "", "heading").QWidget)
	cw := qt.NewQWidget2()
	pp.catalog = vbox(cw, nil, 10)
	b.AddWidget(cw)
	b.AddStretch()
	return pp
}

// install installs from a path/URL, asking before running build commands.
func (pp *pluginsPage) install(src, name string) {
	u := pp.u
	u.toast(tr("Installing {name}…", "name", firstNonEmpty(name, filepath.Base(src))))
	var m *plugin.Manifest
	u.runAsync(func() error {
		var err error
		m, err = u.core.Plugins.InstallFrom(src)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, tr("Could not install"), err.Error())
			return
		}
		if m.Runtime == plugin.RuntimeProcess {
			msg := tr("“{name}” is a process plugin: it runs as a program on your computer.", "name", m.Name)
			if m.Build != "" {
				msg += "\n\n" + tr("It has to be built with this command:") + "\n\n    " + m.Build + "\n\n" + tr("Only continue if you trust where this plugin came from. Build it now?")
				if u.confirm(tr("Build plugin?"), msg) {
					pp.build(m)
					return
				}
				u.core.Plugins.SetEnabled(m.ID, false)
			}
		}
		u.toast(tr("Plugin installed: {name}", "name", m.Name))
		pp.refresh()
	})
}

func (pp *pluginsPage) build(m *plugin.Manifest) {
	u := pp.u
	u.toast(tr("Building {name}…", "name", m.Name))
	var out string
	u.runAsync(func() error {
		var err error
		out, err = u.core.Plugins.Build(m.ID)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, tr("Build failed"), err.Error()+"\n\n"+ellipsis(out, 1500))
		} else {
			u.toast(tr("{name} built and enabled", "name", m.Name))
		}
		pp.refresh()
	})
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func (pp *pluginsPage) refresh() {
	u := pp.u
	clearLayout(pp.list.QLayout)
	installed := map[string]string{}
	for _, p := range u.core.Plugins.All() {
		installed[p.Manifest.ID] = p.Manifest.Version
		pp.list.AddWidget(pp.pluginCard(p))
	}
	clearLayout(pp.catalog.QLayout)
	entries, errs := u.core.Catalog.List("plugin", u.cfg.Catalogs, config.UserAgent)
	shown := 0
	for _, e := range entries {
		if ver, ok := installed[e.ID]; ok && ver == e.Version {
			continue
		}
		pp.catalog.AddWidget(pp.catalogCard(e, installed[e.ID] != ""))
		shown++
	}
	if shown == 0 {
		pp.catalog.AddWidget(label(tr("Everything in the catalog is installed."), "", "hint").QWidget)
	}
	for _, err := range errs {
		pp.catalog.AddWidget(label(err.Error(), "", "card-error").QWidget)
	}
}

func (pp *pluginsPage) pluginCard(p *plugin.Plugin) *qt.QWidget {
	u := pp.u
	m := p.Manifest
	enabled := u.core.Plugins.Enabled(m.ID)
	f, b := card(enabled && p.Err == nil)
	top := qt.NewQHBoxLayout2()
	top.SetSpacing(8)
	name := m.Name
	if p.Builtin {
		name = tr(m.Name) // "Built-in"
	}
	top.AddWidget(label(name, "", "card-title").QWidget)
	for _, t := range m.Types {
		top.AddWidget(typeChip(t).QWidget)
	}
	top.AddWidget(chip(runtimeName(m.Runtime)).QWidget)
	top.AddStretch()
	tog := qt.NewQCheckBox3(tr("Enabled"))
	tog.SetChecked(enabled)
	tog.OnToggled(func(on bool) {
		err := u.core.Plugins.SetEnabled(m.ID, on)
		if err != nil {
			u.toast(m.Name + ": " + err.Error())
		}
		singleShot(0, pp.refresh)
	})
	top.AddWidget(tog.QWidget)
	b.AddLayout(top.QLayout)
	sub := fmt.Sprintf("v%s", m.Version)
	if m.Author != "" {
		sub += " · " + m.Author
	}
	if len(m.Modules) > 0 {
		sub += " · " + tr("Go modules:") + " " + strings.Join(m.Modules, ", ")
	}
	b.AddWidget(label(sub, "", "card-sub").QWidget)
	if m.Description != "" {
		d := label(m.Description, "", "card-sub")
		d.SetWordWrap(true)
		b.AddWidget(d.QWidget)
	}
	if caps := p.Caps(); len(caps) > 0 {
		b.AddWidget(label(tr("Provides: {caps}", "caps", strings.Join(caps, ", ")), "", "card-sub").QWidget)
	}
	if den := p.Denied(); len(den) > 0 {
		w := label(tr("Not allowed by its types: {caps}", "caps", strings.Join(den, ", ")), "", "card-error")
		w.SetWordWrap(true)
		b.AddWidget(w.QWidget)
	}
	if p.Err != nil {
		e := label("⚠ "+p.Err.Error(), "", "card-error")
		e.SetWordWrap(true)
		b.AddWidget(e.QWidget)
	}
	btns := qt.NewQHBoxLayout2()
	btns.SetSpacing(8)
	if len(m.Settings) > 0 {
		cfgBtn := qt.NewQPushButton3(tr("Configure"))
		cfgBtn.OnClicked(func() { u.pluginSettings(p, pp.refresh) })
		btns.AddWidget(cfgBtn.QWidget)
	}
	if m.Build != "" && !p.Builtin {
		bb := qt.NewQPushButton3(tr("Build"))
		bb.OnClicked(func() {
			if u.confirm(tr("Build plugin?"), tr("Run this in the plugin folder?")+"\n\n    "+m.Build) {
				pp.build(&m)
			}
		})
		btns.AddWidget(bb.QWidget)
	}
	if !p.Builtin {
		folder := qt.NewQPushButton3(tr("Folder"))
		folder.OnClicked(func() { u.openFolder(p.Dir) })
		btns.AddWidget(folder.QWidget)
		rm := qt.NewQPushButton3(tr("Remove"))
		role(rm.QWidget, "danger")
		rm.OnClicked(func() {
			if !u.confirm(tr("Remove plugin"), tr("Remove “{name}”?", "name", m.Name)) {
				return
			}
			if err := u.core.Plugins.Uninstall(m.ID); err != nil {
				u.toast(err.Error())
			}
			singleShot(0, pp.refresh)
		})
		btns.AddWidget(rm.QWidget)
	}
	btns.AddStretch()
	b.AddLayout(btns.QLayout)
	return f.QWidget
}

func (pp *pluginsPage) catalogCard(e catalog.Entry, update bool) *qt.QWidget {
	u := pp.u
	f, b := card(false)
	top := qt.NewQHBoxLayout2()
	top.SetSpacing(8)
	top.AddWidget(label(e.Name, "", "card-title").QWidget)
	for _, t := range e.Types {
		top.AddWidget(typeChip(t).QWidget)
	}
	if e.Runtime != "" {
		top.AddWidget(chip(runtimeName(e.Runtime)).QWidget)
	}
	top.AddStretch()
	btnText := tr("Install")
	if update {
		btnText = tr("Update")
	}
	inst := qt.NewQPushButton3(btnText)
	role(inst.QWidget, "primary")
	inst.OnClicked(func() {
		src, cleanup, err := u.core.Catalog.Materialize(e)
		if err != nil {
			u.toast(err.Error())
			return
		}
		// keep bundled temp folders until the install finishes
		pp.installThen(src, e.Name, cleanup)
	})
	top.AddWidget(inst.QWidget)
	b.AddLayout(top.QLayout)
	d := label(fmt.Sprintf("v%s · %s · %s", e.Version, firstNonEmpty(e.Author, "?"), e.Description), "", "card-sub")
	d.SetWordWrap(true)
	b.AddWidget(d.QWidget)
	return f.QWidget
}

func (pp *pluginsPage) installThen(src, name string, cleanup func()) {
	pp.install(src, name)
	singleShot(60000, cleanup) // the install copies the files; clean the temp copy later
}

// pluginSettings shows a form for a plugin's settings.
func (u *UI) pluginSettings(p *plugin.Plugin, after func()) {
	m := p.Manifest
	dlg := qt.NewQDialog(u.win.QWidget)
	dlg.SetWindowTitle(tr("Configure {name}", "name", m.Name))
	dlg.SetMinimumWidth(520)
	b := vbox(dlg.QWidget, []int{22, 20, 22, 18}, 8)
	b.AddWidget(label(m.Name, "", "card-title").QWidget)
	cur := u.core.Plugins.Settings(m.ID)
	getters := map[string]func() string{}
	for _, s := range m.Settings {
		s := s
		b.AddWidget(label(firstNonEmpty(s.Label, s.Key), "", "card-sub").QWidget)
		switch s.Type {
		case "bool":
			cb := qt.NewQCheckBox3(firstNonEmpty(s.Description, tr("Enabled")))
			cb.SetChecked(cur[s.Key] == "true")
			b.AddWidget(cb.QWidget)
			getters[s.Key] = func() string { return boolStr(cb.IsChecked()) }
			continue
		case "choice":
			combo := qt.NewQComboBox2()
			for i, c := range s.Choices {
				combo.AddItem(c)
				if c == cur[s.Key] {
					combo.SetCurrentIndex(i)
				}
			}
			b.AddWidget(combo.QWidget)
			getters[s.Key] = combo.CurrentText
		default:
			le := qt.NewQLineEdit2()
			le.SetText(cur[s.Key])
			le.SetPlaceholderText(s.Default)
			if s.Type == "password" {
				le.SetEchoMode(qt.QLineEdit__Password)
			}
			b.AddWidget(le.QWidget)
			getters[s.Key] = le.Text
		}
		if s.Description != "" {
			h := label(s.Description, "", "hint")
			h.SetWordWrap(true)
			b.AddWidget(h.QWidget)
		}
	}
	row := qt.NewQHBoxLayout2()
	row.AddStretch()
	cancel := qt.NewQPushButton3(tr("Cancel"))
	cancel.OnClicked(dlg.Reject)
	save := qt.NewQPushButton3(tr("Save"))
	role(save.QWidget, "primary")
	save.OnClicked(dlg.Accept)
	row.AddWidget(cancel.QWidget)
	row.AddWidget(save.QWidget)
	b.AddSpacing(8)
	b.AddLayout(row.QLayout)
	if dlg.Exec() == int(qt.QDialog__Accepted) {
		vals := map[string]string{}
		for k, g := range getters {
			vals[k] = strings.TrimSpace(g())
		}
		if err := u.core.Plugins.SetSettings(m.ID, vals); err != nil {
			u.toast(m.Name + ": " + err.Error())
		} else {
			u.toast(tr("Settings saved"))
		}
		if after != nil {
			after()
		}
		if m.Has(plugin.TypeSrc) {
			u.home.popFor = ""
			u.home.refresh()
		}
	}
	dlg.DeleteLater()
}

// ---------------------------------------------------------------- themes

type themesPage struct {
	w       *qt.QScrollArea
	grid    *qt.QGridLayout
	catalog *qt.QGridLayout
	css     *qt.QPlainTextEdit
	live    *qt.QCheckBox
	u       *UI
}

func (u *UI) buildThemesPage() *themesPage {
	tp := &themesPage{u: u}
	sa, b := scrollPage(tr("Themes"), tr("Everything you see comes from the theme’s CSS: colors, fonts, animations and lyrics. Fine-tune it in Settings → Appearance."))
	tp.w = sa

	actions := qt.NewQHBoxLayout2()
	actions.SetSpacing(8)
	for _, a := range []struct {
		text string
		fn   func()
	}{
		{tr("Install .zip…"), func() {
			f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, tr("Install theme"), homeDir(), tr("Theme (*.zip)"))
			if f != "" {
				tp.install(f, true)
			}
		}},
		{tr("Install folder…"), func() {
			d := qt.QFileDialog_GetExistingDirectory2(u.win.QWidget, tr("Folder with theme.json"))
			if d != "" {
				tp.install(d, true)
			}
		}},
		{tr("Install from URL…"), func() {
			url := qt.QInputDialog_GetText(u.win.QWidget, tr("Install from URL"), tr("URL of a .zip with theme.json:"))
			if strings.TrimSpace(url) != "" {
				tp.install(strings.TrimSpace(url), true)
			}
		}},
		{tr("Create my theme"), tp.createTheme},
		{tr("Open folder"), func() { u.openFolder(u.core.Paths.Themes) }},
	} {
		btn := qt.NewQPushButton3(a.text)
		btn.OnClicked(a.fn)
		actions.AddWidget(btn.QWidget)
	}
	actions.AddStretch()
	b.AddLayout(actions.QLayout)

	b.AddWidget(label(tr("Installed"), "", "heading").QWidget)
	gw := qt.NewQWidget2()
	tp.grid = qt.NewQGridLayout(gw)
	tp.grid.SetSpacing(14)
	tp.grid.SetContentsMargins(0, 0, 0, 0)
	b.AddWidget(gw)

	b.AddWidget(label(tr("Available"), "", "heading").QWidget)
	cw := qt.NewQWidget2()
	tp.catalog = qt.NewQGridLayout(cw)
	tp.catalog.SetSpacing(14)
	tp.catalog.SetContentsMargins(0, 0, 0, 0)
	b.AddWidget(cw)

	b.AddWidget(label(tr("User CSS"), "", "heading").QWidget)
	h := label(tr("Applied over any theme. E.g. :root { --red: #ff7aa2; }  or  #FullLyrics { -meo-align: center; }"), "", "hint")
	h.SetWordWrap(true)
	b.AddWidget(h.QWidget)
	tp.css = qt.NewQPlainTextEdit2()
	tp.css.SetObjectName("UserCSS")
	tp.css.SetMinimumHeight(160)
	tp.css.SetPlaceholderText("/* " + tr("your CSS here") + " */")
	tp.css.SetPlainText(u.cfg.UserCSS)
	b.AddWidget(tp.css.QWidget)
	row := qt.NewQHBoxLayout2()
	apply := qt.NewQPushButton3(tr("Apply CSS"))
	role(apply.QWidget, "primary")
	apply.OnClicked(func() {
		u.cfg.UserCSS = tp.css.ToPlainText()
		_ = u.cfg.Save()
		u.applyTheme(true)
		u.toast(tr("CSS applied"))
	})
	row.AddWidget(apply.QWidget)
	tp.live = qt.NewQCheckBox3(tr("Reload the theme when its files change"))
	tp.live.SetChecked(u.cfg.LiveReload)
	tp.live.OnToggled(func(on bool) { u.cfg.LiveReload = on; _ = u.cfg.Save() })
	row.AddSpacing(12)
	row.AddWidget(tp.live.QWidget)
	row.AddStretch()
	b.AddLayout(row.QLayout)
	b.AddStretch()
	return tp
}

func (tp *themesPage) install(src string, apply bool) {
	u := tp.u
	var t *theme.Theme
	u.runAsync(func() error {
		var err error
		t, err = u.core.Themes.Install(src)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, tr("Could not install the theme"), err.Error())
			return
		}
		if apply {
			u.setTheme(t.ID)
		} else {
			tp.refresh()
		}
	})
}

// createTheme makes a new theme that extends the current one, ready to edit.
func (tp *themesPage) createTheme() {
	u := tp.u
	name := strings.TrimSpace(qt.QInputDialog_GetText(u.win.QWidget, tr("Create theme"), tr("Name of the new theme:")))
	if name == "" {
		return
	}
	id := strings.Trim(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return '-'
	}, name), "-")
	if id == "" {
		id = "my-theme"
	}
	base := u.currentTheme()
	dir := filepath.Join(u.core.Paths.Themes, id)
	if _, err := os.Stat(dir); err == nil {
		u.toast(tr("A theme with this id already exists: {id}", "id", id))
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	manifest := fmt.Sprintf(`{
  "id": %q,
  "name": %q,
  "version": "1.0.0",
  "author": "",
  "description": "My theme, based on %s.",
  "extends": %q,
  "style": "style.css"
}
`, id, name, base.Name, base.ID)
	css := `/* ` + name + ` — extends the theme "` + base.Name + `".
   Change the variables below or override any rule.
   With "Reload the theme when its files change" on, every save shows up at once.
   Full reference: docs/themes/ */

:root {
  /* --red: #d9737a; */
  /* --pink-light: #f4b4c6; */
  /* --wine-1: #241015; */
}

/* #FullLyrics { -meo-align: center; -meo-font-size: 40px; } */
`
	_ = os.WriteFile(filepath.Join(dir, "theme.json"), []byte(manifest), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "style.css"), []byte(css), 0o644)
	u.cfg.LiveReload = true
	tp.live.SetChecked(true)
	u.setTheme(id)
	u.openFolder(dir)
}

// swatch paints a theme's preview colors.
func (u *UI) swatch(colors []theme.Color) *qt.QWidget {
	w := qt.NewQWidget2()
	w.SetObjectName("Swatch")
	w.SetFixedHeight(64)
	w.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		p := qt.NewQPainter2(w.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		W, H := float64(w.Width()), float64(w.Height())
		r := u.style.For(w, "QWidget").Len("radius", 10)
		path := qt.NewQPainterPath()
		path.AddRoundedRect2(0, 0, W, H, r, r)
		p.SetClipPath(path)
		if len(colors) > 0 {
			// background fills, others as stripes on the right
			fillRounded(p, 0, 0, W, H, 0, colors[0])
			n := float64(len(colors) - 1)
			for i, c := range colors[1:] {
				x := W*0.45 + float64(i)*(W*0.55/n)
				fillRounded(p, x, 0, W*0.55/n+1, H, 0, c)
			}
		}
		path.Delete()
		p.End()
		p.Delete()
	})
	return w
}

func previewColors(m map[string]string) []theme.Color {
	var out []theme.Color
	for _, k := range []string{"background", "surface", "accent", "highlight", "text"} {
		if c, ok := theme.ParseColor(m[k]); ok {
			out = append(out, c)
		}
	}
	return out
}

func (tp *themesPage) refresh() {
	u := tp.u
	clearLayout(tp.grid.QLayout)
	cur := u.currentTheme()
	installed := map[string]string{}
	cols := 3
	for i, t := range u.core.Themes.List() {
		installed[t.ID] = t.Version
		t := t
		active := cur != nil && cur.ID == t.ID
		f, b := card(active)
		b.AddWidget(u.swatch(previewColors(t.Preview)))
		top := qt.NewQHBoxLayout2()
		top.AddWidget2(label(t.Name, "", "card-title").QWidget, 1)
		if t.Extends != "" {
			top.AddWidget(chip(tr("EXTENDS {theme}", "theme", strings.ToUpper(t.Extends))).QWidget)
		}
		b.AddLayout(top.QLayout)
		d := label(firstNonEmpty(t.Description, "—"), "", "card-sub")
		d.SetWordWrap(true)
		b.AddWidget(d.QWidget)
		row := qt.NewQHBoxLayout2()
		row.SetSpacing(6)
		if active {
			in := qt.NewQPushButton3(tr("In use"))
			in.SetEnabled(false)
			row.AddWidget(in.QWidget)
		} else {
			ap := qt.NewQPushButton3(tr("Apply"))
			role(ap.QWidget, "primary")
			ap.OnClicked(func() { u.setTheme(t.ID) })
			row.AddWidget(ap.QWidget)
		}
		folder := qt.NewQPushButton3(tr("Folder"))
		folder.OnClicked(func() { u.openFolder(t.Dir) })
		row.AddWidget(folder.QWidget)
		if t.ID != "fishalike" {
			rm := qt.NewQPushButton3(tr("Remove"))
			role(rm.QWidget, "danger")
			rm.OnClicked(func() {
				if !u.confirm(tr("Remove theme"), tr("Remove “{name}”?", "name", t.Name)) {
					return
				}
				_ = u.core.Themes.Remove(t.ID)
				if u.cfg.Theme == t.ID {
					u.setTheme("fishalike")
				} else {
					singleShot(0, tp.refresh)
				}
			})
			row.AddWidget(rm.QWidget)
		}
		row.AddStretch()
		b.AddLayout(row.QLayout)
		tp.grid.AddWidget2(f.QWidget, i/cols, i%cols)
	}

	clearLayout(tp.catalog.QLayout)
	entries, errs := u.core.Catalog.List("theme", u.cfg.Catalogs, config.UserAgent)
	n := 0
	for _, e := range entries {
		if v, ok := installed[e.ID]; ok && v == e.Version {
			continue
		}
		e := e
		f, b := card(false)
		b.AddWidget(u.swatch(previewColors(e.Preview)))
		b.AddWidget(label(e.Name, "", "card-title").QWidget)
		d := label(firstNonEmpty(e.Description, "—")+"  ·  "+firstNonEmpty(e.Author, "?"), "", "card-sub")
		d.SetWordWrap(true)
		b.AddWidget(d.QWidget)
		get := qt.NewQPushButton3(tr("Download and apply"))
		role(get.QWidget, "primary")
		get.OnClicked(func() {
			u.toast(tr("Downloading {name}…", "name", e.Name))
			u.runAsync(func() error { return u.core.InstallEntry(e) }, func(err error) {
				if err != nil {
					qt.QMessageBox_Warning(u.win.QWidget, tr("Error"), err.Error())
					return
				}
				u.setTheme(e.ID)
			})
		})
		b.AddWidget3(get.QWidget, 0, qt.AlignLeft)
		tp.catalog.AddWidget2(f.QWidget, n/cols, n%cols)
		n++
	}
	if n == 0 {
		tp.catalog.AddWidget2(label(tr("Every theme in the catalog is installed."), "", "hint").QWidget, 0, 0)
	}
	for i, err := range errs {
		tp.catalog.AddWidget2(label(err.Error(), "", "card-error").QWidget, 100+i, 0)
	}
	tp.css.SetPlainText(u.cfg.UserCSS)
}
