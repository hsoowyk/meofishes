package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	qt "github.com/mappu/miqt/qt"

	"meofishes/internal/config"
	"meofishes/internal/i18n"
	"meofishes/internal/library"
	"meofishes/internal/offline"
	"meofishes/internal/player"
	"meofishes/internal/plugin"
	"meofishes/internal/theme"
)

// settingsPage is the dedicated settings screen: categories on the left of
// the content area, the options of the chosen category on the right.
// Plugins and Themes live here too.
type settingsPage struct {
	u     *UI
	w     *qt.QWidget
	nav   *qt.QVBoxLayout
	stack *qt.QStackedWidget
	cats  []settingsCat
	btns  map[string]*qt.QPushButton
	pages map[string]*qt.QScrollArea
	cur   string
}

type settingsCat struct {
	id, icon, title string
	build           func(s *sb)
	fixed           *qt.QWidget // plugins / themes pages
}

func (u *UI) buildSettingsPage() *settingsPage {
	sp := &settingsPage{u: u, btns: map[string]*qt.QPushButton{}, pages: map[string]*qt.QScrollArea{}}
	sp.w = qt.NewQWidget2()
	h := hbox(sp.w, []int{0, 0, 0, 0}, 0)

	navW := qt.NewQWidget2()
	navW.SetObjectName("SettingsNav")
	navW.SetAttribute(qt.WA_StyledBackground)
	navW.SetFixedWidth(220)
	sp.nav = vbox(navW, []int{14, 24, 14, 14}, 2)
	sp.nav.AddWidget(label(tr("Settings"), "", "title").QWidget)
	sp.nav.AddSpacing(8)
	h.AddWidget(navW)

	sp.stack = qt.NewQStackedWidget2()
	sp.stack.SetObjectName("SettingsStack")
	h.AddWidget2(sp.stack.QWidget, 1)

	sp.cats = []settingsCat{
		{id: "appearance", icon: "theme", title: tr("Appearance"), build: sp.appearance},
		{id: "themes", icon: "theme", title: tr("Themes"), fixed: u.themePg.w.QWidget},
		{id: "animations", icon: "refresh", title: tr("Animations"), build: sp.animations},
		{id: "lyrics", icon: "lyrics", title: tr("Lyrics"), build: sp.lyrics},
		{id: "playback", icon: "play", title: tr("Playback & audio"), build: sp.playback},
		{id: "streaming", icon: "import", title: tr("Streaming"), build: sp.streaming},
		{id: "library", icon: "library", title: tr("Library & privacy"), build: sp.library},
		{id: "plugins", icon: "plugin", title: tr("Plugins"), fixed: u.plugPg.w.QWidget},
		{id: "priority", icon: "queue", title: tr("Plugin priority"), build: sp.priority},
		{id: "language", icon: "info", title: tr("Language"), build: sp.language},
		{id: "shortcuts", icon: "more", title: tr("Shortcuts"), build: sp.shortcuts},
		{id: "storage", icon: "folder", title: tr("Storage"), build: sp.storage},
		{id: "developer", icon: "code", title: tr("Developer"), build: sp.developer},
		{id: "about", icon: "fish", title: tr("About"), build: sp.about},
	}
	for _, c := range sp.cats {
		c := c
		b := qt.NewQPushButton3(c.title)
		role(b.QWidget, "settings-nav")
		setProp(b.QWidget, "active", "false")
		handCursor(b.QWidget)
		b.OnClicked(func() { sp.open(c.id) })
		sp.btns[c.id] = b
		sp.nav.AddWidget(b.QWidget)
		if c.fixed != nil {
			sp.stack.AddWidget(c.fixed)
			continue
		}
		sa := qt.NewQScrollArea2()
		sa.SetWidgetResizable(true)
		sa.SetFrameShape(qt.QFrame__NoFrame)
		sa.SetHorizontalScrollBarPolicy(qt.ScrollBarAlwaysOff)
		sp.pages[c.id] = sa
		sp.stack.AddWidget(sa.QWidget)
	}
	sp.nav.AddStretch()
	return sp
}

func (sp *settingsPage) refresh() {
	if sp.cur == "" {
		sp.open("appearance")
		return
	}
	sp.open(sp.cur)
}

// open shows a category (its page is rebuilt so it shows current values).
func (sp *settingsPage) open(id string) {
	u := sp.u
	var cat *settingsCat
	for i := range sp.cats {
		if sp.cats[i].id == id {
			cat = &sp.cats[i]
		}
	}
	if cat == nil {
		return
	}
	sp.cur = id
	for k, b := range sp.btns {
		setProp(b.QWidget, "active", boolStr(k == id))
	}
	if cat.fixed != nil {
		sp.stack.SetCurrentWidget(cat.fixed)
		switch id {
		case "plugins":
			u.plugPg.refresh()
		case "themes":
			u.themePg.refresh()
		}
		return
	}
	sa := sp.pages[id]
	if old := sa.TakeWidget(); old != nil {
		old.Hide()
		old.DeleteLater()
	}
	inner := qt.NewQWidget2()
	inner.SetObjectName("SettingsContent")
	b := vbox(inner, []int{30, 24, 30, 30}, 6)
	b.AddWidget(label(cat.title, "", "title").QWidget)
	cat.build(&sb{u: u, b: b, refresh: func() { singleShot(0, func() { sp.open(id) }) }})
	b.AddStretch()
	sa.SetWidget(inner)
	sp.stack.SetCurrentWidget(sa.QWidget)
}

// ---------------------------------------------------------------- builder

// sb builds settings rows.
type sb struct {
	u       *UI
	b       *qt.QVBoxLayout
	refresh func()
}

func (s *sb) section(title string) {
	s.b.AddSpacing(10)
	s.b.AddWidget(label(title, "", "heading").QWidget)
}

func (s *sb) note(text string) {
	l := label(text, "", "hint")
	l.SetWordWrap(true)
	s.b.AddWidget(l.QWidget)
}

