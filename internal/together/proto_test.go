package together

import "testing"

func TestActionRoundTrip(t *testing.T) {
	a := Action{Action: "change_track", TrackID: "dQw4w9WgXcQ", PositionMs: 1234, Volume: 0.5,
		Track: &Track{ID: "dQw4w9WgXcQ", Title: "Never", Artist: "Rick", DurationMs: 213000},
		Queue: []Track{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}, HostSequence: 7, CapturedAt: 99}
	b := parseAction(a.marshal())
	if b.Action != a.Action || b.TrackID != a.TrackID || b.PositionMs != 1234 || b.Volume != 0.5 ||
		b.Track == nil || b.Track.Title != "Never" || len(b.Queue) != 2 || b.Queue[1].ID != "b" || b.HostSequence != 7 || b.CapturedAt != 99 {
		t.Fatalf("round trip: %+v", b)
	}
	env := Envelope{Type: "playback_action", Payload: a.marshal(), RequestID: 3}
	e2, err := unmarshalEnvelope(env.marshal())
	if err != nil || e2.Type != "playback_action" || e2.RequestID != 3 || parseAction(e2.Payload).TrackID != a.TrackID {
		t.Fatalf("envelope: %+v %v", e2, err)
	}
}
