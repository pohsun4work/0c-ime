package session

import (
	"cmp"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// #162：中英混打時按空白會組成中文的數字、標點鍵直接顯示讀音（20 → ㄉㄢ），Enter 照畫面送出；要數字按 ↓ 選原按鍵。
// 拼不成音節、數字寫法、已經是中文的組字照字面。
func TestMixedDigitsShowReading(t *testing.T) {
	for _, tc := range []struct{ keys, show string }{
		{"2", "2"}, // ㄉ 自己沒有字：空白是「2 」
		{"20", "ㄉㄢ"},
		{"18", "ㄅㄚ"},
		{"10", "ㄅㄢ"},
		{"5", "ㄓ"},
		{"202", "202"}, // 打 2026 的途中：拼不成就照字面
		{"2026", "2026"},
		{"100", "100"},
		{"15", "15"},
		{"05", "05"},   // 順序顛倒
		{"318", "318"}, // 18 緊接在字面的 3 後面
		{"3.18", "3.18"},
		{"3.5", "3.5"},
		{"6", "6"}, // 單一聲調鍵
		{"ji320", "我ㄉㄢ"},
		{"ji3 20", "我 ㄉㄢ"},
		{"ji3u.3 20", "我有 ㄉㄢ"},
		{"hello 20", "hello ㄉㄢ"},
		{"15 20", "15 ㄉㄢ"},
		{"2;", "ㄉㄤ"}, // #116 不是數字寫法的按鍵
		{"2.", "ㄉㄡ"},
		{"ji3,2.", "我,ㄉㄡ"}, // 標點照字面，後段是中文
		{",20", ",20"},
		{"5pm", "5pm"},
	} {
		s := deferredNumberSession()
		if got := mixedType(s, tc.keys).Text(); got != tc.show {
			t.Errorf("%q 顯示 %q，要 %q", tc.keys, got, tc.show)
		}
		if got := key(s, proto.KeyEnter).Commit; got != tc.show {
			t.Errorf("%q Enter %q，要 %q", tc.keys, got, tc.show)
		}
	}
}

// 顯示與按空白的結果一致：數字鍵顯示成讀音時空白出中文，照字面時空白不增加中文字。
func TestMixedDigitsShowMatchesSpace(t *testing.T) {
	for _, keys := range []string{
		"2", "20", "18", "10", "5", "8", "9", "0", "1", "202", "2026", "100", "12", "15", "05", "81", "318", "3.18",
		"ji320", "ji3 20", "hello 20", "15 20", "20 18", "6 18", "2;", "1;", "5/", "2.", "5.", "1.", "10.", "ji3,2.",
		",20", "-20", "10:30", "1,000", "5 15", "1 5 12", "5 5", "8 8", "1 5 8", "5 2;",
	} {
		s := deferredNumberSession()
		before := mixedType(s, keys)
		after := key(s, " ")
		chinese := hanCount(after.Text()) > hanCount(before.Text())
		if shown := hasZhuyin(before.Text()); shown != chinese {
			t.Errorf("%q 顯示 %q，空白後 %q", keys, before.Text(), after.Text())
		}
	}
}

// 顯示成讀音的數字跟修正讀音（gjn 的 ㄙㄨ）一樣是還沒打完的注音：失焦結束組字時丟掉（#322），
// 單按 Shift 切英文時收起來、馬上切回放回去（#90）。
func TestMixedDigitsReadingIsPendingZhuyin(t *testing.T) {
	for _, tc := range []struct{ keys, shown, left string }{
		{"20", "ㄉㄢ", ""}, {"ji3 20", "我 ㄉㄢ", "我 "}, {"hello 20", "hello ㄉㄢ", "hello "}, {"2.", "ㄉㄡ", ""},
	} {
		s := deferredNumberSession()
		mixedType(s, tc.keys)
		if got := s.Handle(proto.Request{Type: proto.Commit}).Commit; got != tc.left {
			t.Errorf("%q 失焦送出 %q，要 %q", tc.keys, got, tc.left)
		}
		s = deferredNumberSession()
		mixedType(s, tc.keys)
		if got := s.Handle(proto.Request{Type: proto.Toggle}).Text(); got != tc.left {
			t.Errorf("%q 切英文後 %q，要 %q", tc.keys, got, tc.left)
		}
		if got := s.Handle(proto.Request{Type: proto.Toggle, Restore: true}).Text(); got != tc.shown {
			t.Errorf("%q 切回注音 %q，要 %q", tc.keys, got, tc.shown)
		}
	}
}

// ↓ 選了原按鍵就照字面，不再顯示讀音，也不跳換回讀音的小視窗（Tab 不動）。
func TestMixedDigitsChosenKeysStay(t *testing.T) {
	s := deferredNumberSession()
	mixedType(s, "20")
	if snap := key(s, proto.KeyDown); snap.Candidates == nil {
		t.Fatalf("候選沒開：%+v", snap)
	}
	if snap := pickText(t, s, "20"); snap.Text() != "20" || snap.Suggestion != "" {
		t.Fatalf("選了原按鍵：%q 小視窗 %q", snap.Text(), snap.Suggestion)
	}
	if got := key(s, proto.KeyTab).Text(); got != "20" {
		t.Fatalf("選了之後 Tab：%q", got)
	}
	if got := key(s, proto.KeyEnter).Commit; got != "20" {
		t.Fatalf("Enter 提交 %q", got)
	}
}

// 單一個數字鍵（5 的 ㄓ）多半是正要打中文的第一鍵：不跳小視窗、Tab 不動，要數字按 ↓。兩鍵以上沒把握時照跳（10 的〔10〕）。
func TestMixedSingleDigitNoPopup(t *testing.T) {
	for _, tc := range []struct{ keys, shown, popup string }{{"5", "ㄓ", ""}, {"10", "ㄅㄢ", "10"}} {
		s := deferredNumberSession()
		if snap := mixedType(s, tc.keys); snap.Text() != tc.shown || snap.Suggestion != tc.popup {
			t.Errorf("%q 顯示 %q 小視窗 %q，要 %q〔%q〕", tc.keys, snap.Text(), snap.Suggestion, tc.shown, tc.popup)
		}
		if got, want := key(s, proto.KeyTab).Text(), cmp.Or(tc.popup, tc.shown); got != want {
			t.Errorf("%q Tab：%q，要 %q", tc.keys, got, want)
		}
	}
}

func hanCount(text string) int {
	n := 0
	for _, r := range text {
		if zhuyin.IsHan(r) {
			n++
		}
	}
	return n
}
