package inputstyle

import (
	"slices"
	"strings"
)

// Extras 是整合風格的全域開關（設定檔的 blend，internal/settings）：不跟著風格，選哪個風格都照它。
// 引擎用 Apply 換成 Rules 裡 fork 的欄位，session 只看 Rules。
type Extras struct {
	MixedResplit    bool   `json:"mixed_resplit"`     // ↓／Tab 跨英數換整段、多字 ↓ 清單
	MixedRepeats    bool   `json:"mixed_repeats"`     // 自動刪除冗餘注音（ggggk6 是「什」）
	MixedReversed   bool   `json:"mixed_reversed"`    // 反序打的英文字（url␣ 不是「交」）
	MixedZhuyinText bool   `json:"mixed_zhuyin_text"` // 注音文（我a␣ 是「我ㄇ」）
	MixedLiteral    bool   `json:"mixed_literal"`     // 組不成音節（npm␣）、數字鍵盤夾在字母中間（e2e）照字面
	MixedOverwrite  bool   `json:"mixed_overwrite"`   // 打錯又補對的注音鍵
	SymbolBoard     bool   `json:"symbol_board"`      // Ctrl＋Alt＋, 開符號表
	EnglishRanking  string `json:"english_ranking"`   // frequency（常用詞優先）或 alphabetical（靈犀原本的排法）
}

// DefaultExtras：只換掉官方打不出你要的結果（亂碼注音、反序的「交」）的開，注音文與打錯又補對關。
func DefaultExtras() Extras {
	return Extras{MixedResplit: true, MixedRepeats: true, MixedReversed: true, MixedLiteral: true, SymbolBoard: true, EnglishRanking: "frequency"}
}

// AllExtras 全部開著，0c-cli 的 blend 與 blendall 測試用。
func AllExtras() Extras {
	return Extras{true, true, true, true, true, true, true, "frequency"}
}

// 上下文重看的各條規則（mixed_context 列出開著的）。
const (
	ContextInitial  = "initial"
	ContextRepeats  = "repeats"
	ContextReversed = "reversed"
	ContextZhuyin   = "zhuyin"
	ContextLiteral  = "literal"
)

var contextRules = []string{ContextInitial, ContextRepeats, ContextReversed, ContextZhuyin, ContextLiteral}

// Apply 把開關換成 r 的 fork 欄位；標點（symbols）跟著風格，不動。不給開關的（標示、候選窗 Shift＋Tab、
// 字典詞學習、混打記憶、聲母衝突）一律開著。
func (x Extras) Apply(r Rules) Rules {
	on := func(b bool, v string) string {
		if b {
			return v
		}
		return ""
	}
	r.EnglishRanking = on(x.EnglishRanking == "frequency", "frequency")
	r.EnglishLearning = "dictionary"
	r.SymbolBoard = on(x.SymbolBoard, "asus")
	r.MixedResplit = on(x.MixedResplit, "keys")
	context := []string{ContextInitial}
	for _, c := range []struct {
		on   bool
		name string
	}{{x.MixedRepeats, ContextRepeats}, {x.MixedReversed, ContextReversed}, {x.MixedZhuyinText, ContextZhuyin}, {x.MixedLiteral, ContextLiteral}} {
		if c.on {
			context = append(context, c.name)
		}
	}
	r.MixedContext = strings.Join(context, ",")
	r.VocabMark = "extended"
	r.CandidateBackTab = "shorter"
	r.MixedMemory = "context"
	r.MixedOverwrite = on(x.MixedOverwrite, "neural")
	return r
}

// ContextRule 回報上下文重看的這一條開著沒有。
func (r Rules) ContextRule(name string) bool {
	for c := range strings.SplitSeq(r.MixedContext, ",") {
		if c == name {
			return true
		}
	}
	return false
}

// validContext：空值，或 contextRules 照順序、不重複的子集合。
func validContext(v string) bool {
	if v == "" {
		return true
	}
	at := 0
	for _, name := range strings.Split(v, ",") {
		i := slices.Index(contextRules[at:], name)
		if i < 0 {
			return false
		}
		at += i + 1
	}
	return true
}
