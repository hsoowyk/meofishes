package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/catalog"
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

var typeNames = map[string]string{"source": "MÚSICA", "lyrics": "LETRAS", "layout": "LAYOUT", "hook": "EVENTOS"}
var runtimeNames = map[string]string{"declarative": "declarativo", "script": "script JS", "process": "processo", "builtin": "embutido"}

func (u *UI) openFolder(dir string) {
	url := qt.QUrl_FromLocalFile(dir)
	qt.QDesktopServices_OpenUrl(url)
	url.Delete()
}

func (u *UI) confirm(title, text string) bool {
	return qt.QMessageBox_Question(u.win.QWidget, title, text) == qt.QMessageBox__Yes
}

// ---------------------------------------------------------------- plugins

type pluginsPage struct {
	w       *qt.QScrollArea
	list    *qt.QVBoxLayout
	catalog *qt.QVBoxLayout
	layouts *qt.QComboBox
	u       *UI
	busy    bool
}

func (u *UI) buildPluginsPage() *pluginsPage {
	pp := &pluginsPage{u: u}
	sa, b := scrollPage("Plugins", "Fontes de música, letras, layouts e eventos. Tudo que o meofishes toca vem de um plugin.")
	pp.w = sa

	actions := qt.NewQHBoxLayout2()
	actions.SetSpacing(8)
	for _, a := range []struct {
		text string
		fn   func()
	}{
		{"Instalar .zip…", func() {
			f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, "Instalar plugin", homeDir(), "Plugin (*.zip)")
			if f != "" {
				pp.install(f, "")
			}
		}},
		{"Instalar pasta…", func() {
			d := qt.QFileDialog_GetExistingDirectory2(u.win.QWidget, "Pasta com plugin.json")
			if d != "" {
				pp.install(d, "")
			}
		}},
		{"Instalar de URL…", func() {
			url := qt.QInputDialog_GetText(u.win.QWidget, "Instalar de URL", "URL de um .zip com plugin.json:")
			if strings.TrimSpace(url) != "" {
				pp.install(strings.TrimSpace(url), "")
			}
		}},
		{"Recarregar", func() { u.app.plugins.Load(); pp.refresh(); u.toast("Plugins recarregados") }},
		{"Abrir pasta", func() { u.openFolder(u.app.paths.Plugins) }},
	} {
		btn := qt.NewQPushButton3(a.text)
		btn.OnClicked(a.fn)
		actions.AddWidget(btn.QWidget)
	}
	actions.AddStretch()
	b.AddLayout(actions.QLayout)

	b.AddWidget(label("Layout da interface", "", "heading").QWidget)
	pp.layouts = qt.NewQComboBox2()
	pp.layouts.SetObjectName("LayoutPicker")
	pp.layouts.SetMaximumWidth(360)
	b.AddWidget(pp.layouts.QWidget)
	pp.layouts.OnCurrentIndexChanged(func(int) {
		if pp.busy || pp.layouts.Count() == 0 {
			return
		}
		id := comboValue(pp.layouts)
		if id == u.app.cfg.Plugins.Layout {
			return
		}
		u.app.cfg.Plugins.Layout = id
		_ = u.app.cfg.Save()
		u.applyLayout()
		u.enterAll()
		u.toast("Layout aplicado")
	})

	b.AddWidget(label("Instalados", "", "heading").QWidget)
	lw := qt.NewQWidget2()
	pp.list = vbox(lw, nil, 10)
	b.AddWidget(lw)
	b.AddWidget(label("Catálogo", "", "heading").QWidget)
	cw := qt.NewQWidget2()
	pp.catalog = vbox(cw, nil, 10)
	b.AddWidget(cw)
	b.AddStretch()
	return pp
}

func homeDir() string { h, _ := os.UserHomeDir(); return h }

// install installs from a path/URL, asking before running build commands.
func (pp *pluginsPage) install(src, name string) {
	u := pp.u
	u.toast("Instalando " + firstNonEmpty(name, filepath.Base(src)) + "…")
	var m *plugin.Manifest
	u.runAsync(func() error {
		var err error
		m, err = u.app.plugins.InstallFrom(src)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, "Não foi possível instalar", err.Error())
			return
		}
		if m.Runtime == plugin.RuntimeProcess {
			msg := fmt.Sprintf("“%s” é um plugin de processo: ele roda como um programa no seu computador.", m.Name)
			if m.Build != "" {
				msg += "\n\nPara funcionar, ele precisa ser compilado com o comando:\n\n    " + m.Build + "\n\nSó continue se você confia na origem deste plugin. Compilar agora?"
				if u.confirm("Compilar plugin?", msg) {
					pp.build(m)
					return
				}
				u.app.plugins.SetEnabled(m.ID, false)
			}
		}
		u.toast("Plugin instalado: " + m.Name)
		pp.refresh()
	})
}

