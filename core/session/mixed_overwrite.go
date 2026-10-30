package session

import (
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// 神經模型判斷「打錯又補對」的門檻（log10），初值取自 2026-10-30 用本機神經模型實測的句子。
const (
	// overwriteLetterEffect：前面多一個聲母時，那個字母讓蓋完的字的分數掉得比這多才當成打錯（車子的g速 -3.32 蓋掉、
	// 我講過n次 +0.72 留著；43 句裡要留的最低是沿著y軸 -1.59，要換的最高是就是1譬 -1.20）。
	overwriteLetterEffect = -1.6
	// overwriteToneMargin：聲調鍵結尾時，蓋完的中文字比畫面上照字面的樣子逐字加總至少高這麼多才換
	// （不要cl.3 吼 7.90 換、這是ios7 6.83 留）。
	overwriteToneMargin = 7.0
	// overwriteSpaceMargin：空白（一聲）結尾時的門檻（看njg␣ 書 7.05 換、回傳json␣ 4.84 留）。
	overwriteSpaceMargin = 6.0
)

// mixedOverwrite：整合風格（操作規則 mixed_overwrite），要有 mixed_resplit 的按鍵紀錄。
func (s *Session) mixedOverwrite() bool { return s.InputStyle.MixedOverwrite != "" && s.resplit() }

// overwrite 是一段按鍵照微軟新注音「同一格後按的蓋掉先按的」組成的音節（group 的 keys 是全部的按鍵）。
// lead：被蓋掉的是第一個鍵、緊接著另一個聲母（gnj4 的 g、nh4 的 n），那個鍵也可能是要打的字母（n次）。
type overwrite struct {
	group keyGroup
	lead  bool
}

// overwriteReading：keys 是注音鍵接一個聲調鍵（空白是一聲），恰好有一格按了兩次、而且是不同的鍵時（5jiy6 的 ㄓ、ㄗ），
// 照後按的組成國語音節。同一個鍵連按（ggggk6）交給 mixed_context 的重複鍵。
func (s *Session) overwriteReading(keys string) (overwrite, bool) {
	if len(keys) < 3 {
		return overwrite{}, false
	}
	g := keyGroup{keys: keys}
	first := map[zhuyin.Class]int{}
	drop := -1
	for i, k := range keys[:len(keys)-1] {
		sym, ok := s.layout.Symbol(k)
		cl := zhuyin.ClassOf(sym)
		if !ok || cl == zhuyin.None || cl == zhuyin.Tone {
			return overwrite{}, false
		}
		if at, seen := first[cl]; seen {
			if drop >= 0 || keys[at] == keys[i] {
				return overwrite{}, false
			}
			drop = at
		}
		first[cl] = i
		g.comp.Add(sym) // Composer 同一格後加的蓋掉先加的
	}
	tone, ok := s.layout.Symbol(rune(keys[len(keys)-1]))
	if !ok || zhuyin.ClassOf(tone) != zhuyin.Tone || drop < 0 {
		return overwrite{}, false
	}
	g.tone = tone
	if _, ok := g.syllable(); !ok {
		return overwrite{}, false
	}
	return overwrite{g, drop == 0 && s.keyIsInitial(rune(keys[0])) && s.keyIsInitial(rune(keys[1]))}, true
}

// overwriteSyllable 是 keys 蓋完的音節（輕聲沒有字時同 mixedReading 改一聲），要查得到字。
func (s *Session) overwriteSyllable(keys string) (overwrite, zhuyin.Syllable, bool) {
	o, ok := s.overwriteReading(keys)
	if !ok {
		return overwrite{}, "", false
	}
	syl, _ := o.group.syllable()
	syl = s.neutralFallback(syl)
	return o, syl, s.readingReachable(syl)
}

// overwriteAt 同 overwriteSyllable，聲調前的按鍵是英文字或整串是我的詞彙時不算（web3、pi3 照字面）。
func (s *Session) overwriteAt(keys string) (overwrite, zhuyin.Syllable, bool) {
	if s.knownEnglish(keys[:len(keys)-1]) || s.vocabEnglish(keys) {
		return overwrite{}, "", false
	}
	return s.overwriteSyllable(keys)
}

// rejudgeOverwritePending：官方照字面留著的待定按鍵（5jiy6）是打錯又補對的時，換成蓋完的音節。
func (s *Session) rejudgeOverwritePending() {
	p := s.mixed
	if p.choice != 0 || p.accepted != "" || s.englishWord || !s.explicitTone(p.raw) || s.mixedText() != p.raw ||
		s.alnumAt(s.cursor-1) {
		return
	}
	if o, syl, ok := s.overwriteAt(p.raw); ok && s.overwriteWanted(overwriteCase{start: s.cursor, syl: syl, shown: []rune(p.raw), lead: o.lead}) {
		s.clearMixed()
		s.insertMixedTyped(syl, p.raw)
	}
}

// rejudgeOverwrite 看 end 前從英數以外接著照字面打的一串注音鍵：以照字面的聲調鍵結尾（5jiy6），接著官方切開的中文音節
// （g速、n次、cl偶），或接一個空白（gjn␣）。是打錯又補對的時換成蓋完的音節，換了回報 true。
func (s *Session) rejudgeOverwrite(end int) bool {
	for pos := 0; pos < end; pos++ {
		if s.alnumAt(pos-1) || !s.overwriteKeyAt(pos) {
			continue
		}
		j := pos
		var keys strings.Builder
		for ; j < end && s.overwriteKeyAt(j); j++ {
			keys.WriteString(string(s.syls[j]))
		}
		raw, stop, after, space := keys.String(), j, rune(0), false
		switch {
		case s.explicitTone(raw):
		case j < end && !s.literals[j] && s.explicitTone(s.mixedKeys[j]):
			raw += s.mixedKeys[j]
			after, stop = []rune(s.shownChars(j + 1)[j])[0], j+1
		case j < end && s.plainLiteralAt(j) && string(s.syls[j]) == " ":
			raw, stop, space = raw+" ", j+1, true
		default:
			pos = j
			continue
		}
		o, syl, ok := s.overwriteAt(raw)
		// 前面多一個聲母時只跟官方切開的「一個字母＋中文」比（n次）；一聲的（2wu0␣）不換。
		c := overwriteCase{start: pos, syl: syl, shown: []rune(raw[:j-pos]), after: after, lead: o.lead && (after == 0 || j == pos+1), space: space}
		if after != 0 {
			c.shown = append(c.shown, after)
		}
		if ok && !(c.lead && space) && s.overwriteWanted(c) && s.replaceSpan(pos, stop, func() { s.insertMixedReading(syl, raw) }) {
			return true
		}
		pos = j
	}
	return false
}

// overwriteKeyAt：pos 是照字面打的一個注音鍵，不是空白、數字鍵盤、打的符號或 ↓ 選的。
func (s *Session) overwriteKeyAt(pos int) bool {
	u, ok := s.keyUnitAt(pos)
	o := s.mixedOrigins[pos]
	if !ok || !s.plainLiteralAt(pos) || u.keypad || o.keyed || o.explicit || u.keys == " " || u.keys != string(s.syls[pos]) {
		return false
	}
	sym, ok := s.layout.Symbol(rune(u.keys[0]))
	return ok && len(u.keys) == 1 && zhuyin.ClassOf(sym) != zhuyin.None
}

// overwriteCase 是要判斷的一段：start 起畫面上是 shown（不含一聲的空白），after 是官方切開的中文字（n次 的「次」）。
type overwriteCase struct {
	start       int
	syl         zhuyin.Syllable
	shown       []rune
	after       rune
	lead, space bool
}

// overwriteWanted：神經模型照前文（已確認的前文＋組字區 start 前的字）判斷要不要換成蓋完的音節；
// 沒有神經模型、私人、暫停或沒有前文時不換。
//   - lead（前面多一個聲母）：那個字母讓蓋完的字的分數掉得夠多才是打錯（車子的g速），讓它更合理是要打的字母（我講過n次）。
//     比的是蓋完的字，不是官方切開時畫面上的字（車子的g塑 的「塑」）。
//   - 其他：蓋完的中文字比畫面上的樣子逐字加總高得夠多才換（打錯的按鍵照字面是亂碼，英文字照字面很順：ios7、json）。
func (s *Session) overwriteWanted(c overwriteCase) bool {
	left := s.neuralContext() + strings.Join(s.shownChars(c.start), "")
	if !s.neuralText() || left == "" {
		return false
	}
	score := func(before string, r rune) (float64, bool) { return s.neuralChar(s.neuralWindow(before), r, c.syl) }
	char, ok := s.overwriteChar(c.start, c.syl)
	if !ok {
		return false
	}
	chinese, ok := score(left, char)
	if !ok {
		return false
	}
	if c.lead {
		with, ok := score(left+string(c.shown[0]), char)
		return ok && with-chinese < overwriteLetterEffect
	}
	var literal float64
	for i, r := range c.shown {
		v, ok := score(left+string(c.shown[:i]), r)
		if !ok {
			return false
		}
		literal += v
	}
	margin := overwriteToneMargin
	if c.space {
		margin = overwriteSpaceMargin
	}
	return chinese-literal >= margin
}

// overwriteChar 是 syl 接在 start 前面的中文後面時解碼最好的字。
func (s *Session) overwriteChar(start int, syl zhuyin.Syllable) (rune, bool) {
	begin := start
	for begin > 0 && !s.literals[begin-1] && start-begin < decode.MaxWordLen-1 {
		begin--
	}
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model, Beam: s.Opt.Beam, Continue: s.Opt.Continue}
	words, ok := bestWords(dec, append(slices.Clone(s.syls[begin:start]), syl))
	if !ok || len(words) == 0 {
		return 0, false
	}
	w := []rune(words[len(words)-1].word)
	return w[len(w)-1], true
}