// row adds "title / description … control".
func (s *sb) row(title, desc string, ctl ...*qt.QWidget) {
	f := qt.NewQFrame2()
	f.SetObjectName("SettingRow")
	f.SetAttribute(qt.WA_StyledBackground)
	h := hbox(f.QWidget, []int{14, 10, 14, 10}, 12)
	t := qt.NewQVBoxLayout2()
	t.SetSpacing(2)
	t.AddWidget(label(title, "", "setting-title").QWidget)
	if desc != "" {
		d := label(desc, "", "setting-desc")
		d.SetWordWrap(true)
		t.AddWidget(d.QWidget)
	}
	h.AddLayout2(t.QLayout, 1)
	for _, c := range ctl {
		h.AddWidget3(c, 0, qt.AlignVCenter)
	}
	s.b.AddWidget(f.QWidget)
}

func (s *sb) toggle(title, desc string, val bool, set func(bool)) {
	sw := s.u.toggle(val, func(on bool) { set(on); _ = s.u.cfg.Save() })
	s.row(title, desc, sw.QWidget)
}

func (s *sb) choice(title, desc string, opts [][2]string, cur string, set func(string)) {
	cb := qt.NewQComboBox2()
	cb.SetMinimumWidth(190)
	for i, o := range opts {
		v := qt.NewQVariant14(o[0])
		cb.AddItem3(o[1], v)
		v.Delete()
		if o[0] == cur {
			cb.SetCurrentIndex(i)
		}
	}
	cb.OnActivated(func(int) { set(comboValue(cb)); _ = s.u.cfg.Save() })
	s.row(title, desc, cb.QWidget)
}

func (s *sb) number(title, desc string, min, max, val int, suffix string, set func(int)) {
	sp := qt.NewQSpinBox2()
	sp.SetRange(min, max)
	sp.SetValue(val)
	if suffix != "" {
		sp.SetSuffix(" " + suffix)
	}
	sp.SetMinimumWidth(110)
	sp.OnValueChanged(func(v int) { set(v); _ = s.u.cfg.Save() })
	s.row(title, desc, sp.QWidget)
}

func (s *sb) text(title, desc, val, placeholder string, set func(string)) {
	le := qt.NewQLineEdit2()
	le.SetText(val)
	le.SetPlaceholderText(placeholder)
	le.SetMinimumWidth(240)
	le.OnEditingFinished(func() { set(strings.TrimSpace(le.Text())); _ = s.u.cfg.Save() })
	s.row(title, desc, le.QWidget)
}

func (s *sb) button(title, desc, text string, f func()) {
	b := qt.NewQPushButton3(text)
	b.OnClicked(f)
	s.row(title, desc, b.QWidget)
}

// ---------------------------------------------------------------- style rows

// styleValue is the current value of "selector|property": the Settings
// override, or what the theme sets.
func (u *UI) styleValue(key string) (string, bool) {
	if v, ok := u.cfg.Style[key]; ok {
		return v, true
	}
	sel, prop, _ := strings.Cut(key, "|")
	if strings.HasPrefix(prop, "--") {
		v := u.style.c.Var(prop)
		return v, v != ""
	}
	return u.style.c.Prop([]string{sel}, prop)
}

func (s *sb) resetBtn(key string) *qt.QWidget {
	r := s.u.iconButton("SettingReset", "refresh", tr("Back to the theme’s value"))
	_, set := s.u.cfg.Style[key]
	r.SetVisible(set)
	r.OnClicked(func() { s.u.setStyle(key, ""); s.refresh() })
	return r.QWidget
}

func (s *sb) styleChoice(title, desc, key string, opts [][2]string, def string) {
	cur, ok := s.u.styleValue(key)
	if !ok {
		cur = def
	}
	cur = strings.Trim(cur, `"' `)
	cb := qt.NewQComboBox2()
	cb.SetMinimumWidth(190)
	for i, o := range opts {
		v := qt.NewQVariant14(o[0])
		cb.AddItem3(o[1], v)
		v.Delete()
		if o[0] == cur {
			cb.SetCurrentIndex(i)
		}
	}
	reset := s.resetBtn(key)
	cb.OnActivated(func(int) { s.u.setStyle(key, comboValue(cb)); reset.SetVisible(true) })
	s.row(title, desc, cb.QWidget, reset)
}

// styleNumber edits a number with a unit ("px", "ms" or "" for plain).
func (s *sb) styleNumber(title, desc, key, unit string, min, max int, def float64, scale float64) {
	cur, ok := s.u.styleValue(key)
	val := def
	if ok {
		val = theme.Number(strings.TrimSuffix(strings.TrimSuffix(cur, "px"), "ms"), def)
		if unit == "ms" {
			val = float64(theme.Duration(cur, int(def)))
		}
	}
	sp := qt.NewQSpinBox2()
	sp.SetRange(min, max)
	sp.SetValue(int(val*scale + 0.5))
	if unit != "" {
		sp.SetSuffix(" " + unit)
	} else if scale == 100 {
		sp.SetSuffix(" %")
	}
	sp.SetMinimumWidth(110)
	reset := s.resetBtn(key)
	sp.OnValueChanged(func(v int) {
		out := strconv.FormatFloat(float64(v)/scale, 'f', -1, 64)
		s.u.setStyle(key, out+unit)
		reset.SetVisible(true)
	})
	s.row(title, desc, sp.QWidget, reset)
}

func (s *sb) styleToggle(title, desc, key string, def bool) {
	cur, ok := s.u.styleValue(key)
	val := def
	if ok {
		val = theme.Bool(cur, def)
	}
	reset := s.resetBtn(key)
	sw := s.u.toggle(val, func(on bool) { s.u.setStyle(key, boolStr(on)); reset.SetVisible(true) })
	s.row(title, desc, sw.QWidget, reset)
}

