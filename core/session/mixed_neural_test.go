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

// splitSession 的英文字典有 hello、go、zip、the；評分器照 scores 給分（沒列的 -100），收到的請求記在 seen。
func splitSession(scores map[string]float64, seen *[]rerank.TextsRequest) *Session {
	lex, sys := lexicon.NewMem(), lm.NewMem()
	for _, e := range []struct{ w, r string }{
		{"我", "ㄨㄛˇ"}, {"你", "ㄋㄧˇ"}, {"央", "ㄧㄤ"}, {"允", "ㄩㄣˇ"}, {"分", "ㄈㄣ"}, {"嗯", "ㄣ"}, {"誰", "ㄕㄟˊ"},
	} {
		lex.Add(lexicon.Entry{Surface: e.w, Reading: zhuyin.ParseReading(e.r), PRW: 1})
		sys.SetUni(e.w, 1e-3)
	}
	s := New(personal.NewModel(lex, sys))
	var words []english.Word
	for _, w := range []string{"hello", "go", "zip", "the"} {
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
		// 第一次（qji3）照字面留著；接著打完 su3 時切成兩個音節（規則是 qji3你）
		{"多個音節", "qji3su3", map[string]float64{"qji3": 0, "q我你": 0, "qji3你": -50}, "q我你"},
	} {
		if got := splitCommit(splitSession(tc.scores, nil), tc.keys); got != tc.want {
			t.Errorf("%s：%q 送出 %q，要 %q", tc.name, tc.keys, got, tc.want)
		}
	}
}

// 修正切法扣 mixedRepairPenalty：su;;␣ 的 s央（合併冗餘的 ;）要比照字面高出 6 分以上才換。
func TestMixedNeuralRepairPenalty(t *testing.T) {
	if got := splitCommit(splitSession(map[string]float64{"su;;": -10, "s央": -6}, nil), "su;; "); got != "su;; " {
		t.Errorf("差 4 分不換：送出 %q", got)
	}
	if got := splitCommit(splitSession(map[string]float64{"su;;": -10, "s央": -3}, nil), "su;; "); got != "s央" {
		t.Errorf("差 7 分要換：送出 %q", got)
	}
}

// 純中文（只有小寫字母、規則整串組成一個音節）不問神經；英文字典的詞不送修正候選（zip␣ 不是 分）。
func TestMixedNeuralRequests(t *testing.T) {
	var seen []rerank.TextsRequest
	splitCommit(splitSession(map[string]float64{}, &seen), "ji3su3")
	if len(seen) != 0 {
		t.Fatalf("pure Chinese asked the scorer: %+v", seen)
	}
	seen = nil
	if got := splitCommit(splitSession(map[string]float64{"zip": 0}, &seen), "zip "); got != "zip " || len(seen) != 1 {
		t.Fatalf("zip␣ 送出 %q，請求 %+v", got, seen)
	}
	if texts := seen[0].Texts; !slices.Contains(texts, "zip") || slices.Contains(texts, "分") {
		t.Fatalf("zip␣ candidates: %v", texts)
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
