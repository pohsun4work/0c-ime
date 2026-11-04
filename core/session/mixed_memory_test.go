package session

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/core/decode"
	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/inputstyle"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

// wordSession 是整合風格的混打 session，詞庫與語言模型只有 words，英文字典只有 englishWords。
func wordSession(words []word, englishWords ...string) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range words {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, e.p)
	}
	s := New(personal.NewModel(lex, sys))
	s.InputStyle = blendRules()
	s.MixedInput, s.CtrlPunct = true, true
	var dict []english.Word
	for _, w := range englishWords {
		dict = append(dict, english.Word{Text: w, Weight: 3})
	}
	s.English = english.New(dict)
	return s
}

// memorySession：允 單獨很少見（mp3 換掉它），允許、單是常見的詞。
func memorySession() *Session {
	return wordSession([]word{
		{"我", "ㄨㄛˇ", 1e-2}, {"的", "ㄉㄜ˙", 2e-2}, {"你", "ㄋㄧˇ", 5e-3}, {"好", "ㄏㄠˇ", 4e-3}, {"允", "ㄩㄣˇ", 1e-7},
		{"許", "ㄒㄩˇ", 1e-4}, {"允許", "ㄩㄣˇ ㄒㄩˇ", 1e-3}, {"檔", "ㄉㄤˇ", 1e-4}, {"單", "ㄉㄢ", 1e-3}, {"喔", "ㄛ˙", 1e-4},
	}, "a", "is", "hello")
}

var (
	mp3Literal = []personal.MixedToken{{Keys: "mp3", Kind: personal.MixedLiteral}}
	mp3Chinese = []personal.MixedToken{{Keys: "mp3", Kind: personal.MixedChinese}}
)

func remember(s *Session, keys, left, right string, form []personal.MixedToken) {
	s.model.LearnMixedMemory([]personal.MixedChoice{{MixedRef: personal.MixedRef{Keys: keys, Left: left, Right: right}, Form: form}}, nil)
}

func memoryRecords(s *Session) []personal.MixedRecord {
	var out []personal.MixedRecord
	groups, _ := s.model.MixedMemoryIndex()
	for _, rs := range groups {
		out = append(out, rs...)
	}
	return out
}

// pickKeys 在句尾按 ↓，用 Tab 換到 n 個字長，選 text。
func pickKeys(t *testing.T, s *Session, n int, text string) {
	t.Helper()
	key(s, proto.KeyDown)
	for i := 0; s.cand != nil && s.cand.length != n && i < 8; i++ {
		key(s, proto.KeyTab)
	}
	if s.cand == nil || s.cand.length != n {
		t.Fatalf("換不到 %d 個字長", n)
	}
	pickText(t, s, text)
}

