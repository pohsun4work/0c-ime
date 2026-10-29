package session

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// contextSession 是整合風格的混打 session；rules 為 false 時關掉 mixed_context，是官方（加上 Phase 2、3）的判斷。
func contextSession(rules bool) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []word{
		{"我", "ㄨㄛˇ", 1e-2}, {"什", "ㄕㄜˊ", 2e-3}, {"合", "ㄏㄜˊ", 1e-3}, {"交", "ㄐㄧㄠ", 1e-3}, {"要", "ㄧㄠˋ", 3e-3},
		{"你", "ㄋㄧˇ", 5e-3}, {"好", "ㄏㄠˇ", 4e-3}, {"個", "ㄍㄜˋ", 3e-3}, {"啊", "ㄚˋ", 1e-3}, {"卯", "ㄇㄠˇ", 1e-4},
		{"是", "ㄕˋ", 5e-3}, {"景", "ㄐㄧㄥˇ", 1e-3}, {"井", "ㄐㄧㄥˇ", 3e-4}, {"樓", "ㄌㄡˊ", 1e-3},
		{"恩", "ㄣ", 1e-3},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	s := New(personal.NewModel(lex, sys))
	p, _ := inputstyle.Get("blend")
	s.InputStyle = p.Rules
	if !rules {
		s.InputStyle.MixedContext = ""
	}
	s.MixedInput, s.CtrlPunct = true, true
	var words []english.Word
	for _, w := range []string{"a", "url", "is", "apple", "hello", "all", "zoom", "yes"} {
		words = append(words, english.Word{Text: w, Weight: 3})
	}
	s.English = english.New(words)
	return s
}

// typeContext 依序送出按鍵：大寫開頭、不只一個字元的是具名鍵（KP2、Enter、Ctrl+,），其他逐字送出。
func typeContext(s *Session, seq ...string) proto.Snapshot {
	var snap proto.Snapshot
	for _, tok := range seq {
		if len(tok) > 1 && unicode.IsUpper(rune(tok[0])) {
			snap = key(s, tok)
			continue
		}
		snap = mixedType(s, tok)
	}
	return snap
}

