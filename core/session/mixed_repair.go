package session

import (
	"strings"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// P6：混打的冗餘音、打錯鍵照華碩寫死規則（record/ime-probe/asus-ime/asus-redundant.md、asus-typo.md），
// 不用神經判斷、也不事後重判：只在聲調鍵、空白定案，整串組不成音節、已知英文前段也切不出來時（mixedBoundary）才試。

// keyRows 是標準排列照手指分的直排，左上對齊：華碩判斷聲母「相鄰」用的位置。
var keyRows = [...]string{"1234567890-", "qwertyuiop", "asdfghjkl;", "zxcvbnm,./"}

// keysAdjacent：a 在以 b 為中心的 3×3 裡（上下差一排以內、左右差一直排以內）。g 的鄰居是 r t y、f h、v b n。
func keysAdjacent(a, b rune) bool {
	ar, ac := keyPosition(a)
	br, bc := keyPosition(b)
	return ar >= 0 && br >= 0 && max(ar, br)-min(ar, br) <= 1 && max(ac, bc)-min(ac, bc) <= 1
}

func keyPosition(k rune) (row, col int) {
	for r, keys := range keyRows {
		if c := strings.IndexRune(keys, k); c >= 0 {
			return r, c
		}
	}
	return -1, -1
}

// syllableRepair 是從結尾往前收的一個音節。
type syllableRepair struct {
	cut         int               // raw[cut:] 是這個音節的按鍵，前面照字面
	keys        [zhuyin.Tone]rune // 留下的聲母、介音、韻母的按鍵（以 Class 為索引）
	touched     int               // 合併或丟掉的鍵最前面的位置；沒有時是 len(raw)
	merged      bool              // 合併了重複的注音（冗餘音）
	fixed       bool              // 丟掉了打錯的鍵
	nonAdjacent bool              // 停在不相鄰的聲母
	digitKept   bool              // 停在緊接著聲母的數字鍵聲母
}

// kept 照聲母、介音、韻母的順序串起留下的按鍵。
func (r syllableRepair) kept() string {
	var b strings.Builder
	for cl := zhuyin.Initial; cl <= zhuyin.Final; cl++ {
		if r.keys[cl] != 0 {
			b.WriteRune(r.keys[cl])
		}
	}
	return b.String()
}

// scanSyllableRepair 從 raw 的結尾往前收一個音節，遇到收不進來的鍵就停（cut）：
//   - 同一個注音重複按當成一次，相不相鄰都算（冗餘音；KeepRedundantKeys 時不合）。
//   - 聲母：從留下的（最後按的）聲母往前，3×3 相鄰的丟掉，可以連著丟；遇到不相鄰的停。
//     數字鍵聲母（ㄅ1 ㄉ2 ㄓ5）緊接著聲母時停，數字留數字（1qu0␣ 是 1篇）；中間隔著介音、韻母的照字母算（5jiy6 是 昨）。
//   - 介音、韻母：聲母後面緊接著按了同一格的另一個鍵時，後按的算（sju3 是 你、g8k4 是 設）；後按的是 ㄦ 時不算。
//     打在聲母前面的不算：英文字尾的母音鍵（hello824 的 o、ok2k7 的 o）不拿來修正。
//
// strict 時前面還有照字面的按鍵，不能拿來修正：只合併打在聲母後面的冗餘音（youtube 接 ej;3 的 e、sql 接 ql3 的 l
// 是英文字尾），其他衝突都停。
func (s *Session) scanSyllableRepair(raw string, strict bool) syllableRepair {
	r := syllableRepair{cut: len(raw), touched: len(raw)}
	for i := len(raw) - 1; i >= 0; i-- {
		k := rune(raw[i])
		sym, ok := layout.Standard.Symbol(k)
		cl := zhuyin.ClassOf(sym)
		if !ok || cl < zhuyin.Initial || cl > zhuyin.Final {
			return r
		}
		switch {
		case r.keys[cl] == 0:
			r.keys[cl] = k
		case r.keys[cl] == k:
			if s.KeepRedundantKeys || strict && r.keys[zhuyin.Initial] != 0 {
				return r
			}
			r.merged, r.touched = true, i
		case cl == zhuyin.Initial:
			switch {
			case strings.ContainsRune("125", k) && s.keyIsInitial(rune(raw[i+1])):
				r.digitKept = true
				return r
			case !keysAdjacent(k, r.keys[cl]):
				r.nonAdjacent = true
				return r
			case strict:
				return r
			}
			r.fixed, r.touched = true, i
		default:
			next, _ := layout.Standard.Symbol(rune(raw[i+1]))
			if strict || r.keys[zhuyin.Initial] != 0 || r.keys[cl] == '-' || zhuyin.ClassOf(next) != cl {
				return r
			}
			r.fixed, r.touched = true, i
		}
		r.cut = i
	}
	return r
}

// mixedRepair：聲調鍵（或空白）前的待定按鍵照冗餘音、打錯鍵的規則整段組成一個音節，回傳切點與音節，切點前照字面。
// 已知英文前段後面只合併冗餘音（thewuuu/6 是 the停、hellowu//6 是 hello停）；整串修正時不改英文字頭裡的鍵
// （go83 的 o、sqlql3 的 sq 不拿來修正，華碩也是英文優先）。冗餘音和打錯鍵不疊在同一個音節：
// 字典沒收的英文接注音（cpujp␣ 是 cpu溫）合併又丟鍵就變成一個字（婚），留給之後的英數切法。
func (s *Session) mixedRepair(raw, key string, tone rune) (int, zhuyin.Syllable, bool) {
	for at := len(raw) - 2; at >= 0; at-- {
		if at > 0 && !s.mixedLiteralPrefix(raw[:at], raw[at:], key) {
			continue
		}
		r := s.scanSyllableRepair(raw[at:], at > 0)
		if r.cut > 0 || r.merged == r.fixed || at == 0 && s.englishHeadBefore(raw, r.touched) {
			continue
		}
		if done, ok := s.repairedReading(raw, at, r, key, tone); ok {
			return at, done, true
		}
	}
	return 0, "", false
}

// repairFor：待定按鍵接 key 時經過冗餘音、打錯鍵修正組成的音節（mixedRepair，其次 mixedRepairSplit），切點前照字面。
func (s *Session) repairFor(raw, key string, tone rune) (int, zhuyin.Syllable, bool) {
	if at, done, ok := s.mixedRepair(raw, key, tone); ok {
		return at, done, true
	}
	return s.mixedRepairSplit(raw, key, tone)
}

// repairedText：待定按鍵經過修正組成的注音在組字區的樣子（png 是 ㄕㄣ、5wu0 是 5ㄊㄧㄢ）。
func (s *Session) repairedText(raw string) (string, bool) {
	at, done, ok := s.repairFor(raw, " ", zhuyin.Tone1)
	return raw[:at] + string(done.Base()), ok
}

// repairMemo 記下 mixedRepairShown 上次的輸入與結果：一次按鍵的組字區、小視窗、定案會問好幾次，修正要比較切法的常用程度。
type repairMemo struct {
	raw                       string
	rev, choice, cursor, syls int
	at                        int
	done                      zhuyin.Syllable
	ok                        bool
}

// mixedRepairShown：組字區顯示修正讀音時，回傳切點與修正後的音節，切點前照字面。沒選過的（choice 0）照接空白的結果
// （spaceShown：gjn 是 ㄙㄨ、5wu0 是 5ㄊㄧㄢ、aq8 是 aㄆㄚ、20 是 ㄉㄢ）；Tab 換成修正讀音的（choice 3，字典收的 png 是 ㄕㄣ）照修正。
func (s *Session) mixedRepairShown() (int, zhuyin.Syllable, bool) {
	p := s.mixed
	if p.raw == "" || p.accepted != "" || s.english || s.englishWord || p.choice == 1 || p.choice == 2 {
		return 0, "", false
	}
	m := &s.repairMemo
	if m.raw == p.raw && m.rev == s.rev && m.choice == p.choice && m.cursor == s.cursor && m.syls == len(s.syls) {
		return m.at, m.done, m.ok
	}
	var at int
	var done zhuyin.Syllable
	var ok bool
	if p.choice == 3 {
		at, done, ok = s.repairFor(p.raw, " ", zhuyin.Tone1)
	} else {
		at, done, ok = s.spaceShown()
	}
	*m = repairMemo{raw: p.raw, rev: s.rev, choice: p.choice, cursor: s.cursor, syls: len(s.syls), at: at, done: done, ok: ok}
	return at, done, ok
}

// keysReading：顯示的讀音來自沒有字母的數字、標點鍵（20 的 ㄉㄢ、. 的 ㄡ），只是空白的預覽：
// 空白以外的定案（Enter、失焦、切英文、點注音鍵）照按鍵（#162、#187）。
func (s *Session) keysReading() bool {
	at, _, ok := s.mixedRepairShown()
	return ok && s.mixed.choice == 0 && !strings.ContainsFunc(s.mixed.raw[at:], unicode.IsLetter)
}

// mixedShowsZhuyin：待定按鍵在組字區顯示成還沒打完的注音（照順序組的 ㄋㄧ，或修正讀音的 ㄙㄨ）；數字、標點鍵的讀音不算。
func (s *Session) mixedShowsZhuyin() bool {
	if s.keysReading() {
		return false
	}
	if _, phonetic := s.mixedPending(s.mixedText()); phonetic {
		return true
	}
	_, _, repaired := s.mixedRepairShown()
	return repaired
}

// eraseRepaired：顯示修正讀音時退格跟一般注音一樣刪畫面最後一個注音（gjn 的 ㄙㄨ 剩 ㄙ、wu// 的 ㄊㄧㄥ 剩 ㄊㄧ，#309），
// 留下的注音換回它們的按鍵。沒有顯示修正讀音時回傳 false。
func (s *Session) eraseRepaired() bool {
	at, done, ok := s.mixedRepairShown()
	if !ok {
		return false
	}
	syms := []rune(string(done.Base()))
	keys := []rune(s.mixed.raw[:at])
	for _, sym := range syms[:len(syms)-1] {
		k, _ := layout.Standard.Key(sym)
		keys = append(keys, k)
	}
	s.mixed.raw, s.mixed.choice = string(keys), 0
	s.mixed.assistedPrefix = min(s.mixed.assistedPrefix, at)
	return true
}

// englishHeadBefore：raw 有包含 touched 位置的英文字頭（go83 的 go）。
func (s *Session) englishHeadBefore(raw string, touched int) bool {
	for j := max(touched+1, 2); j < len(raw); j++ {
		if s.mixedEnglishWord(raw[:j]) {
			return true
		}
	}
	return false
}

// mixedRepairSplit：從結尾往前只合併冗餘音（xyzwu//6 是 xyz停），在數字鍵聲母（1qu0␣ 是 1篇）或不相鄰的聲母切開，
// 切點前照字面。不相鄰的聲母接聲調鍵時，前段只有一個字母（xwu/6 是 x停）或中間夾著不照順序的韻母（ckg6 是 c什）才切：
// 照順序打的後段（ipadji3）交給原本比較常用程度的切法，接空白的（Dyson␣）是英文字。
func (s *Session) mixedRepairSplit(raw, key string, tone rune) (int, zhuyin.Syllable, bool) {
	r := s.scanSyllableRepair(raw, true)
	if r.cut == 0 || !r.merged && !r.digitKept &&
		!(r.nonAdjacent && key != " " && (r.cut == 1 || !s.keysInSyllableOrder(raw[r.cut:]))) {
		return 0, "", false
	}
	done, ok := s.repairedReading(raw, r.cut, r, key, tone)
	return r.cut, done, ok
}

// repairedReading：raw[at:] 修正後的音節要有字母（我＋1984 是數字）、組得成字。接空白至少要兩個注音（mmm␣、lll␣ 不變字）。
// 丟掉打錯的鍵、在聲母切開的不能是罕用字（mixedRareFloor）；只合併冗餘音的照華碩組字（ssl␣ 是 孬）。
func (s *Session) repairedReading(raw string, at int, r syllableRepair, key string, tone rune) (zhuyin.Syllable, bool) {
	kept := r.kept()
	if key == " " && len(kept) < 2 || !strings.ContainsFunc(raw[at:], unicode.IsLetter) || r.merged && !redundantDigitsOK(raw, at, r) {
		return "", false
	}
	done, ok := s.mixedReading(kept, tone)
	if !ok || tone == '˙' && done.ToneOf() != 5 || (r.fixed || at > 0) && s.mixedSyllableScore(done) < mixedRareFloor {
		return "", false
	}
	return done, true
}

// redundantDigitsOK：華碩冗餘音的數字鍵門檻（asus-typo.md 第 7 節）。音節的數字鍵加上聲調鍵（空白也算），沒有介音時 3 個以內、
// 有介音時 4 個以內（ru8884 是 架、a8883 照字面）；音節沒有聲母時，緊接在前面的數字鍵聲母也算（2u0004 的 2）。
func redundantDigitsOK(raw string, at int, r syllableRepair) bool {
	n := 1
	for _, k := range raw[at:] {
		if unicode.IsDigit(k) {
			n++
		}
	}
	if r.keys[zhuyin.Initial] == 0 && at > 0 && strings.ContainsRune("125", rune(raw[at-1])) {
		n++
	}
	limit := 3
	if r.keys[zhuyin.Medial] != 0 {
		limit = 4
	}
	return n <= limit
}
