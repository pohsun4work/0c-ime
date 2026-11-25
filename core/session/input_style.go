package session

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// insertStyleLiteral preserves the order of the visible pending English or
// mixed-input run before inserting a symbol at its cursor.
func (s *Session) insertStyleLiteral(text string) {
	s.flushMixed()
	s.insertPendingLiteral()
	s.insertLiteral(text)
}

// fullWidthText 逐字轉全形（混打的英數整段轉：don't 的 ' 也轉）。
func (s *Session) fullWidthText(text string) string {
	if utf8.RuneCountInString(text) == 1 {
		return s.fullWidthKey(text)
	}
	var b strings.Builder
	for _, r := range text {
		b.WriteString(s.fullWidthKey(string(r)))
	}
	return b.String()
}

func (s *Session) fullWidthKey(k string) string {
	switch k { // 微軟新注音全形的 ' ^ ` 不是 Unicode 的全形字
	case "'":
		return "’"
	case "^":
		return "︿"
	case "`":
		return "‵"
	}
	return fullWidthASCII(k)
}

func (s *Session) pageSize() int {
	if s.InputStyle.CandidatePageSize == 9 {
		return 9
	}
	return 10
}

func fullWidthASCII(k string) string {
	r := []rune(k)
	for i, c := range r {
		if c == ' ' {
			r[i] = '\u3000'
		} else if c >= '!' && c <= '~' {
			r[i] += 0xfee0
		}
	}
	return string(r)
}

// styleKey 處理按鍵規則（inputstyle.Rules）與照微軟的標點、全形。字寬是 session 的狀態，不是設定。
func (s *Session) styleKey(k string) (handled bool, commit string, done bool) {
	if k == proto.KeyShiftSpace {
		switch s.InputStyle.ShiftSpace {
		case "fullwidth":
			if !s.english {
				s.flushMixed() // 混打切換前打的英數照切換前的字寬定案（全形時混打的英數打全形，insertMixedLiteral）
			}
			s.fullWidth = !s.fullWidth
			return true, "", true
		case "space":
			if s.cand != nil {
				s.cand.cursor = (s.candidatePage() + 1) % s.pages() * s.pageSize()
				return true, "", true
			}
			if !s.composing() {
				return true, " ", true
			}
			if s.cursor == len(s.syls) {
				return true, s.commit() + " ", true
			}
			s.insertStyleLiteral(" ")
			return true, "", true
		case "single":
			s.openCandidates(1)
			return s.composing(), "", true
		default:
			return s.composing(), "", true
		}
	}
	// English autocomplete suggestions are advisory: a printable full-width
	// key ends that pending word without accepting its highlighted completion.
	if s.fullWidth && s.english && s.cand != nil && s.cand.english {
		if r := []rune(k); len(r) == 1 && unicode.IsPrint(r[0]) {
			s.cand = nil
		}
		if _, ok := proto.KeypadChar(k); ok {
			s.cand = nil
		}
	}
	if s.cand != nil {
		if s.InputStyle.CandidateNavigation == "horizontal" && s.cand.kind == "" {
			switch k {
			case proto.KeyLeft:
				s.moveHighlight(-1)
				return true, "", true
			case proto.KeyRight:
				s.moveHighlight(1)
				return true, "", true
			case proto.KeyUp:
				s.cand.cursor = (s.candidatePage() + s.pages() - 1) % s.pages() * s.pageSize()
				return true, "", true
			case proto.KeyDown:
				s.cand.cursor = (s.candidatePage() + 1) % s.pages() * s.pageSize()
				return true, "", true
			}
		}
		if s.InputStyle.CandidateTab == "grid" && k == proto.KeyRight && !s.styleGrid {
			s.styleGrid = true
			return true, "", true
		}
		return false, "", false
	}
	if (k == proto.KeyShiftDelete || k == proto.KeyShiftBackspace) && !s.english && s.InputStyle.ShiftDelete == "tone" {
		if !s.comp.Empty() {
			s.comp = layout.Standard.Backspace(s.comp)
			return true, "", true
		}
		if s.cursor > 0 && !s.literals[s.cursor-1] {
			syl := s.syls[s.cursor-1]
			s.eraseBeforeCursor()
			for _, r := range string(syl) {
				if zhuyin.ClassOf(r) != zhuyin.Tone {
					s.comp.Add(r)
				}
			}
			return true, "", true
		}
		return s.composing(), "", true
	}
	if !s.english && k == proto.KeyUp {
		if !s.composing() {
			return false, "", true
		}
		s.mark = nil // 跟其他按鍵一樣取消加詞標示（vocabKey 輪不到這一鍵）
		if s.comp.Empty() {
			s.cycleStyleCandidate()
		}
		return true, "", true
	}
	if c, ok := proto.KeypadChar(k); ok && s.fullWidth {
		if !s.composing() {
			return true, s.fullWidthText(string(c)), true
		}
		s.insertStyleLiteral(s.fullWidthText(string(c)))
		return true, "", true
	}
	r := []rune(k)
	if len(r) != 1 || !unicode.IsPrint(r[0]) {
		return false, "", false
	}
	if s.english && microsoftRawPunctuation(k) {
		if s.fullWidth {
			if s.englishWord {
				return true, s.commit() + s.fullWidthText(k), true
			}
			// Microsoft keeps wide punctuation as marked text even in English
			// mode; unlike wide letters/digits it can still be cancelled.
			s.insertStyleLiteral(s.fullWidthText(k))
			return true, "", true
		}
		if s.englishWord {
			// A pending autocomplete word delivers the word, then the key (#233).
			return false, "", false
		}
		if !s.composing() {
			return false, "", true
		}
		s.insertStyleLiteral(k)
		return true, "", true
	}
	if s.fullWidth && s.english {
		if s.englishWord {
			// Wide English has no completions: a word pending from half-width
			// typing ends as typed instead of staying marked (#233).
			return true, s.commit() + s.fullWidthText(k), true
		}
		if !s.composing() {
			return true, s.fullWidthText(k), true
		}
		s.insertStyleLiteral(s.fullWidthText(k))
		return true, "", true
	}
	if s.english {
		return false, "", false
	}

	if s.fullWidthMixedKey(r[0]) {
		return false, "", false
	}
	if s.fullWidth && !s.english {
		_, phonetic := layout.Standard.Symbol(r[0])
		_, punct := layout.Standard.Punct(r[0])
		if !phonetic && !punct && r[0] >= '!' && r[0] <= '~' {
			if !s.composing() {
				return true, s.fullWidthText(k), true
			}
			s.insertStyleLiteral(s.fullWidthText(k))
			return true, "", true
		}
	}
	if s.fullWidth && (r[0] >= 'A' && r[0] <= 'Z' || k == " " && !s.composing()) {
		if !s.composing() {
			return true, s.fullWidthText(k), true
		}
		s.insertStyleLiteral(s.fullWidthText(k))
		return true, "", true
	}
	return false, "", false
}

