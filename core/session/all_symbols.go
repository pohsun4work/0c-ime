package session

import (
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/layout"
)

// allSymbolsKey（#282）：注音模式半形時，排列有全形標點的鍵不論有沒有按 Shift，都打鍵帽上的半形字元
// （Shift＋- 是 _、Shift＋, 是 <、[ 是 [）。
//   - 沒在組字時也留在組字區（明確打的符號，hoshi_symbols.go）。
//   - 打到一半的中英混打英文收下識別字、網址、路徑裡的符號（user_name、search?q=1、C:\Users、don't），
//     括號與其他符號讓英文先定案（DNS(我)）。
//   - 組字中插在游標處、留在組字區，↓ 第二項是這一鍵原本的全形標點；未打完的注音同其他標點留成符號。
//
// 全形標點用 Ctrl＋符號（#168）。英文模式、全形（Shift＋空白切的字寬）、候選窗開著時不經這裡。
func (s *Session) allSymbolsKey(k string) (handled bool, commit string, done bool) {
	if s.english || s.fullWidth || s.cand != nil || len(k) != 1 {
		return false, "", false
	}
	p, ok := layout.Standard.Punct(rune(k[0]))
	if !ok {
		return false, "", false
	}
	if strings.ContainsAny(k, ":\\'_^?&") && s.mixedKeepsPunctuation(k) {
		return true, "", true
	}
	s.flushMixed()
	s.insertPendingLiteral()
	s.insertHalfwidth(k, p)
	return true, "", true
}

// insertHalfwidth 在游標處插入標點鍵 k 的鍵帽字元，↓ 列出這一鍵原本的全形標點 p 與 k（同組已列的不重複）。
func (s *Session) insertHalfwidth(k, p string) {
	s.insertLiteral(k)
	known := symbolAlternatives(k)
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
