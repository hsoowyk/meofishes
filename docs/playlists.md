# Playlists and the .mepw format

The library (`~/.local/share/meofishes/library.json`) holds:

- **Liked songs** — the heart buttons and Ctrl+D; always first.
- **Local playlists** — made in meofishes: + in the left panel, "Save as
  playlist" in the queue, or right-click a song → Add to playlist. Drag to
  reorder; right-click to rename, change the cover, export or delete.
- **Plugin playlists** — from `src` plugins with the `playlists` capability
  (Library → From your plugins). Opening one saves a copy that can be
  refreshed.
- **History** — the last 300 songs played ("Recently played", recommendations).
  Can be paused or cleared in Settings → Library & privacy.

## .mepw

A `.mepw` file is UTF-8 JSON (optionally gzip-compressed — meofishes detects
it) with one or more playlists:

```json
{
  "mepw": 1,
  "generator": "meofishes 0.2.0",
  "exported": "2026-11-21T18:00:00Z",
  "playlists": [
    {
      "name": "Road trip",
      "description": "optional",
      "cover": "https://… or a local path (optional)",
      "kind": "local",
      "source": "",
      "remoteId": "",
      "created": "2026-11-20T10:00:00Z",
      "tracks": [
        { "source": "deezer", "id": "3135556", "title": "Harder, Better, Faster, Stronger",
          "artist": "Daft Punk", "album": "Discovery", "durationMs": 224000,
          "cover": "https://…", "isrc": "GBDUW0000059" }
      ]
    }
  ]
}
```

- `kind`: `local`, `liked` (merged into your liked songs on import) or
  `remote` (`source` = plugin id, `remoteId` = its collection id).
- Tracks are regular [tracks](plugins/README.md#track). If the plugin that
  listed a track is not installed, meofishes plays it through an fnt plugin
  with `match` (title + artist), so playlists travel between computers.
- Files with a newer `mepw` version are refused.

Import also accepts `.m3u`/`.m3u8` (`#EXTINF` titles, URLs or paths) and a
bare playlist object. A single playlist can also be exported as `.m3u8`.
