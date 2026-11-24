package session

import (
	"strings"

	"github.com/0c-biggo/0c-ime/core/layout"
	"unicode"
)

// IdleKeys 回報沒在組字、在注音模式時輸入法會處理的可見 ASCII 按鍵（0x21–0x7e，不含空白）：
// 中英混打時字母、數字與注音鍵都由輸入法判斷，標點與明確打的符號（含 `）沒在組字時也留在組字區（hoshi_symbols.go），
// 所以只有不是任何一種的鍵交還應用程式。給自己先決定閒置按鍵要不要交給核心的外殼（Windows 的 TestKeyDown）；
// session 的測試逐鍵比對實際的處理結果。
func IdleKeys() string {
	var b strings.Builder
	for r := rune(0x21); r <= 0x7e; r++ {
		_, punct := layout.Standard.Punct(r)
		if punct || explicitSymbolKey(r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if _, phonetic := layout.Standard.Symbol(r); phonetic {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FullWidthIdleKeys 回報全形（Shift＋空白切的字寬，#278）時沒在組字、注音模式會處理的可見 ASCII 按鍵：
// 全形時不是注音鍵也不是標點的鍵（@ # $ * + 等）與 Shift＋字母打成全形字元（styleKey），其餘照 IdleKeys。
// 數字鍵盤全形時一律打成全形字元，不在這份清單（外殼看字寬自己判斷）。字寬是 session 的狀態：外殼依快照的
// full_width 在這份與 IdleKeys 之間選。
func FullWidthIdleKeys() string {
	idle := IdleKeys()
	var b strings.Builder
	for r := rune(0x21); r <= 0x7e; r++ {
		_, phonetic := layout.Standard.Symbol(r)
		_, punct := layout.Standard.Punct(r)
		if r >= 'A' && r <= 'Z' || !phonetic && !punct || strings.ContainsRune(idle, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
