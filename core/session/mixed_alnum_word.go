package session

import (
	"math"
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
)

// 神經模型判斷中文換回英數的門檻（log10），取自 2026-11-04 用本機神經模型量的 1754 句（410 句該換、1344 句自然句）：
// 換對 59%，誤換 2 句都是官方已經選錯字（這塊欲很漂亮、這類城市慈加到）。
const (
	// alnumWordMargin：換成英數時後面的字的分數差，加上字本身的分數差 × alnumWordSelf，至少要這麼多才換：
	// 我下載了mp3格式 +0.43、傳給我那個mp3播放器 −0.63；我下載了允格（後面只有一個字）−2.29、未指定時採前方 −1.26、
	// 你呢我很好 −6.07 留著。
	alnumWordMargin = -1.25
	// alnumWordSelf：字本身的分數差算幾成。英數的字元本來就比一個中文字少見（我下載了允 比 mp3 高 2.67），整句加總會偏中文。
	alnumWordSelf = 0.9
	// alnumWordBack：換過的字，後面再打的字讓分數低於門檻這麼多才換回。
	alnumWordBack = 1.0
	// alnumWordRight：看後面幾個字，不夠時看已經打的；只看一個字分不出 格（式）、播（放器）。
	alnumWordRight = 3
	// alnumWordUsual：語言模型照前一個詞給這個字的 log10 機率到這麼高是常見的用法（在、我、了），不問神經模型。
	alnumWordUsual = -2.0
	// alnumWordChars：中文那邊給神經模型挑的同音字數（文字評分一次最多 16 個候選）。
	alnumWordChars = 15
)

// alnumWord 是換成英數的一個中文字：[start, end) 是換上的英數，char 是換之前的字。
type alnumWord struct {
	memorySpan
	keys string
	char rune
}

// rejudgeAlnumWord 把官方組成一個中文字的英數按鍵（mp3 是「允」、i7 是「喔」），在後面打的字看得出是英數時換回照字面
// （mixed_context 的 alnum）：我下載了mp3檔案、我買了i7處理器。神經模型比兩種寫法，看後面幾個字的分數差、加上字本身
// 分數差的幾成：英數的字元本身分數低，整句加總會偏中文（允檔案 −12.68、mp3檔案 −13.82）。換過的字，後面的字不再
// 支持時換回。改了回報 true。
func (s *Session) rejudgeAlnumWord(end int) bool {
	if !s.contextRule(inputstyle.ContextAlnum) || !s.neuralText() {
		return false
	}
	s.refresh()
	shown := s.shownChars(len(s.syls))
	need := alnumWordMargin - alnumWordBack
	for i := 0; i < len(s.memory.words); i++ {
		w := s.memory.words[i]
		if w.end > end || s.memory.taken(w.memorySpan) {
			continue
		}
		if !s.memoryFormIs(w.memorySpan, literalForm(w.keys)) { // 被編輯過的照畫面
			s.memory.words = slices.Delete(s.memory.words, i, i+1)
			i--
			continue
		}
		if gain, ok := s.alnumWordGain(w, end, shown, need); !ok || gain >= need {
			continue
		}
		chinese := []personal.MixedToken{{Keys: w.keys, Kind: personal.MixedChinese}}
		if _, ok := s.replaceMemory(w.memorySpan, chinese); ok {
			return true // 換掉那段時 memory.shift 一起拿掉 w
		}
	}
	for pos := 0; pos+1 < end; pos++ { // 後面打了字才看得出來，句尾的字不換
		w, ok := s.alnumWordAt(pos, shown)
		if !ok {
			continue
		}
		if gain, ok := s.alnumWordGain(w, end, shown, alnumWordMargin); !ok || gain < alnumWordMargin {
			continue
		}
		if w.end, ok = s.replaceMemory(w.memorySpan, literalForm(w.keys)); ok {
			s.memory.words = append(s.memory.words, w)
			return true
		}
	}
	return false
}

