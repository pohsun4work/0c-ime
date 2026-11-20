package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// 按鍵記法：一個字元一鍵，␣ 是空白鍵；{名稱} 是具名鍵（{Enter}、{Down}、{BackTab}、{KP2}、{Ctrl+,}…）。
// 大寫字母、Shift 符號就是那個字元（A、<、$）。{pick:字} 是在開著的候選裡選那一項（找不到會翻頁）。
// 不是已知鍵名的 { 照字面算一鍵。
const (
	spaceMark  = "␣"
	pickPrefix = "pick:"
)

var namedKeys = map[string]bool{
	proto.KeyEnter: true, proto.KeyEscape: true, proto.KeyBackspace: true, proto.KeyDelete: true,
	proto.KeyTab: true, proto.KeyBackTab: true, proto.KeyShiftSpace: true,
	proto.KeyLeft: true, proto.KeyRight: true, proto.KeyUp: true, proto.KeyDown: true,
	proto.KeyPageUp: true, proto.KeyPageDown: true, proto.KeyHome: true, proto.KeyEnd: true,
	proto.KeyShiftLeft: true, proto.KeyShiftRight: true, proto.KeyShiftDelete: true, proto.KeyShiftBackspace: true,
}

func isNamed(k string) bool {
	if namedKeys[k] || len(k) > len(pickPrefix) && strings.HasPrefix(k, pickPrefix) {
		return true
	}
	if _, ok := ctrlChar(k); ok {
		return true
	}
	_, ok := proto.KeypadChar(k)
	return ok
}

// ctrlChar 解出 Ctrl＋可見字元（{Ctrl+,}、{Ctrl+[}）；引擎認不認得由 info 的 ctrl_punctuation_keys 決定。
func ctrlChar(k string) (byte, bool) {
	rest, ok := strings.CutPrefix(k, proto.CtrlPrefix)
	if !ok || len(rest) != 1 || rest[0] < 0x21 || rest[0] > 0x7e {
		return 0, false
	}
	return rest[0], true
}

func parseKeys(s string) ([]string, error) {
	var keys []string
	for s != "" {
		if s[0] == '{' {
			if name, _, ok := strings.Cut(s[1:], "}"); ok && isNamed(name) {
				keys = append(keys, name)
				s = s[len(name)+2:]
				continue
			}
		}
		r, n := utf8.DecodeRuneInString(s)
		switch {
		case r == ' ' || r == '\t':
			return nil, fmt.Errorf("按鍵裡有空白字元：空白鍵寫 %s", spaceMark)
		case string(r) == spaceMark:
			keys = append(keys, " ")
		case r < 0x21 || r > 0x7e:
			return nil, fmt.Errorf("按鍵 %q 不是可見 ASCII 字元，也不是 %s 或 {鍵名}", r, spaceMark)
		default:
			keys = append(keys, string(r))
		}
		s = s[n:]
	}
	return keys, nil
}

func formatKeys(keys []string) string {
	var b strings.Builder
	for _, k := range keys {
		switch {
		case k == " ":
			b.WriteString(spaceMark)
		case isNamed(k):
			b.WriteString("{" + k + "}")
		default:
			b.WriteString(k)
		}
	}
	return b.String()
}

// showSpaces 把文字裡的半形空白畫成 ␣，報告與語料檔裡才看得到；readText 反過來。
func showSpaces(s string) string { return strings.ReplaceAll(s, " ", spaceMark) }
func readText(s string) string   { return strings.ReplaceAll(s, spaceMark, " ") }

// Case 是一則語料：照 Keys 打完（不送出）後，畫面上（已送出＋組字區）應該是 Want。
type Case struct {
	ID   string `json:"id"`
	Keys string `json:"keys"`
	Want string `json:"want"`
	Note string `json:"note,omitempty"`
}

// Corpus 是一個語料檔（hoshi/eval/corpus/*.tsv）。檔頭註解：
//
//	# profile: user       用 hoshi/eval/profiles/<名稱>.json 的設定跑（預設 user）
//	# match: baseline     這一組要跟基準逐字一樣（純中文），報告看跟基準比有沒有變，不只看 want
//	# match: asus         want 是華碩的實測畫面，不是規格的目標，正確率只供參考
type Corpus struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Match   string `json:"match,omitempty"`
	Cases   []Case `json:"cases"`
}

func readCorpus(path string) (Corpus, error) {
	f, err := os.Open(path)
	if err != nil {
		return Corpus{}, err
	}
	defer f.Close()
	c := Corpus{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Profile: "user"}
	seen := map[string]bool{}
	scan := bufio.NewScanner(f)
	for line := 1; scan.Scan(); line++ {
		text := strings.TrimRight(scan.Text(), "\r")
		if meta, ok := strings.CutPrefix(text, "#"); ok {
			if k, v, ok := strings.Cut(meta, ":"); ok {
				switch strings.TrimSpace(k) {
				case "profile":
					c.Profile = strings.TrimSpace(v)
				case "match":
					c.Match = strings.TrimSpace(v)
				}
			}
			continue
		}
		if text == "" || strings.HasPrefix(text, "id\t") {
			continue
		}
		cols := strings.Split(text, "\t")
		if len(cols) < 3 {
			return c, fmt.Errorf("%s:%d：要有 id、keys、want 三欄（tab 分隔）", path, line)
		}
		k := Case{ID: cols[0], Keys: cols[1], Want: readText(cols[2])}
		if len(cols) > 3 {
			k.Note = cols[3]
		}
		if seen[k.ID] {
			return c, fmt.Errorf("%s:%d：id %s 重複", path, line, k.ID)
		}
		if _, err := parseKeys(k.Keys); err != nil {
			return c, fmt.Errorf("%s:%d：%w", path, line, err)
		}
		seen[k.ID] = true
		c.Cases = append(c.Cases, k)
	}
	return c, scan.Err()
}
