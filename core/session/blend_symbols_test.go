package session

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/proto"
)

func blendSession() *Session {
	s := styleSession("blend")
	p, _ := inputstyle.Get("blend")
	s.MixedInput = p.Behavior.MixedInput
	s.EnglishAutocomplete = p.Behavior.EnglishAutocomplete
	return s
}

type microsoftCase struct {
	n                   string
	block, prefix, keys string
	text, left          string
}

func readMicrosoftSymbols(t *testing.T) []microsoftCase {
	t.Helper()
	f, err := os.Open("testdata/microsoft_symbols.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []microsoftCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "#") {
			continue
		}
		f := strings.Split(sc.Text(), "\t")
		c := microsoftCase{n: f[0], block: f[1], prefix: f[2], keys: f[3]}
		if json.Unmarshal([]byte(f[4]), &c.text) != nil || json.Unmarshal([]byte(f[5]), &c.left) != nil {
			t.Fatalf("bad row: %q", sc.Text())
		}
		out = append(out, c)
	}
	return out
}

// shiftedKeys 是實測按鍵寫法 S+x 打出的字元（美式排列）。
var shiftedKeys = strings.NewReplacer(",", "<", ".", ">", "/", "?", ";", ":", "'", "\"", "[", "{", "]", "}", "\\", "|", "`", "~", "-", "_", "=", "+",
	"1", "!", "2", "@", "3", "#", "4", "$", "5", "%", "6", "^", "7", "&", "8", "*", "9", "(", "0", ")")

// playBlend 照 Windows 外殼與應用程式的做法重播實測的按鍵：引擎不處理的可見字元由應用程式打出，
// 外殼不送的 Ctrl 組合鍵先送出組字再交給應用程式（#106）。最後按 Enter 直到沒有組字，回傳文件裡的文字與剩下的組字。
func playBlend(block, prefix, keys string) (text, left string) {
	s := blendSession()
	var doc strings.Builder
	press := func(k string) {
		snap := key(s, k)
		doc.WriteString(snap.Commit)
		if snap.Handled {
			return
		}
		if c, ok := proto.KeypadChar(k); ok {
			doc.WriteRune(c)
		} else if k == proto.KeyEnter {
			doc.WriteString("\r\n")
		} else if len([]rune(k)) == 1 {
			doc.WriteString(k)
		}
	}
	if strings.HasPrefix(block, "en") {
		s.Handle(proto.Request{Type: proto.Mode, On: true})
	}
	if strings.HasSuffix(block, "full") {
		key(s, proto.KeyShiftSpace)
	}
	for _, tok := range strings.Fields(prefix + " " + keys) {
		switch {
		case tok == "Space":
			press(" ")
		case tok == "Esc":
			press(proto.KeyEscape)
		case strings.HasPrefix(tok, "S+"):
			press(shiftedKeys.Replace(tok[2:]))
		case strings.HasPrefix(tok, "C+"):
			name := proto.CtrlPrefix + tok[2:]
			_, official := proto.CtrlPunct(name)
			_, extra := proto.CtrlPunctExtra(name)
			if official || extra {
				press(name)
			} else if s.Composing() {
				doc.WriteString(s.Handle(proto.Request{Type: proto.Commit}).Commit)
			}
		default:
			press(tok)
		}
	}
	for range 3 {
		if !s.Composing() {
			break
		}
		// 英文模式自動補字打到一半的字是組字，微軟沒有：Enter 會連換行一起交給應用程式，改用送出。
		if s.View().EnglishPending {
			doc.WriteString(s.Handle(proto.Request{Type: proto.Commit}).Commit)
			continue
		}
		press(proto.KeyEnter)
	}
	return doc.String(), s.View().Text()
}

