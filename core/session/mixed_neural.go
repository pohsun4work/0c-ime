package session

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

const (
	// mixedNeuralDeadline：每鍵 P7 評分（切法比較、回頭比）合計最多等這麼久（每鍵不比官方慢 30 ms 以上）。
	mixedNeuralDeadline = 30 * time.Millisecond
	// mixedRepairPenalty：修正切法（合併冗餘音、丟打錯的鍵、換順序）的神經分數扣分。整串評分偏好字少的文字，
	// 修正把好幾個鍵變成一個字，不扣會吃掉照字面的英文。
	mixedRepairPenalty = 6.0
	// mixedNeuralMargin：神經選的切法要比規則結果高出這麼多才換。
	mixedNeuralMargin = 0.0
	// mixedNeuralNBest：每種切法取的同音字數；切法多時減少，總數不超過 rerank.MaxTexts。
	mixedNeuralNBest = 8
	// mixedNeuralMin：這一鍵剩下的期限不到這麼多就不評分，照規則、留到下一鍵：超過期限的評分會讓下一個請求等它算完。
	mixedNeuralMin = 10 * time.Millisecond
	// mixedNeuralKeys、mixedNeuralSyllables：後段最多幾個按鍵、幾個音節。
	mixedNeuralKeys      = 16
	mixedNeuralSyllables = 4
)

// mixedSplit 是一種切法：前段 raw[:at] 照字面，後段照順序組成 syls；literal 是整串照字面。
// texts 是整串文字（前段加中文 N-best），engine 是中文部分的引擎分數。
type mixedSplit struct {
	at      int
	syls    []zhuyin.Syllable
	literal bool
	repair  bool
	wordCut bool // 切在詞的後面（mixedWordPrefix）或開頭
	texts   []string
	engine  []float64
}

// neuralMixedTone：同一種切法（同切點同讀音）的同音字以 引擎＋權重×神經 選代表，各切法比代表的神經分數
// （修正扣分，repairPenalty），最好的不是規則結果 act、而且高出 mixedNeuralMargin 才換；評分器沒有、失敗或超過期限照規則。
// 結果是中文音節時另回傳之後要回頭再比的紀錄（revisitMixed；位置由呼叫端填）。
func (s *Session) neuralMixedTone(act mixedToneAction, raw, key string, tone rune) (mixedToneAction, *mixedRevisit) {
	if !s.mixedNeuralWanted(act, raw, key) {
		return act, nil
	}
	deadline := mixedNeuralDeadline - s.mixedSpent
	if deadline < mixedNeuralMin {
		return act, nil
	}
	splits, rule := s.mixedSplits(act, raw, key, tone)
	var texts []string
	for _, sp := range splits {
		texts = append(texts, sp.texts...)
	}
	if len(splits[rule].texts) == 0 || len(texts) < 2 {
		return act, nil
	}
	scores, ok := s.scoreTexts(texts, s.cursor, deadline)
	if !ok {
		return act, nil
	}
	heads, scored := make([]int, len(splits)), make([]float64, len(splits))
	best := -1
	for n, sp := range splits {
		got := scores[:len(sp.texts)]
		scores = scores[len(sp.texts):]
		heads[n] = -1
		if len(got) == 0 {
			continue
		}
		head := 0
		for j := range got {
			if sp.engine[j]+s.Rerank.Weight*got[j] > sp.engine[head]+s.Rerank.Weight*got[head] {
				head = j
			}
		}
		heads[n], scored[n] = head, got[head]
		if n != rule {
			scored[n] -= repairPenalty(sp, splits[rule])
		}
		if best < 0 || scored[n] > scored[best] {
			best = n
		}
	}
	if best == rule || scored[best]-scored[rule] <= mixedNeuralMargin {
		best = rule
	} else if sp := splits[best]; sp.literal {
		if act.kind != mixedToneSpace && act.kind != mixedToneSpaceAsTyped {
			act = mixedToneAction{}
		}
	} else {
		act = mixedToneAction{kind: mixedToneSyllable, at: sp.at, syls: sp.syls}
	}
	// 整串組成一個音節的只有英文字典的詞、英數詞（up␣ 的 音、mp3 的 允）回頭再比，純中文（c96 的 還）不比；
	// 贏其他切法 mixedRevisitGap 以上的（cl3 的 好）後文也翻不過來，不比。
	if act.kind != mixedToneSyllable || act.at == 0 && len(act.syls) == 1 && !s.mixedEnglishWord(raw) &&
		english.Alnum(raw+key) != english.AlnumKeysSecond {
		return act, nil
	}
	for n := range splits {
		if n != best && heads[n] >= 0 && scored[best]-scored[n] < mixedRevisitGap {
			return act, &mixedRevisit{raw: raw, key: key, splits: splits, heads: heads, shown: best}
		}
	}
	return act, nil
}

