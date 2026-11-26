package session

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// P7：聲調鍵、空白收尾待定按鍵時，規則結果（decideMixedTone）跟其他切法一起送整串文字評分（rerank.ModeTexts），
// 選到不同的切法才換；評分器沒有、失敗或超過期限照規則。

const (
	// mixedNeuralDeadline：每次決定最多等評分這麼久（每鍵不比官方慢 30 ms 以上）。
	mixedNeuralDeadline = 30 * time.Millisecond
	// mixedRepairPenalty：修正切法（合併冗餘音、丟打錯的鍵、換順序）的神經分數扣分。整串評分偏好字少的文字，
	// 修正把好幾個鍵變成一個字，不扣會吃掉照字面的英文。
	mixedRepairPenalty = 6.0
	// mixedNeuralMargin：神經選的切法要比規則結果高出這麼多才換。
	mixedNeuralMargin = 0.0
	// mixedNeuralNBest：每種切法取的同音字數；切法多時減少，總數不超過 rerank.MaxTexts。
	mixedNeuralNBest = 8
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
	texts   []string
	engine  []float64
}

// neuralMixedTone：同一種切法（同切點同讀音）的同音字以 引擎＋權重×神經 選代表，各切法比代表的神經分數
// （修正扣 mixedRepairPenalty），最好的不是規則結果 act、而且高出 mixedNeuralMargin 才換。
func (s *Session) neuralMixedTone(act mixedToneAction, raw, key string, tone rune) mixedToneAction {
	if !s.mixedNeuralWanted(act, raw, key) {
		return act
	}
	splits, rule := s.mixedSplits(act, raw, key, tone)
	var texts []string
	for _, sp := range splits {
		texts = append(texts, sp.texts...)
	}
	if len(splits[rule].texts) == 0 || len(texts) < 2 {
		return act
	}
	context := ""
	if s.Rerank.ContextRunes > 0 {
		context = lastRunes(s.neuralContext()+s.spanText(0, s.cursor), min(s.Rerank.ContextRunes, rerank.MaxContextRunes))
	}
	req := rerank.TextsRequest{Revision: s.rev + 1, Context: context, Texts: texts}
	if !rerank.SupportedTexts(req, s.Rerank.Chars) {
		return act
	}
	scores, err := s.Rerank.Texts.ScoreTexts(req, mixedNeuralDeadline)
	if err != nil || len(scores) != len(texts) {
		return act
	}
	best, bestScore, ruleScore := -1, 0.0, 0.0
	for n, sp := range splits {
		got := scores[:len(sp.texts)]
		scores = scores[len(sp.texts):]
		if len(got) == 0 {
			continue
		}
		head := 0
		for j := range got {
			if sp.engine[j]+s.Rerank.Weight*got[j] > sp.engine[head]+s.Rerank.Weight*got[head] {
				head = j
			}
		}
		score := got[head]
		if sp.repair {
			score -= mixedRepairPenalty
		}
		if !finite(score) {
			return act
		}
		if n == rule {
			ruleScore = score
		}
		if best < 0 || score > bestScore {
			best, bestScore = n, score
		}
	}
	if best == rule || bestScore-ruleScore <= mixedNeuralMargin {
		return act
	}
	if sp := splits[best]; !sp.literal {
		return mixedToneAction{kind: mixedToneSyllable, at: sp.at, syls: sp.syls}
	}
	if act.kind == mixedToneSpace || act.kind == mixedToneSpaceAsTyped {
		return act
	}
	return mixedToneAction{}
}

// mixedNeuralWanted：純中文不多問神經。待定按鍵要有英文字母，而且有數字、大寫或符號（包括注音的數字、標點鍵），
// 或整串是英文字典的詞（go␣、zip␣），或接聲調鍵是 alnum.tsv 照舊出字的英數詞（mp3 照規則是 允），
// 或規則結果不是整串組成一個音節。只有數字標點的（5␣、10␣）、使用者選過的、留注音的照規則。
func (s *Session) mixedNeuralWanted(act mixedToneAction, raw, key string) bool {
	if s.Rerank.Texts == nil || !s.allowRerank || s.cand != nil || s.private || s.paused || s.english || s.englishWord ||
		s.mixed.choice != 0 || s.mixed.accepted != "" || act.kind == mixedToneKeep {
		return false
	}
	lower := func(r rune) bool { return r >= 'a' && r <= 'z' }
	if !strings.ContainsFunc(raw, func(r rune) bool { return lower(r) || r >= 'A' && r <= 'Z' }) {
		return false
	}
	return strings.ContainsFunc(raw, func(r rune) bool { return !lower(r) }) || s.mixedEnglishWord(raw) ||
		key != " " && english.Alnum(raw+key) == english.AlnumKeysSecond || act.kind != mixedToneSyllable || act.at != 0
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
		splits = append(splits, mixedSplit{at: at, syls: syls, repair: repair})
		return len(splits) - 1
	}
	rule := 0
	if act.kind == mixedToneSyllable {
		rule = add(act.at, act.syls, false)
	}
	first := max(0, len(raw)-mixedNeuralKeys)
	for at := first; at < len(raw); at++ {
		if syls, ok := s.strictSyllables(raw[at:] + key); ok {
			add(at, syls, false)
		}
	}
	if !s.mixedEnglishWord(raw) {
		for at := first; at < len(raw)-1; at++ {
			if done, ok := s.repairedSplit(raw, at, key, tone); ok {
				add(at, []zhuyin.Syllable{done}, true)
			}
		}
	}
	s.fillSplitTexts(splits, raw)
	return splits, rule
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

// repairedSplit：raw[at:] 照 P6 規則整段組成一個音節（合併冗餘音、丟打錯的鍵擇一，或只是聲母打在後面）。
// 跟 mixedRepair 不同，英文字頭裡的鍵也拿來修正（go83 的 o），由神經跟照字面比。
func (s *Session) repairedSplit(raw string, at int, key string, tone rune) (zhuyin.Syllable, bool) {
	body := raw[at:]
	r := s.scanSyllableRepair(body, false)
	reordered := r.keys[zhuyin.Initial] != 0 && rune(body[0]) != r.keys[zhuyin.Initial]
	if r.cut > 0 || r.merged && r.fixed || !r.merged && !r.fixed && !reordered {
		return "", false
	}
	return s.repairedReading(raw, at, r, key, tone)
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
