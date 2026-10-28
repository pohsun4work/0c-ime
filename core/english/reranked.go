package english

// Reranked keeps the same words and no-suggest marks under host-supplied
// ranking priors. Prior thresholds (NearCommon) still read the original
// priors, so hosts can replace the dictionary outright.
func (d *Dictionary) Reranked(prior func(text string) float64) *Dictionary {
	if d == nil {
		return nil
	}
	r := &Dictionary{words: make([]Word, len(d.words)), noSuggest: d.noSuggest, priors: d}
	if d.priors != nil {
		r.priors = d.priors
	}
	for i, w := range d.words {
		r.words[i] = Word{w.Text, prior(w.Text)}
	}
	r.buildIndex()
	return r
}
