package personal

import (
	"cmp"
	"maps"
	"slices"
)

// 整合風格（操作規則 mixed_memory）的混打修正記憶：使用者用 ↓ 把一段按鍵換成別的解釋（英數、中文、注音）再送出時，
// 記下「按鍵＋前一個字＋後一個音 → 每段當什麼」；下次同樣的按鍵要不要照記憶換，由 session 照上下文決定。
// 中文要哪個字不記，交給更正（corrections.go）。
//
// 照天數淘汰，同選過的注音文（zhuyin_keys.go）：一次確認約 4 個月，照記憶換到或留著、沒被改掉的暫停衰減；
// maxMixedMemory 是保險，超過時先丟最久沒用到的。

const (
	maxMixedMemory   = 1024
	mixedMemoryFloor = zhuyinKeyFloor
)

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
	Confirmed int           // 確認次數；被反向選取時扣一次
	Serial    uint64        // 最後一次學到或用到的順序，滿了先丟最舊的
	Evidence  UsageEvidence // 學到時累加、用到時暫停衰減
}

// alive：舊版沒有 Evidence 的紀錄不衰減，下次學的時候起算。
func (r MixedRecord) alive(now int64) bool {
	e := r.Evidence.at(now)
	return r.Evidence == UsageEvidence{} || e.Short+e.Long >= mixedMemoryFloor
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

// mixedIndex 快取依按鍵分組的紀錄；Store 換掉、記憶改變（changedMixed）或換日時重建。
type mixedIndex struct {
	store   *Store
	count   int
	day     int64
	groups  map[string][]MixedRecord
	longest int
}

// MixedMemoryIndex 依按鍵分組還沒淘汰的紀錄（組內順序固定）與最長的按鍵長度。回傳的 map 是共用的，不要修改。
func (m *Model) MixedMemoryIndex() (map[string][]MixedRecord, int) {
	idx := &m.mixedIndex
	day := m.Store.Time / daySeconds
	if idx.groups == nil || idx.store != m.Store || idx.count != len(m.Store.MixedMemory) || idx.day != day {
		*idx = mixedIndex{store: m.Store, count: len(m.Store.MixedMemory), day: day, groups: map[string][]MixedRecord{}}
		for _, k := range slices.Sorted(maps.Keys(m.Store.MixedMemory)) {
			r := m.Store.MixedMemory[k]
			if !r.alive(m.Store.Time) {
				continue
			}
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
// used 是這次送出裡照記憶換或留著、沒被改掉的段落，用到的紀錄移到最新、暫停衰減。淘汰的紀錄這時拿掉。
func (m *Model) LearnMixedMemory(choices []MixedChoice, used []MixedApplied) {
	if len(choices) == 0 && len(used) == 0 {
		return
	}
	defer m.changedMixed()
	if m.Store.MixedMemory == nil {
		m.Store.MixedMemory = map[string]MixedRecord{}
	}
	serial, now := m.mixedSerial(), m.Store.Time
	for k, r := range m.Store.MixedMemory {
		if r.Evidence == (UsageEvidence{}) {
			r.Evidence = r.Evidence.add(now, confirmUnit)
			m.Store.MixedMemory[k] = r
		}
	}
	touch := func(k string, r MixedRecord) {
		serial++
		r.Serial = serial
		m.Store.MixedMemory[k] = r
	}
	for _, a := range used {
		for _, k := range m.appliedKeys(a) {
			r := m.Store.MixedMemory[k]
			r.Evidence.Updated = max(r.Evidence.Updated, now)
			touch(k, r)
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
		r.Evidence = r.Evidence.add(now, confirmUnit)
		touch(k, r)
	}
	maps.DeleteFunc(m.Store.MixedMemory, func(_ string, r MixedRecord) bool { return !r.alive(now) })
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

// ID 是這筆記憶在 Store.MixedMemory 的鍵，「混打記憶」頁用它指出要刪哪幾筆。
func (r MixedRef) ID() string { return r.key() }

// MixedMemoryRecords 是還沒淘汰的混打記憶，最近學到或用到的在前。
func (m *Model) MixedMemoryRecords() []MixedRecord {
	var out []MixedRecord
	for _, r := range m.Store.MixedMemory {
		if r.alive(m.Store.Time) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b MixedRecord) int { return cmp.Compare(b.Serial, a.Serial) })
	return out
}

// RemoveMixedMemory 刪掉「混打記憶」頁選的紀錄（MixedRef.ID），回傳刪了幾筆。
func (m *Model) RemoveMixedMemory(ids []string) int {
	n := 0
	for _, id := range ids {
		if _, ok := m.Store.MixedMemory[id]; ok {
			delete(m.Store.MixedMemory, id)
			n++
		}
	}
	if n > 0 {
		m.changedMixed()
	}
	return n
}
