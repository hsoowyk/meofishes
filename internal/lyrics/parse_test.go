package lyrics

import (
	"testing"

	"meofishes/internal/core"
)

func TestTTMLWordsSyllablesAndBackground(t *testing.T) {
	src := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" xmlns:itunes="http://music.apple.com/lyric-ttml-internal">
<head><metadata><iTunesMetadata><translations><translation><text for="L1">ignored</text></translation></translations></iTunesMetadata></metadata></head>
<body><div>
<p begin="1.000" end="3.000" ttm:agent="v1"><span begin="1.000" end="1.500">Hello</span> <span begin="1.500" end="1.800">beau</span><span begin="1.800" end="2.200">tiful</span> <span ttm:role="x-bg"><span begin="2.200" end="3.000">(ooh)</span></span></p>
<p begin="00:01:02.500" end="00:01:04.000" ttm:agent="v2">Line synced only</p>
</div></body></tt>`
	l, err := ParseTTML(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Lines) != 2 {
		t.Fatalf("lines = %d", len(l.Lines))
	}
	a := l.Lines[0]
	if a.Text != "Hello beautiful" {
		t.Errorf("text = %q", a.Text)
	}
	if len(a.Words) != 3 || !a.Words[2].Part || a.Words[1].Part {
		t.Errorf("words = %+v", a.Words)
	}
	if len(a.Background) != 1 || a.Background[0].Text != "(ooh)" {
		t.Errorf("bg = %+v", a.Background)
	}
	b := l.Lines[1]
	if b.Start != 62500 || b.Text != "Line synced only" || !b.Opposite {
		t.Errorf("line2 = %+v", b)
	}
	if l.Sync != core.SyncWord {
		t.Errorf("sync = %v", l.Sync)
	}
}

func TestEnhancedLRC(t *testing.T) {
	l := ParseLRC("[ar:x]\n[00:01.00]<00:01.00>Hel<00:01.30>lo <00:01.60>world<00:02.00>\n[00:03.00]plain line\n")
	if len(l.Lines) != 2 {
		t.Fatalf("lines = %d", len(l.Lines))
	}
	w := l.Lines[0].Words
	if len(w) != 3 || w[0].Text != "Hel" || !w[1].Part || w[2].Part || w[2].End != 2000 {
		t.Errorf("words = %+v", w)
	}
	if l.Lines[0].Text != "Hello world" {
		t.Errorf("text = %q", l.Lines[0].Text)
	}
}

func TestKRC(t *testing.T) {
	l, err := ParseKRC("[id:1]\n[1000,2000]<0,500,0>I <500,500,0>love <1000,1000,0>you\n[3000,1000]<0,300,0>你<300,700,0>好\n")
	if err != nil {
		t.Fatal(err)
	}
	if l.Lines[0].Text != "I love you" || l.Lines[0].Words[2].Start != 2000 {
		t.Errorf("line0 = %+v", l.Lines[0])
	}
	if l.Lines[1].Text != "你好" {
		t.Errorf("line1 = %q", l.Lines[1].Text)
	}
}

func TestClock(t *testing.T) {
	for in, want := range map[string]int64{"83.4": 83400, "1:23.4": 83400, "00:01:23.456": 83456, "12.5s": 12500, "250ms": 250} {
		if got, ok := ParseClock(in); !ok || got != want {
			t.Errorf("%s = %d", in, got)
		}
	}
}