// 官方組不成字或組錯、整合風格照上下文改的例子；official 是關掉 mixed_context 時的樣子（前提）。
func TestMixedContextRules(t *testing.T) {
	for _, tc := range []struct {
		keys           []string
		official, want string
	}{
		// 重複鍵：同一個字母多按了幾次再按明確的聲調鍵。
		{[]string{"ggggk6"}, "ggg什", "什"},
		{[]string{"ggk6"}, "ggk6", "什"},
		{[]string{"jji3"}, "jji3", "我"},
		{[]string{"ji3ggk6"}, "我ggk6", "我什"},
		// 聲母衝突：在兩個聲母之間切開，前段是英文。
		{[]string{"gck6"}, "gck6", "g合"},
		// 反序打的英文字接空白。
		{[]string{"url "}, "交", "url "},
		{[]string{"ji3url "}, "我交", "我url "},
		// 不是國語音節的注音接空白。
		{[]string{"npm "}, "ㄙㄩㄣ", "npm "},
		{[]string{"img "}, "ㄕㄩㄛ", "img "},
		{[]string{"tmp "}, "ㄔㄩㄣ", "tmp "},
		// 數字鍵盤夾在字母中間的英數詞。
		{[]string{"e", "KP2", "e"}, "ㄍ2ㄍ", "e2e"},
		{[]string{"e", "KP2", "es"}, "ㄍ2es", "e2es"},
		{[]string{"e", "KP2", "su"}, "ㄍ2ㄋㄧ", "e2su"},
		{[]string{"e", "KP2"}, "ㄍ2", "e2"},
		{[]string{"p", "KP2", "p "}, "ㄣ2恩", "p2p "},
		{[]string{"e", "KP2", "su "}, "ㄍ2ㄋㄧ", "e2su "},
	} {
		if got := typeContext(contextSession(false), tc.keys...).Text(); got != tc.official {
			t.Errorf("%q 前提（官方）：%q，要 %q", tc.keys, got, tc.official)
		}
		if got := typeContext(contextSession(true), tc.keys...).Text(); got != tc.want {
			t.Errorf("%q：%q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 規則以外照官方：英文字、帳號、型號、版本、數字、接空白的重複鍵、組得成音節的注音、數字鍵盤前面不是字母、
// 數字鍵盤英數詞後面接聲調鍵的中文（B1樓）。
func TestMixedContextKeepsOfficial(t *testing.T) {
	for _, keys := range [][]string{
		{"kevin87"}, {"hello4"}, {"zoom3"}, {"gpt4"}, {"hello2.4"}, {"all3"}, {"884"}, {"ggk "}, {"su "}, {"up "},
		{"ji3hello "}, {"hello ji3"}, {"KP2", "ek7"}, {"ji3", "KP2", "e"}, {"su", "KP2", "su"}, {"ji3", "KP2", " "},
		{"ji3a"}, {"ji3a", "Enter"}, {"ji3x2 "}, {"a "}, {"hello a "}, {"ji3!a "}, {"ji3 a "},
		{"ji3", "Ctrl+,", "a "}, {"B", "KP1", "x.6"}, {"B", "KP1", "x.6", "Enter"},
	} {
		official, got := typeContext(contextSession(false), keys...), typeContext(contextSession(true), keys...)
		if got.Text() != official.Text() || got.Commit != official.Commit {
			t.Errorf("%q：%q／%q，官方 %q／%q", keys, got.Text(), got.Commit, official.Text(), official.Commit)
		}
	}
}

// 注音文（a␣ 2␣ 1␣ 是 ㄇ ㄉ ㄅ）：緊接在中文或注音後面一個就改；句首、空白、標點後面要連著兩個以上，第一個先照字面；
// 英數後面、沒打空白、數字鍵盤不改。改成的 ㄇ 後面接著打完英文字時改回 a。
func TestMixedContextZhuyinText(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		want string
	}{
		// 習慣二：只用空白斷句，注音文接在中文後面。
		{[]string{"ji3a "}, "我ㄇ"},
		{[]string{"ji32 "}, "我ㄉ"},
		{[]string{"ji31 "}, "我ㄅ"},
		{[]string{"ji3a a "}, "我ㄇㄇ"},
		{[]string{"ji3c a "}, "我ㄏㄇ"},
		{[]string{"ji3a ji3"}, "我ㄇ我"},
		{[]string{"ji3a 2 su3"}, "我ㄇㄉ你"},
		// 句首、空白或標點後面：連著兩個以上才改。
		{[]string{"a 2 "}, "ㄇㄉ"},
		{[]string{"1 2 "}, "ㄅㄉ"},
		{[]string{"ji3!a 2 "}, "我!ㄇㄉ"},
		{[]string{"ji3", "Ctrl+,", "a 2 "}, "我，ㄇㄉ"},
		// 改成的 ㄇ 後面接著打英文字：改回 a。
		{[]string{"ji3a is "}, "我a is "},
		{[]string{"ji3a apple "}, "我a apple "},
		{[]string{"ji3a a is "}, "我ㄇa is "},
		{[]string{"g4a yes "}, "是a yes "},
	} {
		if got := typeContext(contextSession(true), tc.keys...).Text(); got != tc.want {
			t.Errorf("%q：%q，要 %q", tc.keys, got, tc.want)
		}
	}
	// 待定的按鍵定案後看整段：英文字打完直接按 Enter，或游標移到前面。
	for _, keys := range [][]string{{"ji3a is", "Enter"}, {"ji3a is", "Home", "Enter"}} {
		if snap := typeContext(contextSession(true), keys...); snap.Commit != "我a is" {
			t.Errorf("%q 送出：%q", keys, snap.Commit)
		}
	}
	if snap := typeContext(contextSession(true), "ji3a is", "Home"); snap.Text() != "我a is" {
		t.Errorf("Home：%q", snap.Text())
	}
}

// 兩種打字習慣整段打下來：習慣一（盤古之白、中文全形標點）照官方；習慣二的注音文照規則。
func TestMixedContextTypingHabits(t *testing.T) {
	for _, keys := range [][]string{
		{"ji3 hello ji3", "Ctrl+,", "su3cl3"},
		{"ji3u04 zoom ji3", "Ctrl+,"},
	} {
		official, got := typeContext(contextSession(false), keys...), typeContext(contextSession(true), keys...)
		if got.Text() != official.Text() {
			t.Errorf("習慣一 %q：%q，官方 %q", keys, got.Text(), official.Text())
		}
	}
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"ji3a su3 "}, "我ㄇ你 "},
		{[]string{"su3cl3a hello "}, "你好a hello "},
		{[]string{"g4a 2 hello "}, "是ㄇㄉhello "},
		{[]string{"ji3 url ", "Ctrl+.", " hello"}, "我 url 。 hello"},
	} {
		if got := typeContext(contextSession(true), tc.keys...).Text(); got != tc.want {
			t.Errorf("習慣二 %q：%q，要 %q", tc.keys, got, tc.want)
		}
	}
}

