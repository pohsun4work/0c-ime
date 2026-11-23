package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/proto"
)

func candTexts(snap proto.Snapshot) []string {
	if snap.Candidates == nil {
		return nil
	}
	var out []string
	for _, c := range snap.Candidates.Items {
		out = append(out, c.Text)
	}
	return out
}

// Ctrl+[ ↓ 照微軟的括號順序；數字鍵照畫面順序選。
func TestCtrlBracketFollowsMicrosoft(t *testing.T) {
	s := hoshiSymbolSession()
	if snap := key(s, proto.KeyCtrlBracketLeft); !snap.Handled || snap.Text() != "【" {
		t.Fatalf("Ctrl+[：%+v", snap)
	}
	snap := key(s, proto.KeyDown)
	if got := candTexts(snap); !slices.Equal(got, []string{"【", "「", "『", "〔", "︹", "︻", "﹁", "﹃", "﹝", "［", "｢"}[:len(got)]) || len(got) < 5 {
		t.Fatalf("Ctrl+[ ↓：%v", got)
	}
	if snap = key(s, "2"); snap.Text() != "「" || snap.Candidates != nil {
		t.Fatalf("↓ 2：%+v", snap)
	}
	if snap = key(s, proto.KeyCtrlBracketRight); snap.Text() != "「】" {
		t.Fatalf("Ctrl+]：%+v", snap)
	}
	if got := candTexts(key(s, proto.KeyDown)); len(got) < 2 || got[0] != "】" || got[1] != "」" {
		t.Fatalf("Ctrl+] ↓：%v", got)
	}
}

// [ 打半形，↓ 第一個是 [、第二個是 「。
func TestBracketHalfwidthThenCorner(t *testing.T) {
	s := hoshiSymbolSession()
	typeSyl(s, "ㄨㄛˇ")
	if snap := key(s, "["); snap.Text() != "我[" {
		t.Fatalf("[：%+v", snap)
	}
	if got := candTexts(key(s, proto.KeyDown)); len(got) < 2 || got[0] != "[" || got[1] != "「" {
		t.Fatalf("[ ↓：%v", got)
	}
}

// ` 打 `，全形時打 ‵；不是前導鍵。
func TestBacktickIsLiteral(t *testing.T) {
	s := hoshiSymbolSession()
	if snap := key(s, "`"); !snap.Handled || snap.Text() != "`" || snap.Candidates != nil {
		t.Fatalf("`：%+v", snap)
	}
	if snap := key(s, "<"); snap.Text() != "`<" {
		t.Fatalf("` 後面照常打：%+v", snap)
	}
	key(s, proto.KeyEscape)
	key(s, proto.KeyShiftSpace)
	if snap := key(s, "`"); snap.Text() != "‵" {
		t.Fatalf("全形 `：%+v", snap)
	}
}

// 明確打的符號沒在組字時也留在組字區，混打開關都一樣；不參與組字。
func TestExplicitSymbolsStayInComposition(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, k := range []string{"$", "@", "[", "<", "?", "`", "\\"} {
			s := hoshiSymbolSession()
			s.MixedInput = mixed
			if snap := key(s, k); !snap.Handled || snap.Commit != "" || snap.Text() != k {
				t.Errorf("mixed=%v %q：%+v", mixed, k, snap)
			}
			if snap := key(s, proto.KeyEnter); snap.Commit != k {
				t.Errorf("mixed=%v %q Enter：%+v", mixed, k, snap)
			}
		}
	}
	// 符號夾在注音中間不黏到注音上。（測試詞庫只有少數字；規格的 ji3 $ wu0␣、$su3 在真引擎 eval 驗）
	s := hoshiSymbolSession()
	for _, k := range []string{"j", "i", "3", "$", "j", "i", "3"} {
		key(s, k)
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "我$我" {
		t.Fatalf("ji3 $ ji3：%+v", snap)
	}
	s = hoshiSymbolSession()
	for _, k := range []string{"$", "j", "i", "3"} {
		key(s, k)
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "$我" {
		t.Fatalf("$ji3：%+v", snap)
	}
	// 緊接在符號後面的數字照數字留；隔了別的鍵就照常組字。
	for _, tc := range []struct{ keys, want string }{{"$20 ", "$20 "}, {"$2.5", "$2.5"}, {"ji3$ji3", "我$我"}} {
		s := hoshiSymbolSession()
		mixedType(s, tc.keys)
		if snap := key(s, proto.KeyEnter); snap.Commit != tc.want {
			t.Errorf("%q：%+v", tc.keys, snap)
		}
	}
}

// 全形時混打照常，整段轉全形。
func TestFullWidthMixedInput(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"hello ", "ｈｅｌｌｏ　"},
		{"gmail.com", "ｇｍａｉｌ．ｃｏｍ"},
		{"iPhone ", "ｉＰｈｏｎｅ　"}, // 大寫字母與英數裡的符號照樣併成一串
		{"DNS ", "ＤＮＳ　"},
		{"c++ ", "ｃ＋＋　"},
		{"$20 ", "＄２０　"},
		{"don't ", "ｄｏｎ’ｔ　"}, // 照微軟逐字轉：' 是 ’、^ 是 ︿
		{"x^2 ", "ｘ︿２　"},
	} {
		s := hoshiSymbolSession()
		key(s, proto.KeyShiftSpace)
		mixedType(s, tc.keys)
		if snap := key(s, proto.KeyEnter); snap.Commit != tc.want {
			t.Errorf("全形 %q：%+v", tc.keys, snap)
		}
	}
}

// 倚天 41 鍵的 ' 是 ㄘ：0c 風格的微軟參考標點讓給排列，照樣打注音。
func TestETenQuoteStaysZhuyinUnderMicrosoftPunctuation(t *testing.T) {
	s := hoshiSymbolSession()
	s.SetLayout(layout.ETen)
	if snap := key(s, "'"); !snap.Handled || snap.Text() != "ㄘ" {
		t.Fatalf("倚天 '：%+v", snap)
	}
}
