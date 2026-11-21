// Package ctl lets the command line control the running app through a unix
// socket ($XDG_RUNTIME_DIR/meofishes.sock): one JSON request per connection.
//
//	→ {"cmd":"next","args":[]}
//	← {"ok":true,"result":…}   or   {"ok":false,"error":"…"}
package ctl

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Request is one command.
type Request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

// Response is the answer.
type Response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ErrNotRunning means no meofishes is open.
var ErrNotRunning = errors.New("meofishes is not running")

// SocketPath is where the running app listens.
func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "meofishes.sock")
}

// Serve listens until the listener is closed. handle runs each command.
func Serve(handle func(cmd string, args []string) (any, error)) (func(), error) {
	path := SocketPath()
	if c, err := net.DialTimeout("unix", path, 300*time.Millisecond); err == nil {
		c.Close()
		return nil, errors.New("another meofishes is already running")
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveConn(c, handle)
		}
	}()
	return func() { ln.Close(); os.Remove(path) }, nil
}

func serveConn(c net.Conn, handle func(string, []string) (any, error)) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(60 * time.Second))
	var req Request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		return
	}
	res, err := handle(req.Cmd, req.Args)
	out := Response{OK: err == nil}
	if err != nil {
		out.Error = err.Error()
	} else if res != nil {
		out.Result, _ = json.Marshal(res)
	}
	_ = json.NewEncoder(c).Encode(out)
}

// Send runs a command in the running app.
func Send(cmd string, args ...string) (json.RawMessage, error) {
	c, err := net.DialTimeout("unix", SocketPath(), time.Second)
	if err != nil {
		return nil, ErrNotRunning
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(60 * time.Second))
	if err := json.NewEncoder(c).Encode(Request{Cmd: cmd, Args: args}); err != nil {
		return nil, err
	}
	var res Response
	if err := json.NewDecoder(c).Decode(&res); err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, errors.New(res.Error)
	}
	return res.Result, nil
}
