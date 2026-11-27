package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// #162：中英混打時按空白會組成中文的數字、標點鍵直接顯示讀音（20 → ㄉㄢ），Enter 照樣送出按鍵。
// 拼不成音節、數字寫法、已經是中文的組字照字面。
func TestMixedDigitsShowReading(t *testing.T) {
	for _, tc := range []struct{ keys, show, enter string }{
		{"2", "2", "2"}, // ㄉ 自己沒有字：空白是「2 」
		{"20", "ㄉㄢ", "20"},
		{"18", "ㄅㄚ", "18"},
		{"10", "ㄅㄢ", "10"},
		{"5", "ㄓ", "5"},
		{"202", "202", "202"}, // 打 2026 的途中：拼不成就照字面
		{"2026", "2026", "2026"},
		{"100", "100", "100"},
		{"15", "15", "15"},
		{"05", "05", "05"},    // 順序顛倒
		{"318", "318", "318"}, // 18 緊接在字面的 3 後面
		{"3.18", "3.18", "3.18"},
		{"3.5", "3.5", "3.5"},
		{"6", "6", "6"}, // 單一聲調鍵
		{"ji320", "我ㄉㄢ", "我20"},
		{"ji3 20", "我 ㄉㄢ", "我 20"},
		{"ji3u.3 20", "我有 ㄉㄢ", "我有 20"},
		{"hello 20", "hello ㄉㄢ", "hello 20"},
		{"15 20", "15 ㄉㄢ", "15 20"},
		{"2;", "ㄉㄤ", "2;"}, // #116 不是數字寫法的按鍵
		{"2.", "ㄉㄡ", "2."},
		{"ji3,2.", "我,ㄉㄡ", "我,2."}, // 標點照字面，後段是中文
		{",20", ",20", ",20"},
		{"5pm", "5pm", "5pm"},
	} {
		s := deferredNumberSession()
		if got := mixedType(s, tc.keys).Text(); got != tc.show {
			t.Errorf("%q 顯示 %q，要 %q", tc.keys, got, tc.show)
		}
		if got := key(s, proto.KeyEnter).Commit; got != tc.enter {
			t.Errorf("%q Enter %q，要 %q", tc.keys, got, tc.enter)
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

// 顯示成讀音的數字是空白的預覽：失焦結束組字、切英文都照按鍵留下，不當成沒打完的注音丟掉。
func TestMixedDigitsReadingKeepsKeys(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{{"20", "20"}, {"ji3 20", "我 20"}, {"hello 20", "hello 20"}, {"2.", "2."}} {
		s := deferredNumberSession()
		mixedType(s, tc.keys)
		if got := s.Handle(proto.Request{Type: proto.Commit}).Commit; got != tc.want {
			t.Errorf("%q 失焦送出 %q，要 %q", tc.keys, got, tc.want)
		}
		s = deferredNumberSession()
		mixedType(s, tc.keys)
		if got := s.Handle(proto.Request{Type: proto.Toggle}).Text(); got != tc.want {
			t.Errorf("%q 切英文後 %q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// ↓ 選了原按鍵就照字面，不再顯示讀音。
func TestMixedDigitsChosenKeysStay(t *testing.T) {
	s := deferredNumberSession()
	mixedType(s, "20")
	if snap := key(s, proto.KeyDown); snap.Candidates == nil {
		t.Fatalf("候選沒開：%+v", snap)
	}
	if snap := pickText(t, s, "20"); snap.Text() != "20" {
		t.Fatalf("選了原按鍵：%q", snap.Text())
	}
	if got := key(s, proto.KeyEnter).Commit; got != "20" {
		t.Fatalf("Enter 提交 %q", got)
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
