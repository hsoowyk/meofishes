// Package lyrics parses the lyric formats used by the providers (TTML, LRC,
// enhanced LRC and Kugou KRC) into core.Lyrics.
package lyrics

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"meofishes/internal/core"
)

// Parse detects the format and parses it.
func Parse(format, content string) (*core.Lyrics, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("empty lyrics")
	}
	switch strings.ToLower(format) {
	case "ttml", "xml":
		return ParseTTML(content)
	case "krc":
		return ParseKRC(content)
	case "lrc", "elrc":
		return ParseLRC(content), nil
	case "plain", "txt":
		return ParsePlain(content), nil
	}
	switch {
	case strings.HasPrefix(content, "<"):
		return ParseTTML(content)
	case krcLine.MatchString(content):
		return ParseKRC(content)
	case lrcStamp.MatchString(content):
		return ParseLRC(content), nil
	}
	return ParsePlain(content), nil
}

// ParsePlain turns unsynced text into lines without timing.
func ParsePlain(s string) *core.Lyrics {
	l := &core.Lyrics{}
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimRight(ln, "\r ")
		if ln == "" {
			continue
		}
		l.Lines = append(l.Lines, core.Line{Text: ln})
	}
	l.Normalize()
	l.Sync = core.SyncNone
	return l
}

// ---------------------------------------------------------------- time parsing

// ParseClock understands "83.4", "83.4s", "1:23.4", "00:01:23.456", "1234ms".
func ParseClock(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.HasSuffix(s, "ms") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "ms"), 64)
		return int64(v), err == nil
	}
	s = strings.TrimSuffix(s, "s")
	parts := strings.Split(s, ":")
	var total float64
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + v
	}
	return int64(total*1000 + 0.5), true
}

// ---------------------------------------------------------------- TTML

// ParseTTML parses Apple-style TTML (used by BetterLyrics, lrc.red, Unison,
// Paxsenix), supporting word/syllable spans, background vocals and agents.
func ParseTTML(src string) (*core.Lyrics, error) {
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	out := &core.Lyrics{}
	var (
		cur        *core.Line
		inHead     int
		spanDepth  int   // nesting of spans inside <p>
		timedAt    []int // depths of timed spans
		bgAt       = -1  // depth of the x-bg span
		skipAt     = -1  // depth of a translation/romanization span
		word       *core.Word
		spaceSeen  = true
		firstAgent string
	)
	attr := func(se xml.StartElement, name string) string {
		for _, a := range se.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	pushWord := func() {
		if word == nil {
			return
		}
		txt := word.Text
		trailing := strings.HasSuffix(txt, " ") || strings.HasSuffix(txt, " ")
		leading := strings.HasPrefix(txt, " ")
		word.Text = strings.TrimSpace(txt)
		if word.Text != "" {
			target := &cur.Words
			if bgAt >= 0 {
				target = &cur.Background
			}
			word.Part = len(*target) > 0 && !spaceSeen && !leading
			*target = append(*target, *word)
		}
		spaceSeen = trailing
		word = nil
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "head":
				inHead++
			case "p":
				if inHead > 0 {
					continue
				}
				b, _ := ParseClock(attr(t, "begin"))
				e, _ := ParseClock(attr(t, "end"))
				agent := attr(t, "agent")
				if firstAgent == "" && agent != "" {
					firstAgent = agent
				}
				cur = &core.Line{Start: b, End: e, Agent: agent}
				cur.Opposite = agent != "" && agent != firstAgent && agent != "v1000"
				spanDepth, timedAt, bgAt, skipAt, spaceSeen = 0, nil, -1, -1, true
			case "span":
				if cur == nil {
					continue
				}
				spanDepth++
				role := attr(t, "role")
				switch role {
				case "x-bg":
					bgAt = spanDepth
					spaceSeen = true
				case "x-translation", "x-roman", "x-romanization":
					skipAt = spanDepth
				}
				if skipAt >= 0 {
					continue
				}
				if b, ok := ParseClock(attr(t, "begin")); ok && role != "x-bg" {
					e, _ := ParseClock(attr(t, "end"))
					pushWord()
					word = &core.Word{Start: b, End: e}
					timedAt = append(timedAt, spanDepth)
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "head":
				inHead--
			case "p":
				if cur == nil {
					continue
				}
				pushWord()
				cur.Text = strings.TrimSpace(cur.Text)
				if len(cur.Words) > 0 {
					cur.Text = core.JoinWords(cur.Words)
				}
				if cur.Text != "" || len(cur.Background) > 0 {
					out.Lines = append(out.Lines, *cur)
				}
				cur = nil
			case "span":
				if cur == nil {
					continue
				}
				if n := len(timedAt); n > 0 && timedAt[n-1] == spanDepth {
					pushWord()
					timedAt = timedAt[:n-1]
				}
				if bgAt == spanDepth {
					bgAt = -1
					spaceSeen = true
				}
				if skipAt == spanDepth {
					skipAt = -1
				}
				spanDepth--
			}
		case xml.CharData:
			if cur == nil || inHead > 0 || skipAt >= 0 {
				continue
			}
			s := string(t)
			if word != nil {
				word.Text += s
				continue
			}
			if strings.TrimSpace(s) == "" {
				if s != "" {
					spaceSeen = true
				}
				continue
			}
			if bgAt < 0 {
				cur.Text += s
			}
		}
	}
	if len(out.Lines) == 0 {
		return nil, errors.New("ttml has no lines")
	}
	out.Normalize()
	return out, nil
}

