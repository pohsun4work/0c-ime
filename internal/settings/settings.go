// Package settings 是使用者可切換的引擎設定（個人詞庫旁的 settings.json）。
// 引擎只在啟動時讀取；檔案不存在或欄位缺漏時使用預設值。
//
// 檔案前後相容：較新版本寫入本版不認得的欄位、已知欄位的未知值，都不會讓整份設定被拒收或重設，
// 只有讀不懂的那個欄位退回預設；存檔時原文保留，回復到舊版再改任何設定，新版的設定也不會消失。
// 檔案的 schema 比本版新時只讀不寫（ErrReadOnly）。已拿掉的設定（retiredKeys）讀檔時忽略、存檔時不寫回。
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0c-biggo/0c-ime/core/fuzzy"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

const (
	Neural      = "neural"      // 神經模型依前文重排統計引擎的候選（預設）
	Statistical = "statistical" // 只用統計引擎
	FileName    = "settings.json"

	// SchemaVersion 是本版寫入、也完全理解的檔案格式。
	SchemaVersion = 2
)

// 讀取結果的檔案狀態。
const (
	StateOK         = "ok"
	StateMissing    = "missing"
	StateCorrupt    = "corrupt"
	StateUnreadable = "unreadable"
)

// SetError 的代碼，供機器可讀的輸出使用。
const (
	CodeUnknownKey   = "unknown_key"
	CodeInvalidValue = "invalid_value"
)

var (
	// ErrReadOnly：設定檔來自較新版本，本版不覆寫。
	ErrReadOnly = errors.New("設定檔來自較新版本，本版只讀不寫")
	// ErrCorrupt：設定檔不是可解析的 JSON 物件。
	ErrCorrupt = errors.New("設定檔無法解析")
)

type Settings struct {
	Selection           string
	Punctuation         bool // 提交時補標點（Beta）
	DocumentContext     bool
	EnglishAutocomplete bool
	// ToggleKey 是單按哪個 Shift 切換注音／英文（#117）：right、both（預設，#206）、left、none。Shift 加字母一律打大寫，不算切換。
	ToggleKey string
	// ShiftTapMS 是單按 Shift 切換中英的時限（毫秒，#319）：按下到放開在這個時間內才算單按。只接受 ShiftTapChoices。
	ShiftTapMS int
	// ModeMemory 是切換視窗或輸入框後的中英模式（#115）：native 每個程式各自記住，shared 全部共用同一個模式。
	ModeMemory string
	// StartMode 是開始時的中英模式（#160）：zhuyin 或 english。沒有記住的模式時從它開始。
	StartMode          string
	EscClear           bool // 按 Esc 清除整段組字；false 時只清未打完的注音
	AdvanceAfterSelect bool // 從候選窗選字後游標移到所選詞後面（#133）
	CandidateCursor    string
	CandidateLayout    string
	// SelectionKeys 是候選窗的選字鍵（#112）：1234567890、1234qweras 或 asdfghjkl;。換成字母鍵時數字 1–0 仍照位置選字。
	SelectionKeys string
	// PersonalLearning：記住我選的字詞（#297）。關閉時照常選字，只是送出與手選不再寫進個人資料；「我的詞彙」不受影響。
	PersonalLearning bool
	// RedundantKeys：中英混打時同一個注音重複按當成一次（P6，照華碩）。關閉時照官方。
	RedundantKeys bool
	// 以下是按鍵規則（原本官方「輸入習慣」風格的規則，fork 拿掉風格後是一般設定），值見 inputstyle.Rules。
	ShiftSpace          string
	CandidateTab        string
	CandidateSpace      string
	CandidateEnter      string
	PageSize            int
	ShiftDelete         string
	CandidateNavigation string
	Fuzzy               Fuzzy

	// extra 是本版沒有解讀、但存檔要原樣寫回的成員（JSON 物件，鍵已排序）：未知欄位，
	// 以及已知欄位中本版讀不懂的值。用原文字串保存，Settings 才能維持可比較。
	extra string
	// readOnly：檔案的 schema 比本版新，或不是整數，所以不覆寫。
	readOnly bool
}

// Fuzzy 是各組模糊音的啟用方式：off、select（預設，開候選時合併近音）或 auto（另在語境明顯支持近音時自動改字）。
// 欄位缺漏視為預設 select，關閉要明確寫 off；值不認得（包含不是字串的 JSON）時只有那一組當作 off。
// 本版不認得的成員（較新版本新增的配對、已知配對的非字串值、型別不同的整個 fuzzy 欄位）
// 原文保留到下次存檔：較新版本寫入的設定不被舊版沖掉，也不會讓整份設定檔被拒收。
type Fuzzy struct {
	ZZh   string // ㄗ／ㄓ
	CCh   string // ㄘ／ㄔ
	SSh   string // ㄙ／ㄕ
	EnEng string // ㄣ／ㄥ

	extra string // 其他成員的原文（JSON 物件，鍵已排序）；沒有時為空字串
	other string // fuzzy 欄位不是物件時的原文
}

