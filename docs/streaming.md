# Streaming, cache and the visualizer

## Chunked downloads

Songs with a known length that play over HTTP go through a local proxy
(`127.0.0.1`) that downloads them **in parts, several at a time** (range
requests), and hands them to the player:

- the player starts as soon as the first part arrives;
- with **Download the whole song in the background**, the rest keeps coming,
  so seeking is instant;
- servers that ignore ranges (live radios) are passed straight through.

YouTube/SoundCloud pages are first resolved with **yt-dlp** into the audio URL
(Settings → Streaming → Resolve web pages with yt-dlp), so they can be chunked
too; if that fails, mpv opens the page itself.

The first part is small (128 KB), so playback starts as soon as it arrives;
the following parts are bigger and come in order from where the player is
reading. Changing songs stops the previous one at once.

## Quality

Settings → Playback → **Maximum quality**: Max (lossless when the source has
it), High (~192 kbps), Normal (~128 kbps) or Data saver (~64 kbps). It picks
the yt-dlp format for web pages, and plugins receive it as `app.quality`
(scripts, declarative `{{app.quality}}`) or `quality` in `initialize`
(process) to pick their own streams.

## Prefetch

A few seconds after a song starts, meofishes resolves the **next** song and
downloads its first parts. When it comes up, it starts almost immediately.

## Settings

Settings → Streaming: parallel parts on/off, part size (KB), parts at the
same time, whole-song download, prefetch, yt-dlp resolution, mpv cache size
and read-ahead. Developer mode → **Stream** shows the proxy: parts done,
speed, what is prefetched.

## Visualizer

When a song has no lyrics, the fullscreen player shows a line drawn from the
song's **frequencies**, updated every 0.1 s. A second, silent mpv decodes the
song (through the proxy, so nothing is downloaded twice) to mono 11 kHz PCM;
every 100 ms becomes one frame of log-spaced bands (40 Hz – 5 kHz). Style it
with `#FullVisualizer` ([properties](themes/properties.md#visualizer--fullvisualizer)).

## Songs on disk (offline)

- **Keep played songs** (on by default): a song downloaded in full by the
  proxy and played to the end (or skipped after it finished downloading) is
  saved in `~/.cache/meofishes/songs`. Next time it plays from disk —
  instantly, even offline. The cache has a size limit (Settings → Storage →
  Song cache size); the least recently played songs go first.
- **Download**: right-click a song → Download, or **Download all** on a
  playlist. Downloads go to `~/Music/meofishes` (or the folder in Settings →
  Storage) and stay until removed. **Library → Downloaded songs** lists them.

Songs on disk are matched by track (plugin + id), so they are used whatever
plugin would resolve them. The index is `~/.local/share/meofishes/offline.json`.

## Video

The video button in the fullscreen player (or Settings → Playback → Show
video automatically) plays the song's video — YouTube pages and local video
files — in the right side of the fullscreen player, at the quality chosen in
Settings → Playback → Video quality. Turning it on reloads the song at the same
position, with mpv opening the page itself (no chunked download while video is
on).

mpv draws the video inside the window through a native child window
(`--wid`). That only works on X11, so on Wayland meofishes runs through
XWayland (Settings → Playback → Run on X11 for video, on by default; set
`QT_QPA_PLATFORM` yourself to override).

## YouTube asks to sign in

Sometimes YouTube answers "Sign in to confirm you're not a bot" for every
song from your connection. meofishes then tells you so instead of waiting.
Choose your browser in **Settings → Playback → YouTube account** (or give a
`cookies.txt`): yt-dlp and mpv then use the YouTube account you are signed in
to in that browser, the way yt-dlp documents (`--cookies-from-browser`).
