package player

import "testing"

func TestTechBadge(t *testing.T) {
	for _, c := range []struct {
		t     Tech
		badge string
		s     string
	}{
		{Tech{Codec: "flac", SampleRate: 44100, Bits: 16, Channels: 2, BitrateKbps: 912}, "Lossless", "FLAC · 44.1 kHz · 16-bit · 912 kbps"},
		{Tech{Codec: "flac", SampleRate: 96000, Bits: 24, Channels: 2}, "Hi-Res", "FLAC · 96 kHz · 24-bit"},
		{Tech{Codec: "opus", SampleRate: 48000, Channels: 2, BitrateKbps: 140}, "HQ", "OPUS · 48 kHz · 140 kbps"},
		{Tech{Codec: "aac", SampleRate: 44100, Channels: 2, BitrateKbps: 128}, "", "AAC · 44.1 kHz · 128 kbps"},
	} {
		if c.t.Badge() != c.badge || c.t.String() != c.s {
			t.Errorf("%+v: %q %q", c.t, c.t.Badge(), c.t.String())
		}
	}
}
