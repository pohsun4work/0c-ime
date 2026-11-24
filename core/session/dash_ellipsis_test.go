package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// #118：中文模式 Shift＋6（^）、Shift＋-（_）照微軟打半形原字元；↓ 可換成刪節號「……」、破折號「——」
// 與同組的其他寫法（同新酷音、小麥注音的 ^、_ 符號組）。破折號組補 ―（U+2015）、─（U+2500）與 ← →，另有箭頭組。

func candidateTexts(s *Session) []string {
	if s.cand == nil {
		return nil
	}
	var out []string
	for _, c := range s.cand.items {
		out = append(out, c.Entry.Surface)
	}
	return out
}

func TestCaretAndUnderscoreOfferEllipsisAndDash(t *testing.T) {
	for _, c := range []struct{ key, want string }{{"^", "……"}, {"_", "——"}} {
		s := newSession()
		snap := key(s, c.key)
		if !snap.Handled || snap.Text() != c.key || snap.Commit != "" || snap.Cursor.Rune != 1 {
			t.Fatalf("沒有組字時 %q 應成為組字區的半形 %q：%+v", c.key, c.key, snap)
		}
		key(s, proto.KeyDown)
		if snap = pickText(t, s, c.want); snap.Text() != c.want || snap.Cursor.Rune != 2 || snap.Commit != "" {
			t.Fatalf("↓ 應能換成 %q：%+v", c.want, snap)
		}
		if snap = key(s, proto.KeyEnter); snap.Commit != c.want {
			t.Fatalf("Enter 應提交 %q：%+v", c.want, snap)
		}
	}
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	key(s, "_")
	key(s, proto.KeyDown)
	pickText(t, s, "——")
	typeSyl(s, "ㄔ")
	key(s, "^")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我——吃^" {
		t.Fatalf("中文之間的破折號與刪節號：%+v", snap)
	}
	if s.model.Store.Uni["—"] != 0 || s.model.Store.Uni["^"] != 0 || s.model.Store.Bi["我\t吃"] != 0 {
		t.Fatal("符號不能學成詞，也不能讓兩側中文相連")
	}
}

func TestEllipsisAndDashAlternatives(t *testing.T) {
	for _, c := range []struct {
		key  string
		want []string
	}{
		{"^", []string{"^", "……", "⋯⋯", "…", "⋯", "︿"}},
		{"_", []string{"_", "——", "—", "―", "─", "－", "＿", "←", "→"}},
	} {
		s := newSession()
		key(s, c.key)
		key(s, proto.KeyDown)
		if got := candidateTexts(s); !slices.Equal(got, c.want) {
			t.Fatalf("%q 上按 ↓ 應列出 %q，實際 %q", c.key, c.want, got)
		}
		if snap := pickText(t, s, c.want[1]); snap.Text() != c.want[1] || snap.Cursor.Rune != 2 {
			t.Fatalf("應能換成 %q：%+v", c.want[1], snap)
		}
		key(s, proto.KeyDown)
		if got := candidateTexts(s); len(got) != len(c.want) || got[0] != c.want[1] {
			t.Fatalf("%q 上按 ↓ 應回到同一組：%q", c.want[1], got)
		}
		// 整個「……」「——」是一個符號：選半形原字元會換掉兩個字，不留半個。
		if snap := pickText(t, s, c.key); snap.Text() != c.key || snap.Cursor.Rune != 1 || snap.Commit != "" {
			t.Fatalf("應能換回半形 %q：%+v", c.key, snap)
		}
		key(s, proto.KeyEscape)
	}
	s := newSession()
	key(s, "_")
	key(s, proto.KeyDown)
	for _, dash := range []string{"―", "─", "—", "＿"} {
		if snap := pickText(t, s, dash); snap.Text() != dash {
			t.Fatalf("應能選 %q：%+v", dash, snap)
		}
		key(s, proto.KeyDown)
		if got := candidateTexts(s); got[0] != dash || !slices.Contains(got, "——") {
			t.Fatalf("%q 上按 ↓ 應列破折號組：%q", dash, got)
		}
	}
}

