package session

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/input"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

const (
	// mixedNeuralDeadline：每鍵 P7（列切法、評分、回頭比）合計最多花這麼久（每鍵不比官方慢 30 ms 以上）。
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
	// mixedBusyKeys：評分超過期限（電腦忙）時，連同這一鍵共這麼多鍵不問神經、照規則，之後再試；
	// 忙時照樣送，評分會偶爾超過 worker 的硬時限（-neural-timeout），worker 被殺要停 3 秒。
	mixedBusyKeys = 8
	// mixedPopupGap：照空白的決定顯示的待定按鍵，決定贏其他切法不到這麼多才跳小視窗（spaceSure）。
	mixedPopupGap = 2.0
	// mixedNeuralKeys、mixedNeuralSyllables：後段最多幾個按鍵、幾個音節。
	mixedNeuralKeys      = 16
	mixedNeuralSyllables = 4
)

// mixedSplit 是一種切法：前段 raw[:at] 照字面，後段照順序組成 syls；literal 是整串照字面。
// rows 是整串文字（前段加中文 N-best）與中文部分的引擎分數。
type mixedSplit struct {
	at      int
	syls    []zhuyin.Syllable
	literal bool
	repair  bool
	wordCut bool // 切在詞的後面（mixedWordPrefix）或開頭
	rows    []decode.Hypothesis
}

// neuralMixedTone：各切法比所有同音字的神經機率加總（不受哪個字常用影響：常打 喔 的 2jo4i␣ 照樣比 對喔＋對啊… 對 對i；
// 修正扣分，repairPenalty），最好的不是規則結果 act、而且高出 mixedNeuralMargin 才換；評分器沒有、失敗或超過期限照規則。
// 結果是中文音節或照原樣接空白時另回傳之後要回頭再比的紀錄（revisitMixed；位置由呼叫端填）。gap 是選到的切法贏其他切法多少：
// 規則就決定得了（不問神經）、只有一種切法是 +Inf，沒比到是 0。decided：神經比過或不用比；
// 現在不能問、超過期限的是 false，之後能問時可以再算。
func (s *Session) neuralMixedTone(act mixedToneAction, raw, key string, tone rune) (_ mixedToneAction, _ *mixedRevisit, gap float64, decided bool) {
	if !s.mixedNeuralWanted(act, raw, key) {
		return act, nil, math.Inf(1), true
	}
	if !s.mixedNeuralReady() {
		return act, nil, 0, false
	}
	started := time.Now()
	defer func() { s.mixedSpent += time.Since(started) }()
	splits, rule := s.mixedSplits(act, raw, key, tone)
	var texts []string
	for _, sp := range splits {
		for _, h := range sp.rows {
			texts = append(texts, h.Text)
		}
	}
	if len(splits[rule].rows) == 0 || len(texts) < 2 {
		return act, nil, math.Inf(1), true
	}
	s.mixedAsked = true
	scores, err := s.scoreTexts(texts, s.cursor, mixedNeuralDeadline-s.mixedSpent-time.Since(started))
	if err != nil {
		return act, nil, 0, !errors.Is(err, rerank.ErrDeadline)
	}
	heads, scored := make([]int, len(splits)), make([]float64, len(splits))
	best := -1
	for n, sp := range splits {
		got := scores[:len(sp.rows)]
		scores = scores[len(sp.rows):]
		heads[n] = -1
		if len(got) == 0 {
			continue
		}
		heads[n], scored[n] = rerank.Choose(sp.rows, got, s.Rerank.Weight, 0), logSumExp(got) // 代表字給回頭比用
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
		act = mixedToneAction{} // 規則不是音節時規則結果就是照字面（rule 0），換成照字面的只有音節
	} else {
		act = mixedToneAction{kind: mixedToneSyllable, at: sp.at, syls: sp.syls}
	}
	gap = math.Inf(1)
	for n := range splits {
		if n != best && heads[n] >= 0 {
			gap = min(gap, scored[best]-scored[n])
		}
	}
	// 回頭再比的：照原樣接空白（win115␣ 的 win115，後文 後 讓 win11之 翻過來）、中文音節；整串組成一個音節的
	// 只有英文字典的詞、英數詞（up␣ 的 音、mp3 的 允）、有數字鍵的（5␣ 的 之，後文 分鐘 讓 5 翻過來），純中文（c96 的 還）不比。
	// 贏其他切法 mixedRevisitGap 以上的（cl3 的 好）後文也翻不過來，不比。
	literal := act.kind == mixedToneLiteral && key == " "
	chinese := act.kind == mixedToneSyllable && (act.at > 0 || len(act.syls) > 1 || s.mixedEnglishWord(raw) ||
		english.Alnum(raw+key) == english.AlnumKeysSecond || strings.ContainsFunc(raw, unicode.IsDigit))
	if (literal || chinese) && gap < mixedRevisitGap {
		return act, &mixedRevisit{raw: raw, key: key, splits: splits, heads: heads, shown: best, gap: gap}, gap, true
	}
	return act, nil, gap, true
}

