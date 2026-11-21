// Package discord shows "Listening to …" on the user's Discord profile
// through the local Discord client's IPC socket (Rich Presence). It needs a
// Discord application id (discord.com/developers → New Application).
package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Presence is what is shown.
type Presence struct {
	Title, Artist, Album string
	CoverURL             string // https image (Discord shows external images)
	StartMs, EndMs       int64  // unix ms; 0 = no timestamps
	Paused               bool
}

// Client is a connection to the Discord app.
type Client struct {
	AppID string
	mu    sync.Mutex
	conn  net.Conn
	nonce int
}

// socketPaths lists where Discord (native, Flatpak, Snap) puts its IPC socket.
func socketPaths() []string {
	var dirs []string
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if d := os.Getenv(env); d != "" {
			dirs = append(dirs, d, filepath.Join(d, "app/com.discordapp.Discord"), filepath.Join(d, "snap.discord"),
				filepath.Join(d, ".flatpak/com.discordapp.Discord/xdg-run"), filepath.Join(d, "app/dev.vencord.Vesktop"))
		}
	}
	dirs = append(dirs, "/tmp")
	var out []string
	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			out = append(out, filepath.Join(d, fmt.Sprintf("discord-ipc-%d", i)))
		}
	}
	return out
}

func (c *Client) frame(op uint32, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, op)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = c.conn.Write(b.Bytes())
	return err
}

func (c *Client) read() (uint32, []byte, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(c.conn, hdr[:]); err != nil {
		return 0, nil, err
	}
	op := binary.LittleEndian.Uint32(hdr[:4])
	n := binary.LittleEndian.Uint32(hdr[4:])
	if n > 1<<20 {
		return 0, nil, errors.New("discord: frame too big")
	}
	data := make([]byte, n)
	_, err := io.ReadFull(c.conn, data)
	return op, data, err
}

// connect opens the socket and sends the handshake (c.mu held).
func (c *Client) connect() error {
	if c.AppID == "" {
		return errors.New("discord: no application id")
	}
	var last error = errors.New("discord is not running")
	for _, p := range socketPaths() {
		conn, err := net.DialTimeout("unix", p, 500*time.Millisecond)
		if err != nil {
			continue
		}
		c.conn = conn
		if err := c.frame(0, map[string]any{"v": 1, "client_id": c.AppID}); err != nil {
			conn.Close()
			last = err
			continue
		}
		if _, data, err := c.read(); err != nil {
			conn.Close()
			last = err
			continue
		} else if bytes.Contains(data, []byte(`"evt":"ERROR"`)) {
			conn.Close()
			return fmt.Errorf("discord: %s", data)
		}
		return nil
	}
	c.conn = nil
	return last
}

// Set shows a presence (nil clears it).
func (c *Client) Set(p *Presence) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		if err := c.connect(); err != nil {
			return err
		}
	}
	var activity any
	if p != nil {
		a := map[string]any{
			"type":    2, // Listening
			"details": clip(p.Title, 128),
			"state":   clip("by "+p.Artist, 128),
		}
		assets := map[string]any{"large_text": clip(firstNonEmpty(p.Album, p.Title), 128)}
		if p.CoverURL != "" {
			assets["large_image"] = p.CoverURL
		}
		a["assets"] = assets
		if !p.Paused && p.StartMs > 0 {
			ts := map[string]any{"start": p.StartMs}
			if p.EndMs > p.StartMs {
				ts["end"] = p.EndMs
			}
			a["timestamps"] = ts
		}
		activity = a
	}
	c.nonce++
	err := c.frame(1, map[string]any{"cmd": "SET_ACTIVITY", "nonce": fmt.Sprint(c.nonce),
		"args": map[string]any{"pid": os.Getpid(), "activity": activity}})
	if err == nil {
		_, _, err = c.read()
	}
	if err != nil {
		c.conn.Close()
		c.conn = nil
	}
	return err
}

// Close disconnects (Discord clears the presence).
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) < 2 {
		return s + "  " // Discord wants at least 2 characters
	}
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
