package session

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/suggest"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

type textsFunc func(rerank.TextsRequest) ([]float64, error)

func (f textsFunc) ScoreTexts(r rerank.TextsRequest, _ time.Duration) ([]float64, error) { return f(r) }

// splitSession 的英文字典有 hello、go、zip、up、upon、the、thew、a、as；評分器照 scores 給分（沒列的 -100），收到的請求記在 seen。
func splitSession(scores map[string]float64, seen *[]rerank.TextsRequest) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct{ w, r string }{
		{"我", "ㄨㄛˇ"}, {"你", "ㄋㄧˇ"}, {"央", "ㄧㄤ"}, {"允", "ㄩㄣˇ"}, {"分", "ㄈㄣ"}, {"嗯", "ㄣ"}, {"誰", "ㄕㄟˊ"}, {"音", "ㄧㄣ"},
		{"停", "ㄊㄧㄥˊ"}, {"營", "ㄧㄥˊ"}, {"趴", "ㄆㄚ"}, {"班", "ㄅㄢ"}, {"之", "ㄓ"}, {"張", "ㄓㄤ"}, {"萬", "ㄨㄢˋ"},
		{"斷", "ㄉㄨㄢˋ"}, {"正", "ㄓㄥˋ"},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, 1e-3)
	}
	lex.Add(lexicon.Entry{Surface: "因", Reading: zhuyin.ParseReading("ㄧㄣ"), PRW: 1}) // 音 的同音字、比較少用
	sys.SetUni("因", 1e-4)
	lex.Add(lexicon.Entry{Surface: "段", Reading: zhuyin.ParseReading("ㄉㄨㄢˋ"), PRW: 1}) // 斷 的同音字、比較少用
	sys.SetUni("段", 1e-4)
	s := New(personal.NewModel(lex, sys))
	var words []english.Word
	for _, w := range []string{"hello", "go", "zip", "up", "upon", "the", "thew", "a", "as"} {
		words = append(words, english.Word{Text: w, Weight: 65})
	}
	s.English = english.New(words)
	if scores != nil {
		s.Rerank = rerank.Options{Weight: 2, ContextRunes: rerank.MaxContextRunes, Texts: textsFunc(func(r rerank.TextsRequest) ([]float64, error) {
			if seen != nil {
				*seen = append(*seen, r)
			}
			out := make([]float64, len(r.Texts))
			for i, t := range r.Texts {
				out[i] = -100
				if v, ok := scores[t]; ok {
					out[i] = v
				}
			}
			return out, nil
		})}
	}
	return s
}

