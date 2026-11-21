# Developer mode

Turn it on in **Settings → Developer** or with **Ctrl+Shift+D**. It adds the
**Dev** tab to the logo menu, the **F12** inspector, and extra menu entries
(right-click a song → Inspect (JSON)).

## The Dev tab

Updates every second while visible (Pause freezes it, Copy tab copies it).

| tab | shows |
|---|---|
| Overview | version, Qt/Go, the playing track and position, player, queue, page, theme, language, frames per second and screen refresh rate, running tweens/tickers, animation speed, Go memory/GC/goroutines, request count, cover cache, every folder |
| Log | the app log (plugins, player, prefetch, theme, stream…), filterable |
| Network | every HTTP request (time, method, status, duration, size, URL, errors), newest first, filterable |
| Plugins | for each plugin: types, runtime, capabilities, capabilities denied by its types, folder, Go modules, hosts, dependencies, load error, **calls per capability** (count, errors, average and last duration, last error), settings (passwords hidden); priority orders; event names |
| Lyrics | every provider attempt of the last search, the parsed lyrics with line timings (▶ marks the current line) and the word timings of the current line |
| Stream | the stream playing (plugin, player URL, source URL, headers), the chunked-download proxy (parts done, bytes, speed, errors), prefetched songs, the visualizer's progress |
| Player | live mpv properties: codec, bitrate, cache, device, filters, versions… |
| Theme | theme chain, variables, every `-meo-*` property by selector, the Settings layer as CSS, custom easings, the compiled QSS |
| Queue | the queue with sources, ids and covers |
| Config | `config.json` as JSON |
| Translations | coverage of each language and strings missing in the current one |

## F12 inspector

Shows the widget under the mouse and its parents: class, `#objectName`,
`[role]`, size and position, and the `-meo-*` properties that apply to it.

## FPS meter

Settings → Developer → Show FPS: frames per second of the animation engine,
tweens and tickers running (0 fps when nothing moves — the app is idle).
