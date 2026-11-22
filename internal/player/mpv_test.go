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
	tech := m.Tech()
	t.Logf("tech %q badge %q", tech.String(), tech.Badge())
	if tech.Codec == "" || tech.SampleRate != 44100 {
		t.Errorf("tech info: %+v", tech)
	}
	af, _ := m.Get("af")
	t.Logf("af = %v", af)
	pos, _ := m.Get("time-pos")
	if a, _ := m.Get("ab-loop-a"); a != 0.2 {
		t.Errorf("ab-loop-a = %v", a)
	}
	// the generated tone cannot seek exactly: the loop goes back to 0
	if p, ok := pos.(float64); !ok || p < 0 || p > 1.2 {
		t.Fatalf("not playing in the A-B loop: time-pos %v", pos)
	}
}

// TestAddVideoKeepsAudio adds a video while audio plays: the audio must not
// pause or restart, and the video must be reported as playing.
func TestAddVideoKeepsAudio(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not installed")
	}
	m, err := Start(t.TempDir(), Options{Extra: []string{"--ao=null", "--vo=null"}}, Events{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Load("av://lavfi:aevalsrc=sin(440*2*PI*t):s=44100:d=60", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	before := posOf(m)
	if err := m.AddVideo("av://lavfi:testsrc=size=320x240:rate=25:d=60"); err != nil {
		t.Fatal(err)
	}
	ok := false
	for i := 0; i < 20 && !ok; i++ {
		time.Sleep(150 * time.Millisecond)
		ok = m.VideoPlaying()
	}
	after := posOf(m)
	paused, _ := m.Get("pause")
	t.Logf("pos %.2f → %.2f, video playing %v, pause %v", before, after, ok, paused)
	if !ok {
		t.Error("video not reported as playing")
	}
	if after < before || paused == true {
		t.Errorf("audio restarted or paused: %.2f → %.2f pause=%v", before, after, paused)
	}
	if err := m.RemoveVideo(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if m.VideoPlaying() {
		t.Error("video still on")
	}
	if p := posOf(m); p < after {
		t.Errorf("audio restarted after removing the video: %.2f", p)
	}
}

func posOf(m *MPV) float64 {
	v, _ := m.Get("time-pos")
	f, _ := v.(float64)
	return f
}