func (f *Fuzzy) field(p fuzzy.Pair) *string {
	switch p {
	case fuzzy.ZZh:
		return &f.ZZh
	case fuzzy.CCh:
		return &f.CCh
	case fuzzy.SSh:
		return &f.SSh
	}
	return &f.EnEng
}

func (f *Fuzzy) UnmarshalJSON(b []byte) error {
	*f = Fuzzy{}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(b, &members); err != nil {
		f.other = string(b) // 型別不同：視為全部關閉，原文保留
		return nil
	}
	for _, p := range fuzzy.Pairs() {
		raw, ok := members[p.ID()]
		var value string
		if ok && json.Unmarshal(raw, &value) == nil {
			*f.field(p) = value
			delete(members, p.ID())
		}
	}
	if len(members) > 0 {
		extra, err := json.Marshal(members)
		if err != nil {
			return err
		}
		f.extra = string(extra)
	}
	return nil
}

func (f Fuzzy) MarshalJSON() ([]byte, error) {
	members := map[string]json.RawMessage{}
	if f.extra != "" {
		if err := json.Unmarshal([]byte(f.extra), &members); err != nil {
			return nil, err
		}
	}
	for _, p := range fuzzy.Pairs() {
		if v := *f.field(p); v != "" {
			raw, _ := json.Marshal(v)
			members[p.ID()] = raw
		}
	}
	if len(members) == 0 && f.other != "" {
		return []byte(f.other), nil
	}
	return json.Marshal(members)
}

// set 修改一組配對；寫入明確的值就蓋掉先前保留的非字串原文。
func (f *Fuzzy) set(p fuzzy.Pair, value string) {
	*f.field(p) = value
	f.other = ""
	if f.extra == "" {
		return
	}
	var members map[string]json.RawMessage
	if json.Unmarshal([]byte(f.extra), &members) != nil {
		return
	}
	delete(members, p.ID())
	f.extra = ""
	if len(members) > 0 {
		if b, err := json.Marshal(members); err == nil {
			f.extra = string(b)
		}
	}
}

// hasMember：這組配對在檔案裡有本版讀不懂的值（保留在 extra），不能當成缺漏套用預設。
func (f Fuzzy) hasMember(p fuzzy.Pair) bool {
	if f.extra == "" {
		return false
	}
	var members map[string]json.RawMessage
	if json.Unmarshal([]byte(f.extra), &members) != nil {
		return false
	}
	_, ok := members[p.ID()]
	return ok
}

// hasUnknown：fuzzy 欄位含本版不認得、存檔時原樣保留的內容。
func (f Fuzzy) hasUnknown() bool { return f.extra != "" || f.other != "" }

// DefaultFuzzy 是沒有設定時各組模糊音的模式。
const DefaultFuzzy = "select"

// FuzzyConfig 轉成核心使用的設定。整個 fuzzy 欄位型別不對時全部關閉。
func (s Settings) FuzzyConfig() fuzzy.Config {
	var c fuzzy.Config
	if s.Fuzzy.other != "" {
		return c
	}
	for _, p := range fuzzy.Pairs() {
		value := *s.Fuzzy.field(p)
		if value == "" && !s.Fuzzy.hasMember(p) {
			value = DefaultFuzzy
		}
		if m, ok := fuzzy.ParseMode(value); ok {
			c = c.With(p, m)
		}
	}
	return c
}

// Rules 是引擎用的按鍵規則。
func (s Settings) Rules() inputstyle.Rules {
	return inputstyle.Rules{ShiftSpace: s.ShiftSpace, CandidateTab: s.CandidateTab, CandidateSpace: s.CandidateSpace,
		CandidateEnter: s.CandidateEnter, CandidatePageSize: s.PageSize, ShiftDelete: s.ShiftDelete, CandidateNavigation: s.CandidateNavigation}
}

// Default 是全新安裝的設定，也是每個欄位缺漏時的值。
func Default() Settings {
	var s Settings
	for _, f := range fields() {
		f.set(&s, f.def)
	}
	return s
}

// 選字鍵（#112）。值就是依序對應第 1–10 項的鍵；不開放自訂。
const (
	SelectDigits = "1234567890"
	SelectQwer   = "1234qweras"
	SelectHome   = "asdfghjkl;"
)

// 中英切換鍵（#117）。預設左右 Shift 都切換（#206；舊用戶的 right 由 AdoptBothShift 換一次）。
const (
	ToggleRight = "right"
	ToggleBoth  = "both"
	ToggleLeft  = "left"
	ToggleNone  = "none"
)

// 單按 Shift 切換中英的時限（#319）：選單的選項與預設，單位毫秒。按得慢的人要長一點，常誤切的人要短一點。
var ShiftTapChoices = []int{200, 300, 400, 500, 600}

const DefaultShiftTapMS = 300