// scoreTexts 以 neuralContext 加組字區 [0, end) 為前文評 texts，花掉的時間記進 mixedSpent；不支援、失敗或分數不是有限值時回 false。
func (s *Session) scoreTexts(texts []string, end int, deadline time.Duration) ([]float64, bool) {
	context := ""
	if s.Rerank.ContextRunes > 0 {
		context = lastRunes(s.neuralContext()+s.spanText(0, end), min(s.Rerank.ContextRunes, rerank.MaxContextRunes))
	}
	req := rerank.TextsRequest{Revision: s.rev + 1, Context: context, Texts: texts}
	if !rerank.SupportedTexts(req, s.Rerank.Chars) {
		return nil, false
	}
	started := time.Now()
	scores, err := s.Rerank.Texts.ScoreTexts(req, deadline)
	s.mixedSpent += time.Since(started)
	if err != nil || len(scores) != len(texts) || slices.ContainsFunc(scores, func(x float64) bool { return !finite(x) }) {
		return nil, false
	}
	return scores, true
}

const (
	// mixedRevisitWindow：神經判成中文的決定，之後組字區多了字時最多再比幾次（約四個音節的後文）。
	mixedRevisitWindow = 4
	// mixedRevisitGap：決定時贏其他切法不到這麼多才回頭比（mp3 的 允 贏 3.4、後文 格式 但 翻過來 5.4）。
	mixedRevisitGap = 10.0
	// mixedRevisitReserve：回頭比的期限先扣掉這麼多，留給換掉之後官方重新評分後面的字。
	mixedRevisitReserve = 10 * time.Millisecond
)

// mixedRevisit 是最近一次神經判成中文的決定：之後組字區多了字時帶著後文再比一次，夠強才改，最多改一次。
type mixedRevisit struct {
	start, end int               // 這次決定插入的組字位置
	syls       []zhuyin.Syllable // 插入後的 s.syls[start:end]；之後被刪改就作廢
	raw, key   string
	splits     []mixedSplit
	heads      []int // 各切法的代表文字（texts 的索引；-1 沒有）
	shown      int   // 選到的切法
	seen       int   // 之後比過幾次
	checked    int   // 上次比（或記下）時組字區的音節數
}