func (s *sb) styleColor(title, desc, key string) {
	u := s.u
	cur, _ := u.styleValue(key)
	c, _ := theme.ParseColor(cur)
	btn := qt.NewQPushButton2()
	btn.SetObjectName("ColorSwatch")
	btn.SetFixedSize2(58, 30)
	handCursor(btn.QWidget)
	btn.SetToolTip(cur)
	btn.OnPaintEvent(func(super func(*qt.QPaintEvent), e *qt.QPaintEvent) {
		p := qt.NewQPainter2(btn.QPaintDevice)
		p.SetRenderHint(qt.QPainter__Antialiasing)
		w, h := float64(btn.Width()), float64(btn.Height())
		fillRounded(p, 0, 0, w, h, 8, theme.Color{R: 255, G: 255, B: 255, A: .12})
		fillRounded(p, 3, 3, w-6, h-6, 6, c)
		p.End()
		p.Delete()
	})
	reset := s.resetBtn(key)
	btn.OnClicked(func() {
		init := qc(c)
		got := qt.QColorDialog_GetColor4(init, u.win.QWidget, title, qt.QColorDialog__ShowAlphaChannel)
		init.Delete()
		if got == nil || !got.IsValid() {
			return
		}
		v := got.Name()
		if got.Alpha() < 255 {
			v = fmt.Sprintf("rgba(%d, %d, %d, %.2f)", got.Red(), got.Green(), got.Blue(), float64(got.Alpha())/255)
		}
		c, _ = theme.ParseColor(v)
		btn.SetToolTip(v)
		btn.Update()
		u.setStyle(key, v)
		reset.SetVisible(true)
	})
	s.row(title, desc, btn.QWidget, reset)
}

func (s *sb) styleFont(title, key string) {
	u := s.u
	cur, _ := u.styleValue(key)
	fc := qt.NewQFontComboBox2()
	fc.SetMinimumWidth(220)
	if fam := firstFamily(cur); fam != "" {
		f := qt.NewQFont2(fam)
		fc.SetCurrentFont(f)
		f.Delete()
	}
	reset := s.resetBtn(key)
	fc.OnActivated(func(int) {
		u.setStyle(key, `"`+fc.CurrentText()+`", sans-serif`)
		reset.SetVisible(true)
	})
	s.row(title, "", fc.QWidget, reset)
}

func firstFamily(css string) string {
	for _, x := range strings.Split(css, ",") {
		x = strings.Trim(strings.TrimSpace(x), `"'`)
		if x != "" && x != "sans-serif" && x != "serif" && x != "monospace" {
			return x
		}
	}
	return ""
}

var easingOpts = [][2]string{{"out-cubic", "out-cubic"}, {"out-quint", "out-quint"}, {"out-expo", "out-expo"},
	{"out-back", "out-back"}, {"out-elastic", "out-elastic"}, {"out-bounce", "out-bounce"}, {"in-out-sine", "in-out-sine"},
	{"in-out-cubic", "in-out-cubic"}, {"linear", "linear"}}

// ---------------------------------------------------------------- categories

func (sp *settingsPage) appearance(s *sb) {
	u := sp.u
	s.section(tr("Theme"))
	var themes [][2]string
	for _, t := range u.core.Themes.List() {
		themes = append(themes, [2]string{t.ID, t.Name})
	}
	s.choice(tr("Theme"), tr("Install more in Settings → Themes."), themes, u.cfg.Theme, func(id string) { u.setTheme(id) })

	s.section(tr("Colors"))
	s.note(tr("Changes apply on top of the current theme. The ↻ button returns a color to the theme’s value."))
	var vars []string
	for k, v := range u.style.c.Vars {
		if _, ok := theme.ParseColor(v); ok {
			vars = append(vars, k)
		}
	}
	sort.Strings(vars)
	for _, k := range vars {
		s.styleColor(k, "", ":root|"+k)
	}

	s.section(tr("Fonts"))
	s.styleFont(tr("Interface font"), ":root|--font")
	s.styleNumber(tr("Interface text size"), "", ":root|--font-size", "px", 9, 24, 13, 1)
	s.styleFont(tr("Lyrics font"), "#FullLyrics|-meo-font-family")

	s.section(tr("Shape and layout"))
	s.styleNumber(tr("Corner radius"), tr("Cards, panels and buttons."), ":root|--radius", "px", 0, 40, 14, 1)
	s.styleNumber(tr("Left panel width"), "", "#LeftPanel|-meo-width", "px", 220, 460, 290, 1)
	s.choice(tr("Card size"), tr("Covers on the home page and in the library."), [][2]string{{"small", tr("Small")}, {"medium", tr("Medium")}, {"large", tr("Large")}},
		u.cfg.Prefs.GridSize, func(v string) { u.cfg.Prefs.GridSize = v; u.home.lastSeed = "-"; u.home.popFor = "" })
	s.toggle(tr("Covers in song lists"), "", u.cfg.Prefs.ListCovers, func(on bool) { u.cfg.Prefs.ListCovers = on })
	s.choice(tr("Start page"), "", [][2]string{{"home", tr("Home")}, {"search", tr("Search")}, {"library", tr("Library")}},
		u.cfg.Prefs.StartPage, func(v string) { u.cfg.Prefs.StartPage = v })

	s.section(tr("Background"))
	s.styleToggle(tr("Color from the album cover"), tr("The background slowly takes the color of the song playing."), "#Root|-meo-backdrop-dynamic", true)
	s.styleNumber(tr("Strength of the cover color"), "", "#Root|-meo-backdrop-dynamic-strength", "", 0, 100, 0.38, 100)
	s.styleNumber(tr("Breathing light"), tr("Period of the soft pulsing light; 0 turns it off."), "#Root|-meo-backdrop-glow-pulse", "ms", 0, 30000, 0, 1)
	s.styleToggle(tr("Potion background in the fullscreen player"), tr("The cover’s three main colors swirl and mix behind the player."), "#FullPlayer|-meo-potion", true)
	s.styleNumber(tr("Potion speed"), "", "#FullPlayer|-meo-potion-speed", "", 10, 500, 1, 100)
	s.styleNumber(tr("Potion frames per second"), tr("0 = your screen’s refresh rate."), "#FullPlayer|-meo-potion-fps", "", 0, 240, 30, 1)
	s.styleNumber(tr("Blurred cover in the fullscreen player"), tr("Opacity of the cover behind the fullscreen player (when the potion is off)."), "#FullPlayer|-meo-backdrop-image-opacity", "", 0, 100, 0, 100)
	s.text(tr("Your name"), tr("Shown in the greeting on the home page."), u.cfg.Prefs.UserName, "", func(v string) { u.cfg.Prefs.UserName = v })
}