// logSumExp 是 log(Σ exp(x))：同一種切法各同音字的機率加總。
func logSumExp(xs []float64) float64 {
	top := slices.Max(xs)
	sum := 0.0
	for _, x := range xs {
		sum += math.Exp(x - top)
	}
	return top + math.Log(sum)
}

var errTextsUnusable = errors.New("texts not scorable")

// scoreTexts 以 neuralContext 加組字區 [0, end) 為前文評 texts；送不出去、分數數量不對或不是有限值時回 errTextsUnusable。
func (s *Session) scoreTexts(texts []string, end int, deadline time.Duration) ([]float64, error) {
	context := ""
	if s.Rerank.ContextRunes > 0 {
		context = lastRunes(s.neuralContext()+s.spanText(0, end), min(s.Rerank.ContextRunes, rerank.MaxContextRunes))
	}
	req := rerank.TextsRequest{Revision: s.rev + 1, Context: context, Texts: texts}
	if !s.Rerank.SupportsTexts(req) {
		return nil, errTextsUnusable
	}
	scores, err := s.Rerank.Texts.ScoreTexts(req, deadline)
	if errors.Is(err, rerank.ErrDeadline) {
		s.mixedBusy = mixedBusyKeys
	} else if err == nil && (len(scores) != len(texts) || slices.ContainsFunc(scores, func(x float64) bool { return !finite(x) })) {
		err = errTextsUnusable
	}
	return scores, err
}

const (
	// mixedRevisitWindow：神經比過的決定，之後組字區多了字時最多再比幾次（約四個音節的後文）。
	mixedRevisitWindow = 4
	// mixedRevisitGap：決定時贏其他切法不到這麼多才回頭比（mp3 的 允 贏 3.4、後文 格式 但 翻過來 5.4）。
	mixedRevisitGap = 10.0
	// mixedRevisitReserve：回頭比的期限先扣掉這麼多，留給換掉之後官方重新評分後面的字。
	mixedRevisitReserve = 10 * time.Millisecond
	// mixedRevisitSlots：同時最多等幾個決定回頭比（下載了 mp3 但 的 允、但 都要等後文）。
	mixedRevisitSlots = 2
)

// mixedRevisit 是神經比過的決定（中文音節、照原樣接空白）：之後組字區多了字時帶著後文再比一次，夠強才改，最多改一次。
type mixedRevisit struct {
	start    int               // 這次決定插入的組字位置
	syls     []zhuyin.Syllable // 插入的音節（s.syls[start:end()]）；之後被刪改就作廢
	raw, key string
	splits   []mixedSplit
	heads    []int   // 各切法的代表文字（rows 的索引；-1 沒有）
	shown    int     // 選到的切法
	gap      float64 // 決定時贏第二名多少；超過 mixedRevisitSlots 個時放掉 gap 最大的（keepRevisit）
	seen     int     // 之後比過幾次
	checked  int     // 上次比（或記下）時組字區的音節數
}

func (v *mixedRevisit) end() int { return v.start + len(v.syls) }

// revisitMixed：按鍵處理完、組字區多了字時，等著的決定依序以「畫面上的樣子＋之後的字」對
// 「其他切法的代表＋之後的字」再比一次，其他切法高出 mixedNeuralMargin（修正另扣分）就換掉；
// 換過、比滿 mixedRevisitWindow 次、或被刪改、手選過就固定。一鍵最多換一個（換完官方重排整句）。
// 期限是這一鍵 P7 還沒用掉的時間；不夠、這一鍵已經為決定問過神經（評分、換字、重排加起來會超過期限），
// 或還有待定按鍵、打到一半的注音時留到下一鍵。
func (s *Session) revisitMixed() {
	s.dropRevisits()
	if s.mixedAsked || !s.mixedNeuralReady() || s.mixed.raw != "" || !s.comp.Empty() {
		return
	}
	for i, v := range s.mixedRevisits {
		if len(s.syls) <= v.checked {
			continue
		}
		left := mixedNeuralDeadline - s.mixedSpent - mixedRevisitReserve
		if left < mixedNeuralMin {
			return
		}
		started := time.Now()
		sp, ok := s.revisitBetter(v, left)
		if ok {
			s.mixedRevisits = slices.Delete(s.mixedRevisits, i, i+1)
			s.replaceMixed(v, sp)
		}
		s.mixedSpent += time.Since(started)
		if ok {
			return
		}
	}
}

