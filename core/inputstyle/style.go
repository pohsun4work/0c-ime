// Package inputstyle defines versioned, serializable input habits. It contains
// no platform I/O; shells and the core share the same immutable preset catalog.
package inputstyle

import "fmt"

const Version = 1

// Rules contains behavior that cannot be expressed by the older settings.
// A zero Rules preserves the existing engine behavior for older callers.
type Rules struct {
	CapsLetter          string `json:"caps_letter"`
	CandidateNavigation string `json:"candidate_navigation"`
	ShiftDelete         string `json:"shift_delete"`
	Up                  string `json:"up"`
	CandidateTab        string `json:"candidate_tab"`
	CandidatePageSize   int    `json:"candidate_page_size"`
	Version             int    `json:"version"`
	Punctuation         string `json:"punctuation"`
	ShiftSpace          string `json:"shift_space"`
	Tab                 string `json:"tab"`
	CandidateSpace      string `json:"candidate_space"`
	CandidateEnter      string `json:"candidate_enter"`
	ShiftLetter         string `json:"shift_letter"`
	// 以下是整合風格加的欄位（fork:"blend"）；空值是靈犀原本的做法，沒有這些欄位的設定檔照舊讀得懂。
	// 風格只存 symbols（標點操作的「微軟半形參考」），其他由全域開關（Extras.Apply）在引擎啟動時填上。
	EnglishRanking  string `json:"english_ranking,omitempty" fork:"blend"`  // frequency：英文補全與拼字修正依語料詞頻排序
	EnglishLearning string `json:"english_learning,omitempty" fork:"blend"` // dictionary：英文字典裡的詞確認 3 次就學會、不限天數
	// microsoft：中文模式的標點照按鍵出半形（全形時照微軟新注音的全形），` 不開符號選單；Ctrl＋標點多 [ ] Shift+;，英文模式也打全形標點
	Symbols string `json:"symbols,omitempty" fork:"blend"`
	// shift_space：舊版整合風格的 Shift＋空白，讀進來換成 shift_space＝fullwidth（混打開著時判成英數的段落轉全形）
	Width string `json:"width,omitempty" fork:"blend"`
	// asus：Ctrl＋Alt＋, 開華碩注音的符號表（鍵盤形狀，一般／數學／特殊三頁）
	SymbolBoard string `json:"symbol_board,omitempty" fork:"blend"`
	// keys：↓／Tab 換詞長可以跨英數，整段換成英文、注音或中文
	MixedResplit string `json:"mixed_resplit,omitempty" fork:"blend"`
	// 每次按鍵後與送出前照上下文重看混打的判斷，逗號列出開著的規則（舊的 rules 是全部）：initial 聲母衝突、
	// repeats 重複鍵、reversed 反序的英文字、zhuyin 注音文、literal 不是音節的注音與數字鍵盤夾在字母中間的英數詞
	MixedContext string `json:"mixed_context,omitempty" fork:"blend"`
	// extended：組字中的標示多 Shift＋Home／End 標到頭尾、↓ 換整段、整段英文按 Enter 加成英文詞
	VocabMark string `json:"vocab_mark,omitempty" fork:"blend"`
	// shorter：候選開著時 Shift＋Tab 是 Tab 的反方向，換到較短的詞長，最短再按回到最長（candidate_tab 是 length 時）
	CandidateBackTab string `json:"candidate_backtab,omitempty" fork:"blend"`
	// context：記住 ↓ 換過的混打解釋（按鍵＋前一個字＋後一個音），下次照上下文決定要不要照記憶換；要開著 mixed_resplit
	MixedMemory string `json:"mixed_memory,omitempty" fork:"blend"`
	// neural：打錯一個注音鍵又補上同一格的鍵時照微軟新注音後按的蓋掉先按的（5jiy6 是「昨」），由神經模型照前文決定換不換，
	// ↓ 清單也列蓋完的字
	MixedOverwrite string `json:"mixed_overwrite,omitempty" fork:"blend"`
}

func (r Rules) Validate() error {
	if r == (Rules{}) {
		return nil
	}
	if r.CandidatePageSize != 9 && r.CandidatePageSize != 10 {
		return fmt.Errorf("invalid candidate page size %d", r.CandidatePageSize)
	}
	if r.Version != Version {
		return fmt.Errorf("unsupported input style rules version %d", r.Version)
	}
	for _, f := range []struct {
		key, value string
		allowed    []string
	}{
		{"caps_letter", r.CapsLetter, []string{"platform", "lower"}},
		{"candidate_navigation", r.CandidateNavigation, []string{"0c", "horizontal"}},
		{"shift_delete", r.ShiftDelete, []string{"0c", "tone"}},
		{"up", r.Up, []string{"0c", "cycle"}},
		{"candidate_tab", r.CandidateTab, []string{"length", "grid", "next"}},
		{"punctuation", r.Punctuation, []string{"0c", "microsoft", "standard", "chewing"}},
		{"shift_space", r.ShiftSpace, []string{"disabled", "fullwidth", "space", "single"}},
		{"tab", r.Tab, []string{"0c", "cycle", "segment"}},
		{"candidate_space", r.CandidateSpace, []string{"page", "select"}},
		{"candidate_enter", r.CandidateEnter, []string{"select", "commit"}},
		{"shift_letter", r.ShiftLetter, []string{"inline", "commit", "lower"}},
		{"english_ranking", r.EnglishRanking, []string{"", "frequency"}},
		{"english_learning", r.EnglishLearning, []string{"", "dictionary"}},
		{"symbols", r.Symbols, []string{"", "microsoft"}},
		{"width", r.Width, []string{"", "shift_space"}},
		{"symbol_board", r.SymbolBoard, []string{"", "asus"}},
		{"mixed_resplit", r.MixedResplit, []string{"", "keys"}},
		{"vocab_mark", r.VocabMark, []string{"", "extended"}},
		{"candidate_backtab", r.CandidateBackTab, []string{"", "shorter"}},
		{"mixed_memory", r.MixedMemory, []string{"", "context"}},
		{"mixed_overwrite", r.MixedOverwrite, []string{"", "neural"}},
	} {
		ok := false
		for _, v := range f.allowed {
			ok = ok || f.value == v
		}
		if !ok {
			return fmt.Errorf("invalid input style %s: %q", f.key, f.value)
		}
	}
	if !validContext(r.MixedContext) {
		return fmt.Errorf("invalid input style mixed_context: %q", r.MixedContext)
	}
	return nil
}