func (sp *settingsPage) animations(s *sb) {
	u := sp.u
	s.section(tr("General"))
	s.number(tr("Animation speed"), tr("100% is the theme’s speed. Animations run at your screen’s refresh rate."), 25, 300, int(u.cfg.Prefs.AnimSpeed*100), "%",
		func(v int) { u.cfg.Prefs.AnimSpeed = float64(v) / 100 })
	s.toggle(tr("Reduce motion"), tr("Turns off transitions, cascades and the breathing light."), u.cfg.Prefs.ReduceMotion, func(on bool) {
		u.cfg.Prefs.ReduceMotion = on
		u.applyTheme(false)
	})
	s.section(tr("Pages"))
	s.styleChoice(tr("Page transition"), "", "#Pages|-meo-transition",
		[][2]string{{"fade", tr("Fade")}, {"slide-up", tr("Slide up")}, {"slide-left", tr("Slide left")}, {"slide-right", tr("Slide right")}, {"zoom", tr("Zoom")}, {"none", tr("None")}}, "slide-up")
	s.styleNumber(tr("Transition duration"), "", "#Pages|-meo-transition-duration", "ms", 0, 2000, 380, 1)
	s.styleChoice(tr("Transition easing"), "", "#Pages|-meo-transition-easing", easingOpts, "out-quint")
	s.styleChoice(tr("Songs appearing in lists"), "", "#TrackRow|-meo-enter",
		[][2]string{{"slide-up", tr("Slide up")}, {"fade", tr("Fade")}, {"slide-left", tr("Slide left")}, {"none", tr("None")}}, "slide-up")
	s.styleNumber(tr("Delay between songs"), "", "#TrackRow|-meo-enter-stagger", "ms", 0, 200, 22, 1)
	s.section(tr("Current lyric line"))
	s.styleChoice(tr("Swipe direction"), "", "#CurrentLyric|-meo-swipe",
		[][2]string{{"up", tr("Up")}, {"down", tr("Down")}, {"left", tr("Left")}, {"right", tr("Right")}, {"fade", tr("Fade")}}, "up")
	s.styleNumber(tr("Swipe duration"), "", "#CurrentLyric|-meo-swipe-duration", "ms", 0, 2000, 420, 1)
	s.styleChoice(tr("Swipe easing"), "", "#CurrentLyric|-meo-swipe-easing", easingOpts, "out-cubic")
	s.section(tr("Fullscreen player"))
	s.styleNumber(tr("Opening duration"), "", "#FullPlayer|-meo-open-duration", "ms", 0, 2000, 420, 1)
	s.styleChoice(tr("Opening easing"), "", "#FullPlayer|-meo-open-easing", easingOpts, "out-cubic")
	s.toggle(tr("Tilt the cover under the mouse"), "", u.cfg.Prefs.CoverTilt, func(on bool) { u.cfg.Prefs.CoverTilt = on })
	s.styleNumber(tr("Maximum tilt (degrees)"), "", "#FullCover|-meo-tilt-max", "", 0, 35, u.cfg.Prefs.TiltMax, 1)
	s.styleNumber(tr("Tilt smoothness"), tr("Lower is smoother and slower."), "#FullCover|-meo-tilt-smoothing", "", 1, 100, 0.12, 100)
	if names := theme.CustomEasings(); len(names) > 0 {
		s.note(tr("Easings from ani plugins: {names}", "names", strings.Join(names, ", ")))
	}
}

func (sp *settingsPage) lyrics(s *sb) {
	u := sp.u
	const sel = "#FullLyrics|"
	s.section(tr("Display"))
	s.styleNumber(tr("Text size"), "", sel+"-meo-font-size", "px", 14, 90, 36, 1)
	s.styleChoice(tr("Weight"), "", sel+"-meo-font-weight", [][2]string{{"400", tr("Regular")}, {"600", tr("Semibold")}, {"700", tr("Bold")}, {"800", tr("Extra bold")}, {"900", tr("Black")}}, "800")
	s.styleChoice(tr("Alignment"), "", sel+"-meo-align", [][2]string{{"left", tr("Left")}, {"center", tr("Center")}, {"right", tr("Right")}}, "left")
	s.styleNumber(tr("Line spacing"), "", sel+"-meo-line-spacing", "px", 0, 80, 22, 1)
	s.styleNumber(tr("Fade of distant lines"), "", sel+"-meo-distance-fade", "", 0, 50, 0.16, 100)
	s.styleColor(tr("Sung text"), "", sel+"-meo-active-color")
	s.styleColor(tr("Other lines"), "", sel+"-meo-color")
	s.toggle(tr("Background vocals"), "", u.cfg.Prefs.BgVocals, func(on bool) { u.cfg.Prefs.BgVocals = on; u.setLyrics(u.lyrics, "") })
	s.toggle(tr("Show credits"), "", u.cfg.Prefs.LyricsCredits, func(on bool) { u.cfg.Prefs.LyricsCredits = on })
	s.styleToggle(tr("Dots during instrumental parts"), "", sel+"-meo-interlude-dots", true)

	s.section(tr("Animation"))
	s.styleChoice(tr("Word-synced lyrics"), tr("How each word lights up while it is sung."), sel+"-meo-word-animation",
		[][2]string{{"fill", tr("Fill (gradient)")}, {"fade", tr("Fade in")}, {"bounce", tr("Bounce")}, {"glow", tr("Glow")}, {"font", tr("Font change")}, {"none", tr("None")}}, "fill")
	s.styleFont(tr("Font of the sung word"), sel+"-meo-sung-font-family")
	s.styleChoice(tr("Weight of the sung word"), tr("For “Font change”."), sel+"-meo-sung-font-weight",
		[][2]string{{"400", tr("Regular")}, {"600", tr("Semibold")}, {"700", tr("Bold")}, {"800", tr("Extra bold")}, {"900", tr("Black")}}, "900")
	s.styleToggle(tr("Sung word in italic"), "", sel+"-meo-sung-italic", false)
	s.styleToggle(tr("Sung word underlined"), "", sel+"-meo-sung-underline", false)
	s.styleChoice(tr("Line-synced lyrics"), tr("Line-synced lyrics light up as a whole line, never word by word."), sel+"-meo-line-animation",
		[][2]string{{"fade", tr("Fade in")}, {"none", tr("None")}}, "fade")
	s.styleNumber(tr("Glow"), tr("0 turns it off."), sel+"-meo-glow-radius", "px", 0, 12, 3, 1)
	s.styleNumber(tr("Word lift"), "", sel+"-meo-word-lift", "px", 0, 12, 2, 1)
	s.styleNumber(tr("Size of the active line"), "", sel+"-meo-active-scale", "", 80, 130, 1, 100)
	s.styleNumber(tr("Size of the other lines"), "", sel+"-meo-inactive-scale", "", 60, 110, 0.94, 100)
	s.styleNumber(tr("Scroll duration"), "", sel+"-meo-scroll-duration", "ms", 0, 2000, 620, 1)
	s.styleChoice(tr("Scroll easing"), "", sel+"-meo-scroll-easing", easingOpts, "out-quint")

	s.section(tr("Sync"))
	s.number(tr("Lyrics offset"), tr("Positive shows the lyrics later, negative earlier."), -5000, 5000, u.cfg.Prefs.LyricsOffsetMs, "ms",
		func(v int) { u.cfg.Prefs.LyricsOffsetMs = v })
	s.button(tr("Lyrics providers"), tr("Order and keys of the built-in lyrics providers (BetterLyrics, lrc.red, Unison, Paxsenix, Kugou, LRCLIB)."), tr("Configure"), func() {
		if p := u.core.Plugins.Get("builtin"); p != nil {
			u.pluginSettings(p, nil)
		}
	})
}

