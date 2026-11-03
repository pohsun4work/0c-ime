package settings

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

// blendSpec 是整合風格的一個全域開關（設定檔的 blend，inputstyle.Extras）：不跟著輸入習慣，選哪個風格都照它。
// mixed 的只在中英混打作用時有效。
type blendSpec struct {
	key, label, description string
	values                  []string
	labels                  map[string]string
	mixed                   bool
	get                     func(inputstyle.Extras) string
	set                     func(*inputstyle.Extras, string)
}

func blendSwitch(key, label, description string, mixed bool, field func(*inputstyle.Extras) *bool) blendSpec {
	return blendSpec{key, label, description, []string{"on", "off"}, nil, mixed,
		func(x inputstyle.Extras) string { return onOff(*field(&x)) },
		func(x *inputstyle.Extras, v string) { *field(x) = v == "on" }}
}

func blendSpecs() []blendSpec {
	return []blendSpec{
		blendSwitch("mixed-resplit", "↓／Tab 跨過英數換整段", "↓ 可把幾個字一起換成英文、注音或中文，Tab 換詞長可跨過英數；↓ 換過的會記住，下次照前後文換",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedResplit }),
		blendSwitch("mixed-repeats", "自動刪除冗餘注音", "同一個字母多按幾次再打聲調時合成一個字：ggggk6 是「什」",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedRepeats }),
		blendSwitch("mixed-reversed-english", "反序打的英文字", "照注音順序打不出來的英文字加空白時留英文：url 加空白是 url，不是「交」",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedReversed }),
		blendSwitch("mixed-zhuyin-text", "注音文", "中文後面的 a、2、1 加空白打成 ㄇ、ㄉ、ㄅ：我 a 空白是「我ㄇ」",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedZhuyinText }),
		blendSwitch("mixed-literal", "組不成字的按鍵照字面", "不是國語音節的注音加空白留英數：cpu 加空白是 cpu；數字鍵盤夾在字母中間的也是（b2c）",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedLiteral }),
		blendSwitch("mixed-overwrite", "打錯又補對的注音鍵", "打錯一個注音鍵又補上同一格的鍵時，後按的蓋掉先按的（5jiy6 是「昨」），由神經選字依前文決定",
			true, func(x *inputstyle.Extras) *bool { return &x.MixedOverwrite }),
		{"english-ranking", "英文補全排序", "英文補全與拼字修正的候選順序", []string{"frequency", "alphabetical"},
			map[string]string{"frequency": "常用詞優先", "alphabetical": "字母序"}, false,
			func(x inputstyle.Extras) string { return x.EnglishRanking },
			func(x *inputstyle.Extras, v string) { x.EnglishRanking = v }},
		blendSwitch("symbol-board", "Ctrl＋Alt＋, 開符號表", "注音模式按 Ctrl＋Alt＋, 開鍵盤形狀的符號表（一般、數學、特殊三頁），按對應的鍵輸入",
			false, func(x *inputstyle.Extras) *bool { return &x.SymbolBoard }),
	}
}

// BlendSetting 回報 key 是整合風格的全域開關。
func BlendSetting(key string) bool {
	_, ok := lookupBlend(key)
	return ok
}

func lookupBlend(key string) (blendSpec, bool) {
	for _, sp := range blendSpecs() {
		if sp.key == key {
			return sp, true
		}
	}
	return blendSpec{}, false
}

func blendSettingSpecs() []spec {
	var out []spec
	def := inputstyle.DefaultExtras()
	for _, b := range blendSpecs() {
		out = append(out, spec{b.key, b.label, b.description, b.get(def), b.values, func(s Settings) string { return b.get(s.Blend) }})
	}
	return out
}

const blendMixedOff = "沒開中英混打時關閉"

// blendDisabledReason：混打的開關跟著中英混打停用，停用原因照中英混打的（倚天 41 鍵、不打聲調…）。
func (s Settings) blendDisabledReason(goos, key string) string {
	b, ok := lookupBlend(key)
	switch {
	case !ok || !b.mixed:
		return ""
	case !s.MixedInput:
		return blendMixedOff
	}
	return s.disabledReasonOn(goos, "mixed-input")
}

func blendDisabledBy(reason string) string {
	if reason == blendMixedOff {
		return "mixed-input"
	}
	return ""
}