// Behavior is the complete snapshot of the style-managed legacy settings.
// Keeping scalar values allows settings snapshots to remain comparable.
type Behavior struct {
	ToggleKey           string `json:"toggle_key"`
	ModeMemory          string `json:"mode_memory"`
	StartMode           string `json:"start_mode"`
	CapsLockEnglish     bool   `json:"caps_lock_english"`
	EscClear            bool   `json:"esc_clear"`
	SpaceSelects        bool   `json:"space_selects"`
	AdvanceAfterSelect  bool   `json:"advance_after_select"`
	CandidateCursor     string `json:"candidate_cursor"`
	CandidateLayout     string `json:"candidate_layout"`
	SelectionKeys       string `json:"selection_keys"`
	MixedInput          bool   `json:"mixed_input"`
	EnglishAutocomplete bool   `json:"english_autocomplete"`
	EmojiShortcut       string `json:"emoji_shortcut"`
	EmojiEqualsFirst    bool   `json:"emoji_equals_first"`
	CtrlPunctuation     bool   `json:"ctrl_punctuation"`
	Toneless            bool   `json:"toneless"`
}

type Preset struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Rules    Rules    `json:"rules"`
	Behavior Behavior `json:"behavior"`
}

func Get(id string) (Preset, bool) { return GetForPlatform(id, "linux") }

func GetForPlatform(id, goos string) (Preset, bool) {
	b := Behavior{ToggleKey: "right", ModeMemory: "native", StartMode: "zhuyin", EscClear: true, SpaceSelects: true, CandidateCursor: "after", CandidateLayout: "vertical", SelectionKeys: "1234567890", EmojiShortcut: "equals", EmojiEqualsFirst: true, CtrlPunctuation: true}
	if goos == "windows" {
		b.ToggleKey = "both"
	}
	if goos == "darwin" {
		b.ToggleKey = "none"
		b.ModeMemory = "shared"
		b.CandidateCursor = "before"
	}
	r := Rules{Version: Version, CapsLetter: "platform", CandidateNavigation: "0c", ShiftDelete: "0c", Up: "0c", CandidateTab: "length", CandidatePageSize: 10, Punctuation: "0c", ShiftSpace: "disabled", Tab: "0c", CandidateSpace: "page", CandidateEnter: "select", ShiftLetter: "inline"}
	p := Preset{ID: id, Rules: r, Behavior: b}
	switch id {
	case "0c":
		p.Name = "靈犀注音"
	case "blend": // 不列進內建清單：用 0c-update profile copy blend 複製成自訂風格，不鎖設定。混打的開關是全域的（Extras）。
		p.Name = "整合風格"
		p.Behavior.MixedInput = true
		p.Behavior.EnglishAutocomplete = true
		p.Behavior.EmojiShortcut = "off"
		p.Rules.Symbols = "microsoft"
		p.Rules.ShiftSpace = "fullwidth"
	case "microsoft":
		p.Name = "微軟新注音"
		p.Behavior.ToggleKey = "both"
		p.Behavior.EmojiShortcut = "off"
		p.Rules.Punctuation = "microsoft"
		p.Rules.ShiftSpace = "fullwidth"
		p.Rules.Up = "cycle"
		p.Rules.CandidateTab = "grid"
	case "chewing":
		p.Name = "新酷音"
		p.Behavior.CandidateCursor = "before"
		p.Behavior.CandidateLayout = "horizontal"
		p.Behavior.EmojiShortcut = "off"
		p.Rules.Tab = "segment"
		p.Rules.Punctuation = "chewing"
		p.Rules.CandidateNavigation = "horizontal"
		p.Rules.ShiftSpace = "fullwidth"
	case "mcbopomofo":
		p.Name = "小麥注音"
		p.Behavior.ToggleKey = "none"
		p.Behavior.CandidateCursor = "before"
		p.Behavior.CapsLockEnglish = true
		p.Behavior.EscClear = false
		p.Behavior.EmojiShortcut = "off"
		p.Rules.Punctuation = "standard"
		p.Rules.Tab = "cycle"
		p.Rules.ShiftSpace = "space"
		p.Rules.ShiftLetter = "commit"
		p.Rules.CandidatePageSize = 9
		p.Rules.CapsLetter = "lower"
	case "macos":
		p.Name = "macOS 內建注音"
		p.Behavior.ToggleKey = "none"
		p.Behavior.CandidateCursor = "before"
		p.Behavior.CapsLockEnglish = true
		p.Behavior.EmojiShortcut = "off"
		p.Rules.ShiftDelete = "tone"
	default:
		return Preset{}, false
	}
	return p, true
}

func Presets() []Preset { return PresetsForPlatform("linux") }
func PresetsForPlatform(goos string) []Preset {
	var out []Preset
	for _, id := range []string{"0c", "microsoft", "chewing", "mcbopomofo", "macos"} {
		p, _ := GetForPlatform(id, goos)
		out = append(out, p)
	}
	return out
}