// ---------------------------------------------------------------- LRC

var (
	lrcStamp = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2}(?:[.:]\d{1,3})?)\]`)
	lrcWord  = regexp.MustCompile(`<(\d{1,3}):(\d{1,2}(?:[.:]\d{1,3})?)>`)
	lrcMeta  = regexp.MustCompile(`^\[([a-zA-Z#]+):(.*)\]$`)
)

func stampMs(min, sec string) int64 {
	m, _ := strconv.ParseInt(min, 10, 64)
	sec = strings.Replace(sec, ":", ".", 1)
	s, _ := strconv.ParseFloat(sec, 64)
	return m*60000 + int64(s*1000+0.5)
}

// ParseLRC parses plain and enhanced (word-timed) LRC.
func ParseLRC(src string) *core.Lyrics {
	out := &core.Lyrics{}
	var offset int64
	for _, raw := range strings.Split(src, "\n") {
		raw = strings.TrimRight(raw, "\r")
		if m := lrcMeta.FindStringSubmatch(strings.TrimSpace(raw)); m != nil && !lrcStamp.MatchString(raw) {
			if strings.EqualFold(m[1], "offset") {
				offset, _ = strconv.ParseInt(strings.TrimSpace(m[2]), 10, 64)
			}
			continue
		}
		stamps := lrcStamp.FindAllStringSubmatchIndex(raw, -1)
		if len(stamps) == 0 {
			continue
		}
		// leading consecutive stamps, e.g. [00:10.00][01:20.00]text
		var starts []int64
		pos := 0
		for _, s := range stamps {
			if s[0] != pos {
				break
			}
			starts = append(starts, stampMs(raw[s[2]:s[3]], raw[s[4]:s[5]]))
			pos = s[1]
		}
		body := raw[pos:]
		agent := ""
		if strings.HasPrefix(body, "v1:") || strings.HasPrefix(body, "v2:") {
			agent, body = body[:2], body[3:]
		}
		words := parseLRCWords(body)
		text := strings.TrimSpace(lrcWord.ReplaceAllString(body, ""))
		for _, st := range starts {
			ln := core.Line{Start: st - offset, Text: text, Agent: agent, Opposite: agent == "v2"}
			for _, w := range words {
				w.Start -= offset
				w.End -= offset
				ln.Words = append(ln.Words, w)
			}
			out.Lines = append(out.Lines, ln)
		}
	}
	sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[i].Start < out.Lines[j].Start })
	// drop empty spacer lines that only mark the end of the previous line
	clean := out.Lines[:0]
	for i, ln := range out.Lines {
		if ln.Text == "" {
			if len(clean) > 0 && clean[len(clean)-1].End == 0 {
				clean[len(clean)-1].End = ln.Start
			}
			_ = i
			continue
		}
		clean = append(clean, ln)
	}
	out.Lines = clean
	out.Normalize()
	return out
}

