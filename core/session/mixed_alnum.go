package session

import (
	"math"
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// 神經模型判斷英數接中文的門檻（log10），取自 2026-11-04 用本機神經模型實測的 2430 個案例（英數接中文換對 47% → 89%）。
const (
	// alnumMargin：換成的樣子逐字加總至少要比現在的樣子高這麼多：3nm製程 +10.0、4k螢幕 +5.7、i5 顯示卡 +3.1 換；
	// 接單字的 2k的 +2.8、4k嗎 +2.2 留給 ↓；再低就換錯 arm6 → a局（+2.6、這個arm6 +2.46）、我買了pr支援 → pr5 元（+2.2）、
	// nina87 → nin嗎（+1.2）。這個a1影片 → a餅片（+4.5）是神經模型本身判錯，門檻擋不住。
	alnumMargin = 3.0
	// alnumRecut：已經照這條換過的一串，後面打的字讓別的切法更合理時差這麼多就改：2f名 接著打 幕 是 2fa螢幕（+0.51），
	// arm6 先換成 a局、接著打 4 換回 arm64（+0.86）。1 時 2fa螢幕 換不過去。
	alnumRecut = 0.5
	// alnumTail：一串的中文超過這麼多個音節就不再重看，切點已經由前面的字定了。
	alnumTail = 4
)

// alnumReading 是評照字面的字時給神經模型的讀音：文字評分不看讀音，只要是合法的音節。
const alnumReading = zhuyin.Syllable("ㄧ")

// alnumCut 是英數接中文換過的一段，現在還是 form 的樣子時 ↓ 開整段（changedSpan、alnumCutSpan）。
type alnumCut struct {
	memorySpan
	form []personal.MixedToken
}

// alnumStretch 是要重看的一串：組字區 [start, end)（end 是游標）連同官方照字面留著的待定按鍵。
type alnumStretch struct {
	memorySpan
	keys    string                // 整串的原按鍵
	current []personal.MixedToken // 現在的樣子
	pending bool
}

// rejudgeAlnum 重看英數後面直接接中文（mixed_context 的 alnum，一律開著），換了回報 true。官方在聲調鍵當下就決定英數到哪裡
// 為止：3nm 接「製程」（54t/6）的 54 被當成帳號，4k 接「螢幕」前段只有一個字母切不開（4ku/6木），也會切錯位置（5爽路）、
// 把英數後面的空白當成一聲（i之顯示卡）。這裡把游標前這一串照原按鍵重新列切點，神經模型評分明顯比現在的樣子合理才換。
// 沒有神經模型（私人、暫停、還沒載好）時照官方；記憶說這串照現在的樣子留著時不換（官方照字面留著的待定按鍵
// rejudgeMemoryPending 不記成留著）。
func (s *Session) rejudgeAlnum(index map[string][]personal.MixedRecord) bool {
	if !s.contextRule(inputstyle.ContextAlnum) || !s.neuralText() {
		return false
	}
	st, ok := s.alnumStretch()
	if !ok {
		return false
	}
	cut := s.memory.cutAt(st.memorySpan)
	forms := s.alnumForms(st, cut)
	if len(forms) == 0 && !cut && !hasKind(st.current, personal.MixedChinese) {
		// 接著打的英數自己切不出中文時連著前面換過的一串看，才換得回整串照字面（a局＋4 是 arm64）
		if st, ok = s.alnumJoinCut(st); !ok {
			return false
		}
		cut, forms = true, s.alnumForms(st, true)
	}
	if len(forms) == 0 {
		return false
	}
	s.refresh()
	if a, ok := s.memoryDecide(index[st.keys], st.memorySpan, st.current, nil); ok && a.hold() {
		return false
	}
	best, ok := s.alnumBest(st, forms, cut)
	if !ok {
		return false
	}
	insert := func() {
		if st.pending {
			s.clearMixed()
		}
		s.insertMemoryForm(best)
	}
	if !s.replaceOver(st.start, st.end, insert) {
		return false
	}
	if hasKind(best, personal.MixedChinese) { // 整串換回照字面是官方的樣子
		s.memory.cuts = append(s.memory.cuts, alnumCut{memorySpan{st.start, s.cursor}, best})
	}
	return true
}

// alnumBest 是 forms 裡神經模型評分最高、比現在的樣子高出門檻（cut：已經照這條換過，alnumRecut）的切法。
// 兩邊照同樣的方法評，換上之後再評一次還是同樣的分數，不會換來換去。
func (s *Session) alnumBest(st alnumStretch, forms [][]personal.MixedToken, cut bool) ([]personal.MixedToken, bool) {
	before := s.neuralContext() + strings.Join(s.shownChars(st.start), "")
	current, ok := s.alnumFormScore(before, st.current, math.Inf(-1))
	if !ok {
		return nil, false
	}
	margin := alnumMargin
	if cut {
		margin = alnumRecut
	}
	best, need := []personal.MixedToken(nil), current+margin
	for _, f := range forms {
		if v, ok := s.alnumFormScore(before, f, need); ok && (v > need || best == nil && v == need) {
			best, need = f, v
		}
	}
	return best, best != nil
}

// alnumCutSpan 是 c 連同後面接著打的中文（同一串最多 alnumTail 個音節）：4k螢 接著打出 4k螢幕，↓ 開整段。
func (s *Session) alnumCutSpan(c alnumCut) memorySpan {
	q := c.memorySpan
	for n := len(c.form) - 1; q.end < len(s.syls) && n < alnumTail && s.alnumTokenAt(q.end, personal.MixedChinese); n++ {
		q.end++ // c.form 是一段英數接幾個中文音節
	}
	return q
}

// alnumStretch 找游標前的一串：英數開頭，接著官方切出的中文（最多 alnumTail 個音節）或照字面留著、以聲調鍵結尾的待定按鍵。
// 注音還沒打完、碰到空白或標點、記憶換過或明確選過的字都是邊界；游標後面緊接著同一串的字（游標移到中間）時不看。
func (s *Session) alnumStretch() (alnumStretch, bool) {
	pending := s.mixed.raw != ""
	end := s.cursor
	if pending && !s.pendingTone() || end < len(s.syls) && (s.alnumTokenAt(end, personal.MixedLiteral) || s.alnumTokenAt(end, personal.MixedChinese)) {
		return alnumStretch{}, false
	}
	chinese := end
	if !pending {
		for chinese > 0 && s.alnumTokenAt(chinese-1, personal.MixedChinese) {
			if chinese--; end-chinese > alnumTail {
				return alnumStretch{}, false
			}
		}
	}
	start := chinese
	for start > 0 && s.alnumTokenAt(start-1, personal.MixedLiteral) {
		start--
	}
	for start < chinese && !alnumText(string(s.syls[start])) {
		start++
	}
	return s.alnumStretchFrom(start, end)
}

// alnumJoinCut 是 st 連同緊接在前面、英數接中文換過的一段。
func (s *Session) alnumJoinCut(st alnumStretch) (alnumStretch, bool) {
	i := slices.IndexFunc(s.memory.cuts, func(c alnumCut) bool { return c.end == st.start })
	if i < 0 {
		return alnumStretch{}, false
	}
	return s.alnumStretchFrom(s.memory.cuts[i].start, st.end)
}

// alnumStretchFrom 是組字區 [start, end) 連同待定按鍵的一串，要以英數開頭。
func (s *Session) alnumStretchFrom(start, end int) (alnumStretch, bool) {
	tokens, ok := s.memoryTokens(start, end)
	if !ok {
		return alnumStretch{}, false
	}
	pending := s.mixed.raw != ""
	if pending {
		tokens = append(tokens, memoryToken{memorySpan{end, end}, personal.MixedToken{Keys: s.mixed.raw, Kind: personal.MixedLiteral}})
	}
	current := tokensOf(mixedForm(tokens))
	if len(current) == 0 || current[0].Kind != personal.MixedLiteral || !alnumText(current[0].Keys[:1]) {
		return alnumStretch{}, false
	}
	return alnumStretch{memorySpan{start, end}, joinKeys(current), current, pending}, true
}

// alnumTokenAt：pos 是一串裡 kind 的一格：查得回按鍵、不是空白，沒被鎖定、明確選過、照記憶或 ↓ 換過、換回英數（忘掉記憶的段落照看）。
func (s *Session) alnumTokenAt(pos int, kind string) bool {
	t, ok := s.memoryTokenAt(pos)
	if !ok || t.Kind != kind || s.memory.changed(t.memorySpan) || s.memory.wordAt(t.memorySpan) || s.pinnedAt(pos) {
		return false
	}
	return kind != personal.MixedLiteral || t.Keys != " " && t.Keys[0] <= '~'
}

func alnumText(t string) bool { return englishLetter(t) || isDigitText(t) }

// alnumForms 列出 st 的切法：前段照字面（至少一個字元），後段照順序拆成音節，現在的樣子不算。
//   - 後段的第一個音節用聲調鍵結尾：緊接在英數後面的空白可能是分隔（4k␣ 不是 4阿），交給官方。
//   - 後段只有一個音節、全是數字鍵的是帳號或編號（kevin87、lisa03），照官方。
//   - 前段可以收回官方當成一聲的空白（i5␣顯示卡 官方是 i之顯示卡）。
//
// 已經照這條換過的一串（cut）也可以整串換回照字面：arm6 先換成 a局，接著打 4 是 arm64。
func (s *Session) alnumForms(st alnumStretch, cut bool) [][]personal.MixedToken {
	var out [][]personal.MixedToken
	if lit := literalForm(st.keys); cut && !slices.Equal(lit, st.current) {
		out = append(out, lit)
	}
	limit := len(st.current[0].Keys) // 切點最遠到官方第一個中文音節的結尾
	if len(st.current) > 1 {
		limit += len(st.current[1].Keys)
	}
	for at := 1; at <= min(limit, len(st.keys)-1); at++ {
		groups := s.groupsOf(st.keys[at:])
		if !s.alnumChinese(groups) {
			continue
		}
		form := literalForm(st.keys[:at])
		for _, g := range groups {
			form = append(form, personal.MixedToken{Keys: g.keys, Kind: personal.MixedChinese})
		}
		if !slices.Equal(form, st.current) && s.memoryInsertable(form) {
			out = append(out, form)
		}
	}
	return out
}

// alnumChinese：groups 可以是英數後面接的中文：每一段都打了聲調（音節合不合法由 memoryInsertable 看），第一段是明確的聲調鍵，
// 不是只有一個全是數字鍵的音節。
func (s *Session) alnumChinese(groups []keyGroup) bool {
	if len(groups) == 0 || !s.explicitTone(groups[0].keys) {
		return false
	}
	if len(groups) == 1 && strings.Trim(groups[0].keys, "0123456789") == "" {
		return false
	}
	return !slices.ContainsFunc(groups, func(g keyGroup) bool { return g.tone == 0 })
}

// alnumChar 是要評分的一個字：choices 給神經模型挑分數最高的（照字面的字、解碼出的字只有一個），syl 是它的讀音。
type alnumChar struct {
	choices []rune
	syl     zhuyin.Syllable
}

// alnumFormScore 是 form 的字（alnumFormChars）接在 before（已確認的前文＋組字區這一串前面的字）後面，神經模型逐字加總的
// log10 機率。每個切法的第一個字都是同一個英數，只當前文、不評分（句首沒有前文時神經模型評不了第一個字）。
// 加到一半就低於 floor 時直接回傳：後面的字只會再扣分。
func (s *Session) alnumFormScore(before string, form []personal.MixedToken, floor float64) (float64, bool) {
	chars, ok := s.alnumFormChars(form)
	if !ok || len(chars) == 0 {
		return 0, false
	}
	before += string(chars[0].choices[0])
	var sum float64
	for _, c := range chars[1:] {
		best, top, ok := s.neuralBest(s.neuralWindow(before), c.choices, c.syl)
		if !ok {
			return 0, false
		}
		if sum += top; sum < floor {
			return sum, true
		}
		before += string(best)
	}
	return sum, true
}

// alnumFormChars 是 form 的字：英數照按鍵；中文在照字面的字後面重新解碼（同 scoreCells），只有一個音節時另給幾個
// 常用單字讓神經模型挑（commonChars）。
func (s *Session) alnumFormChars(form []personal.MixedToken) ([]alnumChar, bool) {
	var out []alnumChar
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model, Beam: s.Opt.Beam, Continue: s.Opt.Continue}
	for i := 0; i < len(form); {
		if form[i].Kind != personal.MixedChinese {
			for _, r := range s.formText(form[i]) {
				out = append(out, alnumChar{[]rune{r}, alnumReading})
			}
			i++
			continue
		}
		var syls []zhuyin.Syllable
		for ; i < len(form) && form[i].Kind == personal.MixedChinese; i++ {
			syl, ok := s.chineseSyllable(form[i].Keys)
			if !ok {
				return nil, false
			}
			syls = append(syls, syl)
		}
		words, ok := bestWords(dec, syls)
		if !ok {
			return nil, false
		}
		for _, w := range words {
			for k, r := range []rune(w.word) {
				out = append(out, alnumChar{[]rune{r}, syls[w.at+k]})
			}
		}
		if len(syls) == 1 {
			c := &out[len(out)-1]
			c.choices = s.commonChars(c.choices, syls[0], commonCharLimit)
		}
	}
	return out, true
}