// 神經選到的切法跟規則不同時換掉規則結果，包括換成照字面、照順序組成多個音節。
func TestMixedNeuralChoosesSplit(t *testing.T) {
	for _, tc := range []struct {
		name, keys string
		scores     map[string]float64
		want       string
	}{
		{"沒有評分器照規則", "helloji3", nil, "hello我"},
		{"規則的切法", "helloji3", map[string]float64{"hello我": 0, "helloji3": -10}, "hello我"},
		{"換成照字面", "helloji3", map[string]float64{"helloji3": 0, "hello我": -10}, "helloji3"},
		{"英數詞照字面", "mp3", map[string]float64{"mp3": 0, "允": -10}, "mp3"},
		{"英數詞出字", "mp3", map[string]float64{"允": 0, "mp3": -10}, "允"},
		{"英文字換回英文、空白還回來", "up ", map[string]float64{"up": 0, "音": -10}, "up "},
		{"英文字照規則出字", "up ", map[string]float64{"音": 0, "up": -10}, "音"},
		// 第一次（qji3）照字面留著；接著打完 su3 時切成兩個音節（規則是 qji3你）
		{"多個音節", "qji3su3", map[string]float64{"qji3": 0, "q我你": 0, "qji3你": -50}, "q我你"},
	} {
		if got := repairCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
}

// 各切法比所有同音字的機率加總：引擎選的代表 音 單獨輸給 up，音＋因 加起來贏。
func TestMixedNeuralSumsHomophones(t *testing.T) {
	if got := repairCommit(splitSession(map[string]float64{"up": -5, "音": -5.5, "因": -5.5}, nil), "up "); got != "音" {
		t.Errorf("送出 %q，要 音", got)
	}
	if got := repairCommit(splitSession(map[string]float64{"up": -5, "音": -6, "因": -6}, nil), "up "); got != "up " {
		t.Errorf("加起來也輸：送出 %q，要 up␣", got)
	}
}

// 修正切法扣 mixedRepairPenalty：su;;␣ 的 s央（合併冗餘的 ;）要比照字面高出 6 分以上才換；規則結果也是修正時不扣。
func TestMixedNeuralRepairPenalty(t *testing.T) {
	if got := repairCommit(splitSession(map[string]float64{"su;;": -10, "s央": -6}, nil), "su;; "); got != "su;; " {
		t.Errorf("差 4 分不換：送出 %q", got)
	}
	if got := repairCommit(splitSession(map[string]float64{"su;;": -10, "s央": -3}, nil), "su;; "); got != "s央" {
		t.Errorf("差 7 分要換：送出 %q", got)
	}
	// 規則結果本身不扣：zsuu3 的 z你（合併冗餘音、切在不是詞的 z 後面）贏照字面就留著。
	if got := repairCommit(splitSession(map[string]float64{"z你": -10, "zsuu3": -14}, nil), "zsuu3"); got != "z你" {
		t.Errorf("規則結果不扣分：送出 %q", got)
	}
	// 規則的 thew營 也是合併冗餘音：the停 不扣分，高一點就換。
	if got := repairCommit(splitSession(map[string]float64{"thew營": -10, "the停": -8}, nil), "thewuuu/6"); got != "the停" {
		t.Errorf("修正對修正：送出 %q", got)
	}
}

// 不問神經的：純中文（規則整串照順序組成一個音節，u;␣ 的 ; 是注音 ㄤ）、規則照字面的英文字典詞（zip␣ 不是 分）。
// 照順序就組得成同一個讀音時不送修正候選（asu3 有 a你，不送丟掉 a 的 你）。
func TestMixedNeuralRequests(t *testing.T) {
	for _, keys := range []string{"ji3su3", "u; ", "zip "} {
		var seen []rerank.TextsRequest
		if got := repairCommit(splitSession(map[string]float64{}, &seen), keys); len(seen) != 0 {
			t.Fatalf("%q（送出 %q）不該問神經：%+v", keys, got, seen)
		}
	}
	var seen []rerank.TextsRequest
	repairCommit(splitSession(map[string]float64{}, &seen), "asu3")
	if len(seen) != 1 || !slices.Contains(seen[0].Texts, "a你") || slices.Contains(seen[0].Texts, "你") {
		t.Fatalf("asu3 candidates: %+v", seen)
	}
	// 前文是組字區游標前的字。
	seen = nil
	repairCommit(splitSession(map[string]float64{}, &seen), "ji3helloji3")
	if len(seen) != 1 || seen[0].Context != "我" || !slices.Contains(seen[0].Texts, "hello我") {
		t.Fatalf("context: %+v", seen)
	}
}

// 評分器失敗或超過期限時照規則。
func TestMixedNeuralFallsBack(t *testing.T) {
	s := splitSession(nil, nil)
	s.Rerank = rerank.Options{Weight: 2, Texts: textsFunc(func(rerank.TextsRequest) ([]float64, error) { return nil, rerank.ErrDeadline })}
	if got := repairCommit(s, "helloji3"); got != "hello我" {
		t.Errorf("deadline 送出 %q", got)
	}
}

// 神經判成中文的決定，之後帶著後文再比一次：mp3 先出 允，接著打的字讓 mp3 比較好時換回來；
// 空白被當一聲用掉的 up 換回英文時，空白還回來。原本的樣子比較好時不動。
func TestMixedNeuralRevisit(t *testing.T) {
	for _, tc := range []struct {
		name, keys string
		scores     map[string]float64
		want       string
	}{
		{"換回英數詞", "mp3ji3", map[string]float64{"允": 0, "mp3": -5, "mp3我": 0, "允我": -10}, "mp3我"},
		{"換回後接著打", "mp3ji3su3", map[string]float64{"允": 0, "mp3": -5, "mp3我": 0, "允我": -10}, "mp3我你"},
		{"換回英文、空白還回來", "up ji3", map[string]float64{"音": 0, "up": -5, "up 我": 0, "音我": -10}, "up 我"},
		{"原本的比較好", "mp3ji3", map[string]float64{"允": 0, "mp3": -5, "允我": 0, "mp3我": -10}, "允我"},
		{"決定時差太多不回頭比", "mp3ji3", map[string]float64{"允": 0, "mp3": -20, "mp3我": 0, "允我": -10}, "允我"},
		// 前面有字、後文兩個字：比的是 允我你 對 mp3我你（前文是 你）。
		{"前面有字", "su3mp3ji3su3", map[string]float64{"允": 0, "mp3": -5, "允我你": 0, "mp3我你": -5}, "你允我你"},
	} {
		if got := repairCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
}

// 照原樣接空白的決定也回頭比：aq8␣ 神經判成照字面，接著打的 我 讓 a趴 比較好時換掉，空白收回來當一聲。
func TestMixedNeuralRevisitLiteral(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		want   string
	}{
		{"換成中文", map[string]float64{"aq8": 0, "a趴": -2, "a趴我": 0, "aq8 我": -10}, "a趴我"},
		{"照字面比較好", map[string]float64{"aq8": 0, "a趴": -2, "aq8 我": 0, "a趴我": -10}, "aq8 我"},
		{"決定時差太多不回頭比", map[string]float64{"aq8": 0, "a趴": -20, "a趴我": 0, "aq8 我": -10}, "aq8 我"},
	} {
		if got := repairCommit(splitSession(tc.scores, nil), "aq8 ji3"); got != tc.want {
			t.Errorf("%s：送出 %q，要 %q", tc.name, got, tc.want)
		}
	}
}

// 評分超過期限時，連同那一鍵共 mixedBusyKeys 鍵不問神經，之後再試。
func TestMixedNeuralBusyBackoff(t *testing.T) {
	calls := 0
	s := splitSession(nil, nil)
	s.Rerank = rerank.Options{Weight: 2, Texts: textsFunc(func(rerank.TextsRequest) ([]float64, error) {
		calls++
		return nil, rerank.ErrDeadline
	})}
	// 第 2 鍵（up）問了、超過期限，第 3～9 鍵不問（第 10 鍵的 u 本來就不問）。
	if got := mixedType(s, "up up up u").Text(); calls != 1 || got != "音音音ㄧ" {
		t.Fatalf("忙時照規則、不問神經：顯示 %q，問了 %d 次", got, calls)
	}
	if mixedType(s, "p"); calls != 2 {
		t.Errorf("第 11 鍵再試：問了 %d 次", calls)
	}
}

// 回頭比最多 mixedRevisitWindow 次，換過就固定。
func TestMixedNeuralRevisitWindow(t *testing.T) {
	var seen []rerank.TextsRequest
	repairCommit(splitSession(map[string]float64{"允": 0, "mp3": -5}, &seen), "mp3ji3su3ji3su3ji3su3")
	if len(seen) != 1+mixedRevisitWindow {
		t.Fatalf("要 1 次決定＋%d 次回頭，問了 %d 次", mixedRevisitWindow, len(seen))
	}
	seen = nil
	repairCommit(splitSession(map[string]float64{"允": 0, "mp3": -5, "mp3我": 0}, &seen), "mp3ji3su3ji3")
	if len(seen) != 2 {
		t.Fatalf("換過就固定：問了 %d 次", len(seen))
	}
}

// 組字區照空白的結果顯示：規則接空白切成英文＋中文的（aq8␣ 是 a趴）顯示 aㄆㄚ，神經判成照字面時顯示原按鍵；
// 神經判成英文的 up 顯示 up，Tab 在它跟規則的 ㄧㄣ 之間換。空白直接用顯示時的決定，不再問一次神經。
func TestMixedDisplayFollowsSpace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		want   string
	}{
		{"規則", nil, "aㄆㄚ"},
		{"神經照規則", map[string]float64{"a趴": 0, "aq8": -10}, "aㄆㄚ"},
		{"神經照字面", map[string]float64{"aq8": 0, "a趴": -10}, "aq8"},
	} {
		s := splitSession(tc.scores, nil)
		if got := mixedType(s, "aq8").Text(); got != tc.want {
			t.Errorf("%s：aq8 顯示 %q，要 %q", tc.name, got, tc.want)
		}
	}
	var seen []rerank.TextsRequest
	s := splitSession(map[string]float64{"up": 0, "音": -1}, &seen) // 沒把握，有小視窗 Tab 才換
	if got := mixedType(s, "up").Text(); got != "up" {
		t.Fatalf("神經判成英文：顯示 %q", got)
	}
	// 不是按鍵的請求（不能問神經）不換掉神經的決定。
	if got := s.Handle(proto.Request{Type: proto.Display, Columns: 5}).Text(); got != "up" {
		t.Errorf("欄數請求之後：%q", got)
	}
	if got := key(s, proto.KeyTab).Text(); got != "ㄧㄣ" {
		t.Errorf("Tab 換成規則的注音：%q", got)
	}
	for i := range 3 { // Tab 換了就定了，再按不動
		if got := key(s, proto.KeyTab).Text(); got != "ㄧㄣ" {
			t.Errorf("第 %d 次 Tab：%q", i+2, got)
		}
	}
	n := len(seen)
	if got := key(s, " "); got.Text() != "音" || len(seen) != n {
		t.Errorf("空白照 Tab 選的、不再問神經：%q，多問了 %d 次", got.Text(), len(seen)-n)
	}
}

