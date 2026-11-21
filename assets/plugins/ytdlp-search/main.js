// A script plugin (types fnt + src): plain JavaScript with host Go modules.
// "ytdlp" and "lyrics" are declared in plugin.json "modules".
const ytdlp = require("ytdlp");
const lyrics = require("lyrics");

function need() {
  if (!ytdlp.available()) throw new Error("yt-dlp is not installed");
}

// src: search
exports.search = function (query) {
  need();
  const tracks = ytdlp.search(query, Number(settings.limit) || 20, settings.site);
  // each track has id/title/artist/durationMs/cover/streamUrl (the page URL)
  return { tracks: tracks };
};

// fnt: resolve. Our own tracks already have the page URL. Tracks of other
// plugins ("match": true in plugin.json) are found by title + artist.
exports.resolve = function (track) {
  if (track.source === app.plugin && track.streamUrl) {
    return { url: track.streamUrl, page: true };
  }
  need();
  const q = (track.artist ? track.artist + " - " : "") + track.title;
  const found = ytdlp.search(q, 5, settings.matchSite || "youtube");
  let best = null, bestScore = 0;
  for (const c of found) {
    const s = lyrics.matchScore(track, c.title, c.artist, c.durationMs || 0);
    if (s > bestScore) { best = c; bestScore = s; }
  }
  if (!best || bestScore < (Number(settings.minScore) || 0.45)) {
    throw new Error("not found on " + (settings.matchSite || "youtube") + ": " + q);
  }
  return { url: best.streamUrl, page: true };
};

// src: recommend — the YouTube "mix" of a video
exports.recommend = function (track) {
  need();
  const id = track.videoId || (track.source === app.plugin ? track.id : "");
  if (!id) return { tracks: [] };
  const list = ytdlp.playlist("https://www.youtube.com/watch?v=" + id + "&list=RD" + id, 25);
  return { tracks: list.filter(function (t) { return t.id && t.id !== id; }) };
};
