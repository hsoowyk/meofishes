// Subsonic API client (https://www.subsonic.org/pages/api.jsp), v1.16.1 +
// OpenSubsonic extensions when the server has them.
const http = require("http");
const crypto = require("crypto");

function base() {
  let s = (settings.server || "").trim().replace(/\/+$/, "");
  if (s && !/^https?:\/\//.test(s)) s = "https://" + s;
  return s;
}

function auth() {
  const salt = crypto.uuid().replace(/-/g, "").slice(0, 12);
  return {
    u: settings.user || "", t: crypto.md5((settings.password || "") + salt), s: salt,
    v: "1.16.1", c: "meofishes", f: "json",
  };
}

function url(method, params) {
  const q = Object.assign(auth(), params || {});
  return base() + "/rest/" + method + "?" + http.encode(q);
}

function call(method, params) {
  if (!base()) throw new Error("Set the server address in the plugin settings");
  let r;
  try {
    r = http.get(url(method, params), { timeout: 30000 });
  } catch (e) { // the message has the URL with the login token: keep it out of logs
    const m = String(e.message || e);
    throw new Error("Subsonic: " + (/deadline|timeout/i.test(m) ? "the server did not answer" : "could not reach " + base()));
  }
  const root = r.json && r.json["subsonic-response"];
  if (!r.ok || !root) throw new Error("Subsonic: HTTP " + r.status);
  if (root.status !== "ok") throw new Error("Subsonic: " + ((root.error && root.error.message) || "error"));
  return root;
}

function cover(id) {
  return id ? url("getCoverArt", { id: id, size: 600 }) : "";
}

function track(s) {
  return {
    id: s.id, title: s.title, artist: s.artist || (s.artists && s.artists.map((a) => a.name).join(", ")) || "",
    album: s.album || "", cover: cover(s.coverArt || s.albumId), durationMs: (s.duration || 0) * 1000,
    extra: { artistId: s.artistId || "", albumId: s.albumId || "", suffix: s.suffix || "", bitRate: String(s.bitRate || "") },
  };
}

function lim() { return Math.max(1, Math.min(500, parseInt(settings.limit, 10) || 40)); }

exports.search = function (query) {
  const r = call("search3", { query: query, songCount: lim(), albumCount: 0, artistCount: 0 });
  return { tracks: ((r.searchResult3 && r.searchResult3.song) || []).map(track) };
};

exports.home = function () {
  // a mix: random songs from the library
  const r = call("getRandomSongs", { size: lim() });
  return { title: "Your server", tracks: ((r.randomSongs && r.randomSongs.song) || []).map(track) };
};

exports.recommend = function (t) {
  const id = (t.extra && t.extra.artistId) || "";
  let r;
  try {
    r = call("getSimilarSongs2", { id: id || t.id, count: 25 });
    const songs = (r.similarSongs2 && r.similarSongs2.song) || [];
    if (songs.length) return { tracks: songs.map(track) };
  } catch (e) { /* server without similar songs (no Last.fm key): fall back */ }
  r = call("getRandomSongs", { size: 25 });
  return { tracks: ((r.randomSongs && r.randomSongs.song) || []).map(track) };
};

exports.playlists = function () {
  const out = [];
  const r = call("getPlaylists");
  for (const p of (r.playlists && r.playlists.playlist) || [])
    out.push({ id: "pl:" + p.id, title: p.name, cover: cover(p.coverArt), count: p.songCount });
  const a = call("getAlbumList2", { type: "newest", size: 30 });
  for (const al of (a.albumList2 && a.albumList2.album) || [])
    out.push({ id: "al:" + al.id, title: al.name + " — " + (al.artist || ""), cover: cover(al.coverArt || al.id), count: al.songCount });
  return out;
};

exports.open = function (c) {
  const [kind, id] = [c.id.slice(0, 3), c.id.slice(3)];
  if (kind === "pl:") {
    const r = call("getPlaylist", { id: id });
    return Object.assign({}, c, { tracks: ((r.playlist && r.playlist.entry) || []).map(track) });
  }
  const r = call("getAlbum", { id: id });
  return Object.assign({}, c, { tracks: ((r.album && r.album.song) || []).map(track) });
};

exports.resolve = function (t) {
  const p = { id: t.id };
  if (settings.format && settings.format !== "raw") p.format = settings.format;
  const br = parseInt(settings.bitrate, 10) || 0;
  if (br > 0) p.maxBitRate = br;
  return url("stream", p);
};

exports.artwork = function (q) {
  return q.track && q.track.source === "subsonic" ? cover(q.track.id) : "";
};

exports.lyrics = function (q) {
  if (!q.track || q.track.source !== "subsonic") return null;
  try { // OpenSubsonic: synced lyrics
    const r = call("getLyricsBySongId", { id: q.track.id });
    const sl = r.lyricsList && r.lyricsList.structuredLyrics && r.lyricsList.structuredLyrics[0];
    if (sl && sl.line && sl.line.length) {
      if (sl.synced) {
        const lrc = sl.line.map((l) => {
          const ms = (l.start || 0) + (sl.offset || 0);
          const m = Math.floor(ms / 60000), s = ((ms % 60000) / 1000).toFixed(2).padStart(5, "0");
          return "[" + String(m).padStart(2, "0") + ":" + s + "]" + l.value;
        }).join("\n");
        return { format: "lrc", content: lrc };
      }
      return { format: "plain", content: sl.line.map((l) => l.value).join("\n") };
    }
  } catch (e) { /* not OpenSubsonic */ }
  const r = call("getLyrics", { artist: q.artist || "", title: q.title || "" });
  const v = r.lyrics && r.lyrics.value;
  return v ? { format: "plain", content: v } : null;
};

exports.onEvent = function (name, data) {
  if (String(settings.scrobble) === "false" || !data.track || data.track.source !== "subsonic") return;
  try {
    if (name === "track.start") call("scrobble", { id: data.track.id, submission: false });
    else if (name === "track.end" && data.positionMs >= Math.min(240000, (data.track.durationMs || 0) / 2))
      call("scrobble", { id: data.track.id, submission: true, time: Date.now() });
  } catch (e) { console.warn("subsonic scrobble", e.message); }
};
