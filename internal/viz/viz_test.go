package viz

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestSineBand(t *testing.T) {
	// 1 s of a 440 Hz sine: the band containing 440 Hz must be the loudest
	var buf bytes.Buffer
	for i := 0; i < Rate; i++ {
		v := int16(0.8 * 32767 * math.Sin(2*math.Pi*440*float64(i)/Rate))
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	s := &Spectrum{Bands: 24}
	if err := s.consume(&buf); err != nil {
		t.Fatal(err)
	}
	if len(s.frames) != 10 {
		t.Fatalf("frames: %d", len(s.frames))
	}
	f := s.frames[9]
	edges := bandEdges(24)
	best := 0
	for i := range f {
		if f[i] > f[best] {
			best = i
		}
	}
	hz := 440.0
	bin := int(hz / Rate * fftSize)
	if bin < edges[best] || bin >= edges[best+1] {
		t.Fatalf("loudest band %d (%d-%d), 440 Hz is bin %d; %v", best, edges[best], edges[best+1], bin, f)
	}
	if f[best] < 0.7 {
		t.Fatalf("level too low: %v", f[best])
	}
}
