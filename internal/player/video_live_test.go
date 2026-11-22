//go:build live

package player

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLiveYouTubeVideoAdd plays a YouTube song's audio, then adds its video
// stream: the music must keep going and the video must start.
func TestLiveYouTubeVideoAdd(t *testing.T) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("no yt-dlp")
	}
	page := "https://www.youtube.com/watch?v=b9Y4TACmvE8"
	out, err := exec.Command("yt-dlp", "--ignore-config", "--no-warnings", "-f", "bestaudio", "-g", page).Output()
	if err != nil {
		t.Fatal(err)
	}
	audio := strings.TrimSpace(string(out))
	out, err = exec.Command("yt-dlp", "--ignore-config", "--no-warnings", "-f", "bestvideo[height<=?480][vcodec!=none]", "-g", page).Output()
	if err != nil {
		t.Fatal(err)
	}
	video := strings.TrimSpace(string(out))
	m, err := Start(t.TempDir(), Options{Extra: []string{"--ao=null", "--vo=null"}}, Events{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Load(audio, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && posOf(m) < 1; i++ {
		time.Sleep(250 * time.Millisecond)
	}
	before := posOf(m)
	t0 := time.Now()
	if err := m.AddVideo(video); err != nil {
		t.Fatal(err)
	}
	ok := false
	for i := 0; i < 80 && !ok; i++ {
		time.Sleep(250 * time.Millisecond)
		ok = m.VideoPlaying()
	}
	after := posOf(m)
	t.Logf("audio %.1fs → %.1fs while the video loaded in %v (playing=%v)", before, after, time.Since(t0).Round(time.Millisecond), ok)
	if !ok || after <= before {
		t.Fatal("video did not start, or the music stopped")
	}
}
