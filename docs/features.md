# Features at a glance

## Playback

- **Instant start**: parallel chunked downloads, small first part, the next
  song prepared ahead of time ([streaming](streaming.md)).
- **Maximum quality**: Max (lossless when available), High, Normal, Data saver.
- **Equalizer**: 10 bands (31 Hz – 16 kHz, ±12 dB) with presets (Flat, Bass,
  Treble, Vocal, Rock, Pop, Electronic, Acoustic, Loudness) or your own curve;
  a pre-amp keeps boosts from clipping. Applied live.
- **Speed** 0.5× – 2× (pitch kept), also `meofishes speed 1.25`.
- **Loudness**: ReplayGain and dynamic normalization.
- **Sleep timer**: 5–90 minutes or the end of the song; the volume fades out
  for the last 15 s, then playback pauses. In the fullscreen player bar, the
  tray menu and `meofishes sleep 30 | end | off`.
- **Autoplay / radio**: similar songs when the queue ends; "Start radio" on
  any song.
- **Video** of songs that have one, inside the fullscreen player.
- **Offline**: played songs are kept (with a size limit), any song can be
  downloaded.

## Library

- Liked songs, local playlists, plugin playlists, `.mepw` import/export,
  `.m3u` import.
- **Import from link**: a YouTube / YouTube Music / SoundCloud / Bandcamp
  playlist or album becomes a local playlist (needs yt-dlp).
- **Stats**: listening time (30 days and total), top songs and top artists;
  "Your most played" on the home page. Only real listening counts (a play
  after 30 s or half the song). Kept on your computer; cleared with the
  history.
- **Recent searches** under the search box.

## Desktop integration

- **MPRIS**: media keys, the panel's media widget (KDE/GNOME), the lock
  screen and `playerctl` show and control meofishes (cover, title, position,
  volume, shuffle, repeat, speed).
- **Notifications** when a song starts (optional, only when the window is not
  in front).
- **Tray icon** and **background playback**.
- **Terminal control**: `meofishes now | next | lyrics | sleep …`
  ([cli](cli.md)).
- **Single instance**: opening meofishes again brings the window back.

## Look

- Themes: Fishalike (default), Abyss, Sakura Paper, Ink (OLED black), Moss,
  Daylight — and everything adjustable in Settings or CSS.
- Fullscreen player: tilting cover with title on hover, "potion" background,
  lyrics panel, visualizer.
- Six languages: English, Português, Français, Türkçe, Deutsch, Español.

## Platforms

Linux (x86_64) is the main platform. The code also builds for macOS and
Windows: process handling, mpv's IPC (a named pipe on Windows), paths and the
updater adapt to each system. Those builds are **experimental**: the CI makes
them, but they are not tested by hand yet. On Windows the frequency
visualizer is off and the binary needs the Qt 5 DLLs next to it.
