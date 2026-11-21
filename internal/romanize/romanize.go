// Package romanize writes lyrics in Latin letters: Japanese kana (Hepburn),
// Korean hangul (Revised Romanization) and Cyrillic. Kanji and Chinese
// characters are left as they are (they need a dictionary).
package romanize

import (
	"strings"
	"unicode"
)

// Needed reports whether s has something this package can romanize.
func Needed(s string) bool {
	for _, r := range s {
		if isKana(r) || isHangul(r) || isCyr(r) {
			return true
		}
	}
	return false
}

// Text romanizes s.
func Text(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case isKana(r):
			n := kana(rs[i:], &b)
			i += n - 1
		case isHangul(r):
			b.WriteString(hangul(r, next(rs, i)))
		case isCyr(r):
			c, ok := cyr[unicode.ToLower(r)]
			if !ok {
				b.WriteRune(r)
			} else if c != "" && unicode.IsUpper(r) {
				b.WriteString(strings.ToUpper(c[:1]) + c[1:])
			} else {
				b.WriteString(c)
			}
		case r == '、' || r == '，':
			b.WriteString(", ")
		case r == '。':
			b.WriteString(". ")
		case r == '　':
			b.WriteByte(' ')
		case r == '「' || r == '」' || r == '『' || r == '』':
			b.WriteByte('"')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func next(rs []rune, i int) rune {
	if i+1 < len(rs) {
		return rs[i+1]
	}
	return 0
}

// ---------------------------------------------------------------- kana

func isKana(r rune) bool { return (r >= 0x3041 && r <= 0x3096) || (r >= 0x30A1 && r <= 0x30FC) }

// toHira maps katakana to hiragana.
func toHira(r rune) rune {
	if r >= 0x30A1 && r <= 0x30F6 {
		return r - 0x60
	}
	return r
}

var kanaTab = map[string]string{
	"あ": "a", "い": "i", "う": "u", "え": "e", "お": "o",
	"か": "ka", "き": "ki", "く": "ku", "け": "ke", "こ": "ko",
	"さ": "sa", "し": "shi", "す": "su", "せ": "se", "そ": "so",
	"た": "ta", "ち": "chi", "つ": "tsu", "て": "te", "と": "to",
	"な": "na", "に": "ni", "ぬ": "nu", "ね": "ne", "の": "no",
	"は": "ha", "ひ": "hi", "ふ": "fu", "へ": "he", "ほ": "ho",
	"ま": "ma", "み": "mi", "む": "mu", "め": "me", "も": "mo",
	"や": "ya", "ゆ": "yu", "よ": "yo",
	"ら": "ra", "り": "ri", "る": "ru", "れ": "re", "ろ": "ro",
	"わ": "wa", "ゐ": "i", "ゑ": "e", "を": "o", "ん": "n",
	"が": "ga", "ぎ": "gi", "ぐ": "gu", "げ": "ge", "ご": "go",
	"ざ": "za", "じ": "ji", "ず": "zu", "ぜ": "ze", "ぞ": "zo",
	"だ": "da", "ぢ": "ji", "づ": "zu", "で": "de", "ど": "do",
	"ば": "ba", "び": "bi", "ぶ": "bu", "べ": "be", "ぼ": "bo",
	"ぱ": "pa", "ぴ": "pi", "ぷ": "pu", "ぺ": "pe", "ぽ": "po",
	"ぁ": "a", "ぃ": "i", "ぅ": "u", "ぇ": "e", "ぉ": "o", "ゔ": "vu",
	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo", "しゃ": "sha", "しゅ": "shu", "しょ": "sho", "しぇ": "she",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho", "ちぇ": "che", "にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo", "みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo", "ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo", "じぇ": "je", "びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo", "ふぁ": "fa", "ふぃ": "fi", "ふぇ": "fe", "ふぉ": "fo",
	"てぃ": "ti", "でぃ": "di", "うぃ": "wi", "うぇ": "we", "うぉ": "wo", "ゔぁ": "va", "ゔぃ": "vi", "ゔぇ": "ve", "ゔぉ": "vo",
}

// kana writes the romaji of the kana at the start of rs; returns runes used.
func kana(rs []rune, b *strings.Builder) int {
	r := toHira(rs[0])
	switch r {
	case 'ー': // long vowel: repeat the last vowel
		s := b.String()
		if n := len(s); n > 0 && strings.ContainsRune("aiueo", rune(s[n-1])) {
			b.WriteByte(s[n-1])
		}
		return 1
	case 'っ': // small tsu doubles the next consonant
		if len(rs) > 1 && isKana(rs[1]) {
			var nb strings.Builder
			kana(rs[1:], &nb)
			if c := nb.String(); c != "" && !strings.ContainsRune("aiueon", rune(c[0])) {
				if strings.HasPrefix(c, "ch") {
					b.WriteByte('t')
				} else {
					b.WriteByte(c[0])
				}
			}
		}
		return 1
	}
	if len(rs) > 1 {
		if s, ok := kanaTab[string([]rune{r, toHira(rs[1])})]; ok {
			b.WriteString(s)
			return 2
		}
	}
	if s, ok := kanaTab[string(r)]; ok {
		if r == 'ん' && len(rs) > 1 {
			if n := toHira(rs[1]); kanaTab[string(n)] != "" && strings.ContainsRune("aiueoy", rune(kanaTab[string(n)][0])) {
				b.WriteString("n'")
				return 1
			}
		}
		b.WriteString(s)
		return 1
	}
	b.WriteRune(rs[0])
	return 1
}

// ---------------------------------------------------------------- hangul

func isHangul(r rune) bool { return r >= 0xAC00 && r <= 0xD7A3 }

var (
	hInitial = []string{"g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "ch", "k", "t", "p", "h"}
	hVowel   = []string{"a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe", "yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i"}
	hFinal   = []string{"", "k", "k", "k", "n", "n", "n", "t", "l", "k", "m", "l", "l", "l", "p", "l", "m", "p", "p", "t", "t", "ng", "t", "t", "k", "t", "p", "t"}
	// before a vowel the final consonant is pronounced as the next initial
	hFinalLink = []string{"", "g", "kk", "ks", "n", "nj", "nh", "d", "r", "lg", "lm", "lb", "ls", "lt", "lp", "lh", "m", "b", "ps", "s", "ss", "ng", "j", "ch", "k", "t", "p", "h"}
)

func hangul(r, nextR rune) string {
	i := int(r - 0xAC00)
	ini, vow, fin := i/588, (i%588)/28, i%28
	s := hInitial[ini] + hVowel[vow]
	if fin == 0 {
		return s
	}
	if isHangul(nextR) && int(nextR-0xAC00)/588 == 11 { // next syllable starts with ㅇ (silent)
		return s + hFinalLink[fin]
	}
	return s + hFinal[fin]
}

// ---------------------------------------------------------------- cyrillic

var cyr = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "yu", 'я': "ya", 'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g", 'ў': "w",
}

func isCyr(r rune) bool { return r >= 0x400 && r <= 0x4FF }
