package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// markExtSession：整合風格的混打 session，打開 #123 的標示。
func markExtSession() *Session {
	s := contextSession(true)
	s.Preview02 = true
	return s
}

func markedText(snap proto.Snapshot) string {
	var b strings.Builder
	for _, g := range snap.Preedit {
		if g.Style == proto.StyleMarked {
			b.WriteString(g.Text)
		}
	}
	return b.String()
}

// Shift＋Home／End 一次標到組字區頭尾，同一直按 Shift＋←／→：結束標示時游標回到開始標示的位置，超過詞長上限時標到上限。
func TestMarkShiftHomeEnd(t *testing.T) {
	s := markExtSession()
	typeContext(s, "ji3su3cl3", "Left")
	if snap := key(s, proto.KeyShiftHome); markedText(snap) != "我你" || snap.Cursor.Rune != 0 {
		t.Fatalf("Shift＋Home：%q %+v", markedText(snap), snap.Cursor)
	}
	if snap := key(s, proto.KeyShiftEnd); markedText(snap) != "好" || snap.Cursor.Rune != 3 {
		t.Fatalf("再按 Shift＋End 從開始標示的位置標到尾：%q", markedText(snap))
	}
	if snap := key(s, proto.KeyEscape); hasMarked(snap) || snap.Cursor.Rune != 2 {
		t.Fatalf("Esc：%+v", snap.Cursor)
	}

	// 沒有這個規則或沒開 #123：就是 Home／End。
	for _, off := range []func(*Session){func(s *Session) { s.InputStyle.VocabMark = "" }, func(s *Session) { s.Preview02 = false }} {
		s := markExtSession()
		off(s)
		typeContext(s, "ji3su3")
		if snap := key(s, proto.KeyShiftHome); hasMarked(snap) || snap.Cursor.Rune != 0 {
			t.Fatalf("同 Home：%+v", snap)
		}
	}
	if key(markExtSession(), proto.KeyShiftEnd).Handled {
		t.Fatal("沒在組字時交還應用程式")
	}
	s = markExtSession()
	typeContext(s, strings.Repeat("x", 33)+" ")
	if snap := key(s, proto.KeyShiftHome); len(markedText(snap)) != 32 || !slices.Contains(snap.Notices, "最多選 32 個字") {
		t.Fatalf("超過上限：%q %v", markedText(snap), snap.Notices)
	}
}

// 標示中按 ↓ 同從標示開頭按 ↓ 再按 Tab 換到同樣長度（中文、英數都是）；選了換掉整段，游標回到開始標示的位置。
func TestMarkDownMatchesTabToLength(t *testing.T) {
	for _, tc := range []struct{ keys, tabs []string }{
		{[]string{"su3cl3"}, []string{"Tab"}},
		{[]string{"ji3hello "}, []string{"Tab", "Tab", "Tab", "Tab", "Tab", "Tab"}},
	} {
		marked := markExtSession()
		typeContext(marked, append(tc.keys, "ShiftHome")...)
		got := itemTexts(key(marked, proto.KeyDown))
		tabbed := markExtSession()
		typeContext(tabbed, append(append(tc.keys, "Home", "Down"), tc.tabs...)...)
		if want := itemTexts(tabbed.snapshot()); !slices.Equal(got, want) || len(got) < 2 {
			t.Fatalf("%q 標示 ↓：%q，Tab 到同樣長度：%q", tc.keys, got, want)
		}
	}
	s := markExtSession()
	typeContext(s, "ji3hello ", "ShiftHome", "Down")
	if snap := pickText(t, s, "ji3hello "); snap.Text() != "ji3hello " || snap.Cursor.Rune != 9 {
		t.Fatalf("選原按鍵，游標回到尾端：%q %+v", snap.Text(), snap.Cursor)
	}
	// 從頭往後標：選了之後同 ↓＋Tab 選同一項。
	s = markExtSession()
	typeContext(s, "su3cl3", "Home", "ShiftEnd", "Down")
	tabbed := markExtSession()
	typeContext(tabbed, "su3cl3", "Home", "Down", "Tab")
	if got, want := pickText(t, s, "su3cl3"), pickText(t, tabbed, "su3cl3"); got.Text() != want.Text() || got.Cursor != want.Cursor {
		t.Fatalf("從頭往後標：%q %+v，↓＋Tab：%q %+v", got.Text(), got.Cursor, want.Text(), want.Cursor)
	}

	// 之後的 Tab 照舊換詞長。
	s = markExtSession()
	typeContext(s, "su3cl3", "ShiftHome", "Down")
	if snap := key(s, proto.KeyTab); snap.Candidates == nil {
		t.Fatal("Tab 換詞長")
	}

	// 一個字、或沒有按鍵可換：照一般的 ↓，游標不回到開始標示的位置。
	s = markExtSession()
	typeContext(s, "su3cl3", "Home", "ShiftRight")
	plain := markExtSession()
	typeContext(plain, "su3cl3", "Home", "Right")
	if snap, want := key(s, proto.KeyDown), itemTexts(key(plain, proto.KeyDown)); hasMarked(snap) || !slices.Equal(itemTexts(snap), want) {
		t.Fatalf("一個字：%q，要 %q", itemTexts(snap), want)
	}
}

