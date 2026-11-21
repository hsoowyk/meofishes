// Package player drives mpv through its JSON IPC socket. mpv plays local
// files, HTTP streams and, through yt-dlp, web pages (YouTube, SoundCloud…).
package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// State is a snapshot of playback.
type State struct {
	PositionMs int64
	DurationMs int64
	Paused     bool
	Idle       bool
	Volume     float64
	Buffering  bool
}

// Backend is a player: mpv by default, or an xtm "player" plugin.
type Backend interface {
	Name() string
	Load(url string, headers map[string]string) error
	SetPaused(p bool) error
	TogglePause() error
	Stop() error
	SetVolume(v float64) error // 0..100
	SeekTo(ms int64) error
	State() State
	Close()
}

// Options configure mpv.
type Options struct {
	CacheMB       int    // demuxer cache (default 64)
	ReadaheadSecs int    // how far mpv reads ahead (default 30)
	ReplayGain    string // no | track | album (default track)
	Normalize     bool   // dynamic loudness normalization
	Device        string // audio device ("" = auto)
	Exclusive     bool   // exclusive audio output where supported
	Volume        float64
	Extra         []string // extra mpv arguments
}

// Events from the player (called from a background goroutine).
type Events struct {
	OnState func(State)         // position/duration/pause changes
	OnEnd   func(reason string) // "eof", "error", "stop"
	OnError func(error)
}

// MPV is a running mpv instance.
type MPV struct {
	cmd    *exec.Cmd
	conn   net.Conn
	sock   string
	ev     Events
	mu     sync.Mutex
	st     State
	reqID  atomic.Int64
	wait   sync.Map
	closed atomic.Bool
	// position interpolation
	posAt time.Time
	// get_property results by request id
	dataMu sync.Mutex
	data   map[int64]json.RawMessage
}

// Available reports whether mpv is installed.
func Available() bool { _, err := exec.LookPath("mpv"); return err == nil }

// Name is "mpv".
func (m *MPV) Name() string { return "mpv" }

// Start launches mpv in idle mode.
func Start(runtimeDir string, opt Options, ev Events) (*MPV, error) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		return nil, errors.New("mpv not found: install it (e.g. xbps-install mpv)")
	}
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	sock := filepath.Join(runtimeDir, fmt.Sprintf("meofishes-mpv-%d.sock", os.Getpid()))
	_ = os.Remove(sock)
	if opt.CacheMB <= 0 {
		opt.CacheMB = 64
	}
	if opt.ReadaheadSecs <= 0 {
		opt.ReadaheadSecs = 30
	}
	if opt.ReplayGain == "" {
		opt.ReplayGain = "track"
	}
	if opt.Volume <= 0 {
		opt.Volume = 80
	}
	args := []string{
		"--idle=yes", "--no-video", "--no-terminal", "--force-window=no",
		"--input-ipc-server=" + sock, "--audio-display=no", "--keep-open=no",
		"--ytdl-format=bestaudio/best", "--cache=yes",
		fmt.Sprintf("--demuxer-max-bytes=%dMiB", opt.CacheMB),
		fmt.Sprintf("--demuxer-readahead-secs=%d", opt.ReadaheadSecs),
		"--cache-pause-initial=no", "--cache-pause-wait=1",
		fmt.Sprintf("--volume=%g", opt.Volume), "--replaygain=" + opt.ReplayGain,
	}
	if opt.Normalize {
		args = append(args, "--af=lavfi=[dynaudnorm=f=250:g=15]")
	}
	if opt.Device != "" {
		args = append(args, "--audio-device="+opt.Device)
	}
	if opt.Exclusive {
		args = append(args, "--audio-exclusive=yes")
	}
	args = append(args, opt.Extra...)
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	m := &MPV{cmd: cmd, sock: sock, ev: ev}
	m.st.Volume = 80
	m.st.Idle = true
	var conn net.Conn
	for i := 0; i < 100; i++ {
		conn, err = net.Dial("unix", sock)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("connect to mpv: %w", err)
	}
	m.conn = conn
	go m.read()
	go func() { _ = cmd.Wait(); m.closed.Store(true) }()
	for i, p := range []string{"time-pos", "duration", "pause", "idle-active", "volume", "paused-for-cache"} {
		_ = m.send(map[string]any{"command": []any{"observe_property", i + 1, p}})
	}
	return m, nil
}

