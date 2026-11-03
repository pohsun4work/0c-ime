package settings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/inputstyle"
)

func itemsByKey(s Settings) map[string]Item {
	items := map[string]Item{}
	for _, item := range s.Items() {
		items[item.Key] = item
	}
	return items
}

func roundTrip(t *testing.T, s Settings) (Settings, string) {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Settings
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	return back, string(raw)
}

func TestBlendIsAnUnlockedCustomCopyThatKeepsMixedInput(t *testing.T) {
	s := freshSettings()
	if err := s.ProfileAction("copy", "blend", "整合風格"); err != nil {
		t.Fatal(err)
	}
	p, r := s.ActiveProfile(), s.ProfileRules()
	if p.Builtin || p.Source != "blend" || p.Name != "整合風格" {
		t.Fatalf("not a custom copy: %+v", p)
	}
	want := inputstyle.Rules{Punctuation: "0c", ShiftSpace: "fullwidth", Symbols: "microsoft"}
	if !s.MixedInput || !s.EnglishAutocomplete || s.Toneless || s.EmojiShortcut != "off" || !s.CtrlPunctuation ||
		r.Punctuation != want.Punctuation || r.ShiftSpace != want.ShiftSpace || r.Symbols != want.Symbols ||
		profileRules(r) != r || s.BlendWidth() != "shift_space" {
		t.Fatalf("blend recipe not applied: %+v %+v", s, r)
	}
	if e := s.EngineRules(); e != inputstyle.DefaultExtras().Apply(r) {
		t.Fatalf("engine rules: %+v", e)
	}
	items := itemsByKey(s)
	if items["style-punctuation"].Value != "microsoft-halfwidth" || items["style-shift-space"].Value != "fullwidth" {
		t.Fatalf("style items: %+v %+v", items["style-punctuation"], items["style-shift-space"])
	}
	for key, item := range items {
		if item.Locked {
			t.Errorf("%s locked", key)
		}
		if item.Disabled {
			t.Errorf("%s disabled: %s", key, item.DisabledReason)
		}
	}
	back, _ := roundTrip(t, s)
	if back.ReadOnly() || back.ProfileRules() != r || !back.MixedInput || back.Blend != s.Blend {
		t.Fatalf("blend copy did not round trip: %+v", back.ProfileRules())
	}
	// 官方的規則照官方逐條判斷能不能跟混打並用：Tab 輪換可以，Tab 調整斷詞要先關掉混打才能改。
	if err := back.Set("style-tab", "cycle"); err != nil || !back.MixedInput {
		t.Fatalf("a compatible official rule: %v", err)
	}
	if err := back.Set("style-tab", "segment"); err == nil {
		t.Fatal("a conflicting official rule was accepted with mixed input")
	}
	if err := back.Set("mixed-input", "off"); err != nil {
		t.Fatal(err)
	}
	if err := back.Set("style-tab", "segment"); err != nil {
		t.Fatal(err)
	}
	if reason := itemsByKey(back)["mixed-input"].DisabledReason; reason != "Tab 調整斷詞時關閉" {
		t.Fatalf("mixed input should be disabled by the official rule: %q", reason)
	}
	if back.BlendWidth() != "" {
		t.Fatal("official full width without mixed input")
	}
}

// 整合風格改了官方的規則（標點、Shift＋空白），同官方的自訂風格只在標準排列可選。
func TestBlendFollowsOfficialKeyboardLayoutLimit(t *testing.T) {
	s := freshSettings()
	if err := s.Set("keyboard-layout", "eten"); err != nil {
		t.Fatal(err)
	}
	if err := s.ProfileAction("copy", "blend", "整合風格"); err == nil || !strings.Contains(err.Error(), "標準注音排列") {
		t.Fatalf("blend on eten: %v", err)
	}
}

