# Plugins

Everything meofishes plays comes from plugins. A plugin is a folder with a
`plugin.json`, installed in `~/.local/share/meofishes/plugins/<id>/` or from
**Settings → Plugins** (.zip, folder, URL, or the catalog).

## Types: informative and permissive

`types` says what a plugin is **and what it is allowed to do**. A plugin can
only use the capabilities of the types it declares (the Plugins page shows
anything it implements but is not allowed to use). Plugins are shown in one
list; the types are labels.

| type | for | allows |
|---|---|---|
| `fnt` | sources of music/video/lyrics/covers | `stream`, `match`, `lyrics`, `artwork` |
| `src` | search, home page, recommendations, playlists | `search`, `home`, `recommend`, `playlists`, `open` |
| `ani` | animation changes | `animation` (CSS layer + easing curves) |
| `dep` | tools for other plugins | `library` (`require("dep:<id>")`, executables) |
| `xtm` | extreme changes | `player` (replace mpv), `icons` (icon pack) |
| `evt` | reacting to events (scrobblers…) | `events` |

`meofishes types` prints this table. Old names still load: `source` →
`fnt`+`src`, `lyrics` → `fnt`, `hook` → `evt`. Layouts are no longer plugins.

## Capabilities

| capability | what meofishes asks |
|---|---|
| `search` | tracks for a query (Search page, home search box) |
| `home` | tracks for the home page ("Popular near you") |
| `recommend` | tracks similar to a seed track (home, autoplay, "Start radio") |
| `playlists` / `open` | the user's playlists on a service / the tracks of a collection |
| `stream` | turn one of its own tracks into a playable URL |
| `match` | play **any** track (from a metadata-only plugin) by searching title + artist. Set `"match": true` |
| `lyrics` | lyrics for a song (joins the lyrics priority list) |
| `artwork` | a cover image URL for a song without one |
| `events` | player events |
| `player`, `icons`, `animation`, `library` | see [special.md](special.md) |

How a song is played: its own plugin's `stream` → its `streamUrl` → the
`match` plugins in priority order. This is how the **Deezer** plugin (metadata
and real covers only) plays through **yt-dlp**.

When several plugins can do the same thing, **Settings → Plugin priority**
decides the order (lyrics, covers, match, recommendations, search).

## Runtimes

| runtime | when |
|---|---|
| [`declarative`](declarative.md) | an HTTP + JSON API: describe endpoints and fields, no code |
| [`script`](script.md) | logic in JavaScript, with the app's Go modules |
| [`process`](process.md) | any program (Go SDK included) over JSON on stdin/stdout |
| *(none)* | asset-only plugins: an `ani` CSS file or an `xtm` icon pack |

## plugin.json

```jsonc
{
  "id": "my-plugin",               // lowercase, digits, . _ -
  "name": "My plugin",
  "version": "1.0.0",
  "author": "you",
  "description": "What it does",
  "homepage": "https://…",
  "types": ["fnt", "src"],
  "runtime": "script",
  "entry": "main.js",              // script: JS file; process: command line
  "build": "go build -o bin .",    // process: built at install time (the user confirms)
  "modules": ["http", "ytdlp"],    // Go modules for scripts
  "hosts": ["api.example.com", "*.cdn.example.com"],  // limits the http module
  "match": true,                   // fnt: can play any track by title/artist
  "requires": ["my-dep"],          // dep plugins it needs
  "events": ["track.*"],           // evt: events to receive ("*" = all)
  "settings": [                    // a form in Settings → Plugins → Configure
    { "key": "token", "label": "Token", "type": "password", "description": "…" },
    { "key": "site", "type": "choice", "choices": ["a", "b"], "default": "a" },
    { "key": "hq", "type": "bool", "default": "true" }
  ],
  "source": { … }, "lyrics": { … }, "artwork": { … },   // declarative blocks
  "animations": "anim.css", "easings": { "snappy": [0.2, 0.9, 0.1, 1] },  // ani
  "icons": "icons", "iconsTint": true, "player": false, // xtm
  "bin": "bin"                                         // dep
}
```

Setting types: `string` (default), `password`, `bool`, `number`, `choice`, `list`.

## Track

```json
{
  "id": "unique inside the plugin", "source": "set by meofishes",
  "title": "Song", "artist": "Artist", "album": "Album",
  "durationMs": 215000,
  "cover": "https://…/cover.jpg",
  "streamUrl": "https://…/audio.mp3  or  https://youtube.com/watch?v=…",
  "isrc": "USUG11904206",
  "videoId": "fHI8X4OXluQ",
  "extra": { "anything": "comes back to you" }
}
```

A stream is `{ "url": …, "headers": {…}, "page": false }`. `page: true` means
the URL is a web page (YouTube, SoundCloud…): meofishes resolves it with yt-dlp.

## Events

| event | payload |
|---|---|
| `track.start` | `{ track }` |
| `track.end` | `{ track, positionMs }` |
| `track.like` | `{ track, liked }` |
| `playback.pause`, `playback.resume` | — |
| `playback.seek` | `{ positionMs }` |
| `lyrics.found` | `{ track, provider, sync }` |
| `queue.change` | `{ size, index }` |
| `playlist.change` | `{ id, name, size }` |
| `theme.change` | `{ theme }` |

`"events": ["track.*"]` receives every track event.

## Testing

```sh
meofishes check ./my-plugin
meofishes search my-plugin "daft punk"
meofishes lyrics "Artist" "Title"
```

In developer mode, **Dev → Plugins** shows every plugin's capabilities, call
counts, timings and last errors.

## Distributing

Zip the folder (`plugin.json` at the root or in a single subfolder). To show
up in the catalog, list it in an `index.json` and add its URL in
**Settings → Plugin priority → Catalogs**:

```json
{ "name": "My catalog", "entries": [
  { "kind": "plugin", "id": "my-plugin", "name": "My plugin", "version": "1.0.0",
    "author": "you", "description": "…", "types": ["src"], "runtime": "script", "url": "my-plugin-1.0.0.zip" },
  { "kind": "theme", "id": "my-theme", "name": "My theme", "version": "1.0.0",
    "preview": { "background": "#111", "surface": "#222", "accent": "#f0a", "highlight": "#fcd", "text": "#fff" },
    "url": "https://example.com/my-theme.zip" } ] }
```

Relative URLs are resolved against the `index.json`.
