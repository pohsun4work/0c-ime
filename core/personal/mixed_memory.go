package personal

import (
	"cmp"
	"maps"
	"slices"
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

// MixedRef 指出一筆記憶。Left 是前一個字，空字串是句首；Right 是後面接的中文音節，空字串是空白、標點、英數或沒有東西。
type MixedRef struct{ Keys, Left, Right string }

func (r MixedRef) key() string { return r.Keys + "\t" + r.Left + "\t" + r.Right }

// MixedRecord 是一筆記憶。
type MixedRecord struct {
	MixedRef
	Form      []MixedToken
	Confirmed int    // 確認次數；被反向選取時扣一次
	Serial    uint64 // 最後一次學到或用到的順序，滿了先丟最舊的
}

// MixedChoice 是一次送出時使用者用 ↓ 換過的一段。Contest 是選之前那段照前後都對上的記憶換過（或留著），換它的那筆。
type MixedChoice struct {
	MixedRef
	Form    []MixedToken
	Contest *MixedRef
}

// MixedApplied 是讓一段照記憶換的紀錄：Exact 是前後都對上的那一筆，否則是按鍵相同、換成 Form 的每一筆（從別處套用）。
type MixedApplied struct {
	MixedRef
	Exact bool
	Form  []MixedToken
}

// mixedIndex 快取依按鍵分組的紀錄；Store 換掉或記憶改變（changedMixed）時重建。
type mixedIndex struct {
	store   *Store
	count   int
	groups  map[string][]MixedRecord
	longest int
}

// MixedMemoryIndex 依按鍵分組全部紀錄（組內順序固定）與最長的按鍵長度。回傳的 map 是共用的，不要修改。
func (m *Model) MixedMemoryIndex() (map[string][]MixedRecord, int) {
	idx := &m.mixedIndex
	if idx.groups == nil || idx.store != m.Store || idx.count != len(m.Store.MixedMemory) {
		*idx = mixedIndex{store: m.Store, count: len(m.Store.MixedMemory), groups: map[string][]MixedRecord{}}
		for _, k := range slices.Sorted(maps.Keys(m.Store.MixedMemory)) {
			r := m.Store.MixedMemory[k]
			idx.groups[r.Keys] = append(idx.groups[r.Keys], r)
			idx.longest = max(idx.longest, len(r.Keys))
		}
	}
	return idx.groups, idx.longest
}

// changedMixed 讓索引下次重建。
func (m *Model) changedMixed() { m.mixedIndex.groups = nil }

// LearnMixedMemory 學一次送出裡的選擇。記憶換的段落被改成別的樣子時：前後都對上的那筆（Contest）扣一次確認
// （只確認過一次就刪），這次不記新的，再選一次才記；從別處套用的不動原本那筆，另記這個位置的紀錄。
// used 是這次送出裡照記憶換、沒被改掉的段落，用到的紀錄移到最新。
func (m *Model) LearnMixedMemory(choices []MixedChoice, used []MixedApplied) {
	if len(choices) == 0 && len(used) == 0 {
		return
	}
	defer m.changedMixed()
	if m.Store.MixedMemory == nil {
		m.Store.MixedMemory = map[string]MixedRecord{}
	}
	serial := m.mixedSerial()
	touch := func(k string, r MixedRecord) {
		serial++
		r.Serial = serial
		m.Store.MixedMemory[k] = r
	}
	for _, a := range used {
		for _, k := range m.appliedKeys(a) {
			touch(k, m.Store.MixedMemory[k])
		}
	}
	for _, c := range choices {
		if c.Contest != nil {
			m.contestMixed(c.Contest.key())
			continue
		}
		k := c.key()
		r, ok := m.Store.MixedMemory[k]
		if ok && !slices.Equal(r.Form, c.Form) {
			m.contestMixed(k)
			continue
		}
		r.MixedRef, r.Form = c.MixedRef, slices.Clone(c.Form)
		r.Confirmed++
		touch(k, r)
	}
	if extra := len(m.Store.MixedMemory) - maxMixedMemory; extra > 0 {
		mm := m.Store.MixedMemory
		oldest := slices.SortedFunc(maps.Keys(mm), func(a, b string) int { return cmp.Compare(mm[a].Serial, mm[b].Serial) })
		for _, k := range oldest[:extra] {
			delete(mm, k)
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

// ForgetMixedMemory（候選窗的 Shift＋Delete）刪掉讓那段被換的紀錄，回報有沒有刪掉。
func (m *Model) ForgetMixedMemory(a MixedApplied) bool {
	keys := m.appliedKeys(a)
	for _, k := range keys {
		delete(m.Store.MixedMemory, k)
	}
	m.changedMixed()
	return len(keys) > 0
}

// appliedKeys 是讓一段照記憶換的紀錄：前後都對上的是那一筆，從別處套用的是按鍵相同、換成同樣子的每一筆。
func (m *Model) appliedKeys(a MixedApplied) []string {
	if a.Exact {
		if _, ok := m.Store.MixedMemory[a.key()]; ok {
			return []string{a.key()}
		}
		return nil
	}
	var out []string
	for k, r := range m.Store.MixedMemory {
		if r.Keys == a.Keys && slices.Equal(r.Form, a.Form) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func (m *Model) mixedSerial() uint64 {
	var serial uint64
	for _, r := range m.Store.MixedMemory {
		serial = max(serial, r.Serial)
	}
	return serial
}
