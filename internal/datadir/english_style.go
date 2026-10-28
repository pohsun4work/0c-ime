package datadir

import (
	"github.com/0c-biggo/0c-ime/core/engine"
	"github.com/0c-biggo/0c-ime/internal/englishpack"
)

// ApplyEnglishStyle 依 e.InputStyle 準備整合風格的英文詞頻排序與學習門檻；其他風格不動。
// 在 Open 與設定好 InputStyle 之後、開 session 之前呼叫。
func ApplyEnglishStyle(e *engine.Engine) {
	if e.InputStyle.EnglishRanking == "frequency" {
		e.English = englishpack.Frequency(e.English)
	}
	if e.InputStyle.EnglishLearning == "dictionary" {
		e.Model.KnownEnglish = e.English
	}
}
