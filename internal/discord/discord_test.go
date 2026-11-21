package discord

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

// fake Discord IPC server
func TestPresence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "discord-ipc-0"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 4)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		for {
			var hdr [8]byte
			if _, err := io.ReadFull(c, hdr[:]); err != nil {
				return
			}
			n := binary.LittleEndian.Uint32(hdr[4:])
			data := make([]byte, n)
			io.ReadFull(c, data)
			got <- string(data)
			reply, _ := json.Marshal(map[string]any{"evt": "READY"})
			out := make([]byte, 8)
			binary.LittleEndian.PutUint32(out, 1)
			binary.LittleEndian.PutUint32(out[4:], uint32(len(reply)))
			c.Write(append(out, reply...))
		}
	}()
	cl := &Client{AppID: "123"}
	if err := cl.Set(&Presence{Title: "Song", Artist: "Art", StartMs: 1000, EndMs: 5000}); err != nil {
		t.Fatal(err)
	}
	hs, act := <-got, <-got
	if !strings.Contains(hs, `"client_id":"123"`) || !strings.Contains(act, `"SET_ACTIVITY"`) || !strings.Contains(act, `"type":2`) {
		t.Fatalf("frames: %s / %s", hs, act)
	}
	cl.Close()
}
