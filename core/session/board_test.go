package session

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/proto"
)

func TestBoardTablesLineUp(t *testing.T) {
	total := 0
	for _, r := range boardRows {
		total += r.Keys
	}
	if total != len(boardKeys) {
		t.Fatalf("rows %v keys %d", boardRows, len(boardKeys))
	}
	for _, p := range boardPages {
		if n := utf8.RuneCountInString(p.symbols); n != len(boardKeys) {
			t.Errorf("%s: %d symbols", p.name, n)
		}
	}
}

// Ctrl＋Alt＋, 開華碩注音的符號表：鍵盤形狀，反白在 1 鍵；按鍵把那一格放進組字區並關閉。
func TestSymbolBoardSelectsByKey(t *testing.T) {
	s := blendSession()
	snap := key(s, proto.KeySymbolBoard)
	c := snap.Candidates
	if !snap.Handled || c == nil || c.Kind != boardKind || len(c.Items) != len(boardKeys) || c.Highlight != 1 || c.Page != 0 || c.Pages != 3 ||
		c.Items[1].Key != "1" || c.Items[1].Text != "１" || !slices.Equal(c.Rows, boardRows) || !strings.Contains(c.Title, "［一般］") {
		t.Fatalf("open: %+v", snap)
	}
	if snap = key(s, "q"); snap.Text() != "《" || snap.Candidates != nil || snap.Commit != "" {
		t.Fatalf("select q: %+v", snap)
	}
	if snap = keys(blendSession(), proto.KeySymbolBoard, "="); snap.Text() != "＝" {
		t.Fatalf("= is a cell, not the emoji palette's equals: %q", snap.Text())
	}
	if snap = keys(blendSession(), "j", "i", "3", proto.KeySymbolBoard, "q"); snap.Text() != "我《" {
		t.Fatalf("after a word: %q", snap.Text())
	}
	if snap = keys(blendSession(), "1", "2", proto.KeySymbolBoard, "q"); snap.Text() != "12《" {
		t.Fatalf("after pending keys: %q", snap.Text())
	}
}

// Tab／Shift＋Tab 換頁、頭尾相接；方向鍵照鍵盤位置移動；Enter 選反白；Esc 與再按一次 Ctrl＋Alt＋, 關閉。
func TestSymbolBoardNavigation(t *testing.T) {
	s := blendSession()
	key(s, proto.KeySymbolBoard)
	if snap := key(s, proto.KeyTab); !strings.Contains(snap.Candidates.Title, "［數學］") || highlighted(snap) != "1" {
		t.Fatalf("Tab keeps the highlighted key: %+v", snap.Candidates)
	}
	if snap := key(s, "3"); snap.Text() != "＞" {
		t.Fatalf("math 3: %q", snap.Text())
	}
	s = blendSession()
	key(s, proto.KeySymbolBoard)
	if snap := key(s, proto.KeyBackTab); !strings.Contains(snap.Candidates.Title, "［特殊］") {
		t.Fatalf("Shift+Tab wraps: %q", snap.Candidates.Title)
	}
	for _, tc := range []struct {
		moves []string
		want  string // 反白的鍵
	}{
		{[]string{proto.KeyRight}, "2"},
		{[]string{proto.KeyDown}, "q"},
		{[]string{proto.KeyDown, proto.KeyDown, proto.KeyDown}, "z"},
		{[]string{proto.KeyUp}, "1"},
		{[]string{proto.KeyLeft, proto.KeyDown}, "q"},
		{[]string{proto.KeyDown, proto.KeyRight, proto.KeyUp}, "2"},
		{[]string{proto.KeyRight, proto.KeyDown, proto.KeyUp}, "2"},
		{[]string{proto.KeyRight, proto.KeyRight, proto.KeyDown, proto.KeyUp}, "3"},
	} {
		snap := keys(blendSession(), append([]string{proto.KeySymbolBoard}, tc.moves...)...)
		if got := highlighted(snap); got != tc.want {
			t.Errorf("%q: highlight %q, want %q", tc.moves, got, tc.want)
		}
	}
	if snap := keys(blendSession(), proto.KeySymbolBoard, proto.KeyEnter); snap.Text() != "１" || snap.Candidates != nil {
		t.Fatalf("Enter: %+v", snap)
	}
	for _, closing := range []string{proto.KeyEscape, proto.KeySymbolBoard} {
		if snap := keys(blendSession(), "j", "i", "3", proto.KeySymbolBoard, closing); snap.Text() != "我" || snap.Candidates != nil {
			t.Fatalf("close with %q: %+v", closing, snap)
		}
	}
}

func TestSymbolBoardSelectsByPointer(t *testing.T) {
	s := blendSession()
	item := key(s, proto.KeySymbolBoard).Candidates.Items[strings.IndexByte(boardKeys, 'w')]
	if snap := s.Handle(proto.Request{Type: proto.Select, Index: item.ID}); snap.Text() != "》" || snap.Candidates != nil {
		t.Fatalf("%+v", snap)
	}
}

// 英文模式與其他風格不處理 Ctrl＋Alt＋,，交給應用程式。
func TestSymbolBoardOnlyInBlendChineseMode(t *testing.T) {
	s := blendSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap := key(s, proto.KeySymbolBoard); snap.Handled || snap.Candidates != nil {
		t.Errorf("English mode: %+v", snap)
	}
	if snap := key(styleSession("0c"), proto.KeySymbolBoard); snap.Handled || snap.Candidates != nil {
		t.Errorf("0c style: %+v", snap)
	}
}

// 官方的候選操作規則（Tab／→ 開多排）不搶符號表的方向鍵；微軟前導鍵打到一半時開符號表，前導鍵取消。
func TestSymbolBoardKeepsItsKeys(t *testing.T) {
	s := blendSession()
	s.InputStyle.CandidateTab = "grid"
	key(s, proto.KeySymbolBoard)
	if snap := key(s, proto.KeyRight); highlighted(snap) != "2" {
		t.Errorf("grid rule took Right: %+v", snap.Candidates)
	}
	s = blendSession()
	s.InputStyle.Punctuation = "microsoft"
	if snap := keys(s, "`", proto.KeySymbolBoard, "q"); snap.Text() != "《" || snap.Candidates != nil {
		t.Errorf("backtick prefix: %+v", snap)
	}
}

func highlighted(snap proto.Snapshot) string {
	return snap.Candidates.Items[snap.Candidates.Highlight].Key
}
