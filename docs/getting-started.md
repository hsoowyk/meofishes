# Getting started

## Install

Needs: **mpv** (plays audio), **Qt 5** libraries, and optionally **yt-dlp**
(YouTube/SoundCloud). On Void Linux: `xbps-install mpv qt5 yt-dlp`.

With a built `meofishes` binary next to the repository:

```sh
./install.sh            # /opt/meofishes + applications menu (asks for sudo)
./install.sh --user     # ~/.local/opt/meofishes, only for you
./install.sh --uninstall [--user]
```

The installer copies the binary, adds `meofishes.desktop` and the icon to the
applications menu, links `meofishes` into your PATH and runs `meofishes setup`,
which installs the base themes and plugins (they are embedded in the binary).

## First run

meofishes installs the **Fishalike** theme and the default plugins (Meo
Lyrics, Meo Artwork, Local files, yt-dlp, Radio Browser, Deezer) and offers a
**short tutorial** — you can skip it and replay it later in
**Settings → About → Tutorial**.

## The window

```
┌─[logo]─[menu: Home Search Library Queue Settings]──────────── ─ □ ✕ ┐
│ ┌ Left panel ──────┐ ┌ Content ─────────────────────────────────────┐ │
│ │ Your library     │ │ Good evening, name          [ search…     ] │ │
│ │  ♥ Liked songs   │ │ Recently played  ▢ ▢ ▢ ▢ ▢ →                 │ │
│ │  ▢ playlists…    │ │ Recommended      ▢ ▢ ▢ ▢ ▢ →                 │ │
│ │                  │ │ Popular near you ▢ ▢ ▢ ▢ ▢ →                 │ │
│ │ current lyric ↑  │ │                                              │ │
│ │ ◯ title / artist │ │                                              │ │
│ │ ⏮ ⏯ ⏭  ♪ ≡ 🔊    │ │                                              │ │
│ └──────────────────┘ └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

- **Logo**: click to open the menu with every tab, double-click to go home,
  the arrow hides the menu. Its color follows the theme (dark on light themes).
- **Left panel**: your library (liked songs and playlists), the line being
  sung (it swipes to the next one), and the player card.
- **Fullscreen player** (click the cover or press Ctrl+L): blurred cover
  background, a big cover that tilts toward the mouse, the controls and the
  lyrics — or a frequency visualizer when the song has no lyrics.
- **Settings** (in the menu) also contain **Themes** and **Plugins**.

Right-click any song for: play, play next, add to queue, start radio, like,
add to playlist, move, remove, copy title (and inspect, in developer mode).
The queue and local playlists can be reordered by dragging.

## Shortcuts

| keys | action |
|---|---|
| Ctrl+Space | play / pause |
| Ctrl+→ / Ctrl+← | next / previous |
| Shift+→ / Shift+← | seek (step in Settings → Playback) |
| Ctrl+↑ / Ctrl+↓ | volume |
| Ctrl+L | fullscreen player and lyrics |
| Ctrl+D | like the current song |
| Ctrl+F | search |
| Ctrl+H, Ctrl+1…4 | home, search, library, queue |
| Ctrl+, | settings |
| Esc | close the fullscreen player |
| F11 | fullscreen window |
| F5 | reload the theme |
| Ctrl+Shift+D | developer mode |
| F12 | inspect the widget under the mouse (developer mode) |
| Ctrl+Q | quit |