func (pp *pluginsPage) build(m *plugin.Manifest) {
	u := pp.u
	u.toast("Compilando " + m.Name + "…")
	var out string
	u.runAsync(func() error {
		var err error
		out, err = u.app.plugins.Build(m.ID)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, "Falha ao compilar", err.Error()+"\n\n"+ellipsis(out, 1500))
		} else {
			u.toast(m.Name + " compilado e ativo")
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
	a := u.app
	// layout picker
	pp.busy = true
	pp.layouts.Clear()
	v := qt.NewQVariant14("")
	pp.layouts.AddItem3("Padrão do meofishes", v)
	v.Delete()
	for _, p := range a.plugins.Layouts() {
		v := qt.NewQVariant14(p.Manifest.ID)
		pp.layouts.AddItem3(p.Manifest.Name, v)
		v.Delete()
		if p.Manifest.ID == a.cfg.Plugins.Layout {
			pp.layouts.SetCurrentIndex(pp.layouts.Count() - 1)
		}
	}
	pp.busy = false

	clearLayout(pp.list.QLayout)
	installed := map[string]string{}
	for _, p := range a.plugins.All() {
		installed[p.Manifest.ID] = p.Manifest.Version
		pp.list.AddWidget(pp.pluginCard(p))
	}
	clearLayout(pp.catalog.QLayout)
	entries, errs := a.catalog.List("plugin", a.cfg.Catalogs, a.host.UserAgent)
	shown := 0
	for _, e := range entries {
		if ver, ok := installed[e.ID]; ok && ver == e.Version {
			continue
		}
		pp.catalog.AddWidget(pp.catalogCard(e, installed[e.ID] != ""))
		shown++
	}
	if shown == 0 {
		pp.catalog.AddWidget(label("Tudo do catálogo já está instalado.", "", "hint").QWidget)
	}
	for _, err := range errs {
		pp.catalog.AddWidget(label(err.Error(), "", "card-error").QWidget)
	}
}

func (pp *pluginsPage) pluginCard(p *plugin.Plugin) *qt.QWidget {
	u := pp.u
	m := p.Manifest
	enabled := u.app.plugins.Enabled(m.ID)
	f, b := card(enabled && p.Err == nil)
	top := qt.NewQHBoxLayout2()
	top.SetSpacing(8)
	top.AddWidget(label(m.Name, "", "card-title").QWidget)
	for _, t := range m.Types {
		top.AddWidget(chip(typeNames[t]).QWidget)
	}
	rt := m.Runtime
	if rt == "" {
		rt = "declarative"
	}
	top.AddWidget(chip(runtimeNames[rt]).QWidget)
	top.AddStretch()
	tog := qt.NewQCheckBox3("Ativo")
	tog.SetChecked(enabled)
	tog.OnToggled(func(on bool) {
		err := u.app.plugins.SetEnabled(m.ID, on)
		if err != nil {
			u.toast(m.Name + ": " + err.Error())
		}
		if m.Has(plugin.TypeLayout) && u.app.cfg.Plugins.Layout == m.ID {
			u.applyLayout()
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
		sub += " · módulos Go: " + strings.Join(m.Modules, ", ")
	}
	b.AddWidget(label(sub, "", "card-sub").QWidget)
	if m.Description != "" {
		d := label(m.Description, "", "card-sub")
		d.SetWordWrap(true)
		b.AddWidget(d.QWidget)
	}
	if p.Err != nil {
		e := label("⚠ "+p.Err.Error(), "", "card-error")
		e.SetWordWrap(true)
		b.AddWidget(e.QWidget)
	}
	btns := qt.NewQHBoxLayout2()
	btns.SetSpacing(8)
	if len(m.Settings) > 0 {
		cfgBtn := qt.NewQPushButton3("Configurar")
		cfgBtn.OnClicked(func() { u.pluginSettings(p, pp.refresh) })
		btns.AddWidget(cfgBtn.QWidget)
	}
	if m.Build != "" && !p.Builtin {
		bb := qt.NewQPushButton3("Compilar")
		bb.OnClicked(func() {
			if u.confirm("Compilar plugin?", "Executar:\n\n    "+m.Build+"\n\nna pasta do plugin?") {
				pp.build(&m)
			}
		})
		btns.AddWidget(bb.QWidget)
	}
	if !p.Builtin {
		folder := qt.NewQPushButton3("Pasta")
		folder.OnClicked(func() { u.openFolder(p.Dir) })
		btns.AddWidget(folder.QWidget)
		rm := qt.NewQPushButton3("Remover")
		role(rm.QWidget, "danger")
		rm.OnClicked(func() {
			if !u.confirm("Remover plugin", "Remover “"+m.Name+"”?") {
				return
			}
			if err := u.app.plugins.Uninstall(m.ID); err != nil {
				u.toast(err.Error())
			}
			if u.app.cfg.Plugins.Layout == m.ID {
				u.app.cfg.Plugins.Layout = ""
				u.applyLayout()
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
		top.AddWidget(chip(typeNames[t]).QWidget)
	}
	if e.Runtime != "" {
		top.AddWidget(chip(runtimeNames[e.Runtime]).QWidget)
	}
	top.AddStretch()
	btnText := "Instalar"
	if update {
		btnText = "Atualizar"
	}
	inst := qt.NewQPushButton3(btnText)
	role(inst.QWidget, "primary")
	inst.OnClicked(func() {
		src, cleanup, err := u.app.catalog.Materialize(e)
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
	dlg.SetWindowTitle("Configurar " + m.Name)
	dlg.SetMinimumWidth(520)
	b := vbox(dlg.QWidget, []int{22, 20, 22, 18}, 8)
	b.AddWidget(label(m.Name, "", "card-title").QWidget)
	cur := u.app.plugins.Settings(m.ID)
	getters := map[string]func() string{}
	for _, s := range m.Settings {
		s := s
		b.AddWidget(label(firstNonEmpty(s.Label, s.Key), "", "card-sub").QWidget)
		switch s.Type {
		case "bool":
			cb := qt.NewQCheckBox3(firstNonEmpty(s.Description, "Ativado"))
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
	cancel := qt.NewQPushButton3("Cancelar")
	cancel.OnClicked(dlg.Reject)
	save := qt.NewQPushButton3("Salvar")
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
		if err := u.app.plugins.SetSettings(m.ID, vals); err != nil {
			u.toast(m.Name + ": " + err.Error())
		} else {
			u.toast("Configurações salvas")
		}
		if after != nil {
			after()
		}
		if m.Has(plugin.TypeSource) {
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
	sa, b := scrollPage("Temas", "Tudo na interface vem do CSS do tema: cores, fontes, animações, letras e layout dos widgets.")
	tp.w = sa

	actions := qt.NewQHBoxLayout2()
	actions.SetSpacing(8)
	for _, a := range []struct {
		text string
		fn   func()
	}{
		{"Instalar .zip…", func() {
			f := qt.QFileDialog_GetOpenFileName4(u.win.QWidget, "Instalar tema", homeDir(), "Tema (*.zip)")
			if f != "" {
				tp.install(f, true)
			}
		}},
		{"Instalar pasta…", func() {
			d := qt.QFileDialog_GetExistingDirectory2(u.win.QWidget, "Pasta com theme.json")
			if d != "" {
				tp.install(d, true)
			}
		}},
		{"Instalar de URL…", func() {
			url := qt.QInputDialog_GetText(u.win.QWidget, "Instalar de URL", "URL de um .zip com theme.json:")
			if strings.TrimSpace(url) != "" {
				tp.install(strings.TrimSpace(url), true)
			}
		}},
		{"Criar meu tema", tp.createTheme},
		{"Abrir pasta", func() { u.openFolder(u.app.paths.Themes) }},
	} {
		btn := qt.NewQPushButton3(a.text)
		btn.OnClicked(a.fn)
		actions.AddWidget(btn.QWidget)
	}
	actions.AddStretch()
	b.AddLayout(actions.QLayout)

	b.AddWidget(label("Instalados", "", "heading").QWidget)
	gw := qt.NewQWidget2()
	tp.grid = qt.NewQGridLayout(gw)
	tp.grid.SetSpacing(14)
	tp.grid.SetContentsMargins(0, 0, 0, 0)
	b.AddWidget(gw)

	b.AddWidget(label("Para baixar", "", "heading").QWidget)
	cw := qt.NewQWidget2()
	tp.catalog = qt.NewQGridLayout(cw)
	tp.catalog.SetSpacing(14)
	tp.catalog.SetContentsMargins(0, 0, 0, 0)
	b.AddWidget(cw)

	b.AddWidget(label("CSS do usuário", "", "heading").QWidget)
	h := label("Aplicado por cima de qualquer tema. Ex.: :root { --accent: #ff7aa2; }  ou  #LyricsView { -meo-align: center; }", "", "hint")
	h.SetWordWrap(true)
	b.AddWidget(h.QWidget)
	tp.css = qt.NewQPlainTextEdit2()
	tp.css.SetObjectName("UserCSS")
	tp.css.SetMinimumHeight(160)
	tp.css.SetPlaceholderText("/* seu CSS aqui */")
	tp.css.SetPlainText(u.app.cfg.UserCSS)
	b.AddWidget(tp.css.QWidget)
	row := qt.NewQHBoxLayout2()
	apply := qt.NewQPushButton3("Aplicar CSS")
	role(apply.QWidget, "primary")
	apply.OnClicked(func() {
		u.app.cfg.UserCSS = tp.css.ToPlainText()
		_ = u.app.cfg.Save()
		u.applyTheme(true)
		u.toast("CSS aplicado")
	})
	row.AddWidget(apply.QWidget)
	tp.live = qt.NewQCheckBox3("Recarregar o tema automaticamente quando os arquivos mudarem")
	tp.live.SetChecked(u.app.cfg.LiveReload)
	tp.live.OnToggled(func(on bool) { u.app.cfg.LiveReload = on; _ = u.app.cfg.Save() })
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
		t, err = u.app.themes.Install(src)
		return err
	}, func(err error) {
		if err != nil {
			qt.QMessageBox_Warning(u.win.QWidget, "Não foi possível instalar o tema", err.Error())
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
	name := strings.TrimSpace(qt.QInputDialog_GetText(u.win.QWidget, "Criar tema", "Nome do novo tema:"))
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
		id = "meu-tema"
	}
	base := u.currentTheme()
	dir := filepath.Join(u.app.paths.Themes, id)
	if _, err := os.Stat(dir); err == nil {
		u.toast("Já existe um tema com esse id: " + id)
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	manifest := fmt.Sprintf(`{
  "id": %q,
  "name": %q,
  "version": "1.0.0",
  "author": "",
  "description": "Meu tema, baseado em %s.",
  "extends": %q,
  "style": "style.css"
}
`, id, name, base.Name, base.ID)
	css := `/* ` + name + ` — estende o tema "` + base.Name + `".
   Mude as variáveis abaixo ou sobrescreva qualquer regra.
   Ative "Recarregar automaticamente" na página Temas para ver as mudanças ao salvar.
   Referência completa: docs/THEMES.md */

:root {
  /* --red: #d9737a; */
  /* --pink-light: #f4b4c6; */
  /* --wine-1: #241015; */
}

/* #LyricsView { -meo-align: center; -meo-font-size: 40px; } */
`
	_ = os.WriteFile(filepath.Join(dir, "theme.json"), []byte(manifest), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "style.css"), []byte(css), 0o644)
	u.app.cfg.LiveReload = true
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
	a := u.app
	clearLayout(tp.grid.QLayout)
	cur := u.currentTheme()
	installed := map[string]string{}
	cols := 3
	for i, t := range a.themes.List() {
		installed[t.ID] = t.Version
		t := t
		active := cur != nil && cur.ID == t.ID
		f, b := card(active)
		b.AddWidget(u.swatch(previewColors(t.Preview)))
		top := qt.NewQHBoxLayout2()
		top.AddWidget2(label(t.Name, "", "card-title").QWidget, 1)
		if t.Extends != "" {
			top.AddWidget(chip("ESTENDE " + strings.ToUpper(t.Extends)).QWidget)
		}
		b.AddLayout(top.QLayout)
		d := label(firstNonEmpty(t.Description, "—"), "", "card-sub")
		d.SetWordWrap(true)
		b.AddWidget(d.QWidget)
		row := qt.NewQHBoxLayout2()
		row.SetSpacing(6)
		if active {
			in := qt.NewQPushButton3("Em uso")
			in.SetEnabled(false)
			row.AddWidget(in.QWidget)
		} else {
			ap := qt.NewQPushButton3("Aplicar")
			role(ap.QWidget, "primary")
			ap.OnClicked(func() { u.setTheme(t.ID) })
			row.AddWidget(ap.QWidget)
		}
		folder := qt.NewQPushButton3("Pasta")
		folder.OnClicked(func() { u.openFolder(t.Dir) })
		row.AddWidget(folder.QWidget)
		if t.ID != "fishalike" {
			rm := qt.NewQPushButton3("Remover")
			role(rm.QWidget, "danger")
			rm.OnClicked(func() {
				if !u.confirm("Remover tema", "Remover “"+t.Name+"”?") {
					return
				}
				_ = a.themes.Remove(t.ID)
				if a.cfg.Theme == t.ID {
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
	entries, errs := a.catalog.List("theme", a.cfg.Catalogs, a.host.UserAgent)
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
		get := qt.NewQPushButton3("Baixar e aplicar")
		role(get.QWidget, "primary")
		get.OnClicked(func() {
			u.toast("Baixando " + e.Name + "…")
			u.runAsync(func() error { return a.installEntry(e) }, func(err error) {
				if err != nil {
					qt.QMessageBox_Warning(u.win.QWidget, "Erro", err.Error())
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
		tp.catalog.AddWidget2(label("Todos os temas do catálogo já estão instalados.", "", "hint").QWidget, 0, 0)
	}
	for i, err := range errs {
		tp.catalog.AddWidget2(label(err.Error(), "", "card-error").QWidget, 100+i, 0)
	}
	tp.css.SetPlainText(a.cfg.UserCSS)
}
