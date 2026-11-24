package session

import (
	"slices"
	"testing"

	"github.com/0c-biggo/0c-ime/core/proto"
)

// ups 按 n 次 ↑，回傳每次之後的組字文字；↑ 直接換字，不開候選窗、不送出。
func ups(t *testing.T, s *Session, n int) []string {
	t.Helper()
	var out []string
	for range n {
		a := key(s, proto.KeyUp)
		if !a.Handled || a.Candidates != nil || a.Commit != "" {
			t.Fatalf("↑ 要直接換字、不開候選窗：%+v", a)
		}
		out = append(out, a.Text())
	}
	return out
}

// P4.5：↑ 照微軟直接換游標處的字詞，連按接著換。原始按鍵與原樣注音不在輪換裡（以前「我們很好」↑ 會變成
// 我們cp3cl3）；只有一個候選時不動。
func TestUpCyclesCandidatesWithoutRawKeys(t *testing.T) {
	s := newSession()
	typeSyl(s, "ㄐㄧㄥˇ") // ↓ 清單：景 井 ru/3（原始按鍵） ㄐㄧㄥˇ
	if got := ups(t, s, 4); !slices.Equal(got, []string{"井", "景", "井", "景"}) {
		t.Fatalf("ㄐㄧㄥˇ ↑：%q", got)
	}
	s = newSession()
	typeSyl(s, "ㄨㄛˇ")
	typeSyl(s, "ㄔ") // ↓ 清單：吃 t␣（原始按鍵） ㄔ
	if got := ups(t, s, 2); !slices.Equal(got, []string{"我吃", "我吃"}) {
		t.Fatalf("只有一個候選時不動：%q", got)
	}
}

// 詞沒有別的候選時換游標旁那個字：風景 ↑ 是 風井。
func TestUpFallsBackToTheCharacterByTheCursor(t *testing.T) {
	s := newSession()
	symbolSyl(s, "ㄈㄥ")
	if a := symbolSyl(s, "ㄐㄧㄥˇ"); a.Text() != "風景" {
		t.Fatalf("setup: %+v", a)
	}
	if got := ups(t, s, 2); !slices.Equal(got, []string{"風井", "風景"}) {
		t.Fatalf("風景 ↑：%q", got)
	}
}

// 全形符號照 ↓ 清單的順序換下一個全形；半形符號、數字、英文不動。
func TestUpCyclesFullWidthSymbolsOnly(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want []string
	}{
		{"Ctrl+,", []string{"我、", "我﹐", "我､", "我，"}},
		{"Ctrl+[", []string{"我「", "我『"}},
	} {
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		key(s, tc.key)
		if got := ups(t, s, len(tc.want)); !slices.Equal(got, tc.want) {
			t.Fatalf("%s ↑：%q", tc.key, got)
		}
	}
	for _, keys := range [][]string{{","}, {"["}, {"1"}, {" ", "t", "h", "e"}} {
		s := newSession()
		typeSyl(s, "ㄨㄛˇ")
		var before proto.Snapshot
		for _, k := range keys {
			before = key(s, k)
		}
		if got := ups(t, s, 2); !slices.Equal(got, []string{before.Text(), before.Text()}) {
			t.Fatalf("%q ↑ 要不動：%q", keys, got)
		}
	}
}
