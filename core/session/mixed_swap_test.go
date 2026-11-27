package session

import (
	"cmp"
	"slices"
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
	for _, e := range []struct{ w, r string }{{"單", "ㄉㄢ"}, {"音", "ㄧㄣ"}} {
		s.model.Lex.(*lexicon.Mem).Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
	}
	var words []english.Word
	for _, w := range []string{"a", "hello", "png", "up", "upon"} {
		words = append(words, english.Word{Text: w, Weight: 65})
	}
	s.English = english.New(words)
	return s
}

// 組字區顯示的就是按空白的結果：修正讀音的顯示修正後的注音，怎麼補都組不成音節的照英文。
func TestMixedDisplayIsSpaceResult(t *testing.T) {
	for _, tc := range []struct{ keys, shown, space string }{
		{"gjn", "ㄙㄨ", "蘇"}, {"wu//", "ㄊㄧㄥ", "聽"}, {"5wu0", "5ㄊㄧㄢ", "5天"}, {"1qu0", "1ㄆㄧㄢ", "1篇"},
		{"cpu", "cpu", "cpu "}, {"png", "png", "png "}, // 組不成音節的、字典收的縮寫照英文
		{"20", "ㄉㄢ", "單"}, // 數字也是（#162）
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

// Tab 換成小視窗的那一個：數字換成原按鍵（之後的空白就是空白），字典收的縮寫換成修正讀音；沒有小視窗時 Tab 不動。
// 照空白的決定顯示的，決定有把握時不跳（gjn 不必問神經）；要問神經卻問不到的（20，這裡沒有神經）算沒把握。
// 照順序組的注音不是英文詞時沒有另一個（wu0）。
func TestMixedTabSwapsToTheOther(t *testing.T) {
	for _, tc := range []struct{ keys, popup, tab, space string }{
		{"gjn", "", "ㄙㄨ", "蘇"}, {"20", "20", "20", "20 "}, {"png", "ㄕㄣ", "ㄕㄣ", "深"}, {"wu0", "", "ㄊㄧㄢ", "天"},
	} {
		s := swapSession()
		if got := mixedType(s, tc.keys); got.Suggestion != tc.popup || tc.popup != "" && got.SuggestedText() != tc.popup {
			t.Errorf("%s 的 Tab 提示 %q（%q），要 %q", tc.keys, got.Suggestion, got.SuggestedText(), tc.popup)
		}
		if got := key(s, proto.KeyTab).Text(); got != tc.tab {
			t.Errorf("%s Tab 是 %q，要 %q", tc.keys, got, tc.tab)
		}
		if got := key(s, " ").Text(); got != tc.space {
			t.Errorf("%s Tab␣ 是 %q，要 %q", tc.keys, got, tc.space)
		}
	}
}

// 英文補全收下就定了：hel → hello 之後沒有小視窗，再按 Tab 不換。↓ 選了原按鍵是明確選英文，Tab 換成補全（up → upon）。
func TestMixedTabCompletionStays(t *testing.T) {
	for _, tc := range []struct{ keys, pick, want string }{{"hel", "", "hello"}, {"up", "up", "upon"}} {
		s := swapSession()
		mixedType(s, tc.keys)
		if tc.pick != "" {
			key(s, proto.KeyDown)
			pickText(t, s, tc.pick)
		}
		if got := key(s, proto.KeyTab); got.Text() != tc.want || got.Suggestion != "" {
			t.Fatalf("%s Tab：%q 提示 %q", tc.keys, got.Text(), got.Suggestion)
		}
		if got := key(s, proto.KeyTab).Text(); got != tc.want {
			t.Errorf("%s Tab Tab：%q", tc.keys, got)
		}
	}
}

// Tab 是使用者主動選的，換了就不換回去（png 換成 ㄕㄣ、20 換成原按鍵之後 Tab 不動）；換成的英文照樣補全
// （up 換成原按鍵後跳〔upon〕，收下就定了）。小視窗就是下一次 Tab 換成的樣子。
func TestMixedTabStays(t *testing.T) {
	for _, want := range [][]string{
		{"up", "ㄧㄣ", "up", "upon", "upon"},
		{"png", "png", "ㄕㄣ", "ㄕㄣ"},
		{"20", "ㄉㄢ", "20", "20"},
	} {
		s := swapSession()
		prev := mixedType(s, want[0])
		got := []string{prev.Text()}
		for range want[2:] {
			snap := key(s, proto.KeyTab)
			if popup := cmp.Or(prev.Suggestion, prev.Text()); snap.Text() != popup {
				t.Errorf("%s Tab 是 %q，小視窗說 %q", want[0], snap.Text(), popup)
			}
			got, prev = append(got, snap.Text()), snap
		}
		if prev.Suggestion != "" {
			t.Errorf("%s 最後還有小視窗 %q", want[0], prev.Suggestion)
		}
		if !slices.Equal(got, want[1:]) {
			t.Errorf("%s 連按 Tab：%q，要 %q", want[0], got, want[1:])
		}
	}
}

// ↓ 選的跟 Tab 換的一樣不換回去：up ↓ 選 ㄧㄣ 之後沒有小視窗、Tab 不動（20 ↓ 選 20 見 TestMixedDigitsChosenKeysStay）。
// 選了原按鍵的英文照樣補全（TestMixedTabCompletionStays）。
func TestMixedPickedStays(t *testing.T) {
	s := swapSession()
	mixedType(s, "up")
	key(s, proto.KeyDown)
	if snap := pickText(t, s, "ㄧㄣ"); snap.Suggestion != "" {
		t.Errorf("↓ 選 ㄧㄣ 之後小視窗 %q", snap.Suggestion)
	}
	if got := key(s, proto.KeyTab).Text(); got != "ㄧㄣ" {
		t.Errorf("↓ 選 ㄧㄣ 之後 Tab：%q", got)
	}
}

// → 選原按鍵，同 ↓ 選原按鍵：選了就定，游標不動；空白是空白、接著打的另起一串、退格刪最後一鍵。
// 再按 → 在最後面沒地方移。英文照樣補全（up〔upon〕）。
func TestMixedRightPicksKeys(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		seq  []string
		want string
	}{
		{"up", []string{proto.KeyRight}, "up"},
		{"up", []string{proto.KeyRight, " "}, "up "},
		{"up", []string{proto.KeyRight, proto.KeyRight, " "}, "up "},
		{"up", []string{proto.KeyRight, "s", "u", "3"}, "up你"},
		{"up", []string{proto.KeyRight, proto.KeyBackspace}, "u"},
		{"up", []string{proto.KeyRight, proto.KeyBackspace, "p"}, "ㄧㄣ"}, // 退格改過再打：整串照一般打字判斷
		{"gjn", []string{proto.KeyRight, " "}, "gjn "},
		{"20", []string{proto.KeyRight, " "}, "20 "},
		{"png", []string{proto.KeyTab, proto.KeyRight, " "}, "png "},
		{"hel", []string{proto.KeyTab, proto.KeyRight}, "hel"},                      // Tab 收下的 hello 回到原按鍵
		{"hel", []string{proto.KeyTab, proto.KeyBackspace, proto.KeyRight}, "hell"}, // 退格改過的本來就是原按鍵
	} {
		s := swapSession()
		mixedType(s, tc.raw)
		if snap := keys(s, tc.seq...); snap.Text() != tc.want {
			t.Errorf("%s %q：%q，要 %q", tc.raw, tc.seq, snap.Text(), tc.want)
		}
	}
	s := swapSession()
	mixedType(s, "up")
	if snap := key(s, proto.KeyRight); snap.Suggestion != "upon" {
		t.Errorf("up → 小視窗 %q，要 upon", snap.Suggestion)
	}
}

// ← Home End、點游標照畫面定案（同 Enter）：ㄧㄣ 留注音。
func TestMixedCursorKeysKeepShown(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		seq  []string
		want string
	}{
		{"up", []string{proto.KeyLeft}, "ㄧㄣ"},
		{"up", []string{proto.KeyHome, proto.KeyEnd, " "}, "音"},
		{"gjn", []string{proto.KeyLeft}, "ㄙㄨ"},
		{"20", []string{proto.KeyLeft, proto.KeyRight}, "ㄉㄢ"},
	} {
		s := swapSession()
		mixedType(s, tc.raw)
		if snap := keys(s, tc.seq...); snap.Text() != tc.want {
			t.Errorf("%s %q：%q，要 %q", tc.raw, tc.seq, snap.Text(), tc.want)
		}
	}
}