func (sp *settingsPage) playback(s *sb) {
	u := sp.u
	s.section(tr("Behavior"))
	s.toggle(tr("Icon in the system tray"), tr("Next to the clock, with play/pause, next and quit."), u.cfg.Prefs.TrayIcon, func(on bool) {
		u.cfg.Prefs.TrayIcon = on
		if u.tray != nil {
			u.tray.SetVisible(on)
		}
	})
	s.toggle(tr("Media keys and desktop widget"), tr("MPRIS: media keys, the media widget of the panel, the lock screen and playerctl control meofishes."), u.cfg.Prefs.MPRIS, func(on bool) {
		u.cfg.Prefs.MPRIS = on
		if on {
			u.startMPRIS()
		} else {
			u.stopMPRIS()
		}
	})
	s.toggle(tr("Notify when a song starts"), tr("Only while the window is not in front."), u.cfg.Prefs.Notify, func(on bool) { u.cfg.Prefs.Notify = on })
	s.toggle(tr("Keep playing when the window is closed"), tr("Closing the window hides it in the tray; quit from the tray icon."), u.cfg.Prefs.Background, func(on bool) { u.cfg.Prefs.Background = on })
	s.toggle(tr("Autoplay"), tr("When the queue ends, keep playing similar songs."), u.cfg.Prefs.Autoplay, func(on bool) { u.cfg.Prefs.Autoplay = on })
	s.toggle(tr("Stop after the current song"), "", u.cfg.Prefs.StopAfterTrack, func(on bool) { u.cfg.Prefs.StopAfterTrack = on })
	s.toggle(tr("Remember the queue"), tr("Restore the queue when the app opens."), u.cfg.Prefs.PersistQueue, func(on bool) { u.cfg.Prefs.PersistQueue = on })
	s.toggle(tr("Resume playing on start"), "", u.cfg.Prefs.ResumeOnStart, func(on bool) { u.cfg.Prefs.ResumeOnStart = on })
	s.number(tr("Seek step"), tr("Shift+← / Shift+→"), 1, 60, u.cfg.Prefs.SeekStepSecs, "s", func(v int) { u.cfg.Prefs.SeekStepSecs = v })

	s.section(tr("Audio"))
	s.choice(tr("Maximum quality"), tr("The best a source may send. Lossless sources stay lossless on Max; lower levels save data."),
		[][2]string{{"max", tr("Max (lossless when available)")}, {"high", tr("High (~192 kbps)")}, {"normal", tr("Normal (~128 kbps)")}, {"low", tr("Data saver (~64 kbps)")}},
		u.cfg.Prefs.AudioQuality, func(v string) {
			u.cfg.Prefs.AudioQuality = v
			u.core.ApplyQuality()
			u.prefetched = map[string]prefetch{}
			u.restartPlayer()
		})
	s.note(tr("Audio changes restart the player (the song continues where it was)."))
	s.choice(tr("Volume normalization (ReplayGain)"), "", [][2]string{{"no", tr("Off")}, {"track", tr("Per song")}, {"album", tr("Per album")}},
		u.cfg.Prefs.ReplayGain, func(v string) { u.cfg.Prefs.ReplayGain = v; u.restartPlayer() })
	s.toggle(tr("Even out loudness"), tr("Dynamic normalization: quiet and loud songs sound alike."), u.cfg.Prefs.Normalize, func(on bool) { u.cfg.Prefs.Normalize = on; u.applyFilters() })
	s.choice(tr("Speed"), tr("Pitch is kept."), [][2]string{{"0.5", "0.5×"}, {"0.75", "0.75×"}, {"0.9", "0.9×"}, {"1", "1×"}, {"1.1", "1.1×"}, {"1.25", "1.25×"}, {"1.5", "1.5×"}, {"2", "2×"}},
		strconv.FormatFloat(u.cfg.Prefs.Speed, 'f', -1, 64), func(v string) {
			f, _ := strconv.ParseFloat(v, 64)
			u.setSpeed(f)
		})

	s.section(tr("Equalizer"))
	s.toggle(tr("Equalizer"), "", u.cfg.Prefs.EQOn, func(on bool) { u.cfg.Prefs.EQOn = on; u.applyFilters() })
	presets := [][2]string{{"flat", tr("Flat")}, {"bass", tr("Bass boost")}, {"treble", tr("Treble boost")}, {"vocal", tr("Vocal")},
		{"rock", "Rock"}, {"pop", "Pop"}, {"electronic", tr("Electronic")}, {"acoustic", tr("Acoustic")}, {"loudness", tr("Loudness")}, {"custom", tr("Custom")}}
	cur := u.cfg.Prefs.EQPreset
	if len(u.cfg.Prefs.EQ) == len(player.EQBands) {
		cur = "custom"
	}
	s.choice(tr("Preset"), "", presets, cur, func(v string) {
		u.cfg.Prefs.EQPreset = v
		if v != "custom" {
			u.cfg.Prefs.EQ = nil
		} else if len(u.cfg.Prefs.EQ) != len(player.EQBands) {
			u.cfg.Prefs.EQ = make([]float64, len(player.EQBands))
		}
		u.cfg.Prefs.EQOn = true
		u.applyFilters()
		s.refresh()
	})
	gains := u.eqGainsFor(u.cfg.Prefs.EQPreset)
	for i, f := range player.EQBands {
		i := i
		name := itoa(f) + " Hz"
		if f >= 1000 {
			name = itoa(f/1000) + " kHz"
		}
		s.number(name, "", -12, 12, int(gains[i]), "dB", func(v int) {
			if len(u.cfg.Prefs.EQ) != len(player.EQBands) {
				u.cfg.Prefs.EQ = append([]float64(nil), u.eqGainsFor(u.cfg.Prefs.EQPreset)...)
			}
			u.cfg.Prefs.EQ[i] = float64(v)
			u.cfg.Prefs.EQPreset = "custom"
			u.cfg.Prefs.EQOn = true
			if u.eqTimer == nil {
				u.eqTimer = qt.NewQTimer()
				u.eqTimer.SetSingleShot(true)
				u.eqTimer.OnTimeout(u.applyFilters)
			}
			u.eqTimer.Start(250)
		})
	}
	devs := [][2]string{{"", tr("Automatic")}}
	if u.mpv != nil {
		var names []string
		all := u.mpv.Devices()
		for n := range all {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if n != "auto" {
				devs = append(devs, [2]string{n, firstNonEmpty(all[n], n)})
			}
		}
	}
	s.choice(tr("Output device"), "", devs, u.cfg.Prefs.AudioDevice, func(v string) { u.cfg.Prefs.AudioDevice = v; u.restartPlayer() })
	s.toggle(tr("Exclusive mode"), tr("Bit-perfect output where the system supports it."), u.cfg.Prefs.Exclusive, func(on bool) { u.cfg.Prefs.Exclusive = on; u.restartPlayer() })
	s.text(tr("Extra mpv options"), tr("For experts, e.g. --audio-samplerate=48000"), u.cfg.Prefs.ExtraMpvArgs, "--option=value", func(v string) {
		if v != u.cfg.Prefs.ExtraMpvArgs {
			u.cfg.Prefs.ExtraMpvArgs = v
			u.restartPlayer()
		}
	})

	s.section(tr("Video"))
	s.choice(tr("Video quality"), tr("Maximum height of videos from web pages."), [][2]string{{"360", "360p"}, {"480", "480p"}, {"720", "720p"}, {"1080", "1080p"}, {"2160", "4K"}},
		itoa(u.cfg.Prefs.VideoQuality), func(v string) { u.cfg.Prefs.VideoQuality = atoiDef(v, 720) })
	s.toggle(tr("Show video automatically"), tr("Turn the video on for songs that have one (YouTube)."), u.cfg.Prefs.VideoAuto, func(on bool) { u.cfg.Prefs.VideoAuto = on })
	s.toggle(tr("Run on X11 for video"), tr("On Wayland, mpv can only draw video inside the window through XWayland. Needs a restart."), u.cfg.Prefs.VideoX11, func(on bool) { u.cfg.Prefs.VideoX11 = on })

	s.section(tr("Player"))
	opts := [][2]string{{"", "mpv"}}
	for _, p := range u.core.Plugins.All() {
		if p.Manifest.Player && p.Manifest.Allows(plugin.CapPlayer) {
			opts = append(opts, [2]string{p.Manifest.ID, p.Manifest.Name})
		}
	}
	s.choice(tr("Player backend"), tr("xtm plugins can replace mpv by another player (e.g. VLC)."), opts, u.cfg.Plugins.Player,
		func(v string) { u.cfg.Plugins.Player = v; u.restartPlayer() })
	name := "—"
	if u.player != nil {
		name = u.player.Name()
	}
	s.note(tr("In use: {name}", "name", name))
}

