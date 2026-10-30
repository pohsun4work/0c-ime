package session

import (
	"slices"
	"strings"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// resplit：整合風格（操作規則 mixed_resplit），組字區查得回原按鍵，↓／Tab 換詞長可以跨過英數，整段換成英文、注音或中文。
func (s *Session) resplit() bool { return s.InputStyle.MixedResplit != "" && s.mixedInput() }

// resplitKeepsLiteral：整合風格只有接著打的字、↓ 與 Esc 把游標前的原樣注音收回待定按鍵。方向鍵、Enter 這類鍵收回後
// 同一鍵又定案，定案沿用收回前的路徑，神經選字時會多出一個字（ㄍ2ㄍ 按 ← 變 ㄍㄍ2ㄍ）。
func (s *Session) resplitKeepsLiteral(k string) bool {
	return s.resplit() && len([]rune(k)) != 1 && k != proto.KeyDown && k != proto.KeyEscape
}

// keyUnit 是組字區裡一段查得回原按鍵的位置：中文音節、英數與符號各一格，原樣注音連同聲調整個算一段。
// 標示整段按 ↓（openMarkSpan）時，查不到按鍵的字與分隔的空白照畫面留著（fixed 是畫面上的字，沒有按鍵）。
type keyUnit struct {
	start, end int
	keys       string
	keypad     bool
	fixed      string
}

// keyUnitAt 回傳位置 pos 所在的 keyUnit。補字接受的英文、= 選單與符號表選的字查不到按鍵。
func (s *Session) keyUnitAt(pos int) (keyUnit, bool) {
	if !s.literals[pos] {
		keys := s.mixedKeys[pos]
		return keyUnit{start: pos, end: pos + 1, keys: keys}, keys != ""
	}
	if p, ok := s.pendingLiteralAt(pos); ok {
		keys := s.mixedKeys[p.start]
		return keyUnit{start: p.start, end: p.end() + p.mark(), keys: keys}, keys != ""
	}
	// 符號的另一種寬度（insertPairedBracket）也記在 literalKeys，不是按鍵；按鍵都是 ASCII。
	if keys := s.literalKeys[pos]; keys != "" && !strings.ContainsFunc(keys, func(r rune) bool { return r > unicode.MaxASCII }) {
		return keyUnit{start: pos, end: pos + 1, keys: keys}, true
	}
	origin, ok := s.mixedOrigins[pos]
	text := string(s.syls[pos])
	if !ok || origin.assisted || len(text) != 1 || text[0] > unicode.MaxASCII {
		return keyUnit{}, false
	}
	return keyUnit{start: pos, end: pos + 1, keys: text, keypad: origin.keypad}, true
}

// pendingLiteralAt 回傳涵蓋 pos 的原樣注音（不論有沒有聲調）。
func (s *Session) pendingLiteralAt(pos int) (pendingLiteral, bool) {
	for _, p := range s.pendingLiterals {
		if p.start <= pos && pos < p.end()+p.mark() {
			return p, true
		}
	}
	return pendingLiteral{}, false
}

// keyUnits 把 [start, end) 切成 keyUnit。分隔用的空白只能是最後一格（當一聲，$20␣ 是「$單」），不能跨過；
// 有查不到按鍵的位置、切到原樣注音的一半，或整段只有空白時回傳 false。
func (s *Session) keyUnits(start, end int) ([]keyUnit, bool) {
	var units []keyUnit
	for pos := start; pos < end; {
		u, ok := s.keyUnitAt(pos)
		if !ok || u.start != pos || u.end > end || len(units) > 0 && units[len(units)-1].keys == " " {
			return nil, false
		}
		units = append(units, u)
		pos = u.end
	}
	return units, len(units) > 0 && units[0].keys != " "
}

// literalTabbable：整合風格的字面清單（mp3 的 3、npm␣ 的空白）按 Tab 換得到別的詞長時，標題一樣提示 Tab（條件同 cycleCandidateLength）。
func (s *Session) literalTabbable() bool {
	if !s.resplit() || !s.canStepCandidate() {
		return false
	}
	_, maxLen := s.candidateAnchor(s.cand)
	return maxLen > 1
}

// keyMaxLen 是 Tab 換詞長的上限：n 是詞庫最長的詞長，連續查得到按鍵的範圍裡有照字面的字時延伸到最遠的那個字。
func (s *Session) keyMaxLen(anchor int, before bool, n int) int {
	if !s.resplit() {
		return n
	}
	return max(n, s.keyRun(anchor, before))
}

// keyRun 是從 anchor 往後（before 時往前）連續查得到按鍵、到最遠一個照字面的字的長度；空白只能是最右邊一格。
func (s *Session) keyRun(anchor int, before bool) int {
	pos, far := anchor, anchor
	if before {
		for pos > 0 {
			u, ok := s.keyUnitAt(pos - 1)
			if !ok || u.end != pos || u.keys == " " && pos != anchor {
				break
			}
			if pos = u.start; s.literals[pos] {
				far = pos
			}
		}
		return anchor - far
	}
	for pos < len(s.syls) {
		u, ok := s.keyUnitAt(pos)
		if !ok || u.start != pos {
			break
		}
		if pos = u.end; s.literals[u.start] {
			far = pos
		}
		if u.keys == " " {
			break
		}
	}
	return far - anchor
}

// keyGroup 是重新拆成注音的一段：一個音節的按鍵，或照字面的一個字元（不是注音鍵、數字鍵盤的數字、前面沒有注音的聲調鍵），
// 或照畫面留著的字（fixed，keys 是畫面上的字）。
type keyGroup struct {
	keys    string
	comp    zhuyin.Composer // 音節：聲調之前的注音
	tone    rune            // 音節：按下的聲調；0 是沒打聲調
	literal bool
	keypad  bool
	fixed   bool
}

// keyGroups 照「聲母 介音 韻母」的順序把按鍵接成音節，接上就不是任何音節的開頭時開新的音節，聲調鍵結束音節，
// 同聲調選填（untonedBreak）。
func (s *Session) keyGroups(units []keyUnit) []keyGroup {
	var out []keyGroup
	var cur keyGroup
	end := func() {
		if !cur.comp.Empty() {
			out = append(out, cur)
		}
		cur = keyGroup{}
	}
	for _, u := range units {
		if u.fixed != "" {
			end()
			out = append(out, keyGroup{keys: u.fixed, literal: true, fixed: true})
			continue
		}
		for _, r := range u.keys {
			sym, ok := s.layout.Symbol(r)
			cl := zhuyin.ClassOf(sym)
			switch {
			case u.keypad || !ok || cl == zhuyin.None || cl == zhuyin.Tone && cur.comp.Empty():
				end()
				out = append(out, keyGroup{keys: string(r), literal: true, keypad: u.keypad})
			case cl == zhuyin.Tone:
				cur.keys += string(r)
				cur.tone = sym
				end()
			default:
				if !cur.comp.Empty() && !zhuyin.LegalPrefix(zhuyin.Syllable(cur.comp.String()+string(sym))) {
					end()
				}
				cur.keys += string(r)
				cur.comp.Add(sym)
			}
		}
	}
	end()
	return out
}

// syllable 回傳音節的讀音：沒打聲調時由解碼決定（zhuyin.Untone）。不是國語音節時回傳 false。
func (g keyGroup) syllable() (zhuyin.Syllable, bool) {
	c := g.comp.Clone()
	if g.tone == 0 {
		base := zhuyin.Syllable(c.String())
		return zhuyin.Untone(base), base.Legal()
	}
	done, ok := c.Add(g.tone)
	return done, ok && done.Legal()
}

// zhuyin 是音節在畫面上的注音，連同打的聲調（一聲不標）。
func (g keyGroup) zhuyin() string {
	c := g.comp.Clone()
	if done, ok := c.Add(g.tone); ok {
		return string(done)
	}
	return g.comp.String()
}

// chineseWord 是重新拆成中文的一個詞，從第 at 個 keyGroup 起。
type chineseWord struct {
	at      int
	word    string
	reading []zhuyin.Syllable
}

// chineseChoices 把 groups 換成中文：音節只有一段（照字面的字元只在頭尾，$20）時，列讀音相符、蓋滿那段的詞，
// 跟中文的 ↓ 一樣；沒有這種詞或音節分成好幾段時，各段取解碼最好的結果，列成一項。有組不成國語音節的按鍵時沒有中文。
func (s *Session) chineseChoices(groups []keyGroup) [][]chineseWord {
	type run struct {
		at   int
		syls []zhuyin.Syllable
	}
	var runs []run
	for i, g := range groups {
		if g.literal {
			continue
		}
		syl, ok := g.syllable()
		if !ok {
			return nil
		}
		if n := len(runs); n > 0 && runs[n-1].at+len(runs[n-1].syls) == i {
			runs[n-1].syls = append(runs[n-1].syls, syl)
		} else {
			runs = append(runs, run{i, []zhuyin.Syllable{syl}})
		}
	}
	if len(runs) == 0 {
		return nil
	}
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model}
	if len(runs) == 1 {
		syls := runs[0].syls
		pinned := func(w string) bool { // 同 mixedToneChars：選過的單字與自訂詞排前面
			return len(syls) == 1 && s.model.PickedSingle(syls[0], w) || s.model.VocabReading(w, syls)
		}
		var out [][]chineseWord
		for _, c := range dec.Candidates(input.Exact{}.Expand(syls), 0, len(syls), dec.InitialLeft(), pinned) {
			if len(c.Entry.Reading) == len(syls) && len([]rune(c.Entry.Surface)) == len(syls) {
				out = append(out, []chineseWord{{runs[0].at, c.Entry.Surface, c.Entry.Reading}})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	var best []chineseWord
	for _, r := range runs {
		words, ok := bestWords(dec, r.syls)
		if !ok {
			return nil
		}
		for _, w := range words {
			w.at += r.at
			best = append(best, w)
		}
	}
	return [][]chineseWord{best}
}

// bestWords 是 syls 解碼最好的一串詞（at 從 0 起），讀音是詞庫的讀音；有詞跟音節對不上時回傳 false。
func bestWords(dec *decode.Decoder, syls []zhuyin.Syllable) ([]chineseWord, bool) {
	lat := input.Exact{}.Expand(syls)
	path, _ := dec.Best(lat, nil, false)
	var out []chineseWord
	covered := 0
	for _, g := range path {
		reading, ok := decode.Reading(dec.Lex, lat, g.Start, g.End, g.Word)
		if g.Partial || g.Start != covered || !ok || len(reading) != g.End-g.Start || len([]rune(g.Word)) != len(reading) {
			return nil, false
		}
		out = append(out, chineseWord{g.Start, g.Word, reading})
		covered = g.End
	}
	return out, covered == len(syls)
}

// keyGroupsText 是 groups 在畫面上的樣子：words 是 nil 時是注音，否則是中文。
func (s *Session) keyGroupsText(groups []keyGroup, words []chineseWord) string {
	var b strings.Builder
	for i, g := range groups {
		switch {
		case g.fixed:
			b.WriteString(g.keys)
		case g.literal:
			b.WriteString(s.typedText(g.keys))
		case words == nil:
			b.WriteString(g.zhuyin())
		default:
			for _, w := range words {
				if w.at <= i && i < w.at+len(w.reading) {
					b.WriteRune([]rune(w.word)[i-w.at])
				}
			}
		}
	}
	return b.String()
}

// openKeySpan 開 [pos, pos+length) 的跨英數候選：第一項是畫面上的樣子，接著同中文的 ↓ 是中文、原按鍵、注音。
// 只有一個字元的符號照舊接著列它的符號選項。沒有別的寫法時不開，回傳 false。
func (s *Session) openKeySpan(pos, length int) bool {
	units, ok := s.keyUnits(pos, pos+length)
	return ok && s.openUnits(pos, length, units)
}

// openUnits 開 units（[pos, pos+length)）的跨英數候選，見 openKeySpan。
func (s *Session) openUnits(pos, length int, units []keyUnit) bool {
	s.refresh()
	c := &candState{start: pos, length: length, keyLiterals: map[int]string{}, replace: map[int]func(){}, across: true}
	seen := map[string]bool{}
	add := func(text string, insert func()) bool {
		if text == "" || seen[text] {
			return false
		}
		seen[text] = true
		c.replace[len(c.items)] = insert
		c.items = append(c.items, rawCandidate(text))
		return true
	}
	add(s.shownText(pos, pos+length), nil)
	groups := s.keyGroups(units)
	for _, words := range s.chineseChoices(groups) {
		add(s.keyGroupsText(groups, words), func() { s.insertKeyGroups(groups, words) })
	}
	var english strings.Builder
	for _, u := range units {
		english.WriteString(u.keys + u.fixed)
	}
	if add(s.typedText(english.String()), func() { s.insertKeyUnits(units) }) {
		c.keyLiterals[len(c.items)-1] = english.String()
	}
	add(s.keyGroupsText(groups, nil), func() { s.insertKeyGroups(groups, nil) })
	if _, pending := s.pendingLiteralAt(pos); length == 1 && s.literals[pos] && !pending {
		for _, alt := range s.symbolChoices(string(s.syls[pos])) {
			add(alt, func() {
				s.insertKeyText(alt)
				s.setLiteralKey(s.cursor-1, units[0].keys) // 換了符號也查得回按鍵
			})
		}
	}
	if len(c.items) < 2 {
		return false
	}
	s.cand = c
	return true
}

// spanUnits 把 [start, end) 切成 keyUnit，不要求每個位置都查得回按鍵：空白照留、查不到按鍵的字照畫面、
// 沒記按鍵的中文用讀音的按鍵。keyed 是有沒有任何一段有按鍵。
func (s *Session) spanUnits(start, end int) (units []keyUnit, keyed bool) {
	for pos := start; pos < end; {
		u, ok := s.keyUnitAt(pos)
		switch {
		case ok && u.start == pos && u.end <= end && (u.keys != " " || u.end == end):
		case !s.literals[pos]:
			u = keyUnit{start: pos, end: pos + 1, keys: s.layout.Keys(s.syls[pos])}
		default:
			u = keyUnit{start: pos, end: pos + 1, fixed: s.shownText(pos, pos+1)}
		}
		keyed = keyed || u.fixed == ""
		units = append(units, u)
		pos = u.end
	}
	return units, keyed
}

// addSpanChoices：整合風格多字的中文候選同跨英數候選的順序，畫面上的字不是候選的詞時排第一個（選了不換），
// 原始按鍵後面接整段的注音。
func (s *Session) addSpanChoices(c *candState) {
	if !s.resplit() || c.length < 2 {
		return
	}
	start, end := c.start, c.start+c.length
	c.replace = map[int]func(){}
	if shown := s.shownText(start, end); !slices.ContainsFunc(c.items, func(x decode.Candidate) bool { return x.Entry.Surface == shown }) {
		c.items = slices.Insert(c.items, 0, rawCandidate(shown))
		if c.matched != nil {
			c.matched = slices.Insert(c.matched, 0, "")
		}
		c.keyLiterals = shiftKeys(c.keyLiterals, 0, 1)
		c.replace[0] = nil
	}
	units, _ := s.spanUnits(start, end)
	groups := s.keyGroups(units)
	c.replace[len(c.items)] = func() { s.insertKeyGroups(groups, nil) }
	c.items = append(c.items, rawCandidate(s.keyGroupsText(groups, nil)))
}

// selectReplace 套用 replace 的第 id 項：把那段換掉，游標原本在尾端（前方模式）就留在新的尾端，否則留在開頭；
// 開了「選字後游標移到下一個字」也移到尾端。畫面上的樣子只關閉候選。
func (s *Session) selectReplace(id int) bool {
	c := s.cand
	s.cand = nil
	insert := c.replace[id]
	if insert == nil {
		return true
	}
	start, end := c.start, c.start+c.length
	tail := s.AdvanceAfterSelect || s.cursor == end
	left, right := s.cutHalves(start, end)
	s.eraseSpan(start, end)
	insert()
	if left.Len > 0 {
		s.locks[start-left.Len] = left
	}
	if right.Len > 0 {
		s.locks[s.cursor] = right
	}
	if !tail {
		s.cursor = start
	}
	s.refresh()
	return true
}

// cutHalves 是 [start, end) 切開的詞在範圍外的兩半，鎖成被字面文字切開的詞（同 insertLiteral）：換成照字面的字時
// 照畫面留著（「打幾」換成注音，「幾句」的「句」不會重新解成「據」），換成中文時 refresh 拿掉鎖、照常重新解碼。
func (s *Session) cutHalves(start, end int) (left, right decode.Lock) {
	for _, g := range s.path {
		word := []rune(g.Word)
		if len(word) != g.End-g.Start {
			continue
		}
		if g.Start < start && start < g.End {
			left = decode.Lock{Word: string(word[:start-g.Start]), Len: start - g.Start, LiteralBoundary: 1, Matched: s.matchedFor(g.Start, start-g.Start)}
		}
		if g.Start < end && end < g.End {
			right = decode.Lock{Word: string(word[end-g.Start:]), Len: g.End - end, LiteralBoundary: -1, Matched: s.matchedFor(end, g.End-end)}
		}
	}
	return left, right
}

// eraseSpan 刪掉 [start, end)，游標停在 start。
func (s *Session) eraseSpan(start, end int) {
	for s.cursor = end; s.cursor > start; {
		s.eraseBeforeCursor()
	}
}

// insertKeyText 在游標處照字面插入 text。不用 insertLiteral：它每次先解碼、把跨過游標的詞切開鎖住兩半，
// 這裡是替換使用者選的整段，鎖定只留原有的和選的詞，插完才解碼一次。
func (s *Session) insertKeyText(text string) {
	if s.literals == nil {
		s.literals = map[int]bool{}
	}
	for _, r := range text {
		at := s.cursor
		s.insert(zhuyin.Syllable(string(r)))
		s.literals[at] = true
	}
}

// insertKeyUnits 把原按鍵照字面插回去，當成明確選了英數（學習同 ↓ 選回原按鍵）。
func (s *Session) insertKeyUnits(units []keyUnit) {
	for _, u := range units {
		if u.fixed != "" {
			s.insertKeyText(u.fixed)
			continue
		}
		s.insertKeyLiteral(u.keys, u.keypad)
	}
}

func (s *Session) insertKeyLiteral(keys string, keypad bool) {
	at := s.cursor
	s.insertKeyText(keys)
	s.markWide(at)
	if s.mixedOrigins == nil {
		s.mixedOrigins = map[int]mixedOrigin{}
	}
	for pos := at; pos < s.cursor; pos++ {
		s.mixedOrigins[pos] = mixedOrigin{explicit: true, keypad: keypad}
	}
}

// insertKeyGroups 插入重新拆好的音節：words 是 nil 時是注音（照字面，每個符號記下自己的按鍵），
// 否則是中文（音節換成詞庫讀音、記下原按鍵，詞當成手選鎖定）。照字面的字元照字面插回。
func (s *Session) insertKeyGroups(groups []keyGroup, words []chineseWord) {
	start, next := 0, 0 // words[next] 是插到一半的詞，從 start 開始
	for i, g := range groups {
		switch {
		case g.fixed:
			s.insertKeyText(g.keys)
		case g.literal:
			s.insertKeyLiteral(g.keys, g.keypad)
		case words == nil:
			s.insertZhuyinLiteral(g)
		default:
			w := words[next]
			if i == w.at {
				start = s.cursor
			}
			s.insertMixedReading(w.reading[i-w.at], g.keys)
			if i == w.at+len(w.reading)-1 {
				s.locks[start] = decode.Lock{Word: w.word, Len: len(w.reading)}
				next++
			}
		}
	}
}

// insertZhuyinLiteral 把一個音節的注音照字面插入；一聲的空白記在最後一個符號上。
func (s *Session) insertZhuyinLiteral(g keyGroup) {
	text, keys := []rune(g.zhuyin()), []rune(g.keys)
	for i, r := range text {
		s.insertKeyText(string(r))
		key := string(keys[i])
		if i == len(text)-1 {
			key = string(keys[i:])
		}
		s.setLiteralKey(s.cursor-1, key)
	}
}

// shownText 是 [start, end) 在畫面上的字，全形時打的字照畫面換成全形。
func (s *Session) shownText(start, end int) string {
	var text strings.Builder
	if len(s.path) == 0 {
		for i := start; i < end; i++ {
			displayed, _ := s.fallbackDisplay(i, s.syls[i])
			text.WriteString(displayed)
		}
		return text.String()
	}
	for _, g := range s.path {
		if g.End <= start || g.Start >= end {
			continue
		}
		word := s.wideWord(g)
		if g.End-g.Start == 1 {
			text.WriteString(word)
		} else {
			runes := []rune(word)
			text.WriteString(string(runes[max(0, start-g.Start):min(len(runes), end-g.Start)]))
		}
	}
	return text.String()
}

// tabFromMixed 在還沒定案的混打按鍵候選上按 Tab（d 是 1）或 Shift+Tab（-1）：先照畫面定案，再從剛定案那段的長度換詞長。
func (s *Session) tabFromMixed(d int) {
	n := len([]rune(s.mixedText())) // 定案時可能改到前面的字（reviseLetterI），長度照畫面上的待定按鍵算
	s.cand = nil
	s.flushMixed()
	if n == 0 || n > s.cursor {
		return
	}
	flushed := &candState{start: s.cursor - n, length: n, before: true, literal: true}
	s.cand = flushed
	s.cycleCandidateLength(d)
	if s.cand == flushed {
		s.cand = nil
	}
}

// keepResplitKeys 記下位置 at 的音節或原樣注音的原按鍵。
func (s *Session) keepResplitKeys(at int, keys string) {
	if keys == "" || !s.resplit() {
		return
	}
	if s.mixedKeys == nil {
		s.mixedKeys = map[int]string{}
	}
	s.mixedKeys[at] = keys
}

// dropToneKey 去掉位置 at 的原按鍵最後的聲調鍵：原樣音節的聲調被刪掉時，按鍵跟著畫面。
func (s *Session) dropToneKey(at int) {
	keys := s.mixedKeys[at]
	if keys == "" || !s.resplit() {
		return
	}
	if sym, ok := s.layout.Symbol(rune(keys[len(keys)-1])); ok && zhuyin.ClassOf(sym) == zhuyin.Tone {
		s.mixedKeys[at] = keys[:len(keys)-1]
	}
}

// markKeyed 記下 at 到游標之間剛打的字的來源（mixedOrigin.keyed）。
func (s *Session) markKeyed(at int, keypad bool) {
	if !s.resplit() {
		return
	}
	if s.mixedOrigins == nil {
		s.mixedOrigins = map[int]mixedOrigin{}
	}
	for pos := at; pos < s.cursor; pos++ {
		origin, ok := s.mixedOrigins[pos]
		origin.keyed = origin.keyed || !ok
		origin.keypad = origin.keypad || keypad
		s.mixedOrigins[pos] = origin
	}
}
