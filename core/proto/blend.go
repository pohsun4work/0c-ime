package proto

// 整合風格（操作規則 symbols）另外照微軟新注音的 Ctrl＋[ ]；引擎在 ctrl_punctuation_keys 多列 CtrlPunctExtraKeys，
// 外殼看到列了才送。
const (
	KeyCtrlLeftBracket  = "Ctrl+[" // 【
	KeyCtrlRightBracket = "Ctrl+]" // 】
	CtrlPunctExtraKeys  = "[]"
)

var ctrlPunctExtra = map[string]string{KeyCtrlLeftBracket: "【", KeyCtrlRightBracket: "】"}

// CtrlPunctExtra 回傳整合風格多的 Ctrl＋標點鍵打出的全形標點；不是這兩個鍵回傳 false。
func CtrlPunctExtra(k string) (string, bool) {
	p, ok := ctrlPunctExtra[k]
	return p, ok
}
