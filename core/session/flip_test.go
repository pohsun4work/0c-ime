package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// P8 ↓ 翻邊：神經判成中文的字，同一串按鍵的另一種切法排第二，選了照那樣切；切出來的字再按 ↓ 翻得回去。
func TestDownJudgedCharOffersOtherSplit(t *testing.T) {
	s := splitSession(map[string]float64{"斷": 0, "2萬": -10}, nil)
	mixedType(s, "ji32j04")
	if got := itemTexts(key(s, proto.KeyDown)); len(got) < 2 || got[0] != "斷" || got[1] != "2萬" {
		t.Fatalf("我斷 的 斷 按 ↓：%q，要 斷、2萬…", got)
	}
	s.Handle(proto.Request{Type: proto.Select, Index: 1})
	if got := itemTexts(key(s, proto.KeyDown)); len(got) < 2 || got[0] != "萬" || got[1] != "斷" {
		t.Fatalf("選 2萬 之後 萬 按 ↓：%q，要 萬、斷…", got)
	}
	s.Handle(proto.Request{Type: proto.Select, Index: 1})
	if commit := key(s, proto.KeyEnter).Commit; commit != "我斷" {
		t.Errorf("翻回 斷 後送出 %q", commit)
	}
}

// 神經判成中文、沒有其他切法的字（a趴 的 趴），原按鍵排第二；被切出去的字母照舊（a 的 ㄇ）。
func TestDownJudgedCharKeysSecond(t *testing.T) {
	s := splitSession(map[string]float64{}, nil)
	mixedType(s, "ji3aq8 ")
	got := itemTexts(key(s, proto.KeyDown))
	if len(got) < 2 || got[0] != "趴" || got[1] != "q8 " {
		t.Fatalf("我a趴 的 趴 按 ↓：%q，要 趴、q8…", got)
	}
	key(s, proto.KeyEscape)
	key(s, proto.KeyLeft)
	key(s, proto.KeyLeft)
	if got := itemTexts(key(s, proto.KeyDown)); !slices.Equal(got, []string{"a", "ㄇ"}) {
		t.Errorf("我a趴 的 a 按 ↓：%q，要 a、ㄇ", got)
	}
}

// 判成英文、照字面留下的英數（go up 的 up）：↓ 列出規則組得出的字與注音，空白當一聲用掉；第一項是英數本身，選了不動。
func TestDownEnglishRunOffersChinese(t *testing.T) {
	for _, tc := range []struct {
		pick int
		want string
	}{{0, "go up "}, {1, "go 音"}, {3, "go ㄧㄣ"}} {
		s := splitSession(map[string]float64{"up": 0, "音": -10, "go": 0}, nil)
		mixedType(s, "go up ")
		got := itemTexts(key(s, proto.KeyDown))
		if !slices.Equal(got, []string{"up", "音", "因", "ㄧㄣ"}) {
			t.Fatalf("go up␣ 按 ↓：%q，要 up、音、因、ㄧㄣ", got)
		}
		s.Handle(proto.Request{Type: proto.Select, Index: tc.pick})
		if commit := key(s, proto.KeyEnter).Commit; commit != tc.want {
			t.Errorf("選第 %d 個送出 %q，要 %q", tc.pick, commit, tc.want)
		}
	}
}

// 組不出一個音節的英數（hello）照舊只有字面的選項。
func TestDownEnglishRunWithoutReading(t *testing.T) {
	s := splitSession(nil, nil)
	mixedType(s, "hello")
	key(s, proto.KeyLeft) // 定案、游標移到 o 前面
	if got := itemTexts(key(s, proto.KeyDown)); s.cand == nil || s.cand.flips != nil {
		t.Fatalf("hello 的 o 按 ↓ 不該開英數翻邊：%q", got)
	}
}
