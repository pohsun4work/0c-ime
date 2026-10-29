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
	m := NewModel(lexicon.NewMem(), lm.NewMem())
	m.Store.Time = 1000
	return m
}

func TestMixedMemoryLearnsAndConfirms(t *testing.T) {
	m := mixedModel()
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	rs := m.MixedMemoryIndex()["mp3"]
	if len(rs) != 1 || rs[0].Confirmed != 2 || rs[0].Ref() != mp3Of {
		t.Fatalf("同一個位置選兩次要確認兩次：%+v", rs)
	}
}

func TestMixedMemoryContestsExactRecord(t *testing.T) {
	m := mixedModel()
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	back := MixedChoice{MixedRef: mp3Of, Form: mp3Chinese, Applied: &MixedApplied{mp3Of, true}}
	m.LearnMixedMemory([]MixedChoice{back}, nil)
	if rs := m.MixedMemoryIndex()["mp3"]; len(rs) != 1 || rs[0].Confirmed != 1 || rs[0].Form[0].Kind != MixedLiteral {
		t.Fatalf("確認兩次的被選回一次只扣一次、不記新的：%+v", rs)
	}
	m.LearnMixedMemory([]MixedChoice{back}, nil)
	if len(m.Store.MixedMemory) != 0 {
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
	m.LearnMixedMemory([]MixedChoice{{MixedRef: good, Form: []MixedToken{{"i7", MixedChinese}}, Applied: &MixedApplied{i7, false}}}, nil)
	rs := m.MixedMemoryIndex()["i7"]
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
	m.Store.Time++
	m.LearnMixedMemory(nil, []MixedRef{{"0", "", ""}}) // 用到的不會先被丟
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

func TestForgetMixedMemory(t *testing.T) {
	m := mixedModel()
	other := MixedRef{"mp3", "我", ""}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}, {MixedRef: other, Form: mp3Literal}}, nil)
	if !m.ForgetMixedMemory(MixedApplied{mp3Of, true}, mp3Literal) || len(m.Store.MixedMemory) != 1 {
		t.Fatalf("前後都對上的只刪那一筆：%+v", m.Store.MixedMemory)
	}
	m.LearnMixedMemory([]MixedChoice{{MixedRef: mp3Of, Form: mp3Literal}}, nil)
	if !m.ForgetMixedMemory(MixedApplied{MixedRef{"mp3", "柳", ""}, false}, mp3Literal) || len(m.Store.MixedMemory) != 0 {
		t.Fatalf("從別處套用的，刪掉換成同樣子的每一筆：%+v", m.Store.MixedMemory)
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
	if r := s.MixedMemory[mp3Of.key()]; r.Confirmed != 1 || len(r.Form) != 1 || r.Form[0] != mp3Literal[0] {
		t.Fatalf("存檔讀回不一樣：%+v", r)
	}
}