// 顯示時神經沒比到（超過期限）的決定，忙時退讓結束後按空白時再問一次。
func TestMixedSpaceRetriesUndecided(t *testing.T) {
	fail := true
	s := splitSession(nil, nil)
	s.Rerank = rerank.Options{Weight: 2, ContextRunes: rerank.MaxContextRunes, Texts: textsFunc(func(r rerank.TextsRequest) ([]float64, error) {
		if fail {
			return nil, rerank.ErrDeadline
		}
		out := make([]float64, len(r.Texts))
		for i, t := range r.Texts {
			if out[i] = -10; t == "up" {
				out[i] = 0
			}
		}
		return out, nil
	})}
	if got := mixedType(s, "up").Text(); got != "ㄧㄣ" {
		t.Fatalf("超過期限照規則：顯示 %q", got)
	}
	fail, s.mixedBusy = false, 0
	if got := key(s, " ").Text(); got != "up " {
		t.Errorf("空白再問神經：%q", got)
	}
}

// 只有數字的待定按鍵（10␣ 的 班 對 10）、數字接注音（205;␣ 的 20張）、數字鍵開頭的整個音節（2j04 的 斷 對 2萬）也交給神經比。
func TestMixedNeuralDigits(t *testing.T) {
	for _, tc := range []struct {
		name, keys string
		scores     map[string]float64
		want       string
	}{
		{"沒有評分器照規則", "10 ", nil, "班"},
		{"數字照字面", "10 ", map[string]float64{"10": 0, "班": -10}, "10 "},
		{"數字出字", "10 ", map[string]float64{"班": 0, "10": -10}, "班"},
		// 純數字緊接在中文後面、規則出中文的照規則（上 10␣ 是 上班）；後面有注音字母的照樣比（可是要 2j04 的 2萬），
		// 規則照字面的也照樣比（205; 對 20張）。
		{"中文後面的數字照規則", "ji310 ", map[string]float64{"10": 0, "班": -10}, "我班"},
		{"中文後面的數字鍵開頭問神經", "ji32j04", map[string]float64{"2萬": 0, "斷": -10}, "我2萬"},
		{"中文後面的數字鍵開頭神經出字", "ji32j04", map[string]float64{"斷": 0, "2萬": -10}, "我斷"},
		{"數字接注音", "ji3205; ", map[string]float64{"20張": 0, "205;": -10}, "我20張"},
		{"數字鍵開頭的音節", "2j04", map[string]float64{"2萬": 0, "斷": -10}, "2萬"},
		{"數字鍵開頭照規則", "2j04", map[string]float64{"斷": 0, "2萬": -10}, "斷"},
		// 之 先贏；接著打的 分 讓 5 分 比較好時換回數字，空白還回來。
		{"回頭換回數字", "5 zp ", map[string]float64{"之": 0, "5": -5, "5 分": 0, "之分": -10}, "5 分"},
		{"數字寫法照規則", "15/4", map[string]float64{"1正": 0, "15/4": -10}, "15/4"},
		// 後面 10␣ 的 班 也等著回頭比時，mp3 的 允 照樣等：接著打的 班 讓 mp3 翻回來（下載了 mp3 但）。
		{"回頭比同時記兩個", "mp3 10 ", map[string]float64{"允": 0, "mp3": -5, "班": 0, "10": -3, "mp3 班": 0, "允 班": -10}, "mp3 班"},
	} {
		if got := repairCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
	// 純中文沒有數字鍵開頭的切法時照舊不問。
	var seen []rerank.TextsRequest
	if repairCommit(splitSession(map[string]float64{}, &seen), "ji3"); len(seen) != 0 {
		t.Errorf("ji3 不該問神經：%+v", seen)
	}
}

// 回頭比同時最多等 mixedRevisitSlots 個決定，多的放掉最有把握的；前面的換掉時後面的跟著移，下一鍵照樣比。
func TestMixedNeuralRevisitSlots(t *testing.T) {
	s := splitSession(map[string]float64{"允": 0, "mp3": -5, "班": 0, "10": -3, "之": 0, "5": -2}, nil)
	mixedType(s, "mp3 10  5 ")
	var raws []string
	for _, v := range s.mixedRevisits {
		raws = append(raws, v.raw)
	}
	if !slices.Equal(raws, []string{"10", "5"}) {
		t.Errorf("要放掉贏最多的 允：留著 %q", raws)
	}
	// 我 讓 mp3 翻回來（班 往後移兩格），你 再讓 10 翻回來。
	s = splitSession(map[string]float64{"允": 0, "mp3": -5, "班": 0, "10": -3, "mp3 班我": 0, "允 班我": -10, "班我你": -10, "10 我你": 0}, nil)
	if got := repairCommit(s, "mp3 10 ji3su3"); got != "mp3 10 我你" {
		t.Errorf("前面換掉後面跟著移：送出 %q", got)
	}
}

// 這一鍵已經為決定問過神經時，回頭比留到下一鍵（同一鍵評分、換字、官方重排加起來會超過期限）。
func TestMixedNeuralRevisitWaitsAskedKey(t *testing.T) {
	var seen []rerank.TextsRequest
	s := splitSession(map[string]float64{"允": 0, "mp3": -5}, &seen)
	mixedType(s, "mp3 mp")
	n := len(seen)
	if mixedType(s, "3"); len(seen) != n+1 {
		t.Fatalf("決定的那一鍵只問一次：問了 %d 次", len(seen)-n)
	}
	if mixedType(s, "ji3"); len(seen) != n+3 {
		t.Errorf("下一鍵兩個決定都回頭比：問了 %d 次", len(seen)-n-1)
	}
}

// 數字照空白的決定顯示：判成數字顯示 10、判成中文直接顯示注音 ㄅㄢ（Enter 照畫面送出）。決定有把握時不跳小視窗、
// Tab 不動；沒把握時跳，Tab 換成另一個，換了就定了、再按不動。
func TestMixedDigitsDisplayFollowsSpace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		shown  string
	}{
		{"判成數字", map[string]float64{"10": 0, "班": -10}, "10"},
		{"判成中文", map[string]float64{"班": 0, "10": -10}, "ㄅㄢ"},
	} {
		s := splitSession(tc.scores, nil)
		if snap := mixedType(s, "10"); snap.Text() != tc.shown || snap.Suggestion != "" {
			t.Errorf("%s：顯示 %q、小視窗 %q", tc.name, snap.Text(), snap.Suggestion)
		}
		if got := key(s, proto.KeyTab).Text(); got != tc.shown {
			t.Errorf("%s：沒有小視窗時 Tab %q", tc.name, got)
		}
		if got := key(s, proto.KeyEnter).Commit; got != tc.shown {
			t.Errorf("%s：Enter %q", tc.name, got)
		}
	}

	s := splitSession(map[string]float64{"班": 0, "10": -1}, nil)
	if snap := mixedType(s, "10"); snap.Text() != "ㄅㄢ" || snap.Suggestion != "10" {
		t.Fatalf("沒把握：顯示 %q、小視窗 %q", snap.Text(), snap.Suggestion)
	}
	for i := range 2 {
		if got := key(s, proto.KeyTab); got.Text() != "10" || got.Suggestion != "" {
			t.Errorf("第 %d 次 Tab：%q、小視窗 %q", i+1, got.Text(), got.Suggestion)
		}
	}
	if got := key(s, " ").Text(); got != "10 " {
		t.Errorf("Tab 換成數字後空白：%q", got)
	}
}

