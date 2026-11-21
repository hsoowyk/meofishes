// Package together implements Metrolist's "Listen Together" protocol
// (github.com/MetrolistGroup/metroserver + metroproto): a WebSocket of
// protobuf Envelopes. meofishes can host or join rooms with Metrolist,
// SimpMusic and other meofishes users. The messages are encoded by hand with
// protowire (field numbers from listentogether.proto).
package together

import (
	"bytes"
	"compress/gzip"
	"io"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// ---------------------------------------------------------------- encoding

type enc struct{ b []byte }

func (e *enc) str(n protowire.Number, s string) {
	if s != "" {
		e.b = protowire.AppendTag(e.b, n, protowire.BytesType)
		e.b = protowire.AppendString(e.b, s)
	}
}
func (e *enc) bytes(n protowire.Number, v []byte) {
	if len(v) > 0 {
		e.b = protowire.AppendTag(e.b, n, protowire.BytesType)
		e.b = protowire.AppendBytes(e.b, v)
	}
}
func (e *enc) varint(n protowire.Number, v uint64) {
	if v != 0 {
		e.b = protowire.AppendTag(e.b, n, protowire.VarintType)
		e.b = protowire.AppendVarint(e.b, v)
	}
}
func (e *enc) int64(n protowire.Number, v int64) { e.varint(n, uint64(v)) }
func (e *enc) bool(n protowire.Number, v bool) {
	if v {
		e.varint(n, 1)
	}
}
func (e *enc) float(n protowire.Number, v float32) {
	if v != 0 {
		e.b = protowire.AppendTag(e.b, n, protowire.Fixed32Type)
		e.b = protowire.AppendFixed32(e.b, math.Float32bits(v))
	}
}

// ---------------------------------------------------------------- decoding

type field struct {
	typ protowire.Type
	u   uint64
	b   []byte
}

type msg map[protowire.Number][]field

func parse(b []byte) (msg, error) {
	m := msg{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		f := field{typ: typ}
		switch typ {
		case protowire.VarintType:
			f.u, n = protowire.ConsumeVarint(b)
		case protowire.Fixed32Type:
			var v uint32
			v, n = protowire.ConsumeFixed32(b)
			f.u = uint64(v)
		case protowire.Fixed64Type:
			f.u, n = protowire.ConsumeFixed64(b)
		case protowire.BytesType:
			f.b, n = protowire.ConsumeBytes(b)
		default:
			n = protowire.ConsumeFieldValue(num, typ, b)
		}
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		m[num] = append(m[num], f)
	}
	return m, nil
}

func (m msg) str(n protowire.Number) string {
	if fs := m[n]; len(fs) > 0 {
		return string(fs[len(fs)-1].b)
	}
	return ""
}
func (m msg) raw(n protowire.Number) []byte {
	if fs := m[n]; len(fs) > 0 {
		return fs[len(fs)-1].b
	}
	return nil
}
func (m msg) u64(n protowire.Number) uint64 {
	if fs := m[n]; len(fs) > 0 {
		return fs[len(fs)-1].u
	}
	return 0
}
func (m msg) i64(n protowire.Number) int64 { return int64(m.u64(n)) }
func (m msg) bool(n protowire.Number) bool { return m.u64(n) != 0 }
func (m msg) float(n protowire.Number) float32 {
	if fs := m[n]; len(fs) > 0 && fs[len(fs)-1].typ == protowire.Fixed32Type {
		return math.Float32frombits(uint32(fs[len(fs)-1].u))
	}
	return 0
}
func (m msg) all(n protowire.Number) [][]byte {
	var out [][]byte
	for _, f := range m[n] {
		out = append(out, f.b)
	}
	return out
}

// ---------------------------------------------------------------- messages

// Envelope wraps every frame.
type Envelope struct {
	Type       string
	Payload    []byte
	Compressed bool
	RequestID  uint64
}

func (e Envelope) marshal() []byte {
	var w enc
	w.str(1, e.Type)
	w.bytes(2, e.Payload)
	w.bool(3, e.Compressed)
	w.varint(4, e.RequestID)
	return w.b
}

func unmarshalEnvelope(b []byte) (Envelope, error) {
	m, err := parse(b)
	if err != nil {
		return Envelope{}, err
	}
	e := Envelope{Type: m.str(1), Payload: m.raw(2), Compressed: m.bool(3), RequestID: m.u64(4)}
	if e.Compressed && len(e.Payload) > 0 {
		zr, err := gzip.NewReader(bytes.NewReader(e.Payload))
		if err != nil {
			return e, err
		}
		data, err := io.ReadAll(io.LimitReader(zr, 2<<20))
		if err != nil {
			return e, err
		}
		e.Payload, e.Compressed = data, false
	}
	return e, nil
}

// Track is TrackInfo. Metrolist uses the YouTube video id as Id.
type Track struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album,omitempty"`
	DurationMs  int64  `json:"durationMs,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	SuggestedBy string `json:"suggestedBy,omitempty"`
}

func (t Track) marshal() []byte {
	var w enc
	w.str(1, t.ID)
	w.str(2, t.Title)
	w.str(3, t.Artist)
	w.str(4, t.Album)
	w.int64(5, t.DurationMs)
	w.str(6, t.Thumbnail)
	w.str(7, t.SuggestedBy)
	return w.b
}

func parseTrack(b []byte) Track {
	m, _ := parse(b)
	return Track{ID: m.str(1), Title: m.str(2), Artist: m.str(3), Album: m.str(4), DurationMs: m.i64(5), Thumbnail: m.str(6), SuggestedBy: m.str(7)}
}

func parseTracks(bs [][]byte) []Track {
	out := make([]Track, 0, len(bs))
	for _, b := range bs {
		out = append(out, parseTrack(b))
	}
	return out
}

// User is a room member.
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      bool   `json:"host"`
	Connected bool   `json:"connected"`
}

// State is the room's playback state (RoomState / SyncStatePayload).
type State struct {
	RoomCode   string  `json:"roomCode,omitempty"`
	HostID     string  `json:"hostId,omitempty"`
	Users      []User  `json:"users,omitempty"`
	Track      *Track  `json:"track,omitempty"`
	Playing    bool    `json:"playing"`
	PositionMs int64   `json:"positionMs"`
	LastUpdate int64   `json:"lastUpdate"` // server unix ms
	Volume     float32 `json:"volume"`
	Queue      []Track `json:"queue,omitempty"`
	Revision   uint64  `json:"revision"`
	QueueTitle string  `json:"queueTitle,omitempty"`
}

func parseRoomState(b []byte) State {
	m, _ := parse(b)
	s := State{RoomCode: m.str(1), HostID: m.str(2), Playing: m.bool(5), PositionMs: m.i64(6),
		LastUpdate: m.i64(7), Volume: m.float(8), Queue: parseTracks(m.all(9)), Revision: m.u64(10), QueueTitle: m.str(11)}
	for _, ub := range m.all(3) {
		um, _ := parse(ub)
		s.Users = append(s.Users, User{ID: um.str(1), Name: um.str(2), Host: um.bool(3), Connected: um.bool(4)})
	}
	if tb := m.raw(4); tb != nil {
		t := parseTrack(tb)
		s.Track = &t
	}
	return s
}

func parseSyncState(b []byte) State {
	m, _ := parse(b)
	s := State{Playing: m.bool(2), PositionMs: m.i64(3), LastUpdate: m.i64(4), Queue: parseTracks(m.all(5)),
		Volume: m.float(6), Revision: m.u64(7), QueueTitle: m.str(8)}
	if tb := m.raw(1); tb != nil {
		t := parseTrack(tb)
		s.Track = &t
	}
	return s
}

// Action is PlaybackActionPayload (host → server, and sync_playback back).
type Action struct {
	Action       string  `json:"action"` // play pause seek change_track queue_add queue_remove queue_clear sync_queue set_volume update_track
	TrackID      string  `json:"trackId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Track        *Track  `json:"track,omitempty"`
	InsertNext   bool    `json:"insertNext,omitempty"`
	Queue        []Track `json:"queue,omitempty"`
	QueueTitle   string  `json:"queueTitle,omitempty"`
	Volume       float32 `json:"volume,omitempty"`
	ServerTime   int64   `json:"serverTime,omitempty"`
	Revision     uint64  `json:"revision,omitempty"`
	CapturedAt   int64   `json:"capturedAt,omitempty"` // server time the position was captured
	ReplaceQueue bool    `json:"replaceQueue,omitempty"`
	OriginUserID string  `json:"originUserId,omitempty"`
	HostSequence uint64  `json:"hostSequence,omitempty"`
}

