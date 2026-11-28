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

// 問不到神經時不翻邊：規則出的字照舊，清單裡沒有切法。
func TestDownJudgedNeedsNeural(t *testing.T) {
	s := splitSession(nil, nil)
	mixedType(s, "ji32j04")
	if got := itemTexts(key(s, proto.KeyDown)); len(got) == 0 || got[0] != "斷" || slices.Contains(got, "2萬") {
		t.Errorf("沒有評分器時 ↓ 不列切法：%q", got)
	}
}

// 全形時混打照常：全形的英數一樣列得出組得出的字，選了連全形空白一起換掉。
func TestDownEnglishRunFullWidth(t *testing.T) {
	s := splitSession(map[string]float64{"up": 0, "音": -10, "go": 0}, nil)
	s.fullWidth = true
	mixedType(s, "go up ")
	if got := itemTexts(key(s, proto.KeyDown)); !slices.Equal(got, []string{"ｕｐ", "音", "因", "ㄧㄣ"}) {
		t.Fatalf("全形 go up␣ 按 ↓：%q", got)
	}
	s.Handle(proto.Request{Type: proto.Select, Index: 1})
	if commit := key(s, proto.KeyEnter).Commit; commit != "ｇｏ　音" {
		t.Errorf("選 音 送出 %q", commit)
	}
}

// 同一串按鍵的清單照「修改字詞的位置」只往一個方向包：句尾和前方模式從 萬 多一步、後方模式從照字面的 2 換過去，再 Tab 回來；
// 2|萬 兩種模式都不包 2，照字面清單換不過去就沒有標題。
func TestTabFlipSpan(t *testing.T) {
	scores := map[string]float64{"2萬": 0, "段": -5, "斷": -10}
	title := func(w string) string { return "修改「" + w + "」 · Tab 切換詞長" }
	for _, tc := range []struct {
		name          string
		keys          string
		before        bool
		lefts         int
		first, header string
		span          bool
	}{
		{"萬|", "ji32j04", false, 0, "萬", title("萬"), true},
		{"前方 萬|", "ji32j04", true, 0, "萬", title("萬"), true},
		{"前方 萬|我", "ji32j04ji3", true, 1, "萬", title("萬"), true},
		{"|2萬", "ji32j04", false, 2, "2", title("2"), true},
		{"後方 2|萬", "ji32j04", false, 1, "萬", title("萬"), false},
		{"前方 2|萬", "ji32j04", true, 1, "2", "", false},
	} {
		s := splitSession(scores, nil)
		s.CandidateBeforeCursor = tc.before
		mixedType(s, tc.keys)
		for range tc.lefts {
			key(s, proto.KeyLeft)
		}
		snap := key(s, proto.KeyDown)
		if got := itemTexts(snap); len(got) == 0 || got[0] != tc.first || snap.Candidates.Title != tc.header {
			t.Fatalf("%s ↓：%q %q，要 %s 開頭、標題 %q", tc.name, snap.Candidates.Title, got, tc.first, tc.header)
		}
		snap = key(s, proto.KeyTab)
		if !tc.span {
			if got := itemTexts(snap); s.cand == nil || s.cand.span != nil || len(got) == 0 || got[0] != tc.first {
				t.Errorf("%s ↓ Tab 不該包到 2萬：%q", tc.name, got)
			}
			continue
		}
		want := []string{"2萬", "段", "斷", "ㄉㄨㄢˋ"}
		if tc.keys != "ji32j04" {
			want = []string{"2萬", "斷", "段", "ㄉㄨㄢˋ"} // 後面有 我，斷 段 照前後文排
		}
		if got := itemTexts(snap); !slices.Equal(got, want) || snap.Candidates.Title != title("2萬") {
			t.Fatalf("%s ↓ Tab：%q %q", tc.name, snap.Candidates.Title, got)
		}
		if got := itemTexts(key(s, proto.KeyTab)); len(got) == 0 || got[0] != tc.first {
			t.Errorf("%s ↓ Tab Tab 要回到 %s 開頭的清單：%q", tc.name, tc.first, got)
		}
	}
	s := splitSession(scores, nil)
	mixedType(s, "ji32j04")
	key(s, proto.KeyDown)
	key(s, proto.KeyTab)
	s.Handle(proto.Request{Type: proto.Select, Index: 1})
	if got := itemTexts(key(s, proto.KeyDown)); len(got) < 2 || got[0] != "段" || got[1] != "2萬" {
		t.Errorf("選 段 之後 ↓：%q，要 段、2萬…", got)
	}
	key(s, proto.KeyEscape)
	if commit := key(s, proto.KeyEnter).Commit; commit != "我段" {
		t.Errorf("選 段 送出 %q", commit)
	}
}

// 原按鍵只在同一串按鍵裡才列：萬我 是 j04｜ji3 兩串，沒有詞就不開這個詞長（官方行為）。
func TestRawKeysSameRunOnly(t *testing.T) {
	s := splitSession(map[string]float64{"2萬": 0, "段": -5}, nil)
	mixedType(s, "ji32j04ji3")
	key(s, proto.KeyLeft)
	key(s, proto.KeyLeft)
	key(s, proto.KeyDown)
	if got := itemTexts(key(s, proto.KeyTab)); s.cand == nil || s.cand.length != 1 || len(got) == 0 || got[0] != "萬" {
		t.Errorf("我2|萬我 ↓ Tab：%q，不該開只有 j04ji3 的清單", got)
	}
}

// 前段不是照字面切出去的（斷 是整串 2j04）：Tab 沒有同一串按鍵的清單，照舊換詞長。
func TestTabFlipSpanOnlyWithLiteralPrefix(t *testing.T) {
	s := splitSession(map[string]float64{"斷": 0, "2萬": -10}, nil)
	mixedType(s, "ji32j04")
	key(s, proto.KeyDown)
	if got := itemTexts(key(s, proto.KeyTab)); s.cand == nil || s.cand.span != nil {
		t.Errorf("斷 ↓ Tab 不該開同一串按鍵的清單：%q", got)
	}
}