// 範圍中間可以有分隔的空白（照留）與查不到按鍵的字（照畫面）；沒記按鍵的中文用讀音的按鍵。
func TestMarkDownAcrossSpacesAndUnkeyed(t *testing.T) {
	s := markExtSession()
	typeContext(s, "ji3 su3", "ShiftHome")
	items := itemTexts(key(s, proto.KeyDown))
	if !slices.Contains(items, "ji3 su3") || !slices.Contains(items, "ㄨㄛˇ ㄋㄧˇ") {
		t.Fatalf("分隔的空白：%q", items)
	}

	s = markExtSession()
	typeContext(s, "ji3", "Ctrl+,", "su3", "ShiftHome")
	items = itemTexts(key(s, proto.KeyDown))
	if !slices.Contains(items, "ji3，su3") || !slices.Contains(items, "ㄨㄛˇ，ㄋㄧˇ") {
		t.Fatalf("全形標點照畫面：%q", items)
	}
	if snap := pickText(t, s, "ji3，su3"); snap.Text() != "ji3，su3" {
		t.Fatalf("選原按鍵：%q", snap.Text())
	}

	s = markExtSession()
	typeContext(s, "su3cl3")
	s.mixedKeys = nil
	typeContext(s, "ShiftHome")
	if items := itemTexts(key(s, proto.KeyDown)); !slices.Contains(items, "su3cl3") {
		t.Fatalf("沒記按鍵的中文用讀音：%q", items)
	}
}

// 整段英文按 Enter 加成英文詞（大小寫照打的，頭尾空白不算）；有中文的範圍照舊不加。
func TestMarkEnterAddsEnglishWord(t *testing.T) {
	s := markExtSession()
	snap := typeContext(s, "iPhone ", "ShiftHome")
	if !slices.Contains(snap.Notices, "Enter 加入英文詞：iPhone　Esc 取消") {
		t.Fatalf("說明：%v", snap.Notices)
	}
	snap = key(s, proto.KeyEnter)
	if !slices.Contains(snap.Notices, "已加入詞庫：iPhone") || !snap.VocabChanged || !s.model.VocabEnglish("iPhone") || snap.Text() != "iPhone " {
		t.Fatalf("加入：%+v", snap)
	}
	if snap := key(s, proto.KeyShiftHome); !slices.Contains(snap.Notices, "「iPhone」已在詞庫　Esc 取消") {
		t.Fatalf("已在詞庫：%v", snap.Notices)
	}
	if snap := key(s, proto.KeyEnter); !slices.Contains(snap.Notices, "「iPhone」已在詞庫") || snap.VocabChanged {
		t.Fatalf("重複：%+v", snap)
	}

	s = markExtSession()
	typeContext(s, "ji3hello ", "ShiftHome")
	if snap := key(s, proto.KeyEnter); !slices.Contains(snap.Notices, "只能把注音打出的字加入詞庫") || snap.VocabChanged {
		t.Fatalf("有中文：%+v", snap)
	}
	s = markExtSession()
	s.Handle(proto.Request{Type: proto.Key, Key: "s", Private: true})
	typeContext(s, "ql ", "ShiftHome")
	if snap := key(s, proto.KeyEnter); !slices.Contains(snap.Notices, "這裡不加入詞庫") || snap.VocabChanged {
		t.Fatalf("私人欄位：%+v", snap)
	}
	s = markExtSession()
	s.InputStyle.VocabMark = ""
	typeContext(s, "sql ", "ShiftLeft", "ShiftLeft", "ShiftLeft", "ShiftLeft")
	if snap := key(s, proto.KeyEnter); snap.VocabChanged {
		t.Fatalf("沒有這個規則：%+v", snap)
	}
}
