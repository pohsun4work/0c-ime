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
	// 官方的非預設規則照官方的做法擋混打：混打或英文補全開著時改不了，都關掉才能改。
	if err := back.Set("style-tab", "cycle"); err == nil {
		t.Fatal("an official non-default rule was accepted with mixed input")
	}
	for _, key := range []string{"mixed-input", "english-autocomplete"} {
		if err := back.Set(key, "off"); err != nil {
			t.Fatal(err)
		}
	}
	if err := back.Set("style-tab", "cycle"); err != nil {
		t.Fatal(err)
	}
	if reason := itemsByKey(back)["mixed-input"].DisabledReason; reason != styleExtensionReason {
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