// alnumWordAt：pos 是官方組成單獨一個字的英數按鍵：字母開頭、只有英文字母與數字、以聲調鍵結尾，前面不是照字面的英數
// （那是英數接中文 rejudgeAlnum 看的），語言模型照前一個詞看不是常見的用法；沒有鎖定、明確選過、照記憶或 ↓ 換過。
func (s *Session) alnumWordAt(pos int, shown []string) (alnumWord, bool) {
	t, ok := s.memoryTokenAt(pos)
	if !ok || t.Kind != personal.MixedChinese || !alnumWordKeys(t.Keys) || !s.explicitTone(t.Keys) || s.alnumAt(pos-1) ||
		s.pinnedAt(pos) || s.memory.taken(t.memorySpan) || s.memory.cutAt(t.memorySpan) {
		return alnumWord{}, false
	}
	char := []rune(shown[pos])
	left, ok := s.singleWordLeft(pos)
	if !ok || len(char) != 1 || math.Log10(s.model.Prob(left, shown[pos])) >= alnumWordUsual {
		return alnumWord{}, false
	}
	return alnumWord{t.memorySpan, t.Keys, char[0]}, true
}

// singleWordLeft：官方把 pos 解成單獨一個字時，回傳語言模型的前一個詞（照字面的字後面重新開始，同解碼）。
func (s *Session) singleWordLeft(pos int) (string, bool) {
	left := s.contextLeft(0)
	for _, g := range s.path {
		switch {
		case g.Start == pos:
			return left, g.End == pos+1
		case s.literals[g.Start]:
			left = lm.BOS
		default:
			left = g.Word
		}
	}
	return "", false
}

func alnumWordKeys(keys string) bool {
	return len(keys) >= 2 && englishLetter(keys[:1]) && !strings.ContainsFunc(keys, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// alnumWordGain 是 w 照字面（w.keys）比中文多出的分數：後面 alnumWordRight 個字（組字區 end 前）的分數差，加上字本身的
// 分數差 × alnumWordSelf。中文那邊取畫面上的字與同音字裡神經模型最看好的字之中分數高的：官方選錯字時（執行續）錯字
// 不合理，照字面反而顯得合理。照字面那邊評到一半就確定低於 floor 時直接回傳：分數只會再扣。
// 沒有前文時評不了（神經模型要前文）。
func (s *Session) alnumWordGain(w alnumWord, end int, shown []string, floor float64) (float64, bool) {
	before := s.neuralContext() + strings.Join(shown[:w.start], "")
	syl, ok := s.chineseSyllable(w.keys)
	if before == "" || !ok {
		return 0, false
	}
	right := []rune(strings.Join(shown[w.end:min(end, w.end+alnumWordRight)], ""))
	window := s.neuralWindow(before)
	chineseOf := func(c rune) (float64, bool) {
		self, ok := s.neuralChar(window, c, syl)
		after, ok2 := s.neuralSum(before+string(c), right, alnumReading, math.Inf(-1))
		return after + alnumWordSelf*self, ok && ok2
	}
	chinese, ok := chineseOf(w.char)
	if !ok {
		return 0, false
	}
	literal := append([]rune(w.keys), right...)
	var self, after float64
	for i, r := range literal {
		v, ok := s.neuralChar(s.neuralWindow(before+string(literal[:i])), r, alnumReading)
		if !ok {
			return 0, false
		}
		if i < len(w.keys) {
			self += v
		} else {
			after += v
		}
		if gain := after + alnumWordSelf*self - chinese; gain < floor {
			return gain, true
		}
	}
	// 贏過畫面上的字才評同音字（多半到不了這裡）
	if best, _, _ := s.neuralBest(window, s.commonChars([]rune{w.char}, syl, alnumWordChars), syl); best != w.char {
		v, ok := chineseOf(best)
		if !ok {
			return 0, false
		}
		chinese = max(chinese, v)
	}
	return after + alnumWordSelf*self - chinese, true
}
