const lavalink = require("lavalink");

function client() {
  return lavalink.connect({
    host: settings.host, port: settings.port,
    password: settings.password, secure: settings.secure === "true",
  });
}

exports.search = function (query) {
  // URLs are loaded directly, text goes through the search prefix
  const isUrl = /^https?:\/\//.test(query);
  const c = client();
  const tracks = isUrl ? c.loadTracks(query).tracks : c.search(query, settings.prefix);
  return { tracks: tracks };
};

// Lavalink streams to Discord only; for local playback we hand the track's
// page URL to mpv (which uses yt-dlp). Spotify/Deezer/Apple tracks from LavaSrc
// have no audio of their own, so we search them on YouTube Music by ISRC/title.
exports.resolve = function (track) {
  const src = track.extra && track.extra.sourceName;
  if (src === "spotify" || src === "deezer" || src === "applemusic") {
    const c = client();
    const q = track.isrc ? "ytmsearch:\"" + track.isrc + "\"" : "ytmsearch:" + track.artist + " " + track.title;
    const found = c.loadTracks(q).tracks;
    if (found.length > 0) return { url: found[0].streamUrl, page: true };
  }
  return { url: track.streamUrl, page: true };
};
