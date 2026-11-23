package session

import (
	"slices"
	"unicode"

	"github.com/0c-biggo/0c-ime/core/layout"
)

// fork 的符號規則（規格「符號」）：原則照微軟新注音，以下兩點優先於它。
//   - ` 打 `（全形時 ‵），不是微軟參考的前導鍵，也不開官方的符號選單（選單只留「游標在符號上按 ↑」）。
//   - 明確打的符號（不是注音鍵的符號鍵）沒在組字時也留在組字區、不交給應用程式，但不參與組字：
//     中英混打時不併進後面的按鍵（`ji3 $ wu0␣` 是 我$天）。緊接在符號後面的數字照數字留（`$20␣` 是 $20␣）。打到一半的英數照舊收下識別字、網址裡的符號（c++、user@x）。

// explicitSymbolKey：排列 l 的這一鍵是明確打的符號：鍵帽上不是字母、數字、空白，排列也不把它當注音鍵；
// = 開 Emoji 選單時除外（emojiEquals）。
func explicitSymbolKey(l layout.Layout, r rune, emojiEquals bool) bool {
	if r < '!' || r > '~' || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '=' && emojiEquals {
		return false
	}
	_, phonetic := l.Symbol(r)
	return !phonetic
}

func (s *Session) explicitSymbol(k string) bool {
	return !s.english && len(k) == 1 && explicitSymbolKey(s.layout, rune(k[0]), s.emojiEquals())
}

// hoshiSymbolKey 排在官方的符號處理（allSymbolsKey、styleKey）之前：` 與全形時的符號。
// 微軟參考的標點全形時照微軟：符號鍵都打全形（' ^ ` 是 ’ ︿ ‵，fullWidthText），留在組字區；其他風格照官方。
func (s *Session) hoshiSymbolKey(k string) bool {
	if s.cand != nil || !s.explicitSymbol(k) || s.fullWidthMixedKey(rune(k[0])) {
		return false
	}
	switch {
	case s.fullWidth && s.InputStyle.Punctuation == "microsoft":
		s.insertStyleLiteral(s.fullWidthText(k))
	case s.fullWidth && k == "`":
		s.insertStyleLiteral("‵")
	case k == "`":
		s.insertStyleLiteral(k)
	default:
		return false
	}
	return true
}

// microsoftBrackets 是微軟新注音 Ctrl+[ ] 打的【】按 ↓ 的清單（2026-10-29 實測，照畫面上的順序；
// 微軟 Ctrl+] 按數字選到的跟畫面不一致，不照抄）。
var microsoftBrackets = [][]string{runes("【「『〔︹︻﹁﹃﹝［｢"), runes("】」』〕︺︼﹂﹄﹞］｣")}

// literalAlternatives 是組字區裡的符號 current 按 ↓ 的清單，第一項是它自己：【】照微軟，其餘照官方的分組。
func literalAlternatives(current string) []string {
	for _, group := range microsoftBrackets {
		if group[0] == current {
			return slices.Clone(group)
		}
	}
	return symbolAlternatives(current)
}

// digitAfterExplicitSymbol：數字鍵 k 緊接在明確打的符號（或接在它後面的字面數字）後面，照數字留（$20、#3）。
func (s *Session) digitAfterExplicitSymbol(k string) bool {
	if len(k) != 1 || k[0] < '0' || k[0] > '9' {
		return false
	}
	at := s.cursor - 1
	for s.literalDigitAt(at) {
		at--
	}
	return at >= 0 && s.literals[at] && s.explicitSymbol(string(s.syls[at]))
}

// fullWidthMixedKey：全形時混打照常（fork）：大寫字母與打到一半的英數裡的符號跟半形一樣交給 mixedKey 併成一串
// （iPhone、c++），顯示與送出時再轉全形（snapshot、insertMixedLiteral）。
func (s *Session) fullWidthMixedKey(r rune) bool {
	if !s.fullWidth || s.english || !s.mixedInput() {
		return false
	}
	return r >= 'A' && r <= 'Z' || s.mixed.raw != "" && r >= '!' && r <= '~'
}