// blendDiffers 回傳整合風格刻意不照微軟新注音的原因；照微軟的回傳空字串。這些例子各自另有測試。
func blendDiffers(c microsoftCase) string {
	tokens := strings.Fields(c.keys)
	switch {
	case strings.Contains(c.keys, "C+A+,"):
		return "Ctrl+Alt+, 開華碩的符號表，不是微軟的符號輸入模式"
	case tokens[0] == "`" && len(tokens) > 1:
		return "` 不是前導鍵"
	case c.prefix == "1":
		return "打到一半的按鍵碰到符號或數字鍵盤，照混打保留，不像微軟丟掉"
	case c.keys == "e KP2 e" || c.keys == "e 2 e" || c.keys == "a KP1" || strings.HasPrefix(c.keys, "c o n s t"):
		return "中英混打的上下文判斷（Phase 4）"
	}
	return ""
}

// 整合風格的符號照微軟新注音（456 例實測）：按完再按 Enter 之後文件裡的字要一樣。什麼時候送出不比：
// 整合風格沒在組字時打的符號留在組字區，給混打當上下文（$20 不被組成「$單」）。
func TestBlendSymbolsFollowMicrosoft(t *testing.T) {
	differs := map[string]int{}
	for _, c := range readMicrosoftSymbols(t) {
		if why := blendDiffers(c); why != "" {
			differs[why]++
			continue
		}
		text, left := playBlend(c.block, c.prefix, c.keys)
		if text != c.text || left != c.left {
			t.Errorf("#%s %s [%s] %s：得到 %q（組字 %q），微軟 %q（組字 %q）", c.n, c.block, c.prefix, c.keys, text, left, c.text, c.left)
		}
	}
	t.Logf("刻意不同：%v", differs)
}

// pressKeys 依序送出按鍵，回傳最後的畫面與這段期間送出的文字。
func pressKeys(s *Session, ks ...string) (proto.Snapshot, string) {
	var snap proto.Snapshot
	var commits strings.Builder
	for _, k := range ks {
		snap = key(s, k)
		commits.WriteString(snap.Commit)
	}
	return snap, commits.String()
}

// 例外一：` 不是微軟的前導鍵，也不開靈犀的符號選單；混打時跟 @ # 一樣併進英數串（`hi` 照字面）。
func TestBlendBacktickIsALiteral(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"`"}, "`"},
		{[]string{"`", "1"}, "`1"},
		{[]string{"`", "h", "i", "`"}, "`hi`"},
		{[]string{"j", "i", "3", "`"}, "我`"},
		{[]string{"~"}, "~"},
		{[]string{proto.KeyShiftSpace, "`"}, "‵"},
		{[]string{proto.KeyShiftSpace, "~"}, "～"},
	} {
		if snap, commit := pressKeys(blendSession(), tc.keys...); snap.Text() != tc.want || commit != "" || snap.Candidates != nil {
			t.Errorf("%q: %+v", tc.keys, snap)
		}
	}
}

// 例外二：打到一半的按鍵碰到符號或數字鍵盤，照混打保留在組字區，微軟會丟掉。
func TestBlendKeepsPendingKeysBeforeSymbols(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"1", "="}, "1="},
		{[]string{"1", "("}, "1("},
		{[]string{"1", "KP2"}, "12"},
		{[]string{"e", "("}, "ㄍ("},
	} {
		if snap := keys(blendSession(), tc.keys...); snap.Text() != tc.want {
			t.Errorf("%q: %q", tc.keys, snap.Text())
		}
	}
}

// 沒在組字時打的符號留在組字區，給混打當上下文（$20 不被組成「$單」）；微軟與華碩直接送出。
func TestBlendIdleSymbolsStayInComposition(t *testing.T) {
	for _, k := range []string{"$", "<", "(", "@"} {
		if snap := key(blendSession(), k); !snap.Handled || snap.Commit != "" || snap.Text() != k {
			t.Errorf("%q: %+v", k, snap)
		}
	}
}

