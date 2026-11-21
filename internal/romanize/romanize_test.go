package romanize

import "testing"

func TestText(t *testing.T) {
	for in, want := range map[string]string{
		"こんにちは":      "konnichiha",
		"きっと":        "kitto",
		"ラーメン":       "raamen",
		"しゃしん":       "shashin",
		"いっしょに":      "isshoni",
		"マッチ":        "matchi",
		"안녕하세요":      "annyeonghaseyo",
		"사랑해":        "saranghae",
		"한국어":        "hangugeo",
		"Привет мир": "Privet mir",
		"夢 ゆめ":       "夢 yume",
		"hello":      "hello",
	} {
		if got := Text(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if Needed("hello") || !Needed("ありがとう") || !Needed("Привет") {
		t.Fatal("Needed")
	}
}
