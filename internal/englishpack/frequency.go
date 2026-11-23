package englishpack

import (
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/0c-biggo/0c-ime/core/english"
)

// 來源與重產方式見 wordfreq/README.md。編進執行檔、不改英文資料包，打包照樣沿用官方的英文子包。
//
//go:embed wordfreq/wordfreq.tsv
var wordfreqTSV string

// Frequency 回傳同一份詞表照語料詞頻排序的字典：詞頻高的先，一樣時短詞先，再照字母序。
// 先驗落在 (0, 200)，比個人學會的英文詞（320 起）低，學會的詞照樣排最前面。
func Frequency(d *english.Dictionary) *english.Dictionary {
	zipf := make(map[string]float64, strings.Count(wordfreqTSV, "\n"))
	for _, line := range strings.Split(strings.TrimSuffix(wordfreqTSV, "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		word, value, _ := strings.Cut(line, "\t")
		z, err := strconv.ParseFloat(value, 64)
		if err != nil || z <= 0 {
			panic(fmt.Sprintf("wordfreq.tsv 詞頻不合法：%q", line))
		}
		zipf[word] = z
	}
	return d.Reranked(func(text string) float64 {
		return (math.Round(zipf[text]*100)*64 + float64(64-len(text))) / 256
	})
}
