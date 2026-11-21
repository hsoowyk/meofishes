# Settings

Open them from the logo menu (Ctrl+,). Categories:

| category | what you can change |
|---|---|
| Appearance | theme, every color variable of the theme, interface and lyrics fonts, text size, corner radius, left panel width, card size, covers in lists, start page, cover-colored background, breathing light, blurred cover in the fullscreen player, your name (greeting) |
| Themes | install (zip, folder, URL, catalog), apply, create your own, user CSS, live reload |
| Animations | global speed (animations always run at your screen's refresh rate), reduce motion, page transition (type, duration, easing), list cascade, current-lyric swipe (direction, duration, easing), fullscreen player opening, cover tilt (on/off, angle, smoothness) |
| Lyrics | size, weight, alignment, spacing, fade, colors, background vocals, credits, interlude dots, word animation, line animation, glow, lift, line sizes, scroll; sync offset; Meo Lyrics providers |
| Playback & audio | video (quality, automatic, X11), autoplay, stop after the current song, remember the queue, resume on start, seek step, ReplayGain, loudness normalization, output device, exclusive mode, extra mpv options, player backend (mpv or an xtm plugin) |
| Streaming | parallel parts, part size, parts at once, whole-song download, prefetch, yt-dlp resolution, cache size, read-ahead |
| Library & privacy | find missing covers, prefer album art over video thumbnails, import/export, listening history (keep, clear) |
| Plugins | install, enable, configure, build, remove, catalog |
| Plugin priority | order per capability (lyrics, covers, match, recommendations, search), icon pack, extra catalogs |
| Language | English, Português (Brasil), Français, Türkçe, Deutsch, Español |
| Shortcuts | the list of keyboard shortcuts |
| Storage | songs on disk (keep played songs, cache size, downloads folder), cover/lyrics caches, found covers, folders, reset all settings |
| Developer | developer mode, FPS meter, network recording, live reload, documentation |
| About | version, tutorial, project page, mpv / yt-dlp paths, the contact sent to lyrics services |

## Where they are stored

`~/.config/meofishes/config.json`:

| key | |
|---|---|
| `theme`, `userCss`, `liveReload` | theme selection |
| `style` | the **Settings layer**: `"selector\|property": "value"` pairs compiled into CSS between the theme and the user CSS, e.g. `"#FullLyrics\|-meo-font-size": "40px"`, `":root\|--red": "#e06070"`. The ↻ button next to a setting removes its key. |
| `prefs` | behaviour settings (animation speed, lyrics offset, streaming, audio, privacy, developer…) |
| `plugins` | `disabled`, `settings` per plugin, `order` per capability, `iconPack`, `player` |
| `queue`, `queueIndex`, `queuePosMs` | the remembered queue |
| `language`, `volume`, `shuffle`, `repeat`, window size | |