// revisitMixed：按鍵處理完、組字區多了字時，上一次神經判成中文的決定以「畫面上的樣子＋之後的字」對
// 「其他切法的代表＋之後的字」再比一次，其他切法高出 mixedNeuralMargin（修正另扣分）就換掉；
// 換過、比滿 mixedRevisitWindow 次、或被刪改、手選過就固定。期限是這一鍵 P7 還沒用掉的時間；不夠、
// 或還有待定按鍵、打到一半的注音時留到下一鍵。
func (s *Session) revisitMixed() {
	v := s.mixedRevisit
	if v == nil || len(s.syls) <= v.checked {
		return
	}
	if v.seen >= mixedRevisitWindow || !s.revisitValid(v) {
		s.mixedRevisit = nil
		return
	}
	left := mixedNeuralDeadline - s.mixedSpent - mixedRevisitReserve
	if left < mixedNeuralMin || s.Rerank.Texts == nil || !s.allowRerank || s.private || s.paused || s.english ||
		s.mixed.raw != "" || !s.comp.Empty() {
		return
	}
	v.seen, v.checked = v.seen+1, len(s.syls)
	follow := s.spanText(v.end, s.cursor-v.end)
	texts := []string{s.spanText(v.start, v.end-v.start) + follow}
	var owners []int
	for i, sp := range v.splits {
		if i == v.shown || v.heads[i] < 0 {
			continue
		}
		t := sp.texts[v.heads[i]]
		if sp.literal && v.key == " " {
			t += " " // 空白被當一聲用掉的英文換回來時，空白還回來（go up and down）
		}
		if t += follow; s.textEncodable(t) && !slices.Contains(texts, t) {
			texts = append(texts, t)
			owners = append(owners, i)
		}
	}
	if len(texts) < 2 {
		return
	}
	scores, ok := s.scoreTexts(texts, v.start, left)
	if !ok {
		return
	}
	best, bestScore := -1, scores[0]+mixedNeuralMargin
	for j, i := range owners {
		if score := scores[j+1] - repairPenalty(v.splits[i], v.splits[v.shown]); score > bestScore {
			best, bestScore = i, score
		}
	}
	if best >= 0 {
		s.mixedRevisit = nil
		s.replaceMixed(v, v.splits[best])
	}
}

// revisitValid：游標在組字區最後、那次插入的字沒被刪改、沒有手選過。
func (s *Session) revisitValid(v *mixedRevisit) bool {
	if s.cursor != len(s.syls) || v.end > s.cursor || !slices.Equal(s.syls[v.start:v.end], v.syls) {
		return false
	}
	for start, lock := range s.locks {
		if start < v.end && v.start < start+lock.Len {
			return false
		}
	}
	return true
}

// replaceMixed 把組字區 [v.start, v.end) 換成切法 sp，後面的字不動，游標回到最後。
// 換到一半時不評分（insertLiteral、insertMixedTyped 會先 refresh），換完整句重解評分一次。
func (s *Session) replaceMixed(v *mixedRevisit, sp mixedSplit) {
	allow := s.allowRerank
	s.allowRerank = false
	s.cursor = v.end
	for s.cursor > v.start {
		s.eraseBeforeCursor()
	}
	if sp.literal {
		s.insertMixedPrefix(v.raw+v.key, false, false)
	} else {
		s.insertMixedPrefix(v.raw[:sp.at], false, false)
		s.insertMixedSyllables(sp.syls, v.raw[sp.at:]+v.key)
	}
	s.allowRerank = allow
	s.cursor = len(s.syls)
	s.pathSig = ""
	s.refresh()
}

// mixedNeuralWanted：純中文不多問神經。待定按鍵要有英文字母，而且是英文字典的詞（up␣ 照規則是 音）、
// 接聲調鍵是 alnum.tsv 照舊出字的英數詞（mp3 照規則是 允），或規則結果不是整串照順序組成一個音節，
// 或有數字、大寫或符號（整串照順序組成一個音節時注音的數字、標點鍵不算：2k7 的、q96 排）。
// 只有數字標點的（5␣、10␣）、使用者選過的、留注音的照規則；
// 英文字典的詞、alnum.tsv 照字面的英數詞，規則照字面的也照規則（sum3 不是 su與、win10␣ 不是 win班）。
func (s *Session) mixedNeuralWanted(act mixedToneAction, raw, key string) bool {
	if s.Rerank.Texts == nil || !s.allowRerank || s.cand != nil || s.private || s.paused || s.english || s.englishWord ||
		s.mixed.choice != 0 || s.mixed.accepted != "" || act.kind == mixedToneKeep {
		return false
	}
	if act.kind == mixedToneLiteral && (s.mixedEnglishWord(raw) || english.Alnum(raw) == english.AlnumLiteral || s.literalToken(raw+key)) {
		return false
	}
	lower := func(r rune) bool { return r >= 'a' && r <= 'z' }
	if !strings.ContainsFunc(raw, func(r rune) bool { return lower(r) || r >= 'A' && r <= 'Z' }) {
		return false
	}
	if s.mixedEnglishWord(raw) || key != " " && english.Alnum(raw+key) == english.AlnumKeysSecond ||
		act.kind != mixedToneSyllable || act.at != 0 {
		return true
	}
	if strict, ok := s.strictSyllables(raw + key); ok && slices.Equal(strict, act.syls) {
		return false
	}
	return strings.ContainsFunc(raw, func(r rune) bool { return !lower(r) })
}