// ShiftTapMillis 是外殼用的單按時限；零值設定也回報預設，外殼不會拿到 0。
func (s Settings) ShiftTapMillis() int {
	if slices.Contains(ShiftTapChoices, s.ShiftTapMS) {
		return s.ShiftTapMS
	}
	return DefaultShiftTapMS
}

// 切換視窗後的中英模式（#115）。
const (
	ModeNative = "native"
	ModeShared = "shared"
)

// 開始時的中英模式（#160）；值與引擎快照的 mode 相同。
const (
	StartZhuyin  = "zhuyin"
	StartEnglish = "english"
)

// StartsEnglish 回報開始時是否為英文模式。不用 Shift 切換時也能按 Ctrl＋空白鍵或點工作列的中／英切回注音。
func (s Settings) StartsEnglish() bool { return s.StartMode == StartEnglish }

// TogglesLeft／TogglesRight 回報單按左／右 Shift 是否切換中英。
func (s Settings) TogglesLeft() bool  { return s.ToggleKey == ToggleBoth || s.ToggleKey == ToggleLeft }
func (s Settings) TogglesRight() bool { return s.ToggleKey == ToggleBoth || s.ToggleKey == ToggleRight }

// field 是設定檔的一個欄位與它的命令列寫法。kind 決定檔案裡的 JSON 型別：
// 開關（JSON 布林，命令列 on／off）、選項（JSON 字串）或數字（JSON 整數）。
type field struct {
	key, json, label, description, def string
	kind                               kind
	values                             []string
	labels                             map[string]string // 選項的顯示名稱；nil 時設定程式用自己的對照表
	get                                func(Settings) string
	set                                func(*Settings, string) // value 已核對過是 values 之一
}

type kind int

const (
	kindSwitch kind = iota
	kindChoice
	kindNumber
)

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func switchField(key, label, description string, def bool, p func(*Settings) *bool) field {
	return field{key: key, json: strings.ReplaceAll(key, "-", "_"), label: label, description: description, def: onOff(def), kind: kindSwitch,
		values: []string{"on", "off"},
		get:    func(s Settings) string { return onOff(*p(&s)) },
		set:    func(s *Settings, v string) { *p(s) = v == "on" }}
}

func choiceField(key, label, description, def string, values []string, labels map[string]string, p func(*Settings) *string) field {
	return field{key: key, json: strings.ReplaceAll(key, "-", "_"), label: label, description: description, def: def, kind: kindChoice,
		values: values, labels: labels,
		get: func(s Settings) string { return *p(&s) },
		set: func(s *Settings, v string) { *p(s) = v }}
}

func numberField(key, label, description string, def int, choices []int, labels map[string]string, p func(*Settings) *int) field {
	values := make([]string, len(choices))
	for i, n := range choices {
		values[i] = strconv.Itoa(n)
	}
	return field{key: key, json: strings.ReplaceAll(key, "-", "_"), label: label, description: description, def: strconv.Itoa(def), kind: kindNumber,
		values: values, labels: labels,
		get: func(s Settings) string { return strconv.Itoa(*p(&s)) },
		set: func(s *Settings, v string) { *p(s), _ = strconv.Atoi(v) }}
}

