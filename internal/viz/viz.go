// Package viz computes the frequency bands of a track for the visualizer
// shown when a song has no lyrics. A second, silent mpv decodes the track
// (through the local chunk proxy, so nothing is downloaded twice) to mono
// 11 kHz PCM; every 100 ms of audio becomes one frame of N log-spaced bands.
package viz

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"math"
	"math/cmplx"
	"os/exec"
	"sync"
)

const (
	Rate    = 11025
	FrameMs = 100
	fftSize = 2048
)

// Spectrum is filled progressively while the track is decoded.
type Spectrum struct {
	Bands  int
	mu     sync.RWMutex
	frames [][]float32
	done   bool
	err    error
}

// Frame returns the bands at a position (nil when not decoded yet).
func (s *Spectrum) Frame(posMs int64) []float32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := int(posMs / FrameMs)
	if i < 0 || i >= len(s.frames) {
		return nil
	}
	return s.frames[i]
}

// Progress returns the decoded length and whether decoding finished.
func (s *Spectrum) Progress() (ms int64, done bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.frames)) * FrameMs, s.done, s.err
}

// Analyze starts decoding url with mpv. Cancel ctx to stop.
func Analyze(ctx context.Context, url string, headers map[string]string, bands int) *Spectrum {
	s := &Spectrum{Bands: bands}
	go func() {
		err := s.run(ctx, url, headers)
		s.mu.Lock()
		s.done, s.err = true, err
		s.mu.Unlock()
	}()
	return s
}

func (s *Spectrum) run(ctx context.Context, url string, headers map[string]string) error {
	args := []string{"--no-config", "--really-quiet", "--no-video", "--untimed", "--ao=pcm",
		"--ao-pcm-file=/dev/stdout", "--ao-pcm-waveheader=no", "--audio-format=s16",
		"--audio-channels=mono", "--audio-samplerate=11025", "--ytdl-format=bestaudio/best"}
	if len(headers) > 0 {
		hs := ""
		for k, v := range headers {
			if hs != "" {
				hs += ","
			}
			hs += k + ": " + v
		}
		args = append(args, "--http-header-fields="+hs)
	}
	cmd := exec.CommandContext(ctx, "mpv", append(args, url)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	err = s.consume(bufio.NewReaderSize(out, 64<<10))
	_ = cmd.Wait()
	return err
}

// consume reads s16 mono PCM and appends one frame per 100 ms.
func (s *Spectrum) consume(r io.Reader) error {
	hop := Rate * FrameMs / 1000 // samples per frame
	win := make([]float64, fftSize)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)) // Hann
	}
	edges := bandEdges(s.Bands)
	ring := make([]float64, fftSize)
	buf := make([]byte, hop*2)
	filled := 0
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		copy(ring, ring[hop:])
		for i := 0; i < hop; i++ {
			ring[fftSize-hop+i] = float64(int16(binary.LittleEndian.Uint16(buf[2*i:]))) / 32768
		}
		filled += hop
		frame := Bands(ring, win, edges)
		s.mu.Lock()
		s.frames = append(s.frames, frame)
		s.mu.Unlock()
	}
}

// bandEdges splits 40 Hz–5 kHz into n log-spaced bands (FFT bin indexes).
func bandEdges(n int) []int {
	lo, hi := 40.0, 5000.0
	edges := make([]int, n+1)
	for i := 0; i <= n; i++ {
		f := lo * math.Pow(hi/lo, float64(i)/float64(n))
		b := int(f / Rate * fftSize)
		if i > 0 && b <= edges[i-1] {
			b = edges[i-1] + 1
		}
		edges[i] = b
	}
	return edges
}

// Bands computes normalized band levels (0..1) of one window of samples.
func Bands(samples, win []float64, edges []int) []float32 {
	x := make([]complex128, fftSize)
	for i := range x {
		x[i] = complex(samples[i]*win[i], 0)
	}
	fft(x)
	n := len(edges) - 1
	out := make([]float32, n)
	for b := 0; b < n; b++ {
		avg := 0.0 // the peak of the band (averaging hides tones in wide bands)
		for k := edges[b]; k < edges[b+1] && k < fftSize/2; k++ {
			avg = math.Max(avg, cmplx.Abs(x[k]))
		}
		// dB → 0..1 (−60 dB … 0 dB relative to a full-scale sine)
		db := 20 * math.Log10(avg/(fftSize/4)+1e-9)
		out[b] = float32(math.Max(0, math.Min(1, (db+60)/60)))
	}
	return out
}

// fft is an in-place radix-2 Cooley–Tukey transform (len must be a power of 2).
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[start+k], a[start+k+size/2]*wk
				a[start+k], a[start+k+size/2] = u+v, u-v
				wk *= w
			}
		}
	}
}
