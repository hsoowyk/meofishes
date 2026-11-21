# Themes

A theme is a folder with `theme.json` and CSS files, installed in
`~/.local/share/meofishes/themes/<id>/` (or from **Settings → Themes**).

```json
{
  "id": "my-theme",
  "name": "My theme",
  "version": "1.0.0",
  "author": "you",
  "description": "…",
  "extends": "fishalike",
  "style": "style.css",
  "dark": true,
  "fonts": ["fonts/MyFont.ttf"],
  "preview": { "background": "#111", "surface": "#222", "accent": "#f0a", "highlight": "#fcd", "text": "#fff" }
}
```

- `extends`: start from another theme; your CSS is applied after it. The
  three bundled themes show the idea: Abyss and Sakura Paper only change
  Fishalike's variables and a few properties.
- `fonts`: font files loaded before the CSS.
- **Settings → Themes → Create my theme** makes a new theme that extends the
  current one, opens its folder and turns on live reload.

## The CSS

meofishes CSS is Qt stylesheets (QSS) plus:

| feature | example |
|---|---|
| variables | `:root { --red: #d9737a; }` … `color: var(--red, #f00);` |
| color functions | `alpha(var(--red), .5)`, `mix(#000, #fff, .3)`, `lighten(c, 10%)`, `darken`, `saturate`, `desaturate` |
| imports | `@import "parts/lyrics.css";` |
| painted widgets | `-meo-*` properties ([full list](properties.md)) |

Qt selectors work: `#ObjectName`, `QPushButton[role="primary"]`,
`#TrackRow[playing="true"] QLabel`, `:hover`, `:focus`… The object names of
every part of the window are in [interface.md](../interface.md).

Durations accept `ms` or `s`; lengths accept `px`; booleans `true/false`;
easings: `linear`, `out-cubic`, `out-quint`, `out-expo`, `out-back`,
`out-elastic`, `out-bounce`, `in-out-sine`, `in-out-cubic`, `cubic-bezier(a,b,c,d)`
and the names registered by `ani` plugins.

## Layers

The final stylesheet is built in this order (later wins):

1. the parent themes (`extends`), then the theme;
2. the CSS of enabled **ani** plugins;
3. the **Settings layer** — what you change in Settings (colors, fonts,
   lyrics, animations) — stored in `config.json` → `style` as
   `"selector|property": "value"`;
4. the **user CSS** (Settings → Themes → User CSS).

`F5` reloads the theme; with live reload on, saving a file applies it.
`meofishes css <folder>` prints the compiled QSS of a theme.
