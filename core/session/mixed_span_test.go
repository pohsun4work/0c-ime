package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

func resplitSession() *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct {
		w, r string
		p    float64
	}{
		{"我", "ㄨㄛˇ", 1e-2}, {"單", "ㄉㄢ", 1e-3}, {"但", "ㄉㄢˋ", 2e-3},
		{"你", "ㄋㄧˇ", 5e-3}, {"好", "ㄏㄠˇ", 4e-3}, {"你好", "ㄋㄧˇ ㄏㄠˇ", 1e-3}, {"黑", "ㄏㄟ", 1e-3},
		{"欸乃", "ㄟˇ ㄋㄞˇ", 1e-4}, // ㄟˇ 沒有單字，只能當詞頭（#107）
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	s := New(personal.NewModel(lex, sys))
	p, _ := inputstyle.Get("blend")
	s.InputStyle = p.Rules
	s.MixedInput = true
	s.English = english.New([]english.Word{{Text: "even", Weight: 3}})
	return s
}

func wantItems(t *testing.T, snap proto.Snapshot, want ...string) {
	t.Helper()
	if snap.Candidates == nil || !slices.Equal(itemTexts(snap), want) {
		t.Fatalf("候選 %q，要 %q", itemTexts(snap), want)
	}
}

// 使用者回報：數字鍵盤打斷的 e 留成 ㄍ，第一個 ㄍ 的候選只有 ㄍ。現在 ↓ 有原按鍵，Tab 換詞長跨過數字鍵盤的 2，
// 整段換得成英文或注音（第二個 e 後面的空白是一聲，選英文時照按鍵留下）。數字鍵盤的 2 在注音裡也是 2。
// 整合風格的 mixed_context 把 e KP2 e 直接判成英數詞（mixed_context_test.go），這裡換成注音再選回英文。
func TestResplitKeypadWordAcrossLengths(t *testing.T) {
	s := resplitSession()
	if snap := keys(s, "e", "KP2", "e", " "); snap.Text() != "e2e " {
		t.Fatalf("前提：%q", snap.Text())
	}
	key(s, proto.KeyHome)
	wantItems(t, key(s, proto.KeyDown), "e", "ㄍ")
	wantItems(t, key(s, proto.KeyTab), "e2", "ㄍ2")
	wantItems(t, key(s, proto.KeyTab), "e2e", "ㄍ2ㄍ")
	wantItems(t, key(s, proto.KeyTab), "e2e ", "ㄍ2ㄍ")
	if snap := pickText(t, s, "ㄍ2ㄍ"); snap.Text() != "ㄍ2ㄍ" {
		t.Fatalf("選注音：%q", snap.Text())
	}
	if items := itemTexts(key(s, proto.KeyDown)); len(items) < 2 || items[0] != "ㄍ" || items[1] != "e" {
		t.Fatalf("選的注音上按 ↓：%q", items)
	}
	wantItems(t, key(s, proto.KeyTab), "ㄍ2", "e2")
	wantItems(t, key(s, proto.KeyTab), "ㄍ2ㄍ", "e2e ")
	if snap := pickText(t, s, "e2e "); snap.Text() != "e2e " {
		t.Fatalf("選英文：%q", snap.Text())
	}
	if got := s.mixedEnglishCommits(); len(got) != 1 || got[0].Text != "e2e" || !got[0].Explicit {
		t.Fatalf("明確選的英文照英文詞學：%+v", got)
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "e2e " {
		t.Fatalf("送出：%q", snap.Commit)
	}
}

// 還沒定案的按鍵候選按 Tab：照畫面定案，再從那段的長度換到下一個詞長。
func TestResplitTabFromPendingKeys(t *testing.T) {
	s := resplitSession()
	keys(s, "s", "u", "KP2", "s", "u", proto.KeyDown)
	wantItems(t, key(s, proto.KeyTab), "2ㄋㄧ", "2su", "2你")
	wantItems(t, key(s, proto.KeyTab), "ㄋㄧ2ㄋㄧ", "su2su", "你2你")
	if snap := pickText(t, s, "su2su"); snap.Text() != "su2su" {
		t.Fatalf("選英文：%q", snap.Text())
	}
}

// $20␣ 照字面留下；換到整段時空白當一聲，有「$單」（同華碩注音）。選了之後在字上按 ↓ 有原按鍵，選回英數。
func TestResplitDigitsToChinese(t *testing.T) {
	s := resplitSession()
	if snap := keys(s, "$", "2", "0", " "); snap.Text() != "$20 " {
		t.Fatalf("前提：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	for range 3 {
		key(s, proto.KeyTab)
	}
	if snap := pickText(t, s, "$單"); snap.Text() != "$單" {
		t.Fatalf("選中文：%q", snap.Text())
	}
	snap := key(s, proto.KeyDown)
	if !slices.Contains(itemTexts(snap), "20 ") {
		t.Fatalf("中文字上的 ↓ 有原按鍵：%q", itemTexts(snap))
	}
	if snap := pickText(t, s, "20 "); snap.Text() != "$20 " {
		t.Fatalf("選回原按鍵：%q", snap.Text())
	}
}

// 沒打聲調的音節列每個聲調的字；選的字當成手選，後面接著打也不會被改掉。
func TestResplitChineseChoiceStaysLocked(t *testing.T) {
	s := resplitSession()
	keys(s, "$", "2", "0", proto.KeyHome, proto.KeyDown, proto.KeyTab, proto.KeyTab)
	if items := itemTexts(s.snapshot()); !slices.Contains(items, "$但") || !slices.Contains(items, "$單") {
		t.Fatalf("各聲調的字：%q", items)
	}
	pickText(t, s, "$單")
	key(s, proto.KeyEnd)
	if snap := keys(s, "j", "i", "3"); snap.Text() != "$單我" {
		t.Fatalf("手選的字不變：%q", snap.Text())
	}
}

// 多個音節列讀音相符、蓋滿整段的詞（同中文的 ↓）；組不成音節的按鍵（cons 的 ㄋ）沒有中文，只有英文與注音。
func TestResplitSyllablesAndLeftoverKeys(t *testing.T) {
	s := resplitSession()
	keys(s, "s", "u", "c", "l", proto.KeyHome, proto.KeyDown)
	for range 3 {
		key(s, proto.KeyTab)
	}
	wantItems(t, s.snapshot(), "sucl", "ㄋㄧㄏㄠ", "你好")
	s = resplitSession()
	keys(s, "c", "o", "n", "s", proto.KeyHome, proto.KeyDown)
	for range 3 {
		key(s, proto.KeyTab)
	}
	wantItems(t, s.snapshot(), "cons", "ㄏㄟㄙㄋ")
}

// 數字鍵盤的數字只是數字：↓ 不列 ㄉ。主鍵盤的 2 照舊有 ㄉ。
func TestResplitKeypadDigitIsNotZhuyin(t *testing.T) {
	s := resplitSession()
	keys(s, "j", "i", "3", "KP2", proto.KeyLeft)
	if items := itemTexts(key(s, proto.KeyDown)); slices.Contains(items, "ㄉ") {
		t.Fatalf("數字鍵盤的 2：%q", items)
	}
	s = resplitSession()
	keys(s, "j", "i", "3", "2", proto.KeyLeft)
	if items := itemTexts(key(s, proto.KeyDown)); !slices.Contains(items, "ㄉ") {
		t.Fatalf("主鍵盤的 2：%q", items)
	}
}

// 留成注音的按鍵回到待定按鍵：退格照按鍵退，接著打照混打判斷（e、退掉數字鍵盤的 2、再打 2e␣ 是 e2e）。
func TestResplitRestoresPendingKeys(t *testing.T) {
	s := resplitSession()
	keys(s, "e", "KP2", proto.KeyBackspace, proto.KeyBackspace)
	if s.Composing() {
		t.Fatalf("退完：%q", s.snapshot().Text())
	}
	s = resplitSession()
	if snap := keys(s, "e", "KP2", proto.KeyBackspace, "2", "e", " "); snap.Text() != "e2e " {
		t.Fatalf("接著打：%q", snap.Text())
	}
}

// 使用者回報：神經選字時打 e、數字鍵盤的 2、e 再按三下 ←，ㄍ2ㄍ 變成 ㄍㄍ2ㄍ。方向鍵與 Enter 不收回原樣注音，畫面與送出的字照舊。
// （整合風格的 mixed_context 把 e KP2 e 判成英數詞，這裡用兩個字母的 su。）
func TestResplitNavigationKeepsLiteral(t *testing.T) {
	s := resplitSession()
	s.Rerank = rerank.Options{Weight: .1, Scorer: rankFunc(func(r rerank.Request) ([]float64, error) {
		return make([]float64, len(r.Candidates)), nil
	})}
	keys(s, "s", "u", "KP2", "s", "u")
	for _, k := range []string{proto.KeyLeft, proto.KeyLeft, proto.KeyLeft, proto.KeyRight, proto.KeyLeft, proto.KeyEnd, proto.KeyHome, proto.KeyRight} {
		if snap := key(s, k); snap.Text() != "ㄋㄧ2ㄋㄧ" {
			t.Fatalf("按 %s：%q", k, snap.Text())
		}
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "ㄋㄧ2ㄋㄧ" {
		t.Fatalf("送出：%q", snap.Commit)
	}
}

// 游標緊接在原樣注音後面按 ↓ 照舊收回成待定按鍵，候選有原按鍵；Esc 照舊先清掉它。
func TestResplitDownAndEscapeRestoreLiteral(t *testing.T) {
	s := resplitSession()
	keys(s, "s", "u", "KP2", proto.KeyLeft)
	if items := itemTexts(key(s, proto.KeyDown)); !slices.Contains(items, "su") {
		t.Fatalf("↓ 的候選：%q", items)
	}
	s = resplitSession()
	keys(s, "s", "u", "KP2", proto.KeyLeft)
	if snap := key(s, proto.KeyEscape); snap.Text() != "2" {
		t.Fatalf("Esc：%q", snap.Text())
	}
}

// 全形時判成英數的段落照樣換得成中文，照字面的字元照畫面是全形。
func TestResplitWide(t *testing.T) {
	s := resplitSession()
	keys(s, proto.KeyShiftSpace, "$", "2", "0", " ", proto.KeyDown)
	for range 3 {
		key(s, proto.KeyTab)
	}
	if snap := pickText(t, s, "＄單"); snap.Text() != "＄單" {
		t.Fatalf("全形：%q", snap.Text())
	}
}

// 補字接受的英文沒有完整的按鍵，不重新拆。
func TestResplitSkipsAcceptedCompletion(t *testing.T) {
	s := resplitSession()
	s.EnglishAutocomplete = true
	keys(s, "e", "v", proto.KeyTab, " ", proto.KeyHome, proto.KeyDown)
	if s.snapshot().Text() != "even " || s.cand == nil || s.cand.keySpan != nil {
		t.Fatalf("補字接受的段落：%q %+v", s.snapshot().Text(), s.cand)
	}
}

// 混打打出來的每個位置都查得回按鍵：中文、英數、符號、原樣注音、數字鍵盤。
func TestResplitEveryTypedPositionHasKeys(t *testing.T) {
	for _, ks := range [][]string{
		{"j", "i", "3", "c", "o", "n", "s", " ", "s", "u", "3"},
		{"$", "2", "0", " ", "j", "i", "3", ",", "KP1", "KP."},
		{"e", "KP2", "e", " ", "c", " ", "u", "1", "6"},
		{proto.KeyShiftSpace, "h", "i", " ", "s", "u", "3", "!"},
	} {
		s := resplitSession()
		keys(s, ks...)
		key(s, proto.KeyHome) // 待定的按鍵照畫面定案
		for pos := range s.syls {
			if _, ok := s.keyUnitAt(pos); !ok {
				t.Errorf("%q：位置 %d（%q）查不到按鍵，畫面 %q", ks, pos, s.syls[pos], s.snapshot().Text())
			}
		}
	}
}

// 刪掉聲調的原樣音節（↓ 選了 ㄉ 再打 4）按鍵裡有聲調，退格後照舊回到注音，不變成按鍵 24。
func TestResplitToneDeletedSyllableStaysZhuyin(t *testing.T) {
	s := resplitSession()
	keys(s, "j", "i", "3", "2", proto.KeyDown)
	pickText(t, s, "ㄉ")
	key(s, "4")
	if snap := key(s, proto.KeyBackspace); snap.Text() != "我ㄉ" {
		t.Fatalf("退掉聲調：%q", snap.Text())
	}
	if snap := key(s, "3"); snap.Text() != "我ㄉˇ" {
		t.Fatalf("改打聲調：%q", snap.Text())
	}
}

// 選中文只鎖選的字：後面接著插的字面字元不會讓它跟後文解成一個詞再切開，鎖住使用者沒選的字。
func TestResplitChineseChoiceLocksOnlyTheSpan(t *testing.T) {
	s := resplitSession()
	keys(s, "c", "l", "3", proto.KeyHome, "s", "u", "$", proto.KeyHome, proto.KeyDown, proto.KeyTab, proto.KeyTab)
	if snap := pickText(t, s, "你$"); snap.Text() != "你$好" {
		t.Fatalf("選中文：%q", snap.Text())
	}
	if _, ok := s.locks[2]; ok || len(s.locks) != 1 {
		t.Fatalf("只鎖「你」：%+v", s.locks)
	}
}

// 閉括號跟著開括號的寬度時，另一種寬度也記在 literalKeys；原按鍵照舊是打的 )。
func TestResplitPairedBracketKeepsTypedKey(t *testing.T) {
	s := resplitSession()
	keys(s, "j", "i", "3", "(", "a", ")")
	if u, ok := s.keyUnitAt(len(s.syls) - 1); !ok || u.keys != ")" {
		t.Fatalf("閉括號的按鍵：%+v %v，畫面 %q", u, ok, s.snapshot().Text())
	}
}

// 用 Del 刪掉原樣音節的聲調，按鍵也去掉聲調鍵：↓ 不會把刪掉的聲調帶回來。
func TestResplitDeletedToneLeavesKeys(t *testing.T) {
	s := resplitSession()
	keys(s, "j", "i", "3", "2", proto.KeyDown)
	pickText(t, s, "ㄉ")
	if snap := keys(s, "4", proto.KeyLeft, proto.KeyDelete); snap.Text() != "我ㄉ" {
		t.Fatalf("刪掉聲調：%q", snap.Text())
	}
	if items := itemTexts(key(s, proto.KeyDown)); slices.Contains(items, "24") || slices.Contains(items, "ㄉˋ") {
		t.Fatalf("聲調沒有回來：%q", items)
	}
}

// 只能當詞頭、原樣留下的注音（ㄟˇ）記著按鍵，跟下一個音節收回成詞後也是。
func TestResplitPrefixSyllableKeepsKeys(t *testing.T) {
	s := resplitSession()
	keys(s, "o", "3")
	if u, ok := s.keyUnitAt(0); !ok || u.keys != "o3" {
		t.Fatalf("原樣留下的詞頭：%+v %v，畫面 %q", u, ok, s.snapshot().Text())
	}
	if snap := keys(s, "s", "9", "3"); snap.Text() != "欸乃" || s.mixedKeys[0] != "o3" || s.mixedKeys[1] != "s93" {
		t.Fatalf("收回成詞：%q %v", snap.Text(), s.mixedKeys)
	}
}

// 純中文的 Tab 照舊只換到最長的詞長，不會一路延伸到整句。
func TestResplitChineseTabStaysWithinWordLength(t *testing.T) {
	s := resplitSession()
	limit := decode.MaxLen(s.dec.Lex)
	for range limit + 2 {
		keys(s, "j", "i", "3")
	}
	keys(s, proto.KeyHome, proto.KeyDown)
	for range limit + 2 {
		if key(s, proto.KeyTab); s.cand == nil || s.cand.length > limit {
			t.Fatalf("詞長上限 %d：%+v", limit, s.cand)
		}
	}
}
