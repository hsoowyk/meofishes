package player

import (
	"os/exec"
	"testing"
	"time"
)

// TestStartRealMpv starts the installed mpv with the options the app uses
// (an option mpv rejects makes it exit at once and nothing plays).
func TestStartRealMpv(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not installed")
	}
	for _, opt := range []Options{
		{},
		{Wid: 12345, X11Video: true, Normalize: true, AudioFmt: "bestaudio/best", EQ: EQPresets["rock"], Speed: 1.25, YtdlRaw: "cookies-from-browser=firefox", VinylPitch: true,
			FX: Effects{Karaoke: true, Pitch: -3, Reverb: 1, Mono: true, Noise: "pink", NoiseVol: 1}},
	} {
		m, err := Start(t.TempDir(), opt, Events{})
		if err != nil {
			t.Fatalf("start %+v: %v", opt, err)
		}
		if err := m.SetVolume(50); err != nil {
			t.Fatalf("command: %v", err)
		}
		if err := m.SetFilters(Filters(true, EQPresets["bass"], Effects{Karaoke: true, Pitch: 2, Reverb: 0.5, Balance: 0.2, Spatial: true, Crossfeed: true, Noise: "brown", NoiseVol: 0.5})); err != nil {
			t.Fatalf("filters rejected by mpv: %v", err)
		}
		if err := m.SetSpeed(0.8); err != nil {
			t.Fatalf("speed: %v", err)
		}
		if opt.Wid != 0 && m.NoVideo {
			t.Errorf("mpv rejected the video options (fell back to audio only)")
		}
		m.Close()
		time.Sleep(200 * time.Millisecond)
	}
}

// TestEffectsPlay plays a generated tone through every effect: mpv must keep
// playing (a filter it cannot build would stop the audio).
func TestEffectsPlay(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not installed")
	}
	ended := make(chan string, 4)
	m, err := Start(t.TempDir(), Options{Extra: []string{"--ao=null"}, Normalize: true, EQ: EQPresets["bass"], VinylPitch: true, Speed: 1.25,
		FX: Effects{Karaoke: true, Pitch: 2, Reverb: 0.6, Balance: -0.3, Spatial: true, Crossfeed: true, Noise: "brown", NoiseVol: 0.4}},
		Events{OnEnd: func(r string) { ended <- r }})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Load("av://lavfi:aevalsrc=sin(440*2*PI*t)|sin(550*2*PI*t):s=44100:d=30", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	if err := m.SetABLoop(0.2, 0.9); err != nil {
		t.Fatalf("ab loop: %v", err)
	}
	if err := m.SetFilters(Filters(false, nil, Effects{Mono: true, Noise: "white", NoiseVol: 0.2})); err != nil {
		t.Fatalf("filters: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	select {
	case r := <-ended:
		t.Fatalf("playback ended: %s", r)
	default:
	}
	af, _ := m.Get("af")
	t.Logf("af = %v", af)
	pos, _ := m.Get("time-pos")
	if p, ok := pos.(float64); !ok || p <= 0 || p > 1.2 {
		t.Fatalf("not playing in the A-B loop: time-pos %v", pos)
	}
}
