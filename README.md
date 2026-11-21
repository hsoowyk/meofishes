# meofishes 🐟

A music player in Go + Qt5 where **all the music comes from plugins**: want to
listen from somewhere? Install the plugin and enjoy.

- **Plugins** with informative, permissive types — `fnt` (sources of music,
  lyrics, covers), `src` (search, home, recommendations, playlists), `ani`
  (animations), `dep` (tools for other plugins), `xtm` (replace the player or
  the icons), `evt` (events) — written as plain JSON, JavaScript with Go
  modules, or any program (Go SDK included).
- **Word-synced lyrics** by default (Lyprio → BetterLyrics → lrc.red → Unison
  → Paxsenix → Kugou), a swiping current-lyric line, a fullscreen player with
  a tilting cover, and a frequency visualizer when a song has no lyrics.
- **Instant playback**: songs download in parallel parts and the next song is
  prepared ahead of time.
- **Playlists**, liked songs, plugin playlists, **.mepw** import/export.
- **100% CSS** interface, themes from the app, and Settings that change
  almost everything; animations at your screen's refresh rate.
- **Developer mode** that shows everything the app does.
- English, Português, Français, Türkçe, Deutsch, Español.

## Install

```sh
go build -ldflags="-s -w" -o meofishes ./cmd/meofishes   # see docs/building.md
./install.sh            # or ./install.sh --user
```

Needs mpv (and yt-dlp for YouTube/SoundCloud). The first build compiles the Qt
bindings: it is slow and needs a lot of RAM — `tools/qtlib/qtlib.sh` makes it
much faster on machines with many cores.

## Documentation

Everything is in **[docs/](docs/README.md)**: getting started, building,
interface map, themes and every `-meo-*` property, plugins (all runtimes,
types, the player and icon-pack protocols), lyrics, playlists and `.mepw`,
streaming, settings, developer mode and translations.

## Em português

O meofishes é um player de música em que toda música vem de plugins. A
interface pode ficar em português (Ajustes → Idioma). Para compilar e
instalar, veja os comandos acima; a documentação completa está em `docs/`.

## Layout of the code

```
cmd/meofishes        main + CLI
internal/app         wiring of everything that is not the interface
internal/ui          Qt interface (miqt)
internal/core        types (Track, Lyrics…)
internal/plugin      manifest, types/permissions, manager, runtimes, Go modules, player plugins
internal/builtin     built-in plugins (Meo Lyrics, Meo Artwork, Local files)
internal/lyrics      TTML/LRC/KRC parsers, providers, priority chain
internal/theme       CSS compiler (variables, color functions, -meo-*), easings
internal/player      mpv over JSON IPC
internal/stream      chunked download proxy
internal/library     playlists, liked songs, history, .mepw
internal/viz         frequency analysis for the visualizer
internal/i18n        translations
internal/netlog      network log (developer mode)
internal/catalog     theme/plugin catalog
sdk/                 Go SDK for process plugins
assets/              bundled themes, plugins, catalog, translations
tools/               qtlib (fast Qt build), uicheck (type-check without Qt), i18nkeys
packaging/           .desktop file and icon
docs/                documentation
```