// cycleStyleCandidate 是 ↑：照微軟，游標處的字詞直接換成下一個候選、不開候選窗，連按接著換。
// 先換整個詞；詞沒有別的候選時換游標旁那個字（混合清單後段的單字）。原始按鍵與原樣注音不在輪換裡
// （「我們很好」↑ 不會變回 cp3cl3）。全形符號照 ↓ 清單的順序換下一個全形；半形符號、英數、
// 還沒定案的混打按鍵與原樣音節不動。
func (s *Session) cycleStyleCandidate() {
	if s.mixed.raw != "" {
		return // openCandidates 會把結尾是符號的待定按鍵定案（hello<），還沒定案的不動
	}
	s.refresh()
	continuing := s.styleCycle != nil
	if continuing {
		s.cand = s.styleCycle
	} else {
		s.openCandidates(0)
	}
	c := s.cand
	s.cand = nil
	if c == nil || c.mixed || c.english || c.syllable != nil {
		return
	}
	words := len(c.items)
	if c.split > 0 {
		words = c.split
	}
	ids, at := s.cycleChoices(c, 0, words)
	if len(ids) < 2 && c.split > 0 {
		ids, at = s.cycleChoices(c, c.split, len(c.items))
	}
	if continuing {
		at = slices.Index(ids, c.cursor)
	}
	if len(ids) == 0 || len(ids) == 1 && at == 0 {
		return
	}
	id := ids[(max(at, 0)+1)%len(ids)]
	c.cursor = id
	s.cand, s.styleCycle = c, c
	start, length := c.span(id)
	cursor, advance, before := s.cursor, s.AdvanceAfterSelect, len(s.syls)
	s.AdvanceAfterSelect = false
	s.selectCandidate(s.candidateID(id))
	s.AdvanceAfterSelect = advance
	// 全形符號的字數不一定一樣（——／—、……／…）：之後的輪換與游標照換上去的長度。
	grown := len(s.syls) - before
	if c.split == 0 || id < c.split {
		c.length += grown
	}
	if cursor >= start+length {
		cursor += grown
	} else if cursor > start {
		cursor = min(cursor, start+length+grown)
	}
	s.cursor = cursor
}

// cycleChoices 列出 items[lo:hi] 裡 ↑ 輪得到的項目，以及目前顯示的那一項在其中的位置（不在其中時是 -1）。
func (s *Session) cycleChoices(c *candState, lo, hi int) (ids []int, at int) {
	current := s.spanText(c.span(lo))
	if c.literal && !fullWidthSymbol(current) {
		return nil, -1
	}
	at = -1
	for id := lo; id < hi; id++ {
		text := c.items[id].Entry.Surface
		if c.keyLiterals[id] != "" || c.raw && id == len(c.items)-1 || c.literal && !fullWidthSymbol(text) {
			continue
		}
		if text == current && at < 0 {
			at = len(ids)
		}
		ids = append(ids, id)
	}
	return ids, at
}

// fullWidthSymbol：全形的符號（，、【 ……），不含半形字元與注音符號（半形 , 的 ↓ 第二項 ㄝ）。
func fullWidthSymbol(text string) bool {
	for _, r := range text {
		if r <= unicode.MaxASCII || zhuyin.ClassOf(r) != zhuyin.None {
			return false
		}
	}
	return text != ""
}

// These raw mappings were observed in the isolated Windows 11
// Microsoft Bopomofo reference run; unobserved keys retain their prior routing.
func microsoftRawPunctuation(k string) bool {
	switch k {
	case "\\", "'", "\"", "<", ">", "?":
		return true
	}
	return false
}