// overwriteGroup：units 的按鍵是打錯又補對的（↓ 清單先列蓋完的字，不看神經模型）。
func (s *Session) overwriteGroup(units []keyUnit) (keyGroup, bool) {
	if !s.mixedOverwrite() {
		return keyGroup{}, false
	}
	var keys strings.Builder
	for _, u := range units {
		if u.fixed != "" || u.keypad {
			return keyGroup{}, false
		}
		keys.WriteString(u.keys)
	}
	o, ok := s.overwriteReading(keys.String())
	return o.group, ok
}

// notePendingOverwrite：待定按鍵的 ↓ 選了蓋完的字（5jiy6 選「昨」）時記成這次的選擇，送出時學（同 noteMemoryChoice）。
func (s *Session) notePendingOverwrite(id int) func() {
	c, raw, at := s.cand, s.mixed.raw, s.cursor
	if _, ok := c.mixedChars[id]; !ok || c.mixedAt != 0 || !s.mixedOverwrite() {
		return func() {}
	}
	if _, ok := s.overwriteReading(raw); !ok {
		return func() {}
	}
	return func() {
		p, form := memorySpan{at, at + 1}, []personal.MixedToken{{Keys: raw, Kind: personal.MixedChinese}}
		if s.memoryFormIs(p, form) {
			s.memory.choices = append(s.memory.choices, memoryChoice{memorySpan: p, form: form})
		}
	}
}
