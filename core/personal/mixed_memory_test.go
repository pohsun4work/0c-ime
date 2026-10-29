package personal

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
)

var (
	mp3Literal = []MixedToken{{"mp3", MixedLiteral}}
	mp3Chinese = []MixedToken{{"mp3", MixedChinese}}
	mp3Of      = MixedRef{"mp3", "的", ""}
)

func mixedModel() *Model {
	return NewModel(lexicon.NewMem(), lm.NewMem())
}

func mixedRecords(m *Model, keys string) []MixedRecord {
	groups, _ := m.MixedMemoryIndex()
	return groups[keys]
}

func TestMixedMemoryLearnsAndConfirms(t *testing.T) {
	m := mixedModel()
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	rs := mixedRecords(m, "mp3")
	if len(rs) != 1 || rs[0].Confirmed != 2 || rs[0].MixedRef != mp3Of {
		t.Fatalf("同一個位置選兩次要確認兩次：%+v", rs)
	}
	if _, longest := m.MixedMemoryIndex(); longest != 3 {
		t.Fatalf("最長的按鍵：%d", longest)
	}
}

func TestMixedMemoryContestsExactRecord(t *testing.T) {
	m := mixedModel()
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	back := MixedChoice{MixedRef: mp3Of, Form: mp3Chinese, Contest: &mp3Of}
	m.LearnMixedMemory([]MixedChoice{back}, nil)
	if rs := mixedRecords(m, "mp3"); len(rs) != 1 || rs[0].Confirmed != 1 || rs[0].Form[0].Kind != MixedLiteral {
		t.Fatalf("確認兩次的被選回一次只扣一次、不記新的：%+v", rs)
	}
	m.LearnMixedMemory([]MixedChoice{back}, nil)
	if len(m.Store.MixedMemory) != 0 || len(mixedRecords(m, "mp3")) != 0 {
		t.Fatalf("只確認一次的被選回就刪掉、不記新的：%+v", m.Store.MixedMemory)
	}
	// 沒有記憶換、但這個位置有相反的紀錄（例如被鎖定擋住沒換）：同樣抵銷。
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Chinese}}, nil)
	if len(m.Store.MixedMemory) != 0 {
		t.Fatalf("相反的選擇要抵銷：%+v", m.Store.MixedMemory)
	}
}

func TestMixedMemoryBackoffCorrectionAddsLocalRecord(t *testing.T) {
	m := mixedModel()
	i7 := MixedRef{"i7", "的", ""}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: i7, Form: []MixedToken{{"i7", MixedLiteral}}}}, nil)
	good := MixedRef{"i7", "好", ""}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: good, Form: []MixedToken{{"i7", MixedChinese}}}}, nil)
	rs := mixedRecords(m, "i7")
	if len(rs) != 2 || m.Store.MixedMemory[i7.key()].Confirmed != 1 || m.Store.MixedMemory[good.key()].Form[0].Kind != MixedChinese {
		t.Fatalf("從別處套用的被改回：原本那筆不動、另記這個位置：%+v", rs)
	}
}

func TestMixedMemoryUsedRefreshesAndEvictsOldest(t *testing.T) {
	m := mixedModel()
	var choices []MixedChoice
	for i := range maxMixedMemory {
		choices = append(choices, MixedChoice{MixedRef: MixedRef{fmt.Sprint(i), "", ""}, Form: mp3Literal})
	}
	m.LearnMixedMemory(choices, nil)
	m.LearnMixedMemory(nil, []MixedApplied{{MixedRef: MixedRef{"0", "", ""}, Exact: true}}) // 用到的不會先被丟
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	if len(m.Store.MixedMemory) != maxMixedMemory {
		t.Fatalf("上限 %d 筆，現在 %d 筆", maxMixedMemory, len(m.Store.MixedMemory))
	}
	if _, ok := m.Store.MixedMemory[MixedRef{"0", "", ""}.key()]; !ok {
		t.Fatal("剛用到的紀錄被丟掉")
	}
	if _, ok := m.Store.MixedMemory[MixedRef{"1", "", ""}.key()]; ok {
		t.Fatal("最久沒用到的要先丟")
	}
}

func TestMixedMemoryBackoffUseTouchesEveryAgreeingRecord(t *testing.T) {
	m := mixedModel()
	other := MixedRef{"mp3", "我", ""}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}, {MixedRef: other, Form: mp3Literal}}, nil)
	before := m.mixedSerial()
	m.LearnMixedMemory(nil, []MixedApplied{{MixedRef: MixedRef{"mp3", "柳", ""}, Form: mp3Literal}})
	for _, r := range mixedRecords(m, "mp3") {
		if r.Serial <= before {
			t.Fatalf("從別處套用時，換成同樣子的每一筆都算用到：%+v", mixedRecords(m, "mp3"))
		}
	}
}

func TestForgetMixedMemory(t *testing.T) {
	m := mixedModel()
	other := MixedRef{"mp3", "我", ""}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}, {MixedRef: other, Form: mp3Literal}}, nil)
	if !m.ForgetMixedMemory(MixedApplied{MixedRef: mp3Of, Exact: true}) || len(m.Store.MixedMemory) != 1 || len(mixedRecords(m, "mp3")) != 1 {
		t.Fatalf("前後都對上的只刪那一筆：%+v", m.Store.MixedMemory)
	}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	if !m.ForgetMixedMemory(MixedApplied{MixedRef: MixedRef{"mp3", "柳", ""}, Form: mp3Literal}) || len(m.Store.MixedMemory) != 0 {
		t.Fatalf("從別處套用的，刪掉換成同樣子的每一筆：%+v", m.Store.MixedMemory)
	}
	if m.ForgetMixedMemory(MixedApplied{MixedRef: mp3Of, Exact: true}) {
		t.Fatal("沒有紀錄時回報沒刪")
	}
}

func TestMixedMemoryIndexFollowsStore(t *testing.T) {
	m := mixedModel()
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.Store = NewStore() // 重新載入個人資料
	if rs := mixedRecords(m, "mp3"); len(rs) != 0 {
		t.Fatalf("換了個人資料要重建：%+v", rs)
	}
}

func TestMixedMemorySavesAndLoads(t *testing.T) {
	m := mixedModel()
	var empty bytes.Buffer
	if err := m.Store.Save(&empty); err != nil || strings.Contains(empty.String(), "MixedMemory") {
		t.Fatalf("沒有記憶時不寫這一欄：%v %s", err, empty.String())
	}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	var b bytes.Buffer
	if err := m.Store.Save(&b); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStore(&b)
	if err != nil {
		t.Fatal(err)
	}
	if r := s.MixedMemory[mp3Of.key()]; r.Confirmed != 1 || r.MixedRef != mp3Of || len(r.Form) != 1 || r.Form[0] != mp3Literal[0] {
		t.Fatalf("存檔讀回不一樣：%+v", r)
	}
}
