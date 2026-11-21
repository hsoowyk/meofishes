# Process plugins (Go or any language)

An executable that talks JSON, one message per line, on stdin/stdout. In Go,
use the SDK (`sdk/sdk.go`, standard library only — import it or copy it) and
**any Go library you want**.

```go
package main

import "meofishes/sdk"

func main() {
	sdk.Serve(sdk.Plugin{
		Search: func(ctx *sdk.Context, q string) (*sdk.SearchResult, error) {
			ctx.Log("searching %s", q)
			return &sdk.SearchResult{Tracks: []sdk.Track{{ID: "1", Title: "Song", StreamURL: "https://…/a.mp3"}}}, nil
		},
		Resolve:   func(ctx *sdk.Context, t sdk.Track) (*sdk.Stream, error) { return &sdk.Stream{URL: t.StreamURL}, nil },
		Recommend: func(ctx *sdk.Context, seed sdk.Track) (*sdk.SearchResult, error) { return &sdk.SearchResult{}, nil },
		Artwork:   func(ctx *sdk.Context, q sdk.LyricsQuery) (string, error) { return "", sdk.ErrNotFound },
	})
}
```

```json
{ "id": "my-go", "name": "My Go plugin", "version": "1.0.0",
  "types": ["fnt", "src"], "runtime": "process",
  "entry": "./my-go", "build": "go build -o my-go ." }
```

`build` runs once at install time, **after the user confirms** (it executes
the plugin's code). Full example: `assets/plugins/somafm-go`.

Environment: `MEOFISHES_PLUGIN`, `MEOFISHES_VERSION`, and for each dep plugin
in `requires`: `MEOFISHES_DEP_<ID>=<folder>`; their `bin` folders are added to
`PATH`.

## Protocol

```
→ {"id":1,"method":"initialize","params":{"plugin":"…","version":"…","userAgent":"…","settings":{…},"dataDir":"…"}}
← {"id":1,"result":{"capabilities":["search","home","open","resolve","recommend","playlists","lyrics","artwork","events","player"]}}
→ {"id":2,"method":"search","params":{"query":"…"}}          ← {"id":2,"result":{"tracks":[…]}}
→ {"id":3,"method":"home"}                                   ← {"id":3,"result":{"tracks":[…]}}
→ {"id":4,"method":"resolve","params":{"track":{…}}}         ← {"id":4,"result":{"url":"…","page":false}}
→ {"id":5,"method":"recommend","params":{"track":{…}}}       ← {"id":5,"result":{"tracks":[…]}}
→ {"id":6,"method":"playlists"}                              ← {"id":6,"result":[{"id":"…","title":"…"}]}
→ {"id":7,"method":"open","params":{"collection":{…}}}       ← {"id":7,"result":{…,"tracks":[…]}}
→ {"id":8,"method":"lyrics","params":{"query":{…}}}          ← {"id":8,"result":{"format":"ttml","content":"…"}}
→ {"id":9,"method":"artwork","params":{"query":{…}}}         ← {"id":9,"result":{"url":"…"}}
→ {"method":"event","params":{"name":"track.start","payload":{…}}}   (no id = notification)
→ {"method":"settings","params":{…}}                                 (settings changed)
← {"method":"log","params":{"message":"…"}}
← {"id":10,"error":"not found"}                                      (no result)
```

## Player plugins (xtm)

With `"player": true` (and type `xtm`), the plugin can replace mpv
(**Settings → Playback → Player backend**). It must answer:

```
→ player.load   {"url":"…","headers":{…}}
→ player.pause  {"paused":true}
→ player.stop
→ player.volume {"volume":0..100}
→ player.seek   {"positionMs":…}
→ player.state                       ← {"positionMs":…,"durationMs":…,"paused":false,"idle":false,"volume":80,"buffering":false}
← {"method":"player.end","params":{"reason":"eof"}}     when a track finishes
← {"method":"player.error","params":{"message":"…"}}
```

With the SDK: set `Player: &sdk.Player{Load: …, Pause: …, Stop: …, Volume: …,
Seek: …, State: …}` and call `ctx.Notify("player.end", map[string]string{"reason": "eof"})`.
A VLC player plugin, for example, can drive `vlc --intf rc` or VLC's HTTP API.
