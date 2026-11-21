# Declarative plugins (JSON only)

Describe where each function gets its data. Templates `{{…}}` work in URLs,
headers and bodies; in URLs values are escaped automatically. Filters:
`{{query|raw}}`, `|url`, `|json`, `|lower`, `|upper`.

Variables: `query` (search), `settings.*`, `app.version`, `app.userAgent`,
`track.*` (resolve, recommend), `item.*` (inside `map`).

## src / fnt: the `source` block

```json
{
  "id": "deezer", "name": "Deezer", "version": "1.0.0",
  "types": ["src"], "runtime": "declarative", "hosts": ["api.deezer.com"],
  "source": {
    "baseUrl": "https://api.deezer.com",
    "headers": { "Accept": "application/json" },
    "search": {
      "url": "/search?q={{query}}&limit=40",
      "items": "data",
      "map": { "id": "id", "title": "title", "artist": "artist.name", "album": "album.title",
               "cover": "album.cover_xl || album.cover_big", "durationSec": "duration", "artistId": "artist.id" }
    },
    "home":      { "url": "/chart/0/tracks", "items": "data", "map": { "…": "…" } },
    "recommend": { "url": "/artist/{{track.extra.artistId}}/radio", "items": "data", "map": { "…": "…" } },
    "resolver": "match"
  }
}
```

- `items`: path to the list (`$` = root, `data.results`, `items[0].tracks`).
- `map`: track field → path in the item. `a || b` tries alternatives,
  `'text'` is a literal, `"https://img/{{item.id}}.jpg"` is a template.
  `durationSec` converts seconds. Unknown fields go to `extra`
  (`artistId` above → `track.extra.artistId`).
- `resolve` (optional): `{ "url": "/stream/{{track.id}}", "path": "url" }`.
- `resolver`: `direct` (the URL is audio), `ytdlp` (the URL is a web page), or
  `match` (metadata only: played by an fnt plugin with `"match": true`).

Capabilities come from the blocks: `search`, `home`, `recommend` from their
endpoints; `stream` unless `resolver` is `match`. A declarative plugin with
`"match": true` and a `search` endpoint plays other plugins' tracks by
searching `artist title` and taking the first result.

## fnt: the `lyrics` block

```json
"lyrics": {
  "request": { "url": "https://lrclib.net/api/search?track_name={{cleanTitle}}&artist_name={{primaryArtist}}" },
  "path": "[0].syncedLyrics || [0].plainLyrics",
  "format": "auto",
  "credit": "Lyrics from LRCLIB"
}
```

Variables: `title`, `artist`, `album`, `durationSec`, `durationMs`, `isrc`,
`videoId`, `cleanTitle` (without "(Official Video)"…), `primaryArtist`.
Formats: `ttml`, `lrc` (also word-timed `<mm:ss.xx>`), `krc`, `plain`, `auto`;
or `formatPath` to read the format from the response.

## fnt: the `artwork` block

```json
"artwork": {
  "request": { "url": "https://itunes.apple.com/search?entity=song&limit=1&term={{primaryArtist}}%20{{cleanTitle}}" },
  "path": "results[0].artworkUrl100",
  "replace": ["100x100bb", "600x600bb"]
}
```
