package proto

// 整合風格（操作規則 symbols）另外照微軟新注音的 Ctrl＋[ ]，但打最常用的「」，不是微軟的【】；引擎在 ctrl_punctuation_keys
// 多列 CtrlPunctExtraKeys，外殼看到列了才送。
const (
	KeyCtrlLeftBracket  = "Ctrl+[" // 「
	KeyCtrlRightBracket = "Ctrl+]" // 」
	CtrlPunctExtraKeys  = "[]"
)

// KeyRow 是鍵盤形狀候選的一列：幾個鍵、這一列的起點（以鍵寬計，照實體鍵盤錯開）。
type KeyRow struct {
	Keys   int     `json:"keys"`
	Offset float64 `json:"offset"`
}

// KeySymbolBoard 開關整合風格（操作規則 symbol_board）的符號表；外殼只在引擎回報 symbol_board=true、中文模式時送。
const KeySymbolBoard = "Ctrl+Alt+,"

var ctrlPunctExtra = map[string]string{KeyCtrlLeftBracket: "「", KeyCtrlRightBracket: "」"}

// CtrlPunctExtra 回傳整合風格多的 Ctrl＋標點鍵打出的全形標點；不是這兩個鍵回傳 false。
func CtrlPunctExtra(k string) (string, bool) {
	p, ok := ctrlPunctExtra[k]
	return p, ok
}
