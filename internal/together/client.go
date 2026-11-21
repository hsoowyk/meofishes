package together

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultServer is the community server Metrolist uses.
const DefaultServer = "wss://metroserverx.meowery.eu/ws"

// Event is something the server told us.
type Event struct {
	Type         string // room_created, join_request, join_approved, join_rejected, user_joined, user_left, sync_playback, sync_state, host_changed, kicked, error, suggestion_received, …
	RoomCode     string
	UserID       string
	Name         string
	SessionToken string
	Reason       string
	Code         string // error code
	SuggestionID string
	State        *State
	Action       *Action
	Track        *Track
	IsHost       bool
}

// Client is one connection to a Listen Together server.
type Client struct {
	conn    *websocket.Conn
	wmu     sync.Mutex
	onEvent func(Event)
	reqID   atomic.Uint64
	pingSeq atomic.Uint64
	hostSeq atomic.Uint64
	offset  atomic.Int64 // server clock − local clock (ms)
	rtt     atomic.Int64
	done    chan struct{}
	once    sync.Once
	CanHost bool

	mu       sync.Mutex
	RoomCode string
	UserID   string
	Token    string
	Host     bool
}

// Dial connects and negotiates capabilities. onEvent is called from the
// reading goroutine.
func Dial(ctx context.Context, url, userAgent, clientVersion string, onEvent func(Event)) (*Client, error) {
	if url == "" {
		url = DefaultServer
	}
	h := http.Header{}
	h.Set("User-Agent", userAgent)
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := d.DialContext(ctx, url, h)
	if err != nil {
		if resp != nil {
			return nil, errors.New("listen together: " + resp.Status)
		}
		return nil, err
	}
	conn.SetReadLimit(2 << 20)
	c := &Client{conn: conn, onEvent: onEvent, done: make(chan struct{})}
	var cw enc
	cw.bool(1, true) // protobuf
	cw.bool(2, true) // gzip
	cw.str(3, clientVersion)
	for _, f := range []string{"request_id", "server_restarting", "suggestion_ack", "replace_queue", "host_sequence", "track_revision", "drift_report"} {
		cw.str(4, f)
	}
	if err := c.send("client_capabilities", cw.b); err != nil {
		conn.Close()
		return nil, err
	}
	// the first answer must be server_capabilities
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}
	env, err := unmarshalEnvelope(data)
	if err != nil || env.Type != "server_capabilities" {
		conn.Close()
		if env.Type == "error" {
			m, _ := parse(env.Payload)
			return nil, errors.New("listen together: " + m.str(2))
		}
		return nil, errors.New("listen together: unexpected answer " + env.Type)
	}
	sm, _ := parse(env.Payload)
	c.CanHost = sm.bool(5)
	_ = conn.SetReadDeadline(time.Time{})
	conn.SetPingHandler(func(s string) error {
		c.wmu.Lock()
		defer c.wmu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(s), time.Now().Add(5*time.Second))
	})
	go c.readLoop()
	go c.pingLoop()
	return c, nil
}

func (c *Client) send(typ string, payload []byte) error {
	env := Envelope{Type: typ, Payload: payload, RequestID: c.reqID.Add(1)}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.BinaryMessage, env.marshal())
}

// Close leaves the room and disconnects.
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.wmu.Lock()
		_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		c.wmu.Unlock()
		c.conn.Close()
	})
}

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// ServerTime is the server clock now (unix ms), estimated from pings.
func (c *Client) ServerTime() int64 { return time.Now().UnixMilli() + c.offset.Load() }

// RTT is the last measured round trip (ms).
func (c *Client) RTT() int64 { return c.rtt.Load() }

func (c *Client) pingLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	c.ping()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			c.ping()
		}
	}
}

func (c *Client) ping() {
	var w enc
	w.int64(1, time.Now().UnixMilli())
	w.varint(2, c.pingSeq.Add(1))
	_ = c.send("ping", w.b)
}

func (c *Client) readLoop() {
	defer c.Close()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.emit(Event{Type: "disconnected", Reason: err.Error()})
			return
		}
		env, err := unmarshalEnvelope(data)
		if err != nil {
			continue
		}
		c.handle(env)
	}
}

func (c *Client) emit(e Event) {
	if c.onEvent != nil {
		c.onEvent(e)
	}
}