// revisitBetter 帶著後文比 v 一次（算一次 seen），回傳贏過畫面上的樣子的切法。
func (s *Session) revisitBetter(v *mixedRevisit, left time.Duration) (mixedSplit, bool) {
	started := time.Now()
	v.seen, v.checked = v.seen+1, len(s.syls)
	follow := s.spanText(v.end(), s.cursor-v.end())
	texts := []string{s.spanText(v.start, len(v.syls)) + follow}
	var owners []int
	for i, sp := range v.splits {
		if i == v.shown || v.heads[i] < 0 {
			continue
		}
		t := sp.rows[v.heads[i]].Text
		if sp.literal && v.key == " " {
			t += " " // 空白被當一聲用掉的英文換回來時，空白還回來（go up and down）
		}
		if t += follow; s.Rerank.FitsText(t) && !slices.Contains(texts, t) {
			texts = append(texts, t)
			owners = append(owners, i)
		}
	}
	if len(texts) < 2 {
		return mixedSplit{}, false
	}
	scores, err := s.scoreTexts(texts, v.start, left-time.Since(started))
	if err != nil {
		return mixedSplit{}, false
	}
	best, bestScore := -1, scores[0]+mixedNeuralMargin
	for j, i := range owners {
		if score := scores[j+1] - repairPenalty(v.splits[i], v.splits[v.shown]); score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return mixedSplit{}, false
	}
	return v.splits[best], true
}

// keepRevisit 記下新的決定；超過 mixedRevisitSlots 個時放掉最有把握（gap 最大）的。
func (s *Session) keepRevisit(v *mixedRevisit) {
	s.dropRevisits()
	s.mixedRevisits = append(s.mixedRevisits, v)
	if len(s.mixedRevisits) > mixedRevisitSlots {
		drop := 0
		for i, o := range s.mixedRevisits {
			if o.gap > s.mixedRevisits[drop].gap {
				drop = i
			}
		}
		s.mixedRevisits = slices.Delete(s.mixedRevisits, drop, drop+1)
	}
}

// revisitAwaits：組字區 start 以後有還在等後文回頭比的決定。
func (s *Session) revisitAwaits(start int) bool {
	return slices.ContainsFunc(s.mixedRevisits, func(v *mixedRevisit) bool { return v.end() > start && s.revisitLive(v) })
}

// dropRevisits 放掉組字區多了字、卻已經比滿或失效的決定。
func (s *Session) dropRevisits() {
	s.mixedRevisits = slices.DeleteFunc(s.mixedRevisits, func(v *mixedRevisit) bool {
		return len(s.syls) > v.checked && !s.revisitLive(v)
	})
}

// revisitLive：還沒比滿 mixedRevisitWindow 次，也還有效（revisitValid）。
func (s *Session) revisitLive(v *mixedRevisit) bool {
	return v.seen < mixedRevisitWindow && s.revisitValid(v)
}

// revisitValid：游標在組字區最後、那次插入的字沒被刪改、沒有手選過。
func (s *Session) revisitValid(v *mixedRevisit) bool {
	if s.cursor != len(s.syls) || v.end() > s.cursor || !slices.Equal(s.syls[v.start:v.end()], v.syls) {
		return false
	}
	for start, lock := range s.locks {
		if start < v.end() && v.start < start+lock.Len {
			return false
		}
	}
	return true
}

// replaceMixed 把組字區 [v.start, v.end()) 換成切法 sp，後面的字不動（後面等著的決定跟著移），游標回到最後；
// 照原樣打出的空白換成中文時收回來當一聲（win115␣ → win11之）。
// 換到一半時不評分（insertLiteral、insertMixedTyped 會先 refresh），換完整句重解評分一次（Rescore）。
func (s *Session) replaceMixed(v *mixedRevisit, sp mixedSplit) {
	allow, before := s.allowRerank, len(s.syls)
	s.allowRerank = false
	s.cursor = v.end()
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
	for _, o := range s.mixedRevisits {
		if o.start >= v.end() {
			o.start += len(s.syls) - before
			o.checked += len(s.syls) - before
		}
	}
	s.cursor = len(s.syls)
	s.Rescore()
}

// spaceMemo 記下待定按鍵接空白的決定（mixedSpaceAction）：組字區照它顯示，按空白時直接套用，不再評分一次。
// 前文確認等不是按鍵的請求不能問神經，看的是待定按鍵與組字區（pathSig）有沒有變，不看 rev。
type spaceMemo struct {
	raw, sig  string
	cursor    int
	decided   bool    // 神經比過或不用比（neuralMixedTone）；沒比到的之後能問時再算
	gap       float64 // 決定贏其他切法多少（spaceSure）
	rule, act mixedToneAction
	visit     *mixedRevisit
}