func parseLRCWords(body string) []core.Word {
	idx := lrcWord.FindAllStringSubmatchIndex(body, -1)
	if len(idx) == 0 {
		return nil
	}
	var ws []core.Word
	for i, m := range idx {
		start := stampMs(body[m[2]:m[3]], body[m[4]:m[5]])
		end := len(body)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		raw := body[m[1]:end]
		if len(ws) > 0 && ws[len(ws)-1].End == 0 {
			ws[len(ws)-1].End = start
		}
		if strings.TrimSpace(raw) == "" {
			// a closing stamp: <00:12.30> at the end of the line
			continue
		}
		w := core.Word{Start: start, Text: strings.TrimSpace(raw)}
		w.Part = len(ws) > 0 && !strings.HasPrefix(raw, " ") && !strings.HasSuffix(body[:m[0]], " ")
		ws = append(ws, w)
	}
	return ws
}

// ---------------------------------------------------------------- KRC

var (
	krcLine = regexp.MustCompile(`(?m)^\[(\d+),(\d+)\]`)
	krcWord = regexp.MustCompile(`<(\d+),(\d+),\d+>`)
	krcKey  = []byte{0x40, 0x47, 0x61, 0x77, 0x5e, 0x32, 0x74, 0x47, 0x51, 0x36, 0x31, 0x2d, 0xce, 0xd2, 0x6e, 0x69}
)

// DecodeKRC decrypts the base64 "krc1" payload Kugou returns.
func DecodeKRC(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	if len(raw) < 4 || string(raw[:4]) != "krc1" {
		return "", errors.New("not a krc payload")
	}
	data := raw[4:]
	for i := range data {
		data[i] ^= krcKey[i%len(krcKey)]
	}
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(string(out), "\ufeff"), nil
}

// ParseKRC parses decrypted Kugou KRC text.
func ParseKRC(src string) (*core.Lyrics, error) {
	out := &core.Lyrics{}
	for _, raw := range strings.Split(src, "\n") {
		raw = strings.TrimRight(raw, "\r")
		m := krcLine.FindStringSubmatchIndex(raw)
		if m == nil {
			continue
		}
		start, _ := strconv.ParseInt(raw[m[2]:m[3]], 10, 64)
		dur, _ := strconv.ParseInt(raw[m[4]:m[5]], 10, 64)
		body := raw[m[1]:]
		ln := core.Line{Start: start, End: start + dur}
		idx := krcWord.FindAllStringSubmatchIndex(body, -1)
		for i, w := range idx {
			off, _ := strconv.ParseInt(body[w[2]:w[3]], 10, 64)
			wd, _ := strconv.ParseInt(body[w[4]:w[5]], 10, 64)
			end := len(body)
			if i+1 < len(idx) {
				end = idx[i+1][0]
			}
			txt := body[w[1]:end]
			if strings.TrimSpace(txt) == "" {
				continue
			}
			word := core.Word{Start: start + off, End: start + off + wd, Text: strings.TrimSpace(txt)}
			if len(ln.Words) > 0 {
				prev := body[idx[i-1][1]:idx[i][0]]
				word.Part = !strings.HasPrefix(txt, " ") && !strings.HasSuffix(prev, " ") && !isCJK(word.Text)
				if isCJK(word.Text) {
					word.Part = true
				}
			}
			ln.Words = append(ln.Words, word)
		}
		if len(idx) == 0 {
			ln.Text = strings.TrimSpace(body)
		}
		if len(ln.Words) > 0 || ln.Text != "" {
			out.Lines = append(out.Lines, ln)
		}
	}
	if len(out.Lines) == 0 {
		return nil, errors.New("krc has no lines")
	}
	out.Normalize()
	return out, nil
}

func isCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}
