package mpris

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fake struct {
	mu    sync.Mutex
	calls []string
}

func (f *fake) add(s string)        { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fake) PlayPause()          { f.add("playpause") }
func (f *fake) Play()               { f.add("play") }
func (f *fake) Pause()              { f.add("pause") }
func (f *fake) Stop()               { f.add("stop") }
func (f *fake) Next()               { f.add("next") }
func (f *fake) Previous()           { f.add("prev") }
func (f *fake) SeekBy(int64)        { f.add("seekby") }
func (f *fake) SeekTo(int64)        { f.add("seekto") }
func (f *fake) SetVolume(v float64) { f.add("volume") }
func (f *fake) SetShuffle(bool)     { f.add("shuffle") }
func (f *fake) SetLoop(string)      { f.add("loop") }
func (f *fake) SetRate(float64)     { f.add("rate") }
func (f *fake) Raise()              { f.add("raise") }
func (f *fake) Quit()               { f.add("quit") }

func TestMPRIS(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}
	f := &fake{}
	s, err := Start(f)
	if err != nil {
		t.Skip("cannot publish:", err)
	}
	defer s.Close()
	s.Update(State{Track: Track{Key: "x:1", Title: "Song", Artist: "Art", LengthMs: 1000}, Status: "Playing", Volume: 0.5, Loop: "None", Rate: 1})
	s.SetPosition(500)

	c, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	obj := c.Object(busName, objPath)
	if err := obj.Call(playerIfac+".Next", 0).Err; err != nil {
		t.Fatal(err)
	}
	if err := obj.Call("org.freedesktop.DBus.Properties.Set", 0, playerIfac, "Volume", dbus.MakeVariant(0.3)).Err; err != nil {
		t.Fatal(err)
	}
	var st dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIfac, "PlaybackStatus").Store(&st); err != nil || st.Value() != "Playing" {
		t.Fatalf("status %v %v", st, err)
	}
	var md dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIfac, "Metadata").Store(&md); err != nil {
		t.Fatal(err)
	}
	if m, ok := md.Value().(map[string]dbus.Variant); !ok || m["xesam:title"].Value() != "Song" {
		t.Fatalf("metadata %v", md)
	}
	time.Sleep(100 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) < 2 || f.calls[0] != "next" || f.calls[1] != "volume" {
		t.Fatalf("calls %v", f.calls)
	}
}
