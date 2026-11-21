# meofishes documentation

meofishes is a music player in Go + Qt5 where **everything comes from
plugins**: search, playback, lyrics, covers, recommendations. The look is
**100% CSS**.

| guide | what it covers |
|---|---|
| [Features](features.md) | everything meofishes does, in one page |
| [Getting started](getting-started.md) | install, first run, the interface, shortcuts, tutorial |
| [Building](building.md) | compiling, the fast Qt shared library, CI, `uicheck` |
| [Command line](cli.md) | `meofishes lyrics`, `search`, `check`, `css`, `setup`… |
| [Interface](interface.md) | every part of the window and its CSS object names |
| [Themes](themes/README.md) | writing a theme: variables, color functions, `theme.json`, layers |
| [Theme properties](themes/properties.md) | every `-meo-*` property of every painted widget |
| [Plugins](plugins/README.md) | types and permissions, capabilities, `plugin.json`, events |
| [Declarative plugins](plugins/declarative.md) | JSON only: endpoints + field mapping |
| [Script plugins](plugins/script.md) | JavaScript with the Go modules (`http`, `ytdlp`, …) |
| [Process plugins](plugins/process.md) | Go (or any language) over JSON-RPC, the SDK, the player protocol |
| [ani, dep, xtm, evt](plugins/special.md) | animation layers, libraries for plugins, icon packs, player replacement, events |
| [Lyrics](lyrics.md) | providers, formats (TTML, LRC, KRC), sync types, animation, offset |
| [Playlists and .mepw](playlists.md) | library, liked songs, the `.mepw` format |
| [Streaming](streaming.md) | chunked downloads, prefetch, cache, the visualizer |
| [Settings](settings.md) | every setting and where it is stored |
| [Developer mode](developer-mode.md) | the Dev tab, F12 inspector, logs, network |
| [Translations](i18n.md) | languages and how to add or fix one |

Files and folders (XDG):

| path | contents |
|---|---|
| `~/.config/meofishes/config.json` | settings, Settings style layer, plugin state, saved queue |
| `~/.local/share/meofishes/plugins/<id>/` | installed plugins |
| `~/.local/share/meofishes/themes/<id>/` | installed themes |
| `~/.local/share/meofishes/library.json` | playlists, liked songs, history |
| `~/.local/share/meofishes/plugin-data/<id>/` | plugin storage |
| `~/.local/share/meofishes/locales/` | your own translations |
| `~/.cache/meofishes/` | covers, lyrics cache, found artwork |
