package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"meofishes/internal/player"
)

// PluginPlayer drives an xtm "player" plugin (a process that implements the
// player.* methods) as a player.Backend.
type PluginPlayer struct {
	p    *Plugin
	proc *process
	ev   player.Events
	mu   sync.Mutex
	st   player.State
	at   time.Time
	stop chan struct{}
}

// NewPluginPlayer starts using p as the player.
func NewPluginPlayer(p *Plugin, ev player.Events) (*PluginPlayer, error) {
	proc, ok := p.impl.(*process)
	if !ok || !p.Manifest.Player {
		return nil, errors.New(p.Manifest.ID + " is not a running player plugin")
	}
	pp := &PluginPlayer{p: p, proc: proc, ev: ev, stop: make(chan struct{})}
	pp.st.Idle, pp.st.Paused = true, true
	proc.OnNotify(func(method string, params json.RawMessage) {
		if method == "player.end" && ev.OnEnd != nil {
			var x struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(params, &x)
			if x.Reason == "" {
				x.Reason = "eof"
			}
			ev.OnEnd(x.Reason)
		}
		if method == "player.error" && ev.OnError != nil {
			var x struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(params, &x)
			ev.OnError(errors.New(x.Message))
		}
	})
	go pp.poll()
	return pp, nil
}

func (pp *PluginPlayer) call(method string, params any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return pp.proc.Call(ctx, method, params, out)
}

func (pp *PluginPlayer) poll() {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-pp.stop:
			return
		case <-t.C:
			var st player.State
			if err := pp.call("player.state", nil, &st); err == nil {
				pp.mu.Lock()
				pp.st, pp.at = st, time.Now()
				pp.mu.Unlock()
			}
		}
	}
}

func (pp *PluginPlayer) Name() string { return pp.p.Manifest.Name }
func (pp *PluginPlayer) Load(url string, headers map[string]string) error {
	return pp.call("player.load", map[string]any{"url": url, "headers": headers}, nil)
}
func (pp *PluginPlayer) SetPaused(p bool) error {
	return pp.call("player.pause", map[string]any{"paused": p}, nil)
}
func (pp *PluginPlayer) TogglePause() error { return pp.SetPaused(!pp.State().Paused) }
func (pp *PluginPlayer) Stop() error        { return pp.call("player.stop", nil, nil) }
func (pp *PluginPlayer) SetVolume(v float64) error {
	return pp.call("player.volume", map[string]any{"volume": v}, nil)
}
func (pp *PluginPlayer) SeekTo(ms int64) error {
	return pp.call("player.seek", map[string]any{"positionMs": ms}, nil)
}

// State interpolates the position between polls.
func (pp *PluginPlayer) State() player.State {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	st := pp.st
	if !st.Paused && !st.Idle && !pp.at.IsZero() {
		if d := time.Since(pp.at); d < time.Second {
			st.PositionMs += d.Milliseconds()
		}
	}
	return st
}

func (pp *PluginPlayer) Close() {
	select {
	case <-pp.stop:
	default:
		close(pp.stop)
	}
	_ = pp.Stop()
}