func (a Action) marshal() []byte {
	var w enc
	w.str(1, a.Action)
	w.str(2, a.TrackID)
	w.int64(3, a.PositionMs)
	if a.Track != nil {
		w.bytes(4, a.Track.marshal())
	}
	w.bool(5, a.InsertNext)
	for _, t := range a.Queue {
		w.bytes(6, t.marshal())
	}
	w.str(7, a.QueueTitle)
	w.float(8, a.Volume)
	w.int64(9, a.ServerTime)
	w.varint(10, a.Revision)
	w.int64(11, a.CapturedAt)
	w.bool(12, a.ReplaceQueue)
	w.str(13, a.OriginUserID)
	w.varint(14, a.HostSequence)
	return w.b
}

func parseAction(b []byte) Action {
	m, _ := parse(b)
	a := Action{Action: m.str(1), TrackID: m.str(2), PositionMs: m.i64(3), InsertNext: m.bool(5), Queue: parseTracks(m.all(6)),
		QueueTitle: m.str(7), Volume: m.float(8), ServerTime: m.i64(9), Revision: m.u64(10), CapturedAt: m.i64(11),
		ReplaceQueue: m.bool(12), OriginUserID: m.str(13), HostSequence: m.u64(14)}
	if tb := m.raw(4); tb != nil {
		t := parseTrack(tb)
		a.Track = &t
	}
	return a
}
