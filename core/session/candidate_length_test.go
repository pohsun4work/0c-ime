package session

import (
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// P8：↓ 一律先開游標旁那一個字，Tab 換更長（到最長再回到單字），每份清單只列那個詞長的候選，
// ↓／↑ 在同一份清單裡頭尾繞圈（取代官方「先開現有分詞」與 #161 的詞＋單字混合清單）。

// shiwuSession 是「如果事物」：ㄕˋ 有十個單字，超過一頁。
func shiwuSession() *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct {
		word, reading string
		p             float64
	}{
		{"如果", "ㄖㄨˊ ㄍㄨㄛˇ", .01}, {"如", "ㄖㄨˊ", .001}, {"果", "ㄍㄨㄛˇ", .001},
		{"事物", "ㄕˋ ㄨˋ", .002}, {"事務", "ㄕˋ ㄨˋ", .001}, {"飾物", "ㄕˋ ㄨˋ", .0001},
		{"是", "ㄕˋ", .01}, {"事", "ㄕˋ", .005}, {"試", "ㄕˋ", .004}, {"視", "ㄕˋ", .003},
		{"室", "ㄕˋ", .002}, {"式", "ㄕˋ", .002}, {"飾", "ㄕˋ", .001}, {"士", "ㄕˋ", .001},
		{"世", "ㄕˋ", .001}, {"市", "ㄕˋ", .001},
		{"物", "ㄨˋ", .003}, {"務", "ㄨˋ", .002}, {"誤", "ㄨˋ", .001},
	} {
		lex.Add(lexicon.Entry{Surface: e.word, Reading: zhuyin.ParseReading(e.reading), PRW: 1})
		sys.SetUni(e.word, e.p)
	}
	return New(personal.NewModel(lex, sys))
}

func typeRuguoShiwu(t *testing.T, s *Session) {
	t.Helper()
	for _, y := range []string{"ㄖㄨˊ", "ㄍㄨㄛˇ", "ㄕˋ", "ㄨˋ"} {
		symbolSyl(s, y)
	}
	if got := s.snapshot().Text(); got != "如果事物" {
		t.Fatalf("fixture decodes %q", got)
	}
}

// allItems 收集整張清單（所有頁），之後回到第一頁第一項。
func allItems(s *Session) []proto.CandidateItem {
	var items []proto.CandidateItem
	for p := range s.pages() {
		items = append(items, s.Handle(proto.Request{Type: proto.Page, Index: p}).Candidates.Items...)
	}
	s.cand.cursor = 0
	for i := range items {
		items[i].ID, items[i].Key = 0, ""
	}
	return items
}

// 後方模式開游標後面的字，前方模式（與後方模式在句尾）開游標前面的字；Tab 以同一側為錨點換成較長的詞，
// 沒有那種詞長的跳過，到最長再回到單字。
func TestDownOpensCaretCharacterThenTabLengthens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before bool
		at     int
		single int
		char   string
	}{
		{"後方模式", false, 2, 2, "事"},
		{"前方模式", true, 4, 3, "物"},
		{"後方模式在句尾", false, 4, 3, "物"}, // 句尾改用前方的錨點（candidateBefore）
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := shiwuSession()
			s.CandidateBeforeCursor = tc.before
			typeRuguoShiwu(t, s)
			s.Handle(proto.Request{Type: proto.Cursor, To: tc.at})
			snap := key(s, proto.KeyDown)
			cursorRange(t, snap, tc.single, 1)
			if want := "修改「" + tc.char + "」 · Tab 切換詞長"; snap.Candidates.Title != want {
				t.Fatalf("title %q, want %q", snap.Candidates.Title, want)
			}
			singles := allItems(s)
			if last := singles[len(singles)-1].Text; !strings.HasPrefix(last, "ㄕ") && !strings.HasPrefix(last, "ㄨ") {
				t.Fatalf("raw zhuyin must stay last: %q", last)
			}
			// Tab 換到兩個字：清單只有這個詞長的詞，不接單字。
			snap = key(s, proto.KeyTab)
			cursorRange(t, snap, 2, 2)
			if got := texts(allItems(s)); !slices.Equal(got, []string{"事物", "事務", "飾物"}) {
				t.Fatalf("words only: %v", got)
			}
			// 沒有更長的詞：再按 Tab 回到單字，清單跟第一次一樣。
			cursorRange(t, key(s, proto.KeyTab), tc.single, 1)
			if again := allItems(s); !slices.Equal(again, singles) {
				t.Fatalf("Tab back must reopen the same singles\nfirst %+v\nagain %+v", singles, again)
			}
			if snap := s.snapshot(); snap.Text() != "如果事物" || snap.Commit != "" {
				t.Fatal("switching lengths must not change the composition")
			}
		})
	}
}

