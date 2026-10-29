package session

import (
	"slices"
	"strings"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/proto"
)

// wideKey 處理全形時混打不收的鍵。英文模式與數字鍵盤照微軟新注音：字母、數字、空白沒在組字時直接送出，
// 其餘接在游標處。中文模式沒在組字時的空白直接送出全形空白；沒開混打時（其他鍵盤排列），
// 不是注音也不是標點的鍵同官方的全形直接送出。其餘交給混打與一般注音。
func (s *Session) wideKey(k string) (commit string, ok bool) {
	if !s.wide {
		return "", false
	}
	c, keypad := proto.KeypadChar(k)
	if keypad {
		k = string(c)
	}
	r := []rune(k)
	if len(r) != 1 || r[0] > unicode.MaxASCII || !unicode.IsPrint(r[0]) {
		return "", false
	}
	switch {
	case keypad || s.english:
		if (r[0] == ' ' || unicode.IsLetter(r[0]) || unicode.IsDigit(r[0])) && !s.composing() {
			return wideText(k), true
		}
		s.flushMixed()
		s.insertPendingLiteral()
		at := s.cursor
		s.insertTyped(k)
		if keypad {
			s.markKeyed(at, true)
		}
		return "", true
	case s.composing():
	case k == " ":
		return wideText(k), true
	case !s.mixedInput():
		_, phonetic := s.layout.Symbol(r[0])
		_, punct := s.layout.Punct(r[0])
		if !phonetic && !punct && k != "`" {
			return wideText(k), true
		}
	}
	return "", false
}

// toggleWide 是 Shift＋空白：切換前打的英數照切換前的字寬定案，英文模式打到一半的字直接送出。
func (s *Session) toggleWide() string {
	s.cand = nil
	var commit string
	if s.englishWord {
		commit = s.commit()
	} else {
		s.flushMixed()
	}
	s.wide = !s.wide
	return commit
}

// insertTyped 在游標處插入打出來的英數與符號，全形時記下位置。組字區照存半形按鍵，混打的上下文判斷讀的就是它；
// 畫面上（wideWord）才換成全形，送出的是畫面的文字。從 ↓ 選回原按鍵也用目前的字寬（insertMixedLiteral）。
func (s *Session) insertTyped(text string) {
	at := s.cursor
	s.insertLiteral(text)
	s.markWide(at)
	s.markKeyed(at, false)
}

// markWide 把 at 到游標之間剛插入的字記成全形（全形時）。
func (s *Session) markWide(at int) {
	if !s.wide {
		return
	}
	if s.wideTyped == nil {
		s.wideTyped = map[int]string{}
	}
	for pos := at; pos < s.cursor; pos++ {
		s.wideTyped[pos] = wideText(string(s.syls[pos]))
	}
}

// wideWord 是畫面上的 g.Word：全形時打的字換成全形。
func (s *Session) wideWord(g decode.Seg) string {
	if len(s.wideTyped) == 0 {
		return g.Word
	}
	r := []rune(g.Word)
	if len(r) != g.End-g.Start {
		return g.Word
	}
	for i := range r {
		if w, ok := s.wideTyped[g.Start+i]; ok {
			r[i] = []rune(w)[0]
		}
	}
	return string(r)
}

// typedText 是還沒定案的英數在畫面上的樣子：全形時轉全形。
func (s *Session) typedText(text string) string {
	if !s.wide {
		return text
	}
	return wideText(text)
}

// 微軟新注音的全形 ' ^ ` 不是 Unicode 的全形字。
var wideSymbols = strings.NewReplacer("'", "’", "^", "︿", "`", "‵")

func wideText(text string) string { return fullWidthASCII(wideSymbols.Replace(text)) }

// microsoftBrackets 是整合風格（操作規則 symbols）的括號 ↓ 清單：微軟新注音 Ctrl＋[ ] 按 ↓ 的清單，但「」與【】對調，
// 跟 Ctrl＋[ ] 打的「」一樣排第一。清單裡的符號不列靈犀的分組（symbolGroups）。
var microsoftBrackets = [][]string{runes("「【『〔︹︻﹁﹃﹝［｢"), runes("」】』〕︺︼﹂﹄﹞］｣")}

// symbolChoices 是符號按 ↓ 的清單，第一項是目前的符號：整合風格的括號列 microsoftBrackets，其餘同 symbolAlternatives。
func (s *Session) symbolChoices(current string) []string {
	if s.InputStyle.Symbols != "" {
		for _, group := range microsoftBrackets {
			if !slices.Contains(group, current) {
				continue
			}
			out := []string{current}
			for _, item := range group {
				if item != current {
					out = append(out, item)
				}
			}
			return out
		}
	}
	return symbolAlternatives(current)
}

// symbolsCtrlPunct 是整合風格（操作規則 symbols）照微軟新注音多的 Ctrl＋標點：Ctrl＋[ ] 與 Ctrl＋Shift＋; 打「」：；
// 英文模式的 Ctrl＋標點也打全形，連同打到一半的英文字直接送出，不留在組字區。中文模式的官方六個鍵照官方（ctrlPunct）。
func (s *Session) symbolsCtrlPunct(k string) (handled bool, commit string, ok bool) {
	if s.InputStyle.Symbols == "" {
		return false, "", false
	}
	p, ok := proto.CtrlPunctExtra(k)
	if !ok && s.english {
		p, ok = proto.CtrlPunct(k)
	}
	switch {
	case !ok:
		return false, "", false
	case !s.CtrlPunct:
		return false, "", true
	case s.english:
		return true, s.commit() + p, true
	}
	return s.ctrlPunct(p), "", true
}
