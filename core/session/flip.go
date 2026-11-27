package session

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// ↓ 翻邊（P8）：混打判定之後，↓ 清單翻得回另一邊。同一串按鍵是兩次結算（聲調鍵、空白）之間打的按鍵（市值2萬 的 2j04）。
//   - 神經判過的字（mixedOrigin.judged）：同一串按鍵的其他切法排在畫面上的字後面（斷 的 2萬、2萬 的 斷）。
//     前段照字面的（2萬 的 2）Tab 換到整串的清單：修改「2萬」列其他切法的所有字（斷 段 緞…）。
//   - 照字面留下的英數（ssl、up）：規則組得出的字與注音（孬…、ㄋㄠ）。

const (
	// flipFollowRunes：切法的字給神經評分時帶的後文字數（萬 後面的 多、塊錢）。
	flipFollowRunes = 4
	// flipMargin：切法比畫面上的樣子差這麼多以內才列。
	flipMargin = 10.0
)

// flipPick 是 ↓ 翻邊的一項：組字區 [start, end) 換成前段 prefix 照字面、後段音節 syl（原按鍵 keys）鎖定成 char；
// char 是空字串時原樣留下注音（ㄋㄠ）。keep 是畫面上的樣子本身（ssl、2萬、說不），選了不動。
type flipPick struct {
	start, end int
	prefix     string
	syl        zhuyin.Syllable
	keys       string
	char       string
	keep       bool
}

// markJudged 把 [from, s.cursor) 裡混打組成的音節記成神經判過的；prefix 是同一串按鍵照字面留在
// 第一個音節前面的前段（2萬 的 2）。
func (s *Session) markJudged(from int, prefix string) {
	for at := max(0, from); at < s.cursor; at++ {
		if s.literals[at] || s.mixedKeys[at] == "" {
			continue
		}
		if s.mixedOrigins == nil {
			s.mixedOrigins = map[int]mixedOrigin{}
		}
		o := s.mixedOrigins[at]
		o.judged, o.prefix = s.syls[at], prefix
		s.mixedOrigins[at] = o
		prefix = ""
	}
}

// judgedKeys：pos 是神經判過、還沒改過讀音的字時，同一串按鍵從 start 開始，all 是前段照字面的文字加上這格的按鍵，
// cut 是前段的長度。前段被改過時只看這格自己的按鍵。
func (s *Session) judgedKeys(pos int) (start int, all string, cut int, ok bool) {
	o, keys := s.mixedOrigins[pos], s.mixedKeys[pos]
	if o.judged == "" || keys == "" || s.literals[pos] || s.syls[pos] != o.judged {
		return 0, "", 0, false
	}
	start = pos - utf8.RuneCountInString(o.prefix)
	if !s.literalTextIs(start, pos, o.prefix) {
		start, o.prefix = pos, ""
	}
	return start, o.prefix + keys, len(o.prefix), true
}

// judgedSplits：同一串按鍵的其他切法（還沒選字）：前段沒有或是詞、數字、標點（mixedWordPrefix），後段有字母、照順序組成一個音節
// （斷 的 2萬、第 的 2億、2萬 的 斷；八 的 1啊 只有數字鍵，要的是 18）。
func (s *Session) judgedSplits(pos int) []flipPick {
	start, all, cut, ok := s.judgedKeys(pos)
	if !ok {
		return nil
	}
	var flips []flipPick
	for at := range len(all) {
		if at == cut || at > 0 && !s.mixedWordPrefix(all[:at]) || !strings.ContainsFunc(all[at:], unicode.IsLetter) {
			continue
		}
		if syls, ok := s.strictSyllables(all[at:]); ok && len(syls) == 1 {
			flips = append(flips, flipPick{start: start, end: pos + 1, prefix: all[:at], syl: syls[0], keys: all[at:]})
		}
	}
	return flips
}

// spanBack：同一串按鍵的清單（openFlipSpan）是從哪份清單 Tab 過來的，再 Tab 回得去。literal 時 at 是照字面清單的位置
// （市值|2萬 的 2），否則是字的清單的錨點（cycleCandidateLength）。
type spanBack struct {
	literal bool
	at      int
}

// openFlipSpan：q 是神經判過、前段照字面還在的字（2萬 的 萬）時，開同一串按鍵 [start, q+1) 的清單：畫面上的樣子（選了不動）、
// 其他切法的所有字（神經選的排第一，其他照前文）與注音。沒有其他切法的字時回 false、不動候選。
func (s *Session) openFlipSpan(q int, back spanBack, before bool) bool {
	if q < 0 || q >= len(s.syls) {
		return false
	}
	start, _, cut, ok := s.judgedKeys(q)
	if !ok || cut == 0 {
		return false
	}
	splits := s.judgedSplits(q)
	preferred := map[string]string{}
	for _, f := range s.chooseFlipChars(slices.Clone(splits)) {
		preferred[f.keys] = f.char
	}
	c := &candState{start: start, length: q + 1 - start, before: before, flips: map[int]flipPick{}, span: &back}
	add := func(f flipPick, text string) {
		c.flips[len(c.items)] = f
		c.items = append(c.items, rawCandidate(text))
	}
	add(flipPick{keep: true}, s.spanText(start, q+1-start))
	chars := 0
	for _, f := range splits {
		list := s.syllableChars(f.syl, start)
		if p := preferred[f.keys]; p != "" {
			list = slices.Insert(slices.DeleteFunc(list, func(c string) bool { return c == p }), 0, p)
		}
		for _, char := range list {
			f.char = char
			add(f, s.mixedLiteralText(f.prefix)+char)
		}
		chars += len(list)
		f.char = ""
		add(f, s.mixedLiteralText(f.prefix)+f.syl.Text())
	}
	if chars == 0 {
		return false
	}
	s.cand = c
	return true
}

