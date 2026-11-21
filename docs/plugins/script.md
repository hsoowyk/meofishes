# Script plugins (JavaScript + Go modules)

JavaScript runs in an embedded engine (goja, ES2015+) **synchronously**:
module calls just return their result, no `await`.

```js
const http = require("http");       // must be listed in "modules"
const util = require("./util");     // another file of the plugin
const lib  = require("dep:my-dep"); // a dep plugin listed in "requires"

exports.search    = (query) => ({ tracks: [...] });          // src
exports.home      = () => ({ tracks: [...] });               // src
exports.recommend = (track) => ({ tracks: [...] });          // src
exports.playlists = () => [{ id, title, cover }];            // src
exports.open      = (collection) => ({ ...collection, tracks: [...] });
exports.resolve   = (track) => "https://…" /* or { url, headers, page } */;  // fnt
exports.lyrics    = (q) => ({ format: "lrc", content: "[00:01.00]…" });       // fnt
exports.artwork   = (q) => "https://…/cover.jpg";                             // fnt
exports.onEvent   = (name, data) => {};                                       // evt
```

The capabilities are the functions you export (and that your `types` allow).
A script that exports `search` can play its own tracks even without
`resolve`. With `"match": true`, `resolve(track)` also receives tracks of other
plugins (`track.source !== app.plugin`) — search for them by title/artist.
See `assets/plugins/ytdlp-search/main.js`.

Globals: `settings` (read only), `app.version`, `app.userAgent`, `app.plugin`,
`console.log/warn/error` (shown in Dev → Log). Exceptions become error
messages in the interface.

## Go modules (`"modules"`)

| module | functions |
|---|---|
| `http` | `get(url, opts)`, `getJSON(url, opts)`, `post(url, body, opts)`, `postJSON(url, body, opts)`, `request({url, method, body, headers, query, timeout})`, `encode(obj)`. `opts = { headers, query, timeout }`. Response `{ status, ok, headers, body, json }`. Respects `hosts`. |
| `ytdlp` | `available()`, `search(query, limit, site)` (`youtube`, `ytmusic`, `soundcloud`), `info(url)`, `streamUrl(url)`, `playlist(url, limit)`. Needs yt-dlp. |
| `lavalink` | `connect({host, port, password, secure})` → `{ search(query, prefix), loadTracks(id), decode(encoded), info() }` |
| `lyrics` | `parse(format, content)`, `cleanTitle(s)`, `primaryArtist(s)`, `matchScore(query, title, artist, durationMs)`, `decodeKRC(b64)` |
| `crypto` | `md5`, `sha1`, `sha256`, `hmacSha256(key, msg)`, `base64Encode`, `base64Decode`, `uuid` |
| `storage` | `get(k)`, `set(k, v)`, `remove(k)`, `all()` — persistent, per plugin |

`meofishes modules` lists them.