func (sp *settingsPage) streaming(s *sb) {
	u := sp.u
	apply := func() { u.core.StartProxy() }
	s.note(tr("Songs are downloaded in parts, several at a time, so they start almost immediately. The next song starts downloading before the current one ends."))
	s.toggle(tr("Download in parallel parts"), "", u.cfg.Prefs.Chunked, func(on bool) { u.cfg.Prefs.Chunked = on; apply() })
	s.number(tr("Part size"), "", 64, 8192, u.cfg.Prefs.ChunkKB, "KB", func(v int) { u.cfg.Prefs.ChunkKB = v; apply() })
	s.number(tr("Parts at the same time"), "", 1, 16, u.cfg.Prefs.Workers, "", func(v int) { u.cfg.Prefs.Workers = v; apply() })
	s.toggle(tr("Download the whole song in the background"), tr("Seeking becomes instant."), u.cfg.Prefs.FullDownload, func(on bool) { u.cfg.Prefs.FullDownload = on; apply() })
	s.toggle(tr("Prepare the next song early"), "", u.cfg.Prefs.Prefetch, func(on bool) { u.cfg.Prefs.Prefetch = on })
	s.toggle(tr("Resolve web pages with yt-dlp"), tr("Needed for the parallel download of YouTube/SoundCloud songs."), u.cfg.Prefs.DirectYtdlp, func(on bool) { u.cfg.Prefs.DirectYtdlp = on })
	s.section(tr("Player cache"))
	s.number(tr("Cache size"), "", 16, 2048, u.cfg.Prefs.CacheMB, "MB", func(v int) { u.cfg.Prefs.CacheMB = v })
	s.number(tr("Read ahead"), "", 5, 600, u.cfg.Prefs.ReadaheadSecs, "s", func(v int) { u.cfg.Prefs.ReadaheadSecs = v })
	s.button(tr("Apply cache settings"), tr("Restarts the player."), tr("Apply"), u.restartPlayer)
}