// loadBlend 讀設定檔的 blend（ok 是檔案有這一項）；讀不懂時回傳 false，原文留到存檔。沒有這一項時從舊版的整合風格
// （風格規則裡的 fork 欄位）搬過來。風格裡的 fork 欄位一律拿掉，只留 symbols；舊的 width 換成 Shift＋空白切換全半形。
func (s *Settings) loadBlend(raw json.RawMessage, ok bool) bool {
	x, readable := inputstyle.DefaultExtras(), true
	if ok {
		known := json.Unmarshal(raw, &x) == nil && (x.EnglishRanking == "frequency" || x.EnglishRanking == "alphabetical")
		if !known {
			x = inputstyle.DefaultExtras()
		}
		strict := json.NewDecoder(bytes.NewReader(raw))
		strict.DisallowUnknownFields()
		readable = known && strict.Decode(&inputstyle.Extras{}) == nil // 新版多的開關：認得的照讀，原文留到使用者改它
	}
	s.Blend = x
	if s.profiles == "" || s.readOnly || s.profileInvalid {
		return readable
	}
	st := s.store()
	var legacy inputstyle.Extras
	found, changed := false, false
	for i, p := range st.Items {
		if from, has := legacyExtras(p.Rules); has && (!found || p.ID == st.Active) {
			legacy, found = from, true
		}
		st.Items[i].Rules = profileRules(p.Rules)
		changed = changed || st.Items[i].Rules != p.Rules
	}
	if found && !ok {
		s.Blend = legacy
	}
	if changed {
		s.putStore(st)
	}
	return readable
}

// profileRules 拿掉風格規則裡 symbols 以外的 fork 欄位（改由全域開關管），舊的 width 換成 Shift＋空白切換全半形。
// 標點操作改過、不是靈犀的也拿掉 symbols：照使用者選的標點，不顯示成微軟半形參考。
func profileRules(r inputstyle.Rules) inputstyle.Rules {
	if r.Width == "shift_space" {
		r.ShiftSpace = "fullwidth"
	}
	if r.Punctuation != "0c" {
		r.Symbols = ""
	}
	v := reflect.ValueOf(&r).Elem()
	for _, f := range reflect.VisibleFields(v.Type()) {
		if f.Tag.Get("fork") != "" && f.Name != "Symbols" {
			v.FieldByIndex(f.Index).SetZero()
		}
	}
	return r
}

// legacyExtras：舊版整合風格存在風格規則裡的開關。
func legacyExtras(r inputstyle.Rules) (inputstyle.Extras, bool) {
	if r.MixedResplit == "" && r.MixedContext == "" && r.MixedOverwrite == "" && r.SymbolBoard == "" && r.EnglishRanking == "" {
		return inputstyle.Extras{}, false
	}
	x := inputstyle.Extras{
		MixedResplit: r.MixedResplit != "", MixedOverwrite: r.MixedOverwrite != "", SymbolBoard: r.SymbolBoard != "",
		MixedRepeats: r.ContextRule(inputstyle.ContextRepeats), MixedReversed: r.ContextRule(inputstyle.ContextReversed),
		MixedZhuyinText: r.ContextRule(inputstyle.ContextZhuyin), MixedLiteral: r.ContextRule(inputstyle.ContextLiteral),
		EnglishRanking: "alphabetical",
	}
	if r.EnglishRanking != "" {
		x.EnglishRanking = "frequency"
	}
	return x, true
}

// EngineRules 是引擎用的操作規則：風格的規則加上整合風格的全域開關。
func (s Settings) EngineRules() inputstyle.Rules { return s.Blend.Apply(s.ProfileRules()) }

// BlendWidth 是 health 的 input_style_width：Shift＋空白切換全半形、中英混打作用時是 shift_space，外殼照整合風格的
// 全形把可見字元、空白與數字鍵盤都交給引擎（混打照常判斷，判成英數的段落轉全形）；其他時候照官方的全形。
func (s Settings) BlendWidth() string {
	if s.ProfileRules().ShiftSpace == "fullwidth" && s.MixedInput && s.disabledReason("mixed-input") == "" {
		return "shift_space"
	}
	return ""
}

// BlendKey 是 health 的 blend：引擎啟動時的全域開關，0c-update 用 BlendRunning 換回設定值跟設定檔比較。
func (s Settings) BlendKey() string {
	b, _ := json.Marshal(s.Blend)
	return string(b)
}

// BlendRunning 把 health 的 blend 換成設定的命令列寫法；舊引擎沒有這一項時是空的。
func BlendRunning(key string) map[string]string {
	out := map[string]string{}
	var x inputstyle.Extras
	if key == "" || json.Unmarshal([]byte(key), &x) != nil {
		return out
	}
	for _, b := range blendSpecs() {
		out[b.key] = b.get(x)
	}
	return out
}

// 標點操作的「微軟半形參考」是整合風格的 symbols（標點照靈犀的規則，再照微軟出半形），不是另一種 punctuation。
func punctuationRule(r inputstyle.Rules) string {
	if r.Punctuation == "0c" && r.Symbols == "microsoft" {
		return "microsoft-halfwidth"
	}
	return r.Punctuation
}

func setPunctuationRule(r *inputstyle.Rules, v string) {
	r.Symbols = ""
	if v == "microsoft-halfwidth" {
		r.Punctuation, r.Symbols = "0c", "microsoft"
		return
	}
	r.Punctuation = v
}