// mp3 出「允」時用 ↓ 選回原按鍵，送出時記住；同一個位置（前一個字、後一個音都一樣）再打就照記憶換。
func TestMixedMemoryLearnsChoiceAndAppliesAtSamePlace(t *testing.T) {
	s := memorySession()
	if snap := typeContext(s, "ji3mp3"); snap.Text() != "我允" {
		t.Fatalf("前提（官方）：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "mp3")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我mp3" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	rs := memoryRecords(s)
	if len(rs) != 1 || rs[0].MixedRef != (personal.MixedRef{Keys: "mp3", Left: "我"}) || !slices.Equal(rs[0].Form, mp3Literal) {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "ji3mp3"); snap.Text() != "我mp3" {
		t.Fatalf("同一個位置再打：%q", snap.Text())
	}
	if snap := key(s, proto.KeyEnter); snap.Commit != "我mp3" || memoryRecords(s)[0].Confirmed != 1 {
		t.Fatalf("照記憶換的送出不算再確認一次：%q %+v", snap.Commit, memoryRecords(s))
	}
}

// 我ㄇ（mixed_context 把接在中文後面的 a␣ 改成注音）用 ↓ 選回 a，記住後同一個位置照字面留著，規則不再改它。
func TestMixedMemoryKeepsLiteralAgainstRules(t *testing.T) {
	s := memorySession()
	if snap := typeContext(s, "ji3a "); snap.Text() != "我ㄇ" {
		t.Fatalf("前提（mixed_context）：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "a ")
	key(s, proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].Keys != "a " || rs[0].Form[0].Kind != personal.MixedLiteral {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "ji3a "); snap.Text() != "我a " {
		t.Fatalf("照記憶留著：%q", snap.Text())
	}
}

// 只有按鍵對上（前一個字不同）時看合理度：「的允」不像中文，照記憶換成 mp3；接著打出「允許」時換回中文。
func TestMixedMemoryBackoffByPlausibility(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	if snap := typeContext(s, "2k7mp3"); snap.Text() != "的mp3" {
		t.Fatalf("別的位置、允不合理：%q", snap.Text())
	}
	if snap := typeContext(s, "vm3"); snap.Text() != "的允許" {
		t.Fatalf("接著打出允許：%q", snap.Text())
	}
	// 送出時換成同樣子的每一筆都算用到（滿了不會先被丟）。
	s = memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	remember(s, "mp3", "你", "", mp3Literal)
	serial := func() (n uint64) {
		for _, r := range memoryRecords(s) {
			n = max(n, r.Serial)
		}
		return n
	}
	before := serial()
	typeContext(s, "2k7mp3", "Enter")
	for _, r := range memoryRecords(s) {
		if r.Serial <= before {
			t.Fatalf("只有按鍵對上，換成同樣子的每一筆都算用到：%+v", memoryRecords(s))
		}
	}
	// 每筆紀錄換成的樣子不一致時照官方。
	s = memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	remember(s, "mp3", "你", "", mp3Chinese)
	if snap := typeContext(s, "2k7mp3"); snap.Text() != "的允" {
		t.Fatalf("紀錄不一致：%q", snap.Text())
	}
}

// 照記憶換成中文（$20␣ 是 $單）：中文合理才換。
func TestMixedMemoryIntroducesChinese(t *testing.T) {
	s := memorySession()
	if snap := typeContext(s, "ji3$20 "); snap.Text() != "我$20 " {
		t.Fatalf("前提（官方）：%q", snap.Text())
	}
	pickKeys(t, s, 4, "$單")
	key(s, proto.KeyEnter)
	rs := memoryRecords(s)
	want := []personal.MixedToken{{Keys: "$", Kind: personal.MixedLiteral}, {Keys: "20 ", Kind: personal.MixedChinese}}
	if len(rs) != 1 || rs[0].Keys != "$20 " || rs[0].Left != "我" || !slices.Equal(rs[0].Form, want) {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "ji3$20 "); snap.Text() != "我$單" {
		t.Fatalf("同一個位置：%q", snap.Text())
	}
	key(s, proto.KeyEscape)
	if snap := typeContext(s, "su3$20 "); snap.Text() != "你$單" {
		t.Fatalf("別的位置、單合理：%q", snap.Text())
	}
}

// 照記憶換的地方用 ↓ 改回：前後都對上的那筆只確認過一次就刪掉，這次不記新的。
func TestMixedMemoryExactCorrectionContests(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	if snap := typeContext(s, "ji3mp3"); snap.Text() != "我mp3" {
		t.Fatalf("前提：%q", snap.Text())
	}
	pickKeys(t, s, 3, "允")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我允" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("改回後要刪掉、不記新的：%+v", rs)
	}
	if snap := typeContext(s, "ji3mp3"); snap.Text() != "我允" {
		t.Fatalf("刪掉之後照官方：%q", snap.Text())
	}
}

// 從別處套用的被改回：原本那筆不動，另記這個位置。
func TestMixedMemoryBackoffCorrectionAddsLocal(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	typeContext(s, "2k7mp3")
	pickKeys(t, s, 3, "允")
	key(s, proto.KeyEnter)
	rs := memoryRecords(s)
	slices.SortFunc(rs, func(a, b personal.MixedRecord) int { return strings.Compare(a.Left, b.Left) })
	if len(rs) != 2 || rs[0].Left != "我" || rs[0].Confirmed != 1 || rs[1].Left != "的" || !slices.Equal(rs[1].Form, mp3Chinese) {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "2k7mp3"); snap.Text() != "的允" || len(s.memory.applied) != 1 || !s.memory.applied[0].hold() {
		t.Fatalf("這個位置照新記的（跟官方一樣，不看合理度直接留著）：%q %+v", snap.Text(), s.memory.applied)
	}
}

// 前後都對上的記憶要換成別的樣子時也看合理度（學過「我的i7」，「好的喔」不換），門檻照那一筆自己的確認次數放寬。
func TestMixedMemoryExactNeedsPlausibility(t *testing.T) {
	for _, tc := range []struct {
		score     float64
		confirmed int
		want      string
	}{
		{-6, 1, "的mp3"},
		{-3.5, 1, "的允"},
		{-3.5, 3, "的mp3"}, // -4＋0.5×2＝-3
	} {
		s := memorySession()
		var calls int
		s.Rerank = rerank.Options{Weight: .1, TextWeight: 1, ContextRunes: rerank.MaxContextRunes, Scorer: memoryScorer(map[string]float64{"的允": tc.score}, &calls)}
		for range tc.confirmed {
			remember(s, "mp3", "的", "", mp3Literal)
		}
		if snap := typeContext(s, "2k7mp3"); snap.Text() != tc.want {
			t.Errorf("的允 %v、確認 %d 次：%q，要 %q", tc.score, tc.confirmed, snap.Text(), tc.want)
		}
	}
}

// ↓ 落在照記憶換過、或這次用 ↓ 換過解釋的段落時開整段（mp3／允…），Shift＋Tab 照樣換到較短的；其他英數照舊從最後一個字開始。
func TestMixedMemoryDownOpensWholeSpan(t *testing.T) {
	whole := func(t *testing.T, s *Session) {
		t.Helper()
		snap := key(s, proto.KeyDown)
		if items := itemTexts(snap); s.cand.start != 1 || s.cand.length != 3 || len(items) < 2 || items[0] != "mp3" || !slices.Contains(items, "允") {
			t.Fatalf("整段：start=%d length=%d %q", s.cand.start, s.cand.length, items)
		}
		if snap.Candidates.Title != "修改「mp3」 · Tab 切換詞長" {
			t.Fatalf("標題：%q", snap.Candidates.Title)
		}
	}
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	typeContext(s, "ji3mp3")
	whole(t, s)
	if key(s, proto.KeyBackTab); s.cand.start != 2 || s.cand.length != 2 {
		t.Fatalf("Shift＋Tab：start=%d length=%d", s.cand.start, s.cand.length)
	}
	s = memorySession()
	typeContext(s, "ji3mp3")
	key(s, proto.KeyDown)
	pickText(t, s, "mp3")
	whole(t, s)
	s = memorySession()
	if snap := typeContext(s, "ji3npm "); snap.Text() != "我npm " {
		t.Fatalf("前提（mixed_context）：%q", snap.Text())
	}
	if key(s, proto.KeyDown); s.cand.length != 1 {
		t.Fatalf("沒照記憶換的英數：length=%d", s.cand.length)
	}
}

// 候選窗反白照記憶換成的樣子時，Shift＋Delete 刪掉讓它換的紀錄、換回原本的樣子，這次組字不再換。
func TestMixedMemoryShiftDeleteForgets(t *testing.T) {
	s := memorySession()
	s.Preview02 = true
	remember(s, "mp3", "我", "", mp3Literal)
	typeContext(s, "ji3mp3")
	key(s, proto.KeyDown)
	snap := key(s, proto.KeyShiftDelete)
	if snap.Text() != "我允" || !snap.Forgot || !slices.Contains(snap.Notices, "不再自動換：mp3") {
		t.Fatalf("Shift＋Delete：%q forgot=%v %q", snap.Text(), snap.Forgot, snap.Notices)
	}
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("要刪掉：%+v", rs)
	}
	// 反白的不是畫面上的樣子時照官方（忘掉學到的詞）。
	s = memorySession()
	s.Preview02 = true
	remember(s, "mp3", "我", "", mp3Literal)
	typeContext(s, "ji3mp3")
	key(s, proto.KeyDown)
	if items := itemTexts(key(s, proto.KeyDown)); s.cand.length != 3 || len(items) < 2 || items[s.cand.cursor] == "mp3" {
		t.Fatalf("前提：反白 mp3 以外的項目：%q", items)
	}
	key(s, proto.KeyShiftDelete)
	if rs := memoryRecords(s); len(rs) != 1 {
		t.Fatalf("反白別的項目不刪記憶：%+v", rs)
	}
}

// 私人欄位照記憶換，但不學。
func TestMixedMemoryPrivateAppliesWithoutLearning(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	private := func(k string) proto.Snapshot { return s.Handle(proto.Request{Type: proto.Key, Key: k, Private: true}) }
	var snap proto.Snapshot
	for _, k := range "ji3mp3" {
		snap = private(string(k))
	}
	if snap.Text() != "我mp3" {
		t.Fatalf("私人欄位照記憶換：%q", snap.Text())
	}
	// 照記憶換的用 ↓ 改回（不是私人時會刪掉那筆）。
	private(proto.KeyDown)
	for s.cand != nil && s.cand.length != 3 {
		private(proto.KeyTab)
	}
	pickText(t, s, "允")
	private(proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].Confirmed != 1 {
		t.Fatalf("私人欄位不學：%+v", rs)
	}
}

// 組字區開頭的前一個字是外殼確認過的前文最後一個字。
func TestMixedMemoryLeftFromConfirmedContext(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	remember(s, "mp3", "", "", mp3Chinese) // 句首照官方，前文對上「我」時才換
	s.documentEnabled, s.Rerank.ContextRunes, s.neuralLeft = true, rerank.MaxContextRunes, "你和我"
	if snap := typeContext(s, "mp3"); snap.Text() != "mp3" {
		t.Fatalf("前文最後一個字是我：%q", snap.Text())
	}
}

// memoryScorer 是假的神經模型：逐字評分（scoreChars，候選沒有統計分數）時每個候選照 table（log10，前文＋字）給分，
// 沒列的給 -10；一般的單音節重排全給 0、不算次數。
func memoryScorer(table map[string]float64, calls *int) rerank.Scorer {
	return rankFunc(func(r rerank.Request) ([]float64, error) {
		if r.Mode != rerank.ModeText || slices.ContainsFunc(r.Candidates, func(h decode.Hypothesis) bool { return h.LogScore != 0 }) {
			return make([]float64, len(r.Candidates)), nil
		}
		*calls++
		out := make([]float64, len(r.Candidates))
		for i, c := range r.Candidates {
			v, ok := table[r.Context+c.Text]
			if !ok {
				v = -10
			}
			if math.IsNaN(v) {
				return nil, errors.New("worker 不能用")
			}
			out[i] = v * math.Ln10
		}
		return out, nil
	})
}

// 有神經模型時照前文評分（我的允不合理、很公允合理）；評分失敗時改用語言模型。
func TestMixedMemoryNeuralPlausibility(t *testing.T) {
	for _, tc := range []struct {
		table map[string]float64
		want  string
	}{
		{map[string]float64{"這首歌的允": -6}, "的mp3"},
		{map[string]float64{"這首歌的允": -2}, "的允"},
		{map[string]float64{"這首歌的允": math.NaN()}, "的mp3"}, // 語言模型：允單獨很少見
	} {
		s := memorySession()
		var calls int
		s.Rerank = rerank.Options{Weight: .1, TextWeight: 1, ContextRunes: rerank.MaxContextRunes, Scorer: memoryScorer(tc.table, &calls)}
		s.documentEnabled, s.neuralLeft = true, "這首歌"
		remember(s, "mp3", "我", "", mp3Literal)
		if snap := typeContext(s, "2k7mp3"); snap.Text() != tc.want {
			t.Errorf("%v：%q，要 %q", tc.table, snap.Text(), tc.want)
		}
		if calls == 0 {
			t.Errorf("%v：沒有問神經模型", tc.table)
		}
	}
}

// 合理度門檻：同一個位置每多確認一次、附近有英文都比較容易把中文換掉；別的位置確認再多次也照門檻。
func TestMixedMemoryThresholdAdjustments(t *testing.T) {
	for _, tc := range []struct {
		keys, left string
		confirmed  int
		want       string
	}{
		{"2k7mp3", "的", 1, "的允"},
		{"2k7mp3", "的", 3, "的mp3"},             // -4＋0.5×2＝-3
		{"2k7mp3", "我", 3, "的允"},               // 別的位置：-4
		{"hello 2k7mp3", "我", 1, "hello 的mp3"}, // -4＋1＝-3
	} {
		s := memorySession()
		var calls int
		s.Rerank = rerank.Options{Weight: .1, TextWeight: 1, ContextRunes: rerank.MaxContextRunes, Scorer: memoryScorer(map[string]float64{"的允": -3.5, "hello 的允": -3.5}, &calls)}
		for range tc.confirmed {
			remember(s, "mp3", tc.left, "", mp3Literal)
		}
		if snap := typeContext(s, tc.keys); snap.Text() != tc.want {
			t.Errorf("%q 在「%s」後確認 %d 次：%q，要 %q", tc.keys, tc.left, tc.confirmed, snap.Text(), tc.want)
		}
	}
}

// 兩邊都沒有中文的記憶直接換，但附近有英文時不換成注音；單一個注音鍵接空白（a␣ ↔ ㄇ）交給官方選過的注音文，
// 不照記憶換。也不從英文字中間切出來。
func TestMixedMemoryZhuyinNotInEnglish(t *testing.T) {
	a, one := personal.MixedToken{Keys: "a ", Kind: personal.MixedZhuyin}, personal.MixedToken{Keys: "1 ", Kind: personal.MixedZhuyin}
	for _, tc := range []struct {
		n, keys, left, typed, want string
		form                       []personal.MixedToken
	}{
		{"句首", "a 1 ", "", "a 1 ", "ㄇㄅ", []personal.MixedToken{a, one}},
		{"附近有英文", "a 1 ", " ", "hello a 1 ", "hello a 1 ", []personal.MixedToken{a, one}},
		{"單一個注音鍵", "a ", "", "a ", "a ", []personal.MixedToken{a}},
	} {
		s := memorySession()
		s.InputStyle.MixedContext = inputstyle.ContextInitial // 不讓注音文的規則先換
		remember(s, tc.keys, tc.left, "", tc.form)
		if snap := typeContext(s, tc.typed); snap.Text() != tc.want {
			t.Errorf("%s：%q，要 %q", tc.n, snap.Text(), tc.want)
		}
	}
	s := memorySession()
	remember(s, "is ", "h", "", []personal.MixedToken{{Keys: "is ", Kind: personal.MixedChinese}})
	if !s.memoryBoundary(0) {
		t.Fatal("空的組字區")
	}
	typeContext(s, "his ")
	if s.memoryBoundary(1) || !s.memoryBoundary(0) {
		t.Fatal("英文字中間不是一段的開頭")
	}
}

// 插入音節解不出來而整個還原（saveSyllables）時，照記憶換過的段落跟著還原，位置不會多移一格。
func TestMixedMemorySurvivesRestoredInsert(t *testing.T) {
	s := memorySession()
	remember(s, "mp3", "我", "", mp3Literal)
	typeContext(s, "ji3mp3")
	want := []memoryApplied{s.memory.applied[0]}
	if want[0].memorySpan != (memorySpan{1, 4}) {
		t.Fatalf("前提：%+v", s.memory.applied)
	}
	saved := s.saveSyllables()
	s.cursor = 0
	s.insert("ㄟˇ")
	s.restoreSyllables(saved)
	if !slices.EqualFunc(s.memory.applied, want, func(a, b memoryApplied) bool { return a.memorySpan == b.memorySpan }) {
		t.Fatalf("還原後：%+v", s.memory.applied)
	}
}

// reversedSession 收了反序打得出的字（ps4 是嫩、u2 是低、i16 是博），照語言模型都夠合理。
func reversedSession() *Session {
	return wordSession([]word{
		{"我", "ㄨㄛˇ", 1e-2}, {"很", "ㄏㄣˇ", 5e-3}, {"嫩", "ㄋㄣˋ", 1e-3}, {"低", "ㄉㄧ", 1e-3}, {"博", "ㄅㄛˊ", 1e-3},
		{"億", "ㄧˋ", 1e-3}, {"允", "ㄩㄣˇ", 1e-3},
	}, "hello")
}

// 混打修正記憶開著時，字母開頭的反序在打聲調或空白之前照字面（Enter 送出字母），打了就是中文，句首、英文後面也是；
// 數字開頭照官方，官方表上的英數詞（ps4）照字面。個人英文認得的英數詞交給混打記憶，不再一律照字面。
func TestMixedReversedBecomesChineseAfterTone(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"u2", "u2"}, {"u2 ", "低"}, {"cp3u2", "很u2"}, {"cp3u2 ", "很低"}, {"hello u2 ", "hello 低"}, {"i16", "博"},
		{"ps4", "ps4"}, {"70u4", "70億"},
	} {
		s := reversedSession()
		mixedType(s, tc.keys)
		if got := key(s, proto.KeyEnter).Commit; got != tc.want {
			t.Errorf("%q：%q，要 %q", tc.keys, got, tc.want)
		}
	}
	s := reversedSession()
	s.model.Store.English = map[string]personal.EnglishEvidence{
		"mp3": {Text: "mp3", Confirmed: 3, Days: 2, Updated: s.model.Store.Time, Day: s.model.Store.Time / 86400},
	}
	if snap := mixedType(s, "mp3"); snap.Text() != "允" {
		t.Errorf("個人英文認得的 mp3：%q", snap.Text())
	}
}