// ↓／↑ 在同一份清單裡頭尾繞圈：最後一個再 ↓ 回到第一個，第一個 ↑ 到最後一個，詞長與範圍不變。
func TestDownUpWrapWithinOneLength(t *testing.T) {
	s := shiwuSession()
	typeRuguoShiwu(t, s)
	s.Handle(proto.Request{Type: proto.Cursor, To: 2})
	key(s, proto.KeyDown)
	n := len(s.cand.items)
	snap := key(s, proto.KeyUp)
	cursorRange(t, snap, 2, 1)
	if s.cand.cursor != n-1 {
		t.Fatalf("↑ at the first item must wrap to the last: %d of %d", s.cand.cursor, n)
	}
	snap = key(s, proto.KeyDown)
	cursorRange(t, snap, 2, 1)
	if s.cand.cursor != 0 {
		t.Fatalf("↓ at the last item must wrap to the first: %d", s.cand.cursor)
	}
	key(s, proto.KeyTab)
	for range len(s.cand.items) {
		snap = key(s, proto.KeyDown)
	}
	cursorRange(t, snap, 2, 2)
	if s.cand.cursor != 0 {
		t.Fatalf("a full round stays in the word list: %d", s.cand.cursor)
	}
}

// 選單字只換掉那個字，Tab 之後選詞換整個詞；兩種都手選鎖定。
func TestDownSelection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		keys        []string
		text        string
		lock, lockN int
	}{
		{"選單字", []string{"3"}, "如果", 2, 1},
		{"Tab 之後選詞", []string{proto.KeyTab, "2"}, "如果事務", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := shiwuSession()
			typeRuguoShiwu(t, s)
			s.Handle(proto.Request{Type: proto.Cursor, To: 2})
			snap := key(s, proto.KeyDown)
			for _, k := range tc.keys {
				snap = key(s, k)
			}
			if snap.Candidates != nil || snap.Commit != "" || !strings.HasPrefix(snap.Text(), tc.text) || len([]rune(snap.Text())) != 4 {
				t.Fatalf("got %+v", snap)
			}
			if l, ok := s.locks[tc.lock]; !ok || l.Len != tc.lockN {
				t.Fatalf("locks %+v", s.locks)
			}
			if snap.Cursor.Rune != 2 {
				t.Fatalf("cursor moved without advance-after-select: %d", snap.Cursor.Rune)
			}
		})
	}
}

// 選單字之後學到的是單字（下一次開窗排第一），不是被換掉的詞。
func TestDownSingleSelectionLearnsTheCharacter(t *testing.T) {
	s := shiwuSession()
	typeRuguoShiwu(t, s)
	s.Handle(proto.Request{Type: proto.Cursor, To: 2})
	snap := cursorChoose(t, s, key(s, proto.KeyDown), "試")
	if !strings.HasPrefix(snap.Text(), "如果試") {
		t.Fatalf("%q", snap.Text())
	}
	key(s, proto.KeyEnter)
	if !s.model.PickedSingle(zhuyin.Syllable("ㄕˋ"), "試") {
		t.Fatal("single pick not learned")
	}
}

