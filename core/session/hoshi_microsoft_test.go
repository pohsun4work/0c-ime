package session

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// hoshiSymbolSession 是 fork 寫死的符號行為（標點照微軟、所有符號鍵打半形、混打），
// 加上 Shift＋空白切全形（實測有全形的例子）。
func hoshiSymbolSession() *Session { return fullWidthSession() }

type microsoftCase struct {
	n, block, prefix, keys string
	text, left             string
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

// playMicrosoft 照 Windows 外殼與應用程式的做法重播實測的按鍵：引擎不處理的可見字元由應用程式打出；
// Ctrl 組合鍵只送引擎認得的 Ctrl＋標點（中文模式），其餘組字中先送出組字再交給應用程式（#106）。
// 最後按 Enter 直到沒有組字，回傳文件裡的文字與剩下的組字。
func playMicrosoft(block, prefix, keys string) (text, left string) {
	s := hoshiSymbolSession()
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
		case tok == "Down":
			press(proto.KeyDown)
		case strings.HasPrefix(tok, "S+"):
			press(shiftedKeys.Replace(tok[2:]))
		case strings.HasPrefix(tok, "C+"):
			name := proto.CtrlPrefix + tok[2:]
			if _, ok := proto.CtrlPunct(name); ok && !s.english {
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
		press(proto.KeyEnter)
	}
	return doc.String(), s.View().Text()
}

// microsoftDiffers 回傳 fork 刻意不照微軟新注音的原因；照微軟的回傳空字串。
func microsoftDiffers(c microsoftCase) string {
	tokens := strings.Fields(c.keys)
	switch {
	case strings.Contains(c.keys, "C+A+,"):
		return "Ctrl+Alt+, 開符號表（P10），不是微軟的符號輸入模式"
	case tokens[0] == "`" && len(tokens) > 1:
		return "` 不是前導鍵"
	case c.prefix == "1":
		return "混打：打到一半的按鍵碰到符號、數字鍵盤或空白照字面保留，微軟會丟掉或組成注音"
	case c.keys == "e KP2 e" || c.keys == "e 2 e" || c.keys == "a KP1" || strings.HasPrefix(c.keys, "c o n s t"):
		return "混打：微軟沒有中英混打"
	case c.block == "en-half" && strings.HasPrefix(c.keys, "C+") && c.text != "":
		return "英文模式的 Ctrl＋標點照官方交給應用程式"
	}
	return ""
}

// fork 的符號照微軟新注音（456 例實測）：按完再按 Enter 之後文件裡的字要一樣。什麼時候送出不比：
// 明確打的符號沒在組字時也留在組字區（規格「明確打的符號」）。
func TestSymbolsFollowMicrosoft(t *testing.T) {
	differs := map[string]int{}
	for _, c := range readMicrosoftSymbols(t) {
		if why := microsoftDiffers(c); why != "" {
			differs[why]++
			continue
		}
		text, left := playMicrosoft(c.block, c.prefix, c.keys)
		if text != c.text || left != c.left {
			t.Errorf("#%s %s [%s] %s：得到 %q（組字 %q），微軟 %q（組字 %q）", c.n, c.block, c.prefix, c.keys, text, left, c.text, c.left)
		}
	}
	t.Logf("刻意不同：%v", differs)
}
