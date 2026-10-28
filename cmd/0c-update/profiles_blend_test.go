package main

import (
	"path/filepath"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/settings"
)

// 安裝說明的 0c-update profile copy blend --name 整合風格：整合風格不在內建清單，照樣能複製成自訂風格。
func TestProfileCLICopiesBlendRecipe(t *testing.T) {
	base := t.TempDir()
	f := &fakeEngine{composing: "0", info: map[string]string{}}
	res, err := runProfile(base, f.dial, profileOptions{action: "copy", id: "blend", name: "整合風格"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Profiles.Items {
		if p.ID == "blend" {
			t.Fatal("blend must not be listed as a built-in")
		}
	}
	s, err := settings.Load(settings.Path(filepath.Join(base, "user.json")))
	if err != nil {
		t.Fatal(err)
	}
	if p := s.ActiveProfile(); p.Builtin || p.Source != "blend" || !s.MixedInput || s.ProfileRules().EnglishRanking != "frequency" {
		t.Fatalf("blend copy not applied: %+v", p)
	}
}