// Ctrl＋[ ] Shift+; 照微軟打【】：；英文模式的 Ctrl＋標點也打全形，連同打到一半的英文字直接送出。
// 設定關閉時、靈犀風格都交給應用程式。
func TestBlendCtrlPunctuation(t *testing.T) {
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{proto.KeyCtrlLeftBracket}, "【"},
		{[]string{"j", "i", "3", proto.KeyCtrlRightBracket}, "我】"},
		{[]string{proto.KeyCtrlColon}, "："},
	} {
		if snap := keys(blendSession(), tc.keys...); !snap.Handled || snap.Text() != tc.want {
			t.Errorf("%q: %+v", tc.keys, snap)
		}
	}
	s := blendSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap, commit := pressKeys(s, "h", "e", "l", proto.KeyCtrlComma); !snap.Handled || commit != "hel，" || snap.Text() != "" {
		t.Errorf("English mode: %+v", snap)
	}
	if snap, commit := pressKeys(s, proto.KeyCtrlLeftBracket, "y"); commit != "【" || !snap.EnglishPending {
		t.Errorf("autocomplete resumes after an English-mode punctuation: %+v", snap)
	}
	s = blendSession()
	s.CtrlPunct = false
	if snap := key(s, proto.KeyCtrlLeftBracket); snap.Handled {
		t.Errorf("setting off: %+v", snap)
	}
	s = styleSession("0c")
	if snap := key(s, proto.KeyCtrlLeftBracket); snap.Handled {
		t.Errorf("0c style: %+v", snap)
	}
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap := key(s, proto.KeyCtrlComma); snap.Handled {
		t.Errorf("0c style English mode: %+v", snap)
	}
}

// 全形照華碩：混打照常判斷，判成英數的段落與後面的空白轉全形；切換前打的字照切換前的字寬。
// 沒在組字時的空白、數字鍵盤的數字直接送出，Shift＋字母、數字鍵盤的運算鍵留在組字區。
func TestBlendWideKeepsMixedInput(t *testing.T) {
	for _, tc := range []struct {
		keys         []string
		text, commit string
	}{
		{[]string{"h", "e", "l", "l", "o", " "}, "ｈｅｌｌｏ\u3000", ""},
		{[]string{"j", "i", "3", "h", "e", "l", "l", "o"}, "我ｈｅｌｌｏ", ""},
		{[]string{"j", "i", "3", "<", "^", "'"}, "我＜︿’", ""},
		{[]string{"A"}, "Ａ", ""},
		{[]string{" "}, "", "\u3000"},
		{[]string{"KP2"}, "", "２"},
		{[]string{"KP."}, "．", ""},
		{[]string{"j", "i", "3", "KP2"}, "我２", ""},
	} {
		s := blendSession()
		if snap := key(s, proto.KeyShiftSpace); !snap.FullWidth {
			t.Fatalf("Shift+Space: %+v", snap)
		}
		if snap, commit := pressKeys(s, tc.keys...); snap.Text() != tc.text || commit != tc.commit {
			t.Errorf("%q: text %q commit %q", tc.keys, snap.Text(), commit)
		}
	}
	s := blendSession()
	if snap := keys(s, "h", "e", "l", proto.KeyShiftSpace, "l", "o"); snap.Text() != "helｌｏ" {
		t.Errorf("keys before the toggle keep their width: %q", snap.Text())
	}
	if snap := keys(s, proto.KeyShiftSpace, "!"); snap.FullWidth || snap.Text() != "helｌｏ!" {
		t.Errorf("back to halfwidth: %+v", snap)
	}
	s = blendSession()
	if snap := keys(s, "h", "e", "l", proto.KeyDown, proto.KeyShiftSpace); snap.Candidates != nil || snap.Text() != "hel" {
		t.Errorf("toggling with candidates open: %+v", snap)
	}
	// 英文模式打到一半的字照切換前的字寬送出，之後全形的字母直接送出，切回半形照常自動補字。
	s = blendSession()
	s.Handle(proto.Request{Type: proto.Mode, On: true})
	if snap, commit := pressKeys(s, "h", "e", "l", proto.KeyShiftSpace, "l", "o"); commit != "helｌｏ" || snap.Text() != "" {
		t.Errorf("English mode toggle: %+v %q", snap, commit)
	}
	if snap := keys(s, proto.KeyShiftSpace, "x"); !snap.EnglishPending {
		t.Errorf("English mode back to halfwidth: %+v", snap)
	}
}

