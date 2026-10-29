package personal

import (
	"slices"
	"sort"
)

// 整合風格（操作規則 mixed_memory）的混打修正記憶：使用者用 ↓ 把一段按鍵換成別的解釋（英數、中文、注音）再送出時，
// 記下「按鍵＋前一個字＋後一個音 → 每段當什麼」；下次同樣的按鍵要不要照記憶換，由 session 照上下文決定。
// 中文要哪個字不記，交給更正（corrections.go）。

const maxMixedMemory = 1024

// MixedToken 是一段按鍵與它的解釋。
type MixedToken struct {
	Keys string
	Kind string // MixedLiteral、MixedChinese 或 MixedZhuyin
}

const (
	MixedLiteral = "e" // 照字面的英數、符號
	MixedChinese = "c" // 中文音節
	MixedZhuyin  = "z" // 原樣留下的注音
)

// MixedRecord 是一筆記憶。Left 是前一個字，空字串是句首；Right 是後面接的中文音節，空字串是空白、標點、英數或沒有東西。
type MixedRecord struct {
	Keys        string
	Left, Right string
	Form        []MixedToken
	Confirmed   int    // 確認次數；被反向選取時扣一次
	Updated     int64  // 最後一次學到或用到的 Store.Time，滿了先丟最舊的
	Serial      uint64 // 同一時間的先後
}

// MixedRef 指出一筆記憶。
type MixedRef struct{ Keys, Left, Right string }

func (r MixedRef) key() string { return r.Keys + "\t" + r.Left + "\t" + r.Right }

// Ref 回傳 r 的位置。
func (r MixedRecord) Ref() MixedRef { return MixedRef{r.Keys, r.Left, r.Right} }

// MixedChoice 是一次送出時使用者用 ↓ 換過的一段。Applied 是選之前那段是記憶換的樣子時，換它的紀錄
// （Exact：前後都對上的那一筆；否則是按鍵相同、從別處套用的）。
type MixedChoice struct {
	MixedRef
	Form    []MixedToken
	Applied *MixedApplied
}

type MixedApplied struct {
	MixedRef
	Exact bool
}

// MixedMemoryIndex 依按鍵分組全部紀錄，組內順序固定。session 每次重看建一次，不每個位置都掃一遍。
func (m *Model) MixedMemoryIndex() map[string][]MixedRecord {
	if len(m.Store.MixedMemory) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m.Store.MixedMemory))
	for k := range m.Store.MixedMemory {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string][]MixedRecord{}
	for _, k := range keys {
		r := m.Store.MixedMemory[k]
		out[r.Keys] = append(out[r.Keys], r)
	}
	return out
}

// LearnMixedMemory 學一次送出裡的選擇。記憶換的段落被改成別的樣子時：前後都對上的那筆扣一次確認（只確認過一次就刪），
// 這次不記新的，再選一次才記；從別處套用的不動原本那筆，另記這個位置的紀錄。used 是這次送出裡照記憶換、沒被改掉的紀錄。
func (m *Model) LearnMixedMemory(choices []MixedChoice, used []MixedRef) {
	if len(choices) == 0 && len(used) == 0 {
		return
	}
	if m.Store.MixedMemory == nil {
		m.Store.MixedMemory = map[string]MixedRecord{}
	}
	serial := m.mixedSerial()
	touch := func(r MixedRecord) MixedRecord {
		serial++
		r.Updated, r.Serial = m.Store.Time, serial
		return r
	}
	for _, ref := range used {
		if r, ok := m.Store.MixedMemory[ref.key()]; ok {
			m.Store.MixedMemory[ref.key()] = touch(r)
		}
	}
	for _, c := range choices {
		if a := c.Applied; a != nil && a.Exact {
			m.contestMixed(a.key())
			continue
		}
		k := c.key()
		if old, ok := m.Store.MixedMemory[k]; ok && !slices.Equal(old.Form, c.Form) {
			m.contestMixed(k)
			continue
		}
		r := m.Store.MixedMemory[k]
		r.Keys, r.Left, r.Right, r.Form = c.Keys, c.Left, c.Right, slices.Clone(c.Form)
		r.Confirmed++
		m.Store.MixedMemory[k] = touch(r)
	}
	if len(m.Store.MixedMemory) > maxMixedMemory {
		keys := make([]string, 0, len(m.Store.MixedMemory))
		for k := range m.Store.MixedMemory {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := m.Store.MixedMemory[keys[i]], m.Store.MixedMemory[keys[j]]
			if a.Updated != b.Updated {
				return a.Updated < b.Updated
			}
			if a.Serial != b.Serial {
				return a.Serial < b.Serial
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys[:len(keys)-maxMixedMemory] {
			delete(m.Store.MixedMemory, k)
		}
	}
}

// contestMixed：確認過不只一次的扣一次，只確認過一次的刪掉。
func (m *Model) contestMixed(k string) {
	r, ok := m.Store.MixedMemory[k]
	switch {
	case !ok:
	case r.Confirmed > 1:
		r.Confirmed--
		m.Store.MixedMemory[k] = r
	default:
		delete(m.Store.MixedMemory, k)
	}
}

// ForgetMixedMemory（候選窗的 Shift＋Delete）刪掉讓那段被換的紀錄：前後都對上的就是那一筆，
// 從別處套用的是按鍵相同、換成同樣子的每一筆。回報有沒有刪掉。
func (m *Model) ForgetMixedMemory(a MixedApplied, form []MixedToken) bool {
	if a.Exact {
		_, ok := m.Store.MixedMemory[a.key()]
		delete(m.Store.MixedMemory, a.key())
		return ok
	}
	forgot := false
	for k, r := range m.Store.MixedMemory {
		if r.Keys == a.Keys && slices.Equal(r.Form, form) {
			delete(m.Store.MixedMemory, k)
			forgot = true
		}
	}
	return forgot
}

func (m *Model) mixedSerial() uint64 {
	var serial uint64
	for _, r := range m.Store.MixedMemory {
		serial = max(serial, r.Serial)
	}
	return serial
}
