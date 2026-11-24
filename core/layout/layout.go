// Package layout 把標準（大千）排列的實體按鍵對應到注音符號與全形標點。
package layout

import "github.com/0c-biggo/0c-ime/core/zhuyin"

// Layout 是一鍵一個符號的鍵盤排列。
type Layout struct {
	sym   map[rune]rune
	key   map[rune]rune
	punct map[rune]string
}

func newLayout(punct map[rune]string, pairs ...string) *Layout {
	l := &Layout{sym: map[rune]rune{}, key: map[rune]rune{}, punct: punct}
	for _, p := range pairs {
		r := []rune(p)
		l.sym[r[0]] = r[1]
		l.key[r[1]] = r[0]
	}
	return l
}

// Symbol 回傳按鍵代表的注音符號（聲調含一聲記號 zhuyin.Tone1），也就是「這個鍵是不是注音鍵」。
func (l *Layout) Symbol(key rune) (rune, bool) {
	r, ok := l.sym[key]
	return r, ok
}

// Press 在組音狀態 c 按下一鍵，回傳按下後的組音狀態與這一鍵打出的符號；不是注音鍵時 ok 為 false。
// 一般符號已經加進回傳的狀態；聲調（含空白的一聲）不加，由呼叫端加上聲調完成音節。c 不會被修改。
func (l *Layout) Press(c zhuyin.Composer, key rune) (next zhuyin.Composer, sym rune, ok bool) {
	sym, ok = l.sym[key]
	if !ok {
		return c, 0, false
	}
	if zhuyin.ClassOf(sym) == zhuyin.Tone {
		return c, sym, true
	}
	c = c.Clone()
	c.Add(sym)
	return c, sym, true
}

// Backspace 退掉組音狀態的最後一鍵。c 不會被修改。
func (l *Layout) Backspace(c zhuyin.Composer) zhuyin.Composer {
	c = c.Clone()
	c.Backspace()
	return c
}

// Punct 回傳按鍵對應的全形標點（按鍵是實際產生的字元，例如 Shift＋, 為 '<'）。
func (l *Layout) Punct(key rune) (string, bool) {
	p, ok := l.punct[key]
	return p, ok
}

// Keys 回傳打出某音節所需的按鍵序列；供測試與工具使用。
func (l *Layout) Keys(s zhuyin.Syllable) string {
	out := make([]rune, 0, 4)
	hasTone := false
	for _, r := range string(s) {
		out = append(out, l.key[r])
		hasTone = hasTone || zhuyin.ClassOf(r) == zhuyin.Tone
	}
	if !hasTone {
		out = append(out, l.key[zhuyin.Tone1])
	}
	return string(out)
}

// standardPunct 是標準排列下的標點：注音用掉了 , . / ; - 與數字，標點改由 Shift 與其餘按鍵產生。
// 頓號 ' 與 \ 都可以打：\ 是 macOS 內建注音的習慣（#24）。& 打全形的＆（#46）；要半形時在符號上按 ↓ 重選，或切英文模式。
// ^ 打刪節號「……」、_ 打破折號「——」（#118，同 RIME 的全形標點），↓ 可換其他寫法或半形 ^ _。
var standardPunct = map[rune]string{
	'<': "，", '>': "。", '?': "？", ':': "：", '"': "；", '!': "！", '~': "～",
	'[': "「", ']': "」", '{': "『", '}': "』", '\'': "、", '\\': "、", '(': "（", ')': "）", '&': "＆",
	'^': "……", '_': "——",
}

// Standard 是標準（大千）排列。
var Standard = newLayout(standardPunct,
	"1ㄅ", "qㄆ", "aㄇ", "zㄈ", "2ㄉ", "wㄊ", "sㄋ", "xㄌ", "eㄍ", "dㄎ", "cㄏ",
	"rㄐ", "fㄑ", "vㄒ", "5ㄓ", "tㄔ", "gㄕ", "bㄖ", "yㄗ", "hㄘ", "nㄙ",
	"uㄧ", "jㄨ", "mㄩ",
	"8ㄚ", "iㄛ", "kㄜ", ",ㄝ", "9ㄞ", "oㄟ", "lㄠ", ".ㄡ", "0ㄢ", "pㄣ", ";ㄤ", "/ㄥ", "-ㄦ",
	" ˉ", "6ˊ", "3ˇ", "4ˋ", "7˙",
)
