package rerank

import (
	"strconv"
	"strings"
	"testing"
)

func TestSupportsTexts(t *testing.T) {
	chars := map[rune]bool{}
	for _, c := range "mp3允格式我下載了" {
		chars[c] = true
	}
	many := make([]string, MaxTexts+1)
	for i := range many {
		many[i] = strconv.Itoa(i)
	}
	for _, c := range []struct {
		name  string
		r     TextsRequest
		chars map[rune]bool
		ok    bool
	}{
		{"ok", TextsRequest{Context: "我下載了", Texts: []string{"mp3格式", "允格式"}}, chars, true},
		{"no vocab check", TextsRequest{Texts: []string{"zip", "分"}}, nil, true},
		{"one text", TextsRequest{Texts: []string{"mp3"}}, chars, false},
		{"too many", TextsRequest{Texts: many}, nil, false},
		{"duplicate", TextsRequest{Texts: []string{"允", "允"}}, chars, false},
		{"empty text", TextsRequest{Texts: []string{"", "允"}}, chars, false},
		{"too long", TextsRequest{Texts: []string{strings.Repeat("允", MaxTextRunes+1), "允"}}, chars, false},
		{"out of vocab", TextsRequest{Texts: []string{"zip", "允"}}, chars, false},
		{"long context", TextsRequest{Context: strings.Repeat("我", MaxContextRunes+1), Texts: []string{"mp3", "允"}}, chars, false},
	} {
		if got := (Options{Chars: c.chars}).SupportsTexts(c.r); got != c.ok {
			t.Errorf("%s: SupportsTexts = %v", c.name, got)
		}
	}
}