func (c *Client) handle(env Envelope) {
	m, _ := parse(env.Payload)
	e := Event{Type: env.Type}
	switch env.Type {
	case "pong":
		now := time.Now().UnixMilli()
		sent, recv, sendT := m.i64(1), m.i64(2), m.i64(3)
		if sent > 0 && recv > 0 {
			c.rtt.Store((now - sent) - (sendT - recv))
			c.offset.Store(((recv - sent) + (sendT - now)) / 2)
		}
		return
	case "ack":
		return
	case "room_created":
		e.RoomCode, e.UserID, e.SessionToken = m.str(1), m.str(2), m.str(3)
		c.mu.Lock()
		c.RoomCode, c.UserID, c.Token, c.Host = e.RoomCode, e.UserID, e.SessionToken, true
		c.mu.Unlock()
		e.IsHost = true
	case "join_request", "user_joined", "user_left", "user_reconnected", "user_disconnected":
		e.UserID, e.Name = m.str(1), m.str(2)
	case "join_approved":
		e.RoomCode, e.UserID, e.SessionToken = m.str(1), m.str(2), m.str(3)
		st := parseRoomState(m.raw(4))
		e.State = &st
		c.mu.Lock()
		c.RoomCode, c.UserID, c.Token, c.Host = e.RoomCode, e.UserID, e.SessionToken, false
		c.mu.Unlock()
	case "reconnected":
		e.RoomCode, e.UserID = m.str(1), m.str(2)
		st := parseRoomState(m.raw(3))
		e.State, e.IsHost = &st, m.bool(4)
		c.mu.Lock()
		c.RoomCode, c.UserID, c.Host = e.RoomCode, e.UserID, e.IsHost
		c.mu.Unlock()
	case "join_rejected", "kicked":
		e.Reason = m.str(1)
	case "sync_playback":
		a := parseAction(env.Payload)
		e.Action = &a
	case "sync_state":
		st := parseSyncState(env.Payload)
		e.State = &st
	case "host_changed":
		e.UserID, e.Name = m.str(1), m.str(2)
		c.mu.Lock()
		c.Host = e.UserID == c.UserID
		e.IsHost = c.Host
		c.mu.Unlock()
	case "error":
		e.Code, e.Reason = m.str(1), m.str(2)
	case "suggestion_received":
		e.SuggestionID, e.UserID, e.Name = m.str(1), m.str(2), m.str(3)
		t := parseTrack(m.raw(4))
		e.Track = &t
	case "suggestion_approved", "suggestion_submitted":
		e.SuggestionID = m.str(1)
		t := parseTrack(m.raw(2))
		e.Track = &t
	case "suggestion_rejected":
		e.SuggestionID, e.Reason = m.str(1), m.str(2)
	case "server_restarting":
		e.Reason = m.str(2)
	}
	c.emit(e)
}

// IsHost reports whether we control the room.
func (c *Client) IsHost() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.Host }

// ---------------------------------------------------------------- commands

// Create opens a room; the answer is a room_created event.
func (c *Client) Create(username string) error {
	var w enc
	w.str(1, username)
	return c.send("create_room", w.b)
}

// Join asks the host to let us in; join_approved or join_rejected follows.
func (c *Client) Join(code, username string) error {
	var w enc
	w.str(1, code)
	w.str(2, username)
	return c.send("join_room", w.b)
}

// Leave leaves the room (the connection stays open).
func (c *Client) Leave() error { return c.send("leave_room", nil) }

func (c *Client) userCmd(typ, id, reason string) error {
	var w enc
	w.str(1, id)
	w.str(2, reason)
	return c.send(typ, w.b)
}

func (c *Client) Approve(userID string) error         { return c.userCmd("approve_join", userID, "") }
func (c *Client) Reject(userID, reason string) error  { return c.userCmd("reject_join", userID, reason) }
func (c *Client) Kick(userID, reason string) error    { return c.userCmd("kick_user", userID, reason) }
func (c *Client) TransferHost(userID string) error    { return c.userCmd("transfer_host", userID, "") }
func (c *Client) ApproveSuggestion(id string) error   { return c.userCmd("approve_suggestion", id, "") }
func (c *Client) RejectSuggestion(id, r string) error { return c.userCmd("reject_suggestion", id, r) }

// Playback sends a host action. The position is stamped with the server
// time so listeners compensate the network delay.
func (c *Client) Playback(a Action) error {
	if a.CapturedAt == 0 && (a.Action == "play" || a.Action == "seek") {
		a.CapturedAt = c.ServerTime()
	}
	a.HostSequence = c.hostSeq.Add(1)
	return c.send("playback_action", a.marshal())
}

// BufferReady tells the server the track is loaded on our side.
func (c *Client) BufferReady(trackID string, revision uint64) error {
	var w enc
	w.str(1, trackID)
	w.varint(2, revision)
	return c.send("buffer_ready", w.b)
}

// RequestSync asks for the full room state (sync_state).
func (c *Client) RequestSync() error { return c.send("request_sync", nil) }

// Suggest proposes a track to the host.
func (c *Client) Suggest(t Track) error {
	var w enc
	w.bytes(1, t.marshal())
	return c.send("suggest_track", w.b)
}

// Reconnect resumes a session after a dropped connection.
func (c *Client) Reconnect(token string) error {
	var w enc
	w.str(1, token)
	return c.send("reconnect", w.b)
}