// fields 依固定順序列出所有欄位：設定檔的寫出順序、Items 與命令列都照這個順序。模糊音另見 fuzzyItems。
func fields() []field {
	tapLabels := map[string]string{}
	for _, ms := range ShiftTapChoices {
		tapLabels[strconv.Itoa(ms)] = strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64) + " 秒"
	}
	return []field{
		choiceField("selection", "選字方式", "神經選字會依前文重新排序候選，需要載入模型；只用統計引擎則完全依詞頻與個人詞庫，不載入模型",
			Neural, []string{Neural, Statistical}, nil, func(s *Settings) *string { return &s.Selection }),
		switchField("punctuation", "提交時補標點（Beta）", "提交時依句意補上標點，可能漏補或錯插；預設關閉，可自行開啟試用",
			false, func(s *Settings) *bool { return &s.Punctuation }),
		switchField("document-context", "依前文選字", "依自己剛提交的文字選字",
			true, func(s *Settings) *bool { return &s.DocumentContext }),
		switchField("english-autocomplete", "英文模式自動補字", englishAutocompleteDescription,
			false, func(s *Settings) *bool { return &s.EnglishAutocomplete }),
		choiceField("toggle-key", "中英切換鍵", toggleKeyDescription, ToggleBoth, []string{ToggleRight, ToggleBoth, ToggleLeft, ToggleNone},
			map[string]string{ToggleRight: "右 Shift", ToggleBoth: "左右 Shift", ToggleLeft: "左 Shift", ToggleNone: "不用 Shift"},
			func(s *Settings) *string { return &s.ToggleKey }),
		numberField("shift-tap-ms", shiftTapLabel, shiftTapDescription, DefaultShiftTapMS, ShiftTapChoices, tapLabels,
			func(s *Settings) *int { return &s.ShiftTapMS }),
		choiceField("start-mode", "開始時的中英模式", "新開的程式從這個模式開始；切換視窗後「全部共用同一個模式」時，是登入後一開始的模式",
			StartZhuyin, []string{StartZhuyin, StartEnglish}, map[string]string{StartZhuyin: "注音", StartEnglish: "英文"},
			func(s *Settings) *string { return &s.StartMode }),
		choiceField("mode-memory", "切換視窗後的中英模式", "換到別的程式或輸入框時，中英模式要不要跟著",
			ModeNative, []string{ModeNative, ModeShared}, map[string]string{ModeNative: "每個程式各自記住", ModeShared: "全部共用同一個模式"},
			func(s *Settings) *string { return &s.ModeMemory }),
		switchField("esc-clear", "按 Esc 清除整段組字", "關閉時 Esc 只清除還沒打完的注音，已組好的字保留",
			true, func(s *Settings) *bool { return &s.EscClear }),
		choiceField("candidate-cursor", "修改字詞的位置", "按 ↓ 開候選時，修改游標前方或後方的字詞；橫排時前方是左側，後方是右側。游標在句尾時後方沒有字，一律修改前方的詞",
			"after", []string{"before", "after"}, nil, func(s *Settings) *string { return &s.CandidateCursor }),
		switchField("advance-after-select", "選字後游標移到下一個字", "從候選窗選好字後，游標跳到所選的詞後面，可以接著按 ↓ 改下一個字；修改游標前方字詞時往後移一個字。游標在句尾時不動",
			false, func(s *Settings) *bool { return &s.AdvanceAfterSelect }),
		choiceField("candidate-layout", "選字排列", "直排，或每排五個的多排選字（選字鍵 1234qweras 時每排四個）",
			"vertical", []string{"vertical", "grid", "horizontal"}, map[string]string{"vertical": "直排", "grid": "多排", "horizontal": "橫排"},
			func(s *Settings) *string { return &s.CandidateLayout }),
		choiceField("selection-keys", "選字鍵", "候選窗用哪一排鍵選字（每頁 9 個時用前 9 個鍵）；換成字母鍵時，數字鍵仍可照位置選字",
			SelectDigits, []string{SelectDigits, SelectQwer, SelectHome},
			map[string]string{SelectDigits: "1 2 3 4 5 6 7 8 9 0", SelectQwer: "1 2 3 4 q w e r a s", SelectHome: "a s d f g h j k l ;"},
			func(s *Settings) *string { return &s.SelectionKeys }),
		switchField("personal-learning", "記住我選的字詞", personalLearningDescription,
			true, func(s *Settings) *bool { return &s.PersonalLearning }),
		switchField("redundant-keys", "重複按的注音當成一次", redundantKeysDescription,
			true, func(s *Settings) *bool { return &s.RedundantKeys }),
		choiceField("shift-space", "Shift＋空白", "切換全半形時也包含英文與數字",
			inputstyle.ShiftSpaceDisabled, []string{inputstyle.ShiftSpaceDisabled, inputstyle.ShiftSpaceFullWidth, inputstyle.ShiftSpaceSpace, inputstyle.ShiftSpaceSingle},
			map[string]string{inputstyle.ShiftSpaceDisabled: "不使用", inputstyle.ShiftSpaceFullWidth: "切換全半形", inputstyle.ShiftSpaceSpace: "插入空白", inputstyle.ShiftSpaceSingle: "開單字候選"},
			func(s *Settings) *string { return &s.ShiftSpace }),
		choiceField("candidate-tab", "候選窗的 Tab", "已顯示候選字時的 Tab 操作",
			inputstyle.CandidateTabLength, []string{inputstyle.CandidateTabLength, inputstyle.CandidateTabGrid, inputstyle.CandidateTabNext},
			map[string]string{inputstyle.CandidateTabLength: "切換字詞長度", inputstyle.CandidateTabGrid: "展開多排候選", inputstyle.CandidateTabNext: "移到下一個候選"},
			func(s *Settings) *string { return &s.CandidateTab }),
		choiceField("candidate-space", "候選窗的空白鍵", "候選窗開啟時按空白鍵",
			inputstyle.CandidateSpacePage, []string{inputstyle.CandidateSpacePage, inputstyle.CandidateSpaceSelect},
			map[string]string{inputstyle.CandidateSpacePage: "下一頁", inputstyle.CandidateSpaceSelect: "選取候選字"},
			func(s *Settings) *string { return &s.CandidateSpace }),
		choiceField("candidate-enter", "候選窗的 Enter", "確認候選字後是否同時送出整段組字",
			inputstyle.CandidateEnterSelect, []string{inputstyle.CandidateEnterSelect, inputstyle.CandidateEnterCommit},
			map[string]string{inputstyle.CandidateEnterSelect: "選字並保留組字", inputstyle.CandidateEnterCommit: "選字並送出"},
			func(s *Settings) *string { return &s.CandidateEnter }),
		numberField("page-size", "每頁候選數", "每頁顯示幾個候選字詞", 9, []int{9, 10}, nil,
			func(s *Settings) *int { return &s.PageSize }),
		choiceField("shift-delete", "Shift＋Delete", "沒開候選窗時按 Shift＋Delete：同 Delete（反白學到的英文建議時是忘掉它），或在注音沒打完時只退回聲調",
			inputstyle.ShiftDeleteDelete, []string{inputstyle.ShiftDeleteDelete, inputstyle.ShiftDeleteTone},
			map[string]string{inputstyle.ShiftDeleteDelete: "同 Delete", inputstyle.ShiftDeleteTone: "退回聲調"},
			func(s *Settings) *string { return &s.ShiftDelete }),
		choiceField("candidate-navigation", "候選方向鍵", "水平操作為左右選候選、上下翻頁；排列外觀可另選",
			inputstyle.NavigationLayout, []string{inputstyle.NavigationLayout, inputstyle.NavigationHorizontal},
			map[string]string{inputstyle.NavigationLayout: "依候選排列", inputstyle.NavigationHorizontal: "水平操作"},
			func(s *Settings) *string { return &s.CandidateNavigation }),
	}
}

