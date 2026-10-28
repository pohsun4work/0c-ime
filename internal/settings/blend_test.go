package settings

import (
	"encoding/json"
	"testing"
)

func itemsByKey(s Settings) map[string]Item {
	items := map[string]Item{}
	for _, item := range s.Items() {
		items[item.Key] = item
	}
	return items
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
	if !s.MixedInput || !s.EnglishAutocomplete || s.Toneless || s.EmojiShortcut != "off" || !s.CtrlPunctuation ||
		r.ShiftSpace != "disabled" || r.EnglishRanking != "frequency" || r.EnglishLearning != "dictionary" {
		t.Fatalf("blend recipe not applied: %+v %+v", s, r)
	}
	for key, item := range itemsByKey(s) {
		if item.Locked {
			t.Errorf("%s locked", key)
		}
		if (key == "mixed-input" || key == "english-autocomplete") && item.Disabled {
			t.Errorf("%s disabled: %s", key, item.DisabledReason)
		}
	}
	raw, _ := json.Marshal(s)
	var back Settings
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.ReadOnly() || back.ProfileRules() != r || !back.MixedInput {
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
}

func TestBlendIsAvailableOnEveryKeyboardLayout(t *testing.T) {
	s := freshSettings()
	if err := s.Set("keyboard-layout", "eten"); err != nil {
		t.Fatal(err)
	}
	// 整合風格沒改官方的操作規則，不受「此風格目前支援標準注音排列」限制；混打照倚天排列的規則停用。
	if err := s.ProfileAction("copy", "blend", "整合風格"); err != nil {
		t.Fatal(err)
	}
	if reason := itemsByKey(s)["mixed-input"].DisabledReason; reason != "倚天 41 鍵時關閉" {
		t.Fatalf("unexpected mixed input state on eten: %q", reason)
	}
}
