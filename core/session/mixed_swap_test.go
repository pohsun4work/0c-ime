package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// swapSession 是 repairSession 加上 單（20 的 ㄉㄢ）與字典收的縮寫 png。
func swapSession() *Session {
	s := repairSession()
	s.model.Lex.(*lexicon.Mem).Add(lexicon.Entry{Surface: "單", Reading: zhuyin.ParseReading("ㄉㄢ"), PRW: 1})
	s.English = english.New([]english.Word{{Text: "a", Weight: 65}, {Text: "hello", Weight: 65}, {Text: "png", Weight: 65}})
	return s
}

// 組字區顯示的就是按空白的結果：修正讀音的顯示修正後的注音，怎麼補都組不成音節的照英文。
func TestMixedDisplayIsSpaceResult(t *testing.T) {
	for _, tc := range []struct{ keys, shown, space string }{
		{"gjn", "ㄙㄨ", "蘇"}, {"wu//", "ㄊㄧㄥ", "聽"}, {"5wu0", "5ㄊㄧㄢ", "5天"}, {"1qu0", "1ㄆㄧㄢ", "1篇"},
		{"cpu", "cpu", "cpu "}, {"png", "png", "png "}, // 組不成音節的、字典收的縮寫照英文
		{"20", "20", "單"}, // 數字照字面顯示，讀音另外提示（#162）
	} {
		s := swapSession()
		if got := mixedType(s, tc.keys).Text(); got != tc.shown {
			t.Errorf("%s 顯示 %q，要 %q", tc.keys, got, tc.shown)
		}
		if got := key(s, " ").Text(); got != tc.space {
			t.Errorf("%s␣ 是 %q，要 %q", tc.keys, got, tc.space)
		}
	}
}

// Tab 換成小視窗顯示的另一個：修正讀音、數字換成原按鍵（之後的空白就是空白），字典收的縮寫換成修正讀音；
// 照順序組的注音不是英文詞時沒有提示。
func TestMixedTabSwapsToTheOther(t *testing.T) {
	for _, tc := range []struct{ keys, popup, space string }{
		{"gjn", "gjn", "gjn "}, {"20", "20", "20 "}, {"png", "ㄕㄣ", "深"}, {"wu0", "", "天"},
	} {
		s := swapSession()
		if got := mixedType(s, tc.keys); got.Suggestion != tc.popup || tc.popup != "" && got.SuggestedText() != tc.popup {
			t.Errorf("%s 的 Tab 提示 %q（%q），要 %q", tc.keys, got.Suggestion, got.SuggestedText(), tc.popup)
		}
		if tc.popup != "" {
			if got := key(s, proto.KeyTab).Text(); got != tc.popup {
				t.Errorf("%s Tab 是 %q，要 %q", tc.keys, got, tc.popup)
			}
		}
		if got := key(s, " ").Text(); got != tc.space {
			t.Errorf("%s Tab␣ 是 %q，要 %q", tc.keys, got, tc.space)
		}
	}
	// 再按一次換回來
	s := swapSession()
	mixedType(s, "gjn")
	key(s, proto.KeyTab)
	if got := key(s, proto.KeyTab); got.Text() != "ㄙㄨ" || got.Suggestion != "gjn" {
		t.Fatalf("gjn Tab Tab：%q 提示 %q", got.Text(), got.Suggestion)
	}
}

// Tab 換成原按鍵後接著打的另起一串：gjn Tab su3 是 gjn你，不是整串照字面。
func TestMixedTabLiteralStartsNewRun(t *testing.T) {
	s := swapSession()
	mixedType(s, "gjn")
	key(s, proto.KeyTab)
	if got := mixedType(s, "su3").Text(); got != "gjn你" {
		t.Fatalf("gjn Tab su3：%q", got)
	}
}

// 顯示修正讀音時退格照畫面刪最後一個注音（#309）。
func TestMixedRepairedBackspaceFollowsDisplay(t *testing.T) {
	for _, tc := range []struct {
		keys string
		want []string
	}{
		{"gjn", []string{"ㄙ", ""}},
		{"wu//", []string{"ㄊㄧ", "ㄊ", ""}},
		{"png\tTab\t", []string{"ㄕ", ""}},
	} {
		s := swapSession()
		for _, k := range splitProbeKeys(tc.keys) {
			key(s, k)
		}
		for i, want := range tc.want {
			if got := key(s, proto.KeyBackspace).Text(); got != want {
				t.Fatalf("%q 退格 %d 次是 %q，要 %q", tc.keys, i+1, got, want)
			}
		}
	}
}

// 修正讀音也是還沒打完的注音：Enter 照畫面送出，外殼結束組字（失焦）時丟掉（#322），單按 Shift 切英文時收起來（#90）。
func TestMixedRepairedReadingIsPendingZhuyin(t *testing.T) {
	s := swapSession()
	mixedType(s, "gjn")
	if got := key(s, proto.KeyEnter).Commit; got != "ㄙㄨ" {
		t.Fatalf("Enter 送出 %q", got)
	}
	s = swapSession()
	mixedType(s, "ji3gjn")
	if got := s.Handle(proto.Request{Type: proto.Commit}).Commit; got != "我" {
		t.Fatalf("結束組字送出 %q", got)
	}
	s = swapSession()
	mixedType(s, "gjn")
	if got := s.Handle(proto.Request{Type: proto.Toggle}).Text(); got != "" {
		t.Fatalf("切英文後組字區 %q", got)
	}
	if got := s.Handle(proto.Request{Type: proto.Toggle, Restore: true}).Text(); got != "ㄙㄨ" {
		t.Fatalf("切回注音 %q", got)
	}
}

// 全形時 Tab 提示跟換上去的一樣是全形。
func TestMixedTabHintFollowsWidth(t *testing.T) {
	s := swapSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	key(s, proto.KeyShiftSpace)
	snap := mixedType(s, "gjn")
	if after := key(s, proto.KeyTab); snap.Suggestion != "ｇｊｎ" || after.Text() != snap.Suggestion {
		t.Fatalf("提示 %q，Tab 之後 %q", snap.Suggestion, after.Text())
	}
}
