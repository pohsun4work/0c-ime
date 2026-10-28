package personal

import (
	"bytes"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
)

func TestKnownEnglishLearnsDictionaryWordsInThreeCommits(t *testing.T) {
	m := englishModel()
	dict := english.New([]english.Word{{Text: "constant", Weight: 40}, {Text: "constancy", Weight: 60}})
	m.KnownEnglish = dict
	m.AdvanceTime(100 * daySeconds)
	for i := 0; i < 2; i++ {
		m.LearnEnglish([]EnglishCommit{{Text: "constant"}})
	}
	if m.EnglishKnown("constant") {
		t.Fatal("two commits promoted")
	}
	m.LearnEnglish([]EnglishCommit{{Text: "constant"}})
	if !m.EnglishKnown("constant") {
		t.Fatalf("three commits on one day must promote a dictionary word: %+v", m.Store.English["constant"])
	}
	if cs := m.EnglishPrefix(dict, "cons", 2); len(cs) != 2 || cs[0].Text != "constant" {
		t.Fatalf("learned word should lead: %+v", cs)
	}
	for i := 0; i < 5; i++ { // 學會之後同一天再打，照每天兩次的上限，不灌高其他風格看到的紀錄
		m.LearnEnglish([]EnglishCommit{{Text: "constant"}})
	}
	// 字典沒有的詞照原本的規則：同一天確認幾次都不算學會。
	for i := 0; i < 5; i++ {
		m.LearnEnglish([]EnglishCommit{{Text: "LingxiCorn"}})
	}
	if m.EnglishKnown("LingxiCorn") {
		t.Fatal("non-dictionary word skipped the two-day rule")
	}
	var b bytes.Buffer
	if err := m.Store.Save(&b); err != nil {
		t.Fatal(err)
	}
	st, err := LoadStore(&b)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := st.English["constant"]; !ok || e.DayConfirmed > 2 || e.Confirmed != 3 {
		t.Fatalf("evidence must stay valid for older versions: %+v %v", e, ok)
	}
	// 其他風格（沒設 KnownEnglish）看同一份紀錄，仍要跨兩天。
	other := englishModel()
	other.Store = st
	if other.EnglishKnown("constant") {
		t.Fatal("dictionary rule leaked into other styles")
	}
}