// 改過的地方按 ↓ 照舊有原按鍵：選回原按鍵是明確選的，之後不再改；選的注音解釋也不改回英文。
func TestMixedContextDownOnChangedSpans(t *testing.T) {
	s := contextSession(true)
	typeContext(s, "ggggk6")
	if items := itemTexts(key(s, proto.KeyDown)); !slices.Contains(items, "ggggk6") {
		t.Fatalf("合成的音節按 ↓：%q", items)
	}

	s = contextSession(true)
	typeContext(s, "ji3a ")
	items := itemTexts(key(s, proto.KeyDown))
	if len(items) < 2 || items[0] != "ㄇ" || items[1] != "a " {
		t.Fatalf("改成的 ㄇ 按 ↓：%q", items)
	}
	pickText(t, s, "a ")
	if snap := typeContext(s, "ji3"); snap.Text() != "我a 我" {
		t.Fatalf("選回的 a 不再改：%q", snap.Text())
	}

	s = contextSession(true)
	typeContext(s, "url", "Down")
	pickText(t, s, "ㄐㄧㄠ")
	if snap := typeContext(s, " ", "ji3"); snap.Text() != "交我" {
		t.Fatalf("選的注音解釋：%q", snap.Text())
	}
	// 選了注音解釋後按數字鍵盤（照官方留下注音），不改回字母。
	for _, rules := range []bool{false, true} {
		s = contextSession(rules)
		typeContext(s, "e", "Down")
		pickText(t, s, "ㄍ")
		if snap := key(s, "KP2"); snap.Text() != "ㄍ2" {
			t.Fatalf("mixed_context %v：選的 ㄍ 接數字鍵盤：%q", rules, snap.Text())
		}
	}
}

// 改成的 ㄇ 照官方留下的注音（c␣ 的 ㄏ）退格。
func TestMixedContextBackspace(t *testing.T) {
	s := contextSession(true)
	typeContext(s, "ji3a ", "Backspace")
	official := contextSession(false)
	typeContext(official, "ji3c ", "Backspace")
	if got, want := s.snapshot().Text(), strings.Replace(official.snapshot().Text(), "ㄏ", "ㄇ", 1); got != want {
		t.Fatalf("退格：%q，同 ㄏ 要 %q", got, want)
	}
}

// 神經選字時，送出前改回的 a 不重新解碼，前面神經選的字照畫面送出。
func TestMixedContextKeepsNeuralDisplay(t *testing.T) {
	s := contextSession(true)
	s.Opt.PrefixAware = false
	s.Rerank = rerank.Options{Weight: .1, Scorer: rankFunc(func(r rerank.Request) ([]float64, error) {
		scores := make([]float64, len(r.Candidates))
		for i, c := range r.Candidates {
			if strings.Contains(c.Text, "井") {
				scores[i] = 1000
			}
		}
		return scores, nil
	})}
	if snap := typeContext(s, "ji3ru/3a is"); snap.Text() != "我井ㄇis" {
		t.Fatalf("前提：%q", snap.Text())
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "我井a is" {
		t.Fatalf("送出：%q", snap.Commit)
	}
}