// mixedSplits 列出要比較的切法與規則結果是第幾個：整串照字面、規則的切法、每個切點的後段照順序組成的音節，
// 和 P6 的修正（整串是英文字典的詞時不修正：dll␣ 不是 尻、zip␣ 不是 分）。
func (s *Session) mixedSplits(act mixedToneAction, raw, key string, tone rune) ([]mixedSplit, int) {
	literal := raw
	if key != " " {
		literal += key
	}
	splits := []mixedSplit{{literal: true, texts: []string{literal}, engine: []float64{0}}}
	add := func(at int, syls []zhuyin.Syllable, repair bool) int {
		for i, sp := range splits {
			if !sp.literal && sp.at == at && slices.Equal(sp.syls, syls) {
				return i
			}
		}
		splits = append(splits, mixedSplit{at: at, syls: syls, repair: repair, wordCut: at == 0 || s.mixedWordPrefix(raw[:at])})
		return len(splits) - 1
	}
	rule := 0
	if act.kind == mixedToneSyllable {
		strict, ok := s.strictSyllables(raw[act.at:] + key)
		rule = add(act.at, act.syls, !ok || !slices.Equal(strict, act.syls))
	}
	first := max(0, len(raw)-mixedNeuralKeys)
	kept := map[zhuyin.Syllable]bool{} // 前段是詞、照順序組得成的讀音
	for at := first; at < len(raw); at++ {
		if syls, ok := s.strictSyllables(raw[at:] + key); ok {
			add(at, syls, false)
			if len(syls) == 1 && s.mixedWordPrefix(raw[:at]) {
				kept[syls[0]] = true
			}
		}
	}
	if !s.mixedEnglishWord(raw) {
		// 前段是詞、照順序就組得成同一個讀音的（asu3 的 a你、1wu/6 的 1停）不修正；前段只是打錯的字母（trup␣ 的 t、
		// rtk␣ 的 r）照樣給丟掉它的修正（金、車）。
		repair := func(at int, done zhuyin.Syllable) {
			if !kept[done] {
				add(at, []zhuyin.Syllable{done}, true)
			}
		}
		if at, done, ok := s.repairFor(raw, key, tone); ok {
			repair(at, done)
		}
		if done, ok := s.englishHeadRepair(raw, key, tone); ok {
			repair(0, done)
		}
		for at := max(1, first); at < len(raw)-1; at++ {
			if done, ok := s.redundantSplit(raw, at, key, tone); ok {
				repair(at, done)
			}
		}
	}
	s.fillSplitTexts(splits, raw)
	return splits, rule
}

// mixedWordPrefix：照字面留下的前段是英文字典的詞、alnum.tsv 的英數詞，或沒有字母（數字、標點）。
func (s *Session) mixedWordPrefix(p string) bool {
	return p != "" && (s.mixedEnglishWord(p) || english.Alnum(p) != english.AlnumUnknown || !strings.ContainsFunc(p, unicode.IsLetter))
}

// repairPenalty：修正切法 sp 對上 base（規則結果或畫面上的樣子，本身不扣）時扣 mixedRepairPenalty；base 也是修正、
// 而且 sp 切在詞的後面時（thewuuu/6 規則的 thew營 對上 the停）只比切在哪裡，不扣。切在不是詞的地方（hellowuuuu/6 的 hellow營）照扣。
func repairPenalty(sp, base mixedSplit) float64 {
	if sp.repair && !(base.repair && sp.wordCut) {
		return mixedRepairPenalty
	}
	return 0
}