// literalPrefixOf：照字面的 [start, start+length) 是神經判過的字 q 的前段（市值2萬 的 2）時回傳 q。
func (s *Session) literalPrefixOf(start, length int) (int, bool) {
	q := start + length
	for q < len(s.syls) && s.literals[q] {
		q++
	}
	if q >= len(s.syls) {
		return 0, false
	}
	from, _, cut, ok := s.judgedKeys(q)
	return q, ok && cut > 0 && from <= start
}

// judgedKeysSecond：神經判過的字，原按鍵排在畫面上的字後面（趴 的 q8、r萬 的 j04）。整串是數字鍵開頭、照順序打的注音
// （斷 和 2萬 的 2j04、邊 的 1u0）只因為數字鍵開頭而問神經，原按鍵不是另一邊，照舊。
func (s *Session) judgedKeysSecond(pos int) bool {
	_, all, _, ok := s.judgedKeys(pos)
	if !ok {
		return false
	}
	whole, strict := s.strictSyllables(all)
	return !('0' <= all[0] && all[0] <= '9' && strict && len(whole) == 1)
}

// literalTextIs：[start, end) 都是字面文字，內容是 text（全形時是它的全形）。
func (s *Session) literalTextIs(start, end int, text string) bool {
	if start < 0 {
		return false
	}
	var b strings.Builder
	for at := start; at < end; at++ {
		if !s.literals[at] {
			return false
		}
		b.WriteString(string(s.syls[at]))
	}
	return b.String() == text || b.String() == s.fullWidthText(text)
}

// chooseFlipChars 替每種切法選字：神經帶前文與後文（flipFollowRunes）比同音字；也跟畫面上的樣子比，不帶後文（後文是照畫面上的字
// 解的：賺快錢 的 快錢 對 5萬 不公平）差 flipMargin 以上的切法不列（翻邊 的 1u0␣ 不列 1煙、多 不列 2窩）。問不到神經時都不列。
func (s *Session) chooseFlipChars(flips []flipPick) []flipPick {
	if len(flips) == 0 || s.Rerank.Texts == nil || s.private || s.paused || s.english || s.mixedBusy > 0 {
		return nil
	}
	start, end := flips[0].start, flips[0].end
	follow := s.spanText(end, min(flipFollowRunes, len(s.syls)-end))
	var texts []string
	seen := map[string]int{}
	add := func(t string) int {
		if i, ok := seen[t]; ok {
			return i
		}
		if !s.Rerank.FitsText(t) {
			return -1
		}
		seen[t] = len(texts)
		texts = append(texts, t)
		return len(texts) - 1
	}
	shown := s.spanText(start, end-start)
	shownAt := add(shown)
	type option struct {
		char       string
		with, bare int // texts 的索引：帶後文、不帶後文；-1 是送不出去
	}
	limit := min(mixedNeuralNBest, (rerank.MaxTexts-1)/(2*len(flips)))
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model}
	options := make([][]option, len(flips))
	for i, f := range flips {
		for _, c := range dec.Candidates(input.Exact{}.Expand([]zhuyin.Syllable{f.syl}), 0, 1, dec.InitialLeft(), nil) {
			if len(options[i]) == limit {
				break
			}
			t := s.mixedLiteralText(f.prefix) + c.Entry.Surface
			options[i] = append(options[i], option{c.Entry.Surface, add(t + follow), add(t)})
		}
	}
	if shownAt < 0 || len(texts) < 2 {
		return nil
	}
	scores, err := s.scoreTexts(texts, start, mixedNeuralDeadline)
	if err != nil {
		return nil
	}
	out := flips[:0]
	for i, f := range flips {
		if len(options[i]) == 0 {
			continue
		}
		best := options[i][0]
		for _, o := range options[i][1:] {
			if o.with >= 0 && (best.with < 0 || scores[o.with] > scores[best.with]) {
				best = o
			}
		}
		if best.bare < 0 || scores[best.bare] < scores[shownAt]-flipMargin {
			continue
		}
		f.char = best.char
		out = append(out, f)
	}
	return out
}