// Ctrl＋, 的「，」照官方的規則不當成半形標點修剪，緊接在後面的英文字不學。
func TestBlendFullwidthPunctuationDoesNotChangeLearning(t *testing.T) {
	s := blendSession()
	keys(s, "h", "e", "l", "l", "o", proto.KeyCtrlComma, " ")
	if commits := s.mixedEnglishCommits(); len(commits) != 0 {
		t.Fatalf("%+v", commits)
	}
}

// 全形打的英文字跟半形打的算同一個詞。
func TestBlendWideWordsAreLearnedHalfwidth(t *testing.T) {
	s := blendSession()
	keys(s, proto.KeyShiftSpace, "h", "e", "l", "l", "o", " ")
	commits := s.mixedEnglishCommits()
	if len(commits) != 1 || commits[0].Text != "hello" {
		t.Fatalf("%+v", commits)
	}
}

// 沒開混打（其他鍵盤排列）時，不是注音也不是標點的鍵同官方的全形直接送出。
func TestBlendWideWithoutMixedInput(t *testing.T) {
	s := blendSession()
	s.MixedInput = false
	if _, commit := pressKeys(s, proto.KeyShiftSpace, "A"); commit != "Ａ" {
		t.Errorf("Shift+A: %q", commit)
	}
}

// 全形只改字形，不改混打的判斷：同一串按鍵半形和全形組出的字一樣，只差英數與符號的字寬，↓ 的選項也一樣。
// 混打的上下文判斷讀的是組字區裡的半形按鍵（緊接在數字後面的 . , 空白等），全形不能把它換掉。
func TestBlendWideKeepsMixedDecisions(t *testing.T) {
	asBlend := func(s *Session) *Session {
		p, _ := inputstyle.Get("blend")
		s.InputStyle, s.MixedInput = p.Rules, true
		return s
	}
	for _, newSession := range []func() *Session{deferredNumberSession, punctKeySession} {
		for _, seq := range [][]string{
			{"3", "1", "8", " "}, {"3", "1", "8", " ", "e", "k", "4"}, {"2", "0", " "},
			{"3", ".", " "}, {"3", ",", " "}, {"j", "i", "3", "3", ".", "5", " "},
			{"h", "e", "l", "l", "o", "<", " "}, {"j", "i", "3", "2", proto.KeyDown},
		} {
			half, wide := asBlend(newSession()), asBlend(newSession())
			key(wide, proto.KeyShiftSpace)
			h, w := keys(half, seq...), keys(wide, seq...)
			if w.Text() != wideText(h.Text()) {
				t.Errorf("%q: 半形 %q，全形 %q", seq, h.Text(), w.Text())
			}
			if (h.Candidates == nil) != (w.Candidates == nil) || h.Candidates != nil && len(h.Candidates.Items) != len(w.Candidates.Items) {
				t.Errorf("%q: ↓ 半形 %+v，全形 %+v", seq, h.Candidates, w.Candidates)
			}
		}
	}
}

// 官方的括號配對（#232）照常：混打的閉括號跟著開括號的寬度。整合風格打的 ( 是半形，所以 ) 也是半形；
// 用官方的 Ctrl＋Shift＋9 打全形「（」、或全形時打的 ( 之後，) 跟著是「）」。
func TestBlendClosingBracketFollowsOpening(t *testing.T) {
	if snap := keys(blendSession(), "(", "1", "2", ")"); snap.Text() != "(12)" {
		t.Errorf("halfwidth: %q", snap.Text())
	}
	if snap := keys(blendSession(), proto.KeyCtrlParenLeft, "1", "2", ")"); snap.Text() != "（12）" {
		t.Errorf("after Ctrl+Shift+9: %q", snap.Text())
	}
	// 全形打的開括號、切回半形之後的閉括號照樣跟著開括號。
	if snap := keys(blendSession(), proto.KeyShiftSpace, "(", proto.KeyShiftSpace, "1", ")"); snap.Text() != "（1）" {
		t.Errorf("width toggled between brackets: %q", snap.Text())
	}
}
