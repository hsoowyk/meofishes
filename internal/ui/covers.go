package ui

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qt "github.com/mappu/miqt/qt"
	"github.com/mappu/miqt/qt/mainthread"

	"meofishes/internal/core"
	"meofishes/internal/theme"
)

// CoverLoader downloads and caches cover images.
type CoverLoader struct {
	dir     string
	ua      func() string
	client  *http.Client
	mu      sync.Mutex
	loading map[string]bool
	// UI thread only:
	pix     map[string]*qt.QPixmap
	order   []string
	targets map[*CoverView]string
	waiting map[string][]func(*qt.QPixmap)
}

func newCoverLoader(dir string, ua func() string) *CoverLoader {
	_ = os.MkdirAll(dir, 0o755)
	return &CoverLoader{dir: dir, ua: ua, client: &http.Client{Timeout: 20 * time.Second},
		loading: map[string]bool{}, pix: map[string]*qt.QPixmap{}, targets: map[*CoverView]string{}, waiting: map[string][]func(*qt.QPixmap){}}
}

func (l *CoverLoader) path(url string) string {
	h := sha1.Sum([]byte(url))
	return filepath.Join(l.dir, hex.EncodeToString(h[:]))
}

// Into shows url in a CoverView when it is ready.
func (l *CoverLoader) Into(url string, c *CoverView) {
	l.targets[c] = url
	if url == "" {
		c.SetPixmap(nil)
		return
	}
	l.Get(url, func(px *qt.QPixmap) {
		if l.targets[c] == url {
			c.SetPixmap(px)
		}
	})
}

// Forget stops delivering images to a deleted CoverView.
func (l *CoverLoader) Forget(c *CoverView) { delete(l.targets, c) }

// Get calls f on the UI thread with the pixmap (nil when it failed).
func (l *CoverLoader) Get(url string, f func(*qt.QPixmap)) {
	if px, ok := l.pix[url]; ok {
		f(px)
		return
	}
	l.waiting[url] = append(l.waiting[url], f)
	l.mu.Lock()
	busy := l.loading[url]
	l.loading[url] = true
	l.mu.Unlock()
	if busy {
		return
	}
	go func() {
		data := l.fetch(url)
		mainthread.Start(func() {
			l.mu.Lock()
			delete(l.loading, url)
			l.mu.Unlock()
			var px *qt.QPixmap
			if len(data) > 0 {
				px = qt.NewQPixmap()
				if !px.LoadFromDataWithData(data) || px.IsNull() {
					px.Delete()
					px = nil
				} else if px.Width() > 640 {
					small := px.Scaled3(640, 640, qt.KeepAspectRatio, qt.SmoothTransformation)
					px.Delete()
					px = qt.NewQPixmap5(small)
				}
			}
			if px != nil {
				l.pix[url] = px
				l.order = append(l.order, url)
				if len(l.order) > 400 { // simple LRU-ish cap
					old := l.order[0]
					l.order = l.order[1:]
					if p := l.pix[old]; p != nil {
						delete(l.pix, old)
						// targets keep their own copies, so this is safe
						p.Delete()
					}
				}
			}
			for _, cb := range l.waiting[url] {
				cb(px)
			}
			delete(l.waiting, url)
		})
	}()
}

// Prefetch warms the disk cache for a list of tracks.
func (l *CoverLoader) Prefetch(ts []core.Track) {}

func (l *CoverLoader) fetch(url string) []byte {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		data, _ := os.ReadFile(strings.TrimPrefix(url, "file://"))
		return data
	}
	p := l.path(url)
	if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
		return data
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", l.ua())
	resp, err := l.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	_ = os.WriteFile(p, data, 0o644)
	return data
}

// dominantColor picks a vivid average color of a pixmap.
func dominantColor(px *qt.QPixmap) (theme.Color, bool) {
	if px == nil || px.IsNull() {
		return theme.Color{}, false
	}
	small := px.Scaled3(24, 24, qt.IgnoreAspectRatio, qt.SmoothTransformation)
	img := small.ToImage()
	var r, g, b, wsum float64
	for y := 0; y < img.Height(); y++ {
		for x := 0; x < img.Width(); x++ {
			c := img.PixelColor(x, y)
			col := fromQ(c)
			_, s, li := col.HSL()
			w := 0.15 + s*s*(1-abs(li-0.5)*1.6) // prefer saturated mid-tones
			r += col.R * w
			g += col.G * w
			b += col.B * w
			wsum += w
		}
	}
	if wsum == 0 {
		return theme.Color{}, false
	}
	return theme.Color{R: r / wsum, G: g / wsum, B: b / wsum, A: 1}, true
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