// redundantSplit：raw[at:] 照 P6 冗餘音規則（前面照字面時的 strict）合併重複的注音後整段組成一個音節（su;;␣ 的 s央）。
// mixedRepair 只在已知的英文前段後面合併，這裡每個切點都試，交給神經比。
func (s *Session) redundantSplit(raw string, at int, key string, tone rune) (zhuyin.Syllable, bool) {
	r := s.scanSyllableRepair(raw[at:], true)
	if r.cut > 0 || !r.merged || r.fixed {
		return "", false
	}
	return s.repairedReading(raw, at, r, key, tone)
}

// strictSyllables：keys 以聲調鍵（空白是一聲）分段，每段照聲母、介音、韻母的順序組成音節。
func (s *Session) strictSyllables(keys string) ([]zhuyin.Syllable, bool) {
	chunks := toneChunks(keys)
	if len(chunks) > mixedNeuralSyllables {
		return nil, false
	}
	syls := make([]zhuyin.Syllable, 0, len(chunks))
	for _, c := range chunks {
		body := c[:len(c)-1]
		tone, ok := layout.Standard.Symbol(rune(c[len(c)-1]))
		if body == "" || !ok || zhuyin.ClassOf(tone) != zhuyin.Tone || !s.keysInSyllableOrder(body) {
			return nil, false
		}
		done, ok := s.mixedReading(body, tone)
		if !ok {
			return nil, false
		}
		syls = append(syls, done)
	}
	return syls, len(syls) > 0
}

// toneChunks 把按鍵切成以聲調鍵（含空白）結尾的段；最後不是聲調鍵結尾的也是一段。
func toneChunks(keys string) []string {
	var out []string
	start := 0
	for i, k := range keys {
		if sym, ok := layout.Standard.Symbol(k); ok && zhuyin.ClassOf(sym) == zhuyin.Tone {
			out = append(out, keys[start:i+1])
			start = i + 1
		}
	}
	if start < len(keys) {
		out = append(out, keys[start:])
	}
	return out
}

// englishHeadRepair：mixedRepair 為了保護英文字頭放棄的整串修正（trup␣ 的 tr、bkg6 的 bk），只限丟掉打錯的鍵，
// 交給神經跟「英文＋中文」比。
func (s *Session) englishHeadRepair(raw, key string, tone rune) (zhuyin.Syllable, bool) {
	r := s.scanSyllableRepair(raw, false)
	if r.cut > 0 || !r.fixed || r.merged || !s.englishHeadBefore(raw, r.touched) {
		return "", false
	}
	return s.repairedReading(raw, 0, r, key, tone)
}

// fillSplitTexts 填每種切法的整串文字：字表外的字、超過 rerank.MaxTextRunes 的、別的切法已經有的不送。
func (s *Session) fillSplitTexts(splits []mixedSplit, raw string) {
	seen := map[string]bool{}
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model}
	total := 0
	for i := range splits {
		sp := &splits[i]
		if sp.literal {
			if s.textEncodable(sp.texts[0]) {
				seen[sp.texts[0]] = true
			} else {
				sp.texts, sp.engine = nil, nil
			}
		} else {
			rows, _ := dec.NBest(input.Exact{}.Expand(sp.syls), nil, false, mixedNeuralNBest)
			for _, h := range rows {
				if t := raw[:sp.at] + h.Text; !seen[t] && s.textEncodable(t) {
					seen[t] = true
					sp.texts = append(sp.texts, t)
					sp.engine = append(sp.engine, h.LogScore)
				}
			}
		}
		total += len(sp.texts)
	}
	for k := mixedNeuralNBest - 1; total > rerank.MaxTexts && k > 0; k-- {
		total = 0
		for i := range splits {
			sp := &splits[i]
			sp.texts, sp.engine = sp.texts[:min(k, len(sp.texts))], sp.engine[:min(k, len(sp.engine))]
			total += len(sp.texts)
		}
	}
}

func (s *Session) textEncodable(t string) bool {
	if utf8.RuneCountInString(t) > rerank.MaxTextRunes {
		return false
	}
	return s.Rerank.Chars == nil || !strings.ContainsFunc(t, func(r rune) bool { return !s.Rerank.Chars[r] })
}