// mixedSpaceAction：沒選過（choice 0）的待定按鍵接空白時規則的決定 rule，與神經切法比較後實際套用的 act。
// 組字區照 act 顯示（組字區＝空白的結果：aq8 是 aㄆㄚ、神經判成英文的 up 是 up），空白鍵直接用。
func (s *Session) mixedSpaceAction() (rule, act mixedToneAction, visit *mixedRevisit) {
	raw := s.mixed.raw
	m := &s.spaceMemo
	if m.raw == raw && m.sig == s.pathSig && m.cursor == s.cursor && (m.decided || !s.mixedNeuralReady()) {
		return m.rule, m.act, m.visit
	}
	rule = s.decideMixedTone(raw, " ", zhuyin.Tone1)
	act, visit, gap, decided := s.neuralMixedTone(rule, raw, " ", zhuyin.Tone1)
	*m = spaceMemo{raw: raw, sig: s.pathSig, cursor: s.cursor, decided: decided, gap: gap, rule: rule, act: act, visit: visit}
	return rule, act, visit
}

// spaceSure：接空白的決定有把握：規則就決定得了、只有一種切法，或神經贏其他切法 mixedPopupGap 以上。
// 選過的注音文照習慣（i 的 ㄛ），不看空白的決定。單一個數字鍵（5 的 ㄓ、9 的 ㄞ）多半是正要打中文的第一鍵，也不跳小視窗，
// 要另一個按 ↓。
func (s *Session) spaceSure() bool {
	if raw := s.mixed.raw; len(raw) == 1 && '0' <= raw[0] && raw[0] <= '9' || s.zhuyinKeyHabit(raw) {
		return true
	}
	s.mixedSpaceAction()
	return s.spaceMemo.decided && s.spaceMemo.gap >= mixedPopupGap
}

// spaceShown：沒選過的待定按鍵接空白會組成一個中文音節、而組字區本來不會這樣顯示時，回傳切點與音節：修正讀音（gjn 的 ㄙㄨ）、
// 切成英文＋中文（aq8 的 aㄆㄚ）、數字與單獨的 . ,（10 的 ㄅㄢ、205; 的 20ㄓㄤ、. 的 ㄡ；要數字、句點按 ↓ 選原按鍵）。
// 使用者加的英文詞、選過的注音文照原本的顯示；有字母時整串照順序組成的（su 的 ㄋㄧ）照一般注音顯示，照順序就是注音、
// 切開是猜的（au/ 打到一半的 名 是 ㄇㄧㄥ，不是 aㄧㄥ）也是。
func (s *Session) spaceShown() (int, zhuyin.Syllable, bool) {
	raw := s.mixed.raw
	letters := strings.ContainsFunc(raw, unicode.IsLetter)
	if s.vocabEnglish(raw) || s.zhuyinKeyHabit(raw) || !letters && !strings.ContainsFunc(raw, unicode.IsDigit) && !s.punctKeyWord(raw) {
		return 0, "", false
	}
	_, act, _ := s.mixedSpaceAction()
	if act.kind != mixedToneSyllable || len(act.syls) != 1 || letters && act.plain {
		return 0, "", false
	}
	if letters {
		if inOrder, ok := s.inOrderSyllable(raw); ok && (act.at > 0 || inOrder == act.syls[0].Base()) {
			return 0, "", false
		}
	}
	return act.at, act.syls[0], true
}

// spaceNeuralLiteral：沒選過的待定按鍵，規則接空白會組成中文、神經判成照原按鍵（go up 的 up、我 10 的 10）：組字區照字面顯示，
// Tab 的「另一個」是規則的注音。
func (s *Session) spaceNeuralLiteral() bool {
	raw := s.mixed.raw
	if !strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) || s.vocabEnglish(raw) || s.zhuyinKeyHabit(raw) {
		return false
	}
	rule, act, _ := s.mixedSpaceAction()
	return rule.kind == mixedToneSyllable && act.kind == mixedToneLiteral
}

// mixedNeuralReady：現在能問神經（不看待定按鍵本身）：有評分器、這個請求可以重排、這一鍵的期限還夠、不在忙時退讓，
// 不在選字、私密、暫停、英文模式，待定按鍵沒選過。
func (s *Session) mixedNeuralReady() bool {
	return s.Rerank.Texts != nil && s.allowRerank && mixedNeuralDeadline-s.mixedSpent >= mixedNeuralMin && s.mixedBusy == 0 &&
		s.cand == nil && !s.private && !s.paused && !s.english && !s.englishWord && s.mixed.choice == 0 && s.mixed.accepted == ""
}

