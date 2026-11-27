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

// ↓ 翻邊（P8）：混打判定之後，↓ 清單翻得回另一邊。
//   - 神經判過的字（mixedOrigin.judged）：同一串按鍵的其他切法排在畫面上的字後面（斷 的 2萬、2萬 的 斷）。
//   - 照字面留下的英數（ssl、up）：規則組得出的字與注音（孬…、ㄋㄠ）。

const (
	// flipFollowRunes：切法的字給神經評分時帶的後文字數（萬 後面的 多、塊錢）。
	flipFollowRunes = 4
	// flipMargin：切法比畫面上的樣子差這麼多以內才列。
	flipMargin = 10.0
)

// flipPick 是 ↓ 翻邊的一項：組字區 [start, end) 換成前段 prefix 照字面、後段音節 syl（原按鍵 keys）鎖定成 char；
// char 是空字串時原樣留下注音（ㄋㄠ）。keep 是畫面上的英數本身，選了不動。
type flipPick struct {
	start, end int
	prefix     string
	syl        zhuyin.Syllable
	keys       string
	char       string
	keep       bool
}

// markJudged 把 [from, s.cursor) 裡混打組成的音節記成神經判過的（mixedNeuralWanted）；prefix 是同一串按鍵照字面留在
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

// judgedFlips：pos 是神經判過、還沒改過讀音的字時，同一串按鍵的其他切法：前段沒有或是詞、數字、標點（mixedWordPrefix），
// 後段有字母、照順序組成一個音節（斷 的 2萬、第 的 2億、2萬 的 斷；八 的 1啊 只有數字鍵，要的是 18）。前段被改過時只看這格自己的按鍵。
// keysFirst：原按鍵排在畫面上的字後面（趴 的 q8、r萬 的 j04）；整串是數字鍵開頭、照順序打的注音（斷 和 2萬 的 2j04、邊 的 1u0）
// 只因為數字鍵開頭而問神經，原按鍵不是另一邊，照舊。
func (s *Session) judgedFlips(pos int) (flips []flipPick, keysFirst bool) {
	o, keys := s.mixedOrigins[pos], s.mixedKeys[pos]
	if o.judged == "" || keys == "" || s.literals[pos] || s.syls[pos] != o.judged {
		return nil, false
	}
	start := pos - utf8.RuneCountInString(o.prefix)
	if !s.literalTextIs(start, pos, o.prefix) {
		start, o.prefix = pos, ""
	}
	all := o.prefix + keys
	for at := range len(all) {
		if at == len(o.prefix) || at > 0 && !s.mixedWordPrefix(all[:at]) || !strings.ContainsFunc(all[at:], unicode.IsLetter) {
			continue
		}
		if syls, ok := s.strictSyllables(all[at:]); ok && len(syls) == 1 {
			flips = append(flips, flipPick{start: start, end: pos + 1, prefix: all[:at], syl: syls[0], keys: all[at:]})
		}
	}
	whole, ok := s.strictSyllables(all)
	digitHead := isASCIIDigit(all[0]) && ok && len(whole) == 1
	return s.chooseFlipChars(flips), !digitHead
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

// chooseFlipChars 替每種切法選字：能問神經時帶前文與後文（flipFollowRunes）比同音字，問不到時取最常用的；沒有字的切法拿掉。
// 問得到神經時也跟畫面上的樣子比，不帶後文（後文是照畫面上的字解的：賺快錢 的 快錢 對 5萬 不公平）差 flipMargin 以上的
// 切法不列（翻邊 的 1u0␣ 不列 1煙、多 不列 2窩）。
func (s *Session) chooseFlipChars(flips []flipPick) []flipPick {
	if len(flips) == 0 {
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
	add(shown + follow)
	type option struct {
		char       string
		with, bare int // texts 的索引：帶後文、不帶後文；-1 是送不出去
	}
	limit := min(mixedNeuralNBest, (rerank.MaxTexts-2)/(2*len(flips)))
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model}
	options := make([][]option, len(flips))
	for i, f := range flips {
		for _, c := range dec.Candidates(input.Exact{}.Expand([]zhuyin.Syllable{f.syl}), 0, 1, dec.InitialLeft(), nil) {
			if len(options[i]) == limit {
				break
			}
			t := f.prefix + c.Entry.Surface
			options[i] = append(options[i], option{c.Entry.Surface, add(t + follow), add(t)})
		}
	}
	var scores []float64
	if s.Rerank.Texts != nil && !s.private && !s.paused && !s.english && s.mixedBusy == 0 && shownAt >= 0 && len(texts) > 1 {
		if got, err := s.scoreTexts(texts, start, mixedNeuralDeadline); err == nil {
			scores = got
		}
	}
	out := flips[:0]
	for i, f := range flips {
		if len(options[i]) == 0 {
			continue
		}
		best := options[i][0] // 問不到神經時是最常用的
		if scores != nil {
			for _, o := range options[i][1:] {
				if o.with >= 0 && (best.with < 0 || scores[o.with] > scores[best.with]) {
					best = o
				}
			}
			if best.bare < 0 || scores[best.bare] < scores[shownAt]-flipMargin {
				continue
			}
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
	run := s.spanText(a, b-a)
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
	if key == " " && b < len(s.syls) && s.syls[b] == " " && s.literals[b] {
		end, keys = b+1, run+" "
	}
	items := []string{run}
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

// typedRun：pos（或 pos 是緊接著的空白時它前面）那一串使用者打的英數 [a, b)，至少兩個字、有字母。
func (s *Session) typedRun(pos int) (a, b int, ok bool) {
	typed := func(at int) bool {
		if at < 0 || at >= len(s.syls) || !s.literals[at] {
			return false
		}
		_, ok := s.mixedOrigins[at]
		t := string(s.syls[at])
		return ok && len(t) == 1 && (isASCIIDigit(t[0]) || asciiLetters(t))
	}
	if !typed(pos) {
		if s.syls[pos] != " " || !typed(pos-1) {
			return 0, 0, false
		}
		pos--
	}
	a, b = pos, pos+1
	for typed(a - 1) {
		a--
	}
	for typed(b) {
		b++
	}
	return a, b, b-a >= 2 && strings.ContainsFunc(s.spanText(a, b-a), func(r rune) bool { return r > '9' })
}

func isASCIIDigit(c byte) bool { return '0' <= c && c <= '9' }

// selectFlip 照 f 換掉組字區 [f.start, f.end)：前段照字面，後段的音節鎖定成選的字（char 空字串時原樣留下注音），原按鍵記在音節上，
// 選了字的記成神經判過的，再按 ↓ 翻得回去。改的是按鍵怎麼切，不只是選字：換完其餘的字照常重排（選了 5萬 之後 快錢 → 塊錢）。
func (s *Session) selectFlip(f flipPick) bool {
	s.cand = nil
	if f.keep {
		return true
	}
	cursor := s.cursor
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
	if delta := s.cursor - f.end; !s.AdvanceAfterSelect {
		switch {
		case cursor >= f.end:
			s.cursor = cursor + delta
		case cursor <= f.start:
			s.cursor = cursor
		}
	}
	s.refresh()
	s.Rescore()
	return true
}

// insertFlips 把切法 flips 插在 items 的 head 之後，回傳插好的 items、matched 與各項的換法。
func insertFlips(items []decode.Candidate, matched []string, head int, flips []flipPick) ([]decode.Candidate, []string, map[int]flipPick) {
	if len(flips) == 0 {
		return items, matched, nil
	}
	at := map[int]flipPick{}
	for i, f := range flips {
		items = slices.Insert(items, head+i, rawCandidate(f.prefix+f.char))
		if matched != nil {
			matched = slices.Insert(matched, min(head+i, len(matched)), "")
		}
		at[head+i] = f
	}
	return items, matched, at
}
