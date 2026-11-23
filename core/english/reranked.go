package english

import "cmp"

// Reranked 回傳同一份詞表（含不推薦的標記）、改用 prior 排序的字典；NearCommon 照樣看原本的先驗。
func (d *Dictionary) Reranked(prior func(text string) float64) *Dictionary {
	if d == nil {
		return nil
	}
	r := &Dictionary{words: make([]Word, len(d.words)), noSuggest: d.noSuggest, priors: cmp.Or(d.priors, d)}
	for i, w := range d.words {
		r.words[i] = Word{w.Text, prior(w.Text)}
	}
	r.buildIndex()
	return r
}