// 游標移走再回來，接著打字、退格跟沒移動一樣，Tab、↓ 選過的也是（up ← → 再打 on 是 upon、png Tab ← → 再打 x 是 pngx）。
func TestMixedCursorRoundTripKeepsTyping(t *testing.T) {
	pickZhuyin := func(t *testing.T, s *Session) {
		key(s, proto.KeyDown)
		pickText(t, s, "ㄧㄣ")
	}
	for _, tc := range []struct {
		raw    string
		before func(*testing.T, *Session)
		after  []string
	}{
		{"up", nil, []string{"o", "n"}},
		{"up", nil, []string{" "}},
		{"up", nil, []string{proto.KeyBackspace}},
		{"up", pickZhuyin, []string{"o"}},
		{"gjn", nil, []string{" "}},
		{"20", nil, []string{" "}},
		{"png", func(_ *testing.T, s *Session) { key(s, proto.KeyTab) }, []string{"x"}},
	} {
		moved, still := swapSession(), swapSession()
		for _, s := range []*Session{moved, still} {
			mixedType(s, tc.raw)
			if tc.before != nil {
				tc.before(t, s)
			}
		}
		keys(moved, proto.KeyLeft, proto.KeyRight)
		if a, b := keys(moved, tc.after...).Text(), keys(still, tc.after...).Text(); a != b {
			t.Errorf("%s ← → %q：%q，沒移動是 %q", tc.raw, tc.after, a, b)
		}
	}
}

