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
- **Skip parts without music** (SponsorBlock): talking, intros, sponsors and
  "subscribe" reminders in YouTube videos are skipped, from the community
  database at sponsor.ajay.app. Categories in Settings → Playback.

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
- **Search by lyrics**: pick "By a line of the lyrics" next to the search box
  and type a line you remember (Genius' lyric search; the songs play from
  YouTube).
- **Your own server**: the bundled Subsonic plugin plays from Navidrome,
  Airsonic, Gonic, Ampache, LMS or Jellyfin (with its Subsonic plugin):
  search, a random mix on the home page, playlists and newest albums,
  radio, lyrics, covers, and plays reported back to the server.

## Desktop integration

- **MPRIS**: media keys, the panel's media widget (KDE/GNOME), the lock
  screen and `playerctl` show and control meofishes (cover, title, position,
  volume, shuffle, repeat, speed).
- **Notifications** when a song starts (optional, only when the window is not
  in front).
- **Tray icon** and **background playback**.
- **Terminal control**: `meofishes now | next | lyrics | sleep …`
  ([cli](cli.md)).
- **Discord**: "Listening to …" on your profile (Settings → Playback; needs
  the Application ID of an application you create at
  discord.com/developers — Discord shows its name and you control it).
- **Single instance**: opening meofishes again brings the window back.

## Listen together

Rooms compatible with Metrolist and SimpMusic: the host controls the music,
listeners follow in sync and suggest songs ([details](listen-together.md)).

## Look

- Themes: Fishalike (default), Abyss, Sakura Paper, Ink (OLED black), Moss,
  Daylight — and everything adjustable in Settings or CSS.
- Fullscreen player: tilting cover with title on hover, "potion" background,
  lyrics panel, visualizer.
- **Latin letters in lyrics**: Japanese kana, Korean hangul and Cyrillic
  romanized under each line or instead of it (Settings → Lyrics).
- Six languages: English, Português, Français, Türkçe, Deutsch, Español.

## Platform

Linux (x86_64). The code keeps the system-specific parts separate
(`internal/sysproc`), so other systems could be added later.
