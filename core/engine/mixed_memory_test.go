package engine

import (
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/vocab"
)

// 整合風格的「混打記憶」頁：列出 ↓ 換過的混打（最近的在前）與自動學會的英文字，可以刪幾筆混打記憶。
func TestVocabMixedMemoryListAndRemove(t *testing.T) {
	e := vocabEngine()
	st := e.Model.Store
	st.MixedMemory = map[string]personal.MixedRecord{}
	for i, r := range []personal.MixedRecord{
		{MixedRef: personal.MixedRef{Keys: "url ", Left: "看"}, Form: []personal.MixedToken{{Keys: "url ", Kind: personal.MixedLiteral}}, Confirmed: 2},
		{MixedRef: personal.MixedRef{Keys: "su3", Right: "ㄏㄠˇ"}, Form: []personal.MixedToken{{Keys: "su3", Kind: personal.MixedChinese}}, Confirmed: 1},
		{MixedRef: personal.MixedRef{Keys: "a ", Left: "我"}, Form: []personal.MixedToken{{Keys: "a ", Kind: personal.MixedZhuyin}}, Confirmed: 3},
	} {
		r.Serial = uint64(i + 1)
		st.MixedMemory[r.ID()] = r
	}
	st.English = map[string]personal.EnglishEvidence{
		"kubectl": {Text: "kubectl", Confirmed: 3, Days: 2, Updated: st.Time},
		"hello":   {Text: "Hello", Confirmed: 3, Days: 2, Updated: st.Time},
		"typo":    {Text: "typo", Confirmed: 1, Days: 1, Updated: st.Time},
	}
	r := e.Vocab(VocabRequest{Op: VocabMixed})
	var got []string
	for _, x := range r.Mixed {
		got = append(got, x.Keys+"|"+x.Left+"|"+x.Right+"|"+x.Form[0].Kind+":"+x.Form[0].Text)
	}
	if !r.OK || r.Changed || strings.Join(got, ",") != "a |我||zhuyin:ㄇ,su3||ㄏㄠˇ|chinese:ㄋㄧˇ,url |看||english:url " ||
		strings.Join(r.English, ",") != "Hello,kubectl" {
		t.Fatalf("%v %v %+v", got, r.English, r)
	}
	if r := e.Vocab(VocabRequest{Op: VocabMixedRemove}); r.OK || r.Error.Code != vocab.CodeEmpty {
		t.Fatalf("沒有選：%+v", r)
	}
	r = e.Vocab(VocabRequest{Op: VocabMixedRemove, Texts: []string{r.Mixed[0].ID, r.Mixed[2].ID, "gone"}})
	if !r.OK || !r.Changed || len(r.Mixed) != 1 || r.Mixed[0].Keys != "su3" || len(st.MixedMemory) != 1 || !LearningOp(VocabMixedRemove) {
		t.Fatalf("%+v", r)
	}
	if r := e.Vocab(VocabRequest{Op: VocabMixedRemove, Texts: []string{"gone"}}); !r.OK || r.Changed {
		t.Fatalf("沒有可刪的：%+v", r)
	}
}

func TestKeysZhuyin(t *testing.T) {
	for keys, want := range map[string]string{"ji3su3": "ㄨㄛˇㄋㄧˇ", "5jiy6": "ㄗㄨㄛˊ", "a ": "ㄇ", "e": "ㄍ", "3": "ˇ", "wu0 ": "ㄊㄧㄢ"} {
		if got := keysZhuyin(keys); got != want {
			t.Errorf("%q：%q，要 %q", keys, got, want)
		}
	}
}