// mixedNeuralWanted：純中文不多問神經。待定按鍵要有英文字母，而且是英文字典的詞（up␣ 照規則是 音）、
// 接聲調鍵是 alnum.tsv 照舊出字的英數詞（mp3 照規則是 允），或規則結果不是整串照順序組成一個音節，
// 或有數字、大寫或符號（整串照順序組成一個音節時注音的數字、標點鍵不算：2k7 的、q96 排，除非數字鍵開頭、
// 後段也組得成：2j04 的 斷 對 2萬）；或沒有字母、有數字（10␣ 的 班 對 10、205;␣ 的 205; 對 20張）。
// 只有標點的（.␣）、留注音的照規則；
// 英文字典的詞、alnum.tsv 照字面的英數詞，規則照字面的也照規則（sum3 不是 su與、win10␣ 不是 win班）。
func (s *Session) mixedNeuralWanted(act mixedToneAction, raw, key string) bool {
	if act.kind == mixedToneKeep {
		return false
	}
	if act.kind == mixedToneLiteral && (s.mixedEnglishWord(raw) || english.Alnum(raw) == english.AlnumLiteral || s.literalToken(raw+key)) {
		return false
	}
	// 規則照數字寫法留下的（15/4、8-4、3.5␣）也照規則：沒有前文時整串評分偏好字少的 1正、8二。
	if full := s.literalNumberBefore() + strings.TrimSuffix(raw+key, " "); act.kind != mixedToneSyllable && numberLike(full) && strings.ContainsAny(full, ".,:/-") {
		return false
	}
	// 數字鍵緊接在中文後面、規則整串組成中文的照規則（帶個 10␣ 是 班、可是要 2j04 是 斷）：要數字多半用數字鍵盤打。
	if act.kind == mixedToneSyllable && act.at == 0 && raw[0] >= '0' && raw[0] <= '9' && s.mixedAfterChinese() {
		return false
	}
	if !strings.ContainsFunc(raw, unicode.IsLetter) {
		return strings.ContainsFunc(raw, unicode.IsDigit)
	}
	if s.mixedEnglishWord(raw) || key != " " && english.Alnum(raw+key) == english.AlnumKeysSecond ||
		act.kind != mixedToneSyllable || act.at != 0 {
		return true
	}
	if strict, ok := s.strictSyllables(raw + key); ok && slices.Equal(strict, act.syls) {
		return s.digitHeadSplit(raw, key)
	}
	return strings.ContainsFunc(raw, func(r rune) bool { return r < 'a' || r > 'z' })
}

// digitHeadSplit：raw 開頭是數字鍵，前段數字照字面、後段照順序也組得成音節（2j04 的 2＋萬）。
func (s *Session) digitHeadSplit(raw, key string) bool {
	for at := 1; at < len(raw) && raw[at-1] >= '0' && raw[at-1] <= '9'; at++ {
		if _, ok := s.strictSyllables(raw[at:] + key); ok {
			return true
		}
	}
	return false
}

// mixedSplits 列出要比較的切法與規則結果是第幾個：整串照字面、規則的切法、每個切點的後段照順序組成的音節，
// 和 P6 的修正（整串是英文字典的詞時不修正：dll␣ 不是 尻、zip␣ 不是 分）。
func (s *Session) mixedSplits(act mixedToneAction, raw, key string, tone rune) ([]mixedSplit, int) {
	literal := raw
	if key != " " {
		literal += key
	}
	splits := []mixedSplit{{literal: true, rows: []decode.Hypothesis{{Text: literal}}}}
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
			if t := sp.rows[0].Text; s.Rerank.FitsText(t) {
				seen[t] = true
			} else {
				sp.rows = nil
			}
		} else {
			rows, _ := dec.NBest(input.Exact{}.Expand(sp.syls), nil, false, mixedNeuralNBest)
			for _, h := range rows {
				if h.Text = raw[:sp.at] + h.Text; !seen[h.Text] && s.Rerank.FitsText(h.Text) {
					seen[h.Text] = true
					sp.rows = append(sp.rows, h)
				}
			}
		}
		total += len(sp.rows)
	}
	for k := mixedNeuralNBest - 1; total > rerank.MaxTexts && k > 0; k-- {
		total = 0
		for i := range splits {
			splits[i].rows = splits[i].rows[:min(k, len(splits[i].rows))]
			total += len(splits[i].rows)
		}
	}
}
