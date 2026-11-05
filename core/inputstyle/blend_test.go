package inputstyle

import "testing"

// 整合風格加的欄位只接受空值（靈犀原本的做法）與各自的值。
func TestBlendRulesRejectUnknownValues(t *testing.T) {
	p, _ := Get("0c")
	for _, edit := range []func(*Rules){
		func(r *Rules) { r.Symbols = "asus" },
		func(r *Rules) { r.SymbolBoard = "microsoft" },
		func(r *Rules) { r.MixedResplit = "on" },
		func(r *Rules) { r.MixedContext = "on" },
		func(r *Rules) { r.MixedContext = "repeats,initial" },
		func(r *Rules) { r.MixedContext = "initial,initial" },
		func(r *Rules) { r.MixedContext = "initial," },
		func(r *Rules) { r.VocabMark = "on" },
		func(r *Rules) { r.CandidateBackTab = "on" },
		func(r *Rules) { r.MixedMemory = "on" },
		func(r *Rules) { r.MixedOverwrite = "on" },
	} {
		r := p.Rules
		edit(&r)
		if r.Validate() == nil {
			t.Fatalf("invalid accepted: %+v", r)
		}
	}
}

// 全域開關換成的規則都過得了驗證。
func TestExtrasApply(t *testing.T) {
	p, _ := Get("0c")
	for _, x := range []Extras{{}, DefaultExtras(), AllExtras()} {
		if err := x.Apply(p.Rules).Validate(); err != nil {
			t.Fatalf("%+v: %v", x, err)
		}
	}
	r := DefaultExtras().Apply(p.Rules)
	if r.MixedContext != "initial,repeats,reversed,literal,alnum" || r.MixedOverwrite != "" || r.MixedResplit != "keys" {
		t.Fatalf("default extras: %+v", r)
	}
	if off := (Extras{}).Apply(p.Rules); off.MixedContext != "initial,alnum" || off.EnglishRanking != "" || off.SymbolBoard != "" {
		t.Fatalf("extras off: %+v", off)
	}
	for _, c := range contextRules {
		if r.ContextRule(c) != (c != ContextZhuyin) {
			t.Fatalf("context rule %s", c)
		}
	}
}