func (m *MPV) send(msg map[string]any) error {
	if m.closed.Load() {
		return errors.New("mpv is not running")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err = m.conn.Write(append(data, '\n'))
	return err
}

func (m *MPV) command(args ...any) error {
	id := m.reqID.Add(1)
	ch := make(chan string, 1)
	m.wait.Store(id, ch)
	if err := m.send(map[string]any{"command": args, "request_id": id}); err != nil {
		m.wait.Delete(id)
		return err
	}
	select {
	case e := <-ch:
		if e != "success" {
			return fmt.Errorf("mpv: %s", e)
		}
		return nil
	case <-time.After(5 * time.Second):
		m.wait.Delete(id)
		return errors.New("mpv: timeout")
	}
}

func (m *MPV) read() {
	sc := bufio.NewScanner(m.conn)
	sc.Buffer(make([]byte, 1<<16), 1<<22)
	for sc.Scan() {
		var msg struct {
			Event     string          `json:"event"`
			Name      string          `json:"name"`
			Data      json.RawMessage `json:"data"`
			Error     string          `json:"error"`
			RequestID int64           `json:"request_id"`
			Reason    string          `json:"reason"`
			FileError string          `json:"file_error"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		if msg.RequestID != 0 && msg.Event == "" {
			m.dataMu.Lock()
			if m.data != nil && len(msg.Data) > 0 {
				m.data[msg.RequestID] = msg.Data
			}
			m.dataMu.Unlock()
			if ch, ok := m.wait.LoadAndDelete(msg.RequestID); ok {
				ch.(chan string) <- msg.Error
			}
			continue
		}
		switch msg.Event {
		case "property-change":
			m.mu.Lock()
			switch msg.Name {
			case "time-pos":
				var f float64
				if json.Unmarshal(msg.Data, &f) == nil {
					m.st.PositionMs = int64(f * 1000)
					m.posAt = time.Now()
				}
			case "duration":
				var f float64
				if json.Unmarshal(msg.Data, &f) == nil {
					m.st.DurationMs = int64(f * 1000)
				}
			case "pause":
				_ = json.Unmarshal(msg.Data, &m.st.Paused)
				m.posAt = time.Now()
			case "idle-active":
				_ = json.Unmarshal(msg.Data, &m.st.Idle)
			case "volume":
				_ = json.Unmarshal(msg.Data, &m.st.Volume)
			case "paused-for-cache":
				_ = json.Unmarshal(msg.Data, &m.st.Buffering)
			}
			st := m.st
			m.mu.Unlock()
			if m.ev.OnState != nil {
				m.ev.OnState(st)
			}
		case "end-file":
			if msg.Reason == "error" && m.ev.OnError != nil {
				m.ev.OnError(fmt.Errorf("could not play: %s", msg.FileError))
			}
			if m.ev.OnEnd != nil && msg.Reason != "redirect" {
				m.ev.OnEnd(msg.Reason)
			}
		case "start-file":
			m.mu.Lock()
			m.st.PositionMs, m.st.DurationMs = 0, 0
			m.mu.Unlock()
		}
	}
}

// Load plays a URL or file. headers are sent with HTTP requests.
func (m *MPV) Load(url string, headers map[string]string) error {
	hs := []string{}
	for k, v := range headers {
		hs = append(hs, k+": "+v)
	}
	_ = m.command("set_property", "http-header-fields", hs)
	_ = m.command("set_property", "pause", false)
	return m.command("loadfile", url, "replace")
}

func (m *MPV) SetPaused(p bool) error    { return m.command("set_property", "pause", p) }
func (m *MPV) TogglePause() error        { return m.command("cycle", "pause") }
func (m *MPV) Stop() error               { return m.command("stop") }
func (m *MPV) SetVolume(v float64) error { return m.command("set_property", "volume", v) }
func (m *MPV) SeekTo(ms int64) error {
	return m.command("seek", float64(ms)/1000, "absolute+exact")
}

// Get reads any mpv property (developer mode).
func (m *MPV) Get(name string) (any, error) {
	id := m.reqID.Add(1)
	ch := make(chan string, 1)
	m.wait.Store(id, ch)
	m.dataMu.Lock()
	if m.data == nil {
		m.data = map[int64]json.RawMessage{}
	}
	m.dataMu.Unlock()
	if err := m.send(map[string]any{"command": []any{"get_property", name}, "request_id": id}); err != nil {
		m.wait.Delete(id)
		return nil, err
	}
	select {
	case e := <-ch:
		m.dataMu.Lock()
		raw := m.data[id]
		delete(m.data, id)
		m.dataMu.Unlock()
		if e != "success" {
			return nil, fmt.Errorf("mpv: %s", e)
		}
		var v any
		_ = json.Unmarshal(raw, &v)
		return v, nil
	case <-time.After(2 * time.Second):
		m.wait.Delete(id)
		return nil, errors.New("mpv: timeout")
	}
}

// Devices lists the audio output devices ("name" → "description").
func (m *MPV) Devices() map[string]string {
	v, err := m.Get("audio-device-list")
	out := map[string]string{}
	if err != nil {
		return out
	}
	list, _ := v.([]any)
	for _, x := range list {
		if d, ok := x.(map[string]any); ok {
			n, _ := d["name"].(string)
			desc, _ := d["description"].(string)
			out[n] = desc
		}
	}
	return out
}

// State returns the last state, with the position interpolated since the
// last update so lyrics animate smoothly.
func (m *MPV) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.st
	if !st.Paused && !st.Idle && !st.Buffering && !m.posAt.IsZero() {
		d := time.Since(m.posAt)
		if d < time.Second {
			st.PositionMs += d.Milliseconds()
		}
	}
	return st
}

// Close quits mpv.
func (m *MPV) Close() {
	if m.closed.Load() {
		return
	}
	_ = m.send(map[string]any{"command": []any{"quit"}})
	time.AfterFunc(time.Second, func() {
		if !m.closed.Load() && m.cmd.Process != nil {
			_ = m.cmd.Process.Kill()
		}
	})
	_ = os.Remove(m.sock)
}
