# Listen together

meofishes speaks the **Listen Together** protocol of Metrolist
([metroserver](https://github.com/MetrolistGroup/metroserver)), so you can
listen in sync with other meofishes users **and** with people on Metrolist or
SimpMusic (Android).

- **Create a room** (logo menu → Listen together) and share the 8-letter code
  or the invite link (`https://metrolist.cc/listen?code=…`).
- **Join** with a code or a link. The host lets you in.
- The **host** controls the music: every song change, play, pause and seek is
  sent to the room; positions are stamped with the server clock so everyone
  hears the same second. The host sees who is in the room, lets people in,
  can give control to someone else or remove people, and accepts or refuses
  suggestions.
- **Listeners** follow the host. Their play/next/seek buttons are locked;
  right-click a song → **Suggest to the room** instead.
- If the connection drops, meofishes reconnects to the same room.

Songs are identified by their **YouTube video id** (what Metrolist uses), so
only songs that come from YouTube (or that meofishes found on YouTube) can be
shared. Listeners play them through the yt-dlp plugin.

The default server is Metrolist's community server
(`wss://metroserverx.meowery.eu/ws`); you can set another one on the Listen
together page, including your own metroserver. meofishes identifies itself
honestly (its own User-Agent).

Protocol notes for developers: `internal/together` — WebSocket of protobuf
`Envelope`s (type, payload, compressed, request_id), hand-encoded with
`protowire` from `listentogether.proto`; `client_capabilities` first, then
`create_room` / `join_room`, host `playback_action`s, `sync_playback` /
`sync_state` to listeners, `ping`/`pong` for the clock offset.
