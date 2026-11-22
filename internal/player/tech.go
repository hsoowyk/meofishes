package player

import (
	"fmt"
	"strings"
)

// Tech is the technical information of what is playing.
type Tech struct {
	Codec       string // flac, opus, aac, mp3…
	SampleRate  int    // Hz
	Bits        int    // 0 = unknown / float
	Channels    int
	BitrateKbps int
}

var losslessCodecs = map[string]bool{"flac": true, "alac": true, "wavpack": true, "ape": true, "tta": true, "truehd": true, "mlp": true}

// Lossless reports a lossless codec (or raw PCM).
func (t Tech) Lossless() bool {
	return losslessCodecs[t.Codec] || strings.HasPrefix(t.Codec, "pcm_")
}

// Badge is a short quality label: "Hi-Res", "Lossless", "HQ" or "".
func (t Tech) Badge() string {
	switch {
	case t.Codec == "":
		return ""
	case t.Lossless() && (t.SampleRate > 48000 || t.Bits >= 24):
		return "Hi-Res"
	case t.Lossless():
		return "Lossless"
	case t.BitrateKbps >= 256 || (t.Codec == "opus" && t.BitrateKbps >= 128):
		return "HQ"
	}
	return ""
}

// String is the full description, e.g. "FLAC · 44.1 kHz · 16-bit · 2 ch · 912 kbps".
func (t Tech) String() string {
	if t.Codec == "" {
		return ""
	}
	parts := []string{strings.ToUpper(strings.TrimPrefix(t.Codec, "pcm_"))}
	if t.SampleRate > 0 {
		parts = append(parts, strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(t.SampleRate)/1000), "0"), ".")+" kHz")
	}
	if t.Bits > 0 {
		parts = append(parts, fmt.Sprintf("%d-bit", t.Bits))
	}
	if t.Channels > 0 && t.Channels != 2 {
		parts = append(parts, fmt.Sprintf("%d ch", t.Channels))
	}
	if t.BitrateKbps > 0 {
		parts = append(parts, fmt.Sprintf("%d kbps", t.BitrateKbps))
	}
	return strings.Join(parts, " · ")
}

// Tech asks mpv what is playing (blocks for a few IPC round trips).
func (m *MPV) Tech() Tech {
	var t Tech
	if v, err := m.Get("audio-codec-name"); err == nil {
		t.Codec, _ = v.(string)
	}
	if v, err := m.Get("audio-params"); err == nil {
		if p, ok := v.(map[string]any); ok {
			if sr, ok := p["samplerate"].(float64); ok {
				t.SampleRate = int(sr)
			}
			if ch, ok := p["channel-count"].(float64); ok {
				t.Channels = int(ch)
			}
			if f, ok := p["format"].(string); ok && t.Lossless() {
				switch strings.TrimSuffix(f, "p") {
				case "s16":
					t.Bits = 16
				case "s32":
					t.Bits = 24 // decoders put 24-bit samples in 32-bit words
				case "u8":
					t.Bits = 8
				}
			}
		}
	}
	if v, err := m.Get("audio-bitrate"); err == nil {
		if b, ok := v.(float64); ok {
			t.BitrateKbps = int(b / 1000)
		}
	}
	return t
}