// 照字面的 ps4 用 ↓ 選「嫩」送出就記住，同一個位置再打時打完聲調就換（Enter 前畫面就是嫩）；用 ↓ 選回 ps4 抵銷，↓ 照樣列嫩。
func TestMixedMemoryPendingAlnum(t *testing.T) {
	s := reversedSession()
	if snap := typeContext(s, "cp3ps4"); snap.Text() != "很ps4" {
		t.Fatalf("前提（官方）：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "嫩")
	if snap := key(s, proto.KeyEnter); snap.Commit != "很嫩" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].MixedRef != (personal.MixedRef{Keys: "ps4", Left: "很"}) || !slices.Equal(rs[0].Form, []personal.MixedToken{{Keys: "ps4", Kind: personal.MixedChinese}}) {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "cp3ps4"); snap.Text() != "很嫩" {
		t.Fatalf("同一個位置再打：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "ps4")
	key(s, proto.KeyEnter)
	if snap := typeContext(s, "cp3ps4"); snap.Text() != "很ps4" {
		t.Fatalf("選回後：%q", snap.Text())
	}
	if items := itemTexts(key(s, proto.KeyDown)); !slices.Contains(items, "嫩") {
		t.Fatalf("↓：%q", items)
	}

	// 句首只有待定按鍵時，前一個字是確認過的前文；畫面跟送出一樣。
	s = reversedSession()
	remember(s, "ps4", "很", "", []personal.MixedToken{{Keys: "ps4", Kind: personal.MixedChinese}})
	s.documentEnabled, s.Rerank.ContextRunes, s.neuralLeft = true, rerank.MaxContextRunes, "牛排很"
	if snap := typeContext(s, "ps4"); snap.Text() != "嫩" {
		t.Fatalf("前文是很的句首：%q", snap.Text())
	}
}

// 單一個注音鍵接空白組成的字（g␣ 是詩）用 ↓ 選回原按鍵：同官方選過的注音文只記前一個字，前面是英數時不記；
// 再從官方清單選注音就忘掉，照官方出注音。
func TestMixedMemoryLoneKeyChinese(t *testing.T) {
	s := wordSession([]word{{"我", "ㄨㄛˇ", 1e-2}, {"詩", "ㄕ", 1e-3}, {"天", "ㄊㄧㄢ", 1e-3}}, "hello")
	if snap := typeContext(s, "hello g "); snap.Text() != "hello 詩" {
		t.Fatalf("前提：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "g ")
	key(s, proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("英數後面不記：%+v", rs)
	}
	typeContext(s, "ji3g wu0 ")
	downPick(t, s, 1, "g ")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我g 天" {
		t.Fatalf("送出：%q", snap.Commit)
	}
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].MixedRef != (personal.MixedRef{Keys: "g ", Left: "我"}) || rs[0].Confirmed != 1 {
		t.Fatalf("記住的：%+v", rs)
	}
	typeContext(s, "ji3g ")
	key(s, proto.KeyDown)
	pickText(t, s, "ㄕ")
	key(s, proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("選注音要忘掉：%+v", rs)
	}
	if snap := typeContext(s, "ji3g "); snap.Text() != "我ㄕ" {
		t.Fatalf("忘掉後：%q", snap.Text())
	}
}

// 注音文把 我2␣ 換成 我ㄉ；用 ↓ 選回 2␣ 送出就記住，同一個前字不再換、別的前字照換，再選回 ㄉ 就忘掉。
func TestMixedMemoryZhuyinDecline(t *testing.T) {
	s := memorySession()
	if snap := typeContext(s, "ji32 "); snap.Text() != "我ㄉ" {
		t.Fatalf("前提（注音文）：%q", snap.Text())
	}
	key(s, proto.KeyDown)
	pickText(t, s, "2 ")
	if snap := key(s, proto.KeyEnter); snap.Commit != "我2 " {
		t.Fatalf("送出：%q", snap.Commit)
	}
	if rs := memoryRecords(s); len(rs) != 1 || rs[0].MixedRef != (personal.MixedRef{Keys: "2 ", Left: "我"}) || rs[0].Form[0].Kind != personal.MixedLiteral {
		t.Fatalf("記住的：%+v", rs)
	}
	if snap := typeContext(s, "su32 "); snap.Text() != "你ㄉ" {
		t.Fatalf("別的前字：%q", snap.Text())
	}
	key(s, proto.KeyEnter)
	if snap := typeContext(s, "ji32 "); snap.Text() != "我2 " {
		t.Fatalf("同一個前字：%q", snap.Text())
	}
	downPick(t, s, 1, "ㄉ")
	key(s, proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("選回 ㄉ 要忘掉：%+v", rs)
	}
	if snap := typeContext(s, "ji32 "); snap.Text() != "我ㄉ" {
		t.Fatalf("忘掉後：%q", snap.Text())
	}

	s = memorySession()
	remember(s, "2 ", "我", "", []personal.MixedToken{{Keys: "2 ", Kind: personal.MixedLiteral}})
	s.InputStyle.MixedContext = inputstyle.ContextInitial
	typeContext(s, "ji32 ")
	downPick(t, s, 1, "ㄉ")
	key(s, proto.KeyEnter)
	if rs := memoryRecords(s); len(rs) != 0 {
		t.Fatalf("注音文關著時選 ㄉ 也要忘掉：%+v", rs)
	}
}
