package session

import (
	"math"
	"slices"
	"strings"

	"github.com/0c-biggo/0c-ime/core/proto"
)

const boardKind = "keyboard"

// boardKeys 是每一格對應的實體鍵，照鍵盤由上而下、由左而右；開著時就是選字鍵（selectionKeys），一頁就是這幾格。
const boardKeys = "`1234567890-=qwertyuiop[]\\asdfghjkl;'zxcvbnm,./"

// boardRows 是鍵盤的每一列：鍵數與起點（以鍵寬計，照標準美式鍵盤錯開），快照照樣送給外殼畫。
var boardRows = []proto.KeyRow{{Keys: 13, Offset: 0}, {Keys: 13, Offset: 1.5}, {Keys: 11, Offset: 1.75}, {Keys: 10, Offset: 2.25}}

// 數學頁的 3 鍵用 ＞，跟 2 鍵的 ＜ 成對。
var boardPages = []struct{ name, symbols string }{
	{"一般", "‵１２３４５６７８９０－＝《》〈〉“”【】（）〔〕＼※＊○●「」『』：；、！？…﹉─｛｝，。／"},
	{"數學", "≦≧＜＞∈∉∃°℃℉±－＝√∞∩∪∂∀∇Δ×÷＋≠≒μαβγδΩ∵∴∑πΦ∫∬∭§ρ∧∨λ′″"},
	{"特殊", "﹋┌┬┐¿¡☑☒✓✗©®™├┼┤↔↕↖↗↙↘←→↑↓└┴┘♫◉♀♂★◼●▲₩¥£€☺☹♥♦♠♣"},
}

func (s *Session) boardOpen() bool { return s.cand != nil && s.cand.kind == boardKind }

// symbolBoardKey 處理 Ctrl＋Alt＋,，開關整合風格（操作規則 symbol_board）照華碩注音的符號表：鍵盤形狀，一般／數學／特殊三頁，
// 三頁接成一份候選、一頁一個鍵盤。開著時按某個鍵就把那一格放進組字區並關閉（沒在打字時也留在組字區）；Tab／Shift＋Tab
// （也可 PageDown／PageUp、空白）換頁，頭尾相接；方向鍵照鍵盤上的位置移動反白。這些在 styleKey 之前處理，官方的候選規則與
// = 選單不會搶走；Enter（選反白的那一格，開啟時在 1 鍵）、Esc、Backspace、數字鍵盤照一般候選的處理。
// 開啟前打到一半的英數照混打定案、未打完的注音留成符號。其他候選開著時吃掉不動作；英文模式交給應用程式。
func (s *Session) symbolBoardKey(k string) (handled, ok bool) {
	if k == proto.KeySymbolBoard && s.InputStyle.SymbolBoard != "" {
		switch {
		case s.english:
			return false, true
		case s.boardOpen():
			s.cand = nil
		case s.cand == nil:
			s.symbolPrefix = false
			s.flushMixed()
			s.insertPendingLiteral()
			var items []string
			for _, p := range boardPages {
				items = append(items, runes(p.symbols)...)
			}
			s.openSymbols(items, s.cursor, 0, false)
			s.cand.kind, s.cand.cursor = boardKind, strings.IndexByte(boardKeys, '1')
		}
		return true, true
	}
	if !s.boardOpen() {
		return false, false
	}
	page := s.candidatePage() * len(boardKeys)
	at := s.cand.cursor - page
	switch k = s.normalizeVocabKey(k); k {
	case proto.KeyTab, proto.KeyPageDown, " ":
		s.cand.cursor = (s.cand.cursor + len(boardKeys)) % len(s.cand.items)
	case proto.KeyBackTab, proto.KeyPageUp:
		s.cand.cursor = (s.cand.cursor - len(boardKeys) + len(s.cand.items)) % len(s.cand.items)
	case proto.KeyLeft:
		s.cand.cursor = page + max(0, at-1)
	case proto.KeyRight:
		s.cand.cursor = page + min(len(boardKeys)-1, at+1)
	case proto.KeyUp:
		s.cand.cursor = page + boardVertical(at, -1)
	case proto.KeyDown:
		s.cand.cursor = page + boardVertical(at, 1)
	default:
		if n := strings.Index(boardKeys, k); len(k) == 1 && n >= 0 {
			return s.selectCandidate(page + n), true
		}
		return false, false
	}
	return true, true
}

// boardVertical 移到上一列或下一列，位置最接近的那一鍵；已在最上或最下列時不動。相鄰兩列錯開半鍵時兩邊一樣近，
// 取欄位編號接近原本的那一鍵，上下來回才會回到同一鍵（2 往下是 w，w 往上回到 2）。index 是一頁裡的第幾格。
func boardVertical(index, step int) int {
	row, col := 0, index
	for col >= boardRows[row].Keys {
		col -= boardRows[row].Keys
		row++
	}
	to := row + step
	if to < 0 || to >= len(boardRows) {
		return index
	}
	x := boardRows[row].Offset + float64(col)
	best := 0
	for c := range boardRows[to].Keys {
		d, b := math.Abs(boardRows[to].Offset+float64(c)-x), math.Abs(boardRows[to].Offset+float64(best)-x)
		if d < b || d == b && math.Abs(float64(c-col)) < math.Abs(float64(best-col)) {
			best = c
		}
	}
	first := 0
	for r := range to {
		first += boardRows[r].Keys
	}
	return first + best
}

func (s *Session) selectBoard(id int) bool {
	text := s.cand.items[id].Entry.Surface
	s.cand = nil
	s.insertLiteral(text)
	return true
}

// boardSnapshot 填入符號表的標題（目前那一頁加框）與鍵盤的每一列。
func (s *Session) boardSnapshot(cl *proto.CandidateList) {
	if !s.boardOpen() {
		return
	}
	names := make([]string, len(boardPages))
	for i, p := range boardPages {
		names[i] = p.name
		if i == s.candidatePage() {
			names[i] = "［" + p.name + "］"
		}
	}
	cl.Title = strings.Join(names, " ") + " · Tab 換頁"
	cl.Rows = slices.Clone(boardRows)
}