// openRunFlips：pos 在使用者打的英數上（判成英文、照字面留下的 ssl、up；pos 是緊接在後的空白也算）時，↓ 第一項是那串英數
// （選了不動），接著是它整串照規則組得出的讀音的字（順序照前一個字的語境），最後是原樣的注音。空白當一聲用掉。
// 組不出一個音節的（ok、hello）、單一個字母（a 的 ㄇ 另見 lonePhoneticKeyAt）不開，回 false。
func (s *Session) openRunFlips(pos int) bool {
	a, b, ok := s.typedRun(pos)
	if !ok {
		return false
	}
	shown := s.spanText(a, b-a)
	run := strings.Map(asciiOf, shown)
	body, key, tone := run, " ", zhuyin.Tone1
	if sym, ok := layout.Standard.Symbol(rune(run[len(run)-1])); ok && zhuyin.ClassOf(sym) == zhuyin.Tone {
		body, key, tone = run[:len(run)-1], run[len(run)-1:], sym // mp3 的 3 是聲調
	}
	syl, ok := zhuyin.Syllable(""), false
	if syls, strict := s.strictSyllables(body + key); strict && len(syls) == 1 {
		syl, ok = syls[0], true
	} else if at, done, repaired := s.repairFor(body, key, tone); repaired && at == 0 {
		syl, ok = done, true
	}
	if !ok {
		return false
	}
	end, keys := b, run
	if key == " " && s.spaceAt(b) {
		end, keys = b+1, run+" "
	}
	items := []string{shown}
	flips := map[int]flipPick{0: {keep: true}}
	pick := flipPick{start: a, end: end, syl: syl, keys: keys}
	for _, char := range s.syllableChars(syl, a) {
		if len(items) > mixedToneCharLimit {
			break
		}
		pick.char = char
		flips[len(items)] = pick
		items = append(items, char)
	}
	pick.char = ""
	flips[len(items)] = pick
	items = append(items, syl.Text())
	s.openSymbols(items, a, end-a, false)
	s.cand.flips = flips
	return true
}

// typedRun：pos（或 pos 是緊接著的空白時它前面）那一串使用者打的英數 [a, b)（全形時是全形），至少兩個字、有字母。
func (s *Session) typedRun(pos int) (a, b int, ok bool) {
	key := func(at int) rune {
		if at < 0 || at >= len(s.syls) || !s.literals[at] {
			return 0
		}
		rs := []rune(string(s.syls[at]))
		if _, typed := s.mixedOrigins[at]; !typed || len(rs) != 1 {
			return 0
		}
		return asciiOf(rs[0])
	}
	alnum := func(at int) bool {
		r := key(at)
		return '0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
	}
	if !alnum(pos) {
		if !s.spaceAt(pos) || !alnum(pos-1) {
			return 0, 0, false
		}
		pos--
	}
	a, b = pos, pos+1
	for alnum(a - 1) {
		a--
	}
	for alnum(b) {
		b++
	}
	letter := false
	for at := a; at < b; at++ {
		letter = letter || key(at) > '9'
	}
	return a, b, b-a >= 2 && letter
}

func (s *Session) spaceAt(at int) bool {
	return at < len(s.syls) && s.literals[at] && (s.syls[at] == " " || s.syls[at] == "　")
}

// asciiOf：全形的英數與符號（ｓ、２）轉回半形按鍵。
func asciiOf(r rune) rune {
	if 0xFF01 <= r && r <= 0xFF5E {
		return r - 0xFEE0
	}
	return r
}

// mixedLiteralText：insertMixedLiteral 插入 text 時畫面上的樣子（全形時混打照常，英數轉全形）。
func (s *Session) mixedLiteralText(text string) string {
	if s.fullWidth && !s.english {
		return s.fullWidthText(text)
	}
	return text
}

// selectFlip 照 f 換掉組字區 [f.start, f.end)：前段照字面，後段的音節鎖定成選的字（char 空字串時原樣留下注音），原按鍵記在音節上，
// 選了字的記成神經判過的，再按 ↓ 翻得回去。改的是按鍵怎麼切，不只是選字：換完其餘的字照常重排（選了 5萬 之後 快錢 → 塊錢）。
func (s *Session) selectFlip(f flipPick) bool {
	s.cand = nil
	if f.keep {
		return true
	}
	cursor, allow := s.cursor, s.allowRerank
	s.allowRerank = false // 換到一半時不評分，換完整句重解評分一次（同 replaceMixed）
	s.cursor = f.end
	for s.cursor > f.start {
		s.eraseBeforeCursor()
	}
	s.insertMixedPrefix(f.prefix, false, true)
	pos := s.cursor
	if f.char == "" {
		s.insertSyllableLiteral(baseComposer(f.syl), f.syl)
		if s.mixedKeys == nil {
			s.mixedKeys = map[int]string{}
		}
		s.mixedKeys[pos] = f.keys
	} else {
		s.insertMixedReading(f.syl, f.keys)
		s.locks[pos] = decode.Lock{Word: f.char, Len: 1}
		s.markJudged(pos, f.prefix)
	}
	s.allowRerank = allow
	if delta := s.cursor - f.end; !s.AdvanceAfterSelect {
		switch {
		case cursor >= f.end:
			s.cursor = cursor + delta
		case cursor <= f.start:
			s.cursor = cursor
		}
	}
	if !s.Rescore() {
		s.refresh()
	}
	return true
}
