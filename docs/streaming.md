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
