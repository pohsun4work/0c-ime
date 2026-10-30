package inputstyle

import "testing"

// 整合風格加的欄位只接受空值（靈犀原本的做法）與各自的值。
func TestBlendRulesRejectUnknownValues(t *testing.T) {
	p, _ := Get("blend")
	for _, edit := range []func(*Rules){
		func(r *Rules) { r.Symbols = "asus" },
		func(r *Rules) { r.Width = "fullwidth" },
		func(r *Rules) { r.SymbolBoard = "microsoft" },
		func(r *Rules) { r.MixedResplit = "on" },
		func(r *Rules) { r.MixedContext = "on" },
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
