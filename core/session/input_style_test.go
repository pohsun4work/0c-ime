package session

import (
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

func TestUpCycleInSecondWord(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄐㄧㄥˇ")
	if a := key(s, proto.KeyUp); a.Text() != "我井" {
		t.Fatalf("second word cycle: %+v", a)
	}
}

func TestCustomCandidateSelectionAndCommit(t *testing.T) {
	s := newSession()
	s.InputStyle.CandidateSpace = inputstyle.CandidateSpaceSelect
	s.InputStyle.CandidateEnter = inputstyle.CandidateEnterCommit
	typeSyl(s, "ㄐㄧㄥˇ")
	key(s, proto.KeyDown)
	key(s, proto.KeyDown)
	if a := key(s, " "); a.Text() != "井" || a.Candidates != nil || a.Commit != "" {
		t.Fatalf("Space select: %+v", a)
	}
	key(s, proto.KeyDown)
	if a := key(s, proto.KeyEnter); a.Commit == "" || a.Text() != "" {
		t.Fatalf("Enter select and commit: %+v", a)
	}
}

// fullWidthSession 是 Shift＋空白設成切換全半形的 session。
func fullWidthSession() *Session {
	s := newSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceFullWidth
	return s
}

func TestMicrosoftWidthIsStateAndIncludesEnglish(t *testing.T) {
	s := fullWidthSession()
	if a := key(s, proto.KeyShiftSpace); !a.FullWidth || a.Text() != "" || a.Commit != "" {
		t.Fatalf("toggle: %+v", a)
	}
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	for _, c := range []struct{ k, w string }{{"a", "ａ"}, {"A", "Ａ"}, {"1", "１"}, {" ", "　"}} {
		if a := key(s, c.k); !a.Handled || a.Commit != c.w {
			t.Fatalf("%q: %+v", c.k, a)
		}
	}
	if a := key(s, proto.KeyShiftSpace); a.FullWidth {
		t.Fatal("width did not toggle back")
	}
	if a := key(s, "a"); a.Handled {
		t.Fatal("halfwidth idle English should pass through")
	}
	s.Handle(proto.Request{Type: proto.Mode, On: false})
	typeSyl(s, "ㄨㄛˇ")
	if a := key(s, proto.KeyShiftSpace); a.Text() != "我" || a.Commit != "" {
		t.Fatalf("toggle touched composition: %+v", a)
	}
	if a := key(s, proto.KeyEnter); a.Commit != "我" || !a.FullWidth {
		t.Fatalf("commit should retain width: %+v", a)
	}
}

// fork：` 不是前導鍵，跟其他明確打的符號一樣留在組字區。
func TestMicrosoftBacktickIsLiteralAndInlineUp(t *testing.T) {
	s := newSession()
	s.InputStyle.CandidateTab = inputstyle.CandidateTabGrid
	if a := key(s, "`"); !a.Handled || a.Text() != "`" || a.Candidates != nil {
		t.Fatalf("backtick: %+v", a)
	}
	if a := key(s, "<"); a.Text() != "`<" {
		t.Fatalf("raw punctuation after backtick: %+v", a)
	}
	if a := key(s, proto.KeyEnter); a.Commit != "`<" {
		t.Fatalf("commit: %+v", a)
	}
	typeSyl(s, "ㄐㄧㄥˇ")
	if a := key(s, proto.KeyUp); a.Text() != "井" || a.Candidates != nil || a.Commit != "" {
		t.Fatalf("inline Up: %+v", a)
	}
	key(s, proto.KeyDown)
	if a := key(s, proto.KeyTab); a.Candidates == nil || a.Candidates.Columns != 5 {
		t.Fatalf("expand grid: %+v", a)
	}
}

// fork：半形原字元沒在組字時也留在組字區；全形時照微軟。
func TestMicrosoftObservedRawPunctuation(t *testing.T) {
	for _, tc := range []struct{ key, wide string }{
		{"\\", "＼"}, {"'", "’"}, {"\"", "＂"}, {"<", "＜"}, {">", "＞"}, {"?", "？"},
	} {
		s := fullWidthSession()
		if a := key(s, tc.key); !a.Handled || a.Commit != "" || a.Text() != tc.key {
			t.Fatalf("halfwidth idle %q stays in composition: %+v", tc.key, a)
		}
		key(s, proto.KeyEscape)
		key(s, proto.KeyShiftSpace)
		if a := key(s, tc.key); a.Text() != tc.wide || a.Commit != "" {
			t.Fatalf("Chinese fullwidth %q: %+v", tc.key, a)
		}
		key(s, proto.KeyEscape)
		s.Handle(proto.Request{Type: proto.Mode, On: true})
		if a := key(s, tc.key); a.Text() != tc.wide || a.Commit != "" {
			t.Fatalf("English fullwidth %q: %+v", tc.key, a)
		}
	}
}

func TestMicrosoftRawPunctuationInComposition(t *testing.T) {
	for _, r := range []string{"\\", "'", "<"} {
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		if a := key(s, r); a.Commit != "" || a.Text() != "我"+r {
			t.Fatalf("completed word + raw %q: %+v", r, a)
		}
		if a := key(s, proto.KeyEscape); a.Commit != "" || a.Text() != "" {
			t.Fatalf("Esc should clear marked raw punctuation: %+v", a)
		}
		typeSyl(s, "ㄨㄛˇ")
		key(s, proto.KeyDown)
		if a := key(s, r); a.Candidates == nil || a.Text() != "我" || a.Commit != "" {
			t.Fatalf("candidate raw must be ignored: %+v", a)
		}
	}
}

func TestStyleLiteralPreservesPendingEnglishOrder(t *testing.T) {
	for _, word := range []string{"h", "he"} {
		s := fullWidthSession()
		s.EnglishAutocomplete = true
		s.Handle(proto.Request{Type: proto.Mode, On: true})
		for _, r := range word {
			key(s, string(r))
		}
		key(s, proto.KeyShiftSpace)
		// Wide English has no completions: the pending word ends as typed, in
		// order, instead of keeping every later wide key marked (#233).
		if a := key(s, "x"); a.Commit != word+"ｘ" || a.Text() != "" || a.Candidates != nil {
			t.Fatalf("%q + wide x reordered or accepted completion: %+v", word, a)
		}
		if a := key(s, "y"); a.Commit != "ｙ" || a.Text() != "" {
			t.Fatalf("later wide key stayed marked: %+v", a)
		}
	}
}

func TestStyleLiteralPreservesMixedRun(t *testing.T) {
	for _, tc := range []struct {
		key, suffix string
		width       bool
	}{
		{"A", "Ａ", true},
		{"\\", "\\", false}, // 打到一半的英文收下 \（C:\Users，#233）
		{"A", "A", false},   // Shift＋字母留在游標處
	} {
		s := fullWidthSession()
		var before proto.Snapshot
		for _, r := range "hello" {
			before = key(s, string(r))
		}
		if tc.width {
			key(s, proto.KeyShiftSpace)
		}
		a := key(s, tc.key)
		want := before.Text() + tc.suffix
		if a.Text() != want || a.Commit != "" {
			t.Fatalf("style literal: got %+v want %q", a, want)
		}
		if a = key(s, proto.KeyEnter); a.Commit != want {
			t.Fatalf("commit: got %+v want %q", a, want)
		}
	}
}

// ↑ 輪換字詞；沒開候選窗時 Shift＋Tab 吃掉不動作（只在候選窗反向切換字詞長度）。
func TestUpCycleAndShiftSpaceInsertsSpace(t *testing.T) {
	s := newSession()
	s.InputStyle.ShiftSpace = inputstyle.ShiftSpaceSpace
	s.EscKeepComposition = true
	typeSyl(s, "ㄐㄧㄥˇ")
	if a := key(s, proto.KeyUp); a.Text() != "井" || a.Candidates != nil || a.Cursor.Rune != 1 {
		t.Fatalf("next: %+v", a)
	}
	if a := key(s, proto.KeyBackTab); !a.Handled || a.Text() != "井" || a.Candidates != nil {
		t.Fatalf("Shift+Tab: %+v", a)
	}
	if a := key(s, proto.KeyEscape); a.Text() != "井" {
		t.Fatalf("Esc cleared completed word: %+v", a)
	}
	if a := key(s, proto.KeyShiftSpace); a.Commit != "井 " || a.FullWidth {
		t.Fatalf("ShiftSpace: %+v", a)
	}
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ")
	key(s, proto.KeyLeft)
	if a := key(s, proto.KeyShiftSpace); a.Text() != "我 吃" || a.Commit != "" {
		t.Fatalf("middle space: %+v", a)
	}
}

func TestUpCycleWithManyCandidates(t *testing.T) {
	s := manySession("1234567890")
	s.CandidateBeforeCursor = true
	before := typeSyl(s, "ㄕˋ").Text()
	if after := key(s, proto.KeyUp).Text(); before == after {
		t.Fatalf("before=%q up=%q", before, after)
	}
	key(s, proto.KeyLeft)
	if s.styleCycle != nil {
		t.Fatal("cursor movement must invalidate the inline cycle")
	}
}

func TestHorizontalCandidateNavigation(t *testing.T) {
	s := newSession()
	s.InputStyle.CandidateNavigation = inputstyle.NavigationHorizontal
	typeSyl(s, "ㄐㄧㄥˇ")
	key(s, proto.KeyDown)
	if a := key(s, proto.KeyRight); a.Candidates == nil || a.Candidates.Highlight != 1 {
		t.Fatalf("horizontal right: %+v", a)
	}
	if a := key(s, proto.KeyEnter); a.Candidates != nil || a.Text() != "井" || a.Commit != "" {
		t.Fatalf("Enter selects only: %+v", a)
	}
}

func TestShiftDeleteReopensTone(t *testing.T) {
	for _, deleteKey := range []string{proto.KeyShiftDelete, proto.KeyShiftBackspace} {
		s := newSession()
		s.InputStyle.ShiftDelete = inputstyle.ShiftDeleteTone
		typeSyl(s, "ㄐㄧㄥˇ")
		if a := key(s, deleteKey); a.Text() != "ㄐㄧㄥ" {
			t.Fatalf("%s remove tone: %+v", deleteKey, a)
		}
		if a := key(s, "3"); a.Text() != "景" {
			t.Fatalf("retype tone: %+v", a)
		}
	}
}

func TestStyleCandidatePageSizeAndSpace(t *testing.T) {
	s := manySession("1234567890")
	s.InputStyle.CandidatePageSize = 9
	typeSyl(s, "ㄕˋ")
	a := key(s, proto.KeyDown)
	if a.Candidates == nil {
		t.Fatal("no candidates")
	}
	if len(a.Candidates.Items) != 9 {
		t.Fatalf("page size: %+v", a.Candidates)
	}
	if a = key(s, "0"); a.Candidates == nil {
		t.Fatal("0 should not select tenth item in 9-item page")
	}
	if a = key(s, " "); a.Candidates.Page != 1 {
		t.Fatalf("Space pages: %+v", a)
	}
	if a = key(s, proto.KeyEnter); a.Candidates != nil || a.Commit != "" {
		t.Fatalf("Enter selects: %+v", a)
	}
}
