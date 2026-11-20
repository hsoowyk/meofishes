const http = require("http");
let started = null;

function submit(type, track, listenedAt) {
  if (!settings.token) return;
  const payload = { track_metadata: { artist_name: track.artist, track_name: track.title } };
  if (track.album) payload.track_metadata.release_name = track.album;
  payload.track_metadata.additional_info = { media_player: "meofishes", submission_client: "meofishes", submission_client_version: app.version };
  if (track.durationMs) payload.track_metadata.additional_info.duration_ms = track.durationMs;
  if (listenedAt) payload.listened_at = listenedAt;
  const r = http.post("https://api.listenbrainz.org/1/submit-listens",
    { listen_type: type, payload: [payload] },
    { headers: { Authorization: "Token " + settings.token } });
  if (!r.ok) console.warn("ListenBrainz", r.status, r.body);
}

exports.onEvent = function (name, data) {
  if (name === "track.start") {
    started = { track: data.track, at: Math.floor(Date.now() / 1000) };
    submit("playing_now", data.track);
  } else if (name === "track.end" && started) {
    // ListenBrainz rule: listened for 4 minutes or half of the track
    const half = (started.track.durationMs || 0) / 2;
    if (data.positionMs >= Math.min(240000, half || 240000)) submit("single", started.track, started.at);
    started = null;
  }
};
