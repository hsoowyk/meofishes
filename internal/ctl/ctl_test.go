package ctl

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestServeSend(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	stop, err := Serve(func(cmd string, args []string) (any, error) {
		if cmd == "fail" {
			return nil, errors.New("nope")
		}
		return map[string]any{"cmd": cmd, "args": args}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	raw, err := Send("next", "a")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m["cmd"] != "next" {
		t.Fatalf("%v", m)
	}
	if _, err := Send("fail"); err == nil || err.Error() != "nope" {
		t.Fatalf("err: %v", err)
	}
	stop()
	if _, err := Send("x"); err != ErrNotRunning {
		t.Fatalf("expected not running, got %v", err)
	}
}
