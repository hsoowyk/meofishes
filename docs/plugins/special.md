# ani, dep, xtm and evt plugins

## ani — animations

An `ani` plugin changes how the app moves. It needs no runtime:

```json
{ "id": "snappy", "name": "Snappy animations", "version": "1.0.0", "types": ["ani"],
  "animations": "anim.css",
  "easings": { "snappy": [0.2, 0.9, 0.1, 1.0] } }
```

- `animations`: a CSS file applied **after the theme** and before the
  Settings layer. Use it for `-meo-enter*`, `-meo-transition*`,
  `-meo-swipe*`, `-meo-scroll-*`, `-meo-word-animation`… (any rule works).
- `easings`: named `cubic-bezier` curves usable anywhere an easing is
  accepted (`-meo-transition-easing: snappy;`).

```css
/* anim.css */
#Pages { -meo-transition: zoom; -meo-transition-easing: snappy; }
#TrackRow { -meo-enter: slide-left; -meo-enter-stagger: 12ms; }
#FullLyrics { -meo-word-animation: bounce; -meo-word-lift: 4px; }
```

## dep — tools for other plugins

A `dep` plugin is a library or a set of executables. Other plugins list it in
`requires`; if it is missing or disabled they show an error.

```json
{ "id": "my-lib", "name": "My library", "version": "1.0.0", "types": ["dep"],
  "runtime": "script", "entry": "lib.js", "bin": "bin" }
```

- Scripts: `const lib = require("dep:my-lib")` loads the dep's `entry` (its
  own `require("./x")` resolve inside the dep folder).
- Process plugins: `MEOFISHES_DEP_MY_LIB` points at the folder and `bin` is
  added to `PATH`.

Only plugins of type `dep` can be required.

## xtm — extreme changes

**Icon pack**: a folder with one `<name>.svg` or `<name>.png` per icon. Icons
it lacks fall back to the built-in ones. With `"iconsTint": true` (default)
single-color icons are recolored by the theme. Pick it in
**Settings → Plugin priority → Icon pack**.

```json
{ "id": "my-icons", "name": "My icons", "version": "1.0.0", "types": ["xtm"], "icons": "icons" }
```

Icon names: `play pause next prev shuffle repeat repeat-one volume volume-low
mute lyrics queue home search library plugin theme settings code plus close
note heart heart-outline folder refresh fish chevron-down chevron-up
chevron-left chevron-right back more drag playlist export import radio trash
edit check info fullscreen play-next minimize maximize restore`.

SVG icons need the Qt SVG image plugin (`qt5-svg`).

**Player**: a process plugin with `"player": true` replaces mpv — see
[process.md](process.md#player-plugins-xtm).

## evt — events

Any runtime that can run code: export `onEvent(name, data)` (script) or
handle `event` notifications (process). List what you want in `events`.
Example: `assets/plugins/listenbrainz`.