// fuzzyItems 是各組模糊音的命令列鍵（fuzzy-z-zh…），排在 fields 後面。
func fuzzyItems() []field {
	var out []field
	for _, p := range fuzzy.Pairs() {
		out = append(out, field{key: "fuzzy-" + p.ID(), label: "模糊音 " + p.Symbols(),
			description: "把近音的字也列進候選，原音仍排在前面；「自動修正」（實驗）會在上下文明顯支持近音時另外自動改字",
			def:         DefaultFuzzy, values: []string{"off", "select", "auto"},
			get: func(s Settings) string { return s.FuzzyConfig().Mode(p).String() },
			set: func(s *Settings, v string) {
				if v == DefaultFuzzy {
					v = "" // 回到預設：不在設定檔留下欄位；off 要明確寫入
				}
				s.Fuzzy.set(p, v)
			}})
	}
	return out
}

func lookup(key string) (field, bool) {
	for _, f := range append(fields(), fuzzyItems()...) {
		if f.key == key {
			return f, true
		}
	}
	return field{}, false
}

// decode 把檔案裡的值換成命令列寫法；型別不對或不是選項之一時回傳 false。
func (f field) decode(raw json.RawMessage) (string, bool) {
	var value string
	switch f.kind {
	case kindSwitch:
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return "", false
		}
		value = onOff(v)
	case kindChoice:
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
	case kindNumber:
		var n int
		if json.Unmarshal(raw, &n) != nil {
			return "", false
		}
		value = strconv.Itoa(n)
	}
	return value, slices.Contains(f.values, value)
}

// encode 是命令列寫法在檔案裡的 JSON。
func (f field) encode(value string) json.RawMessage {
	switch f.kind {
	case kindSwitch:
		return json.RawMessage(strconv.FormatBool(value == "on"))
	case kindNumber:
		return json.RawMessage(value)
	}
	raw, _ := json.Marshal(value)
	return raw
}

// retiredKeys 是已拿掉的設定檔欄位：讀檔時忽略，存檔時不寫回。
// preview_0_2 是 0.2.0 開發期間的內部開關（#128）；其餘是 fork 只留一套行為時拿掉的設定（官方的輸入習慣風格 input_profile、
// 寫死成固定行為的中英混打、標點寬度、Ctrl＋標點、鍵盤排列、Emoji 選單、組字時的 Tab（固定換成小視窗那一個，↑ 輪換字詞），以及拿掉的功能）。
var retiredKeys = []string{"preview_0_2", "input_profile", "left_shift", "mixed_input", "space_selects", "emoji_shortcut", "emoji_equals_first",
	"keyboard_layout", "output_simplified", "simplified_hotkey", "ctrl_punctuation", "shift_symbol_halfwidth", "all_symbols_halfwidth",
	"cjk_latin_space", "zhuyin_error_hint", "caps_lock_english", "toneless", "tab"}

// UnmarshalJSON 逐欄位解讀：讀不懂的欄位退回預設，原文放進 extra 等存檔時寫回。
func (s *Settings) UnmarshalJSON(b []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(b, &members); err != nil {
		return err
	}
	*s = Default()
	kept := map[string]json.RawMessage{}
	for _, f := range fields() {
		raw, ok := members[f.json]
		if !ok {
			continue
		}
		delete(members, f.json)
		if value, ok := f.decode(raw); ok {
			f.set(s, value)
		} else {
			kept[f.json] = raw
		}
	}
	for _, key := range retiredKeys {
		delete(members, key) // 已拿掉的鍵：不論值為何都不保留，下次存檔就從檔案消失
	}
	if raw, ok := members["fuzzy"]; ok {
		delete(members, "fuzzy")
		if err := json.Unmarshal(raw, &s.Fuzzy); err != nil {
			return err
		}
	}
	if raw, ok := members["schema"]; ok {
		delete(members, "schema")
		var n int
		if json.Unmarshal(raw, &n) != nil || n > SchemaVersion {
			s.readOnly = true // 比本版新，或本版讀不懂的格式標記：不覆寫
		}
	}
	for key, raw := range members {
		kept[key] = raw
	}
	if len(kept) > 0 {
		extra, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		s.extra = string(extra)
	}
	return nil
}

