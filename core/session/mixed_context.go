package session

import (
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// rejudgePasses 是每次重看最多改幾處：改了一處就從頭再看，改成的樣子不會再被規則改回去。
const rejudgePasses = 8

// zhuyinTextKeys 是會改成注音文的按鍵：其他字母接空白官方已經留成注音（c␣ 是 ㄏ，#190），a 是英文字、1 2 是數字。
var zhuyinTextKeys = []string{"a", "2", "1"}

// mixedContext：整合風格（操作規則 mixed_context），要有 mixed_resplit 的按鍵紀錄。
func (s *Session) mixedContext() bool { return s.InputStyle.MixedContext != "" && s.resplit() }

// rejudge：混打照官方判斷，每個按鍵處理完與送出前再照上下文重看查得回按鍵的部分，規則成立才改（rejudgePending、
// rejudgeUnits、rejudgeZhuyinText），改成的樣子官方也打得出來。待定按鍵還沒定案時只看游標前，後面的字等它定案再看。
// 鎖定（選過的字）、從 ↓ 明確選的不改；候選窗開著、注音或英文打到一半、標示中不看。
func (s *Session) rejudge() {
	if !s.mixedContext() || s.english || s.cand != nil || !s.comp.Empty() || s.mark != nil {
		return
	}
	s.rejudgePending()
	for range rejudgePasses {
		end := len(s.syls)
		if s.mixed.raw != "" {
			end = s.cursor
		}
		if !s.rejudgeUnits(end) && !s.rejudgeZhuyinText(end) {
			return
		}
	}
}

// rejudgePending 重看官方組不成字、照字面留著的待定「字母＋聲調鍵」：同一個字母多按了幾次（ggggk6、jji3 是什、我），
// 或在兩個聲母之間切開、前段是英文（initialConflict）。
func (s *Session) rejudgePending() {
	p := s.mixed
	if p.choice != 0 || p.accepted != "" || s.englishWord || !s.explicitTone(p.raw) {
		return
	}
	body := p.raw[:len(p.raw)-1]
	if !isLetters(body) || s.knownEnglish(body) || s.mixedText() != p.raw {
		return
	}
	tone, _ := s.layout.Symbol(rune(p.raw[len(p.raw)-1]))
	if merged := collapseRepeats(body); merged != body && s.keysInSyllableOrder(merged) {
		if done, ok := s.mixedReading(merged, tone); ok {
			s.clearMixed()
			s.insertMixedTyped(done, p.raw)
			return
		}
	}
	if at, done, ok := s.initialConflict(body, tone); ok {
		s.clearMixed()
		s.insertMixedLiteral(body[:at], false, false)
		s.insertMixedTyped(done, p.raw[at:])
	}
}

// initialConflict：聲調鍵前的字母 body 組不成音節，在兩個聲母之間切開、後段照順序組得成音節時（gck6 → g合），
// 回傳切點與音節。
func (s *Session) initialConflict(body string, tone rune) (int, zhuyin.Syllable, bool) {
	if _, ok := s.mixedComposer(body); ok {
		return 0, "", false
	}
	for at := 1; at < len(body); at++ {
		if !s.keyIsInitial(rune(body[at-1])) || !s.keyIsInitial(rune(body[at])) || !s.keysInSyllableOrder(body[at:]) {
			continue
		}
		if done, ok := s.mixedReading(body[at:], tone); ok {
			return at, done, true
		}
	}
	return 0, "", false
}

// rejudgeUnits 照規則改 end 前的一個 keyUnit，改了回報 true：反序打的英文字接空白（url␣ 是交，照順序打注音不會反序，
// #119）與不是國語音節的注音接空白（npm␣ 是 ㄙㄩㄣ）改回英數；重複鍵（ggg＋什）合成一個音節（mergeRepeats）；
// 數字鍵盤打斷的單一字母注音（e KP2 的 ㄍ）改回字母。
func (s *Session) rejudgeUnits(end int) bool {
	var units []keyUnit
	for pos := 0; pos < end; {
		u, ok := s.keyUnitAt(pos)
		if !ok || u.start != pos || u.end > end {
			pos++
			continue
		}
		units = append(units, u)
		pos = u.end
	}
	for i, u := range units {
		body, space := strings.CutSuffix(u.keys, " ")
		p, pending := s.pendingLiteralAt(u.start)
		chinese, letters := !s.literals[u.start], space && isLetters(body)
		switch {
		case letters && chinese && !s.keysInSyllableOrder(body) && s.knownEnglish(body),
			letters && pending && p.tone == 1 && !p.untoned && len(body) >= 2 && !p.syllable().Legal():
			if s.replaceSpan(u.start, u.end, func() { s.insertMixedLiteral(u.keys, false, false) }) {
				return true
			}
		case chinese && s.explicitTone(u.keys):
			if s.mergeRepeats(units[:i], u) {
				return true
			}
		case u.keypad && i > 0:
			if prev := units[i-1]; prev.end == u.start && s.untonedLetterAt(prev.start) &&
				s.replaceSpan(prev.start, prev.end, func() { s.insertMixedLiteral(prev.keys, false, false) }) {
				return true
			}
		}
	}
	return false
}

// mergeRepeats：中文音節 u 前面緊接著照字面的、跟它第一個按鍵相同的字母（ggg＋什），合成一個音節（什）。
// 這串字母是更長的英文的一部分（to＋欸）時不合。
func (s *Session) mergeRepeats(before []keyUnit, u keyUnit) bool {
	start, head := u.start, ""
	for i := len(before) - 1; i >= 0; i-- {
		b := before[i]
		if b.end != start || !s.plainLiteralAt(b.start) || b.keys != u.keys[:1] {
			break
		}
		start, head = b.start, b.keys+head
	}
	if head == "" || !isLetters(head) || start > 0 && s.letterAt(start-1) {
		return false
	}
	syl := s.syls[u.start]
	return s.replaceSpan(start, u.end, func() { s.insertMixedReading(syl, head+u.keys) })
}

// zhuyinUnit 是注音文的一格：照字面的 a␣ 2␣ 1␣（literal），或接空白留成注音的音節（改成的 ㄇ、官方的 c␣ ㄏ）。
type zhuyinUnit struct {
	start, end int
	keys       string
	literal    bool
}

func (s *Session) zhuyinUnitAt(pos int) (zhuyinUnit, bool) {
	if p, ok := s.syllableLiteralAt(pos); ok {
		keys := s.mixedKeys[p.start]
		return zhuyinUnit{p.start, p.end() + p.mark(), keys, false}, p.start == pos && !p.untoned && strings.HasSuffix(keys, " ")
	}
	text := string(s.syls[pos])
	if !slices.Contains(zhuyinTextKeys, text) || pos+1 >= len(s.syls) || string(s.syls[pos+1]) != " " {
		return zhuyinUnit{}, false
	}
	for at := pos; at < pos+2; at++ {
		o, ok := s.mixedOrigins[at]
		if !s.plainLiteralAt(at) || !ok || o.assisted || o.explicit || o.keyed || o.keypad {
			return zhuyinUnit{}, false
		}
	}
	return zhuyinUnit{pos, pos + 2, text + " ", true}, true
}

// rejudgeZhuyinText 照上下文把 end 前連續的注音文改成注音，或把 a␣ 留成的 ㄇ 改回 a，改了回報 true。
func (s *Session) rejudgeZhuyinText(end int) bool {
	for pos := 0; pos < end; {
		var run []zhuyinUnit
		for at := pos; at < end; {
			u, ok := s.zhuyinUnitAt(at)
			if !ok || u.end > end {
				break
			}
			run = append(run, u)
			at = u.end
		}
		if len(run) == 0 {
			pos++
			continue
		}
		if s.rejudgeZhuyinRun(run) {
			return true
		}
		pos = run[len(run)-1].end
	}
	return false
}

// rejudgeZhuyinRun：a␣ 2␣ 1␣ 緊接在中文或注音後面的一串都改成 ㄇ ㄉ ㄅ；句首、空白或標點後面要有兩格以上才改
// （第一個先照字面，打了第二個一起改）；英數後面不改。a 後面接著英文字（a apple）照字面，留成的 ㄇ 也改回 a。
func (s *Session) rejudgeZhuyinRun(run []zhuyinUnit) bool {
	at := run[0].start
	var convert bool
	switch {
	case at == 0:
		convert = len(run) >= 2
	case !s.plainLiteralAt(at - 1):
		convert = true
	default:
		before := string(s.syls[at-1])
		convert = len(run) >= 2 && !englishLetter(before) && !isDigitText(before)
	}
	for _, u := range run {
		if u.literal && !convert {
			continue
		}
		word := u.keys == "a " && s.englishWordAt(u.end)
		switch {
		case u.literal && !word:
			if s.replaceSpan(u.start, u.end, func() { s.insertZhuyinUnit(u.keys) }) {
				return true
			}
		case !u.literal && word:
			if s.replaceSpan(u.start, u.end, func() { s.insertMixedLiteral(u.keys, false, false) }) {
				return true
			}
		}
	}
	return false
}

// insertZhuyinUnit 照官方接空白留下注音的樣子（keepMixedSyllable）插入注音文的按鍵（a␣ 是 ㄇ）。
func (s *Session) insertZhuyinUnit(keys string) {
	c, _ := s.mixedComposer(strings.TrimSuffix(keys, " "))
	before := c.Clone()
	done, _ := c.Add(zhuyin.Tone1)
	at := s.cursor
	s.insertSyllableLiteral(before, done)
	s.keepResplitKeys(at, keys)
}

// markChosen：chosen（↓ 選了注音解釋，choice 2）時把待定按鍵完成的音節或注音（at 到游標）記成明確選的。
func (s *Session) markChosen(chosen bool, at int) {
	if !chosen || !s.mixedContext() {
		return
	}
	if s.mixedOrigins == nil {
		s.mixedOrigins = map[int]mixedOrigin{}
	}
	for pos := at; pos < s.cursor; pos++ {
		origin := s.mixedOrigins[pos]
		origin.explicit = true
		s.mixedOrigins[pos] = origin
	}
}

// replaceSpan 把 [start, end) 換成 insert 插入的內容，游標在後面時跟著平移、在中間時移到換上的內容後面。
// 有鎖定或明確選的時不換，回傳 false。換的前後都是照字面的字時，其他字照畫面保留（同 flushMixed），不換掉神經選的字。
func (s *Session) replaceSpan(start, end int, insert func()) bool {
	for pos := start; pos < end; pos++ {
		if s.lockedAt(pos) || s.mixedOrigins[pos].explicit {
			return false
		}
	}
	s.refresh()
	previous, literal := slices.Clone(s.path), s.allLiteral(start, end)
	cursor := s.cursor
	s.eraseSpan(start, end)
	insert()
	n := s.cursor - start
	literal = literal && s.allLiteral(start, s.cursor)
	switch {
	case cursor >= end:
		s.cursor = cursor + n - (end - start)
	case cursor <= start:
		s.cursor = cursor
	}
	s.refresh()
	if literal {
		s.keepSpanDisplay(previous, start, end, n)
	}
	return true
}

// keepSpanDisplay 是 replaceSpan 換掉照字面的字之後的畫面：[start, end) 換成 n 個照字面的字，其他照 previous。
func (s *Session) keepSpanDisplay(previous []decode.Seg, start, end, n int) {
	var kept []decode.Seg
	for _, g := range previous {
		switch {
		case g.End <= start:
			kept = append(kept, g)
		case g.Start >= end:
			g.Start, g.End = g.Start-(end-start), g.End-(end-start)
			kept = append(kept, g)
		}
	}
	s.keepMixedDisplay(kept, start, n)
}

func (s *Session) allLiteral(start, end int) bool {
	for pos := start; pos < end; pos++ {
		if !s.literals[pos] {
			return false
		}
	}
	return true
}

// plainLiteralAt：pos 是照字面的字，不是原樣留下的注音。
func (s *Session) plainLiteralAt(pos int) bool {
	_, pending := s.pendingLiteralAt(pos)
	return s.literals[pos] && !pending
}

// letterAt：pos 是照字面的英文字母。
func (s *Session) letterAt(pos int) bool {
	return s.literals[pos] && englishLetter(string(s.syls[pos]))
}

// keypadWordLetters：待定的字母 raw 接在數字鍵盤夾在字母中間的英數詞後面（e KP2 e 的第二個 e）：畫面照字面、
// 接空白也是英數（mixedText、finishMixedTone），接聲調鍵照舊組成中文（B KP1 x.6 是 B1樓）。
func (s *Session) keypadWordLetters(raw string) bool {
	return s.mixedContext() && isLetters(raw) && s.keypadWordBefore(s.cursor)
}

// keypadWordBefore：pos 前面是字母接數字鍵盤的數字，中間可以隔著照字面的字母（e2、e2e）。
func (s *Session) keypadWordBefore(pos int) bool {
	for pos > 0 && s.letterAt(pos-1) {
		pos--
	}
	digits := pos
	for pos > 0 && s.literals[pos-1] && s.mixedOrigins[pos-1].keypad {
		pos--
	}
	return pos < digits && pos > 0 && s.letterAt(pos-1)
}

// untonedLetterAt：pos 是一個字母打的、沒打聲調的注音（數字鍵盤打斷的 e 留成 ㄍ）。
func (s *Session) untonedLetterAt(pos int) bool {
	p, ok := s.pendingLiteralAt(pos)
	return ok && p.tone == 0 && p.start == pos && p.end() == pos+1 && englishLetter(s.mixedKeys[pos])
}

// englishWordAt：pos 起是照字面打的英文字（兩個字母以上），後面不是數字。
func (s *Session) englishWordAt(pos int) bool {
	end := pos
	for end < len(s.syls) && s.letterAt(end) {
		end++
	}
	if end-pos < 2 || end < len(s.syls) && s.literals[end] && isDigitText(string(s.syls[end])) {
		return false
	}
	var word strings.Builder
	for _, syl := range s.syls[pos:end] {
		word.WriteString(string(syl))
	}
	return s.knownEnglish(word.String())
}

// knownEnglish：英文字典、使用者加的英文詞（#146）或學過的英文。
func (s *Session) knownEnglish(word string) bool {
	return s.mixedEnglishWord(word) || s.vocabEnglish(word)
}

// explicitTone：按鍵以明確的聲調鍵（3 4 6 7）結尾，不是空白。
func (s *Session) explicitTone(keys string) bool {
	if keys == "" || keys[len(keys)-1] == ' ' {
		return false
	}
	sym, ok := s.layout.Symbol(rune(keys[len(keys)-1]))
	return ok && zhuyin.ClassOf(sym) == zhuyin.Tone
}

func isLetters(t string) bool {
	return t != "" && !strings.ContainsFunc(t, func(r rune) bool { return !englishLetter(string(r)) })
}

func collapseRepeats(t string) string {
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		if i == 0 || t[i] != t[i-1] {
			b.WriteByte(t[i])
		}
	}
	return b.String()
}
