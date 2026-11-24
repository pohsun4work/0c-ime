package session

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// shiftedSymbols 是美式鍵盤要按 Shift 才打得出的符號。外殼送來的是按鍵實際產生的字元（Shift＋9 是 "("，
// 見 layout.Layout.Punct），所以核心不必另外知道 Shift 有沒有按著。
const shiftedSymbols = "~!@#$%^&*()_+{}|:\"<>?"

// halfwidthAlternative：未按 Shift 的標點鍵打出的全形標點按 ↓ 第二項是這一鍵的半形原字元
// （[ → 「 的 ↓ 有 [），跟半形符號 ↓ 第二項是全形對稱。
func (s *Session) halfwidthAlternative(k, p string) bool {
	return !s.fullWidth && len(k) == 1 && !strings.Contains(shiftedSymbols, k) && utf8.RuneCountInString(p) == 1
}

// insertHalfwidth 在游標處插入標點鍵 k 的半形 half，↓ 列出這一鍵原本的全形標點 p 與鍵帽字元 k（同組已列的不重複）。
func (s *Session) insertHalfwidth(half, k, p string) {
	s.insertLiteral(half)
	known := symbolAlternatives(half)
	var alts []string
	for _, a := range []string{p, k} {
		if !slices.Contains(known, a) && !slices.Contains(alts, a) {
			alts = append(alts, a)
		}
	}
	if len(alts) > 0 {
		s.setLiteralKey(s.cursor-1, strings.Join(alts, literalKeySep))
	}
}