// 「選字後游標移到下一個字」（#133）：選單字時移到那個字後面；前方模式往後移一個字，句尾不動。
func TestDownAdvanceAfterSingle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		before     bool
		at, cursor int
	}{
		{"後方模式", false, 2, 3},
		{"前方模式", true, 3, 4},
		{"前方模式句尾", true, 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := shiwuSession()
			s.AdvanceAfterSelect, s.CandidateBeforeCursor = true, tc.before
			typeRuguoShiwu(t, s)
			s.Handle(proto.Request{Type: proto.Cursor, To: tc.at})
			key(s, proto.KeyDown)
			s.cand.cursor = 1
			snap := key(s, proto.KeyEnter)
			if snap.Candidates != nil || snap.Cursor.Rune != tc.cursor {
				t.Fatalf("cursor %d, want %d: %+v", snap.Cursor.Rune, tc.cursor, snap)
			}
		})
	}
}

// Home／End（#106）：End 到清單最後一項（原樣注音），選它只把那個字換成注音。
func TestDownEndAndRawZhuyin(t *testing.T) {
	s := shiwuSession()
	typeRuguoShiwu(t, s)
	s.Handle(proto.Request{Type: proto.Cursor, To: 2})
	key(s, proto.KeyDown)
	snap := key(s, proto.KeyEnd)
	cursorRange(t, snap, 2, 1)
	if c := snap.Candidates; c.Page != c.Pages-1 || c.Items[c.Highlight].Text != "ㄕˋ" {
		t.Fatalf("end: %+v", c)
	}
	if snap = key(s, proto.KeyHome); snap.Candidates.Page != 0 || snap.Candidates.Highlight != 0 {
		t.Fatalf("home: %+v", snap.Candidates)
	}
	key(s, proto.KeyEnd)
	snap = key(s, proto.KeyEnter)
	if got := snap.Text(); !strings.HasPrefix(got, "如果ㄕˋ") || len([]rune(got)) != 5 {
		t.Fatalf("raw zhuyin must replace only the caret character: %q", got)
	}
}

// 反悔鍵（#123）作用在反白的那一項，清單留著。
func TestDownUndoKeyActsOnHighlighted(t *testing.T) {
	s := shiwuSession()
	s.Preview02 = true
	typeRuguoShiwu(t, s)
	s.Handle(proto.Request{Type: proto.Cursor, To: 2})
	key(s, proto.KeyDown)
	s.cand.cursor = 1
	word := s.cand.items[s.cand.cursor].Entry.Surface
	snap := key(s, proto.KeyShiftDelete)
	if len(snap.Notices) == 0 || !strings.Contains(snap.Notices[0], "「"+word+"」") {
		t.Fatalf("notice %v", snap.Notices)
	}
	if snap.Candidates == nil {
		t.Fatal("the list stays open")
	}
}

// 模糊音（#48）：各詞長的清單各自帶近音附註。
func TestDownListsKeepNearNotes(t *testing.T) {
	s := fuzzySession(allPairs(), fillers("ㄘ ㄈㄢˋ")...)
	fzType(s, "ㄘ ㄈㄢˋ")
	s.Handle(proto.Request{Type: proto.Cursor, To: 0})
	snap := s.Handle(proto.Request{Type: proto.Candidates, Length: 2})
	cursorRange(t, snap, 0, 2)
	if it, ok := find(allItems(s), "吃飯"); !ok || it.Note != "近音 ㄔ ㄈㄢˋ" {
		t.Fatalf("word note: %+v", allItems(s))
	}
	s.Handle(proto.Request{Type: proto.Candidates, Length: 1})
	near := false
	for _, it := range allItems(s) {
		near = near || strings.HasPrefix(it.Note, "近音")
	}
	if !near {
		t.Fatalf("near singles keep their notes: %+v", allItems(s))
	}
}