// Tab 之後退格：收下的英文詞刪最後一個字母（hel Tab hello 退格 hell），換成的原按鍵刪最後一鍵；不跳小視窗，
// 就此收尾時照字面（up Tab 退格空白是 u␣，不是 ㄧ）。接著打字母、數字時整串照一般打字判斷（hell 再打 o 是 hello、
// hel 再打 pji3 是 help我）。沒退格時接著打的照舊另起一串（TestMixedTabLiteralStartsNewRun）。
func TestMixedTabBackspaceTrims(t *testing.T) {
	for _, tc := range []struct {
		keys         string
		erase        int
		erased, more string
		space        string
	}{
		{"hel", 1, "hell", "", "hell "},
		{"hel", 1, "hell", "o", "hello "},
		{"hel", 2, "hel", "pji3", "help我 "},
		{"up", 1, "u", "", "u "},
		{"20", 1, "2", "", "2 "},
	} {
		s := swapSession()
		mixedType(s, tc.keys)
		key(s, proto.KeyTab)
		var snap proto.Snapshot
		for range tc.erase {
			snap = key(s, proto.KeyBackspace)
		}
		if snap.Text() != tc.erased || snap.Suggestion != "" {
			t.Errorf("%s Tab 退格 %d 次：%q 小視窗 %q，要 %q", tc.keys, tc.erase, snap.Text(), snap.Suggestion, tc.erased)
			continue
		}
		mixedType(s, tc.more)
		if got := key(s, " ").Text(); got != tc.space {
			t.Errorf("%s Tab 退格 %s␣：%q，要 %q", tc.keys, tc.more, got, tc.space)
		}
	}
}

// ↓ 選了原按鍵、退格改過（跟 Tab 一樣不跳小視窗），再用 ↓ 選一次原按鍵：又是選好的，接著打的另起一串
// （s 再打 u3 是 s以，不是併成 su3 的 你）。
func TestMixedReselectAfterEditStartsNewRun(t *testing.T) {
	s := swapSession()
	mixedType(s, "su")
	key(s, proto.KeyDown)
	pickText(t, s, "su")
	if snap := key(s, proto.KeyBackspace); snap.Suggestion != "" {
		t.Fatalf("選過再退格還有小視窗 %q", snap.Suggestion)
	}
	key(s, proto.KeyDown)
	pickText(t, s, "s")
	if got := mixedType(s, "u3").Text(); got != "s以" {
		t.Fatalf("重選後接著打：%q", got)
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
	snap := mixedType(s, "hel")
	if after := key(s, proto.KeyTab); snap.Suggestion != "ｈｅｌｌｏ" || after.Text() != snap.Suggestion {
		t.Fatalf("提示 %q，Tab 之後 %q", snap.Suggestion, after.Text())
	}
}

// 不給的英文：大小寫混著的不修正（iPhone 不換 phone，全小寫的 iphone 照換）、只去掉結尾幾鍵的修正（helloj 是 hello
// 接著打中文）、照順序是注音又不是英文詞的不補全（ao 是 ㄇㄟ，不補 aortic；so 是英文詞，照補 some）。
func TestMixedTabSkipsUnwantedEnglish(t *testing.T) {
	var words []english.Word
	for _, w := range []string{"hello", "phone", "aortic", "so", "some"} {
		words = append(words, english.Word{Text: w, Weight: 65})
	}
	for _, tc := range []struct{ keys, popup string }{
		{"iPhone", ""}, {"iphone", "phone"}, {"helloj", ""}, {"ao", ""}, {"so", "some"}, {"aor", "aortic"},
	} {
		s := splitSession(nil, nil)
		s.English = english.New(words)
		if got := mixedType(s, tc.keys).Suggestion; got != tc.popup {
			t.Errorf("%s 小視窗 %q，要 %q", tc.keys, got, tc.popup)
		}
	}
}