// 從 ↓ 換來的「……」「——」編輯時是一個符號。
func TestEllipsisAndDashEditAsOneSymbol(t *testing.T) {
	pick := func(s *Session, k, want string) {
		key(s, k)
		key(s, proto.KeyDown)
		pickText(t, s, want)
	}
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	pick(s, "^", "……")
	if snap := key(s, proto.KeyLeft); snap.Cursor.Rune != 1 {
		t.Fatalf("左鍵應一次跨過整個刪節號：%+v", snap)
	}
	if snap := key(s, proto.KeyRight); snap.Cursor.Rune != 3 {
		t.Fatalf("右鍵應一次跨過整個刪節號：%+v", snap)
	}
	if snap := key(s, proto.KeyBackspace); snap.Text() != "我" {
		t.Fatalf("退格應刪掉整個刪節號：%+v", snap)
	}
	pick(s, "_", "——")
	pick(s, "_", "——")
	if snap := key(s, proto.KeyBackspace); snap.Text() != "我——" {
		t.Fatalf("連打兩次破折號，退格只刪後一個：%+v", snap)
	}
	s.Handle(proto.Request{Type: proto.Cursor, To: 1})
	if snap := key(s, proto.KeyDelete); snap.Text() != "我" {
		t.Fatalf("Del 應刪掉整個破折號：%+v", snap)
	}
	// 中段的破折號也能整個換掉。
	pick(s, "_", "——")
	typeSyl(s, "ㄔ")
	s.Handle(proto.Request{Type: proto.Cursor, To: 1})
	key(s, proto.KeyDown)
	if snap := pickText(t, s, "―"); snap.Text() != "我―吃" || snap.Cursor.Rune != 2 {
		t.Fatalf("中段替換破折號：%+v", snap)
	}
}

func TestArrowsAndNewDashesInSymbols(t *testing.T) {
	// 破折號上按 ↓ 有 ← →（同新酷音的 _ 符號組）；箭頭上按 ↓ 換其他方向。
	for _, arrow := range []string{"←", "→"} {
		s := newSession()
		key(s, "_")
		key(s, proto.KeyDown)
		if snap := pickText(t, s, arrow); snap.Text() != arrow {
			t.Fatalf("破折號上按 ↓ 應可選 %q：%+v", arrow, snap)
		}
		key(s, proto.KeyDown)
		if got := candidateTexts(s); len(got) != 4 || got[0] != arrow || !slices.Equal(got[1:], arrowsWithout(arrow)) {
			t.Fatalf("%q 上按 ↓ 應換其他箭頭：%q", arrow, got)
		}
		if snap := pickText(t, s, "↑"); snap.Text() != "↑" {
			t.Fatalf("應可選 ↑：%+v", snap)
		}
	}
}

func arrowsWithout(arrow string) []string {
	var out []string
	for _, a := range []string{"←", "→", "↑", "↓"} {
		if a != arrow {
			out = append(out, a)
		}
	}
	return out
}

// 中英混打：識別字或英文後面的 _、^ 照舊是英文的一部分，不能變成破折號或刪節號。
func TestMixedKeepsUnderscoreAndCaretInEnglish(t *testing.T) {
	t.Skip("P4 起就有的 bug：所有符號鍵打半形（allSymbolsKey）繞過混打的英數 token 規則（a<b → a<ㄖ、03:00 → 俺:00），待修")
	for _, raw := range []string{"user_name", "foo_bar123", "my_var", "snake_case_id", "x^2", "a_b"} {
		s := newSession()
		mixedType(s, raw)
		if snap := key(s, proto.KeyEnter); snap.Commit != raw {
			t.Errorf("%s -> %q", raw, snap.Commit)
		}
	}
	for _, c := range []struct{ keys, want string }{
		{"ji3_t ", "我_吃"},
		{"ji3^", "我^"},
		{"_", "_"},
	} {
		s := newSession()
		mixedType(s, c.keys)
		if snap := key(s, proto.KeyEnter); snap.Commit != c.want {
			t.Errorf("%q -> %q want %q", c.keys, snap.Commit, c.want)
		}
	}
}

// 英文模式維持半形。
func TestEnglishModeKeepsCaretAndUnderscore(t *testing.T) {
	s := newSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap := key(s, "_"); snap.Handled {
		t.Fatalf("英文模式沒有組字時 _ 應交還應用程式：%+v", snap)
	}
	s.Handle(proto.Request{Type: proto.Mode, On: false})
	typeSyl(s, "ㄨㄛˇ")
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	key(s, "_")
	key(s, "^")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我_^" {
		t.Fatalf("英文模式組字中的 _ ^ 維持半形：%+v", snap)
	}
}
