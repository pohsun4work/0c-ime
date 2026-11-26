package session

import (
	"slices"
	"testing"
	"time"

	"github.com/0c-biggo/0c-ime/core/english"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

type textsFunc func(rerank.TextsRequest) ([]float64, error)

func (f textsFunc) ScoreTexts(r rerank.TextsRequest, _ time.Duration) ([]float64, error) { return f(r) }

// splitSession 的英文字典有 hello、go、zip、up、the、thew、a、as；評分器照 scores 給分（沒列的 -100），收到的請求記在 seen。
func splitSession(scores map[string]float64, seen *[]rerank.TextsRequest) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct{ w, r string }{
		{"我", "ㄨㄛˇ"}, {"你", "ㄋㄧˇ"}, {"央", "ㄧㄤ"}, {"允", "ㄩㄣˇ"}, {"分", "ㄈㄣ"}, {"嗯", "ㄣ"}, {"誰", "ㄕㄟˊ"}, {"音", "ㄧㄣ"},
		{"停", "ㄊㄧㄥˊ"}, {"營", "ㄧㄥˊ"},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, 1e-3)
	}
	s := New(personal.NewModel(lex, sys))
	var words []english.Word
	for _, w := range []string{"hello", "go", "zip", "up", "the", "thew", "a", "as"} {
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

func splitCommit(s *Session, keys string) string {
	mixedType(s, keys)
	return key(s, proto.KeyEnter).Commit
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
		if got := splitCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
}

// 修正切法扣 mixedRepairPenalty：su;;␣ 的 s央（合併冗餘的 ;）要比照字面高出 6 分以上才換；規則結果也是修正時不扣。
func TestMixedNeuralRepairPenalty(t *testing.T) {
	if got := splitCommit(splitSession(map[string]float64{"su;;": -10, "s央": -6}, nil), "su;; "); got != "su;; " {
		t.Errorf("差 4 分不換：送出 %q", got)
	}
	if got := splitCommit(splitSession(map[string]float64{"su;;": -10, "s央": -3}, nil), "su;; "); got != "s央" {
		t.Errorf("差 7 分要換：送出 %q", got)
	}
	// 規則結果本身不扣：zsuu3 的 z你（合併冗餘音、切在不是詞的 z 後面）贏照字面就留著。
	if got := splitCommit(splitSession(map[string]float64{"z你": -10, "zsuu3": -14}, nil), "zsuu3"); got != "z你" {
		t.Errorf("規則結果不扣分：送出 %q", got)
	}
	// 規則的 thew營 也是合併冗餘音：the停 不扣分，高一點就換。
	if got := splitCommit(splitSession(map[string]float64{"thew營": -10, "the停": -8}, nil), "thewuuu/6"); got != "the停" {
		t.Errorf("修正對修正：送出 %q", got)
	}
}

// 不問神經的：純中文（規則整串照順序組成一個音節，u;␣ 的 ; 是注音 ㄤ）、規則照字面的英文字典詞（zip␣ 不是 分）。
// 照順序就組得成同一個讀音時不送修正候選（asu3 有 a你，不送丟掉 a 的 你）。
func TestMixedNeuralRequests(t *testing.T) {
	for _, keys := range []string{"ji3su3", "u; ", "zip "} {
		var seen []rerank.TextsRequest
		if got := splitCommit(splitSession(map[string]float64{}, &seen), keys); len(seen) != 0 {
			t.Fatalf("%q（送出 %q）不該問神經：%+v", keys, got, seen)
		}
	}
	var seen []rerank.TextsRequest
	splitCommit(splitSession(map[string]float64{}, &seen), "asu3")
	if len(seen) != 1 || !slices.Contains(seen[0].Texts, "a你") || slices.Contains(seen[0].Texts, "你") {
		t.Fatalf("asu3 candidates: %+v", seen)
	}
	// 前文是組字區游標前的字。
	seen = nil
	splitCommit(splitSession(map[string]float64{}, &seen), "ji3helloji3")
	if len(seen) != 1 || seen[0].Context != "我" || !slices.Contains(seen[0].Texts, "hello我") {
		t.Fatalf("context: %+v", seen)
	}
}

// 評分器失敗或超過期限時照規則。
func TestMixedNeuralFallsBack(t *testing.T) {
	s := splitSession(nil, nil)
	s.Rerank = rerank.Options{Weight: 2, Texts: textsFunc(func(rerank.TextsRequest) ([]float64, error) { return nil, rerank.ErrDeadline })}
	if got := splitCommit(s, "helloji3"); got != "hello我" {
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
		if got := splitCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
}

// 回頭比最多 mixedRevisitWindow 次，換過就固定。
func TestMixedNeuralRevisitWindow(t *testing.T) {
	var seen []rerank.TextsRequest
	splitCommit(splitSession(map[string]float64{"允": 0, "mp3": -5}, &seen), "mp3ji3su3ji3su3ji3su3")
	if len(seen) != 1+mixedRevisitWindow {
		t.Fatalf("要 1 次決定＋%d 次回頭，問了 %d 次", mixedRevisitWindow, len(seen))
	}
	seen = nil
	splitCommit(splitSession(map[string]float64{"允": 0, "mp3": -5, "mp3我": 0}, &seen), "mp3ji3su3ji3")
	if len(seen) != 2 {
		t.Fatalf("換過就固定：問了 %d 次", len(seen))
	}
}