func (sp *settingsPage) library(s *sb) {
	u := sp.u
	s.section(tr("Covers"))
	s.toggle(tr("Find missing covers"), tr("Uses the artwork plugins (iTunes and Deezer by default)."), u.cfg.Prefs.FetchArtwork, func(on bool) { u.cfg.Prefs.FetchArtwork = on })
	s.toggle(tr("Prefer album art over video thumbnails"), "", u.cfg.Prefs.PreferArtwork, func(on bool) { u.cfg.Prefs.PreferArtwork = on })
	s.section(tr("Playlists"))
	s.button(tr("Import playlists"), tr(".mepw, .m3u or .m3u8 files."), tr("Import…"), u.importPlaylists)
	s.button(tr("Export everything"), tr("All playlists and liked songs in one .mepw file."), tr("Export…"), func() { u.exportPlaylists(u.core.Library.All()...) })
	s.section(tr("Privacy"))
	s.toggle(tr("Keep listening history"), tr("Used for “Recently played” and recommendations. Nothing leaves your computer."), u.cfg.Prefs.KeepHistory, func(on bool) {
		u.cfg.Prefs.KeepHistory = on
		u.core.Library.KeepHistory = on
	})
	s.button(tr("Clear history"), "", tr("Clear"), func() {
		if u.confirm(tr("Clear history"), tr("Forget every song you played?")) {
			u.core.Library.ClearHistory()
			u.toast(tr("History cleared"))
		}
	})
	n := 0
	if l := u.core.Library.Liked(); l != nil {
		n = len(l.Tracks)
	}
	s.note(tr("{playlists} playlists · {liked} liked songs", "playlists", itoa(len(u.core.Library.All())-1), "liked", itoa(n)))
	_ = library.LikedID
}

func (sp *settingsPage) priority(s *sb) {
	u := sp.u
	s.note(tr("When several plugins can do the same thing, the first one in the list is asked first."))
	for _, c := range []struct{ cap, title string }{
		{plugin.CapLyrics, tr("Lyrics")}, {plugin.CapArtwork, tr("Covers")}, {plugin.CapMatch, tr("Playing songs from other plugins")},
		{plugin.CapRecommend, tr("Recommendations")}, {plugin.CapSearch, tr("Search")},
	} {
		c := c
		ps := u.core.Plugins.WithCap(c.cap)
		s.section(c.title)
		if len(ps) == 0 {
			s.note(tr("No plugin does this yet."))
			continue
		}
		for i, p := range ps {
			i := i
			up := u.iconButton("OrderUp", "chevron-up", tr("Up"))
			down := u.iconButton("OrderDown", "chevron-down", tr("Down"))
			up.SetEnabled(i > 0)
			down.SetEnabled(i < len(ps)-1)
			move := func(d int) {
				ids := make([]string, len(ps))
				for k, x := range ps {
					ids[k] = x.Manifest.ID
				}
				ids[i], ids[i+d] = ids[i+d], ids[i]
				u.core.Plugins.SetOrder(c.cap, ids)
				s.refresh()
			}
			up.OnClicked(func() { move(-1) })
			down.OnClicked(func() { move(1) })
			s.row(fmt.Sprintf("%d. %s", i+1, p.Manifest.Name), "", up.QWidget, down.QWidget)
		}
	}
	s.section(tr("Icons"))
	opts := [][2]string{{"", tr("Built-in icons")}}
	for _, p := range u.core.Plugins.All() {
		if p.Manifest.Icons != "" && p.Manifest.Allows(plugin.CapIcons) {
			opts = append(opts, [2]string{p.Manifest.ID, p.Manifest.Name})
		}
	}
	s.choice(tr("Icon pack"), tr("xtm plugins can replace every icon."), opts, u.cfg.Plugins.IconPack, func(v string) {
		u.cfg.Plugins.IconPack = v
		u.icons.Load(u.core.Plugins.IconPack())
		u.win.Update()
		u.applyTheme(false)
	})
	s.section(tr("Catalogs"))
	s.note(tr("Extra index.json URLs for themes and plugins, one per line. The official catalog is built in."))
	te := qt.NewQPlainTextEdit2()
	te.SetMaximumHeight(110)
	te.SetPlainText(strings.Join(u.cfg.Catalogs, "\n"))
	s.b.AddWidget(te.QWidget)
	save := qt.NewQPushButton3(tr("Save catalogs"))
	save.OnClicked(func() {
		var list []string
		for _, l := range strings.Split(te.ToPlainText(), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				list = append(list, l)
			}
		}
		u.cfg.Catalogs = list
		u.core.Catalog.Refresh()
		_ = u.cfg.Save()
		u.toast(tr("Catalogs saved"))
	})
	s.b.AddWidget3(save.QWidget, 0, qt.AlignLeft)
}

func (sp *settingsPage) language(s *sb) {
	u := sp.u
	keys := uiStrings()
	cur := i18n.Current().Code
	for _, l := range i18n.Languages() {
		l := l
		pct := int(i18n.Coverage(l, keys)*100 + 0.5)
		desc := l.Code
		if l.Code != "en" {
			desc += " · " + tr("{n}% translated", "n", itoa(pct))
		}
		b := qt.NewQPushButton3(tr("Use"))
		if l.Code == cur {
			b.SetText(tr("In use"))
			b.SetEnabled(false)
		} else {
			role(b.QWidget, "primary")
		}
		b.OnClicked(func() {
			u.cfg.Language = l.Code
			_ = u.cfg.Save()
			if u.confirm(tr("Restart?"), tr("The language changes after restarting meofishes. Restart now?")) {
				u.restart()
			}
		})
		s.row(l.Name, desc, b.QWidget)
	}
	s.note(tr("Translations are JSON files. Put your own in {dir} to add or fix a language.", "dir", filepath.Join(u.core.Paths.Data, "locales")))
}

func (sp *settingsPage) shortcuts(s *sb) {
	for _, sc := range Shortcuts {
		k := label(sc[0], "", "kbd")
		s.row(tr(sc[1]), "", k.QWidget)
	}
}

