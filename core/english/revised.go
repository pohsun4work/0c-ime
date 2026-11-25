package english

import (
	"cmp"
	"slices"
)

// Revised 回傳加上 add、拿掉 remove、改用 prior 排序的字典。不推薦的標記照舊，加進來的詞照常推薦；
// NearCommon 照樣看原本的先驗。
func (d *Dictionary) Revised(add, remove []string, prior func(text string) float64) *Dictionary {
	if d == nil {
		return nil
	}
	words := make([]Word, 0, len(d.words)+len(add))
	var blocked []string
	for i, w := range d.words {
		if slices.Contains(remove, w.Text) {
			continue
		}
		words = append(words, Word{w.Text, prior(w.Text)})
		if d.noSuggest[i] && !slices.Contains(add, w.Text) {
			blocked = append(blocked, w.Text)
		}
	}
	for _, text := range add {
		words = append(words, Word{text, prior(text)})
	}
	r := NewFiltered(words, blocked)
	r.priors = cmp.Or(d.priors, d)
	return r
}
