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
		{Wid: 12345, X11Video: true, Normalize: true, AudioFmt: "bestaudio/best", EQ: EQPresets["rock"], Speed: 1.25},
	} {
		m, err := Start(t.TempDir(), opt, Events{})
		if err != nil {
			t.Fatalf("start %+v: %v", opt, err)
		}
		if err := m.SetVolume(50); err != nil {
			t.Fatalf("command: %v", err)
		}
		if err := m.SetFilters(Filters(true, EQPresets["bass"])); err != nil {
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
