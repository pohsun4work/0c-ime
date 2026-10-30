package session

import (
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// mixedMemory：整合風格（操作規則 mixed_memory），記住 ↓ 換過的混打解釋（personal.LearnMixedMemory），
// 下次同樣的按鍵照上下文決定要不要照記憶換；要有 mixed_resplit 的按鍵紀錄。
func (s *Session) mixedMemory() bool { return s.InputStyle.MixedMemory != "" && s.resplit() }

// 只有按鍵對上（前一個字或後一個音不同）的記憶看中文那一側的合理度：每個字平均的 log10 機率。記憶把中文換掉時，
// 中文低於門檻才換；記憶換成中文時，中文到門檻才換。每多確認一次、附近有英文都往「不是中文」那邊調。
const (
	memoryThreshold    = -4.0
	memoryConfirmStep  = 0.5
	memoryEnglishBonus = 1.0
	memoryNeighbors    = 3 // 算合理度時連同後面幾個中文音節一起解碼
)

type memorySpan struct{ start, end int }

// shifted 同 shiftLocks：在段落前面插入或刪除時平移，編輯到段落裡面時回傳 false。
func (p memorySpan) shifted(at, delta int) (memorySpan, bool) {
	switch {
	case at >= p.end:
		return p, true
	case at < p.start || delta > 0 && at == p.start:
		return memorySpan{p.start + delta, p.end + delta}, true
	}
	return p, false
}

func (p memorySpan) overlaps(q memorySpan) bool { return p.start < q.end && q.start < p.end }

// memoryChoice 是這次組字裡用 ↓ 換了解釋的一段，送出時還是這個樣子才學。contest 是選之前那段照前後都對上的記憶換過（或留著）。
type memoryChoice struct {
	memorySpan
	form    []personal.MixedToken
	contest *personal.MixedRef
}

// memoryApplied 是照記憶換成 Form 的一段，official 是換之前的樣子，條件不再成立時換回。兩者一樣時是照記憶留著
// （hold），只擋住 mixed_context 的規則改它。
type memoryApplied struct {
	memorySpan
	personal.MixedApplied
	official []personal.MixedToken
}

func (a memoryApplied) hold() bool { return slices.Equal(a.Form, a.official) }

type memoryScore struct {
	value float64
	ok    bool
}

type memoryState struct {
	choices []memoryChoice
	applied []memoryApplied
	blocked []memorySpan           // Shift＋Delete 忘掉的段落：這次組字不再照記憶換
	chars   map[string]float64     // 神經模型評過的字：前文＋字 → log10 機率
	scores  map[string]memoryScore // 算過的合理度（chineseScore）
}

// shift 平移段落。每次換成新的切片，saveSyllables 存的舊狀態還原得回來。
func (m *memoryState) shift(at, delta int) {
	m.choices = shiftSpans(m.choices, func(c *memoryChoice) *memorySpan { return &c.memorySpan }, at, delta)
	m.applied = shiftSpans(m.applied, func(a *memoryApplied) *memorySpan { return &a.memorySpan }, at, delta)
	m.blocked = shiftSpans(m.blocked, func(p *memorySpan) *memorySpan { return p }, at, delta)
}

func shiftSpans[T any](items []T, span func(*T) *memorySpan, at, delta int) []T {
	var kept []T
	for _, x := range items {
		p := span(&x)
		if moved, ok := p.shifted(at, delta); ok {
			*p = moved
			kept = append(kept, x)
		}
	}
	return kept
}

// guards：pos 在照記憶換過或留著的段落裡，mixed_context 的規則不改。
func (m *memoryState) guards(pos int) bool { return m.appliedAt(memorySpan{pos, pos + 1}, true) >= 0 }

// appliedAt 是跟 p 重疊的照記憶換過的段落；hold 為 false 時不算照記憶留著的。沒有時回傳 -1。
func (m *memoryState) appliedAt(p memorySpan, hold bool) int {
	return slices.IndexFunc(m.applied, func(a memoryApplied) bool { return a.overlaps(p) && (hold || !a.hold()) })
}

// taken：p 碰到照記憶換過的、這次 ↓ 換過的或忘掉的段落。
func (m *memoryState) taken(p memorySpan) bool {
	return m.appliedAt(p, true) >= 0 || slices.ContainsFunc(m.choices, func(c memoryChoice) bool { return c.overlaps(p) }) ||
		slices.ContainsFunc(m.blocked, p.overlaps)
}

// memoryToken 是組字區裡查得回按鍵的一段：中文、原樣注音各一個音節，照字面的英數一個字（mixedForm 再併成一串）。
type memoryToken struct {
	memorySpan
	personal.MixedToken
}

// memoryTokenAt 回傳 pos 起的一段。查不回按鍵、數字鍵盤的數字、補字接受的英文與換了寫法的符號（，）不算。
func (s *Session) memoryTokenAt(pos int) (memoryToken, bool) {
	t := memoryToken{memorySpan: memorySpan{pos, pos + 1}}
	u, ok := s.keyUnitAt(pos)
	isZhuyin := func(p int) bool { return zhuyin.ClassOf([]rune(string(s.syls[p]))[0]) != zhuyin.None }
	switch _, pending := s.pendingLiteralAt(pos); {
	case !s.literals[pos]:
		t.Keys, t.Kind = u.keys, personal.MixedChinese
		if !ok {
			t.Keys = s.layout.Keys(s.syls[pos]) // 沒記按鍵的中文用讀音的按鍵（同 spanUnits）
		}
		return t, t.Keys != ""
	case !ok || u.start != pos || u.keypad:
		return t, false
	case pending:
		p, toned := s.syllableLiteralAt(pos)
		t.end, t.Keys, t.Kind = u.end, u.keys, personal.MixedZhuyin
		return t, toned && !p.untoned
	case isZhuyin(pos):
		// ↓ 換成的注音每個符號記一個按鍵，最後一個連同聲調或空白（insertZhuyinLiteral）。
		for t.end = pos; t.end < len(s.syls) && isZhuyin(t.end) && s.literalKeys[t.end] != "" && !strings.HasSuffix(t.Keys, " ") && !s.explicitTone(t.Keys); t.end++ {
			t.Keys += s.literalKeys[t.end]
		}
		t.Kind = personal.MixedZhuyin
		return t, true
	case u.keys != string(s.syls[pos]):
		return t, false
	}
	t.Keys, t.Kind = u.keys, personal.MixedLiteral
	return t, true
}

// memoryTokens 把 [start, end) 切成 memoryToken；有不算的位置或切到一段的一半時回傳 false。
func (s *Session) memoryTokens(start, end int) ([]memoryToken, bool) {
	var out []memoryToken
	for pos := start; pos < end; {
		t, ok := s.memoryTokenAt(pos)
		if !ok || t.end > end {
			return nil, false
		}
		out = append(out, t)
		pos = t.end
	}
	return out, true
}

// memoryFormIs：p 現在的樣子是 form。
func (s *Session) memoryFormIs(p memorySpan, form []personal.MixedToken) bool {
	tokens, ok := s.memoryTokens(p.start, p.end)
	return ok && slices.Equal(tokensOf(mixedForm(tokens)), form)
}

// mixedForm 是記憶裡的樣子：相鄰的英數併成一串，到空白為止。
func mixedForm(tokens []memoryToken) []memoryToken {
	var out []memoryToken
	for _, t := range tokens {
		if n := len(out); n > 0 && t.Kind == personal.MixedLiteral && out[n-1].Kind == personal.MixedLiteral && !strings.HasSuffix(out[n-1].Keys, " ") {
			out[n-1].Keys += t.Keys
			out[n-1].end = t.end
			continue
		}
		out = append(out, t)
	}
	return out
}

func tokensOf(tokens []memoryToken) []personal.MixedToken {
	out := make([]personal.MixedToken, len(tokens))
	for i, t := range tokens {
		out[i] = t.MixedToken
	}
	return out
}

func joinKeys(form []personal.MixedToken) string {
	var b strings.Builder
	for _, t := range form {
		b.WriteString(t.Keys)
	}
	return b.String()
}

func hasKind(form []personal.MixedToken, kind string) bool {
	return slices.ContainsFunc(form, func(t personal.MixedToken) bool { return t.Kind == kind })
}

// groupsOf 把一段按鍵拆成音節（keyGroups）。
func (s *Session) groupsOf(keys string) []keyGroup { return s.keyGroups([]keyUnit{{keys: keys}}) }

// memoryRuns 把 end 前的 memoryToken 切成連續的幾串：不算的位置、照記憶換過的、這次 ↓ 換過的與忘掉的段落隔開。
func (s *Session) memoryRuns(end int) [][]memoryToken {
	var runs [][]memoryToken
	var cur []memoryToken
	for pos := 0; pos < end; {
		t, ok := s.memoryTokenAt(pos)
		if !ok || t.end > end || s.memory.taken(t.memorySpan) {
			if len(cur) > 0 {
				runs, cur = append(runs, cur), nil
			}
			pos++
			continue
		}
		cur = append(cur, t)
		pos = t.end
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
	}
	return runs
}

// memoryBoundary：pos 不在照字面的英數字中間（data␣ 裡的 a␣、mp3s 裡的 mp3 不是一段）。
func (s *Session) memoryBoundary(pos int) bool { return !s.alnumAt(pos-1) || !s.alnumAt(pos) }

// alnumAt：p 是照字面的英文字母或數字（alnumLiteralAt 連數字標點也算）。
func (s *Session) alnumAt(p int) bool {
	return p >= 0 && p < len(s.syls) && (s.letterAt(p) || s.literals[p] && isDigitText(string(s.syls[p])))
}

// memoryIndex 是每次重看用的記憶索引與最長的按鍵長度。
func (s *Session) memoryIndex() (map[string][]personal.MixedRecord, int) {
	if !s.mixedMemory() || len(s.syls) == 0 {
		return nil, 0
	}
	return s.model.MixedMemoryIndex()
}

// rejudgeMemory 把照記憶換過、條件不再成立的段落換回，或照記憶換 end 前的一段，改了回報 true。
// 每個起點取最長的、有結論的按鍵；照記憶的樣子跟現在一樣時記成留著（hold），擋住 mixed_context 的規則。
func (s *Session) rejudgeMemory(index map[string][]personal.MixedRecord, longest, end int) bool {
	if len(index) == 0 && len(s.memory.applied) == 0 {
		return false
	}
	fresh := sync.OnceFunc(s.refresh) // 前一個字看解碼出的字；沒有要判斷的就不解碼
	for i := 0; i < len(s.memory.applied); {
		a := s.memory.applied[i]
		current := s.memoryFormIs(a.memorySpan, a.Form) // 被編輯過的照畫面
		if current {
			fresh()
			if d, ok := s.memoryDecide(index[a.Keys], a.memorySpan, a.official); ok && slices.Equal(d.Form, a.Form) {
				s.memory.applied[i] = d
				i++
				continue
			}
		}
		s.memory.applied = slices.Delete(s.memory.applied, i, i+1)
		if current && !a.hold() {
			if _, ok := s.replaceMemory(a.memorySpan, a.official); ok {
				return true
			}
			s.memory.applied = slices.Insert(s.memory.applied, i, a) // 選過字（鎖定）的照畫面留著
			i++
		}
	}
	for _, run := range s.memoryRuns(end) {
		for i := 0; i < len(run); {
			n, changed := s.matchMemory(index, longest, run[i:], fresh)
			if changed {
				return true
			}
			i += n
		}
	}
	return false
}

// matchMemory 從 run 的第一段起找記憶，回傳用掉幾段與有沒有換。
func (s *Session) matchMemory(index map[string][]personal.MixedRecord, longest int, run []memoryToken, fresh func()) (int, bool) {
	if !s.memoryBoundary(run[0].start) {
		return 1, false
	}
	var keys strings.Builder
	ends := []int{0} // ends[n] 是前 n 段按鍵的長度
	for _, t := range run {
		if keys.Len()+len(t.Keys) > longest {
			break
		}
		keys.WriteString(t.Keys)
		ends = append(ends, keys.Len())
	}
	all := keys.String()
	for n := len(ends) - 1; n > 0; n-- {
		records := index[all[:ends[n]]]
		if len(records) == 0 || !s.memoryBoundary(run[n-1].end) {
			continue
		}
		official := tokensOf(mixedForm(run[:n]))
		if !s.memoryInsertable(official) {
			continue // 換了之後換不回來
		}
		fresh()
		a, ok := s.memoryDecide(records, memorySpan{run[0].start, run[n-1].end}, official)
		if !ok {
			continue
		}
		if a.hold() {
			s.memory.applied = append(s.memory.applied, a)
			return n, false
		}
		if a.end, ok = s.replaceMemory(a.memorySpan, a.Form); ok {
			s.memory.applied = append(s.memory.applied, a)
			return n, true
		}
	}
	return 1, false
}

// replaceMemory 把 p 換成 form，回傳換上之後的尾端。有鎖定、明確選的或照記憶換過的段落時不換（replaceSpan），
// 換回時先把那段從 memoryState.applied 拿掉。
func (s *Session) replaceMemory(p memorySpan, form []personal.MixedToken) (int, bool) {
	n := len(s.syls)
	ok := s.replaceSpan(p.start, p.end, func() { s.insertMemoryForm(form) })
	return p.end + len(s.syls) - n, ok
}

// insertMemoryForm 在游標處插入 form：英數照字面、中文是音節（記下原按鍵）、注音同 mixed_context 改成的注音文。
func (s *Session) insertMemoryForm(form []personal.MixedToken) {
	for _, t := range form {
		switch t.Kind {
		case personal.MixedLiteral:
			s.insertMixedLiteral(t.Keys, false, false)
		case personal.MixedChinese:
			syl, _ := s.chineseSyllable(t.Keys)
			s.insertMixedReading(syl, t.Keys)
		case personal.MixedZhuyin:
			s.insertZhuyinUnit(t.Keys)
		}
	}
}

// memoryInsertable：form 每一段都插得回去（中文是一個國語音節、注音都是注音鍵）。
func (s *Session) memoryInsertable(form []personal.MixedToken) bool {
	for _, t := range form {
		switch t.Kind {
		case personal.MixedChinese:
			if _, ok := s.chineseSyllable(t.Keys); !ok {
				return false
			}
		case personal.MixedZhuyin:
			if slices.ContainsFunc(s.groupsOf(t.Keys), func(g keyGroup) bool { return g.literal }) {
				return false
			}
		case personal.MixedLiteral:
		default:
			return false
		}
	}
	return len(form) > 0
}

// formText 是照字面的一段在畫面上的字：注音照注音（a␣ 是 ㄇ），英數照按鍵。
func (s *Session) formText(t personal.MixedToken) string {
	if t.Kind != personal.MixedZhuyin {
		return t.Keys
	}
	return s.keyGroupsText(s.groupsOf(t.Keys), nil)
}

// chineseSyllable 是一個音節的按鍵（mp3）的讀音；整合風格（mixed_overwrite）也認得打錯又補對的按鍵（5jiy6）。
func (s *Session) chineseSyllable(keys string) (zhuyin.Syllable, bool) {
	groups := s.groupsOf(keys)
	if len(groups) != 1 || groups[0].literal {
		if !s.mixedOverwrite() {
			return "", false
		}
		if o, ok := s.overwriteReading(keys); ok {
			return o.group.syllable()
		}
		return "", false
	}
	return groups[0].syllable()
}

// memoryDecide 決定 p（現在是 official 的樣子）要不要照 records 換，換成別的樣子時都看合理度：前一個字與後一個音都對上的
// 紀錄照它自己的確認次數（跟官方一樣時直接留著）；只有按鍵對上時，每筆紀錄都換成同樣子才照加總的確認次數，不一致時照官方。
func (s *Session) memoryDecide(records []personal.MixedRecord, p memorySpan, official []personal.MixedToken) (memoryApplied, bool) {
	if len(records) == 0 {
		return memoryApplied{}, false
	}
	here := personal.MixedRef{Keys: records[0].Keys, Left: s.memoryLeft(p.start), Right: s.memoryRight(p.end)}
	for _, r := range records {
		if r.MixedRef == here {
			a := memoryApplied{p, personal.MixedApplied{MixedRef: here, Exact: true, Form: r.Form}, official}
			return a, s.memoryInsertable(r.Form) && (a.hold() || s.memoryPlausible(p, official, r.Form, r.Confirmed))
		}
	}
	form, confirmed := records[0].Form, 0
	for _, r := range records {
		if !slices.Equal(r.Form, form) {
			return memoryApplied{}, false
		}
		confirmed += r.Confirmed
	}
	a := memoryApplied{p, personal.MixedApplied{MixedRef: here, Form: form}, official}
	return a, s.memoryInsertable(form) && s.memoryPlausible(p, official, form, confirmed)
}

// memoryPlausible：把 official 換成 form 時中文那一側合不合理。兩邊都沒有中文（a␣ ↔ ㄇ）時直接換，但附近有英文時不換成注音。
func (s *Session) memoryPlausible(p memorySpan, official, form []personal.MixedToken, confirmed int) bool {
	step := memoryConfirmStep * float64(confirmed-1)
	english := 0.0
	if s.englishNearby(p) {
		english = memoryEnglishBonus
	}
	switch {
	case hasKind(form, personal.MixedChinese):
		score, ok := s.chineseScore(p, form)
		return ok && score >= memoryThreshold-step+english
	case hasKind(official, personal.MixedChinese):
		score, ok := s.chineseScore(p, official)
		return ok && score < memoryThreshold+step+english
	}
	return english == 0 || !hasKind(form, personal.MixedZhuyin)
}

// confirmedText 是外殼確認過的前文（神經模型的前文，沒有時是前一個詞）；私人、暫停時是空字串。
func (s *Session) confirmedText() string {
	if s.private || s.paused {
		return ""
	}
	if s.neuralLeft != "" {
		return s.neuralLeft
	}
	return s.documentLeft
}

// memoryLeft 是 pos 前一個字（中文照解碼的字，英數、空白、標點照原樣）；組字區開頭是前文最後一個字，沒有前文時是空字串（句首）。
func (s *Session) memoryLeft(pos int) string {
	if pos > 0 {
		return s.shownChars(pos)[pos-1]
	}
	return lastRunes(s.confirmedText(), 1)
}

// memoryRight 是 pos 的中文音節；空白、標點、英數或沒有東西時是空字串。
func (s *Session) memoryRight(pos int) string {
	if pos < len(s.syls) && !s.literals[pos] {
		return string(s.syls[pos])
	}
	return ""
}

// shownChars 是 [0, n) 每個位置畫面上的字（全形前的樣子）：中文照解碼的字，解碼的詞跟位置對不上時是讀音；照字面的字照原樣。
func (s *Session) shownChars(n int) []string {
	out := make([]string, n)
	for pos := range n {
		out[pos] = string(s.syls[pos])
	}
	for _, g := range s.path {
		if w := []rune(g.Word); g.Start < n && len(w) == g.End-g.Start {
			for k, r := range w {
				if pos := g.Start + k; pos < n && !s.literals[pos] {
					out[pos] = string(r)
				}
			}
		}
	}
	return out
}

// englishNearby：這一段以外的組字區或已確認的前文有連續兩個以上的英文字母。
func (s *Session) englishNearby(p memorySpan) bool {
	run := 0
	for _, r := range s.confirmedText() {
		if !englishLetter(string(r)) {
			run = 0
		} else if run++; run >= 2 {
			return true
		}
	}
	run = 0
	for pos := range s.syls {
		if p.start <= pos && pos < p.end || !s.letterAt(pos) {
			run = 0
		} else if run++; run >= 2 {
			return true
		}
	}
	return false
}

// memoryCell 是算合理度時的一格：中文音節（syl），或照字面的字（text）；span 是在要判斷的段落裡。
type memoryCell struct {
	syl  zhuyin.Syllable
	text string
	span bool
}

// memorySeg 是要評分的一個詞：before 是它前面的字（組字區連同換上的樣子），left 是語言模型的前一個詞。
type memorySeg struct {
	before, left string
	chineseWord
}

// chineseScore 是 p 換成 form 時，跟 p 重疊的中文詞每個字平均的 log10 機率：連同前面同一串的中文與後面
// memoryNeighbors 個中文音節解碼。有神經模型（單音節文字評分）時看前文評分，沒有前文、私人、暫停或評分失敗時用語言模型。
// 同樣的輸入（每個按鍵都會重看）照上次的結果。
func (s *Session) chineseScore(p memorySpan, form []personal.MixedToken) (float64, bool) {
	var cells []memoryCell
	for pos, text := range s.shownChars(p.start) {
		c := memoryCell{text: text}
		if !s.literals[pos] {
			c.syl = s.syls[pos]
		}
		cells = append(cells, c)
	}
	for _, t := range form {
		if t.Kind == personal.MixedChinese {
			syl, _ := s.chineseSyllable(t.Keys)
			cells = append(cells, memoryCell{syl: syl, span: true})
		} else {
			cells = append(cells, memoryCell{text: s.formText(t), span: true})
		}
	}
	for pos := p.end; pos < min(len(s.syls), p.end+memoryNeighbors) && !s.literals[pos]; pos++ {
		cells = append(cells, memoryCell{syl: s.syls[pos]})
	}
	var key strings.Builder
	key.WriteString(s.contextLeft(0) + "\x00" + s.neuralContext() + "\x00")
	for _, c := range cells {
		key.WriteString(string(c.syl) + "\x01" + c.text + "\x01")
		if c.span {
			key.WriteByte('*')
		}
	}
	if r, ok := s.memory.scores[key.String()]; ok {
		return r.value, r.ok
	}
	value, ok := s.scoreCells(cells)
	if s.memory.scores == nil {
		s.memory.scores = map[string]memoryScore{}
	}
	s.memory.scores[key.String()] = memoryScore{value, ok}
	return value, ok
}

func (s *Session) scoreCells(cells []memoryCell) (float64, bool) {
	dec := &decode.Decoder{Lex: s.model.Source(), Score: s.model, Beam: s.Opt.Beam, Continue: s.Opt.Continue}
	inSpan := func(c memoryCell) bool { return c.span }
	var segs []memorySeg
	for i := 0; i < len(cells); {
		if cells[i].syl == "" {
			i++
			continue
		}
		j := i
		for j < len(cells) && cells[j].syl != "" {
			j++
		}
		if !slices.ContainsFunc(cells[i:j], inSpan) {
			i = j
			continue
		}
		var syls []zhuyin.Syllable
		for _, c := range cells[i:j] {
			syls = append(syls, c.syl)
		}
		words, ok := bestWords(dec, syls)
		if !ok {
			return 0, false
		}
		var before strings.Builder
		for _, c := range cells[:i] {
			before.WriteString(c.text)
		}
		left := lm.BOS // 照字面的字後面重新開始（同解碼）
		if i == 0 {
			left = s.contextLeft(0)
		}
		for _, w := range words {
			at := i + w.at
			for k, r := range []rune(w.word) {
				cells[at+k].text = string(r)
			}
			if slices.ContainsFunc(cells[at:at+len(w.reading)], inSpan) {
				segs = append(segs, memorySeg{before.String(), left, w})
			}
			before.WriteString(w.word)
			left = w.word
		}
		i = j
	}
	if len(segs) == 0 {
		return 0, false
	}
	if score, ok := s.neuralScore(segs); ok {
		return score, true
	}
	var sum float64
	chars := 0
	for _, g := range segs {
		sum += math.Log10(s.model.Prob(g.left, g.word))
		chars += len(g.reading)
	}
	return sum / float64(chars), true
}

// neuralText：神經模型的單音節文字評分用得上（私人、暫停時不用）。
func (s *Session) neuralText() bool {
	opt := s.Rerank
	return opt.Scorer != nil && opt.TextWeight > 0 && opt.ContextRunes > 0 && !s.private && !s.paused
}

// neuralWindow 是 before 最後面神經模型看得到的前文。
func (s *Session) neuralWindow(before string) string {
	return lastRunes(before, min(s.Rerank.ContextRunes, rerank.MaxContextRunes))
}

// neuralScore 是 segs 每個字照前文的平均 log10 機率（神經模型的單音節文字評分）；有一個字評不了就回傳 false。
func (s *Session) neuralScore(segs []memorySeg) (float64, bool) {
	if !s.neuralText() {
		return 0, false
	}
	var sum float64
	chars := 0
	for _, g := range segs {
		before := s.neuralContext() + g.before
		for k, r := range []rune(g.word) {
			v, ok := s.neuralChar(s.neuralWindow(before), r, g.reading[k])
			if !ok {
				return 0, false
			}
			sum += v
			chars++
			before += string(r)
		}
	}
	return sum / float64(chars), true
}

// neuralChar 是 context 後面接 r 的 log10 機率。文字評分要兩個以上的候選，另一個只是陪評（的，r 是「的」時用「一」）。
func (s *Session) neuralChar(context string, r rune, syl zhuyin.Syllable) (float64, bool) {
	key := context + "\x00" + string(r)
	if v, ok := s.memory.chars[key]; ok {
		return v, true
	}
	other := "的"
	if r == '的' {
		other = "一"
	}
	row := func(t string) decode.Hypothesis {
		return decode.Hypothesis{Text: t, Segments: []decode.Seg{{Word: t, Start: 0, End: 1}}}
	}
	req := rerank.Request{AllowPartial: s.Rerank.AllowPartial, Mode: rerank.ModeText, Revision: s.rev + 1, Context: context,
		Reading: []zhuyin.Syllable{syl}, Candidates: []decode.Hypothesis{row(string(r)), row(other)}}
	if !rerank.Supported(req) {
		return 0, false
	}
	scores, err := s.Rerank.Scorer.Score(req)
	if err != nil || len(scores) != 2 || !finite(scores[0]) {
		return 0, false
	}
	v := scores[0] / math.Ln10
	if s.memory.chars == nil {
		s.memory.chars = map[string]float64{}
	}
	s.memory.chars[key] = v
	return v, true
}

// noteMemoryChoice 在選候選前記下那段的按鍵與樣子，回傳選完後呼叫的函式：按鍵相同、解釋換了（英數、中文、注音）時，
// 把換了的部分記成這次的選擇，送出時學（learnMixedMemory）。
func (s *Session) noteMemoryChoice(id int) func() {
	c := s.cand
	if !s.mixedMemory() || c.english {
		return func() {}
	}
	if c.mixed {
		return s.notePendingOverwrite(id)
	}
	start, length := c.span(id)
	tokens, ok := s.memoryTokens(start, start+length)
	if !ok {
		return func() {}
	}
	before, applied, n := mixedForm(tokens), slices.Clone(s.memory.applied), len(s.syls)
	return func() {
		tokens, ok := s.memoryTokens(start, start+length+len(s.syls)-n)
		after := mixedForm(tokens)
		if !ok || joinKeys(tokensOf(before)) != joinKeys(tokensOf(after)) {
			return
		}
		trim := before
		for len(trim) > 0 && len(after) > 0 && trim[0].MixedToken == after[0].MixedToken {
			trim, after = trim[1:], after[1:]
		}
		for len(trim) > 0 && len(after) > 0 && trim[len(trim)-1].MixedToken == after[len(after)-1].MixedToken {
			trim, after = trim[:len(trim)-1], after[:len(after)-1]
		}
		if len(after) == 0 {
			return
		}
		choice := memoryChoice{memorySpan: memorySpan{after[0].start, after[len(after)-1].end}, form: tokensOf(after)}
		was := memorySpan{trim[0].start, trim[len(trim)-1].end}
		if i := slices.IndexFunc(applied, func(a memoryApplied) bool { return a.overlaps(was) }); i >= 0 && applied[i].Exact {
			choice.contest = &applied[i].MixedRef
		}
		s.memory.choices = append(s.memory.choices, choice)
	}
}

// learnMixedMemory 在送出時學這次 ↓ 換過、還是那個樣子的段落；照記憶換過、沒被改掉的紀錄算用到。私人、暫停時不學。
func (s *Session) learnMixedMemory() {
	if !s.mixedMemory() || s.paused || s.private || s.localBlocked {
		return
	}
	var choices []personal.MixedChoice
	for _, c := range s.memory.choices {
		if s.memoryFormIs(c.memorySpan, c.form) {
			ref := personal.MixedRef{Keys: joinKeys(c.form), Left: s.memoryLeft(c.start), Right: s.memoryRight(c.end)}
			choices = append(choices, personal.MixedChoice{MixedRef: ref, Form: c.form, Contest: c.contest})
		}
	}
	var used []personal.MixedApplied
	for _, a := range s.memory.applied {
		if !a.hold() {
			used = append(used, a.MixedApplied)
		}
	}
	s.model.LearnMixedMemory(choices, used)
}

// openMemorySpan：↓ 落在照記憶換過、或這次用 ↓ 換過解釋的段落裡時開整段的候選（mp3／允／狁…），不從最後一個字（3）開始。
func (s *Session) openMemorySpan(pos int, before bool) bool {
	p, ok := s.changedSpan(pos)
	if !ok || p.end-p.start < 2 {
		return false
	}
	anchor := p.start
	if before {
		anchor = p.end
	}
	return s.openAnchored(anchor, p.end-p.start, before)
}

// changedSpan 是 pos 所在、照記憶換過或這次用 ↓ 換過解釋（現在還是那個樣子）的段落。
func (s *Session) changedSpan(pos int) (memorySpan, bool) {
	p := memorySpan{pos, pos + 1}
	if i := s.memory.appliedAt(p, false); i >= 0 {
		return s.memory.applied[i].memorySpan, true
	}
	for _, c := range slices.Backward(s.memory.choices) {
		if c.overlaps(p) && s.memoryFormIs(c.memorySpan, c.form) {
			return c.memorySpan, true
		}
	}
	return memorySpan{}, false
}

// forgetMemoryCandidate：候選窗反白的是照記憶換過的那段畫面上的樣子時，Shift＋Delete 刪掉讓它被換的紀錄、換回原本的樣子，
// 這次組字不再照記憶換那裡。
func (s *Session) forgetMemoryCandidate() bool {
	c := s.cand
	if !s.mixedMemory() || c.cursor < 0 || c.cursor >= len(c.items) {
		return false
	}
	start, length := c.span(c.cursor)
	i := s.memory.appliedAt(memorySpan{start, start + length}, false)
	if i < 0 || c.items[c.cursor].Entry.Surface != s.shownText(start, start+length) {
		return false
	}
	a := s.memory.applied[i]
	s.model.ForgetMixedMemory(a.MixedApplied)
	s.forgot = true
	s.cand = nil
	s.memory.applied = slices.Delete(s.memory.applied, i, i+1)
	end, _ := s.replaceMemory(a.memorySpan, a.official)
	s.memory.blocked = append(s.memory.blocked, memorySpan{a.start, end})
	s.notices = append(s.notices, "不再自動換："+a.Keys)
	return true
}
