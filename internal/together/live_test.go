//go:build live

package together

import (
	"context"
	"testing"
	"time"
)

// TestLiveRoom creates a room on the community server, joins it with a
// second client, and checks that a host action reaches the listener.
func TestLiveRoom(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	ua := "meofishes/test (+https://git.5418.org/kywoosh/meofishes)"
	hostEv := make(chan Event, 32)
	host, err := Dial(ctx, DefaultServer, ua, "meofishes-test", func(e Event) { hostEv <- e })
	if err != nil {
		t.Fatal("dial host:", err)
	}
	defer host.Close()
	t.Logf("server can_host=%v", host.CanHost)
	if err := host.Create("meo-host"); err != nil {
		t.Fatal(err)
	}
	created := wait(t, hostEv, "room_created")
	t.Logf("room %s", created.RoomCode)

	guestEv := make(chan Event, 32)
	guest, err := Dial(ctx, DefaultServer, ua, "meofishes-test", func(e Event) { guestEv <- e })
	if err != nil {
		t.Fatal("dial guest:", err)
	}
	defer guest.Close()
	_ = guest.Join(created.RoomCode, "meo-guest")
	req := wait(t, hostEv, "join_request")
	_ = host.Approve(req.UserID)
	wait(t, guestEv, "join_approved")

	tr := Track{ID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up", Artist: "Rick Astley", DurationMs: 213000}
	_ = host.Playback(Action{Action: "change_track", TrackID: tr.ID, Track: &tr})
	_ = host.Playback(Action{Action: "play", TrackID: tr.ID, PositionMs: 5000})
	for {
		e := wait(t, guestEv, "sync_playback")
		t.Logf("guest got %s %s at %d ms (rtt %d ms)", e.Action.Action, e.Action.TrackID, e.Action.PositionMs, guest.RTT())
		if e.Action.Action == "play" {
			break
		}
	}
	_ = guest.Leave()
	_ = host.Leave()
}

func wait(t *testing.T, ch chan Event, typ string) Event {
	t.Helper()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == "error" {
				t.Fatalf("server error %s: %s", e.Code, e.Reason)
			}
			if e.Type == typ {
				return e
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s", typ)
		}
	}
}
