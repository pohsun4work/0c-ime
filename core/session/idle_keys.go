package session

import "strings"

// IdleKeys 回報沒在組字、在注音模式時輸入法會處理的可見 ASCII 按鍵：0x21–0x7e 每一個（不含空白），半形全形都一樣。
// 中英混打時字母、數字與注音鍵都由輸入法判斷，標點與其他符號沒在組字時也留在組字區（hoshi_symbols.go）。
// 給自己先決定閒置按鍵要不要交給核心的外殼（Windows 的 TestKeyDown）；engine 的測試逐鍵比對實際的處理結果。
func IdleKeys() string {
	var b strings.Builder
	for c := byte(0x21); c <= 0x7e; c++ {
		b.WriteByte(c)
	}
	return b.String()
}
