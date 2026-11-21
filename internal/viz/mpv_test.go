package viz

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestAnalyzeWithMpv(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("no mpv")
	}
	s := Analyze(context.Background(), "av://lavfi:sine=frequency=1000:duration=3", nil, 32)
	for i := 0; i < 100; i++ {
		if _, done, _ := s.Progress(); done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	ms, done, err := s.Progress()
	if !done || err != nil || ms < 2800 {
		t.Fatalf("progress %d %v %v", ms, done, err)
	}
	if f := s.Frame(1500); len(f) != 32 {
		t.Fatalf("frame %v", f)
	}
}
