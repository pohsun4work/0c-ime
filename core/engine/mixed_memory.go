package engine

import (
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/vocab"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// 整合風格的「混打記憶」頁：↓ 換過的混打解釋（personal.MixedRecord）與自動學會的英文字。英文字的移除、不再學
// 用「靈犀學到的」頁同一組指令（unlearn、forget）。
const (
	VocabMixed       = "mixed"        // 混打記憶與自動學會的英文字
	VocabMixedRemove = "mixed-remove" // Texts：要刪的混打記憶（MixedMemoryItem.ID）
)

// MixedMemoryItem 是一筆混打記憶。Left 是前一個字（空字串是句首），Right 是後一個音（注音；空字串是後面不是中文）。
type MixedMemoryItem struct {
	ID        string           `json:"id"`
	Keys      string           `json:"keys"`
	Left      string           `json:"left"`
	Right     string           `json:"right"`
	Form      []MixedFormToken `json:"form"`
	Confirmed int              `json:"confirmed"`
}

// MixedFormToken 是換成的一段：english 照字面的英數（Text 是按鍵），chinese 中文、zhuyin 原樣留下的注音（Text 是注音）。
type MixedFormToken struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}

var mixedKinds = map[string]string{personal.MixedLiteral: "english", personal.MixedChinese: "chinese", personal.MixedZhuyin: "zhuyin"}

func (e *Engine) mixedMemoryOp(req VocabRequest, resp *VocabResponse) error {
	m := e.Model
	if req.Op == VocabMixedRemove {
		if len(req.Texts) == 0 {
			return &vocab.Error{Code: vocab.CodeEmpty, Item: -1, Msg: "沒有選任何紀錄"}
		}
		resp.Changed = m.RemoveMixedMemory(req.Texts) > 0
	}
	for _, r := range m.MixedMemoryRecords() {
		item := MixedMemoryItem{ID: r.ID(), Keys: r.Keys, Left: r.Left, Right: r.Right, Confirmed: r.Confirmed}
		for _, t := range r.Form {
			text := t.Keys
			if t.Kind != personal.MixedLiteral {
				text = keysZhuyin(t.Keys)
			}
			item.Form = append(item.Form, MixedFormToken{text, mixedKinds[t.Kind]})
		}
		resp.Mixed = append(resp.Mixed, item)
	}
	for _, ev := range m.Store.English {
		if m.EnglishKnown(ev.Text) {
			resp.English = append(resp.English, ev.Text)
		}
	}
	slices.SortFunc(resp.English, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return nil
}

// keysZhuyin 照標準排列（混打只用標準排列）把按鍵組成注音：同一格後按的蓋掉先按的（打錯又補對的 5jiy6 是 ㄗㄨㄛˊ），
// 空白是一聲，不顯示。
func keysZhuyin(keys string) string {
	var b strings.Builder
	var c zhuyin.Composer
	for _, k := range keys {
		sym, ok := layout.Standard.Symbol(k)
		if !ok {
			b.WriteString(c.String())
			c.Reset()
			b.WriteRune(k)
			continue
		}
		if done, ok := c.Add(sym); ok {
			b.WriteString(string(done))
		} else if zhuyin.ClassOf(sym) == zhuyin.Tone && k != ' ' {
			b.WriteRune(sym) // 前面沒有注音的聲調鍵
		}
	}
	b.WriteString(c.String())
	return b.String()
}
