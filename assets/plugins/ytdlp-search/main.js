// A script plugin: plain JavaScript with host Go modules via require().
// "ytdlp" is declared in plugin.json "modules", so it can be required here.
const ytdlp = require("ytdlp");

exports.search = function (query) {
  if (!ytdlp.available()) throw new Error("yt-dlp não está instalado");
  const tracks = ytdlp.search(query, Number(settings.limit) || 20, settings.site);
  // Each track already has id/title/artist/durationMs/cover/streamUrl.
  // streamUrl is the page URL: meofishes plays it through mpv + yt-dlp.
  return { tracks: tracks };
};

// resolve() is optional. Returning { page: true } tells the player the URL
// is a web page (mpv extracts the audio itself, which is faster to start).
exports.resolve = function (track) {
  return { url: track.streamUrl, page: true };
};
