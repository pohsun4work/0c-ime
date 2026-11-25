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

//go:embed revise.tsv
var reviseTSV string

// Revise 回傳照 revise.tsv 增刪、照語料詞頻排序的字典：詞頻高的先，一樣時短詞先，再照字母序。
// 先驗落在 (0, 200)，比個人學會的英文詞（320 起）低，學會的詞照樣排最前面。
func Revise(d *english.Dictionary) *english.Dictionary {
	zipf := make(map[string]float64, strings.Count(wordfreqTSV, "\n"))
	for _, fields := range rows(wordfreqTSV, 2) {
		z, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || z <= 0 {
			panic(fmt.Sprintf("wordfreq.tsv 詞頻不合法：%q", fields))
		}
		zipf[fields[0]] = z
	}
	var add, remove []string
	for _, fields := range rows(reviseTSV, 3) {
		switch fields[1] {
		case "add":
			add = append(add, fields[0])
		case "remove":
			remove = append(remove, fields[0])
		default:
			panic(fmt.Sprintf("revise.tsv 處理不合法：%q", fields))
		}
	}
	return d.Revised(add, remove, func(text string) float64 {
		return (math.Round(zipf[text]*100)*64 + float64(64-len(text))) / 256
	})
}

// rows 回傳 tsv 每一列（略過 # 開頭的註解），欄數不對時 panic：兩份都是編進執行檔的檔案，壞了是建置錯誤。
func rows(tsv string, columns int) [][]string {
	var out [][]string
	for _, line := range strings.Split(strings.TrimSuffix(tsv, "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != columns {
			panic(fmt.Sprintf("欄位數不是 %d：%q", columns, line))
		}
		out = append(out, fields)
	}
	return out
}