// 神經判成英文照英文顯示（up）：贏規則的注音夠多時不跳小視窗（補全 upon 也不給：多半是打到一半的注音）、Tab 不動，
// 差不多時才跳、Tab 換成注音。
func TestMixedPopupOnlyWhenUnsure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		popup  string
	}{
		{"有把握", map[string]float64{"up": 0, "音": -10}, ""},
		{"沒把握", map[string]float64{"up": 0, "音": -1}, "ㄧㄣ"},
	} {
		s := splitSession(tc.scores, nil)
		if snap := mixedType(s, "up"); snap.Text() != "up" || snap.Suggestion != tc.popup {
			t.Errorf("%s：顯示 %q、小視窗 %q，要 %q", tc.name, snap.Text(), snap.Suggestion, tc.popup)
		}
		if got, want := key(s, proto.KeyTab).Text(), cmp.Or(tc.popup, "up"); got != want {
			t.Errorf("%s：Tab %q，要 %q", tc.name, got, want)
		}
	}
}

// 神經判成英文、換不回注音的（hep 規則切成 he嗯）照常補全，有把握也跳（hepatic）；換得回注音的見 TestMixedPopupOnlyWhenUnsure。
func TestMixedSureEnglishStillCompletes(t *testing.T) {
	s := splitSession(map[string]float64{"hep": 0, "he嗯": -10}, nil)
	s.English = english.New([]english.Word{{Text: "hepatic", Weight: 65}})
	if snap := mixedType(s, "hep"); snap.Text() != "hep" || snap.Suggestion != "hepatic" {
		t.Fatalf("顯示 %q、小視窗 %q", snap.Text(), snap.Suggestion)
	}
}

// 混打剛決定、還在等後文回頭比的字（我up␣ 的 音）不給官方補全（音樂）：之後可能換回 up。決定有把握的照補。
func TestMixedRevisitSkipsCompletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores map[string]float64
		popup  string
	}{
		{"等回頭比", map[string]float64{"音": 0, "up": -5}, ""},
		{"有把握", map[string]float64{"音": 0, "up": -20}, "音樂"},
	} {
		s := splitSession(tc.scores, nil)
		s.Opt.Suggest = suggest.DefaultOptions()
		s.Opt.Suggest.AttentionCost = 0
		s.model.Lex.(*lexicon.Mem).Add(lexicon.Entry{Surface: "音樂", Reading: zhuyin.ParseReading("ㄧㄣ ㄩㄝˋ"), PRW: 1})
		s.model.LM.(*lm.Mem).SetUni("音樂", 0.01)
		if snap := mixedType(s, "ji3up "); snap.Text() != "我音" || snap.Suggestion != tc.popup {
			t.Errorf("%s：%q 小視窗 %q，要 %q", tc.name, snap.Text(), snap.Suggestion, tc.popup)
		}
	}
}
