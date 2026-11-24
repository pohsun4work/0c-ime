package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// hostText 模擬宿主收到的文字：送出的字加上交還宿主的按鍵，最後一段是還在組字區的文字。
func hostText(s *Session, keys ...string) (string, proto.Snapshot) {
	var out string
	var snap proto.Snapshot
	for _, k := range keys {
		snap = key(s, k)
		out += snap.Commit
		if !snap.Handled && len([]rune(k)) == 1 {
			out += k
		}
	}
	return out, snap
}

func chars(s string) []string {
	var out []string
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

func englishWordsSession() *Session {
	s := newSession()
	s.English = english.New([]english.Word{{Text: "hello", Weight: 100}, {Text: "help", Weight: 50}, {Text: "users", Weight: 10}, {Text: "i", Weight: 10}, {Text: "a", Weight: 10}})
	return s
}

// #233：混打打到一半的英文收下微軟的半形標點（比照 mixedKey 的 : \ '），
// 不先送出英文、把後面的字母當注音（i'm → ㄛ'淤、a?q=1 → a?ㄆ=）。
func TestMixedEnglishKeepsStylePunctuation(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"i'm ", "i'm "},
		{"a?q=1 ", "a?q=1 "},
		{"a<b ", "a<b "},
		{"hello\"world ", "hello\"world "},
		{`C:\Users `, `C:\Users `},
		{`C:\Users\hello `, `C:\Users\hello `},
	} {
		s := englishWordsSession()
		got, _ := hostText(s, append(chars(tc.keys), proto.KeyEnter)...)
		if got != tc.want {
			t.Errorf("%q：%q，要 %q", tc.keys, got, tc.want)
		}
	}
	// 剛打完中文、沒有待定英文時照微軟打半形。
	s := englishWordsSession()
	if got, _ := hostText(s, "j", "i", "3", "\\", proto.KeyEnter); got != "我\\" {
		t.Errorf(`中文後的 \：%q，要 %q`, got, "我\\")
	}
}

// #233：英文補全打到一半的字遇到微軟半形標點，先送出這個字再交給程式，不留在組字區。
func TestEnglishAutocompleteWithMicrosoftPunctuation(t *testing.T) {
	for _, keys := range []string{"a?b ", "a<b ", "say \"hi\" ", "don't "} {
		s := englishWordsSession()
		s.EnglishAutocomplete = true
		s.Handle(proto.Request{Type: proto.Mode, On: true})
		got, snap := hostText(s, chars(keys)...)
		if got != keys || snap.Text() != "" {
			t.Errorf("%q：送出 %q、組字區 %q", keys, got, snap.Text())
		}
	}
}