// 全域開關不跟著風格：換風格（含鎖定的內建風格）照舊、照樣能改；跟預設一樣時不寫進設定檔。
func TestBlendSettingsAreGlobal(t *testing.T) {
	s := freshSettings()
	if _, raw := roundTrip(t, s); strings.Contains(raw, `"blend":{`) {
		t.Fatalf("default extras written: %s", raw)
	}
	if err := s.ProfileAction("select", "microsoft", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("mixed-zhuyin-text", "on"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("english-ranking", "alphabetical"); err != nil {
		t.Fatal(err)
	}
	if err := s.ProfileAction("select", "0c", ""); err != nil {
		t.Fatal(err)
	}
	if !s.Blend.MixedZhuyinText || s.Blend.EnglishRanking != "alphabetical" || s.EngineRules().EnglishRanking != "" ||
		!s.EngineRules().ContextRule(inputstyle.ContextZhuyin) {
		t.Fatalf("extras not kept: %+v", s.Blend)
	}
	back, raw := roundTrip(t, s)
	if back.Blend != s.Blend || !strings.Contains(raw, `"blend":{`) {
		t.Fatalf("extras did not round trip: %s", raw)
	}
	running := BlendRunning(back.BlendKey())
	for _, b := range blendSpecs() {
		if v, _ := back.Value(b.key); running[b.key] != v {
			t.Errorf("%s running %q, setting %q", b.key, running[b.key], v)
		}
	}
	if len(BlendRunning("")) != 0 {
		t.Fatal("old engines report no extras")
	}
}

// 舊版的整合風格把開關存在風格的規則裡：讀進來搬成全域開關、風格只留 symbols，width 換成 Shift＋空白切換全半形。
func TestBlendMigratesLegacyRecipe(t *testing.T) {
	legacy := func(active bool) Settings {
		s := freshSettings()
		if err := s.ProfileAction("copy", "blend", "整合風格"); err != nil {
			t.Fatal(err)
		}
		st := s.store()
		for i := range st.Items {
			if st.Items[i].Source == "blend" {
				r := &st.Items[i].Rules
				r.ShiftSpace, r.Width, r.EnglishRanking, r.EnglishLearning, r.SymbolBoard = "disabled", "shift_space", "frequency", "dictionary", "asus"
				r.MixedResplit, r.MixedContext, r.VocabMark, r.CandidateBackTab, r.MixedMemory, r.MixedOverwrite = "keys", "rules", "extended", "shorter", "context", "neural"
			}
		}
		s.putStore(st)
		if !active {
			if err := s.ProfileAction("select", "0c", ""); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	for _, active := range []bool{true, false} {
		back, raw := roundTrip(t, legacy(active))
		if back.ReadOnly() || back.Blend != inputstyle.AllExtras() || strings.Contains(raw, `"blend":{`) {
			t.Fatalf("legacy recipe (active %v) not migrated: %+v", active, back.Blend)
		}
		for _, p := range back.store().Items {
			if p.Rules != profileRules(p.Rules) || p.Source == "blend" && (p.Rules.Symbols != "microsoft" || p.Rules.ShiftSpace != "fullwidth") {
				t.Fatalf("legacy fields kept: %+v", p.Rules)
			}
		}
		if again, raw := roundTrip(t, back); again.Blend != back.Blend || !strings.Contains(raw, `"blend":{`) {
			t.Fatalf("migrated extras not saved: %s", raw)
		}
	}
	// 檔案已有 blend 時以它為準，風格裡留下的舊欄位照樣拿掉。
	s := legacy(true)
	s.Blend = inputstyle.Extras{EnglishRanking: "alphabetical"}
	if back, _ := roundTrip(t, s); back.Blend != s.Blend || back.ProfileRules() != profileRules(back.ProfileRules()) {
		t.Fatalf("saved extras overridden: %+v", back.Blend)
	}
	// 舊版改過標點操作的（那時只改 punctuation）照改過的標點，不顯示成微軟半形參考。
	s = legacy(true)
	st := s.store()
	for i := range st.Items {
		if st.Items[i].Source == "blend" {
			st.Items[i].Rules.Punctuation = "standard"
		}
	}
	s.putStore(st)
	if back, _ := roundTrip(t, s); back.ProfileRules().Symbols != "" || itemsByKey(back)["style-punctuation"].Value != "standard" {
		t.Fatalf("changed punctuation: %+v", back.ProfileRules())
	}
}

// 讀不懂的 blend 留原文寫回，改了任何一個開關才換成這一版的。
func TestBlendKeepsUnreadableValue(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"schema":1,"blend":{"english_ranking":"newer"}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Blend != inputstyle.DefaultExtras() {
		t.Fatalf("unreadable extras: %+v", s.Blend)
	}
	if _, raw := roundTrip(t, s); !strings.Contains(raw, `"newer"`) {
		t.Fatalf("unreadable extras dropped: %s", raw)
	}
	if err := s.Set("mixed-resplit", "off"); err != nil {
		t.Fatal(err)
	}
	if _, raw := roundTrip(t, s); strings.Contains(raw, `"newer"`) || strings.Count(raw, `"blend":`) != 1 {
		t.Fatalf("explicit change kept the old value: %s", raw)
	}
	// 新版多的開關：認得的照讀，原文寫回。
	s = Settings{}
	if err := json.Unmarshal([]byte(`{"schema":1,"blend":{"mixed_resplit":false,"english_ranking":"frequency","mixed_future":true}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Blend.MixedResplit || s.Blend.EnglishRanking != "frequency" {
		t.Fatalf("known extras not read: %+v", s.Blend)
	}
	if _, raw := roundTrip(t, s); !strings.Contains(raw, `"mixed_future"`) {
		t.Fatalf("newer extras dropped: %s", raw)
	}
}

// 混打的開關跟著中英混打停用；關掉「↓／Tab 跨過英數換整段」不停用其他開關。
func TestBlendDisabledReasons(t *testing.T) {
	s := freshSettings()
	if err := s.ProfileAction("copy", "0c", "custom"); err != nil {
		t.Fatal(err)
	}
	by := func(s Settings) map[string]string {
		out := map[string]string{}
		for _, b := range blendSpecs() {
			out[b.key] = itemsByKey(s)[b.key].DisabledBy
		}
		return out
	}
	off := by(s)
	for _, b := range blendSpecs() {
		if want := map[bool]string{true: "mixed-input"}[b.mixed]; off[b.key] != want {
			t.Errorf("mixed off: %s by %q", b.key, off[b.key])
		}
	}
	if err := s.Set("mixed-input", "on"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("mixed-resplit", "off"); err != nil {
		t.Fatal(err)
	}
	for key, by := range by(s) {
		if by != "" {
			t.Errorf("resplit off: %s by %q", key, by)
		}
	}
	if err := s.Set("toneless", "on"); err == nil {
		if got := by(s)["mixed-repeats"]; got != "toneless" {
			t.Errorf("toneless: mixed-repeats by %q", got)
		}
	} else if !strings.Contains(err.Error(), "聲調") {
		t.Fatal(err)
	}
}

// 標點操作的「微軟半形參考」是 symbols：標點照靈犀，所以跟不打聲調可以並用；換回其他選項就拿掉。
func TestPunctuationMicrosoftHalfwidth(t *testing.T) {
	s := freshSettings()
	if err := s.ProfileAction("copy", "0c", "custom"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("style-punctuation", "microsoft-halfwidth"); err != nil {
		t.Fatal(err)
	}
	if r := s.ProfileRules(); r.Punctuation != "0c" || r.Symbols != "microsoft" {
		t.Fatalf("halfwidth: %+v", r)
	}
	if err := s.Set("toneless", "on"); err != nil {
		t.Fatalf("toneless with halfwidth punctuation: %v", err)
	}
	if err := s.Set("toneless", "off"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("style-punctuation", "microsoft"); err != nil {
		t.Fatal(err)
	}
	if r := s.ProfileRules(); r.Punctuation != "microsoft" || r.Symbols != "" || itemsByKey(s)["style-punctuation"].Value != "microsoft" {
		t.Fatalf("back to microsoft: %+v", r)
	}
}