func (sp *settingsPage) storage(s *sb) {
	u := sp.u
	size := func(dir string) string { return humanBytes(dirSize(dir)) }
	s.section(tr("Songs on disk"))
	dn, dsize := u.core.Offline.Usage(offline.Download)
	cn, csize := u.core.Offline.Usage(offline.Cache)
	s.toggle(tr("Keep played songs"), tr("Songs played to the end stay on disk and play instantly, even offline."), u.cfg.Prefs.AutoCache, func(on bool) { u.cfg.Prefs.AutoCache = on })
	s.number(tr("Song cache size"), "", 64, 65536, u.cfg.Prefs.CacheSongsMB, "MB", func(v int) {
		u.cfg.Prefs.CacheSongsMB = v
		u.core.Offline.MaxCacheMB = v
		u.core.Offline.Trim()
	})
	s.button(tr("Song cache"), plural(cn, "{n} song", "{n} songs")+" · "+humanBytes(csize), tr("Clear"), func() { u.core.Offline.Clear(offline.Cache); s.refresh() })
	s.text(tr("Downloads folder"), plural(dn, "{n} song", "{n} songs")+" · "+humanBytes(dsize), u.cfg.Prefs.DownloadDir, u.core.DownloadDir(), func(v string) {
		u.cfg.Prefs.DownloadDir = v
		u.core.Offline.DownloadDir = u.core.DownloadDir()
	})
	s.button(tr("Downloaded songs"), "", tr("Open"), func() { u.openPlaylist(downloadsID) })
	s.section(tr("Other caches"))
	covers := filepath.Join(u.core.Paths.Cache, "covers")
	lyr := filepath.Join(u.core.Paths.Cache, "lyrics")
	s.button(tr("Cover cache"), size(covers), tr("Clear"), func() { _ = os.RemoveAll(covers); _ = os.MkdirAll(covers, 0o755); s.refresh() })
	s.button(tr("Lyrics cache"), size(lyr), tr("Clear"), func() { _ = os.RemoveAll(lyr); _ = os.MkdirAll(lyr, 0o755); s.refresh() })
	s.button(tr("Found covers"), plural(u.art.Size(), "{n} song", "{n} songs"), tr("Forget"), func() { u.art.Clear(); s.refresh() })
	s.button(tr("Settings folder"), u.core.Paths.Config, tr("Open"), func() { u.openFolder(u.core.Paths.Config) })
	s.button(tr("Data folder"), u.core.Paths.Data+" · "+size(u.core.Paths.Data), tr("Open"), func() { u.openFolder(u.core.Paths.Data) })
	s.button(tr("Reset all settings"), tr("Keeps your library, plugins and themes."), tr("Reset"), func() {
		if u.confirm(tr("Reset settings"), tr("Go back to the default settings? meofishes will restart.")) {
			u.cfg.Prefs = config.DefaultPrefs()
			u.cfg.Prefs.TutorialAsked = true
			u.cfg.Style = map[string]string{}
			u.cfg.UserCSS = ""
			_ = u.cfg.Save()
			u.restart()
		}
	})
}

func (sp *settingsPage) developer(s *sb) {
	u := sp.u
	s.toggle(tr("Developer mode"), tr("Adds the Dev tab (logs, network, plugins, lyrics, streams, theme inspector) and F12 to inspect widgets."), u.cfg.Prefs.DevMode, func(on bool) {
		u.cfg.Prefs.DevMode = on
		u.devModeChanged()
	})
	s.toggle(tr("Show FPS"), "", u.cfg.Prefs.ShowFPS, func(on bool) { u.cfg.Prefs.ShowFPS = on; u.devModeChanged() })
	s.toggle(tr("Record network requests"), "", u.cfg.Prefs.LogHTTP, func(on bool) { u.cfg.Prefs.LogHTTP = on; u.netlogSync() })
	s.toggle(tr("Reload the theme when its files change"), "", u.cfg.LiveReload, func(on bool) { u.cfg.LiveReload = on })
	s.button(tr("Open the developer tab"), "", tr("Open"), func() {
		if !u.cfg.Prefs.DevMode {
			u.cfg.Prefs.DevMode = true
			u.devModeChanged()
		}
		u.goPage("dev")
	})
	s.button(tr("Documentation"), tr("Everything about themes, plugins, lyrics and the .mepw format."), tr("Open"), func() {
		url := qt.NewQUrl3(config.ContactURL + "/src/branch/main/docs")
		qt.QDesktopServices_OpenUrl(url)
		url.Delete()
	})
}

func (sp *settingsPage) about(s *sb) {
	u := sp.u
	s.row("meofishes "+config.Version, tr("Music from plugins.")+" · "+config.Commit, label("", "", "").QWidget)
	s.section(tr("Updates"))
	s.toggle(tr("Check for updates on start"), "", u.cfg.Prefs.AutoUpdate, func(on bool) { u.cfg.Prefs.AutoUpdate = on })
	s.button(tr("Update"), tr("Installs the latest build of meofishes."), tr("Check now"), func() { u.checkUpdate(true) })
	s.button(tr("Tutorial"), tr("A quick tour of the app."), tr("Show the tutorial"), func() { u.startTour(false) })
	s.button(tr("Project page"), config.ContactURL, tr("Open"), func() {
		url := qt.NewQUrl3(config.ContactURL)
		qt.QDesktopServices_OpenUrl(url)
		url.Delete()
	})
	mpv := tr("not found")
	if player.Available() {
		mpv, _ = exec.LookPath("mpv")
	}
	yt := tr("not found")
	if p := plugin.YtdlpPath(); p != "" {
		yt = p
	}
	s.row("mpv", mpv, label("", "", "").QWidget)
	s.row("yt-dlp", yt, label("", "", "").QWidget)
	s.row(tr("Contact sent to lyrics services"), config.UserAgent, label("", "", "").QWidget)
	s.row(tr("Go modules for plugins"), strings.Join(plugin.ModuleNames(), ", "), label("", "", "").QWidget)
}

// ---------------------------------------------------------------- helpers

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func homeDir() string { h, _ := os.UserHomeDir(); return h }

func uiStrings() []string { return i18n.Keys() }

// eqGainsFor returns the gains shown in Settings (custom or a preset).
func (u *UI) eqGainsFor(preset string) []float64 {
	if len(u.cfg.Prefs.EQ) == len(player.EQBands) {
		return u.cfg.Prefs.EQ
	}
	if g, ok := player.EQPresets[preset]; ok {
		return g
	}
	return player.EQPresets["flat"]
}