// MarshalJSON 依固定順序寫出已知欄位，其後接保留的原文成員。
func (s Settings) MarshalJSON() ([]byte, error) {
	kept := s.kept()
	var buf bytes.Buffer
	buf.WriteByte('{')
	put := func(key string, raw []byte) {
		if buf.Len() > 1 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(raw)
	}
	put("schema", []byte(fmt.Sprint(SchemaVersion)))
	for _, f := range fields() {
		if raw, ok := kept[f.json]; ok { // 本版讀不懂的值：原文寫回，直到使用者明確改它
			delete(kept, f.json)
			put(f.json, raw)
			continue
		}
		put(f.json, f.encode(f.get(s)))
	}
	if s.Fuzzy != (Fuzzy{}) {
		raw, err := json.Marshal(s.Fuzzy)
		if err != nil {
			return nil, err
		}
		put("fuzzy", raw)
	}
	names := make([]string, 0, len(kept))
	for key := range kept {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		put(key, kept[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// kept 是 extra 解開後的成員；沒有時為空 map。
func (s Settings) kept() map[string]json.RawMessage {
	kept := map[string]json.RawMessage{}
	if s.extra != "" {
		_ = json.Unmarshal([]byte(s.extra), &kept)
	}
	return kept
}

// forget 丟掉某個欄位保留的原文：使用者明確設定它之後，不再寫回舊值。
func (s *Settings) forget(key string) {
	kept := s.kept()
	if _, ok := kept[key]; !ok {
		return
	}
	delete(kept, key)
	s.extra = ""
	if len(kept) > 0 {
		if b, err := json.Marshal(kept); err == nil {
			s.extra = string(b)
		}
	}
}

// ReadOnly 回報檔案是否來自較新版本：是的話 Save 會拒絕。
func (s Settings) ReadOnly() bool { return s.readOnly }

// notes 說明讀取時的退回與保留，給人讀（settings --json 的 notes）。
func (s Settings) notes() []string {
	var out []string
	kept := s.kept()
	for _, f := range fields() {
		if raw, ok := kept[f.json]; ok {
			out = append(out, fmt.Sprintf("%s：本版不認得 %s，暫用預設值，存檔時保留原值", f.key, raw))
			delete(kept, f.json)
		}
	}
	if len(kept) > 0 {
		names := make([]string, 0, len(kept))
		for key := range kept {
			names = append(names, key)
		}
		sort.Strings(names)
		out = append(out, "含本版不認得的欄位："+strings.Join(names, "、")+"；存檔時原樣保留")
	}
	if s.Fuzzy.hasUnknown() {
		out = append(out, "fuzzy：含本版不認得的成員，存檔時原樣保留")
	}
	if s.readOnly {
		out = append(out, ErrReadOnly.Error())
	}
	return out
}

// Report 是讀取設定檔的完整結果：除了設定值，還有檔案狀態與讀取時的退回說明。
type Report struct {
	Path     string // 實際讀取的路徑
	Settings Settings
	State    string   // StateOK、StateMissing、StateCorrupt、StateUnreadable
	Schema   int      // 檔案中的 schema；沒有時為 0
	ReadOnly bool     // 檔案來自較新版本：Save 會拒絕
	Notes    []string // 讀不懂而退回預設、或原樣保留的欄位
}

// readFile 區分真正缺檔與已存在但目標遺失的符號連結；後者是讀不到，不是缺檔。
func readFile(path string) (b []byte, missing bool, err error) {
	b, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		_, statErr := os.Lstat(path)
		if errors.Is(statErr, fs.ErrNotExist) {
			return nil, true, nil
		}
		if statErr != nil {
			return nil, false, statErr
		}
	}
	return b, false, err
}

// Read 讀取設定；檔案不存在時回傳預設值。個別欄位讀不懂只有那個欄位退回預設，不算錯誤。
// 只有檔案讀不到或根本不是 JSON 物件才回錯誤（同時回傳預設值與對應的 State）。
func Read(path string) (Report, error) {
	b, missing, err := readFile(path)
	if missing {
		return Report{Path: path, Settings: Default(), State: StateMissing}, nil
	}
	if err != nil {
		return Report{Path: path, Settings: Default(), State: StateUnreadable}, err
	}
	var s Settings
	if err = json.Unmarshal(b, &s); err != nil {
		return Report{Path: path, Settings: Default(), State: StateCorrupt}, fmt.Errorf("%s：%w：%v", path, ErrCorrupt, err)
	}
	r := Report{Path: path, Settings: s, State: StateOK, ReadOnly: s.readOnly, Notes: s.notes()}
	var head struct {
		Schema int `json:"schema"`
	}
	_ = json.Unmarshal(b, &head) // schema 不是整數時保持 0；ReadOnly 已另外標記
	r.Schema = head.Schema
	return r, nil
}

// Load 讀取設定；檔案不存在時回傳預設值。讀不懂的個別欄位已退回預設，不算錯誤；
// 檔案不是可解析的 JSON 物件時回傳預設值與錯誤，呼叫端應回退預設值並記錄。
func Load(path string) (Settings, error) {
	r, err := Read(path)
	return r.Settings, err
}

// Save 先寫暫存檔再改名，中斷時保留原設定。讀到的未知欄位與讀不懂的值原樣寫回；
// 來自較新版本的檔案回傳 ErrReadOnly，不覆寫。
func Save(path string, s Settings) error {
	if s.readOnly {
		return ErrReadOnly
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Quarantine 把無法解析的設定檔改名保留（settings.json.corrupt-時間），回傳新路徑；
// 之後 Read 會視為檔案不存在，使用者可以從預設值重新設定而不遺失原檔。
func Quarantine(path string, now time.Time) (string, error) {
	dst := path + ".corrupt-" + now.UTC().Format("20060102T150405Z")
	return dst, os.Rename(path, dst)
}

// SetError 是 Set 的錯誤：Code 供機器判斷（CodeUnknownKey、CodeInvalidValue），Msg 給人讀。
type SetError struct {
	Code string
	Key  string
	Msg  string
}

func (e *SetError) Error() string { return e.Msg }

// disabledReason 回報某一項目前是否因其他設定而不起作用，以及給人讀的原因；空字串表示可用。
func (s Settings) disabledReason(key string) string {
	if key == "shift-tap-ms" && s.ToggleKey == ToggleNone {
		return shiftTapUnusedReason
	}
	return ""
}

// shiftTapUnusedReason：不用 Shift 切換時，單按 Shift 的時限沒有作用（#319）。
const shiftTapUnusedReason = "不用 Shift 切換時不作用"

// disabledBy 回報停用原因來自哪一項設定（Item.DisabledBy）：設定程式把各項分在不同頁面（#207），
// 原因那一項在別頁時，用它帶人過去改。新增停用原因時要一起加在這裡（TestDisabledItemsNameTheirCause）。
func disabledBy(reason string) string {
	if reason == shiftTapUnusedReason {
		return "toggle-key"
	}
	return ""
}

const englishAutocompleteDescription = "單按 Shift 切換到英文模式後，輸入至少兩個字母時顯示補全候選，三個字母起也提供拼字修正；按 Tab 補全或修正拼字、Esc 關閉。" +
	"「我的詞彙」的英文詞排在最前面；自己打過幾次的英文單字會記在本機，之後也會建議（私人欄位與關閉「記住我選的字詞」時不記）。" +
	"空白送出單字並輸入空格；Enter 只送出單字，再按一次才換行或執行"

const redundantKeysDescription = "同一個注音鍵多按了幾下，相不相鄰都當成按一次：wuuuu/6、wu//6、wuwu/6 是「停」，ggggk6 是「什」。" +
	"關閉時多按的鍵照字面留下（wuuuu/6 是 wuuu營）"

// personalLearningDescription 是「記住我選的字詞」（#297）的說明：關閉時什麼照常、什麼不記、已學的怎麼清。
const personalLearningDescription = "選過的字詞會排到前面，常用的英文單字也會記住；學習資料只存在這台電腦。" +
	"關閉後照常選字（神經選字、依前文選字照舊，已經學到的也照常使用），只是不再記下新的選擇和英文單字；" +
	"「我的詞彙」照常可以加詞。要清掉已經學到的，到「我的詞彙」的「靈犀學到的」按「清除學習紀錄」"

// toggleKeyDescription：時限可調（#319），這裡不寫秒數，指向那一項，免得說明跟設定不一致。
// Ctrl＋空白鍵是繁中輸入法的系統熱鍵，外殼接到就切換（#117），不受這一項影響。
const toggleKeyDescription = "單按哪個 Shift 切換注音／英文；按住 Shift 超過「" + shiftTapLabel + "」、或 Shift 加字母（打大寫）不算切換。沒用來切換的 Shift 照系統原本的用途。" +
	"預設左右 Shift 都可以切換，不想讓左 Shift 切換可改成「右 Shift」；按著 Shift 點滑鼠選取也不算切換。也可以按 Ctrl＋空白鍵"

// 「單按 Shift 的時限」（#319）的名稱與說明。
const (
	shiftTapLabel       = "單按 Shift 的時限"
	shiftTapDescription = "按下 Shift 到放開在這個時間內才算單按，切換注音／英文。按得比較慢、常切換不成功時調長；打字時常不小心切到英文時調短"
)

// Keys 列出可用 Set 修改的項目與允許值，供命令列說明使用。
func Keys() map[string]string {
	keys := map[string]string{}
	for _, f := range append(fields(), fuzzyItems()...) {
		keys[f.key] = strings.Join(f.values, "|")
	}
	return keys
}

// Item 描述一個可修改的設定及其目前的值，是 settings --json 契約的一項。
type Item struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"` // 目前都是 choice：從 Values 擇一
	Values      []string `json:"values"`
	Default     string   `json:"default"`
	Value       string   `json:"value"` // 設定檔中的值（讀不懂的值顯示為預設值）
	Label       string   `json:"label"`
	Description string   `json:"description"`
	// Disabled：這一項目前因其他設定而不起作用（例如不用 Shift 切換時的單按時限），介面顯示為不能改，
	// 旁邊顯示 DisabledReason。Value 仍是設定檔中的值，條件解除後照舊生效。
	Disabled       bool   `json:"disabled,omitempty"`
	DisabledReason string `json:"disabled_reason,omitempty"`
	// DisabledBy：造成停用的那一項的 key。
	DisabledBy string `json:"disabled_by,omitempty"`
	// ValueLabels 是選項的顯示名稱；沒有時設定程式照自己的對照表。
	ValueLabels map[string]string `json:"value_labels,omitempty"`
}

// Items 依固定順序列出所有可修改的設定。
func (s Settings) Items() []Item {
	list := append(fields(), fuzzyItems()...)
	out := make([]Item, 0, len(list))
	for _, f := range list {
		reason := s.disabledReason(f.key)
		out = append(out, Item{Key: f.key, Type: "choice", Values: f.values, Default: f.def, Value: f.get(s), Label: f.label, Description: f.description,
			Disabled: reason != "", DisabledReason: reason, DisabledBy: disabledBy(reason), ValueLabels: f.labels})
	}
	return out
}

// HealthInfo 是引擎 health 回報的啟動配置：鍵是設定檔的欄位名（選字方式是 selection_setting），開關寫 true／false，
// 模糊音合成一項 fuzzy（核心設定的 Key）。補標點不在這裡：引擎另外回報模型狀態 punctuation_state。
// 外殼讀其中幾項（toggle_key、shift_space、start_mode…）；0c-update 用 RunningValues 換回命令列寫法跟設定檔比較。
func (s Settings) HealthInfo() map[string]string {
	info := map[string]string{"fuzzy": s.FuzzyConfig().Key()}
	for _, f := range fields() {
		if key, ok := healthKey(f); ok {
			info[key] = f.health(f.get(s))
		}
	}
	return info
}

// RunningValues 把引擎 health 回報的啟動配置換成命令列寫法（Item.Key → 值）；沒有回報的項目不出現。
func RunningValues(info map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range fields() {
		key, ok := healthKey(f)
		if !ok {
			continue
		}
		raw, found := info[key]
		if !found {
			continue
		}
		value := raw
		if f.kind == kindSwitch {
			value = map[string]string{"true": "on", "false": "off"}[raw]
		}
		if value != "" {
			out[f.key] = value
		}
	}
	// 補標點只有模型狀態：依設定關閉時為 disabled_by_setting，其他狀態都表示設定為開。
	if v := info["punctuation_state"]; v != "" {
		out["punctuation"] = onOff(v != "disabled_by_setting")
	}
	if key, ok := info["fuzzy"]; ok { // 欄位存在但為空字串：全部關閉
		on := map[string]string{}
		for _, part := range strings.Split(key, ",") {
			if id, mode, found := strings.Cut(part, "="); found {
				on[id] = mode
			}
		}
		for _, p := range fuzzy.Pairs() {
			mode := "off"
			if m, found := on[p.ID()]; found {
				mode = m
			}
			out["fuzzy-"+p.ID()] = mode
		}
	}
	return out
}

// healthKey 是欄位在 health 的鍵；補標點沒有（見 HealthInfo）。
func healthKey(f field) (string, bool) {
	switch f.key {
	case "punctuation":
		return "", false
	case "selection":
		return "selection_setting", true
	}
	return f.json, true
}

// health 是命令列寫法在 health 裡的寫法：開關是 true／false，其他照原值。
func (f field) health(value string) string {
	if f.kind == kindSwitch {
		return strconv.FormatBool(value == "on")
	}
	return value
}

// Value 回傳某個設定目前的值（命令列的寫法）。
func (s Settings) Value(key string) (string, bool) {
	f, ok := lookup(key)
	if !ok {
		return "", false
	}
	return f.get(s), true
}

// Set 依命令列的鍵與值修改設定；錯誤為 *SetError。明確設定某個欄位會丟掉先前為它保留的原文。
func (s *Settings) Set(key, value string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	f, ok := lookup(key)
	if !ok {
		names := slices.Sorted(maps.Keys(Keys()))
		return &SetError{Code: CodeUnknownKey, Key: key, Msg: fmt.Sprintf("未知的設定 %q；可用：%s", key, strings.Join(names, "、"))}
	}
	if !slices.Contains(f.values, value) {
		return &SetError{Code: CodeInvalidValue, Key: key, Msg: fmt.Sprintf("%s 只接受 %s", key, strings.Join(f.values, " 或 "))}
	}
	f.set(s, value)
	if f.json != "" {
		s.forget(f.json)
	}
	return nil
}
