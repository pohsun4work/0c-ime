package englishpack

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/english"
)

func TestFrequency(t *testing.T) {
	base := english.New([]english.Word{{Text: "constancy", Weight: 50}, {Text: "constant", Weight: 200}, {Text: "construction", Weight: 1}, {Text: "constzz", Weight: 300}})
	d := Frequency(base)
	var got []string
	for _, c := range d.Prefix("const", 8) {
		got = append(got, c.Text)
	}
	// 詞頻高的先；wordfreq 沒收的詞（constzz）當作 0、排最後
	if want := []string{"construction", "constant", "constancy", "constzz"}; !slices.Equal(got, want) {
		t.Fatalf("Prefix = %v，要 %v", got, want)
	}
	for _, c := range d.Prefix("const", 8) {
		if c.Weight <= 0 || c.Weight >= 200 {
			t.Fatalf("%s 的先驗 %g 要在 (0, 200)，排在個人學會的詞（320 起）後面", c.Text, c.Weight)
		}
	}
	// 像不像常用詞拼錯照樣看原本的先驗：constant 原本 200，換排序後不到 150
	if !d.NearCommon("constnat", 150) {
		t.Fatal("NearCommon 要看原本的先驗")
	}
}
