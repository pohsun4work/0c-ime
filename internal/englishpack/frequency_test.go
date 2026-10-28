package englishpack

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
)

func prefixTexts(d *english.Dictionary, prefix string) []string {
	var out []string
	for _, c := range d.Prefix(prefix, 8) {
		out = append(out, c.Text)
	}
	return out
}

func TestFrequencyRanksByCorpusFrequencyThenLength(t *testing.T) {
	base := english.NewFiltered([]english.Word{
		{Text: "constancy", Weight: 60}, {Text: "constant", Weight: 40}, {Text: "constitution", Weight: 50}, {Text: "construction", Weight: 50}, {Text: "constxq", Weight: 70},
		{Text: "academic", Weight: 50}, {Text: "academy", Weight: 50}, {Text: "the", Weight: 65}, {Text: "constructs", Weight: 60},
	}, []string{"constructs"})
	d := Frequency(base)
	// ESDB 的先驗把 const 補成 constxq、constancy；詞頻排序照常用程度，wordfreq 沒收的詞排最後。
	if got := prefixTexts(base, "const"); got[0] != "constxq" {
		t.Fatalf("base ranking changed: %v", got)
	}
	want := []string{"construction", "constitution", "constant", "constancy", "constxq"}
	if got := prefixTexts(d, "const"); !slices.Equal(got, want) {
		t.Fatalf("frequency ranking: %v", got)
	}
	// academy 與 academic 詞頻相同（4.57）：短詞先，不照字母序。
	if got := prefixTexts(d, "acade"); len(got) != 2 || got[0] != "academy" {
		t.Fatalf("same frequency must prefer the shorter word: %v", got)
	}
	if !d.Has("constructs") || d.Suggestible("constructs") || !d.Has("constxq") || d.Len() != base.Len() {
		t.Fatal("reranking changed the vocabulary or no-suggest marks")
	}
	// 像不像常用詞拼錯照原本的先驗：constant 的 ESDB 先驗不到 50，constxq 有 70。
	for _, typed := range []string{"constnat", "constxa"} {
		if d.NearCommon(typed, 50) != base.NearCommon(typed, 50) {
			t.Errorf("%s: NearCommon must keep the original priors", typed)
		}
	}
	if !d.Reranked(func(string) float64 { return 1 }).NearCommon("constxa", 50) {
		t.Error("reranking twice lost the original priors")
	}
	// 個人學會的英文詞以 320 起的權重排在字典詞前面。
	if c := d.Prefix("the", 1); len(c) != 1 || c[0].Weight >= 320 || c[0].Weight <= 0 {
		t.Fatalf("prior out of range: %+v", c)
	}
}
